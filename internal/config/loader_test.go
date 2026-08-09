package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoader_Load_YAML고정값과환경비밀값을결합한다(t *testing.T) {
	configFile, envFile := writeTestConfig(t)
	t.Setenv("WEB_QPS", "8")

	cfg, err := NewLoader().Load(configFile, envFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Web.QPS != 1 {
		t.Fatalf("Web.QPS = %v, want YAML value 1", cfg.Web.QPS)
	}
	if cfg.Database.MaxOpenConns != 10 {
		t.Fatalf("Database.MaxOpenConns = %d, want 10", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.DSN != "test:test@tcp(localhost:3306)/test" {
		t.Fatalf("database DSN was not loaded")
	}
}

func TestConfig_Validate_DSN이비어있으면오류를반환한다(t *testing.T) {
	configFile, envFile := writeTestConfig(t)
	content, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte(strings.ReplaceAll(string(content), "MYSQL_DSN=test:test@tcp(localhost:3306)/test\n", "")), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := NewLoader().Load(configFile, envFile)
	if err == nil {
		t.Fatalf("Load() config = %+v, want error", cfg)
	}
	if !strings.Contains(err.Error(), "MYSQL_DSN") {
		t.Fatalf("error = %q, want MYSQL_DSN", err)
	}
}

func TestConfig_Validate_웹QPS가0이면오류를반환한다(t *testing.T) {
	configFile, envFile := writeTestConfig(t)
	content, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, []byte(strings.ReplaceAll(string(content), "qps: 1", "qps: 0")), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = NewLoader().Load(configFile, envFile)
	if err == nil {
		t.Fatal("Load() error = nil, want web QPS error")
	}
	if !strings.Contains(err.Error(), "0보다 커야 합니다") {
		t.Fatalf("error = %q, want positive web QPS", err)
	}
}

func TestConfig_Validate_웹목록PageSize가50초과면오류를반환한다(t *testing.T) {
	configFile, envFile := writeTestConfig(t)
	content, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, []byte(strings.ReplaceAll(string(content), "list_page_size: 10", "list_page_size: 51")), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = NewLoader().Load(configFile, envFile)
	if err == nil || !strings.Contains(err.Error(), "50 이하") {
		t.Fatalf("Load() error = %v", err)
	}
}

// clearBoundEnv는 loader가 바인딩하는 OS 환경변수를 테스트 동안 비운다.
// OS 값이 .env 파일보다 우선하는 설계라, 이걸 비우지 않으면 개발자 셸에
// MYSQL_DSN이 export되어 있는지에 따라 테스트 결과가 달라진다.
func clearBoundEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"MYSQL_DSN"} {
		value, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Setenv(key, value); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func writeTestConfig(t *testing.T) (string, string) {
	t.Helper()
	clearBoundEnv(t)
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	envFile := filepath.Join(dir, ".env")
	yaml := `
web:
  base_url: https://web.example.com
  list_page_size: 10
  list_workers: 2
  qps: 1
database:
  max_open_conns: 10
  max_idle_conns: 5
  conn_max_lifetime: 30m
  conn_max_idle_time: 5m
retry:
  max_attempts: 3
  delays: [2s, 5s, 15s]
normalization:
  run_limit: 100
  lease_duration: 5m
  max_attempts: 3
  retry_delay: 5m
targets:
  - {name: 위스키, code: C0314210000000000000}
  - {name: 브랜디, code: C0314220000000000000}
  - {name: 일반증류주, code: C0314230000000000000}
  - {name: 리큐르, code: C0314240000000000000}
`
	env := "MYSQL_DSN=test:test@tcp(localhost:3306)/test\n"
	if err := os.WriteFile(configFile, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	return configFile, envFile
}
