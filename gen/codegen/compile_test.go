package codegen_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sofired/grizzle/gen/codegen"
	"github.com/sofired/grizzle/gen/parser"
)

// schemaForCompile exercises every major column type so the compile test
// validates that all generated import paths and type expressions are correct.
const schemaForCompile = `package myschema

import pg "github.com/sofired/grizzle/schema/pg"

var Realms = pg.Table("realms",
	pg.C("id",         pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("name",       pg.Varchar(255).NotNull().Unique()),
	pg.C("created_at", pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
)

var Users = pg.Table("users",
	pg.C("id",         pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("realm_id",   pg.UUID().NotNull()),
	pg.C("username",   pg.Varchar(255).NotNull()),
	pg.C("email",      pg.Varchar(255)),
	pg.C("score",      pg.Numeric(10, 2)),
	pg.C("enabled",    pg.Boolean().NotNull().Default(true)),
	pg.C("meta",       pg.JSONB()),
	pg.C("created_at", pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
	pg.C("deleted_at", pg.Timestamp().WithTimezone()),
)
`

// TestGeneratedCode_Compiles is an end-to-end test that:
//  1. Parses a schema
//  2. Generates Go source files
//  3. Writes them into a temp module that depends on grizzle via a replace directive
//  4. Runs `go build ./...` to verify the output compiles
//
// This catches type errors, bad import paths, and template regressions that
// string-matching tests cannot detect.
func TestGeneratedCode_Compiles(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go binary not found in PATH, skipping compile test")
	}

	moduleRoot := findModuleRoot(t)

	// --- Parse schema ---
	schemaDir := t.TempDir()
	schemaFile := filepath.Join(schemaDir, "schema.go")
	if err := os.WriteFile(schemaFile, []byte(schemaForCompile), 0644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	tables, err := parser.ParseFile(schemaFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}

	// --- Generate code ---
	outDir := t.TempDir()
	files, err := codegen.GenerateAll(tables, codegen.Options{
		PackageName: "myschema",
		OutputDir:   outDir,
	})
	if err != nil {
		t.Fatalf("GenerateAll: %v", err)
	}
	for _, f := range files {
		if err := os.WriteFile(f.FileName, f.Source, 0644); err != nil {
			t.Fatalf("write %s: %v", f.FileName, err)
		}
		t.Logf("generated %s (%d bytes)", filepath.Base(f.FileName), len(f.Source))
	}

	// --- Set up temp module ---
	// go.mod: depend on grizzle via a local replace directive so no network
	// access is needed and the test always uses the current working tree.
	goMod := "module e2etest\n\ngo 1.22\n\n" +
		"require github.com/sofired/grizzle v0.0.0\n\n" +
		"replace github.com/sofired/grizzle => " + moduleRoot + "\n"
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	// Copy go.sum from the module root so `go build` can verify hashes for
	// transitive deps (e.g. github.com/google/uuid) without network access.
	parentSum, err := os.ReadFile(filepath.Join(moduleRoot, "go.sum"))
	if err != nil {
		t.Fatalf("read parent go.sum: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "go.sum"), parentSum, 0644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}

	// --- go mod tidy ---
	// Resolves transitive deps from the replace target's go.mod and updates
	// go.sum with any additional hashes needed.
	tidy := exec.Command(goBin, "mod", "tidy")
	tidy.Dir = outDir
	tidy.Env = goEnv()
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy:\n%s\nerror: %v", out, err)
	}

	// --- go build ./... ---
	build := exec.Command(goBin, "build", "./...")
	build.Dir = outDir
	build.Env = goEnv()
	if out, err := build.CombinedOutput(); err != nil {
		// Print the generated sources to aid diagnosis.
		for _, f := range files {
			t.Logf("=== %s ===\n%s", filepath.Base(f.FileName), f.Source)
		}
		t.Fatalf("go build ./...:\n%s\nerror: %v", out, err)
	}
}

// findModuleRoot walks up from the test file's directory until it finds go.mod.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	// runtime.Caller gives the path of this source file at compile time.
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find module root (no go.mod found)")
		}
		dir = parent
	}
}

// goEnv returns an environment suitable for running go commands in tests.
// It inherits the current environment and ensures GOFLAGS is clean.
func goEnv() []string {
	env := os.Environ()
	// Remove any GOFLAGS that might interfere (e.g. -mod=vendor).
	filtered := env[:0]
	for _, e := range env {
		if !strings.HasPrefix(e, "GOFLAGS=") {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
