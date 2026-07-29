package supervisor

import (
	"path/filepath"
	"strings"
)

const bridgeIPCEnvironmentKey = "PST_BRIDGE_IPC_ROOT"

// palServerEnvironment gives server-side helpers a stable IPC path without
// relying on PalServer's or UE4SS's current working directory. The value is
// always derived from the already validated PalServer.exe path.
func palServerEnvironment(base []string, processConfig ProcessConfig) []string {
	prefix := bridgeIPCEnvironmentKey + "="
	environment := make([]string, 0, len(base)+1)
	for _, item := range base {
		key, _, found := strings.Cut(item, "=")
		if found && strings.EqualFold(strings.TrimSpace(key), bridgeIPCEnvironmentKey) {
			continue
		}
		environment = append(environment, item)
	}
	root := filepath.Dir(strings.TrimSpace(processConfig.ExecutablePath))
	ipcRoot := filepath.Join(root, "Pal", "Saved", "PSTProductionBridge")
	return append(environment, prefix+ipcRoot)
}
