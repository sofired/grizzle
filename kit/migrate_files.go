package kit

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// tagRe is the validation pattern for migration tags. Tags must match this
// pattern; the library enforces it regardless of whether the tag originates
// from the CLI or from Go code.
var tagRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateTag returns an error if tag is not a valid migration tag value.
// Tags are validated against ^[a-zA-Z0-9_-]+$.
func ValidateTag(tag string) error {
	if tag == "" {
		return fmt.Errorf("migration tag must not be empty")
	}
	if !tagRe.MatchString(tag) {
		return fmt.Errorf("invalid migration tag %q: must match ^[a-zA-Z0-9_-]+$", tag)
	}
	return nil
}

// MigrationFile represents a single .sql migration file discovered in the
// migrations directory.
type MigrationFile struct {
	// FileName is the base name of the file (e.g. "0001_initial_schema.sql").
	FileName string
	// Tag is the filename stem without the ".sql" extension
	// (e.g. "0001_initial_schema").
	Tag string
	// SeqNum is the leading numeric prefix parsed from the filename
	// (e.g. 1 for "0001_initial_schema.sql").
	SeqNum int
	// Path is the full absolute or relative path to the file.
	Path string
}

// tagFromStem extracts the sequence number and validates the tag. The sequence
// number is the leading decimal integer before the first underscore.
func tagFromStem(stem string) (int, error) {
	idx := strings.IndexByte(stem, '_')
	if idx <= 0 {
		return 0, fmt.Errorf("migration filename stem %q has no underscore-separated sequence prefix", stem)
	}
	prefix := stem[:idx]
	n, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("migration filename stem %q: sequence prefix %q is not an integer", stem, prefix)
	}
	if n <= 0 {
		return 0, fmt.Errorf("migration filename stem %q: sequence prefix must be a positive integer", stem)
	}
	return n, nil
}

// LoadMigrationFiles reads all .sql files from dir and returns them sorted by
// sequence number. It returns an error if any file has an invalid tag, an
// invalid or missing sequence prefix, or if two files share the same sequence
// prefix (duplicate prefixes are invalid).
func LoadMigrationFiles(dir string) ([]MigrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir %q: %w", dir, err)
	}

	var files []MigrationFile
	seqSeen := make(map[int]string) // seqNum → first filename that used it

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		stem := strings.TrimSuffix(name, ".sql")

		// Validate tag.
		if err := ValidateTag(stem); err != nil {
			return nil, fmt.Errorf("invalid migration filename %q: %w", name, err)
		}

		seq, err := tagFromStem(stem)
		if err != nil {
			return nil, fmt.Errorf("invalid migration filename %q: %w", name, err)
		}

		// Detect duplicate sequence prefixes.
		if prev, dup := seqSeen[seq]; dup {
			return nil, fmt.Errorf("duplicate sequence prefix %04d: files %q and %q", seq, prev, name)
		}
		seqSeen[seq] = name

		files = append(files, MigrationFile{
			FileName: name,
			Tag:      stem,
			SeqNum:   seq,
			Path:     filepath.Join(dir, name),
		})
	}

	// Sort by sequence number, then by filename for determinism.
	sort.Slice(files, func(i, j int) bool {
		if files[i].SeqNum != files[j].SeqNum {
			return files[i].SeqNum < files[j].SeqNum
		}
		return files[i].FileName < files[j].FileName
	})

	return files, nil
}

// ChecksumFile returns the SHA-256 hex digest of the file's byte contents.
func ChecksumFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", path, err)
	}
	return checksumBytes(data), nil
}

// checksumBytes returns the SHA-256 hex digest of b.
func checksumBytes(b []byte) string {
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h)
}

// parseSequenceNumber extracts the sequence number from a migration tag.
// Returns an error if the tag does not begin with a valid integer prefix.
func parseSequenceNumber(tag string) (int, error) {
	return tagFromStem(tag)
}

// MigrateOptions configures the file-based kit.Migrate function.
type MigrateOptions struct {
	// MigrationsDir is the directory containing .sql migration files.
	// Required.
	MigrationsDir string

	// Baseline, when non-empty, marks migration files up to and including the
	// named tag as applied without executing their SQL. All baseline inserts are
	// committed in a single transaction. Must not be used on a fresh database.
	// Subsequent migration files (higher sequence numbers) are applied normally.
	Baseline string

	// SkipSchemaUpgrade disables the automatic ADD COLUMN upgrade for the
	// _grizzle_migrations table. If set and the tag/is_baseline columns are
	// absent, Migrate returns an error immediately.
	SkipSchemaUpgrade bool
}
