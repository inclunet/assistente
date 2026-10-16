package commandconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
)

// PersistedFingerprint identifies the immutable configuration/authority base
// for a lifecycle projection. Job claims are derived runtime state and are
// deliberately excluded; every other claim, rule, grant, binding, layer and
// generation remains part of the identity.
func (s Snapshot) PersistedFingerprint() (string, error) {
	claims := make([]commandactivation.Claim, 0, len(s.ActivationClaims))
	for _, claim := range s.ActivationClaims {
		if claim.SourceType != "job" {
			claims = append(claims, claim)
		}
	}
	persisted := struct {
		Scope            Scope
		Layers           []Layer
		Bindings         []Binding
		Generations      []Generation
		ActivationRules  []commandactivation.Rule
		AutomationGrants []commandautomation.Grant
		ActivationClaims []commandactivation.Claim
	}{
		Scope: s.Scope, Layers: s.Layers, Bindings: s.Bindings, Generations: s.Generations,
		ActivationRules: s.ActivationRules, AutomationGrants: s.AutomationGrants, ActivationClaims: claims,
	}
	raw, err := json.Marshal(persisted)
	if err != nil {
		return "", fmt.Errorf("fingerprint da configuração persistida: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
