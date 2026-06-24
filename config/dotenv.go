package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotEnv reads a .env file (if present) and sets any keys that are NOT
// already present in the real environment. Real env vars therefore always win,
// so a value injected by GoLand / Docker / a secrets manager overrides the file.
//
// It searches the working directory and walks up a few parents, so running from
// a sub-package (e.g. GoLand's per-package run config) still finds the project
// root .env. A missing file is not an error — it simply does nothing.
//
// Supported syntax (intentionally minimal):
//
//	# comment lines and blank lines are ignored
//	KEY=value
//	KEY="quoted value"   (surrounding single/double quotes are stripped)
//	export KEY=value     (a leading `export ` is tolerated)
func LoadDotEnv() {
	path, ok := findDotEnv()
	if !ok {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = unquote(val)
		if key == "" {
			continue
		}
		// Do not clobber a value already set in the real environment.
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, val)
	}
}

// findDotEnv looks for a .env file in the CWD and up to 4 parent directories.
func findDotEnv() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for i := 0; i < 5; i++ {
		p := filepath.Join(dir, ".env")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// unquote strips a single matching pair of surrounding quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
