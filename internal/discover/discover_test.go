package discover

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mightycogs/codebook/internal/lang"
)

func TestDiscoverBasic(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte("def main(): pass\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	files, err := Discover(ctx, dir, nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	for _, f := range files {
		if f.Path == "" {
			t.Error("expected non-empty Path")
		}
		if f.RelPath == "" {
			t.Error("expected non-empty RelPath")
		}
		if f.Language == "" {
			t.Error("expected non-empty Language")
		}
	}
}

func TestDisambiguateM(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		content string
		want    lang.Language
	}{
		{"objc", "@interface Foo\n@end\n", lang.ObjectiveC},
		{"matlab_function", "function y = foo(x)\n  y = x^2;\nend\n", lang.MATLAB},
		{"matlab_comment", "% This is MATLAB\nx = 1;\n", lang.MATLAB},
		{"magma_end_function", "function Fact(n)\n  return n;\nend function;\n", lang.Magma},
		{"magma_procedure", "procedure DoStuff(~x)\n  x := 1;\nend procedure;\n", lang.Magma},
		{"magma_intrinsic", "intrinsic IsSmall(x :: RngIntElt) -> BoolElt\n", lang.Magma},
		{"magma_end_if", "if n le 1 then\n  return 1;\nend if;\n", lang.Magma},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name+".m")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, ok := disambiguateM(path)
			if !ok {
				t.Fatal("disambiguateM returned ok=false")
			}
			if got != tt.want {
				t.Errorf("disambiguateM(%s) = %s, want %s", tt.name, got, tt.want)
			}
		})
	}
}

func TestDisambiguateM_NonexistentFile(t *testing.T) {
	got, ok := disambiguateM("/no/such/file.m")
	if !ok {
		t.Fatal("expected ok=true for nonexistent file (defaults to MATLAB)")
	}
	if got != lang.MATLAB {
		t.Errorf("expected MATLAB default, got %s", got)
	}
}

func TestDisambiguateM_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.m")
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := disambiguateM(path)
	if !ok {
		t.Fatal("expected ok=true for empty file")
	}
	if got != lang.MATLAB {
		t.Errorf("expected MATLAB default for empty file, got %s", got)
	}
}

func TestDisambiguateM_NoMarkers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.m")
	if err := os.WriteFile(path, []byte("x = 42;\ny = x + 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := disambiguateM(path)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != lang.MATLAB {
		t.Errorf("expected MATLAB default for ambiguous content, got %s", got)
	}
}

func TestDiscoverCancellation(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Discover(ctx, dir, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestShouldSkipDir(t *testing.T) {
	tests := []struct {
		name        string
		dirName     string
		rel         string
		extraIgnore []string
		mode        IndexMode
		want        bool
	}{
		{"builtin_git", ".git", ".git", nil, ModeFull, true},
		{"builtin_node_modules", "node_modules", "node_modules", nil, ModeFull, true},
		{"builtin_vendor", "vendor", "vendor", nil, ModeFull, true},
		{"builtin_venv", ".venv", ".venv", nil, ModeFull, true},
		{"normal_dir_full", "src", "src", nil, ModeFull, false},
		{"dot_dir_full_not_in_list", ".custom", ".custom", nil, ModeFull, false},
		{"fast_dot_dir", ".custom", ".custom", nil, ModeFast, true},
		{"fast_generated", "generated", "generated", nil, ModeFast, true},
		{"fast_testdata", "testdata", "testdata", nil, ModeFast, true},
		{"fast_docs", "docs", "docs", nil, ModeFast, true},
		{"fast_examples", "examples", "examples", nil, ModeFast, true},
		{"fast_third_party", "third_party", "third_party", nil, ModeFast, true},
		{"fast_normal_dir", "src", "src", nil, ModeFast, false},
		{"extra_ignore_name_match", "mydir", "mydir", []string{"mydir"}, ModeFull, true},
		{"extra_ignore_rel_match", "sub", "path/sub", []string{"path/sub"}, ModeFull, true},
		{"extra_ignore_glob", "foo_gen", "foo_gen", []string{"*_gen"}, ModeFull, true},
		{"extra_ignore_no_match", "src", "src", []string{"other"}, ModeFull, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipDir(tt.dirName, tt.rel, tt.extraIgnore, tt.mode)
			if got != tt.want {
				t.Errorf("shouldSkipDir(%q, %q, %v, %q) = %v, want %v",
					tt.dirName, tt.rel, tt.extraIgnore, tt.mode, got, tt.want)
			}
		})
	}
}

func TestShouldSkipFile(t *testing.T) {
	tests := []struct {
		name string
		file string
		path string
		size int64
		opts *Options
		want bool
	}{
		{"nil_opts", "main.go", "/repo/main.go", 100, nil, false},
		{"full_mode", "main.go", "/repo/main.go", 100, &Options{Mode: ModeFull}, false},
		{"max_file_size_under", "big.go", "/repo/big.go", 999, &Options{MaxFileSize: 1000}, false},
		{"max_file_size_over", "big.go", "/repo/big.go", 1001, &Options{MaxFileSize: 1000}, true},
		{"max_file_size_exact", "big.go", "/repo/big.go", 1000, &Options{MaxFileSize: 1000}, false},
		{"fast_ignored_filename_license", "LICENSE", "/repo/LICENSE", 100, &Options{Mode: ModeFast}, true},
		{"fast_ignored_filename_gosum", "go.sum", "/repo/go.sum", 100, &Options{Mode: ModeFast}, true},
		{"fast_ignored_filename_yarnlock", "yarn.lock", "/repo/yarn.lock", 100, &Options{Mode: ModeFast}, true},
		{"fast_ignored_suffix_zip", "archive.zip", "/repo/archive.zip", 100, &Options{Mode: ModeFast}, true},
		{"fast_ignored_suffix_pdf", "doc.pdf", "/repo/doc.pdf", 100, &Options{Mode: ModeFast}, true},
		{"fast_ignored_suffix_map", "bundle.map", "/repo/bundle.map", 100, &Options{Mode: ModeFast}, true},
		{"fast_ignored_suffix_minjs", "app.min.js", "/repo/app.min.js", 100, &Options{Mode: ModeFast}, true},
		{"fast_ignored_suffix_mincss", "style.min.css", "/repo/style.min.css", 100, &Options{Mode: ModeFast}, true},
		{"fast_pattern_dts", "types.d.ts", "/repo/types.d.ts", 100, &Options{Mode: ModeFast}, true},
		{"fast_pattern_pbgo", "api.pb.go", "/repo/api.pb.go", 100, &Options{Mode: ModeFast}, true},
		{"fast_pattern_test", "app.test.js", "/repo/app.test.js", 100, &Options{Mode: ModeFast}, true},
		{"fast_pattern_spec", "app.spec.ts", "/repo/app.spec.ts", 100, &Options{Mode: ModeFast}, true},
		{"fast_pattern_mock", "mock_service.go", "/repo/mock_service.go", 100, &Options{Mode: ModeFast}, true},
		{"fast_pattern_stories", "Button.stories.tsx", "/repo/Button.stories.tsx", 100, &Options{Mode: ModeFast}, true},
		{"fast_normal_go_file", "main.go", "/repo/main.go", 100, &Options{Mode: ModeFast}, false},
		{"fast_normal_py_file", "app.py", "/repo/app.py", 100, &Options{Mode: ModeFast}, false},
		{"max_file_size_zero_unlimited", "big.go", "/repo/big.go", 999999, &Options{MaxFileSize: 0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipFile(tt.file, tt.path, tt.size, tt.opts)
			if got != tt.want {
				t.Errorf("shouldSkipFile(%q, %q, %d, opts) = %v, want %v",
					tt.file, tt.path, tt.size, got, tt.want)
			}
		})
	}
}

func TestHasIgnoredSuffix(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/repo/image.png", true},
		{"/repo/image.jpg", true},
		{"/repo/image.jpeg", true},
		{"/repo/font.woff2", true},
		{"/repo/lib.so", true},
		{"/repo/lib.dll", true},
		{"/repo/data.db", true},
		{"/repo/data.sqlite3", true},
		{"/repo/main.exe", true},
		{"/repo/backup~", true},
		{"/repo/compiled.pyc", true},
		{"/repo/compiled.class", true},
		{"/repo/app.wasm", true},
		{"/repo/main.go", false},
		{"/repo/app.py", false},
		{"/repo/index.js", false},
		{"/repo/style.css", false},
		{"/repo/README.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := hasIgnoredSuffix(tt.path)
			if got != tt.want {
				t.Errorf("hasIgnoredSuffix(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsIgnoredJSON(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"package.json", true},
		{"tsconfig.json", true},
		{"package-lock.json", true},
		{".eslintrc.json", true},
		{"angular.json", true},
		{"turbo.json", true},
		{"launch.json", true},
		{"settings.json", true},
		{"biome.json", true},
		{"data.json", false},
		{"config.json", false},
		{"manifest.json", false},
		{"custom.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isIgnoredJSON(tt.name)
			if got != tt.want {
				t.Errorf("isIgnoredJSON(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestLoadIgnoreFile(t *testing.T) {
	t.Run("valid_file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".cgrignore")
		content := "# comment line\n\npattern1\n  pattern2  \n# another comment\npath/to/dir\n"
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}

		patterns, err := loadIgnoreFile(path)
		if err != nil {
			t.Fatalf("loadIgnoreFile: %v", err)
		}

		want := []string{"pattern1", "pattern2", "path/to/dir"}
		if len(patterns) != len(want) {
			t.Fatalf("expected %d patterns, got %d: %v", len(want), len(patterns), patterns)
		}
		for i, p := range patterns {
			if p != want[i] {
				t.Errorf("pattern[%d] = %q, want %q", i, p, want[i])
			}
		}
	})

	t.Run("nonexistent_file", func(t *testing.T) {
		patterns, err := loadIgnoreFile("/no/such/file/.cgrignore")
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
		if patterns != nil {
			t.Errorf("expected nil patterns, got %v", patterns)
		}
	})

	t.Run("empty_file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".cgrignore")
		if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
			t.Fatal(err)
		}

		patterns, err := loadIgnoreFile(path)
		if err != nil {
			t.Fatalf("loadIgnoreFile: %v", err)
		}
		if len(patterns) != 0 {
			t.Errorf("expected 0 patterns, got %d", len(patterns))
		}
	})

	t.Run("only_comments_and_blanks", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".cgrignore")
		content := "# comment\n\n# another\n   \n"
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}

		patterns, err := loadIgnoreFile(path)
		if err != nil {
			t.Fatalf("loadIgnoreFile: %v", err)
		}
		if len(patterns) != 0 {
			t.Errorf("expected 0 patterns, got %d", len(patterns))
		}
	})
}

func TestClassifyFile(t *testing.T) {
	dir := t.TempDir()

	writeFile := func(t *testing.T, name string, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("go_file", func(t *testing.T) {
		path := writeFile(t, "main.go", "package main\n")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		fi := classifyFile(path, "main.go", info, nil)
		if fi == nil {
			t.Fatal("expected non-nil FileInfo for .go file")
		}
		if fi.Language != lang.Go {
			t.Errorf("expected Go, got %s", fi.Language)
		}
		if fi.RelPath != "main.go" {
			t.Errorf("expected RelPath main.go, got %s", fi.RelPath)
		}
	})

	t.Run("ignored_suffix_png", func(t *testing.T) {
		path := writeFile(t, "icon.png", "fake png")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		fi := classifyFile(path, "icon.png", info, nil)
		if fi != nil {
			t.Error("expected nil for .png file")
		}
	})

	t.Run("unknown_extension", func(t *testing.T) {
		path := writeFile(t, "data.xyz", "unknown")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		fi := classifyFile(path, "data.xyz", info, nil)
		if fi != nil {
			t.Error("expected nil for unknown extension")
		}
	})

	t.Run("m_file_matlab", func(t *testing.T) {
		path := writeFile(t, "solver.m", "function y = solve(x)\n  y = x^2;\nend\n")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		fi := classifyFile(path, "solver.m", info, nil)
		if fi == nil {
			t.Fatal("expected non-nil FileInfo for .m file")
		}
		if fi.Language != lang.MATLAB {
			t.Errorf("expected MATLAB, got %s", fi.Language)
		}
	})

	t.Run("m_file_objc", func(t *testing.T) {
		path := writeFile(t, "AppDelegate.m", "@implementation AppDelegate\n@end\n")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		fi := classifyFile(path, "AppDelegate.m", info, nil)
		if fi == nil {
			t.Fatal("expected non-nil FileInfo for .m ObjC file")
		}
		if fi.Language != lang.ObjectiveC {
			t.Errorf("expected ObjectiveC, got %s", fi.Language)
		}
	})

	t.Run("ignored_json_package", func(t *testing.T) {
		path := writeFile(t, "package.json", `{"name":"test"}`)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		fi := classifyFile(path, "package.json", info, nil)
		if fi != nil {
			t.Error("expected nil for package.json")
		}
	})

	t.Run("ignored_json_tsconfig", func(t *testing.T) {
		path := writeFile(t, "tsconfig.json", `{"compilerOptions":{}}`)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		fi := classifyFile(path, "tsconfig.json", info, nil)
		if fi != nil {
			t.Error("expected nil for tsconfig.json")
		}
	})

	t.Run("non_ignored_json", func(t *testing.T) {
		path := writeFile(t, "data.json", `{"key":"value"}`)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		fi := classifyFile(path, "data.json", info, nil)
		if fi == nil {
			t.Fatal("expected non-nil for data.json")
		}
		if fi.Language != lang.JSON {
			t.Errorf("expected JSON, got %s", fi.Language)
		}
	})

	t.Run("large_json_skipped", func(t *testing.T) {
		largeContent := strings.Repeat("x", 101*1024)
		path := writeFile(t, "huge.json", largeContent)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		fi := classifyFile(path, "huge.json", info, nil)
		if fi != nil {
			t.Error("expected nil for JSON > 100KB")
		}
	})

	t.Run("filename_based_detection_makefile", func(t *testing.T) {
		path := writeFile(t, "Makefile", "all:\n\techo hello\n")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		fi := classifyFile(path, "Makefile", info, nil)
		if fi == nil {
			t.Fatal("expected non-nil for Makefile")
		}
		if fi.Language != lang.Makefile {
			t.Errorf("expected Makefile, got %s", fi.Language)
		}
	})

	t.Run("filename_based_detection_dockerfile", func(t *testing.T) {
		path := writeFile(t, "Dockerfile", "FROM alpine\n")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		fi := classifyFile(path, "Dockerfile", info, nil)
		if fi == nil {
			t.Fatal("expected non-nil for Dockerfile")
		}
		if fi.Language != lang.Dockerfile {
			t.Errorf("expected Dockerfile, got %s", fi.Language)
		}
	})

	t.Run("fast_mode_skip_license", func(t *testing.T) {
		path := writeFile(t, "LICENSE", "MIT License...")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		fi := classifyFile(path, "LICENSE", info, &Options{Mode: ModeFast})
		if fi != nil {
			t.Error("expected nil for LICENSE in fast mode")
		}
	})

	t.Run("fast_mode_skip_by_size", func(t *testing.T) {
		path := writeFile(t, "small.go", "package main\n")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		fi := classifyFile(path, "small.go", info, &Options{MaxFileSize: 5})
		if fi != nil {
			t.Error("expected nil for file exceeding MaxFileSize")
		}
	})
}

func TestDiscoverWithIgnoreFile(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "generated", "gen.go"), []byte("package gen\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ignPath := filepath.Join(dir, ".cgrignore")
	if err := os.WriteFile(ignPath, []byte("generated\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	files, err := Discover(ctx, dir, nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file (generated/ ignored), got %d", len(files))
	}
	if files[0].RelPath != "main.go" {
		t.Errorf("expected main.go, got %s", files[0].RelPath)
	}
}

func TestDiscoverWithExplicitIgnoreFile(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "skipme"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skipme", "skip.go"), []byte("package skip\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ignDir := t.TempDir()
	ignPath := filepath.Join(ignDir, "myignore")
	if err := os.WriteFile(ignPath, []byte("skipme\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	files, err := Discover(ctx, dir, &Options{IgnoreFile: ignPath})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file (skipme/ ignored via explicit IgnoreFile), got %d", len(files))
	}
}

func TestDiscoverFastMode(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte("def main(): pass\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "guide.go"), []byte("package docs\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "testdata"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "testdata", "fixture.go"), []byte("package testdata\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("MIT"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), []byte("hash"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	files, err := Discover(ctx, dir, &Options{Mode: ModeFast})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	relPaths := make(map[string]bool)
	for _, f := range files {
		relPaths[f.RelPath] = true
	}

	if !relPaths["main.go"] {
		t.Error("expected main.go to be discovered in fast mode")
	}
	if !relPaths["app.py"] {
		t.Error("expected app.py to be discovered in fast mode")
	}
	if relPaths["docs/guide.go"] {
		t.Error("expected docs/guide.go to be skipped in fast mode")
	}
	if relPaths["testdata/fixture.go"] {
		t.Error("expected testdata/fixture.go to be skipped in fast mode")
	}
	if relPaths["LICENSE"] {
		t.Error("expected LICENSE to be skipped in fast mode")
	}
	if relPaths["go.sum"] {
		t.Error("expected go.sum to be skipped in fast mode")
	}
}

func TestDiscoverSkipsIgnoredDirs(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, d := range []string{".git", "node_modules", "vendor", "__pycache__", ".venv"} {
		subdir := filepath.Join(dir, d)
		if err := os.MkdirAll(subdir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(subdir, "file.go"), []byte("package x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	ctx := context.Background()
	files, err := Discover(ctx, dir, nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(files) != 1 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f.RelPath
		}
		t.Fatalf("expected 1 file, got %d: %v", len(files), names)
	}
}

func TestDiscoverSkipsIgnoredSuffixes(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"image.png", "binary.exe", "backup~", "compiled.pyc", "font.woff2"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	ctx := context.Background()
	files, err := Discover(ctx, dir, nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(files) != 1 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f.RelPath
		}
		t.Fatalf("expected 1 file, got %d: %v", len(files), names)
	}
}

func TestDiscoverMFileIntegration(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "matlab.m"), []byte("function y = f(x)\n  y = x;\nend\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "objc.m"), []byte("@interface Foo\n@end\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	files, err := Discover(ctx, dir, nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	sort.Slice(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })

	langMap := make(map[string]lang.Language)
	for _, f := range files {
		langMap[f.RelPath] = f.Language
	}

	if langMap["matlab.m"] != lang.MATLAB {
		t.Errorf("expected MATLAB for matlab.m, got %s", langMap["matlab.m"])
	}
	if langMap["objc.m"] != lang.ObjectiveC {
		t.Errorf("expected ObjectiveC for objc.m, got %s", langMap["objc.m"])
	}
}

func TestDiscoverJSONFiltering(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"key":"val"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	files, err := Discover(ctx, dir, nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(files) != 1 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f.RelPath
		}
		t.Fatalf("expected 1 JSON file (data.json), got %d: %v", len(files), names)
	}
	if files[0].RelPath != "data.json" {
		t.Errorf("expected data.json, got %s", files[0].RelPath)
	}
}

func TestDiscoverMaxFileSize(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "small.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "big.go"), []byte(strings.Repeat("x", 2000)), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	files, err := Discover(ctx, dir, &Options{MaxFileSize: 100})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(files) != 1 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f.RelPath
		}
		t.Fatalf("expected 1 file (small.go), got %d: %v", len(files), names)
	}
	if files[0].RelPath != "small.go" {
		t.Errorf("expected small.go, got %s", files[0].RelPath)
	}
}

func TestDiscoverEmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	ctx := context.Background()
	files, err := Discover(ctx, dir, nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files for empty dir, got %d", len(files))
	}
}

func TestDiscoverNestedDirectories(t *testing.T) {
	dir := t.TempDir()

	subdir := filepath.Join(dir, "pkg", "sub")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "root.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "mid.go"), []byte("package pkg\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "deep.go"), []byte("package sub\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	files, err := Discover(ctx, dir, nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	relPaths := make(map[string]bool)
	for _, f := range files {
		relPaths[f.RelPath] = true
	}
	for _, want := range []string{"root.go", "pkg/mid.go", "pkg/sub/deep.go"} {
		if !relPaths[want] {
			t.Errorf("expected %s in results", want)
		}
	}
}

func TestDiscoverFastModeDotDirs(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	dotDir := filepath.Join(dir, ".hidden")
	if err := os.MkdirAll(dotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotDir, "secret.go"), []byte("package hidden\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	fullFiles, err := Discover(ctx, dir, &Options{Mode: ModeFull})
	if err != nil {
		t.Fatalf("Discover full: %v", err)
	}

	fastFiles, err := Discover(ctx, dir, &Options{Mode: ModeFast})
	if err != nil {
		t.Fatalf("Discover fast: %v", err)
	}

	if len(fullFiles) != 2 {
		t.Errorf("full mode: expected 2 files, got %d", len(fullFiles))
	}
	if len(fastFiles) != 1 {
		t.Errorf("fast mode: expected 1 file (dot-dir skipped), got %d", len(fastFiles))
	}
}
