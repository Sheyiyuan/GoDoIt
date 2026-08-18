// Package archive 提供限制路径逃逸的 ZIP 解压能力。
package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const defaultExtractedFileMode = 0o644

// ExtractZip 将 ZIP 内容解压到目标目录，并拒绝危险条目。
func ExtractZip(filename, destination string) error {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer reader.Close()
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("create extraction directory: %w", err)
	}
	for _, entry := range reader.File {
		if err := extractEntry(entry, destination); err != nil {
			return err
		}
	}
	return nil
}

func extractEntry(entry *zip.File, destination string) error {
	name := filepath.Clean(filepath.FromSlash(entry.Name))
	if name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
		return fmt.Errorf("unsafe archive path %q", entry.Name)
	}
	if entry.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsupported archive entry %q", entry.Name)
	}
	target := filepath.Join(destination, name)
	if !within(destination, target) {
		return fmt.Errorf("archive path escapes destination %q", entry.Name)
	}
	if entry.FileInfo().IsDir() {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("create archive directory: %w", err)
		}
		return nil
	}
	if !entry.Mode().IsRegular() {
		return fmt.Errorf("unsupported archive entry %q", entry.Name)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create archive parent: %w", err)
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create archive file: %w", err)
	}
	entryReader, err := entry.Open()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("open archive entry: %w", err)
	}
	if _, err := io.Copy(file, entryReader); err != nil {
		_ = entryReader.Close()
		_ = file.Close()
		return fmt.Errorf("write archive file: %w", err)
	}
	if err := entryReader.Close(); err != nil {
		_ = file.Close()
		return fmt.Errorf("close archive entry: %w", err)
	}
	mode := entry.Mode().Perm()
	if mode == 0 {
		mode = defaultExtractedFileMode
	}
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return fmt.Errorf("set archive mode: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close archive file: %w", err)
	}
	return nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
