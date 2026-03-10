package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	return buf.String()
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	return buf.String()
}

func TestPrintRawJSON_PrettyPrintsObject(t *testing.T) {
	out := captureStdout(t, func() {
		printRawJSON(`{"name":"demo","count":2}`)
	})

	if !strings.Contains(out, "\"name\": \"demo\"") {
		t.Fatalf("expected pretty JSON output, got %q", out)
	}
}

func TestPrintRawJSON_FallsBackToPlainText(t *testing.T) {
	out := captureStdout(t, func() {
		printRawJSON("plain text")
	})

	if out != "plain text\n" {
		t.Fatalf("expected plain text output, got %q", out)
	}
}

func TestPrintSummary_ListProjectsArray(t *testing.T) {
	out := captureStdout(t, func() {
		printSummary("list_projects", `[{"name":"demo","nodes":12,"edges":24,"indexed_at":"now","root_path":"/repo","db_path":"/tmp/demo.db","is_session_project":true}]`, "/tmp/demo.db")
	})

	if !strings.Contains(out, "1 project(s) indexed:") {
		t.Fatalf("expected project count, got %q", out)
	}
	if !strings.Contains(out, "demo") || !strings.Contains(out, "/tmp/demo.db") {
		t.Fatalf("expected project details, got %q", out)
	}
}

func TestPrintSummary_Fallbacks(t *testing.T) {
	t.Run("plain_text", func(t *testing.T) {
		out := captureStdout(t, func() {
			printSummary("search_graph", "not json", "/tmp/demo.db")
		})
		if out != "not json\n" {
			t.Fatalf("expected plain text fallback, got %q", out)
		}
	})

	t.Run("unknown_tool_pretty_json", func(t *testing.T) {
		out := captureStdout(t, func() {
			printSummary("unknown_tool", `{"status":"ok","count":2}`, "/tmp/demo.db")
		})
		if !strings.Contains(out, "\"status\": \"ok\"") {
			t.Fatalf("expected pretty JSON fallback, got %q", out)
		}
	})
}

func TestPrintIndexSummary(t *testing.T) {
	out := captureStdout(t, func() {
		printIndexSummary(map[string]any{
			"project":    "demo",
			"nodes":      12,
			"edges":      24,
			"indexed_at": "now",
		}, "/tmp/demo.db")
	})

	if !strings.Contains(out, `Indexed "demo": 12 nodes, 24 edges`) {
		t.Fatalf("expected index summary, got %q", out)
	}
	if !strings.Contains(out, "db: /tmp/demo.db") {
		t.Fatalf("expected db path, got %q", out)
	}
}

func TestPrintIndexStatusSummary_Statuses(t *testing.T) {
	tests := []struct {
		name string
		data map[string]any
		want string
	}{
		{
			name: "no_session",
			data: map[string]any{"status": "no_session", "message": "no project selected"},
			want: "no project selected",
		},
		{
			name: "not_indexed",
			data: map[string]any{"status": "not_indexed", "project": "demo", "db_path": "/tmp/demo.db"},
			want: "not indexed",
		},
		{
			name: "partial",
			data: map[string]any{"status": "partial", "project": "demo"},
			want: "partially indexed",
		},
		{
			name: "indexing",
			data: map[string]any{"status": "indexing", "project": "demo", "index_elapsed_seconds": 9, "index_type": "full"},
			want: "indexing in progress",
		},
		{
			name: "ready",
			data: map[string]any{
				"status":             "ready",
				"project":            "demo",
				"nodes":              12,
				"edges":              24,
				"indexed_at":         "now",
				"index_type":         "full",
				"is_session_project": true,
				"db_path":            "/tmp/demo.db",
			},
			want: "ready (12 nodes, 24 edges)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				printIndexStatusSummary(tt.data)
			})
			if !strings.Contains(out, tt.want) {
				t.Fatalf("expected %q in output, got %q", tt.want, out)
			}
		})
	}
}

func TestPrintDetectChangesSummary(t *testing.T) {
	out := captureStdout(t, func() {
		printDetectChangesSummary(map[string]any{
			"summary": map[string]any{
				"changed_files":   2,
				"changed_symbols": 3,
				"total":           4,
				"critical":        1,
				"high":            1,
				"medium":          1,
				"low":             1,
			},
			"impacted_symbols": []any{
				map[string]any{
					"risk":       "HIGH",
					"label":      "Function",
					"name":       "ProcessOrder",
					"changed_by": "CALLS",
				},
			},
		})
	})

	if !strings.Contains(out, "Changes: 2 file(s), 3 symbol(s) modified") {
		t.Fatalf("expected summary line, got %q", out)
	}
	if !strings.Contains(out, "[HIGH] [Function] ProcessOrder  (via CALLS)") {
		t.Fatalf("expected impacted symbol details, got %q", out)
	}
}

func TestPrintSearchGraphSummary(t *testing.T) {
	out := captureStdout(t, func() {
		printSearchGraphSummary(map[string]any{
			"total":    2,
			"has_more": true,
			"results": []any{
				map[string]any{
					"name":       "ProcessOrder",
					"label":      "Function",
					"file_path":  "service/order.go",
					"start_line": 42,
				},
			},
		})
	})

	if !strings.Contains(out, "2 result(s) found (showing 1, has_more=true)") {
		t.Fatalf("expected search summary, got %q", out)
	}
	if !strings.Contains(out, "[Function] ProcessOrder  service/order.go:42") {
		t.Fatalf("expected search result details, got %q", out)
	}
}

func TestPrintSearchCodeSummary(t *testing.T) {
	out := captureStdout(t, func() {
		printSearchCodeSummary(map[string]any{
			"total":    1,
			"has_more": false,
			"matches": []any{
				map[string]any{
					"file":    "main.go",
					"line":    17,
					"content": "fmt.Println(\"hello\")",
				},
			},
		})
	})

	if !strings.Contains(out, "1 match(es) found") {
		t.Fatalf("expected match count, got %q", out)
	}
	if !strings.Contains(out, "main.go:17  fmt.Println(\"hello\")") {
		t.Fatalf("expected match details, got %q", out)
	}
}

func TestPrintTraceSummary(t *testing.T) {
	out := captureStdout(t, func() {
		printTraceSummary(map[string]any{
			"root":          map[string]any{"name": "ProcessOrder"},
			"total_results": 3,
			"edges":         []any{map[string]any{}, map[string]any{}},
			"hops": []any{
				map[string]any{
					"hop": 1,
					"nodes": []any{
						map[string]any{"name": "Validate", "label": "Function"},
					},
				},
			},
		})
	})

	if !strings.Contains(out, `Trace from "ProcessOrder": 3 node(s), 2 edge(s), 1 hop(s)`) {
		t.Fatalf("expected trace header, got %q", out)
	}
	if !strings.Contains(out, "[Function] Validate") {
		t.Fatalf("expected hop details, got %q", out)
	}
}

func TestPrintQuerySummary(t *testing.T) {
	out := captureStdout(t, func() {
		printQuerySummary(map[string]any{
			"total":   2,
			"columns": []any{"name", "count"},
			"rows": []any{
				map[string]any{"name": "ProcessOrder", "count": 3},
				[]any{"ShipOrder", 2},
			},
		})
	})

	if !strings.Contains(out, "2 row(s) returned  [name, count]") {
		t.Fatalf("expected query summary header, got %q", out)
	}
	if !strings.Contains(out, "ProcessOrder | 3") || !strings.Contains(out, "ShipOrder | 2") {
		t.Fatalf("expected query rows, got %q", out)
	}
}

func TestPrintSchemaSummary(t *testing.T) {
	t.Run("no_projects", func(t *testing.T) {
		out := captureStdout(t, func() {
			printSchemaSummary(map[string]any{"projects": []any{}})
		})
		if out != "No projects indexed.\n" {
			t.Fatalf("expected empty schema message, got %q", out)
		}
	})

	t.Run("project_schema", func(t *testing.T) {
		out := captureStdout(t, func() {
			printSchemaSummary(map[string]any{
				"projects": []any{
					map[string]any{
						"project": "demo",
						"schema": map[string]any{
							"node_labels": []any{
								map[string]any{"label": "Function", "count": 5},
							},
							"relationship_types": []any{
								map[string]any{"type": "CALLS", "count": 7},
							},
						},
					},
				},
			})
		})
		if !strings.Contains(out, "Project: demo") {
			t.Fatalf("expected project name, got %q", out)
		}
		if !strings.Contains(out, "Function") || !strings.Contains(out, "CALLS") {
			t.Fatalf("expected schema details, got %q", out)
		}
	})
}

func TestOtherSummaryPrinters(t *testing.T) {
	t.Run("snippet", func(t *testing.T) {
		out := captureStdout(t, func() {
			printSnippetSummary(map[string]any{
				"name":       "ProcessOrder",
				"label":      "Function",
				"file_path":  "service/order.go",
				"start_line": 10,
				"end_line":   20,
				"source":     "func ProcessOrder() {}",
			})
		})
		if !strings.Contains(out, "[Function] ProcessOrder  (service/order.go:10-20)") {
			t.Fatalf("expected snippet header, got %q", out)
		}
	})

	t.Run("delete", func(t *testing.T) {
		out := captureStdout(t, func() {
			printDeleteSummary(map[string]any{"deleted": "demo"})
		})
		if out != "Deleted project \"demo\"\n" {
			t.Fatalf("expected delete summary, got %q", out)
		}
	})

	t.Run("read_file", func(t *testing.T) {
		out := captureStdout(t, func() {
			printReadFileSummary(map[string]any{
				"path":        "README.md",
				"total_lines": 3,
				"content":     "line1\nline2",
			})
		})
		if !strings.Contains(out, "README.md (3 lines)") {
			t.Fatalf("expected file summary, got %q", out)
		}
	})

	t.Run("list_directory", func(t *testing.T) {
		out := captureStdout(t, func() {
			printListDirSummary(map[string]any{
				"directory": "cmd",
				"count":     2,
				"entries": []any{
					map[string]any{"name": "main.go", "is_dir": false, "size": 128},
					map[string]any{"name": "assets", "is_dir": true},
				},
			})
		})
		if !strings.Contains(out, "cmd (2 entries)") {
			t.Fatalf("expected dir summary, got %q", out)
		}
		if !strings.Contains(out, "assets/") || !strings.Contains(out, "main.go") {
			t.Fatalf("expected dir entries, got %q", out)
		}
	})

	t.Run("ingest", func(t *testing.T) {
		out := captureStdout(t, func() {
			printIngestSummary(map[string]any{
				"matched":     3,
				"boosted":     1,
				"total_spans": 4,
			}, "/tmp/demo.db")
		})
		if !strings.Contains(out, "Ingested 4 span(s): 3 matched, 1 boosted") {
			t.Fatalf("expected ingest summary, got %q", out)
		}
	})
}

func TestHelpers_JSONIntAndMustJSON(t *testing.T) {
	if got := jsonInt(3.9); got != 3 {
		t.Fatalf("jsonInt(float64) = %d, want 3", got)
	}
	if got := jsonInt(7); got != 7 {
		t.Fatalf("jsonInt(int) = %d, want 7", got)
	}
	if got := jsonInt("bad"); got != 0 {
		t.Fatalf("jsonInt(string) = %d, want 0", got)
	}

	out := mustJSON(map[string]any{"name": "demo"})
	if !strings.Contains(out, "\"name\": \"demo\"") {
		t.Fatalf("expected JSON string, got %q", out)
	}
}

func TestRunCLI(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		out := captureStderr(t, func() {
			if code := runCLI([]string{"--help"}); code != 0 {
				t.Fatalf("runCLI help returned %d, want 0", code)
			}
		})
		if !strings.Contains(out, "Usage: codebase-memory-mcp cli") {
			t.Fatalf("expected help output, got %q", out)
		}
	})

	t.Run("unknown_tool", func(t *testing.T) {
		out := captureStderr(t, func() {
			if code := runCLI([]string{"unknown_tool", "{}"}); code != 1 {
				t.Fatalf("runCLI unknown tool returned %d, want 1", code)
			}
		})
		if !strings.Contains(out, "error:") {
			t.Fatalf("expected error output, got %q", out)
		}
	})

	t.Run("raw_success", func(t *testing.T) {
		out := captureStdout(t, func() {
			if code := runCLI([]string{"--raw", "list_projects", "{}"}); code != 0 {
				t.Fatalf("runCLI raw returned %d, want 0", code)
			}
		})
		if !strings.Contains(out, "[") {
			t.Fatalf("expected raw JSON array output, got %q", out)
		}
	})

	t.Run("summary_success", func(t *testing.T) {
		out := captureStdout(t, func() {
			if code := runCLI([]string{"list_projects", "{}"}); code != 0 {
				t.Fatalf("runCLI summary returned %d, want 0", code)
			}
		})
		if !strings.Contains(out, "project(s) indexed") && !strings.Contains(out, "No projects indexed.") {
			t.Fatalf("expected list_projects summary output, got %q", out)
		}
	})
}
