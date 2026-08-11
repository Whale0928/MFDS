package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/reference"
	storemysql "github.com/bottle-note/mfds-crawler/internal/store/mysql"
)

const referenceSourceDSNEnv = "BOTTLENOTE_REFERENCE_DSN"

func runReferenceSync(
	ctx context.Context,
	cfg config.Config,
) (result reference.Result, runErr error) {
	sourceDSN := strings.TrimSpace(os.Getenv(referenceSourceDSNEnv))
	if sourceDSN == "" {
		return reference.Result{}, fmt.Errorf("%s 환경변수가 필요합니다", referenceSourceDSNEnv)
	}
	store, err := storemysql.Open(cfg.Database)
	if err != nil {
		return reference.Result{}, err
	}
	defer func() {
		runErr = errors.Join(runErr, store.Close())
	}()
	if err := store.Migrate(ctx); err != nil {
		return reference.Result{}, err
	}
	return store.SyncReferences(ctx, sourceDSN)
}
