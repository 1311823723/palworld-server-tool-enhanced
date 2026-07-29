package production

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zaigie/palworld-server-tool/internal/supervisor"
)

func prepareInstallerTest(t *testing.T, withUE4SS bool) (*Installer, supervisor.ProcessConfig, InstallPaths) {
	t.Helper()
	root := t.TempDir()
	palRoot := filepath.Join(root, "PalServer")
	if err := os.MkdirAll(palRoot, 0755); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(palRoot, "PalServer.exe")
	if err := os.WriteFile(executable, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	if withUE4SS {
		ue4ss := filepath.Join(palRoot, "Mods", "NativeMods", "UE4SS")
		if err := os.MkdirAll(ue4ss, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ue4ss, "UE4SS.dll"), []byte("loader"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	pstRoot := filepath.Join(root, "PST")
	source := filepath.Join(pstRoot, "extras", BridgeName)
	if err := os.MkdirAll(filepath.Join(source, "Scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Info.json":        `{"Name":"PSTProductionBridge"}`,
		"Scripts/main.lua": `print("bridge")`,
	}
	manifest := Manifest{Name: BridgeName, Version: BridgeVersion, ProtocolVersion: BridgeProtocolVersion}
	for name, content := range files {
		path := filepath.Join(source, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		digest, err := fileSHA256(path)
		if err != nil {
			t.Fatal(err)
		}
		manifest.Files = append(manifest.Files, ManifestFile{Path: name, SHA256: digest})
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(source, "manifest.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	installer := &Installer{GOOS: "windows", ExecutableDir: func() (string, error) { return pstRoot, nil }}
	processConfig := supervisor.ProcessConfig{Enabled: true, ExecutablePath: executable, WorkingDirectory: palRoot}
	paths, err := installer.paths(processConfig)
	if err != nil {
		t.Fatal(err)
	}
	return installer, processConfig, paths
}

func TestDetectRequiresUE4SSWithoutManagingIt(t *testing.T) {
	installer, processConfig, _ := prepareInstallerTest(t, false)
	detection := installer.Detect(processConfig)
	if detection.State != BridgeDependencyMissing {
		t.Fatalf("state = %s, want %s (%s)", detection.State, BridgeDependencyMissing, detection.Message)
	}
	if detection.Automatic {
		t.Fatal("automatic install must remain disabled when UE4SS is absent")
	}
	if detection.ManualGuide().UE4SSDirectory == "" {
		t.Fatal("manual guide should contain the expected UE4SS directory")
	}
}

func TestInstallPreservesINIAndRollback(t *testing.T) {
	installer, processConfig, paths := prepareInstallerTest(t, true)
	original := "; keep this comment\r\nActiveModList=ExistingMod\r\nUnknownSetting=42\r\n"
	if err := os.MkdirAll(filepath.Dir(paths.Settings), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Settings, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	transaction, err := installer.Install(processConfig, false)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	updated, _ := os.ReadFile(paths.Settings)
	text := string(updated)
	for _, expected := range []string{"; keep this comment", "UnknownSetting=42", "ExistingMod", BridgeName, "bGlobalEnableMod=true"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("updated INI is missing %q:\n%s", expected, text)
		}
	}
	if _, err := os.Stat(filepath.Join(paths.Target, "Scripts", "main.lua")); err != nil {
		t.Fatalf("installed file: %v", err)
	}
	if err := transaction.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if _, err := os.Stat(paths.Target); !os.IsNotExist(err) {
		t.Fatalf("target should be removed after rollback, err=%v", err)
	}
	restored, _ := os.ReadFile(paths.Settings)
	if string(restored) != original {
		t.Fatalf("INI was not restored exactly:\n%s", restored)
	}
}

func TestDisableRollbackDoesNotDeleteBridgeFiles(t *testing.T) {
	installer, processConfig, paths := prepareInstallerTest(t, true)
	transaction, err := installer.Install(processConfig, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	disable, err := installer.Disable(processConfig)
	if err != nil {
		t.Fatal(err)
	}
	if modEnabled(paths.Settings) {
		t.Fatal("Bridge should be disabled")
	}
	if err := disable.Rollback(); err != nil {
		t.Fatal(err)
	}
	if !modEnabled(paths.Settings) {
		t.Fatal("Bridge activation should be restored")
	}
	if _, err := os.Stat(filepath.Join(paths.Target, "Info.json")); err != nil {
		t.Fatalf("disable rollback removed Bridge files: %v", err)
	}
}

func TestManifestRejectsPathTraversalAndSymlink(t *testing.T) {
	root := t.TempDir()
	if _, err := safeChild(root, "../outside"); err == nil {
		t.Fatal("path traversal should be rejected")
	}
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("target"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := requireRegularFile(link); err == nil {
		t.Fatal("symlink must not be accepted as a Bridge source file")
	}
}

func TestInstallPathsAlwaysDeriveFromExecutable(t *testing.T) {
	installer, processConfig, expected := prepareInstallerTest(t, true)
	processConfig.WorkingDirectory = filepath.Join(t.TempDir(), "unrelated-working-directory")
	paths, err := installer.paths(processConfig)
	if err != nil {
		t.Fatal(err)
	}
	if paths.PalServerRoot != expected.PalServerRoot || paths.Target != expected.Target {
		t.Fatalf("working directory redirected installation: %#v, want root %s", paths, expected.PalServerRoot)
	}
}

func TestDetectRejectsSymlinkedTarget(t *testing.T) {
	installer, processConfig, paths := prepareInstallerTest(t, true)
	outside := t.TempDir()
	if err := os.Symlink(outside, paths.Target); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	detection := installer.Detect(processConfig)
	if detection.State != BridgeError || !strings.Contains(detection.Message, "符号链接") {
		t.Fatalf("detection = %#v, want unsafe path error", detection)
	}
}

func TestDetectMarksInstalledHashChanges(t *testing.T) {
	installer, processConfig, paths := prepareInstallerTest(t, true)
	transaction, err := installer.Install(processConfig, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.Target, "Scripts", "main.lua"), []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}
	detection := installer.Detect(processConfig)
	if detection.State != BridgeModified || detection.FilesIntact {
		t.Fatalf("detection = %#v, want modified", detection)
	}
}

func TestDetectRejectsDuplicateUE4SSLayout(t *testing.T) {
	installer, processConfig, paths := prepareInstallerTest(t, true)
	legacy := filepath.Join(paths.PalServerRoot, "Pal", "Binaries", "Win64", "ue4ss")
	if err := os.MkdirAll(legacy, 0755); err != nil {
		t.Fatal(err)
	}
	detection := installer.Detect(processConfig)
	if detection.State != BridgeIncompatible {
		t.Fatalf("state = %s, want %s", detection.State, BridgeIncompatible)
	}
}
