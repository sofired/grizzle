package query

import "github.com/sofired/grizzle/expr"

// RelationKind identifies the cardinality of a relation.
type RelationKind string

const (
	RelBelongsTo RelationKind = "belongs_to" // many-to-one  (FK lives on this table)
	RelHasMany   RelationKind = "has_many"   // one-to-many  (FK lives on foreign table)
	RelHasOne    RelationKind = "has_one"    // one-to-one   (FK lives on foreign table)
)

// RelationDef describes a typed relationship between two tables.
// It bundles the foreign table reference and the ON expression so that
// query builders can construct JOIN clauses without repeating the condition.
//
// Example schema-level definition:
//
//	var UserRealm = query.BelongsTo("realm",
//	    RealmsT,
//	    RealmsT.ID.EQ(UsersT.RealmID),
//	)
//
// Example query usage:
//
//	query.Select(UsersT.ID, UsersT.Username, RealmsT.Name).
//	    From(UsersT).
//	    JoinRel(UserRealm)
type RelationDef struct {
	Kind  RelationKind
	Name  string
	Table TableSource     // the foreign / associated table
	On    expr.Expression // JOIN … ON condition
}

// BelongsTo defines a many-to-one relation: the FK column lives on the
// calling table and references a row in the foreign table.
//
//	var UserRealm = query.BelongsTo("realm", RealmsT, RealmsT.ID.EQ(UsersT.RealmID))
func BelongsTo(name string, table TableSource, on expr.Expression) RelationDef {
	return RelationDef{Kind: RelBelongsTo, Name: name, Table: table, On: on}
}

// HasMany defines a one-to-many relation: the FK column lives on the foreign
// table and references the current table. Results must be aggregated in Go.
//
//	var RealmUsers = query.HasMany("users", UsersT, UsersT.RealmID.EQ(RealmsT.ID))
func HasMany(name string, table TableSource, on expr.Expression) RelationDef {
	return RelationDef{Kind: RelHasMany, Name: name, Table: table, On: on}
}

// HasOne defines a one-to-one relation: like HasMany but the foreign table
// holds at most one matching row for each row in the current table.
//
//	var UserProfile = query.HasOne("profile", ProfilesT, ProfilesT.UserID.EQ(UsersT.ID))
func HasOne(name string, table TableSource, on expr.Expression) RelationDef {
	return RelationDef{Kind: RelHasOne, Name: name, Table: table, On: on}
}
