package main

import (
	"bytes"
	"strings"
	"testing"

	gdit "github.com/Sheyiyuan/GoDoIt/core"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KiB"},
		{524930, "512.63 KiB"},
		{1024 * 1024, "1.00 MiB"},
		{6_930_000, "6.61 MiB"},
		{1024 * 1024 * 1024, "1.00 GiB"},
	}
	for _, test := range tests {
		if got := formatBytes(test.bytes); got != test.want {
			t.Fatalf("formatBytes(%d) = %q, want %q", test.bytes, got, test.want)
		}
	}
}

func TestProgressLabelCombinesVersionAndSource(t *testing.T) {
	if got := progressLabel(gdit.ProgressEvent{Version: "4.5.1-dotnet", Source: "godothub"}); got != "4.5.1-dotnet(godothub)" {
		t.Fatalf("unexpected label: %q", got)
	}
	if got := progressLabel(gdit.ProgressEvent{Source: "github"}); got != "github" {
		t.Fatalf("label without version must fall back to source: %q", got)
	}
}

func TestTTYRendererLabelsVersionAndSource(t *testing.T) {
	var stderr bytes.Buffer
	renderer := newProgressRenderer(&stderr)
	renderer.terminal = true
	renderer.noColor = true
	renderer.render(gdit.ProgressEvent{Stage: "resolve", Version: "4.5.1-dotnet", Source: "godothub", Filename: "a.zip"})
	renderer.render(gdit.ProgressEvent{Stage: "download", Version: "4.5.1-dotnet", Source: "godothub", Filename: "a.zip", BytesDownloaded: 10 * 1024 * 1024, TotalBytes: 20 * 1024 * 1024})
	got := stderr.String()
	if !strings.Contains(got, "trying 4.5.1-dotnet from godothub\n") {
		t.Fatalf("resolve line must name the version: %q", got)
	}
	if !strings.Contains(got, "4.5.1-dotnet(godothub)") {
		t.Fatalf("download label must combine version and source: %q", got)
	}
}

func TestNonttyChunkLabelsVersion(t *testing.T) {
	var stderr bytes.Buffer
	renderer := newProgressRenderer(&stderr)
	renderer.render(gdit.ProgressEvent{Stage: "download", Version: "4.5.1-dotnet", Source: "godothub", Filename: "a.zip", BytesDownloaded: 8 * 1024 * 1024, TotalBytes: 16 * 1024 * 1024})
	got := stderr.String()
	if !strings.Contains(got, "downloaded 4.5.1-dotnet 8 MB / 16 MB from godothub") {
		t.Fatalf("nontty chunk must name the version: %q", got)
	}
}

func TestDashLineColorAndNoColor(t *testing.T) {
	renderer := &progressRenderer{noColor: false, barColor: ansiBrand}
	got := renderer.dashLine(0.5, 10)
	if !strings.HasPrefix(got, ansiBrand+"-----") || !strings.Contains(got, ansiGray+"-----") || !strings.HasSuffix(got, ansiReset) {
		t.Fatalf("expected brand-colored dash line, got %q", got)
	}
	renderer.noColor = true
	got = renderer.dashLine(0.5, 10)
	if got != "-----" {
		t.Fatalf("expected plain dash line without color, got %q", got)
	}
}

func TestSupportsTrueColor(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	if !supportsTrueColor() {
		t.Fatal("expected COLORTERM=truecolor to enable truecolor")
	}
	t.Setenv("COLORTERM", "24bit")
	if !supportsTrueColor() {
		t.Fatal("expected COLORTERM=24bit to enable truecolor")
	}
	t.Setenv("COLORTERM", "")
	if supportsTrueColor() {
		t.Fatal("expected empty COLORTERM to disable truecolor")
	}
	t.Setenv("COLORTERM", "256color")
	if supportsTrueColor() {
		t.Fatal("expected 256color to disable truecolor")
	}
}

func TestPickBarColorFallsBackToGreen(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	if got := pickBarColor(); got != ansiBrand {
		t.Fatalf("expected brand color, got %q", got)
	}
	t.Setenv("COLORTERM", "")
	if got := pickBarColor(); got != ansiGreen {
		t.Fatalf("expected green fallback, got %q", got)
	}
}

func TestDashLineClampsRatio(t *testing.T) {
	renderer := &progressRenderer{noColor: true}
	if got := renderer.dashLine(1.5, 10); got != "----------" {
		t.Fatalf("ratio above 1 must clamp to full width, got %q", got)
	}
}

func TestProgressTextWithAndWithoutTotal(t *testing.T) {
	renderer := &progressRenderer{}
	if got := renderer.progressText(524930, 6930000); got != "512.63 KiB/6.61 MiB" {
		t.Fatalf("unexpected progress text: %q", got)
	}
	if got := renderer.progressText(524930, 0); got != "512.63 KiB" {
		t.Fatalf("unexpected progress text without total: %q", got)
	}
}

// TTY 渲染器在终端模式下输出 \r 重绘行并在 warning 前清行。
func TestTTYRendererClearsBeforeWarning(t *testing.T) {
	var stderr bytes.Buffer
	renderer := newProgressRenderer(&stderr)
	renderer.terminal = true
	renderer.noColor = true
	renderer.render(gdit.ProgressEvent{Stage: "download", Source: "github", Filename: "a.zip", BytesDownloaded: 10 * 1024 * 1024, TotalBytes: 20 * 1024 * 1024})
	if !strings.HasPrefix(stderr.String(), "\r") {
		t.Fatalf("expected carriage-return redraw, got %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "\n") {
		t.Fatalf("TTY progress must not emit newlines: %q", stderr.String())
	}
	stderr.Reset()
	renderer.render(gdit.ProgressEvent{Stage: "warning", Source: "github", Message: "fixture"})
	if !strings.HasPrefix(stderr.String(), "\r\x1b[Kwarning: fixture\n") {
		t.Fatalf("expected cleared line before warning, got %q", stderr.String())
	}
}

// TTY 渲染器在节流窗口内跳过重绘，只有超过 100ms 或首个样本才绘制。
func TestTTYRendererThrottlesRedraw(t *testing.T) {
	var stderr bytes.Buffer
	renderer := newProgressRenderer(&stderr)
	renderer.terminal = true
	renderer.noColor = true
	event := gdit.ProgressEvent{Stage: "download", Source: "github", Filename: "a.zip", BytesDownloaded: 10 * 1024 * 1024, TotalBytes: 20 * 1024 * 1024}
	renderer.render(event)
	first := stderr.String()
	renderer.render(gdit.ProgressEvent{Stage: "download", Source: "github", Filename: "a.zip", BytesDownloaded: 11 * 1024 * 1024, TotalBytes: 20 * 1024 * 1024})
	if stderr.String() != first {
		t.Fatalf("redraw inside throttle window must be skipped: %q", stderr.String())
	}
}
