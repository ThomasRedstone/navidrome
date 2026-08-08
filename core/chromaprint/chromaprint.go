// Package chromaprint wraps the fpcalc CLI (from the Chromaprint/AcoustID project) to compute
// audio fingerprints. This is a separate binary from ffmpeg — same "server-side subprocess a
// WASM plugin can't spawn itself" shape as core/ffmpeg's silencedetect wrapping, but a different
// dependency plugin authors must install.
package chromaprint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
)

// FingerprintResult is a computed Chromaprint fingerprint plus the duration fpcalc measured.
type FingerprintResult struct {
	Fingerprint string
	DurationMs  int64
}

type Chromaprint interface {
	// Fingerprint computes the Chromaprint fingerprint for the given file.
	Fingerprint(ctx context.Context, filePath string) (*FingerprintResult, error)
	CmdPath() (string, error)
	IsAvailable() bool
}

func New() Chromaprint {
	return &chromaprint{}
}

type chromaprint struct{}

// fpcalcJSON mirrors `fpcalc -json`'s output: {"duration": 6.5, "fingerprint": "..."}.
type fpcalcJSON struct {
	Duration    float64 `json:"duration"`
	Fingerprint string  `json:"fingerprint"`
}

func (c *chromaprint) Fingerprint(ctx context.Context, filePath string) (*FingerprintResult, error) {
	cmdPath, err := fpcalcCmd()
	if err != nil {
		return nil, err
	}
	if err := fileExists(filePath); err != nil {
		return nil, fmt.Errorf("fpcalc: %w", err)
	}

	args := []string{cmdPath, "-json", filePath}
	log.Trace(ctx, "Executing fpcalc command", "args", args)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) // #nosec
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("fpcalc: %w", err)
	}

	var parsed fpcalcJSON
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil, fmt.Errorf("fpcalc: parsing output: %w", err)
	}
	return &FingerprintResult{
		Fingerprint: parsed.Fingerprint,
		DurationMs:  int64(parsed.Duration * 1000),
	}, nil
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

func (c *chromaprint) CmdPath() (string, error) {
	return fpcalcCmd()
}

func (c *chromaprint) IsAvailable() bool {
	_, err := fpcalcCmd()
	return err == nil
}

var (
	fpcalcOnce sync.Once
	fpcalcPath string
	fpcalcErr  error
)

func fpcalcCmd() (string, error) {
	fpcalcOnce.Do(func() {
		if conf.Server.FpcalcPath != "" {
			fpcalcPath, fpcalcErr = exec.LookPath(conf.Server.FpcalcPath)
		} else {
			fpcalcPath, fpcalcErr = exec.LookPath("fpcalc")
			if errors.Is(fpcalcErr, exec.ErrDot) {
				fpcalcPath, fpcalcErr = exec.LookPath("./fpcalc")
			}
		}
		if fpcalcErr == nil {
			log.Info("Found fpcalc", "path", fpcalcPath)
		}
	})
	return fpcalcPath, fpcalcErr
}
