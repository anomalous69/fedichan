package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/KushBlazingJudah/fedichan/config"
)

const keyDomain = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
const captchaDomain = "ABEFHKMNPQRSUVWXYZ#$&"
const keyLength = 8
const captchaLength = 4
const targetCaptchas = 50

var randomKey = randomGen(keyDomain, keyLength)
var mkCaptcha = randomGen(captchaDomain, captchaLength)
var sbPool = &sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
}

var captchaPtns = []string{
	"pattern:verticalbricks",
	"pattern:verticalsaw",
	"pattern:hs_cross",
}

func randomGen(domain string, defaultSize int) func(fsize ...int) string {
	return func(fsize ...int) string {
		size := defaultSize
		if len(fsize) > 0 {
			size = fsize[0]
		}

		out := sbPool.Get().(*strings.Builder)
		defer sbPool.Put(out)
		out.Reset()

		for ; size > 0; size-- {
			out.WriteByte(domain[rand.Intn(len(domain))])
		}

		return out.String()
	}
}

func runConvert(args ...string) error {
	cmd := exec.Command("convert", args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func createNewCaptcha() error {
	id := randomKey()
	solution := mkCaptcha()
	file := fmt.Sprintf("public/%s.jpg", id)
	pattern := captchaPtns[rand.Intn(len(captchaPtns))]

	if err := runConvert("-size", "200x98", pattern, "-transparent", "white", file); err != nil {
		os.Remove(file)
		return wrapErr(err)
	}

	if err := runConvert(file, "-fill", "blue", "-pointsize", "62", "-annotate", "+0+70", solution, "-tile", "pattern:left30", "-gravity", "center", "-transparent", "white", file); err != nil {
		os.Remove(file)
		return wrapErr(err)
	}

	if err := runConvert(file, "-rotate", fmt.Sprint(rand.Intn(12)), "-wave", "5x35", "-distort", "Arc", "20", "-wave", "2x35", "-transparent", "white", file); err != nil {
		os.Remove(file)
		return wrapErr(err)
	}

	_, err := config.DB.Exec(`insert into captchas(file, id, solution) values($1, $2, $3)`, file, id, solution)
	if err != nil {
		// The database didn't save our captcha, so delete the file
		os.Remove(file)
	}

	return wrapErr(err)
}

func getCaptchaSolution(id string) (string, error) {
	solution := ""

	return solution, config.DB.QueryRow(`select solution from captchas where id = $1`, id).Scan(&solution)
}

func deleteCaptcha(id string) error {
	path := ""

	if err := config.DB.QueryRow(`select file from captchas where id=$1`, id).Scan(&path); err != nil {
		return err
	}

	if _, err := config.DB.Exec(`delete from captcha where id=$1`, id); err != nil {
		return wrapErr(err)
	}

	// This should be removed but don't fail if it doesn't
	os.Remove(path)

	return nil
}

// GetCaptcha returns a random captcha from the database.
//
// The return values are the file path and the ID of the captcha.
func GetCaptcha() (string, string, error) {
	file, id := "", ""

	return file, id, config.DB.QueryRow(`select file, id from captchas order by random()`).Scan(&file, &id)
}

// GetCaptchaTotal returns the total number of captchas available for use in
// the database.
func GetCaptchaTotal() (int, error) {
	count := 0

	return count, config.DB.QueryRow(`select count(*) from captchas`).Scan(&count)
}

// MakeCaptcha generates new captchas.
func MakeCaptchas() error {
	total, err := GetCaptchaTotal()
	if err != nil {
		return wrapErr(err)
	}

	for need := targetCaptchas - total; need > 0; need-- {
		if err := createNewCaptcha(); err != nil {
			return wrapErr(err)
		}
	}

	return nil
}

func CheckCaptcha(id, sol string) (bool, error) {
	solution, err := getCaptchaSolution(id)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		// If there's no data, then there isn't really an error as the
		// user provided the wrong captcha ID.
		return false, nil
	} else if err != nil {
		return false, wrapErr(err)
	}

	if solution == strings.ToUpper(sol) {
		// Ignoring error as not completely fatal but far from ideal
		deleteCaptcha(id)

		// Replenish the used captcha
		createNewCaptcha()

		return true, nil
	}

	return false, nil
}
