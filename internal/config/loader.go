package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type Loader struct {
	v *viper.Viper
}

func NewLoader() *Loader {
	v := viper.New()
	v.SetConfigType("yaml")
	setDefaults(v)
	bindEnvironment(v)
	return &Loader{v: v}
}

func (l *Loader) BindFlag(key string, flag *pflag.Flag) error {
	return l.v.BindPFlag(key, flag)
}

func (l *Loader) Load(configFile, envFile string) (Config, error) {
	restoreEnv, err := loadEnvFile(envFile)
	if err != nil {
		return Config{}, err
	}
	defer restoreEnv()

	l.v.SetConfigFile(configFile)
	if err := l.v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("설정 파일 읽기 실패: %w", err)
	}

	var cfg Config
	if err := l.v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("설정 디코딩 실패: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadEnvFile(path string) (func(), error) {
	if path == "" {
		return func() {}, nil
	}
	values, err := godotenv.Read(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && path == ".env" {
			return func() {}, nil
		}
		return nil, fmt.Errorf("환경 파일 읽기 실패: %w", err)
	}
	var added []string
	for key, value := range values {
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			for _, addedKey := range added {
				_ = os.Unsetenv(addedKey)
			}
			return nil, fmt.Errorf("환경 변수 %s 설정 실패: %w", key, err)
		}
		added = append(added, key)
	}
	return func() {
		for _, key := range added {
			_ = os.Unsetenv(key)
		}
	}, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("timezone", "Asia/Seoul")
	v.SetDefault("api.page_size", 100)
	v.SetDefault("api.partition_workers", 4)
	v.SetDefault("api.page_workers", 2)
	v.SetDefault("api.qps", 3)
	v.SetDefault("api.burst", 3)
	v.SetDefault("web.list_page_size", 50)
	v.SetDefault("web.list_workers", 2)
	v.SetDefault("web.detail_workers", 2)
	v.SetDefault("web.qps", 1)
	v.SetDefault("database.max_open_conns", 10)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", "30m")
	v.SetDefault("database.conn_max_idle_time", "5m")
	v.SetDefault("proxy.mode", "off")
	v.SetDefault("retry.max_attempts", 3)
	v.SetDefault("retry.delays", []string{"2s", "5s", "15s"})
}

func bindEnvironment(v *viper.Viper) {
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	bindings := map[string][]string{
		"timezone":                {"TZ"},
		"api.key":                 {"MFDS_API_KEY"},
		"api.base_url":            {"MFDS_API_BASE_URL"},
		"api.page_size":           {"API_PAGE_SIZE"},
		"api.partition_workers":   {"API_PARTITION_WORKERS"},
		"api.page_workers":        {"API_PAGE_WORKERS"},
		"api.qps":                 {"API_QPS"},
		"web.base_url":            {"MFDS_WEB_BASE_URL"},
		"web.list_page_size":      {"WEB_LIST_PAGE_SIZE"},
		"web.list_workers":        {"WEB_LIST_WORKERS"},
		"web.detail_workers":      {"WEB_DETAIL_WORKERS"},
		"web.qps":                 {"WEB_QPS"},
		"database.dsn":            {"MYSQL_DSN"},
		"database.max_open_conns": {"MYSQL_MAX_OPEN_CONNS"},
		"database.max_idle_conns": {"MYSQL_MAX_IDLE_CONNS"},
		"proxy.mode":              {"MFDS_WEB_PROXY_MODE"},
		"proxy.url":               {"MFDS_WEB_PROXY_URL"},
		"proxy.label":             {"MFDS_WEB_PROXY_LABEL"},
	}
	for key, names := range bindings {
		_ = v.BindEnv(append([]string{key}, names...)...)
	}
}
