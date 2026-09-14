package commandledger

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"assistente/internal/database"
)

func TestAuthenticatedRequestUsesSessionIdentityAndRejectsLogout(t *testing.T) {
	t.Run("revogação observada", func(t *testing.T) { testAuthenticatedRequestLogout(t, false) })
	t.Run("logout coordenado", func(t *testing.T) { testAuthenticatedRequestLogout(t, true) })
}

func testAuthenticatedRequestLogout(t *testing.T, coordinated bool) {
	ctx := context.Background()
	now := time.Now().UTC()
	store, db := testStore(t, &now)
	if err := db.AutoMigrate(&database.User{}, &database.Session{}); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "fixture", PasswordHash: "unused-fixture", Role: database.UserRoleUser, IsActive: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{RefreshTokenPepper: bytes.Repeat([]byte{1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := sessions.IssueSession(ctx, &user, "command-test")
	if err != nil {
		t.Fatal(err)
	}
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	manager := credentials.NewManager(bytes.Repeat([]byte{2}, 32))
	if err := manager.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))); err != nil {
		t.Fatal(err)
	}
	keys, err := NewCredentialKeyProvider(manager)
	if err != nil {
		t.Fatal(err)
	}
	reserve := func(token string) (Reservation, commandsecurity.EpochSnapshot, error) {
		snapshot, err := epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
			principal, err := sessions.AuthenticateLocalAccess(ctx, token)
			if err != nil {
				return "", "", err
			}
			return principal.UserID, principal.SessionID, nil
		})
		if err != nil {
			return Reservation{}, commandsecurity.EpochSnapshot{}, err
		}
		req := validRequest()
		req.AuthGeneration = snapshot.AuthGeneration
		req.SecurityGeneration = snapshot.SecurityGeneration
		req.Owner = Owner{UserID: snapshot.UserID, AuthContextID: snapshot.SessionID}
		req.ReceivedAt = now
		req.ExpiresAt = now.Add(time.Hour)
		req.ArgumentsFingerprint, req.RequestFingerprint, req.RequestFingerprintVersion = "", "", ""
		signed, err := SignLocalRead(ctx, req, "v1", keys)
		if err != nil {
			return Reservation{}, commandsecurity.EpochSnapshot{}, err
		}
		result, err := store.Reserve(ctx, signed)
		if err != nil {
			return Reservation{}, commandsecurity.EpochSnapshot{}, err
		}
		return result, snapshot, nil
	}
	result, snapshot, err := reserve(pair.AccessToken)
	if err != nil || !result.Created || result.Record.Owner.UserID != user.ID || result.Record.Owner.AuthContextID != pair.SessionID {
		t.Fatalf("identidade derivada incorreta: %v", err)
	}
	logout := func() error { return sessions.Logout(ctx, pair.RefreshToken) }
	if coordinated {
		err = epochs.MutatePrincipal(ctx, snapshot.UserID, snapshot.SessionID, logout)
	} else {
		err = logout()
	}
	if err != nil {
		t.Fatal(err)
	}
	// Sem coordenação, a consulta autoritativa recusa; com coordenação, o epoch
	// já está obsoleto. Nenhum dos fluxos deve admitir handoff após logout.
	err = epochs.Admit(ctx, snapshot, func(ctx context.Context) error {
		principal, err := sessions.AuthenticateLocalAccess(ctx, pair.AccessToken)
		if err != nil {
			return err
		}
		if principal.UserID != snapshot.UserID || principal.SessionID != snapshot.SessionID {
			return commandsecurity.ErrStaleEpoch
		}
		return nil
	}, func() error { t.Fatal("handoff após logout"); return nil })
	want := auth.ErrUnauthenticatedLocalSession
	if coordinated {
		want = commandsecurity.ErrStaleEpoch
	}
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
	if _, err := sessions.VerifyAccessToken(pair.AccessToken); err != nil {
		t.Fatal("fixture deveria manter JWT criptograficamente válido")
	}
	denied, _, err := reserve(pair.AccessToken)
	if !errors.Is(err, auth.ErrUnauthenticatedLocalSession) || denied != (Reservation{}) {
		t.Fatal("logout não bloqueou nova reserva")
	}
	var count int64
	if err := db.Model(&ledgerRow{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("reserva indevida após logout: %d %v", count, err)
	}
}
