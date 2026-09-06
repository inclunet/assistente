# Auditoria de código legado mantido por testes (Issue #664)

Branch: `chore/auditoria-legado`
Data: 2026-09-05
Base analisada: `origin/main` em `4e758277`

## Resultado executivo

O levantamento preliminar confundiu compatibilidade exercitada por testes com código mantido apenas por eles. A análise de chamadas, entradas dinâmicas e AEPs corrigiu essa classificação.

Não há lote de produção ACP inequivocamente seguro para remover. `legacyModelState` e `session/set_model` são entradas de produção para agentes que anunciam modelos somente no formato anterior a `configOptions`. `fakeLegacyModels`, `scriptLegado` e `scriptSoLegado` vivem em testes, mas provam esse contrato. `legacyAllPreloaded` preserva perfis legados sem política estruturada de tools.

O lote seguro de remoção tem, portanto, **zero linhas**. Remover os candidatos P0 quebraria compatibilidade documentada ou apagaria sua cobertura. O relatório preliminar e o comentário original da issue foram corrigidos.

## Metodologia

### Inventário lexical

Marcadores pesquisados: `legacy|compat|deprecated|Postel`, sem distinção entre maiúsculas e minúsculas.

```powershell
rg -c -i 'legacy|compat|deprecated|Postel' internal -g '*.go' -g '!*_test.go'
rg -c -i 'legacy|compat|deprecated|Postel' internal -g '*_test.go'
```

O denominador da densidade é a soma das linhas retornadas por `rg -c '^'` nos mesmos conjuntos. A métrica conta linhas com marcadores, não ramos executáveis inteiros. É um proxy reproduzível para priorização, não cobertura nem prova isolada de código morto.

### Análise de chamadas

Para cada candidato prioritário, foram rastreados:

1. declaração e referências em produção;
2. referências em testes;
3. entrada externa dinâmica, como JSON-RPC, arquivo persistido ou migração;
4. inicialização e wiring que tornam o caminho alcançável;
5. decisão arquitetural aplicável.

No ACP, `legacyModelState` não é chamado como função: ele é preenchido pela desserialização das respostas externas de `session/new` e `session/load`. Uma busca apenas por chamadas produz falso positivo de código morto.

### Análise estática

```powershell
staticcheck ./internal/acp
staticcheck ./internal/...
```

Ambos terminaram sem achados. Isso exclui candidatos não exportados detectáveis pelo analisador, mas não prova que APIs exportadas ou entradas dinâmicas estejam inativas.

### Testes de caracterização

O processo ACP falso verifica efeito observável no turno seguinte, não apenas estado interno:

- lê o formato anterior na abertura e na retomada;
- usa `session/set_model` somente após `-32601` no seletor estável;
- não converte outra recusa em fallback;
- confirma que o modelo efetivo do turno seguinte mudou.

### AEPs consultados

- `aep/0084-agentes-acp-como-providers.md`, D2, D6 e Fases 4/9;
- `aep/0086-registro-acp-descoberta-e-instalacao-de-agentes.md`, D1 e D11;
- `aep/0077-tool-planner-and-tools-subsystem-evolution.md`, D3;
- `aep/0081-politica-tools-por-perfil-e-carregamento-sob-demanda.md`, D3;
- `aep/0088-strangler-fig-borda-wails-app.md`, inventário final da borda.

## Métricas reproduzíveis

| Conjunto | Arquivos Go não vazios | Linhas | Arquivos com marcador | Linhas com marcador | Densidade |
|---|---:|---:|---:|---:|---:|
| Produção (`!*_test.go`) | 517 | 133.575 | 102 | 450 | 0,3369% |
| Testes (`*_test.go`) | 457 | 140.440 | 79 | 550 | 0,3916% |

No ACP, produção contém 20 linhas com marcador: `session.go` (10), `mapping.go` (8) e `client.go` (2). Testes contêm 14: `mapping_test.go` (8), `fakeagent_test.go` (5) e `session_options_test.go` (1). Esses números incluem comentários e pontos do mesmo contrato; não equivalem a linhas removíveis.

### Maiores concentrações em produção

| Arquivo | Linhas | Decisão |
|---|---:|---|
| `internal/channels/legacy_cleanup.go` | 32 | manter |
| `internal/channels/legacy_import.go` | 25 | manter |
| `internal/app/app_legacy_imports.go` | 24 | manter |
| `internal/wailsapi/legacy_cleanup.go` | 21 | manter |
| `internal/database/schema_migrations.go` | 20 | manter |
| `internal/portability/legacy_import_service.go` | 19 | avaliar com evidência adicional |
| `internal/app/builtin_skills.go` | 14 | avaliar com evidência adicional |
| `internal/portability/legacy_import.go` | 13 | manter enquanto houver importadores |
| `internal/jobs/migration.go` | 13 | manter |
| `internal/llm/provider.go` | 12 | manter |

Concentração em testes não significa uso exclusivo por testes. Migrações e parsers históricos naturalmente têm mais fixtures do que chamadas estáticas.

## Inventário priorizado

### P0 — ACP: manter

#### `legacyModelState`, `legacyModel` e `withModelOption`

Entram em produção por `newSessionResponse.Models` e `loadSessionResponse.Models`, desserializados em `openSession` e `LoadSession`. O SDK não tipa o campo anterior `models`, mas agentes podem enviá-lo no wire. O AEP-0084 registra o GitHub Copilot CLI como caso observado. Remover deixaria a lista vazia e impediria a troca de modelo.

#### `legacySetModelMethod`, `setLegacyOption` e `legacySelector`

`SetConfigOption` tenta o seletor anterior somente quando `session/set_config_option` retorna JSON-RPC `-32601`. Esse fallback é exigido pelo AEP-0084 D6. Outros erros sobem normalmente, sem mascarar recusas.

#### Fixtures `fakeLegacyModels`, `scriptLegado` e `scriptSoLegado`

Existem apenas em `*_test.go`, porém são cobertura, não produção retida artificialmente. Removê-las enfraqueceria a prova do contrato.

### P0 — política de tools: manter

`legacyAllPreloaded` é alcançado por `ResolveEffectiveToolPolicy`, chamado pelo envio e pelo builder de prompt com configuração derivada do perfil. Preserva `EnabledTools == nil` quando o registry não possui `tool_catalog`. O AEP-0081 D3 exige a semântica de perfis legados. Remoção dependeria de migração obrigatória e garantia do catálogo até em degradação de banco; nenhuma foi estabelecida.

### P1 — migrações, importadores e limpeza: manter

- `database.AdoptLegacyData` é chamado no login e protege dados órfãos anteriores ao multiusuário;
- imports de channels, jobs, MCP e portability são chamados no fluxo pós-login;
- cleanup de channels é usado pela página de restauração, com dry-run e confirmação;
- migrações de schema, credenciais e jobs atendem bancos existentes e incidentes documentados.

Idade da migração ou abundância de fixtures não prova que nenhuma instalação suportada ainda possa chegar nesse estado.

### P2 — avaliar antes de remover

- built-in skills legadas;
- agregador `portability.LegacyImportService`;
- normalização de prefixos de log;
- adapters anteriores de contexto e memória.

Evidência necessária: telemetria de execução, versionamento mínimo de origem ou política explícita de janela de compatibilidade. Sem isso, manter.

## Lote de remoção

Nenhum código de produção ou teste foi removido. O critério exige prova de ausência de entrada de produção e ausência de contrato de compatibilidade, migração alcançável ou teste que o proteja. Nenhum candidato prioritário passou pelos dois lados.

## Validações

- `go test ./internal/acp ./internal/chat ./internal/core/usecases ./internal/app`;
- `go build ./...`;
- `go vet ./...`;
- `go test ./...`;
- `staticcheck ./internal/acp`;
- `staticcheck ./internal/...`;
- `golangci-lint run --timeout 10m`.

Todas terminaram com sucesso; o lint reportou zero achados.

## Conclusão e follow-up

A aceitação da issue — relatório priorizado e remoções onde forem seguras — é atendida. O P2 deve virar trabalho novo somente após definir janela de compatibilidade ou obter telemetria; não bloqueia a conclusão desta auditoria.
