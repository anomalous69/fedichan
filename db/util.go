package db

// Used to avoid a dependance on util.

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"github.com/KushBlazingJudah/fedichan/internal/rx"
)

func wrapErr(err error) error {
	if err == nil {
		return nil
	}

	_, file, l, ok := runtime.Caller(1)
	if !ok {
		// Shouldn't happen
		return err
	}

	return fmt.Errorf("%s:%d: %v", file, l, err)
}

func has(haystack []string, needle string) bool {
	for _, v := range haystack {
		if needle == v {
			return true
		}
	}
	return false
}

func stripTransferProtocol(value string) string {
	re := regexp.MustCompile("(http://|https://)?(www.)?")
	value = re.ReplaceAllString(value, "")

	return value
}

func shortURL(actorName string, url string) string {
	var reply string

	re := regexp.MustCompile(`.+\/`)
	actor := re.FindString(actorName)
	urlParts := strings.Split(url, "|")
	op := urlParts[0]

	if len(urlParts) > 1 {
		reply = urlParts[1]
	}

	temp := rx.WordCharsToEnd.ReplaceAllString(op, "")

	if temp == actor {
		id := localShort(op)

		re := regexp.MustCompile(`.+\/`)
		replyCheck := re.FindString(reply)

		if reply != "" && replyCheck == actor {
			id = id + "#" + localShort(reply)
		} else if reply != "" {
			id = id + "#" + remoteShort(reply)
		}

		return id
	} else {
		id := remoteShort(op)

		re := regexp.MustCompile(`.+\/`)
		replyCheck := re.FindString(reply)

		if reply != "" && replyCheck == actor {
			id = id + "#" + localShort(reply)
		} else if reply != "" {
			id = id + "#" + remoteShort(reply)
		}

		return id
	}
}

func localShort(url string) string {
	return rx.WordCharsToEnd.FindString(stripTransferProtocol(url))
}

func remoteShort(url string) string {
	id := rx.WordCharsToEnd.FindString(stripTransferProtocol(url))
	re := regexp.MustCompile(`.+/.+/`)
	actorurl := re.FindString(stripTransferProtocol(url))
	re = regexp.MustCompile(`/.+/`)
	actorname := re.FindString(actorurl)
	actorname = strings.Replace(actorname, "/", "", -1)

	return "f" + actorname + "-" + id
}
