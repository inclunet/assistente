# Plano — Editor Rico + Edição Inline com Chat

## Objetivo
Adicionar uma nova área no app (uma **Editor Page** com **abas**, como Chat e Terminal) que permita:

1. Editar **texto rico** (conteúdo formatável) e também conteúdo de **programação** (blocos de código, snippets, trechos técnicos).
2. Selecionar um trecho no editor e pedir ao LLM:
   - **formatar** o trecho (ex: transformar em lista, padronizar estilo, melhorar legibilidade)
   - **alterar** o trecho (reescrever, corrigir, refatorar texto/código, etc.)
3. Chamar o **chat atual** de forma inline (atalho) para pedir alterações; a alteração aparece em um **modal de preview** para **Confirmar** ou **Rejeitar**.

Este documento é um plano “maduro para implementação”: decisões técnicas propostas, fluxos de UX, contrato de patch e backlog inicial.

---

## Escopo (MVP)

### Inclui
- Nova página: **EditorPage** com abas (criar, fechar, renomear, navegar por teclado).
- Editor rico com:
  - negrito/itálico/sublinhado, títulos, listas, links
  - inline code + **code blocks** (com highlight)
  - colar/colar como texto, desfazer/refazer
- Ação **“Pedir alteração ao chat”** com:
  - captura de **seleção atual** (ou bloco atual quando não há seleção)
  - input curto do pedido (prompt)
  - envio para o **chat ativo**
  - parsing de uma **proposta de patch** na resposta do LLM
  - modal de **preview/diff** e botões **Aplicar** / **Rejeitar**

### Não inclui (por enquanto)
- Colaboração em tempo real.
- Merge de múltiplos patches.
- Edição “multi-arquivo” tipo IDE.
- Aplicar patch sem confirmação.

---

## Decisões técnicas (fechadas)

### Experiência alvo (decisão)
**Modo duplo na mesma aba (Rico ⇄ Markdown)** com **Markdown como fonte de verdade**.

Implicação: só vamos oferecer no modo rico um **subconjunto de formatações** que tenha representação estável em Markdown.

### Biblioteca do editor rico
Recomendação: **TipTap (ProseMirror)** no frontend para o modo rico.

Motivos:
- Integração React madura.
- API de seleção bem definida (troca de conteúdo no range selecionado).
- Extensível para tables, code blocks, links, etc.

Sugestão de stack:
- `@tiptap/react` + `@tiptap/starter-kit`
- `@tiptap/extension-link`, `@tiptap/extension-placeholder`
- `@tiptap/extension-code-block-lowlight` + `lowlight` (highlight)

### Editor do modo “código/Markdown”
Recomendação: **CodeMirror 6** para editar o Markdown cru (mais leve que Monaco e excelente para Markdown).

### Modelo de documento (fonte de verdade)
- Persistir/armazenar como **Markdown** (string).
- O modo rico é uma *view* derivada: Markdown → parse → TipTap.
- Ao editar no modo rico: TipTap → serialize → Markdown (com debounce + flush ao alternar/salvar).

### Integração com o “chat atual”
- O Editor usa o **chatStore** existente (tab ativa atual) para enviar o pedido.
- A resposta do LLM continua sendo uma mensagem normal do chat, mas inclui um bloco estruturado para o Editor.

---

## UX — Fluxo principal (selecionar → pedir alteração → aplicar)

1. Usuário seleciona um trecho no Editor.
2. Usuário pressiona **Ctrl+Shift+I** (sugestão default) ou clica em botão na toolbar “Perguntar ao chat”.
3. Abre um modal leve (ou popover) “O que você quer fazer com o trecho selecionado?”
4. Ao enviar:
   - Editor entra em estado **busy** (bloqueia edição ou mostra overlay), para evitar seleção mudar.
   - Envia mensagem no chat ativo com:
     - instrução do usuário
     - trecho selecionado (e, opcionalmente, contexto antes/depois)
     - regras de saída (patch)
5. Quando o chat finalizar (evento `chat:done` da conversa), o Editor procura um bloco de código ```editor_patch``` na última resposta do assistente.
6. Se existir:
   - abre modal de preview mostrando **Antes** (seleção original) e **Depois** (proposta)
   - botões: **Aplicar** / **Rejeitar**
7. Aplicar:
  - **Modo rico:** substitui a seleção diretamente no TipTap (convertendo `replacement` em nodes), depois serializa o documento inteiro para Markdown (flush).
  - **Modo Markdown:** substitui a seleção diretamente no CodeMirror (string range).
  - adiciona toast “Alteração aplicada”.
8. Rejeitar:
   - fecha modal; nada muda.

---

## Atalhos de teclado (proposta)

- **Ctrl+Shift+I**: abrir prompt inline do Editor (não conflita com os atalhos documentados do Chat).
- **Ctrl+T / Ctrl+W / Ctrl+Tab / Ctrl+Shift+Tab / Ctrl+1-9**: reaproveitar padrão de abas (como Chat/Terminal), mas **escopado** quando EditorPage está ativa.
- (Opcional) **Ctrl+B / Ctrl+I / Ctrl+K** para formatação clássica.

Observação: `Ctrl+I` já aparece na ajuda como “Perfis de interação”. Mesmo que hoje não esteja 100% implementado, é melhor **não** usar `Ctrl+I` no Editor por padrão.

---

## Alternância de modo (Rico ⇄ Markdown)

### Comportamento
- Um toggle na Toolbar: **Rico** / **Markdown**.
- A aba mantém um estado `mode` e um campo `markdown` (fonte de verdade).

### Regras de sincronização
- Ao entrar no modo rico: `markdown` → parse → setContent no TipTap.
- Ao editar no modo rico: transações do TipTap atualizam `markdown` via serializer **debounced** (ex: 250–500ms).
- Ao sair do modo rico (toggle para Markdown) ou ao salvar: **flush** (serializa imediatamente e atualiza `markdown`).

### Subset de Markdown suportado (MVP)
- Títulos (#, ##, ###)
- Negrito/itálico
- Listas ordenadas/não ordenadas
- Links
- Inline code
- Code blocks (```)

Fora do MVP (evitar no modo rico enquanto Markdown for a fonte de verdade):
- Estilos “livres” sem equivalente em Markdown
- Layouts avançados (colunas), fontes, cores, etc.

---

## Formatação manual (sem escrever Markdown)

Sim — no modo **Rico**, o usuário formata com **botões** e **atalhos**, sem precisar digitar Markdown.

Como funciona mantendo Markdown como fonte de verdade:
- O usuário clica em **Negrito/Itálico/Título/Lista/Link/Code** na toolbar (ou usa atalhos).
- O TipTap aplica marks/nodes internamente.
- Em seguida, o editor **serializa** isso para Markdown e atualiza o campo `markdown` (debounced).

### Toolbar recomendada (MVP)
- Bold, Italic
- H1/H2/H3
- Lista (bullet/ordered)
- Link (Ctrl+K)
- Inline code e code block
- Undo/Redo

### Regras de UX
- Se uma ação não tiver representação estável em Markdown (ex: cor de texto), o botão deve:
  - estar desabilitado **ou**
  - mostrar aviso “não suportado no modo Markdown”.

---

## Extensões desejadas: Tabelas (GFM) e Mermaid

Você mencionou **tabelas**, **listas**, **links** e **Mermaid** — isso é totalmente compatível com “Markdown como fonte de verdade”, mas vale separar **renderização** vs **edição**.

### Tabelas (GitHub Flavored Markdown)

**Representação Markdown:** pipe tables (GFM).

Estratégia recomendada (incremental):

1) **MVP (suporte prático sem risco):**
- Modo Markdown: edição completa de tabelas.
- Modo Rico: 
  - renderiza a tabela como tabela (visual) **ou** como bloco “tabela (Markdown)” com preview;
  - fornece ações de conveniência na toolbar: “Inserir tabela (2x2/3x3)”, “Adicionar linha/coluna”.

2) **Fase 2 (edição rica completa):**
- Implementar edição de tabela no modo rico (adicionar/remover linhas/colunas, header row, etc.).

**Ponto crítico:** serializar uma tabela rica de volta para GFM sem perda exige um conversor confiável.
- Se a implementação com TipTap ficar complexa, considerar trocar o modo rico para um editor markdown-first (ex: Milkdown), que já nasce com GFM/tables mais alinhado ao “Markdown como fonte de verdade”.

### Mermaid

**Representação Markdown:** fenced code block com linguagem `mermaid`:

```\n```mermaid\n...\n```\n```

Estratégia recomendada:
- Modo Markdown: edição do bloco normalmente.
- Modo Rico: mostrar um **preview renderizado** do diagrama e um botão “Editar código”, que abre um editor do bloco (CodeMirror) com **preview ao vivo** (espelhado).

#### Modal/painel de edição Mermaid (UX detalhado)

Objetivo: manter a experiência “Docs-like” no modo rico, mas garantindo que a **fonte de verdade** continue sendo o **código Mermaid**.

**Como abre**
- No modo rico, o bloco Mermaid aparece como “card”: preview + ações.
- A ação “Editar código” abre um `SimpleModal` (ou um “painel” dockado, se preferir) com:
  - editor (CodeMirror) para o conteúdo do bloco ` ```mermaid ... ``` ` (somente o conteúdo interno);
  - preview renderizado ao lado/abaixo.

**Interações no preview (botão + duplo clique + digitação “em cima do diagrama”)**
- Clique simples no preview: apenas **seleciona o bloco** (estado de seleção/outline visível).
- Duplo clique no preview: abre o modal “Editar código” (mesmo comportamento do botão).
- Teclado, com o bloco selecionado:
  - `Enter` ou `F2`: abre “Editar código”.
  - Se o usuário começar a digitar (ex: pressionar `a`, `1`, `(`): abrir “Editar código” imediatamente e **inserir esse primeiro caractere** no CodeMirror (no cursor), para parecer “natural” como em editores Docs.
    - Observação: se isso ficar complexo no MVP, fallback aceitável é abrir o modal e **não** injetar o primeiro caractere, mas mostrar um hint “Você está editando o código do diagrama”.
- Para acessibilidade/descoberta: abaixo do preview, mostrar uma dica curta (apenas quando selecionado):
  - “Duplo clique ou Enter para editar o código”.
- Importante: o SVG/preview **não** deve ser `contentEditable`; qualquer tentativa de input deve ser capturada no container do bloco (handler de `onKeyDown`) e redirecionada para o modal.

**Apagar o diagrama (confirmação)**
- Com o bloco Mermaid selecionado no modo rico:
  - `Backspace` / `Delete` **não** apaga imediatamente.
  - Solicitar confirmação via **sistema de questionários** (reusar o `QuestionnaireDialog` já existente, sem criar um modal novo):
    - Título: “Remover diagrama?”
    - Descrição: “Isso removerá o bloco Mermaid do documento.”
    - Pergunta: boolean (ex: `confirm_remove`) ou single_choice (Remover/Cancelar)
    - Labels: “Remover” (destrutivo) e “Cancelar”.
  - Se confirmar, remover o nó do diagrama (mantendo o undo/redo do editor).
  - Se cancelar, manter seleção no bloco.
- Exceção (atalho “forte”, opcional): `Shift+Delete` pode remover sem confirmação (para usuários avançados).
- Se o foco estiver dentro do CodeMirror do modal de edição, `Backspace/Delete` funciona normalmente (edita texto), sem confirmação.

Nota de implementação (para não duplicar código):
- Centralizar confirmações em um helper único (ex: `uiStore.confirm(...)`) que renderiza pelo `QuestionnaireDialog` global.
- Se, no MVP, ainda não existir um driver de questionário para ações locais de UI, é aceitável usar `ConfirmDialog` **temporariamente**, mas o alvo final é **unificar** no questionário.

**Layout sugerido (MVP): split view**
- Desktop largo: 2 colunas (Código à esquerda, Preview à direita).
- Janela estreita: 2 tabs (Código | Preview) ou stack vertical (Código em cima, Preview embaixo).

**Foco e acessibilidade**
- Ao abrir: foco vai para o CodeMirror; selecionar todo o código (ou posicionar o cursor no início) para facilitar substituição rápida.
- `Esc`: fecha (Cancelar) sem aplicar.
- Ao fechar:
  - se Cancelar: retorna foco para o botão “Editar código” do mesmo bloco.
  - se Aplicar: retorna foco para o bloco Mermaid (card/preview) e anuncia “Diagrama atualizado”.

**Atalhos (MVP)**
- `Ctrl+Enter`: **Aplicar**.
- `Esc`: **Cancelar**.
- `Ctrl+S`: **Aplicar** (opcional, se não conflitar com “Salvar documento”).

**Preview ao vivo (espelhado)**
- Atualiza com debounce (ex: 250–400ms) após digitação.
- Se a renderização for custosa, fazer:
  - “render em background” (spinner discreto) e
  - cancelar render anterior ao digitar novamente.

**Tratamento de erro de Mermaid**
- Se `mermaid.render` falhar:
  - Preview mostra um estado de erro (mensagem curta + detalhes expandíveis);
  - manter o **último preview válido** opcionalmente (para não “piscar”) ou mostrar placeholder.
- Ações no erro:
  - “Copiar erro” (copia mensagem/stack para o clipboard).
  - “Re-renderizar” (força nova tentativa).

**Aplicar vs. persistência no documento**
- Ao clicar “Aplicar”:
  - validar que o texto não está vazio (ou permitir vazio para “remover diagrama”, se fizer sentido);
  - atualizar o **nó/bloco Mermaid** no modo rico (substituir apenas o conteúdo do bloco);
  - serializar o documento inteiro para Markdown.
- O “Aplicar” deve ser atômico: ou substitui o bloco inteiro, ou não altera nada.

**Como identificar o bloco correto (sem depender de offsets no Markdown)**
- No modo rico, cada bloco Mermaid deve carregar um `id` estável (ex: atributo no node do TipTap).
- O modal recebe `{ mermaidBlockId }` e atualiza exatamente aquele nó, mesmo que o documento mude ao redor.

**Critérios de aceite (MVP)**
- Abrir “Editar código” sempre edita o bloco certo.
- Preview reflete mudanças sem travar a UI.
- Erros de renderização não impedem salvar/aplicar (usuário pode aplicar mesmo com erro, se quiser).
- Fechar/Aplicar sempre devolve foco para um alvo previsível.

Decisão explícita:
- **Não** suportar edição direta “clicar no rótulo no SVG e digitar”. O caminho oficial é editar o **código Mermaid** e ver o preview refletir em tempo real.

Obs: o projeto já tem renderer de Mermaid para Markdown (com preview e ações), então dá para reaproveitar a mesma abordagem/estilos no Editor.

### Seleção e patches (ponto crítico)
Para evitar mapear offsets de seleção do TipTap para offsets no Markdown (frágil):

- Quando o usuário pede a alteração no modo rico, o Editor captura:
  - o **slice selecionado** (como nodes) e também
  - o **slice selecionado serializado para Markdown** (chamar serializer apenas no range selecionado).
- O patch retornado pelo LLM vem com `replacement` em Markdown.
- Ao aplicar no modo rico, a substituição é feita no TipTap (parse do `replacement` → nodes) e só então o documento inteiro é serializado para Markdown.

Assim, a aplicação do patch é **posicional** (na seleção do editor) e não depende de achar o trecho no Markdown.

---

## Toolbar do EditorPage (layout)

Inspirada em `Toolbar` existente:

- Esquerda: título do doc/tab atual.
- Direita (ações):
  - **Novo** (nova aba/documento)
  - **Salvar** (quando houver persistência)
  - **Perguntar ao chat** (Ctrl+Shift+I)
  - Botões de formatação: Negrito, Itálico, Lista, Code, Link (MVP pode ser minimal)

---

## Contrato do patch (formato de saída do LLM)

Para ser robusto ao streaming e fácil de parsear, o LLM deve incluir um bloco de código no final:

```editor_patch
{JSON}
```

### JSON (v1)

```json
{
  "v": 1,
  "op": "replace_selection",
  "format": "markdown",
  "replacement": "...",
  "notes": "(opcional) breve justificativa"
}
```

Regras:
- `replacement` é o conteúdo a ser aplicado.
- `format`:
  - `markdown`: o Editor interpreta e insere como conteúdo rico (via parser/conversão).
  - `plain`: insere como texto literal.

Recomendação prática com Markdown como fonte de verdade:
- Para o modo rico e para o modo Markdown, padronizar `format="markdown"` sempre que possível.

### Validações no frontend
- Se não há seleção e `op=replace_selection`, usar bloco atual (parágrafo) como fallback.
- Limite de tamanho do patch (ex: 200KB) para evitar travar UI.
- Se o patch não parsear, mostrar toast “Resposta não contém patch aplicável”.

---

## Como garantir que o LLM sempre emita patch

Opção A (MVP): Instrução no próprio prompt do Editor
- O Editor envia um texto que inclui:
  - trecho selecionado
  - instrução do usuário
  - regras do bloco ```editor_patch```

Opção B (melhor): Criar um **skill** dedicado (ex: `/editor`) com instruções fixas
- Editor envia `/<slug>` para invocar o skill e reduz variabilidade.

---

## Abas do Editor

### Comportamento
- Cada aba representa um “documento” (rascunho).
- Pode renomear com F2 (padrão `ChatTabs`).
- Fechar com Ctrl+W / Delete (quando focado nas tabs).

### Persistência (fase 1 vs fase 2)
- Fase 1: persistir no `localStorage` (rápido de implementar, sem migração de DB).
- Fase 2: persistir no backend (SQLite) com tabela `editor_documents` (nome, conteúdo, timestamps).

---

## Integração com navegação do app

Arquivos que precisam ser atualizados quando for implementar:

- Adicionar rota em [frontend/src/lib/router.tsx](frontend/src/lib/router.tsx): `path: 'editor'` → `<EditorPage />`.
- Adicionar item no menu e detecção de página atual em [frontend/src/components/layout/Topbar.tsx](frontend/src/components/layout/Topbar.tsx) (novo item “Editor”).

Sugestão de rota: `/editor`.

---

## Detalhe importante — como “escutar” o patch sem quebrar o chat

O chat já faz streaming e emite eventos (`chat:stream`, `chat:done`). Para o Editor:

1. O Editor dispara `sendMessage(...)` no `chatStore`.
2. Enquanto o chat está streaming, o Editor fica em **busy**.
3. Quando a conversa emitir `chat:done`, o Editor pega a **última mensagem assistant do turno** e tenta extrair o bloco ```editor_patch```.

Implementação recomendada:
- Um hook tipo `useEditorInlineChatPatch()` que:
  - sabe qual conversa/tab estava ativa quando o pedido foi enviado
  - guarda um `requestId` local (correlation) para evitar aplicar patch de outro turno
  - só parseia quando `isLoading` do chat voltar para false e/ou quando chegar `chat:done` daquela conversa

Se não houver patch válido, o Editor deve:
- desbloquear o busy
- mostrar toast “O chat não retornou um patch aplicável”

---

## Backlog de implementação (sugestão)

### Fase 0 — Prova (1–2 dias)
- Inserir TipTap (modo rico) e CodeMirror (modo Markdown) num componente isolado.
- Implementar toggle Rico ⇄ Markdown com Markdown como fonte de verdade.
- Implementar captura de seleção (nos dois modos) e substituição local.

### Fase 1 — EditorPage + Tabs (2–4 dias)
- Criar `EditorPage.tsx`, `EditorTabs.tsx`, `editorStore.ts`.
- Toolbar básica + atalhos globais escopados.

### Fase 2 — Inline prompt + patch (3–6 dias)
- Modal de prompt (Ctrl+Shift+I).
- Envio para chat ativo + parsing ```editor_patch```.
- Modal de preview/diff + aplicar/rejeitar.

### Fase 3 — Qualidade (2–5 dias)
- Acessibilidade (ARIA, foco, roving tabindex, screen reader).
- Testes unitários do parser de patch.
- Documentos exemplo + help.

---

## Riscos e pontos de atenção
- **Streaming**: patch pode chegar fragmentado; só parsear ao final (`chat:done`).
- **Conflito de atalhos**: definir escopo por página e respeitar inputs.
- **Segurança**: nunca aplicar patch automaticamente.
- **Acessibilidade**: foco ao abrir/fechar modais e ao aplicar mudanças.

---

## Critérios de aceite do MVP
- Conseguir criar 2+ documentos em abas e alternar via Ctrl+Tab.
- Selecionar texto e, com Ctrl+Shift+I, pedir uma alteração.
- Ver preview e aplicar/rejeitar com segurança.
- Alteração aplicada bate exatamente com o preview.
