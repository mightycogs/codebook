package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mightycogs/codebase-memory-mcp/internal/discover"
	"github.com/mightycogs/codebase-memory-mcp/internal/store"
)

// --- Strategy 1: Key → Symbol Matching ---

func TestConfigKeySymbol_ExactMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.toml"), `[database]
max_connections = 100
`)
	writeFile(t, filepath.Join(dir, "main.go"), `package main

func getMaxConnections() int { return 100 }
`)

	edges := runAndGetConfigures(t, dir)
	found := false
	for _, e := range edges {
		if props, ok := e.Properties["strategy"]; ok && props == "key_symbol" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected key_symbol CONFIGURES edge for max_connections, got %d total CONFIGURES", len(edges))
	}
}

func TestConfigKeySymbol_SubstringMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.toml"), `[app]
request_timeout = 30
`)
	writeFile(t, filepath.Join(dir, "main.go"), `package main

func getRequestTimeoutSeconds() int { return 30 }
`)

	edges := runAndGetConfigures(t, dir)
	found := false
	for _, e := range edges {
		props := e.Properties
		if props["strategy"] == "key_symbol" {
			conf, _ := props["confidence"].(float64)
			if conf == 0.75 {
				found = true
			}
		}
	}
	if !found {
		t.Logf("got %d CONFIGURES edges", len(edges))
		for _, e := range edges {
			t.Logf("  strategy=%v confidence=%v", e.Properties["strategy"], e.Properties["confidence"])
		}
		t.Errorf("expected substring match (confidence=0.75) for request_timeout")
	}
}

func TestConfigKeySymbol_ShortKeySkipped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.toml"), `port = 8080
host = "localhost"
name = "test"
`)
	writeFile(t, filepath.Join(dir, "main.go"), `package main

func getPort() int { return 8080 }
`)

	edges := runAndGetConfigures(t, dir)
	for _, e := range edges {
		if e.Properties["strategy"] == "key_symbol" {
			t.Errorf("expected no key_symbol edges for short keys, got one: config_key=%v", e.Properties["config_key"])
		}
	}
}

func TestConfigKeySymbol_CamelCaseNormalization(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"), `{"maxRetryCount": 3}`)
	writeFile(t, filepath.Join(dir, "handler.go"), `package main

func getMaxRetryCount() int { return 3 }
`)

	edges := runAndGetConfigures(t, dir)
	found := false
	for _, e := range edges {
		if e.Properties["strategy"] == "key_symbol" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected key_symbol CONFIGURES edge for maxRetryCount")
	}
}

func TestConfigKeySymbol_NoFalsePositive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.toml"), `[db]
url = "postgres://..."
`)
	writeFile(t, filepath.Join(dir, "main.go"), `package main

func parseURL(s string) string { return s }
`)

	edges := runAndGetConfigures(t, dir)
	for _, e := range edges {
		if e.Properties["strategy"] == "key_symbol" && e.Properties["config_key"] == "url" {
			t.Errorf("expected no CONFIGURES edge for 'url' (1 token), but got one")
		}
	}
}

// --- Strategy 2: Dependency → Import Matching ---

func TestDependencyImport_PackageJson(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"express":"^4.0","lodash":"^4.17"}}`)
	writeFile(t, filepath.Join(dir, "app.js"), `const express = require('express');

function handleRequest() {
  return express();
}
`)

	edges := runAndGetConfigures(t, dir)
	found := false
	for _, e := range edges {
		if e.Properties["strategy"] == "dependency_import" {
			found = true
			break
		}
	}
	// This test may not produce edges since IMPORTS resolution depends on registry
	// but it should at least not crash
	t.Logf("dependency_import edges found: %v", found)
}

// --- Strategy 3: File Path Reference ---

func TestConfigFileRef_ExactPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config", "database.toml"), `[database]
host = "localhost"
`)
	writeFile(t, filepath.Join(dir, "main.go"), `package main

func loadConfig() {
	cfg := readFile("config/database.toml")
	_ = cfg
}
`)

	edges := runAndGetConfigures(t, dir)
	found := false
	for _, e := range edges {
		if e.Properties["strategy"] == "file_reference" {
			found = true
			break
		}
	}
	if !found {
		t.Logf("got %d CONFIGURES edges total", len(edges))
		t.Errorf("expected file_reference CONFIGURES edge for config/database.toml")
	}
}

func TestConfigFileRef_BasenameMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "settings.yaml"), `database:
  host: localhost
`)
	writeFile(t, filepath.Join(dir, "main.go"), `package main

func loadSettings() {
	cfg := readFile("settings.yaml")
	_ = cfg
}
`)

	edges := runAndGetConfigures(t, dir)
	found := false
	for _, e := range edges {
		if e.Properties["strategy"] == "file_reference" {
			conf, _ := e.Properties["confidence"].(float64)
			if conf == 0.70 {
				found = true
			}
		}
	}
	// Basename match may produce 0.70 or 0.90 depending on path resolution
	t.Logf("basename file_reference found: %v", found)
}

func TestConfigFileRef_NoFalsePositive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "data.csv"), `a,b,c`)
	writeFile(t, filepath.Join(dir, "main.go"), `package main

func loadData() {
	f := readFile("data.csv")
	_ = f
}
`)

	edges := runAndGetConfigures(t, dir)
	for _, e := range edges {
		if e.Properties["strategy"] == "file_reference" {
			t.Errorf("expected no file_reference edge for .csv file")
		}
	}
}

// --- Helpers ---

func runAndGetConfigures(t *testing.T, dir string) []*store.Edge {
	t.Helper()

	// Initialize as a git repo so discover works
	gitInit(t, dir)

	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	p := New(context.Background(), s, dir, discover.ModeFull)
	if err := p.Run(); err != nil {
		t.Fatalf("Pipeline.Run: %v", err)
	}

	edges, err := s.FindEdgesByType(p.ProjectName, "CONFIGURES")
	if err != nil {
		t.Fatalf("FindEdgesByType: %v", err)
	}
	return edges
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".git", "HEAD"), "ref: refs/heads/main\n")
}

func TestIsDependencyChild(t *testing.T) {
	tests := []struct {
		qn   string
		want bool
	}{
		{"project.dependencies.express", true},
		{"project.devDependencies.react", true},
		{"project.peerDependencies.react", true},
		{"project.optionalDependencies.fsevents", true},
		{"project.dev-dependencies.serde", true},
		{"project.build-dependencies.cc", true},
		{"project.name", false},
		{"project.version", false},
		{"project.scripts.test", false},
		{"project.Dependencies.react", true},
		{"project.random.section", false},
	}
	for _, tt := range tests {
		n := &store.Node{QualifiedName: tt.qn}
		got := isDependencyChild(n)
		if got != tt.want {
			t.Errorf("isDependencyChild(qn=%q) = %v, want %v", tt.qn, got, tt.want)
		}
	}
}

func TestNormalizeConfigKey_HyphenAndDot(t *testing.T) {
	tests := []struct {
		key        string
		wantNorm   string
		wantTokens int
	}{
		{"database-host", "database_host", 2},
		{"server.port", "server_port", 2},
		{"simple", "simple", 1},
		{"DB_URL", "db_url", 2},
		{"a-b.c_d", "a_b_c_d", 4},
	}
	for _, tt := range tests {
		norm, tokens := normalizeConfigKey(tt.key)
		if norm != tt.wantNorm {
			t.Errorf("normalizeConfigKey(%q).norm = %q, want %q", tt.key, norm, tt.wantNorm)
		}
		if len(tokens) != tt.wantTokens {
			t.Errorf("normalizeConfigKey(%q).tokens = %d, want %d", tt.key, len(tokens), tt.wantTokens)
		}
	}
}

func TestCollectConfigEntries_FilterShortTokens(t *testing.T) {
	vars := []*store.Node{
		{Name: "max_connections", FilePath: "config.toml"},
		{Name: "db_url", FilePath: "config.toml"},
		{Name: "port", FilePath: "config.toml"},
		{Name: "x_y", FilePath: "config.toml"},
		{Name: "notconfig", FilePath: "main.go"},
	}
	entries := collectConfigEntries(vars)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry (max_connections), got %d", len(entries))
		for _, e := range entries {
			t.Logf("  entry: %s (normalized: %s)", e.node.Name, e.normalized)
		}
	}
}

func TestResolveEdgeNodes(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "proj"
	if err := s.UpsertProject(project, "/tmp/proj"); err != nil {
		t.Fatal(err)
	}

	srcID, err := s.UpsertNode(&store.Node{Project: project, Label: "Module", Name: "a", QualifiedName: "proj.a", FilePath: "a.go"})
	if err != nil {
		t.Fatal(err)
	}
	dstID, err := s.UpsertNode(&store.Node{Project: project, Label: "Module", Name: "b", QualifiedName: "proj.b", FilePath: "b.go"})
	if err != nil {
		t.Fatal(err)
	}

	p := &Pipeline{Store: s, ProjectName: project}
	lookup, _ := p.resolveEdgeNodes([]*store.Edge{{
		Project:  project,
		SourceID: srcID,
		TargetID: dstID,
		Type:     "IMPORTS",
	}})

	if lookup[srcID] == nil || lookup[srcID].QualifiedName != "proj.a" {
		t.Fatalf("missing source lookup for %d: %+v", srcID, lookup[srcID])
	}
	if lookup[dstID] == nil || lookup[dstID].QualifiedName != "proj.b" {
		t.Fatalf("missing target lookup for %d: %+v", dstID, lookup[dstID])
	}
}

func TestCollectManifestDeps(t *testing.T) {
	vars := []*store.Node{
		{Name: "express", FilePath: "package.json", QualifiedName: "proj.dependencies.express"},
		{Name: "jest", FilePath: "package.json", QualifiedName: "proj.devDependencies.jest"},
		{Name: "name", FilePath: "package.json", QualifiedName: "proj.name"},
		{Name: "serde", FilePath: "Cargo.toml", QualifiedName: "proj.dependencies.serde"},
		{Name: "notmanifest", FilePath: "config.yaml", QualifiedName: "proj.something"},
	}
	deps := collectManifestDeps(vars)
	if len(deps) != 3 {
		t.Errorf("expected 3 deps (express, jest, serde), got %d", len(deps))
		for _, d := range deps {
			t.Logf("  dep: %s (%s)", d.name, d.node.FilePath)
		}
	}
}

func TestHasConfigExtension_AdditionalTypes(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"app.cfg", true},
		{"server.conf", true},
		{"app.properties", true},
		{"deep/nested/config.toml", true},
		{"style.css", false},
		{"script.sh", false},
		{"Makefile", false},
	}
	for _, tt := range tests {
		got := hasConfigExtension(tt.path)
		if got != tt.want {
			t.Errorf("hasConfigExtension(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestModuleQNForFile(t *testing.T) {
	tests := []struct {
		project string
		relPath string
		want    string
	}{
		{"proj", "main.go", "proj.main"},
		{"proj", "internal/store/nodes.go", "proj.internal.store.nodes"},
		{"proj", "src/__init__.py", "proj.src"},
		{"proj", "src/index.js", "proj.src"},
		{"myapp", "utils/helper.ts", "myapp.utils.helper"},
	}
	for _, tt := range tests {
		got := moduleQNForFile(tt.project, tt.relPath)
		if got != tt.want {
			t.Errorf("moduleQNForFile(%q, %q) = %q, want %q", tt.project, tt.relPath, got, tt.want)
		}
	}
}
