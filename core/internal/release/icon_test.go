package release

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyIcons(t *testing.T) {
	root := t.TempDir()
	pngPath := filepath.Join(root, "icon.png")
	file, err := os.Create(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(root, "appicon.png")
	if err := os.WriteFile(copyPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	icoPath := filepath.Join(root, "icon.ico")
	if err := os.WriteFile(icoPath, fixtureICO(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyIcons(pngPath, copyPath, icoPath); err != nil {
		t.Fatal(err)
	}
	imageData := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	imageData.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	changed, err := os.Create(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(changed, imageData); err != nil {
		t.Fatal(err)
	}
	if err := changed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := VerifyIcons(pngPath, copyPath, icoPath); err == nil {
		t.Fatal("不一致 Wails PNG 未被拒绝")
	}
}

func fixtureICO() []byte {
	sizes := []int{16, 32, 48, 64, 128, 256}
	buffer := bytes.NewBuffer(nil)
	_ = binary.Write(buffer, binary.LittleEndian, uint16(0))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(1))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(len(sizes)))
	for _, size := range sizes {
		width := byte(size)
		if size == 256 {
			width = 0
		}
		buffer.WriteByte(width)
		buffer.WriteByte(width)
		buffer.Write(make([]byte, 14))
	}
	return buffer.Bytes()
}
