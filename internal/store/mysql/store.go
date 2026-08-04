package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	driver "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"

	"github.com/bottle-note/mfds-crawler/git.secrets/project/mfds/migrations"
	"github.com/bottle-note/mfds-crawler/internal/config"
)

var migrationMu sync.Mutex

type Store struct {
	db *sql.DB
}

func Open(cfg config.DatabaseConfig) (*Store, error) {
	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("MySQL 초기화 실패: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	return &Store{db: db}, nil
}

func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("MySQL 연결 실패: %w", err)
	}
	return nil
}

func (s *Store) Migrate(ctx context.Context) error {
	migrationMu.Lock()
	defer migrationMu.Unlock()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("mysql"); err != nil {
		return fmt.Errorf("migration dialect 설정 실패: %w", err)
	}
	if err := goose.UpContext(ctx, s.db, "."); err != nil {
		return fmt.Errorf("migration 적용 실패: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func isRetryableTransactionError(err error) bool {
	var mysqlErr *driver.MySQLError
	return errors.As(err, &mysqlErr) && (mysqlErr.Number == 1205 || mysqlErr.Number == 1213)
}
