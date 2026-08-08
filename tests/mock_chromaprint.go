package tests

import (
	"context"

	"github.com/navidrome/navidrome/core/chromaprint"
)

func NewMockChromaprint() *MockChromaprint {
	return &MockChromaprint{}
}

type MockChromaprint struct {
	Result               *chromaprint.FingerprintResult
	Err                  error
	LastFingerprintPath  string
	FingerprintAvailable bool
}

func (c *MockChromaprint) Fingerprint(_ context.Context, path string) (*chromaprint.FingerprintResult, error) {
	c.LastFingerprintPath = path
	if c.Err != nil {
		return nil, c.Err
	}
	if c.Result != nil {
		return c.Result, nil
	}
	return &chromaprint.FingerprintResult{}, nil
}

func (c *MockChromaprint) CmdPath() (string, error) {
	if c.Err != nil {
		return "", c.Err
	}
	return "fpcalc", nil
}

func (c *MockChromaprint) IsAvailable() bool {
	return c.FingerprintAvailable
}

var _ chromaprint.Chromaprint = (*MockChromaprint)(nil)
