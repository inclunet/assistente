package commandbootstrap

import "testing"

func TestSchemaNormalizationOnlyIgnoresDefinitionOrder(t *testing.T) {
	base:=schemaObject{Type:"table",SQL:`CREATE TABLE t (id TEXT, value TEXT CHECK(value IN ('a,b','x(y)')), CONSTRAINT c CHECK(id <> ''))`}
	reordered:=schemaObject{Type:"table",SQL:`CREATE TABLE t (CONSTRAINT c CHECK(id <> ''), value TEXT CHECK(value IN ('a,b','x(y)')), id TEXT)`}
	if normalizeDDL(base)!=normalizeDDL(reordered) { t.Fatal("ordem de constraints alterou contrato") }
	for _,sql:=range []string{
		`CREATE TABLE t (id TEXT, value TEXT CHECK(value IN ('a,b','x(y)')), CONSTRAINT c CHECK(id = ''))`,
		`CREATE TABLE t (id INTEGER, value TEXT CHECK(value IN ('a,b','x(y)')), CONSTRAINT c CHECK(id <> ''))`,
		`CREATE TABLE t (id TEXT, value TEXT CHECK(value IN ('a, b','x(y)')), CONSTRAINT c CHECK(id <> ''))`,
	} { if normalizeDDL(base)==normalizeDDL(schemaObject{Type:"table",SQL:sql}) { t.Fatal("semântica diferente aceita") } }
	if normalizeDDL(schemaObject{Type:"index",SQL:"CREATE INDEX i ON t (a,b)"})==normalizeDDL(schemaObject{Type:"index",SQL:"CREATE INDEX i ON t (b,a)"}) { t.Fatal("ordem de índice perdida") }
}
