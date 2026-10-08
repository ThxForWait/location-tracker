// photon/finalize.go
package photon

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf8"
)

// FinalizeEvent применяет PostProcessEvent и декодирует параметры на месте.
// После вызова ev.Parameters содержит «читаемые» значения вместо ByteArray.
func FinalizeEvent(ev *EventData) *EventData {
	if ev == nil {
		return nil
	}
	PostProcessEvent(ev)
	ev.Parameters = decodeParams(ev.Parameters)
	return ev
}

// FinalizeRequest аналогично для OperationRequest.
func FinalizeRequest(req *OperationRequest) *OperationRequest {
	if req == nil {
		return nil
	}
	PostProcessRequest(req)
	req.Parameters = decodeParams(req.Parameters)
	return req
}

// FinalizeResponse аналогично для OperationResponse.
func FinalizeResponse(resp *OperationResponse) *OperationResponse {
	if resp == nil {
		return nil
	}
	PostProcessResponse(resp)
	resp.Parameters = decodeParams(resp.Parameters)
	return resp
}

// decodeParams рекурсивно проходит по map[byte]any и превращает
// ByteArray/Hashtable/слайсы в читаемое представление.
func decodeParams(params map[byte]any) map[byte]any {
	if params == nil {
		return nil
	}
	out := make(map[byte]any, len(params))
	for k, v := range params {
		out[k] = decodeValue(v)
	}
	return out
}

// decodeValue — единая точка декодирования любого значения Photon.
func decodeValue(v any) any {
	switch x := v.(type) {

	case nil, bool, string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return x

	case ByteArray:
		return DecodeBlob([]byte(x))
	case []byte:
		return DecodeBlob(x)

	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = decodeValue(e)
		}
		return out

	case []string:
		return x

	case Hashtable:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[fmt.Sprintf("%v", k)] = decodeValue(val)
		}
		return out

	case map[any]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[fmt.Sprintf("%v", k)] = decodeValue(val)
		}
		return out

	default:
		return fmt.Sprintf("%v", v)
	}
}

// DecodeBlob пытается распознать содержимое ByteArray.
func DecodeBlob(b []byte) any {
	if len(b) == 0 {
		return ""
	}

	// 1) GUID: ровно 16 байт, .NET-формат
	if len(b) == 16 {
		return GUIDString(b)
	}

	// 2) Печатаемая строка
	if utf8.Valid(b) && isPrintableUTF8(b) {
		return string(b)
	}

	// 3) Пара float32 (типичные координаты)
	if len(b) == 8 {
		f1 := math.Float32frombits(binary.LittleEndian.Uint32(b[0:4]))
		f2 := math.Float32frombits(binary.LittleEndian.Uint32(b[4:8]))
		if isFiniteF32(f1) && isFiniteF32(f2) {
			return map[string]any{"type": "float2", "x": f1, "y": f2}
		}
	}

	// 4) Один float32
	if len(b) == 4 {
		f := math.Float32frombits(binary.LittleEndian.Uint32(b))
		if isFiniteF32(f) {
			return f
		}
	}

	// 5) Fallback: hex-строка
	if len(b) <= 32 {
		return fmt.Sprintf("hex:%x", b)
	}
	return fmt.Sprintf("blob[%d]:%x...", len(b), b[:16])
}

// GUIDString конвертирует .NET GUID (первые 3 поля little-endian).
func GUIDString(b []byte) string {
	if len(b) != 16 {
		return ""
	}
	d1 := binary.LittleEndian.Uint32(b[0:4])
	d2 := binary.LittleEndian.Uint16(b[4:6])
	d3 := binary.LittleEndian.Uint16(b[6:8])
	return fmt.Sprintf("%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X",
		d1, d2, d3,
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}

func isPrintableUTF8(b []byte) bool {
	for _, r := range string(b) {
		if r < 0x09 || (r > 0x0D && r < 0x20) {
			return false
		}
	}
	return true
}

func isFiniteF32(f float32) bool {
	f64 := float64(f)
	return !math.IsNaN(f64) && !math.IsInf(f64, 0)
}