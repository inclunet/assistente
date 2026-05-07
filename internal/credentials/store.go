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
type KeyWrap struct {
	Kind         string
	Salt         string
	WrappedDEK   string
	ArgonTime    uint32
	ArgonMemory  uint32
	ArgonThreads uint8
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
