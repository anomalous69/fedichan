package db

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"

	"github.com/KushBlazingJudah/fedichan/config"
)

var (
	ErrInvalid = errors.New("invalid username or password")
)

type AcctType int

const (
	None AcctType = iota
	Janitor
	Mod
	Admin
)

const saltLength = 16

type Acct struct {
	Username string
	Email    string
	Type     AcctType
}

type Signature struct {
	KeyId     string
	Headers   []string
	Signature string
	Algorithm string
}

func (a AcctType) String() string {
	switch a {
	case None:
		return "none"
	case Janitor:
		return "janitor"
	case Mod:
		return "mod"
	case Admin:
		return "admin"
	default:
		return "unknown???"
	}
}

func makeSalt() []byte {
	// The panics are here because if there is an error here, its probably
	// fatal anyway.
	// I've been wrong before.

	s := make([]byte, saltLength)
	n := 0

	for n != saltLength {
		j, err := rand.Read(s[n:])
		if err != nil {
			panic(err)
		}
		n += j
	}

	return s
}

func makePwText(pass string, salt []byte) []byte {
	plaintext := make([]byte, len(pass)+len(salt))
	copy(plaintext, []byte(pass))
	copy(plaintext[len(pass):], salt)

	return plaintext
}

// Save creates or updates the basic account information available from the Acct struct.
//
// TODO: Update
func (a Acct) Save() error {
	var email *string
	if a.Email != "" {
		email = &a.Email
	}

	_, err := config.DB.Exec(`insert into accounts (username, email, type) values ($1, $2, $3) on conflict (username) do update set email=excluded.email, type=excluded.type`, a.Username, email, a.Type)
	return wrapErr(err)
}

// SetPassword sets the password for this Acct.
func (a Acct) SetPassword(password string) error {
	salt := makeSalt()
	plaintext := makePwText(password, salt)
	ciphertext := sha256.Sum256(plaintext)

	_, err := config.DB.Exec(`update accounts set password=$1, salt=$2 where username=$3`, ciphertext[:], salt, a.Username)
	return err
}

// Session creates a new session key for the user.
func (a Acct) Session() (string, error) {
	// Reusing makeSalt.
	session := hex.EncodeToString(makeSalt())

	_, err := config.DB.Exec(`update accounts set session=$1 where username=$2`, session, a.Username)
	return session, err
}

// LoginSession fetches user details from their session key.
// If this session is invalid, ErrInvalid is returned.
func LoginSession(session string) (Acct, error) {
	a := Acct{}
	var email sql.NullString
	if err := config.DB.QueryRow(`select username, email, type from accounts where session=$1`, session).Scan(&a.Username, &email, &a.Type); err != nil {
		return a, ErrInvalid
	}
	a.Email = email.String

	return a, nil
}

// CheckPassword determines if the supplied credentials are valid.
func CheckPassword(user, pass string) bool {
	var pwhash, salt []byte

	if config.DB.QueryRow(`select password, salt from accounts where username=$1`, user).Scan(&pwhash, &salt) != nil {
		return false
	}

	plaintext := makePwText(pass, salt)
	ciphertext := sha256.Sum256(plaintext)

	return bytes.Equal(ciphertext[:], pwhash)
}

// UserExists returns true when a user exists.
// Used for creating an Admin user on startup.
func UserExists(user string) bool {
	return config.DB.QueryRow(`select from accounts where username=$1`, user).Scan() == nil
}

// Users returns a list of users.
func Users() ([]Acct, error) {
	rows, err := config.DB.Query(`select username, email, type from accounts order by username asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []Acct

	for rows.Next() {
		var email sql.NullString
		u := Acct{}

		if err := rows.Scan(&u.Username, &email, &u.Type); err != nil {
			return users, err
		}

		u.Email = email.String
		users = append(users, u)
	}

	return users, nil
}

// Fetches details of one specific user.
func User(name string) (Acct, error) {
	user := Acct{
		Username: name,
	}

	var email sql.NullString

	err := config.DB.QueryRow(`select email, type from accounts where username=$1`, name).Scan(&email, &user.Type)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrInvalid
	}

	user.Email = email.String

	return user, err
}
