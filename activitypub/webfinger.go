package activitypub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/util"
)

var Boards []Board
var FollowingBoards []ObjectBase

type Webfinger struct {
	Subject string          `json:"subject,omitempty"`
	Links   []WebfingerLink `json:"links,omitempty"`
}

type WebfingerLink struct {
	Rel  string `json:"rel,omitempty"`
	Type string `json:"type,omitempty"`
	Href string `json:"href,omitempty"`
}

type Board struct {
	Name        string
	Actor       Actor
	Summary     string
	PrefName    string
	InReplyTo   string
	Location    string
	To          string
	RedirectTo  string
	Captcha     string
	CaptchaCode string
	Domain      string
	TP          string
	Restricted  bool
	Post        ObjectBase
}

type BoardSortAsc []Board

func (a BoardSortAsc) Len() int           { return len(a) }
func (a BoardSortAsc) Less(i, j int) bool { return a[i].Name < a[j].Name }
func (a BoardSortAsc) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func GetActor(id string) (Actor, error) {
	var respActor Actor

	if id == "" {
		return respActor, nil
	}

	actor, instance := GetActorAndInstance(id)

	if ActorCache[actor+"@"+instance].Id != "" {
		respActor = ActorCache[actor+"@"+instance]
		return respActor, nil
	}

	req, err := http.NewRequest("GET", strings.TrimSpace(id), nil)
	if err != nil {
		return respActor, util.WrapError(err)
	}

	req.Header.Set("Accept", config.ActivityStreams)

	resp, err := util.RouteProxy(req)

	if err != nil {
		return respActor, util.WrapError(err)
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if err := json.Unmarshal(body, &respActor); err != nil {
		return respActor, util.WrapError(err)
	}

	ActorCache[actor+"@"+instance] = respActor

	return respActor, nil
}

// looks for actor with pattern of board@instance
func FingerActor(path string) (Actor, error) {
	var nActor Actor

	actor, instance := GetActorAndInstance(path)

	if actor == "" && instance == "" {
		return nActor, nil
	}

	if ActorCache[actor+"@"+instance].Id != "" {
		nActor = ActorCache[actor+"@"+instance]
	} else {
		resp, err := FingerRequest(actor, instance)
		if err != nil {
			return nActor, util.WrapError(err)
		}

		if resp != nil && resp.StatusCode == 200 {
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nActor, util.WrapError(err)
			}

			if err := json.Unmarshal(body, &nActor); err != nil {
				return nActor, util.WrapError(err)
			}

			ActorCache[actor+"@"+instance] = nActor
		}
	}

	return nActor, nil
}

func FingerRequest(actor string, instance string) (*http.Response, error) {
	acct := "acct:" + actor + "@" + instance

	// TODO: respect https
	req, err := http.NewRequest("GET", "http://"+instance+"/.well-known/webfinger?resource="+acct, nil)

	if err != nil {
		return nil, util.WrapError(err)
	}

	resp, err := util.RouteProxy(req)
	if err != nil {
		return resp, err
	}

	var finger Webfinger

	if resp.StatusCode == 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return resp, err
		}

		if err := json.Unmarshal(body, &finger); err != nil {
			return resp, util.WrapError(err)
		}
	}

	if len(finger.Links) > 0 {
		for _, e := range finger.Links {
			if e.Type == "application/activity+json" {
				req, err = http.NewRequest("GET", e.Href, nil)

				if err != nil {
					return resp, util.WrapError(err)
				}

				break
			}
		}
	}

	req.Header.Set("Accept", config.ActivityStreams)
	if resp, err = util.RouteProxy(req); err != nil {
		return resp, util.WrapError(err)
	}

	return resp, nil
}

func GetActorByNameFromBoardCollection(name string) Actor {
	var actor Actor

	boards, _ := GetBoardCollection()
	for _, e := range boards {
		if e.Actor.Name == name {
			actor = e.Actor
		}
	}

	return actor
}

func GetBoardCollection() ([]Board, error) {
	var collection []Board

	for _, e := range FollowingBoards {
		var board Board

		boardActor, err := GetActorFromDB(e.Id)

		if err != nil {
			return collection, util.WrapError(err)
		}

		if boardActor.Id == "" {
			boardActor, err = FingerActor(e.Id)

			if err != nil {
				return collection, util.WrapError(err)
			}
		}

		board.Name = boardActor.Name
		board.PrefName = boardActor.PreferredUsername
		board.Location = "/" + boardActor.Name
		board.Actor = boardActor
		board.Restricted = boardActor.Restricted

		collection = append(collection, board)
	}

	sort.Sort(BoardSortAsc(collection))

	return collection, nil
}

func GetActorFromPath(location string, prefix string) (Actor, error) {
	var actor string

	pattern := fmt.Sprintf("%s([^/\n]+)(/.+)?", prefix)
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(location)

	if len(match) < 1 {
		actor = "/"
	} else {
		actor = strings.Replace(match[1], "/", "", -1)
	}

	if actor == "/" || actor == "outbox" || actor == "inbox" || actor == "following" || actor == "followers" {
		actor = "main"
	}

	var nActor Actor

	nActor, err := GetActorByNameFromDB(actor)

	if err != nil {
		return nActor, util.WrapError(err)
	}

	if nActor.Id == "" {
		nActor = GetActorByNameFromBoardCollection(actor)
	}

	return nActor, nil
}

func StartupArchive() error {
	for _, e := range FollowingBoards {
		actor, err := GetActorFromDB(e.Id)

		if err != nil {
			return util.WrapError(err)
		}

		if err := actor.ArchivePosts(); err != nil {
			return util.WrapError(err)
		}
	}

	return nil
}
