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
	Meta     Meta
	Acct     *db.Acct

	Themes      []string
	ThemeCookie string
}

type PageData struct {
	common

	PreferredUsername string
	Pages             []int
	CurrentPage       int
	TotalPage         int
	Posts             []activitypub.ObjectBase
	Key               string
	PostId            string
	ReturnTo          string
	NewsItems         []db.NewsItem
	BoardRemainer     []int
	PostType          string
}

type AdminPage struct {
	common

	Key           string
	Actor         string
	Following     []string
	Followers     []string
	Domain        string
	IsLocal       bool
	PostBlacklist []util.PostBlacklist
	AutoSubscribe bool
	RecentPosts   []activitypub.ObjectBase
}

type Meta struct {
	Title       string
	Description string
	Url         string
	Preview     string
}
