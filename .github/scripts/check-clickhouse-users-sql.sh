#!/usr/bin/env bash
# Fail unless clickhouse-users.sql gives each login the same hosts and grants
# as clickhouse-users.xml. Run: bash .github/scripts/check-clickhouse-users-sql.sh
#
# Both are reduced to "login host <ip>" and "login GRANT ..." lines and
# compared. A SQL statement this does not recognise is printed as
# "unexpected", so it fails the comparison rather than going unchecked.
set -euo pipefail

dir=deployments/standalone/observability
xml=${1:-$dir/clickhouse-users.xml}
sql=${2:-$dir/clickhouse-users.sql}

# The logins in the XML's <users>, with their <ip> and <query> lines.
from_xml() {
  awk '
    /<users>/ { inside = 1; next }
    /<\/users>/ { inside = 0 }
    !inside { next }
    user == "" && /^[ \t]*<[A-Za-z0-9_]+>[ \t]*$/ { user = $0; gsub(/[ \t<>]/, "", user); next }
    user != "" && $0 ~ ("^[ \t]*</" user ">[ \t]*$") { user = ""; next }
    match($0, /<ip>[^<]*<\/ip>/) { print user " host " substr($0, RSTART + 4, RLENGTH - 9) }
    match($0, /<query>[^<]*<\/query>/) { print user " " substr($0, RSTART + 7, RLENGTH - 15) }
  ' "$1" | sort
}

# The SQL's CREATE USER hosts and GRANTs; its settings profile is skipped.
from_sql() {
  awk -v q="'" '
    /^[ \t]*(--|$)/ { next }
    profile { if (/;[ \t]*$/) profile = 0; next }
    /^CREATE SETTINGS PROFILE / { if (!/;[ \t]*$/) profile = 1; next }
    /^CREATE USER / {
      user = $0; sub(/^CREATE USER (IF NOT EXISTS )?/, "", user); sub(/ .*/, "", user)
      hosts = $0
      if (!sub(/.* HOST /, "", hosts)) { print user " host (none)"; next }
      sub(/( SETTINGS .*)?;[ \t]*$/, "", hosts)
      n = split(hosts, part, /, */)
      for (i = 1; i <= n; i++) {
        h = part[i]
        if (h ~ ("^IP " q "[^" q "]*" q "$")) { gsub(q, "", h); sub(/^IP /, "", h); print user " host " h }
        else print user " unexpected host " h
      }
      next
    }
    /^GRANT .* TO [A-Za-z0-9_]+;[ \t]*$/ {
      user = $0; sub(/.* TO /, "", user); sub(/;[ \t]*$/, "", user)
      grant = $0; sub(/ TO [A-Za-z0-9_]+;[ \t]*$/, "", grant)
      print user " " grant
      next
    }
    { print "unexpected " $0 }
  ' "$1" | sort
}

if diff <(from_xml "$xml") <(from_sql "$sql"); then
  echo "$sql gives each login the hosts and grants $xml does"
else
  echo "$sql differs from $xml (< XML, > SQL)" >&2
  exit 1
fi
