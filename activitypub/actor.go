package activitypub

import (
	"crypto"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/util"
	"github.com/gofiber/fiber/v2"
)

var ActorCache = make(map[string]Actor)

func (actor Actor) AddFollower(follower string) error {
	query := `insert into follower (id, follower) values ($1, $2)`
	_, err := config.DB.Exec(query, actor.Id, follower)
	return util.WrapError(err)
}

func (actor Actor) ActivitySign(signature string) (string, error) {
	if actor.PublicKey.Id == "" {
		actor, _ = GetActorFromDB(actor.Id)
	}

	var file string

	query := `select file from publicKeyPem where id=$1 `
	if err := config.DB.QueryRow(query, actor.PublicKey.Id).Scan(&file); err != nil {
		return "", util.WrapError(err)
	}

	file = strings.ReplaceAll(file, "public.pem", "private.pem")

	_, err := os.Stat(file)
	if err != nil {
		log.Println(`\n Unable to locate private key. Now,
this means that you are now missing the proof that you are the
owner of the "` + actor.Name + `" board. If you are the developer,
then your job is just as easy as generating a new keypair, but
if this board is live, then you'll also have to convince the other
owners to switch their public keys for you so that they will start
accepting your posts from your board from this site. Good luck ;)`)
		return "", util.WrapError(err)
	}

	var publickey []byte

	if publickey, err = os.ReadFile(file); err != nil {
		return "", util.WrapError(err)
	}

	block, _ := pem.Decode(publickey)
	pub, _ := x509.ParsePKCS1PrivateKey(block.Bytes)
	rng := crand.Reader
	hashed := sha256.New()
	hashed.Write([]byte(signature))
	cipher, _ := rsa.SignPKCS1v15(rng, pub, crypto.SHA256, hashed.Sum(nil))

	return base64.StdEncoding.EncodeToString(cipher), nil
}

func (actor Actor) ArchivePosts() error {
	if actor.Id != "" && actor.Id != config.Domain {
		col, err := actor.GetAllArchive(165)

		if err != nil {
			return util.WrapError(err)
		}

		for _, e := range col.OrderedItems {
			for _, k := range e.Replies.OrderedItems {
				if err := k.UpdateType("Archive"); err != nil {
					return util.WrapError(err)
				}
			}

			if err := e.UpdateType("Archive"); err != nil {
				return util.WrapError(err)
			}
		}
	}

	return nil
}

func (actor Actor) AutoFollow() error {
	nActor, _ := GetActor(actor.Id)
	following, err := nActor.GetFollowing()

	if err != nil {
		return util.WrapError(err)
	}

	follower, err := nActor.GetFollower()

	if err != nil {
		return util.WrapError(err)
	}

	isFollowing := false

	for _, e := range follower {
		for _, k := range following {
			if e.Id == k.Id {
				isFollowing = true
			}
		}

		if !isFollowing && e.Id != config.Domain && e.Id != nActor.Id {
			followActivity, err := nActor.MakeFollowActivity(e.Id)

			if err != nil {
				return util.WrapError(err)
			}

			nActor, err := FingerActor(e.Id)

			if err != nil {
				return util.WrapError(err)
			}

			if nActor.Id != "" {
				followActivity.Send()
			}
		}
	}

	return nil
}

func (actor Actor) DeleteCache() error {
	query := `select id from cacheactivitystream where id in (select id from cacheactivitystream where actor=$1 and id in (select id from replies where inreplyto='')) `
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return util.WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {
		var obj ObjectBase
		if err := rows.Scan(&obj.Id); err != nil {
			return util.WrapError(err)
		}

		if err := obj.Delete(); err != nil {
			return util.WrapError(err)
		}
	}

	return nil
}

func (actor Actor) GetAllArchive(offset int) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.updated from (select id, updated from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, updated from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, updated from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc offset $2`
	rows, err := config.DB.Query(query, actor.Id, offset)

	if err != nil {
		return nColl, util.WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase

		if err := rows.Scan(&post.Id, &post.Updated); err != nil {
			return nColl, util.WrapError(err)
		}

		post.Replies, err = post.GetReplies()

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetAutoSubscribe() (bool, error) {
	var subscribed bool

	query := `select autosubscribe from actor where id=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&subscribed); err != nil {
		return false, util.WrapError(err)
	}

	return subscribed, nil
}

func (actor Actor) GetCatalogCollection() (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	var err error
	var rows *sql.Rows

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' and id not in (select activity_id from sticky where actor_id=$1) union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' and id not in (select activity_id from sticky where actor_id=$1) union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' and id not in (select activity_id from sticky where actor_id=$1)) as x order by x.updated desc limit 165`

	if rows, err = config.DB.Query(query, actor.Id); err != nil {
		return nColl, util.WrapError(err)
	}

	stickies, _ := actor.GetStickies()
	result = append(result, stickies.OrderedItems...)

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev ObjectBase

		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)

		if err != nil {
			return nColl, util.WrapError(err)
		}

		post.Locked, _ = post.IsLocked()
		post.Actor = actor.Id

		post.Replies = &CollectionBase{}

		post.Replies.TotalItems, post.Replies.TotalImgs, err = post.GetRepliesCount()

		if err != nil {
			return nColl, util.WrapError(err)
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.WrapError(err)
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nColl, util.WrapError(err)
			}
		}

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetCollectionPage(page int) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	var err error
	var rows *sql.Rows

	query := `select count (x.id) over(), x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' and id not in (select activity_id from sticky where actor_id=$1) union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' and id not in (select activity_id from sticky where actor_id=$1) union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where id not in (select activity_id from sticky where actor_id=$1) and actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc limit $2 offset $3`

	limit := 15

	if page == 0 {
		stickies, _ := actor.GetStickies()
		limit = limit - stickies.TotalItems

		result = append(result, stickies.OrderedItems...)
	}

	offset := page * limit

	if rows, err = config.DB.Query(query, actor.Id, limit, offset); err != nil {
		return nColl, util.WrapError(err)
	}

	var count int
	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev ObjectBase

		err = rows.Scan(&count, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)

		if err != nil {
			return nColl, util.WrapError(err)
		}

		post.Locked, _ = post.IsLocked()
		post.Actor = actor.Id

		post.Replies, err = post.GetRepliesLimit(5)

		if err != nil {
			return nColl, util.WrapError(err)
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.WrapError(err)
			}
		}

		if err != nil {
			return nColl, util.WrapError(err)
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nColl, util.WrapError(err)
			}
		}

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.TotalItems = count

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetCollection() (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' order by updated desc`
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return nColl, util.WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev ObjectBase

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, util.WrapError(err)
		}

		post.Sticky, _ = post.IsSticky()
		post.Locked, _ = post.IsLocked()

		post.Actor = actor.Id

		post.Replies, err = post.GetReplies()

		if err != nil {
			return nColl, util.WrapError(err)
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.WrapError(err)
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nColl, util.WrapError(err)
			}
		}

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetCollectionType(nType string) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2) as x order by x.updated desc`
	rows, err := config.DB.Query(query, actor.Id, nType)
	if err != nil {
		return nColl, util.WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev ObjectBase

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, util.WrapError(err)
		}

		post.Actor = actor.Id

		post.Replies = &CollectionBase{}

		post.Replies.TotalItems, post.Replies.TotalImgs, err = post.GetRepliesCount()
		if err != nil {
			return nColl, util.WrapError(err)
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.WrapError(err)
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nColl, util.WrapError(err)
			}
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetCollectionTypeLimit(nType string, limit int) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2 union select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2) as x order by x.updated desc limit $3`
	rows, err := config.DB.Query(query, actor.Id, nType, limit)

	if err != nil {
		return nColl, util.WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev ObjectBase

		if err := rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, util.WrapError(err)
		}

		post.Actor = actor.Id

		post.Replies = &CollectionBase{}

		post.Replies.TotalItems, post.Replies.TotalImgs, err = post.GetRepliesCount()
		if err != nil {
			return nColl, util.WrapError(err)
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.WrapError(err)
			}
		}

		if prev.Id != "" {
			post.Preview, err = post.Preview.GetPreview()
			if err != nil {
				return nColl, util.WrapError(err)
			}
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetFollower() ([]ObjectBase, error) {
	var followerCollection []ObjectBase

	query := `select follower from follower where id=$1`
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return followerCollection, util.WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {
		var obj ObjectBase

		if err := rows.Scan(&obj.Id); err != nil {
			return followerCollection, util.WrapError(err)
		}

		followerCollection = append(followerCollection, obj)
	}

	return followerCollection, nil
}

func (actor Actor) GetFollowing() ([]ObjectBase, error) {
	var followingCollection []ObjectBase

	query := `select following from following where id=$1`
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return followingCollection, util.WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {
		var obj ObjectBase

		if err := rows.Scan(&obj.Id); err != nil {
			return followingCollection, util.WrapError(err)
		}

		followingCollection = append(followingCollection, obj)
	}

	return followingCollection, nil
}

func (actor Actor) GetFollowFromName(name string) ([]string, error) {
	var followingActors []string

	activity := Activity{Id: actor.Following}
	follow, err := activity.GetCollection()
	if err != nil {
		return followingActors, util.WrapError(err)
	}

	re := regexp.MustCompile(`\w+?$`)

	for _, e := range follow.Items {
		if re.FindString(e.Id) == name {
			followingActors = append(followingActors, e.Id)
		}
	}

	return followingActors, nil
}

func (actor Actor) GetFollowingTotal() (int, error) {
	var following int

	query := `select count(following) from following where id=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&following); err != nil {
		return following, util.WrapError(err)
	}

	return following, nil
}

func (actor Actor) GetFollowersTotal() (int, error) {
	var followers int

	query := `select count(follower) from follower where id=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&followers); err != nil {
		return followers, util.WrapError(err)
	}

	return followers, nil
}

func (actor Actor) GetFollowersResp(ctx *fiber.Ctx) error {
	var following Collection
	var err error

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems, err = actor.GetFollowersTotal()

	if err != nil {
		return util.WrapError(err)
	}

	following.Items, err = actor.GetFollower()

	if err != nil {
		return util.WrapError(err)
	}

	enc, _ := json.MarshalIndent(following, "", "\t")
	ctx.Response().Header.Set("Content-Type", config.ActivityStreams)
	_, err = ctx.Write(enc)

	return util.WrapError(err)
}

func (actor Actor) GetFollowingResp(ctx *fiber.Ctx) error {
	var following Collection
	var err error

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems, err = actor.GetFollowingTotal()

	if err != nil {
		return util.WrapError(err)
	}

	following.Items, err = actor.GetFollowing()

	if err != nil {
		return util.WrapError(err)
	}

	enc, _ := json.MarshalIndent(following, "", "\t")
	ctx.Response().Header.Set("Content-Type", config.ActivityStreams)
	_, err = ctx.Write(enc)

	return util.WrapError(err)
}

func (actor Actor) GetImgTotal() (int, error) {
	var count int

	query := `select count(attachment) from activitystream where actor=$1 and id in (select id from replies where inreplyto='' and type='Note' )`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&count); err != nil {
		return count, util.WrapError(err)
	}

	return count, nil
}

func (actor Actor) GetInfoResp(ctx *fiber.Ctx) error {
	enc, _ := json.MarshalIndent(actor, "", "\t")
	ctx.Response().Header.Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")

	_, err := ctx.Write(enc)

	return util.WrapError(err)
}

func (actor Actor) GetPostTotal() (int, error) {
	var count int

	query := `select count(id) from activitystream where actor=$1 and id in (select id from replies where inreplyto='' and type='Note')`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&count); err != nil {
		return count, util.WrapError(err)
	}

	return count, nil
}

func (actor Actor) GetOutbox(ctx *fiber.Ctx) error {
	var collection Collection

	c, err := actor.GetCollection()

	if err != nil {
		return util.WrapError(err)
	}

	collection.OrderedItems = c.OrderedItems
	collection.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	collection.Actor = &actor

	collection.TotalItems, err = actor.GetPostTotal()

	if err != nil {
		return util.WrapError(err)
	}

	collection.TotalImgs, err = actor.GetImgTotal()

	if err != nil {
		return util.WrapError(err)
	}

	enc, _ := json.Marshal(collection)
	ctx.Response().Header.Set("Content-Type", config.ActivityStreams)
	_, err = ctx.Write(enc)

	return util.WrapError(err)
}

func (actor Actor) GetRecentPosts() ([]ObjectBase, error) {
	var collection []ObjectBase

	query := `select id, actor, content, published, attachment from activitystream where actor=$1 and type='Note' union select id, actor, content, published, attachment from cacheactivitystream where actor in (select follower from follower where id=$1) and type='Note' order by published desc limit 20`
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return collection, util.WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {
		var nObj ObjectBase
		var attachment ObjectBase

		err := rows.Scan(&nObj.Id, &nObj.Actor, &nObj.Content, &nObj.Published, &attachment.Id)

		if err != nil {
			return collection, util.WrapError(err)
		}

		isOP, _ := nObj.CheckIfOP()

		nObj.Attachment, _ = attachment.GetAttachment()

		if !isOP {
			var reply ObjectBase
			reply.Id = nObj.Id
			nObj.InReplyTo = append(nObj.InReplyTo, reply)
		}

		collection = append(collection, nObj)
	}

	return collection, nil
}

func (actor Actor) GetReported() ([]ObjectBase, error) {
	var nObj []ObjectBase

	query := `select id, count, reason from reported where board=$1`
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return nObj, util.WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {
		var obj ObjectBase

		err := rows.Scan(&obj.Id, &obj.Size, &obj.Content)

		if err != nil {
			return nObj, util.WrapError(err)
		}

		nObj = append(nObj, obj)
	}

	return nObj, nil
}

func (actor Actor) GetReportedTotal() (int, error) {
	var count int

	query := `select count(id) from reported where board=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&count); err != nil {
		return 0, util.WrapError(err)
	}

	return count, nil
}

func (actor Actor) HasValidation(ctx *fiber.Ctx) bool {
	// TODO: Board-specific account permissions are not yet implemented
	// Assume yes if acct exists.
	a := ctx.Locals("acct")
	return a != nil
}

func (actor Actor) IsAlreadyFollowing(follow string) (bool, error) {
	followers, err := actor.GetFollowing()

	if err != nil {
		return false, util.WrapError(err)
	}

	for _, e := range followers {
		if e.Id == follow {
			return true, nil
		}
	}

	return false, nil
}

func (actor Actor) IsAlreadyFollower(follow string) (bool, error) {
	followers, err := actor.GetFollower()

	if err != nil {
		return false, util.WrapError(err)
	}

	for _, e := range followers {
		if e.Id == follow {
			return true, nil
		}
	}

	return false, nil
}

func (actor Actor) IsLocal() (bool, error) {
	actor, _ = GetActorFromDB(actor.Id)
	return actor.Id != "", nil
}

func (actor Actor) IsValid() (Actor, bool, error) {
	actor, err := FingerActor(actor.Id)
	return actor, actor.Id != "", util.WrapError(err)
}

func (actor Actor) ReportedResp(ctx *fiber.Ctx) error {
	var err error

	auth := ctx.Get("Authorization")
	verification := strings.Split(auth, " ")

	if len(verification) < 2 {
		_, err := ctx.Status(400).Write([]byte(""))
		return util.WrapError(err)
	}

	/* TODO
	if hasAuth, _ := util.HasAuth(verification[1], actor.Id); !hasAuth {
		_, err := ctx.Status(400).Write([]byte(""))
		return util.WrapError(err)
	}
	*/

	actor, err = GetActorFromDB(actor.Id)

	if err != nil {
		return util.WrapError(err)
	}

	var following Collection

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems, err = actor.GetReportedTotal()

	if err != nil {
		return util.WrapError(err)
	}

	following.Items, err = actor.GetReported()

	if err != nil {
		return util.WrapError(err)
	}

	enc, err := json.MarshalIndent(following, "", "\t")

	if err != nil {
		return util.WrapError(err)
	}

	ctx.Response().Header.Set("Content-Type", config.ActivityStreams)
	_, err = ctx.Write(enc)

	return util.WrapError(err)
}

func (actor Actor) SetAutoSubscribe() error {
	current, err := actor.GetAutoSubscribe()

	if err != nil {
		return util.WrapError(err)
	}

	query := `update actor set autosubscribe=$1 where id=$2`
	_, err = config.DB.Exec(query, !current, actor.Id)

	return util.WrapError(err)
}

func (actor Actor) SendToFollowers(activity Activity) error {
	followers, err := actor.GetFollower()

	if err != nil {
		return util.WrapError(err)
	}

	var cc []string

	for _, e := range followers {
		var isTo = false

		for _, k := range activity.To {
			if e.Id != k {
				isTo = true
			}
		}

		if !isTo {
			cc = append(cc, e.Id)
		}
	}

	activity.To = make([]string, 0)
	activity.Cc = cc

	err = activity.Send()

	return util.WrapError(err)
}

func (actor Actor) UnArchiveLast() error {
	col, err := actor.GetCollectionTypeLimit("Archive", 1)

	if err != nil {
		return util.WrapError(err)
	}

	for _, e := range col.OrderedItems {
		for _, k := range e.Replies.OrderedItems {
			if err := k.UpdateType("Note"); err != nil {
				return util.WrapError(err)
			}
		}

		if err := e.UpdateType("Note"); err != nil {
			return util.WrapError(err)
		}
	}

	return nil
}

func (actor Actor) Verify(signature string, verify string) error {
	sig, _ := base64.StdEncoding.DecodeString(signature)

	if actor.PublicKey.PublicKeyPem == "" {
		_actor, err := FingerActor(actor.Id)

		if err != nil {
			return util.WrapError(err)
		}

		actor = _actor
	}

	block, _ := pem.Decode([]byte(actor.PublicKey.PublicKeyPem))

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)

	if err != nil {
		return util.WrapError(err)
	}

	hashed := sha256.New()
	hashed.Write([]byte(verify))

	return rsa.VerifyPKCS1v15(pub.(*rsa.PublicKey), crypto.SHA256, hashed.Sum(nil), sig)
}

func (actor Actor) VerifyHeaderSignature(ctx *fiber.Ctx) bool {
	var sig string
	var path string
	var host string
	var date string
	var method string
	var digest string
	var contentLength string

	s := ParseHeaderSignature(ctx.Get("Signature"))

	for i, e := range s.Headers {
		var nl string
		if i < len(s.Headers)-1 {
			nl = "\n"
		}

		switch e {
		case "(request-target)":
			method = strings.ToLower(ctx.Method())
			path = ctx.Path()
			sig += "(request-target): " + method + " " + path + "" + nl
		case "host":
			host = ctx.Hostname()
			sig += "host: " + host + "" + nl
		case "date":
			date = ctx.Get("date")
			sig += "date: " + date + "" + nl
		case "digest":
			digest = ctx.Get("digest")
			sig += "digest: " + digest + "" + nl
		case "content-length":
			contentLength = ctx.Get("content-length")
			sig += "content-length: " + contentLength + "" + nl
		}
	}
	if s.KeyId != actor.PublicKey.Id {
		return false
	}

	t, _ := time.Parse(time.RFC1123, date)

	if time.Now().UTC().Sub(t).Seconds() > 75 {
		return false
	}

	if actor.Verify(s.Signature, sig) != nil {
		return false
	}

	return true
}

func (actor Actor) WriteCache() error {
	actor, err := FingerActor(actor.Id)

	if err != nil {
		return util.WrapError(err)
	}

	reqActivity := Activity{Id: actor.Outbox}
	collection, err := reqActivity.GetCollection()

	if err != nil {
		return util.WrapError(err)
	}

	for _, e := range collection.OrderedItems {
		if _, err := e.WriteCache(); err != nil {
			return util.WrapError(err)
		}
	}

	return nil
}

func (actor Actor) MakeFollowActivity(follow string) (Activity, error) {
	var followActivity Activity
	var err error

	followActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	followActivity.Type = "Follow"

	var obj ObjectBase
	var nactor Actor

	if actor.Id == config.Domain {
		nactor, err = GetActorFromDB(actor.Id)
	} else {
		nactor, err = FingerActor(actor.Id)
	}

	if err != nil {
		return followActivity, util.WrapError(err)
	}

	followActivity.Actor = &nactor
	followActivity.Object = obj

	followActivity.Object.Actor = follow
	followActivity.To = append(followActivity.To, follow)

	return followActivity, nil
}

func (actor Actor) WantToServePage(page int) (Collection, error) {
	var collection Collection
	var err error

	if page > config.PostCountPerPage {
		return collection, errors.New("above page limit")
	}

	if collection, err = actor.GetCollectionPage(page); err != nil {
		return collection, util.WrapError(err)
	}

	collection.Actor = &actor

	return collection, nil
}

func (actor Actor) ProcessInboxCreate(activity Activity) error {
	if local, _ := actor.IsLocal(); local {
		if local, _ := activity.Actor.IsLocal(); !local {
			reqActivity := Activity{Id: activity.Object.Id}
			col, err := reqActivity.GetCollection()
			if err != nil {
				return util.WrapError(err)
			}

			if len(col.OrderedItems) < 1 {
				return util.WrapError(err)
			}

			if len(activity.Object.InReplyTo) > 0 {
				if locked, _ := activity.Object.InReplyTo[0].IsLocked(); locked {
					return util.WrapError(err)
				}
			}

			if wantToCache, err := activity.Object.WantToCache(actor); !wantToCache {
				return util.WrapError(err)
			}

			if col, _ := activity.Object.GetCollectionLocal(); len(col.OrderedItems) != 0 {
				return nil
			}

			if _, err := activity.Object.WriteCache(); err != nil {
				return util.WrapError(err)
			}

			if err := actor.ArchivePosts(); err != nil {
				return util.WrapError(err)
			}

			activity.Object.Actor = actor.Id

			go func(obj ObjectBase) {
				err := obj.SendEmailNotify()

				if err != nil {
					log.Println(err)
				}
			}(activity.Object)
		}
	}

	return nil
}

func (actor Actor) GetStickies() (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `
select count
(x.id) over(), x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive
	from
	 (select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream
		where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' and id in (select activity_id from sticky where actor_id=$1)
	union
		select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream
		where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' and id in (select activity_id from sticky where actor_id=$1)
	union
		select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1)
		and id in (select id from replies where inreplyto='') and type='Note' and id in (select activity_id from sticky where actor_id=$1)
) as x order by x.updated desc limit 15`

	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return nColl, util.WrapError(err)
	}

	var count int
	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev ObjectBase

		err = rows.Scan(&count, &post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)

		if err != nil {
			return nColl, util.WrapError(err)
		}

		post.Sticky = true
		post.Locked, _ = post.IsLocked()
		post.Actor = actor.Id

		post.Replies, err = post.GetRepliesLimit(5)
		if err != nil {
			return nColl, err
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.WrapError(err)
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nColl, err
			}
		}

		result = append(result, post)
	}

	nColl.TotalItems = count
	nColl.OrderedItems = result

	return nColl, nil
}
