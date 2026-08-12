# Speech/Music Marker Plugin

A `MediaMarkerProvider` plugin that detects spoken content leading into / trailing out of a
track's music — e.g. a DJ's intro before the track proper starts, or an outro announcement after
it fades — and reports it as `skip/intro_speech` / `skip/outro_speech` media markers.

No network access, no external service dependency: it works entirely against the local file via
the host-side `SpeechMusicDetect` service, which runs [inaSpeechSegmenter][ina] (after separating
out the vocal stem with [Demucs][demucs] first — see below) on Navidrome's server. WASM plugins
can't spawn subprocesses (and both tools are Python-based, not something that could be compiled
into the WASM sandbox), so this host function exists specifically to give a plugin that
classification without needing Python inside the sandbox.

[ina]: https://github.com/ina-foss/inaSpeechSegmenter
[demucs]: https://github.com/facebookresearch/demucs

## Server setup

Unlike `silence-marker`'s ffmpeg dependency (a single binary, usually already present), this
needs a dedicated Python virtualenv with both inaSpeechSegmenter and Demucs:

```bash
python3 -m venv /opt/inaspeech-venv
/opt/inaspeech-venv/bin/pip install inaSpeechSegmenter demucs
```

Then point Navidrome at that interpreter:

```toml
InaSpeechPythonPath = "/opt/inaspeech-venv/bin/python3"
```

(or `ND_INASPEECHPYTHONPATH` as an environment variable). There is deliberately no PATH-based
default — a bare `python3` almost never has these installed, and running against the wrong
interpreter would fail confusingly deep inside a Python traceback rather than with a clear "not
configured" error. First run downloads inaSpeechSegmenter's CNN models (~10MB) and Demucs'
separation model (~80MB) from their respective hosts and caches them in the venv; subsequent
runs skip the download.

Classification is much slower than ffmpeg's silencedetect — expect **around a minute per track**
(CPU-bound; no GPU is required, and none of this repo's builds attempt to use one). Demucs
dominates that cost (roughly 5-6x realtime on CPU), not the classifier itself. This plugin's
manifest declares `timeoutSeconds: 150` to give each call room — see `plugins/README.md`'s
Manifest docs for what that field does and why the default 30s wasn't enough here. Scan time
scales accordingly for large libraries; this is squarely a workstation/offline-analysis tool,
not something to run against a resource-constrained production server (see
`~/media-time-markers`'s architecture doc for the intended workstation-computes /
production-reads-published-data split).

### Why Demucs is here at all

`SpeechMusicDetect` originally ran inaSpeechSegmenter directly against the track's audio. That
works for a clean radio-style structure (silence → speech → music, cleanly separated), but fails
completely on **talk-over-a-beat** content — a DJ/host talking from second one over a continuous
rhythmic backing track, which is the norm for workout-mix and DJ-set podcasts. Verified against
real tracks: inaSpeechSegmenter's own "speech" classification on the raw mix, and even a
dedicated voice-activity-detection model (Silero VAD) run directly against the raw mix, both
turned out statistically indistinguishable from pure music in the actual speech region — the
beat's energy dominates the frame-level features either tool looks at, effectively hiding the
voice.

Running the raw mix through Demucs' vocal-isolation model first and classifying *that* instead
resolved this cleanly: once the beat is removed, the classifier (and even a much simpler
"is there significant energy in the isolated channel" check) sees an unambiguous signal exactly
where the talking starts and stops.

## How it decides "intro" vs "outro" vs "ignore"

`SpeechMusicDetect` only ever classifies the leading and trailing `WINDOW_SEC` (150s) of the
track, not the whole thing (see `core/inaspeech/segment.py`) — classifying a full-length episode
routinely exceeded even the extended timeout, and this plugin only ever cares about the edges
anyway. Each returned segment is tagged with which window it came from (`lead` or `trail`); this
plugin processes the two windows **independently**, not as one continuous timeline — there's a
real time gap between them for any track longer than ~2x the window size, and after vocal
separation a `music` label essentially never appears at all (there's no music left in an isolated
vocal stem to classify), so the original "find the music boundary" logic doesn't apply within a
single window's data.

- `introMarker` looks only at the lead window: `skip/intro_speech`, from `0` to the end of the
  last speech-labeled segment in that window — or nothing, if the window shows no speech at all.
- `outroMarker` looks only at the trail window: `skip/outro_speech`, from the start of the first
  speech-labeled segment to the track's duration — or nothing, same condition.
- If a `music`-labeled segment does still turn up in a window (possible for content types Demucs
  handles imperfectly), only speech before it (lead) / after it (trail) counts, matching the
  original non-windowed design as a fallback.
- Because each window is itself bounded (`WINDOW_SEC`), a false-positive "intro" on a genuinely
  talk-only episode is capped at that window's length, not the whole episode — a much smaller
  downside than the pre-windowing design's unbounded full-track risk, so this version doesn't
  require a `music` segment to exist anywhere before emitting a marker.

## Permissions

- `library` with `filesystem: true` — to know the track's path.
- `speechmusicdetect` — to run the host-side classification (requires `library.filesystem`,
  enforced at manifest validation time).
