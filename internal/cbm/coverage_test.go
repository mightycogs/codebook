package cbm

import (
	"testing"

	"github.com/mightycogs/codebook/internal/lang"
)

func TestShutdown(t *testing.T) {
	Init()
	Shutdown()
}

func TestGetProfile(t *testing.T) {
	Init()
	ExtractFile([]byte("package main\nfunc main() {}\n"), lang.Go, "t", "main.go")
	stats := GetProfile()
	if stats.Files == 0 {
		t.Error("expected Files > 0 after extraction")
	}
	if stats.ParseNs == 0 {
		t.Error("expected ParseNs > 0 after extraction")
	}
	if stats.ExtractNs == 0 {
		t.Error("expected ExtractNs > 0 after extraction")
	}
}

func TestExtractFileEmptySource(t *testing.T) {
	Init()
	result, err := ExtractFile([]byte{}, lang.Go, "t", "empty.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for empty source")
	}
	if len(result.Definitions) != 0 {
		t.Errorf("expected 0 definitions for empty source, got %d", len(result.Definitions))
	}
}

func TestExtractFileUnsupportedLanguage(t *testing.T) {
	Init()
	_, err := ExtractFile([]byte("hello"), lang.Language("klingon"), "t", "test.kl")
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestRustImplTraitExtraction(t *testing.T) {
	src := []byte(`
trait Greet {
    fn hello(&self) -> String;
}

struct Person {
    name: String,
}

impl Greet for Person {
    fn hello(&self) -> String {
        format!("Hello, {}", self.name)
    }
}
`)
	result, err := ExtractFile(src, lang.Rust, "t", "lib.rs")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ImplTraits) == 0 {
		t.Fatal("expected at least 1 ImplTrait for 'impl Greet for Person'")
	}
	found := false
	for _, it := range result.ImplTraits {
		if it.TraitName == "Greet" && it.StructName == "Person" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ImplTrait{Greet, Person}, got %+v", result.ImplTraits)
	}
}

func TestPythonEnvAccessExtraction(t *testing.T) {
	src := []byte(`import os

def get_config():
    host = os.environ.get("DB_HOST")
    port = os.getenv("DB_PORT")
    return host, port
`)
	result, err := ExtractFile(src, lang.Python, "t", "config.py")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.EnvAccesses) == 0 {
		t.Fatal("expected at least 1 EnvAccess for os.environ/os.getenv usage")
	}
}

func TestGoEnvAccessExtraction(t *testing.T) {
	src := []byte(`package main

import "os"

func loadConfig() string {
	return os.Getenv("APP_SECRET")
}
`)
	result, err := ExtractFile(src, lang.Go, "t", "config.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.EnvAccesses) == 0 {
		t.Fatal("expected at least 1 EnvAccess for os.Getenv usage")
	}
	found := false
	for _, ea := range result.EnvAccesses {
		if ea.EnvKey == "APP_SECRET" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected EnvKey 'APP_SECRET', got %+v", result.EnvAccesses)
	}
}

func TestPythonTypeAssignExtraction(t *testing.T) {
	src := []byte(`class Engine:
    pass

class Car:
    pass

def build():
    engine = Engine()
    car = Car()
    return car
`)
	result, err := ExtractFile(src, lang.Python, "t", "factory.py")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TypeAssigns) == 0 {
		t.Fatal("expected at least 1 TypeAssign for constructor assignments")
	}
	foundEngine := false
	foundCar := false
	for _, ta := range result.TypeAssigns {
		if ta.VarName == "engine" && ta.TypeName == "Engine" {
			foundEngine = true
		}
		if ta.VarName == "car" && ta.TypeName == "Car" {
			foundCar = true
		}
	}
	if !foundEngine {
		t.Errorf("expected TypeAssign{engine, Engine}, got %+v", result.TypeAssigns)
	}
	if !foundCar {
		t.Errorf("expected TypeAssign{car, Car}, got %+v", result.TypeAssigns)
	}
}
