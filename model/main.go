// Package model defines the GORM data models and DAO functions.
//
// Cross-database rule (mirrors new-api): all three of SQLite, MySQL >= 5.7.8 and
// PostgreSQL >= 9.6 must work. Prefer GORM methods over raw SQL; when raw SQL is
// unavoidable use the commonGroupCol / commonKeyCol / commonTrueVal helpers.
package model

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// DB is the process-wide database handle.
var DB *gorm.DB

// Driver flags, set during Init based on SQL_DSN.
var (
	UsingSQLite     bool
	UsingMySQL      bool
	UsingPostgreSQL bool
)

// Cross-DB column-quoting / boolean-literal helpers (see new-api model/main.go).
var (
	commonGroupCol string
	commonKeyCol   string
	commonTrueVal  string
	commonFalseVal string
)

func initCol() {
	if UsingPostgreSQL {
		commonGroupCol, commonKeyCol, commonTrueVal, commonFalseVal = `"group"`, `"key"`, "true", "false"
	} else {
		commonGroupCol, commonKeyCol, commonTrueVal, commonFalseVal = "`group`", "`key`", "1", "0"
	}
}

// Init opens the database from SQL_DSN, runs migrations and prepares cross-DB
// helpers. SQL_DSN selects the driver:
//   - empty                          -> SQLite at the default file "modex-cloud.db"
//   - "sqlite://PATH" / "file:PATH"  -> SQLite at PATH (explicit)
//   - a bare filesystem path or *.db -> SQLite at that path
//   - "postgres://" / "postgresql://"-> PostgreSQL
//   - anything else                  -> MySQL
func Init() error {
	dsn := os.Getenv("SQL_DSN")
	var dialector gorm.Dialector
	switch {
	case dsn == "":
		UsingSQLite = true
		dialector = sqlite.Open("modex-cloud.db")
	case isSQLiteDSN(dsn):
		UsingSQLite = true
		dialector = sqlite.Open(sqliteFilePath(dsn))
	case strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://"):
		UsingPostgreSQL = true
		dialector = postgres.Open(dsn)
	default:
		UsingMySQL = true
		dialector = mysql.Open(dsn)
	}
	initCol()

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger:                                   gormlogger.Default.LogMode(gormlogger.Warn),
		PrepareStmt:                              true,
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	DB = db
	if err := migrate(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// isSQLiteDSN reports whether the DSN names a SQLite database rather than a
// MySQL/Postgres server. We treat an explicit "sqlite://"/"file:" prefix, an
// absolute or relative filesystem path, or a "*.db" value as SQLite. MySQL DSNs
// (e.g. "user:pass@tcp(host)/db") never match these shapes.
func isSQLiteDSN(dsn string) bool {
	switch {
	case strings.HasPrefix(dsn, "sqlite://"), strings.HasPrefix(dsn, "file:"):
		return true
	case strings.HasPrefix(dsn, "/"), strings.HasPrefix(dsn, "./"), strings.HasPrefix(dsn, "../"):
		return true
	case strings.HasSuffix(dsn, ".db"), strings.HasSuffix(dsn, ".sqlite"):
		return true
	default:
		return false
	}
}

// sqliteFilePath strips the optional "sqlite://" scheme, leaving the raw file
// path the SQLite driver expects. "file:" DSNs are passed through unchanged
// (the driver understands them, including ?cache=... query params).
func sqliteFilePath(dsn string) string {
	if strings.HasPrefix(dsn, "sqlite://") {
		return strings.TrimPrefix(dsn, "sqlite://")
	}
	return dsn
}

func migrate() error {
	return DB.AutoMigrate(
		&User{},
		&Platform{},
		&Grant{},
		&Channel{},
		&AuditLog{},
	)
}

// nowUnix is a thin wrapper so model code reads cleanly. (Date.now equivalent.)
func nowUnix() int64 { return time.Now().Unix() }

func logMigrate(msg string) { log.Println("[migrate]", msg) }
