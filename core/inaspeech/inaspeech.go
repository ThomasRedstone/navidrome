// Package inaspeech wraps an inaSpeechSegmenter Python environment to classify audio into
// speech/music/noise/silence segments. Unlike core/ffmpeg's silencedetect (a single static
// binary) and core/chromaprint's fpcalc (also a single binary), inaSpeechSegmenter is a Python
// package with a TensorFlow dependency — deploying it means a dedicated virtualenv with a
// python3 interpreter that has `inaSpeechSegmenter` installed, not a single downloadable tool.
// conf.Server.InaSpeechPythonPath must point at that interpreter (e.g.
// /opt/inaspeech-venv/bin/python3); there is no bundled/PATH-discovered default, since
// installing it accidentally onto a system python would be a heavy, surprising side effect.
package inaspeech

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
)

//go:embed segment.py
var segmentScript string

// Segment is a labeled span of a track, in milliseconds, as classified by inaSpeechSegmenter.
type Segment struct {
	// Label is one of inaSpeechSegmenter's classes: "speech", "music", "noEnergy" (silence),
	// or "noise". Some model variants also emit "male"/"female" in place of "speech" — see
	// Segment.IsSpeech.
	Label   string
	StartMs int64
	EndMs   int64
	// Window is "lead" or "trail" — which classified window (see segment.py's WINDOW_SEC) this
	// segment came from. Callers must not treat the combined Segment list as one continuous
	// timeline when reasoning about a within-window boundary; see host.SpeechMusicSegment's
	// doc comment for why.
	Window string
}

// IsSpeech reports whether the segment is a speech segment, regardless of which
// gender-labeled variant inaSpeechSegmenter's model reported.
func (s Segment) IsSpeech() bool {
	return s.Label == "speech" || s.Label == "male" || s.Label == "female"
}

type InaSpeech interface {
	// Segment runs inaSpeechSegmenter against filePath and returns its labeled segments in
	// chronological order.
	Segment(ctx context.Context, filePath string) ([]Segment, error)
	CmdPath() (string, error)
	IsAvailable() bool
}

func New() InaSpeech {
	return &inaSpeech{}
}

type inaSpeech struct{}

type segmentJSON struct {
	Label   string `json:"label"`
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Window  string `json:"window"`
}

func (i *inaSpeech) Segment(ctx context.Context, filePath string) ([]Segment, error) {
	pythonPath, err := pythonCmd()
	if err != nil {
		return nil, err
	}
	if err := fileExists(filePath); err != nil {
		return nil, fmt.Errorf("inaspeech: %w", err)
	}
	scriptPath, err := ensureScript()
	if err != nil {
		return nil, fmt.Errorf("inaspeech: %w", err)
	}

	args := []string{pythonPath, scriptPath, filePath}
	log.Trace(ctx, "Executing inaSpeechSegmenter command", "args", args)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) // #nosec
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("inaspeech: %w", detailedExecError(err))
	}

	var raw []segmentJSON
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("inaspeech: parsing output: %w", err)
	}
	segments := make([]Segment, len(raw))
	for idx, r := range raw {
		segments[idx] = Segment{Label: r.Label, StartMs: r.StartMs, EndMs: r.EndMs, Window: r.Window}
	}
	return segments, nil
}

func detailedExecError(err error) error {
	if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, exitErr.Stderr)
	}
	return err
}

func fileExists(path string) error {
	s, err := os.Stat(path)
	if err != nil {
		return err
	}
	if s.IsDir() {
		return fmt.Errorf("'%s' is a directory", path)
	}
	return nil
}

func (i *inaSpeech) CmdPath() (string, error) {
	return pythonCmd()
}

func (i *inaSpeech) IsAvailable() bool {
	_, err := pythonCmd()
	return err == nil
}

var (
	pythonOnce sync.Once
	pythonPath string
	pythonErr  error
)

// pythonCmd resolves conf.Server.InaSpeechPythonPath. Unlike fpcalcCmd/ffmpegCmd, there is no
// PATH fallback — a bare "python3" on PATH almost never has inaSpeechSegmenter installed, and
// silently running against the wrong interpreter would fail confusingly deep inside a Python
// traceback rather than with a clear "not configured" error.
func pythonCmd() (string, error) {
	pythonOnce.Do(func() {
		if conf.Server.InaSpeechPythonPath == "" {
			pythonErr = fmt.Errorf("InaSpeechPythonPath not configured")
			return
		}
		pythonPath, pythonErr = exec.LookPath(conf.Server.InaSpeechPythonPath)
		if pythonErr == nil {
			log.Info("Found inaSpeechSegmenter python interpreter", "path", pythonPath)
		}
	})
	return pythonPath, pythonErr
}

var (
	scriptOnce sync.Once
	scriptPath string
	scriptErr  error
)

// ensureScript writes the embedded segment.py helper to the cache folder once per process and
// returns its path. The script itself is tiny (a few dozen lines); writing it out on demand
// avoids requiring admins to separately deploy it alongside the venv.
func ensureScript() (string, error) {
	scriptOnce.Do(func() {
		dir, err := conf.Server.CacheFolder.Path()
		if err != nil {
			scriptErr = err
			return
		}
		path := filepath.Join(dir, "inaspeech_segment.py")
		if err := os.WriteFile(path, []byte(segmentScript), 0o600); err != nil {
			scriptErr = fmt.Errorf("writing inaspeech segment script: %w", err)
			return
		}
		scriptPath = path
	})
	return scriptPath, scriptErr
}
