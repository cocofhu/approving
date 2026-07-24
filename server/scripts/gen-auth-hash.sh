#!/usr/bin/env bash
# Generate bcrypt password_hash values for approving static auth (auth.users).
#
# Usage:
#   server/scripts/gen-auth-hash.sh <password>
#   server/scripts/gen-auth-hash.sh --username admin <password>
#   server/scripts/gen-auth-hash.sh --username admin   # prompt for password
#   server/scripts/gen-auth-hash.sh                    # prompt for password
#
# Output: bcrypt hash, plus an auth.users YAML snippet ready to paste into
# config.yaml or APPROVING_AUTH_USERS.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

fail() { echo "error: $*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Usage:
  gen-auth-hash.sh <password>
  gen-auth-hash.sh --username <name> <password>
  gen-auth-hash.sh [--username <name>]   # prompt for password (hidden)

Examples:
  gen-auth-hash.sh demo1234
  gen-auth-hash.sh --username admin demo1234
  gen-auth-hash.sh --username ops
EOF
}

username=""
password=""

while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    -u|--username)
      [ $# -ge 2 ] || fail "missing value for $1"
      username="$2"
      shift 2
      ;;
    *)
      [ -z "$password" ] || fail "unexpected argument: $1 (use --help)"
      password="$1"
      shift
      ;;
  esac
done

if [ -z "$password" ]; then
  read -r -s -p "Password: " password
  echo
  [ -n "$password" ] || fail "password cannot be empty"
fi

command -v go >/dev/null 2>&1 || fail "go is not installed / not on PATH"

hash="$(cd "$SERVER_DIR" && go run ./scripts/genauthhash.go "$password")"
[ -n "$hash" ] || fail "failed to generate hash"

echo "password_hash: $hash"
echo

if [ -n "$username" ]; then
  cat <<EOF
# Paste into auth.users (config.yaml):
  - username: $username
    password_hash: "$hash"
EOF
else
  cat <<EOF
# Paste into auth.users (config.yaml):
  - username: <your-username>
    password_hash: "$hash"
EOF
fi
