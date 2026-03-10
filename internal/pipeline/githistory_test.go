package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mightycogs/codebase-memory-mcp/internal/store"
)

func TestIsTrackableFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"src/app.py", true},
		{"node_modules/foo/bar.js", false},
		{"vendor/lib/dep.go", false},
		{"package-lock.json", false},
		{"go.sum", false},
		{"image.png", false},
		{".git/config", false},
		{"__pycache__/mod.pyc", false},
		{"src/style.min.css", false},
		{"README.md", true},
	}
	for _, tt := range tests {
		if got := isTrackableFile(tt.path); got != tt.want {
			t.Errorf("isTrackableFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func gitCommitFile(t *testing.T, repo, name, content, message string) {
	t.Helper()

	path := filepath.Join(repo, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	commands := [][]string{
		{"git", "add", name},
		{"git", "commit", "-m", message},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	commands := [][]string{
		{"git", "init"},
		{"git", "config", "user.name", "Test User"},
		{"git", "config", "user.email", "test@example.com"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
	return repo
}

func TestComputeChangeCoupling(t *testing.T) {
	commits := []CommitFiles{
		{Hash: "aaa", Files: []string{"a.go", "b.go", "c.go"}},
		{Hash: "bbb", Files: []string{"a.go", "b.go"}},
		{Hash: "ccc", Files: []string{"a.go", "b.go"}},
		{Hash: "ddd", Files: []string{"a.go", "c.go"}},
		{Hash: "eee", Files: []string{"d.go", "e.go"}},
	}

	couplings := computeChangeCoupling(commits)

	found := false
	for _, c := range couplings {
		if (c.FileA == "a.go" && c.FileB == "b.go") || (c.FileA == "b.go" && c.FileB == "a.go") {
			found = true
			if c.CoChangeCount != 3 {
				t.Errorf("expected 3 co-changes for a.go/b.go, got %d", c.CoChangeCount)
			}
			if c.CouplingScore < 0.9 {
				t.Errorf("expected high coupling score, got %f", c.CouplingScore)
			}
		}
	}
	if !found {
		t.Error("expected coupling between a.go and b.go")
	}

	for _, c := range couplings {
		if c.FileA == "d.go" || c.FileB == "d.go" {
			t.Error("d.go should not appear (below threshold)")
		}
	}
}

func TestParseGitLog(t *testing.T) {
	repo := initGitRepo(t)
	gitCommitFile(t, repo, "a.go", "package main\n", "add a")
	gitCommitFile(t, repo, "b.go", "package main\n", "add b")
	gitCommitFile(t, repo, "package-lock.json", "{}\n", "add lock file")

	commits, err := parseGitLog(repo)
	if err != nil {
		t.Fatalf("parseGitLog failed: %v", err)
	}
	if len(commits) < 2 {
		t.Fatalf("expected at least 2 commits with tracked files, got %d", len(commits))
	}

	foundA := false
	for _, c := range commits {
		for _, f := range c.Files {
			if f == "a.go" {
				foundA = true
			}
			if f == "package-lock.json" {
				t.Fatal("lock file should be filtered from git history")
			}
		}
	}
	if !foundA {
		t.Fatal("expected tracked file a.go in parsed git log")
	}
}

func TestCreateCouplingEdgesAndFindFileNode(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "proj"
	if err := s.UpsertProject(project, "/tmp/proj"); err != nil {
		t.Fatal(err)
	}

	nodeA, err := s.UpsertNode(&store.Node{Project: project, Label: "File", Name: "a.go", QualifiedName: "proj.a", FilePath: "a.go"})
	if err != nil {
		t.Fatal(err)
	}
	nodeB, err := s.UpsertNode(&store.Node{Project: project, Label: "File", Name: "b.go", QualifiedName: "proj.b", FilePath: "b.go"})
	if err != nil {
		t.Fatal(err)
	}

	p := &Pipeline{ctx: context.Background(), Store: s, ProjectName: project}

	if got := p.findFileNode("a.go"); got == nil || got.ID != nodeA {
		t.Fatalf("findFileNode(a.go) = %+v, want ID %d", got, nodeA)
	}

	count := p.createCouplingEdges([]ChangeCoupling{{
		FileA:         "a.go",
		FileB:         "b.go",
		CoChangeCount: 3,
		TotalChangesA: 4,
		TotalChangesB: 5,
		CouplingScore: 0.75,
	}})
	if count != 1 {
		t.Fatalf("expected 1 coupling edge, got %d", count)
	}

	edges, err := s.FindEdgesByType(project, "FILE_CHANGES_WITH")
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 stored edge, got %d", len(edges))
	}
	if edges[0].SourceID != nodeA || edges[0].TargetID != nodeB {
		t.Fatalf("unexpected edge endpoints: %+v", edges[0])
	}
}

func TestFindFileNode_Fallbacks(t *testing.T) {
	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "proj"
	if err := s.UpsertProject(project, "/tmp/proj"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertNode(&store.Node{Project: project, Label: "Module", Name: "a", QualifiedName: "proj.a", FilePath: "a.go"}); err != nil {
		t.Fatal(err)
	}

	p := &Pipeline{ctx: context.Background(), Store: s, ProjectName: project}
	if got := p.findFileNode("missing.go"); got != nil {
		t.Fatalf("expected nil for missing file, got %+v", got)
	}
	if got := p.findFileNode("a.go"); got == nil || got.Label != "Module" {
		t.Fatalf("expected fallback first node for a.go, got %+v", got)
	}
}

func TestPassGitHistory(t *testing.T) {
	repo := initGitRepo(t)
	gitCommitFile(t, repo, "a.go", "package main\n", "add a")
	gitCommitFile(t, repo, "b.go", "package main\n", "add b")

	for i := 0; i < 3; i++ {
		if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte(fmt.Sprintf("package main\n// %d\n", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "b.go"), []byte(fmt.Sprintf("package main\n// %d\n", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "add", "a.go", "b.go")
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git add failed: %v\n%s", err, out)
		}
		cmd = exec.Command("git", "commit", "-m", fmt.Sprintf("pair %d", i))
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit failed: %v\n%s", err, out)
		}
	}

	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "proj"
	if err := s.UpsertProject(project, repo); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertNode(&store.Node{Project: project, Label: "File", Name: "a.go", QualifiedName: "proj.a", FilePath: "a.go"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertNode(&store.Node{Project: project, Label: "File", Name: "b.go", QualifiedName: "proj.b", FilePath: "b.go"}); err != nil {
		t.Fatal(err)
	}

	p := &Pipeline{ctx: context.Background(), Store: s, ProjectName: project, RepoPath: repo}
	p.passGitHistory()

	edges, err := s.FindEdgesByType(project, "FILE_CHANGES_WITH")
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) == 0 {
		t.Fatal("expected coupling edges after passGitHistory")
	}
}

func TestPassGitHistory_NoCommits(t *testing.T) {
	repo := initGitRepo(t)
	gitCommitFile(t, repo, "package-lock.json", "{}\n", "lock only")

	s, err := store.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	project := "proj"
	if err := s.UpsertProject(project, repo); err != nil {
		t.Fatal(err)
	}

	p := &Pipeline{ctx: context.Background(), Store: s, ProjectName: project, RepoPath: repo}
	p.passGitHistory()

	edges, err := s.FindEdgesByType(project, "FILE_CHANGES_WITH")
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 0 {
		t.Fatalf("expected no coupling edges, got %d", len(edges))
	}
}

func TestComputeChangeCouplingSkipsLargeCommits(t *testing.T) {
	files := make([]string, 25)
	for i := range files {
		files[i] = fmt.Sprintf("file%d.go", i)
	}
	commits := []CommitFiles{{Hash: "large", Files: files}}

	couplings := computeChangeCoupling(commits)
	if len(couplings) != 0 {
		t.Errorf("expected 0 couplings from large commit, got %d", len(couplings))
	}
}

func TestComputeChangeCouplingLimitsTo100(t *testing.T) {
	// Create many commits with overlapping files to generate >100 couplings
	var commits []CommitFiles
	for i := 0; i < 50; i++ {
		for j := i + 1; j < 50; j++ {
			// Create 3 commits per pair to exceed threshold
			for k := 0; k < 3; k++ {
				commits = append(commits, CommitFiles{
					Hash:  fmt.Sprintf("c%d_%d_%d", i, j, k),
					Files: []string{fmt.Sprintf("f%d.go", i), fmt.Sprintf("f%d.go", j)},
				})
			}
		}
	}

	couplings := computeChangeCoupling(commits)
	if len(couplings) > 100 {
		t.Errorf("expected max 100 couplings, got %d", len(couplings))
	}
}

func TestComputeChangeCoupling_EmptyCommits(t *testing.T) {
	couplings := computeChangeCoupling(nil)
	if len(couplings) != 0 {
		t.Errorf("expected 0 couplings from nil commits, got %d", len(couplings))
	}
}

func TestComputeChangeCoupling_SingleFileCommits(t *testing.T) {
	commits := []CommitFiles{
		{Hash: "a", Files: []string{"a.go"}},
		{Hash: "b", Files: []string{"a.go"}},
		{Hash: "c", Files: []string{"a.go"}},
	}
	couplings := computeChangeCoupling(commits)
	if len(couplings) != 0 {
		t.Errorf("expected 0 couplings (no pairs), got %d", len(couplings))
	}
}

func TestComputeChangeCoupling_LowScoreFiltered(t *testing.T) {
	commits := []CommitFiles{
		{Hash: "a", Files: []string{"x.go", "y.go"}},
		{Hash: "b", Files: []string{"x.go", "y.go"}},
		{Hash: "c", Files: []string{"x.go", "y.go"}},
		{Hash: "d", Files: []string{"x.go"}},
		{Hash: "e", Files: []string{"x.go"}},
		{Hash: "f", Files: []string{"x.go"}},
		{Hash: "g", Files: []string{"x.go"}},
		{Hash: "h", Files: []string{"x.go"}},
		{Hash: "i", Files: []string{"x.go"}},
		{Hash: "j", Files: []string{"x.go"}},
		{Hash: "k", Files: []string{"x.go"}},
	}
	couplings := computeChangeCoupling(commits)
	for _, c := range couplings {
		if c.CouplingScore < 0.3 {
			t.Errorf("coupling %s/%s has score %f below 0.3 threshold", c.FileA, c.FileB, c.CouplingScore)
		}
	}
}

func TestComputeChangeCoupling_SortedByScore(t *testing.T) {
	var commits []CommitFiles
	for i := 0; i < 5; i++ {
		commits = append(commits, CommitFiles{
			Hash:  fmt.Sprintf("high%d", i),
			Files: []string{"a.go", "b.go"},
		})
	}
	for i := 0; i < 3; i++ {
		commits = append(commits, CommitFiles{
			Hash:  fmt.Sprintf("low%d", i),
			Files: []string{"c.go", "d.go"},
		})
	}
	couplings := computeChangeCoupling(commits)
	for i := 1; i < len(couplings); i++ {
		if couplings[i].CouplingScore > couplings[i-1].CouplingScore {
			t.Errorf("couplings not sorted: [%d].score=%f > [%d].score=%f",
				i, couplings[i].CouplingScore, i-1, couplings[i-1].CouplingScore)
		}
	}
}

func TestComputeChangeCoupling_PairOrdering(t *testing.T) {
	commits := []CommitFiles{
		{Hash: "a", Files: []string{"z.go", "a.go"}},
		{Hash: "b", Files: []string{"z.go", "a.go"}},
		{Hash: "c", Files: []string{"z.go", "a.go"}},
	}
	couplings := computeChangeCoupling(commits)
	if len(couplings) == 0 {
		t.Fatal("expected at least 1 coupling")
	}
	if couplings[0].FileA > couplings[0].FileB {
		t.Errorf("expected FileA <= FileB, got %q > %q", couplings[0].FileA, couplings[0].FileB)
	}
}

func TestComputeChangeCoupling_ExactlyThresholdCount(t *testing.T) {
	commits := []CommitFiles{
		{Hash: "a", Files: []string{"x.go", "y.go"}},
		{Hash: "b", Files: []string{"x.go", "y.go"}},
		{Hash: "c", Files: []string{"x.go", "y.go"}},
	}
	couplings := computeChangeCoupling(commits)
	found := false
	for _, c := range couplings {
		if (c.FileA == "x.go" && c.FileB == "y.go") || (c.FileA == "y.go" && c.FileB == "x.go") {
			found = true
			if c.CoChangeCount != 3 {
				t.Errorf("expected 3 co-changes, got %d", c.CoChangeCount)
			}
		}
	}
	if !found {
		t.Error("expected coupling between x.go and y.go at threshold=3")
	}
}

func TestComputeChangeCoupling_BelowThreshold(t *testing.T) {
	commits := []CommitFiles{
		{Hash: "a", Files: []string{"x.go", "y.go"}},
		{Hash: "b", Files: []string{"x.go", "y.go"}},
	}
	couplings := computeChangeCoupling(commits)
	if len(couplings) != 0 {
		t.Errorf("expected 0 couplings with only 2 co-changes, got %d", len(couplings))
	}
}

func TestComputeChangeCoupling_Exactly20Files(t *testing.T) {
	files := make([]string, 20)
	for i := range files {
		files[i] = fmt.Sprintf("f%d.go", i)
	}
	commits := []CommitFiles{
		{Hash: "a", Files: files},
		{Hash: "b", Files: files},
		{Hash: "c", Files: files},
	}
	couplings := computeChangeCoupling(commits)
	if len(couplings) == 0 {
		t.Error("expected couplings from 20-file commits (threshold is >20)")
	}
}

func TestIsTrackableFile_Extended(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{".cache/foo.js", false},
		{"Cargo.lock", false},
		{"poetry.lock", false},
		{"composer.lock", false},
		{"Gemfile.lock", false},
		{"Pipfile.lock", false},
		{"pnpm-lock.yaml", false},
		{"yarn.lock", false},
		{"bundle.min.js", false},
		{"style.min.css", false},
		{"source.map", false},
		{"module.wasm", false},
		{"logo.jpg", false},
		{"icon.gif", false},
		{"favicon.ico", false},
		{"drawing.svg", false},
		{"src/deep/nested/file.ts", true},
		{"Makefile", true},
	}
	for _, tt := range tests {
		if got := isTrackableFile(tt.path); got != tt.want {
			t.Errorf("isTrackableFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
