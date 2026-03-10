package main

import "embed"

//go:embed assets/skills/codebook-exploring/SKILL.md
var skillExploring string

//go:embed assets/skills/codebook-tracing/SKILL.md
var skillTracing string

//go:embed assets/skills/codebook-quality/SKILL.md
var skillQuality string

//go:embed assets/skills/codebook-reference/SKILL.md
var skillReference string

//go:embed assets/codex-instructions.md
var codexInstructions string

// skillFiles maps skill directory name to embedded content.
var skillFiles = map[string]string{
	"codebook-exploring": skillExploring,
	"codebook-tracing":   skillTracing,
	"codebook-quality":   skillQuality,
	"codebook-reference": skillReference,
}

// Ensure embed import is used.
var _ embed.FS
