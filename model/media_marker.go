package model

import "time"

// MediaMarker is a time-coded span (or point, when EndMs is nil) on a media file — e.g. a spoken
// intro, or lead/trail silence around a hidden track. Unlike Annotations, this is not a per-user
// opinion: it's the same fact for every listener, so it lives in its own global table rather than
// riding on the (user_id, item_id)-scoped annotation table.
//
// Kind is an open, namespaced string (e.g. "skip/lead_silence", "sample/amen-break") documented
// in the media-time-markers repo's KINDS.md, not a fixed enum here — adding a new kind shouldn't
// need a core code change.
type MediaMarker struct {
	ID         string    `structs:"id"         json:"id"`
	ItemID     string    `structs:"item_id"    json:"itemId"`
	ItemType   string    `structs:"item_type"  json:"itemType"`
	Kind       string    `structs:"kind"       json:"kind"`
	StartMs    int64     `structs:"start_ms"   json:"startMs"`
	EndMs      *int64    `structs:"end_ms"     json:"endMs,omitempty"`
	Source     string    `structs:"source"     json:"source"`
	Confidence *float64  `structs:"confidence" json:"confidence,omitempty"`
	CreatedAt  time.Time `structs:"created_at" json:"createdAt"`
	UpdatedAt  time.Time `structs:"updated_at" json:"updatedAt"`
}

type MediaMarkers []MediaMarker

// MediaMarkerItemType values for MediaMarker.ItemType. Only media_file exists today; the field
// leaves room to mark other item types later without a schema change.
const (
	MediaMarkerItemTypeMediaFile = "media_file"
)

// MediaMarker.Source prefixes. A manual entry uses SourceManual verbatim; plugin- and
// crowdsourced-sourced markers append their name/repo, e.g. "plugin:silence-detect".
const (
	MediaMarkerSourceManual       = "manual"
	MediaMarkerSourcePluginPrefix = "plugin:"
	MediaMarkerSourceCrowdPrefix  = "crowdsourced:"
)

type MediaMarkerRepository interface {
	ResourceRepository
	Get(id string) (*MediaMarker, error)
	GetAll(options ...QueryOptions) (MediaMarkers, error)
	GetByItemID(itemID string) (MediaMarkers, error)
	Put(m *MediaMarker) error
	Delete(id string) error
}
