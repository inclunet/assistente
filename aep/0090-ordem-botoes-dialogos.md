# AEP-0090 — Ordem de botões em diálogos (confirmação antes de cancelar)

**Status:** ✅ Done

## Resumo

Em modais, diálogos e rodapés de formulário do tipo
“cancelar / confirmar”, a ordem no DOM (e portanto a ordem de Tab e a
leitura no NVDA) hoje coloca **Cancelar antes** da ação primária
(Confirmar, OK, Salvar, Ir). Para quem usa leitor de telas, isso atrasa
a ação afirmativa e empurra o fluxo para a opção de desistir primeiro.

Este AEP padroniza: **ação primária/confirmação vem antes de cancelar**
no DOM. O botão **Fechar (X)** do `Modal` permanece no header. A ordem
visual acompanha o DOM (estilo Windows: Confirmar | Cancelar), sem
`flex-direction: row-reverse` que mascare ordem errada.

## Motivação

- Tab e NVDA seguem a **ordem no DOM**, não o alinhamento CSS.
- O `ConfirmDialog` (e o `ConfirmHost` global) hoje renderiza Cancelar →
  Confirmar; a maior parte dos footers de formulário repete o padrão.
- O `QuestionnaireDialog` já documentou, numa variante, ordem
  **Aplicar → motivo → Rejeitar** de propósito — prova de que o princípio
  já foi reconhecido, mas não padronizado.
- O mantenedor usa NVDA; diálogos de confirmação e salvamento são
  caminhos diários.

## Decisões

### D1. Ordem canônica no DOM: primária → secundária (cancelar)

Em qualquer rodapé de diálogo/modal/formulário com par
confirmação/cancelamento:

1. Botão de **confirmação / salvar / OK / aplicar / ir** (ação primária)
2. Botão de **cancelar** (e equivalentes: descartar, rejeitar quando for a
   saída sem aplicar)

Ações auxiliares (ex.: “Testar conexão”) ficam **antes** do par
primária/secundária, ou em outra região — não entre eles de forma a
empurrar a primária para depois do cancelar.

### D2. Fechar (X) do `Modal` não muda de lugar

O botão de fechar do header do `Modal` permanece onde está. Este AEP
trata só do rodapé de ações. O foco inicial do `Modal` (primeiro campo
editável, etc.) continua valendo; a ordem dos botões importa sobretudo
ao tabular o rodapé e ao percorrer o diálogo com o leitor.

### D3. Sem mascarar com CSS

É **proibido** usar `row-reverse` / `order` só para “parecer” Cancelar à
esquerda e Confirmar à direita enquanto o DOM fica invertido. Ordem
visual = ordem DOM = ordem de Tab.

### D4. Componente `DialogActions`

Novo componente em `frontend/src/components/ui/DialogActions` que
recebe slots `primary` e `secondary` e renderiza nessa ordem, com layout
à direita (`justify-content: flex-end`) e gap padronizado. Novos
diálogos devem usá-lo; migração dos existentes é faseada.

### D5. `ConfirmDialog` é o ponto de entrada da Fase 1

Todas as confirmações via `ConfirmHost` passam a obedecer D1 assim que
o `ConfirmDialog` usar `DialogActions`.

## Fases

### Fase 1 (este PR) — contrato + ConfirmDialog

- [x] AEP-0090 e entrada no índice
- [x] Regra curta em `AGENTS.md` (seção Acessibilidade)
- [x] `DialogActions` + testes de ordem
- [x] `ConfirmDialog` passa a usar `DialogActions` (Confirmar → Cancelar)
- [x] Teste do `ConfirmDialog` asserta ordem dos botões

### Fase 2 — rodapés de formulário/modal

Migrar footers que ainda fazem Cancelar → Salvar/Confirmar, preferindo
`DialogActions` (Memories, ProviderForm, Credentials, TaskForm,
JobBuilder, EditorPanel footers, Signal flows, etc.). Um PR por lote
quando fizer sentido.

- [x] Memories, History (export), ProviderForm, JobBuilder, WorkflowEditor
- [x] TaskDetailModal (nota), CustomActionsEditor, MermaidEditorModal
- [x] Credentials, Channels, Allowlist, Skills, Mcp (CRUD com exclusão auxiliar)
- [x] AgentWorkDirControl; AgentInstall (confirm install/update + remove),
      mantendo `initialFocusSelector` no cancelar quando artefato não verificado
- [x] QuestionnaireDialog — Fase 3


### Fase 3 — QuestionnaireDialog (footer padrão)

- [x] Footer padrão via `DialogActions` (Submit → Cancelar)
- [x] Variante com motivo de rejeição preservada (Aplicar → motivo → Rejeitar)
- [x] Removido `column-reverse` no mobile que mascarava a ordem DOM

## Riscos

- Mudança visual (Confirmar à esquerda de Cancelar no grupo alinhado à
  direita) pode surpreender quem esperava o padrão “web/Material”.
  Aceito: prioridade é NVDA/Tab no desktop Windows.
- Testes que localizam botões por posição no array quebram — corrigir
  asserts, não afrouxar cobertura.

## Critérios de aceitação

- [x] `ConfirmDialog` / `ConfirmHost`: no DOM, Confirmar aparece antes de
      Cancelar; teste cobre a ordem.
- [x] `DialogActions` exportado e documentado como caminho preferido.
- [x] `AGENTS.md` e AEP descrevem a regra; Fechar (X) explicitamente fora
      do escopo de reordenação.
- [x] Nenhum uso novo de `row-reverse`/`order` para inverter
      primária/secundária.
- [x] Fases 2–3 concluídas; Fase 1 entregou o contrato e o ponto global de
      confirmação.