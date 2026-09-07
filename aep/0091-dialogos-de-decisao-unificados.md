# AEP-0091 — Diálogos de decisão unificados (estilo Windows + NVDA)

**Status:** ✅ Done

## Resumo

Toda confirmação bloqueante do app — shell, rede, ACP, edição de arquivo,
HTTP mutável, updater, exclusões da UI, AgentInstall e o `ConfirmDialog` —
deve seguir **um único design de diálogo de decisão**, no espírito das
janelas de confirmação do Windows: a pessoa entende **o que** está sendo
perguntado, escolhe com **um clique ou uma tecla**, e consegue **ouvir de
novo** a pergunta se perdeu o anúncio.

O modelo atual (rádios + Confirmar/Negar, ConfirmDialog mudo, `window.confirm`)
é inconsistente e hostil ao NVDA. Este AEP define o contrato e as fases de
migração. Complementa o AEP-0090 (ordem primária→cancelar no DOM); não o
substitui.

## Motivação

- **Permissão ACP / rede / shell** hoje misturam `single_choice` (rádios) com
  botões Confirmar/Negar. No NVDA isso é semântica dupla: “Negar” existe no
  rádio e no botão; a ação afirmativa exige Tab até Confirmar depois de
  escolher.
- **ConfirmDialog** não anuncia título+mensagem na abertura, não tem atalho
  de ação, não tem som de alerta e não oferece repetir a pergunta.
- **Voltar à janela** (Alt+Tab) ou perder o anúncio deixa a pessoa sem forma
  padrão de saber o que o diálogo pergunta.
- Ainda há **`window.confirm`** em alguns fluxos, fora do design system.
- O investimento a11y está concentrado no `QuestionnaireDialog` genérico;
  confirmações simples e decisões de permissão merecem um contrato próprio,
  compartilhado por **todos** os produtores (backend e UI).

## Escopo (obrigatório alinhar)

| Origem | Hoje | Destino |
|--------|------|---------|
| Shell (`shellConfirmationPayload`) | Questionnaire boolean | DecisionDialog |
| Rede (`app_nettrust`) | Rádios de escopo + Autorizar/Negar | DecisionDialog (botão por escopo + negar) |
| ACP (`app_acp_permissions`) | Rádios allow/deny + Confirmar/Negar | DecisionDialog (botão por opção) |
| Edição de arquivo / HTTP / updater | Questionnaire | DecisionDialog quando for decisão; form só se houver campos extras |
| UI `useConfirm` / ConfirmDialog | Modal mudo | DecisionDialog (binário) |
| AgentInstall | Modal ad-hoc | DecisionDialog |
| TaskLists / Jobs / `useEditableList` | `window.confirm` | DecisionDialog |
| Tool `collect_responses` / formulários multi-campo | QuestionnaireDialog | **Permanece** QuestionnaireDialog (não é decisão de um clique) |

**Regra:** se a interação é “escolha uma resposta e siga”, é **DecisionDialog**.
Se precisa preencher vários campos (nome, opções, texto longo), continua
**QuestionnaireDialog**.

## Decisões

### D1. Um componente / contrato de UI: `DecisionDialog`

API conceitual (frontend):

- `title`, `description` (texto falado e `aria-describedby`)
- `body` opcional (conteúdo só leitura: comando, URL, diff — `readonly_code` /
  `readingMode` quando for leitura pesada)
- `actions[]`: lista ordenada de ações `{ id, label, variant, shortcut?, primary? }`
- `onAction(id)` / cancel via ESC ou ação explícita de cancelar
- Sem rádios para escolher entre as ações. **Cada opção é um botão.**

Ordem DOM das ações segue AEP-0090: ações afirmativas/primárias antes das
destrutivas/cancelar quando forem pares; em listas multi-opção, ordem estável
definida pelo backend (allow-once → allow-always → reject…), com a ação de
recusa/cancelamento por último.

### D2. Sem híbrido rádio + Confirmar

Proibido para decisões de permissão/escopo:

- `single_choice` + Submit “Confirmar” + Cancel “Negar”

Substituir por botões de ação direta. O valor enviado ao backend é o `id` da
ação (ex.: `allow-once`, `session`, `reject-once`), não uma resposta de rádio
seguida de submit.

### D3. `role="alertdialog"` + anúncio na abertura

Todo DecisionDialog:

1. Usa `role="alertdialog"` (ou Modal com variante alertdialog).
2. Na abertura: anuncia título + descrição [+ resumo do body] de forma
   **assertive**. Bodies longos (diff/comando) usam o caminho do broker com
   `protectsReading` (`announceWithOrigin` / `announceRequest`), não só
   `useAnnouncer().announce(priority)`, cuja API atual não carrega essa flag.
3. `aria-describedby` aponta para a região da pergunta (e body quando houver).

### D4. Som de alerta na abertura

Reproduzir tom dedicado de alerta via `audioFeedback` quando o
DecisionDialog do topo abre. **Na Fase 1:** adicionar `SOUND_TYPES.ALERT`
(hoje o enum não tem essa constante; não reutilizar `ERROR` sem documentar).

- Preferência do usuário: configurável em Aparência/Acessibilidade
  (default **ligado** no perfil de uso com leitor; default do app a decidir na
  implementação — documentar no PR da Fase 1).
- Não substitui o anúncio; complementa.

### D5. Atalho global: repetir a pergunta — `Ctrl+Shift+R`

Enquanto um DecisionDialog (ou Modal de decisão) for o topo do stack:

- `Ctrl+Shift+R` re-dispara o **mesmo** texto assertivo da abertura
  (título + descrição + resumo do body).
- Implementação: registrar o último payload de anúncio do diálogo ativo no
  announcer/broker (ou store do diálogo); o atalho só age no modal topmost.
- Documentar em `KeyboardShortcutsHelp` e na página de ajuda.
- Não conflitar com atalhos de edição; só com modal de decisão aberto.

### D6. Atalhos de ação (estilo Windows)

- Decisão **binária** (Sim/Não, Permitir/Negar, OK/Cancelar):
  - Cada ação declara um **mnemônico** (letra do rótulo localizado, ex. a
    letra sublinhada). O **atalho de teclado** é `Alt+<mnemônico>` no idioma
    corrente (ex.: pt-BR “Sim” → mnemônico `S`, atalho `Alt+S`; “Não” → `N` /
    `Alt+N`; en “Yes” → `Y` / `Alt+Y`; “No” → `N` / `Alt+N`). Não há atalho
    fixo multi-idioma: só vale o do locale ativo.
  - ESC cancela/fecha sem autorizar (equivalente à ação negativa quando
    existir).
- Decisão **multi-opção**: atalho opcional por ação (`shortcut` no payload,
  também localizado); no mínimo Enter ativa a ação focada; ESC cancela.
- Atalhos de letra **não** disparam com foco em campo editável
  (textarea de motivo, se existir).

### D7. Foco inicial

| Severidade | Foco inicial |
|------------|--------------|
| Destrutivo (apagar, remover) | Ação de cancelar / segura |
| Permissão / shell / rede | Body readonly (comando/URL) se houver; senão primeira ação **segura** (ex. “Permitir uma vez”, não “sempre”) |
| Artefato não verificado (AgentInstall) | Cancelar (já documentado em AEP-0086) |

### D8. Backend: kind `decision` no questionnaire (ou payload equivalente)

Em vez de montar `single_choice` + submit/cancel ambíguos, o backend envia
um payload de decisão:

```text
kind: decision
title, description, body?
actions: [{ id, label(QuestionnaireText), variant, shortcut?(QuestionnaireText) }]
```

- `label` e `shortcut` (quando enviado) são `QuestionnaireText` — localizados
  no idioma ativo, alinhados a D6.
- Se `shortcut` for omitido, o frontend deriva o mnemônico da letra marcada
  no `label` localizado (ex. `&Sim` → `S` / `Alt+S`). Não há atalho fixo
  multi-idioma no schema.

O frontend renderiza `DecisionDialog`. Resposta: `{ actionId }` ou
`cancelled: true`.

Compatibilidade com o padrão antigo (rádio único + submit mapeado para
DecisionDialog) **não foi implementada**. Os produtores de permissão emitem
`kind: decision` diretamente; o frontend só bifurca por `kind === 'decision'`.

Questionários multi-campo (`collect_responses`, formulários) **não** usam
`kind: decision`.

### D9. Uma fila de diálogos bloqueantes (meta) — fechamento

Filas lógicas **permanecem separadas**:

| Fila | Store / origem |
|------|----------------|
| Confirmação binária da UI | `confirmStore` → `ConfirmHost` / `DecisionDialog` |
| Questionário UI-local | `questionnaireUIStore` → `QuestionnaireDialog` |
| Questionário / decisão do backend | `questionnaire.Manager` → `DecisionQuestionnaireHost` ou formulário |

**Mitigação atual:** todos usam `Modal` → `modalRegistry` (`OPEN_MODAL_STACK`).
ESC, focus trap e `Ctrl+Shift+R` respeitam só o modal **topmost**.
`App.tsx` também impede abrir questionário UI-local enquanto há questionário
do backend ativo.

**Gap residual (aceitável):** `requestConfirm` não consulta se já há decisão
backend aberta; empilhamento teórico é raro. Unificar numa fila bloqueante
única fica como evolução futura, fora deste AEP.

## Fases

### Fase 0 — Contrato (este PR de docs)

- [x] AEP-0091 + índice
- [x] Regra curta em `AGENTS.md`
- [x] Matriz de migração acima concordada

### Fase 1 — Fundação no frontend

- [x] `DecisionDialog` (ou evolução do `ConfirmDialog`) com D3–D7
- [x] Adicionar `SOUND_TYPES.ALERT` em `audioFeedback` + preferência do usuário
- [x] `Ctrl+Shift+R` repeat
- [x] Migrar `ConfirmDialog` / `useConfirm`
- [x] Testes: anúncio, som (mock), atalhos, axe, ordem AEP-0090

### Fase 2 — Shell, rede, ACP

- [x] Payload `kind: decision` (ou adapter) em
      `app_tool_confirmations.go`, `app_nettrust.go`, `app_acp_permissions.go`
- [x] UI sem rádio+Confirmar/Negar
- [x] Atualizar testes Go de i18n/permissão/rede/shell
      (+ canal numera ações; frontend `DecisionQuestionnaireHost`)

### Fase 3 — Demais produtores

- [x] Edição de arquivo, HTTP, updater, exclusão de mensagem
- [x] AgentInstall → DecisionDialog
- [x] Eliminar todo `window.confirm` do frontend

### Fase 4 — Fechamento

- [x] Remover caminhos legados de rádio+submit para permissão
      (`scopeOptions` / parsing `session — …`; UI morta
      `QuestionnaireDialog.rejectReason`; contrato testado em
      `decision_permission_contract_test.go`)
- [x] Checklist NVDA documentado abaixo (cobertura automatizada + passo
      interativo do mantenedor)
- [x] AEP → ✅ Done

### Checklist NVDA (validação interativa)

Itens com cobertura de teste automatizada (unitário / e2e parcial):

| Item | Cobertura |
|------|-----------|
| Abertura anuncia título+descrição (assertive) | `DecisionDialog.test.tsx` |
| Som de alerta (preferência) | `DecisionDialog.test.tsx` + toggle em Aparência |
| `Ctrl+Shift+R` repete; não intercepta em textarea | `DecisionDialog.test.tsx` |
| Mnemônicos `Alt+<letra>` | `DecisionDialog.test.tsx` + `decisionMnemonic` |
| Multi-opção rede / ACP | testes i18n Go + contrato `kind=decision` |
| Ordem DOM AEP-0090 | `DecisionDialog.test.tsx` / `DialogActions` |
| Sem `window.confirm` | grep + e2e profiles |

Passo do mantenedor (NVDA no Windows), uma vez após o merge:

- [ ] Shell: abertura fala a pergunta; `Alt+Tab` + `Ctrl+Shift+R` repete
- [ ] Rede: cada escopo é botão (sem rádio); Tab chega em Negar por último
- [ ] ACP: allow-once / always / deny como botões; foco inicial seguro
- [ ] Exclusão via `useConfirm`: anúncio + alerta + mnemônicos

## Riscos

- Mudança de UX em permissões ACP/rede é sensível; testes de contrato de
  `OptionID` / escopo devem falhar alto se o id da ação mudar.
- Atalhos `Alt+<mnemônico>` em layouts não-QWERTY / outros idiomas: o
  mnemônico vem do rótulo localizado (não há par Y/N fixo global).
- Som de alerta pode irritar; precisa toggle.
- Diffs longos: anúncio completo pode ser verboso — anunciar título +
  descrição e indicar que o corpo está no diálogo; Ctrl+Shift+R repete o
  mesmo pacote; readingMode cobre a leitura do body.

## Critérios de aceitação

- [x] Nenhuma confirmação bloqueante do app usa `window.confirm`.
- [x] Shell, rede e ACP usam DecisionDialog (botão por opção; sem híbrido
      rádio+Confirmar/Negar).
- [x] Toda abertura anuncia a pergunta e toca alerta (se preferência ligada).
- [x] `Ctrl+Shift+R` com diálogo de decisão no topo re-anuncia a pergunta.
- [x] ConfirmDialog UI alinhado ao mesmo componente/contrato.
- [x] QuestionnaireDialog multi-campo preservado onde faz sentido.
- [x] AEP-0090 respeitado na ordem das ações.
