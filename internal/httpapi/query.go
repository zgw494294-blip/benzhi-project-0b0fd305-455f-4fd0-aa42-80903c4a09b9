package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"geopack/internal/application"
)

func strictQuery(r *http.Request, allowed ...string) (map[string]string, error) {
	set := map[string]bool{}
	for _, key := range allowed {
		set[key] = true
	}
	result := map[string]string{}
	for key, values := range r.URL.Query() {
		if !set[key] {
			return nil, application.NewError("unknown_query_parameter", "未知查询参数 %s", key)
		}
		if len(values) != 1 {
			return nil, application.NewError("duplicate_query_parameter", "查询参数 %s 不能重复", key)
		}
		result[key] = values[0]
	}
	return result, nil
}

func requiredQueryValue(values map[string]string, key string) (string, bool, error) {
	value, present := values[key]
	if !present {
		return "", false, nil
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", true, application.NewError("blank_query_parameter", "查询参数 %s 不能为空", key)
	}
	return value, true, nil
}

func parseRFC3339Query(values map[string]string, key string) (*time.Time, error) {
	value, present, err := requiredQueryValue(values, key)
	if err != nil || !present {
		return nil, err
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, application.NewError("invalid_time", "%s 必须是 RFC3339 时间", key)
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func parseLimit(values map[string]string, defaultValue int) (int, error) {
	value, present, err := requiredQueryValue(values, "limit")
	if err != nil {
		return 0, err
	}
	if !present {
		return defaultValue, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > 100 {
		return 0, application.NewError("invalid_limit", "limit 必须在 1 至 100 之间")
	}
	return limit, nil
}

func parseRevision(values map[string]string, key string) (*uint32, error) {
	value, present, err := requiredQueryValue(values, key)
	if err != nil || !present {
		return nil, err
	}
	n, err := strconv.ParseUint(value, 10, 32)
	if err != nil || n == 0 {
		return nil, application.NewError("invalid_revision", "%s 必须是大于零的修订号", key)
	}
	result := uint32(n)
	return &result, nil
}
