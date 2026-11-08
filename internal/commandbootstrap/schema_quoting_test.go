package commandbootstrap

import "testing"

func TestSchemaTableQuotingPreservesNameAndConstraints(t *testing.T) {
	base := schemaObject{Type: "table", Name: "receipt", SQL: "CREATE TABLE `receipt` (`id` text PRIMARY KEY, CHECK(id <> 'other'))"}
	quoted := schemaObject{Type: "table", Name: "receipt", SQL: "CREATE TABLE \"receipt\" (`id` text PRIMARY KEY, CHECK(id <> 'other'))"}
	if normalizeDDL(base) != normalizeDDL(quoted) {
		t.Fatal("aspas equivalentes do nome foram recusadas")
	}
	for _, invalid := range []schemaObject{
		{Type: "table", Name: "different", SQL: quoted.SQL},
		{Type: "table", Name: "receipt", SQL: "CREATE TABLE \"other\" (`id` text PRIMARY KEY, CHECK(id <> 'other'))"},
		{Type: "table", Name: "receipt", SQL: "CREATE TABLE \"receipt\" (`id` text PRIMARY KEY, CHECK(id = 'other'))"},
		{Type: "table", Name: "receipt", SQL: "CREATE TABLE \"receipt\" (`id` integer PRIMARY KEY, CHECK(id <> 'other'))"},
	} {
		if normalizeDDL(base) == normalizeDDL(invalid) {
			t.Fatalf("drift aceito: %+v", invalid)
		}
	}
}
