package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/secrets/project/mfds/migrations"
)

var migrationMu sync.Mutex

type Store struct {
	db *sql.DB
}

type MigrationStatus struct {
	Version   int64
	Applied   bool
	AppliedAt time.Time
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

func (s *Store) MigrationStatus(ctx context.Context) ([]MigrationStatus, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT version_id, is_applied, tstamp
		FROM goose_db_version
		ORDER BY id
	`)
	if err != nil {
		var mysqlErr *driver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1146 {
			return nil, nil
		}
		return nil, fmt.Errorf("migration 상태 조회 실패: %w", err)
	}
	defer rows.Close()

	var statuses []MigrationStatus
	for rows.Next() {
		var status MigrationStatus
		if err := rows.Scan(&status.Version, &status.Applied, &status.AppliedAt); err != nil {
			return nil, fmt.Errorf("migration 상태 읽기 실패: %w", err)
		}
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migration 상태 순회 실패: %w", err)
	}
	return statuses, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func isRetryableTransactionError(err error) bool {
	var mysqlErr *driver.MySQLError
	return errors.As(err, &mysqlErr) && (mysqlErr.Number == 1205 || mysqlErr.Number == 1213)
}
