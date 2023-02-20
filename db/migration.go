package db

import (
	"database/sql"
	"errors"

	"github.com/KushBlazingJudah/fedichan/config"

	_ "embed"
)

//go:embed schema.psql
var dbSchema string

var errContinue = errors.New("db continue")

func migrationScript(s string) func(*sql.Tx) error {
	return func(tx *sql.Tx) error {
		_, err := tx.Exec(s)
		return err
	}
}

var migrations = []func(*sql.Tx) error{
	func(tx *sql.Tx) error {
		// Determine if the database is already initialized
		// Table was chosen arbitrarily, but it doesn't exist in Fedichan
		err := tx.QueryRow(`select from information_schema.tables where table_name='boardaccess'`).Scan()
		if errors.Is(err, sql.ErrNoRows) {
			// Database is not initialized
			return migrationScript(dbSchema)(tx)
		}

		// Database is initialized.
		return nil
	},
	migrationScript(`
		CREATE TABLE accounts(
		       username TEXT NOT NULL UNIQUE,
		       email TEXT,
		       type INTEGER,
		       password bytea,
		       salt bytea,
		       session text
		);

		CREATE TABLE captchas(
		       id TEXT NOT NULL UNIQUE,
		       file TEXT NOT NULL UNIQUE,
		       solution TEXT NOT NULL
		);

		DROP TABLE actorauth;
		DROP TABLE boardaccess;
		DROP TABLE crossverification;
		DROP TABLE verification;
		DROP TABLE verificationcooldown;
		DROP TABLE wallet;
	`),
}

func migrate() error {
	ver := 0

	// TODO: Remove this snippet much later.
	err := config.DB.QueryRow(`select from information_schema.tables where table_name='dbinternal'`).Scan()
	if errors.Is(err, sql.ErrNoRows) {
		// Create schema version storage spot hack thingy
		if _, err := config.DB.Exec(`create table dbinternal (version integer); insert into dbinternal(version) values(0);`); err != nil {
			return err
		}
	}

	if err := config.DB.QueryRow(`select version from dbinternal`).Scan(&ver); err != nil {
		return err
	}

	for i, v := range migrations[ver:] {
		tx, err := config.DB.Begin()
		if err != nil {
			return err
		}

		if err := v(tx); err != nil {
			tx.Rollback()
			return err
		}

		if _, err := config.DB.Exec(`update dbinternal set version = $1`, ver+i+1); err != nil {
			tx.Rollback()
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}
