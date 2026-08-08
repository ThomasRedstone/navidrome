// Package mediamarkers invokes MediaMarkerProvider plugins for a track and aggregates whatever
// markers they find (e.g. a spoken intro, or lead/trail silence around a hidden track — see
// model.MediaMarker). Unlike core/agents' metadata-agent lookup, this is not a
// first-match-wins priority list: every installed MediaMarkerProvider plugin runs and
// contributes, since a local silence-detector and a crowdsourced-lookup plugin can find
// genuinely complementary markers on the same track. There is no enable/priority config (yet) —
// installing a plugin with this capability is what opts a server in, matching how
// WebSocket/Scheduler callback plugins run automatically for every installed plugin with that
// capability.
package mediamarkers

import (
	"context"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

// Provider fetches markers for a single media file. It is the contract implemented by
// individual marker sources — currently only plugins.
type Provider interface {
	GetMediaMarkers(ctx context.Context, mf *model.MediaFile) ([]model.MediaMarker, error)
}

// PluginLoader discovers and loads MediaMarkerProvider plugins.
type PluginLoader interface {
	PluginNames(capability string) []string
	LoadMediaMarkerProvider(name string) (Provider, bool)
}

// MediaMarkers resolves markers for media files by calling every installed
// MediaMarkerProvider plugin.
type MediaMarkers interface {
	// GetMarkers returns the union of markers found by every installed MediaMarkerProvider
	// plugin for mf. A plugin error is logged and skipped rather than failing the whole call,
	// so one broken/unreachable plugin doesn't block scanning.
	GetMarkers(ctx context.Context, mf *model.MediaFile) ([]model.MediaMarker, error)
}

type mediaMarkers struct {
	pluginLoader PluginLoader
}

// New creates a MediaMarkers service. pluginLoader may be nil if no plugin system is available,
// in which case GetMarkers always returns no markers.
func New(pluginLoader PluginLoader) MediaMarkers {
	return &mediaMarkers{pluginLoader: pluginLoader}
}

const mediaMarkerProviderCapability = "MediaMarkerProvider"

func (s *mediaMarkers) GetMarkers(ctx context.Context, mf *model.MediaFile) ([]model.MediaMarker, error) {
	if s.pluginLoader == nil {
		return nil, nil
	}
	names := s.pluginLoader.PluginNames(mediaMarkerProviderCapability)
	if len(names) == 0 {
		return nil, nil
	}

	var markers []model.MediaMarker
	for _, name := range names {
		provider, ok := s.pluginLoader.LoadMediaMarkerProvider(name)
		if !ok {
			continue
		}
		found, err := provider.GetMediaMarkers(ctx, mf)
		if err != nil {
			log.Warn(ctx, "mediamarkers: plugin failed to provide markers", "plugin", name, "track", mf.Path, err)
			continue
		}
		for i := range found {
			found[i].Source = model.MediaMarkerSourcePluginPrefix + name
		}
		markers = append(markers, found...)
	}
	return markers, nil
}
