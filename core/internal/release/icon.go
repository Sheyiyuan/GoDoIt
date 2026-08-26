package release

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/png"
	"os"
)

// VerifyIcons 校验统一 PNG 源、Wails PNG 副本与 Windows ICO 尺寸目录。
func VerifyIcons(sourcePNG, wailsPNG, windowsICO string) error {
	source, err := os.ReadFile(sourcePNG)
	if err != nil {
		return err
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(source))
	if err != nil {
		return fmt.Errorf("解析统一图标：%w", err)
	}
	if format != "png" || config.Width != 1024 || config.Height != 1024 || len(source) < 26 || source[25] != 6 {
		return fmt.Errorf("统一图标必须是 1024x1024 RGBA PNG")
	}
	wails, err := os.ReadFile(wailsPNG)
	if err != nil {
		return err
	}
	if !bytes.Equal(source, wails) {
		return fmt.Errorf("Wails appicon.png 不是统一图标的字节级副本")
	}
	ico, err := os.ReadFile(windowsICO)
	if err != nil {
		return err
	}
	if len(ico) < 6 || binary.LittleEndian.Uint16(ico[0:2]) != 0 || binary.LittleEndian.Uint16(ico[2:4]) != 1 {
		return fmt.Errorf("Windows 图标不是有效 ICO")
	}
	count := int(binary.LittleEndian.Uint16(ico[4:6]))
	if count <= 0 || len(ico) < 6+count*16 {
		return fmt.Errorf("Windows ICO 目录不完整")
	}
	sizes := make(map[int]bool)
	for index := 0; index < count; index++ {
		offset := 6 + index*16
		width, height := int(ico[offset]), int(ico[offset+1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		if width == height {
			sizes[width] = true
		}
	}
	for _, size := range []int{16, 32, 48, 64, 128, 256} {
		if !sizes[size] {
			return fmt.Errorf("Windows ICO 缺少 %dx%d 图层", size, size)
		}
	}
	return nil
}
