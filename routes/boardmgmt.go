package routes

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/KushBlazingJudah/fedichan/activitypub"
	"github.com/KushBlazingJudah/fedichan/config"

	"github.com/KushBlazingJudah/fedichan/db"
	"github.com/KushBlazingJudah/fedichan/util"
	"github.com/gofiber/fiber/v2"
)

func BoardBanMedia(ctx *fiber.Ctx) error {
	_, hasAuth := ctx.Locals("acct").(*db.Acct)

	var err error

	postID := ctx.Query("id")
	board := ctx.Query("board")

	if postID == "" || !hasAuth {
		err = errors.New("missing postID or auth")
		return util.WrapError(err)
	}

	var col activitypub.Collection
	activity := activitypub.Activity{Id: postID}

	if col, err = activity.GetCollection(); err != nil {
		return util.WrapError(err)
	}

	if len(col.OrderedItems) == 0 {
		err = errors.New("no collection")
		return util.WrapError(err)
	}

	if len(col.OrderedItems[0].Attachment) == 0 {
		err = errors.New("no attachment")
		return util.WrapError(err)
	}

	var actor activitypub.Actor
	actor.Id = col.OrderedItems[0].Actor

	if !hasAuth {
		err = errors.New("actor does not have auth")
		return util.WrapError(err)
	}

	re := regexp.MustCompile(config.Domain)
	file := re.ReplaceAllString(col.OrderedItems[0].Attachment[0].Href, "")

	f, err := os.Open("." + file)

	if err != nil {
		return util.WrapError(err)
	}

	defer f.Close()

	bytes := make([]byte, 2048)

	if _, err = f.Read(bytes); err != nil {
		return util.WrapError(err)
	}

	if banned, err := db.IsMediaBanned(f); err == nil && !banned {
		query := `insert into bannedmedia (hash) values ($1)`
		if _, err := config.DB.Exec(query, util.HashBytes(bytes)); err != nil {
			return util.WrapError(err)
		}
	}

	var isOP bool
	var local bool
	var obj activitypub.ObjectBase
	obj.Id = postID
	obj.Actor = actor.Id

	if isOP, _ = obj.CheckIfOP(); !isOP {
		if err := obj.Tombstone(); err != nil {
			return util.WrapError(err)
		}
	} else {
		if err := obj.TombstoneReplies(); err != nil {
			return util.WrapError(err)
		}
	}

	if local, _ = obj.IsLocal(); local {
		if err := obj.DeleteRequest(); err != nil {
			return util.WrapError(err)
		}
	}

	if err := actor.UnArchiveLast(); err != nil {
		return util.WrapError(err)
	}

	var OP string
	if len(col.OrderedItems[0].InReplyTo) > 0 {
		OP = col.OrderedItems[0].InReplyTo[0].Id
	}

	if !isOP {
		if !local && OP != "" {
			return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
		} else if OP != "" {
			return ctx.Redirect(OP, http.StatusSeeOther)
		}
	}

	return ctx.Redirect("/"+board, http.StatusSeeOther)
}

func BoardDelete(ctx *fiber.Ctx) error {
	_, hasAuth := ctx.Locals("acct").(*db.Acct)

	var err error

	postID := ctx.Query("id")
	board := ctx.Query("board")

	if postID == "" || !hasAuth {
		err = errors.New("missing postID or auth")
		return util.WrapError(err)
	}

	var col activitypub.Collection
	activity := activitypub.Activity{Id: postID}

	if col, err = activity.GetCollection(); err != nil {
		return util.WrapError(err)
	}

	var OP string
	var actor activitypub.Actor

	if len(col.OrderedItems) == 0 {
		actor, err = activitypub.GetActorByNameFromDB(board)

		if err != nil {
			return util.WrapError(err)
		}
	} else {
		if len(col.OrderedItems[0].InReplyTo) > 0 {
			OP = col.OrderedItems[0].InReplyTo[0].Id
		} else {
			OP = postID
		}

		actor.Id = col.OrderedItems[0].Actor
	}

	if !hasAuth {
		err = errors.New("actor does not have auth")
		return util.WrapError(err)
	}

	var isOP bool
	obj := activitypub.ObjectBase{Id: postID}

	if isOP, _ = obj.CheckIfOP(); !isOP {
		if err := obj.Tombstone(); err != nil {
			return util.WrapError(err)
		}
	} else {
		if err := obj.TombstoneReplies(); err != nil {
			return util.WrapError(err)
		}
	}

	var local bool

	if local, _ = obj.IsLocal(); local {
		if err := obj.DeleteRequest(); err != nil {
			return util.WrapError(err)
		}
	}

	if err := actor.UnArchiveLast(); err != nil {
		return util.WrapError(err)
	}

	if ctx.Query("manage") == "t" {
		return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
	}

	if !isOP {
		if !local && OP != "" {
			return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
		} else if OP != "" {
			return ctx.Redirect(OP, http.StatusSeeOther)
		}
	}

	return ctx.Redirect("/"+board, http.StatusSeeOther)
}

func BoardDeleteAttach(ctx *fiber.Ctx) error {
	_, hasAuth := ctx.Locals("acct").(*db.Acct)

	var err error

	postID := ctx.Query("id")
	board := ctx.Query("board")

	if postID == "" || !hasAuth {
		err = errors.New("missing postID or auth")
		return util.WrapError(err)
	}

	var col activitypub.Collection
	activity := activitypub.Activity{Id: postID}

	if col, err = activity.GetCollection(); err != nil {
		return util.WrapError(err)
	}

	var OP string
	var actor activitypub.Actor

	if len(col.OrderedItems) == 0 {
		actor, err = activitypub.GetActorByNameFromDB(board)

		if err != nil {
			return util.WrapError(err)
		}
	} else {
		if len(col.OrderedItems[0].InReplyTo) > 0 {
			OP = col.OrderedItems[0].InReplyTo[0].Id
		} else {
			OP = postID
		}

		actor.Id = col.OrderedItems[0].Actor
	}

	obj := activitypub.ObjectBase{Id: postID}

	if err := obj.DeleteAttachmentFromFile(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.TombstoneAttachment(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.DeletePreviewFromFile(); err != nil {
		return util.WrapError(err)
	}

	if err := obj.TombstonePreview(); err != nil {
		return util.WrapError(err)
	}

	if ctx.Query("manage") == "t" {
		return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
	} else if local, _ := obj.IsLocal(); !local && OP != "" {
		return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
	} else if OP != "" {
		return ctx.Redirect(OP, http.StatusSeeOther)
	}

	return ctx.Redirect("/"+board, http.StatusSeeOther)
}

func BoardMarkSensitive(ctx *fiber.Ctx) error {
	_, hasAuth := ctx.Locals("acct").(*db.Acct)

	var err error

	postID := ctx.Query("id")
	board := ctx.Query("board")

	if postID == "" || !hasAuth {
		err = errors.New("missing postID or auth")
		return util.WrapError(err)
	}

	var col activitypub.Collection
	activity := activitypub.Activity{Id: postID}

	if col, err = activity.GetCollection(); err != nil {
		return util.WrapError(err)
	}

	var OP string
	var actor activitypub.Actor

	if len(col.OrderedItems) == 0 {
		actor, err = activitypub.GetActorByNameFromDB(board)

		if err != nil {
			return util.WrapError(err)
		}
	} else {
		if len(col.OrderedItems[0].InReplyTo) > 0 {
			OP = col.OrderedItems[0].InReplyTo[0].Id
		} else {
			OP = postID
		}

		actor.Id = col.OrderedItems[0].Actor
	}

	obj := activitypub.ObjectBase{Id: postID}

	if err = obj.MarkSensitive(true); err != nil {
		return util.WrapError(err)
	}

	if isOP, _ := obj.CheckIfOP(); !isOP && OP != "" {
		if local, _ := obj.IsLocal(); !local {
			return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
		}

		return ctx.Redirect(OP, http.StatusSeeOther)
	}

	return ctx.Redirect("/"+board, http.StatusSeeOther)
}

// TODO routes/BoardRemove
func BoardRemove(ctx *fiber.Ctx) error {
	return ctx.SendString("board remove")
}

// TODO routes/BoardAddToIndex
func BoardAddToIndex(ctx *fiber.Ctx) error {
	return ctx.SendString("board add to index")
}

func BoardPopArchive(ctx *fiber.Ctx) error {
	_, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth {
		return send404(ctx)
	}

	id := ctx.Query("id")
	board := ctx.Query("board")

	var obj = activitypub.ObjectBase{Id: id}

	if err := obj.SetRepliesType("Note"); err != nil {
		return util.WrapError(err)
	}

	return ctx.Redirect("/"+board+"/archive", http.StatusSeeOther)
}

func BoardAutoSubscribe(ctx *fiber.Ctx) error {
	_, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth {
		return send404(ctx)
	}

	actor, err := activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return util.WrapError(err)
	}

	board := ctx.Query("board")

	if actor, err = activitypub.GetActorByNameFromDB(board); err != nil {
		return util.WrapError(err)
	}

	if err := actor.SetAutoSubscribe(); err != nil {
		return util.WrapError(err)
	}

	if autoSub, _ := actor.GetAutoSubscribe(); autoSub {
		if err := actor.AutoFollow(); err != nil {
			return util.WrapError(err)
		}
	}

	return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
}

func BoardBlacklist(ctx *fiber.Ctx) error {
	_, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth {
		return send404(ctx)
	}

	if ctx.Method() == "GET" {
		if id := ctx.Query("remove"); id != "" {
			i, _ := strconv.Atoi(id)
			if err := util.DeleteRegexBlacklist(i); err != nil {
				return util.WrapError(err)
			}
		}
	} else {
		regex := ctx.FormValue("regex")
		testCase := ctx.FormValue("testCase")

		if regex == "" {
			return ctx.Redirect("/", http.StatusSeeOther)
		}

		re := regexp.MustCompile(regex)

		if testCase == "" {
			if err := util.WriteRegexBlacklist(regex); err != nil {
				return util.WrapError(err)
			}
		} else if re.MatchString(testCase) {
			if err := util.WriteRegexBlacklist(regex); err != nil {
				return util.WrapError(err)
			}
		}
	}

	return ctx.Redirect("/"+config.Key+"#regex", http.StatusSeeOther)
}

func ReportPost(ctx *fiber.Ctx) error {
	_, hasAuth := ctx.Locals("acct").(*db.Acct)

	id := ctx.FormValue("id")
	board := ctx.FormValue("board")
	reason := ctx.FormValue("comment")
	close := ctx.FormValue("close")

	var obj = activitypub.ObjectBase{Id: id}

	if close == "1" {
		if !hasAuth {
			return send403(ctx)
		}

		if local, _ := obj.IsLocal(); !local {
			if err := db.CloseLocalReport(obj.Id, board); err != nil {
				return send500(ctx, err)
			}

			return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
		}

		if err := obj.DeleteReported(); err != nil {
			return send500(ctx, err)
		}

		return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
	}

	if len(reason) > 100 {
		return send400(ctx, "Report length may contain at most 100 characters.")
	}

	if len(strings.TrimSpace(reason)) == 0 {
		return send400(ctx, "Report reason required.")
	}

	if !hasAuth {
		if ok, _ := db.CheckCaptcha(ctx.FormValue("captchaCode"), ctx.FormValue("captcha")); !ok && close != "1" {
			return send403(ctx, "Invalid captcha.")
		}
	}

	if err := db.CreateLocalReport(obj.Id, board, reason); err != nil {
		return send500(ctx, err)
	}

	go func() {
		if setup := config.IsEmailSetup(); !setup {
			return
		}

		rows, err := config.DB.Query(`select email from accounts where type >= 1 and email is not null`)
		if err != nil {
			log.Println(util.WrapError(err))
			return
		}
		defer rows.Close()

		to := []string{}

		for rows.Next() {
			v := ""
			if err := rows.Scan(&v); err != nil {
				log.Println(util.WrapError(err))
				return
			}

			to = append(to, v)
		}

		msg := strings.Join([]string{
			fmt.Sprintf("From: %s", config.SiteEmailFrom),
			fmt.Sprintf("To: %s", strings.Join(to, ", ")),
			fmt.Sprintf("Subject: New report: %s", obj.Id),
			"",
			reason,
		}, "\n")

		err = smtp.SendMail(config.SiteEmailSMTP,
			smtp.PlainAuth("", config.SiteEmailUser, config.SiteEmailPassword, config.SiteEmailServer),
			config.SiteEmailFrom, to, []byte(msg))

		if err != nil {
			log.Println(util.WrapError(err))
		}
	}()

	return ctx.Redirect(id, http.StatusSeeOther)
}

func ReportGet(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	actor, _ := activitypub.GetActor(ctx.Query("actor"))

	var data pageData
	var err error

	data.Board.Actor = actor
	data.Board.Name = actor.Name
	data.Board.PrefName = actor.PreferredUsername
	data.Board.Summary = actor.Summary
	data.Board.InReplyTo = ctx.Query("post")
	data.Board.To = actor.Outbox
	data.Board.Restricted = actor.Restricted
	data.Acct = acct

	if err := populateCaptcha(hasAuth, &data.Board); err != nil {
		return util.WrapError(err)
	}

	data.Meta.Description = data.Board.Summary
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Instance, err = activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return err
	}

	data.Themes = config.Themes
	data.ThemeCookie = themeCookie(ctx)

	data.Key = config.Key
	data.Board.Domain = config.Domain
	data.Boards = activitypub.Boards

	return ctx.Render("report", data, "layouts/main")
}

func Sticky(ctx *fiber.Ctx) error {
	_, hasAuth := ctx.Locals("acct").(*db.Acct)

	id := ctx.Query("id")
	board := ctx.Query("board")

	actor, _ := activitypub.GetActorByNameFromDB(board)

	if id == "" || !hasAuth {
		return send403(ctx)
	}

	var obj = activitypub.ObjectBase{Id: id}
	col, _ := obj.GetCollectionFromPath()

	if len(col.OrderedItems) < 1 {
		obj.MarkSticky(actor.Id)

		return ctx.Redirect("/"+board, http.StatusSeeOther)
	}

	actor.Id = col.OrderedItems[0].Actor

	var OP string
	if len(col.OrderedItems[0].InReplyTo) > 0 && col.OrderedItems[0].InReplyTo[0].Id != "" {
		OP = col.OrderedItems[0].InReplyTo[0].Id
	} else {
		OP = id
	}

	obj.MarkSticky(actor.Id)

	var op = activitypub.ObjectBase{Id: OP}
	if local, _ := op.IsLocal(); !local {
		return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
	} else {
		return ctx.Redirect(OP, http.StatusSeeOther)
	}
}

func Lock(ctx *fiber.Ctx) error {
	_, hasAuth := ctx.Locals("acct").(*db.Acct)

	id := ctx.Query("id")
	board := ctx.Query("board")

	actor, _ := activitypub.GetActorByNameFromDB(board)

	if id == "" || !hasAuth {
		return send403(ctx)
	}

	var obj = activitypub.ObjectBase{Id: id}
	col, _ := obj.GetCollectionFromPath()

	if len(col.OrderedItems) < 1 {
		obj.MarkLocked(actor.Id)

		return ctx.Redirect("/"+board, http.StatusSeeOther)
	}

	actor.Id = col.OrderedItems[0].Actor

	var OP string
	if len(col.OrderedItems[0].InReplyTo) > 0 && col.OrderedItems[0].InReplyTo[0].Id != "" {
		OP = col.OrderedItems[0].InReplyTo[0].Id
	} else {
		OP = id
	}

	obj.MarkLocked(actor.Id)

	var op = activitypub.ObjectBase{Id: OP}
	if local, _ := op.IsLocal(); !local {
		return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
	} else {
		return ctx.Redirect(OP, http.StatusSeeOther)
	}
}
