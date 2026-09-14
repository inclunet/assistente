package commandledger

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/credentials"
	"assistente/internal/database"
)

func TestAuthenticatedRequestUsesSessionIdentityAndRejectsLogout(t *testing.T) {
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
	manager := credentials.NewManager(bytes.Repeat([]byte{2}, 32))
	if err := manager.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))); err != nil {
		t.Fatal(err)
	}
	keys, err := NewCredentialKeyProvider(manager)
	if err != nil {
		t.Fatal(err)
	}
	// A borda de teste não recebe user_id/session_id. As gerações ainda são
	// fixtures: integração com EpochService e gates permanece pendente.
	reserve := func(token string) (Reservation, error) {
		principal, err := sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return Reservation{}, err
		}
		req := validRequest()
		req.Owner = Owner{UserID: principal.UserID, AuthContextID: principal.SessionID}
		req.ReceivedAt = now
		req.ExpiresAt = now.Add(time.Hour)
		req.ArgumentsFingerprint, req.RequestFingerprint, req.RequestFingerprintVersion = "", "", ""
		signed, err := SignLocalRead(ctx, req, "v1", keys)
		if err != nil {
			return Reservation{}, err
		}
		return store.Reserve(ctx, signed)
	}
	result, err := reserve(pair.AccessToken)
	if err != nil || !result.Created || result.Record.Owner.UserID != user.ID || result.Record.Owner.AuthContextID != pair.SessionID {
		t.Fatalf("identidade derivada incorreta: %v", err)
	}
	if err := sessions.Logout(ctx, pair.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.VerifyAccessToken(pair.AccessToken); err != nil {
		t.Fatal("fixture deveria manter JWT criptograficamente válido")
	}
	denied, err := reserve(pair.AccessToken)
	if !errors.Is(err, auth.ErrUnauthenticatedLocalSession) || denied != (Reservation{}) {
		t.Fatal("logout não bloqueou nova reserva")
	}
	var count int64
	if err := db.Model(&ledgerRow{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("reserva indevida após logout: %d %v", count, err)
	}
}
