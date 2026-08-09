#!/usr/bin/env python3
"""Run inaSpeechSegmenter against a single audio file and print its segments as JSON.

Invoked by core/inaspeech's Go wrapper as:
    <python> segment.py <path>

Prints a JSON array of {"label": str, "startMs": int, "endMs": int} objects to stdout, in
chronological order. Any diagnostic output goes to stderr so stdout stays pure JSON.
"""
import json
import os
import sys


def main():
    if len(sys.argv) != 2:
        print("usage: segment.py <audio-file>", file=sys.stderr)
        sys.exit(2)

    path = sys.argv[1]

    # Imported here (not at module scope) so `--help`/usage errors above don't pay
    # TensorFlow's import cost, and so a missing/broken install fails with the exception
    # attached to the actual invocation rather than on module load.
    from inaSpeechSegmenter import Segmenter

    seg = Segmenter()

    # Keras' predict() writes its own progress lines ("5/5 - 1s - 103ms/step") straight to
    # stdout at the file-descriptor level (not just sys.stdout), ignoring verbose settings on
    # some versions — redirecting sys.stdout alone doesn't catch it. Redirect fd 1 itself to
    # devnull for the classification call, then restore it before printing the JSON result, so
    # stdout stays pure JSON for the Go caller to parse.
    devnull_fd = os.open(os.devnull, os.O_WRONLY)
    saved_stdout_fd = os.dup(1)
    try:
        os.dup2(devnull_fd, 1)
        segments = seg(path)
    finally:
        os.dup2(saved_stdout_fd, 1)
        os.close(saved_stdout_fd)
        os.close(devnull_fd)

    out = [
        {"label": label, "startMs": int(round(start * 1000)), "endMs": int(round(end * 1000))}
        for label, start, end in segments
    ]
    print(json.dumps(out))


if __name__ == "__main__":
    main()
