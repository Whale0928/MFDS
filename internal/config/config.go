package config

import "time"

type Config struct {
	Timezone string         `mapstructure:"timezone"`
	API      APIConfig      `mapstructure:"api"`
	Web      WebConfig      `mapstructure:"web"`
	Database DatabaseConfig `mapstructure:"database"`
	Proxy    ProxyConfig    `mapstructure:"proxy"`
	Retry    RetryConfig    `mapstructure:"retry"`
	Targets  []TargetItem   `mapstructure:"targets"`
}

type APIConfig struct {
	Key              string  `mapstructure:"key"`
	BaseURL          string  `mapstructure:"base_url"`
	PageSize         int     `mapstructure:"page_size"`
	PartitionWorkers int     `mapstructure:"partition_workers"`
	PageWorkers      int     `mapstructure:"page_workers"`
	QPS              float64 `mapstructure:"qps"`
	Burst            int     `mapstructure:"burst"`
}

type WebConfig struct {
	BaseURL       string  `mapstructure:"base_url"`
	ListPageSize  int     `mapstructure:"list_page_size"`
	ListWorkers   int     `mapstructure:"list_workers"`
	DetailWorkers int     `mapstructure:"detail_workers"`
	QPS           float64 `mapstructure:"qps"`
}

type DatabaseConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
}

type ProxyConfig struct {
	Mode  string `mapstructure:"mode"`
	URL   string `mapstructure:"url"`
	Label string `mapstructure:"label"`
}

type RetryConfig struct {
	MaxAttempts int             `mapstructure:"max_attempts"`
	Delays      []time.Duration `mapstructure:"delays"`
}

type TargetItem struct {
	Name string `mapstructure:"name"`
	Code string `mapstructure:"code"`
}
