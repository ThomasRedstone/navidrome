package capabilities

// MediaMarkerProvider supplies time-coded markers for a track — e.g. a spoken intro, or
// lead/trail silence around a hidden track (see media_marker DB table). This is a pull model:
// Navidrome calls GetMediaMarkers for a track at scan time, the same way MetadataAgent methods
// are invoked, rather than the plugin pushing data on its own schedule.
//
// A single method, so this capability is required=true: a plugin implementing it must return
// GetMediaMarkersResponse (which may be empty when the plugin has nothing for this track).
//
//nd:capability name=mediaMarkerProvider required=true
type MediaMarkerProvider interface {
	// GetMediaMarkers returns any markers this plugin can find for the given track.
	//nd:export name=nd_get_media_markers
	GetMediaMarkers(GetMediaMarkersRequest) (GetMediaMarkersResponse, error)
}

// GetMediaMarkersRequest contains the track information markers are requested for.
type GetMediaMarkersRequest struct {
	Track TrackInfo `json:"track"`
}

// GetMediaMarkersResponse contains the markers found by the plugin, if any.
type GetMediaMarkersResponse struct {
	Markers []MediaMarkerInfo `json:"markers"`
}

// MediaMarkerInfo is a single time-coded marker returned by a plugin. Kind is an open,
// namespaced string (e.g. "skip/lead_silence", "sample/amen-break"), not a fixed enum — see
// the media-time-markers repo's KINDS.md for the registry. EndMs is omitted for a point-in-time
// marker (e.g. "sample/wilhelm-scream") rather than a span.
type MediaMarkerInfo struct {
	// Kind is the marker's namespaced type, e.g. "skip/lead_silence".
	Kind string `json:"kind"`
	// StartMs is the marker's start offset in milliseconds.
	StartMs int64 `json:"startMs"`
	// EndMs is the marker's end offset in milliseconds. Omitted for a point-in-time marker.
	EndMs int64 `json:"endMs,omitempty"`
	// Confidence is an optional 0-1 score for anything auto-detected or community-sourced.
	Confidence float32 `json:"confidence,omitempty"`
}
