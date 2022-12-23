package db

import (
	"fmt"
	"html/template"
	"mime/multipart"
	"regexp"
	"strings"
	"time"

	"github.com/KushBlazingJudah/fedichan/activitypub"
	"github.com/KushBlazingJudah/fedichan/config"
)

func ConvertHashLink(domain string, link string) string {
	re := regexp.MustCompile(`(#.+)`)
	parsedLink := re.FindString(link)

	if parsedLink != "" {
		parsedLink = domain + "" + strings.Replace(parsedLink, "#", "", 1)
		parsedLink = strings.Replace(parsedLink, "\r", "", -1)
	} else {
		parsedLink = link
	}

	return parsedLink
}

func ParseCommentForReplies(comment string, op string) ([]activitypub.ObjectBase, error) {
	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
	match := re.FindAllStringSubmatch(comment, -1)

	var links []string

	for i := 0; i < len(match); i++ {
		str := strings.Replace(match[i][0], ">>", "", 1)
		str = strings.Replace(str, "www.", "", 1)
		str = strings.Replace(str, "http://", "", 1)
		str = strings.Replace(str, "https://", "", 1)
		str = config.TP + "" + str
		_, isReply, err := IsReplyToOP(op, str)

		if err != nil {
			return nil, wrapErr(err)
		}

		if !has(links, str) && isReply {
			links = append(links, str)
		}
	}

	var validLinks []activitypub.ObjectBase
	for i := 0; i < len(links); i++ {
		reqActivity := activitypub.Activity{Id: links[i]}
		_, isValid, err := reqActivity.CheckValid()

		if err != nil {
			return nil, wrapErr(err)
		}

		if isValid {
			var reply activitypub.ObjectBase

			reply.Id = links[i]
			reply.Published = time.Now().UTC()
			validLinks = append(validLinks, reply)
		}
	}

	return validLinks, nil
}

func ParseCommentForReply(comment string) (string, error) {
	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
	match := re.FindAllStringSubmatch(comment, -1)

	var links []string

	for i := 0; i < len(match); i++ {
		str := strings.Replace(match[i][0], ">>", "", 1)
		links = append(links, str)
	}

	if len(links) > 0 {
		reqActivity := activitypub.Activity{Id: strings.ReplaceAll(links[0], ">", "")}
		_, isValid, err := reqActivity.CheckValid()

		if err != nil {
			return "", wrapErr(err)
		}

		if isValid {
			return links[0], nil
		}
	}

	return "", nil
}

func ParseLinkTitle(actorName string, op string, content string) string {
	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)\w+(#.+)?)`)
	match := re.FindAllStringSubmatch(content, -1)

	for i := range match {
		link := strings.Replace(match[i][0], ">>", "", 1)
		isOP := ""

		domain := match[i][2]

		if link == op {
			isOP = " (OP)"
		}

		link = ConvertHashLink(domain, link)
		content = strings.Replace(content, match[i][0], ">>"+shortURL(actorName, link)+isOP, 1)
	}

	content = strings.ReplaceAll(content, "'", "&#39;")
	content = strings.ReplaceAll(content, "\"", "&quot;")
	content = strings.ReplaceAll(content, ">", `/\&lt;`)

	return content
}

func IsMediaBanned(f multipart.File) (bool, error) {
	/* TODO: Rewrite
	f.Seek(0, 0)
	fileBytes := make([]byte, 2048)
	_, err := f.Read(fileBytes)

	if err != nil {
		return true, wrapErr(err)
	}

	hash := util.HashBytes(fileBytes)
	f.Seek(0, 0)

	return IsHashBanned(hash)
	*/

	return false, nil
}

func ParseContent(board activitypub.Actor, op string, content string, thread activitypub.ObjectBase, id string, _type string) (template.HTML, error) {
	// TODO: should escape more than just < and >, should also escape &, ", and '
	nContent := strings.ReplaceAll(content, `<`, "&lt;")

	if _type == "new" {
		nContent = ParseTruncate(nContent, board, op, id)
	}

	nContent, err := ParseLinkComments(board, op, nContent, thread)

	if err != nil {
		return "", wrapErr(err)
	}

	nContent = ParseCommentQuotes(nContent)
	nContent = strings.ReplaceAll(nContent, `/\&lt;`, ">")

	return template.HTML(nContent), nil
}

func ParseTruncate(content string, board activitypub.Actor, op string, id string) string {
	if strings.Count(content, "\r") > 30 {
		content = strings.ReplaceAll(content, "\r\n", "\r")
		lines := strings.SplitAfter(content, "\r")
		content = ""

		for i := 0; i < 30; i++ {
			content += lines[i]
		}

		content += fmt.Sprintf("<a href=\"%s\">(view full post...)</a>", board.Id+"/"+shortURL(board.Outbox, op)+"#"+shortURL(board.Outbox, id))
	}

	return content
}

func ParseLinkComments(board activitypub.Actor, op string, content string, thread activitypub.ObjectBase) (string, error) {
	re := regexp.MustCompile(`(>>(https?://[A-Za-z0-9_.:\-~]+\/[A-Za-z0-9_.\-~]+\/)(f[A-Za-z0-9_.\-~]+-)?([A-Za-z0-9_.\-~]+)?#?([A-Za-z0-9_.\-~]+)?)`)
	match := re.FindAllStringSubmatch(content, -1)

	//add url to each matched reply
	for i := range match {
		isOP := ""
		domain := match[i][2]
		link := strings.Replace(match[i][0], ">>", "", 1)

		if link == op {
			isOP = " (OP)"
		}

		parsedLink := ConvertHashLink(domain, link)

		//formate the hover title text
		var quoteTitle string

		// if the quoted content is local get it
		// else get it from the database
		if thread.Id == link {
			quoteTitle = ParseLinkTitle(board.Outbox, op, thread.Content)
		} else {
			for _, e := range thread.Replies.OrderedItems {
				if e.Id == parsedLink {
					quoteTitle = ParseLinkTitle(board.Outbox, op, e.Content)
					break
				}
			}

			if quoteTitle == "" {
				obj := activitypub.ObjectBase{Id: parsedLink}
				col, err := obj.GetCollectionFromPath()
				if err == nil {
					if len(col.OrderedItems) > 0 {
						quoteTitle = ParseLinkTitle(board.Outbox, op, col.OrderedItems[0].Content)
					} else {
						quoteTitle = ParseLinkTitle(board.Outbox, op, parsedLink)
					}
				}
			}
		}

		if replyID, isReply, err := IsReplyToOP(op, parsedLink); err == nil || !isReply {
			id := shortURL(board.Outbox, replyID)

			content = strings.Replace(content, match[i][0], "<a class=\"reply\" title=\""+quoteTitle+"\" href=\"/"+board.Name+"/"+shortURL(board.Outbox, op)+"#"+id+"\">&gt;&gt;"+id+""+isOP+"</a>", -1)
		} else {
			//this is a cross post

			parsedOP, err := GetReplyOP(parsedLink)
			if err == nil {
				link = parsedOP + "#" + shortURL(parsedOP, parsedLink)
			}

			actor, err := activitypub.FingerActor(parsedLink)
			if err == nil && actor.Id != "" {
				content = strings.Replace(content, match[i][0], "<a class=\"reply\" title=\""+quoteTitle+"\" href=\""+link+"\">&gt;&gt;"+shortURL(board.Outbox, parsedLink)+isOP+" →</a>", -1)
			}
		}
	}

	return content, nil
}

func ParseCommentQuotes(content string) string {
	// replace quotes
	re := regexp.MustCompile(`((\r\n|\r|\n|^)>(.+)?[^\r\n])`)
	match := re.FindAllStringSubmatch(content, -1)

	for i := range match {
		quote := strings.Replace(match[i][0], ">", "&gt;", 1)
		line := re.ReplaceAllString(match[i][0], "<span class=\"quote\">"+quote+"</span>")
		content = strings.Replace(content, match[i][0], line, 1)
	}

	//replace isolated greater than symboles
	re = regexp.MustCompile(`(\r\n|\n|\r)>`)

	return re.ReplaceAllString(content, "\r\n<span class=\"quote\">&gt;</span>")
}
