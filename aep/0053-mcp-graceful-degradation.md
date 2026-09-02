# 0053 — Degradação graciosa de MCP nativo no chat

Status: In Progress — política compartilhada e OpenAI entregues; classificação no adapter Anthropic e matriz de regressões permanecem parciais

Autor: Leonardo Gleison Ferreira (Leo) / Assistente
Data: 2026-04-23

## Resumo executivo

Hoje, um único servidor MCP nativo que falha durante autenticação, listagem de tools ou execução server-side pode abortar a resposta inteira do chat. Isso acontece mesmo quando os demais servidores MCP da mesma request continuam válidos e mesmo quando a falha parece recuperável.

Esta AEP define uma política de degradação graciosa por tentativa: o servidor
que falhou é removido apenas da request atual, a chamada ao provider é refeita
sem ele e o `MCP Manager` dispara uma recuperação best-effort para restaurá-lo
em chamadas futuras. A política compartilhada e a tradução OpenAI estão
implementadas. O Anthropic já reutiliza a orquestração, mas a classificação de
`mcp_tool_result.IsError` no stream real ainda está pendente.

O retry é deliberadamente limitado e só acontece dentro de uma janela segura: antes de qualquer saída visível da tentativa atual. Se já houve texto emitido ao usuário, o erro continua fatal para evitar duplicação ou corrupção do streaming.

## Motivação / problema atual

- A checagem atual de elegibilidade (`GetEligibleNativeMCPServers()`) é necessária, mas insuficiente. Ela diz quais servidores podem entrar na request, não quais continuarão saudáveis durante esta tentativa específica.
- OpenAI Responses e Anthropic MCP Connector tratam erros de MCP como falha terminal do stream quando o provider encerra a resposta com erro.
- O `MCP Manager` já possui health check, refresh de token e reconnect, mas esses mecanismos são assíncronos e não participam da decisão do chat em tempo real.
- Na prática, um erro localizado em um conector remoto degrada a experiência inteira do chat, mesmo quando seria aceitável continuar sem aquele servidor.

## Objetivos

- Permitir que o chat continue sem o servidor MCP nativo que falhou, quando isso for seguro.
- Remover apenas o servidor problemático, sem descartar os demais MCPs da request.
- Acionar recuperação best-effort por slug para que o servidor possa voltar nas próximas chamadas.
- Manter a política de degradação provider-agnostic, deixando para cada provider apenas a adaptação de formatos de erro/evento.
- Cobrir a política com testes focados em listagem, autenticação, falha de servidor e continuação da resposta sem MCP.

## Não objetivos

- Não cobre MCP via bridge/adapter local; o escopo é MCP nativo server-side.
- Não tenta "retomar" uma resposta após texto já emitido ao usuário.
- Não altera o contrato de eventos do frontend.
- Não redefine a lógica de elegibilidade base nem a decisão HTTP nativo vs bridge.
- Não depende da migração de persistência proposta na AEP-0049.

## Modelo de estados

Para evitar ambiguidade, esta AEP separa quatro estados diferentes:

| Estado | Escopo | Definição |
|---|---|---|
| Elegível | catálogo local | O servidor passa pelos filtros locais para entrar no caminho MCP nativo: conectado, HTTP remoto elegível, com tools, sem `prefer_bridge`. |
| Saudável para esta tentativa | request atual | Nesta tentativa específica ao provider, ainda não houve evidência de falha degradante para este servidor. |
| Recuperável | runtime + manager | A falha sugere ação de refresh/reconnect/token renewal que pode restaurar o servidor para chamadas futuras. |
| Degradado | request atual | O servidor foi removido do conjunto de MCPs da resposta atual após uma falha classificada como degradável. |

### Leitura correta dos estados

- Um servidor pode ser **elegível** e, ainda assim, não estar **saudável para esta tentativa**.
- Um servidor **degradado** na request atual pode continuar **recuperável** para a próxima.
- A elegibilidade continua sendo decidida antes do envio ao provider; a saúde por tentativa só pode ser inferida durante o runtime do stream.

## Proposta

### 1. Política compartilhada de degradação

Criar um componente compartilhado em `internal/llm/` para tomar decisões de retry/degradação a partir de uma representação normalizada da falha MCP.

Estruturas esperadas:

```go
type MCPFailureStage string

const (
    MCPFailureStageListTools MCPFailureStage = "list_tools"
    MCPFailureStageCall      MCPFailureStage = "call"
    MCPFailureStageHandshake MCPFailureStage = "handshake"
    MCPFailureStageUnknown   MCPFailureStage = "unknown"
)

type MCPAttemptFailure struct {
    ServerName      string
    ServerSlug      string
    Stage           MCPFailureStage
    Message         string
    Recoverable     bool
    Degradable      bool
}
```

A política compartilhada decide entre:

- manter o fluxo atual;
- refazer a tentativa sem um servidor específico;
- propagar erro fatal.

Regras:

1. Só degradar quando a falha estiver associada a um servidor identificável.
2. Só degradar quando ainda não houver saída visível da tentativa.
3. Cada retry remove no máximo um servidor novo.
4. O total de retries por resposta é limitado a `min(3, quantidade_inicial_de_servidores_mcp)`.
5. Um servidor removido da resposta atual não volta para a mesma resposta, mesmo que a recuperação seja bem-sucedida.

### 2. Adaptadores por provider

Cada provider nativo continua responsável apenas por traduzir seus sinais específicos para `MCPAttemptFailure`.

#### OpenAI Responses

Mapear eventos/erros como:

- `response.mcp_list_tools.failed`
- `response.mcp_call.failed`
- `response.failed` quando a causa raiz for claramente um conector MCP

O provider OpenAI não decide a política sozinho; ele apenas informa:

- qual servidor falhou;
- em que estágio;
- se a falha parece degradável;
- se ainda estamos dentro da janela segura de retry.

#### Anthropic Messages + MCP Connector

Mapear para a mesma estrutura comum:

- falhas de stream beta relacionadas a `mcp_servers`;
- erros de autenticação/conector reportados pelo stream;
- falhas em `mcp_tool_result`/`mcp_tool_use` quando o conector já indica o servidor problemático.

Se o erro não puder ser atribuído com segurança a um servidor específico, a política não degrada e o erro permanece fatal.

### 3. Helper síncrono de recuperação no MCP Manager

Adicionar ao `MCP Manager` um helper síncrono e com timeout curto para recuperação best-effort por slug, por exemplo:

```go
type MCPRecoveryResult struct {
    Attempted   bool
    Refreshed   bool
    Reconnected bool
    Err         error
}

func (m *Manager) RecoverServerBestEffort(ctx context.Context, slug string) MCPRecoveryResult
```

Comportamento esperado:

1. Verifica se o slug ainda existe e continua habilitado.
2. Se o servidor usa OAuth, tenta refresh de token quando fizer sentido.
3. Em seguida, tenta `refreshServerOfferings()` ou `Reconnect()` conforme o estado atual.
4. Nunca bloqueia indefinidamente; usa timeout curto e retorna resultado observável em logs.

Esse helper serve para a próxima chamada, não para recolocar o servidor na resposta em andamento.

### 4. Fluxo por tentativa

Fluxo proposto para providers com MCP nativo:

1. O chat recebe o conjunto inicial de servidores **elegíveis**.
2. O provider inicia a tentativa com esse conjunto.
3. Se ocorrer falha MCP degradável antes de qualquer saída visível:
   - o servidor é marcado como **degradado** para a resposta atual;
   - o `MCP Manager` executa `RecoverServerBestEffort()` para o slug;
   - a mesma request é refeita sem esse servidor.
4. Se outro servidor falhar na nova tentativa, o processo pode se repetir dentro do limite configurado.
5. Se não restar servidor MCP, o chat continua sem MCP nativo.
6. Se a falha ocorrer após saída visível, ou se não for classificável/degradável, o erro continua fatal.

## Janela segura de retry

Esta AEP assume explicitamente uma janela segura de retry: antes de qualquer saída visível da tentativa.

Razão:

- O provider não oferece mecanismo confiável de "continuar do ponto exato" após `response.failed`.
- Reiniciar uma resposta depois de já ter emitido texto pode duplicar conteúdo ou quebrar a semântica do stream.
- O ganho principal já vem das falhas precoces de handshake, auth e listagem, que hoje são as que mais interrompem a resposta inteira.

## Observabilidade

Sem introduzir novo contrato de evento para o frontend, o backend deve registrar em log:

- tentativa inicial e retries de degradação;
- slug removido da resposta;
- razão classificada da falha;
- resultado do `RecoverServerBestEffort()`;
- esgotamento do limite de retries.

Logs esperados:

```text
[MCP-DEGRADE] attempt=1 provider=openai server=atlassian stage=list_tools action=retry_without_server
[MCP-RECOVER] slug=atlassian refreshed=true reconnected=false err=nil
```

## Testes

Cobertura mínima:

### OpenAI

1. [x] Falha MCP identificável remove o servidor e a resposta continua sem ele.
2. [ ] Cobrir explicitamente falha de autenticação MCP disparando
   `RecoverServerBestEffort()` e retry sem o servidor.
3. [ ] Cobrir explicitamente `response.failed` não-MCP permanecendo fatal.
4. [ ] Cobrir explicitamente ausência de retry após output visível.

### Anthropic

1. [x] A orquestração compartilhada do provider gera retry sem o servidor
   quando `betaAttemptFn` injeta uma falha já normalizada.
2. [ ] Cobrir no adapter Anthropic falha não atribuível a slug permanecendo
   fatal.
3. [x] O teste de orquestração conclui a resposta após remover o servidor.
4. [ ] Fazer `doStreamBeta` classificar `mcp_tool_result` com `IsError` via
   `inferMCPFailure` antes de considerar o adapter Anthropic completo.

### MCP Manager

1. [x] `RecoverServerBestEffort()` tenta refresh OAuth quando aplicável.
2. [x] `RecoverServerBestEffort()` tenta reconnect/refresh de forma
   idempotente.
3. [x] Erros são retornados e a chamada feita pelo planner usa timeout sem
   travar o chat.

## Fases sugeridas

### Fase 1 — Política compartilhada ✅

- [x] Criar tipos normalizados de falha MCP.
- [x] Implementar a decisão compartilhada de retry/degradação.

### Fase 2 — Hook de recuperação ✅

- [x] Expor `RecoverServerBestEffort()` no `MCP Manager`.
- [x] Adicionar testes unitários do helper.

### Fase 3 — OpenAI ✅

- [x] Adaptar eventos/erros do Responses API para a estrutura comum.
- [x] Cobrir o caminho integrado de degradação e conclusão sem o servidor.

### Fase 4 — Anthropic 🚧

- [x] Integrar o loop de retry compartilhado ao provider Anthropic.
- [x] Cobrir a orquestração com `betaAttemptFn` retornando falha normalizada e
  a conclusão sem o servidor.
- [ ] Classificar no adapter real `doStreamBeta` o
  `mcp_tool_result.IsError` via `inferMCPFailure`; hoje ele apenas emite
  `MCPToolEvent` com erro.
- [ ] Cobrir a classificação real do stream beta sem depender da injeção de
  `mcpStreamAttemptResult` pelo teste.

### Fase 5 — Matriz de regressões 🚧

- [ ] Completar os casos pendentes listados em **Testes**, sobretudo o gate
  após output visível e falhas fatais/não atribuíveis em cada adapter.

## Arquivos afetados

### Novos

- `internal/llm/mcp_degradation.go`
- `internal/llm/mcp_degradation_test.go`

### Modificados

- `internal/llm/openai_provider.go`
- `internal/llm/anthropic_provider.go`
- `internal/mcp/manager.go`
- `internal/chat/tool_defs.go` ou ponto equivalente de montagem do conjunto inicial, se necessário para plugar o helper de runtime

## Dependências e conflitos com AEPs relacionadas

### AEP-0021 — MCP Modo Nativo

Sem conflito. A AEP-0021 define quem é elegível ao caminho nativo; esta AEP adiciona uma camada posterior de saúde por tentativa e degradação controlada.

### AEP-0033 — MCP OAuth Auto-Discovery

Sem conflito. A AEP-0033 trata da descoberta/configuração de OAuth; esta AEP apenas reaproveita o runtime de refresh/reconnect quando a falha indicar recuperação possível.

### AEP-0037 — SDK Migration + ChatProvider Interface

Sem conflito. A política proposta opera dentro das implementações de provider já introduzidas pela AEP-0037 e não exige alteração do contrato público do `ChatProvider`.

### AEP-0039 — Tool Calling Revamp

Sem conflito. AEP-0039 é focada em observabilidade, eventos e resiliência do executor local. Esta AEP trata do caminho MCP nativo server-side, antes da execução local de tools.

### AEP-0040 — Backend-Driven Messaging

Sem conflito. Não há necessidade de alterar o protocolo de eventos do chat; a degradação fica inteiramente no backend/provider.

### AEP-0049 — Migração de MCP Servers para Banco de Dados

Relacionada, mas sem conflito. AEP-0049 muda o backing store e adiciona logs persistidos para servidores MCP. Esta AEP atua na política de runtime do chat. O ponto de integração deve permanecer no `MCP Manager`, de forma que a futura troca de persistência não altere o contrato de recuperação best-effort.

### PRs abertas consultadas para numeração

As PRs abertas já reservam as AEPs `0046` a `0052`. Por isso, esta proposta usa `0053`.

## Critérios de aceitação

1. [x] A política compartilhada e o adapter OpenAI permitem que um servidor
   MCP nativo com falha degradável não aborte a resposta inteira antes de
   output visível.
2. [x] O retry refaz a request sem o servidor problemático.
3. [x] O servidor degradado dispara recuperação best-effort por slug.
4. [x] O retry é limitado e não entra em loop infinito.
5. [x] Falhas já classificadas como não degradáveis ou fora da janela segura
   continuam fatais no código vigente.
6. [ ] OpenAI e Anthropic possuem a matriz mínima completa de testes descrita
   nesta AEP.
7. [ ] O adapter Anthropic real classifica `mcp_tool_result.IsError` como
   candidato à política compartilhada antes de output visível.

## Evidências da implementação

- política compartilhada, remoção por slug e limite de retries:
  `internal/llm/mcp_degradation.go` e
  `internal/llm/mcp_degradation_test.go`;
- OpenAI Responses e gate anterior a output visível:
  `internal/llm/openai_responses.go` e
  `internal/llm/openai_responses_test.go`;
- Anthropic — apenas wiring e orquestração com resultado normalizado injetado:
  `internal/llm/anthropic_provider.go` e
  `internal/llm/chat_provider_test.go`; o teste não prova a classificação de
  `mcp_tool_result.IsError` por `doStreamBeta`;
- recuperação OAuth/reconnect:
  `internal/mcp/manager.go` e `internal/mcp/manager_test.go`;
- wiring do callback de recuperação:
  `internal/chat/tool_selection_policy.go`.

O helper do Manager é síncrono, mas o planner o chama em goroutine com timeout
de cinco segundos. Assim, a recuperação permanece best-effort para chamadas
futuras sem bloquear o retry do chat.

## Referências

- `aep/0021-mcp-native-mode.md`
- `aep/0033-mcp-oauth-autodiscovery.md`
- `aep/0037-sdk-migration-chat-provider.md`
- `aep/0039-tool-calling-revamp.md`
- `internal/mcp/manager.go`
- `internal/llm/openai_provider.go`
- `internal/llm/anthropic_provider.go`
