package supervisor

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPalServerEnvironmentSetsDerivedBridgeIPCPath(t *testing.T) {
	executable := filepath.Join("server", "PalServer.exe")
	environment := palServerEnvironment(
		[]string{"PATH=test", "pst_bridge_ipc_root=untrusted"},
		ProcessConfig{ExecutablePath: executable},
	)

	var values []string
	for _, item := range environment {
		if strings.HasPrefix(strings.ToUpper(item), bridgeIPCEnvironmentKey+"=") {
			values = append(values, item)
		}
	}
	if len(values) != 1 {
		t.Fatalf("Bridge IPC environment values = %v, want exactly one", values)
	}
	want := bridgeIPCEnvironmentKey + "=" + filepath.Join("server", "Pal", "Saved", "PSTProductionBridge")
	if values[0] != want {
		t.Fatalf("Bridge IPC environment = %q, want %q", values[0], want)
	}
}
