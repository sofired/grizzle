package expr

// -------------------------------------------------------------------
// PostgreSQL regex expression types (~, ~*, !~, !~*)
// -------------------------------------------------------------------

// regexpExpr represents col ~ pattern, col ~* pattern, col !~ pattern, col !~* pattern.
// op is one of "~", "~*", "!~", "!~*".
type regexpExpr struct {
	ref     colRefer
	op      string
	pattern string
}

func (e regexpExpr) ToSQL(ctx *BuildContext) string {
	return e.ref.colRef(ctx) + " " + e.op + " " + ctx.Add(e.pattern)
}

// -------------------------------------------------------------------
// PostgreSQL full-text search expression types (@@ operator)
// -------------------------------------------------------------------

// ftsMatchExpr represents col @@ tsquery_fn($query) or col @@ tsquery_fn($config, $query).
// tsFn is one of "to_tsquery", "plainto_tsquery", "phraseto_tsquery", "websearch_to_tsquery".
// hasConfig distinguishes the 2-arg form from the 1-arg form; an empty config string with
// hasConfig==true is treated as an explicit (if unusual) config argument, which keeps the
// SQL shape stable and prevents placeholder numbering surprises.
type ftsMatchExpr struct {
	ref       colRefer
	tsFn      string
	config    string
	query     string
	hasConfig bool
}

func (e ftsMatchExpr) ToSQL(ctx *BuildContext) string {
	var tsq string
	if e.hasConfig {
		tsq = e.tsFn + "(" + ctx.Add(e.config) + ", " + ctx.Add(e.query) + ")"
	} else {
		tsq = e.tsFn + "(" + ctx.Add(e.query) + ")"
	}
	return e.ref.colRef(ctx) + " @@ " + tsq
}

// TsvectorExpr represents to_tsvector(col) or to_tsvector($config, col) —
// used in SELECT or as part of an @@ expression.
// hasConfig distinguishes the 2-arg form from the 1-arg form so that an
// empty config string with hasConfig==true still emits the 2-arg SQL,
// keeping placeholder numbering stable.
//
// TsvectorExpr is exported so downstream code can name the type in APIs
// (e.g. store it in a struct field, accept it as a parameter, or return it
// from a helper).
type TsvectorExpr struct {
	config    string
	ref       colRefer
	alias     string
	hasConfig bool
}

func (e TsvectorExpr) renderCore(ctx *BuildContext) string {
	if e.hasConfig {
		return "to_tsvector(" + ctx.Add(e.config) + ", " + e.ref.colRef(ctx) + ")"
	}
	return "to_tsvector(" + e.ref.colRef(ctx) + ")"
}

func (e TsvectorExpr) ToSQL(ctx *BuildContext) string {
	s := e.renderCore(ctx)
	if e.alias != "" {
		s += " AS " + ctx.Quote(e.alias)
	}
	return s
}

func (e TsvectorExpr) colRef(ctx *BuildContext) string { return e.renderCore(ctx) }

// ColumnName implements SelectableColumn.
func (e TsvectorExpr) ColumnName() string {
	if e.alias != "" {
		return e.alias
	}
	return "to_tsvector"
}

// TableName implements SelectableColumn.
func (e TsvectorExpr) TableName() string { return "" }

// As returns a copy with the given SELECT alias.
func (e TsvectorExpr) As(alias string) TsvectorExpr { e.alias = alias; return e }

// Matches returns an @@ expression: to_tsvector(...) @@ to_tsquery($1).
func (e TsvectorExpr) Matches(query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "to_tsquery", query: query}
}

// MatchesWithConfig returns to_tsvector(...) @@ to_tsquery($config, $query).
// config and query are always both bound, keeping the 2-arg SQL shape stable.
func (e TsvectorExpr) MatchesWithConfig(config, query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "to_tsquery", config: config, query: query, hasConfig: true}
}

// MatchesPlain returns to_tsvector(...) @@ plainto_tsquery($1).
func (e TsvectorExpr) MatchesPlain(query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "plainto_tsquery", query: query}
}

// MatchesPlainWithConfig returns to_tsvector(...) @@ plainto_tsquery($config, $query).
// config and query are always both bound, keeping the 2-arg SQL shape stable.
func (e TsvectorExpr) MatchesPlainWithConfig(config, query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "plainto_tsquery", config: config, query: query, hasConfig: true}
}

// MatchesPhrase returns to_tsvector(...) @@ phraseto_tsquery($1).
func (e TsvectorExpr) MatchesPhrase(query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "phraseto_tsquery", query: query}
}

// MatchesPhraseWithConfig returns to_tsvector(...) @@ phraseto_tsquery($config, $query).
// config and query are always both bound, keeping the 2-arg SQL shape stable.
func (e TsvectorExpr) MatchesPhraseWithConfig(config, query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "phraseto_tsquery", config: config, query: query, hasConfig: true}
}

// MatchesWebSearch returns to_tsvector(...) @@ websearch_to_tsquery($1).
func (e TsvectorExpr) MatchesWebSearch(query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "websearch_to_tsquery", query: query}
}

// MatchesWebSearchWithConfig returns to_tsvector(...) @@ websearch_to_tsquery($config, $query).
// config and query are always both bound, keeping the 2-arg SQL shape stable.
func (e TsvectorExpr) MatchesWebSearchWithConfig(config, query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "websearch_to_tsquery", config: config, query: query, hasConfig: true}
}

// ftsMatchExprOnExpr is like ftsMatchExpr but the left side is an arbitrary colRefer
// (e.g. a TsvectorExpr) rather than a plain column reference.
// hasConfig distinguishes the 2-arg tsquery form from the 1-arg form, so that
// empty config strings with hasConfig==true still emit fn($config, $query) and
// keep placeholder numbering stable.
type ftsMatchExprOnExpr struct {
	left      colRefer
	tsFn      string
	config    string
	query     string
	hasConfig bool
}

func (e ftsMatchExprOnExpr) ToSQL(ctx *BuildContext) string {
	// Render the left side first so its args are bound before the tsquery args,
	// preserving left-to-right parameter numbering ($1, $2, ...).
	left := e.left.colRef(ctx)
	var tsq string
	if e.hasConfig {
		tsq = e.tsFn + "(" + ctx.Add(e.config) + ", " + ctx.Add(e.query) + ")"
	} else {
		tsq = e.tsFn + "(" + ctx.Add(e.query) + ")"
	}
	return left + " @@ " + tsq
}

// tsQueryFnExpr represents a standalone tsquery constructor: fn($query) or fn($config, $query).
// config and query are bound in that order so arg positions match the call-site order.
// hasConfig distinguishes the 2-arg form from the 1-arg form; an empty config with
// hasConfig==true still emits fn($config, $query) to keep the SQL shape and placeholder
// numbering stable.
type tsQueryFnExpr struct {
	fn        string
	config    string
	query     string
	hasConfig bool
}

func (e tsQueryFnExpr) ToSQL(ctx *BuildContext) string {
	if e.hasConfig {
		return e.fn + "(" + ctx.Add(e.config) + ", " + ctx.Add(e.query) + ")"
	}
	return e.fn + "(" + ctx.Add(e.query) + ")"
}
