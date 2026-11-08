# Custo de testes e CI — issue #1008

## Baseline e escopo

O [run 42487226598](https://github.com/inclunet/assistente/actions/runs/42487226598)
do PR #1040 teve 28 jobs, 23,10 minutos de duração total e 158,17 minutos
somados de execução dos jobs. Esta soma não é faturamento. Dos 28 jobs,
20 executaram partições de race; seus passos de testes somaram 113,40 minutos,
incluindo descoberta/compilação. Não se atribui todo esse tempo aos testes.
O grupo `comandos-interface` executou 56 testes top-level em 594,313 segundos
no Go; começou 10,20 minutos depois da criação do run. A demora de início
inclui o escalonamento da matriz, não prova indisponibilidade de runners.

As contagens e tempos vêm de `gh api repos/inclunet/assistente/actions/runs/42487226598/jobs?per_page=100`
e dos logs dos jobs. Duração total: último `completed_at` menos `created_at`
do run. Minutos agregados: soma de `completed_at - started_at` dos jobs.
Reexecuções devem ser separadas por tentativa; não misturar sua espera com
o tempo de uma tentativa inicial.

## Primeiro recorte: preparação da execução contextual

O perfil local de `TestCommandEditorFormatFamilyContextualDeckUsesCommonHandoff`
mostrou repetição de `MutateCommandSettings`, projeção do catálogo e
serialização/fingerprints. As quatro mutações confirmadas eram preparação
repetida para cada comando, não o comportamento sob teste.

A fixture agora insere camada, regra `always` e dois bindings disjuntos numa
transação do banco exclusivo do cenário. Incrementa a geração e chama a
reconstrução real do lifecycle: leitura, validação, projeção e publicação
continuam no percurso. App, banco, sessão, workspace e limpeza continuam
independentes por cenário. Não há cache no produto ou estado global novo.

### Garantias preservadas

- `TestCommandEditorFormatFamilyContextualDeckUsesCommonHandoff`: todos os IDs
  anteriores percorrem oferta física → Begin → Take → Complete; recusa de replay
  da oferta e do handoff; ausência de invocação antecipada; exatamente uma
  invocação persistida por comando e origem `streamdeck.key`.
- Outros usuários de `newContextualDeckTestFixture`: recusas de contexto,
  invalidação, cancelamento, expiração e limpeza continuam com as mesmas
  assertions. Nenhuma condição de sucesso foi relaxada.
- `TestCommandDeckVisualConditionsFromSettingsSelectDistinctCommands`,
  `TestContextualPagePaletteConditionsRealSettings` e
  `TestCommandSettingsManualActivationChangesExecutedBinding` mantêm, em
  conjunto, provas de edição confirmada, publicação/projeção e execução.
  A preparação de outras fixtures
  foi ampliada no recorte contextual descrito adiante.
- `TestCommandSettingsContractContextualRuleActuallyResolves` e a suíte de
  segurança de settings mantêm confirmação, CAS e reautenticação.
- Frontend: `npm run build` continua executando `tsc && vite build`; foi removida
  apenas a execução anterior idêntica de `tsc --noEmit` no mesmo job. O
  `tsconfig.json` já define `noEmit`. O typecheck de bindings recém-gerados
  permanece em seu job, pois verifica outra entrada. O build fica logo após
  a instalação, preservando a recusa de erros de tipos antes da suíte Vitest.

### Comparação reproduzível

Compilar o pacote `internal/app` nas duas revisões com o mesmo Go e executar
os binários a partir de `internal/app`, alternando baseline e alteração três
vezes, com os argumentos:

```text
-test.run=^TestCommandEditorFormatFamilyContextualDeckUsesCommonHandoff$ -test.v -test.timeout=5m
```

Seguir AGENTS.md: binários, caches e temporários sob `build/bin/` do worktree,
com GOTMPDIR/GOCACHE/TEMP/TMP/TMPDIR configurados antes da compilação.
Não executar ACP localmente. Separar esta medição de compilação e de outras
suítes concorrentes. O baseline é `55de67f0e` (conteúdo de App idêntico ao
binário de `f327e03e6`). Medições locais usam Windows e Go 1.27.1; não representam
uma previsão de tempo do runner Linux com race/Go 1.26.

Três pares locais concluídos com sucesso em 08/11/2026, sem compilação ou
outra suíte iniciada pelo agente concorrendo com a medição:

- Par 1: baseline 37,38 s; alteração 9,10 s.
- Par 2: baseline 41,70 s; alteração 9,48 s.
- Par 3: baseline 34,20 s; alteração 8,09 s.

Medianas: **37,38 s → 9,10 s**, redução de **75,7%** neste teste. Mesmos
subtestes e assertions, três execuções aprovadas em cada versão. Um ensaio
anterior concorreu com instalação/compilação e não integra esta comparação.
O ganho do workflow completo precisa ser medido no CI; não extrapolar esta
porcentagem para a suíte nem para race.

## Próximas frentes da issue

A issue permanece aberta: verificar cache útil/atualização da compilação,
reaproveitamento entre partições, distribuição da matriz, esperas reais,
ambiente DOM em testes puros, seleção por impacto e cancelamento/diagnóstico.
Cache já é restaurado pelos jobs de Go medidos: habilitá-lo novamente não
explica nem resolve o custo de execução. Não reduzir grupos antes de medir
o efeito sobre o caminho crítico, nem usar resultados antigos como aprovação.

## Compilação compartilhada e grupos uniformes de race

O run [42495213574](https://github.com/inclunet/assistente/actions/runs/42495213574),
após o primeiro recorte, levou 20m27s e somou 161,33 runner-minutos em 28 jobs.
O teste contextual caiu de 32,73 s para 6,45 s, preservando seus 45 casos.
Isso não foi suficiente para reduzir o custo agregado: a referência anterior
somava 158,17 runner-minutos. O cache de Go foi restaurado nos dois runs.

O segundo recorte substitui os 16 grupos do App por quatro partições estáveis
por nome, com o mesmo critério já usado nas duas partições do Chat. A descoberta
inclui todos os testes, exemplos e fuzz seeds; subtestes permanecem juntos.
Não há lista de famílias ou exceções a manter. Com os tempos individuais dos
1.408 testes aprovados do App no run de referência, as quatro somas são
141,83 / 168,99 / 116,71 / 110,19 s. São tempos sem race, usados apenas para
avaliar distribuição: não estimam a duração com detector de corrida e não
incluem o teste que teve skip. A seleção efetiva usa o inventário completo do
binário, incluindo testes que podem decidir pular durante a execução.

Um produtor compila `internal/app` uma vez com `go test -race -c`. Os quatro
consumidores recebem esse binário por artifact da mesma execução e revisão,
sem reutilizar resultados de testes. A execução parte de `internal/app`, como
`go test`, com `-test.short`, `-test.count=1` e timeout de 20 minutos. `test2json`
registra tempos por teste e propaga falhas. Ausência do artifact é erro; não há
fallback para binário antigo nem recompilação silenciosa nos consumidores.
Go permanece instalado, pois testes podem compilar subprocessos auxiliares.

Os outros quatro grupos iniciam independentemente do produtor. O máximo é
oito executores de testes simultâneos; cada matriz limita-se a quatro. O check
obrigatório `backend-race` exige sucesso do produtor e das duas matrizes,
inclusive quando uma dependência falha, é cancelada ou não executa. Os jobs
do workflow passam de 28 a 17. Cobertura normal, vet/lint, builds, bindings,
frontend e E2E continuam com seus gates próprios.

A validação local usa Go/binário simulados para comprovar partição integral,
disjunta e estável, flags, diretório de execução, tratamento de falhas e
correspondência com a matriz. Não executa ACP. A qualificação do binário Linux
com race e a medição do caminho crítico/custo agregado dependem do run do PR;
a redução de jobs não deve ser apresentada como ganho de tempo já medido.

Validação local do mecanismo: uma fixture Go isolada, compilada com Go 1.26.0
no Windows, comprovou `short`, diretório do pacote, saída JSON e código de
saída diferente de zero em falha intencional. Não usa App/ACP nem qualifica
race no Linux. ShellCheck 0.11.0 e actionlint 1.7.12 aprovaram os arquivos
alterados. Revisão independente do diff: `review_race_build`, rodada 1,
sem achados acionáveis; permanece necessária a qualificação remota acima.


O cache persistente de compilação do produtor e dos consumidores do App usa
`go.sum` e o script de build/seleção na chave do setup-go. Os paths já fazem
parte da versão interna do cache; o namespace adicional separa a compilação
instrumentada do cache imutável de `backend-checks`, que usa o mesmo GOCACHE,
mas compila sem race. O produtor salva antes de liberar os consumidores;
assim eles não criam primeiro uma entrada parcial. O artifact continua sendo
exclusivo do run: cache de objetos compilados não aprova teste nem substitui
execução. Evidência da separação necessária: no run 42498268593,
`backend-checks` restaurou 1.083.452.142 bytes no mesmo GOCACHE e registrou
primary-key hit, sem salvar. Reutilizar essa chave descartaria ao final o
novo trabalho instrumentado do produtor.


A primeira execução direta no Linux revelou uma corrida de preparação antes
escondida pela compilação: o binário abriu o display antes de Xvfb estar pronto.
Os dois runners de race usam agora `xvfb-run` e declaram a dependência `xauth`;
o wrapper aguarda o servidor e preserva o código de saída. Não se adiciona
sleep arbitrário nem se ignora falha de inicialização. A falha interrompeu a
descoberta e reprovou o job, como esperado; não foi falha de teste de produto.

## Imagem de schema para fixtures de Agent e Chat

O terceiro recorte da #1008 atua em `setupTurnRuntime` e `newSnapshotFixture`.
Um perfil local de `TestExecutionWriterChronologySurvivesTerminalAndPersistenceFailure`
atribuiu 7,82 s dos 10,12 s amostrados de CPU (77,27%) a `database.InitPath`.
O teste estava repetindo o bootstrap do banco durante a preparação dos cenários.

`internal/testutil/dbfixture` prepara uma imagem vazia pelo `InitPath` real uma
vez por processo e fecha suas conexões antes de ler o arquivo consolidado.
Cada fixture recebe uma cópia física privada dessa imagem e conexões novas.
O helper cria seu próprio diretório temporário e devolve o caminho absoluto;
não aceita destinos externos que possam conter outro banco ou WAL remanescente.
Os bytes não são expostos nem modificados; não se compartilham usuários,
conversas, transações ou conexões. A imagem não é versionada, salva em cache de
CI nem reaproveitada entre processos/revisões. Não há lista manual de models,
DDL alternativo ou migrações simuladas.

A configuração existente de conexões foi extraída para `database.OpenPath`,
utilizada tanto pelo `InitPath` de produção quanto pela abertura da cópia.
Pool, DSN, WAL, pragmas, logger e registro do caminho permanecem no mesmo código.
`InitPath` continua executando todas as migrações, AutoMigrate e inicialização
FTS no mesmo fluxo. A decomposição não cria flag de atalho para o aplicativo.
O helper depende de `testing` e só é importado pelas fixtures de teste.

Os testes de bootstrap e upgrades publicados continuam chamando `InitPath`
diretamente (AEP-0076). Os contratos dos AEPs 0074 e 0116 não mudam. Os testes
novos verificam schema completo, índices/triggers/FTS, versões de migração,
pragmas e pool contra o bootstrap real, além de isolamento de dados/DDL,
fechamento de conexões e restauração do banco e caminho anteriores. A ordem
textual das constraints nomeadas é normalizada na comparação porque GORM pode
emiti-las em ordens diferentes; o conteúdo SQL, incluindo CHECKs, é preservado.

### Medição local pareada

Windows, Go 1.26.0, sem race; binários antes/depois compilados previamente,
executados a partir do diretório do pacote em processos separados. Três pares,
sem compilações ou suítes concorrentes durante as medições aceitas:

- Agent (`TestExecutionWriterChronologySurvivesTerminalAndPersistenceFailure`):
  12,15 / 11,92 / 10,53 s antes; 2,08 / 2,04 / 1,76 s depois.
  Mediana **11,92 → 2,04 s** (82,9% de redução).
- Chat (`TestNormalizedAudioReachesSendAndRetryHistory`):
  5,29 / 5,72 / 6,30 s antes; 1,00 / 0,93 / 0,95 s depois.
  Mediana **5,72 → 0,95 s** (83,4% de redução).

Todos os casos e assertions originais permanecem. O terceiro par de Chat foi
repetido após terminar uma consulta ao inventário Go; o ensaio concorrente não
integra estes números. Nenhum ACP foi executado localmente. Não extrapolar as
porcentagens para o workflow: a qualificação Linux/race, a contagem da suíte e
o custo total serão registrados no PR e na issue a partir do CI correspondente.

## Fixtures de integração da timeline

O quarto recorte da #1008 aplica `dbfixture.Open` a nove preparações em seis
arquivos de WailsAPI, App e controllers. Esses cenários verificam autorização,
cancelamento, replay, identidade, edição e exclusão; não são testes de bootstrap
ou upgrade. Os dados, sessões, emissores e projetores continuam sendo criados
individualmente, depois da instalação do banco privado. Os cleanups de serviço
continuam executando antes do fechamento das conexões. Nenhum caso, assertion,
prazo de cancelamento ou caminho de produto mudou.

Fixtures de manutenção com `auto_vacuum=NONE`, reinicialização e migração
permanecem no caminho anterior. Não se substitui schema legado pelo schema
atual nem se transforma a suíte em compartilhamento de estado entre testes.
Os contratos dos AEPs 0074, 0076 e 0116 permanecem vigentes.

### Comparação local

Windows/Go 1.26.0, binários antes/depois compilados previamente; três pares em
processos separados, sem outra compilação ou suíte concorrente. Cada processo
novo inclui a preparação inicial da imagem. Os mesmos nomes de testes passaram
em todas as amostras, sem ACP local:

- WailsAPI: 23 testes de timeline, mediana do processo **17,979 → 1,444 s**
  (92,0%). Amostras antes: 20,058 / 17,511 / 17,979; depois:
  26,878 / 1,433 / 1,444 s.
- Controllers: `TestConversationDeletionCoordinatesAtomicBatchBeforeEffects`,
  com seus seis cenários, **2,824 → 0,757 s** (73,2%). Amostras antes:
  4,931 / 2,824 / 2,719; depois: 19,276 / 0,757 / 0,753 s.
- App: oito testes de sessão da timeline, **7,469 → 1,822 s** (75,6%).
  Amostras antes: 10,029 / 7,469 / 7,188; depois:
  37,360 / 1,822 / 1,727 s.

As primeiras execuções dos binários novos tiveram atraso fora do tempo
reportado pelos testes. Essas amostras foram mantidas, sem atribuir a causa a
um componente não medido. A soma das durações dos testes top-level na primeira
execução depois foi 1,38 / 0,70 / 1,64 s, respectivamente. As medianas dessas
somas nos três pares foram 17,59 → 1,37 s em WailsAPI, 2,76 → 0,70 s nos
controllers e 7,10 → 1,68 s no App. Essa soma não inclui inicialização/saída do
processo. O resultado Linux/race e o custo do workflow devem ser medidos no CI;
não extrapolar as porcentagens locais.

## Preparação de comandos contextuais

O quinto recorte da #1008 reúne a preparação de páginas e camadas de paleta e
Deck no helper de teste `contextualCommandConfigFixture`, também usado pela
fixture contextual física já otimizada. Cada cenário mantém seu App, banco,
sessão e dados privados. O helper normaliza regras e bindings com as funções
produtivas, grava a configuração global do usuário em uma transação, avança a
geração e chama o carregador/projetor real. Não instala claims, ofertas, tickets
ou resultados prontos e não acrescenta API nem opção de bypass em produção.

O perfil anterior mostrou `MutateCommandSettings` em 40,27% do tempo amostrado
de CPU em `TestContextualDeckPageRejectsInvalidOffers` e em 46,13% de
`TestContextualPaletteLayerRejectsInvalidAdmission`. Essas chamadas preparavam
o cenário por meio de três, oito ou treze edições confirmadas. A preparação
agora publica uma vez por configuração; a fixture física de camadas mantém
as duas publicações de seus dois estágios de montagem.

As operações e assertions dos cenários permanecem: ofertas físicas, contexto,
autorização, geração obsoleta, ABA, cancelamento, replay, publicação e ledger.
Excluir regra/binding durante o cenário continua usando a API confirmada.
As provas completas de edição permanecem em
`TestCommandDeckVisualConditionsFromSettingsSelectDistinctCommands`,
`TestCommandSettingsManualActivationChangesExecutedBinding`,
`TestContextualPagePaletteConditionsRealSettings` e nos testes de contrato e
segurança de settings. AEP-0103 e índice continuam **In Progress**, sem promover
aceites pendentes. Não há cache de App/conexão nem mudança no produto.

### Medições do preparo contextual

Windows/Go 1.26.0, binários compilados previamente, processos independentes e
sem outras suítes/compilações concorrentes durante a medição. Três pares
antes/depois, com os mesmos casos e assertions em todos os processos:

- `TestContextualDeckPageRejectsInvalidOffers` (11 cenários): antes
  12,504 / 7,762 / 12,063 s; depois 1,896 / 1,960 / 2,508 s.
  Mediana **12,063 → 1,960 s**, redução de **83,8%**.
- `TestContextualPaletteLayerRejectsInvalidAdmission` (10 cenários): antes
  19,526 / 24,892 / 22,283 s; depois 1,972 / 2,077 / 1,905 s.
  Mediana **22,283 → 1,972 s**, redução de **91,2%**.
- `TestContextualDeckLayerRejectsInvalidOffers` (9 cenários): antes
  26,264 / 38,263 / 29,856 s; depois 1,978 / 2,453 / 2,113 s.
  Mediana **29,856 → 2,113 s**, redução de **92,9%**.

O conjunto `TestContextual(Deck|Palette|Page)` também passou antes/depois com
os mesmos **53 testes top-level / 878 casos incluindo subtestes**. Em uma
execução de cada revisão, a soma dos tempos top-level foi **194,74 → 77,42 s**;
essa soma não inclui inicialização/saída do processo nem constitui mediana.
Os tempos pareados acima incluem o processo completo. Nenhum ACP local.
Medições Linux/race e efeito sobre o workflow dependem do CI; não extrapolar
os percentuais locais nem confundir redução de preparo com remoção de cobertura.
