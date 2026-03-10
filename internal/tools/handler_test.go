package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mightycogs/codebook/internal/pipeline"
	"github.com/mightycogs/codebook/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func testServerWithProject(t *testing.T) (*Server, string) {
	t.Helper()

	tmpDir := t.TempDir()
	routerDir := filepath.Join(tmpDir, "db")
	projRoot := filepath.Join(tmpDir, "project")

	if err := os.MkdirAll(projRoot, 0o750); err != nil {
		t.Fatal(err)
	}

	srcContent := `package main

func HandleRequest() error {
	return nil
}

func ProcessOrder(id int) {
	// process
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

	projName := "test-project"
	st, err := router.ForProject(projName)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertProject(projName, projRoot); err != nil {
		t.Fatal(err)
	}

	idHR, _ := st.UpsertNode(&store.Node{
		Project:       projName,
		Label:         "Function",
		Name:          "HandleRequest",
		QualifiedName: "test-project.cmd.server.main.HandleRequest",
		FilePath:      "main.go",
		StartLine:     3,
		EndLine:       5,
		Properties: map[string]any{
			"signature":      "func HandleRequest() error",
			"return_type":    "error",
			"is_exported":    true,
			"is_entry_point": true,
		},
	})
	idPO, _ := st.UpsertNode(&store.Node{
		Project:       projName,
		Label:         "Function",
		Name:          "ProcessOrder",
		QualifiedName: "test-project.cmd.server.main.ProcessOrder",
		FilePath:      "main.go",
		StartLine:     7,
		EndLine:       9,
		Properties:    map[string]any{"signature": "func ProcessOrder(id int)"},
	})
	st.UpsertNode(&store.Node{
		Project:       projName,
		Label:         "File",
		Name:          "main.go",
		QualifiedName: "test-project.main.go",
		FilePath:      "main.go",
	})
	st.UpsertNode(&store.Node{
		Project:       projName,
		Label:         "Module",
		Name:          "main",
		QualifiedName: "test-project.cmd.server.main",
		FilePath:      "main.go",
		Properties:    map[string]any{"constants": "MAX_SIZE=100"},
	})

	_, _ = st.InsertEdge(&store.Edge{Project: projName, SourceID: idHR, TargetID: idPO, Type: "CALLS"})

	srv := NewServer(router)
	srv.sessionProject = projName
	srv.sessionRoot = projRoot

	return srv, projName
}

func callTool(t *testing.T, srv *Server, name string, args map[string]any) map[string]any {
	t.Helper()
	rawArgs, _ := json.Marshal(args)
	result, err := srv.CallTool(context.Background(), name, rawArgs)
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", name, err)
	}
	if result.IsError {
		tc, _ := result.Content[0].(*mcp.TextContent)
		t.Fatalf("CallTool(%s) returned error: %s", name, tc.Text)
	}
	if len(result.Content) == 0 {
		t.Fatalf("CallTool(%s) empty content", name)
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &data); err != nil {
		var arr []any
		if err2 := json.Unmarshal([]byte(tc.Text), &arr); err2 != nil {
			t.Fatalf("unmarshal result: %v (text: %s)", err, tc.Text)
		}
		return map[string]any{"_array": arr}
	}
	return data
}

func callToolRaw(t *testing.T, srv *Server, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	rawArgs, _ := json.Marshal(args)
	result, err := srv.CallTool(context.Background(), name, rawArgs)
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", name, err)
	}
	return result
}

func TestNewServer(t *testing.T) {
	tmpDir := t.TempDir()
	router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)

	srv := NewServer(router)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.MCPServer() == nil {
		t.Error("MCPServer() returned nil")
	}
	if srv.Router() == nil {
		t.Error("Router() returned nil")
	}
	if srv.SessionProject() != "" {
		t.Errorf("SessionProject() = %q, want empty", srv.SessionProject())
	}
}

func TestSetSessionRoot(t *testing.T) {
	tmpDir := t.TempDir()
	router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)

	srv := NewServer(router)
	srv.SetSessionRoot("/tmp/my-project")
	if srv.SessionProject() == "" {
		t.Error("SessionProject() should be set after SetSessionRoot")
	}
	if srv.sessionRoot != "/tmp/my-project" {
		t.Errorf("sessionRoot = %q, want %q", srv.sessionRoot, "/tmp/my-project")
	}

	srv.SetSessionRoot("/tmp/another-project")
	if srv.sessionRoot != "/tmp/my-project" {
		t.Error("SetSessionRoot should only work once (sync.Once)")
	}
}

func TestToolNames(t *testing.T) {
	tmpDir := t.TempDir()
	router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)

	srv := NewServer(router)
	names := srv.ToolNames()
	if len(names) == 0 {
		t.Fatal("expected registered tool names")
	}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("tool names not sorted: %q before %q", names[i-1], names[i])
		}
	}
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	expected := []string{"get_architecture", "search_graph", "trace_call_path", "get_code_snippet", "query_graph", "list_projects", "index_status", "search_code"}
	for _, e := range expected {
		if !found[e] {
			t.Errorf("missing tool: %s", e)
		}
	}
}

func TestCallTool_UnknownTool(t *testing.T) {
	tmpDir := t.TempDir()
	router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.CloseAll)

	srv := NewServer(router)
	_, err = srv.CallTool(context.Background(), "nonexistent_tool", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestCallTool_EmptyArgs(t *testing.T) {
	srv, _ := testServerWithProject(t)
	result, err := srv.CallTool(context.Background(), "list_projects", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("list_projects with nil args should not error")
	}
}

func TestHandleListProjects(t *testing.T) {
	srv, _ := testServerWithProject(t)
	result := callToolRaw(t, srv, "list_projects", map[string]any{})
	if result.IsError {
		t.Fatal("expected success")
	}
	tc, _ := result.Content[0].(*mcp.TextContent)
	var arr []any
	if err := json.Unmarshal([]byte(tc.Text), &arr); err != nil {
		t.Fatalf("expected array: %v", err)
	}
	if len(arr) == 0 {
		t.Fatal("expected at least one project")
	}
	proj := arr[0].(map[string]any)
	if proj["name"] != "test-project" {
		t.Errorf("project name = %v, want test-project", proj["name"])
	}
	if proj["is_session_project"] != true {
		t.Error("expected is_session_project=true")
	}
}

func TestHandleDeleteProject(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("missing_name", func(t *testing.T) {
		result := callToolRaw(t, srv, "delete_project", map[string]any{})
		if !result.IsError {
			t.Fatal("expected error for missing project_name")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		result := callToolRaw(t, srv, "delete_project", map[string]any{"project_name": "nonexistent"})
		if !result.IsError {
			t.Fatal("expected error for nonexistent project")
		}
	})

	t.Run("success", func(t *testing.T) {
		data := callTool(t, srv, "delete_project", map[string]any{"project_name": "test-project"})
		if data["status"] != "ok" {
			t.Errorf("status = %v, want ok", data["status"])
		}
	})
}

func TestHandleIndexStatus(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("existing_project", func(t *testing.T) {
		data := callTool(t, srv, "index_status", map[string]any{})
		if data["project"] != "test-project" {
			t.Errorf("project = %v", data["project"])
		}
		if data["status"] != "ready" {
			t.Errorf("status = %v, want ready", data["status"])
		}
	})

	t.Run("not_indexed", func(t *testing.T) {
		data := callTool(t, srv, "index_status", map[string]any{"project": "unknown-proj"})
		if data["status"] != "not_indexed" {
			t.Errorf("status = %v, want not_indexed", data["status"])
		}
	})

	t.Run("no_session", func(t *testing.T) {
		tmpDir := t.TempDir()
		router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(router.CloseAll)
		noSessionSrv := NewServer(router)
		data := callTool(t, noSessionSrv, "index_status", map[string]any{})
		if data["status"] != "no_session" {
			t.Errorf("status = %v, want no_session", data["status"])
		}
	})
}

func TestHandleGetGraphSchema(t *testing.T) {
	srv, _ := testServerWithProject(t)
	data := callTool(t, srv, "get_graph_schema", map[string]any{})
	projects, ok := data["projects"].([]any)
	if !ok {
		t.Fatalf("expected projects array, got %T", data["projects"])
	}
	if len(projects) == 0 {
		t.Fatal("expected at least one project schema")
	}
}

func TestHandleSearchGraph(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("by_name_pattern", func(t *testing.T) {
		data := callTool(t, srv, "search_graph", map[string]any{
			"name_pattern": "Handle.*",
		})
		results, ok := data["results"].([]any)
		if !ok {
			t.Fatalf("expected results array, got %T", data["results"])
		}
		if len(results) == 0 {
			t.Fatal("expected at least one result for Handle.*")
		}
	})

	t.Run("by_label", func(t *testing.T) {
		data := callTool(t, srv, "search_graph", map[string]any{
			"label": "Function",
		})
		results := data["results"].([]any)
		if len(results) == 0 {
			t.Fatal("expected Function results")
		}
	})

	t.Run("pagination", func(t *testing.T) {
		data := callTool(t, srv, "search_graph", map[string]any{
			"label":  "Function",
			"limit":  float64(1),
			"offset": float64(0),
		})
		if data["limit"] != float64(1) {
			t.Errorf("limit = %v", data["limit"])
		}
	})

	t.Run("exclude_labels_empty", func(t *testing.T) {
		data := callTool(t, srv, "search_graph", map[string]any{
			"exclude_labels": []any{},
		})
		if data["results"] == nil {
			t.Fatal("expected results")
		}
	})
}

func TestHandleQueryGraph(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("valid_query", func(t *testing.T) {
		data := callTool(t, srv, "query_graph", map[string]any{
			"query": "MATCH (f:Function) RETURN f.name LIMIT 5",
		})
		if data["columns"] == nil {
			t.Error("expected columns")
		}
		if data["rows"] == nil {
			t.Error("expected rows")
		}
	})

	t.Run("missing_query", func(t *testing.T) {
		result := callToolRaw(t, srv, "query_graph", map[string]any{})
		if !result.IsError {
			t.Fatal("expected error for missing query")
		}
	})
}

func TestHandleTraceCallPath(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("basic_trace", func(t *testing.T) {
		data := callTool(t, srv, "trace_call_path", map[string]any{
			"function_name": "HandleRequest",
			"direction":     "outbound",
			"depth":         float64(2),
		})
		if data["root"] == nil {
			t.Fatal("expected root")
		}
		root := data["root"].(map[string]any)
		if root["name"] != "HandleRequest" {
			t.Errorf("root name = %v", root["name"])
		}
		if data["hops"] == nil {
			t.Error("expected hops")
		}
	})

	t.Run("both_direction", func(t *testing.T) {
		data := callTool(t, srv, "trace_call_path", map[string]any{
			"function_name": "ProcessOrder",
			"direction":     "both",
		})
		if data["root"] == nil {
			t.Fatal("expected root")
		}
	})

	t.Run("with_risk_labels", func(t *testing.T) {
		data := callTool(t, srv, "trace_call_path", map[string]any{
			"function_name": "HandleRequest",
			"risk_labels":   true,
		})
		if data["impact_summary"] == nil {
			t.Error("expected impact_summary with risk_labels=true")
		}
	})

	t.Run("with_min_confidence", func(t *testing.T) {
		data := callTool(t, srv, "trace_call_path", map[string]any{
			"function_name":  "HandleRequest",
			"min_confidence": float64(0.5),
		})
		if data["root"] == nil {
			t.Fatal("expected root")
		}
	})

	t.Run("not_found_with_suggestions", func(t *testing.T) {
		data := callTool(t, srv, "trace_call_path", map[string]any{
			"function_name": "Handle",
		})
		if data["status"] != "not_found" {
			t.Errorf("status = %v, want not_found", data["status"])
		}
		if data["suggestions"] == nil {
			t.Error("expected suggestions")
		}
	})

	t.Run("not_found_no_suggestions", func(t *testing.T) {
		result := callToolRaw(t, srv, "trace_call_path", map[string]any{
			"function_name": "ZZZZZZZZZZZ_nonexistent",
		})
		if !result.IsError {
			t.Fatal("expected error for truly non-existent function")
		}
	})

	t.Run("missing_function_name", func(t *testing.T) {
		result := callToolRaw(t, srv, "trace_call_path", map[string]any{})
		if !result.IsError {
			t.Fatal("expected error for missing function_name")
		}
	})
}

func TestHandleGetArchitecture(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("all_aspects", func(t *testing.T) {
		data := callTool(t, srv, "get_architecture", map[string]any{
			"aspects": []any{"all"},
		})
		if data["project"] != "test-project" {
			t.Errorf("project = %v", data["project"])
		}
	})

	t.Run("specific_aspects", func(t *testing.T) {
		data := callTool(t, srv, "get_architecture", map[string]any{
			"aspects": []any{"languages", "hotspots"},
		})
		if data["project"] != "test-project" {
			t.Errorf("project = %v", data["project"])
		}
	})

	t.Run("default_aspects", func(t *testing.T) {
		data := callTool(t, srv, "get_architecture", map[string]any{})
		if data["project"] != "test-project" {
			t.Errorf("project = %v", data["project"])
		}
	})

	t.Run("invalid_aspect", func(t *testing.T) {
		result := callToolRaw(t, srv, "get_architecture", map[string]any{
			"aspects": []any{"bogus_aspect"},
		})
		if !result.IsError {
			t.Fatal("expected error for invalid aspect")
		}
	})
}

func TestHandleManageADR(t *testing.T) {
	srv, _ := testServerWithProject(t)

	adrContent := "## PURPOSE\nTest purpose\n\n## STACK\nGo\n\n## ARCHITECTURE\nSimple\n\n## PATTERNS\nMVC\n\n## TRADEOFFS\nSpeed vs safety\n\n## PHILOSOPHY\nKISS"

	t.Run("get_no_adr", func(t *testing.T) {
		data := callTool(t, srv, "manage_adr", map[string]any{
			"mode": "get",
		})
		if data["adr"] != nil {
			t.Error("expected nil adr initially")
		}
		if data["adr_hint"] == nil {
			t.Error("expected adr_hint")
		}
	})

	t.Run("store", func(t *testing.T) {
		data := callTool(t, srv, "manage_adr", map[string]any{
			"mode":    "store",
			"content": adrContent,
		})
		if data["status"] != "stored" {
			t.Errorf("status = %v, want stored", data["status"])
		}
	})

	t.Run("get_after_store", func(t *testing.T) {
		data := callTool(t, srv, "manage_adr", map[string]any{
			"mode": "get",
		})
		if data["text"] == nil {
			t.Error("expected text after store")
		}
		if data["sections"] == nil {
			t.Error("expected sections")
		}
	})

	t.Run("get_with_include_filter", func(t *testing.T) {
		data := callTool(t, srv, "manage_adr", map[string]any{
			"mode":    "get",
			"include": []any{"PURPOSE", "STACK"},
		})
		sections := data["sections"].(map[string]any)
		if sections["PURPOSE"] == nil {
			t.Error("expected PURPOSE section")
		}
		if sections["STACK"] == nil {
			t.Error("expected STACK section")
		}
		if sections["ARCHITECTURE"] != nil {
			t.Error("ARCHITECTURE should be filtered out")
		}
	})

	t.Run("update", func(t *testing.T) {
		data := callTool(t, srv, "manage_adr", map[string]any{
			"mode": "update",
			"sections": map[string]any{
				"PURPOSE": "Updated purpose",
			},
		})
		if data["status"] != "updated" {
			t.Errorf("status = %v, want updated", data["status"])
		}
	})

	t.Run("delete", func(t *testing.T) {
		data := callTool(t, srv, "manage_adr", map[string]any{
			"mode": "delete",
		})
		if data["status"] != "deleted" {
			t.Errorf("status = %v, want deleted", data["status"])
		}
	})

	t.Run("store_missing_content", func(t *testing.T) {
		result := callToolRaw(t, srv, "manage_adr", map[string]any{
			"mode": "store",
		})
		if !result.IsError {
			t.Fatal("expected error for missing content")
		}
	})

	t.Run("store_missing_sections", func(t *testing.T) {
		result := callToolRaw(t, srv, "manage_adr", map[string]any{
			"mode":    "store",
			"content": "## PURPOSE\nonly purpose",
		})
		if !result.IsError {
			t.Fatal("expected error for missing required sections")
		}
	})

	t.Run("update_missing_sections", func(t *testing.T) {
		result := callToolRaw(t, srv, "manage_adr", map[string]any{
			"mode": "update",
		})
		if !result.IsError {
			t.Fatal("expected error for missing sections in update")
		}
	})

	t.Run("update_invalid_section_key", func(t *testing.T) {
		srv2, _ := testServerWithProject(t)
		callTool(t, srv2, "manage_adr", map[string]any{
			"mode":    "store",
			"content": adrContent,
		})
		result := callToolRaw(t, srv2, "manage_adr", map[string]any{
			"mode": "update",
			"sections": map[string]any{
				"INVALID_KEY": "bad",
			},
		})
		if !result.IsError {
			t.Fatal("expected error for invalid section key")
		}
	})

	t.Run("missing_mode", func(t *testing.T) {
		result := callToolRaw(t, srv, "manage_adr", map[string]any{})
		if !result.IsError {
			t.Fatal("expected error for missing mode")
		}
	})

	t.Run("invalid_mode", func(t *testing.T) {
		result := callToolRaw(t, srv, "manage_adr", map[string]any{
			"mode": "invalid",
		})
		if !result.IsError {
			t.Fatal("expected error for invalid mode")
		}
	})

	t.Run("invalid_include_filter", func(t *testing.T) {
		result := callToolRaw(t, srv, "manage_adr", map[string]any{
			"mode":    "get",
			"include": []any{"BOGUS_SECTION"},
		})
		if !result.IsError {
			t.Fatal("expected error for invalid include filter")
		}
	})
}

func TestHandleSearchCode(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("basic_search", func(t *testing.T) {
		data := callTool(t, srv, "search_code", map[string]any{
			"pattern": "HandleRequest",
		})
		if data["matches"] == nil {
			t.Fatal("expected matches")
		}
	})

	t.Run("regex_search", func(t *testing.T) {
		data := callTool(t, srv, "search_code", map[string]any{
			"pattern": "func.*Request",
			"regex":   true,
		})
		if data["matches"] == nil {
			t.Fatal("expected matches")
		}
	})

	t.Run("with_file_pattern", func(t *testing.T) {
		data := callTool(t, srv, "search_code", map[string]any{
			"pattern":      "func",
			"file_pattern": "*.go",
		})
		if data["matches"] == nil {
			t.Fatal("expected matches")
		}
	})

	t.Run("empty_pattern", func(t *testing.T) {
		result := callToolRaw(t, srv, "search_code", map[string]any{})
		if !result.IsError {
			t.Fatal("expected error for empty pattern")
		}
	})
}

func TestResolveStore(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("session_project", func(t *testing.T) {
		st, err := srv.resolveStore("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if st == nil {
			t.Fatal("expected non-nil store")
		}
	})

	t.Run("explicit_project", func(t *testing.T) {
		st, err := srv.resolveStore("test-project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if st == nil {
			t.Fatal("expected non-nil store")
		}
	})

	t.Run("wildcard_rejected", func(t *testing.T) {
		_, err := srv.resolveStore("*")
		if err == nil {
			t.Fatal("expected error for wildcard")
		}
	})

	t.Run("all_rejected", func(t *testing.T) {
		_, err := srv.resolveStore("all")
		if err == nil {
			t.Fatal("expected error for 'all'")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		_, err := srv.resolveStore("nonexistent-project")
		if err == nil {
			t.Fatal("expected error for non-existent project")
		}
	})

	t.Run("no_session_no_project", func(t *testing.T) {
		tmpDir := t.TempDir()
		router, err := store.NewRouterWithDir(filepath.Join(tmpDir, "db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(router.CloseAll)
		noSessionSrv := NewServer(router)
		_, err = noSessionSrv.resolveStore("")
		if err == nil {
			t.Fatal("expected error when no session and no project")
		}
	})
}

func TestResolveProjectRoot(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("session_root", func(t *testing.T) {
		root, err := srv.resolveProjectRoot("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if root == "" {
			t.Fatal("expected non-empty root")
		}
	})

	t.Run("wildcard_rejected", func(t *testing.T) {
		_, err := srv.resolveProjectRoot("*")
		if err == nil {
			t.Fatal("expected error for wildcard")
		}
	})

	t.Run("explicit_project", func(t *testing.T) {
		root, err := srv.resolveProjectRoot("test-project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if root == "" {
			t.Fatal("expected non-empty root")
		}
	})
}

func TestFindNodeAcrossProjects(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("found", func(t *testing.T) {
		node, proj, err := srv.findNodeAcrossProjects("HandleRequest", "test-project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if node == nil {
			t.Fatal("expected node")
		}
		if proj == "" {
			t.Fatal("expected project name")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		_, _, err := srv.findNodeAcrossProjects("NonexistentXYZ", "test-project")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("wildcard_rejected", func(t *testing.T) {
		_, _, err := srv.findNodeAcrossProjects("HandleRequest", "*")
		if err == nil {
			t.Fatal("expected error for wildcard")
		}
	})

	t.Run("session_fallback", func(t *testing.T) {
		node, _, err := srv.findNodeAcrossProjects("HandleRequest")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if node == nil {
			t.Fatal("expected node via session fallback")
		}
	})
}

func TestEmptyDetectResponse(t *testing.T) {
	srv, _ := testServerWithProject(t)
	result := srv.emptyDetectResponse()
	if result.IsError {
		t.Fatal("should not be error")
	}
	tc, _ := result.Content[0].(*mcp.TextContent)
	var data map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	summary := data["summary"].(map[string]any)
	if summary["total"] != float64(0) {
		t.Errorf("total = %v, want 0", summary["total"])
	}
	if summary["has_cross_service"] != false {
		t.Errorf("has_cross_service = %v, want false", summary["has_cross_service"])
	}
}

func TestGetModuleInfo(t *testing.T) {
	srv, _ := testServerWithProject(t)
	st, err := srv.resolveStore("")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("found", func(t *testing.T) {
		funcNode := &store.Node{FilePath: "main.go"}
		info := srv.getModuleInfo(st, funcNode, "test-project")
		if info["name"] != "main" {
			t.Errorf("module name = %v, want main", info["name"])
		}
	})

	t.Run("no_file_path", func(t *testing.T) {
		funcNode := &store.Node{}
		info := srv.getModuleInfo(st, funcNode, "test-project")
		if len(info) != 0 {
			t.Errorf("expected empty info, got %v", info)
		}
	})

	t.Run("no_match", func(t *testing.T) {
		funcNode := &store.Node{FilePath: "other.go"}
		info := srv.getModuleInfo(st, funcNode, "test-project")
		if len(info) != 0 {
			t.Errorf("expected empty info, got %v", info)
		}
	})
}

func TestBuildTraceResponse(t *testing.T) {
	srv, _ := testServerWithProject(t)
	st, err := srv.resolveStore("")
	if err != nil {
		t.Fatal(err)
	}

	rootNode := &store.Node{
		Name:          "HandleRequest",
		QualifiedName: "pkg.HandleRequest",
		Label:         "Function",
		FilePath:      "main.go",
		Properties:    map[string]any{},
	}
	visited := []*store.NodeHop{}
	edges := []store.EdgeInfo{}

	data := buildTraceResponse(st, rootNode, "test-project", nil, visited, edges)
	if data["root"] == nil {
		t.Error("expected root")
	}
	if data["total_results"] != 0 {
		t.Errorf("total_results = %v, want 0", data["total_results"])
	}
}

func TestAddADRToResponse(t *testing.T) {
	srv, _ := testServerWithProject(t)
	st, err := srv.resolveStore("")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("no_adr", func(t *testing.T) {
		data := map[string]any{}
		addADRToResponse(data, []string{"all"}, st, "test-project")
		if data["adr"] != nil {
			t.Error("expected nil adr")
		}
		if data["adr_hint"] == nil {
			t.Error("expected adr_hint")
		}
	})

	t.Run("adr_not_requested", func(t *testing.T) {
		data := map[string]any{}
		addADRToResponse(data, []string{"languages"}, st, "test-project")
		if _, ok := data["adr"]; ok {
			t.Error("adr should not be set when not requested")
		}
	})

	t.Run("with_adr", func(t *testing.T) {
		content := "## PURPOSE\nTest\n\n## STACK\nGo\n\n## ARCHITECTURE\nSimple\n\n## PATTERNS\nMVC\n\n## TRADEOFFS\nSpeed\n\n## PHILOSOPHY\nKISS"
		if err := st.StoreADR("test-project", content); err != nil {
			t.Fatal(err)
		}
		data := map[string]any{}
		addADRToResponse(data, []string{"adr"}, st, "test-project")
		adr := data["adr"].(map[string]any)
		if adr["text"] == nil {
			t.Error("expected adr text")
		}
	})
}

func TestMapChangesToSymbols(t *testing.T) {
	srv, projName := testServerWithProject(t)
	st, err := srv.resolveStore("")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("by_file", func(t *testing.T) {
		files := []pipeline.ChangedFile{
			{Status: "M", Path: "main.go"},
		}
		symbols := mapChangesToSymbols(st, projName, files, nil)
		if len(symbols) == 0 {
			t.Fatal("expected symbols for main.go")
		}
	})

	t.Run("by_hunks", func(t *testing.T) {
		files := []pipeline.ChangedFile{
			{Status: "M", Path: "main.go"},
		}
		hunks := []pipeline.ChangedHunk{
			{Path: "main.go", StartLine: 3, EndLine: 5},
		}
		symbols := mapChangesToSymbols(st, projName, files, hunks)
		if len(symbols) == 0 {
			t.Fatal("expected symbols overlapping hunk lines 3-5")
		}
		found := false
		for _, s := range symbols {
			if s.Name == "HandleRequest" {
				found = true
			}
		}
		if !found {
			t.Error("expected HandleRequest in symbols")
		}
	})

	t.Run("deleted_files_skipped", func(t *testing.T) {
		files := []pipeline.ChangedFile{
			{Status: "D", Path: "main.go"},
		}
		symbols := mapChangesToSymbols(st, projName, files, nil)
		if len(symbols) != 0 {
			t.Errorf("expected no symbols for deleted files, got %d", len(symbols))
		}
	})

	t.Run("dedup", func(t *testing.T) {
		files := []pipeline.ChangedFile{
			{Status: "M", Path: "main.go"},
		}
		hunks := []pipeline.ChangedHunk{
			{Path: "main.go", StartLine: 3, EndLine: 5},
			{Path: "main.go", StartLine: 3, EndLine: 5},
		}
		symbols := mapChangesToSymbols(st, projName, files, hunks)
		ids := map[int64]bool{}
		for _, s := range symbols {
			if ids[s.ID] {
				t.Errorf("duplicate symbol ID %d", s.ID)
			}
			ids[s.ID] = true
		}
	})

	t.Run("unknown_file", func(t *testing.T) {
		files := []pipeline.ChangedFile{
			{Status: "M", Path: "nonexistent.go"},
		}
		symbols := mapChangesToSymbols(st, projName, files, nil)
		if len(symbols) != 0 {
			t.Errorf("expected no symbols for unknown file, got %d", len(symbols))
		}
	})
}

func TestTraceImpact(t *testing.T) {
	srv, _ := testServerWithProject(t)
	st, err := srv.resolveStore("")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("basic", func(t *testing.T) {
		nodes, findErr := st.FindNodesByName("test-project", "ProcessOrder")
		if findErr != nil || len(nodes) == 0 {
			t.Fatal("cannot find ProcessOrder")
		}
		impacted, edges := traceImpact(st, nodes, 3)
		if len(impacted) == 0 {
			t.Fatal("expected impacted symbols (HandleRequest calls ProcessOrder)")
		}
		foundHR := false
		for _, is := range impacted {
			if is.Node.Name == "HandleRequest" {
				foundHR = true
				if is.Hop != 1 {
					t.Errorf("HandleRequest hop = %d, want 1", is.Hop)
				}
				if is.ChangedBy != "ProcessOrder" {
					t.Errorf("changed_by = %q, want ProcessOrder", is.ChangedBy)
				}
			}
		}
		if !foundHR {
			t.Error("expected HandleRequest in impacted")
		}
		_ = edges
	})

	t.Run("empty_symbols", func(t *testing.T) {
		impacted, _ := traceImpact(st, nil, 3)
		if len(impacted) != 0 {
			t.Errorf("expected empty, got %d", len(impacted))
		}
	})
}

func TestResolveFileSymbols(t *testing.T) {
	srv, projName := testServerWithProject(t)
	st, err := srv.resolveStore("")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("with_hunks", func(t *testing.T) {
		hunks := []pipeline.ChangedHunk{
			{Path: "main.go", StartLine: 7, EndLine: 9},
		}
		nodes := resolveFileSymbols(st, projName, "main.go", hunks)
		if len(nodes) == 0 {
			t.Fatal("expected nodes for hunk overlap")
		}
	})

	t.Run("without_hunks", func(t *testing.T) {
		nodes := resolveFileSymbols(st, projName, "main.go", nil)
		if len(nodes) == 0 {
			t.Fatal("expected nodes for file")
		}
	})
}

func TestHandleIngestTraces(t *testing.T) {
	srv, _ := testServerWithProject(t)

	t.Run("missing_params", func(t *testing.T) {
		result := callToolRaw(t, srv, "ingest_traces", map[string]any{})
		if !result.IsError {
			t.Fatal("expected error for missing params")
		}
	})

	t.Run("missing_file", func(t *testing.T) {
		result := callToolRaw(t, srv, "ingest_traces", map[string]any{
			"project":   "test-project",
			"file_path": "/nonexistent/traces.json",
		})
		if !result.IsError {
			t.Fatal("expected error for nonexistent file")
		}
	})
}

func TestHandleIndexStatus_Indexing(t *testing.T) {
	srv, _ := testServerWithProject(t)
	srv.indexStatus.Store("indexing")
	data := callTool(t, srv, "index_status", map[string]any{})
	if data["status"] != "indexing" {
		t.Errorf("status = %v, want indexing", data["status"])
	}
}
