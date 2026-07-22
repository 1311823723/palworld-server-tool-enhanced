//go:build !windows

package supervisor

import "context"

type OSProcessLauncher struct{}

func (OSProcessLauncher) Start(_ context.Context, _ ProcessConfig) (ManagedProcess, error) {
	return nil, ErrUnsupportedPlatform
}

type OSProcessDetector struct{}

func (OSProcessDetector) FindPalServer() (int, error) { return 0, nil }
