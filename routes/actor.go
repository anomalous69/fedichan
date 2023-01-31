package routes

import (
	"encoding/json"
	"io"
	"log"
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
	actor, _ := activitypub.GetActorFromDB(config.Domain + "/" + ctx.Params("actor"))
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
		return ctx.SendStatus(400)
	}

	switch activity.Type {
	case "Accept":
		if activity.Object.Object.Type == "Follow" {
			if err := activity.SetActorFollowing(); err != nil {
				return util.WrapError(err)
			}
		} else {
			return ctx.SendStatus(400)
		}
	case "Create":
		if err := actor.ProcessInboxCreate(activity); err != nil {
			return util.WrapError(err)
		}

		if err := actor.SendToFollowers(activity); err != nil {
			return util.WrapError(err)
		}
	case "Delete":
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

	case "Follow":
		for _, e := range activity.To {
			if _, err := activitypub.GetActorFromDB(e); err == nil {
				response := activity.AcceptFollow(actor)
				response, err := response.SetActorFollower()

				if err != nil {
					return util.WrapError(err)
				}

				if err := response.Send(); err != nil {
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

					if err := followActivity.Send(); err != nil {
						return util.WrapError(err)
					}
				}
			} else if err != nil {
				return util.WrapError(err)
			} else {
				log.Println("follow request for rejected")
				response := activity.Reject()
				return response.Send()
			}
		}
	case "Reject":
		if activity.Object.Object.Type == "Follow" {
			log.Println("follow rejected")
			if err := activity.SetActorFollowing(); err != nil {
				return util.WrapError(err)
			}
		}
	}
	return nil
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
	actor, err := activitypub.GetActorFromDB(config.Domain + "/" + ctx.FormValue("boardName"))
	if err != nil {
		return send404(ctx)
	}

	_, reg := ctx.Locals("acct").(*db.Acct)

	// Waive captcha for authenticated users, otherwise complain
	// Do this as early as possible to prevent wasting time
	if !reg {
		valid, err := db.CheckCaptcha(ctx.FormValue("captchaCode"), ctx.FormValue("captcha"))
		if err != nil {
			// Silently log it
			log.Printf("CheckCaptcha error: %v", err)
		}

		if !valid { // Always false when err != nil
			return send403(ctx, "Incorrect captcha.")
		}
	}

	header, err := ctx.FormFile("file")

	if ctx.FormValue("inReplyTo") == "" && header == nil {
		return send400(ctx, "Media is required for new threads.")
	}

	var file multipart.File
	if header != nil && err == nil {
		file, err = header.Open()
		if err != nil {
			return err
		}
		defer file.Close()

		// TODO: Do this according to the set limit
		if header.Size > (7 << 20) {
			return send400(ctx, "Max file size is 7 MB.")
		} else if isBanned, err := db.IsMediaBanned(file); err == nil && isBanned {
			return send400(ctx, "Media is banned.")
		}

		contentType, _ := util.GetFileContentType(file)
		if !util.SupportedMIMEType(contentType) {
			return send400(ctx, "Unsupported file type.")
		}

		file.Seek(0, io.SeekStart)
	} else {
		// No file attached or couldn't load it
		// Disallow blank posting
		if strings.TrimSpace(ctx.FormValue("comment")) == "" {
			return send400(ctx, "Comment required.")
		}
	}

	// Sanity check values
	if len(ctx.FormValue("comment")) > 4500 {
		return send400(ctx, "Comment limit is 4500 characters.")
	} else if len(ctx.FormValue("subject")) > 100 || len(ctx.FormValue("name")) > 100 || len(ctx.FormValue("options")) > 100 {
		return send400(ctx, "Name, subject, or options limit is 100 characters.")
	} else if strings.Count(ctx.FormValue("comment"), "\n") > 50 {
		return send400(ctx, "Your post has too many lines.")
	} else if is, _ := util.IsPostBlacklist(ctx.FormValue("comment")); is {
		return send400(ctx, "Your post was blocked.")
	}

	nObj, err := objectFromForm(ctx, activitypub.CreateObject("Note"))
	if err != nil {
		return util.WrapError(err)
	}

	if err := newPost(actor, &nObj); err != nil {
		return err
	}

	var id string
	op := len(nObj.InReplyTo) - 1
	if op >= 0 {
		if nObj.InReplyTo[op].Id == "" {
			id = nObj.Id
		} else {
			id = nObj.InReplyTo[0].Id + "|" + nObj.Id
		}
	}

	var obj activitypub.ObjectBase

	obj.Option = parseOptions(ctx)

	for _, e := range obj.Option {
		if e == "noko" || e == "nokosage" {
			return ctx.Redirect("/"+ctx.FormValue("boardName")+"/"+util.ShortURL(actor.Outbox, id), 301)
		}
	}

	if ctx.FormValue("returnTo") == "catalog" {
		return ctx.Redirect("/"+ctx.FormValue("boardName")+"/catalog", 301)
	} else {
		return ctx.Redirect("/"+ctx.FormValue("boardName"), 301)
	}

	return ctx.Redirect("/"+ctx.FormValue("boardName"), 301)
}

func ActorPost(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	actor, err := activitypub.GetActorByNameFromDB(ctx.Params("actor"))

	if err != nil {
		return nil
	}

	// this is a activitpub json request return json instead of html page
	if activitypub.AcceptActivity(ctx.Get("Accept")) {
		return getActorPost(ctx)
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
		return send404(ctx)
	}

	var data pageData

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

	if err := populateCaptcha(hasAuth, &data.Board); err != nil {
		return util.WrapError(err)
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

	var data pageData
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

	if err := populateCaptcha(hasAuth, &data.Board); err != nil {
		return util.WrapError(err)
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
		return send404(ctx)
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

	var data pageData
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

	if err := populateCaptcha(hasAuth, &data.Board); err != nil {
		return util.WrapError(err)
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

	var returnData pageData
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

	/*
		if err := populateCaptcha(hasAuth, &returnData.Board); err != nil {
			return util.WrapError(err)
		}
	*/

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
