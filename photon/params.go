// photon/params.go
package photon

import "fmt"

// GetStr — достаёт параметр по int-ключу как строку.
func GetStr(params map[byte]any, key int) string {
	v, ok := params[byte(key)]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// GetAny — достаёт параметр по int-ключу без преобразования.
func GetAny(params map[byte]any, key int) any {
	return params[byte(key)]
}

// GetInt — пытается достать целое.
func GetInt(params map[byte]any, key int) int64 {
	switch t := params[byte(key)].(type) {
	case byte:
		return int64(t)
	case int16:
		return int64(t)
	case int32:
		return int64(t)
	case int64:
		return t
	case uint16:
		return int64(t)
	case uint32:
		return int64(t)
	case uint64:
		return int64(t)
	case int:
		return int64(t)
	}
	return 0
}