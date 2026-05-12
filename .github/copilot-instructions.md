# Instruções para GitHub Copilot (assistente)

## Sistema de Temas e Cores (OBRIGATÓRIO)
- NUNCA use cores hardcoded (`#hex`, `rgb()`, `rgba()`, nomes de cor) em arquivos CSS.
- Todas as cores DEVEM vir das variáveis CSS definidas em `frontend/src/theme.css`.
- Variáveis principais: `--bg-base`, `--bg-surface`, `--bg-elevated`, `--bg-hover`, `--text-primary`, `--text-secondary`, `--text-muted`, `--border-subtle`, `--border-default`, `--accent`, `--color-success`, `--color-warning`, `--color-danger`, `--color-info`, `--focus-ring`.
- NUNCA use `@media (prefers-color-scheme: dark/light)` — o app usa `data-theme` no `<html>`.
- Use `var(--radius-sm)` / `var(--radius-md)` / `var(--radius-lg)` para border-radius.

## Acessibilidade (NUNCA negligenciar)
- Contraste mínimo: texto sobre fundo ≥ 4.5:1 (AA WCAG 2.1); preferir ≥ 7:1 (AAA).
- Todo elemento interativo deve ser acessível por teclado (Tab, Enter, ESC, setas).
- Usar `announce()` via `useAnnouncer` para feedback de ações para leitores de tela.
- Nunca usar apenas cor para transmitir informação — combinar com ícones ou texto.
- Usar componentes existentes (`DataGrid`, `Modal`, `Button`, `Toolbar`) que já têm ARIA correto.
- Labels obrigatórios: `htmlFor` em `<label>`, `aria-label` em botões sem texto visível.

## Logs (performance + ruído)
- Evite adicionar `console.log` em código de produção.
- Ao mexer em um recurso/arquivo, remova `console.log` existentes no trecho tocado.
- `console.warn` e `console.error` são permitidos quando representam condição anômala real.
- Se for indispensável logar para diagnóstico, prefira:
  - log temporário removido antes do merge; ou
  - log protegido por flag (ex.: `import.meta.env.DEV` + toggle em `localStorage`).

## Escopo de mudanças
- Não faça “varreduras globais” para remover logs em arquivos grandes sem necessidade do PR; aplique a limpeza incrementalmente quando o recurso for alterado.

## Execução (preferir VS Code Tasks)
- Para lint/build/test, prefira rodar **VS Code Tasks** ao invés de comandos via shell/terminal.
- Use `run_task`/`create_and_run_task` quando precisar executar algo repetível e seguro.
- Tasks já disponíveis em `.vscode/tasks.json` (use estas primeiro):
  - `Frontend: lint`
  - `Frontend: test`
  - `Frontend: build`
  - `Go: test ./...`
  - `Check: all (go test + frontend lint+test+build)`
- Só use shell/terminal quando não existir Task equivalente ou quando for necessário diagnosticar algo pontual.

## Testes (não deixar feature descoberta)
- Sempre verificar se a mudança/feature está coberta por testes.
  - Frontend: preferir **Vitest** para utilitários, hooks e componentes.
  - Backend: preferir `go test ./...` com testes de unidade/integração onde fizer sentido.
- Se não houver teste cobrindo o comportamento alterado (ou se a cobertura estiver fraca), criar/atualizar testes junto com a mudança.
- Ao adicionar testes, manter o escopo focado na feature tocada (evitar “mass refactor” de testes).
- Antes de concluir, rodar as tasks de teste relevantes (preferência por `Frontend: test` e/ou `Go: test ./...`).

## Testes (regras rígidas)
- Nunca deletar testes para “resolver” falhas.
- Nunca simplificar testes para facilitar aprovação; corrigir o código é sempre o caminho.

## Messaging — Arquitetura backend-driven (PROIBIÇÕES)

O sistema de envio/recebimento de mensagens segue uma arquitetura backend-driven definida na AEP-0040.

### Regras absolutas
- **NUNCA crie fluxo alternativo de envio de mensagens.** Existe UMA única `SendMessage` no backend (`app_chat.go`) para mensagens novas e `RetryMessage` para retry explícito. No frontend, componentes e controllers por aba/conversa devem reutilizar o contrato/pipeline compartilhado de envio; não duplique validação, serialização, parâmetros ou chamada ao backend em wrappers divergentes.
- **NUNCA crie mensagens locais no frontend.** O frontend não gera IDs, não insere mensagens otimistas, não cria placeholders. Só renderiza o que o backend emite via eventos.
- **Mensagens só podem ser enviadas para conversas que já existem.** `SendMessage` com `conversationID=0` ou inexistente é erro. Nunca crie conversa implicitamente dentro do fluxo de envio.
- **Todo evento de chat DEVE carregar `conversationId`.** Sem exceções.
- **Conversas são independentes de abas.** Conversas existem no banco sem vínculo com UI. Canais (Telegram, Signal) criam conversas sem abas.
- **Controllers por aba/conversa são permitidos.** Cada controller deve filtrar eventos por `conversationId`, manter apenas seu estado visual próprio e delegar envio/retry ao contrato compartilhado.
- **Announcer, TTS e STT são serviços globais arbitrados.** Não crie múltiplas live regions por aba; TTS não pode falar em paralelo; STT local só pode escutar a aba ativa.

### Referência
- Detalhes completos no AEP de backend-driven-messaging em `aep/`.

## AEPs — Architecture Evolution Proposals (OBRIGATÓRIO)

O diretório `aep/` é o repositório único de decisões arquiteturais do projeto. Contém 45+ documentos numerados que definem contratos, protocolos, decisões de design e planos de evolução.

### Regras absolutas
- **NUNCA crie outro diretório** para AEPs (ex.: `aeps/`, `docs/aep/`, `proposals/`). Tudo fica em `aep/`.
- **Antes de implementar qualquer feature significativa**, consulte os AEPs relevantes em `aep/` para verificar se já existe decisão arquitetural sobre o tema.
- **O código DEVE estar alinhado com os AEPs.** Se encontrar divergência entre um AEP e o código:
  1. NÃO assuma que o código está certo e o AEP desatualizado.
  2. Pergunte ao usuário: "O AEP `aep/XXXX` diz X, mas o código faz Y. O AEP precisa ser atualizado ou o código precisa ser corrigido?"
  3. Só prossiga após confirmação.
- **Ao criar novo AEP**, use numeração sequencial a partir do último existente (consulte `aep/` para o maior número).
- **Formato**: Markdown, em português, com seções: Resumo, Motivação, Decisões, Fases, Riscos, Critérios de aceitação.
- **Para descobrir AEPs relevantes**, liste `aep/` e leia os títulos — os nomes dos arquivos descrevem o tema.

## i18n (Internacionalização — OBRIGATÓRIO)
- TODAS as strings visíveis ao usuário DEVEM usar `t('namespace.key')` via `react-i18next`
- NUNCA hardcode strings em qualquer idioma diretamente no JSX
- Idiomas: pt-BR, en, es — locales em `frontend/src/locales/`
- Config: `frontend/src/lib/i18n.ts`
- Ao criar nova string, adicionar chave nos 3 locales (pt-BR.ts, en.ts, es.ts)
- Detecção automática de idioma com fallback para inglês
