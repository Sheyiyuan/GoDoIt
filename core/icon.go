package gdit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"github.com/Sheyiyuan/GoDoIt/core/internal/instance"
	"github.com/Sheyiyuan/GoDoIt/core/internal/lock"
	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
	"github.com/Sheyiyuan/GoDoIt/core/internal/store"
)

const (
	maxCustomIconBytes  = 10 * 1024 * 1024
	maxCustomIconPixels = 32 * 1024 * 1024
)

// SetInstanceIcon 原子更新条目的图标策略。custom 会读取 PNG/JPEG 源文件，居中裁切并
// 规范化为 256x256 PNG；源文件不被修改。非 custom 策略会清理既有自定义图标。
func (m *Manager) SetInstanceIcon(ctx context.Context, name string, request SetInstanceIconRequest) (InstanceInfo, error) {
	if err := instance.ValidateName(name); err != nil {
		return InstanceInfo{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := validateIconRequest(request); err != nil {
		return InstanceInfo{}, err
	}
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return InstanceInfo{}, localIOError("create store root", err)
	}
	guard, err := lock.Acquire(ctx, store.New(m.root).LockPath())
	if err != nil {
		return InstanceInfo{}, contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	item, err := instance.Lookup(m.root, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return InstanceInfo{}, fmt.Errorf("%w: %s", ErrInstanceNotFound, name)
		}
		return InstanceInfo{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if request.Icon == instance.IconCustom {
		if request.SourcePath != "" {
			if err := m.publishCustomIcon(ctx, item.ID, request.SourcePath); err != nil {
				return InstanceInfo{}, err
			}
		} else if err := instance.InspectIcon(m.root, item.ID); err != nil {
			return InstanceInfo{}, fmt.Errorf("%w: custom icon source_path is required when no imported icon exists", ErrInvalidInput)
		}
	}
	if err := instance.SetAppearance(m.root, item.ID, request.Icon, request.Background); err != nil {
		return InstanceInfo{}, localIOError("update instance icon", err)
	}
	if request.Icon != instance.IconCustom {
		if err := removeCustomIcon(m.root, item.ID); err != nil {
			return InstanceInfo{}, localIOError("remove custom icon", err)
		}
	}
	item, err = instance.Lookup(m.root, name)
	if err != nil {
		return InstanceInfo{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	currentID, currentErr := store.New(m.root).ReadCurrent()
	if currentErr != nil && !errors.Is(currentErr, store.ErrNoCurrent) {
		return InstanceInfo{}, localIOError("read current instance", currentErr)
	}
	return instanceToPublic(m.root, item, item.ID == currentID), nil
}

func validateIconRequest(request SetInstanceIconRequest) error {
	if !instance.ValidIconBackground(request.Background) {
		return fmt.Errorf("%w: background must be empty, #RRGGBB or #RRGGBBAA", ErrInvalidInput)
	}
	switch request.Icon {
	case instance.IconDefault, instance.IconGodot, instance.IconCSharp, instance.IconMascot:
		if request.SourcePath != "" {
			return fmt.Errorf("%w: source_path is only valid for a custom icon", ErrInvalidInput)
		}
	case instance.IconCustom:
		if request.SourcePath != "" && !filepath.IsAbs(request.SourcePath) {
			return fmt.Errorf("%w: custom icon source_path must be absolute", ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: unsupported icon strategy %q", ErrInvalidInput, request.Icon)
	}
	return nil
}

func (m *Manager) publishCustomIcon(ctx context.Context, id, sourcePath string) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return localIOError("open custom icon", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return localIOError("inspect custom icon", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxCustomIconBytes {
		return fmt.Errorf("%w: custom icon must be a regular PNG/JPEG no larger than 10 MiB", ErrInvalidInput)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxCustomIconBytes+1))
	if err != nil {
		return localIOError("read custom icon", err)
	}
	if len(data) == 0 || len(data) > maxCustomIconBytes {
		return fmt.Errorf("%w: custom icon is empty or exceeds 10 MiB", ErrInvalidInput)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") {
		return fmt.Errorf("%w: custom icon must be a valid PNG or JPEG", ErrInvalidInput)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width*config.Height > maxCustomIconPixels {
		return fmt.Errorf("%w: custom icon dimensions are too large", ErrInvalidInput)
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%w: decode custom icon: %v", ErrInvalidInput, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if imageFullyTransparent(ctx, decoded) {
		if err := ctx.Err(); err != nil {
			return err
		}
		return fmt.Errorf("%w: custom icon is fully transparent", ErrInvalidInput)
	}
	resized := centerCropNearest(decoded, instance.CustomIconSize)
	iconsDir := filepath.Join(m.root, "icons")
	if err := os.MkdirAll(iconsDir, 0o700); err != nil {
		return localIOError("create icons directory", err)
	}
	temporary, err := os.CreateTemp(iconsDir, ".icon-*.png")
	if err != nil {
		return localIOError("create custom icon staging", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return localIOError("set custom icon permissions", err)
	}
	if err := png.Encode(temporary, resized); err != nil {
		return localIOError("encode custom icon", err)
	}
	if err := temporary.Sync(); err != nil {
		return localIOError("sync custom icon", err)
	}
	if err := temporary.Close(); err != nil {
		return localIOError("close custom icon", err)
	}
	if err := platform.RenameAtomic(temporaryPath, instance.IconPath(m.root, id)); err != nil {
		return localIOError("publish custom icon", err)
	}
	removeTemporary = false
	if err := platform.SyncDir(iconsDir); err != nil {
		return localIOError("sync icons directory", err)
	}
	return nil
}

func imageFullyTransparent(ctx context.Context, source image.Image) bool {
	bounds := source.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if y%64 == 0 && ctx.Err() != nil {
			return true
		}
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := source.At(x, y).RGBA()
			if alpha != 0 {
				return false
			}
		}
	}
	return true
}

func centerCropNearest(source image.Image, size int) *image.NRGBA {
	bounds := source.Bounds()
	side := bounds.Dx()
	if bounds.Dy() < side {
		side = bounds.Dy()
	}
	left := bounds.Min.X + (bounds.Dx()-side)/2
	top := bounds.Min.Y + (bounds.Dy()-side)/2
	result := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		sourceY := top + y*side/size
		for x := 0; x < size; x++ {
			sourceX := left + x*side/size
			result.SetNRGBA(x, y, color.NRGBAModel.Convert(source.At(sourceX, sourceY)).(color.NRGBA))
		}
	}
	return result
}

func removeCustomIcon(root, id string) error {
	path := instance.IconPath(root, id)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(filepath.Dir(path)); err == nil {
		return platform.SyncDir(filepath.Dir(path))
	}
	return nil
}
