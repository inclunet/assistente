---
name: memoria frontend
overview: Adicionar ao plano da AEP-0075 uma tela de gerenciamento de memórias, necessária porque a memória deixará de ser editável via Markdown e passará a ser persistida no banco como records estruturados.
todos:
  - id: memory-ui-aep
    content: Adicionar à AEP-0075 a decisão de criar uma tela frontend para governança de memórias em banco.
    status: completed
  - id: memory-api-plan
    content: Planejar APIs Wails/Go para CRUD, busca e arquivamento de records de memória.
    status: completed
  - id: memory-ui-plan
    content: Planejar página React acessível para listar, filtrar, editar e reclassificar memórias.
    status: completed
isProject: false
---

# Tela De Memórias Na AEP-0075

## Decisão

A AEP-0075 deve incluir uma UI de gerenciamento de memórias no frontend. Ao migrar a memória para records estruturados em banco, o usuário perde a edição direta via `~/.assistente/memory/*.md`; portanto, a aplicação precisa oferecer uma tela própria para visualizar, editar e governar essas memórias.

## Escopo Inicial

Adicionar uma página de memórias com:

- Lista pesquisável/filtrável de records salvos.
- Visualização de conteúdo, classificação e metadados essenciais.
- Edição de conteúdo, `load_policy`, `kind`, `scope`, tags e importância.
- Ações para arquivar/desarquivar e excluir com confirmação.
- Indicação clara de quais memórias entram automaticamente no contexto: `core`, `pinned` e candidatas `auto`.
- Suporte à recomposição assistida: o modelo pode criar records a partir dos arquivos legados, e o usuário revisa na tela.

## UX E Acessibilidade

Usar componentes existentes em `frontend/src/components/ui/`, especialmente `DataGrid`, `Modal`, `ConfirmDialog`, `Button` e `Toolbar`. Todas as strings devem ir para `frontend/src/locales/pt-BR.ts`, `frontend/src/locales/en.ts` e `frontend/src/locales/es.ts`.

A tela deve evitar cores hardcoded e usar somente tokens de `frontend/src/theme.css`.

## Backend Necessário

Expor APIs Wails para CRUD e busca de memória:

- listar com filtros;
- obter detalhe;
- criar/editar record;
- arquivar/desarquivar;
- excluir;
- buscar records relevantes para `memory_search`.

## Arquitetura

```mermaid
flowchart TD
  legacyFiles["Arquivos legados de memória"] --> assistedRebuild["Recomposição assistida pelo modelo"]
  assistedRebuild --> memoryDB["Records de memória no banco"]
  memoryUI["Tela de Memórias"] --> memoryDB
  memoryProvider["Memory Context Provider"] --> memoryDB
  memoryProvider --> promptBlocks["Blocos dinâmicos do prompt"]
  memoryTools["memory_search e memory_get"] --> memoryDB
```

## Critérios De Aceitação

- O usuário consegue ver e editar memórias salvas sem abrir arquivos Markdown.
- O usuário consegue alterar a política de carregamento de uma memória.
- Memórias `archived` não entram no contexto automático.
- Exclusão exige confirmação acessível.
- A tela deixa claro quais memórias impactam o prompt automaticamente.
- A recomposição dos arquivos antigos continua sem migrador automático; a revisão acontece pela UI.