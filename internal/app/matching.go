package app

import (
	"context"
	"errors"

	"github.com/bottle-note/mfds-crawler/internal/config"
	storemysql "github.com/bottle-note/mfds-crawler/internal/store/mysql"
	usecase "github.com/bottle-note/mfds-crawler/internal/usecase/matching"
)

func runMatching(
	ctx context.Context,
	cfg config.Config,
	command usecase.Command,
) (summary usecase.Summary, runErr error) {
	store, err := storemysql.Open(cfg.Database)
	if err != nil {
		return usecase.Summary{}, err
	}
	defer func() {
		runErr = errors.Join(runErr, store.Close())
	}()
	matcher, err := store.LoadMatchingSnapshot(ctx)
	if err != nil {
		return usecase.Summary{}, err
	}
	service, err := usecase.NewService(store, matcher, usecase.Options{DefaultLimit: cfg.Normalization.RunLimit})
	if err != nil {
		return usecase.Summary{}, err
	}
	return service.Execute(ctx, command)
}
