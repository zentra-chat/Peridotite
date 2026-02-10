#!/usr/bin/env sh
set -e

if [ -z "${DATABASE_URL}" ]; then
  echo "DATABASE_URL is not set"
  exit 1
fi

for file in $(ls -1 migrations/*.up.sql | sort); do
  echo "Applying ${file}"
  psql "${DATABASE_URL}" -f "${file}"
done
