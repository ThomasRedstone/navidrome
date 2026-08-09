# Silence Marker Plugin

A `MediaMarkerProvider` plugin that detects lead/trail silence around a track's audio and
reports it as `skip/lead_silence` / `skip/trail_silence` media markers — the first concrete
answer to [navidrome/navidrome#2082](https://github.com/navidrome/navidrome/issues/2082) (skip
silence around hidden tracks).

No network access, no external service dependency: it works entirely against the local file via
the host-side `SilenceDetect` service, which runs ffmpeg's `silencedetect` filter on Navidrome's
server. WASM plugins can't spawn subprocesses themselves, so this host function exists
specifically to give a plugin ffmpeg-backed audio analysis without ffmpeg (or any decoder)
needing to run inside the WASM sandbox.

## How it decides "lead" vs "trail" vs "ignore"

`silencedetect` reports every silent span in the file, not just the ones at the edges — a long
ambient intro with a quiet passage in the middle would also show up. This plugin only turns a
span into a marker when it's within `leadTrailToleranceMs` (250ms) of the track's start or end:

- First span starts at (or very near) 0 → `skip/lead_silence`, from `0` to the span's end.
- Last span ends at (or very near) the track's duration → `skip/trail_silence`, from the span's
  start to the track's duration.
- Everything else is ignored — this plugin answers "silence around a hidden track," not "flag
  every quiet passage."

## Configuration

Configure in the Navidrome UI (Settings → Plugins → silence-marker):

| Key            | Description                                     | Default |
|----------------|--------------------------------------------------|---------|
| `noiseDb`      | ffmpeg `silencedetect` noise threshold, in dB     | `-30`   |
| `minSilenceMs` | Minimum silence duration to report, in ms         | `500`   |

## Permissions

- `library` with `filesystem: true` — to know the track's path.
- `silencedetect` — to run the host-side ffmpeg analysis (requires `library.filesystem`, enforced
  at manifest validation time).

## Building

```sh
cd plugins/examples
make silence-marker
```

Produces `silence-marker.ndp`. Uses TinyGo if available, falls back to mainline Go's `wasip1`
target otherwise (see `plugins/examples/Makefile`).
