// Package utils 提供通用工具函数
package utils

import (
	"fmt"
	"strings"
)

// GenerateURI 根据 uriTemplate 和 stableKeys 生成 URI。
// template 格式如 "device:{serial_number}"，stableKeys 中的字段名
// 必须在 props 中存在且非空，否则返回 error。
func GenerateURI(template string, stableKeys []string, props map[string]any) (string, error) {
	result := template
	for _, key := range stableKeys {
		val, ok := props[key]
		if !ok || val == nil {
			return "", fmt.Errorf("stableKey %q not found in properties", key)
		}
		strVal, ok := val.(string)
		if !ok {
			strVal = fmt.Sprintf("%v", val)
		}
		if strVal == "" {
			return "", fmt.Errorf("stableKey %q is empty", key)
		}
		result = strings.ReplaceAll(result, "{"+key+"}", strVal)
	}
	return result, nil
}
