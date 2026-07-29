package supervisor

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// suppressServerOutput reports whether a PalServer console line is a routine
// successful REST access emitted by the game server itself. These messages are
// produced for every PST polling request and do not carry diagnostic value.
// Errors, warnings, mutations and non-REST server output remain visible.
func suppressServerOutput(line string) bool {
	line = strings.TrimSpace(line)
	return strings.Contains(line, "REST accessed endpoint /v1/api/") && strings.HasSuffix(line, " OK")
}

// serverOutputFilter preserves PalServer output while dropping only routine
// successful REST access lines. A buffered writer is used because os/exec may
// write partial lines to a process output writer.
type serverOutputFilter struct {
	mu      sync.Mutex
	dst     io.Writer
	pending []byte
}

func (f *serverOutputFilter) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	n := len(p)
	f.pending = append(f.pending, p...)
	for {
		lineEnd := bytes.IndexByte(f.pending, '\n')
		if lineEnd < 0 {
			break
		}
		line := f.pending[:lineEnd+1]
		f.pending = f.pending[lineEnd+1:]
		if suppressServerOutput(string(line)) {
			continue
		}
		if _, err := f.dst.Write(line); err != nil {
			return 0, err
		}
	}
	return n, nil
}

func (f *serverOutputFilter) Flush() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.pending) == 0 {
		return nil
	}
	line := f.pending
	f.pending = nil
	if suppressServerOutput(string(line)) {
		return nil
	}
	_, err := f.dst.Write(line)
	return err
}
