package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestLoader_Load_설정우선순위를적용한다(t *testing.T) {
	configFile, envFile := writeTestConfig(t)
	t.Setenv("API_QPS", "8")

	loader := NewLoader()
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Float64("api-qps", 0, "")
	if err := loader.BindFlag("api.qps", flags.Lookup("api-qps")); err != nil {
		t.Fatal(err)
	}
	if err := flags.Parse([]string{"--api-qps", "9"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := loader.Load(configFile, envFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.API.QPS != 9 {
		t.Fatalf("API.QPS = %v, want 9", cfg.API.QPS)
	}
	if cfg.Database.MaxOpenConns != 10 {
		t.Fatalf("Database.MaxOpenConns = %d, want 10", cfg.Database.MaxOpenConns)
	}
}

func TestConfig_Validate_API키가비어있으면오류를반환한다(t *testing.T) {
	configFile, envFile := writeTestConfig(t)
	content, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte(strings.ReplaceAll(string(content), "MFDS_API_KEY=test-key\n", "")), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := NewLoader().Load(configFile, envFile)
	if err == nil {
		t.Fatalf("Load() config = %+v, want error", cfg)
	}
	if !strings.Contains(err.Error(), "MFDS_API_KEY") {
		t.Fatalf("error = %q, want MFDS_API_KEY", err)
	}
}

func TestConfig_Validate_프록시URL이없으면오류를반환한다(t *testing.T) {
	configFile, envFile := writeTestConfig(t)
	t.Setenv("MFDS_WEB_PROXY_MODE", "always")
	t.Setenv("MFDS_WEB_PROXY_URL", "")

	_, err := NewLoader().Load(configFile, envFile)
	if err == nil {
		t.Fatal("Load() error = nil, want proxy URL error")
	}
	if !strings.Contains(err.Error(), "MFDS_WEB_PROXY_URL") {
		t.Fatalf("error = %q, want MFDS_WEB_PROXY_URL", err)
	}
}

func TestConfig_Validate_웹목록PageSize가50초과면오류를반환한다(t *testing.T) {
	configFile, envFile := writeTestConfig(t)
	t.Setenv("WEB_LIST_PAGE_SIZE", "51")

	_, err := NewLoader().Load(configFile, envFile)
	if err == nil || !strings.Contains(err.Error(), "50 이하") {
		t.Fatalf("Load() error = %v", err)
	}
}

func writeTestConfig(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	envFile := filepath.Join(dir, ".env")
	yaml := `
api:
  base_url: https://api.example.com
  page_size: 100
  partition_workers: 4
  page_workers: 2
  qps: 3
  burst: 3
web:
  base_url: https://web.example.com
  list_page_size: 10
  list_workers: 2
  detail_workers: 2
  qps: 1
database:
  max_open_conns: 10
  max_idle_conns: 5
  conn_max_lifetime: 30m
  conn_max_idle_time: 5m
proxy:
  mode: off
retry:
  max_attempts: 3
  delays: [2s, 5s, 15s]
targets:
  - {name: 위스키, code: C0314210000000000000}
  - {name: 브랜디, code: C0314220000000000000}
  - {name: 일반증류주, code: C0314230000000000000}
  - {name: 리큐르, code: C0314240000000000000}
`
	env := "MFDS_API_KEY=test-key\nMYSQL_DSN=test:test@tcp(localhost:3306)/test\nAPI_QPS=7\n"
	if err := os.WriteFile(configFile, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	return configFile, envFile
}
