// Package observation implements the physical encoding of Observation Envelope v1
// as stored in the R2 Observation Archive (UTF-8 JSON + gzip).
//
// The logical contract lives in docs/domain/observations/envelope.md and
// docs/domain/observations/identity.md. The physical JSON is fixed by the
// shared cross-language fixtures under contracts/observation-envelope/v1
// (ADR-0014): the TypeScript producer and this Go reader must both accept and
// reproduce those documents, so field names here follow the fixture exactly.
package observation

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
)

// EnvelopeVersion is the contract version every v1 Observation declares.
const EnvelopeVersion = "v1"

// SourceKind identifies the acquisition path of an Observation.
type SourceKind string

// Source kinds fixed by docs/infrastructure/cloudflare/r2/README.md.
const (
	SourceHTTPBackfill       SourceKind = "http_backfill"
	SourceHTTPReconciliation SourceKind = "http_reconciliation"
	SourceGateway            SourceKind = "gateway"
)

// ObservationID is a UUIDv7 that identifies one observation act.
type ObservationID struct{ uuid.UUID }

// NewObservationID generates a UUIDv7 Observation ID.
func NewObservationID() (ObservationID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return ObservationID{}, fmt.Errorf("generate uuidv7: %w", err)
	}
	return ObservationID{id}, nil
}

// ParseObservationID parses and validates a UUIDv7 string.
func ParseObservationID(s string) (ObservationID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return ObservationID{}, fmt.Errorf("parse observation id: %w", err)
	}
	if id.Version() != 7 {
		return ObservationID{}, fmt.Errorf("observation id %q is uuid version %d, want 7", s, id.Version())
	}
	return ObservationID{id}, nil
}

// Less orders IDs by their 128-bit value; the projection tie-break rule
// (docs/domain/processing/projection.md) requires a deterministic UUID order.
func (o ObservationID) Less(other ObservationID) bool {
	return bytes.Compare(o.UUID[:], other.UUID[:]) < 0
}

// Pagination holds the Discord pagination parameters of the request. Absent
// parameters are JSON null, as the shared fixture requires, so both fields
// are pointers rather than omitted keys.
type Pagination struct {
	Before *string `json:"before"`
	After  *string `json:"after"`
}

// RateLimit carries the diagnostic Discord rate-limit headers. It is never an
// input to identity or completeness (docs/domain/observations/envelope.md).
type RateLimit struct {
	Limit             int     `json:"limit"`
	Remaining         int     `json:"remaining"`
	ResetAfterSeconds float64 `json:"reset_after_seconds"`
	Bucket            string  `json:"bucket"`
}

// HTTPProvenance is the http_backfill / http_reconciliation provenance block.
// Secrets (tokens, cookies) must never be placed here.
type HTTPProvenance struct {
	RunID               string     `json:"run_id"`
	DiscordAPIVersion   string     `json:"discord_api_version"`
	GuildID             string     `json:"guild_id"`
	ChannelID           string     `json:"channel_id"`
	Operation           string     `json:"operation"`
	Endpoint            string     `json:"endpoint"`
	Pagination          Pagination `json:"pagination"`
	Limit               int        `json:"limit"`
	RequestStartedAt    Timestamp  `json:"request_started_at"`
	ResponseCompletedAt Timestamp  `json:"response_completed_at"`
	HTTPStatus          int        `json:"http_status"`
	// Capabilities records application state that affects completeness,
	// e.g. whether the Message Content privileged intent was granted.
	Capabilities map[string]bool `json:"capabilities"`
	RateLimit    *RateLimit      `json:"rate_limit,omitempty"`
}

// Envelope is the common Observation Envelope v1 with the HTTP provenance
// block. Payload keeps the raw Discord response body verbatim so the Archive
// never loses fields the producer did not understand.
//
// Gateway provenance is a different block under the same "provenance" key; it
// is out of scope for the spike and will be modelled when Gateway ingestion
// is implemented, without changing the common part.
type Envelope struct {
	EnvelopeVersion string          `json:"envelope_version"`
	ObservationID   ObservationID   `json:"observation_id"`
	SourceKind      SourceKind      `json:"source_kind"`
	ObservedAt      Timestamp       `json:"observed_at"`
	Payload         json.RawMessage `json:"payload"`
	Provenance      *HTTPProvenance `json:"provenance,omitempty"`
}

// Validate checks the structural invariants of a v1 Envelope.
func (e Envelope) Validate() error {
	if e.EnvelopeVersion != EnvelopeVersion {
		return fmt.Errorf("envelope version %q is not %q", e.EnvelopeVersion, EnvelopeVersion)
	}
	if e.ObservationID.Version() != 7 {
		return errors.New("observation id must be uuid v7")
	}
	switch e.SourceKind {
	case SourceHTTPBackfill, SourceHTTPReconciliation:
		if e.Provenance == nil {
			return fmt.Errorf("source kind %s requires http provenance", e.SourceKind)
		}
	case SourceGateway:
	default:
		return fmt.Errorf("unknown source kind %q", e.SourceKind)
	}
	if e.ObservedAt.IsZero() {
		return errors.New("observed_at is required")
	}
	if len(e.Payload) == 0 {
		return errors.New("payload is required")
	}
	return nil
}

// PayloadSHA256 returns the hex SHA-256 of the raw payload bytes, used for the
// retry consistency check of the create-only Archive write.
func (e Envelope) PayloadSHA256() string {
	sum := sha256.Sum256(e.Payload)
	return hex.EncodeToString(sum[:])
}

// ArchiveKey returns the R2 object key for this Envelope following
// docs/infrastructure/cloudflare/r2/README.md. The key embeds the Observation
// ID only as a physical layout choice; identity is never derived from the key.
func (e Envelope) ArchiveKey() string {
	t := e.ObservedAt.UTC()
	return fmt.Sprintf("observations/v1/source=%s/year=%04d/month=%02d/day=%02d/hour=%02d/%s.json.gz",
		e.SourceKind, t.Year(), int(t.Month()), t.Day(), t.Hour(), e.ObservationID.String())
}

// DecodeJSON parses an uncompressed Envelope document (e.g. a contract fixture)
// and validates it.
func DecodeJSON(data []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(data, &e); err != nil {
		return Envelope{}, fmt.Errorf("decode envelope: %w", err)
	}
	if err := e.Validate(); err != nil {
		return Envelope{}, err
	}
	return e, nil
}

// EncodeGzipJSON serialises the Envelope as gzip-compressed UTF-8 JSON.
func EncodeGzipJSON(e Envelope) ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := json.NewEncoder(zw).Encode(e); err != nil {
		return nil, fmt.Errorf("encode envelope: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("gzip envelope: %w", err)
	}
	return buf.Bytes(), nil
}

// DecodeGzipJSON parses a gzip-compressed Envelope and validates it.
func DecodeGzipJSON(r io.Reader) (Envelope, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return Envelope{}, fmt.Errorf("gunzip envelope: %w", err)
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return Envelope{}, fmt.Errorf("read envelope: %w", err)
	}
	return DecodeJSON(raw)
}
