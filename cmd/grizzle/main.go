// Command grizzle is the CLI for the Grizzle ORM toolkit.
//
// Usage:
//
//	grizzle gen      [--schema <dir>] [--out <dir>] [--package <name>]
//	grizzle sql      [--schema <dir>] [--dialect postgres|mysql|sqlite]
//	grizzle diff     [--schema <dir>] [--snapshot <file>] [--dialect postgres|mysql|sqlite]
//	grizzle snapshot [--schema <dir>] [--out <file>]
//	grizzle migrate  [--migrations <dir>] --db <dsn> [--dialect postgres|mysql|sqlite] [--baseline <tag>] [--skip-schema-upgrade]
//	grizzle status   [--migrations <dir>] --db <dsn> [--dialect postgres|mysql|sqlite]
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sofired/grizzle/gen/codegen"
	"github.com/sofired/grizzle/gen/parser"
	"github.com/sofired/grizzle/kit"
	pg "github.com/sofired/grizzle/schema/pg"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("grizzle: ")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "gen":
		if err := runGen(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "sql":
		if err := runSQL(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "snapshot":
		if err := runSnapshot(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "diff":
		if err := runDiff(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "migrate":
		if err := runMigrate(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "status":
		if err := runStatus(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "help", "--help", "-h":
		printUsage()
	default:
		log.Fatalf("unknown command %q — run 'grizzle help' for usage", os.Args[1])
	}
}

func runGen(args []string) error {
	fs := flag.NewFlagSet("gen", flag.ExitOnError)
	schemaDir := fs.String("schema", ".", "directory containing schema Go files")
	outDir := fs.String("out", "", "output directory for generated files (default: same as --schema)")
	pkgName := fs.String("package", "", "Go package name for generated files (default: inferred from --out)")
	verbose := fs.Bool("v", false, "verbose output")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Resolve schema directory.
	schemaAbs, err := filepath.Abs(*schemaDir)
	if err != nil {
		return fmt.Errorf("resolve schema dir: %w", err)
	}

	// Default out dir to schema dir.
	if *outDir == "" {
		*outDir = schemaAbs
	}
	outAbs, err := filepath.Abs(*outDir)
	if err != nil {
		return fmt.Errorf("resolve out dir: %w", err)
	}

	// Ensure output directory exists.
	if err := os.MkdirAll(outAbs, 0755); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}

	// Infer package name from directory basename if not specified.
	if *pkgName == "" {
		*pkgName = filepath.Base(outAbs)
	}

	if *verbose {
		log.Printf("schema dir : %s", schemaAbs)
		log.Printf("output dir : %s", outAbs)
		log.Printf("package    : %s", *pkgName)
	}

	// Parse schema files.
	tables, err := parser.ParseDir(schemaAbs)
	if err != nil {
		return fmt.Errorf("parse schema: %w", err)
	}
	if len(tables) == 0 {
		log.Printf("warning: no table declarations found in %s (expected pg.Table, mysql.Table, sqlite.Table, pg.SchemaTable, mysql.SchemaTable, or sqlite.SchemaTable calls)", schemaAbs)
		return nil
	}
	if *verbose {
		log.Printf("found %d table(s):", len(tables))
		for _, t := range tables {
			log.Printf("  %s (%s)", t.VarName, t.TableName)
		}
	}

	// Generate code.
	opts := codegen.Options{
		PackageName: *pkgName,
		OutputDir:   outAbs,
	}
	files, err := codegen.GenerateAll(tables, opts)
	if err != nil {
		return fmt.Errorf("codegen: %w", err)
	}

	// Write output files.
	for _, f := range files {
		if err := os.WriteFile(f.FileName, f.Source, 0644); err != nil {
			return fmt.Errorf("write %s: %w", f.FileName, err)
		}
		if *verbose {
			log.Printf("wrote %s", f.FileName)
		} else {
			fmt.Println(f.FileName)
		}
	}

	log.Printf("generated %d file(s)", len(files))
	return nil
}

// runSQL generates full CREATE TABLE + CREATE INDEX SQL from schema files,
// printing it to stdout. Useful for initialising a fresh database.
func runSQL(args []string) error {
	fs := flag.NewFlagSet("sql", flag.ExitOnError)
	schemaDir := fs.String("schema", ".", "directory containing schema Go files")
	dialect := fs.String("dialect", "postgres", "target dialect: postgres, mysql, or sqlite")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tables, err := parseSchemaDir(*schemaDir)
	if err != nil {
		return err
	}
	switch *dialect {
	case "mysql":
		fmt.Println(kit.GenerateCreateSQLMySQL(tables...))
	case "sqlite":
		fmt.Println(kit.GenerateCreateSQLSQLite(tables...))
	default:
		fmt.Println(kit.GenerateCreateSQL(tables...))
	}
	return nil
}

// runSnapshot writes a JSON snapshot of the schema to a file.
func runSnapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	schemaDir := fs.String("schema", ".", "directory containing schema Go files")
	outFile := fs.String("out", "schema.snapshot.json", "output snapshot file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tables, err := parseSchemaDir(*schemaDir)
	if err != nil {
		return err
	}
	snap := kit.FromDefs(tables...)
	if err := kit.SaveJSON(snap, *outFile); err != nil {
		return err
	}
	log.Printf("wrote snapshot: %s (%d table(s))", *outFile, len(snap.Tables))
	return nil
}

// runDiff compares the current schema against a saved snapshot and prints
// the SQL changes that would be needed to migrate from snapshot → schema.
func runDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	schemaDir := fs.String("schema", ".", "directory containing schema Go files")
	snapshotFile := fs.String("snapshot", "schema.snapshot.json", "path to the baseline snapshot file")
	dialect := fs.String("dialect", "postgres", "target dialect: postgres, mysql, or sqlite")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Load old snapshot.
	old, err := kit.LoadJSON(*snapshotFile)
	if err != nil {
		// If the snapshot doesn't exist, treat it as an empty DB.
		if os.IsNotExist(err) {
			old = kit.EmptySnapshot()
			log.Printf("no snapshot found at %s — diffing against empty database", *snapshotFile)
		} else {
			return err
		}
	}

	// Parse new schema.
	tables, err := parseSchemaDir(*schemaDir)
	if err != nil {
		return err
	}
	newSnap := kit.FromDefs(tables...)

	// Compute changes.
	changes := kit.Diff(old, newSnap)
	if len(changes) == 0 {
		fmt.Println("-- No changes")
		return nil
	}

	var stmts []string
	switch *dialect {
	case "mysql":
		stmts = kit.AllChangeSQLMySQL(newSnap, changes)
	case "sqlite":
		stmts = kit.AllChangeSQLSQLite(newSnap, changes)
	default:
		stmts = kit.AllChangeSQL(newSnap, changes)
	}
	fmt.Println(strings.Join(stmts, ";\n") + ";")
	log.Printf("%d change(s), %d statement(s)", len(changes), len(stmts))
	return nil
}

// runMigrate applies pending SQL migration files to a live database and records
// the history in _grizzle_migrations.
func runMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	migrationsDir := fs.String("migrations", "./migrations", "directory containing .sql migration files")
	dsn := fs.String("db", "", "database connection string (required)")
	dialect := fs.String("dialect", "postgres", "target dialect: postgres, mysql, or sqlite")
	baseline := fs.String("baseline", "", "bootstrap file-based workflow: mark this tag (and all preceding) as applied without executing SQL")
	skipUpgrade := fs.Bool("skip-schema-upgrade", false, "skip automatic _grizzle_migrations schema upgrade; error if columns are absent")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dsn == "" {
		return fmt.Errorf("--db <connection-string> is required")
	}

	opts := kit.MigrateOptions{
		MigrationsDir:     *migrationsDir,
		Baseline:          *baseline,
		SkipSchemaUpgrade: *skipUpgrade,
	}

	ctx := context.Background()

	switch *dialect {
	case "mysql":
		return runMigrateMySQL(ctx, *dsn, opts)
	case "sqlite":
		return runMigrateSQLite(ctx, *dsn, opts)
	default:
		return runMigratePostgres(ctx, *dsn, opts)
	}
}

func runMigratePostgres(ctx context.Context, dsn string, opts kit.MigrateOptions) error {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	result, err := kit.Migrate(ctx, pool, opts)
	if err != nil {
		return err
	}
	if result.AlreadyCurrent {
		log.Println("already current — nothing to apply")
		return nil
	}
	if len(result.Baselined) > 0 {
		log.Printf("baselined %d migration(s)", len(result.Baselined))
		for _, f := range result.Baselined {
			log.Printf("  baselined: %s", f.FileName)
		}
	}
	if len(result.Applied) > 0 {
		log.Printf("applied %d migration(s)", len(result.Applied))
		for _, f := range result.Applied {
			log.Printf("  applied: %s", f.FileName)
		}
	}
	return nil
}

func runMigrateMySQL(ctx context.Context, dsn string, opts kit.MigrateOptions) error {
	db, err := openMySQL(dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	result, err := kit.MigrateMySQL(ctx, db, opts)
	if err != nil {
		return err
	}
	if result.AlreadyCurrent {
		log.Println("already current — nothing to apply")
		return nil
	}
	if len(result.Baselined) > 0 {
		log.Printf("baselined %d migration(s)", len(result.Baselined))
		for _, f := range result.Baselined {
			log.Printf("  baselined: %s", f.FileName)
		}
	}
	if len(result.Applied) > 0 {
		log.Printf("applied %d migration(s)", len(result.Applied))
		for _, f := range result.Applied {
			log.Printf("  applied: %s", f.FileName)
		}
	}
	return nil
}

func runMigrateSQLite(ctx context.Context, dsn string, opts kit.MigrateOptions) error {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("open sqlite3: %w", err)
	}
	defer func() { _ = db.Close() }()

	result, err := kit.MigrateSQLite(ctx, db, opts)
	if err != nil {
		return err
	}
	if result.AlreadyCurrent {
		log.Println("already current — nothing to apply")
		return nil
	}
	if len(result.Baselined) > 0 {
		log.Printf("baselined %d migration(s)", len(result.Baselined))
		for _, f := range result.Baselined {
			log.Printf("  baselined: %s", f.FileName)
		}
	}
	if len(result.Applied) > 0 {
		log.Printf("applied %d migration(s)", len(result.Applied))
		for _, f := range result.Applied {
			log.Printf("  applied: %s", f.FileName)
		}
	}
	return nil
}

// runStatus shows migration history and any pending migration files.
func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	migrationsDir := fs.String("migrations", "./migrations", "directory containing .sql migration files")
	dsn := fs.String("db", "", "database connection string (required)")
	dialect := fs.String("dialect", "postgres", "target dialect: postgres or mysql or sqlite")
	skipUpgrade := fs.Bool("skip-schema-upgrade", false, "skip automatic _grizzle_migrations schema upgrade")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dsn == "" {
		return fmt.Errorf("--db <connection-string> is required")
	}

	opts := kit.MigrateOptions{
		MigrationsDir:     *migrationsDir,
		SkipSchemaUpgrade: *skipUpgrade,
	}

	ctx := context.Background()

	var status kit.StatusResult
	switch *dialect {
	case "mysql":
		db, err := openMySQL(*dsn)
		if err != nil {
			return err
		}
		defer func() { _ = db.Close() }()
		status, err = kit.StatusMySQL(ctx, db, opts)
		if err != nil {
			return err
		}
	case "sqlite":
		db, err := sql.Open("sqlite3", *dsn)
		if err != nil {
			return fmt.Errorf("open sqlite3: %w", err)
		}
		defer func() { _ = db.Close() }()
		status, err = kit.StatusSQLite(ctx, db, opts)
		if err != nil {
			return err
		}
	default:
		pool, err := pgxpool.New(ctx, *dsn)
		if err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		defer pool.Close()
		status, err = kit.Status(ctx, pool, opts)
		if err != nil {
			return err
		}
	}

	if len(status.Applied) == 0 {
		fmt.Println("No migrations applied yet.")
	} else {
		fmt.Printf("Applied migrations (%d):\n", len(status.Applied))
		for _, r := range status.Applied {
			baselineMarker := ""
			if r.IsBaseline {
				baselineMarker = " [baseline]"
			}
			tagDisplay := r.Tag
			if tagDisplay == "" {
				tagDisplay = "(legacy)"
			}
			fmt.Printf("  [%s] %s%s  (checksum: %s)\n",
				r.AppliedAt.Format("2006-01-02 15:04:05Z"), tagDisplay, baselineMarker, truncate(r.Checksum, 8))
		}
	}

	if len(status.Pending) == 0 {
		fmt.Println("\nSchema is current — no pending migrations.")
	} else {
		fmt.Printf("\nPending migrations (%d):\n", len(status.Pending))
		for _, f := range status.Pending {
			fmt.Printf("  %s\n", f.FileName)
		}
	}
	return nil
}

// truncate returns at most n characters from s.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// openMySQL opens a *sql.DB for MySQL, ensuring parseTime=true is set so
// that DATETIME columns scan correctly into time.Time.
func openMySQL(dsn string) (*sql.DB, error) {
	if !strings.Contains(dsn, "parseTime") {
		if strings.Contains(dsn, "?") {
			dsn += "&parseTime=true"
		} else {
			dsn += "?parseTime=true"
		}
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	return db, nil
}

// parseSchemaDir parses schema Go files and evaluates them into pg.TableDefiner
// values. Each table is returned as the correct dialect-specific type:
// *pg.TableDef for PostgreSQL schemas, *mysql.TableDef for MySQL schemas, and
// *sqlite.TableDef for SQLite schemas.
func parseSchemaDir(dir string) ([]pg.TableDefiner, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve schema dir: %w", err)
	}
	parsed, err := parser.ParseDir(abs)
	if err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("no table declarations found in %s (expected pg.Table, mysql.Table, sqlite.Table, pg.SchemaTable, mysql.SchemaTable, or sqlite.SchemaTable calls)", abs)
	}
	defs := make([]pg.TableDefiner, 0, len(parsed))
	for _, pt := range parsed {
		td, err := parser.EvalTable(pt)
		if err != nil {
			return nil, fmt.Errorf("eval table %s: %w", pt.VarName, err)
		}
		defs = append(defs, td)
	}
	return defs, nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `grizzle — the Grizzle ORM toolkit

Usage:
  grizzle <command> [flags]

Commands:
  gen       Generate typed Go code from schema definitions
  sql       Print CREATE TABLE SQL for a schema (fresh DB init)
  snapshot  Write schema to a JSON snapshot file
  diff      Compare schema against snapshot, print migration SQL
  migrate   Apply pending .sql migration files to a live DB and record history
  status    Show applied migrations and pending .sql files

gen flags:
  --schema <dir>    Directory containing schema Go files (default: .)
  --out <dir>       Output directory for generated files (default: same as --schema)
  --package <name>  Package name for generated files (default: basename of --out)
  -v                Verbose output

sql / diff flags:
  --schema <dir>        Directory containing schema Go files (default: .)
  --dialect <dialect>   Target SQL dialect: postgres (default), mysql, or sqlite
  --snapshot <file>     (diff only) Baseline snapshot path (default: schema.snapshot.json)

snapshot flags:
  --schema <dir>    Directory containing schema Go files (default: .)
  --out <file>      Output snapshot path (default: schema.snapshot.json)

migrate flags:
  --migrations <dir>       Directory containing .sql migration files (default: ./migrations)
  --db <dsn>               Database connection string (required)
  --dialect <dialect>      Target SQL dialect: postgres (default), mysql, or sqlite
  --baseline <tag>         Bootstrap file-based workflow: mark this tag (and all with
                           lower sequence numbers) as applied without executing SQL
  --skip-schema-upgrade    Skip automatic _grizzle_migrations schema upgrade;
                           returns error if tag/is_baseline columns are absent

status flags:
  --migrations <dir>       Directory containing .sql migration files (default: ./migrations)
  --db <dsn>               Database connection string (required)
  --dialect <dialect>      Target SQL dialect: postgres (default), mysql, or sqlite
  --skip-schema-upgrade    Skip automatic _grizzle_migrations schema upgrade

Examples:
  grizzle gen --schema ./db/schema --out ./db/schema --package schema
  grizzle sql --schema ./db/schema
  grizzle sql --schema ./db/schema --dialect mysql
  grizzle snapshot --schema ./db/schema --out ./db/schema.snapshot.json
  grizzle diff --schema ./db/schema --snapshot ./db/schema.snapshot.json
  grizzle diff --schema ./db/schema --dialect mysql
  grizzle migrate --migrations ./db/migrations --db "postgres://user:pass@localhost/mydb"
  grizzle migrate --migrations ./db/migrations --db "user:pass@tcp(localhost:3306)/mydb" --dialect mysql
  grizzle migrate --migrations ./db/migrations --db "postgres://..." --baseline 0001_initial_schema
  grizzle status  --migrations ./db/migrations --db "postgres://user:pass@localhost/mydb"
  grizzle status  --migrations ./db/migrations --db "user:pass@tcp(localhost:3306)/mydb" --dialect mysql
`)
}
