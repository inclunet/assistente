package commandconfig

import (
	"context"
	"errors"
	"testing"
)

func TestSnapshotStampDoesNotAliasWorkspacePointers(t *testing.T) {
	db := storeTestDB(t)
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	user, workspace := storeTestUUID7(t), storeTestUUID7(t)
	original := workspace
	storeTestGeneration(t, db, user, nil, 1)
	storeTestGeneration(t, db, user, &workspace, 1)
	snapshot, err := store.Load(context.Background(), Scope{UserID: user, WorkspaceID: &workspace})
	if err != nil {
		t.Fatal(err)
	}
	workspace = storeTestUUID7(t)
	*snapshot.Scope.WorkspaceID = storeTestUUID7(t)
	for i := range snapshot.Generations {
		if snapshot.Generations[i].WorkspaceID != nil {
			*snapshot.Generations[i].WorkspaceID = storeTestUUID7(t)
		}
	}
	if err := store.CheckCurrent(context.Background(), snapshot); err != nil {
		t.Fatal("stamp alterado por alias", err)
	}
	if err := db.Model(&Generation{}).Where("user_id = ? AND workspace_id = ?", user, original).Update("generation", 2).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.CheckCurrent(context.Background(), snapshot); !errors.Is(err, ErrStale) {
		t.Fatal("scope original deixou de ser revalidado", err)
	}
}
