package production

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/uuid"
	"github.com/zaigie/palworld-server-tool/internal/supervisor"
)

var (
	ErrDependencyMissing  = errors.New("未检测到兼容的 UE4SS，PST 不会自动安装或覆盖 UE4SS")
	ErrBridgeModified     = errors.New("已安装的 Bridge 文件与官方清单不一致，请使用修复功能确认覆盖")
	ErrBridgeIncompatible = errors.New("Production Bridge 与当前 Palworld 或 Mod 环境不兼容")
	ErrPermissionDenied   = errors.New("Production Bridge 安装目录权限不足")
	ErrExternalProcess    = errors.New("PalServer 由外部进程启动，无法安全协调停服和安装")
)

type ManifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Name            string         `json:"name"`
	Version         string         `json:"version"`
	ProtocolVersion int            `json:"protocol_version"`
	Files           []ManifestFile `json:"files"`
}

type InstallPaths struct {
	PalServerRoot string
	Source        string
	Target        string
	Settings      string
	UE4SS         string
	IPC           string
}

type InstallDetection struct {
	State            BridgeState
	Message          string
	Paths            InstallPaths
	Manifest         Manifest
	InstalledVersion string
	FilesIntact      bool
	Automatic        bool
	BlockReason      string
}

type Installer struct {
	GOOS          string
	ExecutableDir func() (string, error)
}

func NewInstaller() *Installer {
	return &Installer{
		GOOS: runtime.GOOS,
		ExecutableDir: func() (string, error) {
			executable, err := os.Executable()
			if err != nil {
				return "", err
			}
			return filepath.Dir(executable), nil
		},
	}
}

func (installer *Installer) paths(processConfig supervisor.ProcessConfig) (InstallPaths, error) {
	if strings.TrimSpace(processConfig.ExecutablePath) == "" {
		return InstallPaths{}, errors.New("尚未配置 PalServer.exe")
	}
	// Installation paths are always derived from the validated executable.
	// WorkingDirectory is a launch setting and must never redirect Mod writes.
	root := filepath.Dir(strings.TrimSpace(processConfig.ExecutablePath))
	root, err := filepath.Abs(root)
	if err != nil {
		return InstallPaths{}, fmt.Errorf("解析 PalServer 根目录: %w", err)
	}
	sourceRoot, err := installer.ExecutableDir()
	if err != nil {
		return InstallPaths{}, fmt.Errorf("解析 PST 目录: %w", err)
	}
	sourceRoot, err = filepath.Abs(filepath.Join(sourceRoot, "extras", BridgeName))
	if err != nil {
		return InstallPaths{}, fmt.Errorf("解析 Bridge 安装包目录: %w", err)
	}
	return InstallPaths{
		PalServerRoot: root,
		Source:        sourceRoot,
		Target:        filepath.Join(root, "Mods", "Workshop", BridgeName),
		Settings:      filepath.Join(root, "Mods", "PalModSettings.ini"),
		UE4SS:         filepath.Join(root, "Mods", "NativeMods", "UE4SS"),
		IPC:           filepath.Join(root, "Pal", "Saved", BridgeName),
	}, nil
}

func (installer *Installer) Detect(processConfig supervisor.ProcessConfig) InstallDetection {
	detection := InstallDetection{State: BridgeError}
	if installer.GOOS != "windows" {
		detection.State = BridgeUnsupported
		detection.Message = "PST Production Bridge 仅支持 Windows 本地 PalServer"
		detection.BlockReason = detection.Message
		return detection
	}
	if !processConfig.Enabled || strings.TrimSpace(processConfig.ExecutablePath) == "" {
		detection.State = BridgeUnconfigured
		detection.Message = "请先在配置中心启用进程管理并配置 PalServer.exe"
		detection.BlockReason = detection.Message
		return detection
	}
	if !strings.EqualFold(filepath.Base(processConfig.ExecutablePath), "PalServer.exe") {
		detection.State = BridgeUnconfigured
		detection.Message = "生产 Bridge 只接受 PalServer.exe 所在的专服目录"
		detection.BlockReason = detection.Message
		return detection
	}
	paths, err := installer.paths(processConfig)
	if err != nil {
		detection.Message = err.Error()
		detection.BlockReason = detection.Message
		return detection
	}
	detection.Paths = paths
	if err := validateInstallPaths(paths); err != nil {
		detection.State = BridgeError
		detection.Message = "PalServer 目录布局不安全：" + err.Error()
		detection.BlockReason = detection.Message
		return detection
	}
	if err := requireRegularFile(processConfig.ExecutablePath); err != nil {
		detection.State = BridgeUnconfigured
		detection.Message = "PalServer.exe 无效：" + err.Error()
		detection.BlockReason = detection.Message
		return detection
	}
	manifest, err := readAndVerifyManifest(paths.Source)
	if err != nil {
		detection.Message = "Release 内的 Bridge 安装包无效：" + err.Error()
		detection.BlockReason = detection.Message
		return detection
	}
	detection.Manifest = manifest
	installedManifest, installedErr := readManifest(paths.Target)
	if installedErr == nil {
		detection.InstalledVersion = installedManifest.Version
	}
	if !detectUE4SS(paths.UE4SS) {
		detection.State = BridgeDependencyMissing
		detection.Message = ErrDependencyMissing.Error()
		detection.BlockReason = detection.Message
		return detection
	}
	if hasLegacyUE4SSConflict(paths.PalServerRoot) {
		detection.State = BridgeIncompatible
		detection.Message = "检测到重复的旧版 UE4SS 目录，请先人工确认仅保留一个加载器"
		detection.BlockReason = detection.Message
		return detection
	}
	if _, err := os.Stat(paths.Target); errors.Is(err, os.ErrNotExist) {
		if err := checkDirectoryWritable(filepath.Dir(paths.Target)); err != nil {
			detection.State = BridgePermissionDenied
			detection.Message = "Bridge 目标目录不可写：" + err.Error()
			detection.BlockReason = detection.Message
			return detection
		}
		detection.State = BridgeNotInstalled
		detection.Message = "尚未安装 PST Production Bridge"
		detection.Automatic = true
		return detection
	}
	intact, err := verifyInstalled(paths.Target, manifest)
	if err != nil || !intact {
		detection.State = BridgeModified
		detection.Message = ErrBridgeModified.Error()
		detection.FilesIntact = false
		detection.Automatic = true
		return detection
	}
	detection.FilesIntact = true
	detection.Automatic = true
	if installedManifest.Version != manifest.Version {
		detection.State = BridgeUpgradeAvailable
		detection.Message = fmt.Sprintf("Bridge 可升级：%s → %s", installedManifest.Version, manifest.Version)
		return detection
	}
	if !modEnabled(paths.Settings) {
		detection.State = BridgeRestartRequired
		detection.Message = "Bridge 文件已安装，但尚未在 PalModSettings.ini 中启用"
		return detection
	}
	detection.State = BridgeOffline
	detection.Message = "Bridge 已安装，等待 PalServer 运行时心跳"
	return detection
}

func (detection InstallDetection) ManualGuide() *ManualInstallGuide {
	if detection.Paths.Source == "" {
		return &ManualInstallGuide{
			Steps:               []string{"先在配置中心设置有效的 PalServer.exe 路径。"},
			AutomaticInstallOK:  false,
			AutomaticBlockCause: detection.BlockReason,
		}
	}
	return &ManualInstallGuide{
		SourceDirectory:     detection.Paths.Source,
		TargetDirectory:     detection.Paths.Target,
		SettingsPath:        detection.Paths.Settings,
		UE4SSDirectory:      detection.Paths.UE4SS,
		AutomaticInstallOK:  detection.Automatic,
		AutomaticBlockCause: detection.BlockReason,
		Steps: []string{
			"平滑保存并停止 PalServer，确认 PalServer 进程已经完全退出。",
			"自行安装与当前 Palworld Build 兼容的 UE4SS；PST 不会安装或覆盖 UE4SS。",
			"将 Bridge 安装包完整复制到目标目录，不要改名或合并到其他 Mod。",
			"在 PalModSettings.ini 中设置 bGlobalEnableMod=true，并把 PSTProductionBridge 加入 ActiveModList。",
			"启动 PalServer，回到生产订单页确认 Bridge 心跳和能力状态。",
		},
	}
}

type FileTransaction struct {
	target        string
	backup        string
	settings      string
	settingsData  []byte
	settingsMode  os.FileMode
	settingsExist bool
	applied       bool
	targetChanged bool
}

func (installer *Installer) Install(processConfig supervisor.ProcessConfig, repair bool) (*FileTransaction, error) {
	detection := installer.Detect(processConfig)
	switch detection.State {
	case BridgeNotInstalled, BridgeUpgradeAvailable, BridgeRestartRequired, BridgeOffline:
	case BridgeModified:
		if !repair {
			return nil, ErrBridgeModified
		}
	default:
		if detection.BlockReason != "" {
			return nil, errors.New(detection.BlockReason)
		}
		return nil, errors.New(detection.Message)
	}
	paths := detection.Paths
	manifest, err := readAndVerifyManifest(paths.Source)
	if err != nil {
		return nil, err
	}
	transaction := &FileTransaction{
		target:   paths.Target,
		settings: paths.Settings,
	}
	if info, statErr := os.Stat(paths.Settings); statErr == nil {
		transaction.settingsExist = true
		transaction.settingsMode = info.Mode().Perm()
		transaction.settingsData, err = os.ReadFile(paths.Settings)
		if err != nil {
			return nil, fmt.Errorf("读取 PalModSettings.ini: %w", err)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("检查 PalModSettings.ini: %w", statErr)
	}

	workshop := filepath.Dir(paths.Target)
	if err := os.MkdirAll(workshop, 0755); err != nil {
		return nil, fmt.Errorf("创建 Workshop 目录: %w", err)
	}
	stage := filepath.Join(workshop, "."+BridgeName+"-stage-"+uuid.NewString())
	if err := copyManifestFiles(paths.Source, stage, manifest); err != nil {
		_ = os.RemoveAll(stage)
		return nil, err
	}
	if _, err := os.Stat(paths.Target); err == nil {
		transaction.backup = filepath.Join(workshop, "."+BridgeName+"-backup-"+uuid.NewString())
		if err := os.Rename(paths.Target, transaction.backup); err != nil {
			_ = os.RemoveAll(stage)
			return nil, fmt.Errorf("备份现有 Bridge: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = os.RemoveAll(stage)
		return nil, fmt.Errorf("检查 Bridge 目标目录: %w", err)
	}
	transaction.applied = true
	transaction.targetChanged = true
	if err := os.Rename(stage, paths.Target); err != nil {
		_ = transaction.Rollback()
		return nil, fmt.Errorf("安装 Bridge: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Settings), 0755); err != nil {
		_ = transaction.Rollback()
		return nil, err
	}
	updated := enableMod(string(transaction.settingsData), true)
	mode := transaction.settingsMode
	if mode == 0 {
		mode = 0644
	}
	if err := atomicWriteFile(paths.Settings, []byte(updated), mode); err != nil {
		_ = transaction.Rollback()
		return nil, fmt.Errorf("更新 PalModSettings.ini: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(paths.IPC, "requests"), 0700); err != nil {
		_ = transaction.Rollback()
		return nil, fmt.Errorf("创建 Bridge 请求目录: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(paths.IPC, "results"), 0700); err != nil {
		_ = transaction.Rollback()
		return nil, fmt.Errorf("创建 Bridge 结果目录: %w", err)
	}
	return transaction, nil
}

func (installer *Installer) Disable(processConfig supervisor.ProcessConfig) (*FileTransaction, error) {
	if installer.GOOS != "windows" {
		return nil, supervisor.ErrUnsupportedPlatform
	}
	paths, err := installer.paths(processConfig)
	if err != nil {
		return nil, err
	}
	transaction := &FileTransaction{target: paths.Target, settings: paths.Settings}
	info, err := os.Stat(paths.Settings)
	if err != nil {
		return nil, fmt.Errorf("读取 PalModSettings.ini: %w", err)
	}
	transaction.settingsExist = true
	transaction.settingsMode = info.Mode().Perm()
	transaction.settingsData, err = os.ReadFile(paths.Settings)
	if err != nil {
		return nil, err
	}
	if err := atomicWriteFile(paths.Settings, []byte(enableMod(string(transaction.settingsData), false)), transaction.settingsMode); err != nil {
		return nil, err
	}
	transaction.applied = true
	return transaction, nil
}

func (transaction *FileTransaction) Commit() error {
	if transaction == nil {
		return nil
	}
	if transaction.backup != "" {
		if err := os.RemoveAll(transaction.backup); err != nil {
			return fmt.Errorf("清理 Bridge 备份目录: %w", err)
		}
		transaction.backup = ""
	}
	return nil
}

func (transaction *FileTransaction) Rollback() error {
	if transaction == nil || !transaction.applied {
		return nil
	}
	var rollbackErrors []string
	if transaction.targetChanged && transaction.target != "" {
		if err := os.RemoveAll(transaction.target); err != nil {
			rollbackErrors = append(rollbackErrors, "移除失败版本: "+err.Error())
		}
		if transaction.backup != "" {
			if err := os.Rename(transaction.backup, transaction.target); err != nil {
				rollbackErrors = append(rollbackErrors, "恢复原 Bridge: "+err.Error())
			}
		}
	}
	if transaction.settingsExist {
		if err := atomicWriteFile(transaction.settings, transaction.settingsData, transaction.settingsMode); err != nil {
			rollbackErrors = append(rollbackErrors, "恢复 PalModSettings.ini: "+err.Error())
		}
	} else if err := os.Remove(transaction.settings); err != nil && !errors.Is(err, os.ErrNotExist) {
		rollbackErrors = append(rollbackErrors, "移除新建 PalModSettings.ini: "+err.Error())
	}
	transaction.applied = false
	if len(rollbackErrors) > 0 {
		return errors.New(strings.Join(rollbackErrors, "；"))
	}
	return nil
}

func readManifest(root string) (Manifest, error) {
	var manifest Manifest
	manifestPath := filepath.Join(root, "manifest.json")
	if err := ensureNoSymlinkPath(root, manifestPath); err != nil {
		return manifest, err
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	if manifest.Name != BridgeName || manifest.Version == "" || manifest.ProtocolVersion != BridgeProtocolVersion || len(manifest.Files) == 0 {
		return manifest, errors.New("Bridge 清单名称、版本或协议无效")
	}
	return manifest, nil
}

func readAndVerifyManifest(root string) (Manifest, error) {
	manifest, err := readManifest(root)
	if err != nil {
		return manifest, err
	}
	for _, file := range manifest.Files {
		path, err := safeChild(root, file.Path)
		if err != nil {
			return manifest, err
		}
		if err := ensureNoSymlinkPath(root, path); err != nil {
			return manifest, fmt.Errorf("%s: %w", file.Path, err)
		}
		if err := requireRegularFile(path); err != nil {
			return manifest, fmt.Errorf("%s: %w", file.Path, err)
		}
		digest, err := fileSHA256(path)
		if err != nil {
			return manifest, err
		}
		if !strings.EqualFold(digest, file.SHA256) {
			return manifest, fmt.Errorf("%s SHA-256 不匹配", file.Path)
		}
	}
	return manifest, nil
}

func verifyInstalled(root string, expected Manifest) (bool, error) {
	if _, err := readManifest(root); err != nil {
		return false, err
	}
	for _, file := range expected.Files {
		path, err := safeChild(root, file.Path)
		if err != nil {
			return false, err
		}
		if err := ensureNoSymlinkPath(root, path); err != nil {
			return false, err
		}
		digest, err := fileSHA256(path)
		if err != nil {
			return false, err
		}
		if !strings.EqualFold(digest, file.SHA256) {
			return false, nil
		}
	}
	return true, nil
}

func copyManifestFiles(source, target string, manifest Manifest) error {
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	for _, item := range manifest.Files {
		sourcePath, err := safeChild(source, item.Path)
		if err != nil {
			return err
		}
		targetPath, err := safeChild(target, item.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		if err := copyRegularFile(sourcePath, targetPath); err != nil {
			return err
		}
	}
	manifestData, err := os.ReadFile(filepath.Join(source, "manifest.json"))
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(target, "manifest.json"), manifestData, 0644)
}

func safeChild(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", errors.New("Bridge 清单包含无效路径")
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("Bridge 清单包含路径穿越")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	result := filepath.Join(rootAbs, clean)
	relativeCheck, err := filepath.Rel(rootAbs, result)
	if err != nil || relativeCheck == ".." || strings.HasPrefix(relativeCheck, ".."+string(filepath.Separator)) {
		return "", errors.New("Bridge 清单路径越界")
	}
	return result, nil
}

func validateInstallPaths(paths InstallPaths) error {
	rootInfo, err := os.Lstat(paths.PalServerRoot)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("PalServer 根目录不是普通目录")
	}
	for _, path := range []string{paths.Target, paths.Settings, paths.UE4SS, paths.IPC} {
		if err := ensureNoSymlinkPath(paths.PalServerRoot, path); err != nil {
			return err
		}
	}
	if err := ensureNoSymlinkPath(paths.Source, paths.Source); err != nil {
		return err
	}
	return nil
}

func ensureNoSymlinkPath(root, path string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("路径超出允许目录")
	}
	current := rootAbs
	parts := []string{}
	if relative != "." {
		parts = strings.Split(relative, string(filepath.Separator))
	}
	for index := -1; index < len(parts); index++ {
		if index >= 0 {
			current = filepath.Join(current, parts[index])
		}
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("路径包含符号链接：%s", current)
		}
	}
	return nil
}

func requireRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("不是普通文件")
	}
	return nil
}

func copyRegularFile(source, target string) error {
	if err := requireRegularFile(source); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func fileSHA256(path string) (string, error) {
	input, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer input.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, input); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func detectUE4SS(path string) bool {
	for _, marker := range []string{"UE4SS.dll", "ue4ss.dll", "UE4SS-settings.ini"} {
		if info, err := os.Stat(filepath.Join(path, marker)); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func hasLegacyUE4SSConflict(root string) bool {
	legacy := filepath.Join(root, "Pal", "Binaries", "Win64", "ue4ss")
	if info, err := os.Stat(legacy); err == nil && info.IsDir() {
		return true
	}
	return false
}

func checkDirectoryWritable(parent string) error {
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	testPath := filepath.Join(parent, ".pst-write-test-"+uuid.NewString())
	file, err := os.OpenFile(testPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if closeErr := file.Close(); closeErr != nil {
		return closeErr
	}
	return os.Remove(testPath)
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func modEnabled(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	global := false
	active := false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.EqualFold(line, "bGlobalEnableMod=true") {
			global = true
		}
		if key, value, ok := strings.Cut(line, "="); ok && strings.EqualFold(strings.TrimSpace(key), "ActiveModList") {
			for _, item := range splitModList(value) {
				if strings.EqualFold(item, BridgeName) {
					active = true
				}
			}
		}
	}
	return global && active
}

func enableMod(content string, enabled bool) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	foundGlobal := false
	foundList := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		switch {
		case strings.EqualFold(strings.TrimSpace(key), "bGlobalEnableMod"):
			foundGlobal = true
			if enabled {
				lines[index] = preserveIndent(line) + "bGlobalEnableMod=true"
			}
		case strings.EqualFold(strings.TrimSpace(key), "ActiveModList"):
			foundList = true
			items := splitModList(value)
			filtered := make([]string, 0, len(items)+1)
			present := false
			for _, item := range items {
				if strings.EqualFold(item, BridgeName) {
					present = true
					if !enabled {
						continue
					}
					item = BridgeName
				}
				filtered = append(filtered, item)
			}
			if enabled && !present {
				filtered = append(filtered, BridgeName)
			}
			lines[index] = preserveIndent(line) + "ActiveModList=" + strings.Join(filtered, ",")
		}
	}
	if enabled && !foundGlobal {
		lines = append(lines, "bGlobalEnableMod=true")
	}
	if !foundList {
		value := ""
		if enabled {
			value = BridgeName
		}
		lines = append(lines, "ActiveModList="+value)
	}
	result := strings.Join(lines, "\r\n")
	if !strings.HasSuffix(result, "\r\n") {
		result += "\r\n"
	}
	return result
}

func splitModList(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';'
	})
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if item := strings.TrimSpace(field); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func preserveIndent(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}
