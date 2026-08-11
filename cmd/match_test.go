package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/bottle-note/mfds-crawler/internal/config"
	usecase "github.com/bottle-note/mfds-crawler/internal/usecase/matching"
)

func TestMatchCommand_AllDryRun_옵션과요약을전달한다(t *testing.T) {
	// Given
	var received usecase.Command
	out := &bytes.Buffer{}
	command := newMatchCommand(func() config.Config { return config.Config{} }, func(
		_ context.Context, _ config.Config, value usecase.Command,
	) (usecase.Summary, error) {
		received = value
		return usecase.Summary{Processed: 12, DistilleryMatched: 8, RegionMatched: 7, NoMatch: 3, Remaining: 0, MatchingVersion: "v1:hash"}, nil
	}, out)
	command.SetArgs([]string{"--all", "--dry-run"})

	// When
	err := command.Execute()

	// Then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !received.All || !received.DryRun {
		t.Fatalf("command = %+v", received)
	}
	if !strings.Contains(out.String(), "processed=12") || !strings.Contains(out.String(), "version=v1:hash") {
		t.Fatalf("output = %q", out.String())
	}
}
