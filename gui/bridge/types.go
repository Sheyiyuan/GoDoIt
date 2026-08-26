// Package bridge 提供 Wails 界面到 core 公共 API 的薄适配与结构化任务事件。
package bridge

import (
	gdit "github.com/Sheyiyuan/GoDoIt/core"
)

// AppSnapshot 是窗口启动或重载后可完整重建的只读状态。
type AppSnapshot struct {
	Root      string              `json:"root"`
	Instances []gdit.InstanceInfo `json:"instances"`
	Current   *gdit.InstanceInfo  `json:"current,omitempty"`
	Assets    AssetSnapshot       `json:"assets"`
	Doctor    gdit.DoctorReport   `json:"doctor"`
	GUI       gdit.GUISettings    `json:"gui"`
	Build     BuildInfo           `json:"build"`
	Issues    []string            `json:"issues,omitempty"`
}

// AssetSnapshot 汇总资源页直接消费的 core 结果，不另建业务状态。
type AssetSnapshot struct {
	Engines   []gdit.InstalledVersion `json:"engines"`
	SDKs      []gdit.SDKInfo          `json:"sdks"`
	Sources   []gdit.SourceInfo       `json:"sources"`
	Templates []gdit.TemplateInfo     `json:"templates"`
	Orphans   []gdit.OrphanAsset      `json:"orphans"`
}

// InstanceDetails 是条目详情页一次读取所需的结果。
type InstanceDetails struct {
	Instance   gdit.InstanceInfo      `json:"instance"`
	Env        gdit.EnvView           `json:"env"`
	Configured gdit.ConfiguredEnvView `json:"configured_env"`
	EnvError   string                 `json:"env_error,omitempty"`
	Templates  []gdit.TemplateInfo    `json:"templates"`
}

// EnvironmentDetails 是环境编辑器消费的配置层与有效值快照。
type EnvironmentDetails struct {
	Configured     gdit.ConfiguredEnvView `json:"configured"`
	Effective      EffectiveEnvView       `json:"effective"`
	EffectiveError string                 `json:"effective_error,omitempty"`
}

// EffectiveEnvVar 描述最终启动环境中的值及其敏感标记。
type EffectiveEnvVar struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Origin    string `json:"origin"`
	Sensitive bool   `json:"sensitive"`
}

// EffectiveEnvView 是 GUI 查看最终环境使用的安全视图。
type EffectiveEnvView struct {
	Vars []EffectiveEnvVar `json:"vars"`
	Args []string          `json:"args"`
}

// SessionSnapshot 是 GUI 运行会话列表返回值。
type SessionSnapshot struct {
	Sessions []gdit.SessionInfo `json:"sessions"`
}

// CandidateWarning 描述候选枚举期间未阻断可用结果的局部失败。
type CandidateWarning struct {
	Source  string `json:"source,omitempty"`
	Message string `json:"message"`
}

// EngineCandidateResult 返回按 core 分组的引擎候选及局部失败。
type EngineCandidateResult struct {
	Channels []gdit.EngineChannel `json:"channels"`
	Warnings []CandidateWarning   `json:"warnings"`
}

// SDKCandidateResult 返回按 core 分组的 SDK 候选及局部失败。
type SDKCandidateResult struct {
	Channels []gdit.SDKChannel  `json:"channels"`
	Warnings []CandidateWarning `json:"warnings"`
}

// BuildInfo 描述当前 GUI 构建。
type BuildInfo struct {
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
}

// OperationStart 是异步操作的立即响应。
type OperationStart struct {
	OperationID string `json:"operation_id"`
}

// ProgressEnvelope 是 gdit:progress 事件的稳定外层。
type ProgressEnvelope struct {
	OperationID string              `json:"operation_id"`
	Timestamp   string              `json:"timestamp"`
	Status      string              `json:"status"` // running / complete / failed / canceled
	Operation   string              `json:"operation"`
	Progress    *gdit.ProgressEvent `json:"progress,omitempty"`
	Result      any                 `json:"result,omitempty"`
	Error       string              `json:"error,omitempty"`
}
