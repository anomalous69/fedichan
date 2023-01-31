package routes

import (
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/KushBlazingJudah/fedichan/activitypub"
	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/db"
	"github.com/KushBlazingJudah/fedichan/util"
	"github.com/gofiber/fiber/v2"
)

func AdminVerify(ctx *fiber.Ctx) error {
	user := ctx.FormValue("username")
	pass := ctx.FormValue("password")

	if !db.CheckPassword(user, pass) {
		return ctx.Status(400).SendString("invalid username or password")
	}

	a := db.Acct{Username: user}
	s, err := a.Session()
	if err != nil {
		return err
	}

	ctx.Cookie(&fiber.Cookie{
		Name:    "session",
		Value:   s,
		Expires: time.Now().UTC().Add(60 * 60 * 48 * time.Second),
	})

	return ctx.Redirect("/", http.StatusSeeOther)
}

func AdminIndex(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth {
		return ctx.Render("verify", common{
			Title:  "Login",
			Boards: activitypub.Boards,
			Acct:   nil,
			Key:    config.Key,
		}, "layouts/main")
	}

	actor, _ := activitypub.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")
	if actor.Id == "" {
		actor, _ = activitypub.GetActorByNameFromDB(config.Domain)
	}

	actor, err := activitypub.GetActor(config.Domain)

	if err != nil {
		return util.WrapError(err)
	}

	reqActivity := activitypub.Activity{Id: actor.Following}
	follow, _ := reqActivity.GetCollection()
	follower, _ := reqActivity.GetCollection()

	var following []string
	var followers []string

	for _, e := range follow.Items {
		following = append(following, e.Id)
	}

	for _, e := range follower.Items {
		followers = append(followers, e.Id)
	}

	var adminData adminPage
	adminData.Following = following
	adminData.Followers = followers

	var reported = make(map[string][]db.Reports)

	for _, e := range following {
		re := regexp.MustCompile(`.*/(.+)$`)
		boards := re.FindStringSubmatch(e)
		reports, _ := db.GetLocalReport(boards[1])

		for _, k := range reports {
			reported[k.Actor.Name] = append(reported[k.Actor.Name], k)
		}
	}

	for k, e := range reported {
		sort.Sort(db.ReportsSortDesc(e))
		reported[k] = e
	}

	adminData.Actor = actor.Id
	adminData.Key = config.Key
	adminData.Domain = config.Domain
	adminData.Acct = acct
	adminData.Title = actor.Name + " Admin page"

	adminData.Boards = activitypub.Boards

	adminData.Board.Post.Actor = actor.Id

	adminData.Instance, _ = activitypub.GetActorFromDB(config.Domain)

	adminData.PostBlacklist, _ = util.GetRegexBlacklist()
	adminData.Reports = reported

	adminData.Meta.Description = adminData.Title
	adminData.Meta.Url = adminData.Board.Actor.Id
	adminData.Meta.Title = adminData.Title

	adminData.Themes = config.Themes

	return ctx.Render("admin", adminData, "layouts/main")
}

func AdminFollow(ctx *fiber.Ctx) error {
	follow := ctx.FormValue("follow")
	actorId := ctx.FormValue("actor")

	actor := activitypub.Actor{Id: actorId}
	followActivity, _ := actor.MakeFollowActivity(follow)

	objActor := activitypub.Actor{Id: followActivity.Object.Actor}

	if isLocal, _ := objActor.IsLocal(); !isLocal && followActivity.Actor.Id == config.Domain {
		_, err := ctx.Write([]byte("main board can only follow local boards. Create a new board and then follow outside boards from it."))
		return util.WrapError(err)
	}

	if actor, _ := activitypub.FingerActor(follow); actor.Id != "" {
		if err := followActivity.Send(); err != nil {
			return util.WrapError(err)
		}
	}

	var redirect string
	actor, _ = activitypub.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	if actor.Name != "main" {
		redirect = actor.Name
	}

	time.Sleep(time.Duration(500) * time.Millisecond)

	return ctx.Redirect("/"+config.Key+"/"+redirect, http.StatusSeeOther)
}

func AdminAddBoard(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain)
	if hasValidation := actor.HasValidation(ctx); !hasValidation {
		return nil
	}

	var restrict bool
	if ctx.FormValue("restricted") == "True" {
		restrict = true
	} else {
		restrict = false
	}

	newBoard := *activitypub.CreateNewActor(ctx.FormValue("name"), ctx.FormValue("prefname"), ctx.FormValue("summary"), restrict)
	actor, err := db.CreateNewBoard(newBoard)
	if err != nil {
		return err
	}

	// Set main as following the new board
	ok := false

	// TODO: Replace with a generic Has function
	for _, e := range activitypub.FollowingBoards {
		if e.Id == newBoard.Id {
			ok = true
			break
		}
	}

	if !ok {
		activitypub.FollowingBoards = append(activitypub.FollowingBoards, activitypub.ObjectBase{Id: actor.Id})
	}

	activitypub.Boards, err = activitypub.GetBoardCollection()
	if err != nil {
		return err
	}

	return ctx.Redirect("/"+config.Key, http.StatusSeeOther)
}

func AdminActorIndex(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth {
		return ctx.Render("verify", fiber.Map{"key": config.Key})
	}

	var data adminPage

	actor, _ := activitypub.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	if actor.Id == "" {
		actor, _ = activitypub.GetActorByNameFromDB(config.Domain)
	}

	data.Acct = acct

	reqActivity := activitypub.Activity{Id: actor.Following}
	follow, _ := reqActivity.GetCollection()

	reqActivity.Id = actor.Followers
	follower, _ := reqActivity.GetCollection()

	var following []string
	var followers []string

	for _, e := range follow.Items {
		following = append(following, e.Id)
	}

	for _, e := range follower.Items {
		followers = append(followers, e.Id)
	}

	data.Following = following
	data.Followers = followers

	reports, _ := db.GetLocalReport(actor.Name)

	var reported = make(map[string][]db.Reports)
	for _, k := range reports {
		reported[k.Actor.Name] = append(reported[k.Actor.Name], k)
	}

	for k, e := range reported {
		sort.Sort(db.ReportsSortDesc(e))
		reported[k] = e
	}

	data.Domain = config.Domain
	data.IsLocal, _ = actor.IsLocal()
	data.Title = "Manage /" + actor.Name + "/"
	data.Boards = activitypub.Boards
	data.Board.Name = actor.Name
	data.Board.Actor = actor
	data.Key = config.Key
	data.Board.TP = config.TP

	data.Board.Post.Actor = actor.Id

	data.Instance, _ = activitypub.GetActorFromDB(config.Domain)

	data.AutoSubscribe, _ = actor.GetAutoSubscribe()

	/* TODO
	jannies, err := actor.GetJanitors()
	*/
	jannies := []db.Verify(nil)

	data.Meta.Description = data.Title
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Themes = config.Themes

	data.RecentPosts, _ = actor.GetRecentPosts()
	data.ThemeCookie = themeCookie(ctx)
	data.Reports = reported
	data.Jannies = jannies

	return ctx.Render("manage", data, "layouts/main")
}

func AdminAddJanny(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth || acct.Type != db.Admin {
		return send403(ctx)
	}

	actor, _ := activitypub.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")
	if actor.Id == "" {
		actor, _ = activitypub.GetActorByNameFromDB(config.Domain)
	}

	var verify db.Verify
	verify.Type = "janitor"
	verify.Identifier = actor.Id
	verify.Label = ctx.FormValue("label")

	/* TODO
	if err := actor.CreateVerification(verify); err != nil {
		return util.WrapError(err)
	}
	*/

	var redirect string
	actor, _ = activitypub.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	if actor.Name != "main" {
		redirect = actor.Name
	}

	return ctx.Redirect("/"+config.Key+"/"+redirect, http.StatusSeeOther)
}

func AdminEditSummary(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth || acct.Type != db.Admin {
		return send403(ctx)
	}

	actor, _ := activitypub.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")
	if actor.Id == "" {
		actor, _ = activitypub.GetActorByNameFromDB(config.Domain)
	}

	summary := ctx.FormValue("summary")

	query := `update actor set summary=$1 where id=$2`
	if _, err := config.DB.Exec(query, summary, actor.Id); err != nil {
		return util.WrapError(err)
	}

	var redirect string
	if actor.Name != "main" {
		redirect = actor.Name
	}

	return ctx.Redirect("/"+config.Key+"/"+redirect, http.StatusSeeOther)

}

func AdminDeleteJanny(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth || acct.Type != db.Admin {
		return send403(ctx)
	}

	actor, _ := activitypub.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")

	if actor.Id == "" {
		actor, _ = activitypub.GetActorByNameFromDB(config.Domain)
	}

	var verify db.Verify
	verify.Code = ctx.Query("code")

	/* TODO
	if err := actor.DeleteVerification(verify); err != nil {
		return util.WrapError(err)
	}
	*/

	var redirect string

	if actor.Name != "main" {
		redirect = actor.Name
	}

	return ctx.Redirect("/"+config.Key+"/"+redirect, http.StatusSeeOther)
}
