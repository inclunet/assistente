# Review PR 90 — AEP-0047 Importação e Exportação

Este documento registra o estado do review do PR 90
(`feat/aep-0047-import-export`) após a sincronização com `origin/main`, já com
as mudanças da AEP-0046/PR 89 incorporadas.

## Veredito

O PR está bem avançado, mas ainda não está merge-ready.

O núcleo DB-only previsto pela AEP-0047 já está largamente implementado:
conversas/mensagens, providers persistidos, tasklists persistidas, credenciais
portáteis criptografadas e export HTML/PDF derivado para conversas. As
validações principais passaram.

Ainda faltam ajustes de escopo e acabamento para que o comportamento entregue
fique alinhado com a AEP atual:

- a UI desktop ainda trata o fluxo como "conversas" e não como "dados",
  bloqueando exportações que o backend/CLI já suportam, como providers-only,
  tasklists-only ou credentials-only;
- campos de `ExportRequest` de recursos fora do escopo DB-only são aceitos e
  ignorados silenciosamente;
- a AEP ainda contém critérios/textos antigos que conflitam com o contrato atual
  de `version: 2` e IDs preservados;
- há pequenos pontos de robustez no backend e de cobertura frontend para fechar
  o risco antes do merge.

## Escopo DB-only Considerado Para Este PR

Incluído nesta etapa:

- conversas e mensagens;
- providers LLM persistidos;
- tasklists persistidas, incluindo workflow, tarefas, subtarefas e notas;
- bloco portátil de credenciais criptografadas com senha de exportação;
- export HTML/PDF derivado apenas para conversas;
- CLI para export/analyze/import.

Fora desta etapa até migrações futuras:

- profiles;
- skills;
- allowlists;
- MCP servers;
- jobs;
- channels;
- contacts;
- workspaces.

Esses recursos ainda devem permanecer fora do PR até estarem no banco ou até as
migrações/reestruturações das AEPs dependentes ocorrerem.

## Validação Executada

- `go test ./internal/portability ./internal/app ./cmd/asst`: passou.
- `npm run test -- HistoryPage.test.tsx exportImport.test.ts`: passou.
- `go test ./...`: passou.
- `npm run build`: passou (`tsc` + `vite build`).
- `npm run test`: passou, 176 arquivos e 1083 testes.
- E2E de histórico:
  - `npx playwright test e2e/history/history.spec.ts e2e/history/history-bulk-operations.spec.ts`
  - passou, 10 testes.

Observações:

- `HistoryPage.test.tsx` passa, mas ainda emite warnings de React por mocks
  incompletos de ícones/componentes usados pela página.
- A única alteração local fora do PR no momento do review era whitespace em
  `internal/speech/speech_manager.go`.

## O Que Já Está Implementado

### Backend e Formato Portátil

Arquivos principais:

- `internal/portability/types.go`
- `internal/portability/service.go`
- `internal/portability/providers.go`
- `internal/portability/tasklists.go`
- `internal/portability/crypto.go`
- `internal/portability/render.go`

Implementado:

- Envelope JSON canônico com `version`, `exportedAt`, `appVersion`, `options` e
  `resources`.
- `ExportVersion = 2`.
- Rejeição de versões diferentes de `2` no analyze/import.
- Export/import de conversas com mensagens embutidas.
- Preservação de IDs UUID, `conversationId`, `parentId` e `turnId`.
- Índices auxiliares `parentIndex` e `turnIndex` para referências internas.
- Áudio omitido por padrão, com `audioMimeType` preservado.
- Export/import de providers por ID.
- Export/import de tasklists com workflow, tasks, subtasks e notes.
- Export opcional de credenciais em bloco criptografado com Argon2id + AES-256-GCM.
- Import de credenciais regravando no cofre local da instância destino.
- HTML/PDF derivados para conversas.
- Detecção de recursos fora do escopo em `resources` e warning ao importador.

### App Layer e Wails

Arquivos principais:

- `internal/app/export_import.go`
- `frontend/wailsjs/go/app/App.d.ts`
- `frontend/wailsjs/go/app/App.js`
- `frontend/wailsjs/go/models.ts`

Implementado:

- `ExportData(req)`.
- `ExportDataToFile(req, path)`.
- `AnalyzeImportData(jsonData, credentialExportPassword)`.
- `ImportData(jsonData, credentialExportPassword)`.
- `ImportDataWithResolutions(req)`.
- `ExportConversationsToFile(ids, format)` para HTML/PDF.

### CLI

Arquivo principal:

- `cmd/asst/data.go`

Implementado:

- `asst data export`;
- `asst data analyze`;
- `asst data import`;
- flags para `--all`, `--conversations`, `--providers`, `--tasklists`,
  `--only-credentials`, IDs específicos, `--include-credentials`,
  `--credential-password`, `--format`, `--out`.

### Frontend

Arquivos principais:

- `frontend/src/pages/HistoryPage.tsx`
- `frontend/src/pages/HistoryPage.css`
- `frontend/src/lib/exportImport.ts`
- `frontend/src/locales/pt-BR.ts`
- `frontend/src/locales/en.ts`
- `frontend/src/locales/es.ts`

Implementado:

- modal de export JSON canônico a partir da página de histórico;
- checkboxes para incluir providers persistidos;
- checkboxes para incluir tasklists persistidas;
- opção de incluir credenciais criptografadas com senha;
- import via arquivo JSON com preview;
- análise de conflitos/warnings;
- senha para credenciais criptografadas no import;
- resumo do resultado de import;
- strings adicionadas nos três locales.

## Achados Bloqueantes / Antes De Merge

### 1. Alta — UI desktop ainda não permite export DB-only sem conversas

Arquivos envolvidos:

- `frontend/src/pages/HistoryPage.tsx`

Problema:

O backend e o CLI já suportam exportar providers, tasklists e credenciais no
recorte DB-only. A UI, porém, ainda parte da página de histórico e exige alvo de
conversas em alguns pontos:

```tsx
const openExportModal = useCallback((idsToExport: string[]) => {
  if (idsToExport.length === 0) {
    announce(t('history.noConversationsToExport', 'Nenhuma conversa para exportar'), 'assertive');
    return;
  }
  setExportTargetIds(idsToExport);
  // ...
}, [announce, t]);
```

Na confirmação, também há dependência de `exportTargetIds` e o payload sempre
parte de `conversationIds`:

```tsx
const payload: ExportRequestPayload = {
  conversationIds: idsToExport,
  includeCredentials: options?.includeCredentials === true,
  outputFormat: 'json',
};
```

Impacto:

- Não dá para exportar somente providers.
- Não dá para exportar somente tasklists.
- Não dá para exportar somente credenciais.
- A UI entrega menos que o backend e menos que o recorte DB-only da AEP.

Correção recomendada:

- Renomear o fluxo de "conversas" para "dados" no modal e labels principais.
- Permitir abrir o modal mesmo sem conversas quando providers/tasklists ou
  credenciais forem selecionáveis.
- Montar `ExportRequest` com `explicitSelection` quando a seleção for
  intencionalmente vazia para conversas.
- Decidir se providers/tasklists serão exportados all-or-nothing nesta PR ou se
  haverá seleção individual. All-or-nothing é aceitável para fechar a fase, mas
  deve estar claro no texto da UI.

### 2. Alta — Campos fora do escopo em `ExportRequest` são ignorados silenciosamente

Arquivos envolvidos:

- `internal/portability/types.go`
- `internal/app/export_import.go`

Problema:

`ExportRequest` ainda expõe campos para recursos que a própria AEP declara fora
do escopo imediato:

```go
ProfileSlugs     []string `json:"profileSlugs,omitempty"`
SkillSlugs       []string `json:"skillSlugs,omitempty"`
AllowlistSlugs   []string `json:"allowlistSlugs,omitempty"`
MCPServerSlugs   []string `json:"mcpServerSlugs,omitempty"`
JobIDs           []string `json:"jobIds,omitempty"`
ChannelNames     []string `json:"channelNames,omitempty"`
IncludeContacts  bool     `json:"includeContacts"`
IncludeWorkspace bool     `json:"includeWorkspace"`
```

Esses campos não aparecem em resolvers equivalentes em `internal/app/export_import.go`.
Hoje, se algum cliente chamar `ExportData` com esses campos preenchidos, o pedido
pode parecer aceito, mas o export ignora silenciosamente os recursos.

Impacto:

- Risco de perda silenciosa de expectativa do usuário/API.
- Contrato Wails/CLI fica ambíguo.
- A AEP diz que esses recursos estão fora desta fase; o código deveria comunicar
  isso de forma explícita.

Correção recomendada:

- Adicionar validação em `ExportData` ou helper dedicado para rejeitar campos
  fora do escopo com erro claro.
- Exemplo de mensagem: "profiles/skills/workspaces ainda não são suportados
  nesta fase DB-only da AEP-0047".
- Adicionar teste Go cobrindo essa rejeição.

### 3. Média/Alta — AEP ainda contém critérios antigos/inconsistentes

Arquivo envolvido:

- `aep/0047-import-export.md`

Problemas:

- Critério de aceite fala em roundtrip "exceto IDs", mas o contrato atual da AEP
  e do código preserva IDs UUID.
- Critério de aceite fala em rejeitar `version > 1`, mas o formato atual é
  `version: 2` e o código rejeita qualquer versão diferente de `2`.
- As fases antigas ainda citam implementação de resources em arquivo
  (profiles/skills/allowlists/MCP/jobs/channels/workspaces), enquanto a própria
  AEP foi ajustada para DB-only nesta PR.

Impacto:

- Reviewers podem cobrar itens que não devem entrar agora.
- A divergência entre critérios e implementação enfraquece o contrato de merge.

Correção recomendada:

- Atualizar os critérios de aceitação para o recorte DB-only desta PR.
- Separar claramente "objetivo final da AEP" de "critério de aceite deste PR".
- Trocar o critério de versão para `version: 2` e rejeição de versões
  incompatíveis.
- Trocar o critério de roundtrip para IDs preservados.

### 4. Média — Lookups GORM devem usar `errors.Is` para `ErrRecordNotFound`

Arquivos envolvidos:

- `internal/portability/service.go`
- `internal/portability/providers.go`
- `internal/portability/tasklists.go`

Problema:

Alguns lookups comparam erro diretamente com `gorm.ErrRecordNotFound`, enquanto
outros pontos do próprio pacote já usam `errors.Is`.

Exemplo:

```go
if err == gorm.ErrRecordNotFound {
  return nil, nil
}
```

Impacto:

- Se o erro vier embrulhado por GORM ou por outro wrapper, o import pode tratar
  "não encontrado" como erro real de banco.
- Isso afeta o fluxo de upsert/idempotência por ID.

Correção recomendada:

- Padronizar todos os lookups para `errors.Is(err, gorm.ErrRecordNotFound)`.
- Adicionar ou ajustar testes cobrindo caminho de "not found" nos importadores.

### 5. Média — PDF pode depender de fontes do sistema no ambiente de CI/usuário

Arquivo envolvido:

- `internal/portability/render.go`

Problema:

`RenderConversationsPDF` tenta configurar fonte UTF-8 a partir de fontes do
sistema. Em ambientes mínimos, especialmente Linux/CI, pode não haver fonte TTF
nos caminhos esperados.

Impacto:

- Export PDF pode falhar em ambientes sem fontes instaladas.
- A validação local passou, mas isso precisa ser confirmado no ambiente real do
  CI/release.

Correção recomendada:

- Validar o PDF no CI real.
- Se falhar, embutir uma fonte compatível ou adicionar fallback robusto e teste.

### 6. Média — Import com `includeCredentials: true` e sem bloco pode passar silenciosamente

Arquivo envolvido:

- `internal/portability/service.go`

Problema:

O import de credenciais só roda quando as duas condições são verdadeiras:

```go
if file.Options.IncludeCredentials && file.Resources.Credentials != nil {
  // importa credenciais
}
```

Se um arquivo vier com `options.includeCredentials: true`, mas sem
`resources.credentials`, o import não falha nem emite warning específico.

Impacto:

- Arquivo malformado parece importado com sucesso, apesar de declarar que contém
  credenciais.

Correção recomendada:

- Retornar erro ou warning explícito quando `includeCredentials` for `true` e o
  bloco `credentials` estiver ausente.
- Adicionar teste de regressão.

### 7. Média — Cobertura frontend ainda não valida os fluxos novos de import/export

Arquivos envolvidos:

- `frontend/src/pages/HistoryPage.test.tsx`
- `frontend/src/lib/exportImport.test.ts`
- possivelmente E2E em `frontend/e2e/history/`

Problema:

`HistoryPage.test.tsx` ainda cobre majoritariamente delete/open. O E2E de
histórico cobre um caminho de export chamando `ExportData`, mas não cobre:

- incluir providers no payload;
- incluir tasklists no payload;
- senha obrigatória para credenciais;
- import com preview e senha;
- warnings de recursos fora do escopo;
- bloqueio/validação de senha inválida para credenciais;
- ausência de conversas com export de recursos DB-only.

Impacto:

- A UI é justamente a parte que mais diverge do backend; sem cobertura, regressões
  são prováveis.

Correção recomendada:

- Adicionar testes Vitest para `HistoryPage` cobrindo payloads de export/import.
- Adicionar pelo menos um E2E leve para o modal de export DB-only.

## Achados Não Bloqueantes / Limpeza

### 8. Baixa — Warnings de React nos testes de `HistoryPage`

Arquivo envolvido:

- `frontend/src/pages/HistoryPage.test.tsx`

Problema:

Os testes passam, mas emitem warnings como:

- `React.jsx: type is invalid ... got: undefined`

Isso parece vir de mocks incompletos de ícones/componentes usados por
`HistoryPage.tsx`.

Impacto:

- Ruído nos testes.
- Pode esconder warnings reais.

Correção recomendada:

- Mockar os ícones/componentes ausentes de forma estável ou ajustar o mock de
  `DataGrid`/`Toolbar` para não renderizar nós indefinidos.

### 9. Baixa — Help do CLI para `--include-audio` está impreciso

Arquivo envolvido:

- `cmd/asst/data.go`

Problema:

A flag `--include-audio` fala em "metadados de anexos de áudio", mas o backend
inclui o corpo do áudio quando `IncludeAudio` é verdadeiro.

Correção recomendada:

- Ajustar a descrição para deixar claro que o áudio em si pode ser incluído.
- Alternativamente, se a decisão for não suportar áudio nesta PR, remover a flag
  da UI/CLI e manter apenas `audioMimeType`.

### 10. Baixa — Código de conflitos/resoluções ainda existe apesar da direção DB-only idempotente

Arquivos envolvidos:

- `internal/portability/service.go`
- `internal/portability/types.go`
- `cmd/asst/data.go`
- `frontend/src/pages/HistoryPage.tsx`

Contexto:

A AEP D7 diz que o recorte DB-only deve convergir para import idempotente por
ID, removendo UI/backend de resolução interativa para esses recursos. O código
ainda mantém `ImportResolution`, `ImportDataWithResolutions` e estratégias
`skip/overwrite/rename`.

Avaliação:

- Não é necessariamente bloqueante se a equipe aceitar isso como compatibilidade
  residual para arquivos incompletos/legados dentro do período de transição.
- Mas a decisão precisa estar explícita na AEP ou o código deve ser simplificado.

Correção recomendada:

- Escolher uma das opções:
  - documentar que resolução por chave natural fica como fallback transitório;
  - ou remover/ocultar a camada de resolução para o recorte DB-only.

## Itens Que Devem Ficar Fora Deste PR

Não implementar agora:

- export/import de profiles;
- export/import de skills;
- export/import de allowlists;
- export/import de MCP servers;
- export/import de jobs;
- export/import de channels;
- export/import de contacts;
- export/import de workspaces;
- resolução completa de referências cruzadas envolvendo recursos fora do banco.

Esses itens dependem de migrações ou reestruturações futuras. Para este PR, o
comportamento esperado é rejeitar seleção/export desses recursos ou ignorar
seções importadas com warning claro.

## Checklist Para Finalizar

Antes de mergear:

- [ ] Ajustar UI para export DB-only sem exigir conversas.
- [ ] Renomear textos principais de "conversas" para "dados" onde o fluxo for
      geral.
- [ ] Rejeitar explicitamente campos fora do escopo em `ExportRequest`.
- [ ] Atualizar critérios de aceitação da AEP-0047 para o contrato `version: 2`
      com IDs preservados.
- [ ] Padronizar lookups GORM com `errors.Is`.
- [ ] Validar/exportar PDF no ambiente real de CI/release ou adicionar fallback
      de fonte.
- [ ] Emitir erro/warning para `includeCredentials: true` sem bloco
      `resources.credentials`.
- [ ] Adicionar cobertura frontend para providers/tasklists/credenciais/import
      preview.
- [ ] Limpar warnings de React em `HistoryPage.test.tsx`.

Depois disso, reexecutar:

- `go test ./...`
- `npm run build`
- `npm run test`
- E2E de histórico/import-export relevante.
