package timex

import (
	"encoding/json"
	"strings"
	"time"
)

// JSONTime is a time.Time serialized canonically as "YYYY-MM-DD HH:mm".
type JSONTime struct {
	time.Time
}

// MarshalJSON renders the time in the canonical format.
func (t JSONTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(Format(t.Time))
}

// UnmarshalJSON accepts the canonical format (and RFC3339 for robustness).
func (t *JSONTime) UnmarshalJSON(raw []byte) error {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	if s == "" {
		t.Time = time.Time{}
		return nil
	}
	if v, err := Parse(s); err == nil {
		t.Time = v
		return nil
	}
	if v, err := time.Parse(time.RFC3339, strings.TrimSpace(s)); err == nil {
		t.Time = v.In(Beijing)
		return nil
	}
	return &time.ParseError{Value: s, Layout: Layout}
}
