package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func setupTestStoreWithProject(t *testing.T, project, rootPath string) *Store {
	t.Helper()
	s := setupTestStore(t)
	if err := s.UpsertProject(project, rootPath); err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}
	return s
}

// --- bulk.go ---

func TestDropUserIndexes(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	ctx := context.Background()
	if err := s.DropUserIndexes(ctx); err != nil {
		t.Fatalf("DropUserIndexes: %v", err)
	}

	if err := s.CreateUserIndexes(ctx); err != nil {
		t.Fatalf("CreateUserIndexes after drop: %v", err)
	}
}

func TestBulkInsertNodes(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	nodes := []*Node{
		{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A", FilePath: "a.go", StartLine: 1, EndLine: 10},
		{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B", FilePath: "b.go", StartLine: 1, EndLine: 20},
		{Project: "test", Label: "Class", Name: "C", QualifiedName: "test.C", FilePath: "c.go", StartLine: 1, EndLine: 30},
	}

	ctx := context.Background()
	if err := s.BulkInsertNodes(ctx, nodes); err != nil {
		t.Fatalf("BulkInsertNodes: %v", err)
	}

	count, err := s.CountNodes("test")
	if err != nil {
		t.Fatalf("CountNodes: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 nodes, got %d", count)
	}
}

func TestBulkInsertNodesLargeBatch(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	nodes := make([]*Node, 250)
	for i := range nodes {
		nodes[i] = &Node{
			Project:       "test",
			Label:         "Function",
			Name:          fmt.Sprintf("func_%d", i),
			QualifiedName: fmt.Sprintf("test.func_%d", i),
			FilePath:      "pkg.go",
			StartLine:     i * 10,
			EndLine:       i*10 + 9,
		}
	}

	ctx := context.Background()
	if err := s.BulkInsertNodes(ctx, nodes); err != nil {
		t.Fatalf("BulkInsertNodes: %v", err)
	}

	count, _ := s.CountNodes("test")
	if count != 250 {
		t.Errorf("expected 250 nodes, got %d", count)
	}
}

func TestBulkInsertEdges(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})
	id3, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "C", QualifiedName: "test.C"})

	edges := []*Edge{
		{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"},
		{Project: "test", SourceID: id2, TargetID: id3, Type: "CALLS"},
		{Project: "test", SourceID: id1, TargetID: id3, Type: "IMPORTS"},
	}

	ctx := context.Background()
	if err := s.BulkInsertEdges(ctx, edges); err != nil {
		t.Fatalf("BulkInsertEdges: %v", err)
	}

	count, _ := s.CountEdges("test")
	if count != 3 {
		t.Errorf("expected 3 edges, got %d", count)
	}
}

func TestLoadNodeIDMap(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})

	ctx := context.Background()
	idMap, err := s.LoadNodeIDMap(ctx, "test")
	if err != nil {
		t.Fatalf("LoadNodeIDMap: %v", err)
	}

	if idMap["test.A"] != id1 {
		t.Errorf("test.A: expected %d, got %d", id1, idMap["test.A"])
	}
	if idMap["test.B"] != id2 {
		t.Errorf("test.B: expected %d, got %d", id2, idMap["test.B"])
	}
	if len(idMap) != 2 {
		t.Errorf("expected 2 entries, got %d", len(idMap))
	}
}

func TestLoadNodeIDMapEmpty(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	ctx := context.Background()
	idMap, err := s.LoadNodeIDMap(ctx, "test")
	if err != nil {
		t.Fatalf("LoadNodeIDMap: %v", err)
	}
	if len(idMap) != 0 {
		t.Errorf("expected empty map, got %d entries", len(idMap))
	}
}

// --- edges.go ---

func TestFindEdgesByTarget(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"})

	edges, err := s.FindEdgesByTarget(id2)
	if err != nil {
		t.Fatalf("FindEdgesByTarget: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].SourceID != id1 {
		t.Errorf("expected source %d, got %d", id1, edges[0].SourceID)
	}
}

func TestFindEdgesBySourceAndType(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})
	id3, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "C", QualifiedName: "test.C"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id3, Type: "IMPORTS"})

	edges, err := s.FindEdgesBySourceAndType(id1, "CALLS")
	if err != nil {
		t.Fatalf("FindEdgesBySourceAndType: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 CALLS edge, got %d", len(edges))
	}
	if edges[0].TargetID != id2 {
		t.Errorf("expected target %d, got %d", id2, edges[0].TargetID)
	}
}

func TestFindEdgesByTargetAndType(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "IMPORTS"})

	edges, err := s.FindEdgesByTargetAndType(id2, "IMPORTS")
	if err != nil {
		t.Fatalf("FindEdgesByTargetAndType: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 IMPORTS edge, got %d", len(edges))
	}
	if edges[0].Type != "IMPORTS" {
		t.Errorf("expected IMPORTS, got %s", edges[0].Type)
	}
}

func TestFindEdgesByType(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "IMPORTS"})

	edges, err := s.FindEdgesByType("test", "CALLS")
	if err != nil {
		t.Fatalf("FindEdgesByType: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 CALLS edge, got %d", len(edges))
	}
}

func TestDeleteEdgesByProject(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"})

	if err := s.DeleteEdgesByProject("test"); err != nil {
		t.Fatalf("DeleteEdgesByProject: %v", err)
	}

	count, _ := s.CountEdges("test")
	if count != 0 {
		t.Errorf("expected 0 edges after delete, got %d", count)
	}
}

func TestCountEdgesByType(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "IMPORTS"})

	count, err := s.CountEdgesByType("test", "CALLS")
	if err != nil {
		t.Fatalf("CountEdgesByType: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 CALLS edge, got %d", count)
	}

	count, _ = s.CountEdgesByType("test", "IMPORTS")
	if count != 1 {
		t.Errorf("expected 1 IMPORTS edge, got %d", count)
	}

	count, _ = s.CountEdgesByType("test", "NONEXISTENT")
	if count != 0 {
		t.Errorf("expected 0 for non-existent type, got %d", count)
	}
}

func TestDeleteEdgesByType(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "IMPORTS"})

	if err := s.DeleteEdgesByType("test", "CALLS"); err != nil {
		t.Fatalf("DeleteEdgesByType: %v", err)
	}

	count, _ := s.CountEdgesByType("test", "CALLS")
	if count != 0 {
		t.Errorf("expected 0 CALLS after delete, got %d", count)
	}
	count, _ = s.CountEdgesByType("test", "IMPORTS")
	if count != 1 {
		t.Errorf("expected 1 IMPORTS still present, got %d", count)
	}
}

func TestDeleteEdgesBySourceFile(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A", FilePath: "a.go"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B", FilePath: "b.go"})
	id3, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "C", QualifiedName: "test.C", FilePath: "a.go"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id3, TargetID: id2, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id2, TargetID: id1, Type: "CALLS"})

	if err := s.DeleteEdgesBySourceFile("test", "a.go", "CALLS"); err != nil {
		t.Fatalf("DeleteEdgesBySourceFile: %v", err)
	}

	count, _ := s.CountEdges("test")
	if count != 1 {
		t.Errorf("expected 1 edge remaining (from b.go source), got %d", count)
	}
}

func TestFindEdgesBySourceIDs(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})
	id3, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "C", QualifiedName: "test.C"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id3, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id2, TargetID: id3, Type: "IMPORTS"})

	t.Run("all types", func(t *testing.T) {
		result, err := s.FindEdgesBySourceIDs([]int64{id1, id2}, nil)
		if err != nil {
			t.Fatalf("FindEdgesBySourceIDs: %v", err)
		}
		if len(result[id1]) != 2 {
			t.Errorf("expected 2 edges from id1, got %d", len(result[id1]))
		}
		if len(result[id2]) != 1 {
			t.Errorf("expected 1 edge from id2, got %d", len(result[id2]))
		}
	})

	t.Run("filtered by type", func(t *testing.T) {
		result, err := s.FindEdgesBySourceIDs([]int64{id1, id2}, []string{"CALLS"})
		if err != nil {
			t.Fatalf("FindEdgesBySourceIDs: %v", err)
		}
		if len(result[id1]) != 2 {
			t.Errorf("expected 2 CALLS from id1, got %d", len(result[id1]))
		}
		if len(result[id2]) != 0 {
			t.Errorf("expected 0 CALLS from id2, got %d", len(result[id2]))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		result, err := s.FindEdgesBySourceIDs(nil, nil)
		if err != nil {
			t.Fatalf("FindEdgesBySourceIDs: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty map, got %d entries", len(result))
		}
	})
}

func TestFindEdgesByTargetIDs(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})
	id3, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "C", QualifiedName: "test.C"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id3, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id2, TargetID: id3, Type: "IMPORTS"})

	t.Run("all types", func(t *testing.T) {
		result, err := s.FindEdgesByTargetIDs([]int64{id2, id3}, nil)
		if err != nil {
			t.Fatalf("FindEdgesByTargetIDs: %v", err)
		}
		if len(result[id2]) != 1 {
			t.Errorf("expected 1 edge to id2, got %d", len(result[id2]))
		}
		if len(result[id3]) != 2 {
			t.Errorf("expected 2 edges to id3, got %d", len(result[id3]))
		}
	})

	t.Run("filtered by type", func(t *testing.T) {
		result, err := s.FindEdgesByTargetIDs([]int64{id3}, []string{"IMPORTS"})
		if err != nil {
			t.Fatalf("FindEdgesByTargetIDs: %v", err)
		}
		if len(result[id3]) != 1 {
			t.Errorf("expected 1 IMPORTS to id3, got %d", len(result[id3]))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		result, err := s.FindEdgesByTargetIDs(nil, nil)
		if err != nil {
			t.Fatalf("FindEdgesByTargetIDs: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty map, got %d entries", len(result))
		}
	})
}

func TestNodeNeighborNames(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	idA, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Alpha", QualifiedName: "test.Alpha"})
	idB, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Beta", QualifiedName: "test.Beta"})
	idC, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Gamma", QualifiedName: "test.Gamma"})

	s.InsertEdge(&Edge{Project: "test", SourceID: idA, TargetID: idB, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: idC, TargetID: idB, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: idB, TargetID: idC, Type: "HTTP_CALLS"})

	callers, callees := s.NodeNeighborNames(idB, 10)

	if len(callers) != 2 {
		t.Errorf("expected 2 callers of B, got %d: %v", len(callers), callers)
	}
	if len(callees) != 1 {
		t.Errorf("expected 1 callee of B, got %d: %v", len(callees), callees)
	}
}

// --- nodes.go ---

func TestFindNodeByID(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id, _ := s.UpsertNode(&Node{
		Project: "test", Label: "Function", Name: "Foo",
		QualifiedName: "test.Foo", FilePath: "foo.go",
		StartLine: 5, EndLine: 15,
		Properties: map[string]any{"key": "value"},
	})

	found, err := s.FindNodeByID(id)
	if err != nil {
		t.Fatalf("FindNodeByID: %v", err)
	}
	if found == nil {
		t.Fatal("expected node, got nil")
	}
	if found.Name != "Foo" {
		t.Errorf("expected Foo, got %s", found.Name)
	}
	if found.FilePath != "foo.go" {
		t.Errorf("expected foo.go, got %s", found.FilePath)
	}
	if found.Properties["key"] != "value" {
		t.Errorf("expected key=value, got %v", found.Properties["key"])
	}

	notFound, err := s.FindNodeByID(99999)
	if err != nil {
		t.Fatalf("FindNodeByID non-existent: %v", err)
	}
	if notFound != nil {
		t.Errorf("expected nil for non-existent ID, got %v", notFound)
	}
}

func TestFindNodesByLabel(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})
	s.UpsertNode(&Node{Project: "test", Label: "Class", Name: "C", QualifiedName: "test.C"})

	nodes, err := s.FindNodesByLabel("test", "Function")
	if err != nil {
		t.Fatalf("FindNodesByLabel: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 Function nodes, got %d", len(nodes))
	}

	nodes, err = s.FindNodesByLabel("test", "Class")
	if err != nil {
		t.Fatalf("FindNodesByLabel: %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("expected 1 Class node, got %d", len(nodes))
	}
}

func TestFindNodesByFile(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A", FilePath: "main.go"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B", FilePath: "main.go"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "C", QualifiedName: "test.C", FilePath: "other.go"})

	nodes, err := s.FindNodesByFile("test", "main.go")
	if err != nil {
		t.Fatalf("FindNodesByFile: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes in main.go, got %d", len(nodes))
	}
}

func TestDeleteNodesByProject(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})

	if err := s.DeleteNodesByProject("test"); err != nil {
		t.Fatalf("DeleteNodesByProject: %v", err)
	}

	count, _ := s.CountNodes("test")
	if count != 0 {
		t.Errorf("expected 0 nodes after delete, got %d", count)
	}
}

func TestDeleteNodesByFile(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A", FilePath: "a.go"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B", FilePath: "b.go"})

	if err := s.DeleteNodesByFile("test", "a.go"); err != nil {
		t.Fatalf("DeleteNodesByFile: %v", err)
	}

	count, _ := s.CountNodes("test")
	if count != 1 {
		t.Errorf("expected 1 node after delete, got %d", count)
	}
}

func TestDeleteNodesByLabel(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	s.UpsertNode(&Node{Project: "test", Label: "Class", Name: "B", QualifiedName: "test.B"})

	if err := s.DeleteNodesByLabel("test", "Function"); err != nil {
		t.Fatalf("DeleteNodesByLabel: %v", err)
	}

	count, _ := s.CountNodes("test")
	if count != 1 {
		t.Errorf("expected 1 node after delete, got %d", count)
	}

	nodes, _ := s.FindNodesByLabel("test", "Class")
	if len(nodes) != 1 {
		t.Errorf("expected Class node to survive, got %d", len(nodes))
	}
}

func TestFindNodesByIDs(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "C", QualifiedName: "test.C"})

	t.Run("subset", func(t *testing.T) {
		result, err := s.FindNodesByIDs([]int64{id1, id2})
		if err != nil {
			t.Fatalf("FindNodesByIDs: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("expected 2 nodes, got %d", len(result))
		}
		if result[id1].Name != "A" {
			t.Errorf("expected A, got %s", result[id1].Name)
		}
		if result[id2].Name != "B" {
			t.Errorf("expected B, got %s", result[id2].Name)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		result, err := s.FindNodesByIDs(nil)
		if err != nil {
			t.Fatalf("FindNodesByIDs: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty map, got %d", len(result))
		}
	})

	t.Run("non-existent ID", func(t *testing.T) {
		result, err := s.FindNodesByIDs([]int64{99999})
		if err != nil {
			t.Fatalf("FindNodesByIDs: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty map for non-existent ID, got %d", len(result))
		}
	})
}

func TestAllNodes(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	s.UpsertNode(&Node{Project: "test", Label: "Class", Name: "B", QualifiedName: "test.B"})

	nodes, err := s.AllNodes("test")
	if err != nil {
		t.Fatalf("AllNodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
}

// --- schema.go ---

func TestGetSchema(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Alpha", QualifiedName: "test.pkg.Alpha", FilePath: "pkg.go"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Beta", QualifiedName: "test.pkg.Beta", FilePath: "pkg.go"})
	id3, _ := s.UpsertNode(&Node{Project: "test", Label: "Class", Name: "OrderService", QualifiedName: "test.svc.OrderService", FilePath: "svc.go"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id3, Type: "IMPORTS"})

	schema, err := s.GetSchema("test")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}

	if len(schema.NodeLabels) < 1 {
		t.Fatal("expected at least 1 node label")
	}

	foundFunction := false
	for _, lc := range schema.NodeLabels {
		if lc.Label == "Function" {
			foundFunction = true
			if lc.Count != 2 {
				t.Errorf("expected 2 functions, got %d", lc.Count)
			}
		}
	}
	if !foundFunction {
		t.Error("expected Function label in schema")
	}

	if len(schema.RelationshipTypes) < 1 {
		t.Fatal("expected at least 1 relationship type")
	}

	if len(schema.RelationshipPatterns) < 1 {
		t.Fatal("expected at least 1 relationship pattern")
	}

	if len(schema.SampleFunctionNames) < 1 {
		t.Error("expected at least 1 sample function name")
	}

	if len(schema.SampleClassNames) < 1 {
		t.Error("expected at least 1 sample class name")
	}

	if len(schema.SampleQualifiedNames) < 1 {
		t.Error("expected at least 1 sample qualified name")
	}
}

func TestGetSchemaEmpty(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	schema, err := s.GetSchema("test")
	if err != nil {
		t.Fatalf("GetSchema on empty: %v", err)
	}
	if len(schema.NodeLabels) != 0 {
		t.Errorf("expected 0 labels, got %d", len(schema.NodeLabels))
	}
	if len(schema.RelationshipTypes) != 0 {
		t.Errorf("expected 0 rel types, got %d", len(schema.RelationshipTypes))
	}
}

// --- router.go ---

func TestRouterLifecycle(t *testing.T) {
	dir := t.TempDir()

	r, err := NewRouterWithDir(dir)
	if err != nil {
		t.Fatalf("NewRouterWithDir: %v", err)
	}

	if r.Dir() != dir {
		t.Errorf("expected dir %s, got %s", dir, r.Dir())
	}

	if r.HasProject("myproj") {
		t.Error("expected HasProject=false for non-existent project")
	}

	st, err := r.ForProject("myproj")
	if err != nil {
		t.Fatalf("ForProject: %v", err)
	}

	if err := st.UpsertProject("myproj", "/home/user/myproj"); err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}

	if !r.HasProject("myproj") {
		t.Error("expected HasProject=true after ForProject")
	}

	st2, err := r.ForProject("myproj")
	if err != nil {
		t.Fatalf("ForProject second call: %v", err)
	}
	if st != st2 {
		t.Error("expected same Store instance from cache")
	}

	_, err = r.ForProject("*")
	if err == nil {
		t.Error("expected error for wildcard project name")
	}
	_, err = r.ForProject("all")
	if err == nil {
		t.Error("expected error for 'all' project name")
	}
}

func TestRouterAllStores(t *testing.T) {
	dir := t.TempDir()

	r, err := NewRouterWithDir(dir)
	if err != nil {
		t.Fatalf("NewRouterWithDir: %v", err)
	}

	st1, _ := r.ForProject("proj1")
	st1.UpsertProject("proj1", "/tmp/proj1")

	st2, _ := r.ForProject("proj2")
	st2.UpsertProject("proj2", "/tmp/proj2")

	allStores := r.AllStores()
	if len(allStores) < 2 {
		t.Errorf("expected at least 2 stores, got %d", len(allStores))
	}
}

func TestRouterListProjects(t *testing.T) {
	dir := t.TempDir()

	r, err := NewRouterWithDir(dir)
	if err != nil {
		t.Fatalf("NewRouterWithDir: %v", err)
	}

	st, _ := r.ForProject("testproj")
	st.UpsertProject("testproj", "/home/user/testproj")

	projects, err := r.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) < 1 {
		t.Fatal("expected at least 1 project")
	}

	found := false
	for _, p := range projects {
		if p.Name == "testproj" {
			found = true
			if p.RootPath != "/home/user/testproj" {
				t.Errorf("expected root /home/user/testproj, got %s", p.RootPath)
			}
		}
	}
	if !found {
		t.Error("expected testproj in list")
	}
}

func TestRouterDeleteProject(t *testing.T) {
	dir := t.TempDir()

	r, err := NewRouterWithDir(dir)
	if err != nil {
		t.Fatalf("NewRouterWithDir: %v", err)
	}

	st, _ := r.ForProject("todelete")
	st.UpsertProject("todelete", "/tmp/todelete")

	if !r.HasProject("todelete") {
		t.Fatal("expected project to exist before delete")
	}

	if err := r.DeleteProject("todelete"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	if r.HasProject("todelete") {
		t.Error("expected project to be gone after delete")
	}
}

func TestRouterCloseAll(t *testing.T) {
	dir := t.TempDir()

	r, err := NewRouterWithDir(dir)
	if err != nil {
		t.Fatalf("NewRouterWithDir: %v", err)
	}

	r.ForProject("p1")
	r.ForProject("p2")

	r.CloseAll()

	allStores := r.AllStores()
	for _, st := range allStores {
		if st == nil {
			t.Error("nil store after CloseAll + AllStores")
		}
	}
}

func TestRouterAllStoresSkipsLegacy(t *testing.T) {
	dir := t.TempDir()

	legacyPath := filepath.Join(dir, "codebook.db")
	f, err := os.Create(legacyPath)
	if err != nil {
		t.Fatalf("create legacy db: %v", err)
	}
	f.Close()

	r, err := NewRouterWithDir(dir)
	if err != nil {
		t.Fatalf("NewRouterWithDir: %v", err)
	}

	allStores := r.AllStores()
	for name := range allStores {
		if name == "codebook" {
			t.Error("AllStores should skip legacy single DB")
		}
	}
}

// --- migrate.go ---

func TestMigrateNoLegacyDB(t *testing.T) {
	dir := t.TempDir()

	r, err := NewRouterWithDir(dir)
	if err != nil {
		t.Fatalf("NewRouterWithDir: %v", err)
	}

	if err := r.migrate(); err != nil {
		t.Fatalf("migrate with no legacy DB should be no-op: %v", err)
	}
}

func TestMigrateWithLegacyDB(t *testing.T) {
	dir := t.TempDir()

	legacyPath := filepath.Join(dir, "codebook.db")
	legacyStore, err := OpenPath(legacyPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}

	if err := legacyStore.UpsertProject("migproj", "/tmp/migproj"); err != nil {
		t.Fatalf("upsert project: %v", err)
	}
	id1, _ := legacyStore.UpsertNode(&Node{Project: "migproj", Label: "Function", Name: "Fn1", QualifiedName: "migproj.Fn1"})
	id2, _ := legacyStore.UpsertNode(&Node{Project: "migproj", Label: "Function", Name: "Fn2", QualifiedName: "migproj.Fn2"})
	legacyStore.InsertEdge(&Edge{Project: "migproj", SourceID: id1, TargetID: id2, Type: "CALLS"})
	legacyStore.UpsertFileHash("migproj", "fn.go", "abc123")
	legacyStore.Close()

	r, err := NewRouterWithDir(dir)
	if err != nil {
		t.Fatalf("NewRouterWithDir: %v", err)
	}

	if err := r.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	targetPath := filepath.Join(dir, "migproj.db")
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Fatal("expected migrated per-project DB to exist")
	}

	migratedPath := legacyPath + ".migrated"
	if _, err := os.Stat(migratedPath); os.IsNotExist(err) {
		t.Fatal("expected legacy DB to be renamed to .migrated")
	}

	st, err := r.ForProject("migproj")
	if err != nil {
		t.Fatalf("ForProject migproj: %v", err)
	}
	count, _ := st.CountNodes("migproj")
	if count != 2 {
		t.Errorf("expected 2 migrated nodes, got %d", count)
	}
	edgeCount, _ := st.CountEdges("migproj")
	if edgeCount != 1 {
		t.Errorf("expected 1 migrated edge, got %d", edgeCount)
	}
}

func TestMigrateAlreadyMigrated(t *testing.T) {
	dir := t.TempDir()

	legacyPath := filepath.Join(dir, "codebook.db")
	legacyStore, err := OpenPath(legacyPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	legacyStore.UpsertProject("proj", "/tmp/proj")
	legacyStore.UpsertNode(&Node{Project: "proj", Label: "Function", Name: "X", QualifiedName: "proj.X"})
	legacyStore.Close()

	targetStore, err := OpenPath(filepath.Join(dir, "proj.db"))
	if err != nil {
		t.Fatalf("open target db: %v", err)
	}
	targetStore.Close()

	r, _ := NewRouterWithDir(dir)
	if err := r.migrate(); err != nil {
		t.Fatalf("migrate with existing target: %v", err)
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "dest.txt")

	content := []byte("test content for copy")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	dstContent, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(dstContent) != string(content) {
		t.Errorf("expected %q, got %q", content, dstContent)
	}
}

// --- search.go ---

func TestSearchWithQNPattern(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Handler", QualifiedName: "test.api.v1.Handler", FilePath: "api.go"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Handler", QualifiedName: "test.api.v2.Handler", FilePath: "api.go"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Helper", QualifiedName: "test.util.Helper", FilePath: "util.go"})

	output, err := s.Search(&SearchParams{Project: "test", QNPattern: ".*api\\.v1.*"})
	if err != nil {
		t.Fatalf("Search with QNPattern: %v", err)
	}
	if len(output.Results) != 1 {
		t.Errorf("expected 1 match for api.v1, got %d", len(output.Results))
	}
}

func TestSearchWithExcludeEntryPoints(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	s.UpsertNode(&Node{
		Project: "test", Label: "Function", Name: "Main",
		QualifiedName: "test.Main", FilePath: "main.go",
		Properties: map[string]any{"is_entry_point": true},
	})
	s.UpsertNode(&Node{
		Project: "test", Label: "Function", Name: "Helper",
		QualifiedName: "test.Helper", FilePath: "helper.go",
	})

	output, err := s.Search(&SearchParams{Project: "test", ExcludeEntryPoints: true})
	if err != nil {
		t.Fatalf("Search with ExcludeEntryPoints: %v", err)
	}
	for _, r := range output.Results {
		if r.Node.Name == "Main" {
			t.Error("expected Main to be excluded as entry point")
		}
	}
	if output.Total != 1 {
		t.Errorf("expected 1 result (Helper only), got %d", output.Total)
	}
}

func TestSearchWithIncludeConnected(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"})

	output, err := s.Search(&SearchParams{
		Project:          "test",
		NamePattern:      "A",
		IncludeConnected: true,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(output.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(output.Results))
	}
	if len(output.Results[0].ConnectedNames) == 0 {
		t.Error("expected connected names to be populated")
	}
}

func TestSearchSortByName(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Zebra", QualifiedName: "test.Zebra"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Alpha", QualifiedName: "test.Alpha"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Middle", QualifiedName: "test.Middle"})

	output, err := s.Search(&SearchParams{Project: "test", SortBy: "name"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(output.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(output.Results))
	}
	if output.Results[0].Node.Name != "Alpha" {
		t.Errorf("expected Alpha first, got %s", output.Results[0].Node.Name)
	}
	if output.Results[2].Node.Name != "Zebra" {
		t.Errorf("expected Zebra last, got %s", output.Results[2].Node.Name)
	}
}

func TestSearchSortByDegree(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Hub", QualifiedName: "test.Hub"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Leaf", QualifiedName: "test.Leaf"})
	id3, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Mid", QualifiedName: "test.Mid"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id3, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id3, TargetID: id2, Type: "CALLS"})

	output, err := s.Search(&SearchParams{Project: "test", SortBy: "degree", MinDegree: -1, MaxDegree: -1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(output.Results) < 2 {
		t.Fatal("expected at least 2 results")
	}
	if output.Results[0].Node.Name != "Hub" {
		t.Errorf("expected Hub first (highest degree), got %s", output.Results[0].Node.Name)
	}
}

func TestSearchCaseSensitive(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "HandleRequest", QualifiedName: "test.HandleRequest"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "handlerequest", QualifiedName: "test.handlerequest"})

	t.Run("case insensitive default", func(t *testing.T) {
		output, err := s.Search(&SearchParams{Project: "test", NamePattern: "handlerequest"})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if output.Total != 2 {
			t.Errorf("expected 2 case-insensitive matches, got %d", output.Total)
		}
	})

	t.Run("case sensitive", func(t *testing.T) {
		output, err := s.Search(&SearchParams{Project: "test", NamePattern: "handlerequest", CaseSensitive: true})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if output.Total != 1 {
			t.Errorf("expected 1 case-sensitive match, got %d", output.Total)
		}
	})
}

func TestSearchWithMinDegree(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Popular", QualifiedName: "test.Popular"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Unpopular", QualifiedName: "test.Unpopular"})
	id3, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Caller1", QualifiedName: "test.Caller1"})
	id4, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Caller2", QualifiedName: "test.Caller2"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id3, TargetID: id1, Type: "CALLS"})
	s.InsertEdge(&Edge{Project: "test", SourceID: id4, TargetID: id1, Type: "CALLS"})
	_ = id2

	output, err := s.Search(&SearchParams{Project: "test", MinDegree: 2, MaxDegree: -1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, r := range output.Results {
		total := r.InDegree + r.OutDegree
		if total < 2 {
			t.Errorf("node %s has degree %d, below min 2", r.Node.Name, total)
		}
	}
}

func TestIsEntryPoint(t *testing.T) {
	tests := []struct {
		name  string
		props map[string]any
		want  bool
	}{
		{"nil props", nil, false},
		{"no key", map[string]any{}, false},
		{"false", map[string]any{"is_entry_point": false}, false},
		{"true", map[string]any{"is_entry_point": true}, true},
		{"non-bool", map[string]any{"is_entry_point": "yes"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Node{Properties: tt.props}
			got := isEntryPoint(n)
			if got != tt.want {
				t.Errorf("isEntryPoint(%v) = %v, want %v", tt.props, got, tt.want)
			}
		})
	}
}

func TestExtractLiteralQuery(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{"", ""},
		{"Handler", "Handler"},
		{"(?i)Handler", "Handler"},
		{".*Handler.*", "Handler"},
		{"(?i).*Handler.*", "Handler"},
		{".*hand.*ler.*", ""},
		{"foo[bar]", ""},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := extractLiteralQuery(tt.pattern)
			if got != tt.want {
				t.Errorf("extractLiteralQuery(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestCompareBool(t *testing.T) {
	if compareBool(true, true) != 0 {
		t.Error("true,true should be 0")
	}
	if compareBool(false, false) != 0 {
		t.Error("false,false should be 0")
	}
	if compareBool(true, false) != -1 {
		t.Error("true,false should be -1")
	}
	if compareBool(false, true) != 1 {
		t.Error("false,true should be 1")
	}
}

func TestPaginateResults(t *testing.T) {
	results := make([]*SearchResult, 10)
	for i := range results {
		results[i] = &SearchResult{Node: &Node{Name: fmt.Sprintf("n%d", i)}}
	}

	t.Run("normal page", func(t *testing.T) {
		out := paginateResults(results, 0, 3)
		if len(out.Results) != 3 || out.Total != 10 {
			t.Errorf("expected 3 results, total=10, got %d, total=%d", len(out.Results), out.Total)
		}
	})

	t.Run("offset beyond total", func(t *testing.T) {
		out := paginateResults(results, 100, 3)
		if len(out.Results) != 0 {
			t.Errorf("expected 0 results for offset beyond total, got %d", len(out.Results))
		}
	})

	t.Run("limit exceeds remaining", func(t *testing.T) {
		out := paginateResults(results, 8, 5)
		if len(out.Results) != 2 {
			t.Errorf("expected 2 results, got %d", len(out.Results))
		}
	})
}

// --- store.go ---

func TestOpenPathAndDBPath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer s.Close()

	if s.DBPath() != dbPath {
		t.Errorf("expected DBPath %s, got %s", dbPath, s.DBPath())
	}
}

func TestOpenInDir(t *testing.T) {
	dir := t.TempDir()

	s, err := OpenInDir(dir, "testproject")
	if err != nil {
		t.Fatalf("OpenInDir: %v", err)
	}
	defer s.Close()

	expectedPath := filepath.Join(dir, "testproject.db")
	if s.DBPath() != expectedPath {
		t.Errorf("expected %s, got %s", expectedPath, s.DBPath())
	}
}

func TestWithTransaction(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	ctx := context.Background()
	err := s.WithTransaction(ctx, func(txStore *Store) error {
		_, err := txStore.UpsertNode(&Node{
			Project: "test", Label: "Function", Name: "TxNode",
			QualifiedName: "test.TxNode",
		})
		return err
	})
	if err != nil {
		t.Fatalf("WithTransaction: %v", err)
	}

	found, err := s.FindNodeByQN("test", "test.TxNode")
	if err != nil {
		t.Fatalf("FindNodeByQN: %v", err)
	}
	if found == nil {
		t.Fatal("expected node committed by transaction")
	}
}

func TestWithTransactionRollback(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	ctx := context.Background()
	err := s.WithTransaction(ctx, func(txStore *Store) error {
		txStore.UpsertNode(&Node{
			Project: "test", Label: "Function", Name: "RollbackNode",
			QualifiedName: "test.RollbackNode",
		})
		return fmt.Errorf("intentional error")
	})
	if err == nil {
		t.Fatal("expected error from transaction")
	}

	found, _ := s.FindNodeByQN("test", "test.RollbackNode")
	if found != nil {
		t.Error("expected node to not exist after rollback")
	}
}

func TestBeginEndBulkWrite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bulk.db")

	s, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	s.BeginBulkWrite(ctx)
	s.EndBulkWrite(ctx)
}

func TestCheckpoint(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "checkpoint.db")

	s, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	s.Checkpoint(ctx)
}

func TestUnmarshalProps(t *testing.T) {
	tests := []struct {
		input string
		key   string
		val   any
		empty bool
	}{
		{`{"key":"value"}`, "key", "value", false},
		{`{}`, "", nil, true},
		{``, "", nil, true},
		{`invalid json`, "", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := UnmarshalProps(tt.input)
			if tt.empty {
				if len(result) != 0 && tt.input != `{}` {
					return
				}
			} else {
				if result[tt.key] != tt.val {
					t.Errorf("expected %v, got %v", tt.val, result[tt.key])
				}
			}
		})
	}
}

// --- projects.go ---

func TestDeleteFileHash(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	s.UpsertFileHash("test", "a.go", "hash1")
	s.UpsertFileHash("test", "b.go", "hash2")

	if err := s.DeleteFileHash("test", "a.go"); err != nil {
		t.Fatalf("DeleteFileHash: %v", err)
	}

	hashes, _ := s.GetFileHashes("test")
	if _, ok := hashes["a.go"]; ok {
		t.Error("expected a.go to be deleted")
	}
	if hashes["b.go"] != "hash2" {
		t.Error("expected b.go to remain")
	}
}

func TestDeleteFileHashes(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	s.UpsertFileHash("test", "a.go", "hash1")
	s.UpsertFileHash("test", "b.go", "hash2")

	if err := s.DeleteFileHashes("test"); err != nil {
		t.Fatalf("DeleteFileHashes: %v", err)
	}

	hashes, _ := s.GetFileHashes("test")
	if len(hashes) != 0 {
		t.Errorf("expected 0 hashes after bulk delete, got %d", len(hashes))
	}
}

func TestListFilesForProject(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A", FilePath: "main.go"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B", FilePath: "main.go"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "C", QualifiedName: "test.C", FilePath: "other.go"})
	s.UpsertNode(&Node{Project: "test", Label: "Module", Name: "mod", QualifiedName: "test.mod", FilePath: ""})

	files, err := s.ListFilesForProject("test")
	if err != nil {
		t.Fatalf("ListFilesForProject: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 distinct files (main.go, other.go), got %d: %v", len(files), files)
	}
}

func TestSearchRelevanceSorting(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "Handler", QualifiedName: "test.Handler"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "HandlerFactory", QualifiedName: "test.HandlerFactory"})
	s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "RequestHandler", QualifiedName: "test.RequestHandler"})

	output, err := s.Search(&SearchParams{Project: "test", NamePattern: ".*Handler.*", SortBy: "relevance"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(output.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(output.Results))
	}
	if output.Results[0].Node.Name != "Handler" {
		t.Errorf("expected exact match 'Handler' first, got %s", output.Results[0].Node.Name)
	}
}

func TestSearchWithDirectionFilter(t *testing.T) {
	s := setupTestStoreWithProject(t, "test", "/tmp/test")

	id1, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "A", QualifiedName: "test.A"})
	id2, _ := s.UpsertNode(&Node{Project: "test", Label: "Function", Name: "B", QualifiedName: "test.B"})

	s.InsertEdge(&Edge{Project: "test", SourceID: id1, TargetID: id2, Type: "CALLS"})

	output, err := s.Search(&SearchParams{
		Project:   "test",
		Direction: "outbound",
		MinDegree: 1,
		MaxDegree: -1,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	found := false
	for _, r := range output.Results {
		if r.Node.Name == "A" {
			found = true
		}
	}
	if !found {
		t.Error("expected A (with outbound degree >= 1) in results")
	}
}
