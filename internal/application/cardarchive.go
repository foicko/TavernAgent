package application

import (
	"archive/zip"
	"errors"
	"io"
	"strings"
)

// 角色卡归档（charx / tacard）共用的边界与读取工具。
//
// 此前这些常量与读取器散落在 charx.go，却被 tacard / cardlibrary / archive 共用，
// 名字（maxCharx*）也掩盖了它们其实面向所有 ZIP 容器。集中到本文件后：
//   - 上限只定义一次，三个容器不可能各自漂移；
//   - 条目表构建（归一化路径）与条目数上限只实现一次。
//
// 注意：这里**不做** PK 魔数硬校验。charx 明确支持 JPEG/ZIP polyglot（首字节不是
// PK\x03\x04，只有尾部中央目录能揭示归档本质），所以正确性依赖 zip.OpenReader
// 的目录扫描；加魔数门槛会把合法的 polyglot 卡误判为非归档。
const (
	// maxCardArchiveCardBytes 是卡清单（card.json / tacard.json）解压后的上限。
	maxCardArchiveCardBytes = 8 << 20
	// maxCardArchiveEntryBytes 是单个资源解压后的上限。
	maxCardArchiveEntryBytes = 64 << 20
	// maxCardArchiveTotalBytes 是全部资源解压后的总量上限。
	maxCardArchiveTotalBytes = 2 << 30
	// maxCardArchiveEntries 是归档条目数上限：防止"海量小条目"耗尽 CPU/句柄。
	maxCardArchiveEntries = 8192
)

var (
	// errCardArchiveEntryTooLarge 表示归档条目解压后超过单项上限。
	errCardArchiveEntryTooLarge = errors.New("条目解压后超过单项上限")
	// errCardArchiveTooManyEntries 表示归档条目数超过上限。
	errCardArchiveTooManyEntries = errors.New("归档条目数超过上限")
)

// archiveEntryPath 归一化归档内路径：统一分隔符、去掉 "./" 前缀。
func archiveEntryPath(name string) string {
	return strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "./")
}

// cardArchiveEntries 把归档整理成「归一化路径 → 条目」的映射，并统一施加条目数上限。
func cardArchiveEntries(zr *zip.Reader) (map[string]*zip.File, error) {
	if err := checkArchiveEntryCount(zr); err != nil {
		return nil, err
	}
	byPath := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		byPath[archiveEntryPath(f.Name)] = f
	}
	return byPath, nil
}

// checkArchiveEntryCount 在读取中央目录后立即施加条目数上限：一个几百字节的
// ZIP 可以声明上万个小条目，逐个打开会耗尽句柄与 CPU，必须在解压前拒绝。
func checkArchiveEntryCount(zr *zip.Reader) error {
	if len(zr.File) > maxCardArchiveEntries {
		return errCardArchiveTooManyEntries
	}
	return nil
}

// readZipEntry 读取一个归档条目，最多 limit 字节（防止压缩炸弹）。
// 超限直接报错而不是静默截断：截断的图片/JSON 只会在后续解码时以更费解的方式失败。
// archive/zip 在读取时会校验实际字节数与头部声明一致，所以按声明大小预检是可靠的。
func readZipEntry(f *zip.File, limit int64) ([]byte, error) {
	// limit 为负时 uint64(limit) 会变成巨大值、令比较恒假——显式挡掉（gosec G115）。
	if limit < 0 || f.UncompressedSize64 > uint64(limit) {
		return nil, errCardArchiveEntryTooLarge
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, limit))
}
