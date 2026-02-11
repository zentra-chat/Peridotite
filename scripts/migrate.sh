#!/usr/bin/env sh
set -e

# if [ -z "${DATABASE_URL}" ]; then
#   echo "DATABASE_URL is not set"
#   exit 1
# fi

DATABASE_URL="${DATABASE_URL:-postgres://zentra:zentra_secure_password@localhost:5432/zentra?sslmode=disable}"

for file in $(ls -1 migrations/*.up.sql | sort); do
  echo "Applying ${file}"
  psql "${DATABASE_URL}" -f "${file}"
done
