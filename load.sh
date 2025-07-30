#!/usr/bin/env bash
set -euo pipefail

# connection parameters
PGHOST="localhost"
PGPORT="15432"
PGUSER="postgres"
PGPASSWORD="postgres"
PGDATABASE="postgres"   # change if you have a different DB

export PGPASSWORD

# run the SQL file
psql \
  --host="$PGHOST" \
  --port="$PGPORT" \
  --username="$PGUSER" \
  --dbname="$PGDATABASE" \
  --file="load.sql"
