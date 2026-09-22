package canonical

import (
	"fmt"
	"io"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	"github.com/ojiverse/datawarehouse/internal/projection"
)

// BuildRecord converts projected messages into one Arrow record in schema order.
func BuildRecord(rows []projection.CanonicalMessage) (arrow.RecordBatch, error) {
	sc, err := ArrowSchema()
	if err != nil {
		return nil, err
	}
	b := array.NewRecordBuilder(memory.DefaultAllocator, sc)
	defer b.Release()
	for _, r := range rows {
		b.Field(0).(*array.StringBuilder).Append(r.MessageID)
		b.Field(1).(*array.StringBuilder).Append(r.ChannelID)
		b.Field(2).(*array.StringBuilder).Append(r.GuildID)
		b.Field(3).(*array.StringBuilder).Append(r.AuthorID)
		b.Field(4).(*array.StringBuilder).Append(r.AuthorUsername)
		b.Field(5).(*array.BooleanBuilder).Append(r.AuthorIsBot)
		b.Field(6).(*array.StringBuilder).Append(r.Content)
		b.Field(7).(*array.TimestampBuilder).Append(arrow.Timestamp(r.CreatedAt.UnixMicro()))
		if r.EditedAt == nil {
			b.Field(8).(*array.TimestampBuilder).AppendNull()
		} else {
			b.Field(8).(*array.TimestampBuilder).Append(arrow.Timestamp(r.EditedAt.UnixMicro()))
		}
		b.Field(9).(*array.BooleanBuilder).Append(r.Pinned)
		b.Field(10).(*array.Int32Builder).Append(int32(r.MessageType))
		b.Field(11).(*array.StringBuilder).Append(r.ObservationID.String())
		b.Field(12).(*array.TimestampBuilder).Append(arrow.Timestamp(r.ObservedAt.UnixMicro()))
		b.Field(13).(*array.StringBuilder).Append(string(r.SourceKind))
		b.Field(14).(*array.StringBuilder).Append(r.ProjectionVer)
		b.Field(15).(*array.TimestampBuilder).Append(arrow.Timestamp(r.SnowflakeTime.UnixMicro()))
	}
	return b.NewRecordBatch(), nil
}

// WriteParquet writes the rows as a single Zstandard-compressed Parquet file.
// Zstandard is the fixed baseline from ADR-0013; it is not configurable here
// on purpose so a mis-configured run cannot silently produce Snappy files.
func WriteParquet(w io.Writer, rows []projection.CanonicalMessage) error {
	rec, err := BuildRecord(rows)
	if err != nil {
		return err
	}
	defer rec.Release()
	props := parquet.NewWriterProperties(
		parquet.WithCompression(compress.Codecs.Zstd),
		parquet.WithDictionaryDefault(true),
	)
	fw, err := pqarrow.NewFileWriter(rec.Schema(), w, props, pqarrow.DefaultWriterProps())
	if err != nil {
		return fmt.Errorf("create parquet writer: %w", err)
	}
	if err := fw.Write(rec); err != nil {
		return fmt.Errorf("write parquet rows: %w", err)
	}
	if err := fw.Close(); err != nil {
		return fmt.Errorf("close parquet writer: %w", err)
	}
	return nil
}

// InspectParquet reports row count and the compression codec of every column
// chunk, used by tests and by the spike report to prove the Zstd invariant.
func InspectParquet(r parquet.ReaderAtSeeker) (rows int64, codecs map[string]struct{}, err error) {
	pr, err := file.NewParquetReader(r)
	if err != nil {
		return 0, nil, fmt.Errorf("open parquet: %w", err)
	}
	defer pr.Close()
	codecs = map[string]struct{}{}
	for rg := 0; rg < pr.NumRowGroups(); rg++ {
		md := pr.RowGroup(rg).MetaData()
		for c := 0; c < md.NumColumns(); c++ {
			cc, err := md.ColumnChunk(c)
			if err != nil {
				return 0, nil, err
			}
			codecs[cc.Compression().String()] = struct{}{}
		}
	}
	return pr.NumRows(), codecs, nil
}
