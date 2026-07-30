//go:build legacy

package mysql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
	"github.com/bottle-note/mfds-crawler/secrets/project/mfds/sqlcgen"
)

func appendEvent(ctx context.Context, q *sqlcgen.Queries, params weblist.EventParams) error {
	metadata := params.MetadataJSON
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{"version":1}`)
	}
	id, err := q.CreateCrawlEvent(ctx, sqlcgen.CreateCrawlEventParams{
		RunID: params.RunID, PartitionID: nullInt64(params.PartitionID),
		PageID: nullInt64(params.PageID), WorkerID: nullString(params.WorkerID),
		Level: params.Level, Phase: params.Phase, Message: params.Message,
		MetadataJson: metadata,
	})
	if err != nil || id <= 0 {
		return fmt.Errorf("crawl event 저장 실패: id=%d: %w", id, err)
	}
	return nil
}

func eventJSON(values map[string]any) json.RawMessage {
	values["version"] = 1
	encoded, _ := json.Marshal(values)
	return encoded
}

func (s *Store) AppendLeaseLostEvent(ctx context.Context, params weblist.EventParams) {
	_ = appendEvent(ctx, sqlcgen.New(s.db), params)
}
