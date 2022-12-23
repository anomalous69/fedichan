package util

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/KushBlazingJudah/fedichan/config"
	"github.com/gofiber/fiber/v2"
	_ "github.com/lib/pq"
)

// Captcha is temporarily broken while I sort out import cycles
// I can't generate them anyway

func Captcha() string {
	rand.Seed(time.Now().UTC().UnixNano())
	domain := "ABEFHKMNPQRSUVWXYZ#$&"
	rng := 4
	newID := ""

	for i := 0; i < rng; i++ {
		newID += string(domain[rand.Intn(len(domain))])
	}

	return newID
}

func CreateNewCaptcha() error {
	id := RandomID(8)
	file := "public/" + id + ".png"

	for {
		if _, err := os.Stat("./" + file); err == nil {
			id = RandomID(8)
			file = "public/" + id + ".png"
		} else {
			break
		}
	}

	var pattern string

	captcha := Captcha()
	rnd := fmt.Sprintf("%d", rand.Intn(3))
	srnd := string(rnd)

	switch srnd {
	case "0":
		pattern = "pattern:verticalbricks"
	case "1":
		pattern = "pattern:verticalsaw"
	case "2":
		pattern = "pattern:hs_cross"
	}

	cmd := exec.Command("convert", "-size", "200x98", pattern, "-transparent", "white", file)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return MakeError(err, "CreateNewCaptcha")
	}

	cmd = exec.Command("convert", file, "-fill", "blue", "-pointsize", "62", "-annotate", "+0+70", captcha, "-tile", "pattern:left30", "-gravity", "center", "-transparent", "white", file)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return MakeError(err, "CreateNewCaptcha")
	}

	rnd = fmt.Sprintf("%d", rand.Intn(24)-12)
	cmd = exec.Command("convert", file, "-rotate", rnd, "-wave", "5x35", "-distort", "Arc", "20", "-wave", "2x35", "-transparent", "white", file)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return MakeError(err, "CreateNewCaptcha")
	}

	/*
		var verification db.Verify

		verification.Type = "captcha"
		verification.Code = captcha
		verification.Identifier = file

		return verification.Create()
	*/
	return nil
}

func GetRandomCaptcha() (string, error) {
	var verify string

	query := `select identifier from verification where type='captcha' order by random() limit 1`
	if err := config.DB.QueryRow(query).Scan(&verify); err != nil {
		return verify, MakeError(err, "GetRandomCaptcha")
	}

	return verify, nil
}

func GetCaptchaTotal() (int, error) {
	var count int

	query := `select count(*) from verification where type='captcha'`
	if err := config.DB.QueryRow(query).Scan(&count); err != nil {
		return count, MakeError(err, "GetCaptchaTotal")
	}

	return count, nil
}

func GetCaptchaCode(verify string) (string, error) {
	var code string

	query := `select code from verification where identifier=$1 limit 1`
	if err := config.DB.QueryRow(query, verify).Scan(&code); err != nil {
		return code, MakeError(err, "GetCaptchaCodeDB")
	}

	return code, nil
}

func DeleteCaptchaCode(verify string) error {
	query := `delete from verification where identifier=$1`
	_, err := config.DB.Exec(query, verify)

	if err != nil {
		return MakeError(err, "DeleteCaptchaCode")
	}

	err = os.Remove("./" + verify)
	return MakeError(err, "DeleteCaptchaCode")
}

func GetPasswordFromSession(ctx *fiber.Ctx) (string, string) {
	cookie := ctx.Cookies("session_token")
	if cookie == "" {
		// Try Authorization header
		cookie = ctx.Get("Authorization")
		if cookie == "" || !strings.HasPrefix(cookie, "Bearer ") {
			return "", ""
		}
		cookie = strings.TrimPrefix(cookie, "Bearer ")
	}
	parts := strings.Split(cookie, "|")

	if len(parts) > 1 {
		return parts[0], parts[1]
	}

	return "", ""
}

func MakeCaptchas(total int) error {
	dbtotal, err := GetCaptchaTotal()

	if err != nil {
		return MakeError(err, "MakeCaptchas")
	}

	difference := total - dbtotal

	for i := 0; i < difference; i++ {
		if err := CreateNewCaptcha(); err != nil {
			return MakeError(err, "MakeCaptchas")
		}
	}

	return nil
}

func CheckCaptcha(captcha string) (bool, error) {
	parts := strings.Split(captcha, ":")

	if strings.Trim(parts[0], " ") == "" || strings.Trim(parts[1], " ") == "" {
		return false, nil
	}

	path := "public/" + parts[0] + ".png"
	code, err := GetCaptchaCode(path)

	if err != nil {
		return false, MakeError(err, "")
	}

	if code != "" {
		err = DeleteCaptchaCode(path)
		if err != nil {
			return false, MakeError(err, "")
		}

		err = CreateNewCaptcha()
		if err != nil {
			return false, MakeError(err, "")
		}

	}

	return code == strings.ToUpper(parts[1]), nil
}
