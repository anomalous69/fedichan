package routes

import (
	"html/template"
	"net/http"
	"strconv"

	"github.com/KushBlazingJudah/fedichan/activitypub"
	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/db"
	"github.com/KushBlazingJudah/fedichan/util"
	"github.com/gofiber/fiber/v2"
)

func NewsGet(ctx *fiber.Ctx) error {
	acct, _ := ctx.Locals("acct").(*db.Acct)

	timestamp := ctx.Path()[6:]
	ts, err := strconv.Atoi(timestamp)

	if err != nil {
		return ctx.Status(404).Render("404", nil)
	}

	actor, err := activitypub.GetActorFromDB(config.Domain)

	if err != nil {
		return util.WrapError(err)
	}

	var data PageData
	data.PreferredUsername = actor.PreferredUsername
	data.Boards = activitypub.Boards
	data.Board.Name = ""
	data.Key = config.Key
	data.Board.Domain = config.Domain
	data.Board.Actor = actor
	data.Board.Post.Actor = actor.Id
	data.Board.Restricted = actor.Restricted
	data.NewsItems = make([]db.NewsItem, 1)
	data.Acct = acct

	data.NewsItems[0], err = db.GetNewsItem(ts)
	if err != nil {
		return util.WrapError(err)
	}

	data.Title = actor.PreferredUsername + ": " + data.NewsItems[0].Title

	data.Meta.Description = data.PreferredUsername + " is a federated image board based on ActivityPub. The current version of the code running on the server is still a work-in-progress product, expect a bumpy ride for the time being. Get the server code here: https://git.fchannel.org."
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Themes = config.Themes
	data.ThemeCookie = themeCookie(ctx)

	return ctx.Render("news", data, "layouts/main")
}

func NewsGetAll(ctx *fiber.Ctx) error {
	acct, _ := ctx.Locals("acct").(*db.Acct)

	actor, err := activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return util.WrapError(err)
	}

	var data PageData
	data.PreferredUsername = actor.PreferredUsername
	data.Title = actor.PreferredUsername + " News"
	data.Boards = activitypub.Boards
	data.Board.Name = ""
	data.Key = config.Key
	data.Board.Domain = config.Domain
	data.Board.Actor = actor
	data.Board.Post.Actor = actor.Id
	data.Board.Restricted = actor.Restricted
	data.Acct = acct

	data.NewsItems, err = db.GetNews(0)

	if err != nil {
		return util.WrapError(err)
	}

	if len(data.NewsItems) == 0 {
		return ctx.Redirect("/", http.StatusSeeOther)
	}

	data.Meta.Description = data.PreferredUsername + " is a federated image board based on ActivityPub. The current version of the code running on the server is still a work-in-progress product, expect a bumpy ride for the time being. Get the server code here: https://git.fchannel.org."
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Themes = config.Themes
	data.ThemeCookie = themeCookie(ctx)

	return ctx.Render("anews", data, "layouts/main")
}

func NewsPost(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorFromDB(config.Domain)

	if err != nil {
		return util.WrapError(err)
	}

	if has := actor.HasValidation(ctx); !has {
		return nil
	}

	var newsitem db.NewsItem

	newsitem.Title = ctx.FormValue("title")
	newsitem.Content = template.HTML(ctx.FormValue("summary"))

	if err := db.WriteNews(newsitem); err != nil {
		return util.WrapError(err)
	}

	return ctx.Redirect("/", http.StatusSeeOther)
}

func NewsDelete(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorFromDB(config.Domain)

	if has := actor.HasValidation(ctx); !has {
		return nil
	}

	timestamp := ctx.Path()[13+len(config.Key):]

	tsint, err := strconv.Atoi(timestamp)

	if err != nil {
		return ctx.Status(404).Render("404", nil)
	}

	if err := db.DeleteNewsItem(tsint); err != nil {
		return util.WrapError(err)
	}

	return ctx.Redirect("/news/", http.StatusSeeOther)
}
