#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

for db in loadtest_a loadtest_b loadtest_c loadtest_d; do
	docker compose exec -T target-pg psql -U postgres -v ON_ERROR_STOP=1 -q <<SQL
SELECT 'CREATE DATABASE $db OWNER platform_app'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = '$db') \gexec
SQL
	echo "target database $db ready"
done
