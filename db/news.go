package db

import (
	"database/sql"
	"html/template"
	"strings"
	"time"

	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/KushBlazingJudah/fedichan/util"
)

type NewsItem struct {
	Title   string
	Content template.HTML
	Time    int
}

// if limit less than 1 return all news items
func GetNews(limit int) ([]NewsItem, error) {
	var news []NewsItem
	var query string

	var rows *sql.Rows
	var err error

	if limit > 0 {
		query = `select title, content, time from newsItem order by time desc limit $1`
		rows, err = config.DB.Query(query, limit)
	} else {
		query = `select title, content, time from newsItem order by time desc`
		rows, err = config.DB.Query(query)
	}

	if err != nil {
		return news, util.MakeError(err, "GetNews")
	}

	defer rows.Close()
	for rows.Next() {
		var content string
		n := NewsItem{}

		if err := rows.Scan(&n.Title, &content, &n.Time); err != nil {
			return news, util.MakeError(err, "GetNews")
		}

		content = strings.ReplaceAll(content, "\n", "<br>")
		n.Content = template.HTML(content)

		news = append(news, n)
	}

	return news, nil
}

func GetNewsItem(timestamp int) (NewsItem, error) {
	var news NewsItem
	var content string

	query := `select title, content, time from newsItem where time=$1 limit 1`
	if err := config.DB.QueryRow(query, timestamp).Scan(&news.Title, &content, &news.Time); err != nil {
		return news, util.MakeError(err, "GetNewsItem")
	}

	content = strings.ReplaceAll(content, "\n", "<br>")
	news.Content = template.HTML(content)

	return news, nil
}

func DeleteNewsItem(timestamp int) error {
	query := `delete from newsItem where time=$1`
	_, err := config.DB.Exec(query, timestamp)

	return util.MakeError(err, "DeleteNewsItem")
}

func WriteNews(news NewsItem) error {
	query := `insert into newsItem (title, content, time) values ($1, $2, $3)`
	_, err := config.DB.Exec(query, news.Title, news.Content, time.Now().Unix())

	return util.MakeError(err, "WriteNews")
}
