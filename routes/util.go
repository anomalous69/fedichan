package routes

import (
	"encoding/json"
	"fmt"
	"html/template"
	"regexp"
	"strings"
	"time"

	"github.com/KushBlazingJudah/fedichan/activitypub"
	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/db"
	"github.com/KushBlazingJudah/fedichan/util"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html"
)

func GetThemeCookie(c *fiber.Ctx) string {
	return c.Cookies("theme")
}

func GetActorPost(ctx *fiber.Ctx, path string) error {
	obj := activitypub.ObjectBase{Id: config.Domain + path}
	collection, err := obj.GetCollectionFromPath()

	if err != nil {
		ctx.Status(404)
		return util.MakeError(err, "GetActorPost")
	}

	if len(collection.OrderedItems) > 0 {
		enc, err := json.MarshalIndent(collection, "", "\t")
		if err != nil {
			return util.MakeError(err, "GetActorPost")
		}

		ctx.Response().Header.Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
		_, err = ctx.Write(enc)
		return util.MakeError(err, "GetActorPost")
	}

	return nil
}

func ParseOutboxRequest(ctx *fiber.Ctx, actor activitypub.Actor) error {
	pw, _ := util.GetPasswordFromSession(ctx)
	needCaptcha := pw == ""
	contentType := util.GetContentType(ctx.Get("content-type"))

	if contentType == "multipart/form-data" || contentType == "application/x-www-form-urlencoded" {
		hasCaptcha, err := util.BoardHasAuthType(actor.Name, "captcha")
		if err != nil {
			return util.MakeError(err, "ParseOutboxRequest")
		}

		valid, err := db.CheckCaptcha(ctx.FormValue("captcha"))
		if err != nil {
			return util.MakeError(err, "ParseOutboxRequest")
		}

		if !needCaptcha || (hasCaptcha && valid) {
			header, _ := ctx.FormFile("file")
			if header != nil {
				f, _ := header.Open()
				defer f.Close()
				if header.Size > (7 << 20) {
					_, err := ctx.Status(403).Write([]byte("7MB max file size"))
					return util.MakeError(err, "ParseOutboxRequest")
				} else if isBanned, err := db.IsMediaBanned(f); err == nil && isBanned {
					config.Log.Println("media banned")
					_, err := ctx.Status(403).Write([]byte(""))
					return util.MakeError(err, "ParseOutboxRequest")
				}

				contentType, _ := util.GetFileContentType(f)

				if !util.SupportedMIMEType(contentType) {
					_, err := ctx.Status(403).Write([]byte("file type not supported"))
					return util.MakeError(err, "ParseOutboxRequest")
				}
			}

			nObj, err := db.ObjectFromForm(ctx, activitypub.CreateObject("Note"))
			if err != nil {
				return util.MakeError(err, "ParseOutboxRequest")
			}

			nObj.Actor = config.Domain + "/" + actor.Name

			if locked, _ := nObj.InReplyTo[0].IsLocked(); locked {
				ctx.Response().Header.SetStatusCode(403)
				_, err := ctx.Write([]byte("thread is locked"))
				return util.MakeError(err, "ParseOutboxRequest")
			}

			nObj, err = nObj.Write()
			if err != nil {
				return util.MakeError(err, "ParseOutboxRequest")
			}

			if len(nObj.To) == 0 {
				if err := actor.ArchivePosts(); err != nil {
					return util.MakeError(err, "ParseOutboxRequest")
				}
			}

			go func(nObj activitypub.ObjectBase) {
				activity, err := nObj.CreateActivity("Create")
				if err != nil {
					config.Log.Printf("ParseOutboxRequest Create Activity: %s", err)
				}

				activity, err = activity.AddFollowersTo()
				if err != nil {
					config.Log.Printf("ParseOutboxRequest Add FollowersTo: %s", err)
				}

				if err := activity.MakeRequestInbox(); err != nil {
					config.Log.Printf("ParseOutboxRequest MakeRequestInbox: %s", err)
				}
			}(nObj)

			go func(obj activitypub.ObjectBase) {
				err := obj.SendEmailNotify()

				if err != nil {
					config.Log.Println(err)
				}
			}(nObj)

			var id string
			op := len(nObj.InReplyTo) - 1
			if op >= 0 {
				if nObj.InReplyTo[op].Id == "" {
					id = nObj.Id
				} else {
					id = nObj.InReplyTo[0].Id + "|" + nObj.Id
				}
			}

			_, err = ctx.Status(200).Write([]byte(id))
			return util.MakeError(err, "ParseOutboxRequest")
		}

		_, err = ctx.Status(403).Write([]byte("captcha could not auth"))
		return util.MakeError(err, "")
	} else { // json request
		activity, err := activitypub.GetActivityFromJson(ctx)
		if err != nil {
			return util.MakeError(err, "ParseOutboxRequest")
		}

		if res, _ := activity.IsLocal(); res {
			if res := activity.Actor.VerifyHeaderSignature(ctx); err == nil && !res {
				_, err = ctx.Status(403).Write([]byte(""))
				return util.MakeError(err, "ParseOutboxRequest")
			}

			switch activity.Type {
			case "Create":
				_, err = ctx.Status(403).Write([]byte(""))

			case "Follow":
				validActor := (activity.Object.Actor != "")
				validLocalActor := (activity.Actor.Id == actor.Id)

				var rActivity activitypub.Activity

				if validActor && validLocalActor {
					rActivity = activity.AcceptFollow()
					rActivity, err = rActivity.SetActorFollowing()

					if err != nil {
						return util.MakeError(err, "ParseOutboxRequest")
					}

					if err := activity.MakeRequestInbox(); err != nil {
						return util.MakeError(err, "ParseOutboxRequest")
					}
				}

				actor, _ := activitypub.GetActorFromDB(config.Domain)
				activitypub.FollowingBoards, err = actor.GetFollowing()

				if err != nil {
					return util.MakeError(err, "ParseOutboxRequest")
				}

				activitypub.Boards, err = activitypub.GetBoardCollection()

				if err != nil {
					return util.MakeError(err, "ParseOutboxRequest")
				}

			case "Delete":
				config.Log.Println("This is a delete")
				_, err = ctx.Status(403).Write([]byte("could not process activity"))
			case "Note":
				_, err = ctx.Status(403).Write([]byte("could not process activity"))

			case "New":
				name := activity.Object.Alias
				prefname := activity.Object.Name
				summary := activity.Object.Summary
				restricted := activity.Object.Sensitive

				actor, err := db.CreateNewBoard(*activitypub.CreateNewActor(name, prefname, summary, config.AuthReq, restricted))
				if err != nil {
					return util.MakeError(err, "ParseOutboxRequest")
				}

				if actor.Id != "" {
					var board []activitypub.ObjectBase
					var item activitypub.ObjectBase
					var removed bool = false

					item.Id = actor.Id
					for _, e := range activitypub.FollowingBoards {
						if e.Id != item.Id {
							board = append(board, e)
						} else {
							removed = true
						}
					}

					if !removed {
						board = append(board, item)
					}

					activitypub.FollowingBoards = board
					activitypub.Boards, err = activitypub.GetBoardCollection()
					return util.MakeError(err, "ParseOutboxRequest")
				}

				_, err = ctx.Status(403).Write([]byte(""))

			default:
				_, err = ctx.Status(403).Write([]byte("could not process activity"))
			}

			if err != nil {
				return util.MakeError(err, "ParseOutboxRequest")
			}
		} else if err != nil {
			return util.MakeError(err, "ParseOutboxRequest")
		} else {
			config.Log.Println("is NOT activity")
			_, err = ctx.Status(403).Write([]byte("could not process activity"))
			return util.MakeError(err, "ParseOutboxRequest")
		}
	}

	return nil
}

func TemplateFunctions(engine *html.Engine) {
	engine.AddFunc("mod", func(i, j int) bool {
		return i%j == 0
	})

	engine.AddFunc("sub", func(i, j int) int {
		return i - j
	})

	engine.AddFunc("add", func(i, j int) int {
		return i + j
	})

	engine.AddFunc("unixtoreadable", func(u int) string {
		return time.Unix(int64(u), 0).Format("Jan 02, 2006")
	})

	engine.AddFunc("timeToReadableLong", func(t time.Time) string {
		return t.Format("01/02/06(Mon)15:04:05")
	})

	engine.AddFunc("timeToUnix", func(t time.Time) string {
		return fmt.Sprint(t.Unix())
	})

	engine.AddFunc("proxy", util.MediaProxy)

	// previously short
	engine.AddFunc("shortURL", util.ShortURL)

	engine.AddFunc("parseAttachment", db.ParseAttachment)

	engine.AddFunc("parseContent", db.ParseContent)

	engine.AddFunc("shortImg", util.ShortImg)

	engine.AddFunc("convertSize", util.ConvertSize)

	engine.AddFunc("isOnion", util.IsOnion)

	engine.AddFunc("parseReplyLink", func(actorId string, op string, id string, content string) template.HTML {
		actor, _ := activitypub.FingerActor(actorId)
		title := strings.ReplaceAll(db.ParseLinkTitle(actor.Id+"/", op, content), `/\&lt;`, ">")
		link := "<a href=\"/" + actor.Name + "/" + util.ShortURL(actor.Outbox, op) + "#" + util.ShortURL(actor.Outbox, id) + "\" title=\"" + title + "\" class=\"replyLink\">&gt;&gt;" + util.ShortURL(actor.Outbox, id) + "</a>"
		return template.HTML(link)
	})

	engine.AddFunc("shortExcerpt", func(post activitypub.ObjectBase) template.HTML {
		var returnString string

		if post.Name != "" {
			returnString = post.Name + "| " + post.Content
		} else {
			returnString = post.Content
		}

		re := regexp.MustCompile(`(^(.|\r\n|\n){100})`)

		match := re.FindStringSubmatch(returnString)

		if len(match) > 0 {
			returnString = match[0] + "..."
		}

		returnString = strings.ReplaceAll(returnString, "<", "&lt;")
		returnString = strings.ReplaceAll(returnString, ">", "&gt;")

		re = regexp.MustCompile(`(^.+\|)`)

		match = re.FindStringSubmatch(returnString)

		if len(match) > 0 {
			returnString = strings.Replace(returnString, match[0], "<b>"+match[0]+"</b>", 1)
			returnString = strings.Replace(returnString, "|", ":", 1)
		}

		return template.HTML(returnString)
	})

	engine.AddFunc("parseLinkTitle", func(board string, op string, content string) string {
		nContent := db.ParseLinkTitle(board, op, content)
		nContent = strings.ReplaceAll(nContent, `/\&lt;`, ">")

		return nContent
	})

	engine.AddFunc("parseLink", func(board activitypub.Actor, link string) string {
		var obj = activitypub.ObjectBase{
			Id: link,
		}

		var OP string
		if OP, _ = obj.GetOP(); OP == obj.Id {
			return board.Name + "/" + util.ShortURL(board.Outbox, obj.Id)
		}

		return board.Name + "/" + util.ShortURL(board.Outbox, OP) + "#" + util.ShortURL(board.Outbox, link)
	})

	engine.AddFunc("showArchive", func(actor activitypub.Actor) bool {
		col, err := actor.GetCollectionTypeLimit("Archive", 1)

		if err != nil || len(col.OrderedItems) == 0 {
			return false
		}

		return true
	})
}
