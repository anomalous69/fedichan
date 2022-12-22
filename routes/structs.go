package routes

import (
	"github.com/KushBlazingJudah/fedichan/activitypub"
	"github.com/KushBlazingJudah/fedichan/db"
	"github.com/KushBlazingJudah/fedichan/util"
)

type PageData struct {
	Title             string
	PreferredUsername string
	Board             activitypub.Board
	Pages             []int
	CurrentPage       int
	TotalPage         int
	Boards            []activitypub.Board
	Posts             []activitypub.ObjectBase
	Key               string
	PostId            string
	Instance          activitypub.Actor
	ReturnTo          string
	NewsItems         []db.NewsItem
	BoardRemainer     []int
	Meta              Meta
	PostType          string

	Themes      *[]string
	ThemeCookie string
}

type AdminPage struct {
	Title         string
	Board         activitypub.Board
	Key           string
	Actor         string
	Boards        []activitypub.Board
	Following     []string
	Followers     []string
	Domain        string
	IsLocal       bool
	PostBlacklist []util.PostBlacklist
	AutoSubscribe bool
	RecentPosts   []activitypub.ObjectBase
	Instance      activitypub.Actor
	Meta          Meta

	Themes      *[]string
	ThemeCookie string
}

type Meta struct {
	Title       string
	Description string
	Url         string
	Preview     string
}
