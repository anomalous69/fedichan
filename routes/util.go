package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/KushBlazingJudah/fedichan/activitypub"
	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/db"
	"github.com/KushBlazingJudah/fedichan/util"
	"github.com/gofiber/fiber/v2"
	fhtml "github.com/gofiber/template/html"
)

func themeCookie(c *fiber.Ctx) string {
	return c.Cookies("theme")
}

func GetActorPost(ctx *fiber.Ctx, path string) error {
	obj := activitypub.ObjectBase{Id: config.Domain + path}
	collection, err := obj.GetCollectionFromPath()

	if err != nil {
		ctx.Status(404)
		return util.WrapError(err)
	}

	if len(collection.OrderedItems) > 0 {
		enc, err := json.MarshalIndent(collection, "", "\t")
		if err != nil {
			return util.WrapError(err)
		}

		ctx.Response().Header.Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
		_, err = ctx.Write(enc)
		return util.WrapError(err)
	}

	return nil
}

func NewPost(actor activitypub.Actor, nObj *activitypub.ObjectBase) error {
	nObj.Actor = config.Domain + "/" + actor.Name

	if locked, _ := nObj.InReplyTo[0].IsLocked(); locked {
		return errors.New("locked thread")
	}

	_nObj, err := nObj.Write()
	if err != nil {
		return util.WrapError(err)
	}
	*nObj = _nObj

	if len(nObj.To) == 0 {
		if err := actor.ArchivePosts(); err != nil {
			return util.WrapError(err)
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

		if err := activity.Send(); err != nil {
			config.Log.Printf("ParseOutboxRequest MakeRequestInbox: %s", err)
		}
	}(*nObj)

	go func(obj activitypub.ObjectBase) {
		err := obj.SendEmailNotify()

		if err != nil {
			config.Log.Println(err)
		}
	}(*nObj)

	return nil
}

func TemplateFunctions(engine *fhtml.Engine) {
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

	engine.AddFunc("isAdmin", func(d *db.Acct) bool {
		return d != nil && d.Type == db.Admin
	})

	engine.AddFunc("proxy", util.MediaProxy)
	engine.AddFunc("shortURL", util.ShortURL)
	engine.AddFunc("parseAttachment", ParseAttachment)
	engine.AddFunc("parseContent", db.ParseContent)
	engine.AddFunc("shortImg", util.ShortImg)
	engine.AddFunc("convertSize", util.ConvertSize)
	engine.AddFunc("isOnion", util.IsOnion)

	engine.AddFunc("parseReplyLink", func(actorId string, op string, id string, content string) string {
		actor, _ := activitypub.FingerActor(actorId)
		title := html.EscapeString(db.ParseLinkTitle(actor.Id+"/", op, content))
		return fmt.Sprintf(`<a href="/%s/%s#%s" title="%s" class="replyLink">&gt;&gt;%s</a>`, actor.Name, util.ShortURL(actor.Outbox, op), util.ShortURL(actor.Outbox, id), title, util.ShortURL(actor.Outbox, id))
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

func ObjectFromForm(ctx *fiber.Ctx, obj activitypub.ObjectBase) (activitypub.ObjectBase, error) {
	acct, _ := ctx.Locals("acct").(*db.Acct)

	var err error
	var file multipart.File

	header, _ := ctx.FormFile("file")

	if header != nil {
		file, _ = header.Open()
	}

	if file != nil {
		defer file.Close()
		var tempFile = new(os.File)

		obj.Attachment, tempFile, err = activitypub.CreateAttachmentObject(file, header)

		if err != nil {
			return obj, util.WrapError(err)
		}

		defer tempFile.Close()

		fileBytes, _ := io.ReadAll(file)
		tempFile.Write(fileBytes)

		re := regexp.MustCompile(`image/(jpe?g|png|webp)`)
		if re.MatchString(obj.Attachment[0].MediaType) {
			fileLoc := strings.ReplaceAll(obj.Attachment[0].Href, config.Domain, "")

			cmd := exec.Command("exiv2", "rm", "."+fileLoc)

			if err := cmd.Run(); err != nil {
				return obj, util.WrapError(err)
			}
		}

		obj.Preview = obj.Attachment[0].CreatePreview()
	}

	name, tripcode, _ := db.CreateNameTripCode(ctx.FormValue("name"), acct)

	obj.AttributedTo = util.EscapeString(name)
	obj.TripCode = util.EscapeString(tripcode)
	obj.Name = util.EscapeString(ctx.FormValue("subject"))
	obj.Content = util.EscapeString(ctx.FormValue("comment"))
	obj.Sensitive = (ctx.FormValue("sensitive") != "")
	obj = ParseOptions(ctx, obj)

	var originalPost activitypub.ObjectBase

	originalPost.Id = util.EscapeString(ctx.FormValue("inReplyTo"))
	obj.InReplyTo = append(obj.InReplyTo, originalPost)

	var activity activitypub.Activity

	if !util.IsInStringArray(activity.To, originalPost.Id) {
		activity.To = append(activity.To, originalPost.Id)
	}

	if originalPost.Id != "" {
		if local, _ := activity.IsLocal(); !local {
			actor, err := activitypub.FingerActor(originalPost.Id)
			if err == nil { // Keep things moving if it fails
				if !util.IsInStringArray(obj.To, actor.Id) {
					obj.To = append(obj.To, actor.Id)
				}
			}
		} else if err != nil {
			return obj, util.WrapError(err)
		}
	}

	replyingTo, err := db.ParseCommentForReplies(ctx.FormValue("comment"), originalPost.Id)

	if err != nil {
		return obj, util.WrapError(err)
	}

	for _, e := range replyingTo {
		has := false

		for _, f := range obj.InReplyTo {
			if e.Id == f.Id {
				has = true
				break
			}
		}

		if !has {
			obj.InReplyTo = append(obj.InReplyTo, e)

			var activity activitypub.Activity

			activity.To = append(activity.To, e.Id)

			if local, _ := activity.IsLocal(); !local {
				actor, err := activitypub.FingerActor(e.Id)
				if err != nil {
					return obj, util.WrapError(err)
				}

				if !util.IsInStringArray(obj.To, actor.Id) {
					obj.To = append(obj.To, actor.Id)
				}
			}
		}
	}

	return obj, nil
}

func ParseAttachment(obj activitypub.ObjectBase, catalog bool) template.HTML {
	if len(obj.Attachment) < 1 {
		return ""
	}

	if regexp.MustCompile(`image\/`).MatchString(obj.Attachment[0].MediaType) {
		src := obj.Preview.Href
		if src == "" {
			src = obj.Attachment[0].Href
		}
		src = util.MediaProxy(src)

		return template.HTML(fmt.Sprintf(`<img class="media" main="1" enlarge="0" attachment="%s" src="%s" preview="%s">`, obj.Attachment[0].Href, src, src))
	} else if regexp.MustCompile(`audio\/`).MatchString(obj.Attachment[0].MediaType) {
		return template.HTML(fmt.Sprintf(`<audio class="media" controls preload="metadata"><source src="%s" type="%s">Audio is not supported.</audio>`, util.MediaProxy(obj.Attachment[0].Href), obj.Attachment[0].MediaType))
	} else if regexp.MustCompile(`video\/`).MatchString(obj.Attachment[0].MediaType) {
		return template.HTML(fmt.Sprintf(`<video class="media" controls muted preload="metadata"><source src="%s" type="%s">Audio is not supported.</video>`, util.MediaProxy(obj.Attachment[0].Href), obj.Attachment[0].MediaType))
	}

	return ""
}

func ParseOptions(ctx *fiber.Ctx, obj activitypub.ObjectBase) activitypub.ObjectBase {
	options := util.EscapeString(ctx.FormValue("options"))

	if options != "" {
		option := strings.Split(options, ";")
		email := regexp.MustCompile(`.+@.+\..+`)
		delete := regexp.MustCompile("delete:.+")

		for _, e := range option {
			if e == "noko" {
				obj.Option = append(obj.Option, "noko")
			} else if e == "sage" {
				obj.Option = append(obj.Option, "sage")
			} else if e == "nokosage" {
				obj.Option = append(obj.Option, "nokosage")
			} else if email.MatchString(e) {
				obj.Option = append(obj.Option, "email:"+e)
			} else if delete.MatchString(e) {
				obj.Option = append(obj.Option, e)
			}
		}
	}

	return obj
}
