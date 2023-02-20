package db

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"log"
	"regexp"

	"github.com/KushBlazingJudah/fedichan/activitypub"
	"github.com/KushBlazingJudah/fedichan/config"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const pwDomain = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const pwLength = 24

func Connect() error {
	host := config.DBHost
	port := config.DBPort
	user := config.DBUser
	password := config.DBPassword
	dbname := config.DBName

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s "+
		"dbname=%s sslmode=disable", host, port, user, password, dbname)

	_db, err := sql.Open("pgx", psqlInfo)
	if err != nil {
		return wrapErr(err)
	}

	if err := _db.Ping(); err != nil {
		return wrapErr(err)
	}

	config.DB = _db

	if err := migrate(); err != nil {
		return err
	}

	log.Println("Successfully connected DB")

	return nil
}

func Close() error {
	err := config.DB.Close()

	return wrapErr(err)
}

func CreateNewBoard(actor activitypub.Actor) (activitypub.Actor, error) {
	if _, err := activitypub.GetActorFromDB(actor.Id); err == nil {
		return activitypub.Actor{}, wrapErr(err)
	}

	query := `insert into actor (type, id, name, preferedusername, inbox, outbox, following, followers, summary, restricted) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	_, err := config.DB.Exec(query, actor.Type, actor.Id, actor.Name, actor.PreferredUsername, actor.Inbox, actor.Outbox, actor.Following, actor.Followers, actor.Summary, actor.Restricted)

	if err != nil {
		return activitypub.Actor{}, wrapErr(err)
	}

	log.Println("board added")

	activitypub.CreatePem(actor)

	if actor.Name != "main" {
		var nObject activitypub.ObjectBase
		var nActivity activitypub.Activity

		nActor, err := activitypub.GetActorFromDB(config.Domain)

		if err != nil {
			return actor, wrapErr(err)
		}

		nActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
		nActivity.Type = "Follow"
		nActivity.Actor = &nActor
		nActivity.Object = nObject
		mActor, err := activitypub.GetActorFromDB(actor.Id)

		if err != nil {
			return actor, wrapErr(err)
		}

		nActivity.Object.Actor = mActor.Id
		nActivity.To = append(nActivity.To, actor.Id)

		activityRequest := nActivity.AcceptFollow(mActor)

		if err := activityRequest.SetActorFollowing(); err != nil {
			return actor, wrapErr(err)
		}

		if err := activityRequest.Send(); err != nil {
			return actor, wrapErr(err)
		}
	}

	return actor, nil
}

func IsReplyToOP(op string, link string) (string, bool, error) {
	var id string

	if op == link {
		return link, true, nil
	}

	re := regexp.MustCompile(`f(\w+)\-`)
	match := re.FindStringSubmatch(link)

	if len(match) > 0 {
		re := regexp.MustCompile(`(.+)\-`)
		link = re.ReplaceAllString(link, "")
		link = "%" + match[1] + "/" + link
	}

	query := `select id from replies where id like $1 and inreplyto=$2`
	if err := config.DB.QueryRow(query, link, op).Scan(&id); err != nil {
		return op, false, nil
	}

	return id, id != "", nil
}

func GetReplyOP(link string) (string, error) {
	var id string

	query := `select id from replies where id in (select inreplyto from replies where id=$1) and inreplyto=''`
	if err := config.DB.QueryRow(query, link).Scan(&id); err != nil {
		return "", nil
	}

	return id, nil
}

func IsHashBanned(hash string) (bool, error) {
	var h string

	query := `select hash from bannedmedia where hash=$1`
	_ = config.DB.QueryRow(query, hash).Scan(&h)

	return h == hash, nil
}

func PrintAdminAuth() error {
	log.Printf("Mod key: %v", config.Key)

	if UserExists("admin") {
		return nil
	}

	a := Acct{Username: "admin", Type: Admin}
	if err := a.Save(); err != nil {
		return err
	}

	pw := make([]byte, pwLength)
	rand.Read(pw)
	for i := range pw {
		pw[i] = pwDomain[int(pw[i])%len(pwDomain)]
	}

	log.Printf("Admin password set to: %v", string(pw))

	return a.SetPassword(string(pw))
}

func InitInstance() error {
	if config.InstanceName != "" {
		if _, err := CreateNewBoard(*activitypub.CreateNewActor("", config.InstanceName, config.InstanceSummary, false)); err != nil {
			return wrapErr(err)
		}
	}

	return nil
}

func GetPostIDFromNum(num string) (string, error) {
	var postID string

	query := `select id from activitystream where id like $1`
	if err := config.DB.QueryRow(query, "%"+num).Scan(&postID); err != nil {
		query = `select id from cacheactivitystream where id like $1`
		if err := config.DB.QueryRow(query, "%"+num).Scan(&postID); err != nil {
			return "", wrapErr(err)
		}
	}

	return postID, nil
}
