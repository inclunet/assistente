package auth

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"
)

type ExternalValidator struct {
	Issuer            string
	Audience          string
	AllowedAlgorithms map[string]bool
	ClockSkew         time.Duration
	Now               func() time.Time
}

type ExternalClaims struct {
	Issuer    string   `json:"iss"`
	Audience  audience `json:"aud"`
	Subject   string   `json:"sub"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
	Scope     string   `json:"scope,omitempty"`
	Roles     []string `json:"roles,omitempty"`
}

func (v ExternalValidator) Validate(token string, jwks JWKSet) (*ExternalClaims, error) {
	now := time.Now
	if v.Now != nil {
		now = v.Now
	}
	skew := v.ClockSkew
	if skew == 0 {
		skew = time.Minute
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("token externo inválido")
	}

	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("header externo inválido")
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, errors.New("header externo inválido")
	}
	if header.Algorithm == "" || !v.AllowedAlgorithms[header.Algorithm] {
		return nil, errors.New("algoritmo externo não permitido")
	}

	key, ok := findJWK(jwks, header.KeyID, header.Algorithm)
	if !ok {
		return nil, errors.New("chave externa não encontrada")
	}
	if err := verifyExternalSignature(parts, key); err != nil {
		return nil, err
	}

	var claims ExternalClaims
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("claims externas inválidas")
	}
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, errors.New("claims externas inválidas")
	}
	if claims.Issuer != v.Issuer || !claims.Audience.Contains(v.Audience) {
		return nil, errors.New("issuer ou audience externos inválidos")
	}
	if now().Add(-skew).Unix() > claims.ExpiresAt {
		return nil, errors.New("token externo expirado")
	}
	if claims.IssuedAt != 0 && now().Add(skew).Unix() < claims.IssuedAt {
		return nil, errors.New("token externo emitido no futuro")
	}
	return &claims, nil
}

func findJWK(jwks JWKSet, keyID, algorithm string) (JWK, bool) {
	for _, key := range jwks.Keys {
		if key.KeyID == keyID && key.Algorithm == algorithm {
			return key, true
		}
	}
	return JWK{}, false
}

func verifyExternalSignature(parts []string, key JWK) error {
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return errors.New("assinatura externa inválida")
	}
	signingInput := []byte(parts[0] + "." + parts[1])

	switch key.Algorithm {
	case jwtAlgorithm:
		if key.KeyType != "OKP" || key.Curve != "Ed25519" {
			return errors.New("tipo de chave externa não suportado")
		}
		publicKey, err := base64.RawURLEncoding.DecodeString(key.X)
		if err != nil {
			return errors.New("chave externa inválida")
		}
		if !ed25519.Verify(ed25519.PublicKey(publicKey), signingInput, signature) {
			return errors.New("assinatura externa inválida")
		}
		return nil
	case "RS256":
		if key.KeyType != "RSA" {
			return errors.New("tipo de chave externa não suportado")
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
		if err != nil {
			return errors.New("chave externa inválida")
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
		if err != nil {
			return errors.New("chave externa inválida")
		}
		e := big.NewInt(0).SetBytes(eBytes).Int64()
		if e == 0 {
			return errors.New("expoente RSA externo inválido")
		}
		sum := crypto.SHA256.New()
		_, _ = sum.Write(signingInput)
		if err := rsa.VerifyPKCS1v15(&rsa.PublicKey{N: big.NewInt(0).SetBytes(nBytes), E: int(e)}, crypto.SHA256, sum.Sum(nil), signature); err != nil {
			return errors.New("assinatura externa inválida")
		}
		return nil
	default:
		return errors.New("algoritmo externo não suportado")
	}
}

type audience []string

func (a *audience) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*a = []string{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

func (a audience) Contains(expected string) bool {
	for _, value := range a {
		if value == expected {
			return true
		}
	}
	return false
}
