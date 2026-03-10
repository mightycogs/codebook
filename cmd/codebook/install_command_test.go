package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFakeCLI(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("write fake cli %s: %v", name, err)
	}
	return path
}

func TestRunInstall_DryRun(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Setenv("SHELL", "/bin/zsh")

	binDir := t.TempDir()
	writeFakeCLI(t, binDir, "claude")
	writeFakeCLI(t, binDir, "codex")
	t.Setenv("PATH", binDir)

	out := captureStdout(t, func() {
		if code := runInstall([]string{"--dry-run"}); code != 0 {
			t.Fatalf("runInstall returned %d, want 0", code)
		}
	})

	wantParts := []string{
		"install",
		"[PATH]",
		"[Skills]",
		"[Claude Code] detected",
		"[Codex CLI] detected",
		"[Cursor] MCP config:",
		"[Gemini CLI] MCP config:",
		"[VS Code] MCP config:",
		"[Zed] MCP config:",
		"Done. Restart your editor/CLI to activate.",
	}
	for _, want := range wantParts {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}
}

func TestRunUninstall_DryRun(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	binDir := t.TempDir()
	writeFakeCLI(t, binDir, "claude")
	writeFakeCLI(t, binDir, "codex")
	t.Setenv("PATH", binDir)

	for name := range skillFiles {
		skillDir := filepath.Join(home, ".claude", "skills", name)
		if err := os.MkdirAll(skillDir, 0o750); err != nil {
			t.Fatalf("mkdir skill dir: %v", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(home, ".codex", "instructions"), 0o750); err != nil {
		t.Fatalf("mkdir codex instructions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "instructions", "codebook.md"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write codex instructions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("[mcp_servers.codebook]\ncommand = \"/tmp/cbm\"\n"), 0o600); err != nil {
		t.Fatalf("write codex config: %v", err)
	}
	cursorCfg := filepath.Join(home, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(cursorCfg), 0o750); err != nil {
		t.Fatalf("mkdir cursor cfg: %v", err)
	}
	if err := os.WriteFile(cursorCfg, []byte("{\"mcpServers\":{\"codebook\":{\"command\":\"/tmp/cbm\"}}}"), 0o600); err != nil {
		t.Fatalf("write cursor cfg: %v", err)
	}

	out := captureStdout(t, func() {
		if code := runUninstall([]string{"--dry-run"}); code != 0 {
			t.Fatalf("runUninstall returned %d, want 0", code)
		}
	})

	wantParts := []string{
		"uninstall",
		"[Skills]",
		"[Claude Code] detected",
		"[Codex CLI] detected",
		"[Cursor] MCP config:",
		"[dry-run] Would remove MCP section from:",
		"[dry-run] Would remove:",
		"Done. Binary and databases were NOT removed.",
	}
	for _, want := range wantParts {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}
}
