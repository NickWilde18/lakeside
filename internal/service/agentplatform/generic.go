package agentplatform

import (
	"encoding/json"
	"strings"
)

// decodeJSON 把 JSON 字符串解码为指定类型 T。
// 当输入为空字符串时，返回 T 的零值且不报错。
func decodeJSON[T any](raw string) (T, error) {
	var out T
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	return out, nil
}

// decodeJSONOrZero 解码失败时返回 T 的零值，适合日志/展示等容错场景。
func decodeJSONOrZero[T any](raw string) T {
	out, err := decodeJSON[T](raw)
	if err != nil {
		var zero T
		return zero
	}
	return out
}
