package commandledger

import (
	"github.com/google/uuid"
	"regexp"
	"strings"
)

var commandIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

func validUUID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.Version() == 7 && parsed.Variant() == uuid.RFC4122 && parsed.String() == id
}

func validateOwner(owner Owner) error {
	if !validUUID(owner.UserID) || !validUUID(owner.AuthContextID) {
		return ErrInvalidRequest
	}
	return nil
}

func validateRequest(req LocalReadRequest) error {
	if !commandIDPattern.MatchString(req.CommandID) {
		return ErrInvalidRequest
	}
	if validateOwner(req.Owner) != nil || !validUUID(req.InvocationID) {
		return ErrInvalidRequest
	}
	switch req.SourceType {
	case "palette", "ui.action", "cli":
	default:
		return ErrInvalidRequest
	}
	for _, value := range []string{req.AuthGeneration, req.SecurityGeneration, req.RegistryVersion, req.GlobalConfigGeneration, req.ActiveLayersGeneration, req.CommandID, req.ArgumentsFingerprint, req.RequestFingerprintVersion, req.RequestFingerprint, req.CorrelationID} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return ErrInvalidRequest
		}
	}
	if req.ReceivedAt.IsZero() || req.ExpiresAt.IsZero() || !req.ExpiresAt.After(req.ReceivedAt) {
		return ErrInvalidRequest
	}
	return nil
}
