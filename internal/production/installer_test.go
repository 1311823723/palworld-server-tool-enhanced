package production

import (
	"encoding/json"
	"errors"
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
		"Info.json": `{
			"ModName":"PST Production Bridge",
			"PackageName":"PSTProductionBridge",
			"Version":"` + BridgeVersion + `",
			"InstallRule":[{"Type":"Lua","IsServer":true,"Targets":["./Scripts"]}]
		}`,
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

func TestBundledPackageRequiresOfficialServerLuaRule(t *testing.T) {
	_, _, paths := prepareInstallerTest(t, true)
	infoPath := filepath.Join(paths.Source, "Info.json")
	if err := os.WriteFile(infoPath, []byte(`{
		"PackageName":"PSTProductionBridge",
		"Version":"`+BridgeVersion+`",
		"InstallRule":[{"Type":"Lua","Targets":["./Scripts"]}]
	}`), 0644); err != nil {
		t.Fatal(err)
	}
	manifest, err := readManifest(paths.Source)
	if err != nil {
		t.Fatal(err)
	}
	for index := range manifest.Files {
		if manifest.Files[index].Path == "Info.json" {
			manifest.Files[index].SHA256, err = fileSHA256(infoPath)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.Source, "manifest.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	_, err = readAndVerifyManifest(paths.Source)
	if err == nil || !strings.Contains(err.Error(), "Lua InstallRule") {
		t.Fatalf("invalid server package error = %v", err)
	}
}

func TestRepositoryBundledBridgePackageIsValid(t *testing.T) {
	root := filepath.Join("..", "..", "extras", BridgeName)
	manifest, err := readAndVerifyManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != BridgeVersion {
		t.Fatalf("bundled version = %q, want %q", manifest.Version, BridgeVersion)
	}
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

func TestEnableModAddsOneDedicatedEntryForRepeatedModLists(t *testing.T) {
	original := strings.Join([]string{
		"[PalModSettings]",
		"bGlobalEnableMod=true",
		"ActiveModList=CreativeMenu",
		"ActiveModList=PaldarMinimap",
		"ActiveModList=NoBuildingLimits100",
		"UnknownSetting=42",
		"",
	}, "\r\n")

	updated := enableMod(original, true)
	if count := bridgeActivationCount(updated); count != 1 {
		t.Fatalf("Bridge activation count = %d, want 1:\n%s", count, updated)
	}
	for _, expected := range []string{
		"ActiveModList=CreativeMenu",
		"ActiveModList=PaldarMinimap",
		"ActiveModList=NoBuildingLimits100",
		"ActiveModList=" + BridgeName,
		"UnknownSetting=42",
	} {
		if !strings.Contains(updated, expected) {
			t.Fatalf("updated INI is missing %q:\n%s", expected, updated)
		}
	}
	if strings.Contains(updated, "CreativeMenu,"+BridgeName) {
		t.Fatalf("Bridge must not be appended to an existing mod entry:\n%s", updated)
	}
}

func TestEnableModRepairsPollutedRepeatedModLists(t *testing.T) {
	polluted := strings.Join([]string{
		"[PalModSettings]",
		"bGlobalEnableMod=true",
		"ActiveModList=CreativeMenu," + BridgeName,
		"ActiveModList=PaldarMinimap," + BridgeName,
		"ActiveModList=" + BridgeName,
		"",
	}, "\r\n")

	repaired := enableMod(polluted, true)
	if count := bridgeActivationCount(repaired); count != 1 {
		t.Fatalf("Bridge activation count after repair = %d, want 1:\n%s", count, repaired)
	}
	for _, expected := range []string{"ActiveModList=CreativeMenu", "ActiveModList=PaldarMinimap"} {
		if !strings.Contains(repaired, expected) {
			t.Fatalf("repair removed existing mod %q:\n%s", expected, repaired)
		}
	}

	disabled := enableMod(repaired, false)
	if count := bridgeActivationCount(disabled); count != 0 {
		t.Fatalf("Bridge activation count after disable = %d, want 0:\n%s", count, disabled)
	}
	for _, expected := range []string{"ActiveModList=CreativeMenu", "ActiveModList=PaldarMinimap"} {
		if !strings.Contains(disabled, expected) {
			t.Fatalf("disable removed existing mod %q:\n%s", expected, disabled)
		}
	}
}

func TestDetectAndRepairDuplicateBridgeActivation(t *testing.T) {
	installer, processConfig, paths := prepareInstallerTest(t, true)
	transaction, err := installer.Install(processConfig, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	polluted := "bGlobalEnableMod=true\r\nActiveModList=ExistingMod," + BridgeName +
		"\r\nActiveModList=AnotherMod," + BridgeName + "\r\n"
	if err := os.WriteFile(paths.Settings, []byte(polluted), 0644); err != nil {
		t.Fatal(err)
	}

	detection := installer.Detect(processConfig)
	if detection.State != BridgeModified || !strings.Contains(detection.Message, "重复") {
		t.Fatalf("detection = %#v, want duplicate activation repair state", detection)
	}

	repair, err := installer.Install(processConfig, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := repair.Commit(); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(paths.Settings)
	if err != nil {
		t.Fatal(err)
	}
	if count := bridgeActivationCount(string(updated)); count != 1 {
		t.Fatalf("Bridge activation count after installer repair = %d, want 1:\n%s", count, updated)
	}
	for _, expected := range []string{"ActiveModList=ExistingMod", "ActiveModList=AnotherMod"} {
		if !strings.Contains(string(updated), expected) {
			t.Fatalf("repair removed existing mod %q:\n%s", expected, updated)
		}
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

func TestInstallUsesWorkshopRootDirFromPalModSettings(t *testing.T) {
	installer, processConfig, defaultPaths := prepareInstallerTest(t, true)
	configuredRoot := filepath.Join(defaultPaths.PalServerRoot, "Mods", "1623730")
	if err := os.MkdirAll(filepath.Dir(defaultPaths.Settings), 0755); err != nil {
		t.Fatal(err)
	}
	settings := strings.Join([]string{
		"[PalModSettings]",
		"WorkshopRootDir=\"" + configuredRoot + "\"",
		"bGlobalEnableMod=true",
		"ActiveModList=ExistingMod",
		"",
	}, "\r\n")
	if err := os.WriteFile(defaultPaths.Settings, []byte(settings), 0644); err != nil {
		t.Fatal(err)
	}

	paths, err := installer.paths(processConfig)
	if err != nil {
		t.Fatal(err)
	}
	wantTarget := filepath.Join(configuredRoot, BridgeName)
	if paths.Target != wantTarget {
		t.Fatalf("target = %q, want %q", paths.Target, wantTarget)
	}

	transaction, err := installer.Install(processConfig, false)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wantTarget, "Info.json")); err != nil {
		t.Fatalf("Bridge was not installed under WorkshopRootDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(defaultPaths.Target, "Info.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("default Workshop directory should remain unused, err=%v", err)
	}
	updated, err := os.ReadFile(defaultPaths.Settings)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "WorkshopRootDir=\""+configuredRoot+"\"") {
		t.Fatalf("installer changed WorkshopRootDir:\n%s", updated)
	}
}

func TestWorkshopRootDirSupportsRelativeModsPath(t *testing.T) {
	installer, processConfig, paths := prepareInstallerTest(t, true)
	if err := os.MkdirAll(filepath.Dir(paths.Settings), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Settings, []byte("WorkshopRootDir=Mods/CustomWorkshop\n"), 0644); err != nil {
		t.Fatal(err)
	}
	resolved, err := installer.paths(processConfig)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(paths.PalServerRoot, "Mods", "CustomWorkshop", BridgeName)
	if resolved.Target != want {
		t.Fatalf("target = %q, want %q", resolved.Target, want)
	}
}

func TestWorkshopRootDirCannotEscapePalServerMods(t *testing.T) {
	installer, processConfig, paths := prepareInstallerTest(t, true)
	if err := os.MkdirAll(filepath.Dir(paths.Settings), 0755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(paths.PalServerRoot), "outside")
	if err := os.WriteFile(paths.Settings, []byte("WorkshopRootDir="+outside+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	detection := installer.Detect(processConfig)
	if detection.State != BridgeError || !strings.Contains(detection.Message, "必须位于") {
		t.Fatalf("detection = %#v, want safe path error", detection)
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
