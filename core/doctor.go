package gdit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Sheyiyuan/GoDoIt/core/internal/instance"
	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
	"github.com/Sheyiyuan/GoDoIt/core/internal/store"
)

// CheckStatus 是 doctor 检查项的稳定状态。
type CheckStatus string

const (
	// StatusOK 表示检查通过。
	StatusOK CheckStatus = "ok"
	// StatusWarn 表示不影响使用的提示（不影响退出码）。
	StatusWarn CheckStatus = "warn"
	// StatusError 表示需要修复的错误（退出码 1）。
	StatusError CheckStatus = "error"
)

// CheckResult 描述一条 doctor 检查结果。
type CheckResult struct {
	Code    string      `json:"code"`    // 稳定标识，对应检查项表 code
	Status  CheckStatus `json:"status"`  // ok / warn / error
	Message string      `json:"message"` // 中文描述
	Suggest string      `json:"suggest,omitempty"`
	Details []string    `json:"details,omitempty"`
}

// DoctorReport 是 doctor 的完整报告。
type DoctorReport struct {
	Root       string        `json:"root"`
	Items      []CheckResult `json:"items"`
	OKCount    int           `json:"ok_count"`
	WarnCount  int           `json:"warn_count"`
	ErrorCount int           `json:"error_count"`
}

// networkProbeTimeout 是 --network 来源探测的单个请求超时。
const networkProbeTimeout = 5 * time.Second

// sensitiveKeyPattern 匹配需要掩码值的敏感键名特征（token/secret/password/key）。
var sensitiveKeyPattern = regexp.MustCompile(`(?i)(token|secret|password|key)`)

type doctorCheck func(code string, status CheckStatus, message, suggest string, details ...string)

// Doctor 执行纯只读环境诊断：不获取全局修改锁、不写任何文件、不重建状态、
// 不访问网络（除非 network=true）。检查以收集式执行，单项失败不影响后续检查；
// 只有根目录本身不可读等致命情况才整体返回错误。
func (m *Manager) Doctor(ctx context.Context, network bool) (DoctorReport, error) {
	report := DoctorReport{Root: m.root, Items: make([]CheckResult, 0, 16)}
	check := func(code string, status CheckStatus, message, suggest string, details ...string) {
		report.Items = append(report.Items, CheckResult{Code: code, Status: status, Message: message, Suggest: suggest, Details: details})
		switch status {
		case StatusOK:
			report.OKCount++
		case StatusWarn:
			report.WarnCount++
		default:
			report.ErrorCount++
		}
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	checkPlatform(check)
	m.checkRootDir(ctx, check)
	checks := []func(context.Context, doctorCheck){
		m.checkShim,
		m.checkCurrent,
		m.checkInstances,
		m.checkIcons,
		m.checkEngines,
		m.checkSDKs,
		m.checkTemplates,
		m.checkEnvironment,
		func(ctx context.Context, check doctorCheck) {
			m.checkSources(ctx, network, check)
		},
		m.checkState,
	}
	for _, runCheck := range checks {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		runCheck(ctx, check)
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	return report, nil
}

// checkIcons 检查自定义条目图标是否完整，并报告 icons/ 中没有条目引用的孤立文件。
// 自定义图标故障只影响 GUI 展示，条目会回退到 edition 默认图标，因此状态为 warning。
func (m *Manager) checkIcons(ctx context.Context, check doctorCheck) {
	items, err := instance.ScanDefinitions(m.root)
	if err != nil {
		check("icons", StatusWarn, "条目集合无效，无法核对自定义图标引用", "先修复坏条目")
		return
	}
	expected := make(map[string]string)
	hasWarning := false
	for _, item := range items {
		if instance.IconStrategy(item) != instance.IconCustom {
			continue
		}
		filename := item.ID + ".png"
		expected[filename] = item.Name
		if inspectErr := instance.InspectIcon(m.root, item.ID); inspectErr != nil {
			hasWarning = true
			check("icons", StatusWarn, fmt.Sprintf("条目 %s 的自定义图标缺失或无效：%v", item.Name, inspectErr), "重新导入图标或改用内置图标")
		}
	}
	entries, err := os.ReadDir(filepath.Join(m.root, "icons"))
	if errors.Is(err, os.ErrNotExist) {
		if len(expected) == 0 {
			check("icons", StatusOK, "无自定义条目图标", "")
		}
		return
	}
	if err != nil {
		check("icons", StatusWarn, fmt.Sprintf("读取 icons/ 失败：%v", err), "检查目录权限")
		return
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return
		}
		if _, referenced := expected[entry.Name()]; referenced {
			continue
		}
		hasWarning = true
		check("icons", StatusWarn, fmt.Sprintf("发现孤立自定义图标 %s", entry.Name()), "可手动删除该文件")
	}
	if !hasWarning && len(entries) == len(expected) {
		if len(expected) == 0 {
			check("icons", StatusOK, "无自定义条目图标", "")
		} else {
			check("icons", StatusOK, fmt.Sprintf("全部 %d 个自定义条目图标完整", len(expected)), "")
		}
	}
}

// checkPlatform 检查当前 OS/arch 是否在支持矩阵内。
func checkPlatform(check doctorCheck) {
	target, err := platform.CurrentTarget()
	if err != nil {
		check("platform", StatusError, "当前平台不在支持矩阵内（linux/amd64、darwin/arm64、windows/amd64）", "")
		return
	}
	check("platform", StatusOK, fmt.Sprintf("平台 %s/%s 受支持", target.OS, target.Arch), "")
}

// checkRootDir 检查根目录存在、可访问、权限不对外可写，以及 tmp/ 遗留目录。
// 返回根目录是否可访问（false 时后续检查仍继续，但结果不可靠）。
func (m *Manager) checkRootDir(ctx context.Context, check doctorCheck) bool {
	info, err := os.Stat(m.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			check("root-dir", StatusError, fmt.Sprintf("根目录 %s 不存在（尚未初始化）", m.root), "运行 gdit install 创建条目")
		} else {
			check("root-dir", StatusError, fmt.Sprintf("根目录 %s 不可访问：%v", m.root, err), "")
		}
		return false
	}
	if !info.IsDir() {
		check("root-dir", StatusError, fmt.Sprintf("根目录 %s 不是目录", m.root), "")
		return false
	}
	if accessErr := platform.RootAccessIssue(m.root); accessErr != nil {
		check("root-dir", StatusError, fmt.Sprintf("根目录 %s 不可读写：%v", m.root, accessErr), "检查目录权限")
		return false
	}
	check("root-dir", StatusOK, fmt.Sprintf("根目录 %s 存在且可读写", m.root), "")
	// 权限检查仅 POSIX 有效；Windows 已降级为可访问检查。
	if permErr := platform.RootPermissionIssue(m.root); permErr != nil {
		check("root-dir", StatusWarn, permErr.Error(), "建议 chmod 700 收紧权限")
	}
	m.checkStaleOperations(check)
	return true
}

// checkStaleOperations 检查 tmp/ 下遗留的 operation 目录（上次安装中断残留）。
func (m *Manager) checkStaleOperations(check doctorCheck) {
	entries, err := os.ReadDir(filepath.Join(m.root, "tmp"))
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		return
	}
	var stale []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "operation-") {
			stale = append(stale, entry.Name())
		}
	}
	if len(stale) > 0 {
		check("root-dir", StatusWarn, fmt.Sprintf("tmp/ 下存在 %d 个遗留 operation 目录（上次安装中断残留）：%s", len(stale), strings.Join(stale, "、")), "可安全删除")
	}
}

// checkShim 检查 shim 形态与 PATH 配置。
func (m *Manager) checkShim(ctx context.Context, check doctorCheck) {
	executable, err := os.Executable()
	if err != nil {
		executable = ""
	}
	created, correct := platform.CheckShim(m.root, executable)
	switch {
	case !created:
		check("shim", StatusWarn, "godot shim 尚未创建", "运行 gdit setup")
	case !correct:
		check("shim", StatusError, "godot shim 指向错误或目标不存在", "运行 gdit setup 修复")
	default:
		check("shim", StatusOK, "godot shim 就绪", "")
	}
	binDir := filepath.Dir(platform.ShimPath(m.root))
	if pathContains(binDir) {
		return
	}
	check("shim", StatusWarn, fmt.Sprintf("%s 不在 PATH 中", binDir), platform.PathHint(binDir))
	// 检查 PATH 中是否有其他 godot 抢先（先于 <根目录>/bin）。
	if earlier := earlierGodot(binDir); earlier != "" {
		check("shim", StatusWarn, fmt.Sprintf("PATH 中的其他 godot（%s）先于 %s", earlier, binDir), "建议调整 PATH 顺序")
	}
}

// pathContains 报告 directory 是否在 PATH 中（按平台分隔符拆分）。
func pathContains(directory string) bool {
	for _, item := range filepath.SplitList(os.Getenv("PATH")) {
		if platform.EqualPath(item, directory) {
			return true
		}
	}
	return false
}

// earlierGodot 返回 PATH 中先于 binDir 出现的 godot 可执行文件路径；没有则返回空串。
func earlierGodot(binDir string) string {
	for _, item := range filepath.SplitList(os.Getenv("PATH")) {
		if platform.EqualPath(item, binDir) {
			return ""
		}
		if candidate := platform.FindGodotCommand(item); candidate != "" {
			return candidate
		}
	}
	return ""
}

// checkCurrent 检查 current 指针形态与目标存在性。
func (m *Manager) checkCurrent(ctx context.Context, check doctorCheck) {
	target, err := platform.ReadCurrentPointer(m.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			check("current", StatusWarn, "current 未设置", "运行 gdit install 创建条目，或 gdit default <name>")
			m.checkNoInstances(check)
			return
		}
		check("current", StatusError, fmt.Sprintf("current 无法读取：%v", err), "运行 gdit default <name>")
		return
	}
	if _, err := platform.ParseCurrentPointer(target); err != nil {
		check("current", StatusError, fmt.Sprintf("current 指针非法：%s", err), "运行 gdit default <name>")
		return
	}
	resolved := filepath.Join(m.root, target)
	info, err := os.Lstat(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			check("current", StatusError, fmt.Sprintf("current 指向不存在的条目文件 %s", target), "运行 gdit default <name> 或 gdit install")
			return
		}
		check("current", StatusError, fmt.Sprintf("检查 current 目标失败：%v", err), "")
		return
	}
	if !info.Mode().IsRegular() {
		check("current", StatusError, fmt.Sprintf("current 目标 %s 不是普通条目文件", target), "运行 gdit default <name>")
		return
	}
	check("current", StatusOK, "current 指向合法条目", "")
}

// checkNoInstances 在无 current 时检查是否完全没有任何条目。
func (m *Manager) checkNoInstances(check doctorCheck) {
	items, err := instance.ScanDefinitions(m.root)
	if err != nil {
		// 有坏条目时由 instances 检查项报告，这里不重复。
		return
	}
	if len(items) == 0 {
		check("current", StatusWarn, "尚无任何条目", "运行 gdit install 创建条目")
	}
}

// checkInstances 检查全部条目的可读性、合法性、引擎/SDK 引用完整性（失败关闭哲学）。
func (m *Manager) checkInstances(ctx context.Context, check doctorCheck) {
	storeRoot := store.New(m.root)
	engines, err := storeRoot.ScanValid()
	if err != nil {
		check("instances", StatusError, fmt.Sprintf("扫描引擎资产失败：%v", err), "")
		return
	}
	sdks, err := storeRoot.ScanSDKs()
	if err != nil {
		check("instances", StatusError, fmt.Sprintf("扫描 SDK 资产失败：%v", err), "")
		return
	}
	engineIDs := make(map[string]bool)
	for _, record := range engines {
		engineIDs[record.Manifest.ID] = true
	}
	sdkVersions := make(map[string]bool)
	for _, record := range sdks {
		sdkVersions[record.Manifest.Version] = true
	}
	templates, err := storeRoot.ScanTemplates()
	if err != nil {
		check("instances", StatusError, fmt.Sprintf("扫描导出模板失败：%v", err), "")
		return
	}
	templateIDs := make(map[string]bool, len(templates))
	for _, record := range templates {
		templateIDs[record.Manifest.ID] = true
	}
	items, err := instance.ScanDefinitions(m.root)
	if err != nil {
		check("instances", StatusError, fmt.Sprintf("条目集合存在问题：%v", err), "修复或删除坏条目")
		return
	}
	if len(items) == 0 {
		check("instances", StatusOK, "无条目（空安装）", "")
		return
	}
	hasError := false
	hasWarning := false
	for _, item := range items {
		engineID := item.Engine.Version + "-" + item.Engine.Edition
		if !engineIDs[engineID] {
			check("instances", StatusError, fmt.Sprintf("条目 %s 引用的引擎 %s 未完整安装", item.Name, engineID), "运行 gdit engine install "+engineID+" 或删除条目")
			hasError = true
		}
		if item.Dotnet != nil && item.Dotnet.Strategy == "managed" {
			if !sdkVersions[item.Dotnet.Version] {
				check("instances", StatusError, fmt.Sprintf("条目 %s 引用的托管 SDK %s 未安装", item.Name, item.Dotnet.Version), "运行 gdit sdk install "+item.Dotnet.Version)
				hasError = true
			}
		}
		if item.Template != nil && !templateIDs[item.Template.ID] {
			check("instances", StatusWarn, fmt.Sprintf("条目 %s 绑定的导出模板 %s 未安装", item.Name, item.Template.ID), "运行 gdit template attach "+item.Name)
			hasWarning = true
		}
	}
	if !hasError && !hasWarning {
		check("instances", StatusOK, fmt.Sprintf("全部 %d 个条目引用完整", len(items)), "")
	}
}

// checkEngines 检查 engines/ 下每个目录的完整性，并报告无效目录的原因。
func (m *Manager) checkEngines(ctx context.Context, check doctorCheck) {
	entries, err := os.ReadDir(filepath.Join(m.root, "engines"))
	if errors.Is(err, os.ErrNotExist) {
		check("engines", StatusOK, "无引擎资产", "")
		return
	}
	if err != nil {
		check("engines", StatusError, fmt.Sprintf("读取 engines/ 失败：%v", err), "")
		return
	}
	count := 0
	hasError := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		count++
		dir := filepath.Join(m.root, "engines", entry.Name())
		if inspectErr := store.InspectEngineDir(dir); inspectErr != nil {
			check("engines", StatusError, fmt.Sprintf("引擎资产 %s 无效：%v", entry.Name(), inspectErr), "手动删除目录或重新安装")
			hasError = true
		}
	}
	if count == 0 {
		check("engines", StatusOK, "无引擎资产", "")
		return
	}
	if !hasError {
		check("engines", StatusOK, fmt.Sprintf("全部 %d 个引擎资产完整", count), "")
	}
}

// checkSDKs 检查 sdks/ 下每个目录的完整性，以及系统 SDK 探测结果。
func (m *Manager) checkSDKs(ctx context.Context, check doctorCheck) {
	entries, err := os.ReadDir(filepath.Join(m.root, "sdks"))
	if errors.Is(err, os.ErrNotExist) {
		check("sdks", StatusOK, "无托管 SDK 资产", "")
	} else if err != nil {
		check("sdks", StatusError, fmt.Sprintf("读取 sdks/ 失败：%v", err), "")
	} else {
		count := 0
		hasError := false
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			count++
			dir := filepath.Join(m.root, "sdks", entry.Name())
			if inspectErr := store.InspectSDKDir(dir); inspectErr != nil {
				check("sdks", StatusError, fmt.Sprintf("托管 SDK %s 无效：%v", entry.Name(), inspectErr), "手动删除目录或重新安装")
				hasError = true
			}
		}
		if count == 0 {
			check("sdks", StatusOK, "无托管 SDK 资产", "")
		} else if !hasError {
			check("sdks", StatusOK, fmt.Sprintf("全部 %d 个托管 SDK 完整", count), "")
		}
	}
	// 系统 SDK 探测：失败仅为信息性警告（系统无 SDK 不是错误）。
	system, probeErr := m.sdkProbe(ctx)
	if probeErr != nil {
		if errors.Is(probeErr, context.Canceled) || errors.Is(probeErr, context.DeadlineExceeded) {
			return
		}
		check("sdks", StatusWarn, fmt.Sprintf("系统 SDK 探测失败：%v", probeErr), "")
		return
	}
	if len(system) == 0 {
		check("sdks", StatusWarn, "系统中未发现 dotnet SDK", "dotnet 条目可用 gdit sdk install 安装托管 SDK")
	} else {
		check("sdks", StatusOK, fmt.Sprintf("系统 SDK：%s", systemSDKNames(system)), "")
	}
}

func systemSDKNames(sdks []SDKInfo) string {
	names := make([]string, 0, len(sdks))
	for _, sdk := range sdks {
		names = append(names, sdk.Version)
	}
	return strings.Join(names, "、")
}

// checkTemplates 检查 templates/ 下每个导出模板资产的完整性。
func (m *Manager) checkTemplates(ctx context.Context, check doctorCheck) {
	entries, err := os.ReadDir(filepath.Join(m.root, "templates"))
	if errors.Is(err, os.ErrNotExist) {
		check("templates", StatusOK, "无导出模板资产", "")
		return
	}
	if err != nil {
		check("templates", StatusError, fmt.Sprintf("读取 templates/ 失败：%v", err), "")
		return
	}
	count := 0
	hasError := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		count++
		dir := filepath.Join(m.root, "templates", entry.Name())
		if inspectErr := store.InspectTemplateDir(dir); inspectErr != nil {
			check("templates", StatusError, fmt.Sprintf("导出模板 %s 无效：%v", entry.Name(), inspectErr), "手动删除目录或重新安装")
			hasError = true
		}
	}
	if count == 0 {
		check("templates", StatusOK, "无导出模板资产", "")
		return
	}
	if !hasError {
		check("templates", StatusOK, fmt.Sprintf("全部 %d 个导出模板资产完整", count), "")
	}
}

// checkEnvironment 计算当前条目的注入环境并预览（含敏感键掩码）。
func (m *Manager) checkEnvironment(ctx context.Context, check doctorCheck) {
	view, err := m.EffectiveEnv(ctx, "")
	if err != nil {
		if errors.Is(err, ErrNoDefault) {
			check("environment", StatusWarn, "无 current 条目，无法预览注入环境", "设置 current 后重新运行")
			return
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		check("environment", StatusError, fmt.Sprintf("环境计算失败：%v", err), "")
		return
	}
	details := make([]string, 0, len(view.Vars)+1)
	for _, variable := range view.Vars {
		details = append(details, fmt.Sprintf("%s=%s（%s）", variable.Key, maskSensitive(variable.Key, variable.Value), variable.Origin))
	}
	if len(view.Args) > 0 {
		details = append(details, "参数："+strings.Join(view.Args, " "))
	}
	if len(details) == 0 {
		check("environment", StatusOK, "无注入环境变量", "")
		return
	}
	check("environment", StatusOK, fmt.Sprintf("将注入 %d 个环境变量、%d 个引擎参数", len(view.Vars), len(view.Args)), "", details...)
}

// maskSensitive 对键名匹配敏感特征的变量值掩码（--verbose 也不放开）。
func maskSensitive(key, value string) string {
	if sensitiveKeyPattern.MatchString(key) {
		return "******"
	}
	return value
}

// checkSources 检查来源配置与可达性（--network 时探测）。
func (m *Manager) checkSources(ctx context.Context, network bool, check doctorCheck) {
	providers, err := m.installSources()
	if err != nil {
		check("sources", StatusError, fmt.Sprintf("来源初始化失败：%v", err), "")
		return
	}
	okCount := 0
	available := false
	details := make([]string, 0, len(providers))
	for _, provider := range providers {
		authorizationEnv := providerAuthorizationEnv(provider)
		if authorizationEnv != "" {
			if value, set := os.LookupEnv(authorizationEnv); !set || value == "" {
				check("sources", StatusWarn, fmt.Sprintf("来源 %s 的授权变量 %s 未设置", provider.Name(), authorizationEnv), "配置后重新运行")
				continue
			}
		}
		if !network {
			okCount++
			details = append(details, fmt.Sprintf("来源 %s 配置有效", provider.Name()))
			continue
		}
		endpoint := providerMetadataEndpoint(provider)
		if endpoint == "" {
			check("sources", StatusWarn, fmt.Sprintf("来源 %s 无元数据端点，跳过可达性探测", provider.Name()), "")
			continue
		}
		available = true
		probeCtx, cancel := context.WithTimeout(ctx, networkProbeTimeout)
		probeErr := probeSource(probeCtx, m.client, endpoint)
		cancel()
		if probeErr != nil {
			check("sources", StatusWarn, fmt.Sprintf("来源 %s 探测失败：%v", provider.Name(), probeErr), "")
			continue
		}
		okCount++
		details = append(details, fmt.Sprintf("来源 %s 可达", provider.Name()))
	}
	if network && available && okCount == 0 {
		check("sources", StatusError, "全部启用来源均不可达", "检查网络或来源配置")
		return
	}
	if !network {
		check("sources", StatusOK, fmt.Sprintf("%d 个来源配置有效", len(providers)), "", details...)
	} else if okCount > 0 {
		check("sources", StatusOK, fmt.Sprintf("%d 个来源可达", okCount), "", details...)
	}
}

// providerAuthorizationEnv 返回 provider 的授权环境变量名（自定义源）。
func providerAuthorizationEnv(provider Source) string {
	if custom, ok := provider.(providerAdapter); ok {
		return custom.authorizationEnv()
	}
	return ""
}

// providerMetadataEndpoint 返回 provider 的元数据端点；URL 模板型自定义源返回空串。
// 同时支持公共 MetadataProber 来源（宿主注入的 fixture）与 providerAdapter 包装。
func providerMetadataEndpoint(provider Source) string {
	if prober, ok := provider.(MetadataProber); ok {
		return prober.MetadataEndpoint()
	}
	if adapter, ok := provider.(providerAdapter); ok {
		return adapter.metadataEndpoint()
	}
	return ""
}

// probeSource 对元数据端点做 HEAD 请求探测可达性。
func probeSource(ctx context.Context, client *http.Client, endpoint string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s", response.Status)
	}
	return nil
}

// checkState 检查 state.toml 可解析且与资产目录一致。
func (m *Manager) checkState(ctx context.Context, check doctorCheck) {
	storeRoot := store.New(m.root)
	records, err := storeRoot.ScanValid()
	if err != nil {
		check("state", StatusWarn, fmt.Sprintf("引擎资产扫描失败：%v", err), "")
		return
	}
	sdkRecords, err := storeRoot.ScanSDKs()
	if err != nil {
		check("state", StatusWarn, fmt.Sprintf("SDK 资产扫描失败：%v", err), "")
		return
	}
	matches, err := storeRoot.StateMatches(records, sdkRecords)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if len(records) == 0 && len(sdkRecords) == 0 {
				// 根目录未初始化且无资产时，不需要状态索引。
				check("state", StatusOK, "无状态索引（未初始化）", "")
				return
			}
			check("state", StatusWarn, "state.toml 缺失但资产目录非空", "下次读取时自动重建")
			return
		}
		check("state", StatusWarn, fmt.Sprintf("state.toml 不可解析或不一致：%v", err), "下次读取时自动重建")
		return
	}
	if !matches {
		check("state", StatusWarn, "state.toml 与资产目录不一致", "下次读取时自动重建")
		return
	}
	check("state", StatusOK, "state.toml 与资产目录一致", "")
}
