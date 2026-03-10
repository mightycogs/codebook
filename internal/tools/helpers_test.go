package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/mightycogs/codebook/internal/pipeline"
	"github.com/mightycogs/codebook/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetStringArg(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		key  string
		want string
	}{
		{"present", map[string]any{"name": "hello"}, "name", "hello"},
		{"missing", map[string]any{"other": "val"}, "name", ""},
		{"wrong_type", map[string]any{"name": 42}, "name", ""},
		{"empty_string", map[string]any{"name": ""}, "name", ""},
		{"nil_map", map[string]any{}, "name", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStringArg(tt.args, tt.key)
			if got != tt.want {
				t.Errorf("getStringArg(%v, %q) = %q, want %q", tt.args, tt.key, got, tt.want)
			}
		})
	}
}

func TestGetIntArg(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]any
		key        string
		defaultVal int
		want       int
	}{
		{"present", map[string]any{"depth": float64(5)}, "depth", 3, 5},
		{"missing", map[string]any{}, "depth", 3, 3},
		{"wrong_type_string", map[string]any{"depth": "five"}, "depth", 3, 3},
		{"zero_value", map[string]any{"depth": float64(0)}, "depth", 3, 0},
		{"negative", map[string]any{"depth": float64(-1)}, "depth", 3, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getIntArg(tt.args, tt.key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("getIntArg(%v, %q, %d) = %d, want %d", tt.args, tt.key, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestGetFloatArg(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]any
		key        string
		defaultVal float64
		want       float64
	}{
		{"present", map[string]any{"conf": float64(0.75)}, "conf", 0.0, 0.75},
		{"missing", map[string]any{}, "conf", 0.5, 0.5},
		{"wrong_type", map[string]any{"conf": "high"}, "conf", 0.5, 0.5},
		{"zero", map[string]any{"conf": float64(0)}, "conf", 0.5, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getFloatArg(tt.args, tt.key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("getFloatArg(%v, %q, %f) = %f, want %f", tt.args, tt.key, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestGetBoolArg(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		key  string
		want bool
	}{
		{"true", map[string]any{"flag": true}, "flag", true},
		{"false", map[string]any{"flag": false}, "flag", false},
		{"missing", map[string]any{}, "flag", false},
		{"wrong_type", map[string]any{"flag": "true"}, "flag", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getBoolArg(tt.args, tt.key)
			if got != tt.want {
				t.Errorf("getBoolArg(%v, %q) = %v, want %v", tt.args, tt.key, got, tt.want)
			}
		})
	}
}

func TestGetMapStringArg(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		args := map[string]any{
			"sections": map[string]any{
				"PURPOSE": "build stuff",
				"STACK":   "Go, SQLite",
			},
		}
		got := getMapStringArg(args, "sections")
		if got == nil {
			t.Fatal("expected non-nil map")
		}
		if got["PURPOSE"] != "build stuff" {
			t.Errorf("PURPOSE = %q, want %q", got["PURPOSE"], "build stuff")
		}
		if got["STACK"] != "Go, SQLite" {
			t.Errorf("STACK = %q, want %q", got["STACK"], "Go, SQLite")
		}
	})

	t.Run("missing", func(t *testing.T) {
		got := getMapStringArg(map[string]any{}, "sections")
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("wrong_type", func(t *testing.T) {
		got := getMapStringArg(map[string]any{"sections": "not a map"}, "sections")
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("non_string_values_skipped", func(t *testing.T) {
		args := map[string]any{
			"sections": map[string]any{
				"PURPOSE": "valid",
				"COUNT":   42,
			},
		}
		got := getMapStringArg(args, "sections")
		if got["PURPOSE"] != "valid" {
			t.Errorf("PURPOSE = %q, want %q", got["PURPOSE"], "valid")
		}
		if _, ok := got["COUNT"]; ok {
			t.Error("non-string value should be skipped")
		}
	})
}

func TestParseArgs(t *testing.T) {
	t.Run("valid_json", func(t *testing.T) {
		raw := json.RawMessage(`{"name":"test","count":5}`)
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Name:      "test_tool",
				Arguments: raw,
			},
		}
		got, err := parseArgs(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["name"] != "test" {
			t.Errorf("name = %v, want %q", got["name"], "test")
		}
	})

	t.Run("empty_args", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Name:      "test_tool",
				Arguments: nil,
			},
		}
		got, err := parseArgs(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected empty map, got %v", got)
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Name:      "test_tool",
				Arguments: json.RawMessage(`{invalid`),
			},
		}
		_, err := parseArgs(req)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}

func TestJsonResult(t *testing.T) {
	data := map[string]any{"status": "ok", "count": 42}
	result := jsonResult(data)
	if result.IsError {
		t.Fatal("result should not be error")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if parsed["status"] != "ok" {
		t.Errorf("status = %v, want %q", parsed["status"], "ok")
	}
}

func TestErrResult(t *testing.T) {
	result := errResult("something broke")
	if !result.IsError {
		t.Fatal("result should be error")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if tc.Text != "something broke" {
		t.Errorf("text = %q, want %q", tc.Text, "something broke")
	}
}

func TestAddIndexStatus(t *testing.T) {
	t.Run("indexing", func(t *testing.T) {
		s := &Server{}
		s.indexStatus.Store("indexing")
		data := map[string]any{}
		s.addIndexStatus(data)
		if data["index_status"] != "indexing" {
			t.Errorf("index_status = %v, want %q", data["index_status"], "indexing")
		}
	})

	t.Run("ready", func(t *testing.T) {
		s := &Server{}
		s.indexStatus.Store("ready")
		data := map[string]any{}
		s.addIndexStatus(data)
		if _, ok := data["index_status"]; ok {
			t.Error("index_status should not be set when status is ready")
		}
	})

	t.Run("unset", func(t *testing.T) {
		s := &Server{}
		data := map[string]any{}
		s.addIndexStatus(data)
		if _, ok := data["index_status"]; ok {
			t.Error("index_status should not be set when status is unset")
		}
	})
}

func TestResolveProjectName(t *testing.T) {
	s := &Server{sessionProject: "my-session"}

	t.Run("explicit", func(t *testing.T) {
		got := s.resolveProjectName("explicit-proj")
		if got != "explicit-proj" {
			t.Errorf("got %q, want %q", got, "explicit-proj")
		}
	})

	t.Run("empty_falls_back", func(t *testing.T) {
		got := s.resolveProjectName("")
		if got != "my-session" {
			t.Errorf("got %q, want %q", got, "my-session")
		}
	})
}

func TestSetVersion(t *testing.T) {
	old := Version
	defer func() { Version = old }()

	SetVersion("1.2.3")
	if Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", Version, "1.2.3")
	}
}

func TestDedupNodes(t *testing.T) {
	t.Run("dedup", func(t *testing.T) {
		n1 := &store.Node{ID: 1, Name: "A"}
		n2 := &store.Node{ID: 2, Name: "B"}
		n1dup := &store.Node{ID: 1, Name: "A"}
		result := dedupNodes([]*store.Node{n1, n2}, []*store.Node{n1dup})
		if len(result) != 2 {
			t.Errorf("expected 2 unique nodes, got %d", len(result))
		}
	})

	t.Run("nil_nodes_skipped", func(t *testing.T) {
		n1 := &store.Node{ID: 1, Name: "A"}
		result := dedupNodes([]*store.Node{n1, nil}, []*store.Node{nil})
		if len(result) != 1 {
			t.Errorf("expected 1 node, got %d", len(result))
		}
	})

	t.Run("empty_slices", func(t *testing.T) {
		result := dedupNodes([]*store.Node{}, []*store.Node{})
		if len(result) != 0 {
			t.Errorf("expected 0 nodes, got %d", len(result))
		}
	})
}

func TestReadLines(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("range", func(t *testing.T) {
		got, err := readLines(path, 2, 4)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == "" {
			t.Fatal("expected non-empty output")
		}
		if !contains(got, "line2") || !contains(got, "line4") {
			t.Errorf("expected lines 2-4, got: %s", got)
		}
		if contains(got, "line1") || contains(got, "line5") {
			t.Errorf("should not contain line1 or line5, got: %s", got)
		}
	})

	t.Run("single_line", func(t *testing.T) {
		got, err := readLines(path, 3, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !contains(got, "line3") {
			t.Errorf("expected line3, got: %s", got)
		}
	})

	t.Run("out_of_range", func(t *testing.T) {
		_, err := readLines(path, 100, 200)
		if err == nil {
			t.Fatal("expected error for out-of-range lines")
		}
	})

	t.Run("file_not_found", func(t *testing.T) {
		_, err := readLines(filepath.Join(tmpDir, "nonexistent.go"), 1, 5)
		if err == nil {
			t.Fatal("expected error for non-existent file")
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestParseAspects(t *testing.T) {
	t.Run("default_when_missing", func(t *testing.T) {
		args := map[string]any{}
		got, err := parseAspects(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0] != "all" {
			t.Errorf("expected [all], got %v", got)
		}
	})

	t.Run("default_when_wrong_type", func(t *testing.T) {
		args := map[string]any{"aspects": "all"}
		got, err := parseAspects(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0] != "all" {
			t.Errorf("expected [all], got %v", got)
		}
	})

	t.Run("valid_aspects", func(t *testing.T) {
		args := map[string]any{
			"aspects": []any{"languages", "packages", "hotspots"},
		}
		got, err := parseAspects(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 {
			t.Errorf("expected 3 aspects, got %d", len(got))
		}
	})

	t.Run("unknown_aspect", func(t *testing.T) {
		args := map[string]any{
			"aspects": []any{"languages", "invalid_aspect"},
		}
		_, err := parseAspects(args)
		if err == nil {
			t.Fatal("expected error for unknown aspect")
		}
	})

	t.Run("empty_array_defaults", func(t *testing.T) {
		args := map[string]any{
			"aspects": []any{},
		}
		got, err := parseAspects(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0] != "all" {
			t.Errorf("expected [all], got %v", got)
		}
	})

	t.Run("non_string_elements_skipped", func(t *testing.T) {
		args := map[string]any{
			"aspects": []any{"languages", 42, "packages"},
		}
		got, err := parseAspects(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("expected 2 aspects (skipping non-string), got %d", len(got))
		}
	})

	t.Run("all_valid_aspects", func(t *testing.T) {
		all := []any{"all", "languages", "packages", "entry_points", "routes", "hotspots", "boundaries", "services", "layers", "clusters", "file_tree", "adr"}
		args := map[string]any{"aspects": all}
		got, err := parseAspects(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(all) {
			t.Errorf("expected %d aspects, got %d", len(all), len(got))
		}
	})
}

func TestBuildArchResponse(t *testing.T) {
	t.Run("all_fields", func(t *testing.T) {
		info := &store.ArchitectureInfo{
			Languages:   []store.LanguageCount{{Language: "Go", FileCount: 10}},
			Packages:    []store.PackageSummary{{Name: "pkg1"}},
			EntryPoints: []store.EntryPointInfo{{Name: "main"}},
			Routes:      []store.RouteInfo{{Method: "GET", Path: "/api"}},
			Hotspots:    []store.HotspotFunction{{Name: "hot"}},
			Boundaries:  []store.CrossPkgBoundary{{From: "a", To: "b"}},
			Services:    []store.ServiceLink{{From: "svc1", To: "svc2"}},
			Layers:      []store.PackageLayer{{Name: "pkg", Layer: "domain"}},
			Clusters:    []store.ClusterInfo{{ID: 1}},
			FileTree:    []store.FileTreeEntry{{Path: "/"}},
		}
		data := buildArchResponse("test-proj", info)
		if data["project"] != "test-proj" {
			t.Errorf("project = %v, want %q", data["project"], "test-proj")
		}
		for _, key := range []string{"languages", "packages", "entry_points", "routes", "hotspots", "boundaries", "services", "layers", "clusters", "file_tree"} {
			if data[key] == nil {
				t.Errorf("expected %s to be set", key)
			}
		}
	})

	t.Run("nil_fields_excluded", func(t *testing.T) {
		info := &store.ArchitectureInfo{
			Languages: []store.LanguageCount{{Language: "Go", FileCount: 5}},
		}
		data := buildArchResponse("test-proj", info)
		if data["languages"] == nil {
			t.Error("expected languages to be set")
		}
		for _, key := range []string{"packages", "entry_points", "routes", "hotspots", "boundaries", "services", "layers", "clusters", "file_tree"} {
			if data[key] != nil {
				t.Errorf("expected %s to be nil", key)
			}
		}
	})

	t.Run("empty_info", func(t *testing.T) {
		info := &store.ArchitectureInfo{}
		data := buildArchResponse("empty-proj", info)
		if data["project"] != "empty-proj" {
			t.Errorf("project = %v, want %q", data["project"], "empty-proj")
		}
		if len(data) != 1 {
			t.Errorf("expected only project key, got %d keys", len(data))
		}
	})
}

func TestParseStringArray(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		args := map[string]any{
			"include": []any{"PURPOSE", "STACK"},
		}
		got := parseStringArray(args, "include")
		if len(got) != 2 {
			t.Fatalf("expected 2 items, got %d", len(got))
		}
		if got[0] != "PURPOSE" || got[1] != "STACK" {
			t.Errorf("got %v, want [PURPOSE STACK]", got)
		}
	})

	t.Run("missing", func(t *testing.T) {
		got := parseStringArray(map[string]any{}, "include")
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("wrong_type", func(t *testing.T) {
		got := parseStringArray(map[string]any{"include": "not_array"}, "include")
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("mixed_types_in_array", func(t *testing.T) {
		args := map[string]any{
			"include": []any{"PURPOSE", 42, "STACK", true},
		}
		got := parseStringArray(args, "include")
		if len(got) != 2 {
			t.Errorf("expected 2 string items, got %d", len(got))
		}
	})
}

func TestStringsToMap(t *testing.T) {
	t.Run("non_empty", func(t *testing.T) {
		got := stringsToMap([]string{"A", "B", "C"})
		if len(got) != 3 {
			t.Fatalf("expected 3 keys, got %d", len(got))
		}
		for _, k := range []string{"A", "B", "C"} {
			if _, ok := got[k]; !ok {
				t.Errorf("expected key %q", k)
			}
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := stringsToMap([]string{})
		if len(got) != 0 {
			t.Errorf("expected empty map, got %v", got)
		}
	})
}

func TestValidateSectionFilter(t *testing.T) {
	t.Run("empty_is_valid", func(t *testing.T) {
		if err := validateSectionFilter(nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("valid_sections", func(t *testing.T) {
		if err := validateSectionFilter([]string{"PURPOSE", "STACK"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid_section", func(t *testing.T) {
		err := validateSectionFilter([]string{"PURPOSE", "INVALID_SECTION"})
		if err == nil {
			t.Fatal("expected error for invalid section")
		}
	})
}

func TestBuildFileList(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		files := []pipeline.ChangedFile{
			{Status: "M", Path: "main.go"},
			{Status: "A", Path: "new.go"},
		}
		got := buildFileList(files)
		if len(got) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(got))
		}
		if got[0]["status"] != "M" || got[0]["path"] != "main.go" {
			t.Errorf("first entry = %v", got[0])
		}
		if got[1]["status"] != "A" || got[1]["path"] != "new.go" {
			t.Errorf("second entry = %v", got[1])
		}
	})

	t.Run("rename_includes_old_path", func(t *testing.T) {
		files := []pipeline.ChangedFile{
			{Status: "R", Path: "new_name.go", OldPath: "old_name.go"},
		}
		got := buildFileList(files)
		if got[0]["old_path"] != "old_name.go" {
			t.Errorf("expected old_path, got %v", got[0])
		}
	})

	t.Run("no_old_path_when_empty", func(t *testing.T) {
		files := []pipeline.ChangedFile{
			{Status: "M", Path: "main.go"},
		}
		got := buildFileList(files)
		if _, ok := got[0]["old_path"]; ok {
			t.Error("should not have old_path for non-rename")
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := buildFileList(nil)
		if len(got) != 0 {
			t.Errorf("expected empty, got %d", len(got))
		}
	})
}

func TestBuildSymbolList(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		symbols := []*store.Node{
			{Name: "Func1", QualifiedName: "pkg.Func1", Label: "Function", FilePath: "main.go", StartLine: 1, EndLine: 10},
			{Name: "Class1", QualifiedName: "pkg.Class1", Label: "Class", FilePath: "model.py", StartLine: 5, EndLine: 20},
		}
		got := buildSymbolList(symbols)
		if len(got) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(got))
		}
		if got[0]["name"] != "Func1" {
			t.Errorf("name = %v, want Func1", got[0]["name"])
		}
		if got[0]["qualified_name"] != "pkg.Func1" {
			t.Errorf("qualified_name = %v", got[0]["qualified_name"])
		}
		if got[0]["label"] != "Function" {
			t.Errorf("label = %v", got[0]["label"])
		}
		if got[1]["file_path"] != "model.py" {
			t.Errorf("file_path = %v", got[1]["file_path"])
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := buildSymbolList(nil)
		if len(got) != 0 {
			t.Errorf("expected empty, got %d", len(got))
		}
	})
}

func TestBuildImpactList(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		impacted := []impactedSymbol{
			{
				Node:      &store.Node{Name: "CallerA", QualifiedName: "pkg.CallerA", Label: "Function", FilePath: "a.go"},
				Hop:       1,
				ChangedBy: "TargetFunc",
			},
			{
				Node:      &store.Node{Name: "CallerB", QualifiedName: "pkg.CallerB", Label: "Function", FilePath: "b.go"},
				Hop:       2,
				ChangedBy: "TargetFunc",
			},
			{
				Node:      &store.Node{Name: "CallerC", QualifiedName: "pkg.CallerC", Label: "Function", FilePath: "c.go"},
				Hop:       4,
				ChangedBy: "TargetFunc",
			},
		}
		got := buildImpactList(impacted)
		if len(got) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(got))
		}
		if got[0]["risk"] != "CRITICAL" {
			t.Errorf("hop 1 risk = %v, want CRITICAL", got[0]["risk"])
		}
		if got[0]["hop"] != 1 {
			t.Errorf("hop = %v, want 1", got[0]["hop"])
		}
		if got[0]["changed_by"] != "TargetFunc" {
			t.Errorf("changed_by = %v", got[0]["changed_by"])
		}
		if got[1]["risk"] != "HIGH" {
			t.Errorf("hop 2 risk = %v, want HIGH", got[1]["risk"])
		}
		if got[2]["risk"] != "LOW" {
			t.Errorf("hop 4 risk = %v, want LOW", got[2]["risk"])
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := buildImpactList(nil)
		if len(got) != 0 {
			t.Errorf("expected empty, got %d", len(got))
		}
	})
}

func TestBuildDetectSummary(t *testing.T) {
	t.Run("mixed_risks", func(t *testing.T) {
		files := []pipeline.ChangedFile{
			{Status: "M", Path: "a.go"},
			{Status: "A", Path: "b.go"},
		}
		symbols := []*store.Node{
			{Name: "Func1"},
			{Name: "Func2"},
			{Name: "Func3"},
		}
		impacted := []impactedSymbol{
			{Node: &store.Node{Name: "C1"}, Hop: 1},
			{Node: &store.Node{Name: "C2"}, Hop: 1},
			{Node: &store.Node{Name: "H1"}, Hop: 2},
			{Node: &store.Node{Name: "M1"}, Hop: 3},
			{Node: &store.Node{Name: "L1"}, Hop: 4},
			{Node: &store.Node{Name: "L2"}, Hop: 5},
		}
		edges := []store.EdgeInfo{
			{Type: "CALLS"},
			{Type: "HTTP_CALLS"},
		}

		got := buildDetectSummary(files, symbols, impacted, edges)
		if got["changed_files"] != 2 {
			t.Errorf("changed_files = %v, want 2", got["changed_files"])
		}
		if got["changed_symbols"] != 3 {
			t.Errorf("changed_symbols = %v, want 3", got["changed_symbols"])
		}
		if got["critical"] != 2 {
			t.Errorf("critical = %v, want 2", got["critical"])
		}
		if got["high"] != 1 {
			t.Errorf("high = %v, want 1", got["high"])
		}
		if got["medium"] != 1 {
			t.Errorf("medium = %v, want 1", got["medium"])
		}
		if got["low"] != 2 {
			t.Errorf("low = %v, want 2", got["low"])
		}
		if got["total"] != 6 {
			t.Errorf("total = %v, want 6", got["total"])
		}
		if got["has_cross_service"] != true {
			t.Errorf("has_cross_service = %v, want true", got["has_cross_service"])
		}
	})

	t.Run("no_cross_service", func(t *testing.T) {
		got := buildDetectSummary(nil, nil, nil, []store.EdgeInfo{{Type: "CALLS"}})
		if got["has_cross_service"] != false {
			t.Errorf("has_cross_service = %v, want false", got["has_cross_service"])
		}
	})

	t.Run("async_calls_is_cross_service", func(t *testing.T) {
		got := buildDetectSummary(nil, nil, nil, []store.EdgeInfo{{Type: "ASYNC_CALLS"}})
		if got["has_cross_service"] != true {
			t.Errorf("has_cross_service = %v, want true", got["has_cross_service"])
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := buildDetectSummary(nil, nil, nil, nil)
		if got["total"] != 0 {
			t.Errorf("total = %v, want 0", got["total"])
		}
		if got["has_cross_service"] != false {
			t.Errorf("has_cross_service = %v, want false", got["has_cross_service"])
		}
	})
}

func TestFilterEdgesByConfidence(t *testing.T) {
	edges := []store.EdgeInfo{
		{FromName: "A", ToName: "B", Type: "CALLS", Confidence: 0.9},
		{FromName: "C", ToName: "D", Type: "CALLS", Confidence: 0.3},
		{FromName: "E", ToName: "F", Type: "HTTP_CALLS", Confidence: 0},
		{FromName: "G", ToName: "H", Type: "CALLS", Confidence: 0.7},
	}

	t.Run("filter_low", func(t *testing.T) {
		got := filterEdgesByConfidence(edges, 0.5)
		if len(got) != 3 {
			t.Fatalf("expected 3 edges (0.9, 0 kept, 0.7), got %d", len(got))
		}
		for _, e := range got {
			if e.Confidence != 0 && e.Confidence < 0.5 {
				t.Errorf("edge %s->%s confidence %f should have been filtered", e.FromName, e.ToName, e.Confidence)
			}
		}
	})

	t.Run("zero_confidence_kept", func(t *testing.T) {
		got := filterEdgesByConfidence(edges, 0.99)
		hasZero := false
		for _, e := range got {
			if e.Confidence == 0 {
				hasZero = true
			}
		}
		if !hasZero {
			t.Error("edges with confidence=0 should be kept")
		}
	})

	t.Run("no_filter", func(t *testing.T) {
		got := filterEdgesByConfidence(edges, 0)
		if len(got) != 4 {
			t.Errorf("expected all 4 edges with minConfidence=0, got %d", len(got))
		}
	})

	t.Run("empty_input", func(t *testing.T) {
		got := filterEdgesByConfidence(nil, 0.5)
		if len(got) != 0 {
			t.Errorf("expected empty, got %d", len(got))
		}
	})
}

func TestBuildNodeInfo(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		n := &store.Node{
			Name:          "HandleRequest",
			QualifiedName: "pkg.HandleRequest",
			Label:         "Function",
			FilePath:      "handler.go",
			StartLine:     10,
			EndLine:       25,
			Properties:    map[string]any{},
		}
		info := buildNodeInfo(n)
		if info["name"] != "HandleRequest" {
			t.Errorf("name = %v", info["name"])
		}
		if info["qualified_name"] != "pkg.HandleRequest" {
			t.Errorf("qualified_name = %v", info["qualified_name"])
		}
		if info["label"] != "Function" {
			t.Errorf("label = %v", info["label"])
		}
		if info["file_path"] != "handler.go" {
			t.Errorf("file_path = %v", info["file_path"])
		}
		if info["start_line"] != 10 {
			t.Errorf("start_line = %v", info["start_line"])
		}
		if info["end_line"] != 25 {
			t.Errorf("end_line = %v", info["end_line"])
		}
	})

	t.Run("with_signature_and_return_type", func(t *testing.T) {
		n := &store.Node{
			Name:          "Process",
			QualifiedName: "pkg.Process",
			Label:         "Function",
			Properties: map[string]any{
				"signature":   "func Process(id int) error",
				"return_type": "error",
			},
		}
		info := buildNodeInfo(n)
		if info["signature"] != "func Process(id int) error" {
			t.Errorf("signature = %v", info["signature"])
		}
		if info["return_type"] != "error" {
			t.Errorf("return_type = %v", info["return_type"])
		}
	})

	t.Run("without_optional_props", func(t *testing.T) {
		n := &store.Node{
			Name:       "Simple",
			Properties: map[string]any{},
		}
		info := buildNodeInfo(n)
		if _, ok := info["signature"]; ok {
			t.Error("should not have signature when not in properties")
		}
		if _, ok := info["return_type"]; ok {
			t.Error("should not have return_type when not in properties")
		}
	})
}

func TestBuildHops(t *testing.T) {
	t.Run("multiple_hops", func(t *testing.T) {
		visited := []*store.NodeHop{
			{Node: &store.Node{Name: "A", QualifiedName: "pkg.A", Label: "Function", Properties: map[string]any{}}, Hop: 1},
			{Node: &store.Node{Name: "B", QualifiedName: "pkg.B", Label: "Function", Properties: map[string]any{"signature": "func B()"}}, Hop: 1},
			{Node: &store.Node{Name: "C", QualifiedName: "pkg.C", Label: "Function", Properties: map[string]any{}}, Hop: 2},
		}
		got := buildHops(visited)
		if len(got) != 2 {
			t.Fatalf("expected 2 hop groups, got %d", len(got))
		}
		if got[0].Hop != 1 {
			t.Errorf("first hop = %d, want 1", got[0].Hop)
		}
		if len(got[0].Nodes) != 2 {
			t.Errorf("hop 1 nodes = %d, want 2", len(got[0].Nodes))
		}
		if got[1].Hop != 2 {
			t.Errorf("second hop = %d, want 2", got[1].Hop)
		}
		if len(got[1].Nodes) != 1 {
			t.Errorf("hop 2 nodes = %d, want 1", len(got[1].Nodes))
		}
	})

	t.Run("signature_included", func(t *testing.T) {
		visited := []*store.NodeHop{
			{Node: &store.Node{Name: "X", QualifiedName: "pkg.X", Label: "Function", Properties: map[string]any{"signature": "func X()"}}, Hop: 1},
		}
		got := buildHops(visited)
		if got[0].Nodes[0]["signature"] != "func X()" {
			t.Errorf("signature = %v", got[0].Nodes[0]["signature"])
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := buildHops(nil)
		if len(got) != 0 {
			t.Errorf("expected empty, got %d", len(got))
		}
	})
}

func TestBuildHopsWithRisk(t *testing.T) {
	t.Run("risk_labels", func(t *testing.T) {
		visited := []*store.NodeHop{
			{Node: &store.Node{Name: "A", QualifiedName: "pkg.A", Label: "Function", Properties: map[string]any{}}, Hop: 1},
			{Node: &store.Node{Name: "B", QualifiedName: "pkg.B", Label: "Function", Properties: map[string]any{}}, Hop: 2},
			{Node: &store.Node{Name: "C", QualifiedName: "pkg.C", Label: "Function", Properties: map[string]any{}}, Hop: 3},
			{Node: &store.Node{Name: "D", QualifiedName: "pkg.D", Label: "Function", Properties: map[string]any{}}, Hop: 4},
		}
		got := buildHopsWithRisk(visited)
		if len(got) != 4 {
			t.Fatalf("expected 4 hop groups, got %d", len(got))
		}
		if got[0].Nodes[0]["risk"] != "CRITICAL" {
			t.Errorf("hop 1 risk = %v, want CRITICAL", got[0].Nodes[0]["risk"])
		}
		if got[1].Nodes[0]["risk"] != "HIGH" {
			t.Errorf("hop 2 risk = %v, want HIGH", got[1].Nodes[0]["risk"])
		}
		if got[2].Nodes[0]["risk"] != "MEDIUM" {
			t.Errorf("hop 3 risk = %v, want MEDIUM", got[2].Nodes[0]["risk"])
		}
		if got[3].Nodes[0]["risk"] != "LOW" {
			t.Errorf("hop 4 risk = %v, want LOW", got[3].Nodes[0]["risk"])
		}
	})

	t.Run("hop_field_present", func(t *testing.T) {
		visited := []*store.NodeHop{
			{Node: &store.Node{Name: "A", QualifiedName: "pkg.A", Label: "Function", Properties: map[string]any{}}, Hop: 1},
		}
		got := buildHopsWithRisk(visited)
		if len(got) != 1 {
			t.Fatalf("expected 1 hop group, got %d", len(got))
		}
		if got[0].Nodes[0]["hop"] != 1 {
			t.Errorf("hop = %v, want 1", got[0].Nodes[0]["hop"])
		}
	})
}

func TestBuildEdgeList(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		edges := []store.EdgeInfo{
			{FromName: "A", ToName: "B", Type: "CALLS", Confidence: 0.85},
			{FromName: "C", ToName: "D", Type: "HTTP_CALLS", Confidence: 0},
		}
		got := buildEdgeList(edges)
		if len(got) != 2 {
			t.Fatalf("expected 2 edges, got %d", len(got))
		}
		if got[0]["from"] != "A" || got[0]["to"] != "B" || got[0]["type"] != "CALLS" {
			t.Errorf("first edge = %v", got[0])
		}
		if got[0]["confidence"] != 0.85 {
			t.Errorf("confidence = %v, want 0.85", got[0]["confidence"])
		}
		if _, ok := got[1]["confidence"]; ok {
			t.Error("zero confidence should not be included")
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := buildEdgeList(nil)
		if len(got) != 0 {
			t.Errorf("expected empty, got %d", len(got))
		}
	})
}

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{"exact_match", "*.go", "main.go", true},
		{"no_match", "*.go", "main.py", false},
		{"double_star_prefix", "**/main.go", "src/cmd/main.go", true},
		{"double_star_suffix", "**/*.go", "src/cmd/main.go", true},
		{"double_star_no_match", "**/*.py", "src/main.go", false},
		{"double_star_prefix_filter", "src/**/*.go", "src/cmd/main.go", true},
		{"double_star_wrong_prefix", "lib/**/*.go", "src/cmd/main.go", false},
		{"double_star_only", "**", "anything/here.txt", true},
		{"simple_no_dir", "*.go", "src/main.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := globMatch(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestSearchFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")
	content := `package main

import "fmt"

func main() {
	fmt.Println("hello world")
	fmt.Println("HELLO WORLD")
	// TODO: fix this
}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("literal_case_insensitive", func(t *testing.T) {
		matches := searchFile(path, "test.go", "hello", nil, false, false, 100)
		if len(matches) != 2 {
			t.Fatalf("expected 2 matches (both hello lines), got %d", len(matches))
		}
		if matches[0].Line != 6 {
			t.Errorf("first match line = %d, want 6", matches[0].Line)
		}
		if matches[0].File != "test.go" {
			t.Errorf("file = %q, want %q", matches[0].File, "test.go")
		}
	})

	t.Run("literal_case_sensitive", func(t *testing.T) {
		matches := searchFile(path, "test.go", "hello", nil, false, true, 100)
		if len(matches) != 1 {
			t.Fatalf("expected 1 match (only lowercase), got %d", len(matches))
		}
	})

	t.Run("regex", func(t *testing.T) {
		re := mustCompileRegex("TODO|FIXME")
		matches := searchFile(path, "test.go", "", re, true, false, 100)
		if len(matches) != 1 {
			t.Fatalf("expected 1 match, got %d", len(matches))
		}
		if matches[0].Line != 8 {
			t.Errorf("match line = %d, want 8", matches[0].Line)
		}
	})

	t.Run("limit", func(t *testing.T) {
		matches := searchFile(path, "test.go", "fmt", nil, false, false, 1)
		if len(matches) != 1 {
			t.Fatalf("expected 1 match (limited), got %d", len(matches))
		}
	})

	t.Run("no_matches", func(t *testing.T) {
		matches := searchFile(path, "test.go", "nonexistent_string_xyz", nil, false, false, 100)
		if len(matches) != 0 {
			t.Errorf("expected 0 matches, got %d", len(matches))
		}
	})

	t.Run("file_not_found", func(t *testing.T) {
		matches := searchFile(filepath.Join(tmpDir, "nope.go"), "nope.go", "hello", nil, false, false, 100)
		if len(matches) != 0 {
			t.Errorf("expected 0 matches for missing file, got %d", len(matches))
		}
	})

	t.Run("long_line_truncated", func(t *testing.T) {
		longPath := filepath.Join(tmpDir, "long.go")
		longLine := make([]byte, 300)
		for i := range longLine {
			longLine[i] = 'x'
		}
		if err := os.WriteFile(longPath, longLine, 0o600); err != nil {
			t.Fatal(err)
		}
		matches := searchFile(longPath, "long.go", "xxx", nil, false, false, 100)
		if len(matches) > 0 && len(matches[0].Content) > 203 {
			t.Errorf("content should be truncated to ~200+3 chars, got %d", len(matches[0].Content))
		}
	})
}

func mustCompileRegex(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(err)
	}
	return re
}

func TestParseSearchCodeParams(t *testing.T) {
	makeReq := func(args map[string]any) *mcp.CallToolRequest {
		raw, _ := json.Marshal(args)
		return &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Name:      "search_code",
				Arguments: raw,
			},
		}
	}

	t.Run("basic", func(t *testing.T) {
		req := makeReq(map[string]any{"pattern": "hello"})
		params, errRes := parseSearchCodeParams(req)
		if errRes != nil {
			t.Fatalf("unexpected error result")
		}
		if params.pattern != "hello" {
			t.Errorf("pattern = %q, want %q", params.pattern, "hello")
		}
		if params.maxResults != 10 {
			t.Errorf("maxResults = %d, want 10", params.maxResults)
		}
	})

	t.Run("empty_pattern_error", func(t *testing.T) {
		req := makeReq(map[string]any{})
		_, errRes := parseSearchCodeParams(req)
		if errRes == nil {
			t.Fatal("expected error result for empty pattern")
		}
	})

	t.Run("regex_mode", func(t *testing.T) {
		req := makeReq(map[string]any{"pattern": "TODO|FIXME", "regex": true})
		params, errRes := parseSearchCodeParams(req)
		if errRes != nil {
			t.Fatalf("unexpected error result")
		}
		if params.re == nil {
			t.Fatal("expected compiled regex")
		}
		if !params.isRegex {
			t.Error("expected isRegex=true")
		}
	})

	t.Run("invalid_regex", func(t *testing.T) {
		req := makeReq(map[string]any{"pattern": "[invalid", "regex": true})
		_, errRes := parseSearchCodeParams(req)
		if errRes == nil {
			t.Fatal("expected error result for invalid regex")
		}
	})

	t.Run("case_insensitive_literal_lowered", func(t *testing.T) {
		req := makeReq(map[string]any{"pattern": "HELLO"})
		params, errRes := parseSearchCodeParams(req)
		if errRes != nil {
			t.Fatalf("unexpected error result")
		}
		if params.pattern != "hello" {
			t.Errorf("pattern should be lowered, got %q", params.pattern)
		}
	})

	t.Run("case_sensitive_literal_preserved", func(t *testing.T) {
		req := makeReq(map[string]any{"pattern": "HELLO", "case_sensitive": true})
		params, errRes := parseSearchCodeParams(req)
		if errRes != nil {
			t.Fatalf("unexpected error result")
		}
		if params.pattern != "HELLO" {
			t.Errorf("pattern should be preserved, got %q", params.pattern)
		}
	})

	t.Run("regex_case_insensitive_prefix", func(t *testing.T) {
		req := makeReq(map[string]any{"pattern": "hello", "regex": true})
		params, errRes := parseSearchCodeParams(req)
		if errRes != nil {
			t.Fatalf("unexpected error result")
		}
		if params.re == nil {
			t.Fatal("expected compiled regex")
		}
		if !params.re.MatchString("HELLO") {
			t.Error("regex should match case-insensitively")
		}
	})

	t.Run("regex_already_has_case_flag", func(t *testing.T) {
		req := makeReq(map[string]any{"pattern": "(?i)hello", "regex": true})
		params, errRes := parseSearchCodeParams(req)
		if errRes != nil {
			t.Fatalf("unexpected error result")
		}
		if params.re == nil {
			t.Fatal("expected compiled regex")
		}
	})

	t.Run("all_params", func(t *testing.T) {
		req := makeReq(map[string]any{
			"pattern":        "test",
			"file_pattern":   "*.go",
			"max_results":    float64(20),
			"offset":         float64(5),
			"regex":          false,
			"case_sensitive": true,
			"project":        "myproj",
		})
		params, errRes := parseSearchCodeParams(req)
		if errRes != nil {
			t.Fatalf("unexpected error result")
		}
		if params.fileGlob != "*.go" {
			t.Errorf("fileGlob = %q", params.fileGlob)
		}
		if params.maxResults != 20 {
			t.Errorf("maxResults = %d", params.maxResults)
		}
		if params.offset != 5 {
			t.Errorf("offset = %d", params.offset)
		}
		if params.project != "myproj" {
			t.Errorf("project = %q", params.project)
		}
	})
}

func TestLogZeroSymbolsDiag(t *testing.T) {
	t.Run("less_than_3_files", func(t *testing.T) {
		logZeroSymbolsDiag([]pipeline.ChangedFile{
			{Path: "a.go"},
		})
	})

	t.Run("more_than_3_files", func(t *testing.T) {
		logZeroSymbolsDiag([]pipeline.ChangedFile{
			{Path: "a.go"},
			{Path: "b.go"},
			{Path: "c.go"},
			{Path: "d.go"},
			{Path: "e.go"},
		})
	})
}
