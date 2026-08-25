package archive

import (
	"fmt"
	pathpkg "path"
	"path/filepath"
	"strings"
)

func cleanArchiveEntryPath(value string) (string, error) {
	cleaned := cleanArchivePath(value)
	if archivePathRooted(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("unsafe archive path %q", value)
	}
	return filepath.FromSlash(cleaned), nil
}

func cleanArchiveLinkPath(value string) (string, error) {
	cleaned := cleanArchivePath(value)
	if archivePathRooted(cleaned) {
		return "", fmt.Errorf("archive symlink escapes destination")
	}
	return filepath.FromSlash(cleaned), nil
}

func cleanArchivePath(value string) string {
	return pathpkg.Clean(strings.ReplaceAll(value, `\`, "/"))
}

func archivePathRooted(value string) bool {
	if strings.HasPrefix(value, "/") {
		return true
	}
	return len(value) >= 2 && value[1] == ':' &&
		((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z'))
}
