package activitypub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/util"
)

func (activity Activity) AcceptFollow(a Actor) Activity {
	var accept Activity
	accept.AtContext.Context = activity.AtContext.Context
	accept.Type = "Accept"
	accept.Actor = &a

	accept.Object.Actor = activity.Actor.Id

	accept.Object.Object = &ObjectBase{
		Actor: activity.Object.Actor,
		Type:  "Follow",
	}

	accept.To = append(accept.To, activity.Actor.Id)

	return accept
}

func (activity Activity) AddFollowersTo() (Activity, error) {
	activity.To = append(activity.To, activity.Actor.Id)

	for _, e := range activity.To {
		reqActivity := Activity{Id: e + "/followers"}
		aFollowers, err := reqActivity.GetCollection()
		if err != nil {
			// Safely fail so we can continue to federate
			log.Printf("failed to get collection from %s: %v", reqActivity.Id, err)
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
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nColl, fmt.Errorf("non 200 response code (%d)", resp.StatusCode)
	}

	err = json.NewDecoder(resp.Body).Decode(&nColl)
	return nColl, util.WrapError(err) // no-op if nil
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

func (activity Activity) Reject() Activity {
	var accept Activity
	accept.AtContext.Context = activity.AtContext.Context
	accept.Type = "Reject"

	accept.Object.Actor = activity.Actor.Id
	accept.Actor = &Actor{
		Id: activity.Object.Actor,
	}

	accept.Object.Object = &ObjectBase{
		Actor: activity.Object.Actor,
		Type:  "Follow",
	}
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
	if activity.Actor.Id == activity.Object.Actor {
		// TODO: Non-sensical activity?
		return activity, nil
	}

	alreadyFollower, err := activity.Actor.IsAlreadyFollower(activity.Object.Actor)
	if err != nil {
		return activity, util.WrapError(err)
	}

	if alreadyFollower {
		query := `delete from follower where id=$1 and follower=$2`
		if _, err := config.DB.Exec(query, activity.Actor.Id, activity.Object.Actor); err != nil {
			return activity, util.WrapError(err)
		}

		activity.Type = "Accept"
		activity.Summary = activity.Object.Actor + " Unfollow " + activity.Actor.Id
		return activity, util.WrapError(err)
	}

	query := `insert into follower (id, follower) values ($1, $2)`
	if _, err := config.DB.Exec(query, activity.Actor.Id, activity.Object.Actor); err != nil {
		return activity, util.WrapError(err)
	}

	activity.Type = "Accept"
	activity.Summary = activity.Object.Actor + " Follow " + activity.Actor.Id

	return activity, nil
}

func (activity Activity) SetActorFollowing() error {
	if activity.Actor.Id == activity.Object.Actor {
		// TODO: Non-sensical activity?
		return nil
	}

	objActor, _ := GetActor(activity.Object.Actor)
	alreadyFollowing, err := objActor.IsAlreadyFollowing(activity.Actor.Id)
	if err != nil {
		return util.WrapError(err)
	}

	if !alreadyFollowing {
		if res, _ := activity.Actor.IsLocal(); !res {
			go activity.Actor.WriteCache()
		}

		query := `insert into following (id, following) values ($1, $2)`
		if _, err := config.DB.Exec(query, activity.Object.Actor, activity.Actor.Id); err != nil {
			return util.WrapError(err)
		}
	}

	return nil
}

func (activity Activity) send(actor Actor, j []byte, instance string) {
	for try := 0; try < 5; try++ {
		// no-op on first iter
		time.Sleep(time.Duration(try) * time.Minute)

		req, err := http.NewRequest("POST", actor.Inbox, bytes.NewBuffer(j))
		if err != nil {
			log.Printf("fatal error creating request for activity to %s: %v", actor.Inbox, err)
			return
		}

		// sign the activity
		// must be done every time because of the signing window
		date := time.Now().UTC().Format(time.RFC1123)
		path := strings.Replace(actor.Inbox, instance, "", 1)
		re := regexp.MustCompile("https?://(www.)?")
		path = re.ReplaceAllString(path, "")
		sig := fmt.Sprintf("(request-target): %s %s\nhost: %s\ndate: %s", "post", path, instance, date)
		encSig, err := activity.Actor.ActivitySign(sig)
		if err != nil {
			log.Printf("fatal error signing activity to %s: %v", actor.Inbox, err)
			return
		}

		signature := fmt.Sprintf(`keyId="%s",headers="(request-target) host date",signature="%s"`, activity.Actor.PublicKey.Id, encSig)

		// set headers
		req.Header.Set("Content-Type", config.ActivityStreams)
		req.Header.Set("Date", date)
		req.Header.Set("Signature", signature)
		req.Host = instance

		resp, err := util.RouteProxy(req)
		if err != nil {
			log.Printf("couldn't send activity to %s (try %d): %v", actor.Inbox, try, err)
			continue
		}
		resp.Body.Close() // we don't need it

		switch resp.StatusCode {
		case 200: // done!
			goto out
		case 400, 401, 403:
			// we're unlikely to be able to repeat this request
			log.Printf("couldn't send activity to %s: fatal status code %d", actor.Inbox, resp.StatusCode)
			goto out
		default:
			log.Printf("couldn't send activity to %s (try %d): status code %d", actor.Inbox, try, resp.StatusCode)
			continue
		}

		try += 1
		continue

	out:
		break
	}
}

func (activity Activity) Send() error {
	// TODO: Should requests be async or sync?
	// Currently they are async. Good for responsiveness but may cause confusion

	j, _ := json.MarshalIndent(activity, "", "\t")

	// TODO: debug switch
	log.Println(string(j))

	for _, e := range activity.To {
		if e != activity.Actor.Id {
			// TODO: webfinger
			actor := Actor{Id: e, Inbox: e + "/inbox"}
			name, instance := GetActorAndInstance(actor.Id)

			if name != "main" {
				go activity.send(actor, j, instance)
			}
		}
	}

	return nil
}
