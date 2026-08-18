package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	gdit "github.com/Sheyiyuan/GoDoIt/core"
)

const (
	ansiGreen = "\x1b[32m"
	ansiGray  = "\x1b[90m"
	ansiReset = "\x1b[0m"
	// ansiBrand 是品牌色：Go(#00ADD8)、Godot(#478CBF)、C#(#68217A) 三色取平均
	// 得到 rgb(58,115,176) = #3A73B0，以 truecolor 输出。
	ansiBrand = "\x1b[38;2;58;115;176m"

	// progressRefreshInterval 是 TTY 动画的最小重绘间隔，避免高频下载事件拖慢传输。
	progressRefreshInterval = 100 * time.Millisecond
	// progressNonttyChunk 是非 TTY 环境下每次打点的字节数（8 MiB）。
	progressNonttyChunk = 8 * 1024 * 1024

	defaultTerminalWidth = 80
	minDashWidth         = 10
	maxDashWidth         = 60
)

// progressRenderer 负责下载进度在 stderr 上的渲染。
// TTY 下用 \r 行内重绘破折号线（已下载品牌色、未下载灰色），非 TTY 下按 8 MiB 打点。
type progressRenderer struct {
	stderr    io.Writer
	terminal  bool
	noColor   bool
	barColor  string
	lastDraw  time.Time
	lastBytes int64
	lastTotal int64
	lastLabel string
	lastChunk map[string]int64
}

// newProgressRenderer 创建进度渲染器，TTY 检测与 NO_COLOR 在创建时确定。
// 终端支持 truecolor 时用品牌色，否则回退绿色。
func newProgressRenderer(stderr io.Writer) *progressRenderer {
	return &progressRenderer{
		stderr:    stderr,
		terminal:  term.IsTerminal(int(os.Stderr.Fd())),
		noColor:   os.Getenv("NO_COLOR") != "",
		barColor:  pickBarColor(),
		lastChunk: make(map[string]int64),
	}
}

// pickBarColor 按终端 truecolor 能力选择进度条颜色。
func pickBarColor() string {
	if supportsTrueColor() {
		return ansiBrand
	}
	return ansiGreen
}

// supportsTrueColor 按 COLORTERM 判断终端是否支持 24 位真彩色。
func supportsTrueColor() bool {
	value := strings.ToLower(os.Getenv("COLORTERM"))
	return strings.Contains(value, "truecolor") || strings.Contains(value, "24bit")
}

// defaultLine 渲染 list 输出中的默认版本行：stdout 是 TTY 且未设置 NO_COLOR 时
// 整行用品牌色高亮（truecolor 不支持时回退绿色），否则保持纯文本，保证管道和
// 重定向场景下 stdout 仍然机器可读。
func defaultLine(text string) string {
	if os.Getenv("NO_COLOR") != "" || !stdoutIsTTY() {
		return text
	}
	return pickBarColor() + text + ansiReset
}

// render 处理 ProgressEvent，把进度渲染到 stderr。
func (r *progressRenderer) render(event gdit.ProgressEvent) {
	switch event.Stage {
	case "resolve":
		if event.Version != "" {
			fmt.Fprintf(r.stderr, "trying %s from %s\n", event.Version, event.Source)
		} else {
			fmt.Fprintf(r.stderr, "trying source %s\n", event.Source)
		}
	case "download":
		if r.terminal {
			r.renderBar(event)
		} else {
			r.renderChunk(event)
		}
	case "complete":
		r.clearLine()
		if r.terminal {
			// 下载完成时已下载量等于总量（节流窗口内 lastBytes 可能过期）。
			downloaded := r.lastBytes
			if r.lastTotal > 0 {
				downloaded = r.lastTotal
			}
			// 无 download 样本（如零字节资产）时用事件字段兜底构造标签。
			label := r.lastLabel
			if label == "" {
				label = progressLabel(event)
			}
			text := r.progressText(downloaded, r.lastTotal)
			fmt.Fprintf(r.stderr, "%s  %s  %s\n", label, r.dashLine(1, r.dashWidth(label, text)), text)
		}
	case "warning":
		r.clearLine()
		fmt.Fprintf(r.stderr, "warning: %s\n", event.Message)
	}
}

// renderBar 在 TTY 上重绘当前进度行。
func (r *progressRenderer) renderBar(event gdit.ProgressEvent) {
	now := time.Now()
	if !r.lastDraw.IsZero() && now.Sub(r.lastDraw) < progressRefreshInterval {
		return
	}
	ratio := 0.0
	if event.TotalBytes > 0 {
		ratio = float64(event.BytesDownloaded) / float64(event.TotalBytes)
		if ratio > 1 {
			ratio = 1
		}
	}
	label := progressLabel(event)
	r.lastDraw = now
	r.lastBytes = event.BytesDownloaded
	r.lastTotal = event.TotalBytes
	r.lastLabel = label
	text := r.progressText(event.BytesDownloaded, event.TotalBytes)
	fmt.Fprintf(r.stderr, "\r%s  %s  %s\x1b[K", label, r.dashLine(ratio, r.dashWidth(label, text)), text)
}

// renderChunk 在非 TTY 下按 8 MiB 打点。
func (r *progressRenderer) renderChunk(event gdit.ProgressEvent) {
	key := progressLabel(event) + "/" + event.Filename
	chunk := event.BytesDownloaded / progressNonttyChunk
	if chunk > r.lastChunk[key] {
		r.lastChunk[key] = chunk
		progress := fmt.Sprintf("%d MB", chunk*progressNonttyChunk/(1024*1024))
		if event.TotalBytes > 0 {
			progress += fmt.Sprintf(" / %d MB", event.TotalBytes/(1024*1024))
		}
		if event.Version != "" {
			fmt.Fprintf(r.stderr, "downloaded %s %s from %s\n", event.Version, progress, event.Source)
		} else {
			fmt.Fprintf(r.stderr, "downloaded %s from %s\n", progress, event.Source)
		}
	}
}

// clearLine 清除当前进度行，避免残留行粘连后续输出。
func (r *progressRenderer) clearLine() {
	if r == nil || !r.terminal {
		return
	}
	fmt.Fprint(r.stderr, "\r\x1b[K")
}

// dashWidth 计算破折号线宽度：终端宽度减去标签、数字和间距的固定开销。
func (r *progressRenderer) dashWidth(label, text string) int {
	width := defaultTerminalWidth
	if w, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil && w > 0 {
		width = w
	}
	result := width - len(label) - len(text) - 4
	if result < minDashWidth {
		return minDashWidth
	}
	if result > maxDashWidth {
		return maxDashWidth
	}
	return result
}

// dashLine 生成破折号线：有颜色时已下载段用品牌色（barColor）、未下载段灰色；无颜色时只画已下载段。
func (r *progressRenderer) dashLine(ratio float64, width int) string {
	filled := int(ratio * float64(width))
	if filled > width {
		filled = width
	}
	if r.noColor {
		return strings.Repeat("-", filled)
	}
	var builder strings.Builder
	if filled > 0 {
		builder.WriteString(r.barColor)
		builder.WriteString(strings.Repeat("-", filled))
	}
	if filled < width {
		builder.WriteString(ansiGray)
		builder.WriteString(strings.Repeat("-", width-filled))
	}
	builder.WriteString(ansiReset)
	return builder.String()
}

// progressText 生成“已下载/总量”文本，总量未知时只显示已下载。
func (r *progressRenderer) progressText(downloaded, total int64) string {
	if total > 0 {
		return fmt.Sprintf("%s/%s", formatBytes(downloaded), formatBytes(total))
	}
	return formatBytes(downloaded)
}

// formatBytes 以二进制单位（KiB/MiB/GiB，1024 进制）格式化字节数。
func formatBytes(bytes int64) string {
	const kib int64 = 1024
	switch {
	case bytes >= kib*kib*kib:
		return fmt.Sprintf("%.2f GiB", float64(bytes)/(float64(kib)*float64(kib)*float64(kib)))
	case bytes >= kib*kib:
		return fmt.Sprintf("%.2f MiB", float64(bytes)/(float64(kib)*float64(kib)))
	case bytes >= kib:
		return fmt.Sprintf("%.2f KiB", float64(bytes)/float64(kib))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// progressLabel 生成进度行的标签：版本 ID 与来源的组合（如 4.5.1-dotnet(godothub)），
// 让批量安装时能区分正在下载的版本；事件不带版本 ID 时退回来源名。
func progressLabel(event gdit.ProgressEvent) string {
	if event.Version != "" {
		return event.Version + "(" + event.Source + ")"
	}
	return event.Source
}

// progressWriter 返回渲染 ProgressEvent 的闭包（兼容旧调用方式）。
func progressWriter(stderr io.Writer) func(gdit.ProgressEvent) {
	return newProgressRenderer(stderr).render
}
