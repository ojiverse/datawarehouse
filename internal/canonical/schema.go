// Package canonical defines the Canonical Message Iceberg schema and writes
// Parquet (Zstandard) staging files for it.
package canonical

import (
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"
)

// TableName is the first-MVP mandatory Canonical dataset.
const TableName = "message"

// Column field IDs are part of the persisted contract: Iceberg resolves
// columns by ID, so they must never be renumbered once a table exists.
const (
	fieldMessageID      = 1
	fieldChannelID      = 2
	fieldGuildID        = 3
	fieldAuthorID       = 4
	fieldAuthorUsername = 5
	fieldAuthorIsBot    = 6
	fieldContent        = 7
	fieldCreatedAt      = 8
	fieldEditedAt       = 9
	fieldPinned         = 10
	fieldMessageType    = 11
	fieldObservationID  = 12
	fieldObservedAt     = 13
	fieldSourceKind     = 14
	fieldProjectionVer  = 15
	fieldSnowflakeTime  = 16
)

// MessageSchema returns the Iceberg schema of the Canonical Message table.
//
// Snowflakes are strings (docs/domain/observations/identity.md); the derived
// creation time is a separate timestamp column and never replaces the ID.
func MessageSchema() *iceberg.Schema {
	return iceberg.NewSchema(0,
		iceberg.NestedField{ID: fieldMessageID, Name: "message_id", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: fieldChannelID, Name: "channel_id", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: fieldGuildID, Name: "guild_id", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: fieldAuthorID, Name: "author_id", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: fieldAuthorUsername, Name: "author_username", Type: iceberg.PrimitiveTypes.String, Required: false},
		iceberg.NestedField{ID: fieldAuthorIsBot, Name: "author_is_bot", Type: iceberg.PrimitiveTypes.Bool, Required: true},
		iceberg.NestedField{ID: fieldContent, Name: "content", Type: iceberg.PrimitiveTypes.String, Required: false},
		iceberg.NestedField{ID: fieldCreatedAt, Name: "created_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: fieldEditedAt, Name: "edited_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: false},
		iceberg.NestedField{ID: fieldPinned, Name: "pinned", Type: iceberg.PrimitiveTypes.Bool, Required: true},
		iceberg.NestedField{ID: fieldMessageType, Name: "message_type", Type: iceberg.PrimitiveTypes.Int32, Required: true},
		iceberg.NestedField{ID: fieldObservationID, Name: "observation_id", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: fieldObservedAt, Name: "observed_at", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
		iceberg.NestedField{ID: fieldSourceKind, Name: "source_kind", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: fieldProjectionVer, Name: "projection_version", Type: iceberg.PrimitiveTypes.String, Required: true},
		iceberg.NestedField{ID: fieldSnowflakeTime, Name: "snowflake_time", Type: iceberg.PrimitiveTypes.TimestampTz, Required: true},
	)
}

// ArrowSchema derives the Arrow schema (with Iceberg field IDs embedded as
// Parquet field_id metadata) so data files self-describe their column mapping.
func ArrowSchema() (*arrow.Schema, error) {
	return table.SchemaToArrowSchema(MessageSchema(), nil, true, false)
}
