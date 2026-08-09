package tests

import (
	"context"

	"github.com/navidrome/navidrome/core/inaspeech"
)

func NewMockInaSpeech() *MockInaSpeech {
	return &MockInaSpeech{}
}

type MockInaSpeech struct {
	Error           error
	Segments        []inaspeech.Segment
	LastSegmentPath string
}

func (m *MockInaSpeech) Segment(_ context.Context, filePath string) ([]inaspeech.Segment, error) {
	m.LastSegmentPath = filePath
	if m.Error != nil {
		return nil, m.Error
	}
	return m.Segments, nil
}

func (m *MockInaSpeech) CmdPath() (string, error) {
	return "python3", nil
}

func (m *MockInaSpeech) IsAvailable() bool {
	return true
}
