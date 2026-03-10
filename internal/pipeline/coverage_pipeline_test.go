package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mightycogs/codebook/internal/cbm"
	"github.com/mightycogs/codebook/internal/discover"
	"github.com/mightycogs/codebook/internal/fqn"
	"github.com/mightycogs/codebook/internal/lang"
	"github.com/mightycogs/codebook/internal/store"
)

func TestExtractEnvURLSites_MapStringAny(t *testing.T) {
	node := &store.Node{
		Name:          "docker-compose.yml",
		QualifiedName: "proj.docker-compose-yml",
		Properties: map[string]any{
			"environment": map[string]any{
				"API_URL": "http://localhost:8080/api/v1",
				"DB_HOST": "just-a-hostname",
				"COUNT":   42,
			},
		},
	}
	sites := extractEnvURLSites(node, "environment")
	found := false
	for _, s := range sites {
		if s.SourceName == "docker-compose.yml" && s.SourceLabel == "InfraFile" {
			found = true
		}
	}
	if !found && len(sites) > 0 {
		t.Errorf("expected InfraFile source label in sites")
	}
}

func TestExtractEnvURLSites_MapStringString(t *testing.T) {
	node := &store.Node{
		Name:          "app.env",
		QualifiedName: "proj.app-env",
		Properties: map[string]any{
			"env_vars": map[string]string{
				"SERVICE_URL": "https://api.example.com/v2/users",
				"LOG_LEVEL":   "debug",
			},
		},
	}
	sites := extractEnvURLSites(node, "env_vars")
	if len(sites) == 0 {
		t.Error("expected at least one site from HTTPS URL")
	}
	for _, s := range sites {
		if s.SourceQualifiedName != "proj.app-env" {
			t.Errorf("expected source QN proj.app-env, got %q", s.SourceQualifiedName)
		}
	}
}

func TestExtractEnvURLSites_MissingProperty(t *testing.T) {
	node := &store.Node{
		Properties: map[string]any{},
	}
	sites := extractEnvURLSites(node, "nonexistent")
	if sites != nil {
		t.Errorf("expected nil for missing property, got %v", sites)
	}
}

func TestExtractEnvURLSites_WrongType(t *testing.T) {
	node := &store.Node{
		Properties: map[string]any{
			"env_vars": "not_a_map",
		},
	}
	sites := extractEnvURLSites(node, "env_vars")
	if len(sites) != 0 {
		t.Errorf("expected 0 sites for wrong type, got %d", len(sites))
	}
}

func TestURLSitesFromValue(t *testing.T) {
	node := &store.Node{
		Name:          "svc",
		QualifiedName: "proj.svc",
	}

	t.Run("https_url_with_path", func(t *testing.T) {
		sites := urlSitesFromValue(node, "https://myapp.internal/api/v1/users")
		if len(sites) == 0 {
			t.Error("expected sites from https URL with path")
		}
		for _, s := range sites {
			if s.SourceLabel != "InfraFile" {
				t.Errorf("expected InfraFile label, got %q", s.SourceLabel)
			}
			if s.SourceName != "svc" {
				t.Errorf("expected source name 'svc', got %q", s.SourceName)
			}
		}
	})

	t.Run("no_url", func(t *testing.T) {
		sites := urlSitesFromValue(node, "just-a-string")
		if len(sites) != 0 {
			t.Errorf("expected 0 sites for non-URL, got %d", len(sites))
		}
	})

	t.Run("empty", func(t *testing.T) {
		sites := urlSitesFromValue(node, "")
		if len(sites) != 0 {
			t.Errorf("expected 0 sites for empty string, got %d", len(sites))
		}
	})

	t.Run("no_path_segment", func(t *testing.T) {
		sites := urlSitesFromValue(node, "/v2")
		if len(sites) != 0 {
			t.Errorf("expected 0 sites for short path, got %d", len(sites))
		}
	})
}

func TestPickBestCandidate(t *testing.T) {
	t.Run("empty_candidates", func(t *testing.T) {
		r := pickBestCandidate(nil, "proj.caller", nil)
		if r.QualifiedName != "" {
			t.Errorf("expected empty for nil candidates, got %q", r.QualifiedName)
		}
	})

	t.Run("single_candidate", func(t *testing.T) {
		r := pickBestCandidate([]string{"proj.a.Foo"}, "proj.caller", nil)
		if r.QualifiedName != "" {
			t.Errorf("expected empty for single candidate (<=1 guard), got %q", r.QualifiedName)
		}
	})

	t.Run("two_candidates_no_import", func(t *testing.T) {
		candidates := []string{"proj.svc.Foo", "proj.other.Foo"}
		r := pickBestCandidate(candidates, "proj.svc.caller", nil)
		if r.QualifiedName != "proj.svc.Foo" {
			t.Errorf("expected proj.svc.Foo (closest), got %q", r.QualifiedName)
		}
		if r.Strategy != "suffix_match" {
			t.Errorf("expected suffix_match strategy, got %q", r.Strategy)
		}
	})

	t.Run("two_candidates_with_import_filtering", func(t *testing.T) {
		candidates := []string{"proj.svc.Foo", "proj.other.Foo"}
		imports := map[string]string{"other": "proj.other"}
		r := pickBestCandidate(candidates, "proj.caller", imports)
		if r.QualifiedName != "proj.other.Foo" {
			t.Errorf("expected proj.other.Foo (import reachable), got %q", r.QualifiedName)
		}
	})

	t.Run("all_filtered_out", func(t *testing.T) {
		candidates := []string{"proj.a.Foo", "proj.b.Foo"}
		imports := map[string]string{"c": "proj.c"}
		r := pickBestCandidate(candidates, "proj.a.caller", imports)
		if r.QualifiedName == "" {
			t.Error("expected fallback result when all filtered out")
		}
		if r.Confidence != 0.55*0.5 {
			t.Errorf("expected halved confidence 0.275, got %f", r.Confidence)
		}
	})
}

func TestResolveSuffixMatch_FullCalleeMatch(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Process", "proj.svc.handler.Process", "Function")
	reg.Register("Process", "proj.other.Process", "Function")

	candidates := reg.FindByName("Process")
	result := reg.resolveSuffixMatch("handler.Process", "Process", "proj.caller", nil, candidates)
	if result.QualifiedName != "proj.svc.handler.Process" {
		t.Errorf("expected proj.svc.handler.Process, got %q", result.QualifiedName)
	}
	if result.Strategy != "suffix_match" {
		t.Errorf("expected suffix_match, got %q", result.Strategy)
	}
}

func TestResolveSuffixMatch_MultipleMatches(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Process", "proj.a.Process", "Function")
	reg.Register("Process", "proj.b.Process", "Function")
	reg.Register("Process", "proj.c.Process", "Function")

	candidates := reg.FindByName("Process")
	result := reg.resolveSuffixMatch("x.Process", "Process", "proj.a.caller", nil, candidates)
	if result.QualifiedName == "" {
		t.Error("expected a match from multiple suffix matches")
	}
}

func TestResolveSuffixMatch_ImportFiltered(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Foo", "proj.a.Foo", "Function")
	reg.Register("Foo", "proj.b.Foo", "Function")

	imports := map[string]string{"a": "proj.a"}
	candidates := reg.FindByName("Foo")
	result := reg.resolveSuffixMatch("x.Foo", "Foo", "proj.caller", imports, candidates)
	if result.QualifiedName != "proj.a.Foo" {
		t.Errorf("expected proj.a.Foo (import reachable), got %q", result.QualifiedName)
	}
}

func TestResolveSuffixMatch_NoMatch(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Bar", "proj.a.Bar", "Function")

	result := reg.resolveSuffixMatch("x.Baz", "Baz", "proj.caller", nil, []string{"proj.a.Bar"})
	if result.QualifiedName != "" {
		t.Errorf("expected empty for no matching suffix, got %q", result.QualifiedName)
	}
}

func TestResolveAsClass(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("MyClass", "proj.pkg.MyClass", "Class")
	reg.Register("MyInterface", "proj.pkg.MyInterface", "Interface")
	reg.Register("MyType", "proj.pkg.MyType", "Type")
	reg.Register("MyEnum", "proj.pkg.MyEnum", "Enum")
	reg.Register("MyFunc", "proj.pkg.MyFunc", "Function")

	t.Run("class", func(t *testing.T) {
		qn := resolveAsClass("MyClass", reg, "proj.pkg", nil)
		if qn != "proj.pkg.MyClass" {
			t.Errorf("expected proj.pkg.MyClass, got %q", qn)
		}
	})

	t.Run("interface", func(t *testing.T) {
		qn := resolveAsClass("MyInterface", reg, "proj.pkg", nil)
		if qn != "proj.pkg.MyInterface" {
			t.Errorf("expected proj.pkg.MyInterface, got %q", qn)
		}
	})

	t.Run("type", func(t *testing.T) {
		qn := resolveAsClass("MyType", reg, "proj.pkg", nil)
		if qn != "proj.pkg.MyType" {
			t.Errorf("expected proj.pkg.MyType, got %q", qn)
		}
	})

	t.Run("enum", func(t *testing.T) {
		qn := resolveAsClass("MyEnum", reg, "proj.pkg", nil)
		if qn != "proj.pkg.MyEnum" {
			t.Errorf("expected proj.pkg.MyEnum, got %q", qn)
		}
	})

	t.Run("function_rejected", func(t *testing.T) {
		qn := resolveAsClass("MyFunc", reg, "proj.pkg", nil)
		if qn != "" {
			t.Errorf("expected empty for Function label, got %q", qn)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		qn := resolveAsClass("NonExistent", reg, "proj.pkg", nil)
		if qn != "" {
			t.Errorf("expected empty for missing name, got %q", qn)
		}
	})
}

func TestInferTypesCBM(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Order", "proj.models.Order", "Class")
	reg.Register("process", "proj.utils.process", "Function")

	typeAssigns := []cbm.TypeAssign{
		{VarName: "order", TypeName: "Order"},
		{VarName: "result", TypeName: "process"},
		{VarName: "", TypeName: "Order"},
		{VarName: "x", TypeName: ""},
	}

	tm := inferTypesCBM(typeAssigns, reg, "proj.pkg", nil)

	if tm["order"] != "proj.models.Order" {
		t.Errorf("expected order -> proj.models.Order, got %q", tm["order"])
	}
	if _, ok := tm["result"]; ok {
		t.Error("expected result to be absent (Function, not Class)")
	}
	if _, ok := tm["x"]; ok {
		t.Error("expected x to be absent (empty TypeName)")
	}
}

func TestContainsTestDir(t *testing.T) {
	tests := []struct {
		dir      string
		patterns []string
		want     bool
	}{
		{"src/test/java/com/example", []string{"src/test"}, true},
		{"__tests__/unit", []string{"__tests__"}, true},
		{"tests", []string{"tests"}, true},
		{"pkg/tests/util", []string{"tests"}, true},
		{"src/main/java", []string{"src/test"}, false},
		{".", []string{"tests"}, false},
	}
	for _, tt := range tests {
		got := containsTestDir(tt.dir, tt.patterns...)
		if got != tt.want {
			t.Errorf("containsTestDir(%q, %v) = %v, want %v", tt.dir, tt.patterns, got, tt.want)
		}
	}
}

func TestIsTestFile_TestDirBased(t *testing.T) {
	tests := []struct {
		path     string
		language lang.Language
		want     bool
	}{
		{"src/test/OrderService.java", lang.Java, true},
		{"src/main/OrderService.java", lang.Java, false},
		{"__tests__/handler.js", lang.JavaScript, true},
		{"src/handler.js", lang.JavaScript, false},
		{"tests/test_handler.py", lang.Python, true},
		{"tests/handler.rs", lang.Rust, true},
		{"src/handler.rs", lang.Rust, false},
		{"test/handler_test.cpp", lang.CPP, true},
		{"spec/handler_spec.lua", lang.Lua, true},
	}
	for _, tt := range tests {
		got := isTestFile(tt.path, tt.language)
		if got != tt.want {
			t.Errorf("isTestFile(%q, %v) = %v, want %v", tt.path, tt.language, got, tt.want)
		}
	}
}

func TestIsTestFile_UnknownLanguage(t *testing.T) {
	got := isTestFile("unknown_test.xyz", lang.Language("unknown"))
	if got {
		t.Error("expected false for unknown language")
	}
}

func TestIsTestFunction_JSSpecialCases(t *testing.T) {
	jsSpecial := []string{"describe", "it", "test", "beforeAll", "afterAll", "beforeEach", "afterEach"}
	for _, fn := range jsSpecial {
		if !isTestFunction(fn, lang.JavaScript) {
			t.Errorf("isTestFunction(%q, JavaScript) = false, want true", fn)
		}
		if !isTestFunction(fn, lang.TypeScript) {
			t.Errorf("isTestFunction(%q, TypeScript) = false, want true", fn)
		}
		if !isTestFunction(fn, lang.TSX) {
			t.Errorf("isTestFunction(%q, TSX) = false, want true", fn)
		}
	}
}

func TestIsTestFunction_JuliaSpecialCases(t *testing.T) {
	if !isTestFunction("@testset", lang.Julia) {
		t.Error("expected @testset to be a test function for Julia")
	}
	if !isTestFunction("@test", lang.Julia) {
		t.Error("expected @test to be a test function for Julia")
	}
	if isTestFunction("compute", lang.Julia) {
		t.Error("expected compute to not be a test function for Julia")
	}
}

func TestIsTestFunction_Suffixes(t *testing.T) {
	tests := []struct {
		name     string
		language lang.Language
		want     bool
	}{
		{"createOrderTest", lang.Java, true},
		{"OrderSpec", lang.Scala, true},
		{"handleRequestTest", lang.CSharp, true},
		{"processTest", lang.Kotlin, true},
		{"computeTest", lang.FSharp, true},
		{"notATestSuffix", lang.Java, false},
	}
	for _, tt := range tests {
		got := isTestFunction(tt.name, tt.language)
		if got != tt.want {
			t.Errorf("isTestFunction(%q, %v) = %v, want %v", tt.name, tt.language, got, tt.want)
		}
	}
}

func TestIsTestFunction_NoMatch(t *testing.T) {
	if isTestFunction("regularFunc", lang.Go) {
		t.Error("expected false for non-test function")
	}
	if isTestFunction("helper", lang.Python) {
		t.Error("expected false for non-test function")
	}
}

func TestLangFromFilePath(t *testing.T) {
	tests := []struct {
		path string
		want lang.Language
	}{
		{"src/main.go", lang.Go},
		{"handler.py", lang.Python},
		{"app.ts", lang.TypeScript},
		{"module.rs", lang.Rust},
		{"Main.java", lang.Java},
		{"unknown.xyz", ""},
	}
	for _, tt := range tests {
		got := langFromFilePath(tt.path)
		if got != tt.want {
			t.Errorf("langFromFilePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestTestFileToProductionFile_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"deeply/nested/dir/foo_test.go", "deeply/nested/dir/foo.go"},
		{"test_utils.py", "utils.py"},
		{"component.test.jsx", "component.jsx"},
		{"component.spec.jsx", "component.jsx"},
		{"lib.rs", ""},
		{"Makefile", ""},
		{"noext", ""},
	}
	for _, tt := range tests {
		got := testFileToProductionFile(tt.input)
		if got != tt.want {
			t.Errorf("testFileToProductionFile(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestOnTick_WarmupPhase(t *testing.T) {
	p := newAdaptivePool(4)
	ms := &monitorState{numCPU: 4.0}

	p.onTick(ms)
	if ms.tick != 1 {
		t.Errorf("expected tick 1, got %d", ms.tick)
	}

	p.onTick(ms)
	if ms.tick != 2 {
		t.Errorf("expected tick 2, got %d", ms.tick)
	}

	p.onTick(ms)
	if ms.tick != 3 {
		t.Errorf("expected tick 3, got %d", ms.tick)
	}
}

func TestOnTick_PostWarmupCalibration(t *testing.T) {
	p := newAdaptivePool(4)
	ms := &monitorState{numCPU: 4.0}

	for i := 0; i < monitorWarmupTicks; i++ {
		p.onTick(ms)
	}

	p.bytesProcessed.Add(1000000)
	p.onTick(ms)
	if ms.tick != monitorWarmupTicks+1 {
		t.Errorf("expected tick %d, got %d", monitorWarmupTicks+1, ms.tick)
	}
}

func TestOnTick_CooldownSkip(t *testing.T) {
	p := newAdaptivePool(4)
	ms := &monitorState{
		tick:     monitorWarmupTicks + 2,
		numCPU:   4.0,
		cooldown: 2,
		emaBPS:   5000.0,
		baseBPS:  1000.0,
	}
	p.onTick(ms)
	if ms.cooldown != 1 {
		t.Errorf("expected cooldown decremented to 1, got %d", ms.cooldown)
	}
}

func TestOnTick_CooldownExpiry(t *testing.T) {
	p := newAdaptivePool(4)
	ms := &monitorState{
		tick:     monitorWarmupTicks + 3,
		numCPU:   4.0,
		cooldown: 1,
		emaBPS:   5000.0,
		baseBPS:  1000.0,
	}
	p.onTick(ms)
	if ms.cooldown != 0 {
		t.Errorf("expected cooldown at 0, got %d", ms.cooldown)
	}
	if ms.baseBPS != ms.emaBPS {
		t.Errorf("expected baseBPS reset to emaBPS on cooldown expiry")
	}
}

func TestOnTick_GrowPhase(t *testing.T) {
	p := newAdaptivePool(4)
	ms := &monitorState{
		tick:    monitorWarmupTicks + 5,
		numCPU:  4.0,
		emaBPS:  2000.0,
		baseBPS: 1000.0,
	}
	initial := p.currentLimit()
	p.onTick(ms)
	if p.currentLimit() <= initial {
		t.Errorf("expected growth, limit was %d, now %d", initial, p.currentLimit())
	}
}

func TestOnTick_ShrinkCondition(t *testing.T) {
	p := newAdaptivePool(4)
	p.setLimit(16)

	ms := &monitorState{
		tick:       monitorWarmupTicks + 5,
		numCPU:     4.0,
		tier:       tierCPU,
		emaBPS:     500.0,
		baseBPS:    1000.0,
		emaCPUUtil: 0.95,
	}
	if !p.hasContention(ms) {
		t.Error("expected contention with high CPU util")
	}
	if !(ms.emaBPS < ms.baseBPS*monitorShrinkBPS) {
		t.Error("expected throughput decline")
	}
	if !(p.currentLimit() > p.minLimit) {
		t.Error("expected limit above min")
	}
}

func TestEnrichModuleNodeCBM(t *testing.T) {
	moduleNode := &store.Node{
		Properties: map[string]any{},
	}
	cbmResult := &cbm.FileResult{}
	enrichModuleNodeCBM(moduleNode, cbmResult, nil)
}

func TestResolveFileThrowsCBM(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "test-proj"
	_ = s.UpsertProject(project, "/tmp/test")

	reg := NewFunctionRegistry()
	reg.Register("IOException", "proj.java-io.IOException", "Class")

	p := &Pipeline{
		ctx:             context.Background(),
		Store:           s,
		ProjectName:     project,
		registry:        reg,
		importMaps:      map[string]map[string]string{},
		extractionCache: map[string]*cachedExtraction{},
	}

	ext := &cachedExtraction{
		Language: lang.Java,
		Result: &cbm.FileResult{
			Throws: []cbm.Throw{
				{ExceptionName: "IOException", EnclosingFuncQN: "proj.svc.Service.process"},
				{ExceptionName: "RuntimeException", EnclosingFuncQN: "proj.svc.Service.handle"},
				{ExceptionName: "", EnclosingFuncQN: "proj.svc.Service.empty"},
				{ExceptionName: "Error", EnclosingFuncQN: ""},
				{ExceptionName: "IOException", EnclosingFuncQN: "proj.svc.Service.process"},
			},
		},
	}

	edges := p.resolveFileThrowsCBM("svc/Service.java", ext)

	throwsCount := 0
	raisesCount := 0
	for _, e := range edges {
		switch e.Type {
		case "THROWS":
			throwsCount++
		case "RAISES":
			raisesCount++
		}
	}

	if throwsCount != 1 {
		t.Errorf("expected 1 THROWS edge (IOException is checked), got %d", throwsCount)
	}
	if raisesCount != 1 {
		t.Errorf("expected 1 RAISES edge (RuntimeException is unchecked), got %d", raisesCount)
	}
}

func TestResolveFileThrowsCBM_Dedup(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "test-proj"
	_ = s.UpsertProject(project, "/tmp/test")

	p := &Pipeline{
		ctx:             context.Background(),
		Store:           s,
		ProjectName:     project,
		registry:        NewFunctionRegistry(),
		importMaps:      map[string]map[string]string{},
		extractionCache: map[string]*cachedExtraction{},
	}

	ext := &cachedExtraction{
		Language: lang.Python,
		Result: &cbm.FileResult{
			Throws: []cbm.Throw{
				{ExceptionName: "ValueError", EnclosingFuncQN: "proj.mod.func1"},
				{ExceptionName: "ValueError", EnclosingFuncQN: "proj.mod.func1"},
				{ExceptionName: "ValueError", EnclosingFuncQN: "proj.mod.func1"},
			},
		},
	}

	edges := p.resolveFileThrowsCBM("mod/handler.py", ext)
	if len(edges) != 1 {
		t.Errorf("expected 1 edge after dedup, got %d", len(edges))
	}
}

func TestFuzzyResolve_MultipleCandidates_AllUnreachable(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Handler", "proj.a.Handler", "Function")
	reg.Register("Handler", "proj.b.Handler", "Function")

	imports := map[string]string{"c": "proj.c"}
	result, ok := reg.FuzzyResolve("unknown.Handler", "proj.caller", imports)
	if !ok {
		t.Fatal("expected fuzzy match even when all unreachable")
	}
	if result.Confidence != 0.30*0.5 {
		t.Errorf("expected 0.15 confidence (penalized), got %f", result.Confidence)
	}
}

func TestFuzzyResolve_MultipleCandidates_OneReachable(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Handler", "proj.a.Handler", "Function")
	reg.Register("Handler", "proj.b.Handler", "Function")

	imports := map[string]string{"a": "proj.a"}
	result, ok := reg.FuzzyResolve("unknown.Handler", "proj.caller", imports)
	if !ok {
		t.Fatal("expected fuzzy match")
	}
	if result.QualifiedName != "proj.a.Handler" {
		t.Errorf("expected proj.a.Handler (import reachable), got %q", result.QualifiedName)
	}
	if result.Confidence != 0.40 {
		t.Errorf("expected 0.40 confidence for single filtered, got %f", result.Confidence)
	}
}

func TestResolve_UniqueNameWithImportPenalty(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("UniqueFunc", "proj.remote.UniqueFunc", "Function")

	imports := map[string]string{"local": "proj.local"}
	result := reg.Resolve("UniqueFunc", "proj.caller", imports)
	if result.QualifiedName != "proj.remote.UniqueFunc" {
		t.Fatalf("expected proj.remote.UniqueFunc, got %q", result.QualifiedName)
	}
	if result.Confidence != 0.75*0.5 {
		t.Errorf("expected 0.375 confidence (penalized), got %f", result.Confidence)
	}
}

func TestResolveViaSameModule_QualifiedSuffix(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("method", "proj.pkg.method", "Method")

	result := reg.Resolve("obj.method", "proj.pkg", nil)
	if result.QualifiedName != "proj.pkg.method" {
		t.Errorf("expected proj.pkg.method via same_module suffix, got %q", result.QualifiedName)
	}
	if result.Strategy != "same_module" {
		t.Errorf("expected same_module strategy, got %q", result.Strategy)
	}
}

func TestCreateTestsFileEdges(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "test-proj"
	_ = s.UpsertProject(project, "/tmp/test")

	_, _ = s.UpsertNode(&store.Node{
		Project: project, Label: "Module", Name: "handler.go",
		QualifiedName: "proj.handler-go", FilePath: "handler.go",
	})
	_, _ = s.UpsertNode(&store.Node{
		Project: project, Label: "Module", Name: "handler_test.go",
		QualifiedName: "proj.handler_test-go", FilePath: "handler_test.go",
	})

	p := &Pipeline{
		ctx:         context.Background(),
		Store:       s,
		ProjectName: project,
	}
	count := p.createTestsFileEdges()
	if count != 1 {
		t.Errorf("expected 1 TESTS_FILE edge, got %d", count)
	}
}

func TestCreateTestsFileEdges_NoTestModules(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "test-proj"
	_ = s.UpsertProject(project, "/tmp/test")

	_, _ = s.UpsertNode(&store.Node{
		Project: project, Label: "Module", Name: "handler.go",
		QualifiedName: "proj.handler-go", FilePath: "handler.go",
	})

	p := &Pipeline{
		ctx:         context.Background(),
		Store:       s,
		ProjectName: project,
	}
	count := p.createTestsFileEdges()
	if count != 0 {
		t.Errorf("expected 0 TESTS_FILE edges (no test modules), got %d", count)
	}
}

func TestCreateTestsFileEdges_ViaImports(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "test-proj"
	_ = s.UpsertProject(project, "/tmp/test")

	prodID, _ := s.UpsertNode(&store.Node{
		Project: project, Label: "Module", Name: "handler.py",
		QualifiedName: "proj.handler-py", FilePath: "handler.py",
	})
	testID, _ := s.UpsertNode(&store.Node{
		Project: project, Label: "Module", Name: "test_utils.py",
		QualifiedName: "proj.test_utils-py", FilePath: "test_utils.py",
	})

	_, _ = s.InsertEdge(&store.Edge{
		Project: project, SourceID: testID, TargetID: prodID, Type: "IMPORTS",
	})

	p := &Pipeline{
		ctx:         context.Background(),
		Store:       s,
		ProjectName: project,
	}
	count := p.createTestsFileEdges()
	if count != 1 {
		t.Errorf("expected 1 TESTS_FILE edge via imports, got %d", count)
	}
}

func TestPassThrowsIntegration(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "test-proj"
	_ = s.UpsertProject(project, "/tmp/test")

	reg := NewFunctionRegistry()
	reg.Register("ValueError", "proj.builtins.ValueError", "Class")

	funcID, _ := s.UpsertNode(&store.Node{
		Project: project, Label: "Function", Name: "process",
		QualifiedName: "proj.mod-handler-py.process", FilePath: "mod/handler.py",
	})
	valErrID, _ := s.UpsertNode(&store.Node{
		Project: project, Label: "Class", Name: "ValueError",
		QualifiedName: "proj.builtins.ValueError", FilePath: "builtins.py",
	})

	p := &Pipeline{
		ctx:         context.Background(),
		Store:       s,
		ProjectName: project,
		registry:    reg,
		importMaps: map[string]map[string]string{
			"proj.mod-handler-py": {"builtins": "proj.builtins"},
		},
		extractionCache: map[string]*cachedExtraction{
			"mod/handler.py": {
				Language: lang.Python,
				Result: &cbm.FileResult{
					Throws: []cbm.Throw{
						{ExceptionName: "ValueError", EnclosingFuncQN: "proj.mod-handler-py.process"},
					},
				},
			},
		},
	}

	p.passThrows()

	edges, _ := s.FindEdgesBySourceAndType(funcID, "RAISES")
	if len(edges) != 1 {
		t.Errorf("expected 1 RAISES edge, got %d", len(edges))
		return
	}
	if edges[0].TargetID != valErrID {
		t.Errorf("expected target %d, got %d", valErrID, edges[0].TargetID)
	}
}

func TestConfidenceBand_Boundaries(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{1.0, "high"},
		{0.7, "high"},
		{0.69, "medium"},
		{0.45, "medium"},
		{0.449, "speculative"},
		{0.0, "speculative"},
		{-1.0, "speculative"},
	}
	for _, tt := range tests {
		got := confidenceBand(tt.score)
		if got != tt.want {
			t.Errorf("confidenceBand(%f) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestIsEnvVarName_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"DB_URL", true},
		{"A", false},
		{"AB", true},
		{"A1", true},
		{"a_b", false},
		{"123", false},
		{"_A", true},
		{"", false},
		{"DATABASE_URL_123", true},
		{"MixedCase", false},
	}
	for _, tt := range tests {
		got := isEnvVarName(tt.input)
		if got != tt.want {
			t.Errorf("isEnvVarName(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestResolveViaImportMap_NilMap(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Foo", "proj.pkg.Foo", "Function")

	result := reg.resolveViaImportMap("pkg", "Foo", nil)
	if result.QualifiedName != "" {
		t.Errorf("expected empty for nil import map, got %q", result.QualifiedName)
	}
}

func TestResolveViaImportMap_PrefixNotFound(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Foo", "proj.pkg.Foo", "Function")

	imports := map[string]string{"other": "proj.other"}
	result := reg.resolveViaImportMap("pkg", "Foo", imports)
	if result.QualifiedName != "" {
		t.Errorf("expected empty for missing prefix, got %q", result.QualifiedName)
	}
}

func TestResolveViaImportMap_NoSuffix(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("handler", "proj.handler", "Function")

	imports := map[string]string{"handler": "proj.handler"}
	result := reg.resolveViaImportMap("handler", "", imports)
	if result.QualifiedName != "proj.handler" {
		t.Errorf("expected proj.handler, got %q", result.QualifiedName)
	}
	if result.Strategy != "import_map" {
		t.Errorf("expected import_map, got %q", result.Strategy)
	}
}

func TestResolveViaSameModule_NotFound(t *testing.T) {
	reg := NewFunctionRegistry()

	result := reg.resolveViaSameModule("Missing", "", "proj.pkg")
	if result.QualifiedName != "" {
		t.Errorf("expected empty, got %q", result.QualifiedName)
	}
}

func TestResolveViaNameLookup_NoCandidates(t *testing.T) {
	reg := NewFunctionRegistry()

	result := reg.resolveViaNameLookup("Missing", "", "proj.pkg", nil)
	if result.QualifiedName != "" {
		t.Errorf("expected empty for no candidates, got %q", result.QualifiedName)
	}
}

func TestAdaptivePoolAcquireRelease(t *testing.T) {
	p := newAdaptivePool(4)
	p.acquire()
	p.mu.Lock()
	if p.active != 1 {
		t.Errorf("expected active=1 after acquire, got %d", p.active)
	}
	p.mu.Unlock()

	p.releaseBytes(1024)
	p.mu.Lock()
	if p.active != 0 {
		t.Errorf("expected active=0 after release, got %d", p.active)
	}
	p.mu.Unlock()

	if p.completed.Load() != 1 {
		t.Errorf("expected completed=1, got %d", p.completed.Load())
	}
	if p.bytesProcessed.Load() != 1024 {
		t.Errorf("expected bytesProcessed=1024, got %d", p.bytesProcessed.Load())
	}
}

func TestAdaptivePoolStop(t *testing.T) {
	p := newAdaptivePool(4)
	p.bytesProcessed.Add(1000)
	p.completed.Add(5)
	p.peakBPS = 500.0
	p.peakLimit = 8
	p.stop()
}

func TestResolveViaImportMap_ExactMatch(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Foo", "proj.other.Foo", "Function")

	imports := map[string]string{"other": "proj.other"}
	result := reg.resolveViaImportMap("other", "Foo", imports)
	if result.QualifiedName != "proj.other.Foo" {
		t.Errorf("expected proj.other.Foo, got %q", result.QualifiedName)
	}
	if result.Confidence != 0.95 {
		t.Errorf("expected 0.95 confidence, got %f", result.Confidence)
	}
}

func TestResolveViaImportMap_SuffixMatch(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Foo", "proj.other.sub.Foo", "Function")

	imports := map[string]string{"other": "proj.other"}
	result := reg.resolveViaImportMap("other", "Foo", imports)
	if result.QualifiedName != "proj.other.sub.Foo" {
		t.Errorf("expected proj.other.sub.Foo, got %q", result.QualifiedName)
	}
	if result.Confidence != 0.85 {
		t.Errorf("expected 0.85 confidence, got %f", result.Confidence)
	}
	if result.Strategy != "import_map_suffix" {
		t.Errorf("expected import_map_suffix, got %q", result.Strategy)
	}
}

func TestResolveViaImportMap_NoExactNoSuffix(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Bar", "proj.other.Bar", "Function")

	imports := map[string]string{"other": "proj.other"}
	result := reg.resolveViaImportMap("other", "Foo", imports)
	if result.QualifiedName != "" {
		t.Errorf("expected empty when no match, got %q", result.QualifiedName)
	}
}

func TestStripBOM_Coverage(t *testing.T) {
	t.Run("with_bom", func(t *testing.T) {
		src := append([]byte{0xEF, 0xBB, 0xBF}, []byte("hello")...)
		got := stripBOM(src)
		if string(got) != "hello" {
			t.Errorf("expected 'hello', got %q", string(got))
		}
	})

	t.Run("without_bom", func(t *testing.T) {
		src := []byte("hello")
		got := stripBOM(src)
		if string(got) != "hello" {
			t.Errorf("expected 'hello', got %q", string(got))
		}
	})

	t.Run("short_input", func(t *testing.T) {
		got := stripBOM([]byte{0xEF})
		if len(got) != 1 {
			t.Errorf("expected length 1, got %d", len(got))
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := stripBOM(nil)
		if len(got) != 0 {
			t.Errorf("expected empty, got %d bytes", len(got))
		}
	})
}

func TestProjectNameFromPath_Coverage(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "root"},
		{"/home/user/project", "home-user-project"},
		{"/", "root"},
		{"/a/b/c", "a-b-c"},
	}
	for _, tt := range tests {
		got := ProjectNameFromPath(tt.input)
		if got != tt.want {
			t.Errorf("ProjectNameFromPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCheckCancel(t *testing.T) {
	t.Run("active_context", func(t *testing.T) {
		p := &Pipeline{ctx: context.Background()}
		if err := p.checkCancel(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("canceled_context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		p := &Pipeline{ctx: ctx}
		if err := p.checkCancel(); err == nil {
			t.Fatal("expected context cancellation error")
		}
	})
}

func TestClassifyFiles(t *testing.T) {
	t.Run("no_stored_hashes_means_full_index", func(t *testing.T) {
		s, err := store.OpenMemory()
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()

		project := "proj"
		if err := s.UpsertProject(project, t.TempDir()); err != nil {
			t.Fatal(err)
		}

		p := &Pipeline{ctx: context.Background(), Store: s, ProjectName: project}
		files := []discover.FileInfo{{Path: "/tmp/a.go", RelPath: "a.go"}}

		changed, unchanged := p.classifyFiles(files)
		if len(changed) != 1 || len(unchanged) != 0 {
			t.Fatalf("expected full index behavior, got changed=%d unchanged=%d", len(changed), len(unchanged))
		}
	})

	t.Run("splits_changed_unchanged_and_hash_errors", func(t *testing.T) {
		s, err := store.OpenMemory()
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()

		repo := t.TempDir()
		project := "proj"
		if err := s.UpsertProject(project, repo); err != nil {
			t.Fatal(err)
		}

		samePath := filepath.Join(repo, "same.go")
		changedPath := filepath.Join(repo, "changed.go")
		if err := os.WriteFile(samePath, []byte("package main\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(changedPath, []byte("package main\nvar x = 1\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		sameHash, err := fileHash(samePath)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.UpsertFileHash(project, "same.go", sameHash); err != nil {
			t.Fatal(err)
		}
		if err := s.UpsertFileHash(project, "changed.go", "oldhash"); err != nil {
			t.Fatal(err)
		}
		if err := s.UpsertFileHash(project, "missing.go", "missinghash"); err != nil {
			t.Fatal(err)
		}

		p := &Pipeline{ctx: context.Background(), Store: s, ProjectName: project}
		files := []discover.FileInfo{
			{Path: samePath, RelPath: "same.go"},
			{Path: changedPath, RelPath: "changed.go"},
			{Path: filepath.Join(repo, "missing.go"), RelPath: "missing.go"},
			{Path: filepath.Join(repo, "new.go"), RelPath: "new.go"},
		}
		if err := os.WriteFile(files[3].Path, []byte("package main\nvar y = 2\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		changed, unchanged := p.classifyFiles(files)
		if len(unchanged) != 1 || unchanged[0].RelPath != "same.go" {
			t.Fatalf("expected only same.go unchanged, got %+v", unchanged)
		}

		gotChanged := map[string]bool{}
		for _, f := range changed {
			gotChanged[f.RelPath] = true
		}
		for _, want := range []string{"changed.go", "missing.go", "new.go"} {
			if !gotChanged[want] {
				t.Fatalf("expected %s in changed set, got %+v", want, gotChanged)
			}
		}
	})
}

func TestImportAdjustedConfidence_EdgeCases(t *testing.T) {
	t.Run("empty_import_map", func(t *testing.T) {
		imports := map[string]string{}
		got := importAdjustedConfidence(0.55, "proj.remote.Func", imports)
		if got != 0.275 {
			t.Errorf("expected 0.275, got %f", got)
		}
	})

	t.Run("zero_base", func(t *testing.T) {
		imports := map[string]string{"a": "proj.a"}
		got := importAdjustedConfidence(0.0, "proj.b.Func", imports)
		if got != 0.0 {
			t.Errorf("expected 0.0, got %f", got)
		}
	})
}

func TestIsImportReachable_EdgeCases(t *testing.T) {
	t.Run("candidate_prefix_of_import", func(t *testing.T) {
		imports := map[string]string{"handler": "proj.handler.sub.deep"}
		got := isImportReachable("proj.handler.Func", imports)
		if !got {
			t.Error("expected reachable when import starts with candidate module")
		}
	})

	t.Run("empty_map", func(t *testing.T) {
		got := isImportReachable("proj.handler.Func", map[string]string{})
		if got {
			t.Error("expected unreachable with empty import map")
		}
	})
}

func TestCommonPrefixLen_EdgeCases(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"single", "single", 1},
		{"a", "b", 0},
		{"a.b.c.d", "a.b.x.y", 2},
	}
	for _, tt := range tests {
		got := commonPrefixLen(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("commonPrefixLen(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestModulePrefix_EdgeCases(t *testing.T) {
	if got := modulePrefix(""); got != "" {
		t.Errorf("modulePrefix('') = %q, want empty", got)
	}
	if got := modulePrefix("a.b.c"); got != "a.b" {
		t.Errorf("modulePrefix('a.b.c') = %q, want 'a.b'", got)
	}
}

func TestSimpleName_EdgeCases(t *testing.T) {
	if got := simpleName(""); got != "" {
		t.Errorf("simpleName('') = %q, want empty", got)
	}
	if got := simpleName(".foo"); got != "foo" {
		t.Errorf("simpleName('.foo') = %q, want 'foo'", got)
	}
}

func TestBuildReturnTypeMap(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "test-proj"
	_ = s.UpsertProject(project, "/tmp/test")

	reg := NewFunctionRegistry()
	reg.Register("Order", "proj.models.Order", "Class")

	_, _ = s.UpsertNode(&store.Node{
		Project: project, Label: "Function", Name: "createOrder",
		QualifiedName: "proj.svc.createOrder", FilePath: "svc.py",
		Properties: map[string]any{
			"return_types": []any{"Order"},
		},
	})
	_, _ = s.UpsertNode(&store.Node{
		Project: project, Label: "Function", Name: "getCount",
		QualifiedName: "proj.svc.getCount", FilePath: "svc.py",
		Properties: map[string]any{
			"return_types": []any{"int"},
		},
	})
	_, _ = s.UpsertNode(&store.Node{
		Project: project, Label: "Function", Name: "noReturn",
		QualifiedName: "proj.svc.noReturn", FilePath: "svc.py",
		Properties: map[string]any{},
	})
	_, _ = s.UpsertNode(&store.Node{
		Project: project, Label: "Function", Name: "emptyTypes",
		QualifiedName: "proj.svc.emptyTypes", FilePath: "svc.py",
		Properties: map[string]any{
			"return_types": []any{},
		},
	})
	_, _ = s.UpsertNode(&store.Node{
		Project: project, Label: "Method", Name: "methodWithReturn",
		QualifiedName: "proj.svc.methodWithReturn", FilePath: "svc.py",
		Properties: map[string]any{
			"return_types": []any{"Order"},
		},
	})

	p := &Pipeline{
		ctx:         context.Background(),
		Store:       s,
		ProjectName: project,
		registry:    reg,
		importMaps:  map[string]map[string]string{},
	}

	p.buildReturnTypeMap()

	if p.returnTypes["proj.svc.createOrder"] != "proj.models.Order" {
		t.Errorf("expected createOrder -> proj.models.Order, got %q", p.returnTypes["proj.svc.createOrder"])
	}
	if _, ok := p.returnTypes["proj.svc.getCount"]; ok {
		t.Error("expected getCount absent (int is not a Class)")
	}
	if _, ok := p.returnTypes["proj.svc.noReturn"]; ok {
		t.Error("expected noReturn absent")
	}
	if p.returnTypes["proj.svc.methodWithReturn"] != "proj.models.Order" {
		t.Errorf("expected methodWithReturn -> proj.models.Order, got %q", p.returnTypes["proj.svc.methodWithReturn"])
	}
}

func TestResolveFileUsagesCBM(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "test-proj"
	_ = s.UpsertProject(project, "/tmp/test")

	reg := NewFunctionRegistry()
	reg.Register("Config", "proj.config.Config", "Class")

	p := &Pipeline{
		ctx:             context.Background(),
		Store:           s,
		ProjectName:     project,
		registry:        reg,
		importMaps:      map[string]map[string]string{},
		extractionCache: map[string]*cachedExtraction{},
	}

	ext := &cachedExtraction{
		Language: lang.Python,
		Result: &cbm.FileResult{
			Usages: []cbm.Usage{
				{RefName: "Config", EnclosingFuncQN: "proj.mod-handler-py.process"},
				{RefName: "Config", EnclosingFuncQN: "proj.mod-handler-py.process"},
				{RefName: "", EnclosingFuncQN: "proj.mod-handler-py.process"},
				{RefName: "Unknown", EnclosingFuncQN: "proj.mod-handler-py.process"},
				{RefName: "Config", EnclosingFuncQN: ""},
			},
		},
	}

	edges := p.resolveFileUsagesCBM("mod/handler.py", ext)
	usageCount := 0
	for _, e := range edges {
		if e.Type == "USAGE" {
			usageCount++
		}
	}
	if usageCount != 2 {
		t.Errorf("expected 2 USAGE edges (deduped + module-level), got %d", usageCount)
	}
}

func TestResolveFileReadWritesCBM(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "test-proj"
	_ = s.UpsertProject(project, "/tmp/test")

	reg := NewFunctionRegistry()
	reg.Register("counter", "proj.globals.counter", "Variable")

	p := &Pipeline{
		ctx:             context.Background(),
		Store:           s,
		ProjectName:     project,
		registry:        reg,
		importMaps:      map[string]map[string]string{},
		extractionCache: map[string]*cachedExtraction{},
	}

	ext := &cachedExtraction{
		Language: lang.Python,
		Result: &cbm.FileResult{
			ReadWrites: []cbm.ReadWrite{
				{VarName: "counter", EnclosingFuncQN: "proj.mod.func1", IsWrite: false},
				{VarName: "counter", EnclosingFuncQN: "proj.mod.func1", IsWrite: true},
				{VarName: "counter", EnclosingFuncQN: "proj.mod.func1", IsWrite: false},
				{VarName: "", EnclosingFuncQN: "proj.mod.func1", IsWrite: false},
				{VarName: "unknown", EnclosingFuncQN: "proj.mod.func1", IsWrite: false},
			},
		},
	}

	edges := p.resolveFileReadsWritesCBM("mod/handler.py", ext)
	reads := 0
	writes := 0
	for _, e := range edges {
		if e.Type == "READS" {
			reads++
		}
		if e.Type == "WRITES" {
			writes++
		}
	}
	if reads != 1 {
		t.Errorf("expected 1 READS edge, got %d", reads)
	}
	if writes != 1 {
		t.Errorf("expected 1 WRITES edge, got %d", writes)
	}
}

func TestResolveFileTypeRefsCBM(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "test-proj"
	_ = s.UpsertProject(project, "/tmp/test")

	reg := NewFunctionRegistry()
	reg.Register("Order", "proj.models.Order", "Class")

	p := &Pipeline{
		ctx:             context.Background(),
		Store:           s,
		ProjectName:     project,
		registry:        reg,
		importMaps:      map[string]map[string]string{},
		extractionCache: map[string]*cachedExtraction{},
	}

	ext := &cachedExtraction{
		Language: lang.Java,
		Result: &cbm.FileResult{
			TypeRefs: []cbm.TypeRef{
				{TypeName: "Order", EnclosingFuncQN: "proj.svc.Service.process"},
				{TypeName: "Order", EnclosingFuncQN: "proj.svc.Service.process"},
				{TypeName: "", EnclosingFuncQN: "proj.svc.Service.process"},
				{TypeName: "Unknown", EnclosingFuncQN: "proj.svc.Service.process"},
			},
		},
	}

	edges := p.resolveFileTypeRefsCBM("svc/Service.java", ext)
	usesTypeCount := 0
	for _, e := range edges {
		if e.Type == "USES_TYPE" {
			usesTypeCount++
		}
	}
	if usesTypeCount != 1 {
		t.Errorf("expected 1 USES_TYPE edge (deduped), got %d", usesTypeCount)
	}
}

func TestResolveFileConfiguresCBM(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "test-proj"
	_ = s.UpsertProject(project, "/tmp/test")

	p := &Pipeline{
		ctx:             context.Background(),
		Store:           s,
		ProjectName:     project,
		registry:        NewFunctionRegistry(),
		importMaps:      map[string]map[string]string{},
		extractionCache: map[string]*cachedExtraction{},
	}

	envIndex := map[string]string{
		"DB_URL":  "proj.config-env",
		"API_KEY": "proj.config-env",
	}

	ext := &cachedExtraction{
		Language: lang.Python,
		Result: &cbm.FileResult{
			EnvAccesses: []cbm.EnvAccess{
				{EnvKey: "DB_URL", EnclosingFuncQN: "proj.mod.func1"},
				{EnvKey: "DB_URL", EnclosingFuncQN: "proj.mod.func1"},
				{EnvKey: "API_KEY", EnclosingFuncQN: "proj.mod.func2"},
				{EnvKey: "", EnclosingFuncQN: "proj.mod.func1"},
				{EnvKey: "UNKNOWN_KEY", EnclosingFuncQN: "proj.mod.func1"},
				{EnvKey: "DB_URL", EnclosingFuncQN: ""},
			},
		},
	}

	edges := p.resolveFileConfiguresCBM("mod/handler.py", ext, envIndex)
	configCount := 0
	for _, e := range edges {
		if e.Type == "CONFIGURES" {
			configCount++
		}
	}
	if configCount != 2 {
		t.Errorf("expected 2 CONFIGURES edges (DB_URL from func1 + API_KEY from func2), got %d", configCount)
	}
}

func TestResolveFileConfiguresCBM_DedupByEnvKey(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "test-proj"
	_ = s.UpsertProject(project, "/tmp/test")

	p := &Pipeline{
		ctx:             context.Background(),
		Store:           s,
		ProjectName:     project,
		registry:        NewFunctionRegistry(),
		importMaps:      map[string]map[string]string{},
		extractionCache: map[string]*cachedExtraction{},
	}

	envIndex := map[string]string{
		"DB_URL":  "proj.config-env",
		"API_KEY": "proj.config-env",
	}

	ext := &cachedExtraction{
		Language: lang.Python,
		Result: &cbm.FileResult{
			EnvAccesses: []cbm.EnvAccess{
				{EnvKey: "DB_URL", EnclosingFuncQN: "proj.mod.func1"},
				{EnvKey: "API_KEY", EnclosingFuncQN: "proj.mod.func1"},
				{EnvKey: "DB_URL", EnclosingFuncQN: "proj.mod.func1"},
			},
		},
	}

	edges := p.resolveFileConfiguresCBM("mod/handler.py", ext, envIndex)
	if len(edges) != 2 {
		t.Fatalf("expected 2 CONFIGURES edges for distinct env keys in one function, got %d", len(edges))
	}

	gotKeys := map[string]bool{}
	for _, e := range edges {
		envKey, _ := e.Properties["env_key"].(string)
		gotKeys[envKey] = true
	}
	if !gotKeys["DB_URL"] || !gotKeys["API_KEY"] {
		t.Fatalf("expected DB_URL and API_KEY edges, got %+v", gotKeys)
	}
}

func TestFilterImportReachable_AllReachable(t *testing.T) {
	imports := map[string]string{"a": "proj.a", "b": "proj.b"}
	candidates := []string{"proj.a.Foo", "proj.b.Bar"}
	filtered := filterImportReachable(candidates, imports)
	if len(filtered) != 2 {
		t.Errorf("expected 2 reachable, got %d", len(filtered))
	}
}

func TestFilterImportReachable_NoneReachable(t *testing.T) {
	imports := map[string]string{"c": "proj.c"}
	candidates := []string{"proj.a.Foo", "proj.b.Bar"}
	filtered := filterImportReachable(candidates, imports)
	if len(filtered) != 0 {
		t.Errorf("expected 0 reachable, got %d", len(filtered))
	}
}

func TestDecoratorFunctionName_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"@", ""},
		{"@func()", "func"},
		{"@no_parens", "no_parens"},
		{"@a.b.c(x)", "a.b.c"},
		{"  @spaced  ", "spaced"},
	}
	for _, tt := range tests {
		got := decoratorFunctionName(tt.input)
		if got != tt.want {
			t.Errorf("decoratorFunctionName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractDecoratorWords_NonStringEntries(t *testing.T) {
	n := &store.Node{
		Properties: map[string]any{
			"decorators": []any{42, true, nil, "@valid_decorator"},
		},
	}
	words := extractDecoratorWords(n)
	if len(words) == 0 {
		t.Error("expected at least one word from valid_decorator")
	}
}

func TestBuildSymbolSummary_AllLabels(t *testing.T) {
	moduleQN := "proj.main"
	nodes := []*store.Node{
		{Label: "Module", Name: "main", QualifiedName: moduleQN},
		{Label: "Function", Name: "Foo", QualifiedName: "proj.main.Foo"},
		{Label: "Method", Name: "Bar", QualifiedName: "proj.main.Bar"},
		{Label: "Class", Name: "Baz", QualifiedName: "proj.main.Baz"},
		{Label: "Interface", Name: "Iface", QualifiedName: "proj.main.Iface"},
		{Label: "Type", Name: "MyType", QualifiedName: "proj.main.MyType"},
		{Label: "Enum", Name: "Color", QualifiedName: "proj.main.Color"},
		{Label: "Variable", Name: "count", QualifiedName: "proj.main.count"},
		{Label: "Macro", Name: "MAX", QualifiedName: "proj.main.MAX"},
		{Label: "Field", Name: "name", QualifiedName: "proj.main.name"},
	}
	symbols := buildSymbolSummary(nodes, moduleQN)
	if len(symbols) != 9 {
		t.Errorf("expected 9 symbols (all except Module), got %d: %v", len(symbols), symbols)
	}
}

func TestHasContention_CSWFloor(t *testing.T) {
	p := newAdaptivePool(4)
	ms := &monitorState{
		tier:       tierCSW,
		numCPU:     4.0,
		emaNivcsw:  500.0,
		baseNivcsw: 10.0,
	}
	got := p.hasContention(ms)
	threshold := ms.baseNivcsw * 2
	floor := ms.numCPU * 100
	if threshold < floor {
		threshold = floor
	}
	wantContention := ms.emaNivcsw > threshold
	if got != wantContention {
		t.Errorf("hasContention = %v, want %v (ema=%f, threshold=%f)", got, wantContention, ms.emaNivcsw, threshold)
	}
}

func TestLoadImportMapFromDB(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "proj"
	if err := s.UpsertProject(project, t.TempDir()); err != nil {
		t.Fatal(err)
	}

	moduleQN := fqn.ModuleQN(project, "app/main.go")
	moduleID, err := s.UpsertNode(&store.Node{
		Project:       project,
		Label:         "Module",
		Name:          "main.go",
		QualifiedName: moduleQN,
		FilePath:      "app/main.go",
	})
	if err != nil {
		t.Fatal(err)
	}

	targetWithAliasQN := fqn.ModuleQN(project, "pkg/service.go")
	targetWithAliasID, err := s.UpsertNode(&store.Node{
		Project:       project,
		Label:         "Module",
		Name:          "service.go",
		QualifiedName: targetWithAliasQN,
		FilePath:      "pkg/service.go",
	})
	if err != nil {
		t.Fatal(err)
	}

	targetWithoutAliasID, err := s.UpsertNode(&store.Node{
		Project:       project,
		Label:         "Module",
		Name:          "other.go",
		QualifiedName: fqn.ModuleQN(project, "pkg/other.go"),
		FilePath:      "pkg/other.go",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.InsertEdge(&store.Edge{
		Project:  project,
		SourceID: moduleID,
		TargetID: targetWithAliasID,
		Type:     "IMPORTS",
		Properties: map[string]any{
			"alias": "service",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertEdge(&store.Edge{
		Project:  project,
		SourceID: moduleID,
		TargetID: targetWithoutAliasID,
		Type:     "IMPORTS",
	}); err != nil {
		t.Fatal(err)
	}

	p := &Pipeline{ctx: context.Background(), Store: s, ProjectName: project}
	importMap := p.loadImportMapFromDB(moduleQN)
	if len(importMap) != 1 {
		t.Fatalf("expected 1 aliased import, got %d: %+v", len(importMap), importMap)
	}
	if importMap["service"] != targetWithAliasQN {
		t.Fatalf("expected service alias to resolve to %q, got %+v", targetWithAliasQN, importMap)
	}
	if got := p.loadImportMapFromDB("proj.missing.module"); got != nil {
		t.Fatalf("expected nil import map for missing module, got %+v", got)
	}
}

func TestFindDependentFiles(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	repo := t.TempDir()
	project := "proj"
	if err := s.UpsertProject(project, repo); err != nil {
		t.Fatal(err)
	}

	changed := []discover.FileInfo{{Path: filepath.Join(repo, "pkg", "service.go"), RelPath: "pkg/service.go"}}
	unchanged := []discover.FileInfo{
		{Path: filepath.Join(repo, "consumer.go"), RelPath: "consumer.go"},
		{Path: filepath.Join(repo, "db_consumer.go"), RelPath: "db_consumer.go"},
		{Path: filepath.Join(repo, "ignored.go"), RelPath: "ignored.go"},
	}

	folderQN := fqn.FolderQN(project, "pkg")
	dbConsumerQN := fqn.ModuleQN(project, "db_consumer.go")
	dbConsumerID, err := s.UpsertNode(&store.Node{
		Project:       project,
		Label:         "Module",
		Name:          "db_consumer.go",
		QualifiedName: dbConsumerQN,
		FilePath:      "db_consumer.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	folderID, err := s.UpsertNode(&store.Node{
		Project:       project,
		Label:         "Folder",
		Name:          "pkg",
		QualifiedName: folderQN,
		FilePath:      "pkg",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertEdge(&store.Edge{
		Project:  project,
		SourceID: dbConsumerID,
		TargetID: folderID,
		Type:     "IMPORTS",
		Properties: map[string]any{
			"alias": "pkg",
		},
	}); err != nil {
		t.Fatal(err)
	}

	p := &Pipeline{
		ctx:         context.Background(),
		Store:       s,
		ProjectName: project,
		importMaps: map[string]map[string]string{
			fqn.ModuleQN(project, "consumer.go"): {
				"service": fqn.ModuleQN(project, "pkg/service.go"),
			},
			fqn.ModuleQN(project, "ignored.go"): {
				"other": fqn.ModuleQN(project, "pkg/other.go"),
			},
		},
	}

	dependents := p.findDependentFiles(changed, unchanged)
	if len(dependents) != 2 {
		t.Fatalf("expected 2 dependents, got %d: %+v", len(dependents), dependents)
	}

	got := map[string]bool{}
	for _, f := range dependents {
		got[f.RelPath] = true
	}
	if !got["consumer.go"] || !got["db_consumer.go"] {
		t.Fatalf("expected consumer.go and db_consumer.go, got %+v", got)
	}
	if got["ignored.go"] {
		t.Fatalf("did not expect ignored.go in dependent set: %+v", got)
	}
}

func TestProcessJSONFile(t *testing.T) {
	t.Run("stores_capped_constants", func(t *testing.T) {
		s, err := store.OpenMemory()
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()

		repo := t.TempDir()
		project := "proj"
		if err := s.UpsertProject(project, repo); err != nil {
			t.Fatal(err)
		}

		payload := map[string]string{}
		for i := 0; i < 25; i++ {
			payload["service_url_"+string(rune('a'+i))] = "https://api.example.com/v1/resource"
		}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}

		relPath := "config/settings.json"
		absPath := filepath.Join(repo, relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absPath, data, 0o600); err != nil {
			t.Fatal(err)
		}

		p := &Pipeline{ctx: context.Background(), Store: s, ProjectName: project}
		if err := p.processJSONFile(discover.FileInfo{Path: absPath, RelPath: relPath}); err != nil {
			t.Fatal(err)
		}

		node, err := s.FindNodeByQN(project, fqn.ModuleQN(project, relPath))
		if err != nil {
			t.Fatal(err)
		}
		if node == nil {
			t.Fatal("expected JSON module node to be created")
		}
		constants, ok := node.Properties["constants"].([]any)
		if !ok {
			t.Fatalf("expected constants slice, got %#v", node.Properties["constants"])
		}
		if len(constants) != 20 {
			t.Fatalf("expected constants to be capped at 20, got %d", len(constants))
		}
	})

	t.Run("skips_files_without_url_constants", func(t *testing.T) {
		s, err := store.OpenMemory()
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()

		repo := t.TempDir()
		project := "proj"
		if err := s.UpsertProject(project, repo); err != nil {
			t.Fatal(err)
		}

		relPath := "config/plain.json"
		absPath := filepath.Join(repo, relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absPath, []byte(`{"name":"svc","enabled":true}`), 0o600); err != nil {
			t.Fatal(err)
		}

		p := &Pipeline{ctx: context.Background(), Store: s, ProjectName: project}
		if err := p.processJSONFile(discover.FileInfo{Path: absPath, RelPath: relPath}); err != nil {
			t.Fatal(err)
		}

		node, err := s.FindNodeByQN(project, fqn.ModuleQN(project, relPath))
		if err != nil {
			t.Fatal(err)
		}
		if node != nil {
			t.Fatalf("expected no module node for JSON without URLs, got %+v", node)
		}
	})

	t.Run("returns_parse_error", func(t *testing.T) {
		s, err := store.OpenMemory()
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()

		repo := t.TempDir()
		project := "proj"
		if err := s.UpsertProject(project, repo); err != nil {
			t.Fatal(err)
		}

		relPath := "config/bad.json"
		absPath := filepath.Join(repo, relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absPath, []byte(`{"api_url":`), 0o600); err != nil {
			t.Fatal(err)
		}

		p := &Pipeline{ctx: context.Background(), Store: s, ProjectName: project}
		if err := p.processJSONFile(discover.FileInfo{Path: absPath, RelPath: relPath}); err == nil {
			t.Fatal("expected JSON parse error")
		}
	})
}
