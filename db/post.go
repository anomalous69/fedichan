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
	"github.com/KushBlazingJudah/fedichan/internal/rx"
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
	match := rx.Cite.FindAllStringSubmatch(comment, -1)

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

func ParseLinkTitle(actorName string, op string, content string) string {
	// TODO: Rewrite using ReplaceFunc
	match := rx.LinkTitle.FindAllStringSubmatch(content, -1)

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

func ParseContent(board activitypub.Actor, op string, content string, thread activitypub.ObjectBase, id string, trunc bool) (template.HTML, error) {
	content = template.HTMLEscapeString(content)
	if trunc {
		content = ParseTruncate(content, board, op, id)
	}

	content = ParseLinkComments(board, op, content, thread)
	content = ParseCommentQuotes(content)

	content = strings.ReplaceAll(content, "\r", "")
	content = strings.ReplaceAll(content, "\n", "<br/>")

	return template.HTML(content), nil
}

func ParseTruncate(content string, board activitypub.Actor, op string, id string) string {
	if strings.Count(content, "\n") > 30 {
		content = strings.Join(rx.Newline.Split(content, 30), "\n")

		content += fmt.Sprintf("<a href=\"%s\">(view full post...)</a>", board.Id+"/"+shortURL(board.Outbox, op)+"#"+shortURL(board.Outbox, id))
	}

	return content
}

func ParseLinkComments(board activitypub.Actor, op string, content string, thread activitypub.ObjectBase) string {
	return rx.CiteEsc.ReplaceAllStringFunc(content, func(match string) string {
		v := rx.CiteEsc.FindStringSubmatch(match)

		isOP := ""
		domain := v[2]
		link := strings.Replace(v[0], "&gt;&gt;", "", 1)

		if link == op {
			isOP = " (OP)"
		}

		parsedLink := ConvertHashLink(domain, link)

		/* TODO: Broken until I fix it again.
		//format the hover title text
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
		*/

		if replyID, isReply, err := IsReplyToOP(op, parsedLink); err == nil || !isReply {
			id := shortURL(board.Outbox, replyID)

			return fmt.Sprintf(`<a class="reply" ` /*title="%s" */ +`href="/%s/%s#%s">&gt;&gt;%s%s</a>` /*, quoteTitle*/, board.Name, shortURL(board.Outbox, op), id, id, isOP)
		}

		//this is a cross post
		parsedOP, err := GetReplyOP(parsedLink)
		if err == nil {
			link = parsedOP + "#" + shortURL(parsedOP, parsedLink)
		}

		actor, err := activitypub.FingerActor(parsedLink)
		if err == nil && actor.Id != "" {
			return fmt.Sprintf(`<a class="reply" ` /*title="%s" */ +`href="%s">&gt;&gt;%s%s →</a>` /*, quoteTitle*/, link, shortURL(board.Outbox, parsedLink), isOP)
		}

		return fmt.Sprintf(`<a class="reply dead">&gt;&gt;%s</a>`, link)
	})
}

func ParseCommentQuotes(content string) string {
	return rx.Quote.ReplaceAllString(content, `<span class="quote">&gt;$1</span>`)
}
