package credentials

import (
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"context"
	"errors"
	"gorm.io/gorm"
	"regexp"
	"time"
)

// SaveWithConsumer commits a normal credential and its consumer in one vault
// transaction. Callbacks must not reenter the vault or perform network I/O.
// retire optionally replaces a versioned OAuth grant using the same lifecycle
// guards as explicit OAuth removal. No cache or consumer publication precedes commit.
func (s *oauthStore) SaveWithConsumer(ctx context.Context, pattern string, auth *AuthConfig, retire *oauthflow.Record, update func(*gorm.DB) error, publish func()) error {
	m := s.manager
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if pattern == "" || IsManagedPattern(pattern) || auth == nil || auth.Source == "oauth" || update == nil {
		return ErrCredentialResolution
	}
	if err := ValidateSource(auth); err != nil {
		return err
	}
	rx, err := regexp.Compile(wildcardToRegex(pattern))
	if err != nil {
		return err
	}
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return err
	}
	store, ok := m.store.(*DBStore)
	if !ok || !m.persist {
		return ErrStoreNotReady
	}
	if _, err = store.ensureDB(); err != nil {
		return err
	}
	enc, err := m.encryptAuth(auth)
	if err != nil {
		return err
	}
	var id string
	err = database.WithSQLiteImmediateTransactionOnce(ctx, time.Time{}, store.db, "credentials.consumer.save", func(tx *gorm.DB) error {
		if retire != nil {
			if retire.UserID != user {
				return oauthflow.ErrConflict
			}
			if err := m.deleteOAuthEntryTx(ctx, tx, user, retire.ID, &retire.Revision); err != nil {
				return err
			}
		}
		boundStore := &DBStore{db: tx}
		if err := boundStore.saveCredential(ctx, StoredCredential{UserID: user, Pattern: pattern, Auth: enc}, true); err != nil {
			return err
		}
		var row database.CredentialEntry
		if err := tx.Where("user_id = ? AND pattern = ?", user, pattern).First(&row).Error; err != nil {
			return err
		}
		id = row.ID
		if id == "" {
			return errors.New("credential_identity_missing")
		}
		return update(tx)
	})
	if err != nil {
		return err
	}
	replacement := &DomainCredential{ID: id, UserID: user, Pattern: pattern, regex: rx, Auth: enc}
	updated := false
	kept := m.credentials[:0]
	for _, entry := range m.credentials {
		if entry.UserID == user && entry.Pattern == pattern {
			entry.invalidateCommandCache()
			kept = append(kept, replacement)
			updated = true
			continue
		}
		if retire != nil && entry.UserID == user && entry.ID == retire.ID {
			entry.invalidateCommandCache()
			continue
		}
		kept = append(kept, entry)
	}
	if !updated {
		kept = append(kept, replacement)
	}
	m.credentials = kept
	if retire != nil {
		for _, cancel := range m.oauthRequests[retire.ID] {
			cancel()
		}
		delete(m.oauthRequests, retire.ID)
	}
	if publish != nil {
		publish()
	}
	return nil
}
