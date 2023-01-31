package routes

import (
	"github.com/KushBlazingJudah/fedichan/activitypub"
	"github.com/KushBlazingJudah/fedichan/db"
	"github.com/KushBlazingJudah/fedichan/util"
)

type common struct {
	Title    string
	Board    activitypub.Board
	Boards   []activitypub.Board
	Instance activitypub.Actor
	Meta     meta
	Acct     *db.Acct

	Themes      []string
	ThemeCookie string

	Key string
}

type pageData struct {
	common

	PreferredUsername string
	Pages             []int
	CurrentPage       int
	TotalPage         int
	Posts             []activitypub.ObjectBase
	PostId            string
	ReturnTo          string
	NewsItems         []db.NewsItem
	BoardRemainer     []int
	PostType          string
}

type errorData struct {
	common
	Message string
	Error   error
}

type adminPage struct {
	common

	Actor         string
	Following     []string
	Followers     []string
	Domain        string
	IsLocal       bool
	PostBlacklist []util.PostBlacklist
	AutoSubscribe bool
	RecentPosts   []activitypub.ObjectBase
	Reports       map[string][]db.Reports
	Jannies       []db.Verify
}

type meta struct {
	Title       string
	Description string
	Url         string
	Preview     string
}
