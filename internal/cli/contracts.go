package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bottle-note/mfds-crawler/internal/config"
)

func addCollectorContracts(
	root *cobra.Command,
	getConfig func() config.Config,
	runWebListBackfill RunWebListBackfillFunc,
	runWebListJob RunWebListJobFunc,
) error {
	api := &cobra.Command{Use: "api", Short: "공식 Open API를 수집합니다"}
	apiBackfill := unavailableCommand("backfill", "API 최대 기간을 수집합니다")
	apiBackfill.Flags().String("from", "2022-11-25", "수집 시작일")
	apiBackfill.Flags().String("to", "today", "수집 종료일")
	apiBackfill.Flags().String("items", "위스키,브랜디,일반증류주,리큐르", "대상 품목")
	api.AddCommand(apiBackfill, unavailableCommand("probe-page-size", "API 페이지 크기를 탐색합니다"))

	web := &cobra.Command{Use: "web", Short: "수입식품정보마루를 수집합니다"}
	listBackfill, err := newWebListBackfillCommand(getConfig, runWebListBackfill, root.OutOrStdout())
	if err != nil {
		return err
	}
	listJob, err := newWebListJobCommand(getConfig, runWebListJob, root.OutOrStdout())
	if err != nil {
		return err
	}
	detailBackfill := unavailableCommand("detail-backfill", "웹 상세를 수집합니다")
	detailBackfill.Flags().Bool("only-missing", true, "상세 성공 기록이 없는 rcno만 조회")
	web.AddCommand(listBackfill, listJob, detailBackfill)

	all := &cobra.Command{Use: "all", Short: "전체 수집 파이프라인을 실행합니다"}
	allBackfill := unavailableCommand("backfill", "API와 웹 전체를 수집합니다")
	allBackfill.Flags().String("api-from", "2022-11-25", "API 수집 시작일")
	allBackfill.Flags().String("to", "today", "수집 종료일")
	allBackfill.Flags().String("web-scope", "all-public", "웹 수집 범위")
	all.AddCommand(allBackfill)

	run := &cobra.Command{Use: "run", Short: "수집 실행을 관리합니다"}
	resume := unavailableCommand("resume", "중단된 실행을 재개합니다")
	resume.Flags().Uint64("run-id", 0, "재개할 run ID")
	run.AddCommand(resume)

	verify := &cobra.Command{Use: "verify", Short: "원장 데이터를 검증합니다"}
	overlap := unavailableCommand("overlap", "API와 웹의 겹치는 기간을 비교합니다")
	overlap.Flags().String("from", "", "비교 시작일")
	overlap.Flags().String("to", "", "비교 종료일")
	overlap.Flags().String("item", "", "비교 품목")
	verify.AddCommand(overlap)

	root.AddCommand(api, web, all, run, verify)
	return nil
}

func unavailableCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short + " (준비 중)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("%s 기능은 초기 기반 단계에서 아직 구현되지 않았습니다", use)
		},
	}
}
