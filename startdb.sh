#!/bin/sh

# Helper for a development instance; creates a database if it needs to and
# starts PostgreSQL.
# Run this in the background.

[ -e "pgdata" ] || initdb -D pgdata

# -k /tmp moves the unix socket to /tmp, which is world writable and doesn't
# require /run/postgresql to exist
exec postgres -D pgdata -k /tmp
