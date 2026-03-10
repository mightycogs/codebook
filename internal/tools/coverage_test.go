package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mightycogs/codebook/internal/pipeline"
	"github.com/mightycogs/codebook/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestResolveDetectRepo(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("success", func(t *testing.T) {
		st, repoPath, projName, toolErr := srv.resolveDetectRepo("test-project")
		if toolErr != nil {
			t.Fatalf("unexpected error: %v", toolErr)
		}
		if st == nil {
			t.Fatal("expected non-nil store")
		}
		if repoPath == "" {
			t.Fatal("expected non-empty repo path")
		}
		if projName == "" {
			t.Fatal("expected non-empty project name")
		}
	})

	t.Run("no_project", func(t *testing.T) {
		tmpDir := t.TempDir()
		router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(router.CloseAll)
		noSrv := NewServer(router)
		_, _, _, toolErr := noSrv.resolveDetectRepo("")
		if toolErr == nil {
			t.Fatal("expected error when no session project and empty arg")
		}
	})

	t.Run("no_projects_in_store", func(t *testing.T) {
		tmpDir := t.TempDir()
		router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(router.CloseAll)
		noSrv := NewServer(router)
		noSrv.sessionProject = "empty-proj"
		st2, err2 := router.ForProject("empty-proj")
		if err2 != nil {
			t.Fatal(err2)
		}
		_ = st2
		_, _, _, toolErr := noSrv.resolveDetectRepo("empty-proj")
		if toolErr == nil {
			t.Fatal("expected error for store with no projects")
		}
	})

	t.Run("project_no_root_path", func(t *testing.T) {
		tmpDir := t.TempDir()
		router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(router.CloseAll)
		noSrv := NewServer(router)
		noSrv.sessionProject = "no-root"
		st2, err2 := router.ForProject("no-root")
		if err2 != nil {
			t.Fatal(err2)
		}
		if err := st2.UpsertProject("no-root", ""); err != nil {
			t.Fatal(err)
		}
		_, _, _, toolErr := noSrv.resolveDetectRepo("no-root")
		if toolErr == nil {
			t.Fatal("expected error for project with no root_path")
		}
	})
}

func TestResolveProjectRoot_ExplicitProjectLookup(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("explicit_project_from_store", func(t *testing.T) {
		srv2 := &Server{
			router:         srv.router,
			handlers:       make(map[string]mcp.ToolHandler),
			sessionProject: "",
			sessionRoot:    "",
		}
		root, err := srv2.resolveProjectRoot("test-project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if root == "" {
			t.Fatal("expected non-empty root")
		}
	})

	t.Run("no_session_no_project", func(t *testing.T) {
		tmpDir := t.TempDir()
		router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(router.CloseAll)
		noSrv := NewServer(router)
		_, err = noSrv.resolveProjectRoot("")
		if err == nil {
			t.Fatal("expected error for no session and no project")
		}
	})

	t.Run("project_not_found", func(t *testing.T) {
		srv2, _ := testServerWithProject(t)
		srv2.sessionProject = ""
		srv2.sessionRoot = ""
		_, err := srv2.resolveProjectRoot("nonexistent-proj")
		if err == nil {
			t.Fatal("expected error for non-existent project")
		}
	})

	t.Run("empty_projects_in_store", func(t *testing.T) {
		tmpDir := t.TempDir()
		router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(router.CloseAll)
		noSrv := NewServer(router)
		noSrv.sessionProject = "empty-proj"
		noSrv.sessionRoot = ""
		st, stErr := router.ForProject("empty-proj")
		if stErr != nil {
			t.Fatal(stErr)
		}
		_ = st
		_, err = noSrv.resolveProjectRoot("")
		if err == nil {
			t.Fatal("expected error for store with no project rows")
		}
	})
}

func TestAutoResolveBest_EdgeCases(t *testing.T) {
	srv := testSnippetServer(t)

	t.Run("nil_store_project", func(t *testing.T) {
		tmpDir := t.TempDir()
		router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(router.CloseAll)
		noSrv := &Server{
			router:         router,
			handlers:       make(map[string]mcp.ToolHandler),
			sessionProject: "nonexistent",
		}
		candidates := []*store.Node{
			{ID: 1, Name: "A", QualifiedName: "pkg.A"},
			{ID: 2, Name: "B", QualifiedName: "pkg.B"},
		}
		result := noSrv.autoResolveBest(candidates, "nonexistent")
		if result != nil {
			t.Error("expected nil for non-existent project")
		}
	})

	t.Run("empty_projects_in_store", func(t *testing.T) {
		tmpDir := t.TempDir()
		router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(router.CloseAll)
		noSrv := &Server{
			router:         router,
			handlers:       make(map[string]mcp.ToolHandler),
			sessionProject: "empty-proj",
		}
		st, stErr := router.ForProject("empty-proj")
		if stErr != nil {
			t.Fatal(stErr)
		}
		_ = st
		candidates := []*store.Node{
			{ID: 1, Name: "A", QualifiedName: "pkg.A"},
		}
		result := noSrv.autoResolveBest(candidates, "")
		if result != nil {
			t.Error("expected nil for store with no project rows")
		}
	})

	t.Run("prefer_non_test_file", func(t *testing.T) {
		st, stErr := srv.router.ForProject("test-project")
		if stErr != nil {
			t.Fatal(stErr)
		}
		st.UpsertNode(&store.Node{
			Project:       "test-project",
			Label:         "Function",
			Name:          "HelperCov",
			QualifiedName: "test-project.helpercov",
			FilePath:      "helper.go",
			StartLine:     1,
			EndLine:       3,
			Properties:    map[string]any{},
		})
		st.UpsertNode(&store.Node{
			Project:       "test-project",
			Label:         "Function",
			Name:          "HelperCov",
			QualifiedName: "test-project.helpercov_test",
			FilePath:      "helper_test.go",
			StartLine:     1,
			EndLine:       3,
			Properties:    map[string]any{},
		})
		nodes, _ := st.FindNodesByName("test-project", "HelperCov")
		if len(nodes) < 2 {
			t.Fatal("expected 2 nodes for HelperCov")
		}
		result := srv.autoResolveBest(nodes, "")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if result.node.FilePath == "helper_test.go" {
			t.Error("expected non-test file to be preferred")
		}
	})

	t.Run("tiebreaker_alphabetical", func(t *testing.T) {
		st, stErr := srv.router.ForProject("test-project")
		if stErr != nil {
			t.Fatal(stErr)
		}
		st.UpsertNode(&store.Node{
			Project:       "test-project",
			Label:         "Function",
			Name:          "ZzzCov",
			QualifiedName: "test-project.alpha.ZzzCov",
			FilePath:      "alpha.go",
			StartLine:     1,
			EndLine:       3,
			Properties:    map[string]any{},
		})
		st.UpsertNode(&store.Node{
			Project:       "test-project",
			Label:         "Function",
			Name:          "ZzzCov",
			QualifiedName: "test-project.beta.ZzzCov",
			FilePath:      "beta.go",
			StartLine:     1,
			EndLine:       3,
			Properties:    map[string]any{},
		})
		nodes, _ := st.FindNodesByName("test-project", "ZzzCov")
		if len(nodes) < 2 {
			t.Fatal("expected 2 nodes for ZzzCov")
		}
		result := srv.autoResolveBest(nodes, "")
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if result.node.QualifiedName != "test-project.alpha.ZzzCov" {
			t.Errorf("expected alphabetical winner, got %s", result.node.QualifiedName)
		}
	})
}

func TestResolveByFile_FallbackToOverlap(t *testing.T) {
	srv, projName := testServerWithProject(t)
	st, err := srv.resolveStore("")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("fallback_overlap", func(t *testing.T) {
		st.UpsertNode(&store.Node{
			Project:       projName,
			Label:         "Function",
			Name:          "SpecialFunc",
			QualifiedName: projName + ".special.SpecialFunc",
			FilePath:      "special.go",
			StartLine:     1,
			EndLine:       10,
			Properties:    map[string]any{},
		})
		nodes := resolveByFile(st, projName, "special.go")
		if len(nodes) == 0 {
			t.Fatal("expected nodes from overlap fallback")
		}
	})

	t.Run("unknown_file", func(t *testing.T) {
		nodes := resolveByFile(st, projName, "doesnotexist.go")
		if len(nodes) != 0 {
			t.Errorf("expected empty for unknown file, got %d", len(nodes))
		}
	})
}

func TestResolveByHunks_ErrorPath(t *testing.T) {
	srv, projName := testServerWithProject(t)
	st, err := srv.resolveStore("")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("valid_hunks", func(t *testing.T) {
		hunks := []pipeline.ChangedHunk{
			{Path: "main.go", StartLine: 3, EndLine: 5},
		}
		nodes := resolveByHunks(st, projName, "main.go", hunks)
		if len(nodes) == 0 {
			t.Fatal("expected nodes for valid hunks")
		}
	})

	t.Run("no_overlap", func(t *testing.T) {
		hunks := []pipeline.ChangedHunk{
			{Path: "main.go", StartLine: 99999, EndLine: 99999},
		}
		nodes := resolveByHunks(st, projName, "main.go", hunks)
		if len(nodes) != 0 {
			t.Errorf("expected empty for non-overlapping hunks, got %d", len(nodes))
		}
	})
}

func TestTraceImpact_BFSError(t *testing.T) {
	srv, _ := testServerWithProject(t)
	st, err := srv.resolveStore("")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("dedup_best_hop", func(t *testing.T) {
		hrNodes, _ := st.FindNodesByName("test-project", "HandleRequest")
		poNodes, _ := st.FindNodesByName("test-project", "ProcessOrder")
		if len(hrNodes) == 0 || len(poNodes) == 0 {
			t.Fatal("need both nodes")
		}
		allChanged := append(hrNodes, poNodes...)
		impacted, edges := traceImpact(st, allChanged, 3)
		_ = edges
		for _, is := range impacted {
			if is.Node.Name == "HandleRequest" || is.Node.Name == "ProcessOrder" {
				t.Errorf("changed node %s should not appear in impacted", is.Node.Name)
			}
		}
	})

	t.Run("sort_by_hop_then_name", func(t *testing.T) {
		poNodes, _ := st.FindNodesByName("test-project", "ProcessOrder")
		if len(poNodes) == 0 {
			t.Fatal("need ProcessOrder")
		}
		impacted, _ := traceImpact(st, poNodes, 3)
		for i := 1; i < len(impacted); i++ {
			if impacted[i].Hop < impacted[i-1].Hop {
				t.Errorf("not sorted by hop: %d < %d", impacted[i].Hop, impacted[i-1].Hop)
			}
			if impacted[i].Hop == impacted[i-1].Hop && impacted[i].Node.Name < impacted[i-1].Node.Name {
				t.Errorf("not sorted by name at same hop: %s < %s", impacted[i].Node.Name, impacted[i-1].Node.Name)
			}
		}
	})
}

func TestHandleADRDelete_ErrorCase(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("delete_nonexistent_returns_error", func(t *testing.T) {
		result := callToolRaw(t, srv, "manage_adr", map[string]any{
			"mode": "delete",
		})
		if !result.IsError {
			t.Fatal("expected error for deleting nonexistent ADR")
		}
	})

	t.Run("delete_existing_succeeds", func(t *testing.T) {
		adrContent := "## PURPOSE\nTest\n\n## STACK\nGo\n\n## ARCHITECTURE\nSimple\n\n## PATTERNS\nMVC\n\n## TRADEOFFS\nSpeed\n\n## PHILOSOPHY\nKISS"
		callTool(t, srv, "manage_adr", map[string]any{
			"mode":    "store",
			"content": adrContent,
		})
		data := callTool(t, srv, "manage_adr", map[string]any{
			"mode": "delete",
		})
		if data["status"] != "deleted" {
			t.Errorf("status = %v, want deleted", data["status"])
		}
	})
}

func TestHandleIndexRepository(t *testing.T) {
	tmpDir := t.TempDir()
	routerDir := filepath.Join(tmpDir, "db")
	projRoot := filepath.Join(tmpDir, "myrepo")
	if err := os.MkdirAll(projRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	srcContent := `package main

func Hello() string {
	return "hello"
}
`
	if err := os.WriteFile(filepath.Join(projRoot, "main.go"), []byte(srcContent), 0o600); err != nil {
		t.Fatal(err)
	}

	router, err := store.NewRouterWithDir(routerDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)

	srv := NewServer(router)
	srv.sessionProject = pipeline.ProjectNameFromPath(projRoot)
	srv.sessionRoot = projRoot

	t.Run("success", func(t *testing.T) {
		data := callTool(t, srv, "index_repository", map[string]any{
			"repo_path": projRoot,
		})
		if data["project"] == nil {
			t.Fatal("expected project in response")
		}
		if data["nodes"] == nil {
			t.Fatal("expected nodes in response")
		}
		if data["edges"] == nil {
			t.Fatal("expected edges in response")
		}
		if data["indexed_at"] == nil {
			t.Fatal("expected indexed_at")
		}
		if data["adr_present"] != false {
			t.Errorf("expected adr_present=false, got %v", data["adr_present"])
		}
		if data["adr_hint"] == nil {
			t.Error("expected adr_hint for new project")
		}
	})

	t.Run("missing_repo_path_no_session", func(t *testing.T) {
		tmpDir2 := t.TempDir()
		router2, err := store.NewRouterWithDir(filepath.Join(tmpDir2, "db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(router2.CloseAll)
		srv2 := NewServer(router2)
		result := callToolRaw(t, srv2, "index_repository", map[string]any{})
		if !result.IsError {
			t.Fatal("expected error for missing repo_path and no session")
		}
	})

	t.Run("session_fallback", func(t *testing.T) {
		data := callTool(t, srv, "index_repository", map[string]any{})
		if data["project"] == nil {
			t.Fatal("expected project in response with session fallback")
		}
	})

	t.Run("invalid_mode", func(t *testing.T) {
		result := callToolRaw(t, srv, "index_repository", map[string]any{
			"repo_path": projRoot,
			"mode":      "invalid",
		})
		if !result.IsError {
			t.Fatal("expected error for invalid mode")
		}
	})

	t.Run("fast_mode", func(t *testing.T) {
		data := callTool(t, srv, "index_repository", map[string]any{
			"repo_path": projRoot,
			"mode":      "fast",
		})
		if data["mode"] != "fast" {
			t.Errorf("mode = %v, want fast", data["mode"])
		}
	})
}

func TestHandleDetectChanges(t *testing.T) {
	tmpDir := t.TempDir()
	routerDir := filepath.Join(tmpDir, "db")
	projRoot := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(projRoot, 0o750); err != nil {
		t.Fatal(err)
	}

	srcContent := `package main

func Hello() string {
	return "hello"
}
`
	if err := os.WriteFile(filepath.Join(projRoot, "main.go"), []byte(srcContent), 0o600); err != nil {
		t.Fatal(err)
	}

	if out, err := runGitCmd(projRoot, "init"); err != nil {
		t.Skipf("git init failed: %s %v", out, err)
	}
	if out, err := runGitCmd(projRoot, "add", "."); err != nil {
		t.Skipf("git add failed: %s %v", out, err)
	}
	if out, err := runGitCmd(projRoot, "commit", "-m", "initial"); err != nil {
		t.Skipf("git commit failed: %s %v", out, err)
	}

	router, err := store.NewRouterWithDir(routerDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)

	projName := pipeline.ProjectNameFromPath(projRoot)
	srv := NewServer(router)
	srv.sessionProject = projName
	srv.sessionRoot = projRoot

	callTool(t, srv, "index_repository", map[string]any{
		"repo_path": projRoot,
	})

	t.Run("no_changes", func(t *testing.T) {
		data := callTool(t, srv, "detect_changes", map[string]any{})
		summary := data["summary"].(map[string]any)
		if summary["total"] != float64(0) {
			t.Errorf("expected 0 total, got %v", summary["total"])
		}
	})

	t.Run("with_changes", func(t *testing.T) {
		newContent := `package main

func Hello() string {
	return "world"
}

func NewFunc() {}
`
		if err := os.WriteFile(filepath.Join(projRoot, "main.go"), []byte(newContent), 0o600); err != nil {
			t.Fatal(err)
		}
		data := callTool(t, srv, "detect_changes", map[string]any{
			"scope": "unstaged",
		})
		if data["changed_files"] == nil {
			t.Fatal("expected changed_files")
		}
		if data["summary"] == nil {
			t.Fatal("expected summary")
		}
	})

	t.Run("staged_scope", func(t *testing.T) {
		if out, err := runGitCmd(projRoot, "add", "main.go"); err != nil {
			t.Skipf("git add failed: %s %v", out, err)
		}
		data := callTool(t, srv, "detect_changes", map[string]any{
			"scope": "staged",
		})
		if data["summary"] == nil {
			t.Fatal("expected summary")
		}
	})

	t.Run("depth_clamping", func(t *testing.T) {
		data := callTool(t, srv, "detect_changes", map[string]any{
			"scope": "all",
			"depth": float64(0),
		})
		if data["summary"] == nil {
			t.Fatal("expected summary")
		}
	})

	t.Run("depth_max_clamping", func(t *testing.T) {
		data := callTool(t, srv, "detect_changes", map[string]any{
			"scope": "all",
			"depth": float64(99),
		})
		if data["summary"] == nil {
			t.Fatal("expected summary")
		}
	})
}

func runGitCmd(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	return cmd.CombinedOutput()
}

func TestHandleIndexStatus_Partial(t *testing.T) {
	tmpDir := t.TempDir()
	routerDir := filepath.Join(tmpDir, "db")
	router, err := store.NewRouterWithDir(routerDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)

	projName := "partial-proj"
	st, err := router.ForProject(projName)
	if err != nil {
		t.Fatal(err)
	}
	_ = st

	srv := NewServer(router)
	srv.sessionProject = projName

	data := callTool(t, srv, "index_status", map[string]any{})
	if data["status"] != "partial" {
		t.Errorf("status = %v, want partial", data["status"])
	}
}

func TestHandleIndexStatus_IndexStartedAt(t *testing.T) {
	srv, _ := testServerWithProject(t)
	srv.indexStatus.Store("indexing")

	data := callTool(t, srv, "index_status", map[string]any{})
	if data["status"] != "indexing" {
		t.Errorf("status = %v, want indexing", data["status"])
	}
}

func TestHandleADRStore_TooLong(t *testing.T) {
	srv, _ := testServerWithProject(t)

	longContent := make([]byte, store.MaxADRLength()+1)
	for i := range longContent {
		longContent[i] = 'x'
	}
	result := callToolRaw(t, srv, "manage_adr", map[string]any{
		"mode":    "store",
		"content": string(longContent),
	})
	if !result.IsError {
		t.Fatal("expected error for too-long ADR")
	}
}

func TestHandleTraceCallPath_DepthClamping(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("depth_below_min", func(t *testing.T) {
		data := callTool(t, srv, "trace_call_path", map[string]any{
			"function_name": "HandleRequest",
			"depth":         float64(0),
		})
		if data["root"] == nil {
			t.Fatal("expected root")
		}
	})

	t.Run("depth_above_max", func(t *testing.T) {
		data := callTool(t, srv, "trace_call_path", map[string]any{
			"function_name": "HandleRequest",
			"depth":         float64(100),
		})
		if data["root"] == nil {
			t.Fatal("expected root")
		}
	})
}

func TestHandleSearchCode_ResolveError(t *testing.T) {
	tmpDir := t.TempDir()
	router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)
	srv := NewServer(router)

	result := callToolRaw(t, srv, "search_code", map[string]any{
		"pattern": "test",
	})
	if !result.IsError {
		t.Fatal("expected error when no session project")
	}
}

func TestCollectSearchFilePaths_WithGlob(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("no_glob", func(t *testing.T) {
		paths := srv.collectSearchFilePaths("", "")
		if len(paths) == 0 {
			t.Fatal("expected file paths from indexed project")
		}
	})

	t.Run("with_matching_glob", func(t *testing.T) {
		paths := srv.collectSearchFilePaths("*.go", "")
		foundGo := false
		for _, p := range paths {
			if filepath.Ext(p) == ".go" {
				foundGo = true
			}
		}
		if !foundGo && len(paths) > 0 {
			t.Error("expected .go files with *.go glob")
		}
	})

	t.Run("non_matching_glob", func(t *testing.T) {
		paths := srv.collectSearchFilePaths("*.xyz", "")
		if len(paths) != 0 {
			t.Errorf("expected no paths for *.xyz glob, got %d", len(paths))
		}
	})
}

func TestFindSimilarNodes_Branches(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("no_project", func(t *testing.T) {
		noSrv := &Server{
			router:   srv.router,
			handlers: make(map[string]mcp.ToolHandler),
		}
		nodes := noSrv.findSimilarNodes("Handle", "", 5)
		if len(nodes) != 0 {
			t.Errorf("expected empty for no project, got %d", len(nodes))
		}
	})

	t.Run("nonexistent_project", func(t *testing.T) {
		nodes := srv.findSimilarNodes("Handle", "nonexistent", 5)
		if len(nodes) != 0 {
			t.Errorf("expected empty for nonexistent project, got %d", len(nodes))
		}
	})
}

func TestHandleGetArchitecture_ResolveStoreError(t *testing.T) {
	tmpDir := t.TempDir()
	router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)
	srv := NewServer(router)

	result := callToolRaw(t, srv, "get_architecture", map[string]any{})
	if !result.IsError {
		t.Fatal("expected error when no session project")
	}
}

func TestHandleManageADR_ResolveStoreError(t *testing.T) {
	tmpDir := t.TempDir()
	router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)
	srv := NewServer(router)

	result := callToolRaw(t, srv, "manage_adr", map[string]any{
		"mode": "get",
	})
	if !result.IsError {
		t.Fatal("expected error when no session project")
	}
}

func TestHandleQueryGraph_ResolveStoreError(t *testing.T) {
	tmpDir := t.TempDir()
	router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)
	srv := NewServer(router)

	result := callToolRaw(t, srv, "query_graph", map[string]any{
		"query": "MATCH (f:Function) RETURN f.name LIMIT 1",
	})
	if !result.IsError {
		t.Fatal("expected error when no session project")
	}
}

func TestHandleSearchGraph_ResolveStoreError(t *testing.T) {
	tmpDir := t.TempDir()
	router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)
	srv := NewServer(router)

	result := callToolRaw(t, srv, "search_graph", map[string]any{
		"name_pattern": ".*",
	})
	if !result.IsError {
		t.Fatal("expected error when no session project")
	}
}

func TestHandleGetGraphSchema_ResolveStoreError(t *testing.T) {
	tmpDir := t.TempDir()
	router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)
	srv := NewServer(router)

	result := callToolRaw(t, srv, "get_graph_schema", map[string]any{})
	if !result.IsError {
		t.Fatal("expected error when no session project")
	}
}

func TestHandleIngestTraces_StoreError(t *testing.T) {
	tmpDir := t.TempDir()
	router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)
	srv := NewServer(router)

	result := callToolRaw(t, srv, "ingest_traces", map[string]any{
		"project":   "nonexistent",
		"file_path": "/tmp/traces.json",
	})
	if !result.IsError {
		t.Fatal("expected error for nonexistent project store")
	}
}

func TestBuildSnippetResponse_ErrorPaths(t *testing.T) {
	srv := testSnippetServer(t)

	t.Run("no_file_path", func(t *testing.T) {
		match := &snippetMatch{
			node:    &store.Node{Name: "NoFile", QualifiedName: "pkg.NoFile", FilePath: ""},
			project: "test-project",
			method:  "exact",
		}
		result, err := srv.buildSnippetResponse(match, false, nil)
		if err != nil {
			t.Fatalf("unexpected go error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected tool error for no file path")
		}
	})

	t.Run("no_line_range", func(t *testing.T) {
		match := &snippetMatch{
			node:    &store.Node{Name: "NoLines", QualifiedName: "pkg.NoLines", FilePath: "main.go", StartLine: 0, EndLine: 0},
			project: "test-project",
			method:  "exact",
		}
		result, err := srv.buildSnippetResponse(match, false, nil)
		if err != nil {
			t.Fatalf("unexpected go error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected tool error for no line range")
		}
	})
}

func TestResolveSnippetNode_CrossProjectError(t *testing.T) {
	srv := testSnippetServer(t)

	t.Run("wildcard", func(t *testing.T) {
		_, _, err := srv.resolveSnippetNode("HandleRequest", "*")
		if err == nil {
			t.Fatal("expected error for wildcard project")
		}
	})

	t.Run("all", func(t *testing.T) {
		_, _, err := srv.resolveSnippetNode("HandleRequest", "all")
		if err == nil {
			t.Fatal("expected error for 'all' project")
		}
	})

	t.Run("no_session_no_project", func(t *testing.T) {
		tmpDir := t.TempDir()
		router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(router.CloseAll)
		noSrv := &Server{
			router:   router,
			handlers: make(map[string]mcp.ToolHandler),
		}
		_, _, resolveErr := noSrv.resolveSnippetNode("HandleRequest", "")
		if resolveErr == nil {
			t.Fatal("expected error for no session and no project")
		}
	})

	t.Run("project_not_found", func(t *testing.T) {
		_, _, err := srv.resolveSnippetNode("HandleRequest", "nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent project")
		}
	})
}

func TestHandleDetectChanges_NoProject(t *testing.T) {
	tmpDir := t.TempDir()
	router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)
	srv := NewServer(router)

	result := callToolRaw(t, srv, "detect_changes", map[string]any{})
	if !result.IsError {
		t.Fatal("expected error when no session project")
	}
}

func TestHandleListProjects_StoreError(t *testing.T) {
	srv, _ := testServerWithProject(t)
	data := callTool(t, srv, "list_projects", map[string]any{})
	arr := data["_array"].([]any)
	if len(arr) == 0 {
		t.Fatal("expected at least one project")
	}
	proj := arr[0].(map[string]any)
	if proj["adr_present"] == nil {
		t.Error("expected adr_present field")
	}
}

func TestHandleDeleteProject_StoreDeleteError(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("delete_existing", func(t *testing.T) {
		data := callTool(t, srv, "delete_project", map[string]any{
			"project_name": "test-project",
		})
		if data["status"] != "ok" {
			t.Errorf("status = %v", data["status"])
		}
	})
}

func TestRunTraceBFS_Branches(t *testing.T) {
	srv, _ := testServerWithProject(t)
	st, err := srv.resolveStore("")
	if err != nil {
		t.Fatal(err)
	}
	nodes, _ := st.FindNodesByName("test-project", "HandleRequest")
	if len(nodes) == 0 {
		t.Fatal("need HandleRequest")
	}
	rootID := nodes[0].ID

	t.Run("both_with_confidence", func(t *testing.T) {
		visited, edges, bfsErr := runTraceBFS(st, rootID, "both", []string{"CALLS"}, 3, 0.5)
		if bfsErr != nil {
			t.Fatalf("unexpected error: %v", bfsErr)
		}
		_ = visited
		_ = edges
	})

	t.Run("outbound_with_confidence", func(t *testing.T) {
		visited, edges, bfsErr := runTraceBFS(st, rootID, "outbound", []string{"CALLS"}, 3, 0.5)
		if bfsErr != nil {
			t.Fatalf("unexpected error: %v", bfsErr)
		}
		_ = visited
		_ = edges
	})
}

func TestHandleTraceCallPath_StoreError(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("inbound_direction", func(t *testing.T) {
		data := callTool(t, srv, "trace_call_path", map[string]any{
			"function_name": "ProcessOrder",
			"direction":     "inbound",
		})
		if data["root"] == nil {
			t.Fatal("expected root")
		}
	})
}

func TestHandleSearchGraph_ExcludeLabelsArray(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("custom_exclude", func(t *testing.T) {
		data := callTool(t, srv, "search_graph", map[string]any{
			"label":          "Function",
			"exclude_labels": []any{"Module"},
		})
		if data["results"] == nil {
			t.Fatal("expected results")
		}
	})

	t.Run("sort_by_name", func(t *testing.T) {
		data := callTool(t, srv, "search_graph", map[string]any{
			"label":   "Function",
			"sort_by": "name",
		})
		if data["results"] == nil {
			t.Fatal("expected results")
		}
	})

	t.Run("sort_by_degree", func(t *testing.T) {
		data := callTool(t, srv, "search_graph", map[string]any{
			"label":   "Function",
			"sort_by": "degree",
		})
		if data["results"] == nil {
			t.Fatal("expected results")
		}
	})
}

func callToolGetText(t *testing.T, srv *Server, name string, args map[string]any) string {
	t.Helper()
	rawArgs, _ := json.Marshal(args)
	result, err := srv.CallTool(context.Background(), name, rawArgs)
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", name, err)
	}
	if len(result.Content) == 0 {
		t.Fatalf("CallTool(%s) empty content", name)
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}
