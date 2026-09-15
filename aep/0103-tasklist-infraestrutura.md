# AEP-0103 — Tasklist de infraestrutura e entrega
Documento de acompanhamento, não nova AEP nem alteração dos contratos.
Baseline v1: 14/09/2026 • código examinado: `11c10c578051c7276b7345cd608d6460a3b1803c`.
Branch: `feat/aep-0103-comandos`. AEP principal continua **In Progress**.

## Continuação — 15/09/2026, renderer preparatório de Stream Deck

Avanço em **I13.5/C39/C40/C41/C42**, agora fechado após validação física.
`internal/commanddeck` introduz um renderer puro para dispositivos
tipo Stream Deck: valida modelo/geometria, mantém estado por dispositivo, emite
frame completo após abertura/reconexão, volta a diff incremental depois do
primeiro envio, atualiza somente teclas alteradas e cacheia hashes de imagem por
modelo/tamanho/conteúdo. O pacote também ganhou um `Manager` sem HID que impõe
posse exclusiva lógica, abre sempre em estado seguro, exige ativação por geração,
renderiza estado seguro em lock/logout, rejeita geração obsoleta e modela
reconexão com backoff. A borda `DeviceAdapter` recebe eventos normalizados do
futuro driver HID, recusa teclas enquanto o dispositivo está seguro, valida
índice/repeat, encaminha `streamdeck.key:<device>`/`key:<index>` ao
`commandadapter.Controller` e força frame completo seguro em lock/logout. O
pacote agora define também `Driver`/`Handle` e um `Runtime` abstrato: enumera
dispositivos, abre handle, escreve frame seguro inicial, ativa somente após
estado seguro, encaminha eventos físicos normalizados, desconecta com backoff em
erro de leitura e escreve frame seguro no shutdown. Nesta rodada, a mesma base ganhou diagnóstico
testável de driver/modelos (`DriverValidation`/`ValidationReport`), com o
candidato `rafaelmartins.com/p/streamdeck` inicialmente marcado como não pronto
até confirmação de licença, manutenção, Windows/Wails, modelos e HID físico.
`Runtime.DiscoverDetailed` passou a reportar falhas parciais por
dispositivo sem bloquear os demais, e testes multi-device provam que render e
remoção de um deck não contaminam outro.

Complemento final antes do hardware: a dependência real
`rafaelmartins.com/p/streamdeck` foi adicionada e compila no pacote. O driver
concreto `StreamDeckDriver` implementa `Driver`/`Handle` com `Enumerate`,
`GetDevice`, `Open`, `Listen`, `AddKeyHandler`, `SetKeyImage`, `ClearKey` e
`Close`, mantendo callbacks convertidos para `PhysicalKeyEvent` e renderização
via geometria real do dispositivo. A biblioteca ficou registrada como
BSD-3-Clause, pure Go/sem CGO, Windows/Linux/macOS/Wails compatível no
diagnóstico local; antes do teste físico, o único motivo restante em
`BuiltinCandidateReport` era `physical-hid-unverified`. O runbook
`docs/operations/streamdeck-manual-validation.md` e o teste opt-in
`ASSISTENTE_STREAMDECK_MANUAL=1 go test ./internal/commanddeck -run
TestManualStreamDeckPhysicalRoundTrip -count=1 -v` deixam a etapa manual
reduzida a conectar o hardware, observar a tecla vermelha, pressionar a primeira
tecla e confirmar `PASS`.

Validação física executada em 15/09/2026 no worktree: Stream Deck serial
`AL28K2C54852`, modelo `Stream Deck`, 15 teclas. A primeira tecla recebeu o frame
vermelho, o teste recebeu `{SourceInstance:streamdeck.key:AL28K2C54852
Key:key:0 Kind:1 Repeat:false}` e terminou com `PASS`.

**Contagem: 47/84 critérios encerrados; 37 abertos; 2/15 pacotes completos.**
I13.5 encerrado como infraestrutura de biblioteca/licença/build/modelos HID,
gerência segura, reconexão/backoff e renderer cache/diff. A população de mapas
reais no produto permanece em P04 e validação ampliada de foco/janela em I13.6.

Validação focada: `go test ./internal/commanddeck -count=1` passou.

## Continuação — 15/09/2026, ambiente físico I13.6

Avanço preparatório em **I13.6/C43/C44/C58**, sem fechar o critério agregado.
`internal/commandphysical` consolida evidências de ambiente físico: foreground
nativo, suporte a hotkey global, sessão interativa observável/desbloqueada e
Stream Deck físico validado. O teste manual opt-in
`ASSISTENTE_PHYSICAL_MANUAL=1 go test ./internal/commandphysical -run
TestManualPhysicalEnvironment -count=1 -v` gera um relatório fail-closed. A suíte
automatizada cobre reports completos e ausentes; `commandforeground` e
`ossession` continuam cobrindo redaction/foreground-before-show e lock/unlock
fail-closed.

Execução do teste manual pelo processo do agente confirmou a limitação esperada
do ambiente sem janela foreground: `GetForegroundWindow` retornou identidade
desconhecida. O runbook `docs/operations/physical-input-validation.md` registra
que a evidência final precisa ser rodada no PowerShell visível do usuário. A
contagem permanece **47/84 critérios encerrados; 37 abertos; 2/15 pacotes
completos** até essa prova manual de foreground/foco/janela.

## Continuação — 15/09/2026, prova de escopo de diálogo

Avanço focado em **I13.3**, ainda sem migrar handlers reais de decisão. A ponte
agora transporta `DialogCommandProof`/`DialogProof` com `dialogId`, geração do
scope, comando e trigger reservado. O resultado precisa devolver a mesma prova,
fechando replay/stale result no round-trip. A composição autenticada do
frontend permite atravessar a barreira modal somente para `decision.respond`
local, originado de `keyboard.local`, com prova que corresponda ao
`DialogCommandScope` topmost atual; provas ausentes, obsoletas, de outro diálogo
ou globais continuam bloqueadas antes de bindings de fundo.

**Contagem anterior: 46/84 critérios encerrados; 38 abertos; 2/15 pacotes completos.**
I13.3 encerrado como infraestrutura de reserva/invariante de diálogo. Isso não
habilita registro global real, HID/Stream Deck, nem migra o `DecisionDialog` para
o executor genérico; esses limites seguem em I13.5/I13.6 e P01–P06.

Validação focada: `go test ./internal/commandbridge -count=1`, `npm test -- --run
src/lib/commandBridge.test.ts src/lib/commandBridgeContext.test.ts`, `npx tsc
--noEmit` e `npx eslint src/lib/commandBridge.ts src/lib/commandBridge.test.ts
src/lib/commandBridgeContext.ts src/lib/commandBridgeContext.test.ts` passaram.

## Continuação anterior — 15/09/2026, ocorrências físicas e sequências

Avanço focado em **I13.2**, ainda sem instalar listeners reais. `commandbridge`
agora transporta `sourceEventId` opcional, validado como UUIDv7, e exige que o
resultado retorne o mesmo ID. `commandadapter` gera um `sourceEventId` UUIDv7
somente no primeiro `down` aceito com binding; `repeat`, `keyup` e entrada sem
binding não criam ocorrência. O controller também ganhou contrato genérico de
sequência: prefixo como `Ctrl+N` abre uma janela com timeout, o ramo autorizado
vira uma chave composta (`Ctrl+N C`), e timeout/blur/troca de geração limpam o
estado sem disparo duplo.

**Contagem anterior: 45/84 critérios encerrados; 39 abertos; 2/15 pacotes completos.**
I13.2 encerrado como contrato de infraestrutura para ownership local/global já
validado pela ponte, geração, ocorrência UUIDv7, repeat/release/blur/reconexão e
sequências do inventário. Isso ainda não registra hotkeys do SO, não prova
layouts reais, não implementa Stream Deck/HID e não popula comandos do produto;
esses limites continuam em I13.5/I13.6 e P01–P06.

Validação focada: `go test ./internal/commandadapter ./internal/commandinput
./internal/commandbridge ./internal/app -run
"TestController|TestPress|TestBridge|TestAppCommandBridge|TestAppCommandLifecycle"
-count=1` passou. Frontend: `npm test -- --run src/lib/commandBridge.test.ts
src/lib/commandBridgeContext.test.ts src/lib/commandBridgeWails.test.ts`,
`npx tsc --noEmit` e `npx eslint src/lib/commandBridge.ts
src/lib/commandBridge.test.ts src/lib/commandBridgeWails.ts
src/lib/commandBridgeWails.test.ts` passaram. `go vet` passou nos pacotes
focados.

## Continuação — 15/09/2026, ciclo genérico de adapters físicos

Avanço focado em **I13.4**, ainda sem registrar teclado global, HID ou comandos
de produto. `internal/commandadapter` introduz um controller de ciclo de vida
para listeners físicos futuros: callbacks recebem geração/session/owner vigentes,
resolvem apenas uma invocação candidata e fazem handoff por `commandbridge.Input`.
O controller nunca chama handler final, não conhece catálogo de produto e não
executa comando. Lock/logout/shutdown suspendem novas entradas; nova geração
reabre somente após `LifecycleGeneration` aceito pela ponte. Conclusões atrasadas
de lifecycle não regredem a geração.

**Contagem histórica: 44/84 critérios encerrados; 40 abertos; 2/15 pacotes completos.**
I13.4 encerrado como lifecycle genérico de adapter. Naquele momento ainda não
fechava I13.2 porque source_event_id e contrato de sequência ficaram para
rodada posterior; também não fecha I13.5/I13.6 porque não há
biblioteca HID, gerenciamento de dispositivo ou validação física.

Validação focada: `go test ./internal/commandadapter ./internal/commandinput
./internal/commandbridge ./internal/app -run
"TestController|TestPress|TestBridge|TestAppCommandBridge|TestAppCommandLifecycle"
-count=1` passou.

## Continuação — 15/09/2026, ponte UI/backend autenticada

Avanço focado em **I13.1**, sem migrar comandos de produto nem editar bindings
gerados do Wails. O App agora possui uma montagem privada da ponte
`commandbridge.Bridge`, métodos exportados para invoke/input/result/cancel/lifecycle
e drenagem no `Shutdown`. Antes de encaminhar qualquer chamada, o App revalida
`userId` e `sessionId` contra a sessão autenticada corrente; capability, owner,
geração e invocation_id continuam validados pela ponte tipada. `Input` e
`LifecycleEvent` agora têm wire camelCase com generation string e enums textuais
compatíveis com o TypeScript. O frontend ganhou um adapter `commandBridgeWails`
que aguarda `window.go` e chama esses métodos sem depender de edição manual de
`frontend/wailsjs`.

**Contagem histórica: 43/84 critérios encerrados; 41 abertos; 2/15 pacotes completos.**
I13.1 encerrado como contrato de transporte UI/backend: ack, resultado,
cancelamento, lifecycle, sessão, generation string e invocation_id atravessam o
App por tipos compartilhados. Isso ainda não habilita listener de teclado,
ownership local/global real, Stream Deck/HID, providers de contexto autoritativos
ou comandos de produto; I13.2–I13.6 e I14 permanecem abertos.

Validação focada: `go test ./internal/commandinput ./internal/commandbridge ./internal/app -run
"TestPress|TestBridge|TestAppCommandBridge|TestAppCommandLifecycle" -count=1`
passou.
Frontend: `npm test -- --run src/lib/commandBridge.test.ts
src/lib/commandBridgeContext.test.ts src/lib/commandBridgeWails.test.ts` passou
com **3 arquivos/36 testes**; `npx tsc --noEmit` e `npx eslint
src/lib/commandBridgeWails.ts src/lib/commandBridgeWails.test.ts` passaram. A
primeira tentativa do Vitest dentro do sandbox falhou em `spawn EPERM` do
esbuild; repetida com permissão elevada e paths relativos corretos.

## Rodada atual — 15/09/2026, quinze pacotes existentes

Rodada atravessa **I01–I15**, sem criar pacotes. Seis agentes Luna reutilizados,
com revisão cruzada, implementação e correções centrais. I01/I02 e a parte de
outbox/heartbeat de I09 recebem regressão, não uma nova implementação artificial.
Trabalhar nos quinze pacotes não significa concluir os quinze.

- **I03/I13:** entrada interna de teclado local respeita input/IME/repeat,
  bloqueio modal e reserva da releitura; compara foco completo e surface antes
  do handoff. Não instala listener. `decision.respond` não atravessa a ponte
  genérica sem prova de diálogo; respostas reais continuam nos handlers atuais.
- **I04/I15:** medição integrada revelou SQLITE_BUSY nas transações do ledger.
  Reserva/CAS usam retry central apenas na transação raiz, nunca no handler;
  resetam resultados por tentativa e reavaliam relógio sem renovar UUID/prazos.
  Transação externa não ganha retry interno. Cancelamento após commit não
  transforma sucesso em falha. Testes cobrem lock real, rollback, cancelamento,
  replay vencido e preservação de registro existente.
- **I05/I06/I07/I08:** ciclo regrant→disable→regrant testa hook real, rollback de
  receipt/grant/geração/auditoria, workspace estrangeiro, cópia e restore sem
  autoridade herdada. Preview não escreve. Simulação dinâmica e montagem de
  todas as fontes de ativação continuam pendentes.
- **I10:** identidade job_service verifica o grant real pela geração exata,
  UUID, owner e fingerprint. Consulta final somente leitura, testada com SQLite
  query_only; regrant posterior não autoriza identidade antiga. Runtime ainda
  é porta confiável de teste, sem montagem produtiva nem inferência de grants.
- **I11:** envelope interno v1/v2 liga serialização/parse ao writer confirmado.
  Validação compartilhada recusa segredos legados também em condição e
  apresentação. Regras importadas são canonizadas na ordem do Store antes do
  CAS. Export local continua falhando fechado se o global herdado estiver
  estruturalmente inválido; sem bypass da validação agregada. UI genérica,
  multi-escopo e exportação sensível permanecem abertos.
- **I09/I12:** recuperação de receipts usa prova real do core atual, cursor e
  CAS/evento compartilhado. Coordinator testado com receipts e invocações reais
  e outros domínios spies; continuação impede compactação. Não prova restart.
- **I14:** wrapper privado de mutação global reconstrói a partir do banco e
  vincula a geração à auditoria do commit. Revalida JWT, política e projeção
  antes de publicar. Resultado distingue `Committed` de `Rebuilt`; conjunto de
  camadas normaliza ordem/nil/vazio, sem aceitar duplicatas. Sem startup público.

**Contagem histórica: 42/84 critérios encerrados; 42 abertos; 2/15 pacotes completos.**
I11.1 encerrado como infraestrutura interna: v1/v2, bindings executáveis,
regras user/builtin, deltas/needs_review e remapeamento entre workspaces distintos
passam pelo envelope e pelo writer confirmado, com reexportação comparada.
Regras event-driven entram desabilitadas e sem concessões. Testes não usam JWT
real nesse percurso, nem habilitam a UI genérica. Nenhum atalho de produto,
transporte Wails, hardware ou segredo real habilitado. BASE-PRONTA continua
pendente. Contagens das rodadas anteriores são históricas, não esforço percentual.

Validação: suíte Go geral com dados temporários e módulos readonly teve **108
pacotes aprovados; ACP/acpregistry falharam com `0xffffffff`**, como na reprodução
anterior à rodada, ainda sem causa determinada. Após congelar as correções,
**32 pacotes command*/App/jobs/database/config/portability/questionnaire/
jobprofilegrant passaram juntos**. Ajustes finais exclusivamente de lint nos
testes foram revalidados com casos focados de App/ledger/portability. Build e
vet globais passaram; golangci-lint terminou com **zero apontamentos**.
Frontend: **312 arquivos/2.933 testes passaram**; TypeScript e linters sem erros
(1 warning ESLint e 2.159 Stylelint existentes). Corrigida espera assíncrona do
teste TaskDetailModal: continua exigindo exatamente duas regiões acessíveis.
Nenhum teste removido, nenhuma assinatura/binding Wails ou texto visível alterado.

Medição final opt-in, Go 1.26.2 Windows/amd64, GOMAXPROCS=22, SQLite temporário
WAL/NORMAL, quatro conexões e busy_timeout=100 ms, 20 aquecimentos + 200 amostras:
serial p50 **3,8313 ms**, p95 **5,4742 ms**, p99 **7,0847 ms**; contenção sintética
p50 **19,0721 ms**, p95 **191,6481 ms**, p99 **317,7258 ms**, 114.809 mutações.
Cada cenário confirmou um handler e um par terminal por invocação (220).
O pool da fixture mantém quatro conexões idle para configurar os pragmas,
enquanto produto usa duas idle e timeout via DSN. Não mede foco/UI/hardware,
não prova crash-durability e **não atende a meta experimental p95 < 1 ms**.
I15.3 permanece aberto: retry corrige contenção transitória, não garante baixa
latência nem sucesso sob saturação persistente. Race/C-GCC, Bugbot, CI, NVDA e
hardware não qualificados. Sem push/PR/merge.

Próximas saídas continuam nos pacotes existentes: prova de restart e composição
de todos os domínios (I12/I14); providers e transporte autenticado com montagem
do App (I03/I06/I10/I13/I14); simulação dinâmica e portabilidade pública/sensível
(I05/I11); qualificação integrada e decisão sobre contenção (I15). Nenhuma dessas
pendências foi descartada para melhorar artificialmente a contagem.

## Rodada anterior — 15/09/2026, dez frentes existentes

Frentes: **I03, I04, I05, I06, I08, I09, I11, I12, I13 e I14**.
Seis agentes Luna reutilizados, com implementação, revisão e correções centrais.
Não foram criados pacotes novos, segredos reais ou atalhos de produto.

- **I03:** contexto de sessão/surface/foco/diálogo composto com a ponte TS; descarte limpa leitores e superfícies. Backend continua exigindo providers autoritativos: a composição frontend não autentica fatos no Wails.
- **I04:** falha de Snapshot pode virar recusa durável quando a fonte ainda entrega as gerações obrigatórias reais. Reentrega consulta o terminal e não executa depois da recuperação. Sem gerações, em cancelamento ou sem proveniência física/evento, continua sem reserva. O contrato não foi relaxado; I04.3 permanece parcial.
- **I05:** Preview reutiliza a preparação autenticada do serviço comum, sem receipt/escrita. Preview de regrant é somente estrutural, exige RuleEnable e escopo exato; não promete simulação dinâmica completa. I05.5 permanece aberto.
- **I06:** fábrica interna do App compõe sessão/JWT, questionnaire/presenter, chaves e stores na mesma raiz SQL. BeforeCommit invalida a projeção antiga dentro do gate exclusivo; rollback não a republica. A montagem pública/lifecycle ainda não está habilitada.
- **I08:** RegrantEventRule confirma o fingerprint exato da regra/produtores e usa o writer comum para receipt, regra, grant, hook, geração e auditoria. Epoch observado antes de abrir decisão; chave usa KeyVersion, não versão do catálogo. Alteração semântica durante a decisão falha fechado. **I08.2 encerrado localmente**, sem declarar I08 completo.
- **I09:** heartbeat concreto é uma porta do mesmo coordinator, com cursor, lote máximo 100 e TTL da passagem. Não duplica a sequência de manutenção nem revive lease expirada. Agendamento automático do consumidor no App permanece pendente.
- **I11:** round-trip por SQLite/serviço interno cobre deltas builtin, needs_review, Keep/Replace/Copy e workspace. Cópia gera IDs novos, remapeia bindings e preserva global. Autenticação nesse teste é uma porta controlada; fluxo público/multi-escopo continua aberto.
- **I12:** recuperação all-owner pagina invocações de usuários/system e consome somente prova real de gerações drenadas no processo atual; reutiliza o writer existente. Manager pode receber coordinator antes de Start, com adapters reais legados, e usa seu único timer para heartbeat/continuações. Prova de restart e montagem de todos os domínios ainda faltam.
- **I13:** ponte TS interna valida sessão/geração, filtra resultados de outra sessão, impede regressão de geração por respostas fora de ordem e encerra recursos com ownership exclusivo. Sem transporte Wails ou listener físico novo.
- **I14:** composição do App recusa dependências trocadas, DB inadequado, closing e estado bloqueado. Stop do Manager cancela e aguarda manutenção fora dos locks; Start e remontagem recusados durante drenagem. Montagem integrada de startup/login/restart ainda pendente.

**Contagem: 41/84 critérios encerrados; 43 abertos; 2/15 pacotes completos.**
É um critério agregado adicional (I08.2), além de avanços internos nos outros
pacotes. Dez frentes trabalhadas não significam dez pacotes completos nem uma
porcentagem de esforço. BASE-PRONTA não foi atingido.

Validação final: suíte Go completa com arquivos estáveis, dados temporários e
módulos readonly: **108 pacotes passaram; ACP/acpregistry encerraram com
`0xffffffff`**. A causa continua indeterminada, com reprodução na base anterior
documentada; não declarar a suíte global verde. A tentativa anterior durante
edições concorrentes encontrou teste incompleto de configuração e foi substituída
por esta repetição final. Todos os command* e App/jobs/database/config/portability/
questionnaire passaram juntos. Build/vet globais e lint Go passaram (zero
apontamentos). Frontend completo: **312 arquivos/2.915 testes**, TypeScript e
linters sem erros (1 warning ESLint e 2.159 Stylelint existentes). Fábrica real
do App e shutdown/loop foram repetidos três vezes. Nenhum teste foi removido.
Sem nova assinatura Wails, binding gerado ou texto visível de UI. Race/C-GCC,
Bugbot, CI, NVDA e hardware não qualificados; sem push/PR/merge.

### Próximas saídas verificáveis, sem novos pacotes

1. **Recuperação de restart:** provar exclusão do processo/geração anterior e integrar receipts abandonadas, invocações e claims na montagem única (I12.1–4/I14.3).
2. **Montagem real do App:** fornecer catálogo/projeção/políticas/hook reais, reconstruir e publicar o mapa após mutações, ligar transporte autenticado de contexto/dispatch (I03.1/I06/I13.1/I14.2–5).
3. **Fechar simulação e portabilidade:** diff dinâmico exato, integração genérica/publicação, lote multi-escopo e referências sensíveis (I05.5/I11).
4. **Qualificar antes da migração:** latência integrada, crash/replay, race com C/CGO, CI/review, SO/HID e NVDA (I13.2–6/I15). Migração de comandos permanece P01–P06.

## Rodada anterior — 15/09/2026, ligação do core e cinco frentes

Seis agentes Luna reutilizados, com revisão e correções centrais. Avanço nos mesmos
I09/I11/I12/I13/I14, sem criar pacotes novos nem habilitar atalhos de produto.

- **I09:** adapters concretos ligam o Consumer à outbox e à recuperação do coordinator. Lotes são limitados a 100; cursor só avança com commit e reinicia ao completar o ciclo, permitindo revisitar leases que expiraram depois. Concorrência é recusada sem esperar outro ciclo.
- **I11:** no-op validado retorna `ErrNoChanges`, distinto de entrada inválida, sem decisão, auditoria ou avanço de geração. Owner e referências revogados continuam recusados. Teste do writer confirmado prova que regra de evento entra desabilitada e sem grants/claims.
- **I12:** o core registra obrigatoriamente os executores construídos por New/NewComplete, fecha admissão permanentemente e espera as operações e sua finalização fora do gate. Só então emite prova das gerações efetivamente emitidas. O ledger sela essa prova e reutiliza o writer existente de recuperação; nenhuma segunda implementação de recovery. Testes reais cobrem finalização bloqueada e falha de CAS seguida de outcome_unknown, sem repetir handler. O coordinator preserva progresso confirmado mesmo ao retornar erro.
- **I13:** DialogCommandScope chega ao stack real de Modal; somente o topo fornece o scope. Trocar o scope de um modal inferior não o promove. Não há novo listener de teclado nem ownership físico habilitado.
- **I14:** shutdown do App drena o core antes de destruir dependências, inclusive sem controller montado. Fábrica interna completa compõe HostState, sessão e FactBus reais; decisões usam o presenter do App e a mesma raiz SQL do ledger. Recusa bancos distintos/transações, estado bloqueado, sessão não reconstruída e instalação após shutdown. Continua sem startup de produto.

**Limites de aceite:** a prova nova é do processo atual, não de um processo anterior.
Restart, cadência única com heartbeat/receipts, transporte autenticado UI/backend,
importação pública/multi-escopo e hardware continuam pendentes. Timeout não reabre
o core; nenhum marker persistido sozinho autoriza recuperação. A espera pelo gate
não ganhou promessa de prazo rígido.

**Contagem: 40/84 critérios encerrados; 44 abertos; 2/15 pacotes completos.**
I11.4 foi encerrado com testes negativos de serialização e teste transacional de
regra importada desabilitada, além da preservação de grants/claims de regra
intocada no destino. Defaults puros e histórico não pertencem ao DTO; referências
a defaults continuam portáveis. Os demais critérios compostos permanecem abertos.
Essa contagem não representa percentual de esforço nem cinco pacotes completos.

Validação: suíte Go completa executada nesta rodada: 108 pacotes passaram e
ACP/acpregistry terminaram com `0xffffffff`, como na investigação anterior.
Após as últimas correções, todos os command* e App/jobs/database/config/portability/
questionnaire passaram juntos. A fábrica completa do App passou em três repetições,
incluindo sessão nova sem mapa reconstruído. Frontend: 311 arquivos/2.909 testes;
Build e vet Go globais passaram; lint Go final: zero apontamentos.
TypeScript, ESLint e Stylelint sem erros (1 e 2.159 warnings existentes,
respectivamente). Nenhum teste removido ou proteção da máquina alterada.
Sem nova assinatura Wails, bindings gerados ou string visível alterados.
Race/C-GCC, Bugbot, CI, NVDA e hardware continuam não qualificados; sem push/PR/merge.

## Rodada anterior — 15/09/2026, continuação das cinco frentes

Seis agentes Luna reutilizados; revisão e correções centrais. Sem migração de atalhos, novos segredos ou habilitação de entradas.

- **I09:** `RunPass` reivindica e consome lote pela transação comum. `HeartbeatPass` percorre leases com cursor/More e TTL atual por passagem, sem alterar o Consumer compartilhado. Revalida fonte, autorização, grant e runtime sob gate; não revive lease vencida. Sem timer/goroutine nova. Agendamento produtivo ainda pendente.
- **I11:** comparação de conjuntos vazios normalizada; Keep repetido é recusado como ausência de mutação (`ErrInvalid`), sem decisão ou incremento. Testes reais de ApplyPlanImport cobrem referência ausente, rollback de lote pelo hook e recusa de global+workspace na API de escopo único. Não confundir no-op seguro com UX pública/idempotência multi-escopo concluída.
- **I12:** adapters reais paginam usuários com cursor por operação, reiniciam ao mudar política, preservam contagem confirmada e continuação em erro/cancelamento. Admissão concorrente no mesmo owner é recusada; More de tools também bloqueia compactação. Limite é de usuários, não linhas de cada limpeza. Recuperação com proof existente agora compara fingerprint/versão/fonte do par e rejeita relógio zero; rollback do lote testado.
- **I13:** DialogCommandScope acompanha fila/diálogo ativo, com allowlist fixa `decision.respond` e invariante local de releitura. Scope de outro diálogo é recusado e copiado de forma imutável. O dispatcher de Modal/Wails e ownership físico continuam não montados.
- **I14:** Stop sinaliza o worker sem depender de enqueue bloqueante, inclusive com contexto cancelado. Rebootstrap retira a geração anterior, também quando readiness inicial falha. App serializa construção/publicação e fecha admissão de montagem permanentemente no shutdown, antes de soltar o mutex e aguardar o worker. Corridas de montagem e cleanup do candidato rejeitado cobertas.

**Contagem mantida: 39/84 critérios; 45 abertos; 2/15 pacotes completos.** A revisão não aceitou substituir a prova real de drenagem por callback/marker: I12.1 continua pendente, sem caminho alternativo de recuperação. O fluxo produtivo ainda depende de dono único que invalide o core, impeça novos executores e aguarde todos os Services da geração.

Validação final: todos os pacotes command* e App/jobs/database/config/portability/questionnaire passaram juntos, com home temporário e módulos readonly. Jobs/manutenção foram repetidos após o último ajuste de contagem parcial. Runtime passou em 10 repetições; lifecycle do App em 3. Frontend completo: 311 arquivos/2.902 testes; TypeScript e ESLint dos arquivos alterados passaram. Build/vet globais e lint Go passaram. A suíte global da rodada anterior teve 108 pacotes aprovados e ACP/acpregistry com `0xffffffff`; não foi repetida inteira neste novo diff, nem declarada verde. Race, Bugbot, CI, NVDA e hardware continuam não qualificados.

## Rodada anterior — 15/09/2026, integração I09/I11/I12/I13/I14

Seis subagentes Luna, revisão e integração central; nenhuma entrada de produto habilitada.

- **I09:** purga transacional em lotes protege fatos de claims com lease viva. Renovação após o prazo de replay, terminalização e purga exercitadas juntas; replay continua fechado. Worker/heartbeat e chamada produtiva da purga ainda não montados.
- **I11:** importação interna passa pelo serviço comum de decisão/CAS, com revalidação final de referências, receipt, auditoria e avanço de geração atômicos. Keep/Replace/Copy preservam os limites de escopo; workspace não escreve globals herdados. Migração 27 reconhece o schema anterior e acrescenta `config_import` sem perder auditorias. Fluxo público, lote multi-escopo e aceite completo continuam pendentes.
- **I12:** retenção de ativações por idade/cap protege estado ativo e ledger dentro do prazo; adapters reais de ledger, jobs, tools e compactação disponíveis. `More` e cancelamento bloqueiam a compactação indevida. Adapter legado de jobs/tools ainda percorre usuários sem paginação. Executor fecha admissão, serializa o último Start com shutdown e espera a finalização durável; isso ainda não produz prova de geração encerrada.
- **I13:** adapter de decisão usa a fila real de questionários, cancela por identidade exata e rejeita IDs duplicados. Ponte Wails autenticada, invariantes de teclado e hardware ainda não montados.
- **I14:** hooks reais de startup/login/refresh/logout/shutdown; bootstrap tardio verifica sessão atual. Timeout mantém controller montado até o worker terminar e impede destruir dependências em uso. Providers reais e montagem completa ainda pendentes.

**39/84 critérios encerrados; 45 abertos; 2/15 pacotes completos (I01/I02).**
I12.5 encerrado no nível de implementação da política/settings; montagem da cadência permanece explicitamente em I12.4. Contagem de critérios não é porcentagem de esforço ou conclusão do AEP.

Próxima sequência: produzir prova real de geração drenada (I12.1/2), montar recuperação/outbox/heartbeat e cadência única (I09/I12.3/4), compor providers/presenter/transporte no App (I03/I13/I14), completar importação pública e multi-escopo (I11). Não há novos pacotes numerados.

Validação estável: Go completo com home temporário e `-p 2`: 108 pacotes passaram, ACP/acpregistry encerraram com `0xffffffff`. Todos os command*, App, jobs e database passaram. Build/vet globais e lint Go (zero apontamentos) passaram. Frontend: 311 arquivos/2.899 testes, TypeScript e linters sem erros (1 warning ESLint, 2.159 Stylelint existentes). Race/C-GCC, Bugbot, CI, NVDA e hardware continuam não qualificados. Evidências abaixo pertencem às rodadas anteriores.

## Rodada anterior — 15/09/2026, fechamento de gaps em cinco pacotes existentes

Frentes: **I05, I09, I11, I12 e I13**, com seis agentes Luna e revisão central.
Não foram inventados pacotes I16–I20. A investigação transversal de ACP/acpregistry
foi priorizada em I15, sem remover testes nem alterar proteções da máquina.

- **I05:** diagnóstico usa a ativação real e stamp privado; troca do conjunto invalida o diagnóstico, reordenação não. Preparação valida a prova contra o estado persistido antes de projetar o depois, permitindo disable/delete de camada ativa. Testes cobrem união contextual/manual e ausência de efeitos antes da decisão. Preview autoritativo completo de expiração/rebind/reconcile continua pendente; I05.5 não está encerrado.
- **I09:** renovação de lease separa continuidade do runtime e admissibilidade de replay. Epoch, deadline, fingerprint, grant e geração continuam validados; o evento antigo continua recusado. Sem a outbox original, falha fechado. Worker, heartbeat real e política de preservação/reconstrução da fonte continuam pendentes.
- **I11:** contêiner portátil explícito de personalizações builtin por escopo, sem camada artificial, claims ou grants. Planejamento e export interno avançaram; o writer confirmado no serviço comum e a ativação no fluxo público continuam pendentes. Não prometer backup funcional de comandos na UI.
- **I12:** seis settings, defaults, persistência, UI/i18n e documentação de usuário entregues. O caminho opcional do Manager relê a política inteira a cada passagem; erros de leitura e overflow não viram limpeza com política substituta. Outbox pendente impede retenção/compactação. Ainda faltam prova de encerramento de geração, adapters reais e montagem de instância.
- **I13:** shutdown idempotente nas pontes Go/TS, invalidação antes do cancelamento e liberação após os lotes admitidos; resultados tardios recusados e recursos em memória liberados. Taxonomia `streamdeck.key` alinhada. Não há montagem de Wails/SO/HID nem validação física; lifecycle genérico de adapter ficou para rodada posterior e foi encerrado depois em I13.4.

**Contagem mantida: 38/84 critérios encerrados, 46 abertos; 2/15 pacotes completos.**
Esta rodada fecha lacunas internas de critérios compostos, não os seus requisitos
de integração restantes. Não converter volume de código ou número de frentes em
porcentagem do AEP.

### Investigação ACP/acpregistry

Comparação com `f36c25ddb739a519cb24c42cf385945b06ebe85f`, merge-base anterior aos
commits desta implementação, em worktree detached e com o mesmo Go 1.26.2.
Os dois pacotes também apresentaram `0xffffffff` nessa base; acpregistry passou
em duas repetições do HEAD e voltou a falhar na suíte geral. Os arquivos desses
pacotes e go.mod/go.sum não diferem da base. O executável ACP também encerra com
`-test.list .`, sem saída de `GODEBUG=inittrace=1`. AppLocker registra execução
permitida; logs recentes consultados não trouxeram causa do término.

Isso demonstra reprodução na base **no ambiente atual**, não comprova o que
ocorria no ambiente histórico do usuário. Causa raiz permanece indeterminada;
não há correção de código justificada por essa evidência nem suíte global verde.
Não foram criadas exclusões de segurança, skips ou fallbacks de testes.

### Validação desta rodada de gaps

- Suíte Go completa com home temporário, módulos readonly e `-p 2`: **108 pacotes passaram; ACP/acpregistry falharam por término de processo**. Log preservado na área de trabalho da tarefa. A paralelização menor não eliminou a falha.
- Frontend: **309 arquivos / 2.886 testes passaram**; TypeScript passou com os bindings oficiais regenerados (seis campos, sem edição manual dos gerados).
- Build e vet Go globais passaram. ESLint: zero erros e um warning existente; Stylelint: zero erros e 2.159 warnings existentes, sem CSS modificado.
- Lint Go v2 passou com **zero apontamentos**, sem limitar achados. Após as correções finais, foram repetidas com sucesso todas as suítes command*, jobs, config, portability e database.
- Testes de regressão adicionais verificam disable/delete de camada ativa com hook real, erro de leitura sem limpeza, política alterada entre passagens e shutdown com dois cancelamentos sucessivos.
- Requeue de leases agora seleciona IDs em transação, com lote máximo 128 e indicação de continuação; leases vivas e linhas pending não são alteradas. Tanto requeue quanto drain pendentes impedem a retenção da passagem.
- Race continua sem C/GCC; Bugbot, CI, NVDA e hardware não foram qualificados. Nenhum push/PR/merge; entradas de produto continuam desabilitadas.

### Próxima fila, sem novos pacotes

1. **I12.1/I12.2:** ligar a prova de encerramento/drenagem do core à recuperação de invocações, inclusive outros usuários/system.
2. **I12.3/I12.4 e I09.5:** compor adapters reais, outbox/heartbeat e a cadência única; TTL salvo precisa chegar ao runtime. Definir preservação/reconstrução da fonte depois da purga, sem reabrir replay.
3. **I05.5/I06.2/I08.2/I11.3:** simulação autoritativa do diff, regrant e writer de importação no mesmo fluxo de decisão/CAS; não criar escritor alternativo.
4. **I03/I13/I14:** montar providers, presenter, transporte, diálogo e lifecycle no App antes de migrar comandos. Hardware/NVDA e qualificação I15 permanecem gates posteriores explícitos.

## Rodada anterior — 15/09/2026, I11–I15 e pendências anteriores

- Cinco pacotes novos trabalhados em seis frentes Luna; a sexta fechou pendências de configuração/regras. Revisão e integração central no mesmo worktree.
- Encerrados localmente nesta rodada: I05.3, I08.3, I09.4, I14.1 e I15.1. Total: **38/84 itens**, 46 abertos. **2/15 pacotes inteiramente encerrados** continuam I01/I02; não equiparar avanço parcial a pacote completo.
- I05.5 ganhou diagnóstico autenticado, mas o preview de transições dinâmicas ainda exige qualificação. I06/importação e I08/regrant não foram contornados com escritores paralelos.
- A maior entrega integrada é o consumidor transacional de eventos: revalidação de grant/runtime, CAS, lease própria, ack atômico e barreira contra reabertura de ciclo compactado. Worker/heartbeat e publicação real continuam desabilitados.
- Próxima sequência concreta: prova real de encerramento/drenagem (I12.1); adapters de manutenção e política completa (I12.3–5); montagem compartilhada do App/UI (I03/I13/I14). Em paralelo, writer confirmado e deltas builtin de I11. Não há novos pacotes de infraestrutura além dos 15 desta lista.
- Contagem mede critérios encerrados, não porcentagem de esforço do AEP.

### Validação I11–I15

- Backend: build e vet globais passaram. `go test -mod=readonly -count=1 ./...` foi executado com HOME/USERPROFILE/APPDATA/XDG apontando para diretório temporário e GOCACHE/GOTMPDIR isolados, preservando GOPATH/GOMODCACHE. App, config, logging, skills, database, jobs, portability e todos os command* passaram. Restam ACP e acpregistry: processo termina com `exit status 0xffffffff`, inclusive ao repetir isoladamente com `-v`, sem diagnóstico de teste. Não atribuir causa definitiva nem declarar suíte global verde.
- Frontend: **309 arquivos / 2.882 testes passaram** na repetição com bindings estáveis; TypeScript e ESLint passaram (um warning em arquivo não alterado). Stylelint: zero erros, 2.159 warnings existentes; nenhum CSS modificado.
- Bindings regenerados pelo binário oficial compilado com `-tags bindings`, sem edição manual. O wrapper `wails generate module` travou e foi interrompido; seu fluxo de geração equivalente concluiu. A tela existente envia `includeCommandLayers: false`, sem habilitar recurso incompleto.
- I15.1: TestMain de App/config isola configuração em diretórios temporários. Ao repetir a suíte global, preservar primeiro `go env GOPATH`/`GOMODCACHE`, definir diretório temporário nas variáveis de home do processo e usar `-mod=readonly`; não alterar ambiente persistente da máquina.
- Qualificação adicional: golangci-lint **v2.11.4 passou globalmente com zero apontamentos**, sem limitar a quantidade de achados por categoria. Correções preservam assertivas; testes deliberados de contexto nil têm exceção pontual justificada. Build/vet foram repetidos e passaram.
- Repetição final Go: **108 pacotes com testes passaram; 2 falharam (ACP/acpregistry)** com o mesmo término de processo. Testes de restore/escopo/serviço completo passaram após a comparação transacional entre estado final e preview.
- Detector de corrida ainda sem GCC/CGO; Bugbot, CI, hardware e NVDA não qualificados. Nenhum push/PR/merge. Atalhos existentes não migrados e nenhum novo segredo criado.

## Rodada anterior — 15/09/2026, I05–I10

- 33/84 itens encerrados localmente; 51 permanecem abertos. Esta rodada fecha 15 itens adicionais e implementa partes dos demais.
- Continuam 2/15 pacotes inteiramente encerrados (I01/I02). Não declarar I05–I10 completos enquanto seus critérios agregados e integrações pendentes não estiverem atendidos.
- As seis frentes foram paralelizadas com revisão e correções na integração. Código continua no worktree isolado; atalhos e consumidores novos não foram habilitados no produto.
- I03.1 ainda depende da ponte autenticada UI/backend. I04 ganhou contextos genéricos e anti-loop, mas a montagem com fontes reais e recusas anteriores ao snapshot continua pendente.
- Próxima conclusão concreta: ampliar o diff de restore/CRUD de regras para incluir estado/grants e compor o consumo de eventos com CAS por regra, lease de claim e manutenção I12. Depois, montar as fontes/runtimes reais de I03/I10/I14.
- Contagem de itens não mede porcentagem do AEP nem esforço restante. Evidências detalhadas ficam nas seções I05–I10 e no adendo desta rodada da AEP principal.

## 1. Objetivo e fonte de verdade

### Validação local da rodada anterior I05–I10

- Passaram as suítes completas de `internal/command...`, `internal/database`, `internal/auth`, `internal/toolinvocations`, `internal/tools` e `internal/jobs`, com `-count=1` e módulos readonly.
- Passaram build e vet globais e o bootstrap isolado do App (`TestCommandStorage`).
- Corrigida regressão real no segundo boot: o cutover do ledger reconstruía `job_runs` sem as novas colunas. Testes agora cobrem bancos publicados e preservação de fila/proveniência.
- Não foi executado `go test ./...`: testes legados do App atingem configuração compartilhada. Detector de corrida indisponível sem GCC; frontend, CI e Bugbot ainda não qualificados nesta rodada. Nenhum push/PR realizado.

Encerrar a infraestrutura prevista antes de migrar/popular o novo sistema com os comandos existentes. Não reduzir a base para antecipar uma demonstração de atalhos.

Arquivo canônico de acompanhamento: `aep/0103-tasklist-infraestrutura.md` no worktree. A cópia entregue em `outputs/` é uma fotografia; atualizar a canônica e republicar a cópia nas revisões do plano.

A fonte normativa permanece `aep/0103-comandos-acionadores-e-camadas-contextuais.md`. Este arquivo é a fila operacional: IDs estáveis, dependências, critérios de saída e registro de mudanças. Em divergência, não mudar arquitetura silenciosamente; esclarecer e atualizar o AEP no mesmo ciclo autorizado.

Este plano substitui a estimativa informal de “35%” como instrumento de acompanhamento. Não há porcentagem total validada. Test coverage, linhas e commits não medem entrega do AEP.

### Fotografia anterior — 14/09/2026, após a rodada I02–I04

- 15 pacotes de infraestrutura; I01/I02 implementados e validados localmente (2/15); qualificação global I15 pendente.
- 66 itens restantes de infraestrutura, dos 84 da baseline; 18 encerrados localmente. I03 e I04 permanecem parciais pelos limites de integração descritos abaixo; I06.1 foi antecipado como dependência da execução destrutiva.
- 4 marcos de infraestrutura, seguidos por 6 pacotes de migração/entrega.
- 83 critérios finais do AEP com responsáveis mapeados no apêndice.
- M1: I01/I02 entregues localmente, I03 com 5/6 itens encerrados; M2: I04 com 3/6 itens encerrados e I06.1 antecipado. M1–M4 ainda não encerrados. Contagem de itens/pacotes não é porcentagem de esforço ou do AEP.
- Checkbox aberto significa obrigação ainda não encerrada no escopo descrito. Não marcar um pacote concluído apenas porque passou um teste do subconjunto.

### O que já existe e será reaproveitado

As marcações abaixo reconhecem somente os limites explicitados, não os critérios finais do AEP. Evidência: código e registro incremental do AEP; nesta rodada documental não foram reexecutadas as suítes.

- [x] E01 — Registro estático, busca localizada e validação de contratos: `internal/commandcatalog/{registry,search,readiness}.go`.
- [x] E02 — Seleção pura indexada, precedência, conflito, composição de defaults e restore em memória: `internal/commandbindings/`.
- [x] E03 — Freshness e versões abstratas de contexto: `internal/commandcontext/`; diagnóstico sem execução: `internal/commandpreflight/`.
- [x] E04 — Epochs, gate, cancelamento e revalidação; host global por usuário: `internal/commandsecurity/` e `internal/commandexecution/host_*.go`.
- [x] E05 — Leitura persistida escopada, schema de layers/bindings/gerações e projeção restrita read/none: `internal/commandconfig/`.
- [x] E06 — Executor/ledger de leituras diretas de sessão local e handles assíncronos: `internal/commandexecution/` e `internal/commandledger/`. Não inclui argumentos, workspace, trigger ou escrita genérica.
- [x] E07 — Decisões persistidas, adapter do diálogo existente e mutação confirmada de enabled global com auditoria atômica: `internal/commanddecision/`, `internal/app/app_command_decision.go`, `internal/commandconfig/{decision,mutation_audit}.go`.
- [x] E08 — Recuperação de receipts da sessão atual em lotes e cancelamento cooperativo de reconstrução: commits `e599be6f5` e `11c10c578`.
- [x] E09 — Máquina de pressão/release e observação do lock do SO: `internal/commandinput/`, `internal/ossession/`. Ainda não equivalem a adapters de teclado/HID completos.
- [x] E10 — Inventário inicial de atalhos: `aep/0103-inventario-atalhos.md`; benchmarks do resolvedor e gate. Não são migração nem SLA ponta a ponta.

## 2. Linha de chegada e marcos

“Infraestrutura pronta” significa I01–I15 encerrados com evidência. É mais amplo que o primeiro bloco de inicialização discutido na conversa.

- **M1 — Contratos e ambiente completos:** I01, I02, I03.
- **M2 — Execução e configuração completas:** I04, I05, I06.
- **M3 — Estado durável e fronteiras de segurança:** I07, I08, I09, I10, I11, I12.
- **M4 — Montagem e qualificação:** I13, I14, I15.
- **BASE-PRONTA:** aceite de M1–M4; então iniciar a migração/população sistemática P01–P06.

Marcos são pontos de aceite, não barreiras artificiais ao paralelismo: um pacote começa assim que suas dependências estiverem satisfeitas. O esqueleto I14.1 pode começar após I01; isso não encerra I14.

Algum código em App, jobs, auth, ferramentas e UI compartilhada precisa mudar durante a infraestrutura para demonstrar seus contratos reais. “Antes da migração” não significa construir uma segunda aplicação isolada ou adiar toda integração estrutural. Significa ainda não transferir a população de comandos/handlers antigos.

### Definição de pronto de cada pacote

- Todos os seus itens entregues, com caminhos/símbolos, commit e testes registrados.
- Testes do contrato e das falhas/concorrência relevantes; integração nas portas reais quando o item exigir.
- Sem fallback que contorne segurança e sem novo problema crítico conhecido.
- Documentação/AEPs relacionados atualizados no mesmo ciclo.
- Revisão local do agente registrada; Bugbot/CI têm estados próprios, nunca inferidos.
- Dependências externas de validação concluídas ou pacote explicitamente bloqueado; ausência de ferramenta não vira aprovação.

## 3. Como estimar e acompanhar sem falsa precisão

Tamanho relativo do restante: **M** = concentrado em um subsistema; **G** = vários contratos/integração; **GG** = transversal, migração ou risco alto. Essas classes NÃO são dias, turnos, commits ou quantidade de agentes.

A previsão de calendário ainda não está calibrada. Não converter 84 checkboxes em “84 rodadas”, nem somar pacotes como se tivessem o mesmo custo. Ao concluir I01 e I02, registrar esforço observado e revisar a previsão dos demais com faixa otimista/provável/conservadora e premissas. Não prometer uma data antes dessa calibração.

Em cada entrega, atualizar:

- IDs encerrados e evidência.
- Pacotes fechados em cada marco.
- Itens em execução e dependências efetivamente bloqueantes.
- Trabalho novo descoberto e impacto no escopo/previsão.
- Próximo pacote concreto — não apenas “próximos passos”.

O usuário não precisa autorizar cada subitem técnico dentro do escopo já aprovado. Pausas para decisão são reservadas a mudança de contrato/escopo, autoridade externa ou dependência que realmente precise dele.

## 4. Tasklist de infraestrutura

### I01 — Banco e chaves operacionais

Estado: **Implementado e validado localmente** em `021d18e07`. Qualificação global/CI/Bugbot permanecem explicitamente pendentes em I15 e no fluxo de publicação.
Dependências: nenhuma de outro pacote; usar serviços existentes.
Referências: D2.1, D11.

Evidência: `internal/commandbootstrap/{schema,keys}.go`, `internal/database/command_migration.go`, `internal/credentials/instance_secret_create.go` e `internal/app/app_command_storage.go`. O App prepara armazenamento no bootstrap/reconfiguração do cofre, sem construir executor ou registrar atalhos. Rotação é manutenção interna, exige admissão suspensa pelo host e conserva todas as versões; aposentadoria seletiva fica em I12 e ligação ao executor em I14.

- [x] I01.1 — Unificar a ordem e o versionamento das migrações no banco real, com teste de banco novo, upgrade e schema incompatível; falha não publica readiness.
- [x] I01.2 — Implementar provisionamento idempotente e carregamento da chave de fingerprint no escopo correto; não reutilizar JWT/pepper nem substituir chave existente ao reiniciar.
- [x] I01.3 — Definir versão ativa, retenção das versões antigas pelo maior deadline dos ledgers e procedimento testado de rotação; ausência/corrupção falha fechado.
- [x] I01.4 — Testar primeira abertura, reinício, cofre indisponível e concorrência de inicialização usando diretórios e credenciais de teste.

Critério de saída: Abrir ou reabrir a instalação prepara o armazenamento e as chaves de forma reproduzível, sem habilitar execução prematuramente.

### I02 — Contratos completos de catálogo, documentos e fingerprints

Estado: **Implementado e validado localmente**. Qualificação transversal I15 pendente.
Dependências: interfaces já existentes; pode avançar em paralelo com I01. A montagem real das chaves depende de I01.
Referências: D2, D2.1, D4, D6, D11.

Evidência: `commandcatalog.NewComplete`, schemas tipados, `commandcontract.Envelope/SignResolved/SignRefusal`, `commandjson`, `commandconfig.BuildSemanticDefault`; corpus lexical compartilhado em `commandjson/testdata/lexical.json`. Signer legado permanece distinto; adapters/importação e publicação do catálogo real têm pacotes próprios.

- [x] I02.1 — Completar schema de argumentos, resultado e envelope versionado para origens/contextos previstos; validar grupos opcionais, nulabilidade e limites antes da reserva.
- [x] I02.2 — Implementar canonicalização RFC 8785 do conjunto suportado e HMACs com separação de domínio para argumentos, request e demais fingerprints; cobrir números, Unicode, objetos e campos excluídos.
- [x] I02.3 — Calcular fingerprint semântico completo dos defaults; apresentação puramente visual não invalida semântica executável.
- [x] I02.4 — Fechar contrato de classificação do handler, origens, contexto, sensibilidade, disponibilidade e apresentação localizada; testar catálogo de contratos, sem cadastrar todos os comandos reais.
- [x] I02.5 — Definir compatibilidade de versões e corpus de testes compartilhado entre persistência, importação e ingresso.

Critério de saída: Nenhum consumidor precisa inventar seu próprio formato, identidade de request ou interpretação de segurança.

### I03 — Contextos autoritativos e isolamento por workspace

Estado: **Parcial**. Esforço restante: **G**.
Dependências: I02.
Referências: D2, D2.1, D7, D8, D12, D14.

Evidência: `internal/commandcontext/{scoped_factbus,scoped_generations,cache}.go`, `internal/workspace/command_snapshot.go`, `internal/commandforeground/`, `internal/app/app_command_context.go` e `frontend/src/lib/commandContext{Providers,Session}.ts`. Fontes reais de workspace/aba/perfil e foreground estão disponíveis por factories privadas do App; foco, superfície e diálogo têm leitura escopada no frontend. Falta ligar a ponte autenticada frontend/backend e registrar superfícies reais: um frame da UI não é autoridade no backend. Providers ausentes continuam recusados. Montagem do ciclo de vida permanece em I14, sem migrar atalhos.

- [ ] I03.1 — Implementar providers de surface, diálogo, foco/controle, aba, workspace, perfil e janela; validar ownership na fonte, não confiar no snapshot enviado pela UI.
- [x] I03.2 — Implementar ContextFactBus e reconciliação síncrona quando uma notificação se perder ou chegar fora de ordem.
- [x] I03.3 — Separar gerações globais, por workspace e de camadas efetivas; mudança em outra conta/workspace não invalida trabalho independente.
- [x] I03.4 — Completar políticas exact_version, max_age_ms e event_snapshot no percurso de admissão, com timestamps confiáveis por provider.
- [x] I03.5 — Implementar captura de foreground antes de bring-to-front, redação de resumo e degradação explícita onde não houver adapter; não persistir títulos/URLs.
- [x] I03.6 — Testar atualização de contexto concorrente, provider ausente e caches positivos/negativos com todas as dimensões de isolamento.

Critério de saída: Todo alvo/contexto usado pelo executor é reconsultável e versionado, inclusive com perda de notificações.

### I04 — Dispatcher, execução e ledger completos

Estado: **Parcial**. Esforço restante: **GG**.
Dependências: I02, I03.
Referências: D2.1, D3, D4, D11, D16.

Evidência: `internal/commandexecution/envelope_{engine,pipeline}.go` amplia o MESMO Service com catálogo completo, argumentos, workspace, resolução direta/trigger, políticas de contexto e read/write/destructive; `internal/commandledger/envelope.go` amplia as mesmas tabelas. `NewComplete` exige portas confiáveis e desabilita as entradas legadas nessa instância. O fluxo local é testado com handlers controlados; não há montagem do executor ampliado no App. Autenticação externa/job/system depende de I10 e a projeção persistida completa depende de I05. Esses limites impedem declarar o pacote inteiro concluído.

- [ ] I04.1 — Unificar execução direta e por trigger: resolver, fixar origem vencedora, derivar ator e normalizar argumentos antes de assinar/reservar.
- [x] I04.2 — Ampliar reserva e auditoria para argumentos, triggers, origem física/evento, workspace e contexto; preservar IDs canônicos e rejeitar fingerprint divergente.
- [ ] I04.3 — Persistir recusas pós-autenticação e marcadores terminais suppressed/rejected_stale; reentrega não pode passar a executar após alteração de configuração.
- [ ] I04.4 — Completar gates evaluating → queued → running para read/write/destructive, revalidando política, catálogo, mapa, contexto e decisão no ponto correto.
- [x] I04.5 — Preservar handoff não bloqueante, finalização atômica, resultado redigido, consulta autorizada independente da versão atual e reconciliação auditada de outcome_unknown sem reexecução.
- [x] I04.6 — Testar filas, duplicidade, perda de ack, panic, cancelamento, mudança de configuração e falhas transacionais com handlers controlados.

Itens ainda abertos, com limite exato: I04.1 tem o fluxo unificado de sessão local, mas não autenticação dos demais contextos; I04.3 persiste recusas quando há envelope autoritativo válido, mas uma falha anterior do Snapshot não produz uma reserva assinável; I04.4 tem os gates completos no fluxo local, faltando exercitá-los nas portas reais de configuração e dos demais contextos. Não ampliar esse escopo por um fallback permissivo.

Critério de saída: O serviço único consegue aplicar os contratos do AEP a todos os tipos previstos de execução, sem depender de handlers de produto.

### I05 — Configuração completa, defaults e restauração

Estado: **Parcial — implementação e integração local ampliadas em 15/09/2026**. Esforço restante: **G/GG**, conforme dependências abaixo.
Dependências: I02, I03.
Referências: D4–D7, D10, D11.

Evidência/limite atual: `ProjectComplete`, `CompleteMutationService`, `defaults_mutation.go` e `activation_hook.go`: projeção semântica, CRUD confirmado com CAS, upgrade/rebase e revogação/reconciliação transacional. Restore agregado agora inclui regras/grants/claims, receipt consumida e auditoria no mesmo TX; testes verificam revogação e rollback. `CheckConflicts`/`RevalidateConflicts` autenticam e usam escopo privado, epochs e versão do provider. I05.5 permanece aberto para preview exato das transições dinâmicas e adoção por importação/UI. Paths sensíveis persistidos seguem recusados até a integração I11.

- [x] I05.1 — Completar projeção de configuração global + workspace, condições, argumentos, tipos de acionador e apresentação; nenhuma leitura bruta vira autorização.
- [x] I05.2 — Implementar criar/editar/excluir/habilitar/desabilitar bindings e layers com ownership, validação de referências e CAS de geração.
- [x] I05.3 — Implementar restauração persistente por binding, camada e conjunto; não apagar defaults nem conceder grants.
- [x] I05.4 — Completar upgrade de defaults: versão sem mudança semântica, needs_review no contexto exato e rebase confirmado; eliminar bloqueio excessivamente amplo do protótipo.
- [ ] I05.5 — Expor serviço interno único de conflito/diagnóstico e diff exato, reutilizável por UI/chat/importação; testar corrida entre checagem e commit.

Critério de saída: Todas as operações de configuração previstas têm uma implementação transacional comum, sem escritores paralelos.

### I06 — Decisões e mutações de capacidade

Estado: **Parcial — implementação e integração local ampliadas em 15/09/2026**. Esforço restante: **G/GG**, conforme dependências abaixo.
Dependências: I04, I05.
Referências: D2, D10, D11; AEP-0091.

Evidência/limite atual: CRUD/defaults/restore, importação interna e regrant usam o writer comum com receipt/diff/auditoria; testes cobrem replay, rollback, alias, versão e semântica alteradas durante decisão. A fábrica do App compõe presenter e sessão reais e invalida o mapa antes do commit. Montagem no lifecycle e classificação/integração integral dos verbos continuam abertas; não confundir factory com publicação runtime.

- [x] I06.1 — Ampliar receipt para invocação e consumir decisão destrutiva na mesma transação do CAS para queued.
- [ ] I06.2 — Aplicar diff/receipt/auditoria a todo CRUD, restore, import e alteração de capacidade; incorporar a classificação obrigatória de cada verbo.
- [ ] I06.3 — Registrar presenter autenticado no ciclo apropriado; cancelamento, timeout, logout e resposta tardia não deixam autorização reutilizável.
- [ ] I06.4 — Revalidar gerações de configuração/grant entre apresentação e commit; negar origem headless onde o contrato exige interlocutor.
- [x] I06.5 — Testar decisão manipulada, replay, rollback e correlação; validar contrato do diálogo compartilhado sem duplicar UI.

Critério de saída: Nenhuma nova rota de configuração ou comando mutável precisa construir um mecanismo próprio de confirmação.

### I07 — Claims e ativação manual/contextual/temporária

Estado: **Parcial — implementação e integração local ampliadas em 15/09/2026**. Esforço restante: **G/GG**, conforme dependências abaixo.
Dependências: I03, I05, I06.
Referências: D8, D11.

Evidência/limite atual: `commandactivation` persiste regras/claims/gerações, implementa pin/toggle/back, expiração terminal e restore autenticado. `ReconcileLayersTx` e `NewActivationMutationHook` integram grants/claims/configuração no mesmo TX. Falta qualificar a composição com fontes reais e publicar o conjunto efetivo no host/projetor; uma lista fornecida por UI continua proibida. I07.6 permanece aberto até essa qualificação, mesmo com testes locais das primitivas.

- [x] I07.1 — Implementar schema/repository de regras, estado e referências builtin/user com isolamento global/workspace.
- [x] I07.2 — Implementar união de claims, pin, toggle/back e manual_stack_key derivada da origem; uma regra não encerra claim de outra.
- [x] I07.3 — Implementar expiração idempotente e estados terminais que não ressuscitam, inclusive após restart.
- [x] I07.4 — Implementar disable/enable com revalidação das claims e atualização atômica das gerações efetivas.
- [x] I07.5 — Implementar restore autenticado de claims manuais persistentes; rebind de sessão/gerações/dispositivo e revisão quando a origem não existir.
- [ ] I07.6 — Testar ciclos concorrentes, condições recalculadas, isolamento e reinicialização sem reativação indevida.

Critério de saída: Ativação deixa de ser lista fornecida ao projetor e passa a ser estado autoritativo com ciclo de vida completo.

### I08 — Grants exclusivos de automação de camadas

Estado: **Parcial — implementação e integração local ampliadas em 15/09/2026**. Esforço restante: **G/GG**, conforme dependências abaixo.
Dependências: I06, I07.
Referências: D8, D11; AEP-0101 como limite de separação.

Evidência/limite atual: `commandautomation` mantém tabela exclusiva, chave natural escopada e fingerprints. RegrantEventRule liga a concessão confirmada ao writer comum de configuração e ao hook real, com receipt, geração e auditoria na mesma transação. Criar/habilitar diretamente regra de evento sem grant continua proibido. Testes cobrem cancelamento da decisão, mudança semântica/versão e rollback, incluindo receipt não consumida após falha. O consumidor revalida grants por evento; resta qualificar a integração completa de importação/cópia/restore e montagem produtiva, sem transportar autoridade.

- [x] I08.1 — Persistir chave natural por owner/workspace/layer/rule, uma concessão ativa e histórico de gerações/revogações.
- [x] I08.2 — Criar/habilitar regra event-driven somente com decisão vinculada ao fingerprint exato da regra e dos produtores.
- [x] I08.3 — Revogar atomicamente ao alterar/excluir/desabilitar regra ou camada; reabilitação exige nova decisão.
- [ ] I08.4 — Revalidar ID, geração e fingerprints autoritativos a cada evento; import/cópia/restore nunca transportam concessão.
- [x] I08.5 — Testar concessão/revogação concorrente, receipt atrasada e isolamento entre grants de delegação e ativação.

Critério de saída: Automação possui autoridade explícita e revogável, sem herdar implicitamente as permissões do usuário.

### I09 — Fatos de jobs, outbox e replay durável

Estado: **Parcial — implementação e integração local ampliadas em 15/09/2026**. Esforço restante: **G/GG**, conforme dependências abaixo.
Dependências: I07, I08.
Referências: D2.1, D8, D11; AEP-0048, AEP-0067, AEP-0074-B.

Evidência/limite atual: `commandjobactivation` compõe lease de entrega, fato revalidado, regra/grant, CAS por sequência, ledger e ack no mesmo TX. Há lease própria de claim, renovação autenticada e reconciliação bounded; testes cobrem replay, conflito, fonte perdida, sessão antiga e ciclo compactado. O heartbeat concreto já compõe a passagem do coordinator com TTL atual e cursor; falta sua montagem automática no App e o bootstrap do epoch. Proveniência persistida valida a cadeia separada de comandos. Migração v26 preserva auditoria anterior. I09.4 reconhece operações transacionais testadas, não consumidor habilitado no App.

- [x] I09.1 — Migrar timeline/status incremental e queued_at/started_at conforme AEP-0048; persistir Job.DatabaseID, slug, run_event_id e root_origin_type sem inferência retroativa.
- [x] I09.2 — Inserir fato elegível e outbox na mesma transação; não usar cascade de runs como fronteira de replay.
- [x] I09.3 — Implementar epochs de política de replay e deadline imutável por ocorrência, preservado quando a retenção mudar.
- [x] I09.4 — Implementar consumo com lease, retry, delivered/dead_letter e processamento de cada regra/escopo por CAS de sequência; replay e conflito de fingerprint são distintos.
- [ ] I09.5 — Implementar lease/heartbeat da claim de job, reconciliação autoritativa e anti-loop com command_chain_history separado, limite versionado 16.
- [ ] I09.6 — Testar queda após commit, entrega duplicada/fora de ordem, count-cap, fonte perdida, raiz externa/unknown e fatos legados ambíguos; atualizar AEPs associados.

Critério de saída: Evento durável nunca perde sua barreira de replay pela limpeza do job e não concede capacidade por payload.

### I10 — Identidades, autorização e delegação entre runtimes

Estado: **Parcial — implementação e integração local ampliadas em 15/09/2026**. Esforço restante: **G/GG**, conforme dependências abaixo.
Dependências: I02, I04, I06.
Referências: D2.1, D10, D14, D16; AEP-0052, AEP-0063, AEP-0101.

Evidência/limite atual: `commandidentity` reconsulta a origem e usa `CoreEpochs` no mesmo domínio do executor; o executor ampliado possui portas local/external/job/system e a ponte de tools mantém o executor comum e redação. Faltam o JobRuntime real com grants AEP-0101, delegação para iniciar jobs, APIs de manutenção e adoção administrativa/middleware externa. Migração v25 prepara FK/unicidade do mapa, sem publicar readiness.

- [x] I10.1 — Implementar política autoritativa de comandos para usuário/agente/job/system, sem tratar allowed_source_types ou confirmação como autorização suficiente.
- [ ] I10.2 — Completar contexto job_service e grants de delegação exatos da AEP-0101; reconsultar owner/definição/profile/grant no gate final.
- [ ] I10.3 — Implementar ponte tipada para tools/jobs com correlação command_invocation, propagação de sensibilidade e redação; preservar executor comum e commandpolicy para shell.
- [ ] I10.4 — Implementar modo system restrito sem usuário e APIs privilegiadas de manutenção; proibir acesso a bindings e delegação que exige owner.
- [ ] I10.5 — Implementar mapeamento administrativo (issuer, subject), migração/readiness externa e revogação em conjunto com AEP-0052; nunca JIT/último token. Adapters físicos externos continuam proibidos sem a AEP futura do broker.
- [ ] I10.6 — Testar fronteiras de ator, cross-profile, externa/local, origem headless e ausência de interlocutor; atualizar AEP-0063 quando alterar origem/redação.

Critério de saída: Cada runtime recebe somente autoridade comprovada; nenhuma integração posterior precisa inventar identidade ou bypass.

### I11 — Importação, exportação e referências sensíveis

Estado: **Parcial — writer interno confirmado, sem montagem pública**. Esforço restante: **G**.
Dependências: I02, I05, I06, I08.
Referências: D10, D11; AEP-0047.

Evidência/limite atual: `commandportability` e `portability` registram commandLayers e planejam manter/substituir/copiar, com autorização de workspace, catálogo completo e patterns exatos. Contêineres `deltaOnly` preservam personalizações builtin sem camada artificial; camadas, defaults e regras usam portas distintas de referência. Grants/claims/histórico não são portáveis. O import/export genérico continua recusando o recurso. O writer interno confirmado e seu rollback agora são exercitados com hook real; montagem pública, multi-escopo e aceite agregado abaixo permanecem pendentes.

- [x] I11.1 — Versionar resources.commandLayers no envelope da AEP-0047 e implementar round-trip de deltas/needs_review e escopo portátil.
- [ ] I11.2 — Resolver UUIDs, refs builtin/user e mapa de workspaces no destino autenticado; conflito foreign_owner não revela conteúdo.
- [ ] I11.3 — Implementar manter/substituir/cópia com remapeamento transacional; nome conflitante exige escolha explícita.
- [x] I11.4 — Excluir grants, claims, defaults puros e histórico; regra event-driven importada fica sem concessão e desabilitada.
- [ ] I11.5 — Validar referências exatas de credenciais por pattern, não IDs locais; separar export sem segredo e export sensível UI-only com criptografia/decisão da AEP-0047.
- [ ] I11.6 — Testar importação repetida, referência ausente, dados antigos com segredo bruto e rollback do lote; atualizar AEP-0047.

Critério de saída: Backup/restore de configuração não transfere permissões nem introduz segredos em bindings.

### I12 — Recuperação e manutenção de toda a instância

Estado: **Parcial**. Esforço restante: **G**.
Dependências: I04, I07, I09, I10.
Referências: D2.1, D8, D11; AEP-0074-B.

Evidência/limite atual: `commandmaintenance.Coordinator` exige portas/política completas e aceita heartbeat para montagens com claims. Ordena heartbeat, outbox, recuperação, jobs, tools, auditorias e compactação. More/erro impede limpeza. O Manager configurado relê settings por passagem, usa um único timer adaptativo e aguarda sua drenagem no Stop. `ConfigureCommandMaintenance` monta adapters legados reais antes de Start. Recuperação pagina usuários/system com prova real do processo atual e preserva progresso em erro. As seis settings estão persistidas e expostas na UI/i18n. Ainda faltam prova para restart, montagem de receipts e composição automática dos domínios no App; marcador de banco sozinho não prova exclusão.

- [ ] I12.1 — Definir prova de encerramento de geração e exclusão de execuções antigas antes de recuperar pendências, incluindo reinício e outros usuários/sessões.
- [ ] I12.2 — Reconciliar invocação+ledger atomicamente para outcome_unknown, incluindo system com capability interna; jamais reexecutar efeito.
- [ ] I12.3 — Integrar recuperação de receipts de sessões abandonadas, claims e leases com lotes, cancelamento e critérios de término.
- [ ] I12.4 — Migrar a cadência de retenção para um único InstanceMaintenanceCoordinator; preservar limpezas legadas e compactação, com outbox antes da retenção de jobs.
- [x] I12.5 — Implementar idade/caps sem remover ledger antes do prazo, nem estado ativo; adicionar as seis settings previstas e UI/i18n correspondente.
- [ ] I12.6 — Testar múltiplos usuários/system, interrupção entre lotes, retenção alterada, compactação e ausência de dois loops; atualizar AEP-0074-B.

Critério de saída: Reinício e limpeza têm um único dono, cobrem toda a instância e não reabrem execução ou ativação antiga.

Complemento da rodada atual: `EpochService.CloseAndDrain` produz prova real do
processo atual e `Store.SealDrainedGeneration` a liga à recuperação já existente.
New/NewComplete registram o lifecycle compartilhado mesmo quando Service é copiado.
Provas vazias ou de outro core são recusadas; escopos local/system são selados
idempotentemente. Reinício de processo ainda impede encerrar I12.1; a composição
produtiva all-users/system ainda impede o aceite agregado de I12.2. O banco não
ganhou outra implementação de recuperação nem migração de schema nesta rodada.

### I13 — Infraestrutura das pontes e adapters físicos

Estado: **Parcial — ponte UI/backend, escopo de diálogo e observador de sessão**. Esforço restante: **GG**.
Dependências: I03, I04, I06, I07.
Referências: D3, D7, D13, D14; AEP-0080, AEP-0091.

Evidência/limite atual: `commandbridge` e `frontend/src/lib/commandBridge.ts` definem sessão, capabilities, ack/resultado/cancelamento, geração transportada como string, UUIDv7 de invocação/evento, `sourceEventId` UUIDv7 opcional e `DialogCommandProof`/`DialogProof` com correspondência exata no resultado. `internal/app/app_command_bridge.go` monta a ponte no App e expõe invoke/input/result/cancel/lifecycle por métodos Wails, revalidando usuário/sessão autenticados antes do handoff. `frontend/src/lib/commandBridgeWails.ts` transporta o contrato sem editar bindings gerados. `internal/commandadapter` fornece lifecycle genérico para listeners físicos futuros: callbacks com geração, suspensão por lock/logout, shutdown terminal, sourceEventId por ciclo aceito, sequências com timeout e handoff por `commandbridge.Input`, sem handler final. Shutdown idempotente invalida antes de cancelar, espera lotes admitidos e libera recursos; uma chamada concorrente Go pode cancelar sua espera. Testes cobrem isolamento, handoff curto, repetição, lock/logout, transporte App/Wails, lifecycle de adapter, UUIDv7 de ocorrência, sequência Ctrl+N genérica e reserva de diálogo topmost. `OccurrenceID` físico segue opaco para ownership claim; `sourceEventId` representa a ocorrência durável do ciclo aceito. O adapter de decisões usa a fila real de questionários com cancelamento por ID; o scope acompanha o stack real de Modal e `decision.respond` só atravessa com prova topmost local. Não há gerenciador HID montado nem migração dos handlers reais de decisão. Portas devem cumprir o contrato de cancelamento; não há promessa de prazo de shutdown para porta defeituosa.

- [x] I13.1 — Fechar ponte tipada de despacho UI com ack/resultado/cancelamento, sessão e invocation_id; registrar capabilities sem handlers reais migrados.
- [x] I13.2 — Implementar ownership local/global por geração, ocorrências UUIDv7, repeat/release/blur/reconexão e contrato de sequências Ctrl+N do inventário.
- [x] I13.3 — Integrar DialogCommandScope ao stack real e reservar invariantes de decisão antes de bindings/ownership, respeitando input/IME e registro global temporário.
- [x] I13.4 — Implementar ciclo de vida genérico de adapter, callbacks com geração, suspensão por lock/logout e shutdown; nenhum listener chama handler final.
- [x] I13.5 — Validar biblioteca/licença/build/modelos HID e implementar gerência de dispositivos com exclusividade, reconexão/backoff e estado seguro; renderer com cache/diff e frame completo após reabrir.
- [ ] I13.6 — Validar teclado/foco/janela e ao menos um Stream Deck real; falha de hardware não derruba App. Registrar explicitamente dependência de dispositivo e ambiente.

Critério de saída: As entradas e a ponte UI cumprem contratos do núcleo antes de receber a população de comandos do aplicativo.

Complemento da rodada atual: o scope acompanha o Modal real e o topo da pilha.
Alteração de scope é atualização in-place, sem reempilhar uma instância inferior.
`decision.respond` só atravessa a composição autenticada quando a prova aponta
para o diálogo topmost atual, com origem/ownership locais e trigger reservado;
input/IME/editáveis continuam ignorados antes de reserva. O registro do scope não
concede autoridade backend sozinho e não migra handlers de teclado.

### I14 — Montagem final no ciclo de vida do App

Estado: **Parcial — hooks e fábricas sem bootstrap de produto**. Esforço restante: **G**.
Dependências: I01, I02, I03, I04, I05, I06, I07, I08, I09, I10, I11, I12, I13.
Referências: D2.1, D8, D11, D13.

Evidência/limite atual: `commandruntime.Controller` serializa bootstrap, invalida/cancela antes do reset e confirma readiness via core compartilhado; exige todas as portas e recusa projeção vazia. `App.commandLifecycle` é um ponteiro atômico privado, com montagem serializada, barreira terminal de shutdown e desmontagem por CAS, sem registry global. Os testes cobrem falhas/cancelamento e configurações concorrentes. As chamadas de startup/login/refresh/logout/shutdown estão ligadas condicionalmente ao controller; shutdown espera o worker e mantém dependências em caso de timeout. Ainda faltam providers e montagem real completa: não publicar readiness de produto a partir dos mocks.

- [x] I14.1 — Construir esqueleto de bootstrap serializado e readiness observável após I01, sem expor novas rotas nem cadastrar comandos de produto; este subitem pode começar cedo.
- [ ] I14.2 — Montar catálogo/defaults de contrato, políticas, stores, presenter, providers, dispatcher e adapters com dependências explícitas; sem fallback permissivo.
- [ ] I14.3 — Orquestrar login/unlock/restart: autenticar → recuperar/reconciliar → carregar/projetar → publicar → habilitar entradas, revalidando cada transição.
- [ ] I14.4 — Impedir retomadas concorrentes e publicação de geração antiga; logout/troca de usuário/falha de monitor cancela trabalho e apaga somente estado em memória.
- [ ] I14.5 — Integrar shutdown, drenagem/cancelamento e manutenção sem goroutines órfãs, mutex durante UI/cofre ou cadências duplicadas.
- [ ] I14.6 — Testar instalação nova, upgrade, restart com pendência, falhas em cada etapa e retomada após erro; nunca mascarar indisponibilidade como mapa vazio pronto.

Critério de saída: A base completa nasce, funciona e encerra dentro do App, ainda sem migrar os comandos existentes.

Complemento da rodada atual: App.Shutdown fecha/drena o core mesmo sem controller,
preservando dependências se falhar. Instalação de HostState/monitor compartilha a
barreira terminal de montagem; uma construção rejeitada fecha o Service ainda não
publicado. A fábrica completa continua interna e restrita a sessão local; sua
existência não equivale a entradas habilitadas nem a providers UI autenticados.

### I15 — Qualificação e aceite da infraestrutura

Estado: **Parcial — testes focados e benchmarks existentes**. Esforço restante: **G**.
Dependências: I14.
Referências: Fase 0, D12, riscos e critérios transversais.

Evidência/limite atual: TestMain de App/config isola dados pessoais e a suíte global foi executada; falhas de processo em ACP/acpregistry permanecem. Frontend completo, TypeScript, build/vet e lint foram exercitados. Não há comprovação integral de p95, race, hardware/NVDA ou review de entrega. Microbenchmarks não substituem latência integrada.

- [x] I15.1 — Isolar os testes legados que escrevem configuração pessoal; disponibilizar suíte geral reproduzível em dados temporários.
- [ ] I15.2 — Executar backend completo, detector de corrida em ambiente com C/CGO e lint compatível v2; verificar frontend/Wails gerado quando as pontes mudarem.
- [ ] I15.3 — Medir p50/p95/p99 no caminho integrado com handlers de teste, incluindo SQLite/gate sob mutações, foco e carga; decidir tratamento de contenção contra a meta experimental p95 < 1 ms.
- [ ] I15.4 — Executar matriz de crash/replay/isolamento/segredos e testes acessíveis da ponte/diálogos; registrar verificações reais de SO, HID e NVDA.
- [ ] I15.5 — Encerrar achados de review local Bugbot antes de push e requisitos de CI/review quando houver PR; não declarar review feita se a ferramenta não estiver disponível.
- [ ] I15.6 — Reconciliar todos os critérios de infraestrutura deste plano com evidência e registrar aceite do marco BASE-PRONTA; os critérios de produto permanecem abertos até P01–P06.

Critério de saída: As garantias estão demonstradas na montagem real; pendências de produto não escondem dívida estrutural.

## 5. Ordem de execução e paralelismo

Primeira frente: **I01 + I02**, em arquivos distintos. Em paralelo, antecipar o isolamento de testes de **I15.1** e a verificação de disponibilidade de hardware/build de **I13.5**. Esses subitens preparatórios não encerram I13/I15 nem autorizam alterações reais de credenciais nesta rodada de planejamento.

Depois:

1. I03 fecha o contexto/escopo comum.
2. I04 e I05 avançam em paralelo; I06 fecha a transação de decisão.
3. I07 e I10 podem avançar em paralelo; I08 segue I07.
4. I09 e I11 avançam depois das suas dependências. I13 pode avançar sem esperar import/export.
5. I12 consolida recuperação/manutenção; I14 completa montagem de todos os componentes.
6. I15 fecha a qualificação; então BASE-PRONTA.

Uma trilha longa de dependências é I02 → I03 → I04/I05 → I06 → I07 → I08 → I09 → I12 → I14 → I15. A trilha de I10 e a validação física também podem governar o prazo. Não é uma estimativa matemática de caminho crítico sem durações calibradas.

Delegação: usar modelos econômicos para implementações/testes com escopo de escrita disjunto. O agente principal mantém contratos, integração e revisão. Não abrir seis frentes por número: abrir só as independentes; schemas compartilhados e ordem de locks têm um responsável por vez.

## 6. Dependências externas e riscos conhecidos

- **R01 — Testes pessoais (mitigado em I15.1):** App/config agora têm TestMain isolado e a suíte global foi exercitada com home temporário. Preservar esse ambiente ao repetir a validação; não executar contra configuração pessoal.
- **R02 — Detector de corrida/lint (parcialmente mitigado):** lint v2.11.4 passou globalmente nesta rodada; GCC/CGO para race continuam ausentes. I15.2 permanece aberto.
- **R03 — Revisão:** disponibilidade de Bugbot ainda não comprovada. Revisão de outro agente não substitui a exigência do repositório. Resolver antes de push.
- **R04 — Hardware/SO:** identificar modelo de Stream Deck e obter janela de teste real para reconexão, lock/unlock e disputa pelo dispositivo. Mocks não encerram I13.6.
- **R05 — NVDA:** reservar validação manual acessível da infraestrutura compartilhada e, depois, das telas finais. Dependência do ambiente/usuário, não do banco.
- **R06 — Desempenho:** microbenchmarks de resolução não cobrem espera pelo gate/SQLite. I15.3 pode revelar necessidade de ajuste estrutural; esse risco já pertence ao escopo, não deve surgir como surpresa.
- **R07 — Migrações transversais:** I09, I10, I11 e I12 exigem alterações coordenadas em AEP-0048, AEP-0052, AEP-0063, AEP-0047 e AEP-0074-B. Não são tarefas de “só ligar uma chamada”.
- **R08 — Canonicalização e compatibilidade:** I02 exige identidade estável entre versões e sem confundir argumentos diferentes depois da redação.
- **R09 — Prova de encerramento:** recuperar ledger antigo sem excluir execução ainda viva pode classificar efeito incorretamente. I12.1 precisa de desenho/testes explícitos.
- **R10 — Escopo integral:** a inicialização isolada I01/I14 não encerra a infraestrutura inteira. Não usar “bootstrap pronto” como sinônimo de BASE-PRONTA.

Esses riscos não são todos bloqueios atuais. Hardware, NVDA e ferramentas precisam ser encaminhados cedo; trabalho de código independente continua.

## 7. Migração e entrega de produto — depois da base

Esta seção mantém visível o restante do AEP. Não foi removida nem contabilizada como infraestrutura pronta. Estados iniciais: pendentes no novo sistema; componentes antigos existentes serão reaproveitados.

### P01 — Popular o catálogo e migrar comandos

Depende de BASE-PRONTA. Referências: fases 1–2 e inventário de atalhos.

- [ ] P01.1 — Revisar o inventário e cadastrar contratos/handlers reais de workspace, chat, editor, terminal, tasklists, menus e diálogos, incluindo defaults localizados.
- [ ] P01.2 — Migrar teclado local, sequências e hotkeys de perfis/jobs usando os adapters comuns.
- [ ] P01.3 — Demonstrar equivalência por surface/foco/input e retirar handlers paralelos somente após a cobertura correspondente.

### P02 — Command Palette

Depende de BASE-PRONTA e catálogo real P01.

- [ ] P02.1 — Busca/aliases, disponibilidade com motivo, atalho efetivo, recentes/favoritos e navegação para configuração.
- [ ] P02.2 — Formulários de argumentos e execução pelo serviço único.
- [ ] P02.3 — Combobox/listbox, foco, anúncios, teclado/axe e validação NVDA.

### P03 — Configuração acessível e ajuda

Depende de BASE-PRONTA e P01.

- [ ] P03.1 — Lista/detalhe de camadas, ativação, bindings, captura de teclas e conflito explicado.
- [ ] P03.2 — Fluxos de restore/rebase/needs_review, prioridades em divulgação progressiva e ajuda derivada do mapa efetivo.
- [ ] P03.3 — Ações operáveis por lista/teclado, sem depender de imagem, drag ou cor; pt-BR/en/es, axe e NVDA.

### P04 — Experiência de dispositivos e contexto externo

Depende de BASE-PRONTA e P01/P03.

- [ ] P04.1 — Mapas reais no Stream Deck, imagens/títulos/estados, navegação por camadas e múltiplos dispositivos.
- [ ] P04.2 — Diagnóstico de disputa/reconexão e estado seguro acessível na UI; validar uso sem software oficial.
- [ ] P04.3 — Camadas por programa em foco e fixação/estabilização com comandos reais, sem acrescentar controle privilegiado não autorizado.

### P05 — Entradas de chat/CLI e portabilidade

Depende de BASE-PRONTA e P01.

- [ ] P05.1 — Expor command_catalog e command_config com ações fechadas, IDs reais e decisões do serviço comum.
- [ ] P05.2 — CLI list/describe/execute/retry com request ID e indisponibilidade explícita para comandos visuais/interativos.
- [ ] P05.3 — Integrar fluxos de export/import e relatórios na UI/tools permitidas, sem export sensível pelo chat.
- [ ] P05.4 — Ligar eventos/jobs e delegações aos comandos reais preservando grants, proveniência e auditoria.

### P06 — Aceite final e entrega

Depende de P01–P05.

- [ ] P06.1 — Reexecutar regressões ponta a ponta, desempenho com ações reais e matriz multiusuário/dispositivo.
- [ ] P06.2 — Concluir docs de usuário, acessibilidade manual e todos os 83 critérios finais com evidências.
- [ ] P06.3 — Zerar review local, CI/review remota e registrar PRs; merge continua decisão do mantenedor.
- [ ] P06.4 — Marcar AEP/índice Done somente após todo escopo aceito concluído.

### F01 — Expansões e limites já previstos, não pendências escondidas

A fase 7 pede avaliar pedais USB, MIDI e gestos/dial. Registrar avaliação/capabilities e decisão; ela não obriga implementar todo dispositivo.

Broker de identidade externa para adapters físicos, novos produtores externos de eventos e injeção privilegiada de teclas exigem AEP/decisão posterior. Permanecem indisponíveis no escopo atual, como determina a AEP-0103; não foram “cortados por prioridade”. Avaliação da biblioteca/modelo do Stream Deck básico, ao contrário, está dentro de I13.

## 8. Política de atualização e evidências

IDs Ixx.n/Pxx.n não mudam quando a ordem mudar. Se um item se dividir, manter o ID pai e criar subitens; não apagar o histórico nem inflar progresso.

Registrar cada encerramento neste formato:

> ID · data · commit · arquivos/símbolos · testes executados e resultado · revisão · limitações restantes.

Implementação aprovada em teste isolado, mas sem integração exigida pelo item, permanece parcial. Uma correção incidental não precisa de novo pacote: entra como subitem do contrato afetado.

Toda descoberta adicional recebe **Δnn** com origem (critério do AEP, defeito de implementação ou expansão), pacote afetado, impacto e decisão. Mudança de escopo/arquitetura exige alinhamento; requisito já previsto e omitido exige corrigir a baseline explicitamente. Nunca adicionar trabalho silenciosamente.

### Registro inicial

- 14/09/2026 — Baseline v1 criada sobre 11c10c578. 15 pacotes / 84 itens de infraestrutura / 4 marcos.
- Evidências históricas E01–E10 reconhecidas somente no seu subconjunto.
- Nenhum item I/P encerrado por esta rodada documental.
- Sem mudança de código de execução, migração real, provisionamento de segredo ou ativação no App.
- Δ: nenhum acréscimo após esta baseline.

### Próxima atualização obrigatória

Consolidar a rodada autorizada I02–I04, mantendo abertos os critérios dependentes de integração. Antes de iniciar I05, explicitar a porta autenticada ainda necessária em I03.1; não confundir a implementação local do executor com montagem final I14. A calibração observada de I01/I02 está registrada abaixo.

### Entrega I01 — 14/09/2026

- I01.1–I01.4 · commit `021d18e07` · implementação e testes revisados localmente pelo agente principal, com três subagentes em credenciais e testes independentes.
- Migração v20 no registro central, adiada na abertura genérica e concluída transacionalmente pelo host; aceita schema experimental conhecido, rejeita drift/colisão, preserva dados e carimbo em reabertura.
- Segredo de fingerprint dedicado, cifrado no cofre existente, criação insert-if-absent e releitura do vencedor; sem reaproveitar JWT/pepper. Metadata UUIDv7 fixa digest/versão ativa. Falhas de cofre/chave/schema não publicam prontidão.
- Rotação v1→v2 com CAS, preservação da assinatura v1 pelo provider real, rollback e reaproveitamento da chave órfã no retry; nenhuma chave antiga é excluída.
- Validação: todos os 12 pacotes `internal/command*` passaram com `-count=3`; bootstrap repetido novamente após os últimos testes. Suites focadas de credenciais, registro/migração e App passaram. `go build -mod=readonly ./...`, `go vet -mod=readonly ./...` e `git diff --check` passaram.
- Testes usaram SQLite temporário e DEKs sintéticas. Não houve abertura do app com dados reais nem acesso deliberado a segredos reais. A suíte global `go test ./...` não foi executada por haver testes legados do App que escrevem configuração compartilhada; isolamento e qualificação completa continuam em I15. Sem testes frontend nesta mudança exclusivamente backend.
- Bugbot, race detector, corpus completo de upgrades publicados, CI e review remota: não executados nesta entrega; sem push/PR. Não equivaler revisão local do agente a essas aprovações.
- Esforço observado: três frentes delegadas (criação atômica, testes de schema e testes de chaves), composição/rotação/App e revisão central; correções de ordem não determinística de constraints do GORM e ID UUIDv7 incluídas no próprio I01. Sem aumento da baseline; previsão de calendário ainda aguarda I02.
- Próximo pacote: I02 — contratos completos de catálogo, documentos e fingerprints. AEP permanece In Progress; comandos atuais não migrados.


### Entrega I02 — 14/09/2026

- I02.1–I02.5: catálogo completo opt-in, mutabilidade/classificação conferidas contra handler, schemas fechados, envelope completo, validação estrita e fingerprint semântico com política/decisão/contexto.
- JCS com vetores numéricos RFC 8785, UTF-16, duplicatas recursivas, limites, Unicode inválido e underflow não suportado. Domínios separados para request/argumentos; signer legado preservado.
- Defaults calculados antes da publicação; apresentação não muda a semântica. Corpus lexical único consumido por ingresso e documentos persistidos; importação I11 reutilizará o contrato.
- Testes de commandjson/catalog/contract/config passaram, incluindo repetição count=2. Revisão central corrigiu vinculação da política, enum JSON aninhado e separação nullable/enum. Sem comandos de produto migrados; Bugbot/CI/race continuam pendentes.
- Calibração: I02 demandou três frentes independentes e integração/revisão central, com correções de contratos entre componentes. O custo dominante foi composição/revisão, não digitação. Não há ainda amostra suficiente para converter I03–I15 em dias com faixa defensável; estimativas G/GG permanecem, sem promessa de número de interações.

### Entrega local I03 — 14/09/2026

- I03.2–I03.6: FactBus escopado com provas opacas, releitura síncrona e notificações apenas indicativas; gerações global/workspace/camadas, cache positivo/negativo isolado e rejeição de provider ausente/panic. A admissão do executor I04 consome a mesma prova, sem roundtrip UI/rede dentro do gate.
- Snapshots do Manager real incluem mudanças em abas inativas, sem exportar conteúdo. Foreground Windows preserva identidade do processo e timestamp capturado antes do foco; resumo não contém títulos/URLs. Outras plataformas falham explicitamente como indisponíveis.
- Frontend: stack modal versionada, foco sem conteúdo de inputs, registro explícito de superfície e leitura vinculada aos stores reais de autenticação/workspace. Revalida owner antes/depois do getter; logout e troca de sessão/workspace não reutilizam registro antigo. Nenhuma ponte aceita payload da UI como principal autenticado.
- I03.1 permanece aberto: falta ponte autenticada e registro das superfícies reais. As factories privadas do App não equivalem à montagem I14. Não há promessa de latência ponta a ponta nem validação física/NVDA nesta rodada.
- Validação: commandcontext, commandforeground e workspace passaram; testes focados do App passaram. TypeScript e ESLint dos arquivos tocados passaram. Vitest: 7 arquivos/90 testes passaram, incluindo contextos, Modal, DecisionDialog, useVirtualModal e workspaceChatModalStore. Dependências copiadas para o worktree de instalação existente com package-lock idêntico; principal não alterado. As expectativas de dois testes novos foram corrigidas para cobrir freeze e duas releituras explícitas, sem remover testes.
- Δ01 — incompatibilidade de implementação já existente: workspaces/abas reais têm IDs opacos, enquanto o schema experimental de commandconfig exige UUIDv7 para workspace. I03 preserva os IDs reais; adaptar a projeção/migração de configuração em I05, sem renomear dados pessoais. Não é expansão de produto nem autorização para migrar a instalação real nesta rodada.

### Entrega local I04 e antecipação I06.1 — 14/09/2026

- I02 commit `daf8036c7`; I03 commit `7736e63ee`. Esta seção acompanha o commit temático de I04; hashes finais ficam na fotografia de entrega e no histórico Git.
- O mesmo Service recebe candidato sem identidade confiável, deriva owner/ator, consulta a reentrega antes de resolver, normaliza argumentos e assina o envelope. Snapshot/Resolve/Authorize são portas locais do bootstrap; não recebem autoridade de Wails. Gate revalida catálogo, mapa, epochs, alvo e providers antes da fila e do Start; espera, diálogo e resultado ficam fora do gate.
- Ledger/auditoria compartilham reserva e transições transacionais. Ownership inclui ator, request tem HMAC de ingresso para reentrega sem argumentos brutos; lookup reautoriza independentemente do catálogo atual. Suppress/rejected_stale não executam após mudança de configuração. Erro após handoff ou resultado fora do schema produz outcome_unknown, não retry de efeitos. Callback/handler recebe envelope destacado para não alterar a solicitação assinada.
- I06.1 antecipado por necessidade de I04: receipt subject invocation vinculado à solicitação/owner/epochs e consumido uma única vez na transação evaluating→queued. Expiração menor que a retenção é aceita; falha do CAS reverte consumo. Presenter usa o diálogo existente, ação Executar e escopo current; efeito destrutivo seleciona severity destructive. Chaves de UI nos três idiomas, sem novo diálogo paralelo.
- Δ02 — requisito persistente de I04/I06.1: migração central v21 acrescenta actor_type/actor_id/input_fingerprint ao ledger e subject invocation ao receipt. Upgrade v20 conhecido preserva dados/índices/carimbos; drift falha fechado e rollback é testado, inclusive colunas antigas em ordem diferente. Não foram criados segredos adicionais nem migrado banco pessoal nesta validação.
- Δ03 — dependência de aceite antes implícita na baseline: I04 pede todos os contextos, mas I10 depende de I04; I03 pede fontes UI reais, mas a montagem está em I14. A implementação pode avançar em sequência, porém o fechamento integral desses pacotes precisa da integração posterior. Mantidos os IDs e critérios originais abertos; não se reduziu a definição de pronto nem se acrescentou feature.
- Testes passaram em todos os 15 pacotes command* e em workspace; executor completo repetido count=2 após revisão de isolamento dos callbacks. Passaram testes focados de App/contextos/presenter e database/migrações, build/vet globais e diff-check. Frontend: tsc, lint dos arquivos tocados e 90 testes focados/regressão passaram. Testes usam SQLite/diretórios temporários e credenciais sintéticas; segredos reais não foram usados deliberadamente.
- Revisão central integrou seis frentes delegadas em modelos econômicos, corrigiu contratos cruzados, deadline de replay, upgrade preservador de dados e imutabilidade. O custo dominante continuou sendo integração/verificação; não há base honesta para promessa de dias/turnos por pacote.
- Limites de qualificação: go test ./... não executado porque testes legados do App escrevem configuração compartilhada; race indisponível sem compilador C; suíte frontend completa, stylelint global, Bugbot, CI, validação física/NVDA e review remota não executados. Sem push/PR/merge. Comandos existentes não migrados e executor ampliado não ativado no App.
- Próximo trabalho concreto: fechar a ponte autenticada de I03.1 com prova de ownership/freshness sem chamada bloqueante à UI dentro do gate; depois I05 (projeção e CRUD persistidos). Em I10 concluir ingresso/ator dos demais contextos e, em I14, montar/revalidar o conjunto. Não requer nova decisão do usuário para os subitens técnicos já aprovados, mas não conta como três pacotes encerrados nesta rodada.

## 9. Rastreabilidade integral dos critérios de aceitação

Os IDs C01–C83 correspondem à ordem dos critérios no AEP em 11c10c578. Texto preservado nesta baseline. Eles são referências de acompanhamento, não uma nova numeração normativa. Se o AEP mudar, reconciliar por texto/contrato e registrar Δ, sem deslocar os IDs antigos silenciosamente.

Todos permanecem abertos como critérios finais. O mapeamento indica onde construir e demonstrar a garantia, não afirma que o critério já passou. I15/P06 também são responsáveis pela verificação transversal.

### C01

Existe registro canônico e pesquisável de comandos com IDs, argumentos, disponibilidade, risco, aliases localizados e apresentação.

Responsáveis: I02 / P01.

### C02

Teclado local, hotkey global, Stream Deck, Command Palette, chat e CLI podem convergir para o mesmo comando sem handlers finais duplicados.

Responsáveis: I04 / I13 / P01 / P05.

### C03

Todo acionamento que resolve para execução produz `CommandInvocation` e passa por `CommandExecutionService`, com sessão, proveniência, autorização, deduplicação e auditoria antes do handler final; `effect = suppress` é consumido sem criar invocação.

Responsáveis: I04.

### C04

A reserva atômica por evento impede reentrega, e ownership exclusivo impede duplicidade entre teclado local/global e listeners de dispositivo.

Responsáveis: I04 / I13.

### C05

Solicitações diretas e triggers sem `source_event_id` usam `invocation:<invocation_id>`; eventos usam `event:<source_event_id>`.

Responsáveis: I04.

### C06

Manter uma tecla pressionada não repete comando: o adapter descarta `KeyboardEvent.repeat`/repetição nativa antes de gerar `source_event_id` e testes cobrem release, blur e reconexão.

Responsáveis: I13.

### C07

Retirada de `queued` revalida todos os gates no mesmo CAS para `running`.

Responsáveis: I04.

### C08

Cada instância física usa geração própria e índice parcial de eventos; invocações diretas deduplicam somente pela PK UUIDv7.

Responsáveis: I04 / I13.

### C09

Execução por agente e automação preserva e revalida os gates da AEP-0101; origem headless não herda a identidade do usuário para autorizar mutações.

Responsáveis: I10.

### C10

Usuário e ator são derivados pelo backend; payload não escolhe identidade de autorização/auditoria.

Responsáveis: I04 / I10.

### C11

Camadas padrão do aplicativo e das surfaces permanecem ativas e um binding ausente em camada superior cai para o default.

Responsáveis: I05 / P01.

### C12

Overrides afetam somente o acionador e contexto declarados.

Responsáveis: I05.

### C13

Tombstone bloqueia o default no contexto declarado, enquanto personalização apenas desabilitada permite fallback.

Responsáveis: I05.

### C14

Tombstones são aplicados antes da deduplicação e nunca produzem invocação.

Responsáveis: I04 / I05.

### C15

Tombstone que consome um acionador grava marcador terminal no ledger; reentrega do mesmo evento não passa a executar um default após mudança de configuração.

Responsáveis: I04.

### C16

Acionamento stale não grava `suppressed`, mas recebe marcador terminal `rejected_stale`; o mesmo ID nunca executa em reentrega posterior.

Responsáveis: I04.

### C17

Override de default persiste ID e versão do default substituído.

Responsáveis: I05.

### C18

É possível restaurar um binding, uma camada ou todas as personalizações.

Responsáveis: I05 / P03.

### C19

Conflitos são detectados considerando a possível interseção de contextos, e empate não executa dois comandos.

Responsáveis: I05.

### C20

Escopo, especificidade e prioridades persistidas produzem resolução determinística após importação/restart; empate termina em conflito fail-closed.

Responsáveis: I05 / I11.

### C21

Bindings equivalentes por comando, argumentos e escopo produzem uma única invocação com proveniência preservada.

Responsáveis: I04 / I05.

### C22

O resolvedor não consulta SQLite nem percorre o catálogo completo a cada acionamento.

Responsáveis: I03 / I15.

### C23

Mudanças de surface, foco, workspace, janela externa e eventos podem ativar e desativar camadas de forma determinística.

Responsáveis: I03 / I07 / I09 / I13.

### C24

Desabilitar camada a remove imediatamente do mapa sem ressuscitar claims stale ao reabilitá-la; expiração local é idempotente após restart.

Responsáveis: I07.

### C25

Ativações por evento têm ID, sequência, correlação e deduplicação; evento atrasado não encerra ciclo mais novo.

Responsáveis: I07 / I09.

### C26

Claim e ledger de ativação preservam o escopo global/workspace, inclusive para refs `builtin`; eventos e replay de outro workspace falham fechado.

Responsáveis: I07 / I08 / I09.

### C27

A primeira versão aceita apenas fatos de `job_run_events` espelhados transacionalmente na outbox durável; EventBus best-effort e produtores externos falham fechado.

Responsáveis: I09.

### C28

Count-cap/cascade de runs não remove a outbox antes do deadline; startup recupera leases e reprocessa pendências antes da retenção de jobs.

Responsáveis: I09 / I12.

### C29

Estado de ativação persistido é reconciliado em modo seguro no startup e preserva autenticação, geração e proveniência anti-loop da AEP-0067.

Responsáveis: I07 / I09 / I12 / I14.

### C30

Claim de job sem lease e fonte autoritativa válidas fica inativa.

Responsáveis: I09 / I12.

### C31

Replay de ativação fora da retenção é rejeitado, e ownership vem do principal autenticado, não do payload.

Responsáveis: I09 / I12.

### C32

A Command Palette busca e descreve comandos disponíveis e indisponíveis com motivo, mas executa somente os disponíveis.

Responsáveis: I02 / P02.

### C33

A Command Palette tem navegação completa por teclado, anúncios e restauração de foco cobertos por testes e validação NVDA.

Responsáveis: P02 / P06.

### C34

A configuração por chat usa tools estruturadas, IDs reais e confirmações de segurança.

Responsáveis: I05 / I06 / P05.

### C35

Toda mutação persistente solicitada por agente mostra diff, exige decisão explícita e falha fechado sem interlocutor.

Responsáveis: I06 / I10.

### C36

`command_catalog.execute` aplica o mesmo gate a comandos que alteram capacidade efetiva, incluindo ativação de camada.

Responsáveis: I06 / I10 / P05.

### C37

A tela de configuração oferece lista de camadas, detalhe de ativação e bindings, captura de teclas e explicação do resultado efetivo.

Responsáveis: I05 / P03.

### C38

Toda configuração é operável por teclado e NVDA sem depender de grade, arrastar, imagem ou cor.

Responsáveis: P03 / P06.

### C39

O Assistente controla ao menos um modelo de Stream Deck diretamente por Go, sem software oficial, com reconexão e shutdown limpo.

Responsáveis: I13 / P04.

### C40

Sem sessão autenticada, e durante logout ou troca de usuário, o Stream Deck fica em estado seguro e rejeita callbacks de gerações anteriores.

Responsáveis: I13 / I14.

### C41

O Stream Deck atualiza somente teclas cujo conteúdo efetivo mudou e usa cache de imagens.

Responsáveis: I13 / P04.

### C42

Abertura/reconexão do Stream Deck invalida o diff e força frame completo.

Responsáveis: I13 / P04.

### C43

Camadas baseadas no programa em primeiro plano funcionam no Windows e degradam explicitamente em plataformas sem adapter.

Responsáveis: I03 / I13 / P04.

### C44

Contexto externo é capturado antes de bring-to-front e não muda no meio do acionamento.

Responsáveis: I03 / I13.

### C45

Comandos disparados fora de foco preservam permissões, decisões e auditoria do executor de destino.

Responsáveis: I04 / I06 / I10 / I13.

### C46

Exportação/importação preserva UUIDs e escopos, relata referências e conflitos e não transfere grants nem histórico de invocações.

Responsáveis: I11 / P05.

### C47

Binding persistente e export não contêm segredos brutos; delegação a tool propaga redação ou permanece indisponível.

Responsáveis: I02 / I10 / I11.

### C48

Referência importada de credencial resolve pattern exato no usuário de destino ou deixa o binding desabilitado.

Responsáveis: I11.

### C49

`command_invocations` tem payload redigido, origem rastreável, índices e retenção por idade e quantidade, sem prometer reconstruir o snapshot completo.

Responsáveis: I04 / I12.

### C50

`command_invocations.invocation_id` é a PK canônica da invocação, consulta e correlação com tools; o ledger tem PK própria `id` e referências UNIQUE explícitas.

Responsáveis: I04 / I10.

### C51

Reentrega dentro da janela retorna status/resultado redigido sem repetir o handler; invocações interrompidas por queda viram `outcome_unknown`.

Responsáveis: I04 / I12 / I14.

### C52

Evento durável preserva a chave pelo horizonte de replay da fonte e, depois dele, é rejeitado por `source_occurred_at` autenticado em vez de ser tratado como solicitação nova.

Responsáveis: I09 / I12.

### C53

Ativações por evento persistem o mesmo epoch/deadline imutável da fonte; aumentar retenção não reabre ocorrência antiga.

Responsáveis: I09.

### C54

Recuperação de startup atualiza auditoria e ledger para `outcome_unknown` na mesma transação.

Responsáveis: I12 / I14.

### C55

Reutilizar `invocation_id` com request fingerprint diferente falha fechado.

Responsáveis: I02 / I04.

### C56

Caps de auditoria não removem os ledgers antes de `expires_at`; compactar registro recente não permite nova execução ou ativação.

Responsáveis: I12.

### C57

Consulta de invocação aplica propriedade por usuário e autorização do ator, sem lookup cross-user apenas pela PK.

Responsáveis: I04 / I10.

### C58

Sessão, geração de segurança e staleness de contexto são revalidados imediatamente antes de todo handler.

Responsáveis: I03 / I04.

### C59

Policies `max_age_ms`/`event_snapshot` falham fechado sem timestamp de cada provider; ingresso não transforma snapshot sem `capturedAt` em contexto recém-capturado.

Responsáveis: I03.

### C60

`handler.Start` confirma handoff sem bloquear; logout/mutação concorrente não espera o trabalho longo nem entra em deadlock.

Responsáveis: I04 / I13 / I15.

### C61

Versões do catálogo e da configuração são revalidadas ao retirar da fila; binding alterado não executa resolução antiga.

Responsáveis: I03 / I04 / I05.

### C62

Cache de resolução inclui usuário, workspace, acionador, origem, `context_version` e todas as versões/gerações de catálogo, configuração e camadas ativas.

Responsáveis: I03 / I05.

### C63

Cada comando declara `context_policy`; nas policies que declaram providers, provider ausente ou versão/TTL inválido falha fechado.

Responsáveis: I02 / I03 / I04.

### C64

`context_policy = none` é rejeitado para qualquer comando não read-only.

Responsáveis: I02 / I04.

### C65

Contextos local, JWT externo, job e system têm fontes de identidade e revogação explícitas; `EpochService` invalida trabalho obsoleto.

Responsáveis: I10 / I14.

### C66

Ativação event-driven usa grants próprios de camada, com chave natural, geração monotônica, histórico de revogação e revalidação autoritativa por evento; não reutiliza nem amplia grants de delegação da AEP-0101.

Responsáveis: I08.

### C67

Adapter de jobs exige `job_slug = Job.ID` e `job_database_id = Job.DatabaseID`, confirma ambos por owner e permanece desabilitado para fatos legados ambíguos.

Responsáveis: I09.

### C68

Evento de ativação recebido é candidato sem autoridade; dispatcher deriva owner, workspace, regra, layer e epochs antes do envelope interno.

Responsáveis: I08 / I09.

### C69

Regras e layers builtin/user usam refs polimórficas consistentes no schema, grants, estado, ownership, importação e restore.

Responsáveis: I05 / I07 / I08 / I11.

### C70

Após o PR atualizar a AEP-0052, identidade externa só acessa usuário local por mapeamento administrativo exato de emissor e subject; antes disso, o command manager fica indisponível nesse modo.

Responsáveis: I10.

### C71

Cada comando declara origens permitidas e o serviço bloqueia origem não autorizada, incluindo comandos visuais solicitados pela CLI.

Responsáveis: I02 / I04 / I10.

### C72

`effect_class` e mutabilidade vêm do contrato do handler; metadata divergente impede o registro.

Responsáveis: I02 / I06.

### C73

CLI não executa comando que exija diálogo/decisão interativa.

Responsáveis: I04 / I10 / P05.

### C74

Comando destrutivo só avança com receipt de decisão criada no backend, vinculada à solicitação e consumida uma vez no CAS para `queued`.

Responsáveis: I04 / I06.

### C75

`cli`, `event` e `system` não registram/executam comando destrutivo; qualquer origem sem presenter interativo falha fechado.

Responsáveis: I02 / I06 / I10.

### C76

Em autenticação externa, adapters físicos permanecem indisponíveis até existir vínculo local explícito e revogável com um principal externo.

Responsáveis: I10 / I13 — broker físico externo fora do escopo atual.

### C77

Estação bloqueada suspende hotkeys globais e dispositivos físicos e apresenta estado seguro até revalidar a sessão após desbloqueio.

Responsáveis: I13 / I14.

### C78

Diálogo topmost bloqueia fallback para camadas inferiores e os atalhos obrigatórios da AEP-0091 não aceitam tombstone.

Responsáveis: I03 / I05 / I13.

### C79

Dispatcher reserva atalhos invariantes do diálogo antes de qualquer binding configurável.

Responsáveis: I13.

### C80

Shell continua passando exclusivamente por `internal/commandpolicy`.

Responsáveis: I10.

### C81

Manutenção em escopo de instância cobre todos os usuários e registros `system` em uma única cadência.

Responsáveis: I12.

### C82

Deep links e configurações importadas não concedem execução arbitrária.

Responsáveis: I10 / I11 / P05.

### C83

Testes cobrem fallback de defaults, sobreposição, múltiplas camadas, modais, inputs, múltiplas abas, troca de foco, reconexão de dispositivo e prevenção de execução duplicada.

Responsáveis: I15 / P06.
