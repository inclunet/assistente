package credentials

import (
	"context"
	"errors"
	"fmt"
	"log"
)

// ErrDEKWouldOverwrite indica que `PersistDEKConsistent` recusou
// sobrescrever a DEK existente no keychain por uma DEK diferente.
//
// É o sinal canônico de "você está prestes a fazer uma operação que
// torna credenciais cifradas com a DEK atual ilegíveis"; só é
// retornado quando as DEKs realmente divergem (mesma DEK no keychain
// e na nova entrada é no-op silencioso).
//
// Callers que queiram forçar a sobrescrita (por exemplo, recovery via
// senha mestre confirmada pelo usuário) devem usar
// `OverwriteKeychainDEK` em vez de `PersistDEKConsistent`.
var ErrDEKWouldOverwrite = errors.New("DEK do keychain divergiria da DEK informada — sobrescrever silenciosamente é proibido (use OverwriteKeychainDEK com confirmação)")

// PersistDEKConsistent é a ÚNICA via para escrever uma DEK no keychain
// do SO em código de produção. Garante a invariante AEP-0061:
//
//	DEKIdentity(DEK_keychain) == credential_key_wraps.dek_id
//
// Comportamento:
//
//  1. Carrega a DEK que estiver no keychain.
//     - Se carregar falha por motivo OUTRO que "não encontrada", erro.
//     - Se "não encontrada", trata como vazio (caso de Setup inicial).
//  2. Compara `DEKIdentity(existing)` com `DEKIdentity(dek)`.
//     - Se já é a mesma DEK, é no-op silencioso (idempotência).
//     - Se existing == "" (keychain vazio), prossegue para gravar.
//     - Se existing != "" e divergente, retorna `ErrDEKWouldOverwrite`.
//       Use `OverwriteKeychainDEK` se a sobrescrita é deliberada.
//  3. Grava `dek` no keychain via `saveDEKToKeychain`.
//  4. Atualiza `dek_id` em todos os wraps existentes (master/recovery)
//     que estiverem com `dek_id == ""`. Wraps sem `dek_id` são
//     herança pré-AEP-0061 que estão sendo "adotados" agora.
//     Wraps que já têm `dek_id` mas que diverge da nova DEK NÃO são
//     tocados — esse é o sinal de "wrap embrulha outra DEK", e o
//     caller deveria ter regenerado o wrap antes de chegar aqui.
//
// `loadKeyring` é injetado para teste; em produção use
// `LoadDEKFromKeychain`.
func PersistDEKConsistent(ctx context.Context, store Store, dek []byte, loadKeyring func() ([]byte, error), saveKeyring func([]byte) error) error {
	if store == nil {
		return ErrStoreNotReady
	}
	if len(dek) == 0 {
		return errors.New("DEK vazia em PersistDEKConsistent")
	}
	if loadKeyring == nil {
		loadKeyring = LoadDEKFromKeychain
	}
	if saveKeyring == nil {
		saveKeyring = saveDEKToKeychain
	}

	newID := DEKIdentity(dek)
	existing, err := loadKeyring()
	switch {
	case err == nil && len(existing) > 0:
		existingID := DEKIdentity(existing)
		if existingID == newID {
			return adoptDekIDInWrapsIfMissing(ctx, store, newID)
		}
		log.Printf("[Credentials] PersistDEKConsistent: keychain tem DEK %s e quiseram persistir DEK %s — recusando sobrescrita", existingID, newID)
		return ErrDEKWouldOverwrite
	case err == nil && len(existing) == 0:
	case IsKeychainNotFound(err):
	default:
		return fmt.Errorf("ler DEK do keychain antes de gravar: %w", err)
	}

	if err := saveKeyring(dek); err != nil {
		return fmt.Errorf("gravar DEK no keychain: %w", err)
	}
	return adoptDekIDInWrapsIfMissing(ctx, store, newID)
}

// OverwriteKeychainDEK sobrescreve incondicionalmente a DEK do
// keychain, INDEPENDENTEMENTE de já ter outra DEK divergente lá. É a
// rota usada quando o usuário deliberadamente recupera a DEK pela
// senha mestre / recovery key e quer reinstalar essa DEK como a do
// keychain — aceitando que credenciais cifradas com a DEK_keychain
// anterior fiquem ilegíveis.
//
// Sempre re-popula `dek_id` nos wraps que estiverem sem ele (e mantém
// os que já bateriam — wraps com `dek_id` divergente do novo são
// indício de bug do caller, então retornamos erro).
//
// Uso esperado: somente em fluxos com confirmação explícita do
// usuário (UI mostra "isso vai apagar acesso a X credenciais").
// Nunca em paths automáticos de boot.
func OverwriteKeychainDEK(ctx context.Context, store Store, dek []byte, saveKeyring func([]byte) error) error {
	if store == nil {
		return ErrStoreNotReady
	}
	if len(dek) == 0 {
		return errors.New("DEK vazia em OverwriteKeychainDEK")
	}
	if saveKeyring == nil {
		saveKeyring = saveDEKToKeychain
	}

	newID := DEKIdentity(dek)
	if err := saveKeyring(dek); err != nil {
		return fmt.Errorf("gravar DEK no keychain: %w", err)
	}
	return adoptDekIDInWrapsIfMissing(ctx, store, newID)
}

// adoptDekIDInWrapsIfMissing varre os wraps conhecidos
// (master/recovery) e popula `dek_id` quando estiver vazio. Wraps com
// `dek_id` já populado são deixados intactos — se divergem do `newID`
// é responsabilidade do caller regravar o wrap explicitamente.
func adoptDekIDInWrapsIfMissing(ctx context.Context, store Store, newID string) error {
	for _, kind := range []string{KeyWrapKindMaster, KeyWrapKindRecovery} {
		wrap, err := store.GetKeyWrap(ctx, kind)
		if err != nil {
			return fmt.Errorf("ler wrap %q: %w", kind, err)
		}
		if wrap == nil {
			continue
		}
		if wrap.DekID == newID {
			continue
		}
		if wrap.DekID != "" && wrap.DekID != newID {
			log.Printf("[Credentials] AVISO: wrap %q tem dek_id=%s mas DEK do keychain=%s — wrap NÃO foi atualizado (regenerá-lo é responsabilidade do caller que rotacionou a DEK)", kind, wrap.DekID, newID)
			continue
		}
		wrap.DekID = newID
		if err := store.SaveKeyWrap(ctx, *wrap); err != nil {
			return fmt.Errorf("popular dek_id no wrap %q: %w", kind, err)
		}
	}
	return nil
}
