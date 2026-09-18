// Command winresgen 生成 Windows 资源对象：`rsrc_windows_<arch>.syso`。
//
// 里面含应用图标、DPI 感知清单与 VERSIONINFO（文件属性里的版本/产品名）。
// Go 构建会自动拾取包目录下的 `*_windows_<arch>.syso`，无需 `wails build` 或 rc.exe。
//
// 为什么不直接用 `wails build`：本项目的前端由 Go 自己的资源服务承载（见 docs/DESKTOP.md），
// 不走 Wails 的资源流水线；但 exe 仍需要图标与清单，所以只借 winres 生成这一份资源。
//
// 用法：
//
//	go run ./cmd/winresgen -version tavernagent/0.1.0
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

const (
	langEnglish = 0x0409
	fileDesc    = "SillyDog / TavernAgent Desktop"
)

func main() {
	iconPath := flag.String("icon", filepath.Join("packaging", "windows", "icon.ico"), "应用图标（.ico）")
	manifestPath := flag.String("manifest", filepath.Join("packaging", "windows", "app.manifest"), "应用清单（XML）")
	outDir := flag.String("out", filepath.Join("cmd", "tavernagent-desktop"), "输出目录（.syso 会写到 Go 包目录下）")
	rawVersion := flag.String("version", "tavernagent/dev", "版本号，形如 tavernagent/1.2.3")
	product := flag.String("product", "SillyDog / TavernAgent", "产品名（文件属性）")
	company := flag.String("company", "TavernAgent", "公司/作者（文件属性）")
	archs := flag.String("arch", "amd64,arm64", "目标架构，逗号分隔")
	flag.Parse()

	if err := run(*iconPath, *manifestPath, *outDir, *rawVersion, *product, *company, *archs); err != nil {
		fmt.Fprintln(os.Stderr, "winresgen:", err)
		os.Exit(1)
	}
}

func run(iconPath, manifestPath, outDir, rawVersion, product, company, archs string) error {
	iconFile, err := os.Open(iconPath)
	if err != nil {
		return fmt.Errorf("打开图标失败: %w", err)
	}
	defer iconFile.Close()
	icon, err := winres.LoadICO(iconFile)
	if err != nil {
		return fmt.Errorf("解析 %s 失败: %w", iconPath, err)
	}

	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("读取清单失败: %w", err)
	}
	manifest, err := winres.AppManifestFromXML(manifestData)
	if err != nil {
		return fmt.Errorf("解析清单失败: %w", err)
	}

	// Wails 的约定：SetIcon(winres.RT_ICON, ...) —— 第一个图标组即资源管理器里显示的应用图标。
	resources := winres.ResourceSet{}
	if err := resources.SetIcon(winres.RT_ICON, icon); err != nil {
		return fmt.Errorf("写入图标资源失败: %w", err)
	}
	resources.SetManifest(manifest)

	info := version.Info{
		FileVersion:    parseVersion(rawVersion),
		ProductVersion: parseVersion(rawVersion),
		Timestamp:      time.Now(),
	}
	for key, value := range map[string]string{
		"ProductName":      product,
		"FileDescription":  fileDesc,
		"CompanyName":      company,
		"OriginalFilename": "tavernagent-desktop.exe",
		"FileVersion":      displayVersion(rawVersion),
		"ProductVersion":   displayVersion(rawVersion),
		"LegalCopyright":   "MIT License",
	} {
		if err := info.Set(langEnglish, key, value); err != nil {
			return fmt.Errorf("写入版本信息 %s 失败: %w", key, err)
		}
	}
	resources.SetVersionInfo(info)

	for _, archName := range strings.Split(archs, ",") {
		archName = strings.TrimSpace(archName)
		if archName == "" {
			continue
		}
		arch, ok := map[string]winres.Arch{"amd64": winres.ArchAMD64, "arm64": winres.ArchARM64, "386": winres.ArchI386}[archName]
		if !ok {
			return fmt.Errorf("不支持的架构 %q（可用 amd64/arm64/386）", archName)
		}
		target := filepath.Join(outDir, "rsrc_windows_"+archName+".syso")
		if err := writeObject(resources, target, arch); err != nil {
			return err
		}
		fmt.Println("winresgen: 已生成", target)
	}
	return nil
}

func writeObject(resources winres.ResourceSet, target string, arch winres.Arch) error {
	file, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("创建 %s 失败: %w", target, err)
	}
	defer file.Close()
	if err := resources.WriteObject(file, arch); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", target, err)
	}
	return nil
}

// parseVersion 把 "tavernagent/1.2.3" 解析成 Windows 需要的四段数字；非数字（如 dev）全为 0。
func parseVersion(raw string) [4]uint16 {
	out := [4]uint16{}
	trimmed := raw
	if idx := strings.LastIndex(raw, "/"); idx >= 0 {
		trimmed = raw[idx+1:]
	}
	for i, part := range strings.Split(trimmed, ".") {
		if i >= len(out) {
			break
		}
		value, err := strconv.ParseUint(strings.TrimSpace(part), 10, 16)
		if err != nil {
			continue
		}
		out[i] = uint16(value)
	}
	return out
}

// displayVersion 保留原始串（版本号可能是 dev 这类非数字），用于文件属性里的可读展示。
func displayVersion(raw string) string {
	if idx := strings.LastIndex(raw, "/"); idx >= 0 {
		return raw[idx+1:]
	}
	return raw
}
