package pipeline

import (
	"testing"

	"github.com/mightycogs/codebook/internal/discover"
	"github.com/mightycogs/codebook/internal/store"
)

func TestContentionTierString(t *testing.T) {
	tests := []struct {
		tier contentionTier
		want string
	}{
		{tierCSW, "1-nivcsw"},
		{tierCPU, "2-cpu"},
		{tierNone, "3-none"},
		{contentionTier(99), "3-none"},
	}
	for _, tt := range tests {
		got := tt.tier.String()
		if got != tt.want {
			t.Errorf("contentionTier(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func TestSetLimit_Clamping(t *testing.T) {
	p := newAdaptivePool(4)

	p.setLimit(0)
	if got := p.currentLimit(); got != p.minLimit {
		t.Errorf("setLimit(0) = %d, want min %d", got, p.minLimit)
	}

	p.setLimit(9999)
	if got := p.currentLimit(); got != p.maxLimit {
		t.Errorf("setLimit(9999) = %d, want max %d", got, p.maxLimit)
	}

	p.setLimit(8)
	if got := p.currentLimit(); got != 8 {
		t.Errorf("setLimit(8) = %d, want 8", got)
	}
}

func TestSetLimit_BroadcastOnGrow(t *testing.T) {
	p := newAdaptivePool(4)
	p.setLimit(8)
	if got := p.currentLimit(); got != 8 {
		t.Errorf("after grow setLimit(8) = %d, want 8", got)
	}

	p.setLimit(4)
	if got := p.currentLimit(); got != 4 {
		t.Errorf("after shrink setLimit(4) = %d, want 4", got)
	}
}

func TestUpdateEMA_FirstTick(t *testing.T) {
	p := newAdaptivePool(4)
	ms := &monitorState{tick: 1, numCPU: 4.0}
	p.updateEMA(ms, 1000.0, 50.0, 0.8)
	if ms.emaBPS != 1000.0 {
		t.Errorf("emaBPS = %f, want 1000.0", ms.emaBPS)
	}
	if ms.emaNivcsw != 50.0 {
		t.Errorf("emaNivcsw = %f, want 50.0", ms.emaNivcsw)
	}
	if ms.emaCPUUtil != 0.8 {
		t.Errorf("emaCPUUtil = %f, want 0.8", ms.emaCPUUtil)
	}
}

func TestUpdateEMA_SubsequentTick(t *testing.T) {
	p := newAdaptivePool(4)
	ms := &monitorState{tick: 2, numCPU: 4.0, emaBPS: 1000.0, emaNivcsw: 50.0, emaCPUUtil: 0.8}
	p.updateEMA(ms, 2000.0, 100.0, 0.9)
	want := 0.3*2000.0 + 0.7*1000.0
	if ms.emaBPS != want {
		t.Errorf("emaBPS = %f, want %f", ms.emaBPS, want)
	}
}

func TestHasContention_TierCSW(t *testing.T) {
	p := newAdaptivePool(4)
	ms := &monitorState{
		tier:       tierCSW,
		numCPU:     4.0,
		emaNivcsw:  2000.0,
		baseNivcsw: 100.0,
	}
	if !p.hasContention(ms) {
		t.Error("expected contention with high emaNivcsw")
	}

	ms.emaNivcsw = 10.0
	if p.hasContention(ms) {
		t.Error("expected no contention with low emaNivcsw")
	}
}

func TestHasContention_TierCPU(t *testing.T) {
	p := newAdaptivePool(4)
	ms := &monitorState{
		tier:       tierCPU,
		numCPU:     4.0,
		emaCPUUtil: 0.95,
	}
	if !p.hasContention(ms) {
		t.Error("expected contention with high CPU util")
	}

	ms.emaCPUUtil = 0.5
	if p.hasContention(ms) {
		t.Error("expected no contention with low CPU util")
	}
}

func TestHasContention_TierNone(t *testing.T) {
	p := newAdaptivePool(4)
	ms := &monitorState{
		tier:       tierNone,
		numCPU:     4.0,
		emaNivcsw:  9999.0,
		emaCPUUtil: 0.99,
	}
	if p.hasContention(ms) {
		t.Error("expected no contention for tierNone (grow-only)")
	}
}

func TestHandleWarmupTick_TierDetection(t *testing.T) {
	p := newAdaptivePool(4)
	ms := &monitorState{tick: 1, numCPU: 4.0, tier: tierNone}

	p.handleWarmupTick(ms, contentionSignal{Nivcsw: 100, CPUTimeSec: 5.0}, p.currentLimit())
	if ms.tier != tierCSW {
		t.Errorf("expected tierCSW, got %v", ms.tier)
	}

	ms2 := &monitorState{tick: 1, numCPU: 4.0, tier: tierNone}
	p.handleWarmupTick(ms2, contentionSignal{Nivcsw: 0, CPUTimeSec: 5.0}, p.currentLimit())
	if ms2.tier != tierCPU {
		t.Errorf("expected tierCPU, got %v", ms2.tier)
	}

	ms3 := &monitorState{tick: 1, numCPU: 4.0, tier: tierNone}
	p.handleWarmupTick(ms3, contentionSignal{Nivcsw: 0, CPUTimeSec: 0}, p.currentLimit())
	if ms3.tier != tierNone {
		t.Errorf("expected tierNone, got %v", ms3.tier)
	}
}

func TestHandleWarmupTick_GrowsOnTick2(t *testing.T) {
	p := newAdaptivePool(4)
	ms := &monitorState{tick: 2, numCPU: 4.0, emaBPS: 2000.0, baseBPS: 1000.0}
	initial := p.currentLimit()
	p.handleWarmupTick(ms, contentionSignal{}, initial)
	if p.currentLimit() <= initial {
		t.Errorf("expected growth on tick 2 with improving BPS, got %d", p.currentLimit())
	}
}

func TestExtractClassFromMethodQN(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"project.path.ClassName.methodName", "project.path.ClassName"},
		{"single", ""},
		{"a.b", "a"},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractClassFromMethodQN(tt.input)
		if got != tt.want {
			t.Errorf("extractClassFromMethodQN(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsCheckedException(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"IOException", true},
		{"SQLException", true},
		{"CustomException", true},
		{"RuntimeException", false},
		{"Error", false},
		{"NullPointerError", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isCheckedException(tt.name)
		if got != tt.want {
			t.Errorf("isCheckedException(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestTestFileToProductionFile(t *testing.T) {
	tests := []struct {
		testPath string
		want     string
	}{
		{"foo_test.go", "foo.go"},
		{"internal/store/nodes_test.go", "internal/store/nodes.go"},
		{"test_handler.py", "handler.py"},
		{"app.test.ts", "app.ts"},
		{"app.spec.ts", "app.ts"},
		{"component.test.tsx", "component.tsx"},
		{"component.spec.tsx", "component.tsx"},
		{"handler.test.js", "handler.js"},
		{"main.go", ""},
		{"handler.py", ""},
		{"utils.ts", ""},
	}
	for _, tt := range tests {
		got := testFileToProductionFile(tt.testPath)
		if got != tt.want {
			t.Errorf("testFileToProductionFile(%q) = %q, want %q", tt.testPath, got, tt.want)
		}
	}
}

func TestImportAdjustedConfidence(t *testing.T) {
	imports := map[string]string{"handler": "proj.handler"}

	t.Run("reachable", func(t *testing.T) {
		got := importAdjustedConfidence(0.55, "proj.handler.Process", imports)
		if got != 0.55 {
			t.Errorf("expected 0.55 for reachable, got %f", got)
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		got := importAdjustedConfidence(0.55, "proj.billing.Process", imports)
		if got != 0.275 {
			t.Errorf("expected 0.275 for unreachable, got %f", got)
		}
	})

	t.Run("nil_importmap", func(t *testing.T) {
		got := importAdjustedConfidence(0.55, "proj.billing.Process", nil)
		if got != 0.55 {
			t.Errorf("expected 0.55 with nil importMap, got %f", got)
		}
	})
}

func TestLabelOf(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Foo", "proj.pkg.Foo", "Function")
	reg.Register("Bar", "proj.pkg.Bar", "Method")

	if got := reg.LabelOf("proj.pkg.Foo"); got != "Function" {
		t.Errorf("LabelOf(proj.pkg.Foo) = %q, want Function", got)
	}
	if got := reg.LabelOf("proj.pkg.Bar"); got != "Method" {
		t.Errorf("LabelOf(proj.pkg.Bar) = %q, want Method", got)
	}
	if got := reg.LabelOf("proj.pkg.Missing"); got != "" {
		t.Errorf("LabelOf(missing) = %q, want empty", got)
	}
}

func TestLabelToSymbolPrefix(t *testing.T) {
	tests := []struct {
		label string
		want  string
	}{
		{"Function", "func"},
		{"Method", "method"},
		{"Class", "class"},
		{"Interface", "interface"},
		{"Type", "type"},
		{"Enum", "enum"},
		{"Variable", "var"},
		{"Macro", "macro"},
		{"Field", "field"},
		{"Unknown", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := labelToSymbolPrefix(tt.label)
		if got != tt.want {
			t.Errorf("labelToSymbolPrefix(%q) = %q, want %q", tt.label, got, tt.want)
		}
	}
}

func TestBuildSymbolSummary(t *testing.T) {
	moduleQN := "proj.main"
	nodes := []*store.Node{
		{Label: "Module", Name: "main", QualifiedName: "proj.main"},
		{Label: "Function", Name: "Foo", QualifiedName: "proj.main.Foo"},
		{Label: "Method", Name: "Bar", QualifiedName: "proj.main.Bar"},
		{Label: "Class", Name: "Baz", QualifiedName: "proj.main.Baz"},
		{Label: "Community", Name: "cluster", QualifiedName: "proj.cluster"},
	}
	symbols := buildSymbolSummary(nodes, moduleQN)
	if len(symbols) != 3 {
		t.Fatalf("expected 3 symbols, got %d: %v", len(symbols), symbols)
	}
	expected := map[string]bool{
		"func:Foo":   true,
		"method:Bar": true,
		"class:Baz":  true,
	}
	for _, s := range symbols {
		if !expected[s] {
			t.Errorf("unexpected symbol %q", s)
		}
	}
}

func TestLabelPriority(t *testing.T) {
	tests := []struct {
		label string
		want  int
	}{
		{"Class", 0},
		{"Interface", 1},
		{"Type", 2},
		{"Function", 3},
		{"Method", 4},
		{"Variable", 5},
		{"Unknown", 5},
	}
	for _, tt := range tests {
		got := labelPriority(tt.label)
		if got != tt.want {
			t.Errorf("labelPriority(%q) = %d, want %d", tt.label, got, tt.want)
		}
	}
}

func TestCommunityCohesion(t *testing.T) {
	t.Run("single_member", func(t *testing.T) {
		got := communityCohesion([]int64{1}, map[int64]*store.Node{1: {ID: 1}})
		if got != 1.0 {
			t.Errorf("expected 1.0, got %f", got)
		}
	})

	t.Run("all_known", func(t *testing.T) {
		nodeMap := map[int64]*store.Node{
			1: {ID: 1},
			2: {ID: 2},
			3: {ID: 3},
		}
		got := communityCohesion([]int64{1, 2, 3}, nodeMap)
		if got != 1.0 {
			t.Errorf("expected 1.0, got %f", got)
		}
	})

	t.Run("some_missing", func(t *testing.T) {
		nodeMap := map[int64]*store.Node{
			1: {ID: 1},
			2: {ID: 2},
		}
		got := communityCohesion([]int64{1, 2, 3, 4}, nodeMap)
		if got != 0.5 {
			t.Errorf("expected 0.5, got %f", got)
		}
	})
}

func TestTopMemberNames(t *testing.T) {
	nodeMap := map[int64]*store.Node{
		1: {ID: 1, Name: "FuncB", Label: "Function"},
		2: {ID: 2, Name: "ClassA", Label: "Class"},
		3: {ID: 3, Name: "FuncA", Label: "Function"},
		4: {ID: 4, Name: "IfaceZ", Label: "Interface"},
	}
	names := topMemberNames([]int64{1, 2, 3, 4}, nodeMap, 3)
	if len(names) != 3 {
		t.Fatalf("expected 3, got %d: %v", len(names), names)
	}
	if names[0] != "ClassA" {
		t.Errorf("expected ClassA first (Class priority), got %q", names[0])
	}
	if names[1] != "IfaceZ" {
		t.Errorf("expected IfaceZ second (Interface priority), got %q", names[1])
	}
}

func TestTopMemberNames_Nil(t *testing.T) {
	nodeMap := map[int64]*store.Node{}
	names := topMemberNames([]int64{1, 2}, nodeMap, 5)
	if len(names) != 0 {
		t.Errorf("expected 0 names from empty node map, got %d", len(names))
	}
}

func TestExtractModuleConstants(t *testing.T) {
	modules := []*store.Node{
		{
			QualifiedName: "proj.config",
			Properties: map[string]any{
				"constants": []any{"DATABASE_URL = postgres://localhost/db", "API_KEY = abc123"},
			},
		},
		{
			QualifiedName: "proj.main",
			Properties:    map[string]any{},
		},
		{
			QualifiedName: "proj.settings",
			Properties: map[string]any{
				"constants": []any{"lowercase_key = value"},
			},
		},
	}
	index := make(map[string]string)
	extractModuleConstants(modules, index)
	if index["DATABASE_URL"] != "proj.config" {
		t.Errorf("expected DATABASE_URL -> proj.config, got %q", index["DATABASE_URL"])
	}
	if index["API_KEY"] != "proj.config" {
		t.Errorf("expected API_KEY -> proj.config, got %q", index["API_KEY"])
	}
	if _, exists := index["lowercase_key"]; exists {
		t.Error("expected lowercase_key to be skipped (not env var name)")
	}
}

func TestExtractModuleConstants_NilConstants(t *testing.T) {
	modules := []*store.Node{
		{QualifiedName: "proj.a"},
		{QualifiedName: "proj.b", Properties: map[string]any{"constants": "not_a_list"}},
	}
	index := make(map[string]string)
	extractModuleConstants(modules, index)
	if len(index) != 0 {
		t.Errorf("expected empty index for nil/invalid constants, got %d", len(index))
	}
}

func TestExtractReceiverType(t *testing.T) {
	tests := []struct {
		recv string
		want string
	}{
		{"(h *Handlers)", "Handlers"},
		{"(s Store)", "Store"},
		{"(*Pipeline)", "Pipeline"},
		{"", ""},
		{"  ", ""},
	}
	for _, tt := range tests {
		got := extractReceiverType(tt.recv)
		if got != tt.want {
			t.Errorf("extractReceiverType(%q) = %q, want %q", tt.recv, got, tt.want)
		}
	}
}

func TestSatisfies(t *testing.T) {
	methods := []ifaceMethodInfo{
		{name: "Read"},
		{name: "Close"},
	}

	t.Run("full_match", func(t *testing.T) {
		set := map[string]bool{"Read": true, "Close": true, "Extra": true}
		if !satisfies(methods, set) {
			t.Error("expected satisfies = true")
		}
	})

	t.Run("partial_match", func(t *testing.T) {
		set := map[string]bool{"Read": true}
		if satisfies(methods, set) {
			t.Error("expected satisfies = false for partial match")
		}
	})

	t.Run("empty_interface", func(t *testing.T) {
		if !satisfies(nil, map[string]bool{"Anything": true}) {
			t.Error("empty interface should be satisfied by any struct")
		}
	})
}

func TestDecoratorFunctionName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"@app.route('/api')", "app.route"},
		{"@pytest.fixture", "pytest.fixture"},
		{"@Override", "Override"},
		{"@Deprecated", "Deprecated"},
		{"@", ""},
	}
	for _, tt := range tests {
		got := decoratorFunctionName(tt.input)
		if got != tt.want {
			t.Errorf("decoratorFunctionName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestQualifiedNamePrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"project.path.module.ClassName", "project.path.module"},
		{"a.b", "a"},
		{"single", "single"},
		{"", ""},
	}
	for _, tt := range tests {
		got := qualifiedNamePrefix(tt.input)
		if got != tt.want {
			t.Errorf("qualifiedNamePrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractJSONURLValues(t *testing.T) {
	t.Run("url_key", func(t *testing.T) {
		data := map[string]any{
			"api_url": "https://api.example.com",
			"name":    "test",
		}
		var out []string
		extractJSONURLValues(data, "", &out, 0)
		if len(out) != 1 {
			t.Fatalf("expected 1 URL value, got %d: %v", len(out), out)
		}
		if out[0] != "api_url = https://api.example.com" {
			t.Errorf("unexpected output: %q", out[0])
		}
	})

	t.Run("url_value", func(t *testing.T) {
		data := map[string]any{
			"server": "https://backend.example.com/api/v1",
		}
		var out []string
		extractJSONURLValues(data, "", &out, 0)
		if len(out) < 1 {
			t.Fatalf("expected at least 1 URL value, got %d", len(out))
		}
	})

	t.Run("nested_objects", func(t *testing.T) {
		data := map[string]any{
			"services": map[string]any{
				"endpoint": "https://svc.example.com",
			},
		}
		var out []string
		extractJSONURLValues(data, "", &out, 0)
		if len(out) != 1 {
			t.Fatalf("expected 1, got %d: %v", len(out), out)
		}
	})

	t.Run("array", func(t *testing.T) {
		data := []any{
			map[string]any{"url": "https://a.com"},
			map[string]any{"url": "https://b.com"},
		}
		var out []string
		extractJSONURLValues(data, "", &out, 0)
		if len(out) != 2 {
			t.Fatalf("expected 2, got %d: %v", len(out), out)
		}
	})

	t.Run("max_depth", func(t *testing.T) {
		data := map[string]any{
			"url": "https://deep.example.com",
		}
		var out []string
		extractJSONURLValues(data, "", &out, 21)
		if len(out) != 0 {
			t.Errorf("expected 0 at max depth, got %d", len(out))
		}
	})

	t.Run("empty_key_or_value", func(t *testing.T) {
		var out []string
		extractJSONURLValues("some_value", "", &out, 0)
		if len(out) != 0 {
			t.Errorf("expected 0 for empty key, got %d", len(out))
		}
		extractJSONURLValues("", "key", &out, 0)
		if len(out) != 0 {
			t.Errorf("expected 0 for empty value, got %d", len(out))
		}
	})
}

func TestLooksLikeURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https://example.com", true},
		{"http://localhost:8080", true},
		{"/api/v1/users", true},
		{"/api/health", true},
		{"/v2", false},
		{"/en", false},
		{"just-a-string", false},
		{"", false},
	}
	for _, tt := range tests {
		got := looksLikeURL(tt.input)
		if got != tt.want {
			t.Errorf("looksLikeURL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestBuildConstantsList(t *testing.T) {
	bindings := []EnvBinding{
		{Key: "DB_URL", Value: "postgres://localhost/db"},
		{Key: "API_KEY", Value: "secret123"},
	}
	constants := buildConstantsList(bindings)
	if len(constants) != 2 {
		t.Fatalf("expected 2, got %d", len(constants))
	}
	if constants[0] != "DB_URL = postgres://localhost/db" {
		t.Errorf("unexpected constant: %q", constants[0])
	}
}

func TestBuildConstantsList_Cap50(t *testing.T) {
	bindings := make([]EnvBinding, 60)
	for i := range bindings {
		bindings[i] = EnvBinding{Key: "K", Value: "V"}
	}
	constants := buildConstantsList(bindings)
	if len(constants) != 50 {
		t.Errorf("expected cap at 50, got %d", len(constants))
	}
}

func TestMergeFiles(t *testing.T) {
	a := []discover.FileInfo{
		{RelPath: "a.go"},
		{RelPath: "b.go"},
	}
	b := []discover.FileInfo{
		{RelPath: "b.go"},
		{RelPath: "c.go"},
	}
	result := mergeFiles(a, b)
	if len(result) != 3 {
		t.Errorf("expected 3 unique files, got %d", len(result))
	}
}

func TestMergeFiles_Empty(t *testing.T) {
	result := mergeFiles(nil, nil)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestHasFrameworkDecorator(t *testing.T) {
	if !hasFrameworkDecorator([]string{"@app.route('/api')"}) {
		t.Error("expected true for @app.route")
	}
	if !hasFrameworkDecorator([]string{"@router.get(\"/users\")"}) {
		t.Error("expected true for @router.get")
	}
	if hasFrameworkDecorator([]string{"@Override"}) {
		t.Error("expected false for @Override")
	}
	if hasFrameworkDecorator(nil) {
		t.Error("expected false for nil")
	}
}

func TestNewAdaptivePool(t *testing.T) {
	t.Run("min_cpu", func(t *testing.T) {
		p := newAdaptivePool(0)
		if p.numCPU != 1 {
			t.Errorf("numCPU = %d, want 1 for input 0", p.numCPU)
		}
		if p.growStep != 2 {
			t.Errorf("growStep = %d, want 2", p.growStep)
		}
	})

	t.Run("normal_cpu", func(t *testing.T) {
		p := newAdaptivePool(8)
		if p.minLimit != 8 {
			t.Errorf("minLimit = %d, want 8", p.minLimit)
		}
		if p.maxLimit != 64 {
			t.Errorf("maxLimit = %d, want 64", p.maxLimit)
		}
		if p.growStep != 4 {
			t.Errorf("growStep = %d, want 4", p.growStep)
		}
	})
}

func TestGroupAndFilter(t *testing.T) {
	nodeCommunity := map[int64]int{
		1: 0, 2: 0,
		3: 1,
		4: 2, 5: 2, 6: 2,
	}
	result := groupAndFilter(nodeCommunity)
	if len(result) != 2 {
		t.Errorf("expected 2 communities (singletons filtered), got %d", len(result))
	}
}

func TestExtractDecoratorWords(t *testing.T) {
	t.Run("with_decorators", func(t *testing.T) {
		n := &store.Node{
			Properties: map[string]any{
				"decorators": []any{"@login_required", "@cache"},
			},
		}
		words := extractDecoratorWords(n)
		if len(words) == 0 {
			t.Fatal("expected words from decorators")
		}
	})

	t.Run("no_decorators", func(t *testing.T) {
		n := &store.Node{Properties: map[string]any{}}
		words := extractDecoratorWords(n)
		if len(words) != 0 {
			t.Errorf("expected empty, got %v", words)
		}
	})

	t.Run("invalid_type", func(t *testing.T) {
		n := &store.Node{
			Properties: map[string]any{
				"decorators": "not_a_list",
			},
		}
		words := extractDecoratorWords(n)
		if len(words) != 0 {
			t.Errorf("expected empty for invalid type, got %v", words)
		}
	})
}

func TestRegistryFindByName(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Foo", "proj.a.Foo", "Function")
	reg.Register("Foo", "proj.b.Foo", "Function")
	reg.Register("Bar", "proj.c.Bar", "Method")

	foos := reg.FindByName("Foo")
	if len(foos) != 2 {
		t.Errorf("expected 2 Foos, got %d", len(foos))
	}
	bars := reg.FindByName("Bar")
	if len(bars) != 1 {
		t.Errorf("expected 1 Bar, got %d", len(bars))
	}
	missing := reg.FindByName("Missing")
	if len(missing) != 0 {
		t.Errorf("expected 0 for missing, got %d", len(missing))
	}
}

func TestRegistryFindEndingWith(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Process", "proj.a.Process", "Function")
	reg.Register("Process", "proj.b.Process", "Function")
	reg.Register("Handle", "proj.c.Handle", "Function")

	result := reg.FindEndingWith("Process")
	if len(result) != 2 {
		t.Errorf("expected 2, got %d", len(result))
	}
}

func TestRegistrySize(t *testing.T) {
	reg := NewFunctionRegistry()
	if reg.Size() != 0 {
		t.Errorf("expected 0, got %d", reg.Size())
	}
	reg.Register("A", "proj.A", "Function")
	reg.Register("B", "proj.B", "Function")
	if reg.Size() != 2 {
		t.Errorf("expected 2, got %d", reg.Size())
	}
}

func TestRegistryDuplicateRegistration(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("Foo", "proj.Foo", "Function")
	reg.Register("Foo", "proj.Foo", "Function")
	if reg.Size() != 1 {
		t.Errorf("expected 1 after duplicate, got %d", reg.Size())
	}
	foos := reg.FindByName("Foo")
	if len(foos) != 1 {
		t.Errorf("expected 1 in byName after duplicate, got %d", len(foos))
	}
}

func TestCommonPrefixLen(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"proj.svc.handler", "proj.svc.caller", 2},
		{"proj.a.b", "proj.a.b", 3},
		{"a.b.c", "x.y.z", 0},
		{"", "", 1},
	}
	for _, tt := range tests {
		got := commonPrefixLen(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("commonPrefixLen(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestFilterImportReachable(t *testing.T) {
	imports := map[string]string{"handler": "proj.handler"}
	candidates := []string{"proj.handler.Process", "proj.billing.Process"}
	filtered := filterImportReachable(candidates, imports)
	if len(filtered) != 1 {
		t.Errorf("expected 1, got %d", len(filtered))
	}
	if filtered[0] != "proj.handler.Process" {
		t.Errorf("expected proj.handler.Process, got %q", filtered[0])
	}
}

func TestFilterImportReachable_NilMap(t *testing.T) {
	candidates := []string{"a", "b"}
	filtered := filterImportReachable(candidates, nil)
	if len(filtered) != 2 {
		t.Errorf("expected passthrough with nil map, got %d", len(filtered))
	}
}

func TestSimpleName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"proj.pkg.Foo", "Foo"},
		{"Foo", "Foo"},
		{"a.b.c.d", "d"},
	}
	for _, tt := range tests {
		got := simpleName(tt.input)
		if got != tt.want {
			t.Errorf("simpleName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestModulePrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"proj.pkg.Foo", "proj.pkg"},
		{"Foo", "Foo"},
		{"a.b", "a"},
	}
	for _, tt := range tests {
		got := modulePrefix(tt.input)
		if got != tt.want {
			t.Errorf("modulePrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBestByImportDistance(t *testing.T) {
	candidates := []string{"proj.svc.handler.Process", "proj.other.Process"}
	best := bestByImportDistance(candidates, "proj.svc.caller")
	if best != "proj.svc.handler.Process" {
		t.Errorf("expected proj.svc.handler.Process, got %q", best)
	}
}

func TestBestByImportDistance_Empty(t *testing.T) {
	best := bestByImportDistance(nil, "proj.caller")
	if best != "" {
		t.Errorf("expected empty for nil candidates, got %q", best)
	}
}
