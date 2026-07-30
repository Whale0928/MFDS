package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/bottle-note/mfds-crawler/internal/config"
	storemysql "github.com/bottle-note/mfds-crawler/internal/store/mysql"
	"github.com/bottle-note/mfds-crawler/internal/usecase/overview"
	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

type fakeDatabase struct {
	pinged   bool
	migrated bool
}

func (f *fakeDatabase) Ping(context.Context) error {
	f.pinged = true
	return nil
}

func (f *fakeDatabase) Migrate(context.Context) error {
	f.migrated = true
	return nil
}

func (f *fakeDatabase) MigrationStatus(context.Context) ([]storemysql.MigrationStatus, error) {
	return nil, nil
}

func (f *fakeDatabase) LoadOverview(context.Context, int32) (overview.Snapshot, error) {
	return overview.Snapshot{}, nil
}

func (f *fakeDatabase) LoadRunDetail(context.Context, uint64) (overview.RunDetail, error) {
	return overview.RunDetail{}, nil
}

func (f *fakeDatabase) Close() error {
	return nil
}

func TestRootCommand_인자가없으면TUI를실행한다(t *testing.T) {
	var called bool
	root, output := newTestRoot(t, func(_ context.Context, _ config.Config, deps TUIDependencies, _ io.Writer) error {
		if deps.OpenDatabase == nil || deps.RunWebListJob == nil {
			t.Fatal("TUI dependencies are incomplete")
		}
		called = true
		return nil
	})
	root.SetArgs(nil)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output = %q", err, output.String())
	}
	if !called {
		t.Fatal("TUI was not called")
	}
}

func TestRootCommand_ConfigValidate_검증결과를출력한다(t *testing.T) {
	root, output := newTestRoot(t, noOpTUI)
	root.SetArgs([]string{"config", "validate"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(output.String(), "설정 정상: targets=4") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRootCommand_Migrate_DB를확인하고Migration을적용한다(t *testing.T) {
	fake := &fakeDatabase{}
	root, output := newTestRootWithDatabase(t, noOpTUI, fake)
	root.SetArgs([]string{"migrate"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !fake.pinged || !fake.migrated {
		t.Fatalf("pinged=%t migrated=%t", fake.pinged, fake.migrated)
	}
	if !strings.Contains(output.String(), "migration 적용 완료") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRootCommand_미구현수집명령은오류를반환한다(t *testing.T) {
	root, _ := newTestRoot(t, noOpTUI)
	root.SetArgs([]string{"api", "backfill"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil")
	}
	if !strings.Contains(err.Error(), "아직 구현되지 않았습니다") {
		t.Fatalf("error = %q", err)
	}
}

func newTestRoot(
	t *testing.T,
	runTUI func(context.Context, config.Config, TUIDependencies, io.Writer) error,
) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	return newTestRootWithRunner(t, runTUI, &fakeDatabase{}, successfulWebListBackfill)
}

func newTestRootWithDatabase(
	t *testing.T,
	runTUI func(context.Context, config.Config, TUIDependencies, io.Writer) error,
	database Database,
) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	return newTestRootWithRunner(t, runTUI, database, successfulWebListBackfill)
}

func newTestRootWithRunner(
	t *testing.T,
	runTUI func(context.Context, config.Config, TUIDependencies, io.Writer) error,
	database Database,
	runWebListBackfill RunWebListBackfillFunc,
) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	configFile, envFile := writeCLIConfig(t)
	output := &bytes.Buffer{}
	root, err := NewRootCommand(Dependencies{
		Loader: config.NewLoader(),
		OpenDatabase: func(config.DatabaseConfig) (Database, error) {
			return database, nil
		},
		RunTUI:             runTUI,
		RunWebListBackfill: runWebListBackfill,
		RunWebListJob: func(context.Context, config.Config, weblist.JobCommand) (weblist.JobResult, error) {
			return weblist.JobResult{}, nil
		},
		Out:    output,
		ErrOut: output,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := root.PersistentFlags().Set("config", configFile); err != nil {
		t.Fatal(err)
	}
	if err := root.PersistentFlags().Set("env-file", envFile); err != nil {
		t.Fatal(err)
	}
	return root, output
}

func noOpTUI(context.Context, config.Config, TUIDependencies, io.Writer) error {
	return nil
}

func successfulWebListBackfill(
	context.Context,
	config.Config,
	weblist.Command,
) (weblist.Result, error) {
	return weblist.Result{}, nil
}

func writeCLIConfig(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	envFile := filepath.Join(dir, ".env")
	yaml := `
api:
  base_url: https://api.example.com
web:
  base_url: https://web.example.com
database:
  conn_max_lifetime: 30m
  conn_max_idle_time: 5m
targets:
  - {name: 위스키, code: C0314210000000000000}
  - {name: 브랜디, code: C0314220000000000000}
  - {name: 일반증류주, code: C0314230000000000000}
  - {name: 리큐르, code: C0314240000000000000}
`
	env := "MFDS_API_KEY=test-key\nMYSQL_DSN=test:test@tcp(localhost:3306)/test\n"
	if err := os.WriteFile(configFile, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	return configFile, envFile
}
