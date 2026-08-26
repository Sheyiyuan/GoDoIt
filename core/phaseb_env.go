package gdit

import (
	"context"
	"errors"
	"os"
	"sort"

	"github.com/Sheyiyuan/GoDoIt/core/internal/config"
	"github.com/Sheyiyuan/GoDoIt/core/internal/instance"
	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
)

// ConfiguredEnv 返回尚未合并的环境变量配置层。
// name 为空时返回全局和当前平台配置；指定条目名时额外返回该条目的配置。
// 返回顺序稳定，且不会执行 SDK 探测或生成派生变量。
func (m *Manager) ConfiguredEnv(ctx context.Context, name string) (ConfiguredEnvView, error) {
	if err := ctx.Err(); err != nil {
		return ConfiguredEnvView{}, err
	}
	cfg, err := config.Load(m.root)
	if err != nil {
		return ConfiguredEnvView{}, errors.Join(ErrInvalidConfig, err)
	}
	target, err := platform.CurrentTarget()
	if err != nil {
		return ConfiguredEnvView{}, errors.Join(ErrUnsupportedPlatform, err)
	}
	view := ConfiguredEnvView{Vars: make([]ConfiguredEnvVar, 0)}
	appendVars := func(values map[string]string, scope EnvScope, editable bool) {
		for key, value := range values {
			view.Vars = append(view.Vars, ConfiguredEnvVar{Key: key, Value: value, Scope: scope, Editable: editable, Sensitive: IsSensitiveEnvironmentKey(key)})
		}
	}
	appendVars(cfg.Environment.Global, EnvScopeGlobal, true)
	appendVars(cfg.Environment.PlatformVars(target.OS), EnvScopePlatform, false)
	if name != "" {
		if err := instance.ValidateName(name); err != nil {
			return ConfiguredEnvView{}, errors.Join(ErrInvalidInput, err)
		}
		item, lookupErr := instance.Lookup(m.root, name)
		if lookupErr != nil {
			if errors.Is(lookupErr, os.ErrNotExist) {
				return ConfiguredEnvView{}, errors.Join(ErrInstanceNotFound, lookupErr)
			}
			return ConfiguredEnvView{}, errors.Join(ErrInvalidConfig, lookupErr)
		}
		appendVars(item.Env, EnvScopeInstance, true)
	}
	sort.SliceStable(view.Vars, func(i, j int) bool {
		left, right := view.Vars[i], view.Vars[j]
		if left.Scope != right.Scope {
			return envScopeRank(left.Scope) < envScopeRank(right.Scope)
		}
		leftKey, rightKey := config.NormalizeEnvironmentKey(left.Key), config.NormalizeEnvironmentKey(right.Key)
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		return left.Key < right.Key
	})
	return view, nil
}

func envScopeRank(scope EnvScope) int {
	switch scope {
	case EnvScopeGlobal:
		return 0
	case EnvScopePlatform:
		return 1
	case EnvScopeInstance:
		return 2
	default:
		return 3
	}
}
