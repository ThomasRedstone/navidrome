#!/usr/bin/env python3
"""Run inaSpeechSegmenter against the leading/trailing windows of an audio file and print the
combined segments as JSON.

Invoked by core/inaspeech's Go wrapper as:
    <python> segment.py <path>

Only the first and last WINDOW_SEC seconds of the track are classified, not the whole thing —
this plugin only ever cares about intro/outro speech (see speech-music-marker's main.go), and
classifying a full 20-70 minute podcast episode routinely took well over the host function's 30s
call timeout in practice (every single real-world call in an initial production run failed with
"context deadline exceeded" — see docs/architecture.md's "Phase 4" notes). Bounding the window
keeps each call's wall time close to the fixed per-call model-load cost (a fresh Python
subprocess reloads TensorFlow + the CNN weights every invocation — there's no cross-call
caching) regardless of how long the track actually is.

If a window's outer edge is still speech right at the boundary, that's a signal the real
intro/outro keeps going past what this window covered — rather than guessing at a bigger WINDOW_SEC
for every track (most don't need it), only tracks that actually hit the boundary get up to
MAX_EXTENSIONS additional WINDOW_SEC chunks appended, extending further in that direction until
the run of speech ends or the extension budget is spent. A track with a 5-second intro pays for
one window; a track with a genuine 4-minute intro pays for the extensions it actually needs.

Each classified chunk is run through Demucs vocal separation before classification. Talk-over-beat
content (a DJ/host talking over a continuous rhythmic track from second one, rather than a clean
silence-then-speech-then-music structure) turned out to be invisible to inaSpeechSegmenter on
the raw mix — verified against real podcast tracks, every "male"/"female" speech region
inaSpeechSegmenter found on the *original* audio dropped to statistically indistinguishable from
pure music once cross-checked. Separating the vocal stem first (Demucs' htdemucs model) resolves
this: once the beat is removed, the isolated vocal track shows a clean, strong energy jump
exactly where the talking starts. Demucs runs on CPU at roughly 5-6x realtime — the dominant
cost of each call, well above inaSpeechSegmenter's own classification time — hence this plugin's
manifest declares timeoutSeconds to extend past the default 30s per-call limit.

Prints a JSON array of {"label": str, "startMs": int, "endMs": int, "window": str} objects to
stdout, in chronological order, with every timestamp already track-relative. Any diagnostic
output goes to stderr so stdout stays pure JSON.
"""
import json
import os
import subprocess
import sys
import tempfile

# 2.5 minutes: covers every intro/outro observed in practice without needing an extension, while
# keeping the common-case cost down — most tracks don't have anywhere near this much lead-in.
# Don't drop below ~120s without checking real data first; a talk-heavy episode's intro can
# legitimately run close to 2 minutes even before the extension mechanism kicks in.
WINDOW_SEC = 150

# How many extra WINDOW_SEC chunks a single track can consume in one direction if it keeps
# hitting the boundary. Caps worst-case cost (a track that's genuinely mostly-speech in both its
# lead and trail windows) at roughly (MAX_EXTENSIONS + 1) x 2 chunks — factor this into the
# plugin manifest's timeoutSeconds if this is ever raised. Overridable via
# INASPEECH_MAX_EXTENSIONS for one-off deeper investigation of specific tracks (e.g. ones that
# hit the shipped default's cap) without changing the default every track pays for.
MAX_EXTENSIONS = int(os.environ.get("INASPEECH_MAX_EXTENSIONS", "1"))

SPEECH_LABELS = ("speech", "male", "female")
BOUNDARY_EPSILON_MS = 500


def probe_duration_sec(path):
    out = subprocess.run(
        ["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path],
        check=True, capture_output=True, text=True,
    )
    return float(out.stdout.strip())


def extract_clip(path, start_sec, duration_sec, out_path):
    # -ss before -i is a fast (keyframe-approximate) seek — fine here since we only need
    # second-level accuracy at the window boundary, not sample-accurate alignment. Demucs wants
    # its own sample rate/channel layout, so this intermediate clip stays at a normal music
    # sample rate (44.1kHz stereo) rather than the 16kHz mono inaSpeechSegmenter ultimately
    # wants — separate_vocals' own ffmpeg call does that final downmix/resample afterward.
    subprocess.run(
        [
            "ffmpeg", "-y", "-ss", str(start_sec), "-i", path, "-t", str(duration_sec),
            "-ar", "44100", "-ac", "2", out_path,
        ],
        check=True, capture_output=True,
    )


def separate_vocals(clip_path, work_dir):
    """Runs Demucs on clip_path (a short extracted window, not the full track — Demucs' cost is
    duration-proportional, unlike inaSpeechSegmenter's fixed model-load cost) and returns a
    16kHz mono WAV of just the isolated vocal stem, ready for inaSpeechSegmenter."""
    subprocess.run(
        [
            sys.executable, "-m", "demucs", "--two-stems=vocals",
            "-o", work_dir, clip_path,
        ],
        check=True, capture_output=True,
    )
    stem = os.path.splitext(os.path.basename(clip_path))[0]
    vocals_path = os.path.join(work_dir, "htdemucs", stem, "vocals.wav")

    # inaSpeechSegmenter wants 16kHz mono, same as the non-separated path.
    downmixed_path = os.path.join(work_dir, f"{stem}_vocals_16k.wav")
    subprocess.run(
        ["ffmpeg", "-y", "-i", vocals_path, "-ar", "16000", "-ac", "1", downmixed_path],
        check=True, capture_output=True,
    )
    return downmixed_path


def classify(seg, path, devnull_fd):
    # Keras' predict() writes its own progress lines ("5/5 - 1s - 103ms/step") straight to
    # stdout at the file-descriptor level (not just sys.stdout), ignoring verbose settings on
    # some versions — redirecting sys.stdout alone doesn't catch it. Redirect fd 1 itself to
    # devnull for the classification call, then restore it, so stdout stays pure JSON.
    saved_stdout_fd = os.dup(1)
    try:
        os.dup2(devnull_fd, 1)
        return seg(path)
    finally:
        os.dup2(saved_stdout_fd, 1)


def classify_chunk(path, start_sec, duration_sec, work_dir, seg, devnull_fd):
    """Extracts, separates, and classifies a single chunk, returning segments with track-relative
    (not chunk-relative) millisecond timestamps."""
    clip = os.path.join(work_dir, f"clip_{round(start_sec * 1000)}.wav")
    extract_clip(path, start_sec, duration_sec, clip)
    vocals = separate_vocals(clip, work_dir)
    offset_ms = int(round(start_sec * 1000))
    return [
        {"label": label, "startMs": offset_ms + int(round(s * 1000)), "endMs": offset_ms + int(round(e * 1000))}
        for label, s, e in classify(seg, vocals, devnull_fd)
    ]


def classify_lead(path, duration_sec, work_dir, seg, devnull_fd):
    """Classifies from the start of the track forward, extending in WINDOW_SEC increments (up
    to MAX_EXTENSIONS times) as long as the run of speech keeps reaching each chunk's end."""
    segments = []
    start = 0.0
    for _ in range(MAX_EXTENSIONS + 1):
        chunk_dur = min(WINDOW_SEC, duration_sec - start)
        if chunk_dur <= 0:
            break
        chunk = classify_chunk(path, start, chunk_dur, work_dir, seg, devnull_fd)
        segments.extend(chunk)
        if not chunk:
            break
        last = chunk[-1]
        chunk_end_ms = int(round((start + chunk_dur) * 1000))
        if last["label"] not in SPEECH_LABELS or (chunk_end_ms - last["endMs"]) > BOUNDARY_EPSILON_MS:
            break
        start += chunk_dur
    for s in segments:
        s["window"] = "lead"
    return segments


def classify_trail(path, duration_sec, work_dir, seg, devnull_fd):
    """Classifies from the end of the track backward, extending in WINDOW_SEC increments (up to
    MAX_EXTENSIONS times) as long as the run of speech keeps starting right at each chunk's
    start."""
    segments = []
    end = duration_sec
    for _ in range(MAX_EXTENSIONS + 1):
        chunk_dur = min(WINDOW_SEC, end)
        if chunk_dur <= 0:
            break
        start = end - chunk_dur
        chunk = classify_chunk(path, start, chunk_dur, work_dir, seg, devnull_fd)
        segments = chunk + segments
        if not chunk:
            break
        first = chunk[0]
        chunk_start_ms = int(round(start * 1000))
        if first["label"] not in SPEECH_LABELS or (first["startMs"] - chunk_start_ms) > BOUNDARY_EPSILON_MS:
            break
        end = start
    for s in segments:
        s["window"] = "trail"
    return segments


def main():
    if len(sys.argv) != 2:
        print("usage: segment.py <audio-file>", file=sys.stderr)
        sys.exit(2)

    path = sys.argv[1]

    # Imported here (not at module scope) so `--help`/usage errors above don't pay
    # TensorFlow's import cost, and so a missing/broken install fails with the exception
    # attached to the actual invocation rather than on module load.
    from inaSpeechSegmenter import Segmenter

    duration_sec = probe_duration_sec(path)
    seg = Segmenter()
    devnull_fd = os.open(os.devnull, os.O_WRONLY)

    out = []
    try:
        with tempfile.TemporaryDirectory() as tmp:
            # classify_lead is always bounded by duration_sec, so on a short track it already
            # covers the whole thing on its own (naturally, via its chunk_dur = min(WINDOW_SEC,
            # duration_sec - start) cap) — no special-casing needed there.
            out = classify_lead(path, duration_sec, tmp, seg, devnull_fd)

            # Only run a separate trail pass if it wouldn't reclassify audio the (possibly
            # extended) lead pass already covered.
            lead_end_ms = max((s["endMs"] for s in out), default=0)
            trail_start_ms = int(round((duration_sec - min(WINDOW_SEC, duration_sec)) * 1000))
            if trail_start_ms > lead_end_ms:
                out += classify_trail(path, duration_sec, tmp, seg, devnull_fd)
    finally:
        os.close(devnull_fd)

    out.sort(key=lambda s: s["startMs"])
    print(json.dumps(out))


if __name__ == "__main__":
    main()
