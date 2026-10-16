package commandautomation

import (
	"context"
	"regexp"
	"strings"

	"assistente/internal/commandjson"
)

const (
	ruleFingerprintDomain     = "commandautomation:rule-fingerprint:v1"
	producerFingerprintDomain = "commandautomation:producer-types-fingerprint:v1"
	grantFingerprintDomain    = "commandautomation:grant-fingerprint:v1"
)

var fingerprintVersion = regexp.MustCompile(`^v[1-9][0-9]*$`)

func parseProducerTypes(value string) ([]string, error) {
	canonical, err := commandjson.Canonicalize([]byte(value))
	if err != nil || string(canonical) != `["jobs.runtime"]` {
		return nil, ErrInvalid
	}
	return []string{JobsRuntime}, nil
}

func canonicalCondition(value string) (string, error) {
	canonical, err := commandjson.Canonicalize([]byte(value))
	if err != nil {
		return "", err
	}
	if len(canonical) == 0 || canonical[0] != '{' {
		return "", ErrInvalid
	}
	return string(canonical), nil
}

func validateRule(rule Rule) error {
	if !validUUID7(rule.ID) || !validOwner(rule.Owner) || !validRef(rule.LayerRef) || !validRef(rule.RuleRef) ||
		rule.Mode != "event" || (rule.Lifecycle != "persistent" && rule.Lifecycle != "session" && rule.Lifecycle != "temporary") || rule.EventName != JobRunStateEvent || rule.Source == "" || rule.ReviewStatus != "active" {
		return ErrInvalid
	}
	if rule.RuleRef.Kind == "user" && rule.RuleRef.Ref != rule.ID {
		return ErrInvalid
	}
	if len(rule.AllowedInternalProducerTypes) != 1 || rule.AllowedInternalProducerTypes[0] != JobsRuntime {
		return ErrInvalid
	}
	if _, err := canonicalCondition(rule.Condition); err != nil {
		return ErrInvalid
	}
	return nil
}

func ownerJSON(owner Owner) map[string]any {
	value := map[string]any{"user_id": owner.UserID}
	if owner.WorkspaceID != nil {
		value["workspace_id"] = *owner.WorkspaceID
	}
	return value
}

func ruleProjection(rule Rule) (map[string]any, error) {
	condition, err := canonicalCondition(rule.Condition)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"version":        1,
		"owner":          ownerJSON(rule.Owner),
		"layer_ref_kind": rule.LayerRef.Kind, "layer_ref": rule.LayerRef.Ref,
		"rule_ref_kind": rule.RuleRef.Kind, "rule_ref": rule.RuleRef.Ref,
		"mode": rule.Mode, "condition": condition, "lifecycle": rule.Lifecycle,
		"event_name": rule.EventName, "allowed_internal_producer_types": `["jobs.runtime"]`,
	}, nil
}

func signProjection(ctx context.Context, projection any, domain, version string, keys FingerprintKeyProvider) (string, error) {
	if ctx == nil || keys == nil || !fingerprintVersion.MatchString(version) {
		return "", ErrFingerprint
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	payload, err := commandjson.Marshal(projection)
	if err != nil {
		return "", ErrFingerprint
	}
	key, err := keys(ctx, "command-request-hmac:"+version)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	if err != nil || len(key) < 32 {
		return "", ErrFingerprint
	}
	copyKey := append([]byte(nil), key...)
	defer clear(copyKey)
	result, err := commandjson.HMAC(copyKey, domain+":"+version, payload)
	if err != nil {
		return "", ErrFingerprint
	}
	return result, nil
}

// FingerprintRule assina somente a semântica da regra. enabled, decisão e
// grant atual ficam fora da projeção para que cada concessão tenha identidade
// própria e a revalidação possa detectar alterações sem confiar no payload.
func FingerprintRule(ctx context.Context, rule Rule, version string, keys FingerprintKeyProvider) (string, error) {
	if err := validateRule(rule); err != nil {
		return "", err
	}
	projection, err := ruleProjection(rule)
	if err != nil {
		return "", err
	}
	return signProjection(ctx, projection, ruleFingerprintDomain, version, keys)
}

func producerTypesFingerprint(ctx context.Context, version string, keys FingerprintKeyProvider) (string, error) {
	return signProjection(ctx, map[string]any{"version": 1, "producer_types": []string{JobsRuntime}}, producerFingerprintDomain, version, keys)
}

func grantProjection(key NaturalKey, ruleFingerprint, producerFingerprint string, generation int64) map[string]any {
	value := map[string]any{"version": 1, "owner": ownerJSON(key.Owner),
		"layer_ref_kind": key.LayerRef.Kind, "layer_ref": key.LayerRef.Ref,
		"rule_ref_kind": key.RuleRef.Kind, "rule_ref": key.RuleRef.Ref,
		"rule_fingerprint": ruleFingerprint, "event_name": JobRunStateEvent,
		"producer_types_fingerprint": producerFingerprint, "automation_grant_generation": generation}
	return value
}

// FingerprintGrant assina a chave natural e todos os vínculos autoritativos
// de uma concessão. generation nunca é convertido em float ou aceito como
// texto do payload.
func FingerprintGrant(ctx context.Context, key NaturalKey, ruleFingerprint, producerFingerprint string, generation int64, version string, keys FingerprintKeyProvider) (string, error) {
	if !validKey(key) || strings.TrimSpace(ruleFingerprint) != ruleFingerprint || ruleFingerprint == "" || strings.TrimSpace(producerFingerprint) != producerFingerprint || producerFingerprint == "" || !validateGeneration(generation) || generation == 0 {
		return "", ErrInvalid
	}
	return signProjection(ctx, grantProjection(key, ruleFingerprint, producerFingerprint, generation), grantFingerprintDomain, version, keys)
}

// ProducerTypesFingerprint deixa a impressão dos produtores verificável pelos
// componentes de integração, sem expor material de chave.
func ProducerTypesFingerprint(ctx context.Context, version string, keys FingerprintKeyProvider) (string, error) {
	return producerTypesFingerprint(ctx, version, keys)
}
