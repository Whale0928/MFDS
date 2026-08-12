package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/reference"
)

func TestReferenceSyncCommand_동기화된행수와해시를출력한다(t *testing.T) {
	// Given
	out := &bytes.Buffer{}
	command := newReferenceSyncCommand(func() config.Config { return config.Config{} }, func(
		context.Context, config.Config,
	) (reference.Result, error) {
		return reference.Result{
			Alcohols:     reference.TableResult{Count: 3305, Hash: "alcohol-hash"},
			Distilleries: reference.TableResult{Count: 177, Hash: "distillery-hash"},
			Regions:      reference.TableResult{Count: 33, Hash: "region-hash"},
		}, nil
	}, out)

	// When
	err := command.Execute()

	// Then
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"alcohols=3305", "distilleries=177", "regions=33", "alcohols_hash=alcohol-hash"} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("output = %q, missing %q", out.String(), expected)
		}
	}
}
