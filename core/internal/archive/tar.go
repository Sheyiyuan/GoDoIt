package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExtractTarGz 将 tar.gz 内容解压到目标目录，并拒绝路径逃逸、设备文件和逃逸 symlink。
func ExtractTarGz(filename, destination string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("open tar.gz: %w", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer gzipReader.Close()
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("create extraction directory: %w", err)
	}
	reader := tar.NewReader(gzipReader)
	for {
		header, nextErr := reader.Next()
		if nextErr == io.EOF {
			return nil
		}
		if nextErr != nil {
			return fmt.Errorf("read tar entry: %w", nextErr)
		}
		if err := extractTarEntry(reader, header, destination); err != nil {
			return err
		}
	}
}

func extractTarEntry(reader io.Reader, header *tar.Header, destination string) error {
	name, err := cleanArchiveEntryPath(header.Name)
	if err != nil {
		return err
	}
	if name == "." {
		// "./" 这类当前目录条目（微软 dotnet-sdk tar 包存在）安全且无内容，跳过。
		return nil
	}
	target := filepath.Join(destination, name)
	if !within(destination, target) {
		return fmt.Errorf("archive path escapes destination %q", header.Name)
	}
	if err := ensureNoSymlinkParents(destination, target); err != nil {
		return fmt.Errorf("unsafe archive path %q: %w", header.Name, err)
	}
	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, 0o755)
	case tar.TypeReg, tar.TypeRegA:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create archive parent: %w", err)
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.FileMode(header.Mode)&0o777)
		if err != nil {
			return fmt.Errorf("create archive file: %w", err)
		}
		_, copyErr := io.Copy(file, reader)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("write archive file: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close archive file: %w", closeErr)
		}
		return nil
	case tar.TypeSymlink:
		link, linkErr := cleanArchiveLinkPath(header.Linkname)
		if linkErr != nil || !within(destination, filepath.Join(filepath.Dir(target), link)) {
			return fmt.Errorf("archive symlink escapes destination %q", header.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create archive parent: %w", err)
		}
		if err := os.Symlink(link, target); err != nil {
			return fmt.Errorf("create archive symlink: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported archive entry %q", header.Name)
	}
}

func ensureNoSymlinkParents(root, target string) error {
	relative, err := filepath.Rel(root, filepath.Dir(target))
	if err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("parent %q is a symlink", current)
		}
	}
	return nil
}
