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


### Sincronização do polling do Deck

A revalidação do PR #1045 no run `42515424910`, com árvore idêntica à do run
aprovado `42513880227`, expôs uma corrida em
`TestCommandDeckInputRefreshesEquivalentJobProjectionAfterWorkspaceTabSwitch`.
Esvaziar `statuses` não impedia uma publicação `connected` de uma iteração
que já havia atualizado a projeção antes da troca de aba. O teste confundia
esse evento com a conclusão do polling posterior à troca.

A fixture agora pausa uma publicação real no emitter, troca a aba e comprova
que o guard ficou obsoleto enquanto esse ciclo está suspenso. Descarta somente
a publicação interceptada e exige a publicação do ciclo seguinte, pelo ticker
real. Preserva a primeira tecla, a segunda tecla, a recusa do guard obsoleto,
a continuidade do handle e a ausência de invalidação do mapa de teclado.
A barreira é liberada também em falhas, antes do encerramento dos workers.
Não há aumento de timeout, repetição da assertion até passar ou mudança produtiva.

## Distribuição e cache dos demais pacotes com race

Depois de reduzir a preparação das fixtures, `internal/database` continuou
no caminho crítico do grupo geral. Nos runs 42505601948, 42510063504 e
42513880227, começou respectivamente **5min45s, 3min51s e 4min57s** após o
início do passo de testes. Esse intervalo inclui compilação e escalonamento
dos pacotes, não apenas execução de testes. O job WailsAPI terminou em cerca
de três minutos nos dois últimos runs e ficou sem trabalho durante a espera
final do workflow.

O grupo `pacotes-database-wailsapi` executa os dois pacotes inteiros, colocando
o banco no começo de uma lista curta. O grupo geral exclui exatamente esses
pacotes, App e Chat; seus subpacotes continuam na descoberta automática.
O inventário recusa ausência/duplicação dos pacotes isolados e lista geral
vazia. Nenhum teste de migração/upgrade, flag de race, prazo ou gate foi removido.
Permanecem **17 jobs** e até **oito executores de testes com race**.

Os executores de race passam a usar o mesmo namespace instrumentado do
produtor App: `go.sum` e o script de race compõem `cache-dependency-path`.
Antes, os demais grupos restauravam a chave dos builds normais, que não
garantia objetos compilados com `-race`. A validação do conteúdo/flags continua
sob responsabilidade do cache do Go; não se cacheiam resultados dos testes
(`-count=1` permanece) nem se reutiliza binário de outra revisão. O artefato
App segue restrito ao run e SHA que o produziram.

O cache da action é imutável por chave: em cache frio, o primeiro job que o
salvar define seu conteúdo. Esse job pode ser Chat/WailsAPI, não necessariamente
o produtor App. Um hit pode, portanto, exigir compilação de objetos ausentes;
não garante que todo o App ou todos os demais pacotes já estejam no cache.
Isso não muda a validação de conteúdo do Go, mas precisa entrar na comparação
de desempenho, inclusive no tempo do produtor App da segunda execução.

Uma mudança no script muda a chave: o primeiro run pode precisar recompilar.
A qualificação deve registrar hits/misses e separar a primeira execução da
repetição com cache restaurado, sem atribuir toda variação do workflow a esta
mudança. Os resultados Linux/race serão registrados no PR e na #1008. Os
contratos Bash verificam a matriz, os pacotes completos, a união disjunta,
subpacotes, flags, falhas e o gate; não executam Go real nem ACP local.


## Disparos concorrentes ao atualizar PRs empilhados

Após o merge do #1043, o #1044 recebeu os runs `42515424147`
(`pull_request`) e `42515419837` (`workflow_dispatch`) para o mesmo head
`4d2648867e5cfed342455fc973ea1c2d998c9bae`. Os dois workflows iniciaram
jobs. A chave anterior usava número do PR no primeiro caso e nome da branch
no segundo: o controle de concorrência não reconhecia a duplicação.

O atualizador passa `pr_number` no dispatch. O CI usa namespaces distintos para
PR, dispatch automático e execução independente: um dispatch não pode cancelar
nem substituir um PR ativo ou pendente. Os checks do PR pertencem ao commit de
merge, e um dispatch no head da branch não os substitui. O primeiro ensaio
(`42524102553` / `42524126708`) expôs esse problema ao compartilhar o grupo;
a solução foi corrigida antes de aceitar a qualificação. Mesmo cancel-in-progress
falso não protegeria o único slot pendente de uma fila compartilhada.

Ao começar, o dispatch consulta CI do mesmo workflow, PR e head no job `scripts`.
Se o PR já tem execução efetiva, solicita cancelamento do próprio dispatch.
Falha/cancelamento do PR não é substituído por verde manual. Um evento
`action_required` sem jobs, já observado no atualizador, não conta como execução:
o dispatch continua necessário para validar esse head. Erros da API ficam
visíveis. A permissão `actions:write` fica somente em `scripts`, que cancela o
próprio run, sem token novo.

Isso elimina o desperdício observado quando o CI do PR já existe. Se o PR
surgir depois da consulta, ambos podem executar: é uma janela residual de custo,
não de perda de validação. Não há polling ou orquestração bidirecional. Os demais
jobs continuam paralelos e podem consumir alguns segundos de setup antes do
cancelamento; não há novo job nem barreira global. Os dois registros de run
continuam existindo. Matriz, gates, testes, flags e limites permanecem.
PRs distintos, inclusive forks com nomes de branch iguais, são separados por
número; cada atualização continua exigindo validação.

Push por `GITHUB_TOKEN` não garante novo CI, por isso o dispatch permanece.
Execuções manuais independentes deixam o input no padrão `0`, preservando a
política anterior, assim como pushes na main. O identificador só usa o input
quando positivo, com o mesmo predicado do namespace: o sentinela zero, mesmo
recebido como texto, não agrupa branches independentes. O atualizador exclui forks,
drafts e PRs fora da janela de manutenção.

Contratos Bash verificam associação branch→PR, namespaces, identidade exata,
ausência de execução, aprovação sem jobs e falhas de consulta/cancelamento.
Actionlint valida contextos/tipos. A qualificação remota deve comprovar que o
run do PR permanece verde e apenas o dispatch redundante é cancelado.
Referências: [concorrência de workflows](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#concurrency)
e [eventos e workflow_dispatch](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#workflow_dispatch).


## Preparo compartilhado de comandos manuais do Deck

`deckConfiguredFixture` e `deckChatPickerFixture` repetiam a mesma sequência
confirmada de criação de camada, regra manual e binding. Agora compartilham o
preparo persistido de configuração e a projeção real já usados pelas fixtures
contextuais. `SetCommandLayerActive` permanece no caminho produtivo: a fixture
não instala claims, ofertas, ocorrências nem invocações prontas. Mutações feitas
pelos cenários e as provas específicas de edição confirmada não mudaram.

A seleção antes/depois passou com os mesmos **20 testes top-level: 19 pass e
1 skip opt-in**. A prova de status vazio também usa `deckConfiguredFixture`
e está incluída no preparo alterado. A soma dos tempos top-level foi
**20,25 → 11,87 s** em uma execução de cada binário. O processo completo ficou em **48,935 / 49,212 s** nessas
primeiras execuções; a redução interna não representou ganho de abertura.
O teste opt-in `TestCommandDeckAppLatency` foi executado separadamente com
`COMMAND_APP_LATENCY=1` e passou nos dois cenários.

Para distinguir preparo de inicialização do binário, três pares posteriores
em processos independentes e sem compilação/suítes concorrentes mediram
`TestCommandLocalFamiliesDeckProjectionAndSingleDispatch`, com os mesmos
16 comandos e assertions de origem, projeção, repetição e dispatch único:
antes **6,352 / 6,584 / 7,010 s**; depois **2,031 / 2,137 / 2,114 s**.
Mediana **6,584 → 2,114 s (−67,9%)**, Windows/Go 1.26.0. Não extrapolar esse
percentual ao workflow: ganho Linux/race ainda depende da qualificação remota.


## Investigação de intermitência de foco no E2E

O run `42517346349` passou, mas registrou retry em
`command-managers.spec.ts:86`: depois de sair do editor e navegar na toolbar,
o botão esperado não tinha foco. Vinte repetições locais sem retry passaram
(1,6 min), portanto a causa daquele episódio **não foi reproduzida**.
Uma cópia diagnóstica, removida antes do commit, registrou chamadas independentes
de restauração por Menu, MenuButton e Modal; isso não demonstra qual causou
a falha remota e não autoriza cortar assertions ou elevar timeouts.

A investigação encontrou uma janela demonstrável no Modal: sua restauração
adiada sobrescrevia uma nova aquisição de foco feita após fechar o diálogo.
Uma regressão com frames controlados falhou antes no caso de nova navegação,
e o caso de retorno padrão passou. O callback agora preserva um alvo conectado
adquirido depois do agendamento; se o foco não mudou ou voltou ao documento,
o retorno padrão continua. A guarda de outro modal aberto permanece.

Os dois casos de regressão e os vinte testes anteriores do Modal passam, com
os spies de frames restaurados ao final. Esta correção não declara resolvido
o E2E original; sua eventual recorrência precisa de nova evidência. Não houve
mudança em retries, timeouts, seleção de testes ou uso de NVDA.


## Reuso do schema produtivo nos testes de portabilidade

`setupPortabilityRuntimeTestDB` usa `dbfixture.Open`: migra o schema produtivo
uma vez por processo e abre uma cópia privada por teste. Cada cenário continua
criando seu usuário, histórico e anexos, com conexão e cleanup próprios. Não
reutiliza resultados de importação nem altera a fixture parcial dos importadores
legados ou os testes de migração. Validações de grafo, rollback, colisões,
seleção de execução, continuidade e round-trip de áudio/mídia permanecem.

A motivação remota é o run `42518594812`: portabilidade levou **328,912 s** no
grupo geral com race; o teste de grafo inválido e rollback consumiu **76,29 s**.
Isso identifica custo, não constitui comparação Linux antes/depois.

Windows/Go 1.26.0, binários previamente compilados e processos independentes:
a seleção permitida localmente passou antes/depois com os mesmos **11 testes
principais / 21 casos incluindo subtestes**. A soma dos tempos dos testes
principais caiu de **12,02 para 1,43 s** em uma execução; os processos completos
levaram **32,200 / 16,306 s**, incluindo sua inicialização.

Três pares isolados, sem outras suítes/compilações concorrentes, executaram
`TestExecutionHistoryRejectsInvalidGraphAndRollsBack` (oito grafos inválidos):
antes **4,214 / 4,091 / 4,208 s**; depois **1,085 / 0,969 / 0,985 s**.
Mediana **4,208 → 0,985 s (−76,6%)**, incluindo o processo inteiro. Não
extrapolar o ganho ao workflow. Testes ACP e funções de arquivos que mencionam
ACP não foram executados localmente; permanecem no CI integral.

### Contenção real no cadastro de identidades externas

A revalidação `42519431471` falhou em
`TestExternalIdentityBootstrapConcurrentConnectionsHasSingleWinner`: um writer
venceu e o outro recebeu `SQLITE_BUSY`, em vez do conflito de unicidade.
O DSN produtivo espera 100 ms e depende da política central de retry, ausente
nesse caminho. A regressão nova força um writer real e só o libera depois de
observar BUSY; falhou antes tanto no bootstrap quanto no cadastro administrativo.

Ambos usam agora `WithSQLiteImmediateTransactionOnce`: a política central
retenta somente a aquisição do writer; corpo e COMMIT não são repetidos.
Auditoria permanece a primeira escrita e vínculo/auditoria continuam atômicos.
O teste concorrente original foi preservado; não houve aumento de timeout,
retry no teste nem redução de concorrência. Isso corrige comportamento produtivo,
não apenas a estabilidade do CI.

## Ambientes Vitest por dependência de DOM — segundo lote (#1008)

52 arquivos de lógica executam em Node, sem carregar jsdom, React e os setups
de UI/axe. Os demais continuam em jsdom com a configuração existente. A lista
é explícita: arquivos novos entram no projeto UI por padrão; a migração exige
qualificar suas dependências. Nenhuma assertion, cenário ou código produtivo
foi alterado. Testes de teclado com KeyboardEvent e conversão de Blob continuam
no projeto UI. Hooks/componentes renderizados e acessibilidade também permanecem.

A seleção foi inspecionada por imports e uso de APIs de navegador, inclusive
nas dependências locais transitivas, e executada sem mocks globais de UI.
Alguns arquivos de contrato leem fontes ou validam DTOs sem renderizar, mesmo
quando seu nome menciona foco, abas ou região viva. Isso não migra as provas
que exercitam a interface correspondente.

Três pares locais, em processos independentes, Windows, quatro workers, com os
mesmos 52 arquivos e 366 testes aprovados em cada execução:

- Antes: 21,995 / 22,288 / 23,749 s.
- Depois: 6,891 / 7,692 / 7,743 s.
- Mediana: 22,288 → 7,692 s (−65,5%), incluindo inicialização do processo.

O inventário de casos/resultados foi comparado entre cada par. O ganho é desta
seleção; não é uma redução equivalente prometida para o workflow completo.
`npm test` continua executando os dois projetos no mesmo processo Vitest.


## Preparo proporcional das provas de catálogo

Sete testes de contrato wire, publicação do mapa e recuperação direta do
catálogo passam a usar o produto real com HOME/configdir isolados, sem criar
jobs/manutenção nem reiniciar previamente o App. SQLite, autenticação,
workspace, bootstrap, executor, bridge e as operações sob teste permanecem
reais. O helper reutiliza `readyCommandProduct`; não introduz mocks de domínio.

Mapa das garantias:

- `TestCommandFrontendKeyboardWireContract` continua comparando o JSON real
  com a fixture consumida pelo frontend.
- As duas provas de readiness continuam verificando a notificação após
  bootstrap e a recusa de mapa quando o bootstrap é cancelado.
- As quatro provas diretas de recovery preservam produto antigo fechado,
  falha/retry de montagem, join de lifecycle parado e invalidação de ticket
  por troca de workspace/logout.
- Worker/singleflight, regeneração de jobs, falha de storage, projeção,
  manutenção e restart conservam suas fixtures completas. Não se substituiu
  uma integração exercitada por essas provas por um stub.

Três pares locais com binários previamente compilados, processos independentes,
Windows/Go 1.26.0, sem outras suítes/compilações locais concorrentes: antes
4,401 / 1,986 / 1,859 s; depois 1,220 / 0,876 / 0,866 s. Mediana
1,986 → 0,876 s (−55,9%). Os mesmos sete testes passaram em todos os pares;
não houve alteração de assertions, prazo ou política produtiva. São tempos
da seleção, sem extrapolação ao CI ou ao pacote completo.


## Prazo da prévia sem espera duplicada na borda

`TestWailsProviderPreviewPreservesCommandBudget` percorre a borda Wails,
controller, serviço, comando real de credencial e HTTP local. Observa o
contexto da requisição no callback Proxy de um `http.Transport` clonado,
retornando nil para continuar a conexão direta. Não simula o serviço nem a
resposta do transporte. A autorização e o modelo retornado continuam verificados.

Os três casos verificam orçamento padrão de 60 s, configurado de 120 s e
preservação exata de prazo pai menor (20 s). A margem de 10 s na observação
dos dois primeiros acomoda execução do helper; não é espera nem alteração
no timeout produtivo. A contraprova, via overlay não versionado que recoloca
15 s no bind, falhou nas três assertions. Nenhum código produtivo foi alterado.

O teste `TestProviderCredentialPreviewAllowsSlowCommand`, no serviço, mantém
o comando real de 16 s. A borda deixa de repetir a prova temporal e passa a
cobrir diretamente seu contrato de encaminhamento; não se remove a garantia
nem se enfraquece uma assertion para esconder falha.

Três pares de processos Windows/Go 1.26.0: antes 45,094 / 16,368 / 16,295 s;
depois 26,549 / 0,496 / 0,673 s. A primeira execução de ambos teve custo extra
de inicialização não atribuído. Medianas 16,368 → 0,673 s; os casos ampliam de
um cenário para três, todos aprovados. Havia outra suíte backend local ativa;
esses valores não são benchmark isolado nem previsão de duração global do CI.
O ganho determinístico é deixar de repetir uma espera artificial de 16 s.


## Schema compartilhado nas provas de envio e áudio

Três preparações passam a usar `dbfixture.Open`: casos de uso de envio/retry,
persistência de áudio coordenada com a timeline e três integrações de áudio.
Somente os bytes do schema vazio são compartilhados por processo; banco,
conexão, usuário, conversa, mídia e callbacks continuam privados de cada caso.
O helper existente fecha a conexão capturada e restaura o singleton no cleanup.
Não há nova infraestrutura de teste ou cache de resultados.

Permanecem as provas de cancelamento/STT único, preservação em falha de MCP,
retry sem duplicar a mensagem, validações de prefill/provedor, recusa antes de
escrever quando falta migração, coordenação de revisão de áudio sem ler payload
e persistência de áudio/mídia nos canais. Bootstrap e testes de migração seguem
usando suas entradas reais. Nenhuma assertion foi alterada.

Três pares isolados por seleção, sem outras suítes ou compilações locais,
Windows/Go 1.26.0, executáveis previamente compilados:

- Envio: antes 12,647 / 11,332 / 12,502 s; depois 3,544 / 2,572 / 2,400 s.
  Mediana 12,502 → 2,572 s (−79,4%); mesmos 27 casos, incluindo subtestes.
- Timeline de áudio: antes 2,396 / 1,705 / 1,971 s; depois 1,731 / 0,921 / 0,935 s.
  Mediana 1,971 → 0,935 s (−52,6%); mesmos 4 casos, incluindo subtestes.
- Integração de áudio: antes 2,052 / 1,480 / 1,516 s; depois 1,638 / 0,683 / 0,817 s.
  Mediana 1,516 → 0,817 s (−46,1%); mesmos 3 casos, incluindo subtestes.

Os 34 nomes/resultados foram comparados em cada par, todos aprovados. Esta
medição substitui a qualificação inicial sob carga concorrente; não é previsão
de ganho no CI completo. A seleção inclui o fallback de provedor com retry
intermitente já registrado na issue; esta mudança de fixture não corrige
aquele problema produtivo.
