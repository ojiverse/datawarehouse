package observation

import (
	"fmt"
	"strings"
	"time"
)

// Timestamp is a UTC instant serialised with fixed millisecond precision
// ("2026-09-21T10:00:00.250Z"). The shared contract fixtures must survive a
// byte-for-byte canonical round-trip between the TypeScript producer and this
// reader, and time.Time's RFC3339Nano trims trailing zeros, which would turn
// ".250Z" into ".25Z" and ".000Z" into "Z".
type Timestamp struct{ time.Time }

const timestampLayout = "2006-01-02T15:04:05.000Z07:00"

// TS wraps a time.Time as a Timestamp.
func TS(t time.Time) Timestamp { return Timestamp{t} }

// MarshalJSON renders the instant in UTC with exactly three fractional digits.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.UTC().Format(timestampLayout) + `"`), nil
}

// UnmarshalJSON accepts any RFC 3339 representation (the producer may emit
// nanoseconds or no fraction); precision beyond milliseconds is preserved in
// memory and only truncated on output, which the contract fixture requires.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return fmt.Errorf("parse timestamp %q: %w", s, err)
	}
	t.Time = parsed.UTC()
	return nil
}
