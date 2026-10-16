package database

import (
	"context"
	"reflect"
	"testing"

	"gorm.io/gorm"
)

func TestMessageRevisionsPublishedUpgradesAndSecondBoot(t *testing.T) {
	for _, release := range []string{"0.1.9", "0.2.0", "0.3.0", "0.4.0", "0.5.0"} {
		t.Run(release, func(t *testing.T) {
			database := loadPublishedReleaseFixture(t, release)
			runCurrentUpgrade(t, database)
			if release == "0.1.9" {
				versions, err := appliedMigrationVersions(database)
				if err != nil || versions[19] || versions[30] {
					t.Fatalf("v30 must wait for the deferred v19 rebuild: %v", err)
				}
				owner := User{Username: "revision-upgrade-owner", PasswordHash: "synthetic", Role: UserRoleAdmin, IsActive: true}
				if err := database.Create(&owner).Error; err != nil {
					t.Fatal(err)
				}
				previous := db
				db = database
				err = AdoptLegacyData(owner.ID)
				db = previous
				if err != nil {
					t.Fatal(err)
				}
				runCurrentUpgrade(t, database)
			}
			var messages []ChatMessage
			if err := database.Find(&messages).Error; err != nil || len(messages) == 0 {
				t.Fatalf("published messages missing: %v", err)
			}
			revisions := make(map[string]string, len(messages))
			for _, message := range messages {
				revision, err := messageStoredRevisionTx(context.Background(), database, message.ID)
				if err != nil {
					t.Fatal(err)
				}
				revisions[message.ID] = revision
			}
			// Repeat the real pre/AutoMigrate/post boot, not just the migration
			// function. Triggers and existing nonces must survive it unchanged.
			runCurrentUpgrade(t, database)
			for _, message := range messages {
				revision, err := messageStoredRevisionTx(context.Background(), database, message.ID)
				if err != nil || revision != revisions[message.ID] {
					t.Fatalf("second boot replaced revision: %v", err)
				}
				for i := 0; i < 2; i++ {
					if err := database.Model(&ChatMessage{}).Where("id = ?", message.ID).
						UpdateColumn("pinned", gorm.Expr("NOT pinned")).Error; err != nil {
						t.Fatal(err)
					}
				}
				changed, err := messageStoredRevisionTx(context.Background(), database, message.ID)
				if err != nil || changed == revision {
					t.Fatalf("upgrade failed to invalidate ABA: %v", err)
				}
				var after ChatMessage
				if err := database.First(&after, "id = ?", message.ID).Error; err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(after, message) {
					t.Fatal("revision maintenance changed the message payload or timestamps")
				}
			}
		})
	}
}

func TestMessageRevisionMigrationFailureRollsBackSchemaAndCanRetry(t *testing.T) {
	database := newMigratorTestDB(t)
	// SQLite's legacy TEXT PRIMARY KEY permits NULL without NOT NULL.
	// A corrupt source must fail the backfill without a half-installed schema.
	if err := database.Exec("CREATE TABLE chat_messages (id TEXT PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("INSERT INTO chat_messages(id) VALUES (NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateMessageRevisions(database); err == nil {
		t.Fatal("invalid legacy message unexpectedly migrated")
	}
	if database.Migrator().HasTable("chat_message_revisions") {
		t.Fatal("failed migration left a partially installed table")
	}
	var triggers int64
	if err := database.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'trigger' AND name LIKE 'chat_message_revision_%'").Scan(&triggers).Error; err != nil || triggers != 0 {
		t.Fatalf("failed migration left triggers: %d, %v", triggers, err)
	}
	if err := database.Exec("UPDATE chat_messages SET id = 'repaired' WHERE id IS NULL").Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateMessageRevisions(database); err != nil {
		t.Fatal(err)
	}
	if _, err := messageStoredRevisionTx(context.Background(), database, "repaired"); err != nil {
		t.Fatal(err)
	}
}
