package config

import "time"

type Config struct {
	Timezone      string              `mapstructure:"timezone"`
	Web           WebConfig           `mapstructure:"web"`
	Database      DatabaseConfig      `mapstructure:"database"`
	Retry         RetryConfig         `mapstructure:"retry"`
	Normalization NormalizationConfig `mapstructure:"normalization"`
	Targets       []TargetItem        `mapstructure:"targets"`
}

type WebConfig struct {
	BaseURL      string  `mapstructure:"base_url"`
	ListPageSize int     `mapstructure:"list_page_size"`
	ListWorkers  int     `mapstructure:"list_workers"`
	QPS          float64 `mapstructure:"qps"`
}

type DatabaseConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
}

type RetryConfig struct {
	MaxAttempts int             `mapstructure:"max_attempts"`
	Delays      []time.Duration `mapstructure:"delays"`
}

type NormalizationConfig struct {
	RunLimit      int           `mapstructure:"run_limit"`
	LeaseDuration time.Duration `mapstructure:"lease_duration"`
	MaxAttempts   int           `mapstructure:"max_attempts"`
	RetryDelay    time.Duration `mapstructure:"retry_delay"`
}

type TargetItem struct {
	Name string `mapstructure:"name"`
	Code string `mapstructure:"code"`
}
