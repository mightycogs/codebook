package cypher

import (
	"strings"
	"testing"

	"github.com/mightycogs/codebook/internal/store"
)

// --- Lexer tests ---

func TestLexBasicQuery(t *testing.T) {
	tokens, err := Lex(`MATCH (f:Function) WHERE f.name = "Hello" RETURN f.name`)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}

	expected := []TokenType{
		TokMatch, TokLParen, TokIdent, TokColon, TokIdent, TokRParen,
		TokWhere, TokIdent, TokDot, TokIdent, TokEQ, TokString,
		TokReturn, TokIdent, TokDot, TokIdent, TokEOF,
	}

	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, tok := range tokens {
		if tok.Type != expected[i] {
			t.Errorf("token[%d]: expected type %d, got %d (%q)", i, expected[i], tok.Type, tok.Value)
		}
	}
}

func TestLexRegexOperator(t *testing.T) {
	tokens, err := Lex(`f.name =~ ".*Handler"`)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	// f, ., name, =~, ".*Handler"
	if tokens[3].Type != TokRegex {
		t.Errorf("expected TokRegex, got type %d (%q)", tokens[3].Type, tokens[3].Value)
	}
}

func TestLexVariableLengthPath(t *testing.T) {
	tokens, err := Lex(`[:CALLS*1..3]`)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	expected := []TokenType{
		TokLBracket, TokColon, TokIdent, TokStar, TokNumber, TokDotDot, TokNumber, TokRBracket, TokEOF,
	}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, tok := range tokens {
		if tok.Type != expected[i] {
			t.Errorf("token[%d]: expected type %d, got %d (%q)", i, expected[i], tok.Type, tok.Value)
		}
	}
}

// --- Parser tests ---

func TestParseNodePattern(t *testing.T) {
	q, err := Parse(`MATCH (f:Function {name: "Hello"}) RETURN f`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if q.Match == nil || q.Match.Pattern == nil {
		t.Fatal("expected match pattern")
	}
	elems := q.Match.Pattern.Elements
	if len(elems) != 1 {
		t.Fatalf("expected 1 element, got %d", len(elems))
	}
	node, ok := elems[0].(*NodePattern)
	if !ok {
		t.Fatalf("expected *NodePattern, got %T", elems[0])
	}
	if node.Variable != "f" {
		t.Errorf("expected variable 'f', got %q", node.Variable)
	}
	if node.Label != "Function" {
		t.Errorf("expected label 'Function', got %q", node.Label)
	}
	if node.Props["name"] != "Hello" {
		t.Errorf("expected prop name='Hello', got %q", node.Props["name"])
	}
}

func TestParseRelationship(t *testing.T) {
	q, err := Parse(`MATCH (f)-[:CALLS]->(g) RETURN f.name, g.name`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	elems := q.Match.Pattern.Elements
	if len(elems) != 3 {
		t.Fatalf("expected 3 elements (node-rel-node), got %d", len(elems))
	}
	rel, ok := elems[1].(*RelPattern)
	if !ok {
		t.Fatalf("expected *RelPattern, got %T", elems[1])
	}
	if len(rel.Types) != 1 || rel.Types[0] != "CALLS" {
		t.Errorf("expected CALLS type, got %v", rel.Types)
	}
	if rel.Direction != "outbound" {
		t.Errorf("expected outbound, got %q", rel.Direction)
	}
	if rel.MinHops != 1 || rel.MaxHops != 1 {
		t.Errorf("expected hops 1..1, got %d..%d", rel.MinHops, rel.MaxHops)
	}
}

func TestParseVariableLength(t *testing.T) {
	q, err := Parse(`MATCH (f)-[:CALLS*1..3]->(g) RETURN g.name`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rel, ok := q.Match.Pattern.Elements[1].(*RelPattern)
	if !ok {
		t.Fatalf("expected *RelPattern, got %T", q.Match.Pattern.Elements[1])
	}
	if rel.MinHops != 1 {
		t.Errorf("expected minHops=1, got %d", rel.MinHops)
	}
	if rel.MaxHops != 3 {
		t.Errorf("expected maxHops=3, got %d", rel.MaxHops)
	}
}

func TestParseWhereRegex(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) WHERE f.name =~ ".*Handler" RETURN f.name`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if q.Where == nil {
		t.Fatal("expected WHERE clause")
	}
	if len(q.Where.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(q.Where.Conditions))
	}
	c := q.Where.Conditions[0]
	if c.Operator != "=~" {
		t.Errorf("expected =~, got %q", c.Operator)
	}
	if c.Value != ".*Handler" {
		t.Errorf("expected '.*Handler', got %q", c.Value)
	}
}

func TestParseReturnWithCount(t *testing.T) {
	q, err := Parse(`MATCH (f)-[:CALLS]->(g) RETURN f.name, COUNT(g) AS cnt ORDER BY cnt DESC LIMIT 10`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if q.Return == nil {
		t.Fatal("expected RETURN clause")
	}
	if len(q.Return.Items) != 2 {
		t.Fatalf("expected 2 return items, got %d", len(q.Return.Items))
	}

	// First item: f.name
	if q.Return.Items[0].Variable != "f" || q.Return.Items[0].Property != "name" {
		t.Errorf("expected f.name, got %s.%s", q.Return.Items[0].Variable, q.Return.Items[0].Property)
	}

	// Second item: COUNT(g) AS cnt
	if q.Return.Items[1].Func != "COUNT" {
		t.Errorf("expected COUNT, got %q", q.Return.Items[1].Func)
	}
	if q.Return.Items[1].Variable != "g" {
		t.Errorf("expected variable 'g', got %q", q.Return.Items[1].Variable)
	}
	if q.Return.Items[1].Alias != "cnt" {
		t.Errorf("expected alias 'cnt', got %q", q.Return.Items[1].Alias)
	}

	// ORDER BY
	if q.Return.OrderBy != "cnt" {
		t.Errorf("expected ORDER BY cnt, got %q", q.Return.OrderBy)
	}
	if q.Return.OrderDir != "DESC" {
		t.Errorf("expected DESC, got %q", q.Return.OrderDir)
	}
	if q.Return.Limit != 10 {
		t.Errorf("expected LIMIT 10, got %d", q.Return.Limit)
	}
}

func TestParseBidirectional(t *testing.T) {
	q, err := Parse(`MATCH (f:Function)-[:CALLS]-(g) RETURN f.name, g.name`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rel, ok := q.Match.Pattern.Elements[1].(*RelPattern)
	if !ok {
		t.Fatalf("expected *RelPattern, got %T", q.Match.Pattern.Elements[1])
	}
	if rel.Direction != "any" {
		t.Errorf("expected 'any' direction, got %q", rel.Direction)
	}
}

func TestParseInbound(t *testing.T) {
	q, err := Parse(`MATCH (f:Function)<-[:CALLS]-(g) RETURN f.name`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rel, ok := q.Match.Pattern.Elements[1].(*RelPattern)
	if !ok {
		t.Fatalf("expected *RelPattern, got %T", q.Match.Pattern.Elements[1])
	}
	if rel.Direction != "inbound" {
		t.Errorf("expected inbound, got %q", rel.Direction)
	}
}

func TestParseMultipleRelTypes(t *testing.T) {
	q, err := Parse(`MATCH (f)-[:CALLS|HTTP_CALLS]->(g) RETURN g.name`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rel, ok := q.Match.Pattern.Elements[1].(*RelPattern)
	if !ok {
		t.Fatalf("expected *RelPattern, got %T", q.Match.Pattern.Elements[1])
	}
	if len(rel.Types) != 2 {
		t.Fatalf("expected 2 types, got %d", len(rel.Types))
	}
	if rel.Types[0] != "CALLS" || rel.Types[1] != "HTTP_CALLS" {
		t.Errorf("expected [CALLS, HTTP_CALLS], got %v", rel.Types)
	}
}

func TestParseWhereStartsWith(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) WHERE f.name STARTS WITH "Send" RETURN f`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c := q.Where.Conditions[0]
	if c.Operator != "STARTS WITH" {
		t.Errorf("expected 'STARTS WITH', got %q", c.Operator)
	}
	if c.Value != "Send" {
		t.Errorf("expected 'Send', got %q", c.Value)
	}
}

func TestParseWhereContains(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) WHERE f.name CONTAINS "Handler" RETURN f`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c := q.Where.Conditions[0]
	if c.Operator != "CONTAINS" {
		t.Errorf("expected CONTAINS, got %q", c.Operator)
	}
}

func TestParseWhereNumericComparison(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) WHERE f.start_line > 10 RETURN f`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c := q.Where.Conditions[0]
	if c.Operator != ">" {
		t.Errorf("expected '>', got %q", c.Operator)
	}
	if c.Value != "10" {
		t.Errorf("expected '10', got %q", c.Value)
	}
}

func TestParseWhereAnd(t *testing.T) {
	q, err := Parse(`MATCH (f) WHERE f.label = "Function" AND f.name = "Foo" RETURN f`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(q.Where.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(q.Where.Conditions))
	}
	if q.Where.Operator != "AND" {
		t.Errorf("expected AND, got %q", q.Where.Operator)
	}
}

func TestParseDistinct(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) RETURN DISTINCT f.label`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !q.Return.Distinct {
		t.Error("expected DISTINCT to be true")
	}
}

// --- Integration test ---

func setupTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatalf("open memory store: %v", err)
	}

	if err := s.UpsertProject("test", "/tmp/test"); err != nil {
		t.Fatalf("upsert project: %v", err)
	}

	// Create nodes
	idA, _ := s.UpsertNode(&store.Node{
		Project: "test", Label: "Function", Name: "HandleOrder",
		QualifiedName: "test.main.HandleOrder", FilePath: "main.go",
		StartLine: 10, EndLine: 30,
		Properties: map[string]any{"signature": "func HandleOrder(w, r)"},
	})
	idB, _ := s.UpsertNode(&store.Node{
		Project: "test", Label: "Function", Name: "ValidateOrder",
		QualifiedName: "test.service.ValidateOrder", FilePath: "service.go",
		StartLine: 5, EndLine: 20,
		Properties: map[string]any{"signature": "func ValidateOrder(o Order) error"},
	})
	idC, _ := s.UpsertNode(&store.Node{
		Project: "test", Label: "Function", Name: "SubmitOrder",
		QualifiedName: "test.service.SubmitOrder", FilePath: "service.go",
		StartLine: 25, EndLine: 50,
		Properties: map[string]any{"signature": "func SubmitOrder(o Order) error"},
	})
	idD, _ := s.UpsertNode(&store.Node{
		Project: "test", Label: "Module", Name: "main",
		QualifiedName: "test.main", FilePath: "main.go",
	})
	idE, _ := s.UpsertNode(&store.Node{
		Project: "test", Label: "Function", Name: "LogError",
		QualifiedName: "test.util.LogError", FilePath: "util.go",
		StartLine: 1, EndLine: 5,
	})

	// Edges: HandleOrder -> ValidateOrder -> SubmitOrder
	//        HandleOrder -> LogError
	mustInsertEdge(t, s, &store.Edge{Project: "test", SourceID: idA, TargetID: idB, Type: "CALLS"})
	mustInsertEdge(t, s, &store.Edge{Project: "test", SourceID: idB, TargetID: idC, Type: "CALLS"})
	mustInsertEdge(t, s, &store.Edge{Project: "test", SourceID: idA, TargetID: idE, Type: "CALLS"})
	mustInsertEdge(t, s, &store.Edge{Project: "test", SourceID: idD, TargetID: idA, Type: "DEFINES"})

	return s
}

// mustInsertEdge inserts an edge and fails the test on error.
func mustInsertEdge(t *testing.T, s *store.Store, edge *store.Edge) {
	t.Helper()
	if _, err := s.InsertEdge(edge); err != nil {
		t.Fatalf("insert edge: %v", err)
	}
}

func TestExecuteSimpleMatch(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) RETURN f.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 4 {
		t.Errorf("expected 4 functions, got %d", len(result.Rows))
	}
}

func TestExecuteRelationshipQuery(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function)-[:CALLS]->(g:Function) RETURN f.name, g.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	// HandleOrder -> ValidateOrder, HandleOrder -> LogError, ValidateOrder -> SubmitOrder
	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result.Rows))
	}

	// Verify columns
	if len(result.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(result.Columns))
	}

	// Check that HandleOrder -> ValidateOrder is in the results
	found := false
	for _, row := range result.Rows {
		if row["f.name"] == "HandleOrder" && row["g.name"] == "ValidateOrder" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected HandleOrder -> ValidateOrder in results")
	}
}

func TestExecuteWhereFilter(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) WHERE f.name = "HandleOrder" RETURN f.name, f.file_path`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0]["f.name"] != "HandleOrder" {
		t.Errorf("expected HandleOrder, got %v", result.Rows[0]["f.name"])
	}
}

func TestExecuteWhereRegex(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) WHERE f.name =~ ".*Order" RETURN f.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	// HandleOrder, ValidateOrder, SubmitOrder
	if len(result.Rows) != 3 {
		t.Errorf("expected 3 rows, got %d", len(result.Rows))
	}
}

func TestExecuteWhereStartsWith(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) WHERE f.name STARTS WITH "Submit" RETURN f.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0]["f.name"] != "SubmitOrder" {
		t.Errorf("expected SubmitOrder, got %v", result.Rows[0]["f.name"])
	}
}

func TestExecuteWhereContains(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) WHERE f.name CONTAINS "Order" RETURN f.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 3 {
		t.Errorf("expected 3 rows (HandleOrder, ValidateOrder, SubmitOrder), got %d", len(result.Rows))
	}
}

func TestExecuteWhereNumeric(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) WHERE f.start_line > 10 RETURN f.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	// SubmitOrder (start_line=25)
	if len(result.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(result.Rows))
	}
}

func TestExecuteVariableLength(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	// HandleOrder calls ValidateOrder (hop 1), ValidateOrder calls SubmitOrder (hop 2)
	result, err := exec.Execute(`MATCH (f:Function {name: "HandleOrder"})-[:CALLS*1..2]->(g:Function) RETURN g.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	// Should include ValidateOrder (hop 1), LogError (hop 1), SubmitOrder (hop 2)
	if len(result.Rows) < 2 {
		t.Errorf("expected at least 2 rows for variable-length path, got %d", len(result.Rows))
	}
}

func TestExecuteWithLimit(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) RETURN f.name LIMIT 2`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result.Rows))
	}
}

func TestExecuteWithOrderBy(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) RETURN f.name ORDER BY f.name ASC`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) < 2 {
		t.Fatalf("expected at least 2 rows, got %d", len(result.Rows))
	}
	// First should be HandleOrder (alphabetically first)
	firstName := result.Rows[0]["f.name"]
	if firstName != "HandleOrder" {
		t.Errorf("expected first row 'HandleOrder', got %v", firstName)
	}
}

func TestExecuteCountAggregation(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function)-[:CALLS]->(g:Function) RETURN f.name, COUNT(g) AS call_count ORDER BY call_count DESC`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) < 1 {
		t.Fatalf("expected at least 1 row, got %d", len(result.Rows))
	}
	// HandleOrder calls 2 functions (ValidateOrder, LogError)
	for _, row := range result.Rows {
		if row["f.name"] == "HandleOrder" {
			count, ok := row["call_count"].(int)
			if !ok {
				t.Errorf("expected int count, got %T", row["call_count"])
			} else if count != 2 {
				t.Errorf("expected call_count=2 for HandleOrder, got %d", count)
			}
		}
	}
}

func TestExecuteInboundRelationship(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	// Who calls ValidateOrder?
	result, err := exec.Execute(`MATCH (f:Function)<-[:CALLS]-(g:Function) WHERE f.name = "ValidateOrder" RETURN g.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 caller, got %d", len(result.Rows))
	}
	if result.Rows[0]["g.name"] != "HandleOrder" {
		t.Errorf("expected HandleOrder, got %v", result.Rows[0]["g.name"])
	}
}

func TestExecuteDistinct(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) RETURN DISTINCT f.label`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Errorf("expected 1 distinct label, got %d", len(result.Rows))
	}
	if result.Rows[0]["f.label"] != "Function" {
		t.Errorf("expected 'Function', got %v", result.Rows[0]["f.label"])
	}
}

func TestExecuteInlinePropertyFilter(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function {name: "SubmitOrder"}) RETURN f.name, f.qualified_name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0]["f.name"] != "SubmitOrder" {
		t.Errorf("expected SubmitOrder, got %v", result.Rows[0]["f.name"])
	}
}

func TestExecuteNoResults(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) WHERE f.name = "NonExistent" RETURN f.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(result.Rows))
	}
}

func TestParseError(t *testing.T) {
	_, err := Parse(`NOT A VALID QUERY`)
	if err == nil {
		t.Error("expected parse error for invalid query")
	}
}

// --- Edge property tests (Feature 2) ---

func setupTestStoreWithHTTPCalls(t *testing.T) *store.Store {
	t.Helper()
	s := setupTestStore(t)

	// Add HTTP_CALLS edge with confidence
	callerNode, _ := s.FindNodeByQN("test", "test.main.HandleOrder")
	targetNode, _ := s.FindNodeByQN("test", "test.service.SubmitOrder")
	if callerNode == nil || targetNode == nil {
		t.Fatal("expected test nodes to exist")
	}
	mustInsertEdge(t, s, &store.Edge{
		Project:  "test",
		SourceID: callerNode.ID,
		TargetID: targetNode.ID,
		Type:     "HTTP_CALLS",
		Properties: map[string]any{
			"url_path":   "/api/orders",
			"confidence": 0.85,
			"method":     "POST",
		},
	})
	return s
}

func TestExecuteEdgePropertyAccess(t *testing.T) {
	s := setupTestStoreWithHTTPCalls(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (a:Function)-[r:HTTP_CALLS]->(b:Function) RETURN a.name, b.name, r.url_path, r.confidence`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	row := result.Rows[0]
	if row["a.name"] != "HandleOrder" {
		t.Errorf("a.name = %v, want HandleOrder", row["a.name"])
	}
	if row["b.name"] != "SubmitOrder" {
		t.Errorf("b.name = %v, want SubmitOrder", row["b.name"])
	}
	if row["r.url_path"] != "/api/orders" {
		t.Errorf("r.url_path = %v, want /api/orders", row["r.url_path"])
	}
	conf, ok := row["r.confidence"].(float64)
	if !ok {
		t.Errorf("r.confidence type = %T, want float64", row["r.confidence"])
	} else if conf != 0.85 {
		t.Errorf("r.confidence = %v, want 0.85", conf)
	}
}

func TestExecuteEdgePropertyInWhere(t *testing.T) {
	s := setupTestStoreWithHTTPCalls(t)
	defer s.Close()

	exec := &Executor{Store: s}
	// Filter by confidence > 0.8
	result, err := exec.Execute(`MATCH (a)-[r:HTTP_CALLS]->(b) WHERE r.confidence > 0.8 RETURN a.name, b.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	// Filter by confidence > 0.9 — should return nothing
	result2, err := exec.Execute(`MATCH (a)-[r:HTTP_CALLS]->(b) WHERE r.confidence > 0.9 RETURN a.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result2.Rows) != 0 {
		t.Errorf("expected 0 rows for confidence > 0.9, got %d", len(result2.Rows))
	}
}

func TestExecuteEdgeType(t *testing.T) {
	s := setupTestStoreWithHTTPCalls(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (a)-[r:HTTP_CALLS]->(b) RETURN r.type`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0]["r.type"] != "HTTP_CALLS" {
		t.Errorf("r.type = %v, want HTTP_CALLS", result.Rows[0]["r.type"])
	}
}

// --- Comprehensive edge property filtering tests ---

// setupTestStoreMultiEdge creates a store with two HTTP_CALLS edges to test filtering.
func setupTestStoreMultiEdge(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}

	project := "testproj"
	if err := s.UpsertProject(project, "/tmp/test"); err != nil {
		t.Fatal(err)
	}

	srcID, _ := s.UpsertNode(&store.Node{
		Project: project, Label: "Function", Name: "SendOrder",
		QualifiedName: "testproj.caller.SendOrder",
		FilePath:      "caller/client.go",
	})

	tgtID, _ := s.UpsertNode(&store.Node{
		Project: project, Label: "Function", Name: "HandleOrder",
		QualifiedName: "testproj.handler.HandleOrder",
		FilePath:      "handler/routes.go",
	})

	tgt2ID, _ := s.UpsertNode(&store.Node{
		Project: project, Label: "Function", Name: "HandleHealth",
		QualifiedName: "testproj.handler.HandleHealth",
		FilePath:      "handler/health.go",
	})

	mustInsertEdge(t, s, &store.Edge{
		Project: project, SourceID: srcID, TargetID: tgtID,
		Type: "HTTP_CALLS",
		Properties: map[string]any{
			"url_path":   "/api/orders",
			"confidence": 0.85,
			"method":     "POST",
		},
	})

	mustInsertEdge(t, s, &store.Edge{
		Project: project, SourceID: srcID, TargetID: tgt2ID,
		Type: "HTTP_CALLS",
		Properties: map[string]any{
			"url_path":   "/health",
			"confidence": 0.45,
		},
	})

	return s
}

func TestEdgePropertyFilterContains(t *testing.T) {
	s := setupTestStoreMultiEdge(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (a)-[r:HTTP_CALLS]->(b) WHERE r.url_path CONTAINS 'orders' RETURN a.name, b.name, r.url_path`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	row := result.Rows[0]
	if row["a.name"] != "SendOrder" {
		t.Errorf("a.name = %v, want SendOrder", row["a.name"])
	}
	if row["b.name"] != "HandleOrder" {
		t.Errorf("b.name = %v, want HandleOrder", row["b.name"])
	}
	if row["r.url_path"] != "/api/orders" {
		t.Errorf("r.url_path = %v, want /api/orders", row["r.url_path"])
	}
}

func TestEdgePropertyFilterNumericGTE(t *testing.T) {
	s := setupTestStoreMultiEdge(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (a)-[r:HTTP_CALLS]->(b) WHERE r.confidence >= 0.6 RETURN a.name, b.name, r.confidence LIMIT 20`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row (only high-confidence edge), got %d", len(result.Rows))
	}

	row := result.Rows[0]
	if row["b.name"] != "HandleOrder" {
		t.Errorf("b.name = %v, want HandleOrder (high confidence)", row["b.name"])
	}
}

func TestEdgePropertyReturnWithoutFilter(t *testing.T) {
	s := setupTestStoreMultiEdge(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (a)-[r:HTTP_CALLS]->(b) RETURN a.name, b.name, r.url_path, r.confidence LIMIT 20`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(result.Rows) < 2 {
		t.Fatalf("expected at least 2 rows, got %d", len(result.Rows))
	}

	foundOrders := false
	foundHealth := false
	for _, row := range result.Rows {
		urlPath, _ := row["r.url_path"].(string)
		if urlPath == "/api/orders" {
			foundOrders = true
		}
		if urlPath == "/health" {
			foundHealth = true
		}
	}
	if !foundOrders {
		t.Error("missing row with url_path=/api/orders")
	}
	if !foundHealth {
		t.Error("missing row with url_path=/health")
	}
}

func TestEdgePropertyFilterEquals(t *testing.T) {
	s := setupTestStoreMultiEdge(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (a)-[r:HTTP_CALLS]->(b) WHERE r.method = 'POST' RETURN a.name, b.name`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0]["b.name"] != "HandleOrder" {
		t.Errorf("b.name = %v, want HandleOrder", result.Rows[0]["b.name"])
	}
}

func TestEdgePropertyFilterStartsWith(t *testing.T) {
	s := setupTestStoreMultiEdge(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (a)-[r:HTTP_CALLS]->(b) WHERE r.url_path STARTS WITH '/api' RETURN a.name, b.name, r.url_path`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row (only /api/orders starts with /api), got %d", len(result.Rows))
	}
	if result.Rows[0]["r.url_path"] != "/api/orders" {
		t.Errorf("r.url_path = %v, want /api/orders", result.Rows[0]["r.url_path"])
	}
}

func TestCombinedNodeAndEdgeFilter(t *testing.T) {
	s := setupTestStoreMultiEdge(t)
	defer s.Close()

	exec := &Executor{Store: s}
	// Filter on both node property (early) and edge property (late)
	result, err := exec.Execute(`MATCH (a:Function)-[r:HTTP_CALLS]->(b:Function) WHERE a.name = 'SendOrder' AND r.confidence >= 0.6 RETURN b.name, r.url_path`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0]["b.name"] != "HandleOrder" {
		t.Errorf("b.name = %v, want HandleOrder", result.Rows[0]["b.name"])
	}
	if result.Rows[0]["r.url_path"] != "/api/orders" {
		t.Errorf("r.url_path = %v, want /api/orders", result.Rows[0]["r.url_path"])
	}
}

func TestEdgePropertyFilterNoMatch(t *testing.T) {
	s := setupTestStoreMultiEdge(t)
	defer s.Close()

	exec := &Executor{Store: s}
	// No edge has method = 'DELETE'
	result, err := exec.Execute(`MATCH (a)-[r:HTTP_CALLS]->(b) WHERE r.method = 'DELETE' RETURN a.name`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(result.Rows))
	}
}

func TestEdgePropertyFilterNumericLT(t *testing.T) {
	s := setupTestStoreMultiEdge(t)
	defer s.Close()

	exec := &Executor{Store: s}
	// Only the health edge (0.45) should match confidence < 0.5
	result, err := exec.Execute(`MATCH (a)-[r:HTTP_CALLS]->(b) WHERE r.confidence < 0.5 RETURN b.name, r.confidence`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0]["b.name"] != "HandleHealth" {
		t.Errorf("b.name = %v, want HandleHealth", result.Rows[0]["b.name"])
	}
}

func TestEdgePropertyFilterRegex(t *testing.T) {
	s := setupTestStoreMultiEdge(t)
	defer s.Close()

	exec := &Executor{Store: s}
	// Regex match on url_path
	result, err := exec.Execute(`MATCH (a)-[r:HTTP_CALLS]->(b) WHERE r.url_path =~ "/api/.*" RETURN b.name`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0]["b.name"] != "HandleOrder" {
		t.Errorf("b.name = %v, want HandleOrder", result.Rows[0]["b.name"])
	}
}

func TestEdgeBuiltinPropertyFilter(t *testing.T) {
	s := setupTestStoreMultiEdge(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (a)-[r]->(b) WHERE r.type = 'HTTP_CALLS' RETURN a.name, b.name LIMIT 20`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows (both HTTP_CALLS edges), got %d", len(result.Rows))
	}
}

// --- Token.String() tests ---

func TestTokenString(t *testing.T) {
	tok := Token{Type: TokMatch, Value: "MATCH", Pos: 0}
	s := tok.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
	if s != `Token(0, "MATCH", pos=0)` {
		t.Errorf("Token.String() = %q", s)
	}
}

func TestTokenString_Ident(t *testing.T) {
	tok := Token{Type: TokIdent, Value: "myVar", Pos: 5}
	s := tok.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
	if !strings.Contains(s, "myVar") {
		t.Errorf("Token.String() should contain value, got %q", s)
	}
	if !strings.Contains(s, "pos=5") {
		t.Errorf("Token.String() should contain position, got %q", s)
	}
}

// --- Lexer additional tests ---

func TestLexLineComment(t *testing.T) {
	tokens, err := Lex("// this is a comment\nMATCH (f) RETURN f")
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	if tokens[0].Type != TokMatch {
		t.Errorf("expected first real token to be MATCH, got type %d (%q)", tokens[0].Type, tokens[0].Value)
	}
}

func TestLexBlockComment(t *testing.T) {
	tokens, err := Lex("/* block comment */ MATCH (f) RETURN f")
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	if tokens[0].Type != TokMatch {
		t.Errorf("expected first real token to be MATCH, got type %d (%q)", tokens[0].Type, tokens[0].Value)
	}
}

func TestLexUnterminatedString(t *testing.T) {
	_, err := Lex(`"unterminated`)
	if err == nil {
		t.Error("expected error for unterminated string")
	}
}

func TestLexUnexpectedChar(t *testing.T) {
	_, err := Lex(`MATCH (f) @ RETURN f`)
	if err == nil {
		t.Error("expected error for @ character")
	}
}

func TestLexEscapedString(t *testing.T) {
	tokens, err := Lex(`"hello \"world\""`)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	strTok := tokens[0]
	if strTok.Type != TokString {
		t.Fatalf("expected TokString, got %d", strTok.Type)
	}
	if strTok.Value != `hello "world"` {
		t.Errorf("expected escaped value, got %q", strTok.Value)
	}
}

func TestLexSingleQuoteString(t *testing.T) {
	tokens, err := Lex(`'single quoted'`)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	if tokens[0].Type != TokString || tokens[0].Value != "single quoted" {
		t.Errorf("expected 'single quoted', got %q (type %d)", tokens[0].Value, tokens[0].Type)
	}
}

func TestLexComparisonOperators(t *testing.T) {
	tokens, err := Lex(">= <= > < =")
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	expected := []TokenType{TokGTE, TokLTE, TokGT, TokLT, TokEQ, TokEOF}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, tok := range tokens {
		if tok.Type != expected[i] {
			t.Errorf("token[%d]: expected type %d, got %d (%q)", i, expected[i], tok.Type, tok.Value)
		}
	}
}

func TestLexDecimalNumber(t *testing.T) {
	tokens, err := Lex(`3.14`)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	if tokens[0].Type != TokNumber || tokens[0].Value != "3.14" {
		t.Errorf("expected number 3.14, got %q (type %d)", tokens[0].Value, tokens[0].Type)
	}
}

func TestLexAllSingleCharTokens(t *testing.T) {
	tokens, err := Lex(`( ) [ ] { } * , | : -`)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	expected := []TokenType{
		TokLParen, TokRParen, TokLBracket, TokRBracket,
		TokLBrace, TokRBrace, TokStar, TokComma, TokPipe, TokColon, TokDash,
		TokEOF,
	}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, tok := range tokens {
		if tok.Type != expected[i] {
			t.Errorf("token[%d]: expected %d, got %d (%q)", i, expected[i], tok.Type, tok.Value)
		}
	}
}

func TestLexAllKeywords(t *testing.T) {
	input := "MATCH WHERE RETURN ORDER BY LIMIT AND OR AS DISTINCT COUNT CONTAINS STARTS WITH NOT ASC DESC"
	tokens, err := Lex(input)
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	expected := []TokenType{
		TokMatch, TokWhere, TokReturn, TokOrder, TokBy, TokLimit,
		TokAnd, TokOr, TokAs, TokDistinct, TokCount, TokContains,
		TokStarts, TokWith, TokNot, TokAsc, TokDesc, TokEOF,
	}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, tok := range tokens {
		if tok.Type != expected[i] {
			t.Errorf("token[%d]: expected %d, got %d (%q)", i, expected[i], tok.Type, tok.Value)
		}
	}
}

func TestLexEmptyInput(t *testing.T) {
	tokens, err := Lex("")
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	if len(tokens) != 1 || tokens[0].Type != TokEOF {
		t.Errorf("expected single EOF token, got %d tokens", len(tokens))
	}
}

// --- Plan step type tests ---

func TestPlanStepTypes(t *testing.T) {
	scan := &ScanNodes{Variable: "f", Label: "Function"}
	if scan.stepType() != "scan" {
		t.Errorf("ScanNodes.stepType() = %q, want scan", scan.stepType())
	}

	expand := &ExpandRelationship{FromVar: "f", ToVar: "g"}
	if expand.stepType() != "expand" {
		t.Errorf("ExpandRelationship.stepType() = %q, want expand", expand.stepType())
	}

	filter := &FilterWhere{Operator: "AND"}
	if filter.stepType() != "filter" {
		t.Errorf("FilterWhere.stepType() = %q, want filter", filter.stepType())
	}

	fused := &fusedExpandMarker{}
	if fused.stepType() != "fused" {
		t.Errorf("fusedExpandMarker.stepType() = %q, want fused", fused.stepType())
	}
}

// --- AST pattern element type assertion tests ---

func TestAsNodePattern_Success(t *testing.T) {
	np := &NodePattern{Variable: "f", Label: "Function"}
	result, err := asNodePattern(np)
	if err != nil {
		t.Fatalf("asNodePattern: %v", err)
	}
	if result.Variable != "f" {
		t.Errorf("variable = %q, want f", result.Variable)
	}
}

func TestAsNodePattern_Failure(t *testing.T) {
	rp := &RelPattern{Variable: "r"}
	_, err := asNodePattern(rp)
	if err == nil {
		t.Error("expected error for RelPattern passed to asNodePattern")
	}
}

func TestAsRelPattern_Success(t *testing.T) {
	rp := &RelPattern{Variable: "r", Types: []string{"CALLS"}}
	result, err := asRelPattern(rp)
	if err != nil {
		t.Fatalf("asRelPattern: %v", err)
	}
	if result.Variable != "r" {
		t.Errorf("variable = %q, want r", result.Variable)
	}
}

func TestAsRelPattern_Failure(t *testing.T) {
	np := &NodePattern{Variable: "f"}
	_, err := asRelPattern(np)
	if err == nil {
		t.Error("expected error for NodePattern passed to asRelPattern")
	}
}

// --- Aggregation function tests ---

func TestSplitAggregateItems(t *testing.T) {
	items := []ReturnItem{
		{Variable: "f", Property: "name"},
		{Variable: "g", Func: "COUNT", Alias: "cnt"},
		{Variable: "f", Property: "label"},
	}
	groupItems, countItem := splitAggregateItems(items)
	if len(groupItems) != 2 {
		t.Fatalf("expected 2 group items, got %d", len(groupItems))
	}
	if countItem.Func != "COUNT" {
		t.Errorf("expected COUNT item, got %q", countItem.Func)
	}
	if countItem.Alias != "cnt" {
		t.Errorf("countItem.Alias = %q, want cnt", countItem.Alias)
	}
}

func TestSplitAggregateItems_NoCount(t *testing.T) {
	items := []ReturnItem{
		{Variable: "f", Property: "name"},
	}
	groupItems, countItem := splitAggregateItems(items)
	if len(groupItems) != 1 {
		t.Errorf("expected 1 group item, got %d", len(groupItems))
	}
	if countItem.Func != "" {
		t.Errorf("expected empty Func, got %q", countItem.Func)
	}
}

func TestBuildGroups(t *testing.T) {
	node1 := &store.Node{Name: "A", Label: "Function"}
	node2 := &store.Node{Name: "B", Label: "Function"}
	bindings := []binding{
		{nodes: map[string]*store.Node{"f": node1}, edges: map[string]*store.Edge{}},
		{nodes: map[string]*store.Node{"f": node1}, edges: map[string]*store.Edge{}},
		{nodes: map[string]*store.Node{"f": node2}, edges: map[string]*store.Edge{}},
	}
	groupItems := []ReturnItem{{Variable: "f", Property: "name"}}
	groups, order := buildGroups(bindings, groupItems)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if len(order) != 2 {
		t.Fatalf("expected 2 order entries, got %d", len(order))
	}
	g1 := groups[order[0]]
	if g1.count != 2 {
		t.Errorf("first group count = %d, want 2", g1.count)
	}
	g2 := groups[order[1]]
	if g2.count != 1 {
		t.Errorf("second group count = %d, want 1", g2.count)
	}
}

func TestBuildGroups_Empty(t *testing.T) {
	groups, order := buildGroups(nil, nil)
	if len(groups) != 0 || len(order) != 0 {
		t.Errorf("expected empty groups from nil bindings, got %d groups, %d order", len(groups), len(order))
	}
}

func TestBuildGroups_WithAlias(t *testing.T) {
	node := &store.Node{Name: "A", FilePath: "main.go"}
	bindings := []binding{
		{nodes: map[string]*store.Node{"f": node}, edges: map[string]*store.Edge{}},
	}
	groupItems := []ReturnItem{{Variable: "f", Property: "name", Alias: "fn"}}
	groups, order := buildGroups(bindings, groupItems)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[order[0]]
	if g.row["fn"] != "A" {
		t.Errorf("expected row[fn] = A, got %v", g.row["fn"])
	}
}

// --- appendFilterConditions tests ---

func TestAppendFilterConditions(t *testing.T) {
	filter := &FilterWhere{
		Conditions: []Condition{
			{Variable: "f", Property: "name", Operator: "=", Value: "Foo"},
			{Variable: "g", Property: "name", Operator: "CONTAINS", Value: "Bar"},
			{Variable: "f", Property: "file_path", Operator: "STARTS WITH", Value: "src/"},
		},
	}
	var sb strings.Builder
	args := make([]any, 0)
	appendFilterConditions(&sb, &args, filter, "f", "src", "tgt")
	got := sb.String()
	if got == "" {
		t.Fatal("expected non-empty SQL conditions")
	}
	if len(args) != 3 {
		t.Errorf("expected 3 args, got %d", len(args))
	}
	if args[0] != "Foo" {
		t.Errorf("args[0] = %v, want Foo", args[0])
	}
	if args[1] != "%Bar%" {
		t.Errorf("args[1] = %v, want %%Bar%%", args[1])
	}
	if args[2] != "src/%" {
		t.Errorf("args[2] = %v, want src/%%", args[2])
	}
}

// --- Default projection tests ---

func TestDefaultProjection_EmptyBindings(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) WHERE f.name = "NonExistent" RETURN f.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(result.Rows))
	}
}

func TestDefaultProjection_NoReturn(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function)`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) == 0 {
		t.Error("expected rows for MATCH without RETURN")
	}
	for _, col := range result.Columns {
		if col == "" {
			t.Error("empty column name in default projection")
		}
	}
}

// --- Compare and sort tests ---

func TestCompareValues(t *testing.T) {
	tests := []struct {
		a, b any
		want int
	}{
		{1, 2, -1},
		{2, 1, 1},
		{1, 1, 0},
		{1.5, 2.5, -1},
		{int64(3), int64(1), 1},
		{"a", "b", -1},
		{"b", "a", 1},
		{"same", "same", 0},
	}
	for _, tt := range tests {
		got := compareValues(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("compareValues(%v, %v) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSortRows(t *testing.T) {
	rows := []map[string]any{
		{"name": "C", "count": 1},
		{"name": "A", "count": 3},
		{"name": "B", "count": 2},
	}
	sortRows(rows, "name", "ASC")
	if rows[0]["name"] != "A" || rows[1]["name"] != "B" || rows[2]["name"] != "C" {
		t.Errorf("ASC sort failed: %v", rows)
	}

	sortRows(rows, "count", "DESC")
	if rows[0]["count"] != 3 || rows[1]["count"] != 2 || rows[2]["count"] != 1 {
		t.Errorf("DESC sort failed: %v", rows)
	}
}

func TestApplyLimit(t *testing.T) {
	rows := make([]map[string]any, 10)
	for i := range rows {
		rows[i] = map[string]any{"i": i}
	}
	limited := applyLimit(rows, 3)
	if len(limited) != 3 {
		t.Errorf("expected 3 rows, got %d", len(limited))
	}

	unlimited := applyLimit(rows, 0)
	if len(unlimited) != 10 {
		t.Errorf("expected 10 rows (0 means default cap), got %d", len(unlimited))
	}
}

// --- ResolveOrderColumn tests ---

func TestResolveOrderColumn(t *testing.T) {
	items := []ReturnItem{
		{Variable: "f", Property: "name"},
		{Variable: "g", Func: "COUNT", Alias: "cnt"},
	}
	cols := buildColumnNames(items)

	t.Run("alias", func(t *testing.T) {
		got := resolveOrderColumn("cnt", items, cols)
		if got != "cnt" {
			t.Errorf("resolveOrderColumn(cnt) = %q, want cnt", got)
		}
	})
	t.Run("count_expr", func(t *testing.T) {
		got := resolveOrderColumn("COUNT(g)", items, cols)
		if got != "cnt" {
			t.Errorf("resolveOrderColumn(COUNT(g)) = %q, want cnt", got)
		}
	})
	t.Run("var_prop", func(t *testing.T) {
		got := resolveOrderColumn("f.name", items, cols)
		if got != "f.name" {
			t.Errorf("resolveOrderColumn(f.name) = %q, want f.name", got)
		}
	})
	t.Run("fallback", func(t *testing.T) {
		got := resolveOrderColumn("unknown", items, cols)
		if got != "unknown" {
			t.Errorf("resolveOrderColumn(unknown) = %q, want unknown", got)
		}
	})
}

// --- BuildColumnNames tests ---

func TestBuildColumnNames(t *testing.T) {
	items := []ReturnItem{
		{Variable: "f", Property: "name"},
		{Variable: "g"},
		{Variable: "f", Property: "label", Alias: "lbl"},
	}
	cols := buildColumnNames(items)
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
	if cols[0] != "f.name" {
		t.Errorf("cols[0] = %q, want f.name", cols[0])
	}
	if cols[1] != "g" {
		t.Errorf("cols[1] = %q, want g", cols[1])
	}
	if cols[2] != "lbl" {
		t.Errorf("cols[2] = %q, want lbl", cols[2])
	}
}

// --- Node/Edge property accessor tests ---

func TestGetNodeProperty(t *testing.T) {
	n := &store.Node{
		ID: 1, Project: "proj", Label: "Function", Name: "Foo",
		QualifiedName: "proj.Foo", FilePath: "main.go",
		StartLine: 10, EndLine: 20,
		Properties: map[string]any{"signature": "func()"},
	}

	tests := []struct {
		prop string
		want any
	}{
		{"name", "Foo"},
		{"qualified_name", "proj.Foo"},
		{"label", "Function"},
		{"file_path", "main.go"},
		{"start_line", 10},
		{"end_line", 20},
		{"id", int64(1)},
		{"project", "proj"},
		{"signature", "func()"},
		{"nonexistent", nil},
	}
	for _, tt := range tests {
		got := getNodeProperty(n, tt.prop)
		if got != tt.want {
			t.Errorf("getNodeProperty(%q) = %v (%T), want %v (%T)", tt.prop, got, got, tt.want, tt.want)
		}
	}
}

func TestGetEdgeProperty(t *testing.T) {
	e := &store.Edge{
		ID: 5, Type: "CALLS", SourceID: 1, TargetID: 2,
		Properties: map[string]any{"weight": 3.14},
	}

	tests := []struct {
		prop string
		want any
	}{
		{"type", "CALLS"},
		{"id", int64(5)},
		{"source_id", int64(1)},
		{"target_id", int64(2)},
		{"weight", 3.14},
		{"nonexistent", nil},
	}
	for _, tt := range tests {
		got := getEdgeProperty(e, tt.prop)
		if got != tt.want {
			t.Errorf("getEdgeProperty(%q) = %v (%T), want %v (%T)", tt.prop, got, got, tt.want, tt.want)
		}
	}
}

func TestGetNodeProperty_NilProperties(t *testing.T) {
	n := &store.Node{Name: "Foo"}
	got := getNodeProperty(n, "custom_prop")
	if got != nil {
		t.Errorf("expected nil for custom prop on node with nil properties, got %v", got)
	}
}

func TestGetEdgeProperty_NilProperties(t *testing.T) {
	e := &store.Edge{Type: "CALLS"}
	got := getEdgeProperty(e, "custom_prop")
	if got != nil {
		t.Errorf("expected nil for custom prop on edge with nil properties, got %v", got)
	}
}

// --- CompareNumeric tests ---

func TestCompareNumeric(t *testing.T) {
	tests := []struct {
		actual   any
		expected string
		op       string
		want     bool
	}{
		{10, "5", ">", true},
		{3, "5", "<", true},
		{5, "5", ">=", true},
		{5, "5", "<=", true},
		{int64(10), "5", ">", true},
		{float64(3.5), "3.0", ">", true},
		{"7", "5", ">", true},
	}
	for _, tt := range tests {
		got, err := compareNumeric(tt.actual, tt.expected, tt.op)
		if err != nil {
			t.Errorf("compareNumeric(%v, %q, %q) error: %v", tt.actual, tt.expected, tt.op, err)
			continue
		}
		if got != tt.want {
			t.Errorf("compareNumeric(%v, %q, %q) = %v, want %v", tt.actual, tt.expected, tt.op, got, tt.want)
		}
	}
}

func TestCompareNumeric_InvalidExpected(t *testing.T) {
	_, err := compareNumeric(10, "abc", ">")
	if err == nil {
		t.Error("expected error for non-numeric expected value")
	}
}

func TestCompareNumeric_InvalidActual(t *testing.T) {
	got, err := compareNumeric("abc", "5", ">")
	if err == nil {
		t.Logf("compareNumeric returned %v (string parse fail)", got)
	}
}

func TestCompareNumeric_NonNumericType(t *testing.T) {
	got, err := compareNumeric(true, "5", ">")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != false {
		t.Errorf("expected false for non-numeric type, got true")
	}
}

func TestCompareNumeric_DefaultOp(t *testing.T) {
	got, err := compareNumeric(5, "5", "==")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != false {
		t.Errorf("expected false for unknown op, got true")
	}
}

// --- ToFloat tests ---

func TestToFloat(t *testing.T) {
	tests := []struct {
		input any
		want  float64
		ok    bool
	}{
		{42, 42.0, true},
		{int64(100), 100.0, true},
		{3.14, 3.14, true},
		{"string", 0, false},
		{nil, 0, false},
		{true, 0, false},
	}
	for _, tt := range tests {
		got, ok := toFloat(tt.input)
		if ok != tt.ok {
			t.Errorf("toFloat(%v) ok = %v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Errorf("toFloat(%v) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

// --- CopyBinding tests ---

func TestCopyBinding(t *testing.T) {
	node := &store.Node{Name: "A"}
	edge := &store.Edge{Type: "CALLS"}
	b := binding{
		nodes: map[string]*store.Node{"f": node},
		edges: map[string]*store.Edge{"r": edge},
	}
	c := copyBinding(b)
	if c.nodes["f"] != node {
		t.Error("expected copied node reference")
	}
	if c.edges["r"] != edge {
		t.Error("expected copied edge reference")
	}
	c.nodes["g"] = &store.Node{Name: "B"}
	if b.nodes["g"] != nil {
		t.Error("mutation on copy affected original")
	}
}

// --- NodeMatchesProps tests ---

func TestNodeMatchesProps(t *testing.T) {
	n := &store.Node{Name: "Foo", Label: "Function", FilePath: "main.go"}
	if !nodeMatchesProps(n, map[string]string{"name": "Foo"}) {
		t.Error("expected match")
	}
	if nodeMatchesProps(n, map[string]string{"name": "Bar"}) {
		t.Error("expected no match")
	}
	if !nodeMatchesProps(n, map[string]string{"name": "Foo", "label": "Function"}) {
		t.Error("expected match for multiple props")
	}
	if nodeMatchesProps(n, map[string]string{"name": "Foo", "label": "Class"}) {
		t.Error("expected no match when one prop differs")
	}
}

// --- FilterNodesByProps tests ---

func TestFilterNodesByProps(t *testing.T) {
	nodes := []*store.Node{
		{Name: "A", Label: "Function"},
		{Name: "B", Label: "Class"},
		{Name: "C", Label: "Function"},
	}
	filtered := filterNodesByProps(nodes, map[string]string{"label": "Function"})
	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered nodes, got %d", len(filtered))
	}
}

// --- ParseWhereOr test ---

func TestParseWhereOr(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) WHERE f.name = "Foo" OR f.name = "Bar" RETURN f`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if q.Where.Operator != "OR" {
		t.Errorf("expected OR, got %q", q.Where.Operator)
	}
	if len(q.Where.Conditions) != 2 {
		t.Errorf("expected 2 conditions, got %d", len(q.Where.Conditions))
	}
}

// --- Execute OR filter test ---

func TestExecuteWhereOr(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function)-[:CALLS]->(g:Function) WHERE g.name = "ValidateOrder" OR g.name = "LogError" RETURN f.name, g.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result.Rows))
	}
}

// --- EdgeTargetID tests ---

func TestEdgeTargetID(t *testing.T) {
	edge := &store.Edge{SourceID: 1, TargetID: 2}

	if edgeTargetID(edge, 0, "outbound") != 2 {
		t.Error("outbound should return TargetID")
	}
	if edgeTargetID(edge, 0, "inbound") != 1 {
		t.Error("inbound should return SourceID")
	}
	if edgeTargetID(edge, 1, "any") != 2 {
		t.Error("any with nodeID=source should return TargetID")
	}
	if edgeTargetID(edge, 2, "any") != 1 {
		t.Error("any with nodeID=target should return SourceID")
	}
}

// --- Planner BuildPlan tests ---

func TestBuildPlan_ScanOnly(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) RETURN f.name`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	plan, err := BuildPlan(q)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].stepType() != "scan" {
		t.Errorf("step[0] type = %q, want scan", plan.Steps[0].stepType())
	}
}

func TestBuildPlan_ScanExpandFilter(t *testing.T) {
	q, err := Parse(`MATCH (f:Function)-[:CALLS]->(g:Function) WHERE g.name = "Foo" RETURN f.name, g.name`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	plan, err := BuildPlan(q)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Steps) < 2 {
		t.Fatalf("expected at least 2 steps, got %d", len(plan.Steps))
	}
	scanFound := false
	expandFound := false
	for _, step := range plan.Steps {
		switch step.stepType() {
		case "scan":
			scanFound = true
		case "expand":
			expandFound = true
		}
	}
	if !scanFound {
		t.Error("expected scan step")
	}
	if !expandFound {
		t.Error("expected expand step")
	}
}

func TestBuildPlan_EarlyFilter(t *testing.T) {
	q, err := Parse(`MATCH (f:Function)-[:CALLS]->(g:Function) WHERE f.name = "HandleOrder" AND g.name = "Foo" RETURN g.name`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	plan, err := BuildPlan(q)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Steps[0].stepType() != "scan" {
		t.Errorf("step[0] should be scan, got %q", plan.Steps[0].stepType())
	}
	if plan.Steps[1].stepType() != "filter" {
		t.Errorf("step[1] should be early filter, got %q", plan.Steps[1].stepType())
	}
}

// --- ClassifyCountItems tests ---

func TestClassifyCountItems(t *testing.T) {
	items := []ReturnItem{
		{Variable: "f", Property: "name"},
		{Variable: "g", Func: "COUNT"},
	}
	countItem, groupItems := classifyCountItems(items)
	if countItem == nil {
		t.Fatal("expected count item")
	}
	if len(groupItems) != 1 {
		t.Errorf("expected 1 group item, got %d", len(groupItems))
	}
}

func TestClassifyCountItems_NoCount(t *testing.T) {
	items := []ReturnItem{
		{Variable: "f", Property: "name"},
	}
	countItem, groupItems := classifyCountItems(items)
	if countItem != nil {
		t.Error("expected nil count item")
	}
	if len(groupItems) != 1 {
		t.Errorf("expected 1 group item, got %d", len(groupItems))
	}
}

// --- CanFuseJoin tests ---

func TestCanFuseJoin(t *testing.T) {
	scan := &ScanNodes{Variable: "f", Label: "Function"}
	expand := &ExpandRelationship{
		MinHops: 1, MaxHops: 1,
		EdgeTypes: []string{"CALLS"},
		Direction: "outbound",
	}
	if !canFuseJoin(scan, expand) {
		t.Error("expected fusible")
	}
}

func TestCanFuseJoin_VariableLength(t *testing.T) {
	scan := &ScanNodes{Variable: "f"}
	expand := &ExpandRelationship{
		MinHops: 1, MaxHops: 3,
		EdgeTypes: []string{"CALLS"},
	}
	if canFuseJoin(scan, expand) {
		t.Error("expected non-fusible for variable-length")
	}
}

func TestCanFuseJoin_NoEdgeTypes(t *testing.T) {
	scan := &ScanNodes{Variable: "f"}
	expand := &ExpandRelationship{MinHops: 1, MaxHops: 1}
	if canFuseJoin(scan, expand) {
		t.Error("expected non-fusible for no edge types")
	}
}

func TestCanFuseJoin_ScanWithProps(t *testing.T) {
	scan := &ScanNodes{Variable: "f", Props: map[string]string{"name": "Foo"}}
	expand := &ExpandRelationship{
		MinHops: 1, MaxHops: 1,
		EdgeTypes: []string{"CALLS"},
	}
	if canFuseJoin(scan, expand) {
		t.Error("expected non-fusible for scan with props")
	}
}

// --- Aggregate COUNT query end-to-end ---

func TestExecuteCountWithAlias(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function)-[:CALLS]->(g:Function) RETURN f.name, COUNT(g) AS cnt`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) == 0 {
		t.Fatal("expected rows for count query")
	}
	found := false
	for _, col := range result.Columns {
		if col == "cnt" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected cnt column, got %v", result.Columns)
	}
	for _, row := range result.Rows {
		if row["f.name"] == "HandleOrder" {
			cnt, ok := row["cnt"].(int)
			if !ok {
				t.Errorf("cnt type = %T, want int", row["cnt"])
			} else if cnt != 2 {
				t.Errorf("cnt = %d, want 2", cnt)
			}
		}
	}
}

// --- Numeric WHERE comparison tests ---

func TestExecuteWhereLTE(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) WHERE f.start_line <= 5 RETURN f.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) < 1 {
		t.Error("expected at least 1 row for start_line <= 5")
	}
}

func TestExecuteWhereGTE(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) WHERE f.start_line >= 25 RETURN f.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Errorf("expected 1 row (SubmitOrder), got %d", len(result.Rows))
	}
}

func TestExecuteWhereLT(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) WHERE f.start_line < 5 RETURN f.name`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Errorf("expected 1 row (LogError at line 1), got %d", len(result.Rows))
	}
}

// --- Resolve item value tests ---

func TestResolveItemValue_Node(t *testing.T) {
	b := newBinding()
	b.nodes["f"] = &store.Node{Name: "Foo", Label: "Function"}
	val := resolveItemValue(b, ReturnItem{Variable: "f", Property: "name"})
	if val != "Foo" {
		t.Errorf("expected Foo, got %v", val)
	}
}

func TestResolveItemValue_Edge(t *testing.T) {
	b := newBinding()
	b.edges["r"] = &store.Edge{Type: "CALLS"}
	val := resolveItemValue(b, ReturnItem{Variable: "r", Property: "type"})
	if val != "CALLS" {
		t.Errorf("expected CALLS, got %v", val)
	}
}

func TestResolveItemValue_NodeWithoutProperty(t *testing.T) {
	b := newBinding()
	b.nodes["f"] = &store.Node{Name: "Foo", Label: "Function", QualifiedName: "proj.Foo", FilePath: "main.go", StartLine: 1, EndLine: 10}
	val := resolveItemValue(b, ReturnItem{Variable: "f"})
	m, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("expected map for whole node, got %T", val)
	}
	if m["name"] != "Foo" {
		t.Errorf("expected name=Foo, got %v", m["name"])
	}
}

func TestResolveItemValue_EdgeWithoutProperty(t *testing.T) {
	b := newBinding()
	b.edges["r"] = &store.Edge{Type: "CALLS", SourceID: 1, TargetID: 2}
	val := resolveItemValue(b, ReturnItem{Variable: "r"})
	m, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("expected map for whole edge, got %T", val)
	}
	if m["type"] != "CALLS" {
		t.Errorf("expected type=CALLS, got %v", m["type"])
	}
}

func TestResolveItemValue_Missing(t *testing.T) {
	b := newBinding()
	val := resolveItemValue(b, ReturnItem{Variable: "x", Property: "name"})
	if val != nil {
		t.Errorf("expected nil for missing variable, got %v", val)
	}
}

// --- ParseFusibleSteps tests ---

func TestParseFusibleSteps_TwoSteps(t *testing.T) {
	steps := []PlanStep{
		&ScanNodes{Variable: "f"},
		&ExpandRelationship{FromVar: "f", ToVar: "g"},
	}
	scan, expand, filter, ok := parseFusibleSteps(steps)
	if !ok {
		t.Fatal("expected fusible")
	}
	if scan == nil || expand == nil {
		t.Fatal("expected non-nil scan and expand")
	}
	if filter != nil {
		t.Error("expected nil filter for 2-step pattern")
	}
}

func TestParseFusibleSteps_ThreeSteps(t *testing.T) {
	steps := []PlanStep{
		&ScanNodes{Variable: "f"},
		&FilterWhere{Conditions: []Condition{{Variable: "f", Property: "name", Operator: "=", Value: "Foo"}}},
		&ExpandRelationship{FromVar: "f", ToVar: "g"},
	}
	scan, expand, filter, ok := parseFusibleSteps(steps)
	if !ok {
		t.Fatal("expected fusible")
	}
	if scan == nil || expand == nil {
		t.Fatal("expected non-nil scan and expand")
	}
	if filter == nil {
		t.Error("expected non-nil filter for 3-step pattern")
	}
}

func TestParseFusibleSteps_OneStep(t *testing.T) {
	steps := []PlanStep{&ScanNodes{Variable: "f"}}
	_, _, _, ok := parseFusibleSteps(steps)
	if ok {
		t.Error("expected non-fusible for 1 step")
	}
}

func TestParseFusibleSteps_WrongTypes(t *testing.T) {
	steps := []PlanStep{
		&FilterWhere{},
		&ScanNodes{},
	}
	_, _, _, ok := parseFusibleSteps(steps)
	if ok {
		t.Error("expected non-fusible for wrong step order")
	}
}

// --- ValidatePushability tests ---

func TestValidatePushability_Success(t *testing.T) {
	scan := &ScanNodes{Variable: "f"}
	expand := &ExpandRelationship{MinHops: 1, MaxHops: 1, EdgeTypes: []string{"CALLS"}}
	groupItems := []ReturnItem{{Variable: "f", Property: "name"}}
	if !validatePushability(scan, expand, nil, groupItems) {
		t.Error("expected pushable")
	}
}

func TestValidatePushability_VariableLength(t *testing.T) {
	scan := &ScanNodes{}
	expand := &ExpandRelationship{MinHops: 1, MaxHops: 3, EdgeTypes: []string{"CALLS"}}
	if validatePushability(scan, expand, nil, nil) {
		t.Error("expected non-pushable for variable-length")
	}
}

func TestValidatePushability_NoEdgeTypes(t *testing.T) {
	scan := &ScanNodes{}
	expand := &ExpandRelationship{MinHops: 1, MaxHops: 1}
	if validatePushability(scan, expand, nil, nil) {
		t.Error("expected non-pushable for no edge types")
	}
}

func TestValidatePushability_NonPushableGroupProp(t *testing.T) {
	scan := &ScanNodes{}
	expand := &ExpandRelationship{MinHops: 1, MaxHops: 1, EdgeTypes: []string{"CALLS"}}
	groupItems := []ReturnItem{{Variable: "f", Property: "start_line"}}
	if validatePushability(scan, expand, nil, groupItems) {
		t.Error("expected non-pushable for non-SQL-column property")
	}
}

func TestValidatePushability_GroupWithoutProperty(t *testing.T) {
	scan := &ScanNodes{}
	expand := &ExpandRelationship{MinHops: 1, MaxHops: 1, EdgeTypes: []string{"CALLS"}}
	groupItems := []ReturnItem{{Variable: "f"}}
	if validatePushability(scan, expand, nil, groupItems) {
		t.Error("expected non-pushable when group item has no property")
	}
}

func TestValidatePushability_ScanWithProps(t *testing.T) {
	scan := &ScanNodes{Props: map[string]string{"name": "Foo"}}
	expand := &ExpandRelationship{MinHops: 1, MaxHops: 1, EdgeTypes: []string{"CALLS"}}
	if validatePushability(scan, expand, nil, nil) {
		t.Error("expected non-pushable for scan with props")
	}
}

func TestValidatePushability_ExpandWithToProps(t *testing.T) {
	scan := &ScanNodes{}
	expand := &ExpandRelationship{
		MinHops: 1, MaxHops: 1, EdgeTypes: []string{"CALLS"},
		ToProps: map[string]string{"name": "Foo"},
	}
	if validatePushability(scan, expand, nil, nil) {
		t.Error("expected non-pushable for expand with ToProps")
	}
}

func TestValidatePushability_UnsupportedFilterOp(t *testing.T) {
	scan := &ScanNodes{}
	expand := &ExpandRelationship{MinHops: 1, MaxHops: 1, EdgeTypes: []string{"CALLS"}}
	filter := &FilterWhere{
		Conditions: []Condition{{Variable: "f", Property: "name", Operator: "=~", Value: ".*"}},
	}
	if validatePushability(scan, expand, filter, nil) {
		t.Error("expected non-pushable for regex filter operator")
	}
}

func TestParseVariableLengthHopRange(t *testing.T) {
	tests := []struct {
		query   string
		minHops int
		maxHops int
	}{
		{`MATCH (a)-[:CALLS*1..3]->(b) RETURN a.name`, 1, 3},
		{`MATCH (a)-[:CALLS*..5]->(b) RETURN a.name`, 1, 5},
		{`MATCH (a)-[:CALLS*2..]->(b) RETURN a.name`, 2, 0},
		{`MATCH (a)-[:CALLS*3]->(b) RETURN a.name`, 1, 3},
		{`MATCH (a)-[:CALLS*]->(b) RETURN a.name`, 1, 0},
	}
	for _, tt := range tests {
		q, err := Parse(tt.query)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.query, err)
		}
		if len(q.Match.Pattern.Elements) < 2 {
			t.Fatalf("expected at least 2 elements for %q", tt.query)
		}
		rel, ok := q.Match.Pattern.Elements[1].(*RelPattern)
		if !ok {
			t.Fatalf("expected RelPattern for %q", tt.query)
		}
		if rel.MinHops != tt.minHops {
			t.Errorf("%q: MinHops = %d, want %d", tt.query, rel.MinHops, tt.minHops)
		}
		if rel.MaxHops != tt.maxHops {
			t.Errorf("%q: MaxHops = %d, want %d", tt.query, rel.MaxHops, tt.maxHops)
		}
	}
}

func TestParseReturnItemWithAlias(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) RETURN f.name AS func_name`)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Return.Items) != 1 {
		t.Fatalf("expected 1 return item, got %d", len(q.Return.Items))
	}
	item := q.Return.Items[0]
	if item.Variable != "f" {
		t.Errorf("Variable = %q, want f", item.Variable)
	}
	if item.Property != "name" {
		t.Errorf("Property = %q, want name", item.Property)
	}
	if item.Alias != "func_name" {
		t.Errorf("Alias = %q, want func_name", item.Alias)
	}
}

func TestParseReturnVariable(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) RETURN f`)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Return.Items) != 1 {
		t.Fatalf("expected 1 return item, got %d", len(q.Return.Items))
	}
	item := q.Return.Items[0]
	if item.Variable != "f" {
		t.Errorf("Variable = %q, want f", item.Variable)
	}
	if item.Property != "" {
		t.Errorf("Property = %q, want empty", item.Property)
	}
}

func TestParseOrderByCountDesc(t *testing.T) {
	q, err := Parse(`MATCH (f:Function)-[:CALLS]->(g:Function) RETURN f.name, COUNT(g) AS cnt ORDER BY COUNT(g) DESC`)
	if err != nil {
		t.Fatal(err)
	}
	if q.Return.OrderBy != "COUNT(g)" {
		t.Errorf("OrderBy = %q, want COUNT(g)", q.Return.OrderBy)
	}
	if q.Return.OrderDir != "DESC" {
		t.Errorf("OrderDir = %q, want DESC", q.Return.OrderDir)
	}
}

func TestParseOrderByFieldAsc(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) RETURN f.name ORDER BY f.name ASC`)
	if err != nil {
		t.Fatal(err)
	}
	if q.Return.OrderBy != "f.name" {
		t.Errorf("OrderBy = %q, want f.name", q.Return.OrderBy)
	}
	if q.Return.OrderDir != "ASC" {
		t.Errorf("OrderDir = %q, want ASC", q.Return.OrderDir)
	}
}

func TestParseInlineProps(t *testing.T) {
	q, err := Parse(`MATCH (f:Function {name: "Foo"}) RETURN f.name`)
	if err != nil {
		t.Fatal(err)
	}
	node := q.Match.Pattern.Elements[0].(*NodePattern)
	if node.Props == nil {
		t.Fatal("expected inline props")
	}
	if node.Props["name"] != "Foo" {
		t.Errorf("props[name] = %v, want Foo", node.Props["name"])
	}
}

func TestParseMultipleReturnItems(t *testing.T) {
	q, err := Parse(`MATCH (f:Function)-[:CALLS]->(g:Function) RETURN f.name, g.name, g.file_path`)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Return.Items) != 3 {
		t.Fatalf("expected 3 return items, got %d", len(q.Return.Items))
	}
}

func TestParseLimit(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) RETURN f.name LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	if q.Return.Limit != 10 {
		t.Errorf("Limit = %d, want 10", q.Return.Limit)
	}
}

func TestParseContainsOperator(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) WHERE f.name CONTAINS "handler" RETURN f.name`)
	if err != nil {
		t.Fatal(err)
	}
	if q.Where == nil || len(q.Where.Conditions) != 1 {
		t.Fatalf("expected 1 where condition, got %v", q.Where)
	}
	if q.Where.Conditions[0].Operator != "CONTAINS" {
		t.Errorf("Operator = %q, want CONTAINS", q.Where.Conditions[0].Operator)
	}
}

func TestParseStartsWithOperator(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) WHERE f.name STARTS WITH "Test" RETURN f.name`)
	if err != nil {
		t.Fatal(err)
	}
	if q.Where == nil || len(q.Where.Conditions) < 1 {
		t.Fatal("expected WHERE clause with conditions")
	}
	if q.Where.Conditions[0].Operator != "STARTS WITH" {
		t.Errorf("Operator = %q, want STARTS WITH", q.Where.Conditions[0].Operator)
	}
}

func TestExecuteContains_Extra(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) WHERE f.name CONTAINS "Order" RETURN f.name`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) == 0 {
		t.Error("expected results for CONTAINS 'Order'")
	}
	for _, row := range result.Rows {
		name := row["f.name"].(string)
		if !strings.Contains(name, "Order") {
			t.Errorf("expected name containing 'Order', got %q", name)
		}
	}
}

func TestExecuteStartsWith_Extra(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) WHERE f.name STARTS WITH "Create" RETURN f.name`)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("STARTS WITH 'Create': %d results", len(result.Rows))
}

func TestExecuteOrderByNameDesc_Extra(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) RETURN f.name ORDER BY f.name DESC`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) < 2 {
		t.Skip("need at least 2 rows for order test")
	}
	for i := 1; i < len(result.Rows); i++ {
		a := result.Rows[i-1]["f.name"].(string)
		b := result.Rows[i]["f.name"].(string)
		if a < b {
			t.Errorf("expected descending order, got %q before %q", a, b)
		}
	}
}

func TestExecuteVariableLengthPath_Extra(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function)-[:CALLS*1..2]->(g:Function) RETURN f.name, g.name LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("variable length path results: %d rows", len(result.Rows))
}

func TestExecuteLeftArrow_Extra(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function)<-[:CALLS]-(g:Function) RETURN f.name, g.name LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("left arrow results: %d rows", len(result.Rows))
}

func TestExecuteRegexMatch_Extra(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) WHERE f.name =~ ".*Order.*" RETURN f.name`)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range result.Rows {
		name := row["f.name"].(string)
		if !strings.Contains(name, "Order") {
			t.Errorf("regex match returned %q which doesn't contain 'Order'", name)
		}
	}
}

func TestExecuteNodePatternOnly_Extra(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function) RETURN f.name LIMIT 3`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) == 0 {
		t.Error("expected at least 1 row for simple scan")
	}
	if len(result.Rows) > 3 {
		t.Errorf("expected at most 3 rows (LIMIT), got %d", len(result.Rows))
	}
}

func TestExecuteCountWithGroupBy_Extra(t *testing.T) {
	s := setupTestStore(t)
	defer s.Close()

	exec := &Executor{Store: s}
	result, err := exec.Execute(`MATCH (f:Function)-[:CALLS]->(g:Function) RETURN f.name, COUNT(g) AS cnt ORDER BY COUNT(g) DESC LIMIT 5`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Columns) < 2 {
		t.Errorf("expected at least 2 columns, got %d: %v", len(result.Columns), result.Columns)
	}
	t.Logf("count with group-by: %d rows", len(result.Rows))
}

func TestEvaluateCondition_UnsupportedOperator(t *testing.T) {
	e := &Executor{}
	b := binding{
		nodes: map[string]*store.Node{
			"f": {Properties: map[string]any{"name": "Foo"}},
		},
	}
	_, err := e.evaluateCondition(b, Condition{
		Variable: "f", Property: "name", Operator: "LIKE", Value: "%Foo%",
	})
	if err == nil {
		t.Error("expected error for unsupported operator LIKE")
	}
}

func TestEvaluateCondition_MissingVariable(t *testing.T) {
	e := &Executor{}
	b := binding{
		nodes: map[string]*store.Node{},
		edges: map[string]*store.Edge{},
	}
	result, err := e.evaluateCondition(b, Condition{
		Variable: "missing", Property: "name", Operator: "=", Value: "x",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result {
		t.Error("expected false for missing variable")
	}
}

func TestEvaluateCondition_RegexNonString(t *testing.T) {
	e := &Executor{}
	b := binding{
		nodes: map[string]*store.Node{
			"f": {Properties: map[string]any{"count": 42}},
		},
	}
	result, err := e.evaluateCondition(b, Condition{
		Variable: "f", Property: "count", Operator: "=~", Value: ".*",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result {
		t.Error("expected false for regex on non-string")
	}
}

func TestEvaluateCondition_ContainsNonString(t *testing.T) {
	e := &Executor{}
	b := binding{
		nodes: map[string]*store.Node{
			"f": {Properties: map[string]any{"count": 42}},
		},
	}
	result, err := e.evaluateCondition(b, Condition{
		Variable: "f", Property: "count", Operator: "CONTAINS", Value: "4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result {
		t.Error("expected false for CONTAINS on non-string")
	}
}

func TestEvaluateCondition_StartsWithNonString(t *testing.T) {
	e := &Executor{}
	b := binding{
		nodes: map[string]*store.Node{
			"f": {Properties: map[string]any{"count": 42}},
		},
	}
	result, err := e.evaluateCondition(b, Condition{
		Variable: "f", Property: "count", Operator: "STARTS WITH", Value: "4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result {
		t.Error("expected false for STARTS WITH on non-string")
	}
}

func TestAggregateResults(t *testing.T) {
	e := &Executor{}
	bindings := []binding{
		{nodes: map[string]*store.Node{
			"f": {Name: "A", Properties: map[string]any{"name": "A"}},
			"g": {Name: "X", Properties: map[string]any{"name": "X"}},
		}},
		{nodes: map[string]*store.Node{
			"f": {Name: "A", Properties: map[string]any{"name": "A"}},
			"g": {Name: "Y", Properties: map[string]any{"name": "Y"}},
		}},
		{nodes: map[string]*store.Node{
			"f": {Name: "B", Properties: map[string]any{"name": "B"}},
			"g": {Name: "Z", Properties: map[string]any{"name": "Z"}},
		}},
	}
	ret := &ReturnClause{
		Items: []ReturnItem{
			{Variable: "f", Property: "name"},
			{Variable: "g", Func: "COUNT", Alias: "cnt"},
		},
	}
	result, err := e.aggregateResults(bindings, ret)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result.Rows))
	}
	for _, row := range result.Rows {
		name := row["f.name"]
		cnt := row["cnt"].(int)
		if name == "A" && cnt != 2 {
			t.Errorf("expected count 2 for A, got %d", cnt)
		}
		if name == "B" && cnt != 1 {
			t.Errorf("expected count 1 for B, got %d", cnt)
		}
	}
}

func TestAggregateResults_WithOrderAndLimit(t *testing.T) {
	e := &Executor{}
	bindings := []binding{
		{nodes: map[string]*store.Node{
			"f": {Name: "A", Properties: map[string]any{"name": "A"}},
			"g": {Name: "X"},
		}},
		{nodes: map[string]*store.Node{
			"f": {Name: "A", Properties: map[string]any{"name": "A"}},
			"g": {Name: "Y"},
		}},
		{nodes: map[string]*store.Node{
			"f": {Name: "A", Properties: map[string]any{"name": "A"}},
			"g": {Name: "Z"},
		}},
		{nodes: map[string]*store.Node{
			"f": {Name: "B", Properties: map[string]any{"name": "B"}},
			"g": {Name: "W"},
		}},
	}
	ret := &ReturnClause{
		Items: []ReturnItem{
			{Variable: "f", Property: "name"},
			{Variable: "g", Func: "COUNT", Alias: "cnt"},
		},
		OrderBy:  "cnt",
		OrderDir: "DESC",
		Limit:    1,
	}
	result, err := e.aggregateResults(bindings, ret)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row after LIMIT, got %d", len(result.Rows))
	}
	if result.Rows[0]["f.name"] != "A" {
		t.Errorf("expected A (highest count), got %v", result.Rows[0]["f.name"])
	}
}

func TestCompiledRegex_Cache(t *testing.T) {
	e := &Executor{}
	re1, err := e.compiledRegex(".*Test.*")
	if err != nil {
		t.Fatal(err)
	}
	re2, err := e.compiledRegex(".*Test.*")
	if err != nil {
		t.Fatal(err)
	}
	if re1 != re2 {
		t.Error("expected same regex pointer from cache")
	}
}

func TestCompiledRegex_Invalid(t *testing.T) {
	e := &Executor{}
	_, err := e.compiledRegex("[invalid")
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestParseWhereOR(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) WHERE f.name = "A" OR f.name = "B" RETURN f.name`)
	if err != nil {
		t.Fatal(err)
	}
	if q.Where == nil {
		t.Fatal("expected WHERE clause")
	}
	if q.Where.Operator != "OR" {
		t.Errorf("expected OR operator, got %q", q.Where.Operator)
	}
	if len(q.Where.Conditions) != 2 {
		t.Errorf("expected 2 OR conditions, got %d", len(q.Where.Conditions))
	}
}

func TestParseMultipleWhereConditions(t *testing.T) {
	q, err := Parse(`MATCH (f:Function) WHERE f.name = "Foo" AND f.file_path = "main.go" RETURN f.name`)
	if err != nil {
		t.Fatal(err)
	}
	if q.Where == nil {
		t.Fatal("expected WHERE clause")
	}
	if len(q.Where.Conditions) != 2 {
		t.Errorf("expected 2 AND conditions, got %d", len(q.Where.Conditions))
	}
	if q.Where.Operator != "AND" {
		t.Errorf("expected AND operator, got %q", q.Where.Operator)
	}
}

func TestParseRelWithoutType(t *testing.T) {
	q, err := Parse(`MATCH (a)-[r]->(b) RETURN a.name, b.name`)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Match.Pattern.Elements) < 2 {
		t.Fatal("expected at least 2 elements")
	}
	rel, ok := q.Match.Pattern.Elements[1].(*RelPattern)
	if !ok {
		t.Fatal("expected RelPattern")
	}
	if rel.Variable != "r" {
		t.Errorf("rel variable = %q, want r", rel.Variable)
	}
}

func TestDefaultProjection_WithEdge(t *testing.T) {
	bindings := []binding{
		{
			nodes: map[string]*store.Node{},
			edges: map[string]*store.Edge{
				"r": {Type: "CALLS", Properties: map[string]any{"weight": 1}},
			},
		},
	}
	e := &Executor{}
	result, err := e.defaultProjection(bindings)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0]["r.type"] != "CALLS" {
		t.Errorf("expected r.type=CALLS, got %v", result.Rows[0]["r.type"])
	}
}
