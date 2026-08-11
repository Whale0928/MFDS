package app

import (
	"context"
	"fmt"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/reference"
)

func runReferenceSync(
	ctx context.Context,
	cfg config.Config,
) (result reference.Result, runErr error) {
	_ = ctx
	_ = cfg
	return reference.Result{}, fmt.Errorf("BottleNote 원본 기준 테이블을 직접 사용하므로 reference-sync를 지원하지 않습니다")
}
