package filemigrate

import (
	"bytes"
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"
)

const (
	statementBreakpointLine = "--> statement-breakpoint"
	validateMigrationSQLOp  = "validate_migration_sql"
)

// migrationSQLDelimiterCandidate identifies one complete physical line whose
// trimmed content is the breakpoint marker. end includes the original LF,
// CRLF, or CR terminator when one is present, so a later execution segmenter
// can remove the candidate line without normalizing surrounding bytes.
//
// These are candidates, not execution-ready delimiters. The execution stage
// must filter marker-like lines inside inactive SQL text such as block comments
// and enforce the empty-segment policy before dispatching statements.
type migrationSQLDelimiterCandidate struct {
	start int
	end   int
}

// validateMigrationSQL validates migration.sql as read-side text without
// mutating or normalizing it. A leading UTF-8 BOM is accepted for read-side
// compatibility; generators must continue to emit BOM-free SQL.
func validateMigrationSQL(migration string, sql []byte) error {
	if !utf8.Valid(sql) {
		return invalidMigrationSQLError(migration, "migration.sql is not valid UTF-8")
	}

	for offset := 0; offset < len(sql); {
		r, size := utf8.DecodeRune(sql[offset:])
		switch {
		case r == 0:
			return invalidMigrationSQLError(
				migration,
				fmt.Sprintf("migration.sql contains NUL at byte offset %d", offset),
			)
		case r == '\ufeff' && offset != 0:
			return invalidMigrationSQLError(
				migration,
				fmt.Sprintf("migration.sql contains a non-leading UTF-8 BOM at byte offset %d", offset),
			)
		case unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r':
			return invalidMigrationSQLError(
				migration,
				fmt.Sprintf("migration.sql contains an unsupported control character at byte offset %d", offset),
			)
		}
		offset += size
	}

	return nil
}

func invalidMigrationSQLError(migration, reason string) error {
	return &Error{
		Code:      CodeInvalidStatementSegmentation,
		Op:        validateMigrationSQLOp,
		Migration: migration,
		Err:       errors.New(reason),
	}
}

// findMigrationSQLDelimiterCandidates finds physical-line marker candidates
// without changing the supplied byte slice. Input must already have passed
// validateMigrationSQL. CRLF and standalone CR are treated like LF only while
// finding line boundaries; the returned spans always refer to original bytes.
func findMigrationSQLDelimiterCandidates(sql []byte) []migrationSQLDelimiterCandidate {
	var candidates []migrationSQLDelimiterCandidate
	for lineStart := 0; lineStart < len(sql); {
		contentEnd := lineStart
		for contentEnd < len(sql) && sql[contentEnd] != '\n' && sql[contentEnd] != '\r' {
			contentEnd++
		}

		lineEnd := contentEnd
		if lineEnd < len(sql) {
			if sql[lineEnd] == '\r' && lineEnd+1 < len(sql) && sql[lineEnd+1] == '\n' {
				lineEnd += 2
			} else {
				lineEnd++
			}
		}

		if bytes.Equal(bytes.TrimSpace(sql[lineStart:contentEnd]), []byte(statementBreakpointLine)) {
			candidates = append(candidates, migrationSQLDelimiterCandidate{start: lineStart, end: lineEnd})
		}
		lineStart = lineEnd
	}
	return candidates
}
