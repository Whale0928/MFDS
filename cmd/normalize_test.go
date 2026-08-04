package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/usecase/normalization"
)

func TestNormalize_기본실행_Config기본값으로Runner를호출한다(t *testing.T) {
	// Given
	var captured normalization.Command
	run := func(_ context.Context, _ config.Config, command normalization.Command) (normalization.Summary, error) {
		captured = command
		return normalization.Summary{Processed: map[normalization.Status]int{
			normalization.StatusNormalized: 2,
		}, RemainingPending: 7, RemainingStale: 1}, nil
	}
	command := newNormalizeCommand(func() config.Config { return config.Config{} }, run, &strings.Builder{})

	// When
	err := command.Execute()

	// Then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if captured.Limit != 0 || captured.RCNO != "" || captured.DryRun {
		t.Fatalf("command = %+v", captured)
	}
}

func TestNormalize_Limit과RCNO동시지정_오류를반환한다(t *testing.T) {
	// Given
	called := false
	command := newNormalizeCommand(func() config.Config { return config.Config{} }, func(context.Context, config.Config, normalization.Command) (normalization.Summary, error) {
		called = true
		return normalization.Summary{}, nil
	}, &strings.Builder{})
	command.SetArgs([]string{"--limit", "2", "--rcno", "R-1"})

	// When
	err := command.Execute()

	// Then
	if err == nil || !strings.Contains(err.Error(), "함께 사용할 수 없습니다") {
		t.Fatalf("Execute() error = %v", err)
	}
	if called {
		t.Fatal("runner was called")
	}
}

func TestNormalize_DryRun_Runner결과를출력한다(t *testing.T) {
	// Given
	output := &strings.Builder{}
	command := newNormalizeCommand(func() config.Config { return config.Config{} }, func(_ context.Context, _ config.Config, received normalization.Command) (normalization.Summary, error) {
		if !received.DryRun {
			t.Fatal("DryRun = false")
		}
		return normalization.Summary{Processed: map[normalization.Status]int{
			normalization.StatusPartial: 1,
		}, RemainingPending: 3, RemainingStale: 2}, nil
	}, output)
	command.SetArgs([]string{"--dry-run"})

	// When
	err := command.Execute()

	// Then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "normalize 결과: normalized=0 partial=1 review_required=0 unparsed=0 system_failures=0 remaining_pending=3 remaining_stale=2 dry_run=true\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestNormalize_SystemFailure_Summary를출력하고오류를반환한다(t *testing.T) {
	// Given
	output := &strings.Builder{}
	command := newNormalizeCommand(func() config.Config { return config.Config{} }, func(context.Context, config.Config, normalization.Command) (normalization.Summary, error) {
		return normalization.Summary{SystemFailures: 1}, errors.New("database unavailable")
	}, output)

	// When
	err := command.Execute()

	// Then
	if err == nil || !strings.Contains(err.Error(), "database unavailable") {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(output.String(), "system_failures=1") {
		t.Fatalf("output = %q", output.String())
	}
}
