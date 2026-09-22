package icebergcat

import (
	"context"
	"fmt"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/iceberg-go/table"

	"github.com/ojiverse/datawarehouse/internal/equivalence"
)

// ScanRows reads the whole table through iceberg-go and converts it into
// comparison rows. This is the engine-independent read path: it proves the
// committed files are valid Iceberg data even when R2 SQL is unavailable.
func ScanRows(ctx context.Context, tbl *table.Table) ([]equivalence.Row, error) {
	atbl, err := tbl.Scan().ToArrowTable(ctx)
	if err != nil {
		return nil, fmt.Errorf("scan table: %w", err)
	}
	defer atbl.Release()
	cols := map[string]*arrow.Column{}
	for i, f := range atbl.Schema().Fields() {
		cols[f.Name] = atbl.Column(i)
	}
	n := int(atbl.NumRows())
	rows := make([]equivalence.Row, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, equivalence.Row{
			MessageID: str(cols["message_id"], i), ChannelID: str(cols["channel_id"], i),
			GuildID: str(cols["guild_id"], i), AuthorID: str(cols["author_id"], i),
			Content: str(cols["content"], i), CreatedAt: ts(cols["created_at"], i), EditedAt: ts(cols["edited_at"], i),
			Pinned: boolean(cols["pinned"], i), ObservationID: str(cols["observation_id"], i),
			ObservedAt: ts(cols["observed_at"], i), ProjectionVer: str(cols["projection_version"], i),
		})
	}
	return rows, nil
}

// locate maps a table-wide row index to (chunk, offset).
func locate(col *arrow.Column, i int) (arrow.Array, int) {
	for _, chunk := range col.Data().Chunks() {
		if i < chunk.Len() {
			return chunk, i
		}
		i -= chunk.Len()
	}
	return nil, -1
}

func str(col *arrow.Column, i int) string {
	chunk, j := locate(col, i)
	if chunk == nil || chunk.IsNull(j) {
		return ""
	}
	switch a := chunk.(type) {
	case *array.String:
		return a.Value(j)
	case *array.LargeString:
		return a.Value(j)
	default:
		return chunk.ValueStr(j)
	}
}

func boolean(col *arrow.Column, i int) bool {
	chunk, j := locate(col, i)
	if chunk == nil || chunk.IsNull(j) {
		return false
	}
	return chunk.(*array.Boolean).Value(j)
}

func ts(col *arrow.Column, i int) string {
	chunk, j := locate(col, i)
	if chunk == nil || chunk.IsNull(j) {
		return ""
	}
	a := chunk.(*array.Timestamp)
	unit := a.DataType().(*arrow.TimestampType).Unit
	return a.Value(j).ToTime(unit).UTC().Format(time.RFC3339Nano)
}
