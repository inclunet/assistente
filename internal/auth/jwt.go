package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const jwtAlgorithm = "EdDSA"

type TokenSigner struct {
	keyID      string
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
}

func NewTokenSigner() (*TokenSigner, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return NewTokenSignerFromPrivateKey(privateKey)
}

func NewTokenSignerFromPrivateKey(privateKey ed25519.PrivateKey) (*TokenSigner, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("chave privada Ed25519 inválida")
	}
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok || len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("chave pública Ed25519 inválida")
	}
	sum := sha256.Sum256(publicKey)
	return &TokenSigner{
		keyID:      base64.RawURLEncoding.EncodeToString(sum[:8]),
		publicKey:  publicKey,
		privateKey: privateKey,
	}, nil
}

func (s *TokenSigner) ExportPrivateKey() (string, error) {
	if s == nil || len(s.privateKey) != ed25519.PrivateKeySize {
		return "", errors.New("token signer não inicializado")
	}
	return base64.RawURLEncoding.EncodeToString(s.privateKey), nil
}

func TokenSignerFromEncodedPrivateKey(encoded string) (*TokenSigner, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, errors.New("chave privada JWT inválida")
	}
	return NewTokenSignerFromPrivateKey(ed25519.PrivateKey(raw))
}

type AccessClaims struct {
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	Subject   string `json:"sub"`
	SessionID string `json:"sid"`
	JTI       string `json:"jti,omitempty"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	Role      string `json:"role,omitempty"`
}

func (s *TokenSigner) SignAccessToken(claims AccessClaims) (string, error) {
	if s == nil || len(s.privateKey) == 0 {
		return "", errors.New("token signer não inicializado")
	}

	header := map[string]string{
		"alg": jwtAlgorithm,
		"kid": s.keyID,
		"typ": "JWT",
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	signature := ed25519.Sign(s.privateKey, []byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (s *TokenSigner) VerifyAccessToken(token, issuer, audience string, now time.Time, skew time.Duration) (*AccessClaims, error) {
	if s == nil || len(s.publicKey) == 0 {
		return nil, errors.New("token signer não inicializado")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("token inválido")
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("assinatura inválida")
	}
	if !ed25519.Verify(s.publicKey, []byte(parts[0]+"."+parts[1]), signature) {
		return nil, errors.New("assinatura inválida")
	}

	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("header inválido")
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, errors.New("header inválido")
	}
	if header.Algorithm != jwtAlgorithm || header.KeyID != s.keyID {
		return nil, errors.New("algoritmo ou chave inválida")
	}

	var claims AccessClaims
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("claims inválidas")
	}
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, errors.New("claims inválidas")
	}
	if claims.Issuer != issuer || claims.Audience != audience {
		return nil, errors.New("issuer ou audience inválidos")
	}
	if now.Add(-skew).Unix() > claims.ExpiresAt {
		return nil, errors.New("token expirado")
	}
	if now.Add(skew).Unix() < claims.IssuedAt {
		return nil, errors.New("token emitido no futuro")
	}
	return &claims, nil
}

type JWKSet struct {
	Keys []JWK `json:"keys"`
}

type JWK struct {
	KeyType   string `json:"kty"`
	KeyID     string `json:"kid"`
	Algorithm string `json:"alg"`
	Use       string `json:"use"`
	Curve     string `json:"crv"`
	X         string `json:"x"`
	N         string `json:"n,omitempty"`
	E         string `json:"e,omitempty"`
}

func (s *TokenSigner) JWKSet() JWKSet {
	if s == nil {
		return JWKSet{}
	}
	return JWKSet{
		Keys: []JWK{
			{
				KeyType:   "OKP",
				KeyID:     s.keyID,
				Algorithm: jwtAlgorithm,
				Use:       "sig",
				Curve:     "Ed25519",
				X:         base64.RawURLEncoding.EncodeToString(s.publicKey),
			},
		},
	}
}
