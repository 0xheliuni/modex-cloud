package config

import (
	"strings"
	"testing"
)

// setEnv sets env vars for a test and restores them afterward.
func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func validKEK() string { return strings.Repeat("a", 64) }

func TestLoad_RequiresStrongMasterKEK(t *testing.T) {
	base := map[string]string{"SESSION_SECRET": "0123456789abcdef0123"}

	cases := []struct {
		name, kek string
		wantErr   bool
	}{
		{"missing", "", true},
		{"too short", "abcd", true},
		{"63 chars", strings.Repeat("a", 63), true},
		{"non-hex", strings.Repeat("z", 64), true},
		{"valid 64 hex", validKEK(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, base)
			t.Setenv("MASTER_KEK", tc.kek)
			_, err := Load()
			if tc.wantErr && err == nil {
				t.Errorf("MASTER_KEK %q: expected error, got nil", tc.kek)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("MASTER_KEK %q: unexpected error %v", tc.kek, err)
			}
		})
	}
}

func TestLoad_RequiresSessionSecret(t *testing.T) {
	t.Setenv("MASTER_KEK", validKEK())
	t.Setenv("SESSION_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Error("expected error for short SESSION_SECRET")
	}
	t.Setenv("SESSION_SECRET", "this-is-long-enough-secret")
	if _, err := Load(); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}

func TestLoad_RejectsWeakRootPassword(t *testing.T) {
	t.Setenv("MASTER_KEK", validKEK())
	t.Setenv("SESSION_SECRET", "this-is-long-enough-secret")
	t.Setenv("ROOT_PASSWORD", "weak")
	if _, err := Load(); err == nil {
		t.Error("expected error for weak ROOT_PASSWORD")
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("MASTER_KEK", validKEK())
	t.Setenv("SESSION_SECRET", "this-is-long-enough-secret")
	t.Setenv("ROOT_PASSWORD", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 3000 {
		t.Errorf("default port = %d, want 3000", cfg.Port)
	}
	if cfg.RootUsername != "admin" {
		t.Errorf("default root username = %q, want admin", cfg.RootUsername)
	}
}
