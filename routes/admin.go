package routes

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/KushBlazingJudah/fedichan/activitypub"
	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/db"
	"github.com/KushBlazingJudah/fedichan/util"
	"github.com/gofiber/fiber/v2"
)

func sendLogin(ctx *fiber.Ctx) error {
	return ctx.Render("verify", common{
		Title:  "Login",
		Boards: activitypub.Boards,
		Acct:   nil,
		Key:    config.Key,
	}, "layouts/main")
}

func AdminVerify(ctx *fiber.Ctx) error {
	user := ctx.FormValue("username")
	pass := ctx.FormValue("password")

	if !db.CheckPassword(user, pass) {
		return send403(ctx, "Invalid username or password.")
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
		return sendLogin(ctx)
	}

	actor, _ := activitypub.GetActorFromPath(ctx.Path(), "/"+config.Key+"/")
	if actor.Id == "" {
		actor, _ = activitypub.GetActorByNameFromDB(config.Domain)
	}

	actor, err := activitypub.GetActor(config.Domain)
	if err != nil {
		return util.WrapError(err)
	}

	users, err := db.Users()
	if err != nil {
		return util.WrapError(err)
	}

	// TODO: should probably pool this.
	var reported = make(map[string][]db.Reports)

	for k, e := range reported {
		sort.Sort(db.ReportsSortDesc(e))
		reported[k] = e
	}

	var adminData adminPage

	adminData.Actor = actor.Id
	adminData.Key = config.Key
	adminData.Domain = config.Domain
	adminData.Acct = acct
	adminData.Title = actor.Name + " Admin page"

	adminData.Users = users

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
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth {
		return sendLogin(ctx)
	}

	if acct.Type < db.Mod {
		return send403(ctx, "Only moderators and admins can manage board relationships.")
	}

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
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth {
		return sendLogin(ctx)
	}

	if acct.Type < db.Admin {
		return send403(ctx, "Only admins can create boards.")
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

func AdminSetBlotter(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth {
		return sendLogin(ctx)
	}

	if acct.Type < db.Admin {
		return send403(ctx, "Only admins can set the blotter.")
	}

	actor, err := activitypub.GetActorByNameFromDB(ctx.FormValue("board", "main"))
	if err != nil {
		return send404(ctx, "Board not found")
	}

	if err := actor.SetBlotter(ctx.FormValue("blotter")); err != nil {
		return send500(ctx, err)
	}

	return ctx.Redirect("/"+config.Key+"/"+ctx.FormValue("board", ""), http.StatusSeeOther)
}

func AdminActorIndex(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth {
		return sendLogin(ctx)
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

	data.Meta.Description = data.Title
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Themes = config.Themes

	data.RecentPosts, _ = actor.GetRecentPosts()
	data.ThemeCookie = themeCookie(ctx)
	data.Reports = reported

	return ctx.Render("manage", data, "layouts/main")
}

func AdminEditSummary(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth || acct.Type < db.Admin {
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

func AdminAddUser(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth || acct.Type < db.Admin {
		return send403(ctx)
	}

	frm := struct {
		Username, Email, Password, Type string
	}{}

	if err := ctx.BodyParser(&frm); err != nil {
		return send400(ctx, "Invalid form contents.")
	} else if frm.Username == "" || frm.Password == "" || frm.Type == "" {
		return send400(ctx, "Must specify username, password, and type.")
	}

	// Create user
	user := db.Acct{
		Username: frm.Username,
		Email:    frm.Email,
	}

	switch frm.Type {
	case "janitor":
		user.Type = db.Janitor
	case "mod":
		user.Type = db.Mod
	case "admin":
		user.Type = db.Admin
	default:
		return send400(ctx, "Invalid user type")
	}

	if err := user.Save(); err != nil {
		return send500(ctx, fmt.Errorf("failed to save user: %v", err))
	} else if err = user.SetPassword(frm.Password); err != nil {
		return send500(ctx, fmt.Errorf("failed to set user password: %v", err))
	}

	return ctx.RedirectBack("/" + config.Key)
}

func AdminDeleteUser(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth || acct.Type < db.Admin {
		return send403(ctx)
	}

	panic("not implemented")
}

func AdminChangePasswd(ctx *fiber.Ctx) error {
	acct, hasAuth := ctx.Locals("acct").(*db.Acct)
	if !hasAuth {
		return send403(ctx)
	}

	// Admin can change the password and type for any user, including other
	// admins.
	// All others can only update their password.
	// If Username is not specified, it defaults to the logged in user.
	//
	// Contrary to this route's name, the e-mail may be updated here too.
	// Simply don't supply a password.

	frm := struct {
		Email, Password, Type string
	}{}

	tgt := ctx.Query("user", acct.Username)

	if acct.Type < db.Admin && tgt != acct.Username {
		return send403(ctx, "Insufficient privileges to update other users' details.")
	}

	// Fetch user details so we don't overwrite all of them with nil values
	// This shadows target but the value is thrown away
	target, err := db.User(tgt)
	if err != nil {
		if errors.Is(err, db.ErrInvalid) {
			return send404(ctx, "User does not exist.")
		}

		return send500(ctx, err)
	}

	if ctx.Method() == "GET" {
		var adminData adminPage

		adminData.Key = config.Key
		adminData.Domain = config.Domain
		adminData.Acct = acct
		adminData.Title = "Change Password"

		adminData.User = &target

		adminData.Boards = activitypub.Boards

		adminData.Instance, _ = activitypub.GetActorFromDB(config.Domain)

		adminData.Themes = config.Themes

		return ctx.Render("chpasswd", adminData, "layouts/main")
	}

	if err := ctx.BodyParser(&frm); err != nil {
		return send400(ctx, "Invalid form contents.")
	} else if frm.Password == "" && frm.Type == "" && frm.Email == "" {
		return send400(ctx, "Specify something to change.")
	}

	// Overwrite values
	if frm.Email != "" {
		target.Email = frm.Email
	}

	if acct.Type >= db.Admin {
		// Only allow admins to update this

		switch frm.Type {
		case "janitor":
			target.Type = db.Janitor
		case "mod":
			target.Type = db.Mod
		case "admin":
			target.Type = db.Admin
			// default: no update
		}
	}

	if err := target.Save(); err != nil {
		return send500(ctx, fmt.Errorf("failed to save user: %v", err))
	}

	if frm.Password != "" {
		if err := target.SetPassword(frm.Password); err != nil {
			return send500(ctx, fmt.Errorf("failed to update password: %v", err))
		}
	}

	return ctx.RedirectBack("/" + config.Key)
}
