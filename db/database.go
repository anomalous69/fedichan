package db

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/KushBlazingJudah/fedichan/activitypub"
	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/util"
	_ "github.com/lib/pq"
)

func Connect() error {
	host := config.DBHost
	port := config.DBPort
	user := config.DBUser
	password := config.DBPassword
	dbname := config.DBName

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s "+
		"dbname=%s sslmode=disable", host, port, user, password, dbname)

	_db, err := sql.Open("postgres", psqlInfo)

	if err != nil {
		return util.MakeError(err, "Connect")
	}

	if err := _db.Ping(); err != nil {
		return util.MakeError(err, "Connect")
	}

	config.Log.Println("Successfully connected DB")

	config.DB = _db

	return nil
}

func Close() error {
	err := config.DB.Close()

	return util.MakeError(err, "Close")
}

func RunDatabaseSchema() error {
	query, err := os.ReadFile("db/schema.psql")
	if err != nil {
		return util.MakeError(err, "RunDatabaseSchema")
	}

	_, err = config.DB.Exec(string(query))
	return util.MakeError(err, "RunDatabaseSchema")
}

func CreateNewBoard(actor activitypub.Actor) (activitypub.Actor, error) {
	if _, err := activitypub.GetActorFromDB(actor.Id); err == nil {
		return activitypub.Actor{}, util.MakeError(err, "CreateNewBoardDB")
	} else {
		query := `insert into actor (type, id, name, preferedusername, inbox, outbox, following, followers, summary, restricted) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
		_, err := config.DB.Exec(query, actor.Type, actor.Id, actor.Name, actor.PreferredUsername, actor.Inbox, actor.Outbox, actor.Following, actor.Followers, actor.Summary, actor.Restricted)

		if err != nil {
			return activitypub.Actor{}, util.MakeError(err, "CreateNewBoardDB")
		}

		config.Log.Println("board added")

		for _, e := range actor.AuthRequirement {
			query = `insert into actorauth (type, board) values ($1, $2)`
			if _, err := config.DB.Exec(query, e, actor.Name); err != nil {
				return activitypub.Actor{}, util.MakeError(err, "CreateNewBoardDB")
			}
		}

		if actor.Id == config.Domain {
			var verify util.Verify
			verify.Type = "admin"
			verify.Identifier = actor.Id

			if err := actor.CreateVerification(verify); err != nil {
				return activitypub.Actor{}, util.MakeError(err, "CreateNewBoardDB")
			}
		}

		activitypub.CreatePem(actor)

		if actor.Name != "main" {
			var nObject activitypub.ObjectBase
			var nActivity activitypub.Activity

			nActor, err := activitypub.GetActorFromDB(config.Domain)

			if err != nil {
				return actor, util.MakeError(err, "CreateNewBoardDB")
			}

			nActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
			nActivity.Type = "Follow"
			nActivity.Actor = &nActor
			nActivity.Object = nObject
			mActor, err := activitypub.GetActorFromDB(actor.Id)

			if err != nil {
				return actor, util.MakeError(err, "CreateNewBoardDB")
			}

			nActivity.Object.Actor = mActor.Id
			nActivity.To = append(nActivity.To, actor.Id)

			activityRequest := nActivity.AcceptFollow()

			if _, err := activityRequest.SetActorFollowing(); err != nil {
				return actor, util.MakeError(err, "CreateNewBoardDB")
			}

			if err := activityRequest.MakeRequestInbox(); err != nil {
				return actor, util.MakeError(err, "CreateNewBoardDB")
			}
		}
	}

	return actor, nil
}

func RemovePreviewFromFile(id string) error {
	var href string

	query := `select href from activitystream where id in (select preview from activitystream where id=$1)`
	if err := config.DB.QueryRow(query, id).Scan(&href); err != nil {
		return nil
	}

	href = strings.Replace(href, config.Domain+"/", "", 1)

	if href != "static/notfound.png" {
		if _, err := os.Stat(href); err != nil {
			return util.MakeError(err, "RemovePreviewFromFile")
		}

		err := os.Remove(href)
		return util.MakeError(err, "RemovePreviewFromFile")
	}

	obj := activitypub.ObjectBase{Id: id}
	err := obj.DeletePreview()
	return util.MakeError(err, "RemovePreviewFromFile")
}

func AddInstanceToInactive(instance string) error {
	var timeStamp string

	query := `select timestamp from inactive where instance=$1`
	if err := config.DB.QueryRow(query, instance).Scan(&timeStamp); err != nil {
		query := `insert into inactive (instance, timestamp) values ($1, $2)`
		_, err := config.DB.Exec(query, instance, time.Now().UTC().Format(time.RFC3339))

		return util.MakeError(err, "AddInstanceToInactive")
	}

	if !IsInactiveTimestamp(timeStamp) {
		return nil
	}

	query = `delete from follower where follower like $1`
	if _, err := config.DB.Exec(query, "%"+instance+"%"); err != nil {
		return util.MakeError(err, "AddInstanceToInactive")
	}

	err := DeleteInstanceFromInactive(instance)
	return util.MakeError(err, "AddInstanceToInactive")
}

func DeleteInstanceFromInactive(instance string) error {
	query := `delete from inactive where instance=$1`
	_, err := config.DB.Exec(query, instance)

	return util.MakeError(err, "DeleteInstanceFromInactive")
}

func IsInactiveTimestamp(timeStamp string) bool {
	stamp, _ := time.Parse(time.RFC3339, timeStamp)

	return time.Now().UTC().Sub(stamp).Hours() > 48
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

func CheckInactive() {
	for {
		CheckInactiveInstances()
		time.Sleep(24 * time.Hour)
	}
}

func CheckInactiveInstances() (map[string]string, error) {
	var rows *sql.Rows
	var err error

	instances := make(map[string]string)

	query := `select following from following`
	if rows, err = config.DB.Query(query); err != nil {
		return instances, util.MakeError(err, "CheckInactiveInstances")
	}

	defer rows.Close()
	for rows.Next() {
		var instance string

		if err := rows.Scan(&instance); err != nil {
			return instances, util.MakeError(err, "CheckInactiveInstances")
		}

		instances[instance] = instance
	}

	query = `select follower from follower`
	if rows, err = config.DB.Query(query); err != nil {
		return instances, util.MakeError(err, "CheckInactiveInstances")
	}

	defer rows.Close()
	for rows.Next() {
		var instance string

		if err := rows.Scan(&instance); err != nil {
			return instances, util.MakeError(err, "CheckInactiveInstances")
		}

		instances[instance] = instance
	}

	re := regexp.MustCompile(config.Domain + `(.+)?`)

	for _, e := range instances {
		actor, err := activitypub.GetActor(e)

		if err != nil {
			return instances, util.MakeError(err, "CheckInactiveInstances")
		}

		if actor.Id == "" && !re.MatchString(e) {
			if err := AddInstanceToInactive(e); err != nil {
				return instances, util.MakeError(err, "CheckInactiveInstances")
			}
		} else {
			if err := DeleteInstanceFromInactive(e); err != nil {
				return instances, util.MakeError(err, "CheckInactiveInstances")
			}
		}
	}

	return instances, nil
}

func GetAdminAuth() (string, string, error) {
	var code string
	var identifier string

	query := `select identifier, code from boardaccess where board=$1 and type='admin'`
	if err := config.DB.QueryRow(query, config.Domain).Scan(&identifier, &code); err != nil {
		return "", "", nil
	}

	return code, identifier, nil
}

func IsHashBanned(hash string) (bool, error) {
	var h string

	query := `select hash from bannedmedia where hash=$1`
	_ = config.DB.QueryRow(query, hash).Scan(&h)

	return h == hash, nil
}

func PrintAdminAuth() error {
	code, identifier, err := GetAdminAuth()

	if err != nil {
		return util.MakeError(err, "PrintAdminAuth")
	}

	config.Log.Println("Mod key: " + config.Key)
	config.Log.Println("Admin Login: " + identifier + ", Code: " + code)
	return nil
}

func InitInstance() error {
	if config.InstanceName != "" {
		if _, err := CreateNewBoard(*activitypub.CreateNewActor("", config.InstanceName, config.InstanceSummary, config.AuthReq, false)); err != nil {
			return util.MakeError(err, "InitInstance")
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
			return "", util.MakeError(err, "GetPostIDFromNum")
		}
	}

	return postID, nil
}
