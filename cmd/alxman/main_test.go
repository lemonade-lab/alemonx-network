package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validManifest = `{
  "id": "alemonx-network",
  "name": "网络、端口转发与防火墙",
  "version": "0.0.1",
  "runtime": "binary",
  "entry": {
    "darwin-arm64": "dist/alemonx-network-darwin-arm64",
    "go": "runner"
  },
  "development": {
    "runtime": "go",
    "entry": {
      "go": "runner"
    }
  },
  "web": {
    "root": "web"
  }
}`

const fullManifest = `{
  "id": "alemonx-network",
  "name": "网络插件",
  "version": "0.0.1",
  "runtime": "binary",
  "entry": {
    "darwin-arm64": "dist/x",
    "go": "runner"
  },
  "development": {
    "runtime": "go",
    "entry": {
      "go": "runner"
    },
    "web": {
      "mode": "dev-server",
      "root": "web",
      "build": {"program": "yarn", "args": ["--cwd", "frontend", "build"]},
      "dev": {"program": "yarn", "args": ["--cwd", "frontend", "dev", "--port", "${ALX_PLUGIN_DEV_PORT}"]},
      "healthPath": "/",
      "hmr": true
    }
  },
  "web": {"root": "web"},
  "statusActions": ["snapshot", "capabilities"],
  "services": [{"id": "webui", "name": "Web UI", "host": "127.0.0.1", "port": 17390, "basePath": "/", "embed": true}],
  "systemPickers": [{"id": "pick-dir", "kind": "directory", "title": "选择一个目录"}],
  "media": [{"id": "qrcode", "action": "make-qrcode", "contentType": "image/png"}],
  "privilegedOperations": [
    {
      "action": "apply-plan",
      "runnerAction": "apply-approved-plan",
      "planAction": "plan",
      "title": "授权网络变更",
      "authorization": "native",
      "platforms": ["darwin", "linux", "windows"]
    },
    {
      "action": "undo-last",
      "runnerAction": "undo-approved",
      "useLatestAudit": true,
      "title": "撤销",
      "authorization": "native",
      "platforms": ["darwin", "linux", "windows"]
    }
  ]
}`

func writeManifest(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "alx.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

func TestValidateOK(t *testing.T) {
	if code := validate(writeManifest(t, validManifest)); code != 0 {
		t.Fatalf("validate returned %d, want 0", code)
	}
}

func TestValidateBadID(t *testing.T) {
	manifest := strings.Replace(validManifest, `"id": "alemonx-network"`, `"id": "Bad_ID"`, 1)
	if code := validate(writeManifest(t, manifest)); code != 1 {
		t.Fatalf("validate returned %d, want 1", code)
	}
}

func TestValidateMissingWeb(t *testing.T) {
	manifest := strings.Replace(validManifest, `  "web": {
    "root": "web"
  }`, "", 1)
	if code := validate(writeManifest(t, manifest)); code != 1 {
		t.Fatalf("validate returned %d, want 1", code)
	}
}

func TestValidateNonObjectRoot(t *testing.T) {
	if code := validate(writeManifest(t, `["not", "an", "object"]`)); code != 1 {
		t.Fatalf("validate returned %d, want 1", code)
	}
}

func TestValidateInvalidJSONReportsLine(t *testing.T) {
	path := writeManifest(t, "{\n  \"id\": broken\n}")
	code := validate(path)
	if code != 1 {
		t.Fatalf("validate returned %d, want 1", code)
	}
}

func TestSetVersionOK(t *testing.T) {
	path := writeManifest(t, validManifest)
	if code := setVersionAt(path, "v1.2.3"); code != 0 {
		t.Fatalf("set-version returned %d, want 0", code)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rewritten manifest: %v", err)
	}
	if !strings.Contains(string(data), `"version": "1.2.3"`) {
		t.Fatalf("version not updated:\n%s", data)
	}
	if validate(path) != 0 {
		t.Fatalf("rewritten manifest does not validate:\n%s", data)
	}
}

func TestSetVersionKeepsUnicode(t *testing.T) {
	path := writeManifest(t, validManifest)
	if code := setVersionAt(path, "v1.2.3"); code != 0 {
		t.Fatalf("set-version returned %d, want 0", code)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rewritten manifest: %v", err)
	}
	if !strings.Contains(string(data), "网络") {
		t.Fatalf("unicode name got escaped:\n%s", data)
	}
}

func TestSetVersionRejectsBadTag(t *testing.T) {
	path := writeManifest(t, validManifest)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if code := setVersionAt(path, "not-a-tag"); code != 1 {
		t.Fatalf("set-version returned %d, want 1", code)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("manifest changed despite invalid tag")
	}
}

func TestCheckVersion(t *testing.T) {
	path := writeManifest(t, validManifest)
	if code := checkVersionAt(path, "v0.0.1"); code != 0 {
		t.Fatalf("check-version returned %d, want 0", code)
	}
	if code := checkVersionAt(path, "v9.9.9"); code != 1 {
		t.Fatalf("check-version returned %d, want 1", code)
	}
}

func TestVersionFromTag(t *testing.T) {
	for tag, want := range map[string]string{
		"v1.2.3":   "1.2.3",
		"v0.0.1":   "0.0.1",
		"1.2.3":    "1.2.3",
		" v2.0.0 ": "2.0.0",
	} {
		if got := versionFromTag(tag); got != want {
			t.Errorf("versionFromTag(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestValidateCatchesRuntimeEntryKey(t *testing.T) {
	manifest := strings.Replace(validManifest, `"go": "runner"`, `"weird-key": "runner"`, 1)
	if code := validate(writeManifest(t, manifest)); code != 1 {
		t.Fatalf("validate returned %d, want 1", code)
	}
}

func mutateManifest(t *testing.T, content string, mutate func(manifest map[string]any)) string {
	t.Helper()
	var manifest map[string]any
	if err := json.Unmarshal([]byte(content), &manifest); err != nil {
		t.Fatalf("unmarshal test manifest: %v", err)
	}
	mutate(manifest)
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal test manifest: %v", err)
	}
	return string(data)
}

func TestValidateFullHostSchema(t *testing.T) {
	if code := validate(writeManifest(t, fullManifest)); code != 0 {
		t.Fatalf("validate returned %d, want 0", code)
	}
}

func TestValidateStatusActionsRejectsDuplicates(t *testing.T) {
	manifest := mutateManifest(t, fullManifest, func(m map[string]any) {
		m["statusActions"] = []any{"snapshot", "snapshot"}
	})
	if code := validate(writeManifest(t, manifest)); code != 1 {
		t.Fatalf("validate returned %d, want 1", code)
	}
}

func TestValidatePrivilegedPlanWithLatestAudit(t *testing.T) {
	manifest := mutateManifest(t, fullManifest, func(m map[string]any) {
		operations := m["privilegedOperations"].([]any)
		operations[1].(map[string]any)["planAction"] = "plan"
	})
	if code := validate(writeManifest(t, manifest)); code != 1 {
		t.Fatalf("validate returned %d, want 1", code)
	}
}

func TestValidateNativeOperationRejectsCommands(t *testing.T) {
	manifest := mutateManifest(t, fullManifest, func(m map[string]any) {
		operations := m["privilegedOperations"].([]any)
		operations[0].(map[string]any)["commands"] = []any{map[string]any{"program": "netsh", "args": []any{"x"}}}
	})
	if code := validate(writeManifest(t, manifest)); code != 1 {
		t.Fatalf("validate returned %d, want 1", code)
	}
}

func TestValidateDevServerRequiresDevCommand(t *testing.T) {
	manifest := mutateManifest(t, fullManifest, func(m map[string]any) {
		development := m["development"].(map[string]any)
		delete(development["web"].(map[string]any), "dev")
	})
	if code := validate(writeManifest(t, manifest)); code != 1 {
		t.Fatalf("validate returned %d, want 1", code)
	}
}

func TestValidateRejectsUnexpectedVariableInCommand(t *testing.T) {
	manifest := mutateManifest(t, fullManifest, func(m map[string]any) {
		development := m["development"].(map[string]any)
		development["web"].(map[string]any)["dev"] = map[string]any{"program": "sh", "args": []any{"echo ${HOME}"}}
	})
	if code := validate(writeManifest(t, manifest)); code != 1 {
		t.Fatalf("validate returned %d, want 1", code)
	}
}

func TestValidateMediaRejectsNonPNG(t *testing.T) {
	manifest := mutateManifest(t, fullManifest, func(m map[string]any) {
		media := m["media"].([]any)
		media[0].(map[string]any)["contentType"] = "image/jpeg"
	})
	if code := validate(writeManifest(t, manifest)); code != 1 {
		t.Fatalf("validate returned %d, want 1", code)
	}
}

func TestValidateServiceMustBeLoopback(t *testing.T) {
	manifest := mutateManifest(t, fullManifest, func(m map[string]any) {
		services := m["services"].([]any)
		services[0].(map[string]any)["host"] = "0.0.0.0"
	})
	if code := validate(writeManifest(t, manifest)); code != 1 {
		t.Fatalf("validate returned %d, want 1", code)
	}
}

func TestValidateRuntimeAcceptsPythonRejectsCommand(t *testing.T) {
	ok := mutateManifest(t, fullManifest, func(m map[string]any) {
		m["runtime"] = "python"
	})
	if code := validate(writeManifest(t, ok)); code != 0 {
		t.Fatalf("python runtime returned %d, want 0", code)
	}
	bad := mutateManifest(t, fullManifest, func(m map[string]any) {
		m["runtime"] = "command"
	})
	if code := validate(writeManifest(t, bad)); code != 1 {
		t.Fatalf("command runtime returned %d, want 1", code)
	}
}
