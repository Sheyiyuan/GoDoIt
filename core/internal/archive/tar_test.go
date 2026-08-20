package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTarGzRejectsUnsafeEntries(t *testing.T) {
	for _, test := range []struct {
		name     string
		header   tar.Header
		wantFile string
	}{
		{name: "parent", header: tar.Header{Name: "../escape", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1}},
		{name: "deep parent", header: tar.Header{Name: "a/../../escape", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1}},
		{name: "absolute", header: tar.Header{Name: "/escape", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1}},
		{name: "device", header: tar.Header{Name: "device", Typeflag: tar.TypeChar}, wantFile: "empty"},
		{name: "hardlink", header: tar.Header{Name: "hard", Typeflag: tar.TypeLink, Linkname: "target"}, wantFile: "empty"},
		{name: "absolute symlink", header: tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}, wantFile: "empty"},
		{name: "escaping symlink", header: tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../outside"}, wantFile: "empty"},
	} {
		t.Run(test.name, func(t *testing.T) {
			content := "x"
			if test.wantFile == "empty" {
				content = ""
			}
			archivePath := writeTarGz(t, []tar.Header{test.header}, []string{content})
			if err := ExtractTarGz(archivePath, t.TempDir()); err == nil {
				t.Fatal("expected unsafe entry to be rejected")
			}
		})
	}
}

func TestExtractTarGzRejectsWriteThroughEarlierSymlink(t *testing.T) {
	headers := []tar.Header{
		{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "real"},
		{Name: "link/file", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1},
	}
	archivePath := writeTarGz(t, headers, []string{"", "x"})
	if err := ExtractTarGz(archivePath, t.TempDir()); err == nil {
		t.Fatal("expected symlink parent to be rejected")
	}
}

func TestExtractTarGzExtractsRegularFilesAndSafeSymlinks(t *testing.T) {
	headers := []tar.Header{
		{Name: "dotnet", Typeflag: tar.TypeReg, Mode: 0o755, Size: 6},
		{Name: "nested/dir/deep", Typeflag: tar.TypeReg, Mode: 0o644, Size: 4},
		{Name: "dotnet-link", Typeflag: tar.TypeSymlink, Linkname: "dotnet"},
	}
	archivePath := writeTarGz(t, headers, []string{"dotnet", "deep", ""})
	destination := t.TempDir()
	if err := ExtractTarGz(archivePath, destination); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(destination, "dotnet")); err != nil || string(data) != "dotnet" {
		t.Fatalf("unexpected extracted file: %q err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(destination, "nested", "dir", "deep")); err != nil || string(data) != "deep" {
		t.Fatalf("nested extraction failed: %q err=%v", data, err)
	}
}

func TestExtractTarGzRejectsCorruptGzip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.tar.gz")
	if err := os.WriteFile(path, []byte("not a gzip stream"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ExtractTarGz(path, t.TempDir()); err == nil {
		t.Fatal("corrupt gzip stream must be rejected")
	}
}

func writeTarGz(t *testing.T, headers []tar.Header, contents []string) string {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for index := range headers {
		header := headers[index]
		if err := tarWriter.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
		if contents[index] != "" {
			if _, err := tarWriter.Write([]byte(contents[index])); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fixture.tar.gz")
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
