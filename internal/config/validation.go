package config

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

var requiredTargets = map[string]string{
	"위스키":   "C0314210000000000000",
	"브랜디":   "C0314220000000000000",
	"일반증류주": "C0314230000000000000",
	"리큐르":   "C0314240000000000000",
}

func (c Config) Validate() error {
	var problems []error
	if strings.TrimSpace(c.API.Key) == "" {
		problems = append(problems, errors.New("MFDS_API_KEY가 비어 있습니다"))
	}
	problems = appendURLProblem(problems, "api.base_url", c.API.BaseURL)
	problems = appendURLProblem(problems, "web.base_url", c.Web.BaseURL)
	if strings.TrimSpace(c.Database.DSN) == "" {
		problems = append(problems, errors.New("MYSQL_DSN이 비어 있습니다"))
	}
	if c.API.PageSize <= 0 || c.API.PartitionWorkers <= 0 || c.API.PageWorkers <= 0 || c.API.QPS <= 0 || c.API.Burst <= 0 {
		problems = append(problems, errors.New("API page size, worker, QPS, burst는 0보다 커야 합니다"))
	}
	if c.Web.ListPageSize <= 0 || c.Web.ListWorkers <= 0 || c.Web.DetailWorkers <= 0 || c.Web.QPS <= 0 {
		problems = append(problems, errors.New("웹 page size, worker, QPS는 0보다 커야 합니다"))
	}
	if c.Web.ListPageSize > 50 {
		problems = append(problems, errors.New("웹 목록 page size는 50 이하여야 합니다"))
	}
	if c.Database.MaxOpenConns <= 0 || c.Database.MaxIdleConns < 0 || c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		problems = append(problems, errors.New("DB connection pool 설정이 유효하지 않습니다"))
	}
	if !slices.Contains([]string{"off", "fallback", "always"}, c.Proxy.Mode) {
		problems = append(problems, fmt.Errorf("proxy.mode %q은 지원하지 않습니다", c.Proxy.Mode))
	}
	if (c.Proxy.Mode == "fallback" || c.Proxy.Mode == "always") && strings.TrimSpace(c.Proxy.URL) == "" {
		problems = append(problems, errors.New("proxy 모드가 활성화되면 MFDS_WEB_PROXY_URL이 필요합니다"))
	}
	if err := validateTargets(c.Targets); err != nil {
		problems = append(problems, err)
	}
	return errors.Join(problems...)
}

func appendURLProblem(problems []error, name, value string) []error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return append(problems, fmt.Errorf("%s이 유효한 HTTP URL이 아닙니다", name))
	}
	return problems
}

func validateTargets(targets []TargetItem) error {
	if len(targets) != len(requiredTargets) {
		return fmt.Errorf("targets는 정확히 %d개여야 합니다", len(requiredTargets))
	}
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		expected, ok := requiredTargets[target.Name]
		if !ok || expected != target.Code {
			return fmt.Errorf("대상 품목 %q의 이름 또는 코드가 유효하지 않습니다", target.Name)
		}
		if _, duplicated := seen[target.Name]; duplicated {
			return fmt.Errorf("대상 품목 %q이 중복되었습니다", target.Name)
		}
		seen[target.Name] = struct{}{}
	}
	return nil
}
