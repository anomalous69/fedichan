package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/KushBlazingJudah/fedichan/activitypub"
	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/db"
	"github.com/KushBlazingJudah/fedichan/util"
	"github.com/gofiber/fiber/v2"
)

func ActorInbox(ctx *fiber.Ctx) error {
	activity, err := activitypub.GetActivityFromJson(ctx)

	if err != nil {
		return util.WrapError(err)
	}

	if activity.Actor.PublicKey.Id == "" {
		nActor, err := activitypub.FingerActor(activity.Actor.Id)
		if err != nil {
			return util.WrapError(err)
		}

		activity.Actor = &nActor
	}

	if !activity.Actor.VerifyHeaderSignature(ctx) {
		response := activity.Reject()
		return response.MakeRequestInbox()
	}

	switch activity.Type {
	case "Create":
		for _, e := range activity.To {
			actor := activitypub.Actor{Id: e}
			if err := actor.ProcessInboxCreate(activity); err != nil {
				return util.WrapError(err)
			}

			if err := actor.SendToFollowers(activity); err != nil {
				return util.WrapError(err)
			}
		}

		for _, e := range activity.Cc {
			actor := activitypub.Actor{Id: e}
			if err := actor.ProcessInboxCreate(activity); err != nil {
				return util.WrapError(err)
			}
		}
	case "Delete":
		for _, e := range activity.To {
			actor, err := activitypub.GetActorFromDB(e)
			if err != nil {
				continue // try again
				// return util.WrapError(err)
			}

			if actor.Id != "" && actor.Id != config.Domain {
				if activity.Object.Replies != nil {
					for _, k := range activity.Object.Replies.OrderedItems {
						if err := k.Tombstone(); err != nil {
							return util.WrapError(err)
						}
					}
				}

				if err := activity.Object.Tombstone(); err != nil {
					return util.WrapError(err)
				}

				if err := actor.UnArchiveLast(); err != nil {
					return util.WrapError(err)
				}
				break
			}
		}

	case "Follow":
		for _, e := range activity.To {
			if _, err := activitypub.GetActorFromDB(e); err == nil {
				response := activity.AcceptFollow()
				response, err := response.SetActorFollower()

				if err != nil {
					return util.WrapError(err)
				}

				if err := response.MakeRequestInbox(); err != nil {
					return util.WrapError(err)
				}

				alreadyFollowing, err := response.Actor.IsAlreadyFollowing(response.Object.Id)

				if err != nil {
					return util.WrapError(err)
				}

				objActor, err := activitypub.FingerActor(response.Object.Actor)

				if err != nil || objActor.Id == "" {
					return util.WrapError(err)
				}

				reqActivity := activitypub.Activity{Id: objActor.Following}
				remoteActorFollowingCol, err := reqActivity.GetCollection()

				if err != nil {
					return util.WrapError(err)
				}

				alreadyFollow := false

				for _, e := range remoteActorFollowingCol.Items {
					if e.Id == response.Actor.Id {
						alreadyFollowing = true
					}
				}

				autoSub, err := response.Actor.GetAutoSubscribe()

				if err != nil {
					return util.WrapError(err)
				}

				if autoSub && !alreadyFollow && alreadyFollowing {
					followActivity, err := response.Actor.MakeFollowActivity(response.Object.Actor)

					if err != nil {
						return util.WrapError(err)
					}

					if err := followActivity.MakeRequestOutbox(); err != nil {
						return util.WrapError(err)
					}
				}
			} else if err != nil {
				return util.WrapError(err)
			} else {
				config.Log.Println("follow request for rejected")
				response := activity.Reject()
				return response.MakeRequestInbox()
			}
		}
	case "Reject":
		if activity.Object.Object.Type == "Follow" {
			config.Log.Println("follow rejected")
			if _, err := activity.SetActorFollowing(); err != nil {
				return util.WrapError(err)
			}
		}
	}
	return nil
}

func PostActorOutbox(ctx *fiber.Ctx) error {
	//var activity activitypub.Activity
	actor, err := activitypub.GetActorFromPath(ctx.Path(), "/")
	if err != nil {
		return util.WrapError(err)
	}

	if activitypub.AcceptActivity(ctx.Get("Accept")) {
		return actor.GetOutbox(ctx)
	}

	return ParseOutboxRequest(ctx, actor)
}

func ActorFollowing(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain + "/" + ctx.Params("actor"))
	return actor.GetFollowingResp(ctx)
}

func ActorFollowers(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain + "/" + ctx.Params("actor"))
	return actor.GetFollowersResp(ctx)
}

func MakeActorPost(ctx *fiber.Ctx) error {
	acct, reg := ctx.Locals("acct").(*db.Acct)

	header, _ := ctx.FormFile("file")

	if ctx.FormValue("inReplyTo") == "" && header == nil {
		return ctx.Render("403", fiber.Map{
			"message": "Media is required for new posts",
		})
	}

	var file multipart.File

	if header != nil {
		file, _ = header.Open()
	}

	if file != nil && header.Size > (7<<20) {
		return ctx.Render("403", fiber.Map{
			"message": "7MB max file size",
		})
	}

	if is, _ := util.IsPostBlacklist(ctx.FormValue("comment")); is {
		config.Log.Println("Blacklist post blocked")
		return ctx.Redirect("/", 301)
	}

	if ctx.FormValue("inReplyTo") == "" || file == nil {
		if strings.TrimSpace(ctx.FormValue("comment")) == "" && ctx.FormValue("subject") == "" {
			return ctx.Render("403", fiber.Map{
				"message": "Comment or Subject required",
			})
		}
	}

	if len(ctx.FormValue("comment")) > 4500 {
		return ctx.Render("403", fiber.Map{
			"message": "Comment limit 4500 characters",
		})
	}

	if strings.Count(ctx.FormValue("comment"), "\r\n") > 50 || strings.Count(ctx.FormValue("comment"), "\n") > 50 || strings.Count(ctx.FormValue("comment"), "\r") > 50 {
		return ctx.Render("403", fiber.Map{
			"message": "Too many new lines - try again.",
		})
	}

	if len(ctx.FormValue("subject")) > 100 || len(ctx.FormValue("name")) > 100 || len(ctx.FormValue("options")) > 100 {
		return ctx.Render("403", fiber.Map{
			"message": "Name, Subject or Options limit 100 characters",
		})
	}

	// Waive captcha for authenticated users, otherwise complain
	if !reg && ctx.FormValue("captcha") == "" {
		return ctx.Render("403", fiber.Map{
			"message": "Incorrect Captcha",
		})
	}

	b := bytes.Buffer{}
	we := multipart.NewWriter(&b)

	if file != nil {
		var fw io.Writer

		fw, err := we.CreateFormFile("file", header.Filename)

		if err != nil {
			return util.WrapError(err)
		}
		_, err = io.Copy(fw, file)

		if err != nil {
			return util.WrapError(err)
		}
	}

	reply, _ := db.ParseCommentForReply(ctx.FormValue("comment"))

	form, _ := ctx.MultipartForm()

	if form.Value == nil {
		return util.WrapError(errors.New("form value nil"))
	}

	for key, r0 := range form.Value {
		if key == "captcha" {
			err := we.WriteField(key, ctx.FormValue("captchaCode")+":"+ctx.FormValue("captcha"))
			if err != nil {
				return util.WrapError(err)
			}
		} else if key == "name" {
			name, tripcode, _ := db.CreateNameTripCode(ctx.FormValue("name"), acct)

			err := we.WriteField(key, name)
			if err != nil {
				return util.WrapError(err)
			}

			err = we.WriteField("tripcode", tripcode)
			if err != nil {
				return util.WrapError(err)
			}
		} else {
			err := we.WriteField(key, r0[0])
			if err != nil {
				return util.WrapError(err)
			}
		}
	}

	if (ctx.FormValue("inReplyTo") == "" || ctx.FormValue("inReplyTo") == "NaN") && reply != "" {
		err := we.WriteField("inReplyTo", reply)
		if err != nil {
			return util.WrapError(err)
		}
	}

	we.Close()

	// TODO: Remove sendTo

	sendTo := ctx.FormValue("sendTo")

	req, err := http.NewRequest("POST", sendTo, &b)
	if err != nil {
		return util.WrapError(err)
	}

	req.Header.Set("Content-Type", we.FormDataContentType())
	if c := ctx.Cookies("session"); c != "" {
		// This is a hack to pass through the token while we still make
		// requests to the outbox
		req.Header.Set("Authorization", "Bearer "+c)
	}

	resp, err := util.RouteProxy(req)
	if err != nil {
		return util.WrapError(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return util.WrapError(err)
	}

	if resp.StatusCode == 200 {
		var obj activitypub.ObjectBase

		obj = ParseOptions(ctx, obj)
		for _, e := range obj.Option {
			if e == "noko" || e == "nokosage" {
				return ctx.Redirect(config.Domain+"/"+ctx.FormValue("boardName")+"/"+util.ShortURL(ctx.FormValue("sendTo"), string(body)), 301)
			}
		}

		if ctx.FormValue("returnTo") == "catalog" {
			return ctx.Redirect(config.Domain+"/"+ctx.FormValue("boardName")+"/catalog", 301)
		} else {
			return ctx.Redirect(config.Domain+"/"+ctx.FormValue("boardName"), 301)
		}
	}

	if resp.StatusCode == 403 {
		return ctx.Render("403", fiber.Map{
			"message": string(body),
		})
	}

	return ctx.Redirect(config.Domain+"/"+ctx.FormValue("boardName"), 301)
}

func ActorPost(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	actor, err := activitypub.GetActorByNameFromDB(ctx.Params("actor"))

	if err != nil {
		return nil
	}

	// this is a activitpub json request return json instead of html page
	if activitypub.AcceptActivity(ctx.Get("Accept")) {
		GetActorPost(ctx, ctx.Path())
		return nil
	}

	re := regexp.MustCompile(`\w+$`)
	postId := re.FindString(ctx.Path())

	inReplyTo, _ := db.GetPostIDFromNum(postId)

	// check if actually OP if not redirect to op to get full thread
	var obj = activitypub.ObjectBase{Id: inReplyTo}
	if OP, _ := obj.GetOP(); OP != obj.Id {
		return ctx.Redirect(config.Domain+"/"+actor.Name+"/"+util.ShortURL(actor.Outbox, OP)+"#"+util.ShortURL(actor.Outbox, inReplyTo), http.StatusMovedPermanently)
	}

	collection, err := obj.GetCollectionFromPath()

	if err != nil {
		return ctx.Status(404).Render("404", nil)
	}

	var data PageData

	if collection.Actor.Id != "" {
		data.Board.Post.Actor = collection.Actor.Id
		data.Board.InReplyTo = inReplyTo
	}

	if len(collection.OrderedItems) > 0 {
		data.Posts = append(data.Posts, collection.OrderedItems[0])
	}

	data.Board.Name = actor.Name
	data.Board.PrefName = actor.PreferredUsername
	data.Board.To = actor.Outbox
	data.Board.Actor = actor
	data.Board.Summary = actor.Summary
	data.Board.Domain = config.Domain
	data.Board.Restricted = actor.Restricted
	data.Acct = acct
	data.ReturnTo = "feed"
	data.PostType = "reply"

	if len(data.Posts) > 0 {
		data.PostId = util.ShortURL(data.Board.To, data.Posts[0].Id)
	}

	// Ignore captcha if we're authenticated
	if !hasAuth {
		capt, err := util.GetRandomCaptcha()
		if err != nil {
			return util.WrapError(err)
		}
		data.Board.Captcha = config.Domain + "/" + capt
		data.Board.CaptchaCode, _ = util.GetCaptchaCode(data.Board.Captcha)
	}

	data.Instance, err = activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return util.WrapError(err)
	}

	data.Key = config.Key
	data.Boards = activitypub.Boards

	data.Title = "/" + data.Board.Name + "/ - " + data.PostId

	if len(data.Posts) > 0 {
		data.Meta.Description = data.Posts[0].Content
		data.Meta.Url = data.Posts[0].Id
		data.Meta.Title = data.Posts[0].Name
		if data.Posts[0].Preview != nil {
			data.Meta.Preview = data.Posts[0].Preview.Href
		}
	}

	data.Themes = config.Themes
	data.ThemeCookie = themeCookie(ctx)

	return ctx.Render("npost", data, "layouts/main")
}

func ActorCatalog(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	actorName := ctx.Params("actor")
	actor, err := activitypub.GetActorByNameFromDB(actorName)

	if err != nil {
		return util.WrapError(err)
	}

	collection, err := actor.GetCatalogCollection()

	if err != nil {
		return util.WrapError(err)
	}

	var data PageData
	data.Board.Name = actor.Name
	data.Board.PrefName = actor.PreferredUsername
	data.Board.InReplyTo = ""
	data.Board.To = actor.Outbox
	data.Board.Actor = actor
	data.Board.Summary = actor.Summary
	data.Board.Domain = config.Domain
	data.Board.Restricted = actor.Restricted
	data.Acct = acct
	data.Key = config.Key
	data.ReturnTo = "catalog"
	data.PostType = "new"

	data.Board.Post.Actor = actor.Id

	data.Instance, err = activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return util.WrapError(err)
	}

	// Ignore captcha if we're authenticated
	if !hasAuth {
		capt, err := util.GetRandomCaptcha()
		if err != nil {
			return util.WrapError(err)
		}
		data.Board.Captcha = config.Domain + "/" + capt
		data.Board.CaptchaCode, _ = util.GetCaptchaCode(data.Board.Captcha)
	}

	data.Title = "/" + data.Board.Name + "/ - catalog"

	data.Boards = activitypub.Boards
	data.Posts = collection.OrderedItems

	data.Meta.Description = data.Board.Summary
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Themes = config.Themes
	data.ThemeCookie = themeCookie(ctx)

	return ctx.Render("catalog", data, "layouts/main")
}

func ActorPosts(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	actor, err := activitypub.GetActorByNameFromDB(ctx.Params("actor"))

	if err != nil {
		return ctx.Status(404).Render("404", nil)
	}

	if activitypub.AcceptActivity(ctx.Get("Accept")) {
		actor.GetInfoResp(ctx)
		return nil
	}

	var page int
	if postNum := ctx.Query("page"); postNum != "" {
		if page, err = strconv.Atoi(postNum); err != nil {
			return util.WrapError(err)
		}
	}

	collection, err := actor.WantToServePage(page)
	if err != nil {
		return util.WrapError(err)
	}

	var offset = 15
	var pages []int
	pageLimit := (float64(collection.TotalItems) / float64(offset))

	if pageLimit > 11 {
		pageLimit = 11
	}

	for i := 0.0; i < pageLimit; i++ {
		pages = append(pages, int(i))
	}

	var data PageData
	data.Board.Name = actor.Name
	data.Board.PrefName = actor.PreferredUsername
	data.Board.Summary = actor.Summary
	data.Board.InReplyTo = ""
	data.Board.To = actor.Outbox
	data.Board.Actor = actor
	data.Board.Domain = config.Domain
	data.Board.Restricted = actor.Restricted
	data.Acct = acct
	data.CurrentPage = page
	data.ReturnTo = "feed"
	data.PostType = "new"

	data.Board.Post.Actor = actor.Id

	// Ignore captcha if we're authenticated
	if !hasAuth {
		capt, err := util.GetRandomCaptcha()
		if err != nil {
			return util.WrapError(err)
		}
		data.Board.Captcha = config.Domain + "/" + capt
		data.Board.CaptchaCode, _ = util.GetCaptchaCode(data.Board.Captcha)
	}

	data.Title = "/" + actor.Name + "/ - " + actor.PreferredUsername

	data.Key = config.Key

	data.Boards = activitypub.Boards
	data.Posts = collection.OrderedItems

	data.Pages = pages
	data.TotalPage = len(data.Pages) - 1

	data.Meta.Description = data.Board.Summary
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Themes = config.Themes
	data.ThemeCookie = themeCookie(ctx)

	return ctx.Render("nposts", data, "layouts/main")
}

func ActorArchive(ctx *fiber.Ctx) error {
	acct, _ := ctx.Locals("acct").(*db.Acct)
	actorName := ctx.Params("actor")
	actor, err := activitypub.GetActorByNameFromDB(actorName)

	if err != nil {
		return util.WrapError(err)
	}

	collection, err := actor.GetCollectionType("Archive")

	if err != nil {
		return util.WrapError(err)
	}

	var returnData PageData
	returnData.Board.Name = actor.Name
	returnData.Board.PrefName = actor.PreferredUsername
	returnData.Board.InReplyTo = ""
	returnData.Board.To = actor.Outbox
	returnData.Board.Actor = actor
	returnData.Board.Summary = actor.Summary
	returnData.Board.Domain = config.Domain
	returnData.Board.Restricted = actor.Restricted
	returnData.Acct = acct
	returnData.Key = config.Key
	returnData.ReturnTo = "archive"

	returnData.Board.Post.Actor = actor.Id

	returnData.Instance, err = activitypub.GetActorFromDB(config.Domain)

	capt, err := util.GetRandomCaptcha()
	if err != nil {
		return util.WrapError(err)
	}
	returnData.Board.Captcha = config.Domain + "/" + capt
	returnData.Board.CaptchaCode, _ = util.GetCaptchaCode(returnData.Board.Captcha)

	returnData.Title = "/" + actor.Name + "/ - " + actor.PreferredUsername

	returnData.Boards = activitypub.Boards

	returnData.Posts = collection.OrderedItems

	returnData.Meta.Description = returnData.Board.Summary
	returnData.Meta.Url = returnData.Board.Actor.Id
	returnData.Meta.Title = returnData.Title

	returnData.Themes = config.Themes
	returnData.ThemeCookie = themeCookie(ctx)

	return ctx.Render("archive", returnData, "layouts/main")
}

func GetActorOutbox(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromPath(ctx.Path(), "/")

	collection, _ := actor.GetCollection()
	collection.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	collection.Actor = &actor

	collection.TotalItems, _ = actor.GetPostTotal()
	collection.TotalImgs, _ = actor.GetImgTotal()

	enc, _ := json.Marshal(collection)

	ctx.Response().Header.Add("Content-Type", config.ActivityStreams)
	_, err := ctx.Write(enc)

	return err
}
