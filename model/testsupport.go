package model

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// InitForTest points the package-global DB at a fresh in-memory SQLite database
// and migrates the schema. Exported so other packages' tests can set up the
// model layer. NOT for production use.
//
// It returns a cleanup function the caller should defer. It deliberately does
// not import "testing", so it never bloats the production binary.
func InitForTest() (cleanup func(), err error) {
	UsingSQLite, UsingMySQL, UsingPostgreSQL = true, false, false
	initCol()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return func() {}, err
	}
	DB = db
	if err := migrate(); err != nil {
		return func() {}, err
	}
	return func() {
		if sqlDB, e := DB.DB(); e == nil {
			_ = sqlDB.Close()
		}
	}, nil
}
