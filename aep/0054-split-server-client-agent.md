# AEP-0054 — Separação Servidor/Clientes e Execução Local

**Status**: 📝 Draft

**Criado em**: 2026-04-29

**Revisado em**: 2026-09-07

**Depende de**: AEP-0040 (Backend-Driven Messaging), AEP-0052 (Contas de Usuário), AEP-0076 (Schema e Migrações)

**Relacionado**: AEP-0042 e AEP-0080 (Surface Context), AEP-0056 (Abas Autocontidas), AEP-0082 e AEP-0092 (Trust/Allowlists), AEP-0084 e AEP-0086 (Providers ACP), AEP-0088 (Borda Wails)

---

## Resumo

Esta AEP propõe separar, no futuro, três papéis hoje distribuídos dentro do
aplicativo desktop:

- **Servidor do Assistente**: fonte de verdade dos recursos persistentes e
  executor do pipeline de conversa;
- **Clientes**: interfaces desktop, CLI ou futuras interfaces remotas;
- **Executor local de tools**: processo associado à máquina da pessoa, capaz de
  executar operações explicitamente autorizadas sobre filesystem, terminal e
  outras capacidades locais.

A separação deve ser feita por adaptadores sobre os mesmos controllers, casos de
uso e contratos de eventos já existentes. Ela não autoriza um segundo fluxo de
mensagens, não transforma a API HTTP de autenticação atual em uma Resource API
completa e não declara prontos os modos server-only ou thin client.

O “executor local” desta proposta **não é um provider ACP**. Providers ACP são
agentes de código que participam como providers LLM, conforme AEP-0084/AEP-0086,
usam suas próprias tools e já têm arquitetura concluída. O executor aqui
descrito seria uma borda de execução delegada pelo servidor para tools do
Assistente e ainda não existe.

---

## Motivação

### 1. Permitir novos clientes sem duplicar regras

O desktop usa binds Wails por domínio, consolidados pela AEP-0088. Uma interface
remota futura precisa reutilizar a mesma camada de aplicação, sem reimplementar
autenticação, ownership, validação ou envio de mensagens em handlers HTTP.

### 2. Separar recursos do servidor de capacidades da máquina

Conversas, mensagens e demais recursos persistentes pertencem à instância que os
armazena. Filesystem, terminal e contexto de editor pertencem à máquina que os
expõe e não podem ser presumidos pelo servidor remoto.

### 3. Preservar o produto local

O modo desktop local e offline continua sendo um requisito. A evolução para
outros modos de distribuição não deve obrigar o frontend local a trocar Wails
por HTTP antes de existir paridade, segurança e benefício demonstrável.

---

## Estado atual e limites

- `internal/httpapi` expõe somente endpoints de cofre, autenticação, sessão e
  JWKS previstos pela AEP-0052. Não há CRUD geral de recursos, endpoint HTTP de
  `SendMessage`/`RetryMessage` nem streaming de chat.
- A borda desktop usa binds de domínio em `internal/wailsapi`; a AEP-0088 está
  concluída. Esses binds são o transporte canônico atual do frontend.
- O pipeline de chat é backend-driven e possui contratos únicos de
  `SendMessage` e `RetryMessage`, conforme AEP-0040.
- O contexto de superfície evoluiu do contrato histórico da AEP-0042 para o
  envelope estrito da AEP-0080.
- Providers ACP executam como providers LLM locais. Eles não implementam o Tool
  RPC servidor→executor proposto aqui.
- Não existe hoje binário server-only, cliente desktop thin nem executor local
  de tools separado.

---

## Decisões

### D1. Casos de uso são o contrato interno; transportes são adaptadores

Wails, CLI e uma Resource API futura devem chamar os mesmos controllers/casos de
uso e compartilhar DTOs neutros. Regras de domínio não podem ser copiadas para
handlers de transporte. A separação física de processos vem depois dessa
fronteira, não antes.

### D2. Messaging continua com um único pipeline

Qualquer transporte futuro deve preservar integralmente a AEP-0040:

- mensagem nova passa por `SendMessage`;
- retry explícito passa por `RetryMessage`;
- conversa existente é pré-requisito;
- o backend é a fonte de verdade, sem mensagens otimistas locais;
- todo evento de chat carrega `conversationId` e origem de superfície;
- streaming remoto é apenas outro transporte para o protocolo vigente.

HTTP, SSE ou WebSocket não podem criar endpoints com semântica paralela.

### D3. Wails permanece canônico no desktop durante a evolução

A Resource API será aditiva. O frontend desktop não será migrado em bloco para
HTTP, e o servidor local não será iniciado obrigatoriamente, enquanto não houver
paridade e um plano de migração próprio. Os binds multi-domínio da AEP-0088 não
são dívida a remover por esta AEP.

### D4. A API HTTP existente é fundação de auth, não prova da Resource API

Os endpoints atuais de cofre/auth/JWKS e os modos `local`/`external` permanecem
regidos pela AEP-0052. Ampliar a superfície HTTP exige inventário de recursos,
autorização por operação, paginação, idempotência, versionamento e testes de
isolamento. Nenhum desses itens é considerado implementado por este documento.

### D5. O servidor é fonte de verdade dos recursos que hospeda

Recursos persistentes expostos remotamente são escopados pelo usuário e
resolvidos na instância servidora. O cliente renderiza snapshots e envia
comandos, sem reconstruir regras de ownership. Migrações e compatibilidade de
schema seguem a AEP-0076.

### D6. O executor local é uma capacidade explícita e autenticada

Uma futura delegação de tool para a máquina cliente deve usar um protocolo
tipado, correlacionado e cancelável, com pelo menos:

- identidade de instância, usuário, sessão, conversa, turno e tool call;
- negociação de capabilities e versão de protocolo;
- timeout, cancelamento e resultado idempotente;
- autorização explícita e auditável por operação;
- limites e saneamento para entrada e saída não confiáveis;
- reconexão que não repita silenciosamente operações com efeito colateral.

Não se presume que toda tool de filesystem ou terminal possa ser delegada.

### D7. Providers ACP e executor local têm papéis distintos

Provider ACP é o interlocutor do turno e usa tools do próprio agente. Executor
local executaria tools do Assistente a pedido do pipeline servidor. Eventos,
permissões e catálogos não podem misturar essas origens. Reutilização de
infraestrutura só é permitida quando o contrato permanecer semanticamente
explícito.

### D8. Contexto de superfície segue o envelope vigente

Clientes enviam identidade e contexto estruturados conforme AEP-0042/AEP-0080,
incluindo `surfaceType`, `surfaceId` e `snapshotVersion` quando houver contexto
transitório. O servidor não infere a origem pela aba ativa e tools mutáveis não
agem sobre alvo ambíguo.

### D9. Workspaces locais não viram filesystem do servidor

O servidor remoto não presume acesso ao diretório do workspace da pessoa.
Persistência, espelhamento ou sincronização de arquivos exigem decisão
arquitetural própria, incluindo conflitos, privacidade, quotas e modo offline.
O PR #101 propõe esse tema, mas não integra o contrato canônico e precisa ser
reavaliado separadamente antes de servir como dependência.

### D10. Trust e decisões bloqueantes valem através da fronteira

Delegação local deve respeitar as políticas de rede/filesystem das AEPs 0082 e
0092. Perguntas bloqueantes usam o contrato acessível da AEP-0091 e são roteadas
à superfície que realmente possui interlocutor; ausência de resposta nunca
equivale a autorização.

### D11. Modos de distribuição são metas, não entregas

Esta proposta admite, sem declará-los implementados:

1. **desktop local**: composição atual, offline, com Wails;
2. **server-only**: processo headless com API versionada e sem dependência de
   runtime gráfico;
3. **thin client**: desktop conectado a servidor remoto, opcionalmente com
   executor local autenticado.

Cada modo exige entrypoint, configuração, threat model e validação operacional
próprios.

---

## Fases

### Fase 0 — Contratos e threat model

- Inventariar recursos e operações candidatas à Resource API.
- Definir versionamento, erros, paginação, idempotência e autorização.
- Definir transporte do protocolo de eventos sem alterar a AEP-0040.
- Modelar confiança, pairing, revogação e reconexão do executor local.

### Fase 1 — Adaptador HTTP de um domínio não crítico

- Expor uma fatia vertical pequena sobre controller/caso de uso existente.
- Provar isolamento entre usuários e equivalência semântica com Wails.
- Manter o desktop no transporte atual.

### Fase 2 — Messaging remoto

- Expor `SendMessage`/`RetryMessage` sem duplicar pipeline.
- Transportar eventos tipados com `conversationId` e origem de superfície.
- Cobrir desconexão, retomada, backpressure e cancelamento.

### Fase 3 — Executor local

- Implementar pairing e negociação de capabilities.
- Delegar uma tool de baixo risco com autorização e auditoria.
- Provar que retry/reconexão não duplica efeitos.

### Fase 4 — Distribuições alternativas

- Criar e validar server-only.
- Só então avaliar thin client e eventual migração seletiva do desktop.

---

## Riscos

- **Segunda arquitetura acidental**: handlers HTTP podem duplicar Wails/casos de
  uso. Mitigação: adaptadores finos e testes de equivalência.
- **Ampliação de superfície de ataque**: auth HTTP hoje limitada pode virar API
  pública sem hardening suficiente. Mitigação: threat model e rollout por
  domínio.
- **Execução remota indevida**: servidor comprometido pode tentar operar a
  máquina cliente. Mitigação: capabilities mínimas, consentimento, revogação e
  fail-closed.
- **Repetição de efeitos**: reconexão pode duplicar comandos ou edições.
  Mitigação: correlação, idempotência e estados terminais persistidos.
- **Confusão com ACP**: duas noções de agente podem compartilhar nomes e eventos.
  Mitigação: papéis e origens distintos conforme D7.
- **Divergência offline/remoto**: modos diferentes podem produzir regras
  distintas. Mitigação: casos de uso compartilhados e suites de contrato.

---

## Critérios de aceitação

- [x] A proposta distingue o estado implementado do alvo futuro.
- [x] A proposta preserva AEP-0040, AEP-0052, AEP-0080 e AEP-0088.
- [x] Executor local e provider ACP têm papéis não ambíguos.
- [ ] Resource API versionada cobre ao menos uma fatia vertical sobre caso de
  uso compartilhado, com isolamento por usuário testado.
- [ ] Messaging remoto reutiliza `SendMessage`/`RetryMessage` e transporta
  eventos com `conversationId` e origem de superfície.
- [ ] Executor local possui pairing, capabilities, autorização, cancelamento,
  idempotência e auditoria.
- [ ] Server-only e thin client têm entrypoints e validação operacional próprios.
