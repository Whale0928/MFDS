package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var requiredTargets = map[string]string{
	"위스키":   "C0314210000000000000",
	"브랜디":   "C0314220000000000000",
	"일반증류주": "C0314230000000000000",
	"리큐르":   "C0314240000000000000",
}

const foodSafetyKoreaBaseURL = "https://openapi.foodsafetykorea.go.kr"

func (c Config) Validate() error {
	var problems []error
	problems = appendURLProblem(problems, "web.base_url", c.Web.BaseURL)
	if strings.TrimSpace(c.Database.DSN) == "" {
		problems = append(problems, errors.New("MYSQL_DSN이 비어 있습니다"))
	}
	if c.Web.ListPageSize <= 0 || c.Web.ListWorkers <= 0 || c.Web.QPS <= 0 {
		problems = append(problems, errors.New("웹 page size, worker, QPS는 0보다 커야 합니다"))
	}
	if c.Web.ListPageSize > 50 {
		problems = append(problems, errors.New("웹 목록 page size는 50 이하여야 합니다"))
	}
	if c.Database.MaxOpenConns <= 0 || c.Database.MaxIdleConns < 0 || c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		problems = append(problems, errors.New("DB connection pool 설정이 유효하지 않습니다"))
	}
	if c.Normalization.RunLimit <= 0 || c.Normalization.LeaseDuration <= 0 ||
		c.Normalization.MaxAttempts <= 0 || c.Normalization.RetryDelay <= 0 {
		problems = append(problems, errors.New("normalization 운영 설정은 모두 0보다 커야 합니다"))
	}
	if err := validateTargets(c.Targets); err != nil {
		problems = append(problems, err)
	}
	return errors.Join(problems...)
}

func (c FoodSafetyConfig) Validate() error {
	var problems []error
	problems = appendHTTPSURLProblem(problems, "foodsafetykorea.base_url", c.BaseURL)
	if strings.TrimRight(c.BaseURL, "/") != foodSafetyKoreaBaseURL {
		problems = append(problems, fmt.Errorf("foodsafetykorea.base_url은 공식 HTTPS endpoint %s이어야 합니다", foodSafetyKoreaBaseURL))
	}
	if strings.TrimSpace(c.APIKey) == "" {
		problems = append(problems, errors.New("FOODSAFETYKOREA_API_KEY가 비어 있습니다"))
	}
	if c.PageSize <= 0 || c.PageSize > 1000 {
		problems = append(problems, errors.New("식품안전나라 page size는 1 이상 1000 이하여야 합니다"))
	}
	if c.MaxPages <= 0 || c.MaxRequests <= 0 || c.MaxRequests > 500 || c.QPS <= 0 || c.RequestTimeout <= 0 {
		problems = append(problems, errors.New("식품안전나라 max pages, QPS, request timeout은 0보다 크고 run당 요청 수는 500 이하여야 합니다"))
	}
	return errors.Join(problems...)
}

func appendHTTPSURLProblem(problems []error, name, value string) []error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return append(problems, fmt.Errorf("%s이 유효한 HTTPS URL이 아닙니다", name))
	}
	return problems
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
