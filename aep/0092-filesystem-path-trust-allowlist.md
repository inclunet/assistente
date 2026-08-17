# AEP-0092 — Autorização explícita e allowlist escopável para paths fora do sandbox

**Status:** 🚧 In Progress — Fase 1 + 1b ✅; Fase 2 pendente

## Resumo

As tools de filesystem (`read_file`, `write_file`, `edit_file`, `grep_search`,
`list_dir`, `search_files`, `delete_file`, `move_file`, `copy_file`,
`make_directory`) aceitam hoje apenas dois conjuntos de paths:

1. o **workspace ativo** (hoje, na prática, o `cwd` congelado no boot — bug);
2. a árvore `~/.assistente`.

Qualquer outro caminho é **hard-deny**, com uma única exceção: arquivo **exato**
aberto numa aba de editor (só para `read`/`write`/`edit`/`grep`). Essa exceção
carrega uma pilha larga (contexto, context provider, testes) e não cobre
diretórios nem operações estruturais.

Este AEP substitui o deny absoluto por um fluxo de **consentimento explícito**
com **allowlist escopável por path + operação**, no mesmo espírito do
AEP-0082 (network trust) e com diálogo AEP-0091. A exceção “aberto no editor”
deixa de ser bypass de segurança.

## Motivação

- Precisa editar/ler pastas fora do workspace sem abrir o arquivo no editor só
  para “desbloquear” o path.
- A exceção do editor é frágil (match exato, fecha a aba = perde permissão) e
  documenta regras especiais no prompt do LLM.
- O hard-deny seco não oferece `once` / `session` / persistência.
- O `workDir` das tools não acompanha o workspace ativo (diferente do
  `nettrust.SetWorkspaceDirFunc`).

## Decisões (questionário fechado)

| # | Decisão |
|---|---|
| D-Q1 | Grant padrão = **path exato** da tentativa. Liberar pasta (`/**`) só com **ação explícita** no diálogo (não automático). |
| D-Q2 | Grant = **path + operação**. Cada operação se autoriza sozinha (read ≠ write ≠ delete…). |
| D-Q3 | Abrir no editor = **só UX**; a tool ainda pede autorização. |
| D-Q4 | Ops estruturais (`list`, `search`, `move`, `delete`, `mkdir`, `copy`) entram no **mesmo** fluxo. |
| D-Q5 | Arquivos sensíveis (`.env`, chaves, `.pem`…) continuam **hard-deny**; allowlist não libera. |
| D-Q6 | Raiz sempre permitida = **workspace ativo** + `~/.assistente` (corrige cwd do boot). |
| D-Q7 | A **raiz** de um walk (`grep`, `list`, `search`) passa pelo fluxo normal e pode pedir autorização; as **entradas percorridas** que escapam do sandbox são **puladas em silêncio**, nunca viram prompt. |

**Fase 2 (não bloqueia Fase 1):** denylist explícita que pode restringir até
dentro do workdir — “sempre permitido” deixa de ser absoluto quando houver
deny. Ver D9.

### D1. Hook único em `validatePathWithPolicy`

Toda tool já passa por `internal/tools/filesystem.validatePathWithPolicy`. O
branch `errOutsideAllowedDirs` deixa de ser deny seco e passa a:

1. se path está nas raízes permitidas → segue (skill allowlist + sensíveis);
2. senão → `fstrust.Authorizer.Authorize(ctx, path, operation)`;
3. match na allowlist → libera;
4. sem match → DecisionDialog; negar = erro acionável; autorizar = persiste
   (se escopo persistente) e libera a tentativa atual.

Skill `FilesystemScope` e `isSensitiveFile` continuam **depois** do trust e
com precedência de deny (skill deny / sensível nunca são anulados por trust).

Toda comparação de sandbox usa o **destino real** do path (symlinks resolvidos,
inclusive quando o alvo ainda não existe). Sem isso, um link dentro da raiz
apontando para fora passaria no `isWithinRoot` pelo nome literal e a leitura
sairia do sandbox sem consentimento.

**Walks não perguntam por entrada (D-Q7).** `grep_search`, `list_dir` e
`search_files` validam a raiz pelo fluxo normal — ali o prompt é legítimo — mas
um prompt por arquivo percorrido inviabilizaria a busca. Então, durante o walk,
entrada cujo destino real cai fora das raízes permitidas é omitida do resultado,
do mesmo jeito que arquivos sensíveis. O usuário que quiser aquele conteúdo pede
pelo caminho real, e aí passa pelo consentimento normal. Na prática o teste só
recai sobre symlinks: `WalkDir` não desce por link, então entrada comum abaixo
de uma raiz já validada continua dentro dela.

### D2. Pacote `internal/fstrust` (espelha `nettrust`)

| Componente | Papel |
|---|---|
| `Manager` | Match + Add/Remove; escopos `once` / `session` / `workspace` / `profile` / `global` |
| `Authorizer` | match → allow; senão prompt → persistir → allow |
| `Prompter` | interface; implementação em `internal/app` via questionnaire |

Ordem de match: sessão → perfil → workspace → global (igual AEP-0082).

Persistência (via `configdir`):

| Escopo | Arquivo |
|---|---|
| `once` | não persiste |
| `session` | memória, por `ConversationID`; limpa em `resetConversationScopedState` junto com nettrust |
| `workspace` | `<workdir>/.assistente/path-allowlist/workspace.json` |
| `profile` | `~/.assistente/path-allowlist/profile-<slug>.json` |
| `global` | `~/.assistente/path-allowlist/global.json` |

Entrada:

```text
AllowlistEntry{
  Path       string   // absoluto normalizado; raiz do diretório se Kind=dir
  Kind       "file" | "dir"
  Operation  string   // operação exata: read, write, edit, grep, list, ...
  Scope      Scope
  CreatedBy  string
  CreatedAt  time.Time
  Reason     string
}
```

Match:

- `Kind=file`: path da request (após `Abs`/`Clean`/normalização OS) == `Path`,
  e `Operation` igual.
- `Kind=dir`: path da request está **dentro** do diretório (mesmo critério
  `isWithinRoot`), e `Operation` igual.
- Não há “write implica read”: cada operação exige entrada própria.

### D3. Consentimento = DecisionDialog (AEP-0091)

O prompter monta `kind: decision` com:

- título/descrição i18n (AEP-0085): path, operação, se está fora do workspace;
- body só leitura com o path;
- **um botão por escopo** para o **path exato + operação atual**
  (`once`, `session`, `workspace`, `profile`, `global`);
- **ação explícita opcional** por escopo (ou um segundo conjunto) para
  autorizar a **pasta pai `/**`** com a **mesma operação** — só se o usuário
  escolher essa ação (D-Q1);
- última ação: Negar.

`actionId` estável (`once`, `session`, …, `dir-once`, `dir-session`, …,
`deny`). Nunca o rótulo traduzido.

### D4. Remove bypass “open editor”

- `isOpenEditorAllowed` deixa de liberar path fora do sandbox.
- `WithOpenEditorPaths` / listagem no context provider podem permanecer para
  **UX** (confirmar diff no doc aberto, links, instruções), mas **não** como
  autorização de path.
- Texto em `<workspace_instructions>` que fala em
  `open_editor_file[...]` como exceção de tool **é atualizado**: arquivos
  abertos são contexto; acesso fora do workspace exige autorização.

Confirmação Antes/Depois (`edit_confirmation`, AEP-0032) **permanece** como
camada de conteúdo, independente do trust de path.

### D5. Raiz permitida = workspace ativo

Injetar resolvedor de workDir (como `nettrust.SetWorkspaceDirFunc`):

```text
root := workspaceMgr.ActivePath()
se vazio → fallback os.Getwd() / configdir
+ sempre ~/.assistente
```

Tools deixam de depender só do cwd congelado no `initToolRegistry`.

### D6. Erro acionável

Negar ou ausência de authorizer → erro estruturado (`DeniedPathError`: path,
operação, motivo “fora do sandbox”) com sugestões e deep link
`assistente://navigate/settings/path-allowlist` para a gestão da allowlist.

### D7. UI de gestão (Fase 1b / Fase 2 leve)

Listar e remover entradas persistidas (workspace / profile / global). **Não**
criar entradas pela tela — nascem só do consentimento (mesmo D7 do AEP-0082).

Pode ir no mesmo PR da Fase 1 se couber; senão PR empilhado imediatamente após.

### D8. Relação com `trustscope` (AEP-0082 D8)

O núcleo escopado idealmente vira `internal/trustscope` compartilhado por
rede e FS. **Fase 1 pode espelhar `nettrust` em `fstrust`** para entregar o
comportamento sem bloquear na extração. Extração para `trustscope` fica como
follow-up explícito (issue/AEP ou fase 1.5), sem mudar o formato em disco das
allowlists de rede.

### D9. Denylist dentro do workdir (Fase 2)

Hoje as raízes são “sempre OK”. Demanda: em situações específicas, **proibir**
operações mesmo no workdir / `~/.assistente`.

Fase 2:

- entradas `Deny` com a mesma forma (`Path`/`Kind`/`Operation`/`Scope`);
- ordem: **deny** (qualquer escopo) → raízes / allow trust → prompt;
- trust **nunca** anula deny;
- UI lista/remove denies (criação só via consentimento “Negar e lembrar” ou
  fluxo dedicado — a decidir na Fase 2).

Fora do escopo da Fase 1.

## Segurança

- Nenhum path fora das raízes é liberado sem ato explícito.
- Sensíveis continuam hard-deny sob `ToolPolicy`.
- Skill deny continua com precedência.
- Symlinks: validar path resolvido (`EvalSymlinks`) antes de match/persistir,
  para não autorizar um path “inocente” que aponta para sensível ou fora do
  que o usuário viu no diálogo.
- Escopo `session` limpa com a conversa (mesmo motivo do nettrust).

## Fases

### Fase 1 — núcleo (este PR / worktree)

- [x] Pacote `internal/fstrust` (Manager, Authorizer, tipos, testes)
- [x] Prompter `DecisionDialog` em `internal/app` + i18n pt-BR/en/es
- [x] Hook em `validatePathWithPolicy`; wire no App
- [x] WorkDir dinâmico = workspace ativo
- [x] Remover bypass de segurança `isOpenEditorAllowed` no sandbox
- [x] Atualizar context provider / docs de instrução do workspace
- [x] Testes: fora do sandbox → prompt; match allow; deny; sensível; dir
      explícito; workspace root

### Fase 1b — UI listar/remover

- [x] Página/settings path-allowlist (espelho network-allowlist)
- [x] Deep link no erro de negação

### Fase 2 — denylist no workdir

- [ ] Modelo Deny + precedência
- [ ] UI e limpeza de sessão
- [ ] Critérios de aceite próprios

## Riscos

- **Prompt fatigue:** muitas operações granulares. Mitigação: escopos
  session/workspace e ação explícita de pasta quando o fluxo for amplo.
- **Duplicação nettrust/fstrust:** dívida até `trustscope`; aceitável na Fase 1.
- **Symlink / junction no Windows:** exigir resolução antes de persistir.
- **Downgrade de UX do editor:** quem dependia do bypass precisará autorizar
  uma vez — intencional.

## Critérios de aceitação (Fase 1)

- [x] Path em workspace ativo ou `~/.assistente` não pergunta.
- [x] Path fora, sem allowlist → DecisionDialog; Negar → tool falha com erro
      claro; autorizar `once` → só aquela tentativa; `session`+ → próximas
      iguais (mesmo path+op) passam sem prompt.
- [x] Autorizar arquivo **não** libera a pasta; liberar pasta exige ação
      explícita `dir-*`.
- [x] Autorizar `read` **não** libera `write`/`delete`/etc.
- [x] Arquivo sensível fora (ou dentro) continua bloqueado sob ToolPolicy.
- [x] Abrir arquivo no editor **não** dispensa o prompt da tool.
- [x] `list`/`delete`/`mkdir`/… fora do sandbox usam o mesmo Authorizer.
- [x] Trocar workspace ativo atualiza a raiz permitida sem reiniciar o app.
- [x] Testes unitários cobrem Manager/Authorizer e o hook de pathutil.
- [ ] CI verde; review Copilot zerada.

## Referências

- AEP-0082 — network trust allowlist (modelo de escopos + prompt)
- AEP-0091 — DecisionDialog
- AEP-0085 — i18n de diálogos do questionnaire
- AEP-0032 — confirmação de edição no editor (camada de conteúdo, mantida)
- `internal/tools/filesystem/pathutil.go` — sandbox atual
- `internal/tools/open_editors.go` — bypass a remover da política de path
