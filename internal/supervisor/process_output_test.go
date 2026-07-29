package supervisor

import (
	"bytes"
	"testing"
)

func TestSuppressServerOutput(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "successful info request",
			line: "[2026-07-27 17:26:46] [LOG] REST accessed endpoint /v1/api/info OK\n",
			want: true,
		},
		{
			name: "successful polling request",
			line: "[LOG] REST accessed endpoint /v1/api/players OK\n",
			want: true,
		},
		{
			name: "failed request",
			line: "[LOG] REST accessed endpoint /v1/api/players 500\n",
			want: false,
		},
		{
			name: "server warning",
			line: "[LOG] REST accessed endpoint /v1/api/players failed\n",
			want: false,
		},
		{
			name: "unrelated output",
			line: "[LOG] PalServer started\n",
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := suppressServerOutput(test.line); got != test.want {
				t.Fatalf("suppressServerOutput() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestServerOutputFilterHandlesPartialLines(t *testing.T) {
	var output bytes.Buffer
	filter := &serverOutputFilter{dst: &output}

	first := "[LOG] REST accessed endpoint /v1/api/me"
	if written, err := filter.Write([]byte(first)); err != nil || written != len(first) {
		t.Fatalf("first Write() = (%d, %v), want (%d, nil)", written, err, len(first))
	}
	second := " OK\n[LOG] PalServer warning\n"
	if written, err := filter.Write([]byte(second)); err != nil || written != len(second) {
		t.Fatalf("second Write() = (%d, %v), want (%d, nil)", written, err, len(second))
	}
	if err := filter.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	if got, want := output.String(), "[LOG] PalServer warning\n"; got != want {
		t.Fatalf("filtered output = %q, want %q", got, want)
	}
}
