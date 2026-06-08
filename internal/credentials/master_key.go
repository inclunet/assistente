package credentials

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	KeyWrapKindMaster   = "master"
	KeyWrapKindRecovery = "recovery"
)

const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 4
	argonKeyLen         = 32
)

var (
	ErrKeyWrapNotFound               = errors.New("embrulho da chave não encontrado")
	ErrExistingCredentialsWithoutDEK = errors.New("credenciais existentes sem DEK no keyring")
)

// MasterKeySetupResult contém dados gerados na configuração inicial.
type MasterKeySetupResult struct {
	DEK         []byte
	RecoveryKey string
}

// SetupMasterKey gera DEK, embrulha com senha mestre e recovery key e salva no banco.
//
// Em produção a DEK gerada é persistida via `PersistDEKConsistent`, que
// recusa sobrescrever uma DEK preexistente no keychain (evita o cenário
// "Setup chamado por engano sobre install já configurado, sobrescreve a
// DEK e órfãas todas as creds antigas" — causa raiz do incidente que
// motivou o AEP-0061).
func SetupMasterKey(store Store, masterPassword string) (*MasterKeySetupResult, error) {
	return setupMasterKey(store, masterPassword, defaultDEKPersister(store))
}

func setupMasterKey(store Store, masterPassword string, persistDEK func([]byte) error) (*MasterKeySetupResult, error) {
	if store == nil {
		return nil, ErrStoreNotReady
	}
	if strings.TrimSpace(masterPassword) == "" {
		return nil, errors.New("senha mestre inválida")
	}

	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, err
	}

	return setupMasterKeyForDEK(store, masterPassword, dek, persistDEK)
}

// SetupMasterKeyAdoptingKeychain cria os wraps de senha/recovery adotando a DEK
// já existente no keyring quando presente. Se a entrada do keyring existir mas
// não puder ser lida, falha em vez de gerar uma nova DEK e tornar credenciais
// já persistidas indecifráveis.
func SetupMasterKeyAdoptingKeychain(store Store, masterPassword string) (*MasterKeySetupResult, error) {
	return setupMasterKeyAdoptingKeychain(store, masterPassword, LoadDEKFromKeychain, defaultDEKPersister(store))
}

func setupMasterKeyAdoptingKeychain(store Store, masterPassword string, loadKeyring func() ([]byte, error), persistDEK func([]byte) error) (*MasterKeySetupResult, error) {
	if store == nil {
		return nil, ErrStoreNotReady
	}
	if loadKeyring == nil {
		loadKeyring = LoadDEKFromKeychain
	}
	existingDEK, err := loadKeyring()
	if err == nil {
		if len(existingDEK) == 0 {
			return nil, errors.New("DEK existente no keyring está vazia")
		}
		return SetupMasterKeyWrapsForDEK(store, masterPassword, existingDEK)
	}
	if !IsKeychainNotFound(err) {
		return nil, err
	}
	hasCredentials, err := HasAnyStoredCredentials(contextBackground(), store)
	if err != nil {
		return nil, err
	}
	if hasCredentials {
		return nil, ErrExistingCredentialsWithoutDEK
	}
	return setupMasterKey(store, masterPassword, persistDEK)
}

// SetupMasterKeyForDEK embrulha uma DEK já existente com senha mestre e recovery key.
// Usado ao adotar instalações que já tinham a DEK no keyring antes da AEP-0052.
func SetupMasterKeyForDEK(store Store, masterPassword string, dek []byte) (*MasterKeySetupResult, error) {
	return setupMasterKeyForDEK(store, masterPassword, dek, defaultDEKPersister(store))
}

// SetupMasterKeyWrapsForDEK cria apenas os wraps para uma DEK já persistida no
// keyring. Não grava a DEK novamente no keyring.
func SetupMasterKeyWrapsForDEK(store Store, masterPassword string, dek []byte) (*MasterKeySetupResult, error) {
	return setupMasterKeyForDEK(store, masterPassword, dek, nil)
}

// setupMasterKeyForDEK é o caminho compartilhado para gravar wraps e
// (opcionalmente) a DEK no keychain. `persistDEK` recebe a DEK e é
// responsável por gravar consistentemente — em produção é
// `defaultDEKPersister(store)`, que delega para `PersistDEKConsistent`
// e recusa sobrescrever DEK divergente. Passar `nil` em `persistDEK`
// pula a gravação no keychain (caminho `SetupMasterKeyWrapsForDEK`,
// usado quando a DEK já está no keychain e só faltam wraps).
func setupMasterKeyForDEK(store Store, masterPassword string, dek []byte, persistDEK func([]byte) error) (*MasterKeySetupResult, error) {
	if store == nil {
		return nil, ErrStoreNotReady
	}
	if strings.TrimSpace(masterPassword) == "" {
		return nil, errors.New("senha mestre inválida")
	}
	if len(dek) == 0 {
		return nil, errors.New("DEK inválida")
	}

	recoveryKey, err := GenerateRecoveryKey()
	if err != nil {
		return nil, err
	}

	masterWrap, err := WrapDEK(masterPassword, dek, KeyWrapKindMaster)
	if err != nil {
		return nil, err
	}
	recoveryWrap, err := WrapDEK(recoveryKey, dek, KeyWrapKindRecovery)
	if err != nil {
		return nil, err
	}

	if err := store.SaveKeyWrap(contextBackground(), *masterWrap); err != nil {
		return nil, err
	}
	if err := store.SaveKeyWrap(contextBackground(), *recoveryWrap); err != nil {
		return nil, err
	}

	if persistDEK != nil {
		if err := persistDEK(dek); err != nil {
			return nil, err
		}
	}

	return &MasterKeySetupResult{
		DEK:         dek,
		RecoveryKey: recoveryKey,
	}, nil
}

// defaultDEKPersister retorna a função de persistência de produção
// que enforça a invariante DEK_keychain == DEK_wraps via
// `PersistDEKConsistent`. Tests devem injetar um persister mais
// simples se quiserem isolar do keychain real.
func defaultDEKPersister(store Store) func([]byte) error {
	return func(dek []byte) error {
		return PersistDEKConsistent(contextBackground(), store, dek, LoadDEKFromKeychain, saveDEKToKeychain)
	}
}

// UnlockDEKWithSecret tenta recuperar a DEK usando senha mestre ou recovery key.
func UnlockDEKWithSecret(store Store, kind, secret string) ([]byte, error) {
	if store == nil {
		return nil, ErrStoreNotReady
	}
	wrap, err := store.GetKeyWrap(contextBackground(), kind)
	if err != nil {
		return nil, err
	}
	if wrap == nil {
		return nil, ErrKeyWrapNotFound
	}
	return UnwrapDEK(secret, wrap)
}

// WrapDEK embrulha a DEK usando Argon2id + AES-GCM.
//
// `KeyWrap.DekID` é sempre populado com `DEKIdentity(dek)` para que,
// no boot, a Manager possa comparar com `DEKIdentity(DEK_keychain)` e
// detectar divergência sem precisar da senha mestre. Ver AEP-0061.
func WrapDEK(secret string, dek []byte, kind string) (*KeyWrap, error) {
	if len(dek) == 0 {
		return nil, errors.New("DEK vazia em WrapDEK")
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	key := argon2.IDKey([]byte(secret), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	wrapped, err := encryptBytes(key, dek)
	if err != nil {
		return nil, err
	}

	return &KeyWrap{
		Kind:         kind,
		Salt:         base64.StdEncoding.EncodeToString(salt),
		WrappedDEK:   wrapped,
		ArgonTime:    argonTime,
		ArgonMemory:  argonMemory,
		ArgonThreads: argonThreads,
		DekID:        DEKIdentity(dek),
	}, nil
}

// UnwrapDEK recupera a DEK a partir do embrulho.
func UnwrapDEK(secret string, wrap *KeyWrap) ([]byte, error) {
	salt, err := base64.StdEncoding.DecodeString(wrap.Salt)
	if err != nil {
		return nil, err
	}

	key := argon2.IDKey([]byte(secret), salt, wrap.ArgonTime, wrap.ArgonMemory, wrap.ArgonThreads, argonKeyLen)
	return decryptBytes(key, wrap.WrappedDEK)
}

// GenerateRecoveryKey cria um código de recuperação amigável.
func GenerateRecoveryKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}

	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	encoded = strings.ToUpper(encoded)

	parts := make([]string, 0, len(encoded)/4+1)
	for i := 0; i < len(encoded); i += 4 {
		end := i + 4
		if end > len(encoded) {
			end = len(encoded)
		}
		parts = append(parts, encoded[i:end])
	}
	return strings.Join(parts, "-"), nil
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
	data, err := base64.StdEncoding.DecodeString(encoded)
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
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext muito curto")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// contextBackground evita importar context em todo arquivo.
func contextBackground() context.Context {
	return context.Background()
}
