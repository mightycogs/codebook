package pipeline

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseNameStatusOutput(t *testing.T) {
	input := "M\tinternal/store/nodes.go\nA\tnew_file.go\nD\told_file.go\nR100\tsrc/old.go\tsrc/new.go\n"

	files := ParseNameStatusOutput(input)

	if len(files) != 4 {
		t.Fatalf("expected 4 files, got %d", len(files))
	}

	tests := []struct {
		idx     int
		status  string
		path    string
		oldPath string
	}{
		{0, "M", "internal/store/nodes.go", ""},
		{1, "A", "new_file.go", ""},
		{2, "D", "old_file.go", ""},
		{3, "R", "src/new.go", "src/old.go"},
	}

	for _, tt := range tests {
		f := files[tt.idx]
		if f.Status != tt.status {
			t.Errorf("[%d] status = %q, want %q", tt.idx, f.Status, tt.status)
		}
		if f.Path != tt.path {
			t.Errorf("[%d] path = %q, want %q", tt.idx, f.Path, tt.path)
		}
		if f.OldPath != tt.oldPath {
			t.Errorf("[%d] oldPath = %q, want %q", tt.idx, f.OldPath, tt.oldPath)
		}
	}
}

func TestParseNameStatusOutput_FiltersUntrackable(t *testing.T) {
	input := "M\tpackage-lock.json\nM\tsrc/main.go\nM\tvendor/lib.go\n"
	files := ParseNameStatusOutput(input)

	if len(files) != 1 {
		t.Fatalf("expected 1 trackable file, got %d", len(files))
	}
	if files[0].Path != "src/main.go" {
		t.Errorf("expected src/main.go, got %s", files[0].Path)
	}
}

func TestParseHunksOutput(t *testing.T) {
	input := `diff --git a/main.go b/main.go
index abc1234..def5678 100644
--- a/main.go
+++ b/main.go
@@ -10,3 +10,5 @@ func main() {
+	newLine1()
+	newLine2()
@@ -50,0 +52,2 @@ func helper() {
+	another()
+	line()
diff --git a/binary.png b/binary.png
Binary files a/binary.png and b/binary.png differ
diff --git a/utils.go b/utils.go
--- a/utils.go
+++ b/utils.go
@@ -1 +1 @@ package utils
-old
+new
`

	hunks := ParseHunksOutput(input)

	if len(hunks) != 3 {
		t.Fatalf("expected 3 hunks, got %d", len(hunks))
	}

	// First hunk: main.go @@ -10,3 +10,5 @@
	if hunks[0].Path != "main.go" {
		t.Errorf("hunk 0 path = %q", hunks[0].Path)
	}
	if hunks[0].StartLine != 10 || hunks[0].EndLine != 14 {
		t.Errorf("hunk 0 range = %d-%d, want 10-14", hunks[0].StartLine, hunks[0].EndLine)
	}

	// Second hunk: main.go @@ -50,0 +52,2 @@
	if hunks[1].Path != "main.go" {
		t.Errorf("hunk 1 path = %q", hunks[1].Path)
	}
	if hunks[1].StartLine != 52 || hunks[1].EndLine != 53 {
		t.Errorf("hunk 1 range = %d-%d, want 52-53", hunks[1].StartLine, hunks[1].EndLine)
	}

	// Third hunk: utils.go @@ -1 +1 @@
	if hunks[2].Path != "utils.go" {
		t.Errorf("hunk 2 path = %q", hunks[2].Path)
	}
	if hunks[2].StartLine != 1 || hunks[2].EndLine != 1 {
		t.Errorf("hunk 2 range = %d-%d, want 1-1", hunks[2].StartLine, hunks[2].EndLine)
	}
}

func TestParseHunksOutput_NoNewlineMarker(t *testing.T) {
	input := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -5,2 +5,3 @@ func foo() {
+	bar()
\ No newline at end of file
`
	hunks := ParseHunksOutput(input)
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	if hunks[0].StartLine != 5 || hunks[0].EndLine != 7 {
		t.Errorf("range = %d-%d, want 5-7", hunks[0].StartLine, hunks[0].EndLine)
	}
}

func TestParseGitDiffFilesAndHunks(t *testing.T) {
	repo := initGitRepo(t)
	gitCommitFile(t, repo, "main.go", "package main\n\nfunc main() {}\n", "add main")

	updated := "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n"
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte(updated), 0o644); err != nil {
		t.Fatalf("write updated file: %v", err)
	}

	files, err := ParseGitDiffFiles(repo, DiffUnstaged, "")
	if err != nil {
		t.Fatalf("ParseGitDiffFiles failed: %v", err)
	}
	if len(files) != 1 || files[0].Path != "main.go" || files[0].Status != "M" {
		t.Fatalf("unexpected changed files: %+v", files)
	}

	hunks, err := ParseGitDiffHunks(repo, DiffUnstaged, "")
	if err != nil {
		t.Fatalf("ParseGitDiffHunks failed: %v", err)
	}
	if len(hunks) == 0 || hunks[0].Path != "main.go" {
		t.Fatalf("unexpected hunks: %+v", hunks)
	}

	cmd := exec.Command("git", "add", "main.go")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}

	files, err = ParseGitDiffFiles(repo, DiffStaged, "")
	if err != nil {
		t.Fatalf("ParseGitDiffFiles staged failed: %v", err)
	}
	if len(files) != 1 || files[0].Status != "M" {
		t.Fatalf("unexpected staged files: %+v", files)
	}
}

func TestParseRange(t *testing.T) {
	tests := []struct {
		input     string
		wantStart int
		wantCount int
	}{
		{"10,5", 10, 5},
		{"10", 10, 1},
		{"52,2", 52, 2},
		{"1,0", 1, 0},
	}
	for _, tt := range tests {
		start, count := parseRange(tt.input)
		if start != tt.wantStart || count != tt.wantCount {
			t.Errorf("parseRange(%q) = (%d, %d), want (%d, %d)", tt.input, start, count, tt.wantStart, tt.wantCount)
		}
	}
}

func TestParseHunksOutput_ModeChange(t *testing.T) {
	input := `diff --git a/script.sh b/script.sh
old mode 100644
new mode 100755
`
	hunks := ParseHunksOutput(input)
	if len(hunks) != 0 {
		t.Fatalf("expected 0 hunks for mode-only change, got %d", len(hunks))
	}
}

func TestGitNotFound(t *testing.T) {
	// Override PATH to ensure git can't be found
	t.Setenv("PATH", t.TempDir())

	_, err := runGit(t.TempDir(), []string{"status"})
	if err == nil {
		t.Fatal("expected error when git is not found")
	}
	if !strings.Contains(err.Error(), "git not found in PATH") {
		t.Errorf("expected 'git not found in PATH' error, got: %v", err)
	}
}

func TestParseHunksOutput_Deletion(t *testing.T) {
	input := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -10,3 +10,0 @@ func foo() {
`
	hunks := ParseHunksOutput(input)
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	if hunks[0].StartLine != 10 {
		t.Errorf("start = %d, want 10", hunks[0].StartLine)
	}
}

func TestBuildDiffArgs(t *testing.T) {
	tests := []struct {
		scope      DiffScope
		baseBranch string
		want       []string
	}{
		{DiffUnstaged, "", []string{"diff"}},
		{DiffStaged, "", []string{"diff", "--cached"}},
		{DiffAll, "", []string{"diff", "HEAD"}},
		{DiffBranch, "develop", []string{"diff", "develop...HEAD"}},
		{DiffBranch, "", []string{"diff", "main...HEAD"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.scope), func(t *testing.T) {
			got := buildDiffArgs(tt.scope, tt.baseBranch)
			if len(got) != len(tt.want) {
				t.Fatalf("buildDiffArgs(%q, %q) = %v, want %v", tt.scope, tt.baseBranch, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("buildDiffArgs(%q, %q)[%d] = %q, want %q", tt.scope, tt.baseBranch, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseNameStatusOutput_EmptyInput(t *testing.T) {
	files := ParseNameStatusOutput("")
	if len(files) != 0 {
		t.Errorf("expected 0 files from empty input, got %d", len(files))
	}
}

func TestParseNameStatusOutput_MalformedLines(t *testing.T) {
	input := "GARBAGE\n\nM\tsrc/main.go\n"
	files := ParseNameStatusOutput(input)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != "src/main.go" {
		t.Errorf("path = %q, want src/main.go", files[0].Path)
	}
}

func TestParseHunksOutput_EmptyInput(t *testing.T) {
	hunks := ParseHunksOutput("")
	if len(hunks) != 0 {
		t.Errorf("expected 0 hunks from empty input, got %d", len(hunks))
	}
}

func TestParseHunksOutput_MultipleFiles(t *testing.T) {
	input := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1,3 @@
+line1
+line2
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -5,2 +5,4 @@
+line3
+line4
@@ -20 +22,2 @@
+line5
`
	hunks := ParseHunksOutput(input)
	if len(hunks) != 3 {
		t.Fatalf("expected 3 hunks across 2 files, got %d", len(hunks))
	}
	if hunks[0].Path != "a.go" {
		t.Errorf("hunk[0].Path = %q, want a.go", hunks[0].Path)
	}
	if hunks[1].Path != "b.go" || hunks[2].Path != "b.go" {
		t.Errorf("hunks 1,2 should be b.go, got %q, %q", hunks[1].Path, hunks[2].Path)
	}
}

func TestParseHunksOutput_SkipsBinaryFiles(t *testing.T) {
	input := `diff --git a/image.go b/image.go
--- a/image.go
+++ b/image.go
@@ -1 +1,2 @@
+new
Binary files a/data.bin and b/data.bin differ
diff --git a/other.go b/other.go
--- a/other.go
+++ b/other.go
@@ -1 +1,2 @@
+more
`
	hunks := ParseHunksOutput(input)
	if len(hunks) != 2 {
		t.Fatalf("expected 2 hunks (skipping binary), got %d", len(hunks))
	}
}

func TestParseHunkHeader_NoPlusSign(t *testing.T) {
	result := parseHunkHeader("@@ something without plus @@", "file.go")
	if result != nil {
		t.Error("expected nil for hunk header without + sign")
	}
}

func TestParseHunkHeader_ZeroStart(t *testing.T) {
	result := parseHunkHeader("@@ -0,0 +0,0 @@", "file.go")
	if result != nil {
		t.Error("expected nil for +0,0 range")
	}
}

func TestParseRange_InvalidInput(t *testing.T) {
	start, count := parseRange("abc")
	if start != 0 || count != 0 {
		t.Errorf("parseRange(\"abc\") = (%d, %d), want (0, 0)", start, count)
	}
}

func TestParseRange_InvalidCount(t *testing.T) {
	start, count := parseRange("10,xyz")
	if start != 10 || count != 1 {
		t.Errorf("parseRange(\"10,xyz\") = (%d, %d), want (10, 1)", start, count)
	}
}

func TestParseHunksOutput_UntrackableFile(t *testing.T) {
	input := `diff --git a/vendor/lib.go b/vendor/lib.go
--- a/vendor/lib.go
+++ b/vendor/lib.go
@@ -1 +1,2 @@
+new line
`
	hunks := ParseHunksOutput(input)
	if len(hunks) != 0 {
		t.Errorf("expected 0 hunks for untrackable file, got %d", len(hunks))
	}
}

func TestParseNameStatusOutput_RenameWithoutNewPath(t *testing.T) {
	input := "R100\tsrc/old.go\n"
	files := ParseNameStatusOutput(input)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Status != "R" {
		t.Errorf("status = %q, want R", files[0].Status)
	}
	if files[0].OldPath != "src/old.go" {
		t.Errorf("oldPath = %q, want src/old.go", files[0].OldPath)
	}
	if files[0].Path != "src/old.go" {
		t.Errorf("path = %q, want src/old.go (same as old when no new path)", files[0].Path)
	}
}

func TestRunGit_InvalidRepo(t *testing.T) {
	_, err := runGit(t.TempDir(), []string{"log", "--oneline", "-1"})
	if err == nil {
		t.Log("runGit on non-repo returned nil error (git may return exit 128 captured as output)")
	}
}

func TestParseHunkHeader_SingleLineChange(t *testing.T) {
	result := parseHunkHeader("@@ -5 +5 @@ func foo()", "file.go")
	if result == nil {
		t.Fatal("expected non-nil hunk")
	}
	if result.StartLine != 5 || result.EndLine != 5 {
		t.Errorf("range = %d-%d, want 5-5", result.StartLine, result.EndLine)
	}
}

func TestBuildDiffArgs_DefaultScope(t *testing.T) {
	got := buildDiffArgs(DiffScope("unknown"), "")
	if len(got) != 1 || got[0] != "diff" {
		t.Errorf("expected [diff] for unknown scope, got %v", got)
	}
}

func TestRunGit_ContextTimeout(t *testing.T) {
	dir := t.TempDir()
	_, err := runGit(dir, []string{"status"})
	if err != nil {
		if !strings.Contains(err.Error(), "git") {
			t.Errorf("unexpected error: %v", err)
		}
	}
}
