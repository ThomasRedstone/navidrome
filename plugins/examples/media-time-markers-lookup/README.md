# Media Time Markers Lookup Plugin

A `MediaMarkerProvider` plugin that looks up skip/sample markers for a track by AcoustID
fingerprint, against a crowdsourced GitHub-hosted marker repository — default
[media-time-markers](https://github.com/ThomasRedstone/media-time-markers), configurable to
point at a fork.

## How it works

1. Fingerprint the track via the host [Fingerprint](../../README.md#fingerprint) service (runs
   `fpcalc` server-side — WASM plugins can't shell out to it themselves).
2. Resolve that fingerprint to an AcoustID UUID via
   [AcoustID's public lookup API](https://acoustid.org/webservice), picking the
   highest-scoring result.
3. Fetch that UUID's marker file (`data/<uuid[0:2]>/<uuid[2:4]>/<uuid>.json`) from the
   configured repo via [jsDelivr's GitHub CDN mirror](https://www.jsdelivr.com/), a plain
   `GET` — no auth needed since jsDelivr only serves public repos.
4. Map its `markers` array to `MediaMarkerInfo`.

A 404 from either AcoustID (no fingerprint match yet) or the marker repo (no markers filed for
this recording yet) is not an error — it just means nothing to report this run.

## Known limitation: no bulk index sync (yet)

The marker repo's design (see its `docs/architecture.md`) intends a bulk-sync read path: fetch
a lightweight index of covered AcoustID UUIDs once per scan cycle, then only fetch per-UUID
files for local matches — avoiding a 404 round-trip for every track that isn't covered yet. This
first version does the simpler thing (fingerprint + lookup + fetch, per track, every time) to
prove the shape end-to-end first. Worth revisiting once real usage shows how much this matters at
library scale — the host [Cache](../../README.md#cache) service is the natural place to hold the
fetched index between calls.

## Configuration

Configure in the Navidrome UI (Settings → Plugins → media-time-markers-lookup):

| Key              | Description                                                    | Default                    |
|-------------------|------------------------------------------------------------------|-----------------------------|
| `acoustidApiKey` | **Required.** AcoustID *application* key (not personal/account). | —                           |
| `repoOwner`      | GitHub owner of the marker repository.                           | `ThomasRedstone`            |
| `repoName`       | GitHub repository name.                                          | `media-time-markers`        |
| `repoRef`        | Branch or tag to read from.                                      | `main`                      |

## Permissions

- `library` with `filesystem: true` — to know the track's path.
- `fingerprint` — to compute the track's Chromaprint fingerprint (requires `library.filesystem`).
- `http` with `requiredHosts: ["api.acoustid.org", "cdn.jsdelivr.net"]`.

## Testing

`main_test.go` runs as plain `go test` (not a WASM build), using the generated PDK's
testify-based mocks (`host.HTTPMock`, `host.FingerprintMock`, `pdk.PDKMock`) to exercise the
AcoustID-response parsing, marker-file parsing, and 404/no-match handling without needing
network access, an API key, or the marker repo to be public. Real fingerprinting is already
verified for real elsewhere (`core/chromaprint`, `plugins/host_fingerprint_test.go`) — this
plugin just calls straight through to the host function, so there's no need to re-prove `fpcalc`
itself here.

```sh
cd plugins/examples/media-time-markers-lookup
go test ./...
```

## Building

```sh
cd plugins/examples
make media-time-markers-lookup
```
