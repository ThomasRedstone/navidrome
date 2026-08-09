package plugins

import (
	"context"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/plugins/capabilities"
)

const CapabilityMediaMarkerProvider Capability = "MediaMarkerProvider"

const (
	FuncMediaMarkerProviderGetMediaMarkers = "nd_get_media_markers"
)

func init() {
	registerCapability(
		CapabilityMediaMarkerProvider,
		FuncMediaMarkerProviderGetMediaMarkers,
	)
}

func newMediaMarkerProviderPlugin(p *plugin) *MediaMarkerProviderPlugin {
	return &MediaMarkerProviderPlugin{name: p.name, plugin: p}
}

// MediaMarkerProviderPlugin adapts a WASM plugin with the MediaMarkerProvider capability.
type MediaMarkerProviderPlugin struct {
	name   string
	plugin *plugin
}

// GetMediaMarkers calls the plugin to find markers for a track (e.g. a spoken intro, or
// lead/trail silence), converting the wire-format response into model.MediaMarker. Source is
// left blank here; the caller (core/mediamarkers) stamps it with the plugin's name.
func (m *MediaMarkerProviderPlugin) GetMediaMarkers(ctx context.Context, mf *model.MediaFile) ([]model.MediaMarker, error) {
	req := capabilities.GetMediaMarkersRequest{
		Track: mediaFileToTrackInfo(m.plugin, mf),
	}
	resp, err := callPluginFunction[capabilities.GetMediaMarkersRequest, capabilities.GetMediaMarkersResponse](
		ctx, m.plugin, FuncMediaMarkerProviderGetMediaMarkers, req,
	)
	if err != nil {
		return nil, err
	}

	markers := make([]model.MediaMarker, 0, len(resp.Markers))
	for _, mi := range resp.Markers {
		marker := model.MediaMarker{
			ItemID:   mf.ID,
			ItemType: model.MediaMarkerItemTypeMediaFile,
			Kind:     mi.Kind,
			StartMs:  mi.StartMs,
		}
		if mi.EndMs > 0 {
			end := mi.EndMs
			marker.EndMs = &end
		}
		if mi.Confidence > 0 {
			confidence := float64(mi.Confidence)
			marker.Confidence = &confidence
		}
		markers = append(markers, marker)
	}
	return markers, nil
}
