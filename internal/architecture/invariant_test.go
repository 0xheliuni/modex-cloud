package architecture

// This test encodes the platform's core security invariant as an executable,
// build-breaking check: channel-key decryption must never reach controllers.
//
// It scans package source for forbidden references so a future edit that, say,
// calls crypto.SyncOpener() from a controller fails CI immediately.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot walks up from this test file to the module root (where go.mod lives).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate module root (go.mod)")
	return ""
}

// scanGoFiles returns non-test .go files under dir whose code (comments stripped)
// matches pred. Comments are excluded so that documentation *describing* the rule
// (e.g. "never call crypto.Vault.Open") does not trip the check.
func scanGoFiles(t *testing.T, dir string, pred func(content string) bool) []string {
	t.Helper()
	var hits []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if pred(stripComments(string(b))) {
			hits = append(hits, path)
		}
		return nil
	})
	return hits
}

// stripComments removes // line comments and /* */ block comments. It is a
// pragmatic stripper (it does not account for these tokens inside string
// literals), which is sufficient for the forbidden identifiers we guard.
func stripComments(src string) string {
	var b strings.Builder
	for i := 0; i < len(src); i++ {
		// Block comment.
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				break
			}
			i += end + 3
			continue
		}
		// Line comment.
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			nl := strings.IndexByte(src[i:], '\n')
			if nl < 0 {
				break
			}
			i += nl
			b.WriteByte('\n')
			continue
		}
		b.WriteByte(src[i])
	}
	return b.String()
}

// TestControllersNeverDecrypt is the enforcement: the controller package must
// not reference the decrypting accessor or call Open.
func TestControllersNeverDecrypt(t *testing.T) {
	root := repoRoot(t)
	controllerDir := filepath.Join(root, "controller")

	forbidden := []string{"SyncOpener", ".Open(", "crypto.Vault"}
	hits := scanGoFiles(t, controllerDir, func(content string) bool {
		for _, f := range forbidden {
			if strings.Contains(content, f) {
				return true
			}
		}
		return false
	})
	if len(hits) > 0 {
		t.Errorf("SECURITY INVARIANT VIOLATED: controllers must never decrypt keys.\n"+
			"Forbidden reference (%v) found in:\n  %s",
			forbidden, strings.Join(hits, "\n  "))
	}
}

// TestSyncOpenerOnlyInSyncPackage asserts SyncOpener() is referenced only inside
// service/sync (and its own definition in crypto). Any other caller is a leak.
func TestSyncOpenerOnlyInSyncPackage(t *testing.T) {
	root := repoRoot(t)
	allowed := map[string]bool{
		filepath.Join(root, "crypto"):          true, // definition + tests
		filepath.Join(root, "service", "sync"): true, // sole legitimate caller
	}

	var offenders []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		dir := filepath.Dir(path)
		if allowed[dir] {
			return nil
		}
		b, _ := os.ReadFile(path)
		if strings.Contains(stripComments(string(b)), "SyncOpener") {
			offenders = append(offenders, path)
		}
		return nil
	})
	if len(offenders) > 0 {
		t.Errorf("SECURITY INVARIANT VIOLATED: SyncOpener() referenced outside service/sync:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}
