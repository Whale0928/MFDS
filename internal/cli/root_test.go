package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/bottle-note/mfds-crawler/internal/config"
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

func (f *fakeDatabase) Close() error {
	return nil
}

func TestRootCommand_인자가없으면도움말을출력한다(t *testing.T) {
	root, output := newTestRoot(t)
	root.SetArgs(nil)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output = %q", err, output.String())
	}
	if !strings.Contains(output.String(), "Usage:") ||
		!strings.Contains(output.String(), "Available Commands:") {
		t.Fatalf("output = %q", output.String())
	}
	for _, command := range []string{"collect", "health", "migrate"} {
		if !strings.Contains(output.String(), command) {
			t.Fatalf("command %q missing from output = %q", command, output.String())
		}
	}
	for _, removed := range []string{"all", "api", "completion", "config", "db", "run", "verify", "web"} {
		if strings.Contains(output.String(), "\n  "+removed+" ") {
			t.Fatalf("removed command %q remains in output = %q", removed, output.String())
		}
	}
}

func TestRootCommand_Health_설정과DB연결결과를출력한다(t *testing.T) {
	fake := &fakeDatabase{}
	root, output := newTestRootWithDatabase(t, fake)
	root.SetArgs([]string{"health"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !fake.pinged {
		t.Fatal("Ping() was not called")
	}
	if !strings.Contains(output.String(), "health 정상: config=ok mysql=ok targets=4") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRootCommand_Migrate_DB를확인하고Migration을적용한다(t *testing.T) {
	fake := &fakeDatabase{}
	root, output := newTestRootWithDatabase(t, fake)
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

func newTestRoot(t *testing.T) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	return newTestRootWithRunner(t, &fakeDatabase{}, successfulWebListJob)
}

func newTestRootWithDatabase(
	t *testing.T,
	database Database,
) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	return newTestRootWithRunner(t, database, successfulWebListJob)
}

func newTestRootWithRunner(
	t *testing.T,
	database Database,
	runWebListJob RunWebListJobFunc,
) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	configFile, envFile := writeCLIConfig(t)
	output := &bytes.Buffer{}
	root, err := NewRootCommand(Dependencies{
		Loader: config.NewLoader(),
		OpenDatabase: func(config.DatabaseConfig) (Database, error) {
			return database, nil
		},
		RunWebListJob: runWebListJob,
		Out:           output,
		ErrOut:        output,
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

func successfulWebListJob(
	context.Context,
	config.Config,
	weblist.JobCommand,
) (weblist.JobResult, error) {
	return weblist.JobResult{}, nil
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
