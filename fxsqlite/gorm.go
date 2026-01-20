package fxsqlite

import (
	"database/sql"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"moul.io/zapgorm2"
)

// NewGormDB creates a new gorm database from the given sqlite sql.DB
func NewGormDB(db *sql.DB, logger *zap.Logger) (*gorm.DB, error) {
	conf := &gorm.Config{}
	if logger != nil {
		conf.Logger = zapgorm2.New(logger)
	}
	return gorm.Open(&sqlite.Dialector{Conn: db}, conf)
}
