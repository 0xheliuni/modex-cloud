package common

import "encoding/json"

// JSON wrappers — centralized so the encoder can be swapped later (mirrors the
// new-api common/json.go convention). Business code should call these rather
// than importing encoding/json directly.

func Marshal(v any) ([]byte, error)      { return json.Marshal(v) }
func Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

// EncodeJSON marshals v to a string, returning "" on error or for nil/empty
// slices. Used to persist whitelist arrays as TEXT columns (cross-DB safe).
func EncodeJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	s := string(b)
	if s == "null" {
		return ""
	}
	return s
}

// DecodeIntList parses a JSON int array stored as TEXT. Empty/invalid -> nil.
func DecodeIntList(s string) []int {
	if s == "" {
		return nil
	}
	var out []int
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// DecodeStringList parses a JSON string array stored as TEXT. Empty/invalid -> nil.
func DecodeStringList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}
