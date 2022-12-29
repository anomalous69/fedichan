package activitypub

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/util"
)

func (activity Activity) AcceptFollow() Activity {
	var accept Activity
	accept.AtContext.Context = activity.AtContext.Context
	accept.Type = "Accept"
	var nActor Actor
	accept.Actor = &nActor
	accept.Actor.Id = activity.Object.Actor
	var nObj ObjectBase
	accept.Object = nObj
	accept.Object.Actor = activity.Actor.Id
	var nNested NestedObjectBase
	accept.Object.Object = &nNested
	accept.Object.Object.Actor = activity.Object.Actor
	accept.Object.Object.Type = "Follow"
	accept.To = append(accept.To, activity.Object.Actor)

	return accept
}

func (activity Activity) AddFollowersTo() (Activity, error) {
	activity.To = append(activity.To, activity.Actor.Id)

	for _, e := range activity.To {
		reqActivity := Activity{Id: e + "/followers"}
		aFollowers, err := reqActivity.GetCollection()
		if err != nil {
			// Safely fail so we can continue to federate
			config.Log.Printf("failed to get collection from %s: %v", reqActivity.Id, err)
			continue

			// return activity, util.WrapError(err)
		}

		// get followers of activity actor
		for _, k := range aFollowers.Items {
			if !util.IsInStringArray(activity.To, k.Id) {
				activity.To = append(activity.To, k.Id)
			} else {
				// We've already been here
				continue
			}

			reqActivity := Activity{Id: k.Id + "/followers"}

			bFollowers, err := reqActivity.GetCollection()
			if err != nil {
				continue
			}

			// get followers of activity actor followers
			for _, j := range bFollowers.Items {
				if !util.IsInStringArray(activity.To, j.Id) {
					activity.To = append(activity.To, j.Id)
				}
			}
		}
	}

	return activity, nil
}

func (activity Activity) CheckValid() (Collection, bool, error) {
	var respCollection Collection

	re := regexp.MustCompile(`.+\.onion(.+)?`)
	if re.MatchString(activity.Id) {
		activity.Id = strings.Replace(activity.Id, "https", "http", 1)
	}

	req, err := http.NewRequest("GET", activity.Id, nil)
	if err != nil {
		return respCollection, false, util.WrapError(err)
	}

	req.Header.Set("Accept", config.ActivityStreams)

	resp, err := util.RouteProxy(req)
	if err != nil {
		return respCollection, false, util.WrapError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return respCollection, false, nil
	}

	if err := json.NewDecoder(resp.Body).Decode(&respCollection); err != nil {
		return respCollection, false, util.WrapError(err)
	}

	if respCollection.AtContext.Context == "https://www.w3.org/ns/activitystreams" && respCollection.OrderedItems[0].Id != "" {
		return respCollection, true, nil
	}

	return respCollection, false, nil
}

func (activity Activity) GetCollection() (Collection, error) {
	var nColl Collection

	req, err := http.NewRequest("GET", activity.Id, nil)
	if err != nil {
		return nColl, util.WrapError(err)
	}

	req.Header.Set("Accept", config.ActivityStreams)
	resp, err := util.RouteProxy(req)
	if err != nil {
		return nColl, util.WrapError(err)
	}

	if resp.StatusCode == 200 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			if err := json.Unmarshal(body, &nColl); err != nil {
				return nColl, util.WrapError(err)
			}
		}
	}

	return nColl, nil
}

func (activity Activity) IsLocal() (bool, error) {

	for _, e := range activity.To {
		if res, _ := GetActorFromDB(e); res.Id != "" {
			return true, nil
		}
	}

	for _, e := range activity.Cc {
		if res, _ := GetActorFromDB(e); res.Id != "" {
			return true, nil
		}
	}

	if activity.Actor != nil {
		if res, _ := GetActorFromDB(activity.Actor.Id); res.Id != "" {
			return true, nil
		}
	}

	return false, nil
}

func (activity Activity) Process() error {
	activityType := activity.Type

	if activityType == "Create" {
		for _, e := range activity.To {
			if res, err := GetActorFromDB(e); res.Id != "" {
				config.Log.Println("actor is in the database")
			} else if err != nil {
				return util.WrapError(err)
			} else {
				config.Log.Println("actor is NOT in the database")
			}
		}
	} else if activityType == "Follow" {
		// TODO: okay?
		return errors.New("not implemented")
	} else if activityType == "Delete" {
		return errors.New("not implemented")
	}

	return nil
}

func (activity Activity) Reject() Activity {
	var accept Activity
	accept.AtContext.Context = activity.AtContext.Context
	accept.Type = "Reject"
	var nObj ObjectBase
	accept.Object = nObj
	var nActor Actor
	accept.Actor = &nActor
	accept.Actor.Id = activity.Object.Actor
	accept.Object.Actor = activity.Actor.Id
	var nNested NestedObjectBase
	accept.Object.Object = &nNested
	accept.Object.Object.Actor = activity.Object.Actor
	accept.Object.Object.Type = "Follow"
	accept.To = append(accept.To, activity.Actor.Id)

	return accept
}

func (activity Activity) Report(reason string) (bool, error) {
	if isLocal, _ := activity.Object.IsLocal(); !isLocal {
		return false, nil
	}

	reqActivity := Activity{Id: activity.Object.Id}
	activityCol, err := reqActivity.GetCollection()

	if err != nil {
		return false, util.WrapError(err)
	}

	query := `insert into reported (id, count, board, reason) values ($1, $2, $3, $4)`
	if _, err = config.DB.Exec(query, activity.Object.Object.Id, 1, activityCol.Actor.Id, reason); err != nil {
		return false, util.WrapError(err)
	}

	return true, nil
}

func (activity Activity) SetActorFollower() (Activity, error) {
	var query string

	alreadyFollower, err := activity.Actor.IsAlreadyFollower(activity.Object.Actor)

	if err != nil {
		return activity, util.WrapError(err)
	}

	if activity.Actor.Id == activity.Object.Actor {
		activity.Type = "Reject"
		return activity, nil
	}

	if alreadyFollower {
		query = `delete from follower where id=$1 and follower=$2`
		if _, err := config.DB.Exec(query, activity.Actor.Id, activity.Object.Actor); err != nil {
			return activity, util.WrapError(err)
		}

		activity.Type = "Accept"
		activity.Summary = activity.Object.Actor + " Unfollow " + activity.Actor.Id
		return activity, util.WrapError(err)
	}

	query = `insert into follower (id, follower) values ($1, $2)`
	if _, err := config.DB.Exec(query, activity.Actor.Id, activity.Object.Actor); err != nil {
		return activity, util.WrapError(err)
	}

	activity.Type = "Accept"
	activity.Summary = activity.Object.Actor + " Follow " + activity.Actor.Id

	return activity, nil
}

func (activity Activity) SetActorFollowing() (Activity, error) {
	objActor, _ := GetActor(activity.Object.Actor)
	alreadyFollowing, err := objActor.IsAlreadyFollowing(activity.Actor.Id)

	if err != nil {
		return activity, util.WrapError(err)
	}

	actor, err := FingerActor(activity.Actor.Id)

	if err != nil {
		return activity, util.WrapError(err)
	}

	reqActivity := Activity{Id: actor.Followers}
	remoteActorFollowerCol, err := reqActivity.GetCollection()

	if err != nil {
		return activity, util.WrapError(err)
	}

	alreadyFollower := false

	for _, e := range remoteActorFollowerCol.Items {
		if e.Id == activity.Object.Actor {
			alreadyFollower = true
		}
	}

	if activity.Actor.Id == activity.Object.Actor {
		activity.Type = "Reject"
		return activity, nil
	}

	var query string

	if alreadyFollowing && alreadyFollower {
		if res, _ := activity.Actor.IsLocal(); !res {
			go activity.Actor.DeleteCache()
		} else if err != nil {
			return activity, util.WrapError(err)
		}

		query = `delete from following where id=$1 and following=$2`
		if _, err := config.DB.Exec(query, activity.Object.Actor, activity.Actor.Id); err != nil {
			return activity, util.WrapError(err)
		}

		activity.Type = "Accept"
		activity.Summary = activity.Object.Actor + " Unfollowing " + activity.Actor.Id

		return activity, nil
	}

	if !alreadyFollowing && !alreadyFollower {
		if res, _ := activity.Actor.IsLocal(); !res {
			go activity.Actor.WriteCache()
		}

		query = `insert into following (id, following) values ($1, $2)`
		if _, err := config.DB.Exec(query, activity.Object.Actor, activity.Actor.Id); err != nil {
			return activity, util.WrapError(err)
		}

		activity.Type = "Accept"
		activity.Summary = activity.Object.Actor + " Following " + activity.Actor.Id

		return activity, nil
	}

	return activity, nil
}

func (activity Activity) Send() error {
	// TODO: Should requests be async or sync?
	// Currently they are async. Good for responsiveness but may cause confusion

	j, _ := json.MarshalIndent(activity, "", "\t")

	for _, e := range activity.To {
		if e != activity.Actor.Id {
			// TODO: webfinger
			actor := Actor{Id: e, Inbox: e + "/inbox"}

			name, instance := GetActorAndInstance(actor.Id)

			if name != "main" {
				go func(actor Actor, activity Activity) error {
					var status int
					var try int

					for try != 5 && status != 200 {
						time.Sleep(time.Duration(try) * time.Minute)

						req, err := http.NewRequest("POST", actor.Inbox, bytes.NewBuffer(j))
						if err != nil {
							return util.WrapError(err)
						}

						date := time.Now().UTC().Format(time.RFC1123)
						path := strings.Replace(actor.Inbox, instance, "", 1)
						re := regexp.MustCompile("https?://(www.)?")
						path = re.ReplaceAllString(path, "")
						sig := fmt.Sprintf("(request-target): %s %s\nhost: %s\ndate: %s", "post", path, instance, date)
						encSig, err := activity.Actor.ActivitySign(sig)

						if err != nil {
							return util.WrapError(err)
						}

						signature := fmt.Sprintf("keyId=\"%s\",headers=\"(request-target) host date\",signature=\"%s\"", activity.Actor.PublicKey.Id, encSig)

						req.Header.Set("Content-Type", config.ActivityStreams)
						req.Header.Set("Date", date)
						req.Header.Set("Signature", signature)
						req.Host = instance

						resp, err := util.RouteProxy(req)

						if err != nil {
							try += 1
							continue
						}

						status = resp.StatusCode
						try += 1
					}

					return nil

				}(actor, activity)
			}
		}

		time.Sleep(150 * time.Millisecond)
	}

	return nil
}
