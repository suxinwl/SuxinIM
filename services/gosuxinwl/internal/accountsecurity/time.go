package accountsecurity

import (
	"errors"
	"time"
)

// Some deployed MySQL drivers expose DATETIME as bytes, others as time.Time.
// Newly introduced locks use UTC Unix seconds; this scanner is legacy-only.
type legacyTime struct {
	Time  time.Time
	Valid bool
}

func (t *legacyTime) Scan(value any) error {
	t.Valid = false
	if value == nil {
		return nil
	}
	if v, ok := value.(time.Time); ok {
		t.Time = v
		t.Valid = true
		return nil
	}
	var text string
	switch v := value.(type) {
	case string:
		text = v
	case []byte:
		text = string(v)
	default:
		return errors.New("invalid legacy lock time")
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999"} {
		if v, e := time.ParseInLocation(layout, text, time.Local); e == nil {
			t.Time = v
			t.Valid = true
			return nil
		}
	}
	return errors.New("invalid legacy lock time")
}
