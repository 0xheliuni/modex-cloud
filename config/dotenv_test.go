package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv_FillsUnsetButNeverOverrides(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "" +
		"# a comment\n" +
		"\n" +
		"MASTER_KEK=deadbeef\n" +
		"export SESSION_SECRET=\"quoted secret\"\n" +
		"ALREADY_SET=from_file\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// Run from the temp dir so findDotEnv locates our file.
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// A pre-set real env var must NOT be overridden by the file.
	t.Setenv("ALREADY_SET", "from_real_env")
	// Ensure the others are unset so the file fills them.
	os.Unsetenv("MASTER_KEK")
	os.Unsetenv("SESSION_SECRET")

	LoadDotEnv()

	if got := os.Getenv("MASTER_KEK"); got != "deadbeef" {
		t.Errorf("MASTER_KEK = %q, want deadbeef (from file)", got)
	}
	if got := os.Getenv("SESSION_SECRET"); got != "quoted secret" {
		t.Errorf("SESSION_SECRET = %q, want unquoted value", got)
	}
	if got := os.Getenv("ALREADY_SET"); got != "from_real_env" {
		t.Errorf("ALREADY_SET = %q, want from_real_env (real env wins)", got)
	}
}

func TestLoadDotEnv_MissingFileIsNoop(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	_ = os.Chdir(dir)
	// Should not panic or error when no .env exists.
	LoadDotEnv()
}
