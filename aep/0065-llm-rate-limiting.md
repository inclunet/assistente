# AEP-0065 — Rate limiting nas chamadas ao provedor LLM

Status: Done
Data: 2026-06-01
Atualizado: 2026-08-21
Autor: Cursor Agent (Inclunet)

## Resumo

Define um mecanismo central de **rate limiting** para as chamadas de geração ao
provedor LLM, escopado **por usuário e perfil**, usando um token bucket
(`golang.org/x/time/rate`). O objetivo é evitar custos inesperados e estouro de
cota causados por uso excessivo, loops de agentes ou erros em cascata, sem
travar a UI e sem espalhar lógica de throttling pelo código.

Resolve o GitHub issue #27 ("security: implementar rate limiting nas chamadas LLM").

## Motivação

Hoje não há controle de taxa nas chamadas ao provedor LLM. Cenários de risco:

- **Loops de agentes**: o loop agêntico pode disparar várias iterações em
  sequência (cada iteração é uma chamada ao provedor). Um perfil mal configurado
  ou um modelo "preso" em tool calling pode gerar muitas chamadas rápidas.
- **Retries em cascata**: a recuperação automática de streaming (AEP-0064) e os
  retries de tool podem multiplicar chamadas em situações de erro.
- **Uso abusivo**: múltiplos envios em rajada.

Cada chamada consome cota/custo no provedor. Sem um teto, o gasto é ilimitado.

## Decisões

### 1) Escopo: por usuário e perfil

O limite é aplicado **por usuário e perfil**, usando o `userID` já carimbado no contexto
(AEP-0052, multi-user). Quando o contexto não tem `userID` (fluxos internos sem
escopo), uma **chave global** é usada para que nenhuma chamada escape do
controle. O slug do perfil completa a chave para que perfis com políticas
distintas não drenem nem reconfigurem o bucket um do outro. Quando não há slug,
usa-se o escopo do perfil ativo.

### 2) Estratégia: token bucket via `golang.org/x/time/rate`

Um `*rate.Limiter` por chave (usuário + perfil), guardado em um mapa protegido por mutex.
Conforme proposto no issue, usamos `golang.org/x/time/rate` (pinado em v0.8.0
para não forçar bump da diretiva `go`/toolchain).

> Nota: o pacote `internal/httpapi` mantém um token bucket próprio para os
> endpoints HTTP públicos (decisão local de evitar dependência para ~30 linhas).
> Aqui seguimos a sugestão explícita do issue e usamos a lib idiomática, pois o
> limitador de LLM precisa de semântica de refill precisa e testável.

### 3) Ponto de aplicação: central, no factory de ChatProvider

O limite é aplicado em **um único ponto**: `providers.Service.GetChatProvider`
embrulha o `ChatProvider` resolvido com um decorator `rateLimitedProvider`
(pacote `llm`). Assim, todos os consumidores que resolvem o provider por esse
factory — o loop agêntico e o streaming simples do pipeline único de
`SendMessage` (AEP-0040) — passam pelo mesmo controle, sem duplicação.

Apenas os métodos de **geração** são contabilizados: `StreamChat`, `SendChat`,
`SimpleChat`. `GetModels` (metadata leve usada pela UI de configurações) **não**
é limitado.

A **sumarização de conversas** (`internal/summarization`) constrói o provider
diretamente via `llm.NewChatProvider(...)` (não passa pelo `GetChatProvider`),
mas também é um vetor de custo. Por isso ela recebe o **mesmo** `*llm.RateLimiter`
e a mesma `RateLimitKeyFunc` por injeção e embrulha o provider com
`llm.NewRateLimitedProvider`, compartilhando o bucket por usuário e perfil com o chat.
Como a sumarização é background/best-effort, um eventual bloqueio apenas adia o
resumo (o erro já é tratado), sem impacto na UI.

### 4) Defaults sensatos e configuração

- `RequestsPerMinute` (default **60**): teto sustentado por usuário (~1/s).
- `Burst` (default **30**): rajada instantânea. Deliberadamente
  `>= MaxAgenticIterations` (default 25) para **não interromper um loop agêntico
  legítimo** que dispare várias iterações em sequência rápida.
- Configuração persistida em `Profile.Chat`, editável na guia **Modelos**:
  - `rate_limit_enabled` (bool; ausente em perfil legado equivale a `true`);
  - `rate_limit_rpm` (int; zero/ausente usa 60);
  - `rate_limit_burst` (int; zero/ausente usa 30).
- As antigas variáveis `ASSISTENTE_LLM_RATE_LIMIT_*` foram removidas. Em um
  aplicativo desktop, o perfil é a fonte única e acessível dessa configuração.
- Uma alteração de política atualiza taxa e rajada do bucket in-place e vale
  para novas chamadas imediatamente, sem devolver tokens já consumidos. A
  política é relida pelo slug antes de cada geração, para que um turno antigo
  não restaure no bucket a configuração que estava vigente quando começou.

### 5) Tratamento ao atingir o limite

- **Streaming** (`StreamChat`): quando barrado, o decorator sinaliza
  `handler.OnError(...)` com uma mensagem clara. O handler finaliza o turno com
  `chat:done` de erro (contrato existente), **sem travar a UI** e sem chamar o
  upstream.
- **Síncrono** (`SendChat`/`SimpleChat`): retorna `*llm.RateLimitError`, que
  carrega `RetryAfter` (tempo sugerido para backoff). `IsRateLimitError(err)`
  permite identificação tipada pelos callers.

### 6) Alerta de proximidade

O `RateLimiter` aceita um callback opcional (`SetNearLimitHandler`) disparado
quando, após permitir uma chamada, os tokens restantes caem abaixo de um limiar
(default 20% da rajada). O alerta é **edge-triggered por chave (usuário + perfil)**:
dispara apenas na transição acima→abaixo do limiar e é rearmado quando a chave
volta acima, evitando spam de log/telemetria sob uso sustentado. O estado por
chave é guardado em um mapa protegido pelo mesmo mutex do limitador; o callback
é invocado fora do lock. Na wiring atual o alerta é **logado**. A propagação do
alerta para a UI (evento dedicado) fica como follow-up.

> Implementação: `allowAt` usa **uma única** `ReserveN(now, 1)` para decidir e,
> quando barrado, derivar o `RetryAfter` (cancelando a reserva só quando há
> atraso > 0). Isso evita o estado inconsistente que surgiria ao combinar
> `AllowN` + `ReserveN` sob concorrência e preserva o contrato "permitido
> consome 1 token; bloqueado não consome".

## Fases

1. **Núcleo** (`internal/llm/ratelimit.go`): `RateLimitConfig`, `RateLimiter`
   reconfigurável e `RateLimitError`. ✅
2. **Decorator** (`internal/llm/ratelimit_provider.go`): `rateLimitedProvider`
   implementando `ChatProvider`. ✅
3. **Wiring** (`providers.Service` + `app.go`): injeção do limitador e da função
   de chave (userID do contexto). ✅
4. **Perfil e UI**: persistência, compatibilidade com perfis legados e controles
   acessíveis na guia Modelos. ✅
5. **Testes**: dentro do limite, acima do limite, reset após intervalo,
   isolamento, reconfiguração dinâmica, persistência e UI. ✅
6. **Follow-up**: evento/UX dedicado para o alerta de proximidade.

## Riscos

- **Burst pequeno demais** interromperia loops agênticos legítimos. Mitigado com
  `Burst` default >= `MaxAgenticIterations` e documentação da relação.
- **Crescimento do mapa de buckets**: um bucket por usuário e perfil. Em uso
  local (desktop), esse conjunto é finito; GC/expiração de buckets ociosos fica
  como follow-up se necessário (mesma postura do `httpapi`).
- **Mensagem de erro em pt-BR no backend**: segue o padrão atual de erros do
  backend exibidos diretamente. i18n completa por código de erro no frontend é
  follow-up alinhado ao restante das mensagens de erro do backend.

## Critérios de aceitação

- [x] Rate limiter por usuário e perfil usando `golang.org/x/time/rate`.
- [x] Limites configuráveis no perfil, com defaults sensatos e compatibilidade legada.
- [x] Nenhuma configuração por variável de ambiente.
- [x] Aplicação central no caminho de chamada LLM (sem espalhar lógica).
- [x] Erro claro com backoff (`RetryAfter`) sem travar a UI.
- [x] Alerta quando o uso se aproxima do limite (log; UI = follow-up).
- [x] Testes: dentro do limite passa, acima é barrado, reset após intervalo.
- [x] `go build ./...` e `go test ./...` (pacotes afetados) verdes.
