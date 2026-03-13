package expr_test

import (
	"testing"
	"time"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
)

// newCtx returns a fresh Postgres BuildContext for testing.
func newCtx() *expr.BuildContext {
	return expr.NewBuildContext(dialect.Postgres)
}

// colBase returns a ColBase with the given table and column name.
func dateCol(table, col string) expr.DateColumn {
	return expr.DateColumn{ColBase: expr.ColBase{TableAlias: table, ColName: col}}
}

func bytesCol(table, col string) expr.BytesColumn {
	return expr.BytesColumn{ColBase: expr.ColBase{TableAlias: table, ColName: col}}
}

func intervalCol(table, col string) expr.IntervalColumn {
	return expr.IntervalColumn{ColBase: expr.ColBase{TableAlias: table, ColName: col}}
}

func enumCol(table, col string) expr.EnumColumn {
	return expr.EnumColumn{ColBase: expr.ColBase{TableAlias: table, ColName: col}}
}

func arrayCol(table, col string) expr.ArrayColumn {
	return expr.ArrayColumn{ColBase: expr.ColBase{TableAlias: table, ColName: col}}
}

func inetCol(table, col string) expr.InetColumn {
	return expr.InetColumn{ColBase: expr.ColBase{TableAlias: table, ColName: col}}
}

func tsvectorCol(table, col string) expr.TsvectorColumn {
	return expr.TsvectorColumn{ColBase: expr.ColBase{TableAlias: table, ColName: col}}
}

// ---------------------------------------------------------------------------
// DateColumn
// ---------------------------------------------------------------------------

func TestDateColumn_EQ(t *testing.T) {
	c := dateCol("events", "published_on")
	ctx := newCtx()
	now := time.Now()
	sql := c.EQ(now).ToSQL(ctx)
	want := `"events"."published_on" = $1`
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
	if len(ctx.Args()) != 1 {
		t.Errorf("expected 1 arg, got %d", len(ctx.Args()))
	}
}

func TestDateColumn_GT(t *testing.T) {
	c := dateCol("events", "published_on")
	ctx := newCtx()
	sql := c.GT(time.Now()).ToSQL(ctx)
	if sql != `"events"."published_on" > $1` {
		t.Errorf("unexpected: %q", sql)
	}
}

func TestDateColumn_Between(t *testing.T) {
	c := dateCol("events", "published_on")
	ctx := newCtx()
	lo := time.Now().AddDate(0, -1, 0)
	hi := time.Now()
	sql := c.Between(lo, hi).ToSQL(ctx)
	if sql != `"events"."published_on" BETWEEN $1 AND $2` {
		t.Errorf("unexpected: %q", sql)
	}
}

func TestDateColumn_EQCol(t *testing.T) {
	c1 := dateCol("orders", "order_date")
	c2 := dateCol("invoices", "invoice_date")
	ctx := newCtx()
	sql := c1.EQCol(c2).ToSQL(ctx)
	want := `"orders"."order_date" = "invoices"."invoice_date"`
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
}

func TestDateColumn_IsNull(t *testing.T) {
	c := dateCol("events", "deleted_on")
	ctx := newCtx()
	sql := c.IsNull().ToSQL(ctx)
	if sql != `"events"."deleted_on" IS NULL` {
		t.Errorf("unexpected: %q", sql)
	}
}

// ---------------------------------------------------------------------------
// BytesColumn
// ---------------------------------------------------------------------------

func TestBytesColumn_EQ(t *testing.T) {
	c := bytesCol("files", "content")
	ctx := newCtx()
	sql := c.EQ([]byte("hello")).ToSQL(ctx)
	if sql != `"files"."content" = $1` {
		t.Errorf("unexpected: %q", sql)
	}
}

func TestBytesColumn_NEQ(t *testing.T) {
	c := bytesCol("files", "content")
	ctx := newCtx()
	sql := c.NEQ([]byte("hello")).ToSQL(ctx)
	if sql != `"files"."content" <> $1` {
		t.Errorf("unexpected: %q", sql)
	}
}

func TestBytesColumn_EQCol(t *testing.T) {
	c1 := bytesCol("versions", "hash")
	c2 := bytesCol("blobs", "sha")
	ctx := newCtx()
	sql := c1.EQCol(c2).ToSQL(ctx)
	want := `"versions"."hash" = "blobs"."sha"`
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
}

// ---------------------------------------------------------------------------
// IntervalColumn
// ---------------------------------------------------------------------------

func TestIntervalColumn_EQ(t *testing.T) {
	c := intervalCol("jobs", "duration")
	ctx := newCtx()
	sql := c.EQ("1 day").ToSQL(ctx)
	if sql != `"jobs"."duration" = $1` {
		t.Errorf("unexpected: %q", sql)
	}
}

func TestIntervalColumn_NEQ(t *testing.T) {
	c := intervalCol("jobs", "duration")
	ctx := newCtx()
	sql := c.NEQ("1 hour").ToSQL(ctx)
	if sql != `"jobs"."duration" <> $1` {
		t.Errorf("unexpected: %q", sql)
	}
}

// ---------------------------------------------------------------------------
// EnumColumn
// ---------------------------------------------------------------------------

func TestEnumColumn_EQ(t *testing.T) {
	c := enumCol("orders", "status")
	ctx := newCtx()
	sql := c.EQ("pending").ToSQL(ctx)
	if sql != `"orders"."status" = $1` {
		t.Errorf("unexpected: %q", sql)
	}
}

func TestEnumColumn_In(t *testing.T) {
	c := enumCol("orders", "status")
	ctx := newCtx()
	sql := c.In("pending", "confirmed").ToSQL(ctx)
	if sql != `"orders"."status" IN ($1, $2)` {
		t.Errorf("unexpected: %q", sql)
	}
}

func TestEnumColumn_In_Empty(t *testing.T) {
	c := enumCol("orders", "status")
	ctx := newCtx()
	sql := c.In().ToSQL(ctx)
	if sql != "FALSE" {
		t.Errorf("expected FALSE for empty In(), got %q", sql)
	}
}

func TestEnumColumn_NotIn(t *testing.T) {
	c := enumCol("orders", "status")
	ctx := newCtx()
	sql := c.NotIn("cancelled").ToSQL(ctx)
	if sql != `"orders"."status" NOT IN ($1)` {
		t.Errorf("unexpected: %q", sql)
	}
}

func TestEnumColumn_NotIn_Empty(t *testing.T) {
	c := enumCol("orders", "status")
	ctx := newCtx()
	sql := c.NotIn().ToSQL(ctx)
	if sql != "TRUE" {
		t.Errorf("expected TRUE for empty NotIn(), got %q", sql)
	}
}

// ---------------------------------------------------------------------------
// ArrayColumn
// ---------------------------------------------------------------------------

func TestArrayColumn_EQCol(t *testing.T) {
	c1 := arrayCol("posts", "tags")
	c2 := arrayCol("drafts", "labels")
	ctx := newCtx()
	sql := c1.EQCol(c2).ToSQL(ctx)
	want := `"posts"."tags" = "drafts"."labels"`
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
}

func TestArrayColumn_Contains(t *testing.T) {
	c := arrayCol("posts", "tags")
	ctx := newCtx()
	sql := c.Contains([]string{"go", "postgres"}).ToSQL(ctx)
	if sql != `"posts"."tags" @> $1` {
		t.Errorf("unexpected: %q", sql)
	}
}

func TestArrayColumn_Overlaps(t *testing.T) {
	c := arrayCol("posts", "tags")
	ctx := newCtx()
	sql := c.Overlaps([]string{"go"}).ToSQL(ctx)
	if sql != `"posts"."tags" && $1` {
		t.Errorf("unexpected: %q", sql)
	}
}

func TestArrayColumn_IsNull(t *testing.T) {
	c := arrayCol("posts", "tags")
	ctx := newCtx()
	sql := c.IsNull().ToSQL(ctx)
	if sql != `"posts"."tags" IS NULL` {
		t.Errorf("unexpected: %q", sql)
	}
}

// ---------------------------------------------------------------------------
// InetColumn
// ---------------------------------------------------------------------------

func TestInetColumn_EQ(t *testing.T) {
	c := inetCol("sessions", "client_ip")
	ctx := newCtx()
	sql := c.EQ("192.168.1.1").ToSQL(ctx)
	if sql != `"sessions"."client_ip" = $1` {
		t.Errorf("unexpected: %q", sql)
	}
}

func TestInetColumn_NEQ(t *testing.T) {
	c := inetCol("sessions", "client_ip")
	ctx := newCtx()
	sql := c.NEQ("127.0.0.1").ToSQL(ctx)
	if sql != `"sessions"."client_ip" <> $1` {
		t.Errorf("unexpected: %q", sql)
	}
}

// ---------------------------------------------------------------------------
// TsvectorColumn
// ---------------------------------------------------------------------------

func TestTsvectorColumn_EQCol(t *testing.T) {
	c1 := tsvectorCol("a", "vec")
	c2 := tsvectorCol("b", "vec")
	ctx := newCtx()
	sql := c1.EQCol(c2).ToSQL(ctx)
	want := `"a"."vec" = "b"."vec"`
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
}

