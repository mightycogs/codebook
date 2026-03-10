package fqn

import "testing"

func TestCompute(t *testing.T) {
	tests := []struct {
		name    string
		project string
		relPath string
		symbol  string
		want    string
	}{
		{
			name:    "regular_file",
			project: "demo",
			relPath: "pkg/service/order.go",
			symbol:  "ProcessOrder",
			want:    "demo.pkg.service.order.ProcessOrder",
		},
		{
			name:    "python_init",
			project: "demo",
			relPath: "pkg/service/__init__.py",
			symbol:  "Bootstrap",
			want:    "demo.pkg.service.Bootstrap",
		},
		{
			name:    "index_file_without_symbol",
			project: "demo",
			relPath: "web/index.ts",
			symbol:  "",
			want:    "demo.web",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Compute(tt.project, tt.relPath, tt.symbol); got != tt.want {
				t.Fatalf("Compute(%q, %q, %q) = %q, want %q", tt.project, tt.relPath, tt.symbol, got, tt.want)
			}
		})
	}
}

func TestModuleQN(t *testing.T) {
	if got := ModuleQN("demo", "pkg/service/order.go"); got != "demo.pkg.service.order" {
		t.Fatalf("ModuleQN() = %q, want %q", got, "demo.pkg.service.order")
	}
}

func TestFolderQN(t *testing.T) {
	if got := FolderQN("demo", "pkg/service/api"); got != "demo.pkg.service.api" {
		t.Fatalf("FolderQN() = %q, want %q", got, "demo.pkg.service.api")
	}
}
