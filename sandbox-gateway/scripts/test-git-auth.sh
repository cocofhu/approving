#!/usr/bin/env bash
# Unit smoke for startup.sh Git HTTPS credential + gh/glab auth helpers.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STARTUP="$ROOT/sandbox/scripts/startup.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Extract helpers from repo_host through setup_bare_github_credentials (inclusive).
START=$(grep -n '^repo_host()' "$STARTUP" | head -1 | cut -d: -f1)
END=$(awk -v s="$START" 'NR>s && /^setup_repo_credentials\(\)/ {print NR-1; exit}' "$STARTUP")
sed -n "${START},${END}p" "$STARTUP" >"$TMP/git-auth.sh"

# HOME-scoped mocks so we never touch the real ~/.config/gh or git creds.
export HOME="$TMP/home"
mkdir -p "$HOME/bin" "$HOME"
export GIT_CONFIG_GLOBAL="$TMP/gitconfig"
touch "$GIT_CONFIG_GLOBAL"
# Prefer mocks over any host-installed gh/glab.
export PATH="$HOME/bin:$PATH"

# Fake gh/glab: record argv + stdin token; succeed unless FAIL_*=1.
cat >"$HOME/bin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
: "${HOME:?}"
mkdir -p "$HOME"
printf 'argv:%s\n' "$*" >"$HOME/gh.last"
cat >"$HOME/gh.token"
if [ "${FAIL_GH:-0}" = "1" ]; then
  echo "mock gh: forced failure" >&2
  exit 1
fi
exit 0
EOF
chmod +x "$HOME/bin/gh"

cat >"$HOME/bin/glab" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
: "${HOME:?}"
mkdir -p "$HOME"
printf 'argv:%s\n' "$*" >"$HOME/glab.last"
if [ "${FAIL_GLAB:-0}" = "1" ]; then
  echo "mock glab: forced failure" >&2
  exit 1
fi
exit 0
EOF
chmod +x "$HOME/bin/glab"

# Helpers hardcode /root/.git-credentials — rewrite to TMP for the test sandbox.
sed -i 's|/root|'"$TMP/root"'|g' "$TMP/git-auth.sh"
mkdir -p "$TMP/root"

# shellcheck disable=SC1091
source "$TMP/git-auth.sh"

GITHUB_TOKEN="ghp_test_token"
GITLAB_TOKEN=""
GITHUB_URL=""
GITLAB_URL=""
rm -f "$TMP/root/.git-credentials" "$HOME/gh.last" "$HOME/gh.token"

setup_https_credentials "https://github.com/cocofhu/approving.git"
grep -q 'x-access-token:ghp_test_token@github.com' "$TMP/root/.git-credentials"
grep -q 'argv:auth login --hostname github.com --with-token' "$HOME/gh.last"
grep -qx 'ghp_test_token' "$HOME/gh.token"
echo "OK: github.com HTTPS + gh auth login"

rm -f "$HOME/gh.last" "$HOME/gh.token" "$TMP/root/.git-credentials"
GITHUB_TOKEN="ghe_token"
GITHUB_URL="https://ghe.example.com"
setup_https_credentials "https://ghe.example.com/org/repo.git"
grep -q 'x-access-token:ghe_token@ghe.example.com' "$TMP/root/.git-credentials"
grep -q 'argv:auth login --hostname ghe.example.com --with-token' "$HOME/gh.last"
echo "OK: GITHUB_URL self-hosted + gh auth login"

rm -f "$HOME/gh.last" "$HOME/gh.token" "$TMP/root/.git-credentials"
GITHUB_TOKEN="bare_token"
GITHUB_URL=""
setup_bare_github_credentials
grep -q 'x-access-token:bare_token@github.com' "$TMP/root/.git-credentials"
grep -q 'argv:auth login --hostname github.com --with-token' "$HOME/gh.last"
echo "OK: bare GITHUB_TOKEN defaults to github.com"

rm -f "$HOME/glab.last" "$TMP/root/.git-credentials"
GITHUB_TOKEN=""
GITLAB_TOKEN="glpat_test"
GITLAB_URL="https://gitlab.com"
setup_https_credentials "https://gitlab.com/group/project.git"
grep -q 'oauth2:glpat_test@gitlab.com' "$TMP/root/.git-credentials"
grep -q 'argv:auth login --hostname gitlab.com --token glpat_test' "$HOME/glab.last"
echo "OK: gitlab.com HTTPS + glab auth login"

# gh auth failure must not roll back HTTPS credential injection.
export FAIL_GH=1
rm -f "$HOME/gh.last" "$TMP/root/.git-credentials"
GITHUB_TOKEN="still_ok"
GITLAB_TOKEN=""
GITHUB_URL=""
setup_https_credentials "https://github.com/cocofhu/approving.git"
grep -q 'x-access-token:still_ok@github.com' "$TMP/root/.git-credentials"
grep -q 'argv:auth login --hostname github.com --with-token' "$HOME/gh.last"
unset FAIL_GH
echo "OK: gh auth failure is non-fatal for HTTPS creds"

echo "OK: git auth unit smoke passed"
