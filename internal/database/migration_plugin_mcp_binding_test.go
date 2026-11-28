package database

import "testing"

func TestMigrationPreservesMCPSourceAndAllowsConsumers(t *testing.T) {
	db := newMigratorTestDB(t)
	for _, sql := range []string{
		"CREATE TABLE plugin_mcp_bindings (installation_id TEXT, component_key TEXT, digest TEXT NOT NULL, server_id TEXT NOT NULL, slug TEXT NOT NULL, config_digest TEXT NOT NULL, PRIMARY KEY (installation_id,component_key))",
		"CREATE UNIQUE INDEX idx_plugin_mcp_bindings_server_id ON plugin_mcp_bindings(server_id)",
		"INSERT INTO plugin_mcp_bindings VALUES ('source','search','v1','server','search','config')",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AutoMigrate(&PluginMCPBinding{}); err != nil {
		t.Fatal(err)
	}
	for _, migration := range schemaMigrations {
		if migration.Version == 51 {
			if err := migration.Run(db); err != nil {
				t.Fatal(err)
			}
		}
	}
	var source PluginMCPBinding
	if err := db.Where("installation_id=?", "source").Take(&source).Error; err != nil || source.ConsumerOnly || source.ConfigDigest != "config" {
		t.Fatal(source, err)
	}
	for _, id := range []string{"one", "two"} {
		row := source
		row.InstallationID, row.ConsumerOnly = id, true
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	source.InstallationID = "other"
	if err := db.Create(&source).Error; err == nil {
		t.Fatal("dois fornecedores para o mesmo servidor MCP")
	}
}
