# Speech/Music Marker Plugin

A `MediaMarkerProvider` plugin that detects spoken content leading into / trailing out of a
track's music — e.g. a DJ's intro before the track proper starts, or an outro announcement after
it fades — and reports it as `skip/intro_speech` / `skip/outro_speech` media markers.

No network access, no external service dependency: it works entirely against the local file via
the host-side `SpeechMusicDetect` service, which runs [inaSpeechSegmenter][ina] on Navidrome's
server. WASM plugins can't spawn subprocesses (and inaSpeechSegmenter is a Python/TensorFlow
tool, not something that could be compiled into the WASM sandbox), so this host function exists
specifically to give a plugin that classification without needing Python inside the sandbox.

[ina]: https://github.com/ina-foss/inaSpeechSegmenter

## Server setup

Unlike `silence-marker`'s ffmpeg dependency (a single binary, usually already present),
inaSpeechSegmenter needs a dedicated Python virtualenv:

```bash
python3 -m venv /opt/inaspeech-venv
/opt/inaspeech-venv/bin/pip install inaSpeechSegmenter
```

Then point Navidrome at that interpreter:

```toml
InaSpeechPythonPath = "/opt/inaspeech-venv/bin/python3"
```

(or `ND_INASPEECHPYTHONPATH` as an environment variable). There is deliberately no PATH-based
default — a bare `python3` almost never has inaSpeechSegmenter installed, and running against the
wrong interpreter would fail confusingly deep inside a Python traceback rather than with a clear
"not configured" error. First run per track downloads inaSpeechSegmenter's CNN models
(~10MB) from GitHub and caches them in the venv; subsequent runs are model-load-only.

Classification is much slower than ffmpeg's silencedetect — expect several seconds to tens of
seconds per track (CPU-bound; no GPU is required, and none of this repo's builds attempt to use
one). Scan time scales accordingly for large libraries.

## How it decides "intro" vs "outro" vs "ignore"

`SpeechMusicDetect` reports every classified segment across the whole track (`speech`/`male`/
`female`, `music`, `noise`, `noEnergy`), not just the edges — a DJ talking over a mid-track
breakdown would also show up as a `speech` segment. This plugin only turns the *leading* and
*trailing* runs into markers, and only when a `music` segment exists at all:

- No `music` segment anywhere in the track → nothing to skip into or out of (a talk-only podcast
  episode, say) — the whole track is left alone rather than flagged as one giant intro.
- The run of segments before the first `music` segment → `skip/intro_speech`, from `0` to the
  run's end, **but only if that run contains an actual speech segment** (a leading `noise` or
  `noEnergy` run with no speech is left to `silence-marker` / ignored, not double-marked).
- The run of segments after the last `music` segment → `skip/outro_speech`, from the run's start
  to the track's duration, same speech-segment requirement.
- Speech in the middle of the track (between two music segments) is ignored — this plugin
  answers "spoken content around the music," not "flag every time someone talks."

## Permissions

- `library` with `filesystem: true` — to know the track's path.
- `speechmusicdetect` — to run the host-side inaSpeechSegmenter analysis (requires
  `library.filesystem`, enforced at manifest validation time).
