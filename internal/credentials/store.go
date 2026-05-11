package credentials

import "context"

// StoredCredential representa credenciais já criptografadas para persistência.
type StoredCredential struct {
	ID         string
	UserID     string
	Pattern    string
	Auth       *AuthConfig
	Unreadable bool
}

// KeyWrap contém a DEK embrulhada com senha mestre ou recovery key.
//
// `DekID` é a `DEKIdentity(dek)` da DEK que está embrulhada em `WrappedDEK`.
// Existe para que possamos detectar divergência entre a DEK que está no
// keychain do SO e a DEK que cifrou as credenciais sem precisar
// desembrulhar o wrap (que requereria a senha mestre). Toda gravação
// dessa struct via `Store.SaveKeyWrap` DEVE ter `DekID` calculado a
// partir da DEK que foi efetivamente embrulhada — wraps gravados sem
// `DekID` são herança de versões anteriores e o boot é responsável por
// repopular o campo. Veja AEP-0061.
type KeyWrap struct {
	Kind         string
	Salt         string
	WrappedDEK   string
	ArgonTime    uint32
	ArgonMemory  uint32
	ArgonThreads uint8
	DekID        string
}

// Store define a interface de persistência de credenciais e chaves.
type Store interface {
	SaveCredential(ctx context.Context, cred StoredCredential) error
	ListCredentials(ctx context.Context) ([]StoredCredential, error)
	DeleteCredential(ctx context.Context, pattern string) error

	SaveKeyWrap(ctx context.Context, wrap KeyWrap) error
	GetKeyWrap(ctx context.Context, kind string) (*KeyWrap, error)
	HasKeyWrap(ctx context.Context, kind string) (bool, error)
}

type credentialPresenceStore interface {
	HasAnyCredentials(ctx context.Context) (bool, error)
}

func HasAnyStoredCredentials(ctx context.Context, store Store) (bool, error) {
	if store == nil {
		return false, ErrStoreNotReady
	}
	if presence, ok := store.(credentialPresenceStore); ok {
		return presence.HasAnyCredentials(ctx)
	}
	entries, err := store.ListCredentials(ctx)
	if err != nil {
		return false, err
	}
	return len(entries) > 0, nil
}
