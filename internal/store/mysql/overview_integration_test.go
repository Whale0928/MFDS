package mysql

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/usecase/overview"
)

func TestOverviewIntegration_현재원장현황과최근실행을조회한다(t *testing.T) {
	dsn := os.Getenv("MFDS_INTEGRATION_DSN")
	if dsn == "" {
		t.Skip("MFDS_INTEGRATION_DSN이 설정되지 않았습니다")
	}
	store, err := Open(config.DatabaseConfig{
		DSN:             dsn,
		MaxOpenConns:    2,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	snapshot, err := store.LoadOverview(context.Background(), overview.RecentRunLimit)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.TotalRuns < int64(len(snapshot.RecentRuns)) {
		t.Fatalf("TotalRuns=%d RecentRuns=%d", snapshot.TotalRuns, len(snapshot.RecentRuns))
	}
	if len(snapshot.RecentRuns) > overview.RecentRunLimit {
		t.Fatalf("RecentRuns=%d", len(snapshot.RecentRuns))
	}
	for index := 1; index < len(snapshot.RecentRuns); index++ {
		if snapshot.RecentRuns[index-1].ID <= snapshot.RecentRuns[index].ID {
			t.Fatalf("최근 실행 정렬이 유효하지 않습니다: %d <= %d",
				snapshot.RecentRuns[index-1].ID,
				snapshot.RecentRuns[index].ID,
			)
		}
	}
	if len(snapshot.RecentRuns) > 0 {
		detail, err := store.LoadRunDetail(context.Background(), snapshot.RecentRuns[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if detail.RunID != snapshot.RecentRuns[0].ID {
			t.Fatalf("RunID=%d want=%d", detail.RunID, snapshot.RecentRuns[0].ID)
		}
		if len(detail.Partitions) > int(snapshot.RecentRuns[0].TotalPartitions) {
			t.Fatalf("Tasks=%d total=%d", len(detail.Partitions), snapshot.RecentRuns[0].TotalPartitions)
		}
	}
}
