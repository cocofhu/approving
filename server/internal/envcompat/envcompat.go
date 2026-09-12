// Package envcompat maps the approving → grasp rename for one release.
//
// COMPAT(approving→grasp): remove after next minor.
package envcompat

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

const (
	NewPrefix = "GRASP_"
	OldPrefix = "APPROVING_"
)

var warned sync.Map

// LegacyKey returns the pre-rename APPROVING_* alias for a GRASP_* key.
func LegacyKey(key string) string {
	if strings.HasPrefix(key, NewPrefix) {
		return OldPrefix + strings.TrimPrefix(key, NewPrefix)
	}
	return ""
}

// CanonicalKey rewrites an APPROVING_* name to GRASP_*; other names are unchanged.
func CanonicalKey(key string) string {
	if strings.HasPrefix(key, OldPrefix) {
		return NewPrefix + strings.TrimPrefix(key, OldPrefix)
	}
	return key
}

// LookupInMap reads key from env, then the APPROVING_* alias when key is GRASP_*.
func LookupInMap(env map[string]string, key string) string {
	if v := strings.TrimSpace(env[key]); v != "" {
		return v
	}
	if legacy := LegacyKey(key); legacy != "" {
		return strings.TrimSpace(env[legacy])
	}
	return ""
}

// MigrateMap renames APPROVING_* keys to GRASP_*. Existing GRASP_* values win.
func MigrateMap(env map[string]string) (map[string]string, int) {
	if env == nil {
		return nil, 0
	}
	n := 0
	out := make(map[string]string, len(env))
	for k, v := range env {
		if !strings.HasPrefix(k, OldPrefix) {
			out[k] = v
		}
	}
	for k, v := range env {
		if !strings.HasPrefix(k, OldPrefix) {
			continue
		}
		n++
		dest := CanonicalKey(k)
		if _, exists := out[dest]; exists {
			continue
		}
		out[dest] = v
	}
	if n == 0 {
		return env, 0
	}
	return out, n
}

// Lookup reads GRASP_* first, then the APPROVING_* alias.
func Lookup(key string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	if legacy := LegacyKey(key); legacy != "" {
		if v := strings.TrimSpace(os.Getenv(legacy)); v != "" {
			if _, loaded := warned.LoadOrStore(legacy, true); !loaded {
				log.Warn().
					Str("env", legacy).
					Str("prefer", key).
					Msg("COMPAT(approving→grasp): deprecated environment variable; use the GRASP_ name")
			}
			return v
		}
	}
	return ""
}

// envKeyLine rewrites only the key at the start of a .env line, never values.
var envKeyLine = regexp.MustCompile(`^([ \t]*#?[ \t]*(?:export[ \t]+)?)APPROVING_`)

// RewriteEnvFileContent renames APPROVING_* keys to GRASP_* in dotenv content.
func RewriteEnvFileContent(src string) (string, bool) {
	if !strings.Contains(src, OldPrefix) {
		return src, false
	}
	lines := strings.SplitAfter(src, "\n")
	changed := false
	for i, line := range lines {
		next := envKeyLine.ReplaceAllString(line, "${1}GRASP_")
		if next != line {
			lines[i] = next
			changed = true
		}
	}
	return strings.Join(lines, ""), changed
}

// PromoteProcessEnv copies APPROVING_* process env onto unset GRASP_* keys.
func PromoteProcessEnv() int {
	n := 0
	for _, e := range os.Environ() {
		k, v, ok := strings.Cut(e, "=")
		if !ok || !strings.HasPrefix(k, OldPrefix) {
			continue
		}
		dest := NewPrefix + strings.TrimPrefix(k, OldPrefix)
		if strings.TrimSpace(os.Getenv(dest)) != "" {
			continue
		}
		if err := os.Setenv(dest, v); err != nil {
			continue
		}
		n++
	}
	return n
}

// RewriteNearbyEnvFiles rewrites `.env` next to the process / config file.
func RewriteNearbyEnvFiles() {
	var paths []string
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, ".env"))
		parent := filepath.Dir(cwd)
		if parent != string(filepath.Separator) && parent != cwd {
			paths = append(paths, filepath.Join(parent, ".env"))
		}
	}
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		paths = append(paths, filepath.Join(filepath.Dir(p), ".env"))
	}
	if p := strings.TrimSpace(os.Getenv("GRASP_ENV_FILE")); p != "" {
		paths = append(paths, p)
	}
	if p := strings.TrimSpace(os.Getenv("HOST_REPO_DIR")); p != "" {
		paths = append(paths, filepath.Join(p, ".env"))
	}
	seen := map[string]struct{}{}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		ok, err := RewriteEnvFile(abs)
		if err != nil {
			log.Warn().Err(err).Str("path", abs).Msg("COMPAT(approving→grasp): could not rewrite .env")
			continue
		}
		if ok {
			log.Info().Str("path", abs).Msg("COMPAT(approving→grasp): rewrote APPROVING_* keys in .env to GRASP_*")
		}
	}
}

// RewriteEnvFile rewrites APPROVING_* keys in path in place when needed.
func RewriteEnvFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	out, changed := RewriteEnvFileContent(string(data))
	if !changed {
		return false, nil
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o600
	}
	if err := os.WriteFile(path, []byte(out), mode); err != nil {
		return false, err
	}
	return true, nil
}

// MigrateLegacyPath renames oldPath → newPath when the target is absent.
func MigrateLegacyPath(oldPath, newPath string) (bool, error) {
	if oldPath == "" || newPath == "" || oldPath == newPath {
		return false, nil
	}
	if _, err := os.Stat(newPath); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if _, err := os.Stat(oldPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return false, err
	}
	return true, nil
}
