package pipeline

import (
	"testing"

	"github.com/mightycogs/codebook/internal/store"
)

func TestGraphBuffer_UpsertNode_New(t *testing.T) {
	b := newGraphBuffer("proj")
	id := b.UpsertNode(&store.Node{
		Label: "Function", Name: "Foo", QualifiedName: "proj.Foo",
		FilePath: "main.go", StartLine: 1, EndLine: 10,
	})
	if id != 1 {
		t.Errorf("expected first ID = 1, got %d", id)
	}
	n := b.FindNodeByQN("proj.Foo")
	if n == nil {
		t.Fatal("expected to find node by QN")
	}
	if n.Name != "Foo" {
		t.Errorf("name = %q, want Foo", n.Name)
	}
	if n.Project != "proj" {
		t.Errorf("project = %q, want proj", n.Project)
	}
}

func TestGraphBuffer_UpsertNode_Update(t *testing.T) {
	b := newGraphBuffer("proj")
	id1 := b.UpsertNode(&store.Node{
		Label: "Function", Name: "Foo", QualifiedName: "proj.Foo",
		FilePath: "main.go", StartLine: 1, EndLine: 10,
	})
	id2 := b.UpsertNode(&store.Node{
		Label: "Method", Name: "Bar", QualifiedName: "proj.Foo",
		FilePath: "other.go", StartLine: 5, EndLine: 15,
	})
	if id1 != id2 {
		t.Errorf("expected same ID for upsert, got %d and %d", id1, id2)
	}
	n := b.FindNodeByQN("proj.Foo")
	if n.Label != "Method" {
		t.Errorf("label = %q, want Method after update", n.Label)
	}
	if n.Name != "Bar" {
		t.Errorf("name = %q, want Bar after update", n.Name)
	}
	if n.FilePath != "other.go" {
		t.Errorf("filePath = %q, want other.go after update", n.FilePath)
	}
}

func TestGraphBuffer_RemoveFromLabelIndex(t *testing.T) {
	b := newGraphBuffer("proj")
	b.UpsertNode(&store.Node{
		Label: "Function", Name: "A", QualifiedName: "proj.A",
	})
	b.UpsertNode(&store.Node{
		Label: "Function", Name: "B", QualifiedName: "proj.B",
	})
	b.UpsertNode(&store.Node{
		Label: "Function", Name: "C", QualifiedName: "proj.C",
	})

	funcs := b.FindNodesByLabel("Function")
	if len(funcs) != 3 {
		t.Fatalf("expected 3 functions, got %d", len(funcs))
	}

	b.UpsertNode(&store.Node{
		Label: "Method", Name: "B_updated", QualifiedName: "proj.B",
	})

	funcs = b.FindNodesByLabel("Function")
	if len(funcs) != 2 {
		t.Errorf("expected 2 functions after label change, got %d", len(funcs))
	}
	methods := b.FindNodesByLabel("Method")
	if len(methods) != 1 {
		t.Errorf("expected 1 method after label change, got %d", len(methods))
	}
}

func TestGraphBuffer_RemoveFromLabelIndex_NotFound(t *testing.T) {
	b := newGraphBuffer("proj")
	b.UpsertNode(&store.Node{
		Label: "Function", Name: "A", QualifiedName: "proj.A",
	})
	b.removeFromLabelIndex("Function", 999)
	funcs := b.FindNodesByLabel("Function")
	if len(funcs) != 1 {
		t.Errorf("expected 1 function (no-op removal), got %d", len(funcs))
	}
}

func TestGraphBuffer_RemoveFromLabelIndex_EmptyLabel(t *testing.T) {
	b := newGraphBuffer("proj")
	b.removeFromLabelIndex("NonExistent", 1)
}

func TestGraphBuffer_FindNodeByID(t *testing.T) {
	b := newGraphBuffer("proj")
	id := b.UpsertNode(&store.Node{
		Label: "Function", Name: "Foo", QualifiedName: "proj.Foo",
	})
	n := b.FindNodeByID(id)
	if n == nil {
		t.Fatal("expected to find node by ID")
	}
	if n.Name != "Foo" {
		t.Errorf("name = %q, want Foo", n.Name)
	}
	if b.FindNodeByID(999) != nil {
		t.Error("expected nil for non-existent ID")
	}
}

func TestGraphBuffer_UpsertNodeBatch(t *testing.T) {
	b := newGraphBuffer("proj")
	nodes := []*store.Node{
		{Label: "Function", Name: "A", QualifiedName: "proj.A"},
		{Label: "Function", Name: "B", QualifiedName: "proj.B"},
		{Label: "Class", Name: "C", QualifiedName: "proj.C"},
	}
	result := b.UpsertNodeBatch(nodes)
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	if result["proj.A"] == 0 || result["proj.B"] == 0 || result["proj.C"] == 0 {
		t.Error("expected non-zero IDs for all nodes")
	}
	funcs := b.FindNodesByLabel("Function")
	if len(funcs) != 2 {
		t.Errorf("expected 2 functions, got %d", len(funcs))
	}
}

func TestGraphBuffer_InsertEdge_New(t *testing.T) {
	b := newGraphBuffer("proj")
	idA := b.UpsertNode(&store.Node{Label: "Function", Name: "A", QualifiedName: "proj.A"})
	idB := b.UpsertNode(&store.Node{Label: "Function", Name: "B", QualifiedName: "proj.B"})

	edgeID := b.InsertEdge(&store.Edge{
		SourceID: idA, TargetID: idB, Type: "CALLS",
		Properties: map[string]any{"weight": 1},
	})
	if edgeID == 0 {
		t.Error("expected non-zero edge ID")
	}

	edges := b.FindEdgesBySourceAndType(idA, "CALLS")
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].Type != "CALLS" {
		t.Errorf("type = %q, want CALLS", edges[0].Type)
	}
}

func TestGraphBuffer_InsertEdge_Dedup(t *testing.T) {
	b := newGraphBuffer("proj")
	idA := b.UpsertNode(&store.Node{Label: "Function", Name: "A", QualifiedName: "proj.A"})
	idB := b.UpsertNode(&store.Node{Label: "Function", Name: "B", QualifiedName: "proj.B"})

	id1 := b.InsertEdge(&store.Edge{
		SourceID: idA, TargetID: idB, Type: "CALLS",
		Properties: map[string]any{"count": 1},
	})
	id2 := b.InsertEdge(&store.Edge{
		SourceID: idA, TargetID: idB, Type: "CALLS",
		Properties: map[string]any{"count": 2, "extra": "val"},
	})
	if id1 != id2 {
		t.Errorf("expected same edge ID on dedup, got %d and %d", id1, id2)
	}
	edges := b.FindEdgesBySourceAndType(idA, "CALLS")
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge after dedup, got %d", len(edges))
	}
	if edges[0].Properties["count"] != 2 {
		t.Errorf("expected merged count=2, got %v", edges[0].Properties["count"])
	}
	if edges[0].Properties["extra"] != "val" {
		t.Errorf("expected merged extra=val, got %v", edges[0].Properties["extra"])
	}
}

func TestGraphBuffer_InsertEdgeBatch(t *testing.T) {
	b := newGraphBuffer("proj")
	idA := b.UpsertNode(&store.Node{Label: "Function", Name: "A", QualifiedName: "proj.A"})
	idB := b.UpsertNode(&store.Node{Label: "Function", Name: "B", QualifiedName: "proj.B"})
	idC := b.UpsertNode(&store.Node{Label: "Function", Name: "C", QualifiedName: "proj.C"})

	b.InsertEdgeBatch([]*store.Edge{
		{SourceID: idA, TargetID: idB, Type: "CALLS"},
		{SourceID: idA, TargetID: idC, Type: "CALLS"},
	})
	edges := b.FindEdgesBySourceAndType(idA, "CALLS")
	if len(edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(edges))
	}
}

func TestGraphBuffer_FindEdgesBySourceAndType_NoMatch(t *testing.T) {
	b := newGraphBuffer("proj")
	edges := b.FindEdgesBySourceAndType(999, "CALLS")
	if edges != nil {
		t.Errorf("expected nil for non-existent source, got %v", edges)
	}
}

func TestGraphBuffer_FindNodesByLabel_Empty(t *testing.T) {
	b := newGraphBuffer("proj")
	nodes := b.FindNodesByLabel("Function")
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes for empty buffer, got %d", len(nodes))
	}
}

func TestGraphBuffer_FindNodeIDsByQNs(t *testing.T) {
	b := newGraphBuffer("proj")
	b.UpsertNode(&store.Node{Label: "Function", Name: "A", QualifiedName: "proj.A"})
	b.UpsertNode(&store.Node{Label: "Function", Name: "B", QualifiedName: "proj.B"})

	result := b.FindNodeIDsByQNs([]string{"proj.A", "proj.B", "proj.C"})
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
	if result["proj.A"] == 0 || result["proj.B"] == 0 {
		t.Error("expected non-zero IDs for existing nodes")
	}
	if _, ok := result["proj.C"]; ok {
		t.Error("expected proj.C to be absent")
	}
}

func TestGraphBuffer_AllNodes(t *testing.T) {
	b := newGraphBuffer("proj")
	b.UpsertNode(&store.Node{Label: "Function", Name: "A", QualifiedName: "proj.A"})
	b.UpsertNode(&store.Node{Label: "Class", Name: "B", QualifiedName: "proj.B"})
	b.UpsertNode(&store.Node{Label: "Module", Name: "C", QualifiedName: "proj.C"})

	all := b.allNodes()
	if len(all) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(all))
	}
}

func TestGraphBuffer_RoundTripProps(t *testing.T) {
	input := map[string]any{
		"tags":  []string{"a", "b"},
		"count": 42,
	}
	result := roundTripProps(input)
	tags, ok := result["tags"].([]any)
	if !ok {
		t.Fatalf("expected []any for tags after round-trip, got %T", result["tags"])
	}
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
}

func TestGraphBuffer_RoundTripProps_Nil(t *testing.T) {
	result := roundTripProps(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestGraphBuffer_RoundTripProps_Empty(t *testing.T) {
	result := roundTripProps(map[string]any{})
	if len(result) != 0 {
		t.Errorf("expected empty map for empty input, got %v", result)
	}
}

func TestGraphBuffer_InsertEdge_NilProperties(t *testing.T) {
	b := newGraphBuffer("proj")
	idA := b.UpsertNode(&store.Node{Label: "Function", Name: "A", QualifiedName: "proj.A"})
	idB := b.UpsertNode(&store.Node{Label: "Function", Name: "B", QualifiedName: "proj.B"})

	b.InsertEdge(&store.Edge{
		SourceID: idA, TargetID: idB, Type: "CALLS",
		Properties: map[string]any{"k": "v"},
	})
	b.InsertEdge(&store.Edge{
		SourceID: idA, TargetID: idB, Type: "CALLS",
	})
	edges := b.FindEdgesBySourceAndType(idA, "CALLS")
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].Properties["k"] != "v" {
		t.Errorf("expected original properties preserved, got %v", edges[0].Properties)
	}
}

func TestGraphBuffer_UpsertNode_PreservesPropertiesOnNilUpdate(t *testing.T) {
	b := newGraphBuffer("proj")
	b.UpsertNode(&store.Node{
		Label: "Function", Name: "Foo", QualifiedName: "proj.Foo",
		Properties: map[string]any{"sig": "func()"},
	})
	b.UpsertNode(&store.Node{
		Label: "Function", Name: "Foo", QualifiedName: "proj.Foo",
	})
	n := b.FindNodeByQN("proj.Foo")
	if n.Properties["sig"] != "func()" {
		t.Errorf("expected properties preserved on nil update, got %v", n.Properties)
	}
}

func TestGraphBuffer_DifferentEdgeTypes(t *testing.T) {
	b := newGraphBuffer("proj")
	idA := b.UpsertNode(&store.Node{Label: "Function", Name: "A", QualifiedName: "proj.A"})
	idB := b.UpsertNode(&store.Node{Label: "Function", Name: "B", QualifiedName: "proj.B"})

	b.InsertEdge(&store.Edge{SourceID: idA, TargetID: idB, Type: "CALLS"})
	b.InsertEdge(&store.Edge{SourceID: idA, TargetID: idB, Type: "IMPORTS"})

	calls := b.FindEdgesBySourceAndType(idA, "CALLS")
	imports := b.FindEdgesBySourceAndType(idA, "IMPORTS")
	if len(calls) != 1 {
		t.Errorf("expected 1 CALLS edge, got %d", len(calls))
	}
	if len(imports) != 1 {
		t.Errorf("expected 1 IMPORTS edge, got %d", len(imports))
	}
}
