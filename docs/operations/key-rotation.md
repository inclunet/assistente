# Rotação de chave JWT

> Procedimento operacional para trocar a chave Ed25519 que assina os
> access tokens emitidos pelo `assistente`. Aplicável quando há
> suspeita de comprometimento da chave, ou em política de rotação
> periódica (não obrigatória hoje).

## Contexto

O assistente assina access tokens com uma chave Ed25519 persistida em
`internal-auth:jwt-signing-key` no cofre (instance secret, não
vinculada a um usuário específico). A chave é gerada na primeira
inicialização e permanece estável até ser explicitamente apagada.

Hoje **não** há rotação automática nem suporte a múltiplas chaves
simultâneas (key set com `kid`). Toda rotação é uma troca: a chave
nova substitui a antiga e tokens emitidos com a antiga deixam de
validar.

Limitação documentada em
[`internal/auth/signer_store.go`](../../internal/auth/signer_store.go)
(função `LoadOrCreateTokenSigner`). Multi-key com grace period entra
quando o claim mudar para "enterprise multi-tenant"
([AEP-0052](../../aep/0052-multi-user-accounts.md)).

## Quando rotacionar

- **Suspeita de comprometimento da chave**: após exposição acidental
  do disco, leak de backup, dump não-autorizado do cofre. Rotacione
  imediatamente.
- **Mudança de política de segurança**: política de rotação
  periódica (ex.: trimestral). Aceitável programar em janela de
  manutenção.
- **Reset do vault**: quando o usuário usa "Resetar cofre" na UI ou
  re-roda `asst setup` em ambiente já configurado, a chave antiga é
  invalidada como efeito colateral — não é necessário rotacionar
  explicitamente.

## Quando NÃO rotacionar

- Em horário de produção / pico de uso. Há janela de até 15 minutos em
  que access tokens em voo deixam de validar — clientes externos
  precisam tentar refresh para receber tokens novos. Em interfaces
  best-effort (Telegram, Signal) o impacto é zero; em integrações
  HTTP que não tolerem `401` por alguns minutos, agendar.
- Como teste exploratório. A operação não é destrutiva no DB, mas
  invalida sessões em voo — não use só para "ver o que acontece".
- Sem registrar quando aconteceu. Mantenha um log operacional
  (audit) de rotações para correlacionar eventuais incidentes
  posteriores ao período sem chave válida.

## Procedimento

A rotação consiste em apagar o instance secret e deixar o app gerar
uma chave nova no próximo boot.

### Passo 1 — Pausar consumidores externos (opcional)

Se há integração externa que consome `/.well-known/jwks.json` (ex.:
gateway HTTP com cache agressivo de JWKS), avisar o operador para
forçar refetch após a rotação. Nem sempre necessário — clientes bem
implementados refazem fetch ao receber `401 invalid signature`.

### Passo 2 — Apagar o instance secret

#### Opção A — Pela UI

1. Login com usuário **admin**.
2. Configurações → Cofre → Resetar cofre (ou trocar senha mestre).
3. Confirmar.

Resultado: a chave antiga é apagada junto com o resto do cofre.
**Atenção**: este caminho descarta TODAS as credenciais cifradas no
cofre — só use se o objetivo é reset completo.

#### Opção B — Via export/import (preserva outras credenciais)

```bash
# 1. Backup completo do cofre
asst data export --only-credentials \
  --credential-password "<senha-temporária-forte>" \
  --out cofre-pre-rotacao.json

# 2. Editar o arquivo: remover a entrada cujo `pattern` é
#    "internal-auth:jwt-signing-key" (preserve todas as outras).

# 3. Resetar o cofre via UI (Configurações → Cofre → Resetar) ou
#    via DB (DELETE FROM credential_entries WHERE pattern =
#    'internal-auth:jwt-signing-key' — só rode se o app estiver
#    parado).

# 4. Re-importar o backup editado
asst data import --credential-password "<senha-temporária-forte>" \
  cofre-pre-rotacao.json
```

### Passo 3 — Boot com chave nova

```bash
# Reiniciar o app
asst chat   # ou abrir o app desktop normalmente
```

No primeiro boot pós-rotação, o `LoadOrCreateTokenSigner` detecta que
o instance secret não existe e gera uma nova chave automaticamente.

### Passo 4 — Verificar

```bash
# Conectar à API HTTP local (porta padrão 18080) e checar JWKS
curl -s http://localhost:18080/.well-known/jwks.json | jq '.keys[].kid'
```

A `kid` deve ter mudado (timestamp diferente). Compare com a `kid`
registrada antes da rotação.

Para confirmar que tokens novos validam:

```bash
# Login (gera access token novo)
curl -s -X POST http://localhost:18080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"..."}' | jq '.accessToken'

# O token retornado deve ter `kid` igual ao da nova chave.
```

## Janela de impacto

- **Access tokens em voo**: param de validar imediatamente após o
  boot pós-rotação. TTL típico de access token = 15min, então no pior
  caso 15min de cliente externo recebendo `401`.
- **Refresh tokens**: continuam válidos. Cada cliente que tenta
  acessar com access token expirado faz refresh e recebe um access
  token novo assinado com a nova chave. Sem ação manual do usuário.
- **Sessões persistidas no DB**: não afetadas. Logout não é exigido.
- **Cofre de credenciais**: depende da opção escolhida (opção A
  recria, opção B preserva).

## Auditoria

Hoje a rotação não é registrada automaticamente. Convenção operacional
recomendada:

- Logar a rotação em ticket / runbook com data, hora, motivo,
  responsável, e `kid` antes/depois.
- Confirmar que `kid` novo aparece no JWKS (passo 4 acima).
- Manter cofre exportado pré-rotação por X dias (em local seguro)
  caso seja necessário reverter.

## Roadmap

- **Hoje (alpha multi-user single-tenant)**: procedimento manual
  documentado neste arquivo. Aceito como compromisso operacional.
- **Multi-key com grace period** (TODO sem prazo): aceitar tokens
  assinados com chave antiga durante uma janela configurável
  (`grace_period`); JWKS expõe ambas. Rotação vira zero-impact para
  consumidores externos. Pré-requisito para claim "enterprise
  multi-tenant".

