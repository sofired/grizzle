package codegen

import (
	"strings"
	"unicode"
)

// commonAcronyms lists word segments that should be fully uppercased per Go naming conventions.
// e.g. "id" → "ID", "url" → "URL", "realm_id" → "RealmID"
var commonAcronyms = map[string]string{
	"id":    "ID",
	"url":   "URL",
	"uri":   "URI",
	"api":   "API",
	"uid":   "UID",
	"uuid":  "UUID",
	"http":  "HTTP",
	"https": "HTTPS",
	"ip":    "IP",
	"ttl":   "TTL",
	"sql":   "SQL",
	"json":  "JSON",
	"xml":   "XML",
	"html":  "HTML",
	"csv":   "CSV",
	"eof":   "EOF",
	"db":    "DB",
}

// snakeToPascal converts "display_name" → "DisplayName", "realm_id" → "RealmID".
func snakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		if acronym, ok := commonAcronyms[strings.ToLower(p)]; ok {
			b.WriteString(acronym)
		} else {
			runes := []rune(p)
			runes[0] = unicode.ToUpper(runes[0])
			b.WriteString(string(runes))
		}
	}
	return b.String()
}

// snakeToCamel converts "display_name" → "displayName".
func snakeToCamel(s string) string {
	p := snakeToPascal(s)
	if p == "" {
		return p
	}
	runes := []rune(p)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// singular returns a naive singular form of a plural table name:
// "users" → "User", "realms" → "Realm", "addresses" → "Address".
// This is intentionally simple — edge cases can be handled with a name map.
func singular(name string) string {
	pascal := snakeToPascal(name)
	switch {
	case strings.HasSuffix(pascal, "ses") || strings.HasSuffix(pascal, "xes") ||
		strings.HasSuffix(pascal, "zes") || strings.HasSuffix(pascal, "shes") ||
		strings.HasSuffix(pascal, "ches"):
		return pascal[:len(pascal)-2] // "Addresses" → "Address"
	case strings.HasSuffix(pascal, "ies"):
		return pascal[:len(pascal)-3] + "y" // "Countries" → "Country"
	case strings.HasSuffix(pascal, "s") && !strings.HasSuffix(pascal, "ss"):
		return pascal[:len(pascal)-1] // "Users" → "User"
	default:
		return pascal
	}
}

// tableTypeName returns the Go type name for the table handle struct.
// "users" → "UsersTable"
func tableTypeName(tableName string) string {
	return snakeToPascal(tableName) + "Table"
}

// tableSingletonName returns the Go var name for the table singleton.
// "users" → "UsersT"
func tableSingletonName(tableName string) string {
	return snakeToPascal(tableName) + "T"
}

// selectModelName returns the Go struct name for the Select model.
// "users" → "UserSelect" (singular)
func selectModelName(tableName string) string {
	return singular(tableName) + "Select"
}

// insertModelName returns the Go struct name for the Insert model.
func insertModelName(tableName string) string {
	return singular(tableName) + "Insert"
}

// updateModelName returns the Go struct name for the Update model.
func updateModelName(tableName string) string {
	return singular(tableName) + "Update"
}

// genFileName returns the output file name for a table's generated code.
// "users" → "users_gen.go"
func genFileName(tableName string) string {
	return tableName + "_gen.go"
}

