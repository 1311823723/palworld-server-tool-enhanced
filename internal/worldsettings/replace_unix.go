//go:build !windows

package worldsettings

import "os"

func replaceFile(source, destination string) error { return os.Rename(source, destination) }
