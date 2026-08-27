package instance

import (
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
)

// CustomIconSize 是规范化自定义图标的固定边长。
const CustomIconSize = 256

// IconPath 返回条目自定义图标的规范路径；调用方应先校验 id。
func IconPath(root, id string) string {
	return filepath.Join(root, "icons", id+".png")
}

// InspectIcon 校验自定义图标存在、不是链接，并且是 256x256 PNG。
func InspectIcon(root, id string) error {
	if !ValidID(id) {
		return fmt.Errorf("invalid instance id %q", id)
	}
	path := IconPath(root, id)
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("custom icon is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoded, err := png.Decode(file)
	if err != nil {
		return fmt.Errorf("decode custom icon: %w", err)
	}
	if decoded.Bounds().Dx() != CustomIconSize || decoded.Bounds().Dy() != CustomIconSize {
		return fmt.Errorf("custom icon must be %dx%d PNG", CustomIconSize, CustomIconSize)
	}
	return nil
}
