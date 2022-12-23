package db

import (
	"fmt"
	"net/smtp"
	"time"

	"github.com/KushBlazingJudah/fedichan/config"
)

type Verify struct {
	Type       string
	Identifier string
	Code       string
	Created    string
	Board      string
	Label      string
}

type VerifyCooldown struct {
	Identifier string
	Code       string
	Time       int
}

type Signature struct {
	KeyId     string
	Headers   []string
	Signature string
	Algorithm string
}

func (verify Verify) Create() error {
	query := `insert into verification (type, identifier, code, created) values ($1, $2, $3, $4)`
	_, err := config.DB.Exec(query, verify.Type, verify.Identifier, verify.Code, time.Now().UTC().Format(time.RFC3339))

	return wrapErr(err)
}

func (verify Verify) CreateBoardAccess() error {
	if hasAccess, _ := verify.HasBoardAccess(); !hasAccess {
		if verify.Label == "" {
			verify.Label = "Anon"
		}
		query := `insert into boardaccess (identifier, board, label) values($1, $2, $3)`
		_, err := config.DB.Exec(query, verify.Identifier, verify.Board, verify.Label)

		return wrapErr(err)
	}

	return nil
}

func (verify Verify) CreateBoardMod() error {
	var pass string

	/* TODO
	var err error
	if pass, err = CreateKey(50); err != nil {
		return wrapErr(err)
	}
	*/

	var code string

	query := `select code from verification where identifier=$1 and type=$2 and code not in (select verificationcode from crossverification)`
	if err := config.DB.QueryRow(query, verify.Board, verify.Type).Scan(&code); err != nil {
		return wrapErr(err)
	}

	var ident string

	query = `select identifier from boardaccess where identifier=$1 and board=$2 and code not in (select code from crossverification)`
	if err := config.DB.QueryRow(query, verify.Identifier, verify.Board).Scan(&ident); err != nil {
		query := `insert into crossverification (verificationcode, code) values ($1, $2)`
		if _, err := config.DB.Exec(query, code, pass); err != nil {
			return wrapErr(err)
		}

		if verify.Label == "" {
			verify.Label = "Anon"
		}

		query = `insert into boardaccess (identifier, code, board, type, label) values ($1, $2, $3, $4, $5)`
		if _, err = config.DB.Exec(query, verify.Identifier, pass, verify.Board, verify.Type, verify.Label); err != nil {
			return wrapErr(err)
		}
	}

	return nil
}

func (verify Verify) DeleteBoardMod() error {
	var code string

	query := `select code from boardaccess where identifier=$1 and board=$1`
	if err := config.DB.QueryRow(query, verify.Identifier, verify.Board).Scan(&code); err != nil {
		return nil
	}

	query = `delete from crossverification where code=$1`
	if _, err := config.DB.Exec(query, code); err != nil {
		return wrapErr(err)
	}

	query = `delete from boardaccess where identifier=$1 and board=$2`
	if _, err := config.DB.Exec(query, verify.Identifier, verify.Board); err != nil {
		return wrapErr(err)
	}

	return nil
}

func (verify Verify) GetBoardMod() (Verify, error) {
	var nVerify Verify

	query := `select code, board, type, identifier from boardaccess where identifier=$1`
	if err := config.DB.QueryRow(query, verify.Identifier).Scan(&nVerify.Code, &nVerify.Board, &nVerify.Type, &nVerify.Identifier); err != nil {
		return nVerify, wrapErr(err)
	}

	return nVerify, nil
}

func (verify Verify) GetCode() (Verify, error) {
	var nVerify Verify

	query := `select type, identifier, code, board from boardaccess where identifier=$1 and board=$2`
	if err := config.DB.QueryRow(query, verify.Identifier, verify.Board).Scan(&nVerify.Type, &nVerify.Identifier, &nVerify.Code, &nVerify.Board); err != nil {
		return verify, nil
	}

	return nVerify, nil
}

func (verify Verify) HasBoardAccess() (bool, string) {
	var _type string

	query := `select type from boardaccess where identifier=$1 and board=$2`
	if err := config.DB.QueryRow(query, verify.Identifier, verify.Board).Scan(&_type); err != nil {
		return false, ""
	}

	return true, _type
}

func (verify Verify) SendVerification() error {
	config.Log.Println("sending email")

	from := config.SiteEmail
	pass := config.SiteEmailPassword
	to := verify.Identifier
	body := fmt.Sprintf("You can use either\r\nEmail: %s \r\n Verfication Code: %s\r\n for the board %s", verify.Identifier, verify.Code, verify.Board)

	msg := "From: " + from + "\n" +
		"To: " + to + "\n" +
		"Subject: Image Board Verification\n\n" +
		body

	err := smtp.SendMail(config.SiteEmailServer+":"+config.SiteEmailPort,
		smtp.PlainAuth("", from, pass, config.SiteEmailServer),
		from, []string{to}, []byte(msg))

	return wrapErr(err)
}

func (verify Verify) VerifyCooldownAdd() error {
	query := `insert into verficationcooldown (identifier, code) values ($1, $2)`
	_, err := config.DB.Exec(query, verify.Identifier, verify.Code)

	return wrapErr(err)
}

func BoardHasAuthType(board string, auth string) (bool, error) {
	return true, nil

	/* TODO
	authTypes, err := GetBoardAuth(board)

	if err != nil {
		return false, wrapErr(err)
	}

	for _, e := range authTypes {
		if e == auth {
			return true, nil
		}
	}

	return false, nil
	*/
}

func GetVerificationByCode(code string) (Verify, error) {
	var verify Verify

	query := `select type, identifier, code, board from boardaccess where code=$1`
	if err := config.DB.QueryRow(query, code).Scan(&verify.Type, &verify.Identifier, &verify.Code, &verify.Board); err != nil {
		return verify, wrapErr(err)
	}

	return verify, nil
}

func GetVerificationByEmail(email string) (Verify, error) {
	var verify Verify

	query := `select type, identifier, code, board from boardaccess where identifier=$1`
	if err := config.DB.QueryRow(query, email).Scan(&verify.Type, &verify.Identifier, &verify.Code, &verify.Board); err != nil {
		return verify, nil
	}

	return verify, nil
}

func GetVerify(access string) (Verify, error) {
	verify, err := GetVerificationByCode(access)

	if err != nil {
		return verify, wrapErr(err)
	}

	if verify.Identifier == "" {
		verify, err = GetVerificationByEmail(access)
	}

	return verify, wrapErr(err)
}

func HasAuthCooldown(auth string) (bool, error) {
	var current VerifyCooldown
	var err error

	if current, err = VerifyCooldownCurrent(auth); err != nil {
		return false, wrapErr(err)
	}

	if current.Time > 0 {
		return true, nil
	}

	return false, nil
}

func HasAuth(code string, board string) (bool, string) {
	verify, err := GetVerificationByCode(code)

	if err != nil {
		return false, ""
	}

	if res, _type := verify.HasBoardAccess(); verify.Board == config.Domain || (res && verify.Board == board) {
		return true, _type
	}

	return false, ""
}

func IsEmailSetup() bool {
	return config.SiteEmail != "" || config.SiteEmailPassword != "" || config.SiteEmailServer != "" || config.SiteEmailPort != ""
}

func VerficationCooldown() error {
	query := `select identifier, code, time from verificationcooldown`
	rows, err := config.DB.Query(query)

	if err != nil {
		return wrapErr(err)
	}

	defer rows.Close()
	for rows.Next() {
		var verify VerifyCooldown

		if err := rows.Scan(&verify.Identifier, &verify.Code, &verify.Time); err != nil {
			return wrapErr(err)
		}

		nTime := verify.Time - 1
		query = `update set time=$1 where identifier=$2`

		if _, err := config.DB.Exec(query, nTime, verify.Identifier); err != nil {
			return wrapErr(err)
		}

		VerficationCooldownRemove()
	}

	return nil
}

func VerficationCooldownRemove() error {
	query := `delete from verificationcooldown where time < 1`
	_, err := config.DB.Exec(query)

	return wrapErr(err)
}

func VerifyCooldownCurrent(auth string) (VerifyCooldown, error) {
	var current VerifyCooldown

	query := `select identifier, code, time from verificationcooldown where code=$1`
	if err := config.DB.QueryRow(query, auth).Scan(&current.Identifier, &current.Code, &current.Time); err != nil {
		query := `select identifier, code, time from verificationcooldown where identifier=$1`
		if err := config.DB.QueryRow(query, auth).Scan(&current.Identifier, &current.Code, &current.Time); err != nil {
			return current, nil
		}

		return current, nil
	}

	return current, nil
}
