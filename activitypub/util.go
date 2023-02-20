package activitypub

import (
	"encoding/json"
	"fmt"
	"mime/multipart"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/util"
	"github.com/gofiber/fiber/v2"
)

// False positive for application/ld+ld, application/activity+ld, application/json+json
var activityRegexp = regexp.MustCompile(`application/(ld|json|activity)((\+(ld|json))|$)`)

func AcceptActivity(header string) bool {
	accept := false
	if strings.Contains(header, ";") {
		split := strings.Split(header, ";")
		accept = accept || activityRegexp.MatchString(split[0])
		accept = accept || strings.Contains(split[len(split)-1], "profile=\"https://www.w3.org/ns/activitystreams\"")
	} else {
		accept = accept || activityRegexp.MatchString(header)
	}
	return accept
}

func CreateAttachmentObject(file multipart.File, header *multipart.FileHeader) ([]ObjectBase, *os.File, error) {
	contentType, err := util.GetFileContentType(file)
	if err != nil {
		return nil, nil, util.WrapError(err)
	}

	filename := header.Filename
	size := header.Size

	fileType := path.Ext(header.Filename)
	name := fmt.Sprint(time.Now().UTC().Unix())

	tempFile, err := os.Create(fmt.Sprintf("./public/%s%s", name, fileType))
	if err != nil {
		return nil, nil, util.WrapError(err)
	}

	var nAttachment []ObjectBase
	var image ObjectBase

	image.Type = "Attachment"
	image.Name = filename
	image.Href = fmt.Sprintf("%s/public/%s%s", config.Domain, name, fileType)
	image.MediaType = contentType
	image.Size = size
	image.Published = time.Now().UTC()

	nAttachment = append(nAttachment, image)

	return nAttachment, tempFile, nil
}

func CreateNewActor(board string, prefName string, summary string, restricted bool) *Actor {
	actor := new(Actor)

	var path string
	if board == "" {
		path = config.Domain
		actor.Name = "main"
	} else {
		path = config.Domain + "/" + board
		actor.Name = board
	}

	actor.Type = "Group"
	actor.Id = path
	actor.Following = fmt.Sprintf("%s/following", actor.Id)
	actor.Followers = fmt.Sprintf("%s/followers", actor.Id)
	actor.Inbox = fmt.Sprintf("%s/inbox", actor.Id)
	actor.Outbox = fmt.Sprintf("%s/outbox", actor.Id)
	actor.PreferredUsername = prefName
	actor.Restricted = restricted
	actor.Summary = summary

	return actor
}

func CreateObject(objType string) ObjectBase {
	var nObj ObjectBase

	now := time.Now().UTC()
	nObj.Type = objType
	nObj.Published = now
	nObj.Updated = &now

	return nObj
}

func GetActivityFromJson(ctx *fiber.Ctx) (Activity, error) {
	var respActivity ActivityRaw
	var nActivity Activity
	var nType string

	if err := json.Unmarshal(ctx.Body(), &respActivity); err != nil {
		return nActivity, util.WrapError(err)
	}

	if res, err := HasContextFromJson(respActivity.AtContextRaw.Context); err == nil && res {
		var jObj ObjectBase

		if respActivity.Type == "Note" {
			jObj, err = GetObjectFromJson(ctx.Body())
			if err != nil {
				return nActivity, util.WrapError(err)
			}

			nType = "Create"
		} else {
			jObj, err = GetObjectFromJson(respActivity.ObjectRaw)
			if err != nil {
				return nActivity, util.WrapError(err)
			}

			nType = respActivity.Type
		}

		actor, err := GetActorFromJson(respActivity.ActorRaw)
		if err != nil {
			return nActivity, util.WrapError(err)
		}

		to, err := GetToFromJson(respActivity.ToRaw)
		if err != nil {
			return nActivity, util.WrapError(err)
		}

		cc, err := GetToFromJson(respActivity.CcRaw)
		if err != nil {
			return nActivity, util.WrapError(err)
		}

		nActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
		nActivity.Type = nType
		nActivity.Actor = &actor
		nActivity.Published = respActivity.Published

		if len(to) > 0 {
			nActivity.To = to
		}

		if len(cc) > 0 {
			nActivity.Cc = cc
		}

		nActivity.Name = respActivity.Name
		nActivity.Object = jObj
	} else if err != nil {
		return nActivity, util.WrapError(err)
	}

	return nActivity, nil
}

func GetObjectFromJson(obj []byte) (ObjectBase, error) {
	var generic interface{}
	var nObj ObjectBase

	if err := json.Unmarshal(obj, &generic); err != nil {
		return ObjectBase{}, util.WrapError(err)
	}

	if generic != nil {
		switch generic.(type) {
		case []interface{}:
			var lObj ObjectBase
			var arrContext ObjectArray

			if err := json.Unmarshal(obj, &arrContext.Object); err != nil {
				return nObj, util.WrapError(err)
			}

			if len(arrContext.Object) > 0 {
				lObj = arrContext.Object[0]
			}
			nObj = lObj

		case map[string]interface{}:
			var arrContext Object

			if err := json.Unmarshal(obj, &arrContext.Object); err != nil {
				return nObj, util.WrapError(err)
			}

			nObj = *arrContext.Object

		case string:
			var lObj ObjectBase
			var arrContext ObjectString

			if err := json.Unmarshal(obj, &arrContext.Object); err != nil {
				return nObj, util.WrapError(err)
			}

			lObj.Id = arrContext.Object
			nObj = lObj
		}
	}

	return nObj, nil
}

func HasContextFromJson(context []byte) (bool, error) {
	var generic interface{}

	err := json.Unmarshal(context, &generic)
	if err != nil {
		return false, util.WrapError(err)
	}

	hasContext := false

	switch generic.(type) {
	case []interface{}:
		var arrContext AtContextArray
		err = json.Unmarshal(context, &arrContext.Context)
		if len(arrContext.Context) > 0 {
			if arrContext.Context[0] == "https://www.w3.org/ns/activitystreams" {
				hasContext = true
			}
		}

	case string:
		var arrContext AtContextString
		err = json.Unmarshal(context, &arrContext.Context)
		if arrContext.Context == "https://www.w3.org/ns/activitystreams" {
			hasContext = true
		}
	}

	return hasContext, util.WrapError(err)
}

func GetActorByNameFromDB(name string) (Actor, error) {
	var nActor Actor
	var publicKeyPem string

	query := `select type, id, name, preferedusername, inbox, outbox, following, followers, restricted, summary, publickeypem from actor where name=$1`
	err := config.DB.QueryRow(query, name).Scan(&nActor.Type, &nActor.Id, &nActor.Name, &nActor.PreferredUsername, &nActor.Inbox, &nActor.Outbox, &nActor.Following, &nActor.Followers, &nActor.Restricted, &nActor.Summary, &publicKeyPem)

	if err != nil {
		return nActor, util.WrapError(err)
	}

	nActor.PublicKey, err = GetActorPemFromDB(publicKeyPem)

	if err != nil {
		return nActor, util.WrapError(err)
	}

	if nActor.Id != "" && nActor.PublicKey == nil {
		if err := CreatePublicKeyFromPrivate(&nActor, publicKeyPem); err != nil {
			return nActor, util.WrapError(err)
		}
	}

	return nActor, nil
}

func GetActorFromDB(id string) (Actor, error) {
	var nActor Actor
	var publicKeyPem string

	query := `select type, id, name, preferedusername, inbox, outbox, following, followers, restricted, summary, publickeypem from actor where id=$1`
	err := config.DB.QueryRow(query, id).Scan(&nActor.Type, &nActor.Id, &nActor.Name, &nActor.PreferredUsername, &nActor.Inbox, &nActor.Outbox, &nActor.Following, &nActor.Followers, &nActor.Restricted, &nActor.Summary, &publicKeyPem)

	if err != nil {
		return nActor, util.WrapError(err)
	}

	nActor.PublicKey, err = GetActorPemFromDB(publicKeyPem)

	if err != nil {
		return nActor, util.WrapError(err)
	}

	if nActor.Id != "" && nActor.PublicKey.PublicKeyPem == "" {
		if err := CreatePublicKeyFromPrivate(&nActor, publicKeyPem); err != nil {
			return nActor, util.WrapError(err)
		}
	}

	return nActor, nil
}

func GetActorFromJson(actor []byte) (Actor, error) {
	var generic interface{}
	var nActor Actor
	err := json.Unmarshal(actor, &generic)
	if err != nil {
		return nActor, util.WrapError(err)
	}

	if generic != nil {
		switch generic.(type) {
		case map[string]interface{}:
			err = json.Unmarshal(actor, &nActor)

		case string:
			var str string
			err = json.Unmarshal(actor, &str)
			nActor.Id = str
		}

		return nActor, util.WrapError(err)
	}

	return nActor, nil
}

func GetToFromJson(to []byte) ([]string, error) {
	var generic interface{}

	if len(to) == 0 {
		return nil, nil
	}

	err := json.Unmarshal(to, &generic)
	if err != nil {
		return nil, util.WrapError(err)
	}

	if generic != nil {
		var nStr []string
		switch generic.(type) {
		case []interface{}:
			err = json.Unmarshal(to, &nStr)
		case string:
			var str string
			err = json.Unmarshal(to, &str)
			nStr = append(nStr, str)
		}
		return nStr, util.WrapError(err)
	}

	return nil, nil
}

func GetActorAndInstance(path string) (string, string) {
	re := regexp.MustCompile(`([@]?([\w\d.-_]+)[@](.+))`)
	atFormat := re.MatchString(path)

	if atFormat {
		match := re.FindStringSubmatch(path)
		if len(match) > 2 {
			return match[2], match[3]
		}
	}

	re = regexp.MustCompile(`(https?://)(www)?([\w\d-_.:]+)(/|\s+|\r|\r\n)?$`)
	mainActor := re.MatchString(path)
	if mainActor {
		match := re.FindStringSubmatch(path)
		if len(match) > 2 {
			return "main", match[3]
		}
	}

	re = regexp.MustCompile(`(https?://)?(www)?([\w\d-_.:]+)\/([\w\d-_.]+)(\/([\w\d-_.]+))?`)
	httpFormat := re.MatchString(path)

	if httpFormat {
		match := re.FindStringSubmatch(path)
		if len(match) > 3 {
			if match[4] == "users" {
				return match[6], match[3]
			}

			return match[4], match[3]
		}
	}

	return "", ""
}
