// Package testschema defines a small representative schema used in G-rizzle's
// own tests. It mirrors the patterns from the uncloak-identity project:
// soft-delete, partial indexes, composite FKs, JSONB columns, UUID PKs.
//
// This package plays a dual role:
//   1. It's used by tests to verify SQL generation without a live database.
//   2. It serves as the canonical example of the G-rizzle schema DSL.
package testschema

import (
	"time"

	"github.com/google/uuid"
	pg "github.com/grizzle-orm/grizzle/schema/pg"
	"github.com/grizzle-orm/grizzle/expr"
	"github.com/grizzle-orm/grizzle/query"
)

// -------------------------------------------------------------------
// Schema definitions
// -------------------------------------------------------------------

var Realms = pg.Table("realms",
	pg.C("id",           pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("name",         pg.Varchar(255).NotNull()),
	pg.C("display_name", pg.Varchar(255)),
	pg.C("enabled",      pg.Boolean().NotNull().Default(true)),
	pg.C("settings",     pg.JSONB().DefaultEmpty()),
	pg.C("created_at",   pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
	pg.C("updated_at",   pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
).WithConstraints(func(t pg.TableRef) []pg.Constraint {
	return []pg.Constraint{
		pg.UniqueIndex("realms_name_idx").On(t.Col("name")).Build(),
		pg.Check("settings_size_check", "pg_column_size(settings) <= 65536"),
	}
})

var Users = pg.Table("users",
	pg.C("id",             pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("realm_id",       pg.UUID().NotNull().References("realms", "id", pg.OnDelete(pg.FKActionRestrict))),
	pg.C("username",       pg.Varchar(255).NotNull()),
	pg.C("email",          pg.Varchar(255)),
	pg.C("email_verified", pg.Boolean().NotNull().Default(false)),
	pg.C("enabled",        pg.Boolean().NotNull().Default(true)),
	pg.C("attributes",     pg.JSONB().DefaultEmpty()),
	pg.C("created_at",     pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
	pg.C("updated_at",     pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
	pg.C("deleted_at",     pg.Timestamp().WithTimezone()),
	pg.C("purged_at",      pg.Timestamp().WithTimezone()),
).WithConstraints(func(t pg.TableRef) []pg.Constraint {
	return []pg.Constraint{
		pg.UniqueIndex("users_realm_username_idx").
			On(t.Col("realm_id"), t.Col("username")).
			Where(pg.IsNull(t.Col("deleted_at"))).
			Build(),
		pg.Index("users_realm_id_idx").On(t.Col("realm_id")).Build(),
		pg.UniqueIndex("users_realm_email_unique_idx").
			On(t.Col("realm_id"), t.Col("email")).
			Where(`email IS NOT NULL AND deleted_at IS NULL`).
			Build(),
		pg.Check("attributes_size_check", "pg_column_size(attributes) <= 65536"),
	}
})

// -------------------------------------------------------------------
// Generated table types (in production these come from `g-rizzle gen`)
// -------------------------------------------------------------------

// RealmsTable is what the code generator would produce for the realms table.
type RealmsTable struct {
	ID          expr.UUIDColumn
	Name        expr.StringColumn
	DisplayName expr.StringColumn
	Enabled     expr.BoolColumn
	Settings    expr.JSONBColumn[map[string]any]
	CreatedAt   expr.TimestampColumn
	UpdatedAt   expr.TimestampColumn
}

func (RealmsTable) GRizTableName() string  { return "realms" }
func (RealmsTable) GRizTableAlias() string { return "realms" }

// RealmsT is the singleton table handle used in queries.
var RealmsT = RealmsTable{
	ID:          expr.UUIDColumn{ColBase: expr.ColBase{TableAlias: "realms", ColName: "id"}},
	Name:        expr.StringColumn{ColBase: expr.ColBase{TableAlias: "realms", ColName: "name"}},
	DisplayName: expr.StringColumn{ColBase: expr.ColBase{TableAlias: "realms", ColName: "display_name"}},
	Enabled:     expr.BoolColumn{ColBase: expr.ColBase{TableAlias: "realms", ColName: "enabled"}},
	Settings:    expr.JSONBColumn[map[string]any]{ColBase: expr.ColBase{TableAlias: "realms", ColName: "settings"}},
	CreatedAt:   expr.TimestampColumn{ColBase: expr.ColBase{TableAlias: "realms", ColName: "created_at"}},
	UpdatedAt:   expr.TimestampColumn{ColBase: expr.ColBase{TableAlias: "realms", ColName: "updated_at"}},
}

// UsersTable is what the code generator would produce for the users table.
type UsersTable struct {
	ID            expr.UUIDColumn
	RealmID       expr.UUIDColumn
	Username      expr.StringColumn
	Email         expr.StringColumn
	EmailVerified expr.BoolColumn
	Enabled       expr.BoolColumn
	Attributes    expr.JSONBColumn[map[string]any]
	CreatedAt     expr.TimestampColumn
	UpdatedAt     expr.TimestampColumn
	DeletedAt     expr.TimestampColumn
	PurgedAt      expr.TimestampColumn
}

func (UsersTable) GRizTableName() string  { return "users" }
func (UsersTable) GRizTableAlias() string { return "users" }

var UsersT = UsersTable{
	ID:            expr.UUIDColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "id"}},
	RealmID:       expr.UUIDColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "realm_id"}},
	Username:      expr.StringColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "username"}},
	Email:         expr.StringColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "email"}},
	EmailVerified: expr.BoolColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "email_verified"}},
	Enabled:       expr.BoolColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "enabled"}},
	Attributes:    expr.JSONBColumn[map[string]any]{ColBase: expr.ColBase{TableAlias: "users", ColName: "attributes"}},
	CreatedAt:     expr.TimestampColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "created_at"}},
	UpdatedAt:     expr.TimestampColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "updated_at"}},
	DeletedAt:     expr.TimestampColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "deleted_at"}},
	PurgedAt:      expr.TimestampColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "purged_at"}},
}

// -------------------------------------------------------------------
// Select / Insert model types (in production these come from `g-rizzle gen`)
// -------------------------------------------------------------------

// RealmSelect is the full select model for the realms table.
// Nullable columns are pointer types.
type RealmSelect struct {
	ID          uuid.UUID      `db:"id"`
	Name        string         `db:"name"`
	DisplayName *string        `db:"display_name"`
	Enabled     bool           `db:"enabled"`
	Settings    map[string]any `db:"settings"`
	CreatedAt   time.Time      `db:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at"`
}

// RealmInsert is the insert model. Optional fields (those with DB defaults
// or that are nullable) use pointer types so the caller can omit them.
type RealmInsert struct {
	ID          *uuid.UUID     `db:"id,omitempty"`    // optional — default gen_random_uuid()
	Name        string         `db:"name"`            // required
	DisplayName *string        `db:"display_name,omitempty"`
	Enabled     *bool          `db:"enabled,omitempty"` // optional — default true
	Settings    map[string]any `db:"settings,omitempty"`
	CreatedAt   *time.Time     `db:"created_at,omitempty"`
	UpdatedAt   *time.Time     `db:"updated_at,omitempty"`
}

// UserSelect is the full select model for the users table.
type UserSelect struct {
	ID            uuid.UUID      `db:"id"`
	RealmID       uuid.UUID      `db:"realm_id"`
	Username      string         `db:"username"`
	Email         *string        `db:"email"`
	EmailVerified bool           `db:"email_verified"`
	Enabled       bool           `db:"enabled"`
	Attributes    map[string]any `db:"attributes"`
	CreatedAt     time.Time      `db:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at"`
	DeletedAt     *time.Time     `db:"deleted_at"`
	PurgedAt      *time.Time     `db:"purged_at"`
}

// UserInsert is the insert model for the users table.
type UserInsert struct {
	ID            *uuid.UUID     `db:"id,omitempty"`
	RealmID       uuid.UUID      `db:"realm_id"`
	Username      string         `db:"username"`
	Email         *string        `db:"email,omitempty"`
	EmailVerified *bool          `db:"email_verified,omitempty"`
	Enabled       *bool          `db:"enabled,omitempty"`
	Attributes    map[string]any `db:"attributes,omitempty"`
}

// UserUpdate is used with UpdateBuilder.SetStruct — only non-nil fields are SET.
type UserUpdate struct {
	Username  *string    `db:"username"`
	Email     *string    `db:"email"`
	Enabled   *bool      `db:"enabled"`
	DeletedAt *time.Time `db:"deleted_at"`
	PurgedAt  *time.Time `db:"purged_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

// -------------------------------------------------------------------
// Relation definitions
// -------------------------------------------------------------------

// UserRealm navigates from a user to its parent realm.
// Use with JoinRel / InnerJoinRel on a query rooted at UsersT.
var UserRealm = query.BelongsTo("realm",
	RealmsT,
	RealmsT.ID.EQCol(UsersT.RealmID),
)

// RealmUsers navigates from a realm to all its users.
// Use with JoinRel / InnerJoinRel on a query rooted at RealmsT.
var RealmUsers = query.HasMany("users",
	UsersT,
	UsersT.RealmID.EQCol(RealmsT.ID),
)
