package model

import "testing"

// TestIsSQLiteDSN guards the driver-selection logic: an explicit SQLite path or
// scheme must be recognized as SQLite, while MySQL/Postgres DSNs must not be.
// This is the exact classification that regressed when SQL_DSN=/data/...db was
// misrouted to the MySQL driver.
func TestIsSQLiteDSN(t *testing.T) {
	sqliteDSNs := []string{
		"/data/modex-cloud.db",
		"./modex-cloud.db",
		"../db/modex-cloud.db",
		"sqlite:///data/modex-cloud.db",
		"file:modex-cloud.db?cache=shared",
		"modex-cloud.db",
		"backup.sqlite",
	}
	for _, dsn := range sqliteDSNs {
		if !isSQLiteDSN(dsn) {
			t.Errorf("isSQLiteDSN(%q) = false, want true", dsn)
		}
	}

	serverDSNs := []string{
		"root:pass@tcp(mysql:3306)/modex_cloud?charset=utf8mb4&parseTime=True",
		"postgres://user:pass@host:5432/modex_cloud?sslmode=require",
		"user:pass@tcp(127.0.0.1:3306)/db",
	}
	for _, dsn := range serverDSNs {
		if isSQLiteDSN(dsn) {
			t.Errorf("isSQLiteDSN(%q) = true, want false", dsn)
		}
	}
}

// TestSQLiteFilePath proves the "sqlite://" scheme is stripped to a bare path
// while plain paths and "file:" DSNs pass through unchanged.
func TestSQLiteFilePath(t *testing.T) {
	cases := map[string]string{
		"sqlite:///data/modex-cloud.db": "/data/modex-cloud.db",
		"/data/modex-cloud.db":          "/data/modex-cloud.db",
		"file:modex-cloud.db?cache=shared": "file:modex-cloud.db?cache=shared",
	}
	for in, want := range cases {
		if got := sqliteFilePath(in); got != want {
			t.Errorf("sqliteFilePath(%q) = %q, want %q", in, got, want)
		}
	}
}
