package gdit

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/Sheyiyuan/GoDoIt/core/internal/instance"
)

func TestSetInstanceIconNormalizesJPEGAndSwitchesPreset(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager := doctorManager(t)
	source := filepath.Join(t.TempDir(), "source.jpg")
	writeJPEGFixture(t, source, 480, 240)
	info, err := manager.SetInstanceIcon(context.Background(), "work", SetInstanceIconRequest{Icon: "custom", SourcePath: source, Background: "#11223380"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Icon != "custom" || info.ResolvedIcon != "custom" || info.IconMissing || info.IconBackground != "#11223380" {
		t.Fatalf("unexpected custom icon result: %+v", info)
	}
	file, err := os.Open(instance.IconPath(manager.root, info.ID))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(file)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != instance.CustomIconSize || decoded.Bounds().Dy() != instance.CustomIconSize {
		t.Fatalf("custom icon was not normalized: %v", decoded.Bounds())
	}
	info, err = manager.SetInstanceIcon(context.Background(), "work", SetInstanceIconRequest{Icon: "custom", Background: "#445566"})
	if err != nil {
		t.Fatal(err)
	}
	if info.IconBackground != "#445566" {
		t.Fatalf("background-only custom icon update failed: %+v", info)
	}
	info, err = manager.SetInstanceIcon(context.Background(), "work", SetInstanceIconRequest{Icon: "mascot", Background: "#abcdef"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Icon != "mascot" || info.ResolvedIcon != "mascot" || info.IconBackground != "#abcdef" {
		t.Fatalf("unexpected preset icon result: %+v", info)
	}
	if _, err := os.Stat(instance.IconPath(manager.root, info.ID)); !os.IsNotExist(err) {
		t.Fatalf("switching preset must remove custom file: %v", err)
	}
}

func TestSetInstanceIconRejectsInvalidAndTransparentFiles(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager := doctorManager(t)
	if _, err := manager.SetInstanceIcon(context.Background(), "work", SetInstanceIconRequest{Icon: "godot", Background: "purple"}); err == nil {
		t.Fatal("non-hex icon background must be rejected")
	}
	if _, err := manager.SetInstanceIcon(context.Background(), "work", SetInstanceIconRequest{Icon: "custom", SourcePath: "relative.png"}); err == nil {
		t.Fatal("relative custom source must be rejected")
	}
	broken := filepath.Join(t.TempDir(), "broken.png")
	if err := os.WriteFile(broken, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SetInstanceIcon(context.Background(), "work", SetInstanceIconRequest{Icon: "custom", SourcePath: broken}); err == nil {
		t.Fatal("broken image must be rejected")
	}
	transparent := filepath.Join(t.TempDir(), "transparent.png")
	file, err := os.Create(transparent)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewNRGBA(image.Rect(0, 0, 64, 64))); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if _, err := manager.SetInstanceIcon(context.Background(), "work", SetInstanceIconRequest{Icon: "custom", SourcePath: transparent}); err == nil {
		t.Fatal("fully transparent image must be rejected")
	}
}

func TestMissingCustomIconFallsBackAndDoctorWarns(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager := doctorManager(t)
	item, err := instance.Lookup(manager.root, "work")
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.SetAppearance(manager.root, item.ID, instance.IconCustom, ""); err != nil {
		t.Fatal(err)
	}
	items, err := manager.Instances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].IconMissing || items[0].ResolvedIcon != "godot" {
		t.Fatalf("missing custom icon did not fall back: %+v", items)
	}
	report, err := manager.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if item := doctorItem(report, "icons"); item == nil || item.Status != StatusWarn {
		t.Fatalf("doctor must warn about missing custom icon: %+v", item)
	}
}

func writeJPEGFixture(t *testing.T, path string, width, height int) {
	t.Helper()
	fixture := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			fixture.SetNRGBA(x, y, color.NRGBA{R: uint8(x % 255), G: uint8(y % 255), B: 140, A: 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := jpeg.Encode(file, fixture, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
}
