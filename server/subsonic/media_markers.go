package subsonic

import (
	"context"
	"net/http"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server/subsonic/responses"
	"github.com/navidrome/navidrome/utils/req"
)

// GetMediaMarkers returns every marker (e.g. a spoken intro, or lead/trail silence) for a track.
// Available to any authenticated user — read access needs no special permission.
func (api *Router) GetMediaMarkers(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	id, err := p.String("id")
	if err != nil {
		return nil, err
	}

	markers, err := api.ds.MediaMarker(r.Context()).GetByItemID(id)
	if err != nil {
		return nil, err
	}

	response := newResponse()
	response.MediaMarkers = &responses.MediaMarkers{Marker: toResponseMediaMarkers(markers)}
	return response, nil
}

// CreateMediaMarker manually adds a marker to a track. Admin only, since the table is global
// (not per-user) — every listener sees the same markers.
func (api *Router) CreateMediaMarker(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	id, err := p.String("id")
	if err != nil {
		return nil, err
	}
	kind, err := p.String("kind")
	if err != nil {
		return nil, err
	}
	startMs, err := p.Int64("startMs")
	if err != nil {
		return nil, err
	}
	endMs := p.Int64Or("endMs", 0)
	confidence := p.Float64Or("confidence", -1)

	marker := model.MediaMarker{
		ItemID:  id,
		Kind:    kind,
		StartMs: startMs,
		Source:  model.MediaMarkerSourceManual,
	}
	if endMs > 0 {
		marker.EndMs = &endMs
	}
	if confidence >= 0 {
		marker.Confidence = &confidence
	}

	log.Debug(r, "Creating media marker", "trackId", id, "kind", kind, "startMs", startMs)
	if err := api.ds.MediaMarker(r.Context()).Put(&marker); err != nil {
		return nil, err
	}

	response := newResponse()
	response.MediaMarkers = &responses.MediaMarkers{Marker: toResponseMediaMarkers(model.MediaMarkers{marker})}
	return response, nil
}

// UpdateMediaMarker corrects an existing marker (manual or plugin-sourced) — the "correct" half
// of "manual create/correct/delete" from the design doc. The marker's source is set to "manual"
// on any edit: once a human has adjusted it, it's no longer purely plugin-provided. Admin only.
func (api *Router) UpdateMediaMarker(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	markerID, err := p.String("markerId")
	if err != nil {
		return nil, err
	}
	kind, err := p.String("kind")
	if err != nil {
		return nil, err
	}
	startMs, err := p.Int64("startMs")
	if err != nil {
		return nil, err
	}
	endMs := p.Int64Or("endMs", 0)
	confidence := p.Float64Or("confidence", -1)

	repo := api.ds.MediaMarker(r.Context())
	marker, err := repo.Get(markerID)
	if err != nil {
		return nil, err
	}
	marker.Kind = kind
	marker.StartMs = startMs
	marker.EndMs = nil
	if endMs > 0 {
		marker.EndMs = &endMs
	}
	marker.Confidence = nil
	if confidence >= 0 {
		marker.Confidence = &confidence
	}
	marker.Source = model.MediaMarkerSourceManual

	log.Debug(r, "Updating media marker", "markerId", markerID, "kind", kind, "startMs", startMs)
	if err := repo.Put(marker); err != nil {
		return nil, err
	}

	response := newResponse()
	response.MediaMarkers = &responses.MediaMarkers{Marker: toResponseMediaMarkers(model.MediaMarkers{*marker})}
	return response, nil
}

// DeleteMediaMarker removes a marker. Admin only.
func (api *Router) DeleteMediaMarker(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	markerID, err := p.String("markerId")
	if err != nil {
		return nil, err
	}

	log.Debug(r, "Deleting media marker", "markerId", markerID)
	if err := api.ds.MediaMarker(r.Context()).Delete(markerID); err != nil {
		return nil, err
	}

	return newResponse(), nil
}

// mediaMarkersByItemID batch-loads markers for a set of tracks in a single query, grouped by
// track ID, for attaching to a getSong/getAlbum/etc. response. A single query keeps this
// additive to those endpoints rather than an N+1 per song in a list.
func mediaMarkersByItemID(ctx context.Context, ds model.DataStore, itemIDs []string) map[string][]responses.MediaMarker {
	if len(itemIDs) == 0 {
		return nil
	}
	markers, err := ds.MediaMarker(ctx).GetAll(model.QueryOptions{Filters: Eq{"item_id": itemIDs}})
	if err != nil {
		log.Error(ctx, "Error loading media markers", "itemIds", itemIDs, err)
		return nil
	}
	byItem := make(map[string]model.MediaMarkers)
	for _, m := range markers {
		byItem[m.ItemID] = append(byItem[m.ItemID], m)
	}
	result := make(map[string][]responses.MediaMarker, len(byItem))
	for itemID, ms := range byItem {
		result[itemID] = toResponseMediaMarkers(ms)
	}
	return result
}

// attachMediaMarkers sets MediaMarkers on each child from a batch-loaded map (see
// mediaMarkersByItemID), skipping children with no markers so the field stays omitted.
func attachMediaMarkers(children []responses.Child, byItemID map[string][]responses.MediaMarker) {
	if len(byItemID) == 0 {
		return
	}
	for i := range children {
		if ms, ok := byItemID[children[i].Id]; ok {
			children[i].MediaMarkers = ms
		}
	}
}

func toResponseMediaMarkers(markers model.MediaMarkers) []responses.MediaMarker {
	res := make([]responses.MediaMarker, len(markers))
	for i, m := range markers {
		res[i] = responses.MediaMarker{
			ID:      m.ID,
			Kind:    m.Kind,
			StartMs: m.StartMs,
			Source:  m.Source,
		}
		if m.EndMs != nil {
			res[i].EndMs = *m.EndMs
		}
		if m.Confidence != nil {
			res[i].Confidence = *m.Confidence
		}
	}
	return res
}
