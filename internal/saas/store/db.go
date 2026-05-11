package store

import (
	"database/sql"
	"fmt"
	"time"

	"quantsaas/internal/saas/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type DB struct {
	gorm *gorm.DB
	sql  *sql.DB
}

func NewDB(cfg config.DatabaseConfig) (*DB, error) {
	gormDB, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open gorm postgres: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeMinutes) * time.Minute)

	if err := gormDB.AutoMigrate(AllModels()...); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	return &DB{gorm: gormDB, sql: sqlDB}, nil
}

func (db *DB) GormDB() *gorm.DB {
	return db.gorm
}

func (db *DB) Close() error {
	if db == nil || db.sql == nil {
		return nil
	}
	return db.sql.Close()
}
