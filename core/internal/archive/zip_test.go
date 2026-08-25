package archive

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractZipRejectsUnsafeEntries(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "parent", path: "../escape"},
		{name: "absolute", path: "/escape"},
		{name: "backslash absolute", path: `\escape`},
		{name: "windows drive", path: `C:\escape`},
		{name: "backslash parent", path: `..\escape`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archivePath := writeZip(t, func(writer *zip.Writer) error {
				entry, err := writer.Create(test.path)
				if err != nil {
					return err
				}
				_, err = entry.Write([]byte("escape"))
				return err
			})
			destination := t.TempDir()
			if err := ExtractZip(archivePath, destination); err == nil {
				t.Fatal("expected unsafe archive path error")
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(destination), "escape")); !os.IsNotExist(err) {
				t.Fatalf("archive escaped destination: %v", err)
			}
		})
	}
}

func TestExtractZipRejectsSymlinkEntry(t *testing.T) {
	archivePath := writeZip(t, func(writer *zip.Writer) error {
		header := &zip.FileHeader{Name: "link", Method: zip.Store}
		header.SetMode(os.ModeSymlink | 0o777)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = entry.Write([]byte("/tmp/outside"))
		return err
	})
	if err := ExtractZip(archivePath, t.TempDir()); err == nil {
		t.Fatal("expected symlink archive entry error")
	}
}

func TestExtractZipRejectsNamedPipeEntry(t *testing.T) {
	archivePath := writeZip(t, func(writer *zip.Writer) error {
		header := &zip.FileHeader{Name: "pipe", Method: zip.Store}
		header.SetMode(os.ModeNamedPipe | 0o600)
		_, err := writer.CreateHeader(header)
		return err
	})
	if err := ExtractZip(archivePath, t.TempDir()); err == nil {
		t.Fatal("expected named pipe archive entry error")
	}
}

func TestExtractZipPreservesRegularFile(t *testing.T) {
	archivePath := writeZip(t, func(writer *zip.Writer) error {
		entry, err := writer.Create("nested/engine")
		if err != nil {
			return err
		}
		_, err = entry.Write([]byte("engine"))
		return err
	})
	destination := t.TempDir()
	if err := ExtractZip(archivePath, destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "nested", "engine"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "engine" {
		t.Fatalf("unexpected extracted content: %q", data)
	}
}

func writeZip(t *testing.T, write func(*zip.Writer) error) string {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	if err := write(writer); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fixture.zip")
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractZipRejectsDotDotAfterCleaning(t *testing.T) {
	archivePath := writeZip(t, func(writer *zip.Writer) error {
		entry, err := writer.Create("nested/../../escape")
		if err != nil {
			return err
		}
		_, err = entry.Write([]byte("escape"))
		return err
	})
	if err := ExtractZip(archivePath, t.TempDir()); err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("expected cleaned path rejection, got %v", err)
	}
}
