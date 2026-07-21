//go:build windows

package supervisor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unsafe"

	"github.com/zaigie/palworld-server-tool/internal/config"
	"golang.org/x/sys/windows"
)

type OSProcessLauncher struct{}

type osManagedProcess struct {
	cmd      *exec.Cmd
	exitCode int
}

func (OSProcessLauncher) Start(_ context.Context, processConfig ProcessConfig) (ManagedProcess, error) {
	settings := config.Default().ServerProcess
	settings.Enabled = processConfig.Enabled
	settings.ExecutablePath = processConfig.ExecutablePath
	settings.WorkingDirectory = processConfig.WorkingDirectory
	settings.Arguments = append([]string(nil), processConfig.Arguments...)
	if err := config.ValidateServerProcess(settings); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	cmd := exec.Command(processConfig.ExecutablePath, processConfig.Arguments...)
	cmd.Dir = processConfig.WorkingDirectory
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &osManagedProcess{cmd: cmd, exitCode: -1}, nil
}

func (process *osManagedProcess) PID() int { return process.cmd.Process.Pid }
func (process *osManagedProcess) Wait() error {
	err := process.cmd.Wait()
	if process.cmd.ProcessState != nil {
		process.exitCode = process.cmd.ProcessState.ExitCode()
	}
	return err
}
func (process *osManagedProcess) Kill() error   { return process.cmd.Process.Kill() }
func (process *osManagedProcess) ExitCode() int { return process.exitCode }

type OSProcessDetector struct{}

func (OSProcessDetector) FindPalServer() (int, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return 0, err
	}
	for {
		name := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		if name == "palserver.exe" || name == "palserver-win64-shipping-cmd.exe" {
			return int(entry.ProcessID), nil
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				return 0, nil
			}
			return 0, err
		}
	}
}
