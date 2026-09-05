# Auditoria de cùdigo legado mantido por testes (Issue #664)

Branch: `chore/auditoria-legado`
Data: 2026-09-05
Base analisada: `origin/main` em `a909281c`

## Resultado executivo

O primeiro levantamento confundiu compatibilidade de protocolo exercitada por
testes com cùdigo mantido **apenas** por testes. A anùlise de chamadas, do
protocolo e dos AEPs corrigiu essa classificaùùo:

- nùo hù lote de produùùo ACP inequivocamente seguro para remover;
- `legacyModelState` e `session/set_model` sùo entradas de produùùo para agentes
  que anunciam modelos somente no formato anterior ao `configOptions`;
- `fakeLegacyModels`, `scriptLegado` e `scriptSoLegado` vivem apenas em
  `*_test.go`, mas sùo fixtures que provam o contrato de compatibilidade;
- `legacyAllPreloaded` preserva a semùntica de perfis legados sem
  `enabled_tools` estruturado e tambùm nùo pode ser removido nesta auditoria.

Portanto, o lote seguro de remoùùo tem **zero linhas**. Remover qualquer um dos
itens prioritùrios originalmente propostos quebraria uma compatibilidade
documentada ou apagaria sua cobertura. Este resultado nùo ù ausùncia de aùùo:
ele impede uma remoùùo incorreta sugerida pelo relatùrio preliminar.

## Metodologia

### 1. Inventùrio lexical separado por tipo de arquivo

Marcadores pesquisados, sem distinùùo entre maiùsculas e minùsculas:
`legacy|compat|deprecated|Postel`.

Comandos executados na raiz:

```powershell
rg -c -i 'legacy|compat|deprecated|Postel' internal -g '*.go' -g '!*_test.go'
rg -c -i 'legacy|compat|deprecated|Postel' internal -g '*_test.go'
```

Para a mùtrica de densidade, o denominador ù a soma de linhas retornada por
`rg -c '^'` nos mesmos conjuntos. A mùtrica conta **linhas com marcadores**, e
nùo linhas executùveis de ramos inteiros. Ela ù um proxy reproduzùvel para
priorizaùùo, nùo cobertura nem prova de cùdigo morto.

### 2. Anùlise de chamadas

Para cada candidato prioritùrio, foram rastreados:

1. declaraùùo e referùncias em produùùo;
2. referùncias em testes;
3. entrada dinùmica externa (JSON-RPC, arquivo persistido ou migraùùo), que nùo
   aparece como chamada Go convencional;
4. inicializaùùo e wiring que tornam o caminho alcanùùvel;
5. decisùo arquitetural aplicùvel.

O ACP exige atenùùo especial ao item 3: `legacyModelState` nùo ù chamado; ele ù
preenchido pela desserializaùùo de respostas externas de `session/new` e
`session/load`.

### 3. Anùlise estùtica

```powershell
staticcheck ./internal/acp
staticcheck ./internal/...
```

## Validaùùes

- `go test ./internal/acp ./internal/chat ./internal/core/usecases ./internal/app`;
- `go build ./...`;
- `go vet ./...`;
- `go test ./...`;
- `staticcheck ./internal/acp`;
- `staticcheck ./internal/...`;
- `golangci-lint run --timeout 10m`.

Todas terminaram com sucesso e o lint reportou zero achados.

Ambos terminaram sem achados. Isso exclui candidatos nùo exportados detectùveis
como nùo usados pelo analisador; nùo prova, isoladamente, que uma API exportada
ou entrada dinùmica esteja ativa.

### 4. Testes de caracterizaùùo

Os testes ACP usam um processo falso que fala JSON-RPC e verificam o efeito no
turno seguinte, nùo somente estado interno. Assim, eles comprovam que:

- o formato anterior ù lido na abertura e na retomada;
- `session/set_model` ù usado somente apùs `-32601` no seletor estùvel;
- uma recusa diferente de ùmùtodo nùo encontradoù nùo aciona fallback;
- a troca realmente altera o modelo observado no turno seguinte.

### 5. Decisùes arquiteturais consultadas

- `aep/0084-agentes-acp-como-providers.md`, D2, D6 e Fases 4/9;
- `aep/0086-registro-acp-descoberta-e-instalacao-de-agentes.md`, D1 e D11;
- `aep/0077-tool-planner-and-tools-subsystem-evolution.md`, D3;
- `aep/0081-politica-tools-por-perfil-e-carregamento-sob-demanda.md`, D3;
- `aep/0088-strangler-fig-borda-wails-app.md`, inventùrio final da borda.

## Mùtricas reproduzùveis

| Conjunto | Arquivos Go nùo vazios | Linhas | Arquivos com marcador | Linhas com marcador | Densidade |
|---|---:|---:|---:|---:|---:|
| Produùùo (`!*_test.go`) | 517 | 133.499 | 102 | 450 | 0,3371% |
| Testes (`*_test.go`) | 457 | 140.347 | 79 | 550 | 0,3919% |

No ACP, hù 20 linhas com marcador em produùùo:

- `internal/acp/session.go`: 10;
- `internal/acp/mapping.go`: 8;
- `internal/acp/client.go`: 2.

E 14 em testes:

- `internal/acp/mapping_test.go`: 8;
- `internal/acp/fakeagent_test.go`: 5;
- `internal/acp/session_options_test.go`: 1.

As 20 linhas de produùùo ACP nùo representam 20 linhas removùveis: incluem
comentùrios explicativos e pontos do mesmo contrato de compatibilidade.

### Maiores concentraùùes em produùùo

| Arquivo | Linhas com marcador | Classificaùùo |
|---|---:|---|
| `internal/channels/legacy_cleanup.go` | 32 | manter |
| `internal/channels/legacy_import.go` | 25 | manter |
| `internal/app/app_legacy_imports.go` | 24 | manter |
| `internal/wailsapi/legacy_cleanup.go` | 21 | manter |
| `internal/database/schema_migrations.go` | 20 | manter |
| `internal/portability/legacy_import_service.go` | 19 | avaliar com telemetria |
| `internal/app/builtin_skills.go` | 14 | avaliar com telemetria |
| `internal/portability/legacy_import.go` | 13 | manter enquanto houver importadores |
| `internal/jobs/migration.go` | 13 | manter |
| `internal/llm/provider.go` | 12 | manter |

### Maiores concentraùùes em testes

| Arquivo | Linhas com marcador |
|---|---:|
| `internal/jobs/migration_slug_test.go` | 51 |
| `internal/credentials/reencrypt_legacy_test.go` | 47 |
| `internal/app/builtin_skills_test.go` | 33 |
| `internal/portability/service_test.go` | 32 |
| `internal/channels/legacy_cleanup_test.go` | 19 |

Concentraùùo em teste nùo significa que o cùdigo correspondente sù exista para
o teste. Migraùùes e parsers de formatos anteriores naturalmente tùm mais
fixtures histùricas do que chamadas estùticas.

## Inventùrio priorizado e decisùo

### P0 ù ACP: manter

#### `legacyModelState`, `legacyModel` e `withModelOption`

**Entrada de produùùo:** `newSessionResponse.Models` e
`loadSessionResponse.Models`, desserializados em `openSession` e `LoadSession`.

**Justificativa:** o SDK nùo tipa o campo `models` anterior, mas agentes podem
enviù-lo no wire. O AEP-0084, Fase 9, registra o GitHub Copilot CLI como caso
observado que anuncia modelos somente nesse formato. Remover deixaria a lista de
modelos vazia e impediria a troca.

#### `legacySetModelMethod`, `setLegacyOption` e `legacySelector`

**Entrada de produùùo:** `SetConfigOption` tenta o seletor anterior somente
quando `session/set_config_option` retorna JSON-RPC `-32601`.

**Justificativa:** fallback explùcito do AEP-0084 D6. Outros erros continuam
subindo, evitando mascarar uma recusa real. O teste confirma o modelo efetivo no
turno seguinte.

#### `fakeLegacyModels`, `scriptLegado` e `scriptSoLegado`

**Localizaùùo:** apenas `*_test.go`.

**Decisùo:** manter. Sùo cùdigo de teste, nùo cùdigo de produùùo indevidamente
retido. Removù-los reduziria a cobertura do contrato acima.

### P0 ù polùtica de tools: manter

#### `legacyAllPreloaded`

**Entrada de produùùo:** `ResolveEffectiveToolPolicy`, chamado pelo pipeline de
envio e pelo builder de prompt com `ProfileToolConfig` derivado do perfil.

**Justificativa:** preserva o comportamento quando `EnabledTools == nil` e o
registry nùo possui `tool_catalog`. Alùm da entrada estùtica, o AEP-0081 D3
exige migraùùo semùntica determinùstica para perfis legados. Remoùùo dependeria
de uma migraùùo obrigatùria e de uma invariante que garanta o catùlogo em todo
runtime, inclusive degradaùùo de banco; nenhuma das duas foi estabelecida.

### P1 ù migraùùes, importadores e limpeza: manter

- `database.AdoptLegacyData` ù chamado no login e protege dados ùrfùos do modelo
  anterior ao multiusuùrio.
- `channels/legacy_import.go`, `jobs/migration.go`, `mcp/migration.go` e
  `portability/legacy_import*` sùo chamados no fluxo pùs-login.
- `channels/legacy_cleanup.go` ù exposto na pùgina de restauraùùo de padrùes
  como limpeza explùcita com dry-run e confirmaùùo.
- `database/schema_migrations.go` e migraùùes de credenciais/jobs sùo executadas
  sobre bancos existentes e cobrem incidentes reais documentados.

Excluir esses caminhos exige evidùncia de que nenhuma instalaùùo suportada pode
mais chegar com o formato anterior. Idade da migraùùo ou alta cobertura por
fixtures nùo fornece essa evidùncia.

### P2 ù avaliar antes de remover

- built-in skills legadas;
- serviùo agregador `portability.LegacyImportService`;
- normalizaùùo de prefixos de log;
- adapters de contexto e memùria anteriores.

Prùxima evidùncia necessùria: telemetria de execuùùo/versionamento mùnimo de
origem ou uma polùtica explùcita de janela de compatibilidade. Sem isso, manter.

## Lote de remoùùo

Nenhum cùdigo de produùùo ou teste foi removido.

Critùrio aplicado: sù remover quando a anùlise estùtica e de chamadas provar
ausùncia de entrada de produùùo **e** o item nùo for compatibilidade exigida,
migraùùo ainda alcanùùvel nem teste que blinda esses contratos. Nenhum candidato
prioritùrio passou pelos dois lados do critùrio.

## Conclusùo e follow-up

A aceitaùùo da issue ù relatùrio priorizado e remoùùes onde forem seguras ù ù
atendida por este relatùrio. O follow-up P2 deve ser aberto como trabalho novo
somente apùs definir a polùtica de janela de compatibilidade ou obter telemetria;
nùo deve bloquear a conclusùo desta auditoria.

