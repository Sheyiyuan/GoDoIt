//go:build linux || darwin

package archive

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTarGzExtractsSafeSymlink(t *testing.T) {
	headers := []tar.Header{
		{Name: "dotnet", Typeflag: tar.TypeReg, Mode: 0o755, Size: 6},
		{Name: "dotnet-link", Typeflag: tar.TypeSymlink, Linkname: "dotnet"},
	}
	archivePath := writeTarGz(t, headers, []string{"dotnet", ""})
	destination := t.TempDir()
	if err := ExtractTarGz(archivePath, destination); err != nil {
		t.Fatal(err)
	}
	link, err := os.Readlink(filepath.Join(destination, "dotnet-link"))
	if err != nil {
		t.Fatal(err)
	}
	if link != "dotnet" {
		t.Fatalf("unexpected symlink target: %q", link)
	}
}
