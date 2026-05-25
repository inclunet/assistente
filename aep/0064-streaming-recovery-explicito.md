# AEP-0064 — Recuperação explícita de resposta interrompida (continuação) e cancelamento de geração

Status: Proposto
Data: 2026-05-21
Autor: Leonardo Gleison (Inclunet) + GitHub Copilot

## Resumo

Este AEP define um fluxo **explícito** (não acidental) para recuperar uma resposta do assistente quando o streaming é interrompido por erro ou por cancelamento do usuário.

A proposta combina três pilares:

1. **Requests normais** de chat continuam terminando em `user` (nunca em `assistant` por acidente).
2. **Continuação de resposta** é uma ação explícita (UI + backend) que pode usar `assistant prefill` somente nesse modo.
3. O usuário ganha controle direto com **Cancelar geração** (botão durante streaming, item no menu e atalho `Esc`), além de **auto-recuperação padrão de 3 tentativas** antes de “falhar de vez”.

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

- O app oferece “Cancelar geração” enquanto houver streaming em andamento.
- UX obrigatória:
  - Botão substitui o botão de enviar durante streaming.
  - Item no menu de contexto da mensagem em streaming.
  - Atalho `Esc` (alinhado ao comportamento do Copilot Chat no VS Code).
- Semântica:
  - Cancelar **para apenas a geração atual**.
  - Cancelar **não deve limpar a fila inteira** da conversa.
  - Se houver resposta parcial ao cancelar, o usuário pode “Continuar resposta” (se suportado).

### 5) Configuração no perfil (termos amigáveis)

As opções ficam no perfil (guia “Modelos”), com rótulos amigáveis e i18n:

- “Tentar recuperar respostas interrompidas automaticamente” (default: ligado)
- “Máximo de tentativas de recuperação” (default: 3)
- “Mostrar ação ‘Continuar resposta’ quando falhar” (default: ligado)

### 6) Compatibilidade por provider/modelo

- A continuação via `assistant prefill` depende de compatibilidade do provider/modelo.
- Em providers/modelos onde o prefill não for seguro (ex.: LocalAI/Qwen com thinking ativo e rejeição de prefill), o app **não deve oferecer** “Continuar resposta” (fica apenas “Regenerar”).
- A detecção começa conservadora e pode evoluir para uma matriz explícita de capacidades por provider/modelo.

### 7) Backend-driven: sem mensagens “fantasma” persistentes no frontend

- O frontend não deve criar mensagens locais persistentes para simular streaming.
- Um placeholder visual temporário é aceitável enquanto o primeiro evento ainda não trouxe o `messageId` persistido, desde que seja migrado para esse ID assim que disponível.
- Para suportar recuperação/continuação, o backend deve ser a fonte da verdade: mensagem do assistant deve existir no banco (placeholder) e ser atualizada conforme conteúdo parcial evolui.

## Fases

1. **Docs**: escrever este AEP e aplicar adendos mínimos em AEPs antigas com exemplos/contratos desatualizados.
2. **Cancelamento**: expor `CancelStreamingForConversation` ao frontend (binding Wails) e implementar botão/menu/atalho `Esc`.
3. **Profile settings**: persistir as opções de recuperação no perfil e aplicar defaults no envio.
4. **Persistência do assistant no início do turno**: criar/reusar placeholder do assistant no backend e garantir `messageId` consistente no `chat:stream`.
5. **Auto-recuperação**: implementar retry interno até N tentativas (default 3).
6. **Continuação explícita**: implementar “Continuar resposta” via `RetryMessage` em modo de continuação, atualizando a mesma mensagem do assistant.
7. **Testes**: Go + Vitest cobrindo cancelamento, auto-recuperação e ausência de prefill acidental.

## Riscos

- **Duplicação de conteúdo**: retomar pode duplicar trechos se o prefill não for tratado de forma idempotente.
- **Ordem e concorrência**: múltiplas superfícies por conversa (AEP-0057) exigem correlação correta por `conversationId`/`turnId`.
- **Providers divergentes**: “OpenAI-compatible” varia muito; algumas implementações não suportam prefill com thinking.
- **UX de `Esc`**: precisa respeitar prioridade (fechar menus do input antes de cancelar geração).

## Critérios de aceitação

- Requests normais nunca enviam `assistant prefill` acidental.
- Em interrupção de streaming com texto parcial, o app tenta recuperar automaticamente até 3 vezes.
- Após falhar (ou após cancelamento), a UI mostra “Continuar resposta” no menu da mensagem quando suportado.
- “Cancelar geração” funciona via botão, menu e `Esc`.
- Cancelamento não limpa fila inteira; apenas interrompe a geração atual.
- Perfis expõem as opções com rótulos amigáveis e i18n (pt-BR, en, es).
- Suite de testes cobre os comportamentos críticos.
