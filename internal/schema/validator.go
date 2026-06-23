package schema

import (
	"fmt"
	"strings"
)

// validateProps 校验 props 是否符合 EntityType 定义。
// 四阶段校验：必填 → 类型 → 枚举 → stableKeys 非空。
// 所有失败项聚合后以 "; " 分隔返回。
// 不修改输入 map。
func validateProps(et *EntityType, props map[string]any) error {
	var errs []string

	// 阶段 1: 必填校验
	for name, spec := range et.Spec.Properties {
		if !spec.Required {
			continue
		}
		val, exists := props[name]
		if !exists || isEmpty(val) {
			errs = append(errs, fmt.Sprintf("required field %q is missing or empty", name))
		}
	}

	// 阶段 2: 类型校验（仅对 props 中存在的值）
	for name, spec := range et.Spec.Properties {
		val, exists := props[name]
		if !exists {
			continue
		}
		if err := checkType(name, val, spec.Type); err != nil {
			errs = append(errs, err.Error())
		}
	}

	// 阶段 3: 枚举校验（仅对存在的值 + 有枚举约束）
	for name, spec := range et.Spec.Properties {
		if len(spec.Enum) == 0 {
			continue
		}
		val, exists := props[name]
		if !exists {
			continue
		}
		strVal, ok := val.(string)
		if !ok || !containsString(spec.Enum, strVal) {
			errs = append(errs, fmt.Sprintf("field %q value %v not in enum %v", name, val, spec.Enum))
		}
	}

	// 阶段 4: stableKeys 非空校验
	for _, key := range et.Spec.Identity.StableKeys {
		val, exists := props[key]
		if !exists || isEmpty(val) {
			errs = append(errs, fmt.Sprintf("stableKey %q must not be empty", key))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation failed for %s: %s", et.Metadata.Name, strings.Join(errs, "; "))
	}
	return nil
}

// applyDefaults 返回一个新 map，将 schema 中定义的默认值填充到缺失的可选字段。
// 原始 props map 不被修改。
func applyDefaults(et *EntityType, props map[string]any) map[string]any {
	result := make(map[string]any, len(props)+len(et.Spec.Properties))
	for k, v := range props {
		result[k] = v
	}
	for name, spec := range et.Spec.Properties {
		if _, exists := result[name]; !exists && spec.Default != nil {
			result[name] = spec.Default
		}
	}
	return result
}

// checkType 校验 val 的 Go 类型是否与 PropertySpec.Type 匹配。
// "int" 类型额外接受 float64（JSON 反序列化兼容）。
// 未知类型（未定义的 PropertySpec.Type）跳过校验，返回 nil。
func checkType(name string, val any, expectedType string) error {
	switch expectedType {
	case "string":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("field %q expected string, got %T", name, val)
		}
	case "int":
		switch val.(type) {
		case int, int8, int16, int32, int64, float64:
			// acceptable
		default:
			return fmt.Errorf("field %q expected int, got %T", name, val)
		}
	case "float":
		switch val.(type) {
		case float32, float64, int, int8, int16, int32, int64:
			// acceptable
		default:
			return fmt.Errorf("field %q expected float, got %T", name, val)
		}
	case "bool":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("field %q expected bool, got %T", name, val)
		}
	default:
		// 未知类型，跳过校验（向前兼容）
	}
	return nil
}

// isEmpty 判断值是否为空。仅 nil 和空字符串视为空。
// 数字 0、布尔 false 均为合法值，不视为空。
func isEmpty(val any) bool {
	if val == nil {
		return true
	}
	if s, ok := val.(string); ok && s == "" {
		return true
	}
	return false
}

// containsString 判断切片中是否包含指定字符串。
func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}