package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io"
	"log"
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

func populateCaptcha(hasAuth bool, b *activitypub.Board) error {
	if hasAuth {
		// No-op because laziness
		return nil
	}

	file, id, err := db.GetCaptcha()
	if err != nil {
		return util.WrapError(err)
	}

	b.Captcha = "/" + file
	b.CaptchaCode = id
	return nil
}

func getActorPost(ctx *fiber.Ctx) error {
	path := ctx.Path()
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

func newPost(actor activitypub.Actor, nObj *activitypub.ObjectBase) error {
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
			log.Printf("ParseOutboxRequest Create Activity: %s", err)
		}

		activity, err = activity.AddFollowersTo()
		if err != nil {
			log.Printf("ParseOutboxRequest Add FollowersTo: %s", err)
		}

		if err := activity.Send(); err != nil {
			log.Printf("ParseOutboxRequest MakeRequestInbox: %s", err)
		}
	}(*nObj)

	go func(obj activitypub.ObjectBase) {
		err := obj.SendEmailNotify()

		if err != nil {
			log.Println(err)
		}
	}(*nObj)

	return nil
}

func parseReplyLink(actorId string, op string, id string, content string) template.HTML {
	actor, _ := activitypub.FingerActor(actorId)
	title := html.EscapeString(db.ParseLinkTitle(actor.Id+"/", op, content))
	return template.HTML(fmt.Sprintf(`<a href="/%s/%s#%s" title="%s" class="replyLink">&gt;&gt;%s</a>`, actor.Name, util.ShortURL(actor.Outbox, op), util.ShortURL(actor.Outbox, id), title, util.ShortURL(actor.Outbox, id)))
}

func timeToReadableLong(t time.Time) string {
	// TODO: Not necessary.
	return t.Format("01/02/06(Mon)15:04:05")
}

func timeToUnix(t time.Time) string {
	// TODO: Not necessary.
	return fmt.Sprint(t.Unix())
}

func TemplateFunctions(engine *fhtml.Engine) {
	postTmpl := template.Must(template.New("").Funcs(template.FuncMap{
		"convertSize":        util.ConvertSize,
		"isOnion":            util.IsOnion,
		"parseAttachment":    parseAttachment,
		"parseContent":       db.ParseContent,
		"parseReplyLink":     parseReplyLink,
		"proxy":              util.MediaProxy,
		"shortImg":           util.ShortImg,
		"timeToReadableLong": timeToReadableLong,
		"timeToUnix":         timeToUnix,
		"shortURL":           util.ShortURL,
	}).ParseFiles("./views/partials/post.html"))

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

	engine.AddFunc("timeToReadableLong", timeToReadableLong)

	engine.AddFunc("timeToUnix", timeToUnix)

	engine.AddFunc("isAdmin", func(d *db.Acct) bool {
		return d != nil && d.Type == db.Admin
	})

	engine.AddFunc("proxy", util.MediaProxy)
	engine.AddFunc("shortURL", util.ShortURL)
	engine.AddFunc("parseAttachment", parseAttachment)
	engine.AddFunc("parseContent", db.ParseContent)
	engine.AddFunc("shortImg", util.ShortImg)
	engine.AddFunc("convertSize", util.ConvertSize)
	engine.AddFunc("isOnion", util.IsOnion)

	engine.AddFunc("parseReplyLink", parseReplyLink)

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

	engine.AddFunc("renderPost", func(p activitypub.ObjectBase, b activitypub.Board, t activitypub.ObjectBase, a *db.Acct, trunc bool) template.HTML {
		buf := &strings.Builder{}
		if err := postTmpl.ExecuteTemplate(buf, "post", struct {
			Board  activitypub.Board
			Acct   *db.Acct
			Thread activitypub.ObjectBase
			Post   activitypub.ObjectBase
			Trunc  bool
		}{b, a, t, p, trunc}); err != nil {
			// A panic is fine in this context
			panic(err)
		}

		return template.HTML(buf.String())
	})
}

func objectFromForm(ctx *fiber.Ctx, obj activitypub.ObjectBase) (activitypub.ObjectBase, error) {
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

		if _, err := io.Copy(tempFile, file); err != nil {
			return obj, util.WrapError(err)
		}

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

	obj.AttributedTo = name
	obj.TripCode = tripcode
	obj.Name = ctx.FormValue("subject")
	obj.Content = ctx.FormValue("comment")
	obj.Sensitive = (ctx.FormValue("sensitive") != "")
	obj.Option = parseOptions(ctx)

	var originalPost activitypub.ObjectBase

	originalPost.Id = html.EscapeString(ctx.FormValue("inReplyTo"))
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

	replyingTo, err := db.ParseCommentForReplies(obj.Content, originalPost.Id)

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

func parseAttachment(obj activitypub.ObjectBase, catalog bool) template.HTML {
	if len(obj.Attachment) < 1 {
		return ""
	}

	if regexp.MustCompile(`image\/`).MatchString(obj.Attachment[0].MediaType) {
		src := obj.Preview.Href
		if src == "" {
			src = obj.Attachment[0].Href
		}
		src = util.MediaProxy(src)

		return template.HTML(fmt.Sprintf(`<img class="media" enlarge="0" attachment="%s" src="%s" preview="%s">`, obj.Attachment[0].Href, src, src))
	} else if regexp.MustCompile(`audio\/`).MatchString(obj.Attachment[0].MediaType) {
		return template.HTML(fmt.Sprintf(`<audio class="media" controls preload="metadata"><source src="%s" type="%s">Audio is not supported.</audio>`, util.MediaProxy(obj.Attachment[0].Href), obj.Attachment[0].MediaType))
	} else if regexp.MustCompile(`video\/`).MatchString(obj.Attachment[0].MediaType) {
		return template.HTML(fmt.Sprintf(`<video class="media" controls muted preload="metadata"><source src="%s" type="%s">Audio is not supported.</video>`, util.MediaProxy(obj.Attachment[0].Href), obj.Attachment[0].MediaType))
	}

	return ""
}

func parseOptions(ctx *fiber.Ctx) []string {
	options := ctx.FormValue("options")
	var opts []string

	if options != "" {
		option := strings.Split(options, ";")
		email := regexp.MustCompile(`.+@.+\..+`)

		for _, e := range option {
			switch e {
			case "noko", "sage", "nokosage":
				opts = append(opts, e)
			default:
				if email.MatchString(e) {
					opts = append(opts, "email:"+e)
				}
			}
		}
	}

	return opts
}

func statusTemplate(num int) func(ctx *fiber.Ctx, msg ...string) error {
	n := fmt.Sprint(num)
	return func(ctx *fiber.Ctx, msg ...string) error {
		acct, _ := ctx.Locals("acct").(*db.Acct)

		var m string
		if msg != nil {
			m = msg[0]
		}

		return ctx.Status(num).Render(n, errorData{
			common: common{
				Title:  "Error",
				Acct:   acct,
				Boards: activitypub.Boards,
				Key:    config.Key,
			},
			Message: m,
		}, "layouts/main")
	}
}

func send500(ctx *fiber.Ctx, err error, msg ...string) error {
	acct, _ := ctx.Locals("acct").(*db.Acct)

	var m string
	if msg != nil {
		m = msg[0]
	}

	// The results of this call do not matter to us
	ctx.Status(500).Render("500", errorData{
		common: common{
			Title:  "Error",
			Acct:   acct,
			Boards: activitypub.Boards,
			Key:    config.Key,
		},
		Message: m,
		Error:   err,
	}, "layouts/main")

	return err
}

var send400 = statusTemplate(400)
var send403 = statusTemplate(403)
var send404 = statusTemplate(404)
