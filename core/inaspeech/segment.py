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

Prints a JSON array of {"label": str, "startMs": int, "endMs": int} objects to stdout, in
chronological order, with trail-window timestamps already offset to be track-relative. Any
diagnostic output goes to stderr so stdout stays pure JSON.
"""
import json
import os
import subprocess
import sys
import tempfile

# 2.5 minutes: covers every intro/outro observed in practice (max seen so far: ~119s) with
# margin, while trimming a bit of per-call time — though most of a call's wall time is the
# fixed TensorFlow/model-load cost (~13-14s), not audio duration, so shrinking this window
# further has rapidly diminishing returns. Don't drop below ~120s without checking real data
# first; a talk-heavy episode's intro can legitimately run close to 2 minutes.
WINDOW_SEC = 150


def probe_duration_sec(path):
    out = subprocess.run(
        ["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path],
        check=True, capture_output=True, text=True,
    )
    return float(out.stdout.strip())


def extract_clip(path, start_sec, duration_sec, out_path):
    # -ss before -i is a fast (keyframe-approximate) seek — fine here since we only need
    # second-level accuracy at the window boundary, not sample-accurate alignment.
    subprocess.run(
        [
            "ffmpeg", "-y", "-ss", str(start_sec), "-i", path, "-t", str(duration_sec),
            "-ar", "16000", "-ac", "1", out_path,
        ],
        check=True, capture_output=True,
    )


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
            lead_dur = min(WINDOW_SEC, duration_sec)
            trail_dur = min(WINDOW_SEC, duration_sec)
            trail_start = max(0.0, duration_sec - trail_dur)

            lead_clip = os.path.join(tmp, "lead.wav")
            extract_clip(path, 0, lead_dur, lead_clip)
            for label, start, end in classify(seg, lead_clip, devnull_fd):
                out.append({"label": label, "startMs": int(round(start * 1000)), "endMs": int(round(end * 1000))})

            # Skip a separate trail pass when the windows already overlap/cover the whole
            # track — avoids double-counting the same audio and a wasted second model call on
            # short tracks.
            if trail_start > lead_dur:
                trail_clip = os.path.join(tmp, "trail.wav")
                extract_clip(path, trail_start, trail_dur, trail_clip)
                offset_ms = int(round(trail_start * 1000))
                for label, start, end in classify(seg, trail_clip, devnull_fd):
                    out.append({
                        "label": label,
                        "startMs": offset_ms + int(round(start * 1000)),
                        "endMs": offset_ms + int(round(end * 1000)),
                    })
    finally:
        os.close(devnull_fd)

    out.sort(key=lambda s: s["startMs"])
    print(json.dumps(out))


if __name__ == "__main__":
    main()
