package store

import (
	"encoding/json"
	"fmt"
)

// marshalStringMap encodes a map[string]string to a JSON string.
// A nil map produces "{}".
func marshalStringMap(m map[string]string) (string, error) {
	if m == nil {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("marshal map: %w", err)
	}
	return string(b), nil
}

// unmarshalStringMap decodes a JSON string into a map[string]string.
// An empty or "{}" string produces an empty (non-nil) map.
func unmarshalStringMap(s string, out *map[string]string) error {
	if s == "" || s == "{}" {
		*out = make(map[string]string)
		return nil
	}
	m := make(map[string]string)
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return fmt.Errorf("unmarshal map: %w", err)
	}
	*out = m
	return nil
}
