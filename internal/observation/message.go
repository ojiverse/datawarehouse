package observation

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// DiscordEpochMillis is the Discord Snowflake epoch (2015-01-01T00:00:00Z).
const DiscordEpochMillis int64 = 1420070400000

// MessageSnapshot is the typed view of one Discord Message object inside an
// HTTP page payload. Only the fields the Canonical projection needs are typed;
// the full object stays in Raw so nothing is lost.
type MessageSnapshot struct {
	ID              string     `json:"id"`
	ChannelID       string     `json:"channel_id"`
	Author          Author     `json:"author"`
	Content         string     `json:"content"`
	Timestamp       time.Time  `json:"timestamp"`
	EditedTimestamp *time.Time `json:"edited_timestamp"`
	Pinned          bool       `json:"pinned"`
	Type            int        `json:"type"`
	Raw             json.RawMessage
}

// Author is the subset of the Discord User object used by the projection.
type Author struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Bot      bool   `json:"bot"`
}

// DecodeHTTPPage splits an HTTP "Get Channel Messages" response body into
// typed message snapshots while retaining each raw object.
func DecodeHTTPPage(payload json.RawMessage) ([]MessageSnapshot, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal(payload, &raws); err != nil {
		return nil, fmt.Errorf("decode http page: %w", err)
	}
	out := make([]MessageSnapshot, 0, len(raws))
	for i, raw := range raws {
		var m MessageSnapshot
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("decode message %d: %w", i, err)
		}
		if m.ID == "" {
			return nil, fmt.Errorf("message %d has no id", i)
		}
		m.Raw = raw
		out = append(out, m)
	}
	return out, nil
}

// SnowflakeTime derives the creation time embedded in a decimal Snowflake.
// The Snowflake string itself remains the canonical identity representation.
func SnowflakeTime(snowflake string) (time.Time, error) {
	v, err := strconv.ParseUint(snowflake, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse snowflake %q: %w", snowflake, err)
	}
	ms := int64(v>>22) + DiscordEpochMillis
	return time.UnixMilli(ms).UTC(), nil
}
