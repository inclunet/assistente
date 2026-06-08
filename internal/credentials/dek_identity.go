package credentials

import (
	"crypto/sha256"
	"encoding/hex"
)

// dekIdentityDomainV1 é o domain-separation tag usado no cálculo do
// `dek_id`. Existe para garantir que o hash NUNCA possa ser confundido
// com qualquer outro hash que use a DEK como input (ex.: HMAC interno,
// fingerprint de outra finalidade). Se um dia precisarmos rotacionar o
// algoritmo, criamos `…-v2` e migramos.
const dekIdentityDomainV1 = "assistente-dek-id-v1\x00"

// DEKIdentity calcula um identificador determinístico de 32 chars hex
// (16 bytes) a partir de uma DEK. A função é one-way (sha256) e usa
// domain-separation, então:
//
//   - DEKs iguais geram o mesmo `dek_id`.
//   - DEKs diferentes geram `dek_id`s diferentes (com probabilidade
//     overwhelmingly close to 1 — colisão em 128 bits é desprezível).
//   - O `dek_id` pode ser persistido junto com os wraps no banco e
//     comparado com `DEKIdentity(DEK_atual_do_keychain)` para detectar
//     divergência sem expor a DEK ou a senha mestre.
//
// É a base do contrato "DEK do keychain == DEK que cifrou as credenciais"
// (ver AEP-0061). Toda escrita de DEK no keychain DEVE atualizar o
// `dek_id` correspondente nos wraps; toda inicialização do
// credentials.Manager DEVE comparar `dek_id` do keychain com o dos wraps
// e, se divergirem, recusar persistir novas credenciais e expor o
// estado pra UI.
//
// Retorna string vazia se `dek` for vazio (caso de boot pré-Setup).
func DEKIdentity(dek []byte) string {
	if len(dek) == 0 {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(dekIdentityDomainV1))
	h.Write(dek)
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16])
}
