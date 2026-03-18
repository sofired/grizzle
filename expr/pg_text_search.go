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
// config is the optional text search configuration (e.g. "english"). Empty string means no config arg.
type ftsMatchExpr struct {
	ref    colRefer
	tsFn   string
	config string
	query  string
}

func (e ftsMatchExpr) ToSQL(ctx *BuildContext) string {
	var tsq string
	if e.config != "" {
		tsq = e.tsFn + "(" + ctx.Add(e.config) + ", " + ctx.Add(e.query) + ")"
	} else {
		tsq = e.tsFn + "(" + ctx.Add(e.query) + ")"
	}
	return e.ref.colRef(ctx) + " @@ " + tsq
}

// toTsvectorExpr represents to_tsvector(col) or to_tsvector($config, col) —
// used in SELECT or as part of an @@ expression.
// config may be empty, in which case the one-arg form is emitted.
type toTsvectorExpr struct {
	config string
	ref    colRefer
	alias  string
}

func (e toTsvectorExpr) renderCore(ctx *BuildContext) string {
	if e.config != "" {
		return "to_tsvector(" + ctx.Add(e.config) + ", " + e.ref.colRef(ctx) + ")"
	}
	return "to_tsvector(" + e.ref.colRef(ctx) + ")"
}

func (e toTsvectorExpr) ToSQL(ctx *BuildContext) string {
	s := e.renderCore(ctx)
	if e.alias != "" {
		s += " AS " + ctx.Quote(e.alias)
	}
	return s
}

func (e toTsvectorExpr) colRef(ctx *BuildContext) string { return e.renderCore(ctx) }

// ColumnName implements SelectableColumn.
func (e toTsvectorExpr) ColumnName() string {
	if e.alias != "" {
		return e.alias
	}
	return "to_tsvector"
}

// TableName implements SelectableColumn.
func (e toTsvectorExpr) TableName() string { return "" }

// As returns a copy with the given SELECT alias.
func (e toTsvectorExpr) As(alias string) toTsvectorExpr { e.alias = alias; return e }

// Matches returns an @@ expression: to_tsvector(...) @@ to_tsquery($1).
func (e toTsvectorExpr) Matches(query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "to_tsquery", query: query}
}

// MatchesWithConfig returns to_tsvector(...) @@ to_tsquery($config, $query).
func (e toTsvectorExpr) MatchesWithConfig(config, query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "to_tsquery", config: config, query: query}
}

// MatchesPlain returns to_tsvector(...) @@ plainto_tsquery($1).
func (e toTsvectorExpr) MatchesPlain(query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "plainto_tsquery", query: query}
}

// MatchesPlainWithConfig returns to_tsvector(...) @@ plainto_tsquery($config, $query).
func (e toTsvectorExpr) MatchesPlainWithConfig(config, query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "plainto_tsquery", config: config, query: query}
}

// MatchesPhrase returns to_tsvector(...) @@ phraseto_tsquery($1).
func (e toTsvectorExpr) MatchesPhrase(query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "phraseto_tsquery", query: query}
}

// MatchesPhraseWithConfig returns to_tsvector(...) @@ phraseto_tsquery($config, $query).
func (e toTsvectorExpr) MatchesPhraseWithConfig(config, query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "phraseto_tsquery", config: config, query: query}
}

// MatchesWebSearch returns to_tsvector(...) @@ websearch_to_tsquery($1).
func (e toTsvectorExpr) MatchesWebSearch(query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "websearch_to_tsquery", query: query}
}

// MatchesWebSearchWithConfig returns to_tsvector(...) @@ websearch_to_tsquery($config, $query).
func (e toTsvectorExpr) MatchesWebSearchWithConfig(config, query string) Expression {
	return ftsMatchExprOnExpr{left: e, tsFn: "websearch_to_tsquery", config: config, query: query}
}

// ftsMatchExprOnExpr is like ftsMatchExpr but the left side is an arbitrary colRefer
// (e.g. a toTsvectorExpr) rather than a plain column reference.
type ftsMatchExprOnExpr struct {
	left   colRefer
	tsFn   string
	config string
	query  string
}

func (e ftsMatchExprOnExpr) ToSQL(ctx *BuildContext) string {
	// Render the left side first so its args are bound before the tsquery args,
	// preserving left-to-right parameter numbering ($1, $2, ...).
	left := e.left.colRef(ctx)
	var tsq string
	if e.config != "" {
		tsq = e.tsFn + "(" + ctx.Add(e.config) + ", " + ctx.Add(e.query) + ")"
	} else {
		tsq = e.tsFn + "(" + ctx.Add(e.query) + ")"
	}
	return left + " @@ " + tsq
}

// tsQueryFnExpr represents a standalone tsquery constructor: fn($query) or fn($config, $query).
// config and query are bound in that order so arg positions match the call-site order.
type tsQueryFnExpr struct {
	fn     string
	config string
	query  string
}

func (e tsQueryFnExpr) ToSQL(ctx *BuildContext) string {
	if e.config != "" {
		return e.fn + "(" + ctx.Add(e.config) + ", " + ctx.Add(e.query) + ")"
	}
	return e.fn + "(" + ctx.Add(e.query) + ")"
}
