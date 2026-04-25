package portability

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	credentialCipherMode                = "encrypted"
	credentialCipherAlgorithm           = "argon2id-aes-256-gcm"
	credentialCipherVersion             = 1
	credentialArgonTime          uint32 = 3
	credentialArgonMemory        uint32 = 64 * 1024
	credentialArgonThreads       uint8  = 4
	credentialArgonKeyLen               = 32
	credentialSaltLen                   = 16
	maxCredentialSaltBytes              = 64
	maxCredentialCiphertextBytes        = 8 * 1024 * 1024
)

func EncryptCredentialsPayload(password string, payload any) (*CredentialCipher, error) {
	if strings.TrimSpace(password) == "" {
		return nil, errors.New("senha de exportação é obrigatória")
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	salt := make([]byte, credentialSaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	key := argon2.IDKey([]byte(password), salt, credentialArgonTime, credentialArgonMemory, credentialArgonThreads, credentialArgonKeyLen)
	ciphertext, err := encryptBytes(key, raw)
	if err != nil {
		return nil, err
	}

	return &CredentialCipher{
		Mode:       credentialCipherMode,
		Algorithm:  credentialCipherAlgorithm,
		Version:    credentialCipherVersion,
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Ciphertext: ciphertext,
	}, nil
}

func DecryptCredentialsPayload(password string, blob *CredentialCipher, out any) error {
	if blob == nil {
		return errors.New("bloco de credenciais ausente")
	}
	if strings.TrimSpace(password) == "" {
		return errors.New("senha de exportação é obrigatória")
	}
	if blob.Mode != credentialCipherMode {
		return errors.New("modo de credenciais não suportado")
	}
	if blob.Algorithm != credentialCipherAlgorithm {
		return errors.New("algoritmo de credenciais não suportado")
	}
	if blob.Version != credentialCipherVersion {
		return errors.New("versão do bloco de credenciais não suportada")
	}

	salt, err := decodeBase64WithLimit(blob.Salt, maxCredentialSaltBytes)
	if err != nil {
		return err
	}
	if len(salt) != credentialSaltLen {
		return errors.New("salt do bloco de credenciais inválido")
	}

	key := argon2.IDKey([]byte(password), salt, credentialArgonTime, credentialArgonMemory, credentialArgonThreads, credentialArgonKeyLen)
	raw, err := decryptBytes(key, blob.Ciphertext)
	if err != nil {
		return err
	}

	return json.Unmarshal(raw, out)
}

func decodeBase64WithLimit(encoded string, maxDecodedBytes int) ([]byte, error) {
	if maxDecodedBytes <= 0 {
		return nil, errors.New("limite inválido para decodificação base64")
	}
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return nil, errors.New("payload base64 ausente")
	}
	if base64.StdEncoding.DecodedLen(len(trimmed)) > maxDecodedBytes {
		return nil, errors.New("payload base64 excede o limite permitido")
	}
	return base64.StdEncoding.DecodeString(trimmed)
}

func encryptBytes(key []byte, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decryptBytes(key []byte, encoded string) ([]byte, error) {
	data, err := decodeBase64WithLimit(encoded, maxCredentialCiphertextBytes)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("ciphertext muito curto")
	}

	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
