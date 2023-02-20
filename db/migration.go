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
		return errContinue
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
	migrationScript(`
		DROP TABLE inactive;

		ALTER TABLE activitystream DROP COLUMN accuracy;
		ALTER TABLE activitystream DROP COLUMN alias;
		ALTER TABLE activitystream DROP COLUMN altitude;
		ALTER TABLE activitystream DROP COLUMN anyOf;
		ALTER TABLE activitystream DROP COLUMN audience;
		ALTER TABLE activitystream DROP COLUMN bcc;
		ALTER TABLE activitystream DROP COLUMN bto;
		ALTER TABLE activitystream DROP COLUMN cc;
		ALTER TABLE activitystream DROP COLUMN closed;
		ALTER TABLE activitystream DROP COLUMN context;
		ALTER TABLE activitystream DROP COLUMN current;
		ALTER TABLE activitystream DROP COLUMN describes;
		ALTER TABLE activitystream DROP COLUMN duration;
		ALTER TABLE activitystream DROP COLUMN endTime;
		ALTER TABLE activitystream DROP COLUMN first;
		ALTER TABLE activitystream DROP COLUMN formerType;
		ALTER TABLE activitystream DROP COLUMN generator;
		ALTER TABLE activitystream DROP COLUMN height;
		ALTER TABLE activitystream DROP COLUMN hreflang;
		ALTER TABLE activitystream DROP COLUMN icon;
		ALTER TABLE activitystream DROP COLUMN image;
		ALTER TABLE activitystream DROP COLUMN instrument;
		ALTER TABLE activitystream DROP COLUMN items;
		ALTER TABLE activitystream DROP COLUMN last;
		ALTER TABLE activitystream DROP COLUMN latitude;
		ALTER TABLE activitystream DROP COLUMN location;
		ALTER TABLE activitystream DROP COLUMN longitude;
		ALTER TABLE activitystream DROP COLUMN next;
		ALTER TABLE activitystream DROP COLUMN oneOf;
		ALTER TABLE activitystream DROP COLUMN origin;
		ALTER TABLE activitystream DROP COLUMN partOf;
		ALTER TABLE activitystream DROP COLUMN prev;
		ALTER TABLE activitystream DROP COLUMN radius;
		ALTER TABLE activitystream DROP COLUMN rel;
		ALTER TABLE activitystream DROP COLUMN relationship;
		ALTER TABLE activitystream DROP COLUMN result;
		ALTER TABLE activitystream DROP COLUMN startIndex;
		ALTER TABLE activitystream DROP COLUMN startTime;
		ALTER TABLE activitystream DROP COLUMN tag;
		ALTER TABLE activitystream DROP COLUMN target;
		ALTER TABLE activitystream DROP COLUMN to_;
		ALTER TABLE activitystream DROP COLUMN totalItems;
		ALTER TABLE activitystream DROP COLUMN units;
		ALTER TABLE activitystream DROP COLUMN width;
		ALTER TABLE cacheactivitystream DROP COLUMN accuracy;
		ALTER TABLE cacheactivitystream DROP COLUMN alias;
		ALTER TABLE cacheactivitystream DROP COLUMN altitude;
		ALTER TABLE cacheactivitystream DROP COLUMN anyOf;
		ALTER TABLE cacheactivitystream DROP COLUMN audience;
		ALTER TABLE cacheactivitystream DROP COLUMN bcc;
		ALTER TABLE cacheactivitystream DROP COLUMN bto;
		ALTER TABLE cacheactivitystream DROP COLUMN cc;
		ALTER TABLE cacheactivitystream DROP COLUMN closed;
		ALTER TABLE cacheactivitystream DROP COLUMN context;
		ALTER TABLE cacheactivitystream DROP COLUMN current;
		ALTER TABLE cacheactivitystream DROP COLUMN describes;
		ALTER TABLE cacheactivitystream DROP COLUMN duration;
		ALTER TABLE cacheactivitystream DROP COLUMN endTime;
		ALTER TABLE cacheactivitystream DROP COLUMN first;
		ALTER TABLE cacheactivitystream DROP COLUMN formerType;
		ALTER TABLE cacheactivitystream DROP COLUMN generator;
		ALTER TABLE cacheactivitystream DROP COLUMN height;
		ALTER TABLE cacheactivitystream DROP COLUMN hreflang;
		ALTER TABLE cacheactivitystream DROP COLUMN icon;
		ALTER TABLE cacheactivitystream DROP COLUMN image;
		ALTER TABLE cacheactivitystream DROP COLUMN instrument;
		ALTER TABLE cacheactivitystream DROP COLUMN items;
		ALTER TABLE cacheactivitystream DROP COLUMN last;
		ALTER TABLE cacheactivitystream DROP COLUMN latitude;
		ALTER TABLE cacheactivitystream DROP COLUMN location;
		ALTER TABLE cacheactivitystream DROP COLUMN longitude;
		ALTER TABLE cacheactivitystream DROP COLUMN next;
		ALTER TABLE cacheactivitystream DROP COLUMN oneOf;
		ALTER TABLE cacheactivitystream DROP COLUMN origin;
		ALTER TABLE cacheactivitystream DROP COLUMN partOf;
		ALTER TABLE cacheactivitystream DROP COLUMN prev;
		ALTER TABLE cacheactivitystream DROP COLUMN radius;
		ALTER TABLE cacheactivitystream DROP COLUMN rel;
		ALTER TABLE cacheactivitystream DROP COLUMN relationship;
		ALTER TABLE cacheactivitystream DROP COLUMN result;
		ALTER TABLE cacheactivitystream DROP COLUMN startIndex;
		ALTER TABLE cacheactivitystream DROP COLUMN startTime;
		ALTER TABLE cacheactivitystream DROP COLUMN tag;
		ALTER TABLE cacheactivitystream DROP COLUMN target;
		ALTER TABLE cacheactivitystream DROP COLUMN to_;
		ALTER TABLE cacheactivitystream DROP COLUMN totalItems;
		ALTER TABLE cacheactivitystream DROP COLUMN units;
		ALTER TABLE cacheactivitystream DROP COLUMN width;
	`),
	migrationScript(`
		ALTER TABLE actor ADD COLUMN blotter TEXT;
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

		if err = v(tx); err != nil {
			if i != 0 && err != errContinue {
				tx.Rollback()
				return err
			}
		}

		if i == 0 && err != errContinue {
			if _, err := config.DB.Exec(`update dbinternal set version = $1`, len(migrations)); err != nil {
				tx.Rollback()
				return err
			}

			if err := tx.Commit(); err != nil {
				return err
			}

			break
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
