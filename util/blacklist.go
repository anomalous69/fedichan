package util

import (
	"regexp"

	"github.com/KushBlazingJudah/fedichan/config"
)

type PostBlacklist struct {
	Id    int
	Regex string
}

func DeleteRegexBlacklist(id int) error {
	query := `delete from postblacklist where id=$1`
	_, err := config.DB.Exec(query, id)

	return WrapError(err)
}

func GetRegexBlacklist() ([]PostBlacklist, error) {
	var list []PostBlacklist

	query := `select id, regex from postblacklist`
	rows, err := config.DB.Query(query)

	if err != nil {
		return list, WrapError(err)
	}

	defer rows.Close()
	for rows.Next() {
		var temp PostBlacklist

		rows.Scan(&temp.Id, &temp.Regex)
		list = append(list, temp)
	}

	return list, nil
}

func IsPostBlacklist(comment string) (bool, error) {
	postblacklist, err := GetRegexBlacklist()

	if err != nil {
		return false, WrapError(err)
	}

	for _, e := range postblacklist {
		re := regexp.MustCompile(e.Regex)

		if re.MatchString(comment) {
			return true, nil
		}
	}

	return false, nil
}

func WriteRegexBlacklist(regex string) error {
	var re string

	query := `select from postblacklist where regex=$1`
	if err := config.DB.QueryRow(query, regex).Scan(&re); err != nil {
		query = `insert into postblacklist (regex) values ($1)`
		_, err := config.DB.Exec(query, regex)
		return WrapError(err)
	}

	return nil
}
