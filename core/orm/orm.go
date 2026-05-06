// Package orm fornece um ORM leve e fluente para o Kyrux Framework.
//
// Design: generics + reflection cacheada + SQL explícito com placeholders.
// Suporta multi-database e multi-tenant via database.DB.Schema.
//
// Uso básico (conexão do registry global):
//
//	// SELECT
//	users, err := orm.From[User]("default").Where("active = ?", true).OrderBy("id DESC").Limit(20).All()
//
//	// INSERT
//	user := User{Name: "Maria"}
//	err = orm.Create(db, &user)
//
//	// UPDATE
//	err = orm.From[User]("default").Where("id = ?", 1).Update(map[string]any{"name": "Carlos"})
//
//	// DELETE
//	err = orm.From[User]("default").Where("id = ?", 1).Delete()
//
//	// Multi-database
//	logs, err := orm.From[Log]("analytics").OrderBy("created_at DESC").Limit(100).All()
//
//	// Multi-tenant (schema por request — use FromDB com WithSchema)
//	db := fw.DB.Use().WithSchema("tenant_abc")
//	users, err := orm.FromDB[User](db).All()
package orm

import "kyrux/core/database"

// From inicia um Query builder para o tipo T usando uma conexão do registry global.
// Registre as conexões com orm.LoadDatabases() no bootstrap antes de usar.
//
//	users, err := orm.From[User]("default").Where("active = ?", true).All()
//	logs,  err := orm.From[Log]("analytics").Limit(100).All()
func From[T any](connName string) *Query[T] {
	return &Query[T]{
		db:   DB(connName),
		meta: metaOf[T](),
	}
}

// FromDB inicia um Query builder usando uma conexão explícita.
// Use quando o código já recebe *database.DB como parâmetro (auth, CLI, multi-tenant).
//
//	users, err := orm.FromDB[User](db).Where("active = ?", true).All()
func FromDB[T any](db *database.DB) *Query[T] {
	return &Query[T]{
		db:   db,
		meta: metaOf[T](),
	}
}
