package release

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRuntimeNPMDependenciesUsesProductionClosure(t *testing.T) {
	lock := `lockfileVersion: '9.0'
importers:
  .:
    dependencies:
      app:
        version: 1.0.0(peer@2.0.0)
    devDependencies:
      test-only:
        version: 9.0.0
snapshots:
  app@1.0.0(peer@2.0.0):
    dependencies:
      runtime-child: 3.0.0
    optionalDependencies:
      type-only: 4.0.0
  runtime-child@3.0.0: {}
  test-only@9.0.0: {}
`
	filename := filepath.Join(t.TempDir(), "pnpm-lock.yaml")
	if err := os.WriteFile(filename, []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
	dependencies, err := runtimeNPMDependencies(filename)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{"app": "1.0.0", "runtime-child": "3.0.0"}
	if !reflect.DeepEqual(dependencies, expected) {
		t.Fatalf("运行时依赖 = %#v，期望 %#v", dependencies, expected)
	}
}

func TestValidateDependencySetRejectsUnknownAndUnusedEntries(t *testing.T) {
	actual := map[string]string{"runtime": "1.0.0"}
	if err := validateDependencySet("npm", actual, nil); err == nil {
		t.Fatal("缺少元数据的依赖未被拒绝")
	}
	metadata := []dependencyMetadata{{Name: "runtime", Version: "1.0.0"}, {Name: "unused", Version: "2.0.0"}}
	if err := validateDependencySet("npm", actual, metadata); err == nil {
		t.Fatal("未进入产物的元数据未被拒绝")
	}
}
