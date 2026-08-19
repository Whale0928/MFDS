package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

const (
	DefaultConfigFile = "data/config.yaml"
	DefaultEnvFile    = "git.secrets/project/mfds/.env"
)

type Loader struct {
	v *viper.Viper
}

func NewLoader() *Loader {
	v := viper.New()
	v.SetConfigType("yaml")
	bindEnvironment(v)
	return &Loader{v: v}
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
		if errors.Is(err, os.ErrNotExist) && (path == ".env" || path == DefaultEnvFile) {
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

func bindEnvironment(v *viper.Viper) {
	bindings := map[string][]string{
		"database.dsn":            {"MYSQL_DSN"},
		"foodsafetykorea.api_key": {"FOODSAFETYKOREA_API_KEY"},
	}
	for key, names := range bindings {
		_ = v.BindEnv(append([]string{key}, names...)...)
	}
}
