# AEP-0064 — Recuperação explícita de resposta interrompida (continuação) e cancelamento de geração

Status: Done — cancelamento, recuperação automática e continuação explícita implementados
Data: 2026-05-21
Autor: Leonardo Gleison (Inclunet) + GitHub Copilot

## Resumo

Este AEP define um fluxo **explícito** (não acidental) para recuperar uma resposta do assistente quando o streaming é interrompido por erro ou por cancelamento do usuário.

A proposta combina três pilares:

1. **Requests normais** de chat continuam terminando em `user` (nunca em `assistant` por acidente).
2. **Continuação de resposta** é uma ação explícita (UI + backend) que pode usar `assistant prefill` somente nesse modo.
3. O usuário ganha controle direto com **Cancelar geração** (botão durante streaming e item no menu, sempre disponíveis; e atalho `Esc` **apenas quando o foco está no campo de edição** — ver Decisão 4), além de **auto-recuperação padrão de 3 tentativas** antes de “falhar de vez”.

O termo “prefill” é um detalhe técnico. A UI usa termos amigáveis: “Continuar resposta”, “Cancelar geração”, “Tentar recuperar automaticamente”.

## Motivação

Durante ajustes anteriores, foi necessário remover mensagens `assistant` no fim do histórico antes de enviar requests OpenAI-compatíveis. Alguns modelos (especialmente Qwen/LocalAI quando “thinking” está ativo por padrão) podem interpretar um `assistant` trailing como **assistant prefill** e rejeitar o request, ou produzir comportamento inconsistente.

O bug imediato foi mitigado removendo `assistant` trailing. Porém, isso expôs uma oportunidade: se uma resposta foi gerada parcialmente antes de um erro de streaming, é útil conseguir **retomar** a partir do texto já produzido — desde que isso seja explícito e controlado.

Além disso, hoje uma falha de streaming tende a simplesmente “parar”, sem (a) tentativa automática de recuperação e sem (b) um mecanismo consistente de cancelamento acessível.

## Decisões

### 1) Invariante: requests normais terminam em `user`

- O payload normal enviado ao provider **não deve** carregar `assistant` trailing como “sobra” de histórico.
- A remoção de `assistant` trailing permanece como defesa padrão.

### 2) Continuação é um modo explícito (não ocorre por acidente)

- “Continuar resposta” é um modo explícito do pipeline (backend + UI).
- Apenas nesse modo é permitido incluir um `assistant` trailing intencional (prefill) para retomar geração.
- O fluxo explícito deve reutilizar o contrato existente de retry: **`RetryMessage`** (sem criar um fluxo alternativo de envio).

### 3) Auto-recuperação padrão

- Quando ocorrer interrupção de streaming e existir conteúdo parcial, o app tenta recuperar automaticamente até **3 tentativas** antes de mostrar a ação manual.
- O número de tentativas é **configurável por perfil**.

### 4) Cancelamento explícito de geração

> Atualizado pela Issue #202 (escopo do `Esc` restrito ao campo de edição).

- O app oferece “Cancelar geração” enquanto houver streaming em andamento.
- UX obrigatória:
  - Botão substitui o botão de enviar durante streaming.
  - Item no menu de contexto da mensagem em streaming.
  - Atalho `Esc`, **escopado ao campo de edição** (input de mensagem):
    - O `Esc` **só cancela a geração quando o foco está no campo de edição**. Esse cancelamento é tratado pelo próprio `ChatInput`.
    - Com o foco em **qualquer outro elemento** do painel (lista de mensagens, nós de mensagem, etc.), o `Esc` **não cancela** — ele apenas **devolve o foco ao campo de edição**, respeitando o foco local por painel (ver AEP-0058).
    - A prioridade de **fechar menus de contexto** abertos e a **guarda de modal aberto** (o `Esc` pertence ao modal) são preservadas: um listener global só atua quando o evento não foi tratado localmente.
- Semântica:
  - Cancelar **para apenas a geração atual**.
  - Cancelar **não deve limpar a fila inteira** da conversa.
  - Se houver resposta parcial ao cancelar, o usuário pode “Continuar resposta” (se suportado).
  - O cancelamento continua **sempre disponível via botão e item de menu**, independentemente de onde está o foco.

### 5) Configuração no perfil (termos amigáveis)

As opções ficam no perfil (guia “Modelos”), com rótulos amigáveis e i18n:

- “Tentar recuperar respostas interrompidas automaticamente” (default: ligado)
- “Máximo de tentativas de recuperação” (default: 3)
- “Mostrar ação ‘Continuar resposta’ quando falhar” (default: ligado)

### 6) Compatibilidade por provider/modelo (matriz explícita de capacidades)

> Atualizado pela Issue #124 (continuação com fallback por mensagem de usuário).

- A continuação via `assistant prefill` depende de compatibilidade do provider/modelo. Modelamos **três casos explícitos** de capacidade (`internal/llm.AssistantPrefillCapability`):
  - **`PrefillWithThinking`**: aceita `assistant` trailing como prefill mesmo com thinking/reasoning ativo. Hoje, apenas OpenAI real via Responses API.
  - **`PrefillWithoutThinking`**: aceita prefill apenas com thinking desativado. É o caso de servidores locais (Qwen via LocalAI/Ollama/llama.cpp) que rejeitam prefill com `enable_thinking`.
  - **`PrefillUnsupported`**: não aceita prefill (demais providers OpenAI-compatible).
- **Fallback por mensagem de usuário**: quando o provider/modelo **não suporta** prefill incondicional (`PrefillWithoutThinking` ou `PrefillUnsupported`), a continuação **não falha mais** — em vez de injetar um `assistant` trailing, o backend monta uma mensagem de `user` do tipo “Continue a resposta a partir deste texto: …\<texto parcial\>” e prossegue o streaming normalmente. Assim o prompt volta a terminar em `user`, compatível com qualquer provider (inclusive Qwen/LocalAI com thinking).
- **Gating pelo perfil continua valendo**: se o perfil desabilita “Continuar resposta” (`StreamingRecoveryShowContinue=false`), o backend falha fechado independentemente da capacidade — não há prefill nem fallback.
- A UI passa a oferecer “Continuar resposta” sempre que o perfil permitir, pois o backend sempre consegue continuar (prefill quando suportado, fallback por mensagem de usuário caso contrário). O flag `supports_assistant_prefill` permanece exposto apenas como informação.
- A detecção começa conservadora: `SupportsAssistantPrefill` (atalho booleano) só é verdadeiro para `PrefillWithThinking`. A matriz pode evoluir para granularidade por modelo no futuro.

### 7) Backend-driven: sem mensagens “fantasma” persistentes no frontend

- O frontend não deve criar mensagens locais persistentes para simular streaming.
- O frontend também não cria placeholder visual local antes de receber
  `messageId`. O placeholder pertence ao backend: é persistido no banco e
  emitido por evento com ID canônico antes de qualquer renderização.
- Para suportar recuperação/continuação, o backend é a única fonte da verdade:
  a mensagem do assistant existe no banco e é atualizada conforme o conteúdo
  parcial evolui. Isso segue AEP-0040 e o contrato backend-driven do projeto.

## Fases

- [x] **Docs**: escrever este AEP e aplicar adendos mínimos em AEPs antigas com exemplos/contratos desatualizados.
- [x] **Cancelamento**: expor `CancelStreamingForConversation` ao frontend (binding Wails) e implementar botão/menu/atalho `Esc` (este último escopado ao campo de edição — ver Decisão 4 / Issue #202).
- [x] **Profile settings**: persistir as opções de recuperação no perfil e aplicar defaults no envio.
- [x] **Persistência do assistant no início do turno**: criar/reusar placeholder do assistant no backend e garantir `messageId` consistente no `chat:stream`.
- [x] **Auto-recuperação**: implementar retry interno até N tentativas (default 3).
- [x] **Continuação explícita**: implementar “Continuar resposta” via `RetryMessage` em modo de continuação, atualizando a mesma mensagem do assistant. Quando o provider/modelo não suporta prefill, usar fallback por mensagem de usuário (Issue #124).
- [x] **Testes**: Go + Vitest cobrindo cancelamento, auto-recuperação e ausência de prefill acidental.

### Evidências

- Recuperação e continuação: `internal/agent/streaming_recovery_test.go` e
  `internal/agent/continuation_test.go`.
- Cancelamento e UX: `frontend/src/components/chat/ChatInput.test.tsx`,
  `ChatSessionView.test.tsx` e `frontend/src/lib/messageMenuItems.test.ts`.
- Configuração de perfil: `frontend/src/components/profiles/ProfileChatSection.test.tsx`.

## Riscos

- **Duplicação de conteúdo**: retomar pode duplicar trechos se o prefill não for tratado de forma idempotente.
- **Ordem e concorrência**: múltiplas superfícies por conversa (AEP-0057) exigem correlação correta por `conversationId`/`turnId`.
- **Providers divergentes**: “OpenAI-compatible” varia muito; algumas implementações não suportam prefill com thinking.
- **UX de `Esc`**: precisa respeitar prioridade e escopo. O `Esc` só cancela a geração com o foco no campo de edição; em outros focos, devolve o foco ao input sem cancelar, preservando o fechamento de menus de contexto e a guarda de modal aberto (Issue #202).

## Critérios de aceitação

- [x] Requests normais nunca enviam `assistant prefill` acidental.
- [x] Em interrupção com texto parcial, o app tenta recuperação automática até
  3 vezes.
- [x] Após falha/cancelamento, “Continuar resposta” usa prefill quando
  suportado ou fallback por mensagem de usuário (Issue #124).
- [x] “Cancelar geração” funciona por botão/menu e por `Esc` somente com foco
  no campo de edição, preservando menus e modais (Issue #202).
- [x] Cancelamento interrompe somente a geração atual, sem limpar a fila.
- [x] Perfis expõem opções com i18n em pt-BR, en e es.
- [x] O frontend renderiza somente mensagem/placeholder persistido e emitido
  pelo backend com `messageId` canônico.
- [x] A suíte cobre recuperação, continuação, cancelamento e configuração.
