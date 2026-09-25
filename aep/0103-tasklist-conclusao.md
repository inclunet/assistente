# AEP-0103 — Tasklist de conclusão integral

Baseline inicial de 16/09/2026; reconciliação de 24/09/2026 atualizada pela seção157. Branch `feat/aep-0103-comandos`; merge `84f98767c` incorpora `origin/main` (`714a47c4e`), com checkpoints posteriores `03ef8f0a4`, `6f516326a`, `f59f7d6c9`, `2a9049481`, `79b385168`, `1aa9e7c6c`, `79f06ab6b`, `6d0411dbb` e `bf7860c65`. Status do AEP: **In Progress**.

Este é o acompanhamento operacional vigente até concluir o AEP inteiro. Substitui as contagens narrativas da [tasklist anterior](0103-tasklist-infraestrutura.md), preservada como histórico. Não substitui contratos do [AEP](0103-comandos-acionadores-e-camadas-contextuais.md). A [revisão técnica](0103-revisao-integral-2026-09-16.md) registra achados, evidências e limitações desta baseline.

## 1. Progresso reconciliado — 24/09/2026, após a seção157

Esta reconciliação considera os commits `1aa9e7c6c` (contexto exato de
recibos externos), `79f06ab6b` (provas seletivas por execução no ciclo reativo),
`6d0411dbb` (retenção de fontes pending/processing e consumo/replay) e
`bf7860c65` (foco da grade), além da integração externa App/HTTP/frontend em
`0548492ab`. C65/C70 e R05.1/R03.4 passam a implementação identificada;
isso não é aceite final nem certificação dos gates de qualificação.

### Implementação dos 84 critérios finais

- **83/84 I — implementação identificada: 98,8%.**
- **1/84 P — parcial: 1,2%.**
- **0/84 N — funcionalidade pública inteiramente ausente.** Isso não elimina
  lacunas dentro dos critérios parciais, como a qualificação transversal.
- Comparação: seção76 **58/24/2**, seção129 **64/18/2**, seção130
  **65 I / 17 P / 2 N**. C11, C18, C23, C28, C37, C44 e C45 foram
  reconhecidos na reconciliação. C22, rebaixado em Δ18, volta a I após
  retirar a composição de condições do caminho por tecla do Deck.
  Na seção131, C62 P→I: **66 I / 16 P / 2 N**, com LRU efetivamente
  conectado à seleção do executor, sem cache de autorização.
  Na seção132, C04/C06 P→I: registro global explicitamente indisponível
  fora do Windows, sem fallback que contorne ownership/no-repeat.
  C77 P→I após prova global lock/unlock com perfil persistido, rejeição de
  ocorrências anteriores e retomada pelo bootstrap produtivo.
  Na seção133, C78/C79 P→I: reserva temporária de Ctrl+Shift+R no diálogo
  topmost, com precedência nativa, ownership compartilhado e teardown.
  Na seção134, C34/C36 N→I e C35 P→I: tools registradas, CRUD/restore/import
  confirmados, execução como agente, sessão do ingresso fixada e recusas headless.
  Na seção135, C71/C73 P→I: borda pública CLI, bloqueio de origens não
  permitidas e ausência de confirmação interativa alternativa.
  Na seção140, C02 P→I: encerradas as três ligações pendentes do inventário,
  com provas do dispatcher até rota, transformação do editor e DOM do chat.
  Produtor backend e consumidor frontend são qualificados separadamente;
  isso não constitui teste físico de HID nem transporte Wails ponta a ponta.
  Na seção141, C09 P→I: autoridade antes/depois da decisão, grants de jobs
  e ligação da tool subagent ao Manager/SQLite reais, com profile alvo
  propagado ao Send. A chamada ao modelo e disponibilidade do provider são
  controladas; o cenário job não é scheduler/executor ponta a ponta.
- Na seção147, C43 P→I: configuração confirmada de camada por programa,
  captura por ocorrência, execução real de ativação e publicação do mapa
  demonstradas no App. Fronteira do SO controlada; aceite físico permanece aberto.
- Na seção156, C65/C70 P→I: composição App/HTTP de identidade JWT mapeada,
  executor e revogação por token, vínculo explícito da interface externa e
  comando backend `workspace.list`. Testes externos App e frontend passaram;
  a revisão independente foi concluída. A falha isolada de golden na rodada
  App ampla foi corrigida e o grupo afetado revalidado; detalhes na seção156.
- Na seção155, recertificação somente leitura dos itens I01/I02/I05/I10/I11/I12/I14
  não encontrou gap concreto novo, sem converter a recertificação em aceite global.
- **Não é porcentagem de esforço, de prazo nem de aceite final.** Critérios
  têm tamanhos distintos; um parcial não recebe meio ponto. Os checkboxes C
  continuam reservados ao aceite final R12. Nenhum foi marcado nesta rodada.
- A contagem reconhece código entregue e corrige classificações antigas;
  não mede o número de alterações nem o tamanho do item parcial restante.

C83 passa a I na seção148: a arbitragem de diálogo/job chega ao executor no
mesmo teste, complementando a matriz automatizada transversal reexecutada.
Qualificação física, desempenho agregado e aceite final continuam em R12.

Na seção157, C51 passa de P para I após teste opt-in de queda real do processo
filho, recuperação com lease nativa e replay sem repetição do efeito.

Parcial atual: **C38**. Cada linha da seção5 informa o motivo,
os arquivos/testes e a fronteira entre lacuna funcional e qualificação.

### Saídas maiores e gates — denominadores separados

- **11/48 A — aceitas anteriormente:** R01.1–R01.4, R03.1–R03.3 e
  R04.1–R04.4. Aceites preservados, não ampliados.
- **20/48 I — implementação identificada, sem aceite integral:** R02.1–R02.4,
  R03.4, R05.1, R05.2, R05.3, R07.3, R08.1–R08.4, R09.1–R09.3, R10.3,
  R11.1–R11.3.
- **16/48 P — parciais; 1/48 N — ausente:** R12.4 (fechamento de review/CI).
- **A+I = 31/48 (64,6%)**. As seções135, 155 e 156 ampliam a identificação de
  implementação sem ampliar aceite final. Só A tem checkbox x.
- **1/12 gates aceito: R04.** R01 permanece em validação; os demais gates
  estão abertos. R02 ter implementação nas quatro saídas não equivale a
  aceite agregado. R07.3 passa a I: a recusa explícita fora do Windows
  da seção132 e a composição temporária da seção133 encerram a lacuna
  de implementação de ownership; o aceite físico continua separado.
- **BASE-PRONTA e AEP-CONCLUÍDO não foram declarados.**

### Inventário, infraestrutura e escopo

- Catálogo `product-v41-agent-commands`: **150 comandos, 61 apresentações
  locais e 67 defaults locais**. Deck contextual: **81 IDs** após Mermaid;
  pin/toggle/back e voz/jobs globais já existem. Nenhum desses números mede
  completude dos 84 critérios.
- Os **84 Ixx.n de infraestrutura** e os **20 Pxx.n de produto** são conjuntos
  históricos distintos, mapeados nas seções6–7. Os 53 I marcados na baseline
  não viram percentual atual: R06.1 ainda exige requalificação individual.
  Atualizar C/R não recertifica I automaticamente.
- C01–C83 mantêm os IDs originais; C84 continua a exceção aprovada de
  apresentação local sem ledger por tecla. Navegação não ganhou auditoria
  nesta revisão.
- Import/export comum já está implementado. Extensões sensíveis de
  portabilidade e gesto longo conservam a prioridade adiada pelo usuário,
  não são silenciosamente excluídos nem retomados.
- R03.4 e R05.1 têm implementação identificada na seção156, sem aceite
  agregado. R06 exige qualificação geral; R12/CI/reviews permanecem abertos.
  R09.4/R10.4 exigem validação manual/física; R05.4 sensível permanece adiada.
  R05.2/R09.3/R10.3 e recursos de execução da paleta passam a I na
  seção155, sem aceite final. A edição textual acessível do Deck já está implementada
  nas seções142–146; C38 aguarda aceite manual, não outra implementação.
  Migração de workspaces permanece fora desta entrega, conforme seção154.

## 2. Regras de aceite e atualização

Estados de execução: pendente, parcial, em validação, bloqueado por dependência, aceito. Checkbox só recebe x em aceito. Cada fechamento exige: ID, data, commit, arquivo/símbolo, comando de teste ou roteiro, resultado e limitações. Teste pulado não é PASS daquele cenário; mock não encerra requisito de wiring real; teste de disponibilidade da plataforma não encerra hotkey/lock físicos.

Uma correção entra no pacote do requisito afetado, com ID de achado Fxx da revisão. Não criar novos pacotes a cada rodada. Trabalho descoberto recebe Δnn, origem no AEP, efeito nos gates e decisão de escopo. Manter C01–C83 estáveis; C84 identifica a cláusula já aprovada na seção 68. Reconciliar por texto e registrar alterações sem renumerar os IDs antigos.

Relatório de cada avanço: estados de implementação C (I/P/N) / 84, separados do aceite final C; saídas R (A/I/P/N) / 48; gates fechados / 12; Ixx/Pxx reconciliados; achados fechados/abertos; próximo gate. Nunca somar contadores diferentes. Mudança de contrato precisa de decisão explícita; requisito já existente entra na fila sem aumentar silenciosamente o escopo.

Após cada lote que mude um estado, atualizar a linha do critério/saída, sua
evidência e os totais desta seção1, do AEP e do índice. Não atualizar apenas
o diário no fim do arquivo. **A** é aceite registrado; **I**, implementação
identificada sem aceite integral; **P**, parcial; **N**, entrega prevista ausente
(mesmo que existam bibliotecas de suporte). Percentual é sempre contagem de
critérios sem peso, nunca estimativa de horas ou de linhas de código.

Não exigir nova confirmação a cada incremento. Trabalhar até fechar um gate coerente ou chegar a dependência real. Não marcar Done por existir muito código, por uma suíte verde ou por todos os atalhos legados funcionarem.

## 3. Caminho de chegada e paralelismo

O encadeamento abaixo preserva os gates de aceite do plano original. A decisão
do usuário registrada na seção46 priorizou uso visível: a implementação de
R07–R10 avançou em paralelo ao fechamento de R06, sem declarar BASE-PRONTA.
Portanto dependência de aceite não significa proibição de continuar produto.

1. R01 fecha a composição base; R02 e R05 avançam em serviços independentes.
2. R03 usa configuração/identidade para conectar jobs. R04 consolida recuperação e a cadência única.
3. R06 requalifica os 84 itens originais e fecha **BASE-PRONTA**. Preserva a prioridade do usuário: base completa antes da migração sistemática.
4. R07 popula catálogo e migra os comandos. R08, R09 e R11 podem seguir em paralelo após contratos/catálogo estáveis; R10 depende da configuração de dispositivos.
5. R12 fecha os 84 critérios finais, documentação, revisão e aceite manual, então **AEP-CONCLUÍDO**.

Responsabilidade principal: contratos, integração, revisão e evidência. Delegar a Luna escopos disjuntos: contextos/bridge; configuração/claims; jobs/manutenção; identidades/portabilidade; UI após contratos. Um responsável por schema/migração e geração de bindings; não permitir seis agentes alterando a mesma montagem simultaneamente.

Tamanhos M/G/GG são relativos, não prazos. Não há estimativa honesta de dias sem fechar e medir R01/R02/R03. A previsibilidade passa por gates finitos abaixo e por não recontar avanço de contrato como produto pronto.

## 4. Pacotes e gates de chegada

### R01 — Composição real de contexto, dispatcher e lifecycle

Estado reconciliado na seção129: **4/4 saídas aceitas; gate em validação de regressão**. Obrigações e limites abaixo permanecem vigentes.

Tamanho: G. Dependências: Nenhuma; usa a base I01/I02 já existente. A seção 24 registra a pendência da suíte global, sem retirar as evidências focadas.

Rastreia: I03, I04, I13.1–I13.4, I14. Base reaproveitável: FactBus, HostState, executor completo, bridge e controller montados; saídas de providers/transporte aceitas. O gate R01 aguarda qualificação de regressão; migração integral de acionadores permanece em R07/R10.

Evidência adicional da seção 15: a paleta/bridge já resolve a configuração
persistida real e prova execução, supressão, recusa e replay estável após rebuild.
Não equivale a ligar teclado/Stream Deck nem a generalizar os fatos visuais.

A seção 18 estende a resolução persistida ao handoff UI e prova recusa de
redirecionamento UI→backend e de admissão obsoleta antes da entrega visual.

Arquivos de referência: `internal/app/app_command_lifecycle_product.go`; `internal/app/app_command_context.go`; `internal/app/app_command_complete_execution.go`; `internal/commandexecution`; `frontend/src/lib/commandBridge*.ts`.

- [x] R01.1 — Ligar providers reais à sessão em seu domínio: surface, diálogo topmost, foco/controle e IME na UI; aba/workspace, perfil e SO no backend. Ownership vem das fontes. Provar notificação perdida, mudança concorrente e provider ausente; efeitos visuais revalidam sincronamente na UI, sem reader Wails sob DispatchGate. Aceite e limites na seção 23.

  **Estado reconciliado: Aceito.** Providers reais e ownership aceitos na seção 23. Falta: Nenhuma lacuna funcional na saída; regressão final do gate permanece separada. Evidência: seções 23; `internal/app/app_command_context.go`, `frontend/src/lib/commandContextSession.ts`.
- [x] R01.2 — Montar o executor completo e a projeção completa no lifecycle com portas reais; readiness deve identificar dependência ausente, sem sentinel usado como evidência de caminho operacional. Aceite e limites na seção 19.

  **Estado reconciliado: Aceito.** Executor/projeção completos montados, sem sentinela. Falta: Nenhuma na saída. Evidência: seções 19, 22; `internal/app/app_command_lifecycle_product.go`.
- [x] R01.3 — Separar ingresso autenticado e handoff autorizado: nenhuma API pública da bridge pode alcançar handler sem resolver, autorizar, reservar ledger e revalidar CAS/gerações. Cobrir execução direta, trigger, suppress e rejected_stale. Aceite e limites na seção 22.

  **Estado reconciliado: Aceito.** Ingresso autenticado e handoff/commit com autorização, ledger e CAS. Falta: Nenhuma na saída; catálogo/novos ingressos são R07/R11. Evidência: seções 22, 75; `internal/app/app_command_ui.go`, `internal/app/app_command_complete_execution.go`.
- [x] R01.4 — Demonstrar boot/login/unlock/logout/troca de usuário/shutdown e recuperação com a composição do App; handlers controlados ficam no teste, não como implementação de produto. Nenhum mapa antigo é publicado. Aceite: provas de lifecycle anteriores e recovery registrado da seção 24; regressão global ainda pendente.

  **Estado reconciliado: Aceito.** Lifecycle/recovery aceitos no escopo de remontagem e prova nativa. Falta: Crash abrupto e qualificação global continuam fora do aceite local. Evidência: seções 24, 28; `internal/app/app_command_maintenance_restart_test.go`.

**Gate R01:** Teste de integração sobe a mesma composição de produção, resolve configuração persistida e demonstra execução única, recusa, supressão, invalidação e shutdown. As provas não se limitam à existência das dependências no MountSpec.

### R02 — Configuração, decisões e claims completas

Estado reconciliado na seção129: **4 saídas implementadas sem aceite integral; gate aberto**. Obrigações e limites abaixo permanecem vigentes.

Tamanho: G. Dependências: R01 para publicação e identidade; preparação dos serviços pode ocorrer em paralelo.

Rastreia: I05, I06, I07, I08. CRUD, projeção, diff/decisão, restore, grants e claims possuem serviços, APIs e testes integrados global/workspace. Nesta reconciliação as quatro saídas têm implementação identificada; aceite agregado e qualificações transversais continuam abertos.

Arquivos de referência: `internal/commandconfig`; `internal/commandactivation`; `internal/commandautomation`; `internal/app/app_command_mutation*.go`.

- [ ] R02.1 — Conectar CRUD de camadas/regras/bindings e diagnóstico único ao serviço autenticado; criação/edição/remoção/enable/disable/restore/import têm ownership, CAS e validação sem caminhos alternativos.

  **Estado reconciliado: Implementado; aceite/qualificação pendente.** CRUD de camadas, regras e bindings, diagnóstico, restore e importação comum passam pelos serviços autenticados com owner, escopo e CAS. Não falta uma nova fachada de configuração. Restam o aceite agregado e as extensões explicitamente separadas em R05/R11. Evidência: `internal/app/app_command_settings_scope.go` (`MutateCommandSettings`), `internal/commandconfig/complete_mutation_service.go`, `internal/app/app_command_settings_contract_test.go`, `internal/app/app_command_settings_advanced_security_test.go`; seções111–128 e129.
- [ ] R02.2 — Usar diff exato, presenter real e receipt de uso único para toda mutação confirmável; cancelamento, expiração, logout e edição concorrente recusam commit sem deixar autorização reaproveitável.

  **Estado reconciliado: Implementado; aceite/qualificação pendente.** Serviço comum vincula decisão ao diff e ao request, consome receipt uma vez e recusa cancelamento, expiração, sessão trocada e edição concorrente. As tools públicas ausentes continuam em R11, sem apagar este serviço. Evidência: `internal/commandconfig/mutation_service.go`, `internal/commandconfig/decision_concurrency_test.go`, `internal/app/app_command_mutation_desktop_test.go`, `internal/app/app_command_settings_advanced_security_test.go`; seções111–118 e129.
- [ ] R02.3 — Completar composição global + workspace, defaults permanentes, tombstones e upgrade/rebase/needs_review no contexto exato; persistência e mapa publicado concordam após sucesso ou rollback.

  **Estado reconciliado: Implementado; aceite/qualificação pendente.** União global/workspace, defaults/tombstones, upgrade/needs_review/rebase e restore têm percurso real e cinco provas de API, rollback e restart. Restore local preserva override global; decisão negada não publica alteração. A validação visual da ajuda é R09.3. Evidência: `internal/app/app_command_settings_requalification_test.go`, `internal/commandconfig/defaults_mutation.go`; seções111–118 e129.
- [ ] R02.4 — Fechar pin/toggle/back/expiração/restart e condições síncronas; grants event-driven revogam com mudanças e exigem nova decisão para reativar; teste de corrida cobre ciclos independentes.

  **Estado reconciliado: Implementado; aceite/qualificação pendente.** Pin/toggle/back, sessão/persistência/expiração, isolamento de origem/workspace e revisão de grants estão ligados aos serviços e aos ingressos teclado/paleta/Deck. Condições suportadas são avaliadas e combinações incompatíveis recusadas explicitamente; qualificação física/estabilização é R10. Evidência: `internal/app/app_command_manual_expiry_test.go`, `internal/app/app_command_settings_activation_security_test.go`, `internal/commandconfig/regrant_cycle_test.go`, `internal/app/app_command_deck_layer_contextual_test.go`; seções115–128 e129.

**Gate R02:** Uma configuração criada pelo serviço real muda o resolvedor, sobrevive a restart, restaura sem apagar defaults e não atravessa usuário/workspace. Todas as mutações previstas reutilizam o mesmo diagnóstico/decisão.

### R03 — Jobs, outbox e ativações em operação

Estado reconciliado na seção156: **3/4 saídas aceitas e 1 com implementação identificada, sem aceite; gate aberto**. Obrigações e limites abaixo permanecem vigentes.

Tamanho: G. Dependências: R01/R02; mantém contratos de AEP-0048/0067/0101.

Rastreia: I09, I08.4, I10.2, P05.4. Base reaproveitável: Timeline incremental, outbox durável, grants e Consumer estão implementados; localizar uma fábrica/teste não comprova consumo pelo App.

Arquivos de referência: `internal/jobs/command_activation_runtime.go`; `internal/commandjobevents`; `internal/commandjobactivation`; `internal/commandautomation`.

- [x] R03.1 — Montar Consumer no App com DB/gate/fingerprint e portas reais de owner, regra, layer, runtime e condição; iniciar/parar junto ao ciclo de vida correto, com indisponibilidade observável. Aceite: seções 26–29; seção 30 integra a projeção no resolvedor. Demais ingressos e replay continuam pendentes no gate.

  **Estado reconciliado: Aceito.** Consumer montado com portas reais e lifecycle. Falta: Nenhuma na saída. Evidência: seções 26–30; `internal/app/app_command_maintenance.go`.
- [x] R03.2 — Fechar fluxo de queued/started/retry_scheduled até completed/failed/skipped para jobs elegíveis manual/cron/interval/hotkey/internal_event; não rejeitar silenciosamente toda origem normal por falta de uma prova nunca emitida. Aceito em 17/09/2026: matriz comum de estados, ingressos Manager/Scheduler/callback hotkey e Service real de tasklists; evidências e limites na seção 33.

  **Estado reconciliado: Aceito.** Estados e ingressos de jobs qualificados no escopo documentado. Falta: Hotkey controlado não substitui roteiro físico de R10. Evidência: seções 33; `internal/jobs/command_activation_origins_test.go`.
- [x] R03.3 — Conectar heartbeat/lease e reconciliação; fonte ausente/legada/externa/unknown não mantém claim; eventos terminais encerram o próprio ciclo e eventos atrasados não encerram o seguinte. Aceite e escopo das provas reais/controladas: seção 29, complementando seções 26–28.

  **Estado reconciliado: Aceito.** Heartbeat, lease e isolamento entre ciclos com fonte real. Falta: Nenhuma na saída. Evidência: seções 28–30; `internal/app/app_command_job_publication_test.go`, `internal/commandjobactivation`.
- [ ] R03.4 — Demonstrar commit → queda → replay, sequência/fingerprint, count-cap de runs, múltiplas regras/workspaces e anti-loop (limite 16); classificar falhas, limitar retries e alcançar dead_letter sem ciclo eterno (F03). Atualizar evidências da AEP-0048/0067.

  **Estado reconciliado: Implementação identificada; aceite/qualificação pendente (P→I, seção156).** `TestProductJobRunEventCommandReplayEndToEnd` integra no App fixture o ingresso global de job por hotkey, execução do job, fato/outbox do runtime, Consumer montado, claim/layer/binding selecionados, comando downstream `workspace.list` e replay sem novo efeito. Não é aceite Wails/UI nem prova de lançamento por candidate da paleta para `job.run`. Cobertura complementar: reabertura SQLite/lease abandonada e consumo único (`TestRecoveryPendingLeaseReopensSQLiteRequeuesAndConsumesOnce`); sequência/fingerprint, global+dois workspaces e regras (`TestRecoveryMatrixSequenceAndFingerprintIsolation`, `TestRecoveryMatrixAdversarialOutboxResetReplayAcrossScopes`); retenção count-cap de fontes pending/processing com consumer (`TestCountRetentionKeepsPendingSourceUntilConsumerAppliesItOnce`); limite anti-loop 16/17 e descendente recusado antes da tool (`TestCommandChainSeparateFromJobsAndLimit`, `TestCommandJobReactiveAppRejectsSameJobDescendantBeforeTool`); retries limitados e `dead_letter` terminal (`TestRunPassRetriesTransientAndDeadLettersAtMaxAttempts`, `TestRetryBoundaryDeadLettersExactlyAtMaxAttemptsAndNeverRequeues`). Reabertura/lease e rollback demonstram recovery após interrupção recuperável, não kill abrupto de processo; C51 permanece P. AEP-0048/0067 atualizadas no commit `6d0411dbb`. Gate R03 não é aceito nesta reconciliação.

**Gate R03:** Job real do runtime ativa e desativa uma camada através da outbox e mantém lease durante execução longa. Restart/reentrega não duplicam claim/efeito; retenção não apaga a fonte prematuramente.

### R04 — Recuperação e manutenção da instância

Estado reconciliado na seção129: **4/4 saídas e gate aceitos em 17/09/2026**. Obrigações e limites abaixo permanecem vigentes.

Tamanho: G. A seção 28 qualifica o ciclo de manutenção com job vivo real; não encerra as obrigações restantes de R03.

Rastreia: I12, I14.3, I14.5. Coordinator, adapters legados, drain e recuperação transacional estão ligados antes de Start (seção 26). Recuperação multiusuário e política dinâmica têm aceite na seção 27; qualificação integrada de restart/manutenção e limites estão na seção 28.

Arquivos de referência: `internal/commandmaintenance`; `internal/jobs/command_maintenance*.go`; `internal/app/app_command_drain.go`; `internal/commandledger`; `internal/commanddecision`.

- [x] R04.1 — Montar uma única cadência de manutenção antes de jobs.Start, preservando todas as limpezas legadas e compactação; incluir purga bounded de outbox terminal após deadline. Provar que não há segundo loop e que outbox/leases precedem retenção de jobs. Aceite da montagem e evidências na seção 26; não encerra R04.3/R04.4 nem o gate multiusuário.

  **Estado reconciliado: Aceito.** Cadência única e ordem outbox/retenção montadas. Falta: Nenhuma na saída. Evidência: seções 26, 28; `internal/app/app_command_maintenance.go`, `internal/commandmaintenance`.
- [x] R04.2 — Estabelecer prova de encerramento de geração no restart, incluindo processos/sessões antigos e system; reconciliar auditoria + ledger como outcome_unknown atomicamente sem reexecutar efeitos. Aceite e limites do protocolo registrado na seção 24; geração sem prova permanece bloqueada, não presumida encerrada.

  **Estado reconciliado: Aceito.** Prova de encerramento e recovery transacional sem reexecução. Falta: Aceite não afirma teste de kill externo. Evidência: seções 24, 28; `internal/commandledger/recovery.go`, `internal/app/app_command_maintenance_restart_test.go`.
- [x] R04.3 — Recuperar receipts abandonadas, claims e leases de todos os usuários em lotes canceláveis; demonstrar progresso até terminar, retomada e isolamento sem depender do usuário ativo. Evidências/limites por domínio na seção 27; gerações desconhecidas não são presumidas encerradas.

  **Estado reconciliado: Aceito.** Recuperação paginada multiusuário/system e progresso. Falta: Nenhuma na saída. Evidência: seções 27–28; `internal/app/app_command_maintenance_restart_test.go`.
- [x] R04.4 — Validar seis settings, idade/caps, replay deadlines imutáveis, exclusão de ativos, compactação e interrupção entre lotes; atualizar AEP-0074-B e diagnóstico operacional. Evidência consolidada na seção 27, incluindo snapshot dinâmico do Consumer e UI existente.

  **Estado reconciliado: Aceito.** Settings, caps, deadlines, compactação e cancelamento qualificados. Falta: Nenhuma na saída. Evidência: seções 27–28; `internal/commandmaintenance`, `internal/app/app_command_maintenance_restart_test.go`.

**Gate R04:** Teste integrado multiusuário/system reinicia com pendências e executa ciclo completo na ordem exigida. Não perde limpeza antiga, não deixa pendência eterna e não classifica trabalho vivo como encerrado.

**Aceite:** seção 28, testes integrados reais e evidências complementares das seções 24–27. Não equivale a certificar encerramento abrupto de processo, integração visual de claims ou o AEP inteiro.

### R05 — Identidades, delegação e portabilidade

Estado reconciliado na seção156: **3 saídas com implementação identificada sem aceite (R05.1–R05.3), 1 parcial (R05.4 adiada); gate aberto**. Obrigações e limites abaixo permanecem vigentes.

Tamanho: G. Dependências: R01/R02; pode avançar em paralelo a R03/R04.

Rastreia: I10, I11. Base reaproveitável: commandidentity, contratos de tools, mapeamento externo e commandportable existem; falta demonstrar as portas e fluxos finais.

Arquivos de referência: `internal/commandidentity`; `internal/commandtoolbridge`; `internal/auth/command_external*.go`; `internal/commandportability`; `internal/commandconfig/import_mutation.go`.

- [ ] R05.1 — Fechar local/agent/job_service/system e fronteira externa com fontes autoritativas; revogar identidade/grant entre fila e Start bloqueia o handler; modo externo respeita readiness administrativa e não habilita dispositivos físicos.

  **Estado reconciliado: Implementação identificada; aceite/qualificação pendente (P→I, seção156).** A composição produtiva conecta mapeamento administrativo `(iss, sub)`, claims JWT (roles/scopes), executor integral por token, HTTP execute/lookup/revoke e conexão UI externa explícita. Testes cobrem isolamento, revogação/shutdown e readiness; o ingresso externo produtivo é somente UI, sem sessão desktop emprestada nem adapters físicos. `workspace.list` é backend por usuário, sem vínculo UI obrigatório, mas exige runtime do mesmo usuário ativo. Testes App externos PASS 3,965 s, repetição consolidada dos grupos afetados PASS 21,938 s e frontend PASS 474/474 arquivos (5.895/5.895 testes); ressalva da rodada App ampla na seção156. Evidência: commit `0548492ab`; `internal/app/app_command_external_execution.go`, `internal/app/app_httpapi.go`, `internal/app/app_command_workspace_list.go`, `internal/httpapi/external_commands.go`, `internal/httpapi/external_ui_connections.go`, `internal/app/app_command_workspace_list_external_test.go`, `internal/httpapi/external_commands_test.go`; seções149–156.
- [ ] R05.2 — Conectar delegação a tools/jobs ao executor comum, preservando owner/profile, decisões/grants exatos, correlação command_invocation e redação de input/output; shell permanece em commandpolicy.

  **Estado reconciliado: Implementação identificada; aceite/qualificação pendente (P→I, seção155).** `newCommandToolHandler` agora tem consumidor produtivo: comandos fixos `tool.execute.t_<UUIDv7>` publicados pela paleta encaminham argumentos validados ao executor/bridge comum, revalidam owner/sessão, schema/geração e alvo, exigem decisão interativa exata e correlacionam `command_invocations`/`ToolInvocation`. Tools permanecem somente ad hoc na paleta; o resultado bruto é descartado e argumentos não viram binding persistente (D11). Jobs globais mantêm `newCommandJobHandler`; isso não publica `job.run` nem fecha o ciclo reativo R03.4. Evidência: commit `2a9049481`; `internal/app/app_command_tool_product.go`, `internal/app/app_command_tool_handler.go`, `internal/app/app_command_tool_palette_integration_test.go`, `internal/commandtoolbridge/bridge.go`; testes de confirmação/cancelamento/drift/redação e catálogo MCP.
- [ ] R05.3 — Conectar import/export resources.commandLayers ao envelope real AEP-0047: UUIDs/escopos/refs, manter/substituir/cópia, nome explícito, foreign_owner sem vazamento e rollback atômico do lote.

  **Estado reconciliado: Implementado; aceite/qualificação pendente.** Import/export comum público: manter/substituir/copiar, UUIDs, nomes, escopos, relatório e transação multi-escopo. Falta: Aceite integral entre usuários/instâncias e visual; export sensível não faz parte desta saída, está em R05.4. Evidência: seções 40–45; `internal/app/app_command_import_desktop.go`, `internal/app/app_command_export_desktop.go`, `frontend/src/components/import/CommandLayerImportPanel.tsx`.
- [ ] R05.4 — Validar secrets por pattern exato no destino e corrigir F02: credencial missing/foreign/ambiguous deve produzir binding desabilitado tanto no plano quanto no snapshot aplicado. Desabilitar referências ausentes e regras sem grant; excluir claims/histórico/grants; export sensível somente pela UI com decisão e criptografia existente.

  **Estado reconciliado: Parcial.** Pattern exato e desabilitação de binding corrigidos no plano e apply. Falta: Implementar ação sensível UI-only com decisão/criptografia e import conjunto de credenciais antes dos bindings; extensão adiada pelo usuário. Evidência: seções 10, 40–45; `internal/commandportability/credential_resolver.go`, `internal/commandportability/import_store_references_test.go`.

**Gate R05:** Matriz de identidade e round-trip entre dois usuários/instâncias passam pelas APIs reais e recusam escalada/cross-owner/segredos brutos. Não se usa fallback de identidade nem aprovação genérica de camada.

### R06 — Qualificação e aceite BASE-PRONTA

Estado atualizado na seção133: **1 saída implementada e 3 parciais; gate aberto**. Obrigações e limites abaixo permanecem vigentes.

Tamanho: G. Dependências: R01–R05 e integração dos adapters de I13; testes/review iniciam antes.

Rastreia: I01, I02, I13, I15; qualificação de todos I01–I14. Base reaproveitável: Há muitas provas locais, hardware básico validado e suítes focadas; faltam qualificação global, desempenho representativo e revisão obrigatória.

Arquivos de referência: `internal/commandbootstrap`; `internal/commandinput`; `internal/commandphysical`; `internal/commanddeck`; `internal/commandexecution/envelope_latency_test.go`; `aep/0103-tasklist-infraestrutura.md`.

- [ ] R06.1 — Revalidar os 84 itens Ixx.n por evidência do escopo completo; manter chaves/migrações já corretas e testar banco novo/upgrade/restart com dados isolados, incluindo compatibilidade publicada.

  **Estado reconciliado: Parcial.** Schema/migração/restart têm provas; F01 corrigido. Falta: Recertificar individualmente 84 itens I no escopo total; não usar contagem histórica como aceite. Evidência: seções 10, 24–28; `internal/database/command_migration_test.go`, `aep/0103-tasklist-infraestrutura.md`.
- [ ] R06.2 — Executar build/vet/test backend completo, race, lint Go compatível, TypeScript/ESLint/Stylelint/Vitest e bindings Wails regenerados; reproduzir e tratar ACP/acpregistry se falharem. Corrigir F01 (fixture de migração v20/v21) e investigar F07 (teste concorrente de lifecycle); bloqueio de ambiente não vira PASS.

  **Estado reconciliado: Parcial.** Evidência recente: seção128 com App amplo, 450 arquivos/5.617 testes frontend, build/vet do App, TypeScript/Vite e lint focado; bindings oficiais regenerados nas rodadas registradas. Isso não equivale a backend completo/race/lint global/CI atuais. ACP e execuções bloqueadas pelo antivírus exigem ambiente aprovado, nunca contorno ou PASS presumido. Evidência: seções109–112,127–129.
- [ ] R06.3 — Medir p50/p95/p99 de composição integrada sob SQLite/mutações/foco/carga e separar resolução, fila/gate, ledger, handoff e render; registrar resultado contra meta experimental p95 < 1 ms e resolver contenção sem dispensar segurança.

  **Estado reconciliado: Parcial.** Microbenchmarks existem e navegação local não espera ledger. A seção130 retira a composição do catálogo do pressionamento do Deck (Δ18/C22), preparando condições por snapshot antes da entrada física. Falta medir p50/p95/p99 por resolução, gate/fila, ledger, handoff e render sob carga representativa. Evidência: `internal/app/app_command_deck.go` (`resolveDeckPress`, `deckTriggerIdentities`), `internal/app/app_command_deck_projection_test.go`, `internal/commandexecution/envelope_latency_test.go`; seções129–130.
- [ ] R06.4 — Fechar achados de revisão, cenários crash/replay/segredos/isolamento e provas de SO/HID/diálogo. Retirar caminhos redundantes do protótipo somente após provar ausência de consumidor/dado publicado; não manter APIs produtivas de demonstração para satisfazer testes.

  **Estado reconciliado: Parcial.** Achados corrigidos e múltiplas provas de segurança/isolamento. Falta: Consolidar crash/HID/SO/diálogos e revisão exigida; não apresentar revisão Luna como Bugbot. Evidência: seções 10–39, 68–75; `aep/0103-revisao-integral-2026-09-16.md`, `internal/commandexecution`.

**Gate R06:** BASE-PRONTA exige I01–I15 atendidos no escopo original, composição real validada e achados bloqueantes resolvidos. Bugbot e race têm evidência própria; ausência de ferramenta não autoriza declarar o gate fechado.

### R07 — Catálogo de produto e migração dos acionadores

Estado reconciliado na seção129: **4 saídas parciais; gate aberto**. Obrigações e limites abaixo permanecem vigentes.

Tamanho: GG. Dependências: BASE-PRONTA (R06). Inventário/contratos podem ser preparados antes; migração sistemática segue depois.

Rastreia: P01. Base reaproveitável: Inventário existe; atalhos antigos funcionam, mas isso não comprova execução pelo sistema novo.

Arquivos de referência: `aep/0103-inventario-atalhos.md`; `frontend/src/hooks/useWorkspaceKeyboardShortcuts.ts`; `frontend/src/components/chat`; `frontend/src/pages/useEditorMenus.tsx`; `controllers/hotkeys_controller.go`; `internal/jobs/manager.go`.

- [ ] R07.1 — Completar inventário e registrar IDs/handlers/defaults localizados reais de aplicativo/workspace/chat/editor/tasklists/terminal/diálogos, com schema, risco, efeito, contexto, origens e disponibilidade.

  **Estado reconciliado: Parcial.** Catálogo v39 com 149 comandos, 61 apresentações locais e 67 defaults locais; chat/editor, listas/perfis/terminal, voz/jobs e camadas têm comandos reais. Falta concluir o inventário por família com acionador, ID, handler, classificação e prova de equivalência, não implementar de novo essas famílias. Evidência: `internal/app/app_command_product_catalog.go`, `internal/app/app_command_product_readiness_test.go`, `aep/0103-inventario-atalhos.md`; seções77–129.
- [ ] R07.2 — Migrar teclado local, sequências Ctrl+N e ações de menus para resolvedor/executor/ponte; provar input/editável, IME, repeat, datagrid, abas e foco sem dupla execução.

  **Estado reconciliado: Parcial.** Teclado local, Ctrl+N/sequências, ajuda, arquivos/formatação, chat, Mermaid, regiões, páginas e controles de lista possuem ingressos comuns e testes. Falta fechar a matriz de equivalência do inventário inteiro por superfície, input/IME/repeat/foco; gestos ARIA/edição nativa não viram migração artificial. Não há evidência nesta revisão de que todos os listeners residuais sejam defeitos. Evidência: `frontend/src/lib/commandLocalKeyboard.ts`, `frontend/src/components/layout/Topbar.tsx` e testes de integração por família; seções77–128.
- [ ] R07.3 — Migrar hotkeys de perfis e jobs para adapters comuns com ownership local/global e captura de foreground antes de bring-to-front; preservar when/políticas do runtime.

  **Estado reconciliado: Implementado; aceite final pendente.** Migração Windows de voz/perfil e job implementada nas seções108–110: ocorrência privada, ownership DOM/nativo, executor, foreground anterior ao bring-to-front, when/perfil dinâmico e prazo do runtime preservados. A seção132 recusa explicitamente registro global fora da plataforma qualificada (C04/C06); a seção133 compõe a reserva prioritária do diálogo sem duplicar captura ou restaurar registros removidos. Evidência: `internal/app/app_hotkeys.go`, `controllers/hotkeys_controller.go`, `internal/jobs/manager.go`, `internal/app/app_command_global_execution.go`, `internal/app/app_command_global_job_integration_test.go`, `internal/hotkey/manager_temporary_test.go`; seções132–133. Aceite físico separado.
- [ ] R07.4 — Retirar listeners/handlers finais paralelos após equivalência automatizada por surface; ajuda e menus consultam bindings efetivos. Registro completo deixa de depender de lifecycle.ready.

  **Estado reconciliado: Parcial.** Ajuda/menus usam mapa efetivo; caminhos finais paralelos foram retirados nas famílias migradas, inclusive voz/jobs. Falta a comprovação inventário→ingresso→efeito de todas as famílias, distinguindo listeners nativos de handlers concorrentes. Evidência: `frontend/src/lib/commandShortcutHints.ts`, `frontend/src/components/layout/Topbar.tsx`, `controllers/hotkeys_controller.go`; seções97–110 e129.

**Gate R07:** Cada família do inventário tem teste de ação final no caminho da sua classificação: execução durável via CommandInvocation e trilha autorizada; apresentação local via mapa efetivo e guards sem ledger (seção 68). Keyboard/palette/dispositivo usam a mesma ação, sem handler concorrente. Não basta cadastrar metadata ou manter atalho legado funcionando.

### R08 — Command Palette funcional no picker compartilhado

Estado reconciliado na seção155: **4 saídas com implementação identificada sem aceite; gate aberto**. Obrigações e limites abaixo permanecem vigentes.

Tamanho: M/G. Dependências: R07 e serviços de R01/R02.

Rastreia: P02; escolha de picker aprovada pelo usuário. Base reaproveitável: Combobox compartilhado, busca, disponibilidade e execução produtivas; recursos de R08.2 identificados na seção155, com qualificação/aceite ainda pendentes. Reaproveitar padrão compartilhado sem perder semântica acessível D9.

Arquivos de referência: `frontend/src/components/layout/Topbar.tsx`; `frontend/src/components/ui/menu/Menu.tsx`; `frontend/src/services/commandCatalog.ts`; `internal/wailsapi/command_catalog.go`.

- [ ] R08.1 — Publicar catálogo real e buscar nome/descrição/categoria/aliases nos três idiomas; disponibilidade/motivo dependem do contexto e executor completo, não somente do CheckReadiness legado read-only.

  **Estado reconciliado: Implementado; aceite/qualificação pendente.** Catálogo/busca localizada e disponibilidade contextual reais. Falta: Qualificação final nos três idiomas/contextos, sem nova lacuna funcional identificada nesta saída. Evidência: seções 53, 69, 75; `frontend/src/components/layout/Topbar.tsx`, `internal/wailsapi/command_catalog.go`.
- [ ] R08.2 — Executar seleção pelo serviço único e solicitar argumentos faltantes; mostrar atalho efetivo, recentes/favoritos e link direto para configuração; sucesso/erro/indisponibilidade são anunciados corretamente.

  **Estado reconciliado: Implementação identificada; aceite/qualificação pendente (P→I, seção155).** A paleta usa o serviço único, exibe atalhos efetivos, favoritos/recentes e ligação à configuração; o diálogo coleta argumentos e apresenta orientação do schema real da ferramenta. O backend revalida schema/alvo e exige confirmação. Evidência: `79b385168`, `Topbar.tsx`, `Topbar.palette.integration.test.tsx`, `CommandArgumentsDialog.tsx`, `commandToolGuidance.ts`, `commandPalettePreferences.ts`; backend `2a9049481`. Regressão final: 469 arquivos/5.862 testes PASS, TypeScript e lint PASS. NVDA não executado.
- [ ] R08.3 — Corrigir F04 (reabertura após Escape/resposta atrasada), repetição/IME e precedência de Ctrl+K; preservar atalho do editor conforme contexto. Fechamento restaura o foco de origem; execução que muda surface transfere foco para ela.

  **Estado reconciliado: Implementado; aceite/qualificação pendente.** Corridas de reabertura, Escape/IME e foco corrigidos; Ctrl+K respeita editor. Falta: Aceite final agrupado com as novas ações; há aceite de uso do recorte69. Evidência: seções 69, 72–75; `frontend/src/components/layout/Topbar.test.tsx`, `frontend/src/components/layout/Topbar.tsx`.
- [ ] R08.4 — Qualificar combobox/listbox no picker, exploração de indisponíveis com motivo, contagem de resultados, teclado/axe e pt-BR/en/es. Validar manualmente com NVDA após a lista e execução funcionarem.

  **Estado reconciliado: Implementado; aceite/qualificação pendente.** Combobox compartilhado, indisponíveis exploráveis, contagem e anúncios implementados. Falta: Roteiro NVDA/idiomas integral, não extrapolar o aceite de uso já recebido. Evidência: seções 69, 75; `frontend/src/components/pickers/Combobox.tsx`, `frontend/src/components/layout/Topbar.palette.integration.test.tsx`.

**Gate R08:** No App, usuário abre, encontra um comando por alias, inspeciona um indisponível, executa um disponível uma única vez, fornece argumentos quando necessários e fecha sem reabertura tardia; teclado/NVDA aprovados.

### R09 — Configurações → Comandos e acionadores

Estado reconciliado na seção155: **3 saídas com implementação identificada sem aceite (R09.1–R09.3), 1 parcial; gate aberto**. Obrigações e limites abaixo permanecem vigentes.

Tamanho: G. Dependências: R02/R07; pode avançar junto de R08.

Rastreia: P03. Tela funcional em escopos global/workspace, com regras, bindings, argumentos/condições, ativação e captura teclado/Deck. Divulgação progressiva e edição textual da apresentação do dispositivo estão implementadas; o aceite manual D15 permanece pendente.

Arquivos de referência: `frontend/src/pages/SettingsPage.tsx`; `frontend/src/components/ui`; `internal/commandconfig`; `frontend/src/components/ui/KeyboardShortcutsHelp.tsx`.

- [ ] R09.1 — Criar entrada acessível em Configurações e lista/detalhe de camadas com seções Quando esta camada fica ativa e Comandos desta camada; defaults inspecionáveis e personalização por delta.

  **Estado reconciliado: Implementado; aceite/qualificação pendente.** Entrada em Configurações, lista/detalhe, Quando ativa, comandos e defaults inspecionáveis. Falta: Aceite completo da saída; usuário já validou criação/ativação/captura básica. Editor de regras avançadas está em R09.2. Evidência: seções 47–60; `frontend/src/pages/CommandSettingsPage.tsx`, `frontend/src/pages/SettingsPage.tsx`.
- [ ] R09.2 — Permitir criar/editar/remover/ativar/desativar camada, regra e binding usando serviços comuns; capturar teclas, explicar resultado efetivo/conflito e confirmar substituição no contexto correto.

  **Estado reconciliado: Implementado; aceite/qualificação pendente.** CRUD escopado de camadas/regras/bindings, captura de teclado/Deck/sequências, argumentos, condições, prioridade e diagnóstico estão expostos pelos serviços comuns. Aba/perfil têm nomes amigáveis e origens/classes incompatíveis são explicadas, não silenciosamente aceitas. Evidência: `frontend/src/pages/CommandSettingsPage.tsx`, `frontend/src/lib/commandSettingsConditions.ts`, `internal/app/app_command_settings_scope.go`, `internal/app/app_command_settings_contract_test.go`; seções111–128. Validação NVDA integral segue separada.
- [ ] R09.3 — Expor restore por binding/camada/tudo, needs_review/rebase e prioridades em divulgação progressiva; ajuda reflete mapa efetivo após alteração e restart.

  **Estado reconciliado: Implementação identificada; aceite/qualificação pendente (P→I, seção155).** Restore por binding/camada/tudo, revisão/rebase contra defaults vigentes, prioridade e divulgação progressiva avançada estão expostos; a ajuda acompanha o mapa/perfil efetivo e a configuração persiste/reabre após reload. Evidência: `79b385168`, `CommandSettingsPage.tsx`, `CommandSettingsPage.advanced.test.tsx` e testes de Topbar/paleta. Regressão frontend integral PASS; restart real/NVDA permanecem na validação manual, sem aceite final.
- [ ] R09.4 — Concluir lista operável por teclado/NVDA, mensagens/erros em pt-BR/en/es e alternativas a imagem/cor/drag; para dispositivo, posição/imagem/título/estados também editáveis textualmente.

  **Estado reconciliado: Parcial; implementação identificada, aceite manual pendente.** UI acessível por lista, labels e três locales; edição textual de imagem/título/estados entregue nas seções142–146. Resta qualificação integral por teclado/NVDA e dispositivo, não reimplementar esses controles. Evidência: `frontend/src/pages/CommandSettingsPage.tsx`, `frontend/src/pages/CommandSettingsPage.test.tsx`; seções142–146.

**Gate R09:** Usuário encontra a tela, remapeia uma ação, entende conflito, testa o efeito, reinicia, restaura e inspeciona defaults; tudo possível por teclado e NVDA com persistência/auditoria corretas.

### R10 — Stream Deck e contexto externo no App

Estado reconciliado na seção155: **1 saída com implementação identificada (R10.3), 3 parciais; gate aberto**. Obrigações e limites abaixo permanecem vigentes.

Tamanho: G. Dependências: R07/R09; núcleo HID qualificado em R06.

Rastreia: P04. Base reaproveitável: Driver/manager/renderização têm testes e round-trip físico básico já passou; mapas reais e execução básica já foram validados no App. Isso não prova reconexão/lock/multidispositivo integrados.

Arquivos de referência: `internal/commanddeck`; `internal/commandphysical`; `internal/commandforeground`; `internal/commandadapter`; `docs/operations/streamdeck-manual-validation.md`.

- [ ] R10.1 — Montar dispositivos reais no App e projetar bindings efetivos em imagem/título/estados, com diff/cache/frame completo e IDs isolados para múltiplos dispositivos.

  **Estado reconciliado: Parcial; implementação identificada, aceite físico pendente.** App projeta título/imagem/estados configuráveis e renderiza mapa real por diff/cache. Resta qualificação física de múltiplos dispositivos/mapas. Evidência: seções142–146; `internal/app/app_command_deck.go`, `internal/commanddeck/renderer.go`.
- [ ] R10.2 — Fechar navegação de camadas/pasta, pin/toggle/back e atualização contextual/estabilização; snapshot externo é capturado antes de foreground mudar e não persiste título/URL.

  **Estado reconciliado: Parcial.** Ativar/pin, toggle/back, pilha por dispositivo e contexto de origem já implementados; os 81 IDs do Deck, inclusive Mermaid, receberam qualificação automatizada na seção128. Restam qualificação integral da navegação/estabilização por programa e aceite físico. Não criar uma entidade pasta paralela para satisfazer terminologia histórica: o contrato usa camadas/pilha. Evidência: `internal/app/app_command_deck_layer_contextual_test.go`, `internal/app/app_command_deck_mermaid_contextual_test.go`, `internal/commandforeground`; seções126–129.
- [ ] R10.3 — Expor status seguro, modelo/capacidades, disputa HID e reconexão na UI acessível; lock/logout/troca de usuário/shutdown removem mapa e rejeitam callbacks antigos.

  **Estado reconciliado: Implementação identificada; aceite/qualificação pendente (P→I, seção155).** O App publica status seguro por dispositivo e a página de configurações apresenta estado, modelo, contagem de teclas e razões genéricas de indisponibilidade/reconexão com strings localizadas; erros HID brutos não são expostos. Evidência: commit `6f516326a`, `internal/app/app_command_deck.go`, `internal/app/app_command_deck_status_test.go`, `frontend/src/pages/CommandSettingsPage.tsx` e teste de status da página. Limite: `open_failed` não identifica causa específica de disputa HID nem enumera todos os recursos do modelo; aceite físico/NVDA e matriz lock/logout/troca/shutdown continuam fora deste I e de R10.4.
- [ ] R10.4 — Executar roteiro físico com Stream Deck informado pelo usuário, sem software oficial, incluindo execução real, unplug/replug, lock/unlock e shutdown; testar isolamento multidispositivo e degradação explícita Linux/macOS.

  **Estado reconciliado: Parcial.** Tecla real, descoberta e uso normal confirmados pelo usuário. Falta: Executar matriz unplug/replug/lock/logout/shutdown/múltiplos dispositivos e degradação de plataformas. Evidência: seções 57–59, 69; `docs/operations/streamdeck-manual-validation.md`, `docs/content/recursos/COMANDOS.md`.

**Gate R10:** Tecla física executa comando real uma vez e acompanha o mapa editado; falhas de dispositivo não derrubam o App, dados da sessão antiga não reaparecem e recuperação funciona sem software oficial.

### R11 — Chat, CLI, automação e portabilidade visível

Estado reconciliado na seção135: **3 saídas implementadas e 1 parcial; gate aberto**. Obrigações e limites abaixo permanecem vigentes.

Tamanho: G. Dependências: R05/R07; configuração do chat usa R02 e formulários de R09.

Rastreia: P05. Tools compostas, portabilidade permitida e entrypoints CLI estão expostos. Falta qualificação completa da convergência/delegação no produto.

Arquivos de referência: `internal/tools`; `cmd`; `internal/commandtoolbridge`; `internal/commandportability`; `internal/jobs`; `frontend/src/pages`.

- [ ] R11.1 — Registrar command_catalog list/describe/execute e command_config com todos os verbos fechados de D10, IDs reais, diff e decisão por mutação de agente; nenhum SQL direto ou autorização implícita.

  **Estado reconciliado: Implementado; aceite final pendente.** Duas tools registradas no bootstrap, ações D10 fechadas, IDs reais, sessão capturada no ingresso GUI e revalidação de caller/turno. CRUD/restore/import usam o applier e decisões comuns; execute preserva ActorAgent e usa CommandExecutionService. Evidência: `internal/tools/command`, `internal/app/app_command_agent*.go`, `internal/app/app_tool_registry.go`, `internal/app/app_wire.go`; seção134.
- [ ] R11.2 — Expor CLI list/describe/execute/retry com ID de solicitação e consulta autorizada; visual/interativo/destrutivo fica indisponível com motivo e não ganha confirmação textual alternativa.

  **Estado reconciliado: Implementado; aceite final pendente.** Grupo commands com list/describe/execute/retry/status, UUIDv7 gerado na primeira tentativa, lookup autenticado e replay sem novo efeito. Borda CLI usa o executor comum, recusa visual/interativo e não inicia hardware ou serviços autônomos. Nenhum dos 149 comandos atuais declara CLI; disponibilidade não foi ampliada artificialmente. Evidência: `cmd/asst/commands.go`, `internal/commandcli`, `internal/app/app_command_cli.go`, testes das três camadas; seção135.
- [ ] R11.3 — Integrar import/export ao produto e às tools permitidas, mostrar relatório de referências/conflitos e testar manter/substituir/cópia; export sensível não aparece no chat.

  **Estado reconciliado: Implementado; aceite final pendente.** Dados e tools usam a portabilidade comum, relatórios e modos manter/substituir/cópia. Chat exporta somente o escopo escolhido, sem credenciais/claims/grants, e importa com diff/decisão e destino autorizado. Evidência: `internal/app/app_command_agent_portability.go`, `internal/app/app_command_agent_portability_test.go`, testes de import/export desktop e painéis de Dados; seção134. Exportação sensível continua vedada no chat.
- [ ] R11.4 — Exercer jobs/eventos/delegações com comandos reais, grants e anti-loop; confirmar que chat/CLI/palette/teclado/Stream Deck convergem ao mesmo executor e correlacionam auditoria.

  **Estado reconciliado: Parcial.** Voz/jobs globais, eventos e bibliotecas de delegação têm provas; tools de chat e CLI convergem ao executor nas seções134–135. Falta qualificação completa por família nas origens permitidas e do ciclo produtivo reativo integral. Nenhum comando produtivo atual permite CLI; não ampliar AllowedSources para fabricar equivalência. Não listar hotkey de job como funcionalidade ausente. Evidência: `internal/app/app_command_global_job_integration_test.go`, `internal/app/app_command_job_reactive_integration_test.go`, `internal/app/app_command_agent_execute.go`, `internal/commandcli/integration_test.go`, `internal/commandtoolbridge`.

**Gate R11:** Fluxos reais de usuário/agente/CLI e automação passam por contratos comuns; retry não duplica efeito, headless não solicita decisões impossíveis e importação não concede capacidade nova sem validação.

### R12 — Aceite integral e encerramento do AEP

Estado reconciliado na seção129: **3 saídas parciais e 1 não implementada; gate aberto**. Obrigações e limites abaixo permanecem vigentes.

Tamanho: G. Dependências: R01–R11; documentação e preparação de testes contínuas.

Rastreia: P06, F01 e todos C01–C84. Base reaproveitável: Relatos manuais parciais ficam preservados; aceite final depende de percursos que ainda serão construídos.

Arquivos de referência: `aep/0103-comandos-acionadores-e-camadas-contextuais.md`; `aep/README.md`; `docs/content`; `suítes backend/frontend/e2e`.

- [ ] R12.1 — Fechar cada C01–C84 da matriz com arquivo/símbolo, commit, teste/cenário, resultado e limitações; verificar regressões ponta a ponta, multiusuário/workspace/dispositivo e desempenho das ações reais.

  **Estado reconciliado: Parcial.** Matriz C01–C84 e saídas R reconciliadas individualmente na seção129, com evidências, lacunas e contagens separadas. Faltam aceite final dos critérios, provas compostas de isolamento/desempenho e recertificação dos 84 I; reconciliação documental não equivale a executar essas qualificações. Evidência: seções5,6 e129 desta tasklist.
- [ ] R12.2 — Concluir roteiro manual de NVDA/configuração/palette/dispositivo, com comando exato, passos, esperado, resultado e checkbox; solicitar ao usuário apenas cenários já implementados e testados automaticamente.

  **Estado reconciliado: Parcial.** Roteiro consolidado em
  [VALIDACAO_MANUAL_COMANDOS.md](../docs/content/guias/VALIDACAO_MANUAL_COMANDOS.md):
  48 casos em 12 blocos nomeados, com IDs, variantes, comandos, esperado,
  resultado e pré-requisitos. Os 13 itens NVDA anteriores estão mapeados sem
  duplicar contagem. Nenhum caso foi aprovado apenas pela organização documental.
  Falta executar e registrar a rodada na versão escolhida, incluindo os fluxos
  condicionais e a qualificação física/acessível. Aceites históricos preservados;
  isso não fecha R12.1, C38 ou os 84 critérios por equivalência numérica.
  Evidência anterior: seções 57–59, 69–75; `docs/content/recursos/COMANDOS.md`.
  Validação documental: 48 IDs únicos, 12 blocos, campos de resultado, links
  locais Hugo e parse Markdown conferidos; `git diff --check` PASS. Godel
  conferiu a cobertura; Franklin fez revisão independente em duas rodadas,
  com ajustes de isolamento e do contrato reativo e sem achados na rodada final.
  Nenhum aplicativo, teste Go, hardware ou banco foi executado nesta organização.
- [ ] R12.3 — Atualizar docs/content e índice, AEPs relacionados e inventário; registrar avaliação da fase 7 (pedais/MIDI/dial/capabilities), mantendo controle privilegiado externo/broker físico externo fora do escopo atual conforme AEP.

  **Estado reconciliado: Parcial.** AEP/índice/tasklist e topo do inventário atualizados até129, sem reescrever números históricos. Faltam inventário completo de equivalência, sincronização final dos AEPs relacionados e avaliação explícita da fase7 (avaliação, não obrigação de implementar pedais/MIDI/dial nesta rodada). Evidência: `aep/0103-inventario-atalhos.md`, `aep/README.md`, `docs/content/recursos/COMANDOS.md`; seção129.
- [ ] R12.4 — Concluir Bugbot local, CI e review remota quando houver PR autorizado; zero achados bloqueantes, zero threads pendentes. Marcar AEP/índice Done somente com todos os gates; merge continua do mantenedor.

  **Estado reconciliado: Entrega não implementada.** Não há evidência do fechamento Bugbot/CI/review deste trabalho. Falta: Executar review local e, quando houver PR autorizado, CI/review remota; merge é do mantenedor. Evidência: seções limites da revisão; `AGENTS.md`, `aep/0103-revisao-integral-2026-09-16.md`.

**Gate R12:** AEP inteiro demonstrado, não apenas 84 itens de base. Documentação, matriz de 84 critérios, testes, revisão e validação manual concordam com a versão entregue.

## 5. Matriz reconciliada dos 84 critérios finais

Texto normativo do AEP vigente, identificado sem renumerar C01–C83. C84 é a
cláusula de apresentação local acrescentada na seção 68. **I** significa
implementação identificada no escopo do critério, com evidência existente;
**P**, parcial; **N**, funcionalidade de produto ainda ausente. Não se tomou
presença de arquivo ou teste isolado como aceite de um percurso completo.
Referências de diretório identificam o subsistema e suas provas; seções citadas
registram os resultados e limitações das execuções anteriores. Todos os 84 IDs
foram relidos na seção129 (22/09/2026); referências históricas mantidas nos itens
sem mudança de status não significam que esses itens deixaram de ser revistos.

Os checkboxes abaixo continuam sendo de **aceite final**, distinto do estado de
implementação. Esta rodada leu código, testes e registros; não executou novas
suítes nem recertificou os gates. C03/C15/C16 devem ser lidos no caminho durável;
C84 explicita a exceção local aprovada, sem nova exigência de auditar navegação.

### C01

- [ ] C01 — Existe registro canônico e pesquisável de comandos com IDs, argumentos, disponibilidade, risco, aliases localizados e apresentação.

**Implementação: I — identificada; aceite final pendente.** Catálogo `product-v39-layer-actions` com 149 comandos, schemas, efeitos/origens, disponibilidade e aliases pt-BR/en/es. Listagem e pesquisa usam o registro canônico; contagem de comandos não é porcentagem do AEP.

Evidência: `internal/app/app_command_product_catalog.go`, `internal/app/app_command_product_readiness_test.go`, `internal/commandcatalog/search_test.go`; seções109–129. Gates: R07.

### C02

- [ ] C02 — Teclado local, hotkey global, Stream Deck, Command Palette, chat e CLI podem convergir para o mesmo comando sem handlers finais duplicados.

**Implementação: I — identificada; aceite final pendente.** Teclado local/global, paleta, Deck, chat e CLI convergem conforme suas origens permitidas, sem handler final alternativo. A apresentação local mantém a exceção C84. A seção140 fecha as três ligações restantes do inventário: Sobre P/K/D até `/about`, Ctrl+Alt+C até TipTap e UI até recolhimento real da thread. As provas de produtor e consumidor são distribuídas, não E2E físico/Wails. A borda CLI existe, mas nenhum comando produtivo atual permite essa origem; sua prova positiva de replay usa registro isolado de teste, não um comando artificial no produto.

Evidência: `internal/app/app_command_ui.go`, `internal/app/app_command_global_execution_test.go`, `internal/app/app_command_layer_origin_convergence_test.go`, `internal/app/app_command_family_origins_test.go`, `frontend/src/components/layout/Topbar{,.palette.integration,.editorMode.integration}.test.tsx`, `frontend/src/components/chat/ChatNavigation.origins.integration.test.tsx`, `frontend/src/lib/commandNavigation.test.ts`, `internal/tools`, `cmd/asst`; seções109–110,129,137 e140. A seção137 cobre o ciclo dos três comandos de camada nas quatro origens permitidas, com replay pelo chat. A restrição contratual da CLI não é funcionalidade a habilitar para fechar a matriz. Gates: R07, R10, R11, ainda sem aceite agregado.

### C03

- [ ] C03 — Todo acionamento classificado para execução durável produz `CommandInvocation` e passa por `CommandExecutionService`, com sessão, proveniência, autorização, deduplicação e auditoria antes do handler final; `effect = suppress` é consumido sem criar invocação.

**Implementação: I — identificada; aceite final pendente.** Execuções duráveis usam reserva, autorização e commit; apresentação local é a exceção aprovada, separada em C84.

Evidência: `internal/commandexecution/envelope_pipeline.go`, `internal/app/app_command_ui.go`. Registros: seções 22, 68, 75. Gates: R01, R07, R11.

### C04

- [ ] C04 — A reserva atômica por evento impede reentrega, e ownership exclusivo impede duplicidade entre teclado local/global e listeners de dispositivo.

**Implementação: I — identificada; aceite final pendente.** Reserva/replay e ownership Windows com ACK antes do registro nativo estão implementados. A seção132 fecha a borda não Windows por degradação explícita: `IsSupported` retorna false, o App não inicializa nem fornece registrador global e o adapter nativo recusa registro mesmo quando chamado diretamente. Não há captura global sem coordenação com DOM. Não é implementação de hotkeys Linux/macOS nem alegação de duplicidade observada.

Evidência: `internal/app/app_hotkeys.go`, `internal/hotkey/manager_ownership_barrier_test.go`, `frontend/src/lib/commandGlobalOwnershipWails.ts`; seções108–110 e129. Gates: R01, R07, R10.

### C05

- [ ] C05 — Solicitações diretas e triggers sem `source_event_id` usam `invocation:<invocation_id>`; eventos usam `event:<source_event_id>`.

**Implementação: I — identificada; aceite final pendente.** Chaves de idempotência direta/evento implementadas e exercitadas no ledger e no ingresso integrado.

Evidência: `internal/commandledger`, `internal/app/app_command_resolution_test.go`. Registros: seções 22, 32. Gates: R01.

### C06

- [ ] C06 — Manter uma tecla pressionada não repete comandos fora da lista explícita de navegação local da D3: repeats elegíveis têm ocorrência independente; demais repeats são descartados antes de gerar `source_event_id`. Testes cobrem identidade da combinação, release, blur, reconexão e saturação.

**Implementação: I — identificada; aceite final pendente.** Repeat local explícito, descarte nas demais ações, release/blur/Deck e no-repeat Windows possuem implementação/provas. Na seção132, `native_other.go` deixa de delegar captura global à biblioteca: retorna adapter que recusa registro explicitamente, sem eventos, ownership ou listener. A garantia fora do Windows é indisponibilidade segura, não equivalência nativa; aceite físico continua pendente.

Evidência: `frontend/src/lib/commandLocalKeyboard.repeat.test.ts`, `internal/commandinput`, `internal/hotkey/native_windows_test.go`, `internal/hotkey/native_other.go`; seções106–110 e129. Gates: R06, R07, R10.

### C07

- [ ] C07 — Retirada de `queued` revalida todos os gates no mesmo CAS para `running`.

**Implementação: I — identificada; aceite final pendente.** Revalidação e CAS da execução durável implementados; protocolo usado pelos commits de produto.

Evidência: `internal/commandexecution/envelope_pipeline.go`, `internal/app/app_command_ui.go`. Registros: seções 22, 62–75. Gates: R01.

### C08

- [ ] C08 — Cada instância física usa geração própria e índice parcial de eventos; invocações diretas deduplicam somente pela PK UUIDv7.

**Implementação: I — identificada; aceite final pendente.** Geração por adapter e deduplicação persistente física/direta implementadas; qualificação multidispositivo continua no gate.

Evidência: `internal/commandadapter`, `internal/commandledger`, `internal/commanddeck/manager.go`. Registros: seções 16–18, 56. Gates: R01, R10.

### C09

- [ ] C09 — Execução por agente e automação preserva e revalida os gates da AEP-0101; origem headless não herda a identidade do usuário para autorizar mutações.

**Implementação: I — identificada; aceite final pendente.** Identidades delegadas/job_service e gates headless são reais. Chat local fixa a sessão no ingresso e não a empresta a canais/jobs/subagentes/CLI. A seção141 qualifica admissão nas duas tools, invalidação durante decisões de configuração/ativação e propagação do profile autorizado pela tool subagent do registry produtivo ao Manager real, com subconversa/run persistidos. Grants e revogação/regrant de jobs são exercitados no App. Send/provider controlados e cenário job por contexto canônico não são E2E scheduler/LLM. A seção155 acrescenta consumidor produtivo command→tool pela paleta, com identidade local e decisão; o ingresso externo permanece separado em R05.1/C65/C70. C09 segue I, sem aceite final.

Evidência: `internal/commandidentity/service.go`, `internal/commandidentity/service_test.go`, `internal/jobs/command_service_identity.go`, `internal/app/app_command_layer_origin_convergence_test.go`, `app_command_agent_admission_test.go`, `app_command_agent_config_delegation_test.go`, `app_command_subagent_wire_test.go`, `app_command_job_dynamic_profile_test.go`; seções34–39,109–110,129,137 e141. O recorte de camadas comprova owner estrangeiro e revogação da sessão após captura do contexto de chat, sem efeito persistido. Gates R05/R11 continuam sem aceite agregado.

### C10

- [ ] C10 — Usuário e ator são derivados pelo backend; payload não escolhe identidade de autorização/auditoria.

**Implementação: I — identificada; aceite final pendente.** Principal desktop e identidades delegadas são derivados das fontes autenticadas, sem owner livre no request.

Evidência: `internal/app/app_command_ui.go`, `internal/commandidentity`. Registros: seções 22, 35–39, 75. Gates: R01, R05.

### C11

- [ ] C11 — Camadas padrão do aplicativo e das surfaces permanecem ativas e um binding ausente em camada superior cai para o default.

**Implementação: I — identificada; aceite final pendente.** Defaults permanentes e fallback estão publicados para as famílias migradas; overrides pessoais não removem as camadas padrão. Provas reais cobrem camada estável, override e restauração, além de requalificação global/workspace. O inventário integral de equivalência permanece em R07, separado deste contrato de defaults.

Evidência: `internal/app/app_command_resolution.go`, `internal/app/app_command_keyboard_defaults_test.go`, `internal/app/app_command_settings_requalification_test.go`; seções111–118 e129. Gates: R02, R07.

### C12

- [ ] C12 — Overrides afetam somente o acionador e contexto declarados.

**Implementação: I — identificada; aceite final pendente.** Overrides e composição por escopo/contexto implementados; projeção e mutação têm testes de isolamento.

Evidência: `internal/commandconfig`, `internal/commandbindings/resolver.go`. Registros: seções 10, 15, 53. Gates: R02.

### C13

- [ ] C13 — Tombstone bloqueia o default no contexto declarado, enquanto personalização apenas desabilitada permite fallback.

**Implementação: I — identificada; aceite final pendente.** Supressão versus disable/fallback exercitados no mapa real, inclusive no default Ctrl+Shift+I.

Evidência: `internal/app/app_command_workspace_context_chat_test.go`, `internal/commandconfig/projection_complete_suppress_test.go`. Registros: seções 53, 68, 75. Gates: R02.

### C14

- [ ] C14 — Tombstones são aplicados antes da deduplicação e nunca produzem invocação.

**Implementação: I — identificada; aceite final pendente.** Tombstones precedem resolução executável; não criam invocação. Apresentação local também não cria ledger.

Evidência: `internal/app/app_command_resolution_test.go`, `internal/commandbindings/resolver.go`. Registros: seções 15, 68. Gates: R01, R02.

### C15

- [ ] C15 — Tombstone que consome um acionador grava marcador terminal no ledger; reentrega do mesmo evento não passa a executar um default após mudança de configuração.

**Implementação: I — identificada; aceite final pendente.** Marcador terminal preserva supressão/replay no caminho durável; local_ui é explicitamente isento de ledger conforme seção 68.

Evidência: `internal/app/app_command_resolution_test.go`, `internal/commandledger/envelope_marker_source_test.go`. Registros: seções 15, 68. Gates: R01.

### C16

- [ ] C16 — Acionamento stale não grava `suppressed`, mas recebe marcador terminal `rejected_stale`; o mesmo ID nunca executa em reentrega posterior.

**Implementação: I — identificada; aceite final pendente.** Recusa stale durável tem marcador próprio e não ressuscita em replay; guards locais não persistem teclas.

Evidência: `internal/app/app_command_resolution_test.go`, `internal/commandexecution/envelope_admission_test.go`. Registros: seções 15, 22, 68. Gates: R01.

### C17

- [ ] C17 — Override de default persiste ID e versão do default substituído.

**Implementação: I — identificada; aceite final pendente.** Referência e versão semântica de defaults estão no delta persistente; upgrades e needs_review têm serviços/testes.

Evidência: `internal/commandconfig/defaults_mutation.go`, `internal/commandconfig/semantic_defaults_test.go`. Registros: seções 10, 53. Gates: R02.

### C18

- [ ] C18 — É possível restaurar um binding, uma camada ou todas as personalizações.

**Implementação: I — identificada; aceite final pendente.** A tela expõe `binding_restore`, `layer_restore` e `config_restore`, usando o serviço comum e escopo explícito. Testes de API comprovam restauração sem remover o global herdado, rollback e persistência após restart; exposição já não é uma pendência.

Evidência: `frontend/src/pages/CommandSettingsPage.tsx`, `internal/app/app_command_settings_contract_test.go`, `internal/app/app_command_settings_requalification_test.go`; seções111–118 e129. Gates: R02, R09.

### C19

- [ ] C19 — Conflitos são detectados considerando a possível interseção de contextos, e empate não executa dois comandos.

**Implementação: I — identificada; aceite final pendente.** Diagnóstico de interseção e empate fail-closed implementados no serviço/resolvedor.

Evidência: `internal/commandconfig/conflict_diagnostics_test.go`, `internal/commandbindings/resolver.go`. Registros: seções 10, 15. Gates: R02, R09.

### C20

- [ ] C20 — Escopo, especificidade e prioridades persistidas produzem resolução determinística após importação/restart; empate termina em conflito fail-closed.

**Implementação: I — identificada; aceite final pendente.** Ordenação persistida e composição determinística têm provas de importação/restart; UI avançada não é necessária para o contrato do resolvedor.

Evidência: `internal/commandbindings/resolver.go`, `internal/app/app_command_storage_settings_restart_test.go`. Registros: seções 40–45, 75. Gates: R02, R05.

### C21

- [ ] C21 — Bindings equivalentes por comando, argumentos e escopo produzem uma única invocação com proveniência preservada.

**Implementação: I — identificada; aceite final pendente.** Equivalentes são deduplicados com proveniência preservada no envelope durável.

Evidência: `internal/commandbindings/resolver.go`, `internal/commandexecution`. Registros: seções 22, 32–33. Gates: R01, R02.

### C22

- [ ] C22 — O resolvedor não consulta SQLite nem percorre o catálogo completo a cada acionamento.

**Implementação: I — identificada; aceite final pendente.** O núcleo `Configuration.Resolve` permanece indexado por trigger e sem SQLite. O mapa físico agora prepara matriz de condições e fatos requeridos antes de abrir a época de entrada. `resolveDeckPress` apenas consulta o índice da tecla, revalida configuração/registry/versões e resolve os candidatos nativos com fatos atuais; não recompõe condições nem lista o catálogo. A seleção contextual continua única e o payload publicado é clonado na borda. Δ18 corrigido na seção130; qualificação de latência integrada permanece em R06.3, sem prometer ganho perceptível ou encerrar C62.

Evidência: `internal/commandbindings/resolver.go`, `internal/app/app_command_deck.go` (`resolveDeckPress`, `deckTriggerIdentities`), `internal/app/app_command_deck_projection_test.go`, `internal/app/app_command_deck_profile_test.go`. Registros: seções 68, 129–130. Gates: R06, R10.

### C23

- [ ] C23 — Mudanças de surface, foco, workspace, janela externa e eventos podem ativar e desativar camadas de forma determinística.

**Implementação: I — identificada; aceite final pendente.** Providers visuais, perfil canônico, snapshot externo e claims/eventos alimentam ativação/resolução determinística. Há prova real de regra contextual editável e invalidação por perfil/ABA. Não se interpreta o critério como todo produto cartesiano de condições/origens: combinações sem suporte são recusadas. A prova integrada positiva específica de programa Windows permanece em C43/R10.

Evidência: `internal/commandconfig/activation_projection.go` (`projectLayerActivation`), `internal/app/app_command_settings_contract_test.go` (`TestCommandSettingsContractContextualRuleActuallyResolves`), `internal/app/app_command_context.go`; seções111–128 e129. Gates: R01, R02, R10.

### C24

- [ ] C24 — Desabilitar camada a remove imediatamente do mapa sem ressuscitar claims stale ao reabilitá-la; expiração local é idempotente após restart.

**Implementação: I — identificada; aceite final pendente.** Enable/disable, revogação e terminalidade das claims implementados; restore global/workspace foi corrigido.

Evidência: `internal/commandconfig/activation_hook.go`, `internal/commandactivation`. Registros: seções 10, 27, 48. Gates: R02.

### C25

- [ ] C25 — Ativações por evento têm ID, sequência, correlação e deduplicação; evento atrasado não encerra ciclo mais novo.

**Implementação: I — identificada; aceite final pendente.** Eventos duráveis, sequência, correlação e isolamento de ciclos ligados ao runtime de jobs.

Evidência: `internal/commandjobevents`, `internal/commandjobactivation`. Registros: seções 26, 29–33. Gates: R03.

### C26

- [ ] C26 — Claim e ledger de ativação preservam o escopo global/workspace, inclusive para refs `builtin`; eventos e replay de outro workspace falham fechado.

**Implementação: I — identificada; aceite final pendente.** Escopos e refs builtin/user são preservados na ativação e projeção, com recusa de owner/workspace divergente.

Evidência: `internal/commandactivation`, `internal/commandjobactivation`. Registros: seções 29–33. Gates: R02, R03.

### C27

- [ ] C27 — A primeira versão aceita apenas fatos de `job_run_events` espelhados transacionalmente na outbox durável; EventBus best-effort e produtores externos falham fechado.

**Implementação: I — identificada; aceite final pendente.** Consumer de outbox montado no App; só fatos duráveis elegíveis entram, não EventBus best-effort.

Evidência: `internal/jobs/command_activation_runtime.go`, `internal/commandjobactivation/consumer.go`. Registros: seções 26–33. Gates: R03.

### C28

- [ ] C28 — Count-cap/cascade de runs não remove a outbox antes do deadline; startup recupera leases e reprocessa pendências antes da retenção de jobs.

**Implementação: I — identificada; aceite final pendente.** Count-cap preserva outbox pending/processing, claim e lease mesmo quando remove runs. Coordinator requeue/drain/recovery precede retenção e impede exclusões enquanto há lotes pendentes; testes reabrem SQLite e verificam recuperação/consumo único, além da sentinela SQL de ordem no restart do App. A matriz adicional command→job→event→command de R03.4 não invalida esta obrigação individual já coberta.

Evidência: `internal/jobs/command_activation_retention_matrix_test.go`, `internal/commandmaintenance/coordinator.go`, `internal/commandjobactivation/recovery_restart_test.go`, `internal/commandjobactivation/maintenance_adapter_test.go`, `internal/app/app_command_maintenance_restart_test.go`; seções28–31 e129. Gates: R03, R04.

### C29

- [ ] C29 — Estado de ativação persistido é reconciliado em modo seguro no startup e preserva autenticação, geração e proveniência anti-loop da AEP-0067.

**Implementação: I — identificada; aceite final pendente.** Recovery seguro ligado ao bootstrap, sem inferir proveniência de registros legados.

Evidência: `internal/app/app_command_maintenance_restart_test.go`, `internal/commandjobactivation`. Registros: seções 24–31. Gates: R03, R04.

### C30

- [ ] C30 — Claim de job sem lease e fonte autoritativa válidas fica inativa.

**Implementação: I — identificada; aceite final pendente.** Lease/heartbeat/fonte autoritativa exercitados com job vivo; perda da fonte encerra claim.

Evidência: `internal/app/app_command_maintenance_restart_test.go`, `internal/commandjobactivation`. Registros: seções 28–30. Gates: R03.

### C31

- [ ] C31 — Replay de ativação fora da retenção é rejeitado, e ownership vem do principal autenticado, não do payload.

**Implementação: I — identificada; aceite final pendente.** Replay expirado e owner não confiável são recusados pelos serviços ligados ao consumer.

Evidência: `internal/commandjobevents`, `internal/commandjobactivation`. Registros: seções 26–33. Gates: R03, R05.

### C32

- [ ] C32 — A Command Palette busca e descreve comandos disponíveis e indisponíveis com motivo, mas executa somente os disponíveis.

**Implementação: I — identificada; aceite final pendente.** Picker real busca/descreve, permite explorar indisponíveis e só executa disponíveis; aceite de uso registrado.

Evidência: `frontend/src/components/layout/Topbar.palette.integration.test.tsx`, `frontend/src/components/layout/Topbar.tsx`. Registros: seções 53, 69, 75. Gates: R08.

### C33

- [ ] C33 — A Command Palette tem navegação completa por teclado, anúncios e restauração de foco cobertos por testes e validação NVDA.

**Implementação: I — identificada; aceite final pendente.** Combobox compartilhado, teclado, anúncios e foco implementados; relato de uso aceito não cobre todo roteiro NVDA final.

Evidência: `frontend/src/components/layout/Topbar.palette.integration.test.tsx`, `frontend/src/components/pickers/Combobox.tsx`. Registros: seções 69, 75. Gates: R08.

### C34

- [ ] C34 — A configuração por chat usa tools estruturadas, IDs reais e confirmações de segurança.

**Implementação: I — identificada; aceite final pendente.** Tools públicas registradas, schemas fechados e IDs reais. Leituras retornam revisões/fingerprints; escrita usa intent, confirmação e publicação compartilhados. Import/export não sensíveis também estão ligados.

Evidência: `internal/tools/command`, `internal/app/app_command_agent_config.go`, `internal/app/app_command_agent_config_test.go`, `internal/app/app_command_agent_portability_test.go`, `internal/app/app_tool_registry.go`; seção134. Gates: R11.

### C35

- [ ] C35 — Toda mutação persistente solicitada por agente mostra diff, exige decisão explícita e falha fechado sem interlocutor.

**Implementação: I — identificada; aceite final pendente.** Todos os verbos mutáveis D10 passam pelo mesmo applier/decisão; CRUD, restore e import têm provas reais. Recusa, sessão revogada após prompt e revisão stale não gravam; headless não recebe a autoridade da GUI. Exportação sensível é rejeitada, não promovida.

Evidência: `internal/app/app_command_agent_config_test.go`, `internal/app/app_command_agent_portability_test.go`, `internal/app/app_command_agent_authority_test.go`, `internal/tools/command/tools_test.go`; seção134 e serviços comuns das seções111–118. Gates: R02, R11.

### C36

- [ ] C36 — `command_catalog.execute` aplica o mesmo gate a comandos que alteram capacidade efetiva, incluindo ativação de camada.

**Implementação: I — identificada; aceite final pendente.** command_catalog.execute usa o executor integral como ActorAgent. workspace.list não pede decisão; layer.activate/toggle/back passam pelo gate de mutabilidade. Receipt consumido, recusa sem efeito e replay pelo ID da tool são exercitados com store/handler reais.

Evidência: `internal/app/app_command_agent_execute.go`, `internal/app/app_command_agent_test.go`, `internal/app/app_command_layer_catalog.go`, `internal/commandexecution`; seção134. Gates: R02, R05, R11.

### C37

- [ ] C37 — A tela de configuração oferece lista de camadas, detalhe de ativação e bindings, captura de teclas e explicação do resultado efetivo.

**Implementação: I — identificada; aceite final pendente.** Lista/detalhe, regras de ativação, bindings, captura teclado/Deck/sequências, condições e diagnósticos estão expostos. Teste de contrato prova que a regra editável altera a resolução real; não se trata só de persistir uma configuração sem execução.

Evidência: `frontend/src/pages/CommandSettingsPage.tsx`, `internal/app/app_command_settings_contract_test.go`, `frontend/src/lib/commandSettingsConditions.ts`; seções111–128 e129. Gates: R09.

### C38

- [ ] C38 — Toda configuração é operável por teclado e NVDA sem depender de grade, arrastar, imagem ou cor.

**Implementação: P — parcial.** Controles existentes são textuais, localizados e operáveis por lista/teclado. As seções142–145 acrescentam títulos Deck por idioma, ícones, imagens e feedback transitório acessível. A seção146 implementou variantes por estado e indicação persistente do estado efetivo de camada. Esses recursos não são mais lacunas de código. A classificação conservadora de C38 é mantida até a qualificação integral de operação por teclado/NVDA e dispositivo; não representa ausência das variantes já entregues.

Seção157: corrigida a perda de foco ao adicionar/remover condições e argumentos;
instruções da DataGrid localizadas e limitadas às ações disponíveis, incluindo
Shift+F10/Menu. Testes de teclado/DOM não substituem NVDA. Roteiro com resultados
por operação: `docs/content/guias/VALIDACAO_COMANDOS_NVDA.md`.

Evidência: `frontend/src/pages/CommandSettingsPage.tsx`, `frontend/src/pages/CommandSettingsPage.test.tsx`, `internal/app/app_command_deck_feedback_announce_test.go`, `frontend/src/lib/subscribeCommandDeckFeedback.test.ts`, `docs/content/recursos/COMANDOS.md`; seções129 e142–146. Gates: R09.

### C39

- [ ] C39 — O Assistente controla ao menos um modelo de Stream Deck diretamente por Go, sem software oficial, com reconexão e shutdown limpo.

**Implementação: I — identificada; aceite final pendente.** Driver e gerência Go ligados ao App e tecla real validada; unplug/replug e shutdown físico integrado ainda exigem aceite.

Evidência: `internal/commanddeck/manager.go`, `internal/app/app_command_deck.go`. Registros: seções 17, 56–58, 69. Gates: R10.

### C40

- [ ] C40 — Sem sessão autenticada, e durante logout ou troca de usuário, o Stream Deck fica em estado seguro e rejeita callbacks de gerações anteriores.

**Implementação: I — identificada; aceite final pendente.** Sessão/geração e callbacks antigos são guardados no runtime do Deck; matriz física de troca de usuário ainda não foi aceita.

Evidência: `internal/app/app_command_deck.go`, `internal/commandadapter`. Registros: seções 17, 56–58. Gates: R01, R10.

### C41

- [ ] C41 — O Stream Deck atualiza somente teclas cujo conteúdo efetivo mudou e usa cache de imagens.

**Implementação: I — identificada; aceite final pendente.** App projeta título/estado em Frame e usa renderer com diff/cache; personalização visual avançada é obrigação mais ampla de R10.

Evidência: `internal/app/app_command_deck.go`, `internal/commanddeck/renderer.go`. Registros: seções 17, 56. Gates: R10.

### C42

- [ ] C42 — Abertura/reconexão do Stream Deck invalida o diff e força frame completo.

**Implementação: I — identificada; aceite final pendente.** Runtime/renderer invalidam frame por handle/reconexão; teste físico completo segue pendente.

Evidência: `internal/commanddeck/manager.go`, `internal/commanddeck/renderer_test.go`. Registros: seções 17. Gates: R10.

### C43

- [ ] C43 — Camadas baseadas no programa em primeiro plano funcionam no Windows e degradam explicitamente em plataformas sem adapter.

**Implementação: I — identificada; aceite final pendente.** Provider Windows e condições por `foreground.process` possuem prova integrada de configuração confirmada → captura por evento → executor real → claim de ativação → novo mapa Deck. Processo divergente e falha de captura recusam a ação; mudança de foco após a captura não altera a ocorrência. A construção do snapshot preserva a identidade opaca e a auditoria persiste somente o resumo permitido em D14. A fronteira do SO é controlada nos testes; não equivale ao aceite físico Win32/HID nem à execução da suíte não-Windows.

Evidência: `internal/app/app_command_foreground_deck_integration_test.go`, `internal/app/app_command_foreground_global_integration_test.go`, `internal/app/app_command_foreground_unavailable_test.go`, `internal/commandforeground/snapshot_test.go`, `internal/commandledger/foreground_redaction_test.go`, `internal/commandforeground/native_other_test.go`; seções116–118,129 e147. Gates: R01, R10, sem novo aceite.

### C44

- [ ] C44 — Contexto externo é capturado antes de bring-to-front e não muda no meio do acionamento.

**Implementação: I — identificada; aceite final pendente.** Migração global captura o foreground antes de bring-to-front e guarda a cópia na ocorrência. A resolução reutiliza o snapshot original; falta de captura não é disfarçada por recaptura depois de mostrar a janela. Aceite físico permanece separado.

Evidência: `internal/app/app_command_global_execution.go` (`prepareGlobalOccurrence`), `internal/app/app_command_context.go` (`commandOriginFactsFromSnapshot`), `internal/app/app_command_physical_origin_guards_test.go`; seções109–110 e129. Gates: R01, R10.

### C45

- [ ] C45 — Comandos disparados fora de foco preservam permissões, decisões e auditoria do executor de destino.

**Implementação: I — identificada; aceite final pendente.** Voz/jobs globais usam ocorrência privada, executor autorizado, decisão e runtime/tool comuns; não há execução direta alternativa no callback. As provas de job real incluem admissão confirmada e negada. Não equivale a aceitar toda a matriz física de segundo plano.

Evidência: `internal/app/app_command_global_execution.go`, `internal/app/app_command_global_job_integration_test.go` (`TestCommandGlobalJobHotkeyRunsThroughAdmissionDecisionAndRealTool`); seções109–110 e129. Gates: R05, R10.

### C46

- [ ] C46 — Exportação/importação preserva UUIDs e escopos, relata referências e conflitos e não transfere grants nem histórico de invocações.

**Implementação: I — identificada; aceite final pendente.** Envelope comum, UUIDs/escopos, relatório e exclusão de grants/histórico ligados à UI; matriz entre instâncias/usuários ainda exige qualificação.

Evidência: `internal/app/app_command_export_desktop.go`, `internal/commandportability`. Registros: seções 40–45. Gates: R05, R11.

### C47

- [ ] C47 — Binding persistente e export não contêm segredos brutos; delegação a tool propaga redação ou permanece indisponível.

**Implementação: I — identificada; aceite final pendente.** Redação e recusa de legado com segredo implementadas; export sensível continua indisponível, não bypass.

Evidência: `internal/commandportability`, `internal/commandtoolbridge`. Registros: seções 37–45. Gates: R05, R11.

### C48

- [ ] C48 — Referência importada de credencial resolve pattern exato no usuário de destino ou deixa o binding desabilitado.

**Implementação: I — identificada; aceite final pendente.** F02 corrigido no plano e snapshot aplicado: pattern literal do destino ou binding desabilitado.

Evidência: `internal/commandportability/import_store_references_test.go`, `internal/commandportability/credential_resolver.go`. Registros: seções 10, 40. Gates: R05, R11.

### C49

- [ ] C49 — `command_invocations` tem payload redigido, origem rastreável, índices e retenção por idade e quantidade, sem prometer reconstruir o snapshot completo.

**Implementação: I — identificada; aceite final pendente.** Auditoria redigida e retenção por owner/idade/caps ligadas à cadência de instância.

Evidência: `internal/commandledger`, `internal/app/app_command_maintenance_restart_test.go`. Registros: seções 22, 27–28. Gates: R01, R04.

### C50

- [ ] C50 — `command_invocations.invocation_id` é a PK canônica da invocação, consulta e correlação com tools; o ledger tem PK própria `id` e referências UNIQUE explícitas.

**Implementação: I — identificada; aceite final pendente.** PK de invocação, ledger separado e correlação de tools implementados.

Evidência: `internal/commandledger/models.go`, `internal/commandtoolbridge`. Registros: seções 22, 37–38. Gates: R01, R04.

### C51

- [ ] C51 — Reentrega dentro da janela retorna status/resultado redigido sem repetir o handler; invocações interrompidas por queda viram `outcome_unknown`.

**Implementação: I — identificada; aceite final pendente (seção157).** Além do replay e recovery transacional, o teste opt-in aprovado encerra à força somente um processo filho de teste após seu efeito SQLite confirmado. A instância seguinte obtém a lease nativa e o RestartProof real, recupera ledger/auditoria para `outcome_unknown` com resumo redigido e repete consulta/recovery/replay sem reiniciar o handler nem repetir o efeito. Não simula queda por cancelamento ou shutdown.

Evidência: `internal/commandledger/recovery.go`, `internal/commandexecution/service_test.go`, `internal/app/app_command_maintenance_restart_test.go`, `internal/commandexecution/crash_recovery_process_test.go`; seções24–28,129 e157. Gates: R01, R04. Prova isolada em banco descartável; não equivale a desligamento do sistema operacional ou teste do app Wails aberto.

### C52

- [ ] C52 — Evento durável preserva a chave pelo horizonte de replay da fonte e, depois dele, é rejeitado por `source_occurred_at` autenticado em vez de ser tratado como solicitação nova.

**Implementação: I — identificada; aceite final pendente.** Horizonte de replay e rejeição por origem temporal autenticada implementados.

Evidência: `internal/commandjobevents`, `internal/commandexecution/replay_policy_test.go`. Registros: seções 27, 31. Gates: R03, R04.

### C53

- [ ] C53 — Ativações por evento persistem o mesmo epoch/deadline imutável da fonte; aumentar retenção não reabre ocorrência antiga.

**Implementação: I — identificada; aceite final pendente.** Epoch/deadline imutáveis por ocorrência, sem reabrir evento por mudança de retenção.

Evidência: `internal/commandjobevents`, `internal/commandmaintenance`. Registros: seções 27–31. Gates: R03, R04.

### C54

- [ ] C54 — Recuperação de startup atualiza auditoria e ledger para `outcome_unknown` na mesma transação.

**Implementação: I — identificada; aceite final pendente.** Recovery transacional auditoria+ledger ligado ao bootstrap com prova de encerramento de geração.

Evidência: `internal/commandledger/recovery.go`, `internal/app/app_command_maintenance_restart_test.go`. Registros: seções 24–28. Gates: R04.

### C55

- [ ] C55 — Reutilizar `invocation_id` com request fingerprint diferente falha fechado.

**Implementação: I — identificada; aceite final pendente.** Fingerprint divergente para o mesmo ID é recusado pelo ledger e pipeline.

Evidência: `internal/commandledger`, `internal/commandexecution`. Registros: seções 15, 22. Gates: R01.

### C56

- [ ] C56 — Caps de auditoria não removem os ledgers antes de `expires_at`; compactar registro recente não permite nova execução ou ativação.

**Implementação: I — identificada; aceite final pendente.** Caps não antecipam expiração dos ledgers; coordinator tem provas de múltiplos owners e manutenção.

Evidência: `internal/commandledger/coordinator_adapter_test.go`, `internal/commandmaintenance`. Registros: seções 27–28. Gates: R04.

### C57

- [ ] C57 — Consulta de invocação aplica propriedade por usuário e autorização do ator, sem lookup cross-user apenas pela PK.

**Implementação: I — identificada; aceite final pendente.** Consulta é autorizada por owner/ator, sem lookup livre por UUID.

Evidência: `internal/commandexecution`, `internal/commandledger/durable_integration_test.go`. Registros: seções 22, 35–39. Gates: R01, R05.

### C58

- [ ] C58 — Sessão, geração de segurança e staleness de contexto são revalidados imediatamente antes de todo handler.

**Implementação: I — identificada; aceite final pendente.** Revalidação imediatamente antes do handler/commit e antes do efeito local implementada.

Evidência: `internal/app/app_command_ui.go`, `frontend/src/lib/commandUIEffect.ts`. Registros: seções 22–23, 61–75. Gates: R01.

### C59

- [ ] C59 — Policies `max_age_ms`/`event_snapshot` falham fechado sem timestamp de cada provider; ingresso não transforma snapshot sem `capturedAt` em contexto recém-capturado.

**Implementação: I — identificada; aceite final pendente.** Policies temporais usam capturedAt dos providers e recusam provas incompletas.

Evidência: `internal/commandcontext`, `internal/app/app_command_context_test.go`. Registros: seções 19–23. Gates: R01.

### C60

- [ ] C60 — `handler.Start` confirma handoff sem bloquear; logout/mutação concorrente não espera o trabalho longo nem entra em deadlock.

**Implementação: I — identificada; aceite final pendente.** Handoff não bloqueante, drain e cancelamento estão implementados; não representam benchmark fim a fim.

Evidência: `internal/commandexecution/core_drain_test.go`, `internal/app/app_command_drain.go`. Registros: seções 22–24. Gates: R01.

### C61

- [ ] C61 — Versões do catálogo e da configuração são revalidadas ao retirar da fila; binding alterado não executa resolução antiga.

**Implementação: I — identificada; aceite final pendente.** Versões/gerações revalidadas no gate; reservas antigas recusadas após rebuild/mutação.

Evidência: `internal/commandexecution/envelope_pipeline.go`, `internal/app/app_command_resolution_test.go`. Registros: seções 15, 18, 22. Gates: R01.

### C62

- [ ] C62 — Cache de resolução inclui usuário, workspace, acionador, origem, `context_version` e todas as versões/gerações de catálogo, configuração e camadas ativas.

**Implementação: I — identificada; aceite final pendente.** O runtime produtivo possui LRU de 256 seleções, usado por `resolvePersistedTrigger`. A chave inclui usuário, workspace nullable, acionador, origem, versão de contexto composta com fatos tipados e snapshots de origem/foreground, registry e gerações global/workspace/camadas. Cada runtime isola sessão/workspace; nova configuração invalida entradas positivas e negativas, e shutdown limpa/fecha o cache. Resultados copiam BindingIDs e LayerRefs. Autenticação, fonte, ocorrência, TTL, UI, autorização e commit permanecem fora do cache. A navegação LOCAL_UI não ganha IPC nem ledger. Evidência de wiring real, suppress/rebuild e cofre bloqueado na seção131; desempenho integrado permanece em R06.3.

Evidência: `internal/commandcontext/cache.go`, `internal/commandcontext/cache_test.go`, `internal/app/app_command_resolution_cache.go`, `internal/app/app_command_resolution_cache_test.go`, `internal/app/app_command_resolution.go`, `internal/app/app_command_product.go`; seções19–23, 129–131. Gates: R01, R06.

### C63

- [ ] C63 — Cada comando declara `context_policy`; nas policies que declaram providers, provider ausente ou versão/TTL inválido falha fechado.

**Implementação: I — identificada; aceite final pendente.** Contratos exigem providers/versionamento coerentes; ausência/TTL inválido falham fechado.

Evidência: `internal/commandcatalog/registry_test.go`, `internal/commandcontext`. Registros: seções 19–23. Gates: R01.

### C64

- [ ] C64 — `context_policy = none` é rejeitado para qualquer comando não read-only.

**Implementação: I — identificada; aceite final pendente.** Registro rejeita none para efeito não read-only; catálogo de produto usa contratos específicos.

Evidência: `internal/commandcatalog/registry_test.go`, `internal/app/app_command_product_catalog.go`. Registros: seções 19, 62–75. Gates: R01.

### C65

- [ ] C65 — Contextos local, JWT externo, job e system têm fontes de identidade e revogação explícitas; `EpochService` invalida trabalho obsoleto.

**Implementação: I — identificada; aceite final pendente.** Identidades local, job e system mantêm fontes próprias. A seção156 completa a composição externa App/HTTP com `(iss, sub)` administrativo, contexto por token, epochs/revogação, execução/lookup/revoke e interface somente quando explicitamente vinculada; `workspace.list` backend mantém escopo por usuário. Não empresta sessão local e não habilita comandos físicos. Suítes externas App/frontend passaram; revisões de hardening, frontend e fingerprint sem achados. Commit `0548492ab`; repetição dos grupos App afetados PASS 21,938 s após corrigir o golden intermediário da rodada ampla. Sem aceite R05.

Evidência: `internal/commandidentity/service_test.go`, `internal/commandidentity/core_epochs_test.go`, `internal/commandexecution/external_test.go`, `internal/app/app_command_external_execution_test.go`, `internal/app/app_httpapi_external_ui_test.go`, `internal/httpapi/external_commands_test.go`, `internal/commandui/external_connections_test.go`; commit `1aa9e7c6c` e código/testes externos da seção156. Gates: R05.

### C66

- [ ] C66 — Ativação event-driven usa grants próprios de camada, com chave natural, geração monotônica, histórico de revogação e revalidação autoritativa por evento; não reutiliza nem amplia grants de delegação da AEP-0101.

**Implementação: I — identificada; aceite final pendente.** Grants próprios e revogação de ativação têm store/caller real; não reutilizam grants de delegação.

Evidência: `internal/commandconfig/activation_hook.go`, `internal/commandautomation`. Registros: seções 26–33. Gates: R02, R03.

### C67

- [ ] C67 — Adapter de jobs exige `job_slug = Job.ID` e `job_database_id = Job.DatabaseID`, confirma ambos por owner e permanece desabilitado para fatos legados ambíguos.

**Implementação: I — identificada; aceite final pendente.** Slug/DatabaseID e owner verificados; fatos legados ambíguos recusados.

Evidência: `internal/commandjobactivation`, `internal/jobs/command_service_identity.go`. Registros: seções 29–33, 39. Gates: R03.

### C68

- [ ] C68 — Evento de ativação recebido é candidato sem autoridade; dispatcher deriva owner, workspace, regra, layer e epochs antes do envelope interno.

**Implementação: I — identificada; aceite final pendente.** Dispatcher deriva autoridade do candidato, sem confiar no owner/regra enviados no fato.

Evidência: `internal/commandjobactivation`, `internal/commandexecution`. Registros: seções 26–33. Gates: R03.

### C69

- [ ] C69 — Regras e layers builtin/user usam refs polimórficas consistentes no schema, grants, estado, ownership, importação e restore.

**Implementação: I — identificada; aceite final pendente.** Refs polimórficas builtin/user usadas por store, grants, importação e restore.

Evidência: `internal/commandactivation`, `internal/commandconfig`, `internal/commandportability`. Registros: seções 10, 29–33, 40–45. Gates: R02, R05.

### C70

- [ ] C70 — Após o PR atualizar a AEP-0052, identidade externa só acessa usuário local por mapeamento administrativo exato de emissor e subject; antes disso, o command manager fica indisponível nesse modo.

**Implementação: I — identificada; aceite final pendente.** O cutover `(iss, sub)` e a montagem App/HTTP usam JWT revalidado, roles/scopes e usuário local mapeado sem fallback para subject ou sessão desktop como identidade. O runtime montado ainda exige o mesmo usuário local ativo. Testes externos App e frontend passam; `go test ./internal/command... -count=1` passou nos 32 pacotes, assim como `commandexecution` após hardening e `tsc --noEmit`. A suíte App ampla teve uma única falha de golden compilada de uma revisão intermediária; a fonte foi corrigida e o rerun focalizado passou em 21,938 s. Commit `0548492ab`. A conexão UI é explícita e temporária. Não se declara aceite/readiness além dos fluxos testados.

Evidência: `internal/auth/command_external.go`, `internal/auth/command_external_identity.go`, `internal/auth/external_identity_enrollment.go`, `internal/app/app_httpapi_external_ui_test.go`, `internal/app/app_command_workspace_list_external_test.go`, `internal/httpapi/external_commands_test.go`, `frontend/src/services/externalUIConnection.test.ts`; seções149–156. Gates: R05.

### C71

- [ ] C71 — Cada comando declara origens permitidas e o serviço bloqueia origem não autorizada, incluindo comandos visuais solicitados pela CLI.

**Implementação: I — identificada; aceite final pendente.** Catálogo e executor restringem origens, inclusive pela CLI pública. Descoberta mostra indisponibilidade; execute recusa comandos visuais/workspace/camada sem presenter e sem efeito. Lookup CLI não lê invocações da paleta; sessão, papel e estado de segurança são revalidados. Evidência adicional: `internal/app/app_command_cli_test.go`, `internal/commandcli/integration_test.go`, `cmd/asst/commands_test.go`; seção135.

Evidência: `internal/commandcatalog/readiness_test.go`, `internal/app/app_command_global_execution_test.go`, `cmd/asst`, `internal/tools`; seção129. Gates: R05, R11.

### C72

- [ ] C72 — `effect_class` e mutabilidade vêm do contrato do handler; metadata divergente impede o registro.

**Implementação: I — identificada; aceite final pendente.** Registro valida Effect/HasMutableTarget/Classificação contra contrato do handler.

Evidência: `internal/commandexecution/envelope_engine_test.go`, `internal/commandcatalog/registry_test.go`. Registros: seções 19, 62–75. Gates: R01, R07.

### C73

- [ ] C73 — CLI não executa comando que exija diálogo/decisão interativa.

**Implementação: I — identificada; aceite final pendente.** CLI list/describe/execute/retry/status registrada, sem presenter e sem flag de aprovação. Disponibilidade e autorização compartilham os gates headless; teste de integração verifica status denied, erro, nenhum questionário e nenhuma execução bem-sucedida para ações restritas. Evidência adicional: `internal/app/app_command_cli_test.go`, `internal/commandcli`, `cmd/asst/commands_test.go`; seção135.

Evidência: `internal/commandcatalog/registry_test.go`, `internal/commandexecution/envelope_pipeline_test.go`, `cmd/asst`; seção129. Gates: R05, R11.

### C74

- [ ] C74 — Comando destrutivo só avança com receipt de decisão criada no backend, vinculada à solicitação e consumida uma vez no CAS para `queued`.

**Implementação: I — identificada; aceite final pendente.** Receipt de decisão destrutiva é backend, vinculada e consumida atomicamente pelo pipeline.

Evidência: `internal/commandexecution/envelope_handler_decision_test.go`, `internal/commanddecision`. Registros: seções 22, 35. Gates: R01, R02.

### C75

- [ ] C75 — `cli`, `event` e `system` não registram/executam comando destrutivo; qualquer origem sem presenter interativo falha fechado.

**Implementação: I — identificada; aceite final pendente.** Catálogo rejeita fontes headless para destrutivos; indisponibilidade é garantia, não necessidade de habilitar esses comandos.

Evidência: `internal/commandcatalog/registry_test.go`. Registros: seções 19, 35. Gates: R01, R05.

### C76

- [ ] C76 — Em autenticação externa, adapters físicos permanecem indisponíveis até existir vínculo local explícito e revogável com um principal externo.

**Implementação: I — identificada; aceite final pendente.** Identidade externa física continua recusada explicitamente; broker fora de escopo.

Evidência: `internal/commandexecution/envelope_identity_test.go`. Registros: seções R05, R10. Gates: R05, R10.

### C77

- [ ] C77 — Estação bloqueada suspende hotkeys globais e dispositivos físicos e apresenta estado seguro até revalidar a sessão após desbloqueio.

**Implementação: I — identificada; aceite final pendente.** Monitor SO, fechamento de mapas e revalidação no unlock implementados; Deck tem prova de estado seguro. A seção132 acrescenta prova na borda privada do ingresso global: perfil persistido, recusa durante lock e antes do rebuild, ocorrências anteriores rejeitadas tanto após unlock quanto após republicação, e nova execução concluída pelo executor/ledger depois do bootstrap produtivo. Não há republicação manual do mapa no teste. A injeção da observação do SO não substitui aceite físico de bloquear/desbloquear Windows.

Evidência: `internal/app/app_command_os_session.go`, `internal/app/app_command_deck_test.go` (`TestCommandDeckNativeDriverExecutesUIAndBlanksOnLock`), `internal/app/app_command_global_execution.go`; seções109–110 e129. Gates: R01, R10.

### C78

- [ ] C78 — Diálogo topmost bloqueia fallback para camadas inferiores e os atalhos obrigatórios da AEP-0091 não aceitam tombstone.

**Implementação: I — identificada; aceite final pendente.** Stack real bloqueia fallback local, mantém invariantes fora de tombstones e governa a reserva temporária nativa de Ctrl+Shift+R. Modal superior não-decisão não repete a decisão inferior; inputs/IME e eventos antigos são recusados. A reserva expira sem renderer e não autoriza a decisão. Aceite físico/NVDA permanece pendente.

Evidência: `frontend/src/lib/commandBridgeContext.ts`, `frontend/src/lib/modalRegistry.ts`, `frontend/src/lib/decisionRepeatHotkey.test.ts`, `frontend/src/components/ui/DecisionDialog.nativeRepeat.integration.test.tsx`, `internal/app/app_decision_repeat_hotkey_test.go`; seção133. Gates: R01, R07.

### C79

- [ ] C79 — Dispatcher reserva atalhos invariantes do diálogo antes de qualquer binding configurável.

**Implementação: I — identificada; aceite final pendente.** Bridge retorna `dialog-reserved` antes do binding configurável; o Manager agora suspende a captura inferior antes de adquirir a reserva temporária e exige ACK de ownership. Há no máximo uma captura por combinação; fechar o diálogo só restaura o registro ainda existente. Falhas permanecem fechadas, sem ressuscitar a reserva encerrada nem disparar o comando inferior. Stop/close concorrentes têm provas próprias.

Evidência: `frontend/src/lib/commandBridgeContext.ts`, `internal/hotkey/hotkey.go`, `internal/hotkey/manager_temporary_test.go`, `internal/app/app_hotkeys.go`, `internal/app/app_decision_repeat_hotkey.go`; seção133. Gates: R01, R07.

### C80

- [ ] C80 — Shell continua passando exclusivamente por `internal/commandpolicy`.

**Implementação: I — identificada; aceite final pendente.** Delegação continua pelo executor de tools e shell/commandpolicy, sem rota alternativa criada.

Evidência: `internal/tools/shell`, `internal/commandtoolbridge`. Registros: seções 35–38. Gates: R05, R11.

### C81

- [ ] C81 — Manutenção em escopo de instância cobre todos os usuários e registros `system` em uma única cadência.

**Implementação: I — identificada; aceite final pendente.** Uma cadência montada cobre múltiplos usuários e system; gate R04 aceito.

Evidência: `internal/app/app_command_maintenance_restart_test.go`, `internal/commandmaintenance`. Registros: seções 26–28. Gates: R04.

### C82

- [ ] C82 — Deep links e configurações importadas não concedem execução arbitrária.

**Implementação: I — identificada; aceite final pendente.** Deep link não é autorização; import exige escopo/owner/decisão e não importa grants.

Evidência: `internal/tools/deeplink`, `internal/commandportability`. Registros: seções 40–45. Gates: R05, R11.

### C83

- [ ] C83 — Testes cobrem fallback de defaults, sobreposição, múltiplas camadas, modais, inputs, múltiplas abas, troca de foco, reconexão de dispositivo e prevenção de execução duplicada.

**Implementação: I — identificada; aceite final pendente.** A seção148 fecha a lacuna integrada diálogo × hotkey global conflitante: captura controlada → Manager real compartilhado → reserva temporária → restauração → admissão/decisão → executor → um único job e registro terminal. A matriz reexecutada cobre defaults, sobreposição, camadas, inputs/IME, abas/foco, reconexão e replay. O adapter do SO e o renderer são qualificados por fronteiras separadas, não por teste físico/Wails ponta a ponta. Aceite físico/NVDA, detector de corrida indisponível nesta máquina e encerramento agregado permanecem em R12.

Evidência: `frontend/src/lib/commandLocalKeyboard.*.test.ts`, `frontend/src/lib/commandContextProduct.integration.test.tsx`, `internal/app/app_command_deck_test.go`, `internal/app/app_command_global_execution_test.go`, `internal/app/app_command_dialog_global_integration_test.go`, `internal/app/app_command_dialog_manager_integration_test.go`, `internal/hotkey/manager_temporary_test.go`; seções108–129,137 e148. O teste antigo de callbacks permanece; o novo compõe o Manager produtivo em vez de pressupor sua arbitragem. Gates: R06, R07, R12, sem promoção de aceite agregado.

### C84

- [ ] C84 — Apresentação local utiliza a projeção efetiva e valida o contexto na UI, sem invocação/ledger por tecla; persistência da última seleção é separada.

**Implementação: I — identificada; aceite final pendente.** Apresentação local usa mapa efetivo e guards, sem invocação/ledger por tecla; última seleção persiste separadamente.

Evidência: `frontend/src/lib/commandLocalUI.ts`, `frontend/src/store/workspaceStore.ts`, `internal/app/app_command_deck.go`. Registros: seções 68–69, 75. Gates: R07, R08, R10, R12.

## 6. Reconciliação dos 84 itens de infraestrutura

Mapeamento histórico integral preservado. “Marcado no histórico” e “aberto” abaixo descrevem a baseline, não o código atual. A situação atual dos gates correspondentes está na seção 4; em particular R01/R04 já têm saídas aceitas. Esta revisão não recertifica individualmente os 84 itens I nem converte seus 53 checkboxes em porcentagem. R06.1 permanece aberto; não reimplementar serviços já prontos para atualizar uma contagem.

- **I01.1** — Unificar a ordem e o versionamento das migrações no banco real, com teste de banco novo, upgrade e schema incompatível; falha não publica readiness. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R06.
- **I01.2** — Implementar provisionamento idempotente e carregamento da chave de fingerprint no escopo correto; não reutilizar JWT/pepper nem substituir chave existente ao reiniciar. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R06.
- **I01.3** — Definir versão ativa, retenção das versões antigas pelo maior deadline dos ledgers e procedimento testado de rotação; ausência/corrupção falha fechado. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R06.
- **I01.4** — Testar primeira abertura, reinício, cofre indisponível e concorrência de inicialização usando diretórios e credenciais de teste. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R06.
- **I02.1** — Completar schema de argumentos, resultado e envelope versionado para origens/contextos previstos; validar grupos opcionais, nulabilidade e limites antes da reserva. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R06.
- **I02.2** — Implementar canonicalização RFC 8785 do conjunto suportado e HMACs com separação de domínio para argumentos, request e demais fingerprints; cobrir números, Unicode, objetos e campos excluídos. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R06.
- **I02.3** — Calcular fingerprint semântico completo dos defaults; apresentação puramente visual não invalida semântica executável. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R06.
- **I02.4** — Fechar contrato de classificação do handler, origens, contexto, sensibilidade, disponibilidade e apresentação localizada; testar catálogo de contratos, sem cadastrar todos os comandos reais. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R06.
- **I02.5** — Definir compatibilidade de versões e corpus de testes compartilhado entre persistência, importação e ingresso. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R06.
- **I03.1** — Implementar providers de surface, diálogo, foco/controle, aba, workspace, perfil e janela; validar ownership na fonte, não confiar no snapshot enviado pela UI. Estado documental anterior: aberto. Fechamento/revalidação: R01.
- **I03.2** — Implementar ContextFactBus e reconciliação síncrona quando uma notificação se perder ou chegar fora de ordem. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01.
- **I03.3** — Separar gerações globais, por workspace e de camadas efetivas; mudança em outra conta/workspace não invalida trabalho independente. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01.
- **I03.4** — Completar políticas exact_version, max_age_ms e event_snapshot no percurso de admissão, com timestamps confiáveis por provider. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01.
- **I03.5** — Implementar captura de foreground antes de bring-to-front, redação de resumo e degradação explícita onde não houver adapter; não persistir títulos/URLs. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01.
- **I03.6** — Testar atualização de contexto concorrente, provider ausente e caches positivos/negativos com todas as dimensões de isolamento. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01.
- **I04.1** — Unificar execução direta e por trigger: resolver, fixar origem vencedora, derivar ator e normalizar argumentos antes de assinar/reservar. Estado documental anterior: aberto. Fechamento/revalidação: R01.
- **I04.2** — Ampliar reserva e auditoria para argumentos, triggers, origem física/evento, workspace e contexto; preservar IDs canônicos e rejeitar fingerprint divergente. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01.
- **I04.3** — Persistir recusas pós-autenticação e marcadores terminais suppressed/rejected_stale; reentrega não pode passar a executar após alteração de configuração. Estado documental anterior: aberto. Fechamento/revalidação: R01.
- **I04.4** — Completar gates evaluating → queued → running para read/write/destructive, revalidando política, catálogo, mapa, contexto e decisão no ponto correto. Estado documental anterior: aberto. Fechamento/revalidação: R01.
- **I04.5** — Preservar handoff não bloqueante, finalização atômica, resultado redigido, consulta autorizada independente da versão atual e reconciliação auditada de outcome_unknown sem reexecução. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01.
- **I04.6** — Testar filas, duplicidade, perda de ack, panic, cancelamento, mudança de configuração e falhas transacionais com handlers controlados. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01.
- **I05.1** — Completar projeção de configuração global + workspace, condições, argumentos, tipos de acionador e apresentação; nenhuma leitura bruta vira autorização. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02.
- **I05.2** — Implementar criar/editar/excluir/habilitar/desabilitar bindings e layers com ownership, validação de referências e CAS de geração. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02.
- **I05.3** — Implementar restauração persistente por binding, camada e conjunto; não apagar defaults nem conceder grants. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02.
- **I05.4** — Completar upgrade de defaults: versão sem mudança semântica, needs_review no contexto exato e rebase confirmado; eliminar bloqueio excessivamente amplo do protótipo. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02.
- **I05.5** — Expor serviço interno único de conflito/diagnóstico e diff exato, reutilizável por UI/chat/importação; testar corrida entre checagem e commit. Estado documental anterior: aberto. Fechamento/revalidação: R02.
- **I06.1** — Ampliar receipt para invocação e consumir decisão destrutiva na mesma transação do CAS para queued. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02.
- **I06.2** — Aplicar diff/receipt/auditoria a todo CRUD, restore, import e alteração de capacidade; incorporar a classificação obrigatória de cada verbo. Estado documental anterior: aberto. Fechamento/revalidação: R02.
- **I06.3** — Registrar presenter autenticado no ciclo apropriado; cancelamento, timeout, logout e resposta tardia não deixam autorização reutilizável. Estado documental anterior: aberto. Fechamento/revalidação: R02.
- **I06.4** — Revalidar gerações de configuração/grant entre apresentação e commit; negar origem headless onde o contrato exige interlocutor. Estado documental anterior: aberto. Fechamento/revalidação: R02.
- **I06.5** — Testar decisão manipulada, replay, rollback e correlação; validar contrato do diálogo compartilhado sem duplicar UI. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02.
- **I07.1** — Implementar schema/repository de regras, estado e referências builtin/user com isolamento global/workspace. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02.
- **I07.2** — Implementar união de claims, pin, toggle/back e manual_stack_key derivada da origem; uma regra não encerra claim de outra. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02.
- **I07.3** — Implementar expiração idempotente e estados terminais que não ressuscitam, inclusive após restart. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02.
- **I07.4** — Implementar disable/enable com revalidação das claims e atualização atômica das gerações efetivas. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02.
- **I07.5** — Implementar restore autenticado de claims manuais persistentes; rebind de sessão/gerações/dispositivo e revisão quando a origem não existir. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02.
- **I07.6** — Testar ciclos concorrentes, condições recalculadas, isolamento e reinicialização sem reativação indevida. Estado documental anterior: aberto. Fechamento/revalidação: R02.
- **I08.1** — Persistir chave natural por owner/workspace/layer/rule, uma concessão ativa e histórico de gerações/revogações. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02/R03.
- **I08.2** — Criar/habilitar regra event-driven somente com decisão vinculada ao fingerprint exato da regra e dos produtores. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02/R03.
- **I08.3** — Revogar atomicamente ao alterar/excluir/desabilitar regra ou camada; reabilitação exige nova decisão. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02/R03.
- **I08.4** — Revalidar ID, geração e fingerprints autoritativos a cada evento; import/cópia/restore nunca transportam concessão. Estado documental anterior: aberto. Fechamento/revalidação: R02/R03.
- **I08.5** — Testar concessão/revogação concorrente, receipt atrasada e isolamento entre grants de delegação e ativação. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R02/R03.
- **I09.1** — Migrar timeline/status incremental e queued_at/started_at conforme AEP-0048; persistir Job.DatabaseID, slug, run_event_id e root_origin_type sem inferência retroativa. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R03.
- **I09.2** — Inserir fato elegível e outbox na mesma transação; não usar cascade de runs como fronteira de replay. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R03.
- **I09.3** — Implementar epochs de política de replay e deadline imutável por ocorrência, preservado quando a retenção mudar. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R03.
- **I09.4** — Implementar consumo com lease, retry, delivered/dead_letter e processamento de cada regra/escopo por CAS de sequência; replay e conflito de fingerprint são distintos. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R03.
- **I09.5** — Implementar lease/heartbeat da claim de job, reconciliação autoritativa e anti-loop com command_chain_history separado, limite versionado 16. Estado documental anterior: aberto. Fechamento/revalidação: R03.
- **I09.6** — Testar queda após commit, entrega duplicada/fora de ordem, count-cap, fonte perdida, raiz externa/unknown e fatos legados ambíguos; atualizar AEPs associados. Estado documental anterior: aberto. Fechamento/revalidação: R03.
- **I10.1** — Implementar política autoritativa de comandos para usuário/agente/job/system, sem tratar allowed_source_types ou confirmação como autorização suficiente. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R05.
- **I10.2** — Completar contexto job_service e grants de delegação exatos da AEP-0101; reconsultar owner/definição/profile/grant no gate final. Estado documental anterior: aberto. Fechamento/revalidação: R05.
- **I10.3** — Implementar ponte tipada para tools/jobs com correlação command_invocation, propagação de sensibilidade e redação; preservar executor comum e commandpolicy para shell. Estado documental anterior: aberto. Fechamento/revalidação: R05.
- **I10.4** — Implementar modo system restrito sem usuário e APIs privilegiadas de manutenção; proibir acesso a bindings e delegação que exige owner. Estado documental anterior: aberto. Fechamento/revalidação: R05.
- **I10.5** — Implementar mapeamento administrativo (issuer, subject), migração/readiness externa e revogação em conjunto com AEP-0052; nunca JIT/último token. Adapters físicos externos continuam proibidos sem a AEP futura do broker. Estado documental anterior: aberto. Fechamento/revalidação: R05.
- **I10.6** — Testar fronteiras de ator, cross-profile, externa/local, origem headless e ausência de interlocutor; atualizar AEP-0063 quando alterar origem/redação. Estado documental anterior: aberto. Fechamento/revalidação: R05.
- **I11.1** — Versionar resources.commandLayers no envelope da AEP-0047 e implementar round-trip de deltas/needs_review e escopo portátil. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R05.
- **I11.2** — Resolver UUIDs, refs builtin/user e mapa de workspaces no destino autenticado; conflito foreign_owner não revela conteúdo. Estado documental anterior: aberto. Fechamento/revalidação: R05.
- **I11.3** — Implementar manter/substituir/cópia com remapeamento transacional; nome conflitante exige escolha explícita. Estado documental anterior: aberto. Fechamento/revalidação: R05.
- **I11.4** — Excluir grants, claims, defaults puros e histórico; regra event-driven importada fica sem concessão e desabilitada. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R05.
- **I11.5** — Validar referências exatas de credenciais por pattern, não IDs locais; separar export sem segredo e export sensível UI-only com criptografia/decisão da AEP-0047. Estado documental anterior: aberto. Fechamento/revalidação: R05.
- **I11.6** — Testar importação repetida, referência ausente, dados antigos com segredo bruto e rollback do lote; atualizar AEP-0047. Estado documental anterior: aberto. Fechamento/revalidação: R05.
- **I12.1** — Definir prova de encerramento de geração e exclusão de execuções antigas antes de recuperar pendências, incluindo reinício e outros usuários/sessões. Estado documental anterior: aberto. Fechamento/revalidação: R04.
- **I12.2** — Reconciliar invocação+ledger atomicamente para outcome_unknown, incluindo system com capability interna; jamais reexecutar efeito. Estado documental anterior: aberto. Fechamento/revalidação: R04.
- **I12.3** — Integrar recuperação de receipts de sessões abandonadas, claims e leases com lotes, cancelamento e critérios de término. Estado documental anterior: aberto. Fechamento/revalidação: R04.
- **I12.4** — Migrar a cadência de retenção para um único InstanceMaintenanceCoordinator; preservar limpezas legadas e compactação, com outbox antes da retenção de jobs. Estado documental anterior: aberto. Fechamento/revalidação: R04.
- **I12.5** — Implementar idade/caps sem remover ledger antes do prazo, nem estado ativo; adicionar as seis settings previstas e UI/i18n correspondente. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R04.
- **I12.6** — Testar múltiplos usuários/system, interrupção entre lotes, retenção alterada, compactação e ausência de dois loops; atualizar AEP-0074-B. Estado documental anterior: aberto. Fechamento/revalidação: R04.
- **I13.1** — Fechar ponte tipada de despacho UI com ack/resultado/cancelamento, sessão e invocation_id; registrar capabilities sem handlers reais migrados. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01/R06/R10.
- **I13.2** — Implementar ownership local/global por geração, ocorrências UUIDv7, repeat/release/blur/reconexão e contrato de sequências Ctrl+N do inventário. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01/R06/R10.
- **I13.3** — Integrar DialogCommandScope ao stack real e reservar invariantes de decisão antes de bindings/ownership, respeitando input/IME e registro global temporário. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01/R06/R10.
- **I13.4** — Implementar ciclo de vida genérico de adapter, callbacks com geração, suspensão por lock/logout e shutdown; nenhum listener chama handler final. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01/R06/R10.
- **I13.5** — Validar biblioteca/licença/build/modelos HID e implementar gerência de dispositivos com exclusividade, reconexão/backoff e estado seguro; renderer com cache/diff e frame completo após reabrir. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01/R06/R10.
- **I13.6** — Validar teclado/foco/janela e ao menos um Stream Deck real; falha de hardware não derruba App. Registrar explicitamente dependência de dispositivo e ambiente. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01/R06/R10.
- **I14.1** — Construir esqueleto de bootstrap serializado e readiness observável após I01, sem expor novas rotas nem cadastrar comandos de produto; este subitem pode começar cedo. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01/R04.
- **I14.2** — Montar catálogo/defaults de contrato, políticas, stores, presenter, providers, dispatcher e adapters com dependências explícitas; sem fallback permissivo. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01/R04.
- **I14.3** — Orquestrar login/unlock/restart: autenticar → recuperar/reconciliar → carregar/projetar → publicar → habilitar entradas, revalidando cada transição. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01/R04.
- **I14.4** — Impedir retomadas concorrentes e publicação de geração antiga; logout/troca de usuário/falha de monitor cancela trabalho e apaga somente estado em memória. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01/R04.
- **I14.5** — Integrar shutdown, drenagem/cancelamento e manutenção sem goroutines órfãs, mutex durante UI/cofre ou cadências duplicadas. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01/R04.
- **I14.6** — Testar instalação nova, upgrade, restart com pendência, falhas em cada etapa e retomada após erro; nunca mascarar indisponibilidade como mapa vazio pronto. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R01/R04.
- **I15.1** — Isolar os testes legados que escrevem configuração pessoal; disponibilizar suíte geral reproduzível em dados temporários. Estado documental anterior: marcado no histórico. Fechamento/revalidação: R06.
- **I15.2** — Executar backend completo, detector de corrida em ambiente com C/CGO e lint compatível v2; verificar frontend/Wails gerado quando as pontes mudarem. Estado documental anterior: aberto. Fechamento/revalidação: R06.
- **I15.3** — Medir p50/p95/p99 no caminho integrado com handlers de teste, incluindo SQLite/gate sob mutações, foco e carga; decidir tratamento de contenção contra a meta experimental p95 < 1 ms. Estado documental anterior: aberto. Fechamento/revalidação: R06.
- **I15.4** — Executar matriz de crash/replay/isolamento/segredos e testes acessíveis da ponte/diálogos; registrar verificações reais de SO, HID e NVDA. Estado documental anterior: aberto. Fechamento/revalidação: R06.
- **I15.5** — Encerrar achados de review local Bugbot antes de push e requisitos de CI/review quando houver PR; não declarar review feita se a ferramenta não estiver disponível. Estado documental anterior: aberto. Fechamento/revalidação: R06.
- **I15.6** — Reconciliar todos os critérios de infraestrutura deste plano com evidência e registrar aceite do marco BASE-PRONTA; os critérios de produto permanecem abertos até P01–P06. Estado documental anterior: aberto. Fechamento/revalidação: R06.

## 7. Reconciliação dos 20 itens de produto

Todos estavam abertos no plano anterior. Correspondência preservada; consultar os estados reconciliados dos R associados na seção 4. As descrições são obrigações completas, não afirmações de que todo seu código ainda falta:

- **P01.1** — Revisar o inventário e cadastrar contratos/handlers reais de workspace, chat, editor, terminal, tasklists, menus e diálogos, incluindo defaults localizados. Gate: R07.
- **P01.2** — Migrar teclado local, sequências e hotkeys de perfis/jobs usando os adapters comuns. Gate: R07.
- **P01.3** — Demonstrar equivalência por surface/foco/input e retirar handlers paralelos somente após a cobertura correspondente. Gate: R07.
- **P02.1** — Busca/aliases, disponibilidade com motivo, atalho efetivo, recentes/favoritos e navegação para configuração. Gate: R08.
- **P02.2** — Formulários de argumentos e execução pelo serviço único. Gate: R08.
- **P02.3** — Combobox/listbox, foco, anúncios, teclado/axe e validação NVDA. Gate: R08.
- **P03.1** — Lista/detalhe de camadas, ativação, bindings, captura de teclas e conflito explicado. Gate: R09.
- **P03.2** — Fluxos de restore/rebase/needs_review, prioridades em divulgação progressiva e ajuda derivada do mapa efetivo. Gate: R09.
- **P03.3** — Ações operáveis por lista/teclado, sem depender de imagem, drag ou cor; pt-BR/en/es, axe e NVDA. Gate: R09.
- **P04.1** — Mapas reais no Stream Deck, imagens/títulos/estados, navegação por camadas e múltiplos dispositivos. Gate: R10.
- **P04.2** — Diagnóstico de disputa/reconexão e estado seguro acessível na UI; validar uso sem software oficial. Gate: R10.
- **P04.3** — Camadas por programa em foco e fixação/estabilização com comandos reais, sem acrescentar controle privilegiado não autorizado. Gate: R10.
- **P05.1** — Expor command_catalog e command_config com ações fechadas, IDs reais e decisões do serviço comum. Gate: R11.
- **P05.2** — CLI list/describe/execute/retry com request ID e indisponibilidade explícita para comandos visuais/interativos. Gate: R11.
- **P05.3** — Integrar fluxos de export/import e relatórios na UI/tools permitidas, sem export sensível pelo chat. Gate: R11.
- **P05.4** — Ligar eventos/jobs e delegações aos comandos reais preservando grants, proveniência e auditoria. Gate: R11 (base de automação em R03).
- **P06.1** — Reexecutar regressões ponta a ponta, desempenho com ações reais e matriz multiusuário/dispositivo. Gate: R12.
- **P06.2** — Concluir docs de usuário, acessibilidade manual e todos os 84 critérios finais com evidências. Gate: R12.
- **P06.3** — Zerar review local, CI/review remota e registrar PRs; merge continua decisão do mantenedor. Gate: R12.
- **P06.4** — Marcar AEP/índice Done somente após todo escopo aceito concluído. Gate: R12.

## 8. Validação manual: só depois de construir o percurso

Preservadas as evidências já recebidas: round-trip físico no Stream Deck em 15/09/2026; captura de foreground/ambiente; relatos de execução de atalhos e Stream Deck, criação/ativação de camada, captura de teclas, bloqueio por modal, uso do picker e responsividade. Não se descartam esses aceites nem se extrapolam para reconexão, lock, múltiplos dispositivos ou NVDA integral dos lotes posteriores.

Antes de solicitar nova rodada, preparar cópia isolada de dados e entregar roteiro com: versão/commit; caminho da tela; combinação exata; comando canônico; passos; resultado esperado; resultado observado; aprovado/falhou. Evitar teste que apenas seta uma variável afirmando a condição a validar.

- NVDA/palette: validar o picker e comandos já implementados; recentes/favoritos e demais lacunas D9 só entram no roteiro após implementação.
- NVDA/configuração: validar CRUD, regras/condições suportadas, captura, restore/rebase e escopos já expostos; não pedir edição de imagens/estados ainda ausente.
- Dispositivo integrado/reconexão/lock: roteiro acumulado de R10.4, incluindo os percursos até128; usar o modelo já informado, sem pedir serial como configuração de uso.
- Aceite de responsividade: preservar o relato positivo do usuário; medição R06.3 permanece pendente após a correção Δ18 (seção130), sem inferir números a partir da impressão de uso.

## 9. Registro de baseline e próxima execução

16/09/2026 — Revisão integral por frentes; corrigida mistura 53 itens I + 28 critérios C. Criados 12 gates e mapeamento integral de 84 I, 20 P e 83 C. Nenhum contrato do AEP foi reduzido; nenhuma implementação corrigida nesta rodada de revisão/planejamento.

Próxima execução vigente: seguir as frentes finitas da seção129 após as entregas130–135, qualificando desempenho integrado, convergência por família/origem e ciclo reativo. A ordem acima de 16/09 é histórica. Não retomar contagem 81/84 nem recriar tela, handlers ou ingressos já implementados para atualizar o placar.

## 10. Execução paralela após a revisão — 16/09/2026

Seis subagentes Luna reutilizados em escopos separados; integração e revisão pelo agente principal. Alterações locais sobre a baseline acima, ainda sem commit/PR. Este registro complementa a revisão histórica, não a reescreve como se os defeitos nunca tivessem existido.

### Correções implementadas e evidências locais

- **F01 / R06:** corrigida a versão das fixtures de migração (comandos começam em v21) e a sequência do teste de registro completo até v28. `internal/database/command_migration_test.go` e `migrator_test.go`; pacote database completo passou. Nenhuma migração de produção foi renumerada.
- **F02 / R05:** credencial ausente, de outro owner ou ambígua mantém o binding desabilitado no plano e na aplicação. Workspace não resolvido recusa importação; export valida cada workspace. Testes de plano, snapshot e apply em `internal/commandportability`.
- **F03 / R03:** consumer distingue erro permanente de transitório, continua o lote e limita tentativas; a oitava tentativa ou lease expirado no limite alcança dead-letter. Regressões em `consumer_retry_test.go` e `attempt_limit_test.go`.
- **R04:** purga bounded de outbox terminal virou contrato obrigatório do coordinator, antes da retenção de jobs; cancelamento entre drain e purge é respeitado. Não equivale à montagem produtiva do coordinator no App.
- **F06 / R08:** textos do picker nos três locales e tradução compatível com o contrato de `t`; não representa execução de comandos pelo picker.
- **F07 / R06:** substituído o falso positivo de TryLock concorrente por prova determinística de lock em testes separados; testes negativos demonstram que o probe detecta lock mantido. Mantidas as asserções de serialização concorrente.
- **H02 / R01:** capability exige origem exata; ingresso público Wails não aceita origem física, e lifecycle público exige a sessão atual. Handoff interno permanece distinto do ingresso público. Testes Go e frontend cobrem recusa de falsificação.
- **H03 / R05:** envelope commandLayers estrito, sem recursos misturados/credenciais/escopo vazio; preflight não redireciona o workspace escolhido. Export amplo ambíguo recusa a operação. Testes completos de `internal/portability` passaram.
- **H04 / R02:** restore encerra claims session/temporary, expira vencidas e restaura persistent idempotentemente, exigindo correspondência de owner/workspace entre claim, regra e camada. Alterações globais e locais usam uma transação.
- **R01.1 parcial (histórico):** perfil vem da aba/workspace canônicos; provider de contexto revalida sessão, substituição da instância e versão de workspace antes/depois da leitura. A proposta de transportar um reader UI síncrono ao backend foi substituída pela divisão de autoridade aprovada em 16/09/2026 (D2/D8); não é mais um requisito implementar essa porta inviável.
- **R02.3 parcial:** applier separa escopo alterado do workspace publicado. Mutações globais, locais e de workspace inativo preservam a união do workspace ativo e revalidam ambas as gerações; teste integrado em `app_command_mutation_test.go`.

### Defeitos encontrados na consolidação

- **Δ01 / R01.4:** reload falho podia continuar o bootstrap com configuração antiga. Pós-auth agora suspende/reset e retira a configuração anterior antes da reconstrução, abortando em erro. `app_command_lifecycle_reload_test.go` cobre cancelamento e falha SQL com runtime padrão.
- **Δ02 / R01.4:** montagem inválida podia instalar dependências parcialmente. Validação/criação precede publicação conjunta de host, bridge e controller. `app_command_lifecycle_mount_atomic_test.go` cobre erro, retry e conflito.
- **Δ03 / R04:** `jobs.Manager.Stop` chamava cancelamento sob mutex. Cancelamento e fechamento ocorrem após liberar o lock; regressão de join/cancelamento passou 20 repetições, sem ampliar timeout.
- **Δ04 / R02.3:** restore agregado consultava regras globais com o owner do workspace, mas o Store resolve regras por escopo exato. A resolução agora usa o escopo de cada claim dentro da mesma transação. Regressão com RulePort/Store real mantém global + workspace e não altera outro workspace; o App também demonstra restauração efetiva da camada global.
- **Δ05 / R06:** `tools/shell` recusava caminho relativo válido quando a raiz existente era canonicalizada e o descendente ainda inexistente preservava alias Windows 8.3. A canonicalização resolve o ancestral existente, conserva o retorno canônico e rejeita symlink dangling/escape. Pacote passou; os dois testes que criam symlinks foram pulados por ausência de privilégio Windows, portanto essa parte exige outro ambiente/CI.

### Qualificação e limites

Frontend completo: **2.966 testes passaram**, sem falhas; TypeScript, ESLint e Stylelint passaram (Stylelint ainda emite warnings existentes). Bindings Wails regenerados, não editados manualmente.

Backend consolidado: `go test -work -json -mod=readonly ./... -count=1 -timeout=180s` terminou com **112 pacotes aprovados e 1 reprovado: `internal/acpregistry`**. App, todos os pacotes command, jobs, database, portability e `internal/acp` passaram. `acpregistry` encerra com `exit status 0xffffffff` antes de iniciar testes, reproduzido inclusive com filtro sem testes; causa ainda não certificada. Não foi classificado como falha dos testes ACP nem como defeito de ambiente comprovado.

Diagnóstico complementar: `go test -c -o <arquivo temporário> ./internal/acpregistry` seguido da execução com `'-test.v' '-test.timeout=120s'` passou **todos os testes do pacote**, exit 0. O agente reproduziu a falha no binário com stripping `-s -w`, também emitido pelo runner normal; isso localiza a divergência no artefato/execução Windows, sem provar se a origem é linker ou proteção do host. Nenhuma configuração de segurança ou do Go foi alterada. O comando geral continua registrado como FAIL, não convertido em PASS pela alternativa. Após as últimas regressões adicionadas, `commandconfig` e `commandactivation` completos passaram novamente.

`go build ./...` e `go vet ./...` passaram na consolidação. `golangci-lint run --timeout=3m`: **0 issues**. `git diff --check` passou excluindo o arquivo gerado `frontend/wailsjs/go/models.ts`; esse arquivo mantém whitespace emitido pelo gerador Wails, sem correção manual. Detector de corrida não executado por ausência de compilador C/CGO; Bugbot indisponível nesta sessão, sem push/PR/merge ou alegação de revisão equivalente. Evidência local não substitui esses gates.

**Nenhum gate R integralmente aceito neste lote.** Não há aceite de BASE-PRONTA: ainda faltam providers/transporte reais, trigger/suppress, recuperação com prova de encerramento de geração, consumo produtivo de jobs/outbox, cadência única montada no App e qualificação integrada. R01–R06 permanecem abertos; não houve migração sistemática de comandos ou construção da tela de configuração.

## 11. Rodada real de produto — 16/09/2026

### Baseline R01

O bootstrap produtivo deixou de usar o sentinela `lifecycle.ready` como base de
evidência. O App monta o executor desktop por
`newCommandDesktopExecutor` e materializa a configuração por
`commandconfig.ProjectComplete`; o catálogo inicial inclui `workspace.list`.
O handler gera um payload de lista internamente, mas a API/UI atualmente entrega
somente resumo/status terminal; o payload ainda não é entregue à UI. Portanto,
isso não é uma feature pronta de listar workspaces.
Autenticação usa a sessão local, sem JWT. A `commandbridge.Bridge` mantém
handoff assíncrono e cancelamento, e o shutdown aguarda o join do lifecycle,
executor e bridge antes de liberar dependências.

O `Recover` do lifecycle agora é um preflight somente leitura e fail-closed:
sem prova de encerramento interprocesso, consulta pendências recuperáveis de
invocations, ledger e decisions em todos os owners, incluindo `system`. Não
reconcilia, não altera o banco e bloqueia o boot com `ErrNotReady` quando há
pendência ou erro de schema/SQL. Esse safety interlock não fecha a recuperação
R04; a integração efetiva continua sendo responsabilidade da montagem central
do App.

### Evidência de testes e limites

Rodada real anterior registrada: App completo PASS em **50.278s**; factory em
**14.469s**; catálogo em **13.274s**; autenticação em **6.714s**; configuração
em **42.481s**. Antes das últimas correções, uma repetição do App passou em
**101.974s**. Esses tempos são evidência histórica da rodada, não fechamento de
gate.

Qualificação corrente: frontend em **315 arquivos / 2.966 testes PASS**;
TypeScript, ESLint e tokens passaram; Stylelint teve **2.160 warnings e 0
errors**. Backend: **112 pacotes PASS, 8 sem testes e 1 FAIL**; `internal/acpregistry`
terminou em **8.747s** com `exit ffffffff`. A repetição complementar atual,
compilou com `go test -c .../acpregistry.test.exe` (exit 0), mas a execução direta
do binário também terminou com `exit ffffffff`, sem output; a causa não está certificada.
Não há alegação alternativa de PASS para `acpregistry` nesta rodada.

Consolidação final, após todas as correções: `go test -mod=readonly ./internal/app -count=1 -timeout=180s` passou em **41.803s**; os cenários `TestCommandProduct|TestMountCommandProduct` passaram em **18.645s**. `internal/wailsapi`, `internal/auth` e `internal/commandconfig` completos passaram em **4.801s**, **3.795s** e **9.907s**, respectivamente. As duas falhas intermediárias do App eram setups de teste: estado de SO conhecido na fixture que exigia desconhecido e origem `ui` em vez do identificador canônico `ui.action`; corrigidos os setups, preservadas as asserções.

`go build ./...`, `go vet ./...` e `golangci-lint run --timeout=3m` passaram; lint com **0 issues**. Bindings Wails regenerados automaticamente. `git diff --check` passou exceto pelo whitespace já emitido no `models.ts` gerado. A suíte geral permanece FAIL por `acpregistry`; detector de corrida e Bugbot continuam sem execução, pelos limites de ambiente/ferramenta já registrados. Nenhum commit, push, PR ou merge foi feito.

Esta rodada não fecha o gate R01. R01.2 permanece parcial até a ligação completa
de providers e trigger/suppress, além do transporte de resultado para a UI; o
picker pode consultar metadata do catálogo, mas ainda não executa comandos nem
recebe o payload de lista. Os atalhos legados permanecem inalterados.

### Correções da consolidação

- **Δ06 / R01:** montagem não infere desbloqueio da existência de credenciais. Preserva o host bloqueado; no primeiro host, lê o cofre fora do gate e condiciona a publicação à mesma geração autenticada. Remontagem cancelada ou posterior ao shutdown recusa a operação.
- **Δ07 / R01:** substituição de sessão, serviços ou workspace invalida a composição antiga. Snapshot, autorização e despacho revalidam as dependências. Testes demonstram recusa do runtime anterior e execução pela nova composição. A ligação automática da troca de workspace ao novo lifecycle ainda faz parte do trabalho restante; não se introduziu remontagem com I/O a cada atalho.
- **Δ08 / R01:** catálogo Wails exige uma porta explícita de prontidão operacional, além do contrato de origens permitidas. Cofre bloqueado, sessão revogada, produto encerrado, workspace trocado ou origem ainda desconectada não são anunciados como disponíveis. Não há assinatura variádica de compatibilidade para uma API ainda nova.
- **R01.4:** teste de shutdown segura o worker na persistência real, confirma timeout sem join prematuro, libera a barreira e comprova encerramento e recusa posterior. As barreiras ficam somente no teste.

Nenhum checkbox ou gate integral é certificado por essas correções isoladas.

## 12. Divisão de autoridade contextual — 16/09/2026

Decisão D2/D8 aprovada pelo usuário: o backend não consulta um reader de DOM
via Wails sob o DispatchGate. A UI revalida foco, modal, composição e surface
sincronamente antes do efeito visual; sessão, autorização, ledger, alvos
persistidos e versões autoritativas permanecem no backend.

O FactBus produtivo passa a expor workspace/aba, perfil derivado do
WorkspaceManager e foreground nativo. A porta síncrona fictícia de UI é retirada;
políticas que tentam consultar fatos visuais nesse bus falham fechado. Os testes
da porta substituída são reformulados para provar a nova fronteira, preservando
os cenários de owner, logout, sessão substituída e workspace divergente.

Na UI, o guard de efeito usa preparações opacas de uso único, releitura das fontes
e invalidação de contexto. O acompanhamento de composição tem lease explícita e
estados active/inactive/unknown; ausência de prova não significa composição
encerrada. O primeiro consumidor produtivo é a apresentação assíncrona do
catálogo no picker, que deve descartar respostas antigas sem reabrir o menu.

Isso não autoriza execução no backend. O handoff correlacionado de comandos
registrados com efeito UI, seu resultado terminal e o transporte do payload
continuam pendentes. Não foi criada uma execução paralela de comandos, nem
migrados atalhos. R01 continua parcial; nenhum gate integral é certificado
somente pela mudança de contrato ou pela proteção de apresentação do catálogo.

### Qualificação desta rodada

- `go test -mod=readonly ./... -count=1 -timeout=180s`: exit 0 em
  108,165s; 111 pacotes PASS, 8 sem testes, nenhuma falha. `internal/skills`
  passou em 3,384s e `internal/acpregistry` em 6,871s. O erro histórico
  `ffffffff` não ocorreu nesta execução; sua causa não foi demonstrada nem
  se declara correção definitiva por esse PASS.
- `go build ./...`, `go vet ./...` e `golangci-lint run`: PASS; lint com
  zero issues.
- Execução intermediária concorrente encontrou `AccessDenied` no fixture
  relativo `.assistente/skills`; a repetição isolada passou em 1,318s e a
  suíte completa acima passou. Não houve alteração de permissões do home,
  banco pessoal ou configuração de segurança para contornar a falha.
- Testes finais de providers, sessão e guard local: 3 arquivos / 30 testes
  PASS, incluindo token estrangeiro, uso único, ABA de login/foco, troca de
  aba sem perfil e mutação reentrante durante commit.
- Stylelint: 0 errors e 2.160 warnings existentes; tokens CSS: PASS.
- Integração focada Topbar/MenuButton: 38 testes PASS. Inclui respostas
  atrasadas após logout, workspace, aba, foco ABA, modal, Escape e unmount;
  somente a consulta mais recente publica; retry e Ctrl+K têm caminho positivo.
- TypeScript e ESLint completos no estado consolidado: PASS.
- Frontend completo final: `npm test -- --reporter=dot`, exit 0;
  **316 arquivos / 3.000 testes PASS** em 134,00s.
- `git diff --check` passou excluindo somente o whitespace histórico do
  arquivo gerado `frontend/wailsjs/go/models.ts`. Esta rodada não alterou
  assinaturas públicas Wails nem editou bindings manualmente.

Race/CGO e Bugbot seguem sem execução nesta sessão; não houve commit,
push, PR ou merge. A validação local não substitui esses gates.

## 13. Handoff produtivo UI/backend — 16/09/2026

Avanço em **R01.1/R01.3**, sem fechamento integral de R01: o percurso de
execução UI é montado no App e consumido pelo picker para a operação real
`help.shortcuts.show`. Reutiliza o painel existente; não migra os atalhos
legados e não converte `workspace.list` em uma feature pronta de listagem.

### Contrato e implementação

- `BeginUICommand` recebe apenas o ID do comando, fixa a origem palette e
  cria IDs no backend. A reserva transitória não equivale a sucesso nem a
  autorização: o worker passa pelo `CommandExecutionService` completo,
  com sessão local, política, ledger, CAS e admissão sob o gate existente.
- O handler `ui/help/shortcuts/show` retorna um `ExecutionHandle` sem
  aguardar UI. `TakeUICommand` espera fora do gate e entrega um handoff de
  uso único, correlacionado à reserva/invocação/comando e ao owner capturado
  pelo backend. Identidade/epochs não são recebidos no payload da UI.
- O consumidor captura o guard antes de Begin, valida a correlação da
  resposta, relê o contexto e executa o efeito síncrono antes de confirmar.
  Não há retry automático de Begin, Take ou do efeito. A entrada antiga
  `ExecutePaletteCommand` recusa HandlerUI, evitando um ingresso sem essa
  preparação visual.
- `CompleteUICommand` aceita somente succeeded/failed/cancelled após Take
  e com o handoff correto. Recusa antes do efeito pode confirmar cancelled;
  cancelamento/expiração após entrega, efeito que lança ou confirmação
  perdida não provam ausência de efeito e ficam sujeitos a outcome_unknown.
  O resultado final é o persistido pelo executor, não o ack de Begin.
- O broker tem até 64 reservas e expiração de 45s. Execuções têm prazo de
  30s; resultados transitórios ficam até 1min em mapa de no máximo 64 itens,
  com descarte preguiçoso na consulta/admissão. Não há timer por resultado.
  Shutdown cancela workers, encerra o broker e aguarda seu expirer.
- Os bindings Wails foram regenerados a partir dos DTOs públicos. A API não
  publica tokens de autenticação, fingerprints ou dados brutos do ledger.

### Limites de aceite

Esta entrega valida o transporte local para uma leitura visual real, não
garante atomicidade distribuída entre DOM e banco e não substitui recovery
interprocesso R04. Reinício/perda de confirmação não autoriza repetir um
efeito. Generalização para argumentos/alvos/surfaces específicos, demais
comandos, origens físicas e migração sistemática continua nos gates já
existentes. A contagem histórica dos 84 itens não é incrementada por pacote
ou por número de testes. O AEP permanece In Progress.

### Qualificação automatizada desta entrega

- Integração real do App: correlação, entrega única, rejeição de replay e de
  sessão diferente, resultados persistidos, expiração e cancelamento no shutdown.
- Frontend completo: **318 arquivos / 3.010 testes PASS** (103,44s).
- Após acrescentar três cenários negativos, execução focal do consumidor e
  transporte: **2 arquivos / 11 testes PASS**. Build TypeScript/Vite: PASS.
- Reexecução completa final, incluindo os três cenários adicionais:
  **318 arquivos / 3.013 testes PASS** (92,09s). TypeScript e ESLint finais: PASS.
- ESLint completo e validação de tokens CSS: PASS. Stylelint: zero erros,
  2.160 avisos preexistentes; não foram feitas alterações CSS nesta entrega.
- Bindings Wails regenerados; nenhuma edição manual de modelos gerados.
- A primeira rodada Go detectou uma expectativa antiga de um comando
  publicado em `TestAppCommandLifecycleRebuildsEmptyProductThenBootstrapsReady`.
  Corrigida para dois comandos, preservando as verificações ready/published.
  O log SQL de tabela ausente não foi a asserção responsável pela falha.
- O teste de shutdown percorre a bridge real até `p.Shutdown`, verifica o
  join dos workers, `p.done` fechado, resultados transitórios descartados e
  recusa de novo handoff. Não foi necessário um cleanup alternativo de teste.
- Suíte Go completa final: `go test -mod=readonly ./... -count=1
  -timeout=180s`, **PASS (exit 0)**; App 60,591s, ACP registry 3,513s,
  broker UI 0,679s. A fixture isolada mantém um comando; o catálogo do
  produto publica dois. Build Go, vet e golangci-lint passaram nesta rodada
  de implementação; as correções posteriores foram somente em testes.
- Não houve uso do banco pessoal, alteração de segredos, commit, push ou merge.
  Race/CGO e Bugbot permanecem gates não executados.

## 14. Resultado efêmero de backend e política de validação — 16/09/2026

Estado: **percurso de resultado efêmero consolidado e validado automaticamente**.
Avanço em R01.3 e R08.2, sem aceite integral de nenhum gate.

- `ExecuteEnvelopeWithResult` usa o mesmo pipeline de autenticação, política,
  ledger e CAS; `ExecuteEnvelope` delega a ele descartando o payload. Não há
  segundo executor, cache global, timer ou nova trilha de autorização.
- O resultado validado existe apenas na chamada que executou o handler e
  confirmou o terminal succeeded. Replay/lookup, falha, cancelamento, schema
  inválido ou falha de finalização não recuperam esse payload.
- `ExecutePaletteCommand` reautentica a composição após a espera e expõe
  somente um DTO fechado de `workspace.list`: id, nome, perfil, número de
  abas e ativo. Paths, tokens e fingerprints não fazem parte da resposta;
  conteúdo da lista não é persistido no ledger/auditoria.
- A apresentação captura o contexto antes da consulta e o relê antes de
  abrir o picker existente. Escape cancela a apresentação, não desfaz uma
  leitura backend admitida. Não há repetição automática. A escolha posterior
  de um workspace continua na operação existente, sem declarar migração R07.
- Foram escritos testes do pipeline, da fachada e do consumidor para replay,
  redação, schemas fechados, entrega após revogação/lock/troca de workspace,
  alteração de foco, duplicação, descarte e perda de retorno.

### Restrição operacional de segurança

O usuário identificou os alertas de infosec nos executáveis diagnósticos
`assistente-acp-stripped-compare.test.exe`,
`assistente-acpregistry-preserved-copy.test.exe` e
`assistente-acpregistry-s.test.exe`. Depois esclareceu que autoriza testes
normais via `go test` e `npm test`. Não usar `go test -c`, variantes stripped,
cópia ou execução manual de binários de diagnóstico. Não contornar a proteção,
criar exclusões de antivírus nem apagar possíveis evidências. Esta rodada não
inicia Wails nem faz nova geração dos bindings.

As sessões de Go/Wails iniciadas antes da restrição deixaram de estar
disponíveis ao agente; seus resultados não foram confirmados. Não havia
processos Go/Wails/testes Go na consulta de processos disponível depois do
aviso. Bindings novos presentes no disco foram produzidos antes da restrição;
não houve edição manual ou nova geração depois dela.

Esta rodada não herda o PASS da seção 13. Os testes normais foram retomados
após o esclarecimento acima; resultados desta consolidação são registrados
separadamente abaixo. Inspeção de código e testes escritos não equivalem a
testes executados. AEP permanece In Progress.

### Evidência de consolidação — 16/09/2026

- `workspace.list` apresenta seu DTO real no picker compartilhado. A seleção
  consome a intenção antes do efeito; callback repetido ou de lista anterior
  não inicia nova troca. Fechar o picker devolve foco ao gatilho correto,
  inclusive fora da rota workspace. Logout antes da seleção da paleta não
  força foco no gatilho da sessão anterior.
- Testes cobrem resultado após logout, sessão ABA, workspace ABA e dispose,
  preservando o status real do backend sem apresentar resultado obsoleto.
- Suíte Go normal completa: `go test -mod=readonly ./... -count=1
  -timeout=180s`, PASS. App 60,749s, ACP 28,485s, ACP registry 2,899s,
  executor 15,532s. `go vet -mod=readonly ./...`: PASS.
- A primeira execução revelou expectativa incorreta no teste novo: a fixture
  tinha duas abas. A asserção agora compara ID, nome e contagem com o workspace
  real da fixture, além de exigir perfil e estado ativo; não foi relaxada para
  aceitar qualquer contagem.
- Frontend completo: **320 arquivos / 3.036 testes PASS**, 66,51s.
  TypeScript sem emissão e ESLint completo: PASS. Após o refinamento final
  de consumo único da seleção, **3 arquivos / 60 testes focais PASS**.
- `git diff --check` global acusa espaços finais no arquivo gerado Wails
  `frontend/wailsjs/go/models.ts`; não houve edição manual desse arquivo.
- Sem novo executável diagnóstico personalizado, execução Wails, banco pessoal,
  commit, push ou merge. Validação manual deste percurso não foi realizada.

**Gate permanece aberto:** esta evidência fecha a entrega de resultado ao vivo,
não R01 inteiro. Falta conectar resolução de triggers/configuração persistida,
supressão e invalidação na composição real, além das provas de lifecycle e
recuperação indicadas em R01.1–R01.4. Nenhuma contagem dos 84 critérios foi
incrementada por número de testes ou por esta entrega parcial.

## 15. Resolução persistida na paleta e consumo de supressões — 16/09/2026

Estado: **percurso produtivo da paleta consolidado; R01 ainda parcial**.
Trabalho realizado no worktree existente, com duas frentes Luna e revisão/integração
principal. Sem commit, push, merge, migração de banco ou edição de bindings Wails.

### Implementação e evidências

- `HostState.ResolutionSnapshot` captura configuração imutável, camadas e versões
  sob um único RLock e exige a sessão publicada exata, cofre/SO desbloqueados.
  Não adquire o DispatchGate, não consulta disco e não faz roundtrip de UI.
- `commandProductProjection` publica o default real `builtin.palette.workspace.list`
  na camada `application.palette`. O fingerprint usa JSON canônico e toda a
  semântica executável, excluindo apresentação visual. Mudanças de risco,
  decisão, capacidade, origem, condição, escopo, prioridade, trigger e argumentos
  são cobertas por testes de invalidação do fingerprint.
- `ExecutePaletteCommand` e o worker da bridge constroem a mesma intenção de
  seleção. `resolvePersistedTrigger` substitui o callback de recusa fixa: resolve
  o snapshot carregado pelo lifecycle, sem SQL no resolvedor. Autorização, CAS,
  autenticação e entrega efêmera continuam no mesmo executor.
- `Candidate/Delta.LayerRef` e `Result.LayerRefs` preservam somente as camadas
  contribuintes dos bindings efetivos, com deduplicação/ordenação. Resolução e
  coleta de proveniência ficam no bucket do acionador, sem varrer o catálogo.
- Condições dependentes de autoridade ainda não ligada recusam o bucket sem
  fallback. Bindings/deltas desabilitados ou inativos não exigem fatos que não
  podem afetar a decisão; pendências de revisão conservam sua recusa.
- Integração com DB isolado: default executa a operação real; delta `suppress`
  persistido gera somente um marker do ledger, sem `command_invocations` e sem
  payload. A bridge não contorna esse delta. Remover o delta e reconstruir permite
  uma nova invocação, mas o replay do ID antigo continua suprimido.
- A rodada encontrou e corrigiu lookup de markers: sem auditoria, o envelope
  do marker é vazio por contrato. `FullRecord.SourceType` agora expõe internamente
  a coluna já persistida no ledger, sem nova coluna ou envelope inventado. Lookup
  exige ownership autenticado e origem correta; teste negativo cobre outra origem.
- Testes cobrem sessão revogada, cofre fechado, publicação obsoleta, origem física
  forjada, argumentos extras, `needs_review`, schema fechado e provider ausente.
- A UI anuncia supressão/indisponibilidade e não abre o picker de resultado;
  mensagens de execução são genéricas ao comando, nos três idiomas.

### Qualificação

- Suíte Go completa normal: `go test -mod=readonly ./... -count=1 -timeout=180s`,
  PASS. App 52,839s, commandconfig 14,854s, executor 21,701s, ledger 6,543s,
  ACP 34,391s e ACP registry 1,762s. `go vet -mod=readonly ./...`: PASS.
- Frontend completo: **320 arquivos / 3.039 testes PASS**, 65,20s.
  TypeScript sem emissão e ESLint completo: PASS.
- A falha inicialmente relatada pelo subagente em commandconfig não foi
  certificada como preexistente: a expectativa nova de ordenação foi corrigida
  para ordem lexicográfica real; a suíte do pacote e a suíte completa passaram.
- `git diff --check` dos arquivos não gerados: PASS. Espaços finais preexistentes
  no modelo gerado Wails continuam sem edição manual.
- Somente testes Go/npm normais; nenhum binário diagnóstico personalizado,
  Wails, acesso ao banco pessoal ou novo segredo. Testes físicos/manuais não
  foram reexecutados nesta rodada.

### Limite e próximo fechamento

Esta entrega cobre resolução, supressão e replay **na origem palette**. O ingresso
de `help.shortcuts.show` continua no handoff UI próprio. Origens físicas seguem
indisponíveis na composição do produto: publicar uma gramática de teclado no
projetor não é prova de listener, ownership ou revalidação local do DOM.

R01 continua exigindo ligação dos providers/contextos necessários aos demais
acionadores, identidade das ocorrências físicas e prova integrada de invalidação
entre resolução e consumo, além do lifecycle completo. A prova local do executor
para `rejected_stale` continua válida, mas não substitui a prova do adapter real.
R04 continua responsável pela recuperação interprocesso. Nenhum gate integral
ou percentual dos 84 itens foi incrementado por este percurso parcial.

## 16. Isolamento de sequências e callbacks — 16/09/2026

- [x] Prefixo capturado em uma fonte não é completado nem consumido por outra.
- [x] Sequência expira também no instante exato do deadline.
- [x] Release/repeat da última tecla usa a identidade composta registrada no
  down; teste com a bridge real prova duas execuções consecutivas após release.
- [x] Resolução em andamento é descartada após blur, lock, shutdown ou troca
  de geração; transições em andamento impedem novos handoffs. A entrega curta
  à bridge é serializada com a invalidação; o resolvedor fica fora do mutex.
- [x] Contexto cancelado não resolve, despacha ou consome prefixo pendente.
- [x] SourceEventID é emitido pelo controller para o down não repetido;
  identidade fornecida pelo resolvedor não é reaproveitada. Repeat/release
  não carregam uma identidade injetada pelo resolvedor.
- [x] Testes de ResolutionSnapshot partem de host efetivamente pronto antes
  de bloquear cofre/SO; verificam owner/sessão, camadas detached e publicação
  coerente de configuração/camadas/versões.

Dois subagentes Luna implementaram testes em arquivos separados; integração,
correção do controller e teste com a bridge real foram feitos no agente principal.
Suíte Go completa normal: PASS (App 50,159s, ACP 20,807s, acpregistry 2,929s).
Após as adições finais, testes de commandadapter/commandexecution e vet dos
pacotes envolvidos passaram. Nenhuma alteração de frontend nesta rodada;
nenhum Wails, binário diagnóstico personalizado, banco pessoal ou hardware.

**Limite:** esta rodada corrige o controller compartilhado, não liga os
acionadores físicos ao executor do App. Ainda faltam essa montagem, identidade
de instância por abertura/reconexão e contexto autoritativo completo. R01 e
BASE-PRONTA permanecem abertos; os 53/84 históricos não foram incrementados.

## 17. Ciclo de vida do Stream Deck — 16/09/2026

A inspeção anterior à montagem no App encontrou defeitos no percurso físico
existente. Esta rodada corrige as fronteiras reais de runtime/driver/manager,
sem habilitar listeners no App ou usar hardware durante os testes.

- [x] Runtime sincroniza descoberta e acesso a handles sem manter mutex global
  durante leitura HID; leitura de um dispositivo não bloqueia render de outro.
- [x] Abertura que falha ao escrever o frame seguro fecha o handle, desfaz o
  estado publicado e permite reconexão após backoff, com frame completo.
- [x] Falha de escrita de frame ativo desconecta o dispositivo: um diff calculado
  mas não entregue nunca é tratado como imagem física confirmada.
- [x] Cancelar uma chamada de leitura não desconecta um handle saudável.
- [x] Shutdown fecha admissão, cancela operações, aguarda as operações admitidas
  e libera os handles; chamadas repetidas preservam os erros de cleanup.
  Cancelar a espera do chamador não abandona o cleanup em andamento.
- [x] A geração usada para ativar é capturada antes da escrita inicial; lock
  durante o I/O impede ativação. Callback de leitura captura geração antes de
  aguardar e não é redirecionado para geração posterior.
- [x] Callback de handle aposentado não entrega eventos nem desconecta uma
  nova abertura do mesmo serial.
- [x] Manager protege estado/mapas com mutex; desconexão é idempotente,
  reativação de desconectado é recusada e backoff satura sem overflow.
- [x] Driver separa terminal de eventos e liberação física: falha não faz
  Close pular a liberação. Overflow de eventos falha fechado, em vez de perder
  silenciosamente um release. Read prioriza cancelamento/falha sobre o buffer.

### Evidência e limites

Dois subagentes Luna atuaram em manager/driver; runtime, testes de integração e
revisão ficaram no agente principal. Testes com fakes de hardware cobrem rollback,
reconexão, múltiplos dispositivos e transições concorrentes. `go test` completo
PASS (App 55,529s, ACP 22,999s, acpregistry 2,543s); `go vet ./...` PASS.
O pacote commanddeck passou também em dez repetições. CGO está desabilitado no
ambiente: não há alegação de validação com detector de corrida. Não foi executado
Wails, binário diagnóstico customizado ou teste físico; frontend não mudou.

**Bloqueio identificado na dependência:** na versão fixada
`rafaelmartins.com/p/streamdeck@v0.0.0-20260905040856-709e442a380b`,
`Key.WaitForRelease` aguarda um canal privado sem cancelamento; `Device.Close`
não libera explicitamente os waiters de teclas pressionadas. Portanto fechar
com uma tecla ainda pressionada pode conservar uma goroutine de espera. A
correção local de canal/handle não certifica shutdown integral da biblioteca.
É necessário tratar esse contrato na dependência (cancelamento/release em
Close e erro de leitura), com teste reproduzível, antes do aceite físico final.

R01/R10 e BASE-PRONTA continuam abertos. Também faltam identidade canônica de
instância por abertura/reconexão, montagem autenticada dos listeners no App e
contextos necessários. As evidências acima não aumentam os 53/84 históricos.

## 18. Resolução e revalidação do handoff UI — 16/09/2026

`help.shortcuts.show` agora usa o mesmo resolvedor persistido que a seleção
backend. Seu default participa da projeção completa; Begin não envia mais um
CommandID direto que contornava deltas. A reserva autenticada permanece
obrigatória para alcançar o handler UI; um delta não pode transformar essa
reserva em execução backend. O ingresso ExecutePaletteCommand continua
recusando comandos UI, mesmo que exista uma supressão configurada.

Testes na composição real do App cobrem default Begin/Take/Complete/result,
supressão sem handoff/auditoria, replay após remoção do delta, needs_review,
redirecionamento e recusa do ingresso incorreto. Defaults são encontrados por
ID nos testes, não pela posição na lista.

O handler captura as gerações realmente admitidas pelo executor. Após esperar
Start, Take revalida sessão/composição/configuração antes de consumir o handoff.
Rebuild, alteração de camadas e lock/unlock invalidam a tentativa anterior.
O broker valida fora de seu mutex, antes de marcar entrega; a UI continua
responsável por validar seus fatos locais sincronamente antes do efeito.
Quando a geração revoga um contexto já running, o executor mantém o resultado
conservador outcome_unknown: a recusa de Take não é confirmação de sucesso.

Frontend preserva suppressed/denied/rejected_stale sem executar efeito. Falha
de Take cancela antes de consultar o resultado, evitando aguardar o timeout
de um handoff perdido; succeeded sem handoff confirmado não é aceito como
sucesso local. Dois subagentes Luna produziram testes/backend e frontend;
a revisão corrigiu testes no ingresso incorreto e a ordem cancelar/consultar.

**R01 permanece aberto.** Faltam providers generalizados com autoridade e
invalidação demonstradas, montagem autenticada dos acionadores reais e prova
integral de lifecycle/recovery (a recuperação depende da prova interprocesso
de R04). Esta rodada fecha o bypass de resolução UI, não esses contratos.
Nenhum item I/R/C integral foi marcado apenas por estas provas parciais.

Validação final: `go test -mod=readonly ./... -timeout=240s` PASS, incluindo
App, ACP e acpregistry; `go vet ./...` PASS. Frontend: 320 arquivos e 3.045
testes PASS; `tsc --noEmit` e ESLint dos dois arquivos alterados PASS.
O teste inicial esperava cancelled após revogação de geração; a inspeção de
awaitEnvelopeOutcomeWithResult confirmou o contrato conservador já existente
(contexto running revogado → outcome_unknown), preservado pela asserção final.
Diff check PASS excluindo o arquivo gerado models.ts com whitespace preexistente.
Sem Wails, executável diagnóstico personalizado, banco pessoal ou hardware.

## 19. Providers reais e aceite de composição — 16/09/2026

**R01.2 aceito na composição produtiva disponível**, sem declarar R01 inteiro
concluído. Referência de código: HEAD da baseline mais alterações locais deste
worktree (ainda sem commit isolado; não houve push). O aceite não depende da
existência de entradas no MountSpec: `readyCommandProduct` monta
`mountCommandProduct` → `newCommandDesktopExecutor` → `NewComplete`, e o
rebuild usa `commandconfig.ProjectComplete` sobre configuração persistida.

Provas operacionais: `TestCommandProductExecutesRealWorkspaceOperationAndPersistsExactlyOnce`,
`TestCommandProductResolutionPersistsSuppressAndKeepsDefaultReplay`,
`TestCommandProductCatalogReflectsOperationalReadiness` e `TestUIResolution*`.
São os handlers de produto `workspace.list` e `help.shortcuts.show`; não um
handler sentinela. `TestCommandMountIdentifiesMissingDependency` complementa
essas provas verificando o diagnóstico individual de host, bridge, providers,
adapter, registry, store, política, envelope, handlers, versões/prazos,
presenter e storage, sem publicar uma composição parcial.

O ingresso operacional continua sendo palette; KeyboardLocalTriggerPort é
normalização de configuração, **não listener de teclado montado**. As origens
não conectadas continuam indisponíveis. Providers completos pertencem a R01.1,
fronteiras de execução a R01.3 e lifecycle/recovery a R01.4. Nenhuma dessas
obrigações foi removida ou considerada cumprida por este aceite.

Contagem vigente: **1/48 saídas R aceitas; 0/12 gates R; 0/83 critérios C**.
Os **53/84 itens históricos** permanecem sem incremento nesta rodada.

Além do aceite de montagem, esta rodada corrige autoridade contextual:

- O WorkspaceManager passa a distinguir fingerprint semântico de época de
  mutação. Trocar aba/perfil/workspace e retornar ao estado anterior invalida
  provas antigas mesmo sem leitura/notificação intermediária. O reader não
  incrementa versões; no-op e apresentação conservam a semântica.
  Retirar a última aba e recriar o mesmo ID também invalida; época saturada
  recusa snapshot, sem reutilizar versão. Entradas de mapas/listas e saídas
  Active/Switch são desanexadas. Checks frequentes do App usam ActiveID sem
  copiar estado; SetActiveTab/SetProfile não fazem hash de todas as abas.
- Os testes `TestCommandFactBusReal*` exercitam o Manager e o bus reais:
  mudanças sem Notify, coalescência, ABA de aba/perfil, owner divergente e
  provider ausente. O contrato correto é provider workspace/fato active_tab;
  não foi inventado um provider-alias para sustentar os testes.
- A sessão de contexto visual aposenta registrations ao observar troca de
  usuário/sessão/workspace, inclusive A→B→A; getters reentrantes não podem
  devolver superfície de uma lease aposentada. Novas capturas relêem os
  stores; dispose remove as assinaturas. Retornar ao owner exige registrar
  novamente a superfície, não ressuscita referências antigas.
- Evidência de IME está vinculada ao documento/elemento focalizado. Troca
  silenciosa de elemento devolve unknown; eventos de composição atrasados de
  outro campo não provam inactive. Teste do guard demonstra recusa do efeito.

Essas provas não fecham R01.1: ainda falta generalizar o registro das surfaces
reais e a composição de condições/contextos nos ingressos previstos. Também
não demonstram o ciclo físico de teclado/Stream Deck nem recovery interprocesso.

Validação após a integração final: `go test -mod=readonly ./... -timeout=240s`
PASS (App 58,298s; workspace 1,283s; pacotes não alterados puderam usar cache),
`go vet ./...` PASS; frontend completo **320 arquivos / 3.051 testes PASS**,
`tsc --noEmit` e ESLint dos arquivos frontend alterados PASS. ActiveID tem teste
de zero alocações; isso não é medição de latência end-to-end. Sem detector de
corrida (CGO desabilitado), Wails, hardware ou executável diagnóstico customizado.
Revisão principal pediu correções aos subagentes antes do aceite: invalidação
em estado sem aba, saturação sem reutilizar versão, retirada de hashing global
dos mutadores simples, cópias sem aliases e IME vinculado ao elemento atual.

## 20. Superfície produtiva da barra de comandos — 16/09/2026

R01.1 continua aberto. A Topbar registra sua própria superfície `toolbar`,
com leitura síncrona da presença/identidade do elemento DOM montado. Não
deduz uma superfície de editor/chat a partir da aba ativa. A consulta do
catálogo, o handoff UI e a apresentação do resultado backend compartilham
essa sessão e exigem o registro da superfície antes de iniciar a operação.

O lifecycle da sessão acompanha usuário/sessão, workspace e identidade da
navegação (pathname, query, hash e chave do router). A troca desmonta guards
e executores antes de registrar o novo contexto. Uma resposta antiga não
abre picker, executa ajuda nem anuncia resultado na nova navegação. Dispose
dos executores não destrói uma sessão emprestada; a Topbar é sua proprietária.

Cada registro de superfície tem identidade opaca própria, separada de ID e
snapshotVersion. Trocar ou remover/recriar o registro com payload idêntico
invalida preparações anteriores. O guard compara a identidade da lease e os
fatos atuais antes do efeito; getters reentrantes que substituem seu próprio
registro não produzem uma captura utilizável. Não há conteúdo de DOM nessa
identidade nem permissão de backend conferida por ela.

Isso não registra automaticamente as superfícies de editor, terminal,
tasklist ou chat, nem habilita acionadores físicos ou condições visuais no
backend. Não há transporte de DOM por Wails sob DispatchGate. Contagens
permanecem **1/48 saídas R, 0/12 gates R, 0/83 critérios C e 53/84 itens I**;
este registro é evidência parcial, não aceite integral de R01.1.

Validação desta rodada: frontend completo **320 arquivos / 3.070 testes
PASS**; após reforçar a prova de limpeza das subscriptions no StrictMode,
os cinco arquivos focais passaram novamente (**117 testes**). `tsc --noEmit`
e ESLint dos dez arquivos de código/testes tocados passaram. `git diff
--check` passou com a exclusão do whitespace preexistente no binding gerado
`frontend/wailsjs/go/models.ts`. Nenhum arquivo Go ou binding foi alterado
nesta rodada; não foi repetida a suíte Go, nem executado Wails/hardware ou
criado executável diagnóstico customizado.

As provas novas cobrem fonte ausente antes de iniciar chamadas, snapshot
alterado sem Notify, execução válida com sessão compartilhada, sessão
emprestada preservada no dispose, replacement de lease com payload idêntico,
mudança de rota/query, handoff após unmount, DOM desconectado e cleanup de
subscriptions no remount/StrictMode. A revisão principal solicitou testes
de mudança real do getter, além dos casos de dispose, antes da consolidação.

## 21. Providers dos quatro painéis e escopo React compartilhado — 16/09/2026

O App monta `CommandContextProvider` dentro do AuthGate. A Topbar e os
painéis usam a mesma sessão contextual; não há singleton global. O
WorkspaceContent fornece a referência do DOM efetivamente montado, e cada
página registra seu getter explicitamente por `useWorkspaceCommandSurface`.
Painel inativo, oculto/inert, desconectado, aba divergente ou fonte exigida
ausente não produzem contexto utilizável. O registro não é fabricado a
partir de activeTabId: o store apenas revalida o painel que já se registrou.

Fontes produtivas conectadas nesta rodada:

- Editor: documento específico no EditorStore, modo, readOnly e loadError.
- Terminal: sessão específica no TerminalStore, estado e shell.
- Tasklist: lista específica no TaskListStore, modo de visualização,
  disponibilidade da página e carregamento.
- Chat: conversa específica da aba, sessão carregada e estado de execução.

Os getters relêem stores sem Wails e não transportam buffers, drafts,
históricos ou coleções de tarefas/mensagens. São projeções mínimas de
identidade/estado, **não providers de seleção/conteúdo nem permissões de
execução**. Comandos que exigirem fatos adicionais precisam de integração
própria; não passam a disponíveis por haver este registro.

Notificações relevantes do recurso aposentam sua lease, inclusive troca e
retorno ao mesmo estado. A releitura síncrona continua obrigatória quando a
notificação se perde. Trocar owner aposenta o escopo inteiro e impede que um
callback antigo registre fontes no owner seguinte. O provider remonta um
escopo novo mesmo em A→B→A agrupado pelo React, sem ressuscitar tokens antigos.

Ctrl+K, quando iniciado num controle não editável de um painel registrado,
captura essa superfície para proteger a resposta do catálogo. Ausência de
dados ou ocultação desse registro não faz fallback para a toolbar. O clique
na barra mantém a origem toolbar; a execução global dos dois comandos já
montados continua pelo executor/ledger existente. Não foram migrados outros
atalhos nem habilitados teclado global/Stream Deck.

R01.1 permanece parcial: esta rodada liga as fontes das quatro superfícies,
mas não demonstra a composição de todos os contextos/condições e ingressos
previstos no AEP. Mantêm-se **1/48 saídas R, 0/12 gates R, 0/83 critérios C
e 53/84 itens históricos I**. R01.2 continua aceito; BASE-PRONTA segue aberto.

Na revisão de integração foram corrigidos: getters que seguiam a aba ativa
em vez do recurso da página, leitura de state capturado em render, timestamp
variável que invalidava toda leitura estável, foco sintético do terminal e
versão com hash/truncamento desnecessário. O registro de painéis usa efeito
passivo porque a referência DOM pertence ao ancestral; antes de estar
registrado, Ctrl+K no painel recusa a preparação em vez de assumir toolbar.
Os leitores continuam síncronos durante captura/commit; só o lifecycle do
registro aguarda o commit React. Não foi medida latência end-to-end.

Validação consolidada: **325 arquivos / 3.111 testes frontend PASS** (41
testes a mais que a seção 20), `tsc --noEmit` PASS e ESLint dos 24 arquivos
tocados PASS com `--max-warnings=0`. `git diff --check` PASS, mantendo apenas
a exclusão do whitespace preexistente no binding gerado `models.ts`.
Os testes incluem montagem tardia de painel com scope já existente,
StrictMode/cleanup, fonte ausente/oculta, notificações perdidas, ABA de fonte
e owner, recuperação em escopo novo e retarget sem rerender nos leitores
reais de páginas. Não houve mudança Go/banco/bindings nesta rodada; não foi
repetida a suíte Go nem iniciado Wails/hardware/executável diagnóstico.

## 22. Fronteira de ingresso e correlação ponta a ponta — 16/09/2026

Rastreia R01.3. Base: HEAD `7e88945ec585e4352c4548aad8cd14e4db0134c1`
mais alterações locais nesta worktree, ainda sem commit desta rodada.

Correções concretas, sem novos handlers ou hooks de teste em produção:

- **Δ09 / R01.3:** `commandbridge.Input` validava ownership, capability e
  vínculo do envelope depois de alterar o tracker. Agora entradas inválidas
  não consomem down nem fabricam release. A normalização de repetição continua
  devolvendo `not-a-dispatch-edge`, sem disparar outra execução.
- **Δ10 / R01.3:** o resultado de `commandProductRuntime.Dispatch` omitira
  identificadores de ocorrência/eventos. A ponte recusava a correlação e
  mantinha o claim após a conclusão. O resultado agora preserva a identidade
  completa, permitindo novo down após release legítimo.
- **Δ11 / R01.3:** prova de diálogo, candidato, ownership e registros entregues
  a callbacks tinham referências compartilhadas. Cópias independentes impedem
  que alterações do chamador, porta ou callback reescrevam a pendência,
  o envelope preparado ou o registro retornado pelo lookup.

Evidência integrada adicionada:

- `app_command_ingress_boundary_test.go`: seis entradas inválidas seguidas
  da entrada válida na mesma tecla, mais release falsificado e ciclo completo
  de nova ativação. Usa a composição produtiva e os handlers existentes.
- `TestCommandProductResolutionPersistsRejectedStaleAfterAuthenticatedOwnerChanges`:
  ingresso público pela bridge, perda de ownership entre resolução e reserva,
  marker `rejected_stale` sem linha em `command_invocations`, restauração do
  owner/remoção do delta, replay terminal do mesmo pedido e sucesso de pedido
  novo. A intercalação ocorre em callback GORM exclusivamente no teste.
- `TestUICommandSucceededResultOnlyAfterCompleteAndTakeOnce`: antes de permitir
  conclusão visual, o handoff já tem ledger **e auditoria** em `running`;
  comprova reserva/CAS anteriores à entrega, sem confundir ack com sucesso.
- Regressões em `internal/commandbridge` e `internal/commandexecution` cobrem
  mutação de campos indiretos e preservação dos dados nas revalidações.

O aceite desta fronteira não liga origens físicas: a composição atual admite
paleta; teclado/Stream Deck forjados na fachada pública continuam recusados.
Providers visuais completos, condições contextuais, listeners físicos e
recovery permanecem nos respectivos itens abertos, não foram descartados.

**Aceite de R01.3:** a composição publicada não oferece caminho alternativo
para handler. `CommandBridgeInvoke/Input` autenticam e entram em `Dispatch`,
que agenda `ExecuteEnvelope`, nunca o handler diretamente. Paleta resolve
defaults/deltas publicados; o executor comum autoriza, reserva e faz CAS antes
de `Start`. UI exige reserva e revalidação antes de `Take`. A execução direta
interna também usa o executor comum, com teste de operação real e replay único
em `TestCommandProductExecutesRealWorkspaceOperationAndPersistsExactlyOnce`.
As provas de suppress e rejected_stale não substituem handlers por sentinelas.

Contagem atual: **2/48 saídas R aceitas (R01.2 e R01.3), 0/12 gates R,
0/83 critérios finais C e 53/84 itens históricos I**. R01 permanece parcial:
R01.1 (contextos) e R01.4 (lifecycle/recovery) ainda precisam de aceite.
Nenhum novo pacote ou critério foi criado; Δ09–Δ11 corrigem a fronteira
prevista em R01.3. Nenhuma conclusão nova é atribuída aos achados F anteriores.

Validação final: `go test ./... -timeout=120s` **PASS** (com reutilização do
cache dos pacotes inalterados); App passou em execução sem cache (58,998 s),
incluindo os ingressos reais desta seção. `commandexecution` passou na rodada
final (7,003 s), e `commandcontract` após corrigir sua fixture. `go vet
./internal/commandbridge ./internal/commandcontract ./internal/commandexecution
./internal/app` **PASS**. `git diff --check` **PASS**, com a mesma exclusão do
whitespace preexistente de `frontend/wailsjs/go/models.ts`.

As rodadas intermediárias detectaram e corrigiram falhas dos testes novos
(ocorrência inválida/fixture incompleta) e uma regressão no lookup de markers:
envelope vazio de marker não deve passar pelo decoder de ingresso para ser
copiado. A solução reutiliza `Envelope.Clone`, a cópia já usada pelo contrato,
sem relaxar sua validação e sem manter código exclusivo para teste em produção.
ACP/acpregistry passaram nesta suíte; isso não identifica retroativamente a
causa dos alarmes históricos. Não houve mudança frontend, schema, bindings,
banco real, Wails ou executável diagnóstico; frontend não foi retestado nesta
rodada. Race/hardware continuam sem certificação nesta máquina.

## 23. Troca produtiva de workspace e composição contextual — 16/09/2026

R01.4: `configureWorkspaceController` liga o controller real a
`reloadCommandsAfterWorkspaceSwitch`. O controller serializa troca, callback
e evento. O App retira a publicação antiga antes de montar o novo produto,
reconstrói deltas do workspace corrente e só então publica. Não há mudança
no caminho de troca de abas ou nos bindings Wails.

Evidências em `app_command_workspace_switch_test.go`: configuração isolada
por workspace, mapa não publicado durante a leitura, runtime antigo fechado,
retorno sem ressuscitar runtime anterior, falha de leitura sem fallback e
retry válido, destino inválido preservando runtime, handoff visual pendente
aposentado com ledger/audit `outcome_unknown` e novo handoff funcional.
Os testes do controller verificam callback antes do evento, falha sem
callback e serialização de trocas concorrentes.

R01.1: `commandContextProduct.integration.test.tsx` compõe os providers React,
stores reais, painel, registro de superfície, modalRegistry, foco/IME e guard
de efeito. Cobre ausência de provider, painel inativo, mudança contextual
sem notificação, owner ABA e desmontagem em StrictMode com todas as
assinaturas encerradas. A auditoria backend confirmou que
workspace/aba/profile vêm de `Manager.CommandSnapshot`, foreground da porta
nativa e lock/SO de `HostState`; não foi criado provider fictício de SO.

**Aceite de R01.1:** a composição real é sustentada pelo teste integrado UI
desta seção, os testes por página/registro da seção 21, `app_command_context_test.go`,
`app_command_context_facts_test.go` e `app_command_os_session_test.go`.
`TestCommandSnapshotAguardaLockEObservaMutacaoAtomica` prova atomicidade na
fonte real sob mutação concorrente; o provider mantém `authMu.RLock` durante
ownership e leitura dessa fonte. `TestCommandFactBusRealNotificacaoPerdidaNaoEscondeMudanca`
e `TestCommandFactBusRealRecusaOwnershipWorkspaceEFonteAusente` completam a
prova de releitura sem depender de hints e recusa de fonte inválida. A UI
revalida imediatamente antes do efeito, sem round-trip Wails sob gate.
Este aceite não certifica condições R02, listeners físicos ou toda a migração
de comandos. Não foi necessário duplicar a prova de atomicidade dentro de
um provider que só retorna a versão produzida pelo mesmo manager.

R01.4/R04.2: a recuperação segue preflight somente leitura e fail-closed.
`DrainedGenerations` prova drain local, não morte de outro processo. O próximo
bloco exige autoridade exclusiva interprocesso vinculada às gerações antes
de conectar reconciliação de ledger/decisions ao bootstrap. PID presumido,
timeout isolado e limpeza de pending não são provas aceitáveis. A ligação
produtiva de `ConfigureCommandMaintenance` antes de `Start` também permanece
pendente. Isso é implementação restante, não dependência de teste manual.

Contagem naquela rodada: **3/48 saídas R (R01.1–R01.3), 0/12 gates R, 0/83 critérios
finais C e 53/84 itens históricos I**. R01.4 continua aberto; esta rodada
fecha os providers contextuais e a lacuna de troca de workspace, não o gate
completo nem a recuperação interprocesso.

Validação: `go test ./... -timeout=120s` PASS (App 45,963 s; pacotes
inalterados com cache, incluindo ACP/acpregistry); testes do controller
reexecutados sem cache após reforçar a prova de ordem. `go vet ./controllers
./internal/app` PASS. Frontend completo: **326 arquivos / 3.115 testes PASS**.
O teste integrado foi reexecutado após reforçar a contagem de unsubscribes
no unmount em StrictMode: 4/4 PASS; ESLint do arquivo PASS. `tsc --noEmit`
PASS. A primeira rodada frontend/TypeScript capturou uma edição intermediária
do teste novo (props incompletas e IME ainda não estabelecido); a suíte final
foi repetida com sucesso, sem relaxar as asserções. `git diff --check` PASS
com exclusão apenas do whitespace preexistente de `frontend/wailsjs/go/models.ts`.
Nenhum Wails, gerador de bindings, executável diagnóstico, banco real ou
dispositivo foi iniciado. Race e certificação física não foram executados.

## 24. Exclusão nativa e recovery registrado no bootstrap — 16/09/2026

R01.4/R04.2 aceitos no escopo abaixo; regressão global ainda pendente:

- `commandinstance.Acquire` mantém um handle do arquivo SQLite existente,
  sem escrever bytes ou alterar seu tamanho. Windows usa `LockFileEx` em um
  byte fora das regiões SQLite; Linux/macOS usam flock. Identidade deriva do
  handle (volume/FileIndex ou dev/inode), não do texto do caminho. Aliases e
  hardlinks concorrem pela mesma posse. Não há PID ou timeout como prova.
- Migração aditiva **v29** registra `command_process_generations`: startup
  UUIDv7 privado e identidade física. `Open` exige schema pronto, não migra
  silenciosamente. Startup não pode ser reassumido por upsert. A prova opaca
  inclui apenas namespaces anteriores registrados no mesmo arquivo e é
  vinculada à raiz SQL; exclui startup atual, desconhecidos e cópias.
- `EpochService.BindInstance` liga a posse antes do primeiro executor em
  `mountCommandProduct`. I/O fica fora do DispatchGate; uma barreira impede
  registrar executores durante o bind. Espera pela serialização é cancelável.
  Shutdown concorrente impede publicação tardia da lease.
- `appCommandLifecycleRuntime.Recover` usa essa prova nos writers existentes
  de decisions/ledger antes de Project/Publish/Enable. O preflight global
  continua recusando qualquer pendência não coberta, inclusive órfãos.
  Ledger e audit viram `outcome_unknown`, sem reexecução; receipts são
  canceladas/expiradas com evento terminal transacional.
- `drainCommandExecutors` só libera a posse depois de CloseAndDrain e recovery
  local bem-sucedidos. Falha conserva a posse. `ResetDatabase` ganhou callback
  de encerramento anterior a qualquer fechamento/remoção; após reset, este
  App permanece terminal para comandos até reinício, sem ressuscitar core.

Os testes usam arquivos temporários, handles reais e duas composições App,
incluindo multiusuário/system, disputa por instância, replay terminal, recusa
de geração atual/desconhecida e bootstrap sem mapa após falha. A revisão
também verifica cancelamento, registro concorrente, raiz SQL estrangeira,
cópia física, hardlink e migração de bancos publicados.

Limites: exclusão vale para participantes deste protocolo; processos legados
não passam a obedecê-lo retroativamente. Seus registros desconhecidos não
são reconciliados. Não foi simulado crash por subprocesso nem executado
binário diagnóstico. A liberação em término de processo é a semântica nativa
documentada de [LockFileEx](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-lockfileex),
distinta da prova automatizada de duas aquisições por handles nesta máquina.
Linux/macOS não foram executados; substituição externa de arquivo SQLite
aberto não é um protocolo suportado de restauração. A verificação do caminho
detecta drift observável, não certifica substituição externa concorrente.
R04.1 (cadência), R04.3 (claims/leases/progresso) e R04.4 (retenção completa)
continuam fora deste bloco; o gate R04 não é fechado por esta entrega.

**Evidência de aceite:** HEAD da baseline mais alterações locais, sem novo
commit. `app_command_restart_test.go`, `app_command_restart_scope_test.go`
e `app_command_restart_proof_test.go` exercem a composição real; os testes
de `commandinstance` e `commandsecurity/instance_test.go` cobrem posse e
autoridade. `app_command_database_reset_test.go` e o teste correspondente
do controller cobrem encerramento anterior ao reset. A migração produtiva
está ligada por `commandbootstrap.Migrate` → `ApplyCommandInstanceMigration`.

Validação final: `go test ./internal/commandinstance ./internal/commandsecurity
./internal/app ./internal/commandledger ./internal/commanddecision
./internal/commandbootstrap ./internal/database ./controllers -timeout=180s`
**PASS** (App 61,993 s). Após fortalecer a fixture do segundo usuário para
usuário e sessão realmente persistidos, o teste multiusuário/system foi
reexecutado com `-count=1`: **PASS**, 19,802 s. `go vet ./...` **PASS**.
Contenção, reaquisição e hardlink passaram explicitamente, sem skip.

`go test ./... -timeout=180s` **FAIL** somente em ACP/acpregistry. A execução
isolada normal `go test ./internal/acp ./internal/acpregistry -count=1 -p=1
-timeout=90s` repetiu `exit status 0xffffffff`, sem stacktrace. Não há alteração
local nesses pacotes ou em go.mod/go.sum. Consultas somente leitura aos logs
locais de Application, CodeIntegrity e Defender não localizaram evento
correspondente no intervalo consultado; **causa não determinada**, sem
atribuição ao antivírus nem declaração de falha inofensiva. Não foram gerados
executáveis diagnósticos, alteradas proteções ou usados bancos pessoais.
Frontend não mudou nesta rodada e não foi reexecutado; race segue pendente.

**Contagem vigente: 5/48 saídas R (R01.1–R01.4 e R04.2), 0/12 gates R,
0/83 critérios finais C e 53/84 itens históricos I.** R01 tem suas quatro
saídas aceitas, mas seu gate permanece em validação enquanto a regressão
global não está qualificada. Próximo bloco de implementação: cadência e
composição produtiva de manutenção (R04.1/R04.3/R04.4), respeitando as
dependências de jobs; a investigação de ACP/acpregistry permanece em R06.2.

## 25. Inicialização e retomada da manutenção — 16/09/2026

Continuação de R04.1/R04.3/R04.4, sem novo aceite integral:

- `jobs.Manager.Start` não executa o coordenador sob `mu/runtimeMu`.
  A passagem inicial passa pelo mesmo loop cancelável, com disparo imediato;
  `Stop` cancela e aguarda esse trabalho. Isso evita deadlock quando uma porta
  consulta o runtime durante a inicialização. A retenção legada não montada
  conserva seu comportamento; não foi criado um segundo timer.
- `MaintenanceHeartbeatAdapter` conserva o cursor do prefixo efetivamente
  confirmado quando uma página falha. A próxima passagem retoma no item que
  falhou; ao concluir a varredura, reinicia para revalidar todas as leases.
- `liveClaim` propaga erros de infraestrutura/cancelamento de Layer,
  Condition e Runtime. Não os converte em indisponibilidade definitiva.
  Reconciliação faz rollback em erro transitório e mantém cleanup para
  rejeições definitivas (fonte ausente, autorização obsoleta etc.). Nenhuma
  dessas falhas renova a lease; o commit revalida cancelamento.
- O adapter de retenção de invocações informa continuação após erro ou
  cancelamento. A transação que falhou informa zero exclusões; lotes anteriores
  permanecem confirmados. O teste usa trigger de falha somente no SQLite
  temporário, remove esse trigger e demonstra retomada.

As provas novas estão em `commandjobactivation/maintenance_resume_test.go`,
`commandjobactivation/lease_errors_test.go`,
`commandledger/coordinator_adapter_progress_test.go` e nos testes da cadência
em `jobs/command_maintenance_pass_test.go`. Portas instrumentadas nos testes
não são factories produtivas e não certificam a montagem completa no App.

**Dependência concreta para R04.1:** `initJobs` ainda não chama
`ConfigureCommandMaintenance`. Antes de habilitá-lo, o App precisa compor
`commandjobactivation.Consumer` com Authorize/Layer/Condition/Runtime e chaves
reais; anexar recuperação vinculada à prova do mesmo banco e as retenções;
configurar o Manager antes de `reloadUserScopedRuntime` chamar `Start`.
Persistência de job_runs sozinha não prova runtime vivo. Não foram adicionadas
portas que aceitam tudo, retornos vazios artificiais ou wiring incompleto.

Contagem permanece **5/48 saídas R, 0/12 gates R, 0/83 critérios finais C e
53/84 itens históricos I**. Esta rodada corrige execução/retomada dos serviços;
não declara R04 fechado nem a manutenção completa habilitada no App.

Validação (baseline HEAD mais alterações locais, sem novo commit):
`go test ./internal/jobs ./internal/commandjobactivation ./internal/commandmaintenance
./internal/commandledger ./internal/commandactivation ./internal/commanddecision
./internal/app -timeout=180s` **PASS**, App 61,343 s. Após a revisão final da
ordem de publicação de `started`, `go test ./internal/jobs -count=1
-timeout=120s` **PASS**, 14,254 s. `go vet` de jobs, commandjobactivation,
commandmaintenance, commandledger e app **PASS**. `git diff --check` **PASS**,
excluindo somente o whitespace preexistente no binding gerado models.ts.
A rodada intermediária detectou uma fixture nova com owners diferentes
no teste de cap por usuário; a fixture foi corrigida para um owner comum,
sem relaxar a prova de rollback/retomada.

A revisão focada de lifecycle/cursor foi realizada por agente Luna e conferida
pelo responsável; não equivale a Bugbot nem a race. O teste de cancelamento
da cadência foi reforçado: agora a contagem está ligada à porta chamada,
em vez de observar uma variável desconectada da execução. Não houve Wails,
geração de bindings, executáveis diagnósticos ou uso de banco pessoal.
Frontend não mudou e não foi reexecutado. ACP/acpregistry não foram
reexecutados nesta rodada; o bloqueio global da seção 24 não foi resolvido.

## 26. Montagem produtiva da manutenção e autoridade viva de jobs

Rodada de 16–17/09/2026, HEAD `7e88945ec585e4352c4548aad8cd14e4db0134c1`
mais alterações locais, sem novo commit. **R04.1 aceito**: a composição antes
ausente agora acontece em `App.configureCommandMaintenance`, chamada por
`reloadUserScopedRuntime` antes de `jobs.Start`, quando o storage está pronto.

- Consumer usa o mesmo DispatchGate, chave em cache, sessão real revalidada
  na mesma transação, layers persistidas e parser estrito de condições.
- O executor gera a identidade privada do run, não aceita a chave reservada
  recebida em proveniência. Manager exige linha persistida, registro vivo e
  watch válido; Stop invalida o registro e capturas atrasadas não atravessam
  Stop/Start. Perder a autoridade de comandos não cancela a tool legada.
- A fence de sessão permite consumir terminais depois da retirada do run;
  não prova liveness. Capturas simultâneas da mesma sessão não cancelam umas
  às outras; substituição da fence passa pelo gate.
- As portas reais de heartbeat, outbox/purga bounded, recovery, retenção de
  invocações/ativações, jobs, tools e compactação usam a cadência existente.
  O primeiro epoch de replay é criado antes de Start quando ainda ausente;
  deadlines existentes não são recalculados.
- Remontagem por mudança de sessão/cofre/versão exige Manager frio. Erro
  preserva o coordenador anterior e impede Start, sem fallback silencioso.
  O lock da instância fica retido em falha de setup; retry é idempotente,
  não libera prova que ainda pode pertencer a executores existentes.
- CloseCommandMaintenance cancela e aguarda a passagem antes da liberação
  da instância. Timeout não vira sucesso em uma segunda chamada: ela espera
  o mesmo join. O fechamento é terminal para Start e reconfiguração.

Evidências: testes reais do App em `app_command_maintenance_test.go` compõem
todos os adapters, executam ciclo até compactação e verificam drain. Testes
do Manager demonstram único loop, ordem e reconfiguração fria; testes de
commandmaintenance verificam heartbeat/outbox antes de retenções e bloqueio
de compactação enquanto há continuação. `commandjobevents/retention_test.go`
comprova deadline, proteção de lease/claim viva, paginação e rollback.
Auth prova revalidação na mesma transação com pool de uma conexão.

Limites: **não fecha R03.1, R04.3/R04.4 nem Gate R04**. Fatos UI/device/foco/
foreground ausentes continuam desconhecidos, não valores fabricados. Ainda
faltam projeção das mudanças de claims no produto, política dinâmica de
lease/retenção do Consumer e prova integrada de retomada multiusuário/system
independente do usuário ativo. A montagem não certifica essas obrigações.

Contagem vigente: **6/48 saídas R (R01.1–R01.4, R04.1–R04.2), 0/12 gates R,
0/83 critérios finais C, 53/84 históricos I**. Não representa percentual de
código nem soma entre denominadores. R01 continua dependendo da regressão
global bloqueada por ACP/acpregistry; esses pacotes não foram reexecutados.

Validação inicial: `go test ./internal/jobs ./internal/auth
./internal/commandconfig ./internal/commandbindings ./internal/commandjobactivation
./internal/commandmaintenance ./internal/commandledger ./internal/commandactivation
./internal/commanddecision ./internal/app -count=1 -timeout=180s` PASS,
App 48,989 s. `go test ./internal/commandjobevents -count=1 -timeout=120s` PASS.
Vet de App/jobs/auth/config/bindings/jobactivation/maintenance PASS.
Correções posteriores da fronteira de workspace têm validação complementar
registrada abaixo. Nenhum Wails, binding gerado, executável diagnóstico,
alteração de antivírus, banco pessoal ou teste físico nesta rodada.

Complemento da revisão: a troca de workspace/perfil não usa o DispatchGate.
Para não avaliar uma condição sobre estado que mudou antes do commit,
`Ports.WithContext` envolve os três caminhos transacionais do Consumer;
o App mantém `workspace.WithCommandSnapshot` até commit/rollback e as portas
usam o snapshot transportado, sem readquirir o lock do Manager. A ordem é
gate → authMu (leitura) → snapshot do workspace → transação. Principal,
dependências e snapshot seguem pelo contexto, sem readquirir esses locks
nas portas, evitando inversão com outros leitores e escritores pendentes.
Não há tool, UI nem rede dentro
dessa fronteira. Testes demonstram bloqueio do escritor e liberação em erro;
fatos ausentes (inclusive app.focused=false) não são presumidos conhecidos.
Esta fronteira mantém a consistência; não é uma certificação de latência.

Validação complementar após a correção: App **PASS 66,273 s**, jobs **PASS
14,897 s**, workspace **PASS 0,949 s** (`-count=1 -timeout=180s`). Essa chamada
capturou um teste do agente ainda em edição e falhou a compilação apenas de
commandjobactivation; finalizado o teste, a reexecução integral desse pacote
com `-count=1 -timeout=120s` deu **PASS 9,521 s**. Vet final de App/workspace/
jobactivation/jobs/auth/config/bindings/maintenance **PASS**. `git diff --check`
**PASS**, excluído apenas o whitespace preexistente em models.ts gerado.

Revisão final da ordem de locks removeu a reconstrução do catálogo da porta
Layer: ela usa exclusivamente o registry imutável já publicado. Authorize e
Condition usam principal/dependências congelados pelo contexto, sem readquirir
authMu. Reexecução dos testes App `TestCommand(Maintenance|Job)` após esse
ajuste: **PASS 24,892 s**. Revisão independente Luna não identificou inversão
restante no caminho do Consumer; isso não substitui race nem teste de latência.

## 27. Política por passagem e recuperação independente da UI

Rodada de 17/09/2026 sobre a mesma worktree e HEAD mais alterações locais,
sem commit, push, Wails, executável diagnóstico ou banco pessoal.

`Coordinator.Run` publica uma cópia validada de Policy no contexto interno
da passagem. `Consumer.durations` usa essa mesma fotografia para novas leases,
renovação e retenção, sem reler config por registro nem alterar campos
compartilhados. Chamadas diretas fora da cadência conservam a política
validada na construção; o App produtivo usa a cadência. A redução da política
não recalcula deadlines de fontes/ledgers existentes e não ressuscita leases
vencidas. Settings inválidos não chegam às portas do coordenador.

`App.withCommandJobContext` distingue indisponibilidade da fonte de UI de
falha transacional: workspace ausente ou sem snapshot mantém a manutenção
apta a retirar claims sem fonte; Authorize/Condition recusam ativar/renovar.
O callback transacional não é repetido nem seu erro convertido em ausência
de contexto. A ordem de locks da seção 26 é preservada.

`Report.Stage` identifica a última etapa iniciada. O log de erro do Manager
inclui essa etapa e contadores confirmados, sem acrescentar identidade de
usuário, argumentos ou segredos. O erro original é preservado. O guia DATABASE
explica releitura, limites de replay e diagnóstico operacional.

Evidências novas:

- `commandmaintenance/policy_context_test.go`: snapshot por passagem,
  isolamento da cópia e recusa antes das portas para política inválida.
- `app/app_command_maintenance_multiuser_test.go`: portas reais do App,
  paginação entre dois usuários, esgotamento/idempotência, cancelamento,
  rollback e retomada; sem usuário/workspace ativo e com workspace ainda
  não inicializado. São claims sem fonte, não uma prova de job vivo completo.
- `app/app_command_job_conditions_test.go`: erro da transação propagado uma
  única vez, sem callback após cancelamento.

**Aceites R04.3 e R04.4 (17/09/2026):** evidências complementares:

- `commandmaintenance/recovery_multiuser_test.go`: receipts e invocações de
  usuários distintos, ledger system reservado pela API real, prova opaca de
  CloseAndDrain, preservação de outra geração viva/desconhecida, batches de
  tamanho 1, cancelamento durante escrita de receipt, erro SQL e retomada
  sem duplicar efeitos. Os demais domínios são spies explicitamente indicados.
  Receipts interativas são local_session; system é qualificado no ledger,
  não por inventar um presenter/consentimento automático para system.
- `jobs/command_maintenance_dynamic_settings_test.go`: SaveMaintenance em
  arquivo temporário e duas chamadas reais de runCommandMaintenance;
  os seis settings mudam no próximo snapshot, PolicyFromContext coincide
  com a policy da passagem; converter recusa zero e overflow. Os spies desse
  teste inspecionam a política, não certificam operações de retenção.
- `commandjobactivation/policy_integration_test.go`: outbox/heartbeat/Consumer
  reais aplicam lease e retenção novas; redução preserva expiries existentes
  e replay; política inválida não modifica claims, leases ou ledgers.
- Idade/caps e exclusão de ativos seguem cobertos nas suítes reais
  `commandactivation/maintenance_test.go`, `commandledger/maintenance_test.go`,
  `commandledger/coordinator_adapter_test.go` e `commandjobevents/retention_test.go`.
  Falha/cancelamento e progresso confirmado constam também nos testes
  `coordinator_adapter_progress_test.go` e `maintenance_resume_test.go`.
- A UI existente dos seis settings foi revalidada por
  `npm test -- src/pages/DataManagementPage.test.tsx`: **28/28 PASS**.

Contagem vigente: **8/48 saídas R, 0/12 gates, 0/83 critérios finais C,
53/84 históricos I**. R04 tem as quatro saídas aceitas, mas **não se certifica
Gate R04**: falta um único cenário integrado de restart no App com todos os
domínios reais e job vivo/claims protegidas ao longo do ciclo. Os testes por
domínio não são apresentados como essa prova. R03 permanece aberto, inclusive
projeção de mudanças de claims no runtime. Não houve aceites novos de C/I.

Regressão final: `go test ./internal/app ./controllers ./internal/auth
./internal/commandledger ./internal/commandactivation ./internal/commanddecision
./internal/commandjobevents ./internal/config -count=1 -timeout=180s` **PASS**
(App 57,242 s); jobs **PASS 5,133 s**, commandmaintenance **PASS 0,857 s**.
Vet de App/jobs/maintenance/jobactivation/ledger/decision/activation **PASS**.
A primeira tentativa ampliada capturou o teste de política do agente ainda
em edição e falhou a compilação apenas de commandjobactivation; sua validação
final é registrada abaixo. ACP/acpregistry não foram reexecutados e a falha
global anterior não está resolvida. Revisão focada Luna não encontrou bloqueio;
não equivale a Bugbot nem a race. Dados e falhas injetadas só em DBs temporários.

Reexecução final de `go test ./internal/commandjobactivation -count=1
-timeout=120s`: **PASS 11,612 s**. Vet final dos pacotes alterados e
`git diff --check` **PASS**, excluindo somente o whitespace preexistente de
models.ts gerado. Não foram criados commits nem feitas alterações remotas.

## 28. Gate R04 — Restart e manutenção integrada com trabalho vivo

Aceite de 17/09/2026 na mesma branch/HEAD mais alterações locais, sem commit.
**Contagem vigente: 8/48 saídas R, 1/12 gates (R04), 0/83 critérios finais C,
53/84 históricos I**. Os contadores anteriores são históricos; não há novo
aceite de saída R nem soma entre denominadores.

`TestCommandMaintenanceRestartRecoversMultipleOwnersAndProtectsLiveJob`, em
`internal/app/app_command_maintenance_restart_test.go`, recria App, core,
workspace Manager e jobs Manager sobre o mesmo SQLite temporário. Fecha e
libera a instância anterior por APIs reais; o novo bootstrap obtém sua prova
de restart e recupera antes de publicar o catálogo. A única cadência automática
de `jobs.Manager.Start` usa Consumer, stores e adapters reais, sem portas de
autorização/runtime falsas nem coordenador paralelo.

Evidências verificadas no mesmo cenário:

- Receipts de duas sessões/usuários terminam uma única vez; invocações dos
  dois usuários e system convergem para `outcome_unknown` no ledger e auditoria.
- Pendências da geração nova, de usuário e system, permanecem `evaluating`.
- Claims sem fonte de dois usuários são reconciliadas; uma sentinela SQL no
  banco temporário recusa exclusão de runs antes dessa reconciliação.
- Runs antigos, dry runs, tools de chat órfãs e antigas são removidos para os
  dois usuários. Auditoria/ledger de invocação expirada e claim terminal antiga
  também são removidos. A conversão real de `auto_vacuum` para incremental
  comprova execução do adapter físico de compactação.
- Um job manual real executa uma tool controlada por canal, com ledger real,
  regra e grant HMAC confirmados pelas APIs reais. A claim permanece ativa e a
  lease é renovada além do TTL inicial durante as passagens de manutenção.
- Após liberar a tool, o job conclui, a outbox `completed` fica `delivered`, a
  lease desaparece e a claim fica `deactivated` com motivo `source_terminal`.
  Os helpers aguardam o encerramento do job inclusive no cleanup de falhas.

**Δ12 / R03–R04, corrigido:** o percurso real revelou mistura UTC/local no
`commandjobevents.Store`. A consulta textual SQLite de epoch em UTC contra um
evento local retornava `ErrBootstrapIncomplete`; leases locais também eram
comparadas com UTC pelo Consumer. O Store agora normaliza gravações/consultas
temporais em UTC antes do fingerprint do novo fato, sem alterar instantes ou
recalcular deadlines persistidos. `timezone_test.go` cobre offsets negativos
e positivos, mesma ocorrência idempotente, epoch futuro não selecionado,
claim/ack e requeue. Não se adicionou fallback de autorização ou migração de
compatibilidade para uma integração ainda nova.

Validação:

- Cenário integrado acima com `-count=3 -timeout=120s`: **PASS 35,452 s**.
- Reexecução final dos dois testes `^TestCommandMaintenanceRestart`, após
  exigir modo inicial `auto_vacuum=0`: **PASS 24,621 s**.
- `TestCommandMaintenanceRestartRemountsRealApp`: **PASS 18,729 s**.
- `TestCommandMaintenanceLiveJobCreatesAndRenewsLeaseUntilToolRelease`:
  **PASS 21,074 s**.
- `go test ./internal/commandjobevents ./internal/jobs
  ./internal/commandjobactivation ./internal/commandmaintenance -count=1
  -timeout=120s`: **PASS** (0,494 s / 14,024 s / 16,189 s / 0,812 s).
- `go vet` de App/jobs/jobevents/jobactivation/maintenance: **PASS**.
- Regressão ampliada `go test ./internal/app ./internal/commandledger
  ./internal/commanddecision ./internal/commandactivation
  ./internal/commandsecurity ./internal/database -count=1 -timeout=180s`:
  **PASS** (App 63,161 s; demais 3,526 s / 2,826 s / 1,567 s / 0,646 s /
  16,561 s). ACP/acpregistry não foram reexecutados; isso não resolve a
  pendência global já registrada. `git diff --check` dos arquivos desta
  rodada: **PASS**.
- Revisão focada Luna dos cenários e da correção UTC: sem bloqueio final;
  não equivale a Bugbot, race ou regressão global.

Limites: restart é remontagem no mesmo processo com liberação nativa real,
não `kill`/crash de processo. Os órfãos sem fonte são inseridos após bootstrap,
não apresentados como configuração válida presente no boot. As pendências
usam APIs reais de ledger/receipt; este cenário verifica seus estados, não um
contador de efeitos de handler anterior. A propriedade de recovery sem
despacho apoia-se também nos serviços e testes das seções 24–27. Não houve
Wails, executável diagnóstico, alteração de banco pessoal ou operação remota.
R03 continua aberto: qualificação das demais origens/retries/replay e projeção
das mudanças de claims no runtime; R01 ainda aguarda a regressão global já
registrada. O AEP permanece **In Progress**.

## 29. R03 — Autoridade viva, condições e isolamento dos ciclos

Aceite de 17/09/2026 na branch `feat/aep-0103-comandos`, HEAD
`7e88945ec585e4352c4548aad8cd14e4db0134c1` mais alterações locais, sem commit.
**R03.1 e R03.3 aceitos; 10/48 saídas R, 1/12 gates (R04), 0/83 critérios
finais C e 53/84 históricos I.** R03.2/R03.4 e o Gate R03 continuam abertos.

**Δ13 / R03, corrigido:** a prova de vida do job usava o watch de execução
de comandos, cancelado em toda publicação de configuração. Isso poderia
invalidar a própria fonte contextual ao publicar sua camada. Em
`internal/commandsecurity/execution.go`, `WatchSecurityEpoch` separa a vida
da fonte da resolução antiga de um comando. `publication.go` continua
cancelando comandos preparados/em execução; sessão, bloqueio, drain e contexto
encerrado cancelam também as fontes. `app_command_maintenance.go` usa essa
porta para a geração privada do run e a autoridade de sessão. Não há bypass
de grants, condições ou validação inicial, nem leitura SQL por tecla.

Evidências novas, complementando montagem, indisponibilidade, shutdown e
manutenção real das seções 26–28:

- `internal/app/app_command_job_publication_test.go`,
  `TestCommandJobSourceSurvivesRealConfigurationRebuild`: App, Consumer, DB,
  grant e job reais; rebuild real cancela watch de comando, preserva fonte e
  renova lease além do TTL inicial. Conclusão entrega outbox e remove lease.
- No mesmo arquivo, `TestCommandJobLiveClaimEndsWhenAuthoritativeProfileChanges`:
  mudança do perfil real remove claim/lease cuja condição ficou falsa, sem
  encerrar o job. A correção do fixture para `Source=user` substitui o valor
  inválido `test`; não foi flexibilizada a validação de produção.
- `internal/commandsecurity/source_watch_test.go`: publicação/mutação,
  assinatura obsoleta/cancelada, liberação idempotente, lock, parent e drain.
- `internal/commandjobactivation/cycle_isolation_integration_test.go`: SQLite
  e Consumer reais, portas de autoridade controladas; terminal atrasado só
  encerra seu run, retry preserva lease, fonte ausente/runtime ausente ou
  divergente revoga claim. Outbox recusa origens externas/desconhecidas.
- `internal/jobs/command_activation_origins_test.go`: executor/ledger/outbox
  reais com identidade confiável controlada e watch real; manual, cron,
  interval, hotkey e internal_event produzem queued/started/completed;
  cenários adicionais cobrem retry_scheduled, failed e skipped. Sequências e
  fingerprints correspondem à timeline. Invoca diretamente o executor:
  **não certifica ingresso do scheduler/hotkey/EventBus nem fecha R03.2**.

Validação final:

- Os dois testes novos do App, `-count=3 -timeout=120s`: **PASS 24,555 s**.
- `go test ./internal/app ./internal/commandsecurity ./internal/commandexecution
  ./internal/commandmaintenance -count=1 -timeout=180s`: **PASS**, respectivamente
  88,049 s / 4,289 s / 10,189 s / 5,129 s.
- `go test ./internal/jobs ./internal/commandjobactivation
  ./internal/commandjobevents -count=1 -timeout=120s`: **PASS**, respectivamente
  14,323 s / 16,954 s / 0,874 s. Matriz de origens revalidada após ajuste final
  do watch: **PASS 10,526 s**.
- `go vet ./internal/app ./internal/commandsecurity ./internal/commandexecution
  ./internal/jobs ./internal/commandjobactivation`: **PASS**.
- Implementação de testes em escopos separados e revisão focada Luna, sem
  bloqueio final; não equivale a revisão global ou race (`CGO_ENABLED=0`).

**Próximo fechamento:** projetar claims autorizadas de jobs no mapa efetivo
do resolvedor, preservando escopo/condição/generation e recuperação de
notificação perdida; completar ingressos e replay de R03.2/R03.4. O loader
atual ainda seleciona claims manuais: claim persistida não é prova de camada
ativa na resolução. Não basta incluir `SourceType=job` sem essas validações.
A regressão global ACP anterior continua pendente; não foi reexecutada aqui.
Sem Wails, executáveis diagnósticos, banco pessoal, commit ou operação remota.

## 30. R03 — Claims de jobs no resolvedor real

Implementação de 17/09/2026, mesma branch/HEAD mais alterações locais, sem
commit. **Contagem mantida: 10/48 saídas R, 1/12 gates, 0/83 critérios C e
53/84 históricos I.** Esta rodada fecha a ligação antes pendente entre claim
autorizada e mapa efetivo, não antecipa aceite de R03.2/R03.4 ou do gate.

`commandjobactivation.Consumer.Projection` revalida claim/lease/fonte,
regra/grant, condição, runtime, sessão e gerações na mesma fronteira
gate/contexto/transação. Retorna cópias, revisão e menor prazo de validade;
não renova nem cria claims. Une global e workspace atual sem incluir outro
owner/escopo. Falhas de infraestrutura não viram autorização ou mapa vazio.

`app_command_job_projection.go` integra essa prova à configuração compilada
real, publicada atomicamente com guard pelo `HostState`. O guard consulta
somente revisão do Consumer, relógio, versão autoritativa do workspace e
identidades vivas do Manager em memória. As entradas de paleta/bridge/UI
reconstroem quando essa prova fica obsoleta; a admissão relê o guard.
Não depende de notificações, não acrescenta timer e não consulta SQLite no
caminho estável da projeção. A leitura durável ocorre na reconstrução, não
por tecla. Acionadores físicos ainda dependem de seus próprios gates.

**Δ14 / R03, resolvido:** autenticação de uma fonte de job não pode depender
do mapa que a própria fonte está reconstruindo. `HostState.SourceSecurityReady`
verifica somente cofre/SO; sessão exata e epochs continuam autenticados pelas
portas do App. Essa consulta não entrega mapa nem permite despacho. O executor
continua exigindo os snapshots guardados completos.
O guard roda fora do mutex do host para evitar inversão com o workspace.

Uma renovação que produz configuração e conjunto ativo equivalentes troca
apenas a prova sob o gate: preserva gerações visíveis e comandos em andamento.
Mudança efetiva continua invalidando o mapa antigo. Refresh contextual não
restaura claims manuais nem apaga o mapa antes de calcular seu substituto.

Provas integradas novas:

- `app_command_job_resolution_test.go`: default executa; claim real com binding
  persistido causa suppress no resolvedor real; término do runtime ou mudança
  de perfil retira o efeito. Preparação de configuração usa SQL somente no
  fixture, não é apresentada como detecção de escrita externa em produção.
- `app_command_job_projection_test.go`: lease expira sem manutenção/notificação
  e não mantém camada; heartbeat real seguido de refresh preserva versões e
  watch de comando; 25 consultas estáveis não fazem Query/Raw SQL adicional.
- `host_projection_guard_test.go`: guard no commit, invalidação dos dois
  snapshots, substituição e segurança independente da frescura do mapa.
- `commandjobactivation/projection_test.go` e
  `jobs/command_activation_projection_test.go`: scopes, provas ausentes/stale,
  falha de infraestrutura, identidade exata, unregister/Stop/cancelamento.

Limites: revisão da projeção é conservadora por Consumer; escritas externas
fora do protocolo de configuração não são suportadas. Há limite defensivo
de leitura, com recusa explícita em vez de projeção parcial. Sem novos aceites
de ingressos scheduler/hotkey/EventBus, recuperação abrupta, count-cap,
anti-loop ou múltiplos workspaces no App real. Esses percursos continuam em
R03.2/R03.4; a revisão global ACP anterior também continua pendente.

Correções encontradas durante a qualificação:

- Reconstrução disputa com queued/started/heartbeat. Uma leitura obsoleta
  pode ser refeita até três vezes antes do despacho; restore manual ocorre
  uma única vez. Não há retry de comando já iniciado ou de efeito externo.
- **Δ15 / lifecycle:** registro do drain, configuração do Manager e publicação
  da montagem passam pela mesma seção exclusiva. Configuração recusada não
  deixa drain registrado; shutdown não atravessa montagem parcialmente
  publicada. `executor_lifecycle.go` e testes focados registram essa fronteira.
- O fixture de recovery multiusuário tinha relógio fixo às 10:00 UTC; depois
  das 10:01 reais, `context.WithDeadline` nascia vencido. O teste agora fixa
  um instante futuro por fixture, preservando todas as asserções e relações
  temporais. Não era timeout de produção nem motivo para retirar testes.

Repetições focadas: resolvedor após terminal/perfil com manutenção parada,
`-count=3`: **PASS 28,933 s**. Rebuild da fonte, heartbeat/caminho sem SQL e
expiração sem manutenção, `-count=3`: **PASS 35,652 s**. Recovery multiusuário,
`-count=3`: **PASS 5,240 s**. A primeira regressão ampliada falhou nos casos
de relógio do fixture e na reconstrução concorrente acima; esses resultados
não são ocultados por reexecuções focadas.

Regressão ampliada após essas correções:
`go test ./internal/app ./internal/commandexecution ./internal/commandsecurity
./internal/commandbindings ./internal/jobs ./internal/commandjobactivation
./internal/commandjobevents ./internal/commandmaintenance -count=1 -timeout=240s`:
**PASS** — respectivamente 97,326 s / 6,178 s / 0,635 s / 0,175 s / 13,234 s /
18,864 s / 0,544 s / 4,529 s. `go vet` de App/execution/security/bindings/jobs/
jobactivation/maintenance: **PASS**. Montagem versus shutdown e retry após
montagem recusada no App real: **PASS 19,243 s** em teste focado.

Revisão final e revalidação:

- O guard também confere a identidade do Manager instalado e mantém o lock
  de autenticação durante a leitura: um Manager aposentado não conserva
  autoridade só porque seu snapshot ainda tem a versão antiga. Teste cobre
  substituição da fonte sem notificação.
- **Δ16 / terminal de job:** o executor retira a prova runtime uma única vez
  no início da finalização, antes de persistir terminal e chamar `OnRunEnd`.
  O defer de segurança continua cobrindo saídas anteriores. Um callback
  terminal bloqueado não mantém a camada ativa. Teste com DB/outbox reais,
  identidade controlada e tracker real em
  `jobs/command_activation_terminal_projection_test.go`; cleanup libera e
  aguarda o worker antes de fechar o DB. A primeira execução do fixture
  recusou IDs inválidos; foram usados UUIDv7 válidos sem relaxar o contrato.
  Teste focado final: **PASS 10,117 s**.
- Após os ajustes finais, `go test ./internal/app -run '^TestCommandJob'
  -count=1 -timeout=120s`: **PASS 42,594 s**; pacote completo `internal/jobs`:
  **PASS 13,921 s**. `commandsecurity` e `commandbindings` também passaram
  na revalidação (0,606 s / 0,210 s); a comparação de equivalência ganhou
  casos de layer, binding, alvo, argumentos, condição, prioridade e review.
- `go vet` final e `git diff --check` dos arquivos versionados da rodada:
  **PASS**. Revisão focada Luna: dois alertas esclarecidos pelo contrato de
  commit/restore já existente e identidade do Manager corrigida; sem outro
  bloqueio concreto nessa revisão. Não equivale a Bugbot/race/global.

Não houve Wails, executáveis diagnósticos, banco pessoal ou operações remotas.
O próximo bloco continua sendo R03.2/R03.4, não testes manuais desta projeção.

## 31. Ingressos encadeados, recuperação e retenção — 17/09/2026

Base: HEAD `7e88945ec585e4352c4548aad8cd14e4db0134c1` mais alterações locais,
sem commit novo. Contagem preservada: **10/48 saídas R, 1/12 gates, 0/83 C,
53/84 históricos I**. Implementação/revisão paralela por seis agentes Luna,
com integração e correções pelo agente principal.

### Implementação efetiva

- `jobs/command_event_origin.go`, `executor.go` e `manager.go`: raiz e
  históricos viajam em contexto privado após `queued`, com cópia profunda e
  vínculo ao usuário. O listener real e a chamada aninhada `RunJobContext`
  preservam a origem; payload não a substitui. Marker de outro usuário vira
  `unknown`, nunca fallback manual. Histórico inválido não é apagado para
  reiniciar cadeia. Webhook legado sem adapter autenticado continua `unknown`.
- **Δ17 / R03.4, corrigido:** `retainableRunQuery` usava `EXISTS` onde precisava
  de `NOT EXISTS`, incluindo runs protegidos na remoção. A seleção/ranking e
  remoção agora compartilham transação; não há janela de concessão de lease
  entre as duas etapas. Ausência das tabelas continua fail-closed.
- Retenção compara instantes com `julianday`, inclusive no ranking, evitando
  comparação textual entre UTC e horário local. Usa `queued_at` quando
  `started_at` é NULL, para que run órfão em fila não seja imortal.
- `commandjobactivation.verifyJob`: run comprovadamente ausente classifica
  como `source_not_found` permanente, sem gastar oito retries. Erro SQL não
  vira ausência; continua sujeito à política de falhas transitórias.

### Evidências e limites

- `command_activation_ingress_scheduler_test.go`: intervalo dispara pelo
  Manager/Scheduler reais; cron executa o callback registrado em `Entry.Job`,
  sem esperar virada de minuto. Ambos chegam à timeline e à outbox real.
  Não é teste físico de hotkey nem teste do relógio do cron.
- `command_event_origin_test.go`: Manager/EventBus, tool e dois saltos reais
  preservam raiz e estados; payload adulterado e contexto foreign não ganham
  autoridade. `command_event_chain_test.go` cobre snapshot profundo, lista
  vazia válida, limite 16/17 e preservação da cadeia no barramento.
- `recovery_matrix_test.go`: nova instância de Consumer recupera lease de
  entrega vencida; falha no ack reverte claims/ledger na mesma transação;
  entrega concluída não é reclamada. Replay adversarial com reset SQL fica
  explicitamente separado da recuperação. Global + dois workspaces,
  sequência, fingerprint e usuário foreign têm provas no Consumer real com
  portas de autoridade controladas. Não houve encerramento de processo.
- `command_activation_retention_matrix_test.go`: modelos/migrações reais,
  preservação de outbox/claims, proteção de run vivo e retorno à retenção
  depois de expiração. `retry_boundary_test.go`: limite exato e dead-letter
  não reaberto; fonte removida é recusada na primeira tentativa.

Validação consolidada dos pacotes jobs/jobactivation/jobevents/execution/
maintenance/tools-job: **PASS** (5,708 / 14,441 / 0,568 / 5,987 / 0,916 /
9,146 s). App completo: **PASS 90,590 s**, antes do último ajuste de fusos na
retenção. `go vet` dos sete pacotes: **PASS**. As primeiras rodadas detectaram
erros nos fixtures novos (slug/UUID e nomes de estado timeline versus outbox)
e a comparação UTC/local; foram corrigidos, sem retirar testes existentes.

Revalidação final: `go test ./internal/app -run 'TestCommand(Job|Maintenance)'
-count=1 -timeout=150s`: **PASS 52,849 s** após o ajuste de retenção.
Pacotes completos jobs/jobactivation/jobevents: **PASS 5,468 / 12,829 /
0,505 s**. Testes de proteção/expiração e ranking com ordem textual inversa
à ordem real de UTC−3/UTC, `-count=3`: **PASS 14,332 s**. A mesma prova
preserva queued recente com `started_at` SQL NULL. `go vet` final e
`git diff --check` dos arquivos versionados tocados: **PASS**.

### Próxima fronteira objetiva, ainda aberta

A projeção de claim no App publica layer IDs e guard de runtime, mas ainda
não leva a proveniência da fonte ao `Envelope.Provenance` do comando resolvido.
O helper `prepareCommandChain` limita a cadeia recebida, porém isso sozinho
não prova herança no percurso job → camada → comando. É requisito já previsto
em D8, não um novo pacote. Por isso **R03.4 não recebe aceite** nesta rodada.
R03.2 também mantém hotkey real e origem raiz `internal_event` autenticada
pendentes; eventos públicos legados não foram promovidos para preencher a
contagem. Nenhum teste manual novo é solicitado neste ponto.

Sem Wails, executáveis diagnósticos customizados, bypass de antivírus,
alteração do banco pessoal ou operação remota.

## 32. Proveniência da camada até o comando e auditoria — 17/09/2026

Base: mesmo HEAD `7e88945ec585e4352c4548aad8cd14e4db0134c1` mais alterações
locais. Seis agentes Luna em implementação/testes/revisão, com integração pelo
agente principal. **10/48 saídas R, 1/12 gates, 0/83 C, 53/84 históricos I**;
esta rodada fecha a ponte identificada na seção 31, não certifica todo R03.

### Implementado e revisado

- `commandjobactivation.Projection`: proveniência vem do fato da outbox após
  verificação de fingerprint e autoridade, nunca do campo solto da claim.
- `commandbindings.Configuration`: snapshot imutável de fontes por camada,
  cópias profundas, seleção apenas das layers resolvidas e preservação após
  restore. Metadata participa da equivalência: heartbeat idêntico preserva
  watches; mudança de fonte/proveniência invalida versões e execução pendente.
- `app_command_job_projection.go` e `app_command_resolution.go`: metadata e
  mapa são publicados juntos sob o guard existente. A resolução lê memória,
  sem nova consulta SQLite. Fontes selecionadas sem cadeia ou com cadeias
  divergentes recusam execução, sem prioridade arbitrária entre claims.
- `app_command_job_provenance.go`: documento de envelope v1 conserva somente
  os campos estruturais de origem/cadeia; identidade privada não sai da
  projeção. Histórico malformado não é apagado para reiniciar cadeia.
- `commandexecution/envelope_pipeline.go`: proveniência confiável do resolvedor
  é separada da fotografia original do host, revalidada antes da admissão e
  herdada antes de acrescentar o comando. Payload do candidato não é fonte
  de autoridade. Repetição/limite inválido retorna `ErrDenied` **antes** da
  reserva; antes desta correção a recusa ainda podia produzir uma reserva.
- `commandledger/envelope.go`: documento auditável passa a conservar campos
  estruturais válidos, incluindo a cadeia de comandos, com `version:1` e
  `redacted:true`. Payloads arbitrários, claims e identidade privada não
  persistem. Documento inválido vira marcador redigido; isso não transforma
  auditoria em autorização. `FullRecord` público continua sem proveniência.

### Evidência e limites de cada prova

- `app_command_job_provenance_integration_test.go`: job/Manager, grant,
  timeline/outbox, Consumer, projeção, binding execute, pipeline e gravação
  SQL reais. Confere raiz, histórico de jobs, entrada atual de comando e layer;
  retorno público não expõe documento. Depois de `Release/Join`, próxima
  execução perde proveniência do job sem esperar reconciliação. A cadência de
  manutenção é pausada depois da claim viva para instalar o binding do fixture
  sem concorrência SQL; este teste não substitui os testes de heartbeat vivo.
- `envelope_resolution_provenance_test.go`: 15→16 aceito com histórico conferido
  no handler e no ledger; 16→17, repetição ancestral e repetição do comando
  atual recusados sem invocação persistida/handler. Prova também divergência
  host/resolvedor, origem alterada entre prepare/admit e preservação da origem
  do host quando o resolvedor não acrescenta fonte.
- `host_layer_provenance_test.go`, `layer_provenance_test.go` e testes de
  projeção/helper: equivalência versus invalidação, restauração, cópias
  profundas, escopo selecionado, fonte adulterada e cadeias incompatíveis.
- `provenance_redaction_test.go`: histórico estrutural persistido sem segredos,
  campos desconhecidos recusados, versão inválida redigida e `redacted:false`
  da entrada não se propaga para a saída.

### Pendências de aceite, sem mudança de escopo

R03.4 ainda exige a composição produtiva **comando → novo job → evento →
comando**, além de qualificação multiorigem simultânea no App. A prova desta
rodada vai de job a comando; os testes de recuperação/múltiplos workspaces da
seção 31 e os testes de limite do pipeline não são renomeados como teste desse
ciclo completo. Queda abrupta de processo não foi executada: recuperação tem
evidência transacional e restart de componentes, conforme seções anteriores.
R03.2 mantém hotkey real e raiz `internal_event` autenticada pendentes.

Sem Wails, executável diagnóstico customizado, alteração do banco pessoal,
teste físico novo, commit ou push. A regressão global ACP anterior continua
pendente; os testes locais abaixo não são apresentados como `go test ./...`.

### Validação consolidada

`go test ./internal/app ./internal/commandexecution ./internal/commandledger
./internal/commandbindings ./internal/commandjobactivation ./internal/commandjobevents
./internal/jobs ./internal/commandmaintenance ./internal/commandruntime
-count=1 -timeout=180s`: **PASS nos nove pacotes**. Tempos reportados: App
186,457 s; execution 10,257 s; ledger 2,965 s; bindings 1,148 s; jobactivation
19,969 s; jobevents 0,582 s; jobs 14,733 s; maintenance 4,680 s; runtime 0,361 s.
Benchmark opt-in de latência não executado, sem alegação de medição nova.
`go vet` dos mesmos pacotes e `git diff --check` do escopo: **PASS**.
Teste de limite/repetição do pipeline, com asserção exata `ErrDenied`,
`-count=3`: **PASS 7,733 s**. Revisão paralela final não identificou defeito
concreto remanescente neste recorte; não substitui o Bugbot pré-push.
Após acrescentar asserções de `_source`, `_source_job_id` e `redacted:true`
na integração, testes focais do App/helper: **PASS 21,095 s**.

## 33. Ingressos autenticados, cadeias e fontes simultâneas — 17/09/2026

Base: HEAD `7e88945ec585e4352c4548aad8cd14e4db0134c1` e alterações locais,
sem commit novo. Implementação/testes divididos entre seis agentes Luna,
com revisão e integração pelo agente principal.

### Alterações e rastreabilidade

- R03.2: `App.wireTaskListDomainEvents` conecta o `tasklist.Service` ao
  `Manager.TasklistDomainEventSink`. Somente nomes do catálogo de tasklist
  são aceitos. Uma nova raiz interna exige usuário explícito e prova viva
  da sessão local; identificador e cadeia são criados no backend.
- O sink remove campos reservados de todas as branches do payload. Contexto
  público `eventctx` não se torna autoridade privada; marcadores existentes
  mantêm a origem sem promover fontes unknown/foreign. Custom actions
  continuam pelo ingresso genérico, sem ganhar essa autoridade.
- A identidade da origem acompanha os jobs descendentes. O listener
  revalida a sessão e o executor compara a origem com a identidade efetiva
  capturada para `queued`. Troca de geração ou guard ausente impede outbox
  elegível; não se afirma que todo job legado será cancelado.
- R03.2: `HotkeyRegistrar` extrai o contrato já utilizado pelo Manager.
  O teste captura o callback registrado por `Manager.Start` e exercita
  executor, repository, timeline e outbox reais; `Stop` desregistra a tecla.
  Trata-se de hardware controlado no teste, não de novo aceite físico.
- R03.4: `commandcontract.DecodeCommandChainHistory` unifica a validação
  em jobs, execução e auditoria. Exige comando namespaced, UUIDv7, referências
  válidas/únicas, comandos/invocações sem repetição e profundidade máxima 16.
  O estado terminal com 16 entradas é válido, mas não admite append 17.
  Testes de repository confirmam que cadeia inválida não cria run, timeline,
  outbox ou chamada à tool. O redactor admite `_source_job_id` vazio válido,
  mas recusa `null`/tipos incorretos e não guarda payload arbitrário.
- `app_command_job_multisource_test.go`: dois jobs vivos, grants e claims
  reais; cadeias divergentes recusadas, término de um run pela outbox e
  execução pelo sobrevivente com as duas camadas equivalentes na cadeia.
- `recovery_restart_test.go`: fecha e reabre a conexão SQLite em arquivo.
  Um item claimed com lease expirada é retomado e entregue uma única vez;
  um item já entregue não volta à fila após reabertura. O teste altera apenas
  o prazo da lease para controlar o tempo; não reverte `delivered`, não
  inventa fingerprint e não equivale a matar o processo fisicamente.

### Dependência restante de R03.4

O catálogo produtivo ainda não tem `job.run`. O ciclo completo
**comando → novo job → evento → comando** depende da delegação segura de
R05.2 e da migração de catálogo de R07. Não foi adicionado handler sentinela,
sem efeito ou exclusivo de teste para encerrar esse critério. Recuperação,
limites e múltiplas fontes acima não são apresentados como esse ciclo.

Sem Wails, binários diagnósticos customizados, alteração do banco pessoal,
commit ou push. Testes Go normais utilizam seus executáveis temporários
usuais; não se afirma ausência de binários gerados pelo próprio `go test`.

### Evidências dos ingressos e revisão

- `TestTasklistServiceSinkCreatesInternalEventJobOriginEndToEnd`: Service e
  DBStore reais, wiring do App, Manager/EventBus, job bloqueado enquanto a
  claim/lease é observada, origem e owner conferidos, timeline/outbox
  queued/started/completed. O fixture usa `Job.DatabaseID`, não o slug.
- `TestTasklistServicePublicProvenanceDoesNotMintInternalAuthority`: mutação
  real com `eventctx` público executa o job legado com origem unknown, sem
  outbox elegível. Testes do sink cobrem campos reservados adulterados.
- `TestTasklistServiceRevokedSessionDoesNotPublishAuthenticatedEvent`:
  revogação durável no banco temporário impede publicar/disparar, mas mantém
  a mutação de domínio conforme a política best-effort.
- `command_domain_epoch_test.go`: troca A→B entre listener e captura do run
  mantém execução legada sem autoridade de ativação; herança preserva o
  guard do contexto intermediário. Essas duas provas são de boundary/unit,
  não são chamadas de clique UI ou de cadeia completa comando→job.
- Cron/interval seguem qualificados pelo Manager/Scheduler; a matriz comum
  de estados do executor cobre retry_scheduled/completed/failed/skipped.
  `when=false` no callback hotkey não cria run, em vez de inventar uma
  timeline skipped inexistente.

Revisão paralela final não encontrou falha concreta remanescente neste
recorte. Foram corrigidos durante a revisão guard ausente, preservação de
sessão nos descendentes, sanitização das branches de origem existente e
redação de `_source_job_id` vazio/null. Não substitui Bugbot pré-push.

### Validação consolidada

`go test ./internal/jobs ./internal/commandjobactivation
./internal/commandcontract ./internal/commandexecution ./internal/commandledger
./internal/tasklist ./internal/commandjobevents ./internal/commandbindings
./internal/commandmaintenance -count=1 -timeout=150s`: **PASS nos nove pacotes**.
Tempos: jobs 13,654 s; jobactivation 21,615 s; contract 0,599 s;
execution 10,755 s; ledger 3,708 s; tasklist 0,735 s; jobevents 0,527 s;
bindings 0,580 s; maintenance 4,290 s. Benchmark opt-in de latência pulado,
sem alegação de nova medição de desempenho.

Integrações `TestTasklistService*`: **PASS 19,757 s**; multiorigem no App:
**PASS 18,824 s**. `go vet` de App/jobs/contract/execution/ledger/
jobactivation/tasklist e `git diff --check` do escopo: **PASS**. Checagem
global de whitespace ainda aponta espaços no binding gerado preexistente
`frontend/wailsjs/go/models.ts`, não alterado nesta rodada.

Regressão completa `go test ./internal/app -count=1 -timeout=300s`:
**PASS 87,076 s**, após corrigir a referência slug/DatabaseID no novo fixture.
Não foi executado `go test ./...`; a pendência global ACP anterior não é
declarada resolvida por estas suítes.

Repetição focal dos ingressos/epoch/proveniência de jobs com `-count=3`:
o pacote reportou **PASS 2,782 s**, porém o comando terminou com **exit 1**
no cleanup: `go: unlinkat .../go-build1805667531/b001/jobs.test.exe: The process
cannot access the file because it is being used by another process.` Não
houve remoção forçada, contorno do antivírus ou atribuição causal sem prova.
A regressão completa de jobs anterior teve exit 0.

**Aceite:** R03.2 fechado; **11/48 saídas R, 1/12 gates (R04), 0/83 critérios
finais C, 53/84 históricos I**. R03.4/Gate R03 continuam abertos pela
dependência descrita acima; próximo trabalho é R05.2 com a composição real
de delegação, antes de certificar o ciclo completo.

## 34. Ponte de delegação para jobs e identidade de tools — 17/09/2026

Base: HEAD `7e88945ec585e4352c4548aad8cd14e4db0134c1` mais alterações
locais, sem commit novo. **R05.2 permanece parcial**: esta seção qualifica
a ponte de runtime, não publica `job.run` na paleta nem fecha sua composição
de autorização no App. Contagem mantida: **11/48 saídas, 1/12 gates, 0/83 C,
53/84 históricos I**.

### Implementação

- `jobs.PrepareCommandJob`: leitura autoritativa por UUID físico e owner
  explícito, sem fallback para usuário do Manager nem resolução ambígua por
  slug. Snapshot independente preserva `DatabaseID` e não executa/habilita job.
- `jobs.DefinitionFingerprint`: versão canônica JCS/SHA-256 com domínio
  `assistente.job-definition.v1` para comparar o conteúdo executável. Não é
  credencial, grant nem o fingerprint de delegação cross-profile da AEP-0101.
  Entradas, tool, pipeline/enable, saídas/eventos, limites/retry e dry-run
  participam; estado de execução e metadata de auditoria não participam.
- `Manager.CommandHandler`: handler tipado `job`, com alvo fixado pelo
  bootstrap e autorização obrigatória. Por poder executar tools arbitrárias,
  exige contrato destrutivo/interativo e alvo mutável; não é publicado como
  leitura ou escrita inofensiva para passar no catálogo.
- O alvo contém UUID/slug/versão de definição. O grupo `job_*` do envelope
  continua reservado à origem com run existente; não se inventa RunID antes
  da execução nem se relaxa esse contrato. Trocar alvo exige remontagem e
  publicação da versão do catálogo/handler pelo host.
- `Start` entrega handle sem SQL/trabalho sob DispatchGate. O worker relê
  definição/owner, revalida autorização antes de queued e a cada tentativa.
  Troca de sessão antes da fila recusa o run, em vez de executar o comando
  como origem unknown. Cancelamento ou terminal não comprovado retorna
  `outcome_unknown`, nunca sucesso presumido.
- O executor de comandos inaugura a cadeia de uma delegação direta antes da
  reserva, com invocation ID e histórico de jobs vazio. O runtime cria seu
  próprio run e preserva a cadeia na proveniência; o resultado do handler
  contém apenas job ID, run ID e estado, sem input/output/erro bruto da tool.
- Paths sensíveis da tool passam pelo executor comum. A autorização/política
  do primeiro job não é herdada por outro job que receba um evento; a raiz e
  as cadeias continuam preservadas separadamente.
- `commandtoolbridge` conserva conversa/turno/superfície e profile-pai,
  recusando metadados parciais/incoerentes; não usa profile alvo como pai e
  não fabrica decisão/grant a partir de campos do envelope.

### Pendências finitas para a composição de R05.2

1. Montar no App as rotas e a autorização final reais, incluindo presenter,
   leitura exata de grants/profile e snapshot do alvo, sem callbacks de teste.
2. Transportar a raiz verificada completa no retorno camada → comando → job.
   Esta ponte aceita apenas a raiz direta inaugurada pelo executor; cadeia
   herdada é recusada, não reiniciada. R03.4 permanece aberto.
3. Qualificar entradas agent/job_service e o catálogo parametrizado de R07.
   A ponte de jobs desta rodada aceita usuário local; não há fallback de ator.

Essas pendências já pertencem a R05.2/R03.4/R07. Não são novos pacotes nem
aceite do catálogo de produto. Sem Wails, executáveis diagnósticos próprios,
alteração de banco pessoal, commit ou push.

### Evidência e revisão da rodada

- O teste integrado atravessa sessão local, catálogo destrutivo/interativo,
  FactBus, DecisionStore, executor de comandos, Manager, SQLite e
  ToolInvocationService reais. Presenter e autorização do alvo são portas
  controladas do teste, não composição de produto. Exige um receipt consumido,
  um run, uma invocação técnica e cadeia; replay não repete decisão nem efeito.
- Esse teste encontrou e corrigiu um defeito do pipeline: o receipt era
  consumido no CAS para queued, mas seu ID não chegava ao handler. Agora o
  ID derivado é copiado somente após esse consumo, sem alterar o envelope
  pré-autorização/fingerprint. Rejeição não inicia handler; NoDecision
  descarta ID injetado pelo snapshot.
- Matriz do handler: contrato Read/Write recusado; owner/origem/definição
  inválidos; revogação antes da fila e durante retry; origem privada não
  reiniciada; paths sensíveis no ledger real; filho sem herdar a política do
  pai. Indisponibilidade de leitura conserva categoria `unavailable`, sem
  retry automático nem exposição do erro SQL no resultado do comando.
- A revisão corrigiu verificação de cancelamento após leitura durável e
  revalidação explícita de sessão/epoch em cada tentativa. Tests de versão
  mantêm o mesmo UUID ao variar conteúdo, evitando falso positivo no digest.
- Falha reproduzida em `TestCommandJobClaimRefreshesPaletteResolutionAfterRuntimeTerminal`:
  manutenção avançava a revisão da claim durante a admissão, produzindo
  `rejected_stale` corretamente. O teste agora drena a manutenção antes de
  instalar seu delta; continua exigindo refresh sem rebuild e retirada da
  camada após terminal sem Consumer. Sem relaxar assert ou retry cego.

Validação final (17/09/2026; comandos Go normais, exit 0):

- `go test ./internal/jobs ./internal/commandexecution ./internal/commandtoolbridge ./internal/toolinvocations ./internal/commandidentity ./internal/commandcatalog -count=1 -timeout=150s`:
  **PASS** nos seis pacotes (jobs 13,525 s; commandexecution 6,267 s).
- `go test ./internal/commandjobactivation -count=1 -timeout=150s`:
  **PASS**, 67,056 s, na bateria anterior da mesma rodada.
- `go test ./internal/app -run '^TestCommandJobClaimRefreshesPaletteResolutionAfterRuntimeTerminal$' -count=5 -timeout=90s`:
  **PASS**, 24,037 s, após sincronização do teste.
- `go test ./internal/app -count=1 -timeout=180s`: **PASS**, 84,139 s,
  após a correção do receipt e a sincronização do teste.
- `go vet` dos seis pacotes alterados e `internal/app`: **PASS**.
- `git diff --check` limitado aos arquivos tracked desta rodada: **PASS**.

Não foi executado `go test ./...`, benchmark opt-in de latência nem validação
manual/Wails/hardware. Nenhum novo gate aceito: **11/48 R, 1/12 gates,
0/83 C e 53/84 históricos I**. O próximo fechamento depende da composição
real listada acima, não de repetir esta bateria ou criar novos pacotes.

## 35. Decisão desktop e autorização de job pelo App — 17/09/2026

Base: mesmo HEAD mais alterações locais, sem commit/push. Trabalho de R05.2,
sem criar novos pacotes nem fechar gate por evidência parcial.

- A fábrica desktop usada pelo produto deixa de recusar todo catálogo
  interativo. Compartilha com a fábrica completa o vínculo ao
  `commandDecisionPresenter`/QuestionnaireManager reais e ao mesmo SQLite
  do command ledger. Store externo, presenter ausente, tabelas ausentes,
  TTL/body ausentes e banco divergente não habilitam confirmação.
- `App.newCommandJobHandler` monta uma rota de alvo fixo com autorização
  própria do App: owner explícito, sessão revalidada, Manager capturado e
  definição imutável. Não recebe callback `Authorize` de quem monta a rota.
- Para `subagent` com profile literal explícito, valida profile/provider,
  captura fingerprint/geração e exige `HasValidGeneration` no préflight,
  antes da fila e em cada tentativa. Regrant posterior não torna válido o
  handler antigo. A decisão do comando não concede nem substitui grant.
- Profile omitido/vazio mantém a herança existente da AEP-0101. Templates
  dinâmicos são recusados nesta montagem fixa até existir preparação do alvo
  resolvido; isso não altera o runtime legado de jobs.

Limite de produto: a fábrica é interna e ainda não publica `job.run` na
paleta. Falta conectar a preparação parametrizada ao catálogo R07 e completar
raiz reativa/agent/job_service. O catálogo produtivo atual não é ampliado por
este trabalho. **11/48 R, 1/12 gates, 0/83 C, 53/84 históricos I**.

Restrição operacional: testes de ACP/acpregistry e `go test ./...` não serão
executados nesta máquina. Não alterar diretório/nome de binário para contornar
o antivírus. Testes dirigidos aos outros pacotes permanecem permitidos.

Cobertura acrescentada nesta rodada:

- Confirmação desktop com receipt consumido entregue ao handler; recusa,
  cancelamento e sessão revogada durante o diálogo não iniciam o efeito.
- Montagem recusa TTL/body/presenter ausentes, store de decisão externo e
  ledger em banco diferente.
- Handler do App usa owner/sessão reais; grant ausente e grant revogado e
  reconcedido não autorizam o handler antigo, mesmo preservando a definição.
- Fluxo integrado desktop → decisão real → runtime real de job → resultado
  durável → replay sem segunda execução. Catálogo e tool de efeito são
  controlados pelo teste; não equivalem à publicação de `job.run` no produto.
- O teste integrado republica pelo lifecycle real após salvar o job: a escrita
  invalida a projeção anterior e o guard deve continuar recusando-a.

Validação final desta rodada (17/09/2026):

- `go test ./internal/app -count=1 -timeout=180s`: **PASS**, 106,516 s
  (saída JSON filtrada apenas para apresentação).
- `go test ./internal/jobs ./internal/commandexecution ./internal/jobprofilegrant ./internal/commandidentity -count=1 -timeout=150s`:
  **PASS** nos quatro pacotes (14,224 s; 8,248 s; 0,910 s; 0,933 s).
- `go vet ./internal/app ./internal/jobs ./internal/commandexecution ./internal/jobprofilegrant ./internal/commandidentity`:
  **PASS**, sem desabilitar verificações.
- `git diff --check` dos arquivos tracked desta rodada: **PASS**.

Nenhum gate adicional aceito: **11/48 R, 1/12 gates, 0/83 C e 53/84 I**.

## 36. Delegação reativa preserva a raiz verificada — 17/09/2026

Base: mesmo HEAD mais alterações locais, sem commit/push. Continuação de
R05.2/R03.4, sem ampliar as origens admitidas nem publicar `job.run`.

- `ProjectionClaim` transporta raiz type/ID do fato verificado na outbox,
  nunca do JSON editável da claim. Esses campos permanecem no backend.
- `App.resolveCommandJobOrigin` relê a projeção autorizada para a sessão e
  workspace exatos; compara as cadeias das camadas selecionadas com a cadeia
  do envelope, admitindo somente a entrada atual acrescentada pelo executor.
  Raízes divergentes, fonte encerrada, prova ausente e cadeia adulterada
  recusam a delegação, sem fallback para uma raiz manual.
- `Manager.CommandHandler` preserva raiz e históricos na entrada herdada,
  revalidando a fonte antes das tentativas. O runtime continua sendo o único
  que acrescenta slugs à cadeia de jobs e executa DetectLoop/MaxChainDepth.
- A autorização do handler pertence ao job e estágio exatos da cadeia;
  eventos/jobs descendentes não herdam seu dispatch, mesmo com o mesmo alvo.
- Teste integrado usa job vivo, outbox, claim, binding SQL, projeção,
  confirmação desktop e executor de jobs reais. Catálogo, resolver de teste
  sobre a configuração real e tool de efeito são controlados; a cadência é
  pausada durante essa montagem. Isso não certifica publicação R07 nem o
  ciclo automático inteiro com consumer concorrente.

R05.2/R03.4 continuam parciais: catálogo parametrizado, demais identidades,
qualificação do ciclo completo/crash/replay e gates correspondentes não
foram substituídos por esse teste. **11/48 R, 1/12 gates, 0/83 C, 53/84 I**.

Validação desta rodada (17/09/2026):

- `go test ./internal/app -run '^TestCommandJobDesktopDecision' -count=3 -timeout=90s`:
  **PASS**, 31,760 s. Fluxos direto, reativo e fonte encerrada durante a
  confirmação passam três vezes; o último persiste `cancelled_stale` sem run
  filho nem efeito da tool.
- `go test ./internal/app ./internal/jobs ./internal/commandjobactivation ./internal/commandexecution ./internal/commandjobevents ./internal/commandcontract -count=1 -timeout=180s`:
  **PASS** nos seis pacotes (App 115,254 s; jobs 8,421 s; activation 25,025 s;
  execution 11,837 s; events 0,581 s; contract 1,566 s).
- Após reforçar o cenário de descendente com o mesmo job real na cadeia,
  `go test ./internal/jobs -count=1 -timeout=90s`: **PASS**, 6,194 s. O
  circuito recusa o loop sem efeito e sem consultar o dispatch do pai.
- `go vet` dos mesmos seis pacotes: **PASS**.
- Não foram executados ACP/acpregistry, `go test ./...`, race, benchmark
  opt-in, Wails ou teste físico. Nenhum gate foi fechado por teste pulado.

## 37. Delegação local de tools pelo executor comum — 17/09/2026

Base: mesmo HEAD mais alterações locais, sem commit/push. Continuação de
R05.2; não é publicação de comandos no catálogo R07.

- `App.newCommandToolHandler` monta alvo fixo pelo ID canônico do catálogo,
  com owner/sessão locais, schema e geração do registro fixados. Confere que
  serviço, ledger e registry pertencem ao mesmo bootstrap/banco.
- O worker usa `ToolInvocationService` real: persiste e marca a invocação,
  então revalida sessão, catálogo, disponibilidade e identidade antes do
  efeito. Erro/panic do guard não vaza sua mensagem no ledger.
- O executor captura tool e geração no mesmo lock. Remover e registrar outra
  tool com o mesmo nome não substitui silenciosamente o alvo autorizado.
  Publicação de tool opt-in e suas flags também ocorre no mesmo lock.
- A ponte copia o envelope antes do worker; redação obrigatória do comando
  chega ao ledger comum, mantendo argumentos e resposta efêmera completos.
- Teste integrado percorre confirmação desktop, executor de comandos,
  factory App, ledger e executor comum reais em banco temporário. Tool de
  efeito e catálogo do comando são controlados. Replay não repete execução.
- Recusa/cancelamento do diálogo: `denied`, nenhuma tool invocation.
  Sessão revogada: `cancelled_stale`, nenhuma tool invocation. Substituição,
  indisponibilidade ou mudança de schema durante o diálogo: `failed` com
  `execution_guard_denied`, uma invocação terminal e nenhum efeito.

Limites explícitos: a factory é interna, sem caller produtivo publicado.
Não admite cadeia reativa, identidade agent/job, delegação de profiles nem
as tools especiais `subagent`/`job`. Políticas próprias das tools continuam
obrigatórias; não há segunda allowlist de shell. A revalidação é uma checagem
antes do efeito, não uma transação que torne o efeito externo e futuras
mutações do catálogo atômicos. Falha simultânea de finalização e cleanup do
ledger ainda depende da recuperação pendente; não se certifica crash aqui.

**R05.2 continua parcial: 11/48 R, 1/12 gates, 0/83 C, 53/84 I**.
Próximo fechamento: demais identidades/delegações de R05.2, seguido da
publicação e qualificação do ciclo real; esta prova não substitui esses gates.

Validação desta rodada:

- Regressão `go test ./internal/app ./internal/tools ./internal/toolinvocations ./internal/commandtoolbridge ./internal/jobs ./internal/commandexecution -count=1 -timeout=180s`:
  **PASS** (App 132,249 s; tools 9,442 s; toolinvocations 1,423 s;
  bridge 3,875 s; jobs 15,339 s; execution 10,773 s).
- `go vet` dos mesmos seis pacotes: **PASS**.
- Após os reforços: tools **1,453 s**, toolinvocations **1,186 s**, bridge
  **0,651 s**, jobs **15,068 s**, execution **11,366 s**: todos **PASS**.
  App completo repetido: **PASS**, 117,920 s; `go vet` final: **PASS**.
- Uma execução intermediária do App falhou no cenário de condição de job
  na paleta; isolado repetido três vezes passou (8,541 s). Revisão identificou
  manutenção concorrente durante a resolução de um teste que não pretende
  medir essa disputa. O fixture agora drena a manutenção antes do delta,
  como o teste vizinho, sem alterar guard nem assertivas de produção.
- O teste novo de serviço trocado também apresentou uma falha intermediária
  não reproduzida; mutação do fixture foi sincronizada com `authMu`, sem
  alegar causa comprovada. Foco `TestNewCommandToolHandler` passou três vezes;
  não se converte falha transitória em aceite de recuperação/crash.
- Validação final após estabilização dos arquivos:
  `go test ./internal/app -run '^Test(CommandJobConditionGuardRefreshesPaletteResolution|CommandToolDesktopDecisionAndRevalidation|NewCommandToolHandler)' -count=3 -timeout=120s`:
  **PASS**, 53,873 s. Todos os novos cenários e a preparação corrigida do
  teste da paleta passaram três vezes. `git diff --check` e gofmt limpos.
- Nenhum ACP/acpregistry, Wails, executável avulso, teste físico ou `go test ./...`.

## 38. Origem de comandos nas tools e eventos descendentes — 17/09/2026

Continuação de R05.2/R03.4 no mesmo worktree. A montagem é interna e não
publica comandos parametrizados nem substitui as identidades ainda pendentes.

- O executor inaugura `command_chain_history` também para tools locais,
  antes de reservar o comando. Cadeias reativas mantêm a origem e os limites
  existentes; tools não acrescentam slugs ao histórico de jobs.
- `Manager.CommandToolContext` instala a origem privada usada pelo sink de
  tasklist e pelo runtime descendente, sem criar run ou dispatch de job.
  Envelope, sessão, owner e raiz são vinculados à invocação exata.
- `ValidateCommandToolContext` relê a autoridade capturada. O App exige essa
  revalidação no guard posterior à persistência, além das verificações de
  catálogo/schema/geração. Encerrar a fonte reativa não autoriza efeito.
- `PrepareContext` roda no worker, nunca no gate curto de atalhos. A ponte
  preserva cancelamento, deadline, owner e metadados; libera os recursos de
  preparação também em falha, sem enviar dois resultados.
- Uma mutação de tasklist produzida pela tool usa a origem privada já
  existente; não recebe uma raiz `internal_event` nova como se fosse humana.

Sem novo aceite: **11/48 R, 1/12 gates, 0/83 C, 53/84 I**.
Permanecem abertos identidade agent/job_service, delegações de profiles,
catálogo parametrizado e qualificação do ciclo automático completo/crash.

Provas integradas: comando direto → tool → `tasklist.Service` → sink → job,
e job fonte → claim/outbox → binding → comando → tool → tasklist → job.
O segundo verifica raiz igual à do job original, os dois slugs na cadeia de
jobs e o comando na cadeia separada. Replay não duplica tool, lista nem run.
Fonte encerrada durante a confirmação termina `cancelled_stale`, sem efeito.

Os testes usam sessão, presenter/receipt, banco, resolvedor sobre configuração
SQL, execução e sink reais, mas catálogo controlado e tool wrapper do Service.
A manutenção é drenada depois da ativação da fonte para montar esse catálogo;
isso não é a prova de ciclo automático concorrente nem de crash/restart.

Validação desta rodada:

- Execução, ponte e jobs: `go test ./internal/commandexecution ./internal/commandtoolbridge ./internal/jobs -count=1 -timeout=150s`: **PASS**.
- Cenário integrado reativo completo: **PASS**, 15,509 s.
- Análise estática dos sete pacotes envolvidos: **PASS**.
- Regressão consolidada:
  `go test ./internal/app ./internal/commandexecution ./internal/commandtoolbridge ./internal/jobs ./internal/toolinvocations ./internal/commandjobactivation ./internal/commandledger -count=1 -timeout=180s`:
  **PASS** nos sete pacotes (App 108,335 s; execution 6,883 s; bridge 0,891 s;
  jobs 6,732 s; toolinvocations 0,850 s; activation 15,951 s; ledger 6,218 s).
  `git diff --check` e gofmt limpos. Sem ACP/acpregistry, Wails, binário
  avulso, teste físico, benchmark opt-in ou `go test ./...`.
- Matriz final `go test ./internal/app -run '^TestCommandTool' -count=3 -timeout=120s`:
  **PASS**, 34,914 s. Inclui os nove cenários desktop e os dois percursos
  reais de tasklist/evento/job (direto e reativo), repetidos três vezes.
- Falhas durante montagem dos testes foram corrigidas: schema opcional
  explícito (`Optional`, não apenas ausência de `Required`), leitura da
  proveniência na auditoria SQL e ordem de drenagem da manutenção. Nenhuma
  validação de produção foi retirada para aceitar o fixture.

## 39. Identidade de automação ligada ao run real — 17/09/2026

Implementado o adapter `commandidentity.JobRuntime` no Manager e o wiring de
`JobProfileGrants` no App. O executor emite um contexto privado por run;
`CommandJobServiceRequest(ctx, source)` deriva dele owner, job, run, profile
e capability, sem receber esses dados como autoridade do chamador.

- A definição executada é comparada à persistida antes de emitir o marker;
  revalidação compara novamente seu fingerprint e a geração exata do grant.
- Capabilities são distintas por emissão. Copiar IDs ou criar outra
  capability não substitui o contexto privado do mesmo Manager/run.
- Todo job filho perde o marker ancestral. Cancelamento original e término
  invalidam o lifetime privado, inclusive para contextos sem cancelamento.
- Falha de preparação passa pela finalização persistida; não abandona queued.
- Resolve/Revalidate não concedem grants nem escrevem epochs de grants.
- Recorte inicial: job `subagent` com profile literal; templates, profile
  herdado e jobs sem runtime/grants configurados não recebem essa identidade.
  A ausência do adapter não elimina a execução anterior desses jobs.

Não habilita catálogo parametrizado, origem agent ou delegação interativa.
Contagens mantidas: **11/48 R, 1/12 gates,
0/83 C, 53/84 I**.

Provas: teste do Manager executa uma tool probe pelo serviço comum e consulta
o adapter real; `query_only` cobre Resolve/Revalidate/Authorize e a autorização
final é exercitada com o DispatchGate adquirido. Regrant restaura a definição
original e demonstra que o grant novo é válido, isolando a recusa da geração
antiga. O App usa `initJobs`, sessão, manutenção, store e runtime reais, com
tool probe no lugar de invocar um LLM externo. Não se certifica ainda o
percurso de comando de automação pelo catálogo e pelo handler final.

Validações:

- Matriz nova App/jobs, repetida três vezes: **PASS** (App 4,054 s;
  jobs 1,603 s), incluindo fronteira de profiles literais, preparação,
  cancelamento, finalização, identidade e revogação.
- Seis pacotes de infraestrutura: **PASS** (`jobs`, `commandidentity`,
  `jobprofilegrant`, `commandexecution`, `toolinvocations`, `commandtoolbridge`).
- `go vet` dos seis pacotes e de `internal/app`: **PASS**.
- Regressão final dos sete pacotes, `-count=1 -timeout=180s`: **PASS**
  (App 101,800 s; jobs 14,182 s; identity 0,827 s; grants 0,595 s;
  execution 7,102 s; toolinvocations 0,986 s; bridge 0,643 s).
- Falhas intermediárias de fixtures corrigidas: catálogo SQL da tool ausente,
  dependências de `initJobs` incompletas e manutenção não montada no App.
  Nenhuma checagem produtiva foi retirada para aprovar os testes.
- `git diff --check` do escopo alterado: limpo; verificação global apontou
  whitespace em `frontend/wailsjs/go/models.ts`, não editado nesta rodada.
  Sem regenerar bindings ou modificar esse arquivo gerado.
- Sem ACP/acpregistry, Wails, binários avulsos ou testes físicos.

## 40. Importação no applier do App e referências reais do destino — 17/09/2026

`commandMutationApplier.ImportEnvelope` compartilha com `Apply` o pipeline
de autenticação, confirmação, auditoria, suspensão e reconstrução do mapa.
Não existe segundo writer. `Committed` e `Rebuilt` continuam distinguindo
falha anterior ao commit de falha na publicação posterior.

- Owner do contexto é substituído pelo principal autenticado do token.
- Posse de UUIDs é consultada em allowlist fixa de camadas, bindings e regras;
  UUID em outro workspace não é tratado como pertencente ao escopo solicitado.
- Conflitos de nome usam a chave usuário+escopo+nome: nomes estrangeiros
  não são revelados nem impedem a importação.
- Credenciais usam igualdade literal do pattern e metadados do banco;
  wildcard no pattern não vira busca aproximada. Não lê/decripta segredos,
  não reaproveita credenciais de outro usuário e recusa patterns gerenciados.
- As portas SQL de nome, posse e credenciais são montadas pelo applier;
  catálogo, triggers e autorização de workspace continuam portas do host.
- Testes com store real ligam resultado do plano à persistência para
  credencial presente, ausente e estrangeira. UUID estrangeiro no lote recusa
  a operação inteira antes da decisão, sem alterar a camada local.

R05.3/R05.4 continuam parciais. Ainda faltam transporte público/UX,
atomicidade multi-escopo e export sensível com decisão e criptografia.
O envelope continua restrito a um escopo e 64 KiB. Não se certifica backup
de comandos no produto nem se fecha um R inteiro apenas por essa composição.
Contagens mantidas: **11/48 R, 1/12 gates, 0/83 C, 53/84 I**.

Validações da seção 40:

- Regressão final `go test ./internal/app ./internal/commandportability ./internal/portability ./internal/commandconfig -count=1 -timeout=180s`:
  **PASS** nos quatro pacotes (121,885 s; 2,631 s; 7,303 s; 9,006 s).
- `TestAppCommandPortability*`, repetidos três vezes: **PASS**, 27,401 s.
  Sessão real, confirmação via questionnaire, persistência do binding,
  equivalência do mapa publicado com a projeção do banco, Keep sem alteração
  e logout durante a decisão sem commit/rebuild; snapshot permanece intacto.
- `go test ./internal/commandportability -count=3 -timeout=120s`: **PASS**,
  6,432 s; inclui metadados, pattern literal, isolamento, duplicidade legada,
  cancelamento, falha de banco e importação com referências reais.
- Regressão `commandportability`, `portability`, `commandconfig`: **PASS**
  (1,594 s; 11,303 s; 9,323 s).
- `go vet` dos três pacotes e `internal/app`: **PASS**. Gofmt e diff check
  dos arquivos versionados editados na rodada: limpos.
- Falhas intermediárias de fixture corrigidas: origem/escopo incompatíveis
  no catálogo de teste, sessão nova não publicada e uso de access token em
  vez de refresh token no logout. Cleanup cancela antes de aguardar workers;
  nenhuma checagem produtiva foi retirada.
- Sem ACP/acpregistry, Wails, executáveis avulsos ou acesso ao banco pessoal.

## 41. Commit atômico de importação global+workspaces — 17/09/2026

Implementado o percurso interno `ApplyCommandEnvelopeBatch` →
`ApplyPlanImportBatch` → `CompleteMutationService.ImportBatch` → writer
compartilhado de configuração. O consumo unitário e o consumo em lote dos
receipts usam o mesmo núcleo transacional, sem esquema ou writer paralelo.

- Até 64 escopos exatos, sem duplicatas, com owner derivado da sessão.
  Todos são autorizados antes de carregar a primeira configuração.
- Payload/plano congelados; Copy mantém IDs coerentes dentro do lote;
  referências são resolvidas novamente no gate final.
- Previews individuais e união final global+workspace são validados.
  Keep sem alterações não cria decisão, geração ou suspensão.
- Cada escopo alterado conserva sua confirmação e auditoria próprias;
  todas as confirmações acontecem antes de qualquer alteração. Recusa,
  expiração ou revogação impedem o lote inteiro. O prazo é único para o lote.
- Uma transação consome todos os receipts e grava os CAS, dados e auditorias.
  Workspaces vêm antes do global para não invalidar suas baselines herdadas.
  Falha no último hook desfaz inclusive os efeitos dos primeiros escopos.
- Ao final, dados e gerações exatos de cada escopo são conferidos novamente:
  um hook posterior não pode alterar silenciosamente um escopo anterior.
- Timestamps de camadas importadas são definidos pelo destino antes da
  confirmação, preservando criação existente e canonizando a ordem das linhas.
  O ORM não pode acrescentar metadados diferentes do preview assinado.

Saída concreta: atomicidade multi-escopo disponível no backend/envelope
interno. O applier do App da seção 40 continua unitário; ainda faltam sua
composição multi-escopo com reconstrução única, transporte/UX pública,
relatório público e export sensível com decisão/criptografia. O limite JSON
permanece 64 KiB. Não se anuncia backup de comandos disponível no produto.
Contagens mantidas: **11/48 R, 1/12 gates, 0/83 C, 53/84 I**.

Provas adicionadas: autorização de todos os escopos antes de leitura,
recusa da segunda decisão, referência stale, união final inválida, rollback
do último hook, hook que adultera escopo anterior sem retornar erro,
replay, expiração, isolamento, Copy com binding remapeado e Keep sem efeito.
O teste do envelope usa portas reais de posse/nome e remapeamento de workspace.
Sessões/presenters da matriz de lote são fixtures, não testes manuais do App.

Validações da seção 41:

- Regressão completa `commandconfig`, `commanddecision`, `commandportability`,
  `portability` e `app`, `-count=1 -timeout=180s`: **PASS** nos cinco pacotes
  (17,420 s; 9,623 s; 6,229 s; 14,578 s; 138,423 s).
- Matriz `-run Batch -count=3 -timeout=120s`: **PASS** nos quatro pacotes
  de infraestrutura (7,000 s; 1,853 s; 1,408 s; 1,710 s). Inclui tentativa
  de alterar a geração de workspace no último hook, sem erro explícito,
  recusada com rollback de configuração, gerações, auditoria e receipts.
- `go vet` dos cinco pacotes: **PASS**; gofmt e diff check do escopo limpos.

- Ajustes de fixture durante a rodada: owner do hook precisa refletir o
  workspace; Load global não inclui workspace; Copy preserva a camada
  preexistente. Timestamps e ordem foram corrigidos no código produtivo,
  sem retirar a conferência final para fazer os testes passarem.
- Sem ACP/acpregistry, Wails, executáveis avulsos, migração do banco pessoal,
  alteração de bindings gerados ou teste físico.

## 42. Lote no applier do App e publicação única — 17/09/2026

`commandMutationApplier.ImportEnvelopeBatch` compõe o lote com sessão real,
portas SQL de posse/nome/credenciais, decisão e commit atômico existentes.
As mutações unitárias e o lote compartilham o mesmo caminho pós-commit.

- A reconstrução usa a união global+workspace **ativo**, não o último alvo
  importado. Não publica um mapa intermediário por escopo.
- Cada diff deve pertencer à sessão, à operação e a um escopo único. Sua
  auditoria deve corresponder à geração exata do escopo, não à global herdada.
- São revalidados tanto o snapshot publicado quanto os snapshots de todos
  os escopos alterados. A conferência final ocorre depois das portas de
  versão/projeção, inclusive para workspaces não ativos.
- O resultado separa `Committed` de `Rebuilt`: falha após gravação mantém
  o commit e o mapa suspenso, sem repetir a importação automaticamente.
- Keep sem alteração não confirma, não grava e não reconstrói.

Não instala entrada pública, bindings Wails, UI de importação ou relatório
público. Exportação sensível também permanece pendente. R05.3/R05.4 seguem
parciais: **11/48 R, 1/12 gates, 0/83 C, 53/84 I**. Nenhum banco pessoal ou
teste físico participa desta rodada.

Evidências da seção 42:

- `app_command_portability_batch_test.go`: confirmação real via questionnaire,
  owner derivado do access token mesmo com contexto estrangeiro, global e
  workspace persistidos juntos, recusa da segunda decisão sem commit.
  Sucesso com workspace importado ativo e não ativo; mapa equivalente à
  projeção do workspace aberto. O par de gerações do host avança exatamente
  uma publicação; Keep preserva o snapshot e versões e não cria auditorias.
- `app_command_portability_batch_rebuild_test.go`: falha de projeção após
  commit e alteração da geração de workspace não ativo durante build ou
  revalidação final. Todos recusam publicação, preservam o lote persistido,
  uma auditoria por escopo e mapa suspenso, sem repetição do commit.
- `app_command_mutation_audit_scope_test.go`: geração global herdada não
  comprova um commit do workspace; geração exata e atual é obrigatória.
- Matriz focal final de App/lote/auditoria, `-count=3 -timeout=120s`:
  **PASS**, 17,060 s. Regressão `commandconfig`, `commanddecision`,
  `commandportability`, `portability`: **PASS**, 10,297 s; 3,088 s;
  2,340 s; 6,625 s. `go vet` dos cinco pacotes: **PASS**.
- Regressão completa `go test ./internal/app -count=1 -timeout=180s`:
  **PASS**, 116,063 s. O cenário adicional de sucesso com workspace não ativo
  foi acrescentado depois do início dessa execução e validado na matriz
  focal final acima; não houve mudança produtiva posterior à regressão.
- Gofmt dos quatro arquivos Go e diff check dos arquivos versionados do
  escopo: limpos. Sem ACP/acpregistry, Wails ou executáveis avulsos.

## 43. Relatório do lote aplicado e correção do writer — 17/09/2026

O resultado interno de `ApplyPlanImportBatch` e `ApplyCommandEnvelopeBatch`
agora separa os diffs privados do `ImportReport`. O applier do App conserva
esse relatório junto de `Committed/Rebuilt`, inclusive se a publicação falhar.
Não se cria um segundo writer ou wrapper de compatibilidade.

- O relatório deriva do plano que alimentou a gravação. Importar como cópia
  não refaz o planejamento após o commit nem retorna UUIDs fictícios.
- Transporta versão, no-op, destino/escopo, escolha keep/replace/copy e
  discriminador de deltas. Não atribui conteúdo proposto como estado efetivo
  de uma camada mantida por Keep.
- Avisos do planejamento validado são agregados por código, em ordem estável,
  sem `Identifier`, que pode conter pattern privado de credencial. Não inclui
  nomes, descrições, argumentos, condições, triggers, owner ou payload do diff.
- Keep integral preserva relatório explícito; recusa/rollback não retorna
  relatório de importação aplicada. Falha pós-commit conserva o relatório.
- O teste com credencial realmente ausente no banco revelou uma falha anterior:
  `Updates(struct)` do GORM trocava `updated_at` da camada existente depois da
  confirmação. O writer compartilhado agora fornece os campos explicitamente,
  incluindo o timestamp confirmado. Mantida a conferência final rígida do lote.
  Teste verifica também timestamps persistidos iguais aos do diff confirmado.

**Limite e sequência restante:** a investigação da entrada pública confirmou
que a fábrica interna usa JWT e a fachada Wails usa sessão desktop. A montagem
produtiva de `Projection/Version/Authorize/OnMutationTx` ainda não existe para
essa entrada. Não se habilitou importação pública com callbacks permissivos.

1. Compor autenticação pela sessão desktop revalidada, autorização real de
   workspace, projeção e hook de ativação transacional no host.
2. Ligar a fachada `ExportImport` ao serviço, expondo somente resultado
   redigido e distinguindo commit concluído de mapa ainda não publicado.
3. Ligar escolha de conflitos, remapeamento e relatório à UI acessível;
   qualificar o fluxo e exportação sensível conforme AEP-0047.

R05.3/R05.4 continuam parciais. Contagens: **11/48 R, 1/12 gates, 0/83 C,
53/84 I**. Sem nova funcionalidade pública ou necessidade de teste manual nesta
rodada; bindings gerados e documentação de uso permanecem inalterados.

Evidências da seção 43:

- `import_report_test.go`: JSON restrito, ausência de conteúdo privado e de
  identifiers de warnings, isolamento das slices, ordem estável e Keep sem
  atribuir estado candidato ao destino.
- `import_batch_report_test.go`: credencial ausente resolvida no banco real,
  binding persistido desabilitado, aviso `credential_missing` sem pattern,
  substituição de camada existente e timestamps iguais ao diff confirmado.
- Matriz de lote/envelope/App reforçada: Copy informa o UUID persistido;
  rollback e recusa não retornam relatório; Keep conserva no-op; falha de
  publicação conserva o relatório do commit.
- Matriz `-run 'Batch|ImportReport' -count=3 -timeout=120s`: **PASS** em
  `commandportability` (2,110 s), `portability` (1,782 s) e `app` (6,586 s).
- Regressão completa `commandconfig`, `commanddecision`, `commandportability`
  e `portability`: **PASS** (14,361 s; 3,470 s; 6,762 s; 12,477 s).
- Regressão completa `app`, `-count=1 -timeout=180s`: **PASS**, 142,027 s.
- `go vet` dos cinco pacotes: **PASS**; gofmt e diff check do escopo limpos.

## 44. Importação desktop conectada ao aplicativo — 17/09/2026

O fluxo deixou de ser apenas um applier privado: **Configurações → Dados**
encaminha arquivos exclusivos de `resources.commandLayers` para a composição
desktop real. Reutiliza `ImportDataWithResolutions` e seus DTOs existentes;
nenhum binding gerado foi editado nesta rodada.

- Sessão local capturada e revalidada, sem JWT do frontend; troca/revogação de
  sessão e troca de produto impedem continuar com autoridade antiga.
- Política explícita manter/substituir/copiar, nomes de destino e remapeamento
  de workspace. O host confere destinos no manager real, catálogo e triggers.
- Confirmações de todos os escopos antes do único commit, com hook real de
  ativação/grants na mesma transação e revalidação transacional da sessão.
- Uma reconstrução da união ativa, incluindo prova e guard das fontes de jobs.
  Commit sem publicação preserva relatório e bloqueia reenvio pelo painel.
- Parser rejeita recursos misturados mesmo vazios, casing ambíguo e duplicatas;
  a fachada nunca faz fallback para restore genérico de um arquivo de comandos.
- UI com labels, foco inicial, feedback traduzido nos três idiomas, bloqueio
  de duplo envio/troca de arquivo pendente e estado novo a cada seleção.
- Render de decisão informa IDs e campos alterados sem argumentos/segredos;
  erros públicos não propagam detalhes privados de erros encadeados.

Evidências automatizadas:

- `app_command_import_wails_test.go`: fachada real, catálogo produtivo,
  global+workspace, confirmação, persistência, publicação e Keep sem escrita;
  rejeições antes da decisão e ausência de vazamento de owner estrangeiro.
- `app_command_import_wails_copy_test.go`: cópia com UUIDs novos iguais aos do
  relatório persistido; negar a segunda decisão impede o lote inteiro.
- `app_command_mutation_desktop_test.go`: token indevido, revogação durante
  confirmação, troca de sessão, timeout e guard recusando publicação pós-commit.
- Testes de resultado/render cobrem redaction, comparação por valor e relatório
  preservado mesmo com cancelamento/timeout depois de gravar.
- Regressão completa de `internal/app`: **PASS**, 158,344 s. Refinos posteriores
  do tratamento de resultado e identidade do produto: matriz focal final
  **PASS**, 15,547 s, incluindo os novos testes de cópia e recusa.
- Regressões completas `commandconfig`, `commanddecision`, `commandportability`,
  `portability` e `wailsapi`: **PASS**. `go vet` dos seis pacotes: **PASS**
  antes do último refino; verificação final registrada abaixo.

Uma execução intermediária informou PASS nos testes, mas terminou com erro de
limpeza de `app.test.exe` por arquivo em uso. Não houve remoção forçada, ajuste
de antivírus, ACP/acpregistry, build/dev do Wails ou executável avulso.

**Não fechado nesta rodada:** exportação pública de comandos, exportação
sensível autorizada/criptografada e aceite integral de import/export. Validação
visual/manual no app não foi executada. R05.3/R05.4 seguem parciais; contagens
mantidas em **11/48 R, 1/12 gates, 0/83 C e 53/84 I**. Isso não reduz o avanço
concreto da importação, mas evita aceitar um item que também exige exportação.

Verificações finais: `go vet` de App/portability/wailsapi **PASS** após o refino;
TypeScript e ESLint **PASS**, Stylelint sem warnings. Matriz frontend da página
de Dados, painel e helpers: **45 testes PASS**, 8,80 s. Testes de UI usam o
transporte mockado; a entrada backend real foi exercitada separadamente acima.

## 45. Exportação comum e roundtrip público — 17/09/2026

O painel **Exportar camadas de comandos**, em Configurações → Dados, gera
JSON v2 exclusivo de `resources.commandLayers`. Seleção global é o padrão;
incluir workspaces é explícito. A API também valida seleção de UUIDs, sem
aceitar owner, catálogo ou autorização fornecidos pelo cliente.

- `app_command_export_desktop.go` autentica a sessão desktop e vincula a
  leitura à identidade do produto/banco. Dados e referências são lidos na
  mesma transação, inclusive com pool de conexão única, sem writes de config.
- Serialização, catálogo e I/O de workspaces ficam fora do DispatchGate.
  Antes de liberar o resultado, o host revalida sessão, estado desbloqueado,
  identidade do produto e época de segurança capturada antes da leitura.
- `ValidateCommandExportRequest` exige seleção explícita/JSON e recusa
  recursos misturados, All, toggles de conteúdo, senha e IncludeCredentials.
  Até 64 UUIDs canônicos sem duplicatas; até 64 entradas no arquivo e 64 KiB.
- O exportador existente continua responsável por excluir defaults puros,
  owner, claims, grants e invocações, e validar referências e dados sensíveis.
  Vazio ou registro legado inválido recusa o arquivo inteiro, sem export parcial.
- `ExportData`/`ExportDataToFile` usam callback privado autenticado antes da
  rota genérica. Falha de validação/export não cria nem trunca arquivo.
- UI reutiliza DTOs gerados e download existentes; impede envios concorrentes,
  ignora resposta após desmontagem e funciona também sob StrictMode. Traduções
  em pt-BR/en/es, labels e feedback pelo announcer; sem bindings editados.

Evidências:

- `TestCommandExportWailsRoundTripCopyPreservesSourceAndPublication`: exporta
  global+workspace pela fachada real; importa o arquivo como cópia com nomes
  e mapa explícitos, aceita duas decisões e confere originais+cópias no banco,
  novos UUIDs, ausência de claims/grants e publicação do mapa.
- `app_command_export_security_test.go`: invalidação de segurança, bloqueio
  do SO e troca do produto durante a leitura não liberam bytes. Pool unitário
  não bloqueia; export não altera snapshot; dados legados inválidos não vazam.
- Validador e fachada têm testes de limites, modos inválidos, sessão ausente,
  callback não montado e preservação do arquivo em falhas.
- Matriz frontend: **51 testes PASS** em cinco arquivos (8,92 s), incluindo
  painel, página de Dados e helpers. TypeScript, ESLint e `go vet`: **PASS**.
- Regressão completa `app`, `portability`, `wailsapi` e `commandportability`:
  **PASS** (149,252 s; 6,100 s; 4,138 s; 2,563 s). Matriz focal final de
  exportação após ajustes de mensagens/limites e cenários de seleção:
  **PASS** em App (23,100 s), portability (1,458 s), wailsapi (2,003 s).
- Dois testes frontend adicionais de recusa segura por escopo vazio/limite:
  painel final **7/7 PASS** (2,68 s). ESLint, Stylelint sem warnings e `go vet`
  aprovados após esses ajustes. Mensagens traduzidas, sem detalhes privados.

**Limites:** a exportação sensível NÃO é ativada por IncludeCredentials.
Conforme D10, exige ação `config_export_sensitive` separada, registrada somente
para UI, decisão de alto risco e criptografia AEP-0047. Também falta qualificar
a importação conjunta de credenciais antes dos bindings; a rota atual rejeita
esse envelope misto. A validação visual/NVDA e a matriz entre instâncias/usuários
do gate R05 não foram encerradas. Sem novo aceite C/I ou gate; **11/48 R,
1/12 gates, 0/83 C, 53/84 I**. Sem ACP, Wails ou executáveis avulsos.

## 46. Prioridade de uso cotidiano e paleta de navegação — 17/09/2026

**Decisão de escopo:** usuário autorizou adiar importação/exportação avançadas
para priorizar uso visível. Nenhuma implementação anterior foi removida e os
itens R05 continuam rastreados. Isso não transforma este recorte em conclusão
do AEP ou da configuração de atalhos.

**Produto:** nove IDs `navigation.<destino>.open`, com destinos workspace,
history, memories, tasklists, jobs, profiles, settings, help e about. São
leituras visuais, sem argumentos e exclusivas da paleta. Catálogo completo,
whitelist local e handlers UI compartilham as mesmas entradas. Frontend usa
rotas fixas, não uma URL recebida do catálogo. Os comandos anteriores
`workspace.list` e `help.shortcuts.show` permanecem disponíveis.

**Execução:** seleção continua passando por Begin/Take/Complete e guard local;
não há navegação antes do handoff. Desmontagem após início do efeito síncrono
não cancela a confirmação do efeito já realizado; erro do handler ainda é
cancelado e o backend permanece responsável pelo resultado. Logout, modal,
mudança de contexto anterior ao efeito e IDs desconhecidos não ganham bypass.
Atalhos antigos não foram alterados nem ganharam roundtrips adicionais.

**Descoberta:** o picker busca nome, descrição, categoria e aliases. Motivo de
indisponibilidade operacional tem precedência sobre metadata de disponibilidade.

**Próximo gate prático, ainda aberto:** configurar e salvar um binding pela UI,
reabrir o aplicativo e executar pelo teclado; depois ligar o Stream Deck ao
mesmo fluxo. A tela D15 ainda falta. Navegar até Configurações não satisfaz
esse gate. Sem nova contagem de R/C/I ou promessa de que só falta teste manual.

**Evidências desta rodada:** matriz frontend em 12 arquivos, **157 testes PASS**;
inclui Topbar, navegação, executor/guard UI, menus e atalhos existentes. Testes
backend focais de metadata/origens/readiness e Begin/Take/Complete dos nove
comandos: **PASS**. Pacotes `wailsapi` e `commandui`: **PASS**. TypeScript,
ESLint e `go vet` dos três pacotes envolvidos: **PASS**. A primeira execução
ampla capturou a revisão intermediária do catálogo/readiness; a reexecução
final está registrada abaixo. Nenhum novo aceite visual/NVDA foi presumido.

A regressão ampla também revelou `SQLITE_BUSY` no setup do segundo job em
`TestCommandJobMultiSourceRejectsDivergentChainsAndKeepsSurvivor`. O erro
reapareceu em 2/5 repetições; após acrescentar diagnóstico por etapa, passou em
5/5 sem alteração de produção. Portanto a causa não está demonstrada nem a
intermitência resolvida. Não foi adicionado retry artificial, sleep ou remoção
de assertiva. O teste de rebuild passou a exigir as 11 entradas reais do
catálogo, em vez das duas anteriores. Essa evidência não fecha gates de jobs.

Reexecução final completa de `go test ./internal/app -count=1 -timeout=240s`:
**PASS (106,776 s)**. `wailsapi` e `commandui` passaram na matriz anterior
(7,134 s e 3,231 s). Topbar/navegação revalidados após revisão: **61 PASS**;
aviso React do novo teste corrigido com `act` e cenário focal novamente PASS.
Intermitência SQLite acima continua registrada, mesmo com a execução final
verde. Sem commit/push, geração de bindings ou execução de Wails/pacotes ACP.

**Conferência manual curta, ainda não executada:**

- [ ] Reiniciar pelo fluxo de desenvolvimento habitual nesta worktree;
  abrir **Comandos**, buscar `histórico`, selecionar **Abrir histórico** e
  pressionar Enter. Esperado: a tela Histórico abre uma única vez.
- [ ] Abrir **Comandos** novamente, buscar `settings` (alias) e executar
  **Abrir configurações**. Esperado: a tela de configurações existente abre.
- [ ] Abrir e fechar a paleta com Escape, sem executar. Esperado: foco volta
  ao botão e a tela não muda. Conferir anúncios e navegação com NVDA.

## 47. Editor global de comandos e acionadores — 17/09/2026

Recorte D15 implementado em código, sem executar Wails nesta máquina. A
geração dos cinco métodos novos do App foi explicitamente deixada para o
usuário, devido ao histórico de alertas de executáveis temporários. Nenhum
arquivo gerado foi editado manualmente nesta rodada.

- [x] Rota `/settings/commands` e aba **Comandos e acionadores** nas
  Configurações, com listas e menus de ações compartilhados.
- [x] Inspeção dos padrões reais, camadas globais pessoais, criação/edição
  e habilitação de camadas, criação/edição/exclusão de acionadores simples.
- [x] Captura explícita de teclado, Escape/Tab/blur para cancelar, proteção
  contra IME, repetição e AltGr; traduções pt-BR/en/es.
- [x] Supressão/restauração confirmada de padrão da paleta, refletida na
  resolução executada pelo produto e preservada após releitura/rebuild.
- [x] Mutação usa o pipeline existente: autenticação, decisão persistida,
  CAS, auditoria, reconciliação e publicação. Não há writer paralelo.
- [x] Registros contextuais/avançados permanecem somente leitura; guarda
  no diff transacional impede sobrescrita por uma pré-leitura desatualizada.
- [x] Sessão revogada durante confirmação não grava; leitura perde validade
  após mudança de época, bloqueio do SO ou troca do produto. Erros redigidos.
- [x] UI descarta respostas antigas, bloqueia envio duplicado e diferencia
  commit de publicação, sem retry automático da operação confirmada.
- [ ] Gerar bindings pelo fluxo normal Wails, conferir TypeScript novamente
  e abrir a página no aplicativo real. O usuário fará isso posteriormente.
- [ ] Aceite visual/NVDA, foco dos diálogos e contraste nos temas.
- [ ] Ligar ingresso produtivo do teclado personalizado. Salvar/habilitar uma
  camada **não** satisfaz este item; ativação pessoal foi tratada na seção 48.
- [ ] Editor e ingresso produtivo de Stream Deck, escopos workspace e demais
  capacidades D15 ainda não implementadas neste recorte.

A grade compartilhada recebeu correção focal: o clique no botão de ações
agora abre o mesmo menu contextual disponível por Enter. O novo cenário
regressivo usa o componente real, sem substituir a grade por um mock.

**Evidências:** matriz focal backend de CRUD/segurança/projeção **PASS
(17,491 s)**, incluindo recusa de edição de condição contextual. Matriz
frontend da página, SettingsPage, DataGrid, captura e serviço: **90 testes
PASS em 6 arquivos**. ESLint e Stylelint sem warnings: **PASS**. `go vet
./internal/app`: **PASS**. TypeScript aponta exatamente os **cinco exports
Wails ausentes**, esperados até a geração autorizada ao usuário; portanto
não se declara build frontend aprovado. Axe não substitui teste de contraste
em navegador nem validação manual com NVDA.

Regressão final completa `go test ./internal/app -count=1 -timeout=240s`:
**PASS (120,630 s)**. Segunda matriz frontend de Jobs/Perfis/Skills, Topbar,
navegação, handoff e menus: **108 PASS em 8 arquivos**; total das duas
matrizes: **198 testes**. Nenhum Wails ou pacote ACP executado. Sem
commit/push; a intermitência SQLite registrada na seção 46 não reapareceu
nesta execução, mas continua sem causa demonstrada.

**Contagem:** nenhum critério integral novo presumido. D15 e o gate
configurar → reiniciar → executar por teclado continuam abertos. Mantidos
**11/48 R, 1/12 gates, 0/83 C, 53/84 I históricos**. Importação/exportação
avançadas seguem adiadas; este recorte não reabre essa frente.

## 48. Ativação manual das camadas globais — 17/09/2026

Este recorte liga as ações do editor ao serviço de ativação real. Não habilita
o ingresso de teclado personalizado nem altera os atalhos legados.

- [x] Preparação confirmada de regra manual persistente global, separada da
  ativação. Regra existente é reutilizada; ambiguidade não escolhe uma regra
  arbitrariamente, e o diff transacional impede criação duplicada.
- [x] Ativar/desativar com Pin/Back sob o gate compartilhado, identidade e
  origem derivadas pelo host, revalidação de configuração, sessão e epochs.
- [x] Origem UI compartilhada com a restauração; habilitar não implica ativar.
  Ativações de outras origens não são retiradas pela ação desta tela.
- [x] UI distingue atividade efetiva e ativação manual própria, impede duplo
  envio, usa menus compartilhados e traduz as ações em pt-BR/en/es.
- [x] Teste integrado das fachadas públicas confirma a troca do binding
  efetivamente executado na paleta e o retorno ao padrão ao desativar,
  inclusive após rebuild e pedidos repetidos.
- [x] Falha de publicação depois do commit mantém a claim persistida, retorna
  `committed=true/published=false` e impede execução pelo mapa antigo.
- [ ] Gerar os bindings Wails pelo fluxo habitual do usuário. São **sete**
  exports pendentes ao todo, incluindo os cinco da seção 47.
- [ ] Aceite no aplicativo real/NVDA e reinício físico. Não substituído pelos
  testes automatizados de reconstrução.
- [ ] Conectar o ingresso produtivo do teclado personalizado ao mapa ativo.
  Esta é a próxima ligação necessária para executar as combinações salvas.

**Conferência manual, quando os bindings forem gerados:**

- [ ] Em Configurações → Comandos e acionadores, criar uma camada pessoal.
  Resultado esperado: habilitada, mas ainda inativa.
- [ ] No menu Ações, escolher **Preparar ativação manual**, confirmar e
  conferir que a camada continua inativa.
- [ ] Escolher **Ativar manualmente** e conferir o estado ativo.
- [ ] Desabilitar a camada: fica sem efeito, preservando a ativação manual.
  Escolher **Desativar ativação manual** ainda deve ser possível.
- [ ] Habilitar novamente: a ativação retirada não deve reaparecer.
- [ ] Ativar, reiniciar pelo fluxo habitual e conferir a restauração; retirar
  a ativação depois do reinício e confirmar que continua removida.
- [ ] Conferir navegação por teclado, anúncios NVDA e foco no menu de ações.

**Validação automática:** frontend **98 PASS em seis arquivos**, ESLint PASS;
TypeScript continua bloqueado somente pelos sete exports gerados ausentes.
Módulos `commandactivation`, `commandbindings` e `commandconfig`: PASS.
Oito testes focais novos de ativação/segurança: **PASS (17,896 s)**,
incluindo restauração com nova época de segurança e retirada da claim depois
da restauração, rollback transacional de sessão revogada e outra origem
preservada. `go vet ./internal/app`: **PASS**.
Matriz final restrita a `TestCommandSettings`, `TestCommandProductResolution`
e `TestCommandLifecycle`: **PASS (7,650 s)** após todas as correções.

**Limitação da regressão:** a tentativa completa de `internal/app` terminou
em **FAIL (154,021 s)**. O log volumoso foi truncado sem preservar o teste
responsável; não há causa demonstrada e não se atribui a falha à intermitência
SQLite anterior. A revisão automática de segurança bloqueou a repetição
ampla porque a suíte também inclui testes ACP no próprio pacote app. Não
foi contornado o bloqueio. Nenhum pacote `internal/acp`/`acpregistry`, Wails
ou binário manual foi executado nesta rodada; a suíte geral **não** está
declarada aprovada. Repeti-la exige autorização específica e avaliação desse
risco. As validações focais não equivalem ao aceite da suíte inteira.

**Contagem:** nenhum aceite integral inferido desta fatia. Mantidos
**11/48 R, 1/12 gates, 0/83 C, 53/84 I históricos**; D15 e o gate de teclado
continuam abertos. Importação/exportação avançadas permanecem adiadas.

## 49. Teclado personalizado → mapa ativo → picker real — 17/09/2026

Primeiro comando operacional: **Listar workspaces** (`workspace.list`). O
acionador salvo em camada global ativa chega ao executor comum como
`keyboard.local`, com IDs de ocorrência gerados pelo host. Não há execução
pela paleta disfarçada de teclado nem handler sentinela.

- [x] Publicar cópia do mapa resolvido, com geração e somente combinações
  elegíveis. Camada inativa, condição contextual não disponível e resolução
  sem seleção não anunciam um atalho executável.
- [x] Resolver a combinação no frontend em memória. Eventos não mapeados
  preservam o caminho anterior, sem consulta ao backend por tecla.
- [x] Usar o executor completo para autenticação, autorização, resolução,
  ledger, handler real e proveniência de teclado. Revogação e mapas antigos
  não autorizam execução.
- [x] Deduplicar down/repeat, liberar em up e invalidar em blur/geração;
  não capturar edição, IME, AltGr ou modal. Somente Control/Alt/Meta entram
  neste mapa operacional; teclas sem modificador ficam com os controles.
- [x] Apresentar o resultado no picker compartilhado com revalidação de
  usuário, sessão, workspace, aba, rota e superfície. Usar o mesmo caminho
  de apresentação da paleta, sem repetir sua execução.
- [x] Atualizar a indicação de disponibilidade do editor e orientações em
  pt-BR/en/es, explicitando o recorte em vez de prometer todos os atalhos.
- [ ] Regenerar bindings pelo fluxo habitual do usuário: os sete exports
  pendentes de Configurações e os métodos de mapa/dispatch/reset de teclado.
  Não foram editados arquivos gerados nem executado Wails nesta rodada.
- [ ] Aceite no aplicativo/NVDA conforme checklist de `COMANDOS.md`, incluindo
  ativação/desativação, foco, picker, repeat e preservação de Ctrl+N/Ctrl+Tab.

**Evidências:** matriz final de `TestCommandProduct`, `TestCommandKeyboard`,
`TestCommandSettings` e `TestCommandLifecycle`: **PASS (11,380 s)**. Os cinco
testes de teclado usam configuração/ativação e handler reais, comprovam
proveniência no banco, mapa copiado/geração, up/repeat, recusa de origem não
admitida no catálogo e sessão revogada. Módulos `commandbindings` e
`commandexecution`, testes focais de índice e autorização: **PASS**.

Frontend: **159 testes PASS em oito arquivos**, incluindo 81 do Topbar
(23 novos de teclado), seis do controller de teclado e a página de
Configurações. Testes cobrem respostas tardias, resets, modal, campos
editáveis, 1.000 eventos não mapeados sem consulta adicional e picker sem
execução de paleta. ESLint sem warnings e `go vet ./internal/app`: **PASS**.
TypeScript continua com **sete exports gerados ausentes**, todos do serviço
de Configurações; build completo não declarado aprovado. Não foi repetida
a suíte ampla que inclui ACP, nem executado Wails ou binário manual.

A matriz intermediária falhou num cenário novo que fabricava no banco um
comando com origem não admitida. Corrigido o teste para verificar a recusa
pela API pública, sem enfraquecer a validação de produção. Não se conta esse
cenário como prova de resolução de conflito entre dois comandos elegíveis.

**Limites:** não migra os atalhos legados, não habilita teclado global nem
Stream Deck e não completa os provedores de condições contextuais do teclado.
Somente `workspace.list` já possui origem local de teclado admitida pelo
catálogo e handler operacional neste recorte. D3/D7/D15 continuam parciais.

**Contagem:** mantidos **11/48 R, 1/12 gates, 0/83 C, 53/84 I históricos**.
Esta ligação de produto não é evidência de aceite integral dos gates abertos.
Portabilidade avançada permanece adiada, conforme a decisão do usuário.

## 50. Falha no startup depois de salvar configuração — 17/09/2026

Os startups do usuário às 22:22 e 22:23 recusaram `command-storage`.
Reproduzida em SQLite temporário uma inconsistência concreta: os signers
canônicos retornam HMAC hexadecimal opaco, mas `PrepareKeys` exigia prefixo
textual de versão em todos os receipts/auditorias. O formato `vN:digest`
pertence a outro signer já existente, não a todos os produtores.

- [x] Reconhecer ambos os formatos efetivamente produzidos, exigindo digest
  hexadecimal minúsculo de 64 caracteres e versão conhecida quando explícita.
- [x] Continuar exigindo todas as chaves registradas e seus digests íntegros;
  nenhuma inferência de versão para assinatura opaca, nenhuma reescrita de
  auditoria, coleta de chave ou aprovação de decisão pelo bootstrap.
- [x] Códigos fechados de diagnóstico para schema/storage, chaves, referência
  de fingerprint e cancelamento, sem logar segredos ou erros arbitrários.
- [x] Erro específico de carregamento na UI; disponibilidade do teclado não
  é anunciada enquanto não houver snapshot válido da identidade atual.
- [ ] Repetir dois startups no ambiente do usuário e confirmar que a camada
  e a combinação já criadas reaparecem, sem apagar dados ou credenciais.
- [x] Executar o atalho com a camada ativa: o usuário confirmou que agora
  tudo funciona, inclusive o atalho ao ativar a camada. Isso não presume
  aceite detalhado de NVDA ou a repetição de dois startups.

O teste `TestKeysRestartRecognizesStoredFingerprintFormats/canonical` foi
executado antes da correção e falhou com `ErrKeys`; o mesmo cenário passa
depois da correção. Referência desconhecida, digest malformado e chave
ausente permanecem recusados. O log original não traz a causa interna;
esta reprodução não presume inspeção nem modificação do banco do usuário.

**Evidências finais:** suíte `internal/commandbootstrap` **PASS (9,526 s)**;
matriz restrita `TestCommandStorage|TestCommandKeyboard|TestCommandSettings`
em `internal/app` **PASS (9,147 s)**. O teste integrado novo usa o CRUD e
confirmações reais, manager persistente, shutdown e novo App; preserva IDs
de camada/binding, restaura a ativação e executa o atalho antes/depois. Ele
recria o App/manager sobre SQLite de teste, não abre outra instância Wails
nem substitui o aceite de reinício físico no ambiente do usuário.

Frontend: **131 testes PASS em quatro arquivos**; `tsc --noEmit`, ESLint e
`go vet ./internal/commandbootstrap ./internal/app`: **PASS**. Os bindings
já haviam sido gerados pelo usuário; nenhum gerador foi executado por esta
rodada. Dois agentes Luna cobriram UI e teste integrado, com revisão local.
Não houve acesso ao banco real, alteração de chaves reais, Wails ou ACP.

**Contagem:** permanecem **11/48 R, 1/12 gates, 0/83 C, 53/84 I históricos**.
Correção de regressão não é fechamento de novo gate nem conclusão do AEP.

## 51. Teclado local para os nove destinos de navegação — 17/09/2026

- [x] Publicar mapa tipado de combinações, comandos e handlers, limitado
  ao catálogo operacional; sem fallback para o formato antigo.
- [x] Admitir `keyboard.local` nos nove comandos `navigation.*.open`.
  `help.shortcuts.show` permanece exclusivo da paleta.
- [x] Derivar o comando no host pela combinação; resolver novamente o
  binding persistido, com autoria real de teclado e auditoria do binding.
- [x] Reutilizar reserva, autorização, handoff e confirmação de UI; nenhum
  caminho de navegação direta nem chamada de paleta para simular teclado.
- [x] Recusar mapa obsoleto, sessão revogada, mutação antes do handoff e
  troca indevida entre rotas de handlers UI/backend.
- [x] Permitir confirmação depois do reset causado pela própria navegação;
  repetição e keyup conservam a semântica de uma ocorrência por pressão.
- [ ] Aceite manual dos novos destinos no app e com NVDA, seguindo
  `docs/content/recursos/COMANDOS.md`, após geração normal dos bindings.

O percurso anterior de criação/ativação/execução do primeiro atalho foi
confirmado pelo usuário. Não se estende esse aceite à ampliação desta rodada.
Sem mudança de schema, acesso ao banco real, criação de chaves reais,
migração de atalhos legados, ativação global ou Stream Deck. Portabilidade
avançada segue adiada. Dois agentes Luna cobrem frontend e testes Go;
integração, revisão de produção e documentação ficam com o agente principal.

**Evidências finais:** matriz Go restrita
`TestCommandKeyboard|TestCommandStorage|TestCommandSettings|TestCommandUI|TestCommandProduct|TestCommandLifecycle`
em `internal/app`: **PASS (13,497 s)**; `internal/commandui`: **PASS (2,949 s)**.
Frontend: **142 testes PASS em seis arquivos** (teclado, porta Wails,
executor UI, navegação, Topbar e Configurações). TypeScript, ESLint sem
warnings e `go vet ./internal/app`: **PASS**. Revisão corrigiu validação de
handler antes da deduplicação e envio de keyup para comandos UI, com testes
da mesma geração; também cobre Escape, resposta tardia e desmontagem da
Topbar pela própria navegação antes de confirmar o resultado. O teste de
origem não permitida verifica recusa síncrona, sem fabricar confirmação.
Não foram executados Wails, ACP, suíte ampla Go ou executáveis manuais;
bindings gerados permanecem para o fluxo habitual do usuário.

**Contagem:** mantidos **11/48 R, 1/12 gates, 0/83 C, 53/84 I históricos**.
Avanço funcional no produto não equivale ao aceite integral dos gates.

## 52. Paleta aparentemente vazia e navegação do picker — 17/09/2026

O usuário esclareceu que aparece apenas a busca e as setas não alcançam
itens. Não afirmou ter recebido a mensagem de catálogo vazio. A investigação
separou entrega do catálogo de interação do menu; não criou catálogo local
alternativo nem enfraqueceu disponibilidade/autorização.

- [x] Corrigir propagação das teclas da busca para o menu pai: seta para
  baixo foca o primeiro resultado, Enter seleciona uma única vez e espaço
  continua edição de texto, sem acionar comando.
- [x] Anunciar abertura da paleta com totais carregados e disponíveis;
  catálogo vazio tem anúncio próprio. Respostas obsoletas não anunciam.
- [x] Testar Topbar com Menu/useAnchoredContextMenu e guard/executor reais,
  catálogo/API simulados: 11 itens, pesquisa, seta, Enter e handoff.
- [x] Provar no backend de teste que o bind instalado antes do bootstrap
  entrega 11 comandos disponíveis antes da ativação, depois dela e após
  execução do atalho. Sem `wireCommandCatalog` manual no teste.
- [x] Documentar distinção: paleta independe de atalhos; os nove comandos
  ampliados aceitam combinações pessoais, mas não ganharam novas teclas padrão.
- [ ] Confirmar com NVDA no ambiente do usuário a abertura, o anúncio de
  quantidade, a navegação pelas setas e execução de um destino pelo picker.

Evidências: **144 testes frontend PASS em seis arquivos**; TypeScript e
ESLint sem warnings aprovados. Testes Go de bootstrap/catálogo e readiness
em `internal/app`: **PASS (3,155 s)**. O diagnóstico automatizado não
reproduziu catálogo vazio ou todos os itens indisponíveis nesse percurso;
a correção de interação não é certificação do comportamento do NVDA real.
Não foram executados Wails, ACP ou binários manuais; banco real intocado.

**Contagem:** mantidos **11/48 R, 1/12 gates, 0/83 C, 53/84 I históricos**.

## 53. Disponibilidade contextual e padrões de navegação — 17/09/2026

O anúncio informado pelo usuário confirmou 11 comandos carregados e zero
disponíveis. Foi reproduzida uma falha que o fixture da seção 52 não cobria:
com a projeção real de jobs/manutenção montada, mudar o perfil invalida o
guard contextual. A execução já atualizava essa projeção; a consulta do
catálogo não. Isso deixava os comandos listados, mas bloqueados.

- [x] Reproduzir a falha antes da correção usando manutenção real em banco
  temporário e mudança real de perfil; sem simular indisponibilidade.
- [x] Reutilizar a atualização de projeção na descoberta da paleta, sem
  ignorar sessão, cofre, lifecycle, registro ou identidade do runtime.
- [x] Provar catálogo 11/11 após atualização e handoff UI concluído.
- [x] Provar que a mesma atualização não libera comandos com cofre fechado.
- [x] Publicar camada integrada `application.keyboard`, sempre ativa, com
  oito padrões versionados: Alt+W/C/H/L/T/J/P e Alt+Backspace.
- [x] Remover despacho legado desses oito atalhos; usar somente mapa local,
  origem real de teclado e handoff existente. Sem fallback na ausência do mapa.
- [x] Preservar F1, Alt+M, Alt+E/I, Ctrl+K e atalhos de abas nesta rodada.
  Alt+Backspace continua impedindo retorno nativo sem navegar atrás de modal.
- [x] Testar supressão/restauração confirmada de padrão, rejeição do
  acionamento suprimido, handoff restaurado e auditoria de origem/binding.
- [x] Testar precedência pessoal Alt+H para Memórias e retorno ao padrão
  Histórico após desativação, sem alterar as demais combinações.
- [x] Documentar padrões, precedência pessoal e conferência manual com
  supressão/restauração em `docs/content/recursos/COMANDOS.md`.
- [ ] Aceite manual: paleta 11/11 após reinício e mudança de contexto,
  navegação por setas/NVDA, padrões sem camada pessoal e personalização.

Evidência inicial: regressão reproduzida antes da correção (**FAIL 19,270 s**);
catálogo/readiness aprovado depois (**PASS 21,674 s**). Matriz adicional de
catálogo, cofre e projeção de jobs: **PASS 25,082 s**. Não foram executados
Wails, ACP ou executáveis manuais; banco real intocado.

Matriz `TestCommand(Product|Palette|Catalog|UI|Lifecycle)`:
**PASS 6,748 s**. O helper de supressão da paleta agora seleciona a camada
por ID, sem presumir que ela é a única integrada; as verificações de replay,
autorização e recusa sem fallback foram preservadas. TypeScript, ESLint dos
arquivos alterados e `go vet ./internal/app`: **PASS**.

Matriz `TestCommand(Keyboard|Settings|StorageSettingsRestart)`:
**PASS 11,927 s**. As expectativas dos testes foram adaptadas aos oito padrões
adicionais; combinações pessoais são selecionadas por identidade, não pela
posição na lista, preservando os testes de origem, revogação e persistência.

Frontend final: **164 testes PASS em 10 arquivos**, sem warnings React
na execução final. Inclui Menu/paleta real, Topbar, mapa local, executor UI,
portas Wails simuladas, navegação e configurações. Os novos testes verificam
handoff único para os oito padrões e ausência de rota legada sem binding.

**Contagem:** mantidos **11/48 R, 1/12 gates, 0/83 C, 53/84 I históricos**.
Esta migração parcial e a correção não equivalem ao aceite integral do AEP.

## 54. Bootstrap antes da observação do SO — 18/09/2026

O aceite manual da seção 53 **falhou**: paleta ainda 11/0, Alt+C sem efeito,
erro de carregamento do teclado e relato adicional de Alt+M sem resposta.
No log do worktree, às 23:47:13 de 17/09, a configuração inicial é recusada
após autenticação. Há também contenção SQLite em jobs/MCP; não foi atribuída
a ela a causa da paleta sem reprodução.

Reprodução determinística encontrou uma lacuna de montagem: o bootstrap
chega antes da primeira observação do Windows, é recusado por `osKnown=false`,
e o callback posterior apenas descarta mapas ao atualizar o estado do SO.
Não havia reconstrução após essa observação. Fixtures anteriores marcavam
o SO como desbloqueado antes do bootstrap, ocultando a sequência real.

- [x] Reproduzir a recusa inicial e a ausência de recuperação antes da correção.
- [x] Reconstruir a configuração autenticada após observação de desbloqueio,
  fora da recepção de eventos do SO, com uma fila limitada à observação atual.
- [x] Cancelar reconstrução anterior em nova observação; lock e perda do
  monitor continuam descartando mapas imediatamente.
- [x] Serializar bootstrap por autenticação/cofre/SO; encerrar o worker junto
  do monitor, sem goroutines de recuperação soltas no shutdown.
- [x] Notificar o mapa novamente **após** readiness: publicar a projeção
  antes de habilitar o lifecycle não basta para o frontend conseguir lê-la.
- [x] Provar recuperação do mapa de oito padrões, configurações e catálogo
  11/11; lock recusa a geração anterior, unlock produz nova geração e Alt+H
  chega ao handoff UI pelo comando de Histórico.
- [x] Revalidar a sessão antes de reconstruir no worker de SO; cofre fechado
  e sessão revogada recusam mapa/execução, sem invocações registradas.
- [x] Aquisição do serializador cancelável pelo worker: o monitor encerra
  mesmo enquanto outro bootstrap mantém a exclusão, sem atrasar o lock.
- [x] Tela de configurações recupera carga inicial falhada após o aviso de
  prontidão, inclusive se recebido durante uma leitura pendente. Não apaga
  editor aberto, não interrompe mutação e descarta eventos de identidade antiga.
- [x] Testar Alt+M com Topbar/MenuButton/Menu reais, inclusive com foco na
  busca da paleta com todos os comandos indisponíveis. Funcionou nesse
  cenário; a causa do relato no ambiente real ainda não foi reproduzida.
- [ ] Aceite manual no app/NVDA após reinício, incluindo Alt+M.

Evidências: teste de regressão **FAIL antes da correção (22,723 s)**,
com o mesmo aviso de recusa no bootstrap; testes de observação/monitor e
recarga **PASS após correção (20,501 s)**. Matriz ampliada de produto,
catálogo, teclado, configurações, restart e observação: **PASS (33,021 s)**.
Sem Wails, testes ACP, executáveis manuais ou alterações no banco real.

Validação final: matriz Go incluindo lifecycle completo e os novos cenários
de SO **PASS (33,578 s)**; frontend **169 testes PASS em 10 arquivos**, mais
um cenário adicional Alt+M com paleta indisponível (**3/3** no arquivo de
integração, total de **170 cenários distintos**). TypeScript, ESLint dos
arquivos alterados, `go vet ./internal/app` e verificação de whitespace: PASS.

**Contagem:** mantidos **11/48 R, 1/12 gates, 0/83 C, 53/84 I históricos**.

## 55. Contenção SQLite no bootstrap — 18/09/2026

O aceite da seção 54 falhou novamente. O log de 06:02:56 demonstra
`SQLITE_BUSY` no UPDATE de restauração das claims e no INSERT de contadores
do escopo. Ambas as tentativas encerravam a inicialização sem mapa pronto.

- [x] Aplicar a política limitada `WithSQLiteBusyRetry` ao `EnsureScope`.
- [x] Repetir a transação de restauração abortada por BUSY, fora do gate
  de segurança, com epochs novos e revalidação da mesma sessão por tentativa.
- [x] Não repetir comandos, efeitos externos ou bootstrap inteiro; falha
  persistente e cancelamento continuam recusando o mapa.
- [x] Adicionar testes com writer SQLite real em banco isolado de `t.TempDir`,
  com guarda que recusa qualquer banco fora da fixture temporária.
- [ ] Aceite manual: reiniciar pelo fluxo habitual, abrir a paleta sem busca,
  conferir comandos executáveis e testar Alt+C fora de campo editável/modal.

Nenhum acesso ao banco real, Wails, testes ACP ou executável manual nesta
rodada. Outras etapas anteriores (migração, chaves e BindInstance) não recebem
retry indiscriminado; não são a falha identificada neste log.

Evidências: matriz focada de bootstrap, SO, lifecycle, produto, catálogo,
teclado, configurações e restart **PASS (38,949 s)**. Os testes de recuperação
exigem observar BUSY real antes de liberar o writer; verificam 11 comandos
disponíveis, handoff Alt+C e restauração sem duplicação de claims. Cancelamento
e deadline recusam mapa operacional. `go vet ./internal/app` e whitespace
dos arquivos desta rodada: PASS. O check global ainda aponta whitespace
preexistente nos bindings gerados, que não foram editados nesta rodada.

Repetibilidade das quatro regressões iniciais: **PASS em três execuções
(25,087 s)**. Revisão adicional: quatro testes de segurança **PASS**, incluindo
lock do SO com gate livre e troca de identidade durante o retry; ambos negam
a tentativa seguinte e mantêm o mapa indisponível.

## 56. Menu padrão e primeiro ingresso Stream Deck — 18/09/2026

**Aceite da seção 55:** usuário informou “tudo funciona normalmente”,
confirmando paleta, Alt+C e carga de configurações. Esse aceite não é o do
Stream Deck no App, que começa nesta rodada.

Escopo aprovado: Alt+M abre o menu pelo catálogo comum; configuração de uma
tecla física executa comandos de navegação/ajuda por camada pessoal ativa.
Não habilita shell/macros, injeção de teclas externas, jobs nem ações ainda
não migradas de editor/chat/abas. Listar workspaces fica fora do ingresso
físico inicial por usar resultado backend em vez de handoff visual.

- [x] Qualificar Alt+M padrão, paleta e ausência de listener legado duplicado.
- [x] Qualificar serial/posição no contrato backend, persistência e ativação.
- [x] Qualificar driver → ocorrência física → executor → reserva/handoff UI
  com driver controlado, executor e SQLite temporário reais.
- [x] Qualificar invalidação, limpeza visual, desconexão e reconexão em
  testes de integração; o hardware real permanece no aceite separado.
- [ ] Aceite no dispositivo do usuário: navegação/menu, release, reconexão,
  lock/unlock, mudança de camada e restart.

O runtime só abre dispositivos que tenham bindings efetivos. Sem bindings,
não abre HID nem consulta SQLite no ciclo de descoberta. Títulos são
renderizados a partir do catálogo localizado; a seleção de idioma acompanha
a tela de configurações. Pastas, edição de imagens/estados e contexto externo
continuam fora deste recorte. Não se considera R09/R10 integralmente aceito.

Evidências backend: matriz de comandos/boot/SO/lifecycle/catálogo/teclado/
configurações/restart e Stream Deck **PASS (40,082 s)**; testes focados do
dispositivo repetidos duas vezes **PASS (24,697 s)**. Pacotes commandconfig,
commandexecution e commanddeck **PASS**; `go vet` **PASS**. O teste de origem
forjada chama o executor real e exige recusa sem handoff; o teste de reconexão
recusa a reserva antiga e executa a nova. Detector de corrida não executou:
`go test -race` exige CGO, desabilitado neste ambiente. Nenhum Wails, ACP,
executável manual ou acesso ao banco/dispositivo real nesta rodada.

**Contagem:** mantidos **11/48 R, 1/12 gates, 0/83 C, 53/84 I históricos**.

Evidências frontend desta rodada: **165/165 testes PASS** (Topbar, integração
real da paleta/menu, configurações, navegação e execução/efeitos UI).
`tsc --noEmit` e ESLint dos arquivos alterados passaram. Corrigida disputa
de foco ao abrir o menu com a paleta aberta e preservado o preenchimento
independente de serial/posição. Revalidação final Stream Deck backend:
**PASS (23,778 s)**, incluindo publicação de lista vazia ao desconectar.

## 57. Descoberta de dispositivos e aceite básico — 18/09/2026

**UX substituída pela seção 58 por decisão do usuário:** seletor de dispositivo
e entrada manual de serial não permanecem no produto. Evidências abaixo são
históricas, não requisitos de compatibilidade da nova captura.

O usuário informou: “funcionou muito! deu tudo certo”, após testar o ingresso
físico. Registrado como aceite do fluxo básico, não como confirmação individual
de todos os cenários de reconexão, bloqueio, restart ou múltiplos aparelhos.

- [x] Aceite manual do acionamento básico no Stream Deck do usuário.
- [x] Enumerar modelos, seriais e quantidade de teclas sem abrir dispositivos,
  inclusive sem bindings prévios; ordenar e eliminar duplicatas.
- [x] Publicar descoberta somente depois da revalidação de sessão/epoch;
  erro nativo não expõe detalhes nem impede carregar configurações.
- [x] Seleção acessível na tela, atualização explícita e entrada manual.
- [ ] Aceite manual da seleção detectada e cenários físicos específicos.

Backend: `TestCommand(Deck|Settings)` PASS (26,778 s), descoberta incluindo
cancelamento PASS (21,482 s), `go vet ./internal/app` PASS. Testes usam driver
simulado e banco temporário, sem Wails, ACP ou acesso físico nesta rodada.
Nenhuma nova assinatura pública Wails ou edição de arquivo gerado: descoberta
é payload tipado de evento emitido ao carregar configurações autenticadas.

Contagens globais preservadas; este recorte não encerra D13/D15 nem o AEP.

Frontend: 129/129 testes PASS (configurações, Topbar e integração real de
paleta/menu); TypeScript e ESLint PASS. Cobertos escolha sem perder posição,
lista vazia, erro, atualização sem evento e separação entre descoberta e
status operacional. Avisos preexistentes de `act` em dois testes de Topbar
continuam presentes, sem falhas. Regressão backend ampliada de descoberta,
configurações, bootstrap e produto: PASS (13,155 s).

## 58. Captura física sem identificadores técnicos — 18/09/2026

Decisão explícita do usuário: configurar Stream Deck como um acionador
capturado, sem escolher aparelho nem informar serial/posição, inclusive sem
modo avançado de aparelho desconectado. Pressionar a tecla identifica ambos.

- [x] Remover publicação automática de seriais em GetCommandSettings; manter
  enumeração privada apenas para preparar captura.
- [x] Captura autenticada de 30 segundos, vinculada ao epoch, pelo mesmo
  runtime/HID, incluindo dispositivos ainda sem bindings.
- [x] Bloquear comandos durante captura, preservar supressão até release e
  invalidar reservas/leitores de antes da captura mesmo após cancelamento.
- [x] Qualificar captura e cancelamento com driver simulado, sem banco real.
- [x] Qualificar UI acessível sem IDs, cancelamento/foco e respostas tardias.
- [ ] Aceite manual do novo fluxo pelo usuário no aplicativo.

Novos métodos BeginCommandDeckCapture e CancelCommandDeckCapture usam a
fachada Wails tipada, seguindo as fachadas de comandos existentes; eventos
carregam requestId para correlação, nunca aceitam evento físico da UI. Nenhum
arquivo gerado foi editado à mão nem Wails iniciado nesta rodada.

Escopo de comandos permanece navegação/ajuda; não amplia shell, macros ou
ações de outros aplicativos. Sem promoção dos gates globais nem conclusão
integral de D13/D15. AEP permanece In Progress.

Evidências finais: regressão `TestCommand(Deck|Settings|Bootstrap|Product)`
PASS (22,088 s); suíte de captura PASS (10,493 s), com dois aparelhos, ausência
de ledger/UI run na gravação, invalidação de reserva anterior, lock e execução
real via Take/Complete somente após cancelamento/release. A captura só publica
o resultado ao soltar a tecla, evitando perder o release ao encerrar o leitor.
Frontend: matriz 132/132 PASS; revalidação focada final 33/33 PASS; TypeScript,
ESLint e `go vet ./internal/app` PASS. Dois avisos históricos de `act` em
Topbar permanecem, sem falhas. Não houve execução Wails, ACP, acesso ao banco
real ou ao dispositivo físico. Detector de corrida continua não validado
neste ambiente sem CGO. Aceite físico da nova gravação permanece pendente.

## 59. Prioridade: ampliar ações; gesto longo adiado — 18/09/2026

Decisão do usuário: registrar pressionamento longo como melhoria futura e
priorizar expansão do funcionamento e das ações disponíveis.

- [ ] Futuro, não bloqueante: distinguir toque curto e pressionamento longo
  na mesma tecla. Definir limiar, cancelamento e exclusão mútua antes de
  implementar; preservar despacho imediato em teclas sem ação longa.
- [ ] Próximo lote proposto: ações de workspace/abas já existentes (criar,
  próxima/anterior e fechar), reutilizando os handlers reais e os atalhos
  padrão, sem listener legado duplicado.
- [ ] Lote seguinte proposto: ações de conversa/chat; depois ações de editor.

Para cada ação migrada: disponibilidade contextual correta na paleta,
teclado e Stream Deck conforme suporte; foco/modal e confirmações preservados;
teste do efeito real, sem lógica de negócio paralela e sem atrasar atalhos
simples. Inventariar os handlers e confirmar o recorte antes da implementação.
Esta seção registra prioridade, não declara novas ações implementadas nem
substitui o aceite manual pendente da captura da seção 58.

## 60. Clareza das camadas e fronteira da migração — 18/09/2026

Os nomes aprovados são **Comandos padrão** (disponibilização na paleta) e
**Mapa de teclado padrão** (associações de teclas). A mudança é de apresentação,
sem trocar IDs, fingerprints, nomes de camadas pessoais ou prioridades.

- [x] Nomes e descrições atualizados em português, inglês e espanhol, com guia
  de usuário atualizado. Teste focalizado `TestCommandSettingsBuiltinText`
  PASS (20,330 s), cobrindo os três idiomas e fallback de ID desconhecido.

Por instrução do usuário, a migração deve parar ao encontrar suporte ausente,
em vez de adicionar adaptações que enfraqueçam o contrato existente.

- [ ] Criação/fechamento de abas: integrar operações assíncronas existentes ao
  ciclo de execução, com resultado, erro e alvo autorizado. O executor atual
  de efeitos de UI rejeita AsyncFunction e resultados Promise; disparar com
  `void` não comprova sucesso nem preserva o ciclo de execução.
- [ ] Teclado contextual: preservar Ctrl+T em textarea, prioridade de abas
  aninhadas para Ctrl+Tab e roteamento por painel. O ingresso atual descarta
  todo alvo editável; remover essa proteção global não é uma solução aceita.
- [ ] Sequências: preservar Ctrl+N seguido de C/E/R/T e cancelamento por modal;
  não confundir esse gesto com Ctrl+N de criação contextual em outras telas.
- [ ] Exceções explícitas: F1 sem modificador e ajuda com modal precisam de
  política própria; o mapa atual exige Control/Alt/Meta e bloqueia modal.

Evidência: `commandUIEffect.ts`, `commandUIExecution.ts`,
`commandLocalKeyboard.ts` e `useWorkspaceKeyboardShortcuts.ts` no frontend.
Nenhum listener legado desses comandos foi retirado. Nenhum novo comando
é declarado migrado nesta seção. Abrir a paleta por Ctrl+K é um candidato
simples a uma rodada posterior; não depende das operações de workspace,
mas ainda não foi migrado. AEP permanece In Progress, sem promover gates.

## 61. Primeiro efeito restrito ao painel ativo — 18/09/2026

O usuário autorizou preencher o contexto de interface antes das migrações.
Esta rodada conecta `workspace.panel.focus` ao foco do painel ativo, mantendo
as navegações globais anteriores. Não é migração de criar/fechar abas nem do
chat contextual: este último também pode criar/vincular conversa e não pode
ser declarado uma leitura visual apenas por abrir um modal.

Contrato do recorte:

- Capturar workspace, aba, tipo, owner, raiz DOM e handler de foco imediato
  antes do handoff. Revalidar o mesmo alvo; nunca selecionar substituto.
- Paleta indica indisponibilidade fora do workspace; teclado não consome
  uma combinação contextual recusada; Stream Deck obedece ao mesmo preparo.
- Um registro de foco adiado não prova efeito concluído. Usar capacidade
  separada de foco imediato, sem filas, timers ou espera por painel carregar.
- Preservar os nove atalhos padrão e listeners legados. O novo comando não
  recebe combinação padrão; aceita associações pessoais pelo editor existente.
- Executores de mutação assíncrona continuam pendentes. Não remover a recusa
  de Promise do executor visual nem usar `void` para simular sucesso.

Backend: `go test ./internal/app -run '^TestCommand(Product|Settings|Bootstrap|Deck|UI)' -count=1 -timeout 180s`
PASS (39,003 s), `go vet ./internal/app` PASS. O catálogo passa a 13 comandos,
mantendo os nove bindings padrão existentes.

- [x] Preparo de alvo por comando antes do Begin; revalidação antes do efeito,
  descarte em todos os desfechos e sem retarget durante o handoff.
- [x] Capability de foco imediato separada do pedido legado adiado; prontidão
  consultável sem efeito, ligada às quatro interfaces reais. TipTap usa
  `view.focus()` sem agendar `commands.focus()`.
- [x] Catálogo/handler/default de paleta e bindings pessoais de teclado/Deck.
  A paleta considera o contexto também na contagem anunciada ao leitor de tela.
- [x] Recusa física anterior ao Take libera a reserva já criada pelo host.
- [x] Matriz frontend: 14 arquivos, **354 testes PASS**; inclui 25 casos de
  contexto, efeitos reais das quatro interfaces, Topbar, settings, registro
  de foco e regressão dos atalhos. TypeScript e ESLint dos arquivos tocados PASS.
- [x] Picker real: mais 3 testes PASS em `Topbar.palette.integration.test.tsx`
  (357 testes frontend distintos no total). Regressão backend
  `^TestCommand(Keyboard|LocalKeyboard)` PASS (5,620 s).
- [ ] Aceite manual do novo comando com NVDA e hardware; passos no guia.

Próxima lacuna concreta (não implementada): `Manager.AddTab`, `RemoveTab` e
`SetActiveTab` operam sobre `m.active`, sem receber identidade/versão esperada
do alvo. O adapter de comando precisa conferir alvo e aplicar mutação sob a
mesma exclusão do Manager, sem janela entre verificação e escrita. O atual
`WithCommandSnapshot` mantém read lock e proíbe mutadores no callback: não
pode simplesmente envolver essas chamadas. Depois, ligar o executor backend
com resultado correlacionado e apresentação apenas no contexto ainda válido.
`createWorkspaceTab` também orquestra recursos de terminal; reutilização desse
contrato e compensação em falha não podem ser substituídas por uma criação de
aba isolada. Ctrl+N em sequência e roteamento de Ctrl+Tab continuam pendentes.

Sem Wails, geração manual de executável, testes ACP, banco real ou hardware
nesta rodada. Não promove gates globais nem conclui a migração do AEP.

## 62. Criar aba de chat pelo executor contextual — 18/09/2026

Recorte autorizado: `workspace.tab.chat.create`, com Ctrl+T padrão, paleta e
Stream Deck. Não conclui outros tipos de aba nem os demais atalhos. Substitui
a pendência de AddTab da seção 61 somente para criação de chat.

- [x] Escrita `HandlerBackend`, alvo mutável e fato `workspace/active_tab`
  ExactVersion. Não usa ContextNone nem confirma escrita como efeito visual.
- [x] `AddTabForCommand` compara identidade/versão e altera clone sob lock
  exclusivo; erro restaura memória/epoch. Retorno detached; temporário e retries
  canceláveis isolados desta operação. Não promete durabilidade crash-safe.
- [x] UI prepara alvo e revalida antes de submeter. Commit correlacionado de
  uso único no backend, com persistência fora do DispatchGate. Complete visual
  positivo recusado; sucesso somente depois da escrita real.
- [x] Ctrl+T permite input/textarea, não Monaco, modal ou IME. Evidência local
  de keydown não composing pode resolver unknown, sem apagar composição ativa.
  Listener Ctrl+T legado retirado; demais listeners preservados.
- [x] Evento atualiza a aba sem segunda AddTab. Snapshot de outro workspace
  é ignorado. O evento legado não tem versão: esse filtro não garante ordem
  nem proteção ABA de snapshots do mesmo workspace.
- [x] Frontend: **263 testes, 13 arquivos PASS**. Inclui Topbar, paleta real,
  teclado, submissão, contexto/IME, stores e regressão dos atalhos restantes.
- [x] `commandui` e `workspace` PASS; `wailsapi -run '^TestCommandCatalog'` PASS.
- [x] TypeScript, ESLint focado e `go vet` dos quatro pacotes alterados PASS.
- [x] Regressão final integrada do App, incluindo teclado e Deck: `go test
  ./internal/app -run '^TestCommand(Product|Settings|Bootstrap|Deck|UI|Keyboard|LocalKeyboard|WorkspaceChat)'
  -count=1 -timeout 180s` PASS (45,221s). Inclui origem auditada nas duas entradas,
  recusa de outros alvos mutáveis e supressão/restauração dos dez atalhos padrão.
- [ ] Aceite manual com NVDA e Stream Deck físico; roteiro em COMANDOS.md.

Catálogo: 14 comandos; mapa padrão: nove associações Alt preservadas e Ctrl+T.
Criar outros tipos, fechar/trocar abas, Ctrl+N em sequência e chat contextual
assíncrono seguem pendentes. Sem Wails/dev/build/generate, ACP/acpregistry,
banco real ou hardware. Não promove gates globais nem os 84 itens históricos.

Após reinício durante a atualização documental, três documentos ficaram com
conteúdo zerado. Restaurados pelo replay verificável do histórico de mudanças
concluídas, sem voltar à versão antiga do Git; nenhuma outra fonte alterada
inspecionada continha bytes nulos. O acompanhamento acima prossegue da seção 61.

## 63. Navegação a partir do campo de mensagem — 18/09/2026

Relato manual: criação de chat funcionou e o comando não executou fora do
contexto autorizado. Navegação por Alt+C/M/J e Deck falhava no campo de texto.

- [x] Teclado admite apenas IDs conhecidos de navegação em input/textarea nativos.
- [x] Deck admite navegação com composição unknown nesses campos, sem fabricar
  evidência inactive. O ID integra a preparação e a revalidação do efeito.
- [x] Composição ativa, modal, foco perdido/alterado, sessão e demais provas
  continuam bloqueantes. Contextuais/mutações e IDs futuros não herdam a exceção.
- [x] Regressões cobrem teclado, Deck, texto preservado, Monaco, AltGraph,
  repetição, keyCode 229, composição entre captura/commit e política estrita.
- [x] Vitest: 218 testes em 9 arquivos PASS; `tsc --noEmit` e ESLint dos seis
  arquivos alterados PASS. Sem alterações no backend nesta correção.
- [ ] Aceite manual da navegação no campo de mensagem, conforme COMANDOS.md.

Sem novos comandos ou mudança de escopo backend; nenhuma execução de Wails,
ACP, hardware ou banco real. Não promove gates globais do AEP.

## 64. Criação contextual de editor e lista de tarefas — 18/09/2026

Escopo: `workspace.tab.editor.create` e `workspace.tab.tasklist.create`, usando
o mesmo contrato de escrita de chat. Não migra terminal, Ctrl+N em sequência,
fechamento/troca de abas nem menus legados de criação.

- [x] Catálogo fechado dos três tipos, Write/Backend/Mutable/ExactVersion;
  tipo derivado do ID no backend, nunca fornecido pelo frontend.
- [x] Paleta, teclado pessoal e Deck ligados ao mesmo commit; somente chat
  conserva Ctrl+T padrão. Contexto visual/owner/aba continuam obrigatórios.
- [x] CAS, rollback e persistência para editor/tasklist; recusa explícita de
  terminal e estado de recurso não vazio nos novos tipos.
- [x] Executor frontend isola handoff/submissão por comando, inclusive sob
  concorrência entre tipos. Sem Complete visual positivo ou retry de escrita.
- [x] Títulos iniciais localizados e preservação de títulos existentes.
- [x] Frontend: 312 testes em 12 arquivos PASS; TypeScript e ESLint focado PASS.
  Inclui três tipos/origens, concorrência de handoffs, contexto, títulos,
  proteção de Monaco (incluindo textarea interna) e atalhos legados.
- [x] `go test ./internal/workspace -count=1 -timeout 120s` PASS; casos stale,
  duplicidade, concorrência e rollback parametrizados para os três tipos.
- [x] `go test ./internal/app -run '^TestCommand(Product|Settings|Bootstrap|Deck|UI|Keyboard|LocalKeyboard|WorkspaceChat|WorkspaceTab)' -count=1 -timeout 180s`
  PASS; `go vet` em app/workspace/commandui/wailsapi PASS.
- [ ] Aceite manual dos novos comandos com NVDA e Deck, roteiro em COMANDOS.md.

Catálogo: 16 comandos; dez atalhos padrão preservados. Terminal exige
orquestrar sessão e compensação e não é liberado por esta expansão. O evento
legado continua sem versão; não se declara resolvido seu risco de ordenação/ABA.

## 65. Fechar aba ativa pelo mecanismo contextual — 18/09/2026

Recorte: `workspace.tab.close`, Ctrl+W e Ctrl+F4 padrão, paleta e Stream Deck.
Não migra troca de abas/Ctrl+Tab, terminal, sequência Ctrl+N nem botões de fechar
do tablist. Preserva remoção apenas da aba, sem excluir recursos de domínio.

- [x] Alvo fechado à aba ativa do snapshot autorizado; Write/Backend/Mutable,
  ExactVersion e mesmo protocolo de commit de uso único.
- [x] Remoção e substituição da última aba por chat vazio em uma só persistência,
  com CAS, sucessora na mesma posição, rollback de memória/epoch e cancelamento.
- [x] Ctrl+W/Ctrl+F4 exclusivos do mapa novo; listeners legados retirados,
  sem fallback quando suprimidos ou mapa indisponível. DataGrid preserva prioridade.
- [x] Restauro de foco pós-sucesso separado da escrita: painel pronto ou botão
  da sucessora, sem fila que possa roubar foco após mudança de contexto.
- [x] Testes das três origens, última aba, stale/replay/cancel, falha persistência,
  foco, modais, campos editáveis e regressões dos atalhos não migrados.
- [x] Frontend: 351 testes integrados em 13 arquivos PASS; helper de foco
  revisado com 17 testes PASS (três adicionais; 354 distintos). Cobertura de
  fechamento/foco migrou do hook legado para Topbar e helper reais.
- [x] `tsc --noEmit`, ESLint focado e `git diff --check` escopado PASS.
- [x] `go test ./internal/workspace -count=1 -timeout 120s` PASS (4,025s);
  `go test ./internal/app -run '^TestCommand(Product|Settings|Bootstrap|Deck|UI|Keyboard|LocalKeyboard|WorkspaceChat|WorkspaceTab)' -count=1 -timeout 180s`
  PASS; `go vet` em app/workspace/commandui/wailsapi PASS.
- [ ] Aceite manual com NVDA/Stream Deck, roteiro em COMANDOS.md.

Catálogo: 17 comandos; 12 atalhos padrão. Sucesso vem da persistência
real; falta de alvo visual após sucesso não autoriza repetir a mutação.
O evento legado continua sem versão; nenhuma garantia nova de ordenação/ABA.

## 66. Família de navegação de abas e ordenação de snapshots — 18/09/2026

Entrega parcial validada: 11 comandos (`workspace.tab.next`, `previous`, `first`
até `ninth`), com paleta, Stream Deck e bindings pessoais. O catálogo passa
de 17 para 28 comandos; os 12 atalhos padrão permanecem intactos. Não promove
os 84 critérios históricos nem declara a migração do teclado concluída.

- [x] Catálogo fechado Write/Backend/Mutable/ExactVersion, sem argumentos de
  direção arbitrários. Posições de 1 a 9 e navegação circular.
- [x] Porta específica de commit recebe workspace, aba-fonte e destino
  capturados pela UI. Backend compara a fonte autorizada e resolve novamente
  o destino sob lock; divergência recusa, sem retarget. A porta genérica de
  criação/fechamento não admite navegação.
- [x] Persistência com CAS/rollback, cancelamento e resultado real. Posição
  ausente recusa; selecionar a aba atual é no-op sem gravação.
- [x] UI revalida rota, owner, root, IDs/ordem e aba capturados; foco pós-sucesso
  limitado ao destino esperado, com fallback imediato para seu botão e anúncio
  localizado. Modais, IME, Monaco, DataGrid e escopos aninhados preservados.
- [x] Regressão frontend final: **604 testes em 17 arquivos PASS**, incluindo
  legado, integração das três origens, snapshots e concorrência no store.
- [x] Regressão backend App (catálogo, readiness, commits, teclado pessoal,
  Deck simulado e guardas), workspace e controllers PASS. `wailsapi` com
  `^TestCommandCatalog|^TestWorkspace` PASS; vet dos cinco pacotes PASS.
- [x] Store versionado usa consumidor único para snapshots ativos, rejeita
  duplicatas/ordem antiga/epoch divergente/payload inválido e protege bootstrap
  contra respostas de sessão antiga. Testes imitam a ordem real de montagem
  (initialize antes de listeners), resposta antiga de switch, evento mais novo
  de outro workspace e transições A/B/A. TypeScript e ESLint focado PASS.
- [x] Persistência dos atalhos legados em FIFO limitado, mantendo mudança
  visual imediata; reconciliação autoritativa após sucesso/falha. Intenções
  pendentes são invalidadas na troca de workspace/sessão. Duas falhas não
  restauram uma intenção intermediária não persistida. Não migra esses
  atalhos para o executor de comandos nem generaliza sua segurança.
- [x] Medição diagnóstica reproduzível: `go test ./internal/app -run
  '^TestCommandWorkspaceTabNavigationLatencySample$' -count=1 -timeout 60s -v`.
  Amostra do parent com 20 ciclos no mesmo produto: total mediana **26,28 ms**,
  p95 **27,74 ms**. Por fase (mediana/p95): Begin 0,54/1,02 ms; Take
  17,64/18,91 ms; Commit 2,70/3,53 ms; Result 5,24/5,72 ms. Inclui App,
  autenticação/ledger e persistência local; não inclui IPC, renderização nem
  demonstra o orçamento experimental de 1 ms. Não é garantia de latência do
  produto nem asserção temporal em teste. A medição reforça manter o teclado
  rápido legado enquanto esse gate não for resolvido.
- [ ] Substituir Ctrl+Tab/Shift+Tab/PageUp/PageDown/1…9 somente depois de tratar
  ocorrências rápidas consecutivas e medir o caminho completo com IPC/render.
- [ ] Aceite manual NVDA/Deck dos comandos novos, roteiro em COMANDOS.md.

Ordenação entregue no recorte de snapshots: clones de transporte recebem epoch do Manager e
sequência decimal de publicação, capturadas sob lock. Não são versões de
autorização, não participam do CommandSnapshot e não são gravadas no YAML.
Eventos e retornos compartilham o consumidor monotônico; bootstrap
autoritativo ancora o epoch e não há comparação lexical entre epochs.
Atualizações locais legadas de `updateTab`, `reorderTabs`, rename e perfil
não foram convertidas em operações versionadas nesta rodada. A ordenação dos
snapshots não é uma prova de ordenação global de todas as operações do app.

Limite deliberado: os atalhos padrão de navegação continuam no caminho rápido
existente. A deduplicação/cancelamento do executor atual não demonstra que cada
pressionamento rápido consecutivo produzirá uma navegação distinta. Não se
resolve isso removendo autorização/ledger, executando antes da admissão ou
anunciando sucesso antes da persistência. Ctrl+N já tem infraestrutura de
sequências, mas ainda requer integração UI; terminal requer ciclo de sessão e
compensação. Esses itens permanecem abertos, não dependem de um novo teste do
usuário para continuar seu desenvolvimento.

## 67. Ocorrências rápidas e repetição de navegação — 18/09/2026

Registro histórico, substituído pela seção 68. O mantenedor aprovou repetição
automática seletiva para navegação; ações de criar/fechar continuam exigindo
novo pressionamento. A tentativa de enviar cada repetição ao host/ledger foi
abandonada: as pendências abaixo não são requisitos da solução local aprovada.

- [x] Corrigida leitura da aba ativa no caminho existente: cada Ctrl+Tab,
  Ctrl+Shift+Tab ou Ctrl+PageUp/PageDown consulta o store síncrono, sem esperar
  render React. Seleção por posição também consulta a lista atual.
- [x] Handler existente respeita evento consumido, IME/AltGraph e os
  modificadores exatos de navegação; preserva prioridade de grid/abas internas.
- [x] Contrato D3 atualizado com aprovação explícita para repeat seletivo,
  independente por ocorrência e validado no host. Não altera SO/Deck.
- [x] Ingresso host confirma repeat somente após down da mesma combinação,
  geração e código. Cada repeat elegível recebe ticket/UUID próprios; create e
  close continuam sem repeat. Testes `^TestCommandKeyboard` e vet App PASS.
- [ ] Fila local limitada, destinos imutáveis planejados e Down/Up ordenados;
  resultado desconhecido ou alteração externa invalida a continuação.
- [ ] Regressões integradas de bursts, tecla mantida e invalidação/ABA.
- [ ] Medição e revisão do custo backend sem remover auth/ledger/CAS.
- [ ] Migrar os padrões somente após validar o caminho integrado.
- [ ] Aceite de latência, foco e anúncios no aplicativo/NVDA.

Diagnóstico de custo (não profiling completo): o teste de 20 ciclos da seção
66 mediu mediana de 26,19 ms e Take de 17,92 ms. Um A/B removendo apenas
`refreshCommandJobProjection` mediu 26,03/17,86 ms, sem ganho demonstrado; a
alteração experimental foi revertida. Logs SQL e inspeção indicam custo no
percurso executor/ledger, mas não houve spans individuais de CAS nem profiling
de CPU. Uma amostra sob contenção não foi usada como comparação. Não declarar
otimização de latência nem remover garantias com base nesses números.

## 68. Navegação e apresentação sem auditoria por acionamento — 18/09/2026

Mudança arquitetural aprovada explicitamente pelo mantenedor. Substitui a
tentativa de serializar cada navegação pelo ledger da seção 67: uma fila de
transações não é a solução para uma ação visual local. Estado: implementação e
validação automatizada concluídas neste recorte; aceite manual pendente.
O AEP completo permanece In Progress; não promove os 84 itens históricos.

Escopo: 23 comandos de apresentação (onze de abas, nove destinos de tela,
menu, ajuda de atalhos e foco do painel). A classificação é uma lista fechada
do produto, não uma flag controlada pelo usuário nem uma dispensa para todo
comando Read/UI. Os quatro comandos de criação/fechamento e a consulta
`workspace.list` preservam o fluxo existente neste recorte; não se afirma
que qualquer consulta ou alteração comum exija auditoria por princípio.

- [x] Contrato aprovado: separar apresentação, operação de domínio e execução
  durável; auditoria não é requisito universal do catálogo.
- [x] Publicar mapa de teclado e seleções efetivas da paleta para apresentação
  local, respeitando camadas, overrides e supressões.
- [x] Executar apresentação sem Begin/Take/Commit/Result por acionamento.
- [x] Migrar Ctrl+Tab/Shift+Tab/PageUp/PageDown/1…9, retirando handlers legados
  concorrentes; repeat restrito à navegação de abas.
- [x] Deck entrega evento visual efêmero validado, sem invocação persistente.
- [x] Guardas de owner, foco, modal, IME, rota, grid e abas internas preservadas.
- [x] Persistência da última seleção coalescente, workspace/aba explícitos,
  rollback/reconciliação e isolamento de sessão/workspace.
- [x] Mutação dependente aguarda a seleção e revalida o alvo capturado.
- [x] Remover tentativa de fila auditada substituída, sem código morto.
- [x] Testes provam zero linhas de ledger/auditoria para apresentação e
  preservação das proteções de comandos duráveis.
- [x] Regressões Go/frontend, tipos e lint focados.
- [ ] Aceite manual de fluidez, repetição e anúncios NVDA/Deck.

Evidência automatizada final (18/09/2026):

- Vitest: **541 testes em 25 arquivos PASS**, incluindo Topbar (196), store e
  snapshots integrados (57), teclado/repeat, contexto, executores, foco e rollback.
- Regressões App de Product/Settings/Bootstrap/Deck/UI/Keyboard/Workspace,
  Palette/Resolution/ExecutePalette/LocalPalette/GetPalette PASS (42,919 s).
- `go test ./internal/workspace ./controllers ./internal/wailsapi` PASS;
  `go vet` desses pacotes e App, `tsc --noEmit` e ESLint focado PASS.
- Casos fechados: bursts sem IPC de invocação, supressão/remapeamento sem
  ressuscitar seleção de paleta, geração/owner obsoletos, falha de gravação e
  reconciliação, confirmação inválida, recuperação após erro de transporte,
  logout com request pendurada sem bloquear a sessão nova, Escape durante a
  barreira de persistência e foco tardio cancelado após mudança de contexto.
- Removidos o commit backend de navegação e os branches genéricos auditados
  sem consumidor na Topbar. A biblioteca compartilhada de handoff permanece
  porque é usada pelas quatro mutações contextuais, não como fallback local.
- A corrida do teste de sessão foi corrigida sincronizando a admissão antes
  de trocar a identidade; a regressão da paleta confere 25 padrões + 1 pessoal.

Não foram executados Wails dev/build/generate, ACP/acpregistry, testes físicos
ou operações no banco real. Não há nova medição de latência fim a fim nem
aceite NVDA/Deck. Roteiro com teclas e checkboxes em
`docs/content/recursos/COMANDOS.md`, seção de navegação. Este fechamento não
promove o AEP completo nem reconta os 84 critérios históricos.

## 69. Aceite de fluidez e unificação da paleta com o picker — 18/09/2026

O mantenedor confirmou o funcionamento dos comandos, velocidade perfeita e
Stream Deck funcionando. Reportou duas pendências: Ctrl+K no campo de texto e
navegação errática da paleta. Esclareceu que espera o mesmo picker usado para
modelos, não o Menu com busca empregado até aqui.

- [x] Aceite manual relatado de fluidez e Stream Deck do recorte 68.
- [x] Ctrl+K em input/textarea preserva rascunho, composição e contexto.
- [x] Paleta usa o Combobox compartilhado com ModelPicker; sem navegação paralela.
- [x] Busca, setas, seleção, indisponíveis e foco cobertos na integração real.
- [x] Regressões dos pickers existentes, tipos e lint.
- [x] Aceite manual da paleta após esta correção. O usuário confirmou:
  “pode continuar. validamos. se aparecer algo errado arrumamos.”

Estado: implementado, validado automaticamente e aceito manualmente pelo
usuário. Não extrapola o relato para todos os cenários do roteiro NVDA nem
promove o AEP completo a Done.

Evidências: **314 testes em 15 arquivos PASS**, cobrindo Topbar, integração
real do Combobox, guardas de efeito, todos os pickers e menus compartilhados;
TypeScript e ESLint focado PASS. A integração real percorre opções com
`aria-activedescendant`, preserva rascunhos input/textarea, consulta opções
indisponíveis sem executar, filtra e executa uma ação, abre Alt+M com/sem
paleta e impede restauração atrasada de foco após mudança de sessão.

O Combobox mantém o modo não controlado dos demais pickers. A paleta controla
a abertura somente após revalidar o catálogo/contexto; carregamento usa
aria-busy, não desabilita o próprio botão. Foi removido o Menu da paleta;
outros menus continuam usando o componente existente. Um erro de hover em
itens desabilitados desse Menu também recebeu correção e regressão focada.
Sem Wails dev/build/generate, testes Go/ACP ou alterações de banco nesta rodada.

## 70. Implementação validada de `workspace.tab.terminal.create` — 18/09/2026

Recorte: `workspace.tab.terminal.create` como pré-requisito para uma futura
migração de Ctrl+N. O frontend integrado atende as três origens previstas com
admissão contextual, invalidação de owner/sessão/workspace e commit único.
O lifecycle e a compensação backend seguem o AEP-0089, mas seus testes com
manager fake são gates distintos da validação integrada no App.

- [x] Frontend integrado registra `workspace.tab.terminal.create` nas três origens, sem
  ativar um novo default de teclado.
- [x] Manter o catálogo em 29 comandos, com 23 de apresentação local e 25
  bindings padrão.
- [x] Deduplicar `session_created`, preservar histórico e recarregar a sessão
  numa surface existente quando o evento for perdido, sem recriar o terminal.
- [x] Cobrir no frontend admissão contextual, identidade e invalidação de
  owner/sessão/workspace, com commit único. O vínculo observado é
  `state.sessionId`; `SessionInfo.id` identifica a sessão criada.
- [x] Manter testes backend isolados com manager fake para criação/persistência,
  falhas, cancelamento antes/durante, sessão morta/vazia, erro de criação e
  falha de cleanup.
- [x] Regressão do App, catálogo e readiness fail-closed. O caminho produtivo
  usa o manager existente e preserva o cwd do controller; criação e compensação
  foram exercitadas com sessões simuladas e workspace real de teste, sem PTY.
  A fixture operacional agora inicializa o manager sem iniciar processos;
  um teste separado mantém a prova de indisponibilidade quando ele está ausente.
- [ ] Demonstrar no aceite manual que fechar uma aba desconecta a visualização
  sem encerrar o terminal, conforme o AEP-0089.
- Ctrl+N e seu novo default ficam fora do escopo desta seção, para a próxima
  sequência; não são um gate de conclusão deste recorte.

Estado: implementado e validado automaticamente; somente o aceite manual do
novo terminal permanece pendente. Evidência: frontend **317 testes em 10
arquivos PASS**, TypeScript e lint PASS; pacote `workspace` completo PASS
(1,716 s), `go vet` de App/workspace PASS e regressão App PASS (53,621 s):
`go test ./internal/app -run 'Test(CommandProduct|CommandKeyboard|CommandWorkspace|CommandDeck|CommandUI|UICommand|UINavigation|CommandCatalogTerminal|CreateTerminalSessionAndCommit)' -count=1 -timeout=180s`.
Sem Wails, ACP, PTY real ou banco real. O AEP permanece In Progress.

### Roteiro manual pendente

- [ ] No workspace, abrir Ctrl+K, escolher **Criar aba de terminal** e
  confirmar uma nova sessão.
- [ ] Configurar uma tecla pessoal e uma tecla do Stream Deck para o mesmo
  comando; cada invocação deve criar uma nova sessão.
- [ ] Fora do workspace, confirmar que o comando fica indisponível.
- [ ] Fechar a aba e confirmar que a sessão do terminal não é encerrada.

## 71. Ctrl+N e menu de criação pelo mecanismo de comandos — 18/09/2026

Escopo autorizado: migrar Ctrl+N seguido de C/E/R/T e o botão Nova aba,
preservando os comandos contextuais das demais telas. Não migra CRUD de
configurações nem cria um handler alternativo de escrita.

- [x] Gramática v2 estrita de dois passos; identidades canônicas, round-trip e
  rejeição de documentos inválidos. V1 e fingerprints existentes preservados.
- [x] Quatro defaults, resolução de camadas/supressão e origem keyboard.local
  no executor contextual, sem invocação para o prefixo.
- [x] Adapter local único: timeout, repeat, composição, Escape, blur,
  cancelamento e invalidação da geração; release ordenado com a admissão.
- [x] Menu compartilhado com seleção pela paleta especializada; remover
  criação direta e o evento global legado, mantendo foco e alvo originais.
- [x] Preservar Ctrl+N das outras telas, grids, editor, modais e campos;
  invalidar escolhas atrasadas após mudança de contexto.
- [x] Testes integrados das quatro letras e do menu; regressões frontend,
  backend, TypeScript, lint e vet.
- [ ] Aceite manual com NVDA no aplicativo.

Estado: implementado e validado automaticamente; aceite manual pendente.
O catálogo permanece com 29 comandos e 23 comandos de apresentação local;
o mapa padrão tem 29 bindings (25 v1 preservados e quatro sequências v2).
A captura pessoal continua v1; a tela mostra sequências padrão sem convertê-las
em combinação simples e permite suprimir/restaurar cada padrão.

Evidências de 18/09/2026:

- Frontend: **449 testes em 14 arquivos PASS**, incluindo Topbar + Toolbar +
  Menu reais, quatro commits por teclado, clique e setas/Enter, resposta de
  catálogo atrasada, timeout, modal ABA e ausência de restauração tardia de foco.
- `npx tsc --noEmit` e ESLint focado em produção/testes: PASS.
- `go test ./internal/commandconfig -count=1 -timeout=120s`: PASS.
- `go test ./internal/app -run '^TestCommand(Product|Settings|Bootstrap|Deck|UI|Keyboard|LocalKeyboard|WorkspaceChat|WorkspaceTab)' -count=1 -timeout=180s`: PASS.
- `go vet ./internal/app`: PASS. A fixture Bootstrap sem TerminalManager
  verifica indisponibilidade somente do terminal; demais 28 comandos exigidos.
- Nenhum Wails, ACP, PTY real, banco real ou executável manual foi usado.

O roteiro manual está em `docs/content/recursos/COMANDOS.md`, seção
“Criar abas: Ctrl+N e menu Nova aba”. AEP completo permanece In Progress;
esta evidência não declara todos os 84 critérios nem os aceites físicos concluídos.

## 72. Paleta e navegação para Dados no mapa efetivo — 18/09/2026

Lote autorizado para validação manual posterior junto da seção 71.

- [x] Catálogo pt/en/es e defaults para `navigation.palette.open` (Ctrl+K),
  `navigation.data.export.open` (Alt+E) e `navigation.data.import.open` (Alt+I).
- [x] Execução local sem ledger/handoff; suporte por teclado, paleta e Deck,
  respeitando mapa, proprietário, sessão, foco, composição e modais.
- [x] Remover listeners legados concorrentes; supressão e remapeamento têm
  efeito real. Botão da paleta permanece disponível sem o atalho padrão.
- [x] Reutilizar Combobox da paleta e fluxos existentes de Dados. Importação
  abre a seleção de arquivo, mas não grava dados sem confirmação; exportação
  leva à seção existente sem exportar automaticamente.
- [x] Regressões de tipos, lint, catálogo, projeção e interface integradas.
- [ ] Aceite manual conjunto com Ctrl+N, NVDA e Stream Deck.

Estado: implementado e validado automaticamente. Catálogo com **32 comandos**,
**26 locais**, **32 bindings** (28 v1 + quatro v2); demais fingerprints
preservados. Ajustada também a admissão de Shift junto de Control/Alt/Meta
nas sequências, conforme a gramática já aceita; Shift sozinho continua recusado.

Evidências: **474 testes frontend em 14 arquivos PASS**, TypeScript e ESLint
focado PASS. Backend: defaults/supressão, classificação, rejeição de handoff
para navegação sem criar ledger e contrato de Shift PASS; `go vet ./internal/app`
PASS. Nenhum Wails, ACP, PTY real ou banco real executado.

Regressão App consolidada final: PASS (32,177 s),
`go test ./internal/app -run '^Test(Command(Product|Settings|Bootstrap|Deck|UI|Keyboard|LocalKeyboard|WorkspaceChat|WorkspaceTab)|UINavigation)' -count=1 -timeout=180s`.

Aceite manual das seções 71–72 continua pendente, por escolha do mantenedor;
roteiro unificado em `docs/content/recursos/COMANDOS.md`. F1 permanece separado: seu comportamento legado
de ajuda durante modais exige contrato explícito antes da migração. Não altera
os demais atalhos contextuais nem declara o AEP integralmente concluído.

## 73. F1 efetivo e fronteiras dos próximos comandos — 18/09/2026

Estado: implementado e validado automaticamente; aceite manual agrupado com 71–72.

- [x] Publicar F1 sem modificadores para `navigation.help.open`, preservando
  os defaults anteriores e a recusa de outras teclas simples sem modificadores.
- [x] Remover execução legada de F1; supressão/remapeamento usam o mapa efetivo.
- [x] Exceção modal somente para ajuda pelo teclado local, nunca por apenas
  usar a tecla F1. Deck/paleta e comandos de fundo permanecem bloqueados.
- [x] Regressão consolidada de mapa, host, configurações, tipos e backend.
- [ ] Aceite manual de F1, personalização e modal com NVDA.

Lacunas concretas para o lote seguinte, sem marcar migração incompleta como pronta:

- [x] **Ctrl+Shift+N / novo workspace (concluído na seção 74):** registrar comando de escrita no executor
  comum; persistir workspace e índice com erro explícito/compensação, revalidar
  sessão/runtime/cancelamento antes do commit. `Manager.Create` atual não recebe
  contexto e `touchIndex` não propaga falhas. Testar falha de índice, cancelamento,
  troca de sessão e ausência de troca automática do workspace ativo. Só então
  remover o listener legado e ligar teclado/paleta/Deck.
- [x] **Ctrl+Shift+I / chat contextual (implementado na seção 75):** fixar alvo e lifecycle na preparação
  assíncrona. Criar/vincular conversa é uma escrita: não classificar o fluxo
  inteiro como `local_ui` nem contornar o executor com Wails direto. Separar a
  preparação/apresentação da fronteira durável antes de publicar o comando.
  Preservar reserva de DevTools, readonly, foco e adaptadores das três superfícies.
- [x] Impedir abertura visual após cancelamento durante `prepare` ou durante
  criação/vínculo, troca ABA de workspace/aba/modal e substituição do adapter.
  Solicitação antiga não invalida a nova; assinatura removida em `finally`.

A correção de abertura atrasada do store é independente da migração completa:
não cancela nem desfaz uma escrita backend já iniciada. O teste manual fica
agrupado; o AEP integral permanece In Progress.

Evidências: **516 testes frontend em 15 arquivos PASS**; `tsc --noEmit`, ESLint
focado e `git diff --check` dos arquivos tocados PASS. Regressão App ampla
PASS (36,393 s) e `go vet ./internal/app` PASS. Testes adicionais
`TestCommandKeyboardBareF1Admission` e
`TestCommandKeyboardHelpDefaultPublishedAsLocalUI` PASS (19,976 s), incluindo
supressão/restauração pela API real. Os 28 fingerprints v1 anteriores são
verificados por valores SHA-256 fixos. Nenhum Wails, ACP, PTY real ou banco
real executado; nenhum executável gerado manualmente.

## 74. Criação de workspace no mecanismo comum — 18/09/2026

Estado: implementado e validado automaticamente. Aceite manual em lote pendente.
Não inclui migração de Ctrl+Shift+I.

- [x] `workspace.create` no catálogo pt/en/es, classificação de escrita,
  default Ctrl+Shift+N e origens teclado/paleta/Stream Deck.
- [x] Persistência do novo workspace e publicação do índice com erros
  explícitos; falha anterior à publicação compensa somente o diretório novo.
  Preservar bytes do índice anterior, workspace ativo e `LastOpened`.
- [x] Handoff e commit únicos, principal/sessão/geração/snapshot de origem
  revalidados; recusar transporte direto sem reserva, replay e origem obsoleta.
- [x] Interface das três origens e Novo workspace dos dois menus usam o mesmo comando;
  retirar listener legado de Ctrl+Shift+N. Sem repetição automática nem troca
  implícita de workspace. Disponível fora da tela principal, com contexto pronto.
- [x] Regressões de persistência, cancelamento, isolamento e interface; tipos,
  lint, testes App/workspace e vet.
- [ ] Aceite manual em lote com NVDA e Stream Deck.

Contrato: a origem é o workspace/aba versionados de onde partiu a solicitação;
não é o novo destino. O evento `workspace:created` atualiza a lista, sem emitir
`workspace:switched`/`workspace:tab_added`. A porta existente de commit é
reutilizada sem adicionar bindings Wails manuais. Publicar o índice é o ponto
de sucesso; cancelamento posterior não deve desfazer um workspace publicado.
Não promete atomicidade entre arquivos em queda do processo nem reconciliação
entre instâncias externas escrevendo o mesmo índice.

Evidências: **589 testes frontend em 17 arquivos PASS**, incluindo Menu/Toolbar
reais, feedback acessível de teclado/Deck e invalidação durante handoff.
TypeScript e ESLint focado PASS; App amplo PASS (37,780 s), pacote workspace
PASS e `go vet ./internal/app ./internal/workspace` PASS. Teste Windows abre
o índice sem `FILE_SHARE_DELETE`: comprova erro de publicação, preservação
dos bytes anteriores e compensação do diretório novo. Índice ausente/inválido
também recusa criação; reload conserva o workspace ativo. Os 29 fingerprints
v1 anteriores permanecem fixos. A disponibilidade real do catálogo inclui
`workspace.create` sem ampliar o preflight somente-leitura de outros comandos.
Nenhum Wails, ACP, PTY real ou banco real executado; nenhum executável gerado
manualmente. Checklist de aceite em `docs/content/recursos/COMANDOS.md`, seção
Criar workspace. AEP integral In Progress; próximo lote: Ctrl+Shift+I.

## 75. Chat contextual no mecanismo comum — 18/09/2026

Estado: implementado e validado automaticamente.
Aceite manual agrupado com os lotes anteriores, sem declarar o AEP concluído.

- [x] Registrar `workspace.chat.open`, default Ctrl+Shift+I e origens teclado,
  paleta e Stream Deck; os botões existentes delegam ao mesmo host de comandos.
- [x] Separar preparação local dos adapters de editor/terminal/tasklist da
  criação/vínculo autenticados. Conteúdo selecionado não vai aos argumentos
  do comando nem ao ledger; abrir não envia mensagem.
- [x] Capturar workspace/aba versionados e revalidar sessão/runtime no commit.
  Reutilizar conversa pertencente ao usuário; recusar referência inválida ou
  de outro usuário. Em aba de chat, apenas apresentar foco, sem nova conversa.
- [x] Persistir vínculo por CAS e compensar somente a conversa recém-criada
  se a persistência falhar. Não substituir vínculo preexistente.
- [x] Abrir modal somente após sucesso confirmado e releitura do snapshot
  pela API existente; sem transporte novo ou bindings gerados à mão.
- [x] Invalidar preparação/apresentação após cancelamento, troca de contexto,
  sessão, modal, foco ou adapter; respeitar editor readonly e composição.
- [x] Retirar execução legada; manter reserva de DevTools sem impedir o
  dispatcher novo. Permitir supressão/remapeamento, sem autorepeat.
- [x] Consolidar regressão da interface, backend, tipos e lint.
- [ ] Aceite manual de Ctrl+Shift+I, paleta, botões, NVDA e Stream Deck.

Catálogo: **34 comandos, 26 locais e 35 bindings (31 v1 + quatro v2)**.
Classificação conservadora de escrita, pois pode criar e vincular conversa;
não altera a política sem auditoria dos comandos locais de navegação.
Evento `workspace:conversation_bound` aplica snapshot sem troca de aba ou
anúncio de perfil. A compensação trata falha em execução, não promete
atomicidade SQLite/YAML resistente a encerramento abrupto do processo.

Roteiro manual em `docs/content/recursos/COMANDOS.md`, seção Abrir chat contextual.
Sem Wails, ACP, PTY real, banco real ou executáveis gerados manualmente.

Evidências frontend finais: **690 testes em 22 arquivos PASS**; `tsc --noEmit`,
ESLint focado e `git diff --check` dos arquivos rastreados tocados PASS.
Inclui picker real, Topbar junto do listener legado reservado, Deck em
Monaco/contenteditable sem tecla prévia, botão cancelado por Escape, snapshot
readonly e preparação descartada quando o adapter real muda durante o await.
A reserva de DevTools roda em capture no document, depois do dispatcher em
capture no window e antes de um editor poder interromper a propagação.
Os testes das três superfícies e do modal compartilhado integram essa contagem.

Backend: suíte App `^Test(Command|UINavigation)` PASS; pacote workspace completo
PASS (1,940 s) e `go vet ./internal/app ./internal/workspace` PASS. Inclui teclado
e Deck via reservas reais, auditoria da origem, reutilização por proprietário,
recusa de conversa alheia, replay, cancelamento, stale e compensação em falha
de arquivo. A revisão cruzada não identificou defeitos adicionais de lock order,
ownership ou compensação. Atualizadas fixtures antigas de catálogo/bootstrap
para readiness de terminal sem PTY, contagens de camadas/defaults e protocolo
de commit contextual; nenhuma verificação de produção foi afrouxada.


## 76. Reconciliação de implementação e aceite — 18/09/2026

Esta é uma reconciliação documental, não um novo pacote de implementação.
Baseline inspecionada: branch `feat/aep-0103-comandos`, HEAD
`7e88945ec585e4352c4548aad8cd14e4db0134c1` **mais alterações locais até a seção 75**.
O HEAD sozinho não reproduz as entregas ainda não commitadas.

### Método e resultado

Leitura principal do AEP, matriz, código e testes; três frentes Luna somente
leitura conferiram R01–R04, R07–R10 e R05/R06/R11/R12. A integração principal
corrigiu classificações excessivamente otimistas: R07.2 ainda tem famílias
legadas; R09.2 não expõe CRUD completo de regras/camadas; R09.4 não oferece
edição textual completa de imagem/título/estados. Wails não é uma tool de chat,
logo R11.1 não está entregue. Em contrapartida, R05.3 já tem implementação
pública comum desde 44–45; qualificação pendente não apaga esse código nem se
confunde com export sensível de R05.4.

- C: **58 I, 24 P, 2 N = 84**. Implementação identificada: **69,0%** dos
  critérios, sem ponderação por esforço. Critérios compostos só recebem I
  quando não foi identificada lacuna funcional no próprio texto; a nota mantém
  limites de teste/aceite explícitos. Não é previsão de prazo nem recertificação.
- R: **11 A, 5 I, 29 P, 3 N = 48**. A+I = **16/48 (33,3%)** saídas maiores
  com implementação integral identificada; somente A está com checkbox x.
- Gates: **R04 aceito (1/12)**; R01 em validação; dez outros abertos.
- I: **84 itens históricos**, sem nova promoção automática; R06.1 continua
  exigindo requalificação. P: **20 itens** preservados e mapeados, não somados.
- AEP contém **84** checkboxes finais após a alteração aprovada da seção 68,
  não 83. C84 recebe a cláusula nova já existente; C01–C83 não mudam de ID.
  Os dois conjuntos com 84 elementos (infraestrutura e critérios finais) são
  distintos e sua igualdade numérica é coincidência.

Não houve novo build/teste, hardware, consulta ao banco pessoal, commit, push,
PR ou mudança funcional nesta rodada. Evidências são das execuções registradas;
em especial seção75: 690 testes frontend/22 arquivos, App ampliado, workspace,
tipos, lint e vet. Isso não substitui race, Bugbot ou CI completo atual.

A conferência final reduziu a leitura preliminar de 61 para **58** critérios:
C28 requer fechar a obrigação composta de retenção/replay; C51 inclui prova de
queda real não coberta pela remontagem; C62 tem cache testado, mas sem caller
produtivo localizado. Essas lacunas não foram convertidas em “só aceite manual”.

### Achados da revisão histórica

Os registros de entrega já tratam os nove achados F01–F09: F01/F02/F03/F06/F07
na seção10; F04/F05/F09 nos refinamentos da paleta, culminando nas seções53/69;
F08 é a correção da contagem, atualizada nesta seção. Isso não equivale a zero
defeitos no projeto nem fecha os riscos H01–H05 e os gates de qualificação.
Não repetir como trabalho futuro “corrigir F02/F03” quando falta, na verdade,
a matriz completa do recurso do qual fazem parte.

### Frentes restantes, com condição objetiva de chegada

1. **Catálogo e migração (R07):** inventário por família sem `candidate.*`
   indevidamente apresentado como comando real; migrar chat/editor/tarefas/
   terminal/perfis/jobs e retirar listeners concorrentes. Cada ação conserva
   escopo, foco, IME e classificação local/durável. Não auditar navegação.
2. **Configuração completa (R02/R09):** escopos, regras/condições, CRUD completo,
   diagnóstico de conflitos e argumentos; restore por camada/tudo,
   needs_review/rebase, prioridades e ajuda derivada do mapa. Chegada:
   configurar, reiniciar, restaurar e explicar a resolução no App.
3. **Paleta restante (R08.2):** argumentos, atalho efetivo, recentes/favoritos e
   acesso à configuração; manter o Combobox e acessibilidade já aceitos.
4. **Dispositivo/contexto externo (R10):** apresentação configurável, navegação
   entre camadas/pastas, status/diagnóstico e estabilização por programa.
   Chegada: matriz física de reconexão/lock/logout/shutdown e isolamento.
5. **Tools/CLI/delegação (R05/R11/R03.4):** entrypoints públicos com contratos
   fechados e qualificação do ciclo comando→job→evento→comando. Bibliotecas
   internas não contam como entrega pública.
6. **Portabilidade avançada (R05.4/R11.3, prioridade adiada):** export sensível
   separado com decisão/criptografia, import conjunto autorizado e tools
   permitidas. O fluxo comum já está implementado, não deve ser refeito.
7. **Qualificação/encerramento (R06/R12):** matriz I/C, desempenho representativo,
   regressão global em ambiente aprovado, race/review/CI e roteiros manuais
   agrupados. Só então BASE-PRONTA/AEP-CONCLUÍDO conforme os respectivos gates.

Os parciais têm trabalho de implementação e integração real. Não se afirma que
os 26 critérios restantes sejam todos pequenos, nem que baste executar os
checklists para concluir o AEP.

## 77. Fronteira da próxima migração — 18/09/2026

Inspeção de código, sem alteração funcional ou execução de testes. Contagem
preservada: 58 I / 24 P / 2 N; nenhum critério promovido por este levantamento.

O próximo lote pretendido de chat não pode retirar os listeners atuais sem
completar o escopo de apresentação contextual. `ChatToolbar.tsx` abre os
pickers de modelo/histórico/perfil (Ctrl+M/H/P) tanto no chat da aba quanto
no chat modal topmost. `Topbar.tsx:executeLocalUICommand`, o ingresso de
teclado e o ingresso visual do Deck recusam modais, salvo a exceção estreita
de ajuda pelo teclado. `commandBridge.ts:DialogCommandScope` e
`modalRegistry.ts:cloneDialogCommandScope` só admitem `decision.respond` e
seu trigger invariante; não publicam escopo de comandos do chat modal.

Isso é implementação ainda ausente do escopo previsto em D7, não autorização
para remover a barreira modal. Migrar apenas o chat da aba e manter fallback
legado no modal deixaria dois caminhos e não concluiria essa família.

### Próximo lote proposto, antes de retirar os listeners

- [ ] Registrar a superfície de chat e seu alvo ativo de apresentação, com
  identidade de instância, workspace/aba/conversa e descarte no unmount.
- [ ] Completar o escopo do chat modal topmost para comandos explicitamente
  permitidos, preservando os invariantes de DecisionDialog e sem fallback
  para comandos do chat de fundo. Revalidar stack/geração, foco e IME.
- [ ] Publicar os três comandos de abertura de picker e seus defaults;
  teclado, paleta e Deck usam a mesma ação local. Abrir o picker não escolhe
  modelo/perfil/conversa nem autoriza essa mutação.
- [ ] Provar remapeamento/supressão, keep-alive, campo de envio, overlays,
  modal sobre modal, origem obsoleta e ausência de execução duplicada;
  retirar os listeners correspondentes somente depois dessa equivalência.

Outras ações não são desbloqueadas automaticamente por esse lote:
`ChatToolbar.tsx:handleClearConversation` exclui conteúdo (Ctrl+L);
`useEditorMenus.tsx` salva arquivos e persiste modo; `TaskListsPage.tsx`
compõe criação de recurso e abertura de aba; `terminalStore.ts` distingue
envio/interrupção/encerramento de sessão de simplesmente fechar sua aba.
Cada uma exige contrato específico de domínio, alvo e efeitos; não pode ser
registrada como apresentação local. Ações já migradas das seções 60–75
permanecem intactas. A definição desses próximos contratos não está entregue.

Pausa antes da implementação dessa ampliação, conforme a orientação do
mantenedor de explicitar lacunas antes de adaptar comandos não suportados.
Autorização recebida na rodada seguinte; execução acompanhada na seção 78.

## 78. Pickers de chat no dispatcher e escopo modal — 18/09/2026

Estado: implementado e validado automaticamente; aceite manual pendente.
Nenhum critério global promovido por este sublote de R07/C78:
58 I / 24 P / 2 N, gates anteriores preservados.

- [x] Publicar `chat.model.open`, `chat.history.open`, `chat.profile.open`,
  defaults Ctrl+M/H/P e tradução pt/en/es; apresentação local sem ledger.
- [x] Registrar chat de página/modal com allowlist fechada, owner e instância;
  preservar barreira topmost e scope de decisão sem permissões adicionais.
- [x] Teclado/paleta/Deck usam os pickers existentes; retirada dos listeners
  M/H/P; Ctrl+L permanece legado porque limpa conteúdo de domínio.
- [x] Captura da instância antes da busca da paleta; descarte se muda contexto,
  owner/conversa/modal e revalidação síncrona imediatamente antes da abertura.
- [x] Regressões de supressão/remapeamento, modal sobre modal, input, IME,
  repeat, keep-alive, disponibilidade e ausência de ledger/dupla execução.
- [ ] Aceite manual agrupado de teclado, NVDA e Stream Deck, conforme
  `docs/content/recursos/COMANDOS.md`.

Limite temporal explícito do Deck: como os demais comandos de apresentação,
o alvo visual é o contexto vigente no recebimento do evento na UI. O payload
contém owner/workspace/geração do mapa, não um snapshot DOM do instante físico
da tecla. Captura, revalidação e abertura são síncronas nesse recebimento;
nenhuma seleção de modelo/perfil/conversa é aplicada pelo comando. A lease
da paleta, por sua vez, atravessa a busca e não pode redirecionar a ação para
outra instância. Não confundir esses dois instantes nem prometer snapshot
físico que o protocolo não transporta.

Evidências: 488 testes frontend em 13 arquivos PASS, incluindo Topbar,
paleta/Combobox real, ChatToolbar, registry de chat/modal, Modal,
DecisionDialog, ChatSessionView, WorkspaceChatModal, bridge/contexto e
teclado/repeat. `npx tsc --noEmit` e ESLint dos arquivos alterados PASS.
`go test ./internal/app -run '^Test(Command|UINavigation)' -count=1
-timeout=180s` PASS (122,256 s); `go vet ./internal/app` PASS com cache
isolado após erro de acesso no cache global. Catálogo 37 / local UI 29 /
bindings 38 (34 v1 + 4 v2), preservando os 31 defaults v1 anteriores.
Os avisos React `act` e AudioContext do ambiente jsdom não falharam a suíte.
Sem execução de Wails, ACP, hardware real ou banco pessoal nesta rodada.

## 79. Menus e apresentação do editor — 18/09/2026

Estado: implementado e validado automaticamente; sem promoção automática dos
58 I / 24 P / 2 N. Sublote de migração R07, não fechamento de toda a família.

- [x] Publicar `editor.menu.file.open`, `editor.menu.format.open`,
  `editor.menu.mode.open`, `editor.slides.open` e
  `editor.presentation.fullscreen` como apresentação local sem ledger.
- [x] Preservar Alt+S/F5 no mapa efetivo; retirar seus listeners antigos.
  Os outros três comandos não recebem defaults inventados.
- [x] Usar controles existentes do editor ativo nas três origens, recusando
  instância/documento/owner obsoletos, modal, overlay, composição e repeat.
- [x] Capturar o alvo antes da busca na paleta e não redirecionar a seleção.
- [x] Passar testes de catálogo/defaults/Deck, UI real, tipos e lint.
- [ ] Aceite manual agrupado conforme `docs/content/recursos/COMANDOS.md`.

Lacunas explicitamente fora deste lote:

- Alt+I/Inserir compartilha a tecla com `navigation.data.import.open`.
  `GetLocalCommandKeyboardMap` publica somente resoluções globais sem fatos
  requeridos, e o frontend aceita uma ação por combinação. Migrar preservando
  ambos exige projeção contextual de candidatos/supressões e precedência, não
  renomear o comando global ou inventar fallback no frontend. Alt+I permanece
  como antes; Inserir não foi anunciado como comando migrado.
- Ctrl+S/O, Ctrl+Shift+S, Alt+1/2/3 e os itens que formatam/inserem/criam
  slides têm efeitos de domínio/persistência. Abrir seus menus não migra essas
  ações. Precisam de contratos próprios de alvo, autorização e resultado.
- Fullscreen reutiliza `RevealRenderer` e depende da API/permissão do WebView;
  ingresso Deck não inventa um gesto confiável. Confirmar fisicamente se o
  ambiente aceita essa origem; rejeição mantém a apresentação embutida.

Validação final: **481 testes frontend em 13 arquivos PASS**, incluindo os
cinco comandos acionando controles reais do EditorPage, paleta/dispatcher,
remapeamento/supressão, isolamento de documento e regressões de chat/modais.
`npx tsc --noEmit`, ESLint dos arquivos alterados e `git diff --check` PASS.
Backend: `go test ./internal/app -run '^Test(Command|UINavigation)' -count=1
-timeout=180s` PASS (125,626 s), `go vet ./internal/app` PASS. Catálogo 42 /
local UI 34 / bindings 40 (36 v1 + 4 v2); defaults anteriores preservados.
Revisão independente da integração sem achados adicionais após correções de
alvo/foco/capacidades. Warnings `act`/AudioContext do jsdom não falham a suíte.
Não executados Wails, ACP, hardware, banco pessoal ou executáveis avulsos.

## 80. Inserir contextual no editor — 19/09/2026

Estado: implementado e validado automaticamente; aceite manual pendente.
Nenhum critério global promovido por este sublote de R07:
**58 I / 24 P / 2 N**, contagem preservada.

- [x] Publicar `editor.menu.insert.open` junto ao registry de apresentação do
  editor; manter `editor.menu.insert.open` como abertura de UI, sem ação de
  conteúdo, seleção ou persistência.
- [x] Usar o botão Inserir existente do editor ativo, com captura de
  owner/session/workspace/aba/documento/instância e revalidação de ABA.
- [x] Remover o listener legado Alt+I e os helpers mortos; não criar fallback
  local. Alt+I contextualiza Inserir no editor apto e Importar fora dele.
- [x] Publicar resoluções host-side por `surface.type`, somente para atalhos
  simples de apresentação; contexto desconhecido é recusado e uma entrada
  `null` impede fallback. Manter o percurso backend de `workspace.list`.
- [x] Preservar barreiras de supressão/revisão, precedência de escopo e cópia
  profunda do mapa. Testar condições herdadas nos acionadores antigo/novo de
  defaults pendentes e serialização explícita de `null`.
- [x] Preservar Alt+1/2/3, Ctrl+S, Ctrl+O e Ctrl+Shift+S fora desta migração.
- [x] Cobrir readonly/view/inatividade/modal/overlay, foco externo, remap,
  supressão, IME, repeat e abertura real pelo lease.
- [x] Confirmar frontend **456 testes em 11 arquivos PASS**, `tsc` e ESLint
  dos arquivos alterados PASS; inclui nove testes do teclado contextual.
- [ ] Aceite manual do editor: verificar Inserir, Importar, supressão sem
  fallback, readonly/view e remapeamento conforme `COMANDOS.md`.

Catálogo reconciliado após a seção80: **43 comandos, 35 locais, 41 defaults
(37 v1 + quatro v2) e 40 combinações efetivas**. Esta seção não fecha R07 nem
promove os critérios globais.

Backend: `go test ./internal/commandbindings -count=1` PASS (1,582 s);
`go test ./internal/app -run '^(TestCommand|TestUINavigation)' -count=1
-timeout=240s` PASS (104,529 s); `go vet ./internal/commandbindings
./internal/app` PASS. A regressão ampliada detectou e permitiu corrigir a
remoção indevida de `workspace.list` na extração do helper de resolução.
As contagens das fixtures com binding pessoal adicional foram ajustadas sem
remover testes de origem, revogação ou despacho. `git diff --check` dos
arquivos fonte/documentação verificados PASS; whitespace preexistente nos
bindings gerados não foi alterado. Não foram executados Wails, ACP, hardware
real ou banco pessoal, nem gerados executáveis avulsos. O aceite manual e as
ações persistentes do editor continuam pendentes.

## 81. Modo persistente do editor — 19/09/2026

Estado: contrato e backend do lote registrados; aceite manual pendente. Não há
promoção de gates: a baseline permanece **58 I / 24 P / 2 N**, com R07 ainda
parcial.

- [x] Publicar `editor.mode.markdown`, `editor.mode.rich` e
  `editor.mode.view`, com defaults `Alt+1`, `Alt+2` e `Alt+3`.
- [x] Exigir `Begin/Take/Commit`, alvo `workspace/active_tab` com
  `ExactVersion`, enum fechado e CAS; replay/ABA, inclusive repetição do mesmo
  modo, falha de storage e concorrência são rejeitados ou revertidos.
- [x] Preservar conteúdo, arquivos e demais campos da aba; o commit altera
  somente `Tab.State.displayMode`.
- [x] Preparar/flush do rich na UI antes da confirmação e aplicar o modo só
  depois dela; mudança de contexto durante a preparação cancela sem roubar
  foco. IME ativo sempre bloqueia; `UnknownIME` é estado distinto e só aceita
  controles `native`/`rich` suportados quando reconhecidos por registry válido.
- [ ] Executar o checklist manual de tecla, menu, paleta, Deck, persistência
  após restart, readonly/modal, IME, remapeamento e ausência de fallback.

Contagens reconciliadas após o lote81: **46 comandos, 35 locais, 44 defaults
(40 v1 + 4 v2) e 43 combinações efetivas**. Ctrl+S, Ctrl+O e Ctrl+Shift+S
continuam legados e não foram promovidos por este lote.

Evidência: **workspace suite PASS**. Frontend consolidado: **654 testes
em 14 arquivos PASS** (587 + 56 + 11), `tsc --noEmit` PASS e lint de produção
PASS. Backend parent: `go test ./internal/app -run
'^(TestCommand|TestUINavigation)' -count=1 -timeout=240s` PASS
(110,704 s); `go vet` de workspace/app PASS. A integração adicional do
Topbar usa registry, coordenador e Combobox reais e cobre teclado, menu,
paleta, reserva Deck sem novo Begin, repetição, falha, espera de confirmação
e mudança de alvo entre Take e Commit.
Não foram executados Wails, ACP, hardware real, banco pessoal ou executáveis
avulsos nesta atualização documental.

## 82. Preparação para arquivos do editor — lote parcial — 19/09/2026

Registro histórico do recorte anterior. A implementação do protocolo e a
migração dos ingressos são acompanhadas na seção83 abaixo; o texto desta
seção descreve o estado antes dessa integração.

Estado: parcial, sem aceite e sem declaração de migração concluída. Ctrl+S,
Ctrl+O e Ctrl+Shift+S continuam fora do catálogo e fora da contagem de
comandos/defaults. Catálogo permanece com 46 comandos; nenhum critério global
foi promovido por este lote.

- [x] Capturar owner/sessão/workspace/documento/caminho, observar ABA e
  desmontagem, recusar operações concorrentes na mesma instância e não
  recuperar foco que mudou. Preservar dirty e draft quando há edição durante
  a gravação; abrir em nova aba quando a antiga deixou de estar virgem.
- [x] Revalidar sessão/owner/epoch no backend antes e depois do diálogo,
  também após ler o conteúdo. Wiring ausente recusa a operação, sem fallback
  de compatibilidade baseado somente no userID.
- [x] Usar salvamento temporário com `sync`, `rename` para preservar o
  original até a substituição; esta etapa não fornece CAS contra alteração
  externa e não deve ser descrita como proteção de receipt.
- [x] Definir broker tipado para os três comandos, sem colocar conteúdo do
  documento em argumentos, ledger ou resultado persistido. Resolvido na
  seção83; reconciliação documental em 20/09/2026, sem novo aceite manual.

Checklist histórico do broker de arquivos, implementado na seção83:

1. Preparação tipada com owner, workspace, aba/documento, operação e file
   target; separar conteúdo transitório do envelope auditável.
2. Cancelamento e expiração de ticket, inclusive diálogo fechado, troca de
   aba/workspace, logout, perda de foco/instância e resposta fora de ordem.
3. Receipt de sobrescrita que detecte alteração externa e exija confirmação
   explícita antes de substituir arquivo existente.
4. Commit com CAS conjunto de workspace/aba e validação da identidade do
   arquivo; falha não pode publicar sucesso parcial.
5. Resultado terminal e `unknown`/incerto sem sucesso falso, com recuperação
   e consulta de resultado coerentes.
6. Ingresso real por teclado local, paleta e Stream Deck, além da retirada
   dos listeners/menu legados somente após equivalência comprovada.

Validação deste recorte: **125 testes frontend em seis arquivos PASS**,
incluindo 15 cenários de arquivos com stores reais; `tsc --noEmit` e ESLint
PASS. Backend: todos os `TestEditor` de wailsapi PASS (16,295 s), captura de
sessão/wiring App PASS (18,937 s), pacote filesystem PASS (3,614 s); `go vet`
nos três pacotes PASS. Nenhum Wails/ACP, executável avulso ou banco pessoal
foi utilizado. Estas evidências não validam o broker de arquivos ainda ausente
nem representam migração dos três atalhos.

## 83. Arquivos no executor comum — 19/09/2026

Continuação da seção82. Estado global preservado: **58 I / 24 P / 2 N**;
R07 e o aceite integral do AEP continuam parciais. O fechamento desta seção
não equivale ao fechamento de todos os critérios do projeto.

- [x] Publicar `editor.file.open`, `editor.file.save`, `editor.file.save_copy`
  e defaults Ctrl+O, Ctrl+S, Ctrl+Shift+S. São bindings resolvidos como os modos
  do editor: a restrição do alvo é revalidada por lease e snapshot hostside,
  não por fatos DOM enviados para a resolução de comandos duráveis.
- [x] Preparação transitória correlacionada a ticket/handoff; conteúdo nunca
  integra envelope auditável nem resultado persistido. Diálogo fora do gate;
  prazo total de cinco minutos e resultado consultável por mais um minuto.
- [x] Receipt de identidade/hash do arquivo, confirmação de sobrescrita no
  primeiro salvamento/cópia e revalidação antes da substituição. Não promete
  CAS interprocessos na janela entre o último check e `rename`.
- [x] Commit real dentro do broker, com exclusão de replay, owner/sessão,
  epoch/configuração e snapshot exato do workspace. Falha de metadados após
  escrita produz desfecho incerto, não sucesso. Panic não retém ticket eterno.
- [x] Teclado, paleta, menu e reserva Deck entram pelo mesmo coordenador;
  Deck conserva a reserva original. O blur de diálogo após Target/Take não
  revoga a ocorrência local consumida, sem dispensar as validações hostside.
- [x] Abertura cria/seleciona aba sem substituir documento vivo. Cópia não
  altera o original; salvamento preserva edições posteriores aos bytes enviados.
- [x] Concluir a consolidação automática final de App e frontend desta seção.
- [ ] Aceite manual: diálogos nativos, NVDA, remapeamento/supressão e três ações
  nas quatro origens. Checklist em `docs/content/recursos/COMANDOS.md`.

Contagens: **49 comandos, 35 locais, 47 defaults (43 v1 + quatro v2) e 46
combinações efetivas**. O caminho de arquivo é durável; não adiciona auditoria
aos comandos locais de navegação. Nenhum Wails, hardware físico, banco pessoal
ou teste ACP foi executado para esta seção.

Evidências frontend: **511 testes em 14 arquivos PASS**, mais **três testes
de tradução real** (pt-BR/en/es); o hook foi reexecutado após ajustar a chave
da confirmação. `tsc --noEmit` e ESLint dos arquivos tocados PASS.
Pacotes completos de commandexecution, commandui, workspace e filesystem
PASS; todos os `TestEditor` de wailsapi PASS (18,099 s), incluindo receipts.
`go vet` de App/commandui/commandexecution/workspace/filesystem/wailsapi PASS.
Não houve edição manual dos bindings gerados: a fachada tipada usa os métodos
Wails registrados, com teste de contrato e recusa de runtime antigo.

Consolidação App final: `go test ./internal/app -run
'^(TestCommand|TestUINavigation|TestEditorFile)' -count=1 -timeout=240s`
**PASS (108,903 s)**. As expectativas de mapa em paleta e restart foram
atualizadas para incluir os novos defaults, preservando assertions de origem,
supressão, persistência e execução. Reexecução final dos quatro pacotes
compartilhados também PASS. **Implementação deste lote concluída; aceite
manual pendente**, sem declarar concluído o AEP inteiro.

## 84. Negrito, itálico e tachado no executor comum — 19/09/2026

Baseline global preservada: **58 I / 24 P / 2 N**, AEP In Progress.
Esta seção numera um lote; não é o critério C84 nem seu fechamento.

- [x] Catálogo `editor.format.bold`, `.italic`, `.strike`, com nomes e
  descrições pt-BR/en/es e defaults Ctrl+B/I/Shift+X.
- [x] Contrato Write/HandlerUI, contexto `workspace.active_tab` ExactVersion,
  broker Begin/Take/Complete e persistência sem conteúdo do documento.
  Formatação não é navegação nem apresentação local isenta de auditoria.
- [x] Teclado, paleta, menu e Deck usam o mesmo executor; reserva do Deck
  não inicia outra ocorrência. Repetição automática não reaplica formatação.
- [x] Capturar seleção, documento TipTap e instância antes da espera;
  alteração invalida definitivamente a captura, sem procurar outro alvo.
  Substituição do documento no store, readonly, sessão/aba, desmontagem e
  composição ativa recusam o efeito. Desmontagem libera subscriptions.
- [x] Remover os três keymaps nativos concorrentes; manter commands e demais
  operações TipTap. Atalho de formatação não atua de outro campo de texto.
- [x] Testes reais TipTap e integração Topbar/Combobox/executor; regressão
  ampla dos comandos no App e análise estática Go/TypeScript/ESLint.
- [ ] Aceite manual acumulado: foco/NVDA, remapeamento/supressão e hardware;
  checklist em `docs/content/recursos/COMANDOS.md`. Usuário adiou esta etapa.

Contagens: **52 comandos, 35 locais, 50 defaults (46 v1 + quatro v2), 49
combinações efetivas** — 48 simples e uma contextual. Teste customizado
adiciona uma combinação simples, sem alterar o catálogo padrão.

Evidências: rodada frontend de sete arquivos **396 PASS** antes dos ajustes
finais de lifecycle, testes adicionais registrados abaixo. Regressão App
`^(TestCommand|TestUINavigation|TestEditorFile|TestEditorFormat)` **PASS
(112,377 s)**; pacotes commandcatalog/commandbindings/commanddeck e testes
focados do novo contrato PASS. `go vet` dos quatro pacotes PASS; `tsc --noEmit`
e ESLint do recorte PASS. Sem Wails, ACP, hardware ou banco pessoal.

Títulos, listas, links, blocos e operações de tabela permanecem fora desta
migração. Não declarar concluídos R07 nem o AEP integral por estes três comandos.

Validação final adicional: **84 testes em sete arquivos PASS**, incluindo
o novo teste de liberação imediata/idempotente das leases; **7 testes do hook
com stores Zustand e TipTap reais PASS** (owner, sessão, aba ABA, substituição
do documento, readonly, inatividade e desmontagem). Esta rodada sobrepõe
arquivos à anterior; não somar os totais como testes únicos. TypeScript e
ESLint reexecutados após os ajustes, PASS. Avisos dos mocks React e ausência
de AudioContext no JSDOM continuam aparecendo, sem testes falhos.

## 85. Formatação estrutural e alterações de tabela — 19/09/2026

Baseline global mantida em **58 I / 24 P / 2 N**; não confundir o número
desta seção com os critérios C. R07 e o AEP permanecem parciais.

- [x] Acrescentar 25 comandos ao contrato UI auditado existente: parágrafo,
  H1–H6, citação, código, listas com marcadores/numerada, limpar marcas,
  remover link; adicionar/remover linhas/colunas, alternar três cabeçalhos,
  mesclar/separar células e apagar tabela. IDs `editor.format.*` com nomes
  e descrições pt-BR/en/es; headings usam segmentos válidos `heading.h1..h6`.
- [x] Reusar Begin/Take/Complete com Write/ExactVersion, sem persistir texto
  ou seleção. Paleta, menu e Deck entram pelo coordenador já existente.
- [x] Preservar seleção real de células e capabilities TipTap; recusar
  operações de tabela fora dela e operações indisponíveis na seleção atual.
  Alteração de documento/seleção/sessão invalida o alvo sem retarget.
- [x] Migrar 11 defaults: Ctrl+Alt+0..6/C e Ctrl+Shift+B/7/8. Remover somente
  os keymaps de comandos migrados, preservando edição nativa de código/listas.
  H1–H6 definem explicitamente o nível, como o menu; Parágrafo volta ao texto
  normal, sem alternância implícita ao selecionar novamente o mesmo título.
- [x] Fechar a lacuna Ctrl+Alt sem liberar AltGr: observar downs separados de
  ControlLeft/Right e AltLeft; flags ambíguas ou AltRight/AltGraph não bastam.
  Release, blur, refresh e dispose descartam essa observação. O gravador de
  atalhos usa a mesma lógica; não há execução alternativa pelo TipTap.
- [x] Testar efeitos reais em tabelas (estrutura, cabeçalhos, seleção),
  menu sem mutação direta, 11 teclas pelo executor e reserva Deck reutilizada.
- [ ] Aceite físico/manual deste lote, acumulado a pedido do usuário:
  foco/NVDA, AltGr e Ctrl+Alt no WebView real, remapeamento/supressão e Deck.
  Checklist em `docs/content/recursos/COMANDOS.md`.

Contagens: **77 comandos, 35 locais, 61 defaults (57 v1 + quatro v2), 60
combinações efetivas**. São 59 simples e uma contextual; a fixture de tecla
customizada acrescenta uma simples, não um comando ao catálogo.

Validação frontend consolidada final: **530 testes em 18 arquivos PASS**, incluindo
Topbar 249, integração Topbar 40, casos ricos expandidos 41, menu 4, Ctrl+Alt 8,
gravador 8 e testes anteriores de editor/contexto/executor. TypeScript e ESLint
verificados no recorte. `go vet` de App/catalog/bindings/Deck PASS. Testes
backend focados cobrem contratos dos 28 formatadores, admissão, handoff,
repetição, catálogo, defaults, readiness e Deck. Consolidação App abaixo.

**Lacunas explicitamente não migradas neste lote:** inserir/editar link abre
diálogo assíncrono que exige um contrato próprio de preparação/commit; inserir
tabela escolhendo dimensões precisa de argumentos validados; mover entre
células deve seguir apresentação local, não Write auditado. Os controles
legados dessas ações permanecem, sem alegar migração integral. Outros itens
do menu Inserir também não são cobertos pelo catálogo de formatação estrutural.
Nenhum Wails, teste ACP, hardware físico ou banco pessoal foi usado.

Consolidação App: `go test ./internal/app -run
'^(TestCommand|TestUINavigation|TestEditorFile|TestEditorFormat)' -count=1
-timeout=300s` **PASS (111,583 s)**. `tsc --noEmit` e ESLint finais PASS,
sem avisos de lint. A revisão adicional corrigiu e cobriu o release de AltRight
quando o WebView ainda informa AltGraph no keyup, evitando modificador preso.
Avisos conhecidos de mocks React/AudioContext no JSDOM não são aceites manuais.
**Implementação do lote concluída; validação física acumulada e pendente.**

## 86. Preparação de conteúdo e navegação entre células — 19/09/2026

Baseline global preservada: **58 I / 24 P / 2 N**, AEP In Progress.
Este lote amplia a migração, não representa fechamento automático de R07.

- [x] Inserir/editar link e inserir tabela com dimensões pelo formulário
  compartilhado, com dados efêmeros validados antes do efeito auditado.
- [x] Migrar inserção rica de código e Mermaid, reutilizando também os
  comandos existentes de listas e citação no menu Inserir.
- [x] Próxima/anterior célula como apresentação local sem ledger, sem
  criar linha ao chegar ao limite da tabela.
- [x] Preservar documento e seleção durante o formulário; cancelar por
  mudança de contexto, desmontagem ou prazo, sem redirecionar a edição.
- [x] Reutilizar a reserva original do Stream Deck nos comandos interativos;
  não abrir outra ocorrência após preencher o formulário.
- [x] Validar contratos, efeitos TipTap reais, menus e executor integrado;
  manter os defaults anteriores, sem atribuir Ctrl+K à edição de links.
- [ ] Aceite manual acumulado: links, dimensões, cancelamento, limites de
  navegação, foco/NVDA e Stream Deck. Não exigido nesta rodada pelo usuário.

Limites: inserções do editor Markdown e templates de slides que acrescentam
conteúdo ao documento dependem de captura/commit próprios, diferentes da
seleção TipTap. Tab nativo dentro da tabela pode acrescentar uma linha e não
é classificado como navegação sem escrita. Não declarar essas operações
migradas por adicionar os comandos locais de próxima/anterior célula.

Backend validado: catálogo **83**, apresentação local **37**, defaults **61**
(57 v1 + quatro v2), **60 combinações efetivas** (59 simples e uma contextual).
Versão `product-v21-editor-format-batch86`. Os quatro novos comandos de
conteúdo são Write/HandlerUI; navegação entre células é localUI/Read.
Somente link/tabela interativos recebem reserva e execução de cinco minutos;
demais formatadores conservam o prazo padrão. O prazo de commit não foi
ampliado globalmente.

`go test ./internal/app -run
'^(TestCommand|TestUINavigation|TestEditorFile|TestEditorFormat)' -count=1
-timeout=300s` **PASS (140,333 s)**; `go vet` em App, commandcatalog,
commandbindings e commanddeck PASS.

Validação frontend final: **546 testes em 20 arquivos PASS**, incluindo
50 casos de integração Topbar e 18 de preparação/formulário real. `tsc
--noEmit` e ESLint dos arquivos alterados PASS, sem avisos de lint.
`git diff --check` no recorte PASS. Avisos conhecidos de mocks React e
AudioContext no JSDOM não equivalem a falhas nem a aceite NVDA.

Revisão consolidada: menu carrega a instância original do editor e não
redireciona a ação para outra aba. Leases de apresentação acompanham mudanças
de seleção desde a captura, inclusive ABA antes da primeira consulta.
Confirmação/cancelamento do formulário restauram foco; desmontagem cancela
a solicitação pendente. Teclado aguarda dados antes de Begin; Deck confirma
ou cancela sua reserva original, sem segundo Begin. Repetição de teclado
é permitida somente na navegação de células, que para nos limites sem
acrescentar linhas. Ações de conteúdo continuam sem repetição automática.

**Implementação deste lote concluída; aceite manual acumulado e pendente.**
Sem Wails, ACP, executável personalizado, hardware físico ou banco pessoal.

## 87. Inserções Markdown e templates de slides — 19/09/2026

Baseline global preservada: **58 I / 24 P / 2 N**, AEP In Progress.
Lote de migração de ações existentes; não equivale a novos critérios globais
concluídos nem encerra R07 por contagem de comandos.

- [x] Reutilizar seis IDs de conteúdo no Monaco: tabela, código, Mermaid,
  listas com marcadores/numerada e citação; sem caminho alternativo do menu.
- [x] Capturar modelo, versão, seleção e documento proprietário; invalidar
  alvo obsoleto, composição, sessão/aba/modelo alterados e desmontagem.
- [x] Preparar tabela Markdown pelo formulário compartilhado, com dimensões
  de 2 a 6 e cabeçalho obrigatório. Payload efêmero, sem texto no ledger.
- [x] Registrar onze templates de slides como Write/HandlerUI e migrar
  menu/toolbar para paleta, teclas configuráveis e reserva original do Deck.
- [x] Acrescentar slides ao documento completo; flush rico antes da
  preparação, preservação dos slides anteriores e commit síncrono único.
- [x] No Monaco, aplicar conteúdo pelo executeEdits com fronteiras de undo;
  não escrever no store como fallback quando o editor não está pronto.
- [x] Gate automatizado consolidado: **637 testes frontend / 25 arquivos
  PASS**, `tsc --noEmit`, ESLint dos 23 arquivos alterados e
  `git diff --check` no recorte PASS.
- [ ] Aceite manual acumulado: Markdown/renderização, onze layouts, undo,
  preservação de edição rica, foco/NVDA e Stream Deck.

Catálogo **94**, apresentação local **37**, defaults **61** (57 v1 + quatro
v2), **60 combinações efetivas**. Versão `product-v22-editor-insert-batch87`.
Onze novas ações no catálogo, seis capacidades Markdown em IDs já existentes;
nenhuma tecla padrão adicionada. Conteúdo não recebe repetição automática.

Limites: isto não migra todas as operações nativas do Monaco/TipTap, Tab de
tabela que pode criar linha, ações de merge/patch ou importação/exportação
de configurações. Não declara aceite manual nem fechamento integral do AEP.

Backend: `go test ./internal/app -run '^(TestCommand|TestEditorFormat)'
-count=1 -timeout=300s` PASS; `go vet ./internal/app ./internal/commandcatalog
./internal/commandbindings ./internal/commanddeck` PASS. Inclui contrato e
Begin/Take/Complete dos onze slides, labels/rotas/metadata em três idiomas,
contexto active_tab ExactVersion e contagens do catálogo/defaults.

Frontend: integração Topbar cobre paleta real, teclado e reserva do Deck;
o adaptador Monaco preserva bytes fora da seleção e fronteiras de undo.
Regressões provam invalidação ABA, cópia de payload antes de await, abort do
formulário na desmontagem e foco após confirmar/cancelar. Slides ricos fazem
flush do documento proprietário antes do commit; Markdown usa executeEdits
sem fallback pelo store. Os onze templates são verificados nos três idiomas.

Avisos conhecidos de mocks/act do React e AudioContext no JSDOM permanecem,
sem falhas ou exceções não tratadas nesta rodada. Não são aceite manual NVDA.
Sem Wails, ACP, executável personalizado, hardware ou banco pessoal.
**Implementação do lote concluída; validação manual acumulada e pendente.**

## 88. Limpar conversa pelo comando destrutivo — 19/09/2026

Baseline global preservada: **58 I / 24 P / 2 N**, AEP In Progress. Este lote
migra uma ação destrutiva completa; não equivale ao fechamento de R07 nem de
todos os comandos de chat.

- [x] Registrar `chat.conversation.clear` como Destructive/Interactive,
  HandlerBackend, com nomes e descrições nos três idiomas.
- [x] Migrar botão, Ctrl+L configurável, paleta e reserva original do Stream
  Deck para Begin/Take/commit contextual. Remover o listener legado de Ctrl+L
  e a chamada direta de limpeza na toolbar.
- [x] Exigir recibo de decisão do backend, vinculado à ocorrência e consumido
  uma única vez; CompleteUICommand não pode declarar sucesso como bypass.
- [x] Fixar owner/sessão/workspace/aba/conversa e fingerprint do conteúdo antes
  da confirmação; não enviar mensagens ou fingerprint nos argumentos auditados.
- [x] Comparar e limpar sob uma transação SQLite imediata: mensagens, resumo
  e invocações de ferramentas, com rollback integral e evento após commit.
- [x] Recusar geração ativa, conteúdo novo/alterado e alvo obsoleto; preservar
  a ordem runtime → lifecycle → auth/workspace e liberar gates na recusa.
- [x] Preservar a superfície original durante o próprio diálogo de decisão;
  bloquear composição, overlays, outros campos/painéis, repetição automática
  e desmontagem. Não repetir um commit com resultado incerto.
- [x] Regressão frontend: **716 testes / 32 arquivos PASS**, incluindo as
  quatro origens de execução e a troca de conversa durante a confirmação.
  `tsc --noEmit` e ESLint dos oito arquivos frontend PASS.
- [x] Testes focados de controllers/database e `go vet` em controllers,
  database, App, commandcatalog, commandbindings e commanddeck PASS.
- [ ] Aceite manual acumulado: confirmar/cancelar pelo botão, Ctrl+L, paleta
  e Deck; chat contextual; foco e leitura pelo NVDA. Roteiro em
  `docs/content/recursos/COMANDOS.md`.

Catálogo **95**, apresentação local **37**, defaults **62**; versão
`product-v23-chat-clear-batch88`. A espera interativa recebe cinco minutos
somente neste comando; a janela de commit conserva seu limite anterior.

A regressão mais ampla identificou duas expectativas antigas que tratavam
navegação de células como ação sem repetição. O teste passou a exigir duas
execuções (primeiro pressionamento + repetição) exclusivamente para próxima/
anterior célula, mantendo execução única para apresentação e ausência de
ledger. Isso segue o contrato de navegação já aprovado, sem mudar produção.

Limites: envio/retry, exclusão individual e demais operações de chat não são
declarados migrados. A limpeza preserva conversa/aba, mas não possui desfazer.
Sem execução de Wails, ACP, executáveis personalizados, hardware ou banco
pessoal. Aceite manual continua pendente.

Integração App: testes consolidados PASS (137,910 s), e nova execução focada
clear/readiness PASS (22,925 s) após separar a disponibilidade do cálculo de
fingerprint. Consultar a paleta verifica apenas owner/vínculo; não lê todo o
histórico. Decisão real cobre confirmação, cancelamento, recibo consumido,
replay, mudança de vínculo/aba/configuração, conteúdo obsoleto e geração ativa.
No frontend, três casos com DecisionDialog e Modal reais verificam confirmação
na página, confirmação sobre chat modal e cancelamento preservando a origem.

**Implementação deste lote concluída; aceite manual acumulado e pendente.**

## 89. Consultas de mensagens fixadas e tokens — 19/09/2026

Baseline global preservada: **58 I / 24 P / 2 N**; AEP In Progress. Migração
de duas ações existentes, sem declarar novos critérios globais concluídos.

- [x] Registrar `chat.pinned.open` e `chat.tokens.open` como apresentação
  local/Read/NoDecision, com labels e aliases em pt-BR, en e es.
- [x] Conectar paleta, teclado personalizado e Stream Deck local, sem broker
  de execução, invocação persistida ou repetição automática.
- [x] Encaminhar os botões pela mesma apresentação registrada, exigindo a
  instância original e sem click recursivo ou fallback para outro chat.
- [x] Reutilizar os modais existentes, sem nova UI paralela. Capturar a
  conversa e invalidar a consulta em mudança de sessão, owner, workspace,
  aba, conversa ou rota, inclusive ABA.
- [x] Ampliar somente a allowlist explícita do chat modal topmost; outro
  diálogo sobreposto bloqueia teclado, botão e Deck. Limpeza destrutiva não
  recebe essa classificação nem entra nessa allowlist.
- [x] Proteger estatísticas contra resultados/erros atrasados de outra
  conversa e contra reload antigo sobrescrevendo resposta ou evento recente.
- [x] Frontend consolidado: **797 testes / 37 arquivos PASS**; TypeScript,
  ESLint dos 12 arquivos frontend e diff-check do recorte PASS.
- [x] Backend consolidado de comandos/editor PASS (121,844 s); testes
  específicos dos dois comandos, projeção de teclado e Deck PASS. `go vet`
  em App, commandcatalog, commandbindings e commanddeck PASS.
- [ ] Aceite manual acumulado: abrir ambas as consultas por botão/paleta,
  configurar teclado/Deck, chat contextual, bloqueio por modal e foco/NVDA.
  Roteiro em `docs/content/recursos/COMANDOS.md`.

Catálogo **97**, apresentação local **39**, defaults **62**; versão
`product-v24-chat-inspection-batch89`. Nenhuma nova tecla padrão.

Limites: abrir a consulta não fixa/desfixa mensagens nem modifica o contexto
do modelo. Ações internas dos modais, envio/retry e outras operações de chat
continuam fora deste lote. Aumento do catálogo não equivale à conclusão do
AEP. Sem Wails, testes ACP, executáveis personalizados, hardware ou banco
pessoal.

Revisão do lote89: a checagem de segredo em TriggerSpec usava busca textual
por `token`, confundindo o ID público `chat.tokens.open` com uma credencial.
O teste agora percorre chaves JSON, incluindo objetos/listas aninhados e
nomes como `access_token`/`RefreshToken`; não libera campos sensíveis para
acomodar o novo comando. Nenhuma regra de segurança de produção foi reduzida.

As fixtures de estatísticas agora respeitam o `conversationId` obrigatório
do DTO real. Regressões verificam que sucesso/erro atrasado de A não altera
B, que uma consulta antiga não encerra o loading de outra e que eventos
recentes não são sobrescritos por snapshots pendentes. Mudanças de rota ou
de contexto fecham os modais sem reabertura automática no retorno.

**Implementação do lote concluída; aceite manual acumulado e pendente.**

## 90. Envio, cancelamento e nova tentativa no chat — 19/09/2026

Migração implementada, com geração oficial dos bindings ainda pendente de
autorização. Baseline global preservada: **58 I / 24 P / 2 N**;
AEP In Progress. O aumento do catálogo não fecha critérios globais por si só.

- [x] Registrar `chat.message.send`, `chat.response.cancel` e
  `chat.message.retry` como operações auditadas, nunca apresentação local.
- [x] Reutilizar `SendMessage` e `RetryMessage` e o pipeline compartilhado,
  sem novo endpoint de envio, mensagens otimistas ou conteúdo no ledger.
- [x] Vincular a correlação de uso único à sessão, workspace, aba e conversa;
  recusar replay, alvo obsoleto e conclusão de sucesso forjada pela UI.
- [x] Fixar a geração do streaming para o cancelamento não atingir um turno
  posterior; preservar o lifetime da resposta após o término da admissão.
- [x] Unificar input/botões, paleta, teclado configurável e Stream Deck;
  preservar prioridade do menu slash, composição e Shift+Enter.
- [x] Preservar rascunho em recusa, impedir limpeza de texto novo e impedir
  reenvio automático em resultado desconhecido.
- [x] Validar fluxo página/modal, mudança de contexto inclusive ABA,
  repetição, filas e falhas de transporte com testes automatizados.
- [ ] Regenerar bindings pelo gerador oficial, sem edição manual.
- [x] Registrar resultados dos testes e roteiro de aceite manual acumulado.
- [ ] Aceite manual do usuário (adiado a pedido; não impede implementação).

Backend: suíte consolidada de comandos/editor em `internal/app` PASS
(158,472 s), testes focados de bind e StreamingManager PASS, `go vet` nos
quatro pacotes alterados e diff-check PASS. Correlação removida de ChatParams
antes do controller; conteúdo/anexos nunca são argumentos ou resultado do
ledger. Erros após possível persistência produzem `outcome_unknown`
sanitizado. O contexto do turno não herda o prazo curto da reserva.

Catálogo **100**, apresentação local **39**, defaults **62**; versão
`product-v25-chat-actions-batch90`. Sem novos defaults. Deep links existentes
sem correlação mantêm seu pipeline autenticado e não são declarados migrados;
metadata presente, mas inválida, nunca usa fallback.

Regressão frontend consolidada: **1.103/1.103 testes, 52 arquivos, PASS**
(28,33 s), incluindo TerminalPage, slash, fluxo real de input/store e quatro
origens do dispatcher. A revisão corrigiu anexos capturados de props antigas,
recovery sem texto e rejeição não observada na cauda interna da fila, sem
engolir o erro retornado ao chamador. Roteiro manual em
`docs/content/recursos/COMANDOS.md`. Nenhum gerador Wails, teste ACP ou
executável personalizado foi executado neste lote. Bindings não editados
manualmente; geração oficial e aceite manual não são considerados concluídos.

Fechamento das fixtures: 51/51 testes das três suítes página/sessão/modal,
TypeScript (`tsc --noEmit`), ESLint dos arquivos alterados e diff-check PASS.

## 91. Ações sobre a mensagem selecionada — 19/09/2026

Implementado e validado automaticamente; geração oficial dos bindings e
aceite manual pendentes. Baseline global **58 I / 24 P / 2 N** inalterada.

- [x] Publicar copiar texto/Markdown, falar, abrir edição, fixar/desfixar e
  excluir no catálogo com locales e classificação por efeito.
- [x] Capturar mensagem selecionada/focada ou alvo explícito do menu;
  ausência de alvo torna indisponível, sem escolher a última mensagem.
- [x] Preservar alvo/contexto, impedir replay, ABA e uso em mensagem de outra
  conversa; preparar alvo backend antes da admissão/decisão.
- [x] Excluir com decisão backend única e mutação atômica condicionada;
  fixar/desfixar sem duplicação em resposta de transporte perdida.
- [x] Abrir edição sem auditoria ou IPC; não confundir com salvar edição.
- [x] Reutilizar clipboard/TTS existentes, propagando falhas e revalidando
  o contexto antes de tocar áudio que chegou após uma espera.
- [x] Integrar menu/controles, paleta, teclado configurável e Stream Deck.
- [x] Validar testes de regressão, TypeScript, lint e backend do recorte.
- [ ] Regenerar bindings oficialmente (autorização ainda pendente).
- [ ] Aceite manual acumulado com NVDA e equipamento físico.

Fora do lote: persistir edição, enviar conteúdo ao editor, ações específicas
de blocos/código/tabelas e restante de voz/perfis/jobs. Não declarar essas
operações migradas pela simples abertura do menu ou do editor de mensagem.

Evidências: **1.257/1.257 testes frontend, 62 arquivos, PASS (28,26 s)**;
App consolidado (`TestCommand|TestEditorFormat` e regressões UI/navigation/
editor-file) **PASS (154,745 s)**; testes focados de App (32,736 s), database
e controllers PASS; TypeScript, ESLint e `go vet` App/controllers/database PASS.
Catálogo **106**, locais **40**, defaults **62**, versão
`product-v26-chat-message-actions-batch91`. Sem novos defaults.

A preparação backend vincula uma mensagem persistida antes de admitir o
comando e apresentar decisão. Pin/delete usam compare-and-mutate transacional;
há testes de rollback, novas respostas, vínculo entre conversas inválido,
owner alheio, ABA, geração, origem Deck preservada e replay. Nenhum conteúdo,
mídia ou ID da mensagem integra os argumentos persistidos do ledger.

A fala confirma início, sem segurar a fila até terminar o áudio. O lifetime
de reprodução tem guarda própria, liberada no fim/falha/invalidação; a
preparação possui espera limitada e resposta tardia não inicia reprodução.
Falha de refresh é de apresentação, não transforma commit confirmado em
reexecução. O fluxo de configuração de voz sem perfil foi preservado.
Controles nativos da lista continuam locais e têm proteção contra repetição,
IME e duplo despacho; não foram promovidos a defaults globais remapeáveis.

Roteiro manual: `docs/content/recursos/COMANDOS.md`, ações sobre mensagem.
Bindings gerados não foram editados à mão. Sem Wails, ACP, hardware, banco
pessoal ou executáveis personalizados. O AEP integral permanece In Progress.

## 92. Persistir edição de mensagem — 19/09/2026

Implementado e validado automaticamente. Recorte: `chat.message.edit.save`,
sem novas teclas padrão. Geração oficial e aceite manual pendentes.
Baseline global **58 I / 24 P / 2 N** preservada; não converter a quantidade
de ações do catálogo em critérios aceitos.

- [x] Capturar mensagem, conteúdo original e rascunho da instância de edição.
- [x] Invalidar troca de superfície/conversa, cancelamento, reabertura e
  alterações do rascunho durante a espera, inclusive ABA.
- [x] Comparar a base original na preparação e a revisão persistida no commit
  atômico; preservar metadados e emitir o evento existente após persistência.
- [x] Integrar botão/Ctrl+Enter local, paleta, teclado configurável e Deck,
  sem fallback para a atualização legada nem repetição automática do efeito.
- [x] Não limpar rascunho novo nem declarar sucesso em resultado desconhecido.
- [x] Validar regressões frontend/backend, TypeScript e lint.
- [ ] Regenerar bindings oficialmente (autorização pendente).
- [ ] Aceite manual acumulado com NVDA e equipamento físico.

Catálogo **107**, locais **40**, defaults **62**, versão
`product-v27-chat-message-edit-batch92`. `PrepareChatMessageEditCommand`
recebe ticket, mensagem, texto original e novo texto de forma efêmera;
`CommitChatMessageCommand` continua sendo o commit de uso único. O backend
recusa mensagem de outra conversa/owner, papel diferente de user, mensagem
interna, texto vazio e conteúdo superior a 512 KiB. Geração ativa impede a
alteração sem cancelar o turno. Texto é liberado da preparação ao terminar e
não integra o ledger. Alteração textual preserva tokens, modelo e mídia.

O evento `message:updated` foi alinhado a `conversationId/messageId/content`
na tela, com filtro por conversa. Não há mensagem otimista nem fallback para
`UpdateMessage` na edição de produto. O sucesso pode chegar depois do evento
da própria atualização sem invalidar o fechamento correto do formulário.
Uma revisão digitada durante o commit permanece aberta e pode ser salva em
seguida, usando como base somente o texto persistido confirmado. Resultado
desconhecido não libera rebase nem dispara reexecução automática.

A guarda de navegação pertence à solicitação capturada, não a toda a vida do
formulário: abrir/fechar modal ou sair/voltar de uma aba sem operação pendente
preserva a edição utilizável. Logout/owner, cancelamento/reabertura e alteração
da mensagem original mantêm proteção própria. O retorno de foco após sucesso
revalida contexto e revisão, sem roubar o foco de outra mensagem/modal.

Evidências: regressão frontend **1.330/1.330 testes em 67 arquivos PASS**
(29,01 s), incluindo 23 testes de dispatcher, 23 do rascunho real, 13 da
sessão/formulário e 13 novos testes de protocolo/porta. Backend:
`go test ./internal/app -run '^(TestCommand|TestUICommand|TestUINavigation|TestEditorFile|TestEditorFormat)' -count=1 -timeout=300s`
**PASS (161,079 s; 688 testes/subtestes)**;
`go test ./internal/database ./controllers -count=1 -timeout=300s`
**PASS (456 testes/subtestes; 17,968 s / 4,949 s)**. TypeScript, ESLint e
`go vet ./internal/app ./internal/database ./controllers` PASS.

Não executados Wails, ACP, equipamento físico, banco pessoal ou executáveis
personalizados. Bindings gerados não foram editados manualmente neste lote.
Roteiro acumulado: `docs/content/recursos/COMANDOS.md`, salvar edição.

### Próximo contrato identificado na seção92: chat → editor

Registro histórico: a investigação identificou ausência de confirmação da
inserção e de identidade da mensagem fonte no payload. A migração do salvamento
não alterou aquele caminho. Os gates de captura, transição segura, ACK e consumo
único são acompanhados na [seção93](#93-transferência-da-mensagem-para-o-editor--19092026),
sem duplicar aqui pendências da implementação posterior.

## 93. Transferência da mensagem para o editor — 19/09/2026

Recorte implementado: `chat.message.send_to_editor`, efeito de escrita UI
auditado, não apresentação local. Por padrão, a mensagem selecionada inteira
em Markdown é enviada a um novo documento. Não há nova tecla padrão.
Baseline de produto: **108 comandos de catálogo / 40 locais / 62 bindings
padrão**, versão **v28**; não confundir esses bindings de comandos com a
geração oficial dos bindings Wails, ainda pendente.
Baseline global **58 I / 24 P / 2 N** inalterada; AEP In Progress.

- [x] Integrar botão/menu, paleta, teclado configurável e Deck com alvo
  capturado antes da admissão, sem fallback para a mensagem mais recente.
- [x] Preservar snippets de código/tabelas/links, formato, título e destino
  explícito dos menus. `ChatSendToEditorPayload` acrescenta `messageId` e
  `originalContent` capturados por valor; clicar não recaptura a fonte atual.
- [x] Preparar a fonte/destino antes de Take e revalidar a transição autorizada
  chat → editor, sem aceitar troca arbitrária de owner, contexto ou documento.
- [x] Aguardar montagem elegível e ACK da aplicação real no editor antes de
  `CompleteUICommand(..., 'succeeded')`; enfileirar ou abrir uma aba não basta.
- [x] Recusar alvo obsoleto/ABA, duplicação, composição IME e repetição de tecla;
  preservar a reserva recebida do Deck, sem iniciar outra invocação.
- [x] Em falha de transferência após Take, usar `CancelUICommand` e reconciliar
  `outcome_unknown`, sem `CompleteUICommand(..., 'failed')` nem replay.
- [ ] Regenerar e validar bindings oficiais (pendente de autorização).
- [ ] Aceite manual acumulado com NVDA e equipamento físico.

A transição pode criar uma aba antes de a aplicação do conteúdo ser confirmada.
Em resultado desconhecido, essa aba pode permanecer: conferir aba e conteúdo
antes de decidir por nova tentativa. Não há rollback presumido nem repetição
automática da criação/inserção. Readonly, falha de carregamento, destino fechado
ou contexto alterado não autorizam aplicação em outro editor.

Na reabertura de documento com `draftId` e sem caminho de arquivo,
`useEditorDocument` lê `EditorReadDraft` para restaurar o autosave. Conteúdo
vazio ou a condição exata de rascunho inexistente resultam em documento vazio,
sem inserir `DEFAULT_MD`. Outras falhas mantêm o documento somente leitura,
sem sobrescrever o rascunho.

Removido o caminho legado de inserção enfileirada (`requestInsert`/`pendingInsert`)
do fluxo migrado. A transferência usa a aplicação confirmada no editor, sem
fallback para o enfileiramento anterior.

Evidências focadas: fontes de menus
e renderização **56/56 testes em quatro arquivos PASS**; integração independente
`Topbar.editorTransfer.integration.test.tsx` **20/20 PASS**; orquestrador
**13/13 PASS**, incluindo modal real; receptor **10/10 PASS** e registry
**7/7 PASS**.
`ChatSessionView.messageActions` **19 testes PASS**, incluindo dois novos casos
de transferência real; transporte Wails **5 testes PASS**. Estas evidências
não substituem bindings oficiais nem aceite manual.
`useEditorDocument.transfer.test.tsx` **5/5 PASS**, cobrindo a restauração do
rascunho. Backend: **710 testes App PASS** e **641 testes de
workspace/database/controllers PASS**. Testes de `commandcatalog`,
`commandruntime` e `commandui` **PASS**; `go vet` **PASS** em `app`,
`workspace`, `database`, `controllers`, `commandcatalog`, `commandruntime`
e `commandui`.

Validações adicionais: `ChatPage` **7 testes PASS** e `useEditorFileActions`
**15 testes PASS** (**22 no total**), registradas separadamente da consolidada
abaixo, sem somar execuções sobrepostas.

Consolidada frontend final após todos os patches: **1.621 testes em 96 arquivos
PASS (27,62 s)**, recorte
`command|Command|Topbar|ChatSessionView|editorSendMenu|messageMenuItems|ChatMessage|useEditorInsert|useEditorDocument|editorStore|EditorPage|EditorSurface|EditorWorkspace`.
Esta execução substitui a rodada provisória de 1.601 testes/92 arquivos,
durante a qual ainda houve alterações no editor. `tsc --noEmit` **PASS**;
ESLint **PASS** em 19 arquivos na rodada final, além das validações focadas.
`git diff --check` **PASS** no escopo `frontend/src`, `internal/app`,
`internal/workspace`, docs e AEP. O check global aponta whitespace preexistente
em `frontend/wailsjs/go/models.ts`, gerado e não editado neste fechamento.

Roteiro: `docs/content/recursos/COMANDOS.md`, transferência para o editor.
Não executados Wails, ACP, equipamento físico, banco pessoal ou executáveis
personalizados neste trabalho de documentação. Os resultados automáticos não
promovem critérios globais nem substituem bindings oficiais ou aceite manual;
o AEP permanece In Progress.

## 94. Edição de diagramas Mermaid — 19/09/2026

Implementados: `editor.mermaid.open` como apresentação local;
`editor.mermaid.apply` e `editor.mermaid.remove` como efeitos UI auditados.
Baseline global **58 I / 24 P / 2 N** inalterada.

- [x] Integrar abertura, aplicação e remoção no dispatcher, preservando alvo.
- [x] Capturar bloco/documento/instância/revisão, sem fallback para outro editor.
- [x] Fechar somente o próprio modal topmost antes de Begin/Take/efeito.
- [x] Preservar origem de teclado/Deck e impedir confirmação duplicada.
- [x] Aplicar no Monaco/TipTap real ou no documento do preview editável,
  confirmando alteração antes de declarar sucesso; readonly é barreira.
- [x] Remover listeners/callbacks legados, preservando cancelamento, foco e IME.
- [x] Validar regressões, TypeScript, lint, backend e documentação.
- [x] Bindings oficiais gerados na qualificação da seção127: export
  `BeginEditorMermaidUIKey` presente em `frontend/wailsjs/go/app/App.js`
  e `App.d.ts`. Não implica aceite manual.
- [ ] Aceite manual acumulado com NVDA e equipamento físico.

Contrato: o modal é validado no frontend pela identidade real da instância e
pela geração do stack, não por uma declaração arbitrária de topmost enviada
ao backend. Apply/remove saem desse modal antes da execução auditada normal;
a confirmação destrutiva usa o componente de decisão existente. Não ampliar
a allowlist de `DecisionDialog` nem permitir paleta/navegação atrás de modais.
Texto do diagrama permanece efêmero e não é argumento do ledger.

Ctrl+S, Cmd+S e Ctrl+Enter são invariantes de confirmação do formulário,
não novos defaults remapeáveis. A porta fechada `BeginEditorMermaidUIKey`
preserva `keyboard.local`, geração e deduplicação; não aceita ID arbitrário
nem atribui camada/binding fictícios. A captura observa keyup desde o evento
original, mesmo que a tecla seja solta antes do fechamento do modal.
Associações pessoais aos comandos continuam pelo mapa configurável normal.

Evidências focadas: modal **15/15**, Topbar Mermaid **24/24**, transporte
de teclado **8/8**, hook **23/23** e registry **3/3 PASS**. O hook usa TipTap,
Modal e ConfirmHost reais; os sete casos Monaco usam fixture da API e não
substituem navegador real. Cobertos ACK negativo, revisão/instância/modelo
obsoletos, readonly, confirmação/cancelamento, retorno de foco, IME,
duplicação e resultado desconhecido sem replay.

Produto **v29: 111 comandos de catálogo / 41 locais / 62 bindings padrão**.
Backend: consolidada App **721 testes/subtestes PASS (160,427 s)**; depois do
ajuste final de timeout, focados Mermaid **PASS (21,104 s)** e vet **PASS**.
Remoção mantém reserva/execução por cinco minutos para a confirmação e
resultado consultável por seis; o prazo de Take começa após a preparação,
não se confunde com o tempo de espera da decisão.
Também passaram testes de `commandbindings`, `commandcatalog`,
`commandruntime` e `commandui`; vet passou nesses quatro pacotes e em `app`.
Workspace/database/controllers não foram reexecutados neste lote.

Revisão adicional fechou as entradas reais do NodeView: botão, Enter/F2,
duplo clique, type-to-edit e remoção por Delete/Backspace/Shift+Delete.
Não há mais aplicação/remoção direta nem Questionnaire legado nesse fluxo;
Shift+Delete também exige confirmação. Removidas as APIs de efeito obsoletas
do handle rico; o lookup por ID continua usado pelo alvo capturado e recusa
IDs duplicados. A referência efêmera do editor de origem impede que um evento
de outro editor alcance o ativo, mesmo com o mesmo ID de bloco.

O schema preserva o ID interno sem renderizá-lo em HTML ou Markdown, e a
atribuição consulta o nó atual antes de escrever. O handle permanece estável
em renderizações inócuas; trocar a instância ainda invalida o alvo. O NodeView
observa a mudança de editabilidade sem mutação artificial de documento.
**18 testes NodeView com RichTextEditor/TipTap reais PASS**, incluindo
somente leitura → editável; **23 testes de EditorContentArea PASS** e
**41 testes de guardas UI PASS**. Um travamento intermediário era causado
por matcher percorrendo a instância circular do TipTap; a asserção final
verifica identidade exata, sem substituir o editor real por mock.

Consolidada frontend final após os patches: **1.767 testes em 110 arquivos
PASS (28,14 s)**, recorte
`command|Command|Topbar|ChatSessionView|editorSendMenu|messageMenuItems|ChatMessage|useEditorInsert|useEditorDocument|editorStore|EditorPage|EditorSurface|EditorWorkspace|EditorContentArea|RichTextEditor|buildRichTextExtensions|Mermaid|mermaid`.
Esta rodada substitui as intermediárias e inclui os ingressos reais do bloco.
`tsc --noEmit` e ESLint em **25 arquivos** PASS; diff-check no escopo de
frontend, backend, docs e AEP PASS. Não houve alteração de CSS neste lote.

Roteiro com checkboxes em `docs/content/recursos/COMANDOS.md`, seção Mermaid.
Não executados Wails dev/build/generate, testes ACP, executáveis de diagnóstico,
banco pessoal ou hardware. Bindings oficiais e aceite manual permanecem gates
abertos; estes resultados não concluem o AEP nem promovem critérios globais.

## 95. Navegação entre regiões da interface — 19/09/2026

Lote implementado: `navigation.landmark.next`, `.previous` e `.default`.
Apresentação local sem invocação persistida/ledger, sem efeitos de domínio.
Baseline global **58 I / 24 P / 2 N** inalterada.

- [x] Publicar catálogo local e defaults F6/Shift+F6 sem ampliar repetição.
- [x] Registrar as regiões existentes, preservar ordem, disponibilidade e foco.
- [x] Unificar mapa de teclado, paleta, Deck e Escape contextual no dispatcher.
- [x] Capturar região de origem antes da paleta; recusar ABA, owner ambíguo,
  troca de rota/sessão/workspace/aba, desmontagem e composição IME.
- [x] Preservar a barreira modal: somente regiões da instância topmost apta;
  nenhum fallback para a página de fundo nem paleta atrás de modal.
- [x] Retirar execução F6 paralela e preservar restauração nativa de foco.
- [x] Validar regressões frontend/backend, TypeScript, lint e documentação.
- [ ] Aceite manual acumulado com NVDA e equipamento físico.

Escape não vira um binding global: continua gesto contextual em bubbling,
depois dos componentes, e só solicita `.default` se não foi consumido e o
foco estava em outra região conhecida. F6/Shift+F6 usam o mapa configurável.
Repetição automática desses três comandos permanece desabilitada conforme
a allowlist atual; não se amplia a exceção de navegação de abas/células.
O callback nativo de restauração de foco usado por Modal/Menu permanece;
ele não é um acionador físico nem fallback do dispatcher.

Produto `product-v30-landmarks-batch95`: **114 comandos / 44 locais /
64 bindings padrão**. WorkspaceLayout é owner apenas no workspace e em
configurações; demais páginas mantêm suas próprias regiões granulares, sem
dois owners concorrentes. A abertura da paleta por mouse captura a origem
no pointerdown, antes de o botão receber foco; Ctrl+K captura antes da abertura.

Evidências automatizadas (19/09/2026):
- Backend: consolidado App **726 testes/subtestes PASS (159,995 s)**;
  `commandcatalog`, `commandbindings`, `commandconfig` e `go vet ./internal/app`
  PASS. Testes focados comprovam classificação local, ausência de ledger,
  F6/Shift+F6, sem default Escape e sem ampliar repetição.
- Frontend: **1916 testes / 118 arquivos PASS (32,19 s)** no recorte do lote94
  acrescido de `Landmark|landmark|WorkspaceLayout|WorkspaceChatModal`.
  Inclui 20 integrações Topbar, 10 WorkspaceChatModal, 14 WorkspaceLayout,
  28 registry, 35 hook e 10 wrappers; não são testes adicionais ao total.
- TypeScript e ESLint no escopo alterado PASS; sem alteração de CSS.

Roteiro com checkboxes em `docs/content/recursos/COMANDOS.md`, seção
“Navegar entre regiões da interface”. Não executados Wails dev/build/generate,
ACP, executáveis de diagnóstico, banco pessoal ou hardware. Geração oficial
de bindings e aceite manual acumulado continuam pendentes. Estes resultados
não promovem critérios globais nem concluem o AEP.

## 96. Apresentação e navegação contextual do chat — 19/09/2026

Lote implementado, reunindo sete ações locais:
`chat.focus.input`, `chat.focus.messages`, `chat.message.read.open`,
`chat.message.menu.open`, `chat.message.reasoning.toggle`,
`chat.message.thread.expand` e `chat.message.thread.collapse`.
Não inclui envio, TTS, alteração do conteúdo, navegação de janelas do histórico
nem conversão de setas da árvore em atalhos globais. Sem novos defaults;
gestos nativos dos componentes preservam seu escopo.

- [x] Catálogo localizado e projeção local, sem ledger ou origem headless.
- [x] Registro de superfícies e mensagens com captura de identidade/contexto,
  recusa de ambiguidade, ABA, IME, desmontagem e modal não pertencente ao chat.
- [x] Foco no campo e na lista reutiliza os elementos existentes.
- [x] Leitura, menu, raciocínio e threads reutilizam ações reais dos nós;
  os controles nativos não mantêm um segundo executor.
- [x] Paleta por teclado/mouse preserva a mensagem anterior à busca;
  teclado configurado e Stream Deck usam as mesmas ações.
- [x] Carregamento assíncrono de filhos não restaura foco nem expande outra
  conversa após troca de contexto.
- [x] Regressões frontend/backend, TypeScript, lint e documentação.
- [ ] Aceite manual acumulado com NVDA e equipamento físico.

Produto `product-v31-chat-navigation-batch96`: **121 comandos / 51 locais /
64 bindings padrão**. Baseline global **58 I / 24 P / 2 N** inalterada.
Atualizadas as descrições de R07 para não repetir como pendências Ctrl+L e
Ctrl+S/O/Shift+S, já migrados em lotes anteriores. Contagem de catálogo não
substitui o fechamento das famílias restantes nem promove critérios finais.

Removidos o sinal `readingMessageId` e seus setters/wrappers sem consumidores:
a leitura é local ao nó capturado. Menu, Enter, R e expansão/recolhimento não
mantêm efeito paralelo; gestos nativos de navegação por irmãos e paginação
continuam próprios da lista. Controles internos recebem Enter sem abrir leitura
acidentalmente. Leitura, menu e raciocínio continuam disponíveis durante
streaming quando suas capacidades reais permitem.

O fechamento do menu não rouba foco da leitura; Escape retorna ao nó original,
inclusive após abertura pela paleta. ChatInput com autocomplete fechado permite
foco por comando; aberto bloqueia. O chat modal sobre editor usa seu vínculo de
conversa, não exige conversationId na aba hospedeira. A lista vazia recebe foco
por atributo declarativo, sem escolher uma última mensagem inexistente.

Evidências automatizadas (19/09/2026):
- Backend: **741 testes/subtestes App PASS (160,612 s)**;
  `commandcatalog`, `commandbindings`, `commandconfig` e `go vet ./internal/app`
  PASS. Classificação local, origem `ui.action` explícita nos sete IDs, ausência de
  ledger/defaults adicionais e recusa headless/global cobertas.
- Frontend: consolidada **2202 testes / 132 arquivos PASS (33,62 s)**.
  Recorte do lote95 ampliado com `ChatSession|ChatSurface|ChatPage|chatSession|chatStore|MessageNode|MessageList|useContextMenu`.
  Inclui registry (36), MessageNode legado (8), Node com Provider/store/
  ChatMessage/leitura reais (19), ChatSessionView messageActions (28), dentre
  outras regressões. Fixtures antigas foram atualizadas sem remover assertions.
- Depois da consolidada: Topbar chatNavigation **42/42 PASS**, incluindo dois
  testes novos de limpeza de capturas quando catálogo falha ou abertura fica
  obsoleta. Transportes de Wails e hardware são doubles; efeitos do chat têm
  provas separadas com componentes/stores reais, não só contadores de callbacks.
- ESLint nos 27 arquivos alterados PASS; sem alteração de CSS. TypeScript e
  diff-check validados no fechamento. Teste de cancelamento de captura em
  configurações passou a usar interação completa de mouse/foco, mantendo as
  duas assertions de cancelamento tardio após desmontagem.

Roteiro acumulado em `docs/content/recursos/COMANDOS.md`, seção “Ações de
apresentação do chat”. Não executados Wails dev/build/generate, ACP, executáveis
de diagnóstico, banco pessoal nem equipamento físico. Geração oficial de
bindings e aceite manual continuam pendentes. AEP permanece **In Progress**.

## 97. Ajuda e rótulos do mapa efetivo — 19/09/2026

Bloco transversal de R07.4: interface deixa de anunciar defaults fixos nas
famílias migradas. Não adiciona comandos, ledger, migração de dados nem IPC
por tecla. Produto v31 permanece em **121 comandos / 51 locais / 64 defaults**;
baseline **58 I / 24 P / 2 N**, sem promoção de critério global.

- [x] Publicar para apresentação somente a projeção aceita pelo controller
  existente; limpar na invalidação e recusar identidade de outro owner,
  sessão ou workspace. Não carregar mapa independente nem resolver prioridades.
- [x] Respeitar remapeamento, supressão, sequência, barreira contextual null
  e ausência de superfície conhecida, sem fallback para atalhos padrão.
- [x] Topbar: menu principal, nomes acessíveis de menu/retorno, botão da paleta,
  criação de workspace e itens do menu de nova aba refletem o mapa.
- [x] WorkspaceToolbar: criação de workspace e prefixo real de novas abas;
  tecla direta de criação não é anunciada como prefixo de menu.
- [x] Chat: limpar, modelo, histórico e perfil; identidade do seletor de modelo
  independe do texto do atalho tanto para provedor nativo quanto para agente.
  Modo do agente não se torna alvo substituto quando não existe modelo.
- [x] Editor: menus de arquivo e controles de Arquivo/Formatar/Inserir/Modo,
  slides, fullscreen e chat contextual. Não anunciar Ctrl+N na ação local
  Novo, que não é equivalente ao menu de criação de aba.
- [x] Ajuda: combinações efetivas, prefixos, ausência de mapa e gestos nativos
  identificados; strings em pt-BR/en/es e Modal compartilhado preservados.
- [ ] Aceite manual acumulado de rótulos, remapeamento e NVDA.

As combinações indicam associação, não disponibilidade de execução naquele
instante. Guards de contexto, composição, modal e alvo permanecem no fluxo
existente. Ctrl+? continua atalho próprio da ajuda; F2 de renomeação e gestos
internos de componentes não foram declarados migrados por esta seção.

Implementação paralela com três agentes Luna e revisão/integração principal.
Na revisão, corrigido o alvo de modelo de agente para não depender de
data-shortcut nem selecionar o controle de modo. Testes reais de pickers,
menus, teclado e foco acompanham os testes da projeção.

Evidências automatizadas (19/09/2026):

- Frontend consolidado: **2315 testes / 140 arquivos PASS (29,30 s)**,
  incluindo comandos, Topbar, chat, editor, pickers, menus e ajuda.
- Projeção: 10 testes de remapeamento, supressão, contexto/barreira,
  identidade, invalidação e prefixos. Topbar: 251 testes, incluindo mapa
  reativo em rótulos e prefixo da toolbar; paleta real: 76 testes.
- ChatToolbar/AgentOptionsPickers: 81 testes, incluindo distinção real entre
  modelo e modo do agente. Ajuda: 7; menus/toolbar editor: 5.
- TypeScript e ESLint no recorte: PASS. Diff-check no escopo: PASS.

Roteiro manual em `docs/content/recursos/COMANDOS.md`, “Atalhos mostrados na
interface”. Não executados Wails, testes ACP, executáveis de diagnóstico,
banco pessoal ou equipamento físico; sem alteração do backend neste lote.

## 98. Apresentação de tarefas, perfis e terminal — 19/09/2026

Bloco implementado em três frentes Luna com revisão/integração principal.
Produto `product-v32-page-presentation-batch98`: **130 comandos / 60 locais /
66 defaults**. São nove ações efetivas adicionais, não nove critérios finais
encerrados. Baseline global **58 I / 24 P / 2 N** preservada; R07 parcial.

- [x] Listas de tarefas: abrir criação, abrir edição do item escolhido e focar
  busca; IDs `tasklists.create.open`, `.edit.open`, `.search.focus`.
- [x] Perfis: as mesmas três apresentações, sob `profiles.*`, reutilizando
  formulário, grid e busca existentes.
- [x] Terminal: `terminal.sessions.open`, `terminal.focus.input` e
  `terminal.focus.history`, usando picker e controles existentes.
- [x] Catálogo localizado pt-BR/en/es, allowlist local fechada e compartilhada
  por paleta, teclado e evento do Deck. Sem ledger para abertura/foco.
- [x] Ctrl+N das duas páginas migra para resolução contextual; não executa
  simultaneamente criação de aba. Sequências do workspace continuam no mapa.
  Supressão/ramo null não permite fallback para sequência.
- [x] Captura de página, owner/sessão/workspace/aba e objeto selecionado antes
  da paleta; invalidação por mudança, desmontagem, modal, blur e IME.
- [x] Rótulos e ajuda respeitam a precedência contextual sobre prefixos.
- [x] Gate automatizado consolidado: backend, frontend, TypeScript e ESLint
  aprovados após revisão, com resultados abaixo.
- [ ] Gate manual: roteiro “Listas de tarefas, perfis e terminal” em
  `docs/content/recursos/COMANDOS.md`, incluindo NVDA/Deck e remapeamento.
- [ ] Geração oficial de bindings/qualificação Wails permanece pendente;
  arquivos gerados não foram alterados à mão.

Limites e próximo bloco de domínio: salvar/excluir/duplicar listas ou perfis,
ativar perfil e criar/interromper/encerrar processo do terminal continuam nos
serviços existentes, não na classificação de apresentação. No terminal,
Ctrl+C/botão chama `terminalStore.interrupt` → `InterruptTerminalCommand`;
criação/encerramento usam `CreateTerminalSession`/`CloseTerminalSession`.
Migrar exige capturar e validar a sessão/processo da operação e seu resultado
no percurso de domínio; abrir um picker não substitui essa migração.
Hotkeys de perfil/job e demais pendências anteriores continuam em R07.

Evidências de código: `commandPagePresentation.ts`, páginas TaskLists/Profiles/
Terminal, Topbar, `app_command_page_presentation.go`, testes de catálogo,
projeção contextual, registry, componentes e integração Topbar.

Evidências automatizadas (19/09/2026): backend
`go test ./internal/app -run TestCommand -count=1` **PASS**, 648 casos/subcasos,
145,559 s. Teste específico de fallback exige `NoMatch`; supressão, revisão,
conflito e destino durável não liberam sequências. Fingerprints e ordem dos
defaults anteriores preservados. Recorte registry/páginas/teclado/rótulos:
62 testes em cinco arquivos PASS.

Consolidação frontend: **2443 testes / 151 arquivos PASS (29,90 s)**.
Inclui 52 testes de integração Topbar + registry real + paleta, pelos
ingressos UI/teclado/Deck e abertura da paleta por mouse ou Ctrl+K; captura
obsoleta e modal/ABA recusados, sem transporte durável. Perfis: 11 testes,
incluindo edição assíncrona concorrente com Novo, blur e composição; respostas
atrasadas não abrem outro formulário. TypeScript, ESLint no recorte e
`git diff --check` no escopo: **PASS**.
Não executados Wails, ACP, binários de diagnóstico, banco pessoal ou hardware.

## 99. Interrupção do terminal e salvamento real de listas — 19/09/2026

Produto `product-v33-terminal-interrupt-batch99`: **131 comandos / 60 locais /
66 defaults**. Um novo comando de efeito backend; não há novo critério global
fechado por contagem de catálogo. Baseline **58 I / 24 P / 2 N**, R07 parcial.

- [x] `terminal.command.interrupt` localizado pt-BR/en/es; somente paleta,
  teclado local e Stream Deck. Não é apresentação/local-ui, CLI ou hotkey global.
- [x] Captura autoritativa da aba/binding e sessão/geração gerenciada antes da
  reserva; preparação compara IDs visíveis sem selecionar outro alvo.
- [x] Token opaco, uso único, barreira entre validação e escrita de Ctrl+C;
  mudança de geração, fechamento/substituição e replay recusados antes do efeito.
  Captura durante envio ainda não estabelecido é recusada.
- [x] Handoff curto reserva somente a sessão; a escrita PTY ocorre fora dos
  locks de autenticação, workspace e registro do terminal. Concorrência ocupada
  é recusada sem espera nesses gates; não cria goroutine destacada.
- [x] Frontend revalida owner/sessão/workspace/aba/execução, modal, blur e IME;
  paleta, binding pessoal e Deck usam reserva/prepare/take/commit/result.
- [x] Ctrl+C e botão do terminal deixam de chamar interrupção diretamente;
  seleção continua copiável. Gesto Ctrl+C restrito à superfície do terminal,
  sem repetição automática e sem novo binding global.
- [x] Auditoria redigida da invocação sem texto de entrada/saída; erro de escrita
  ou resposta perdida após commit não dispara retry nem sucesso presumido.
- [x] Bug independente: editar lista chama persistência real; rejeição backend
  propaga ao formulário, mantém dados e não anuncia sucesso. Rename existente
  trata rejeição do store. Isto não é migração do CRUD para o command manager.
- [x] Gate automatizado consolidado: regressão frontend, backend de comandos,
  testes finais de handoff/lifecycle, TypeScript e ESLint aprovados.
- [ ] Gate manual acumulado: interrupção com seleção/sem seleção, paleta,
  tecla pessoal e Deck; troca de aba/execução e modal. Roteiro em COMANDOS.md.
- [ ] Geração oficial de bindings/qualificação Wails e hardware permanecem
  pendentes, sem editar arquivos gerados à mão.

Limite preciso: o token identifica sessão e geração de input gerenciado pelo
app, não PID de cada subprocesso que o shell iniciar autonomamente. A ação
envia o Ctrl+C normal do PTY; não executa shell arbitrário nem encerra a sessão.
Gestos/botões usam o ingresso UI; bindings pessoais preservam keyboard.local
e reservas físicas preservam streamdeck.key.

Lacunas identificadas para o próximo bloco de domínio, sem wrappers de teste:

1. **Listas:** UpdateTaskList ainda escreve por ID+owner, sem versão esperada
   ou CAS. Criar/salvar/excluir pelo command manager precisa de captura,
   preparo e commit transacional; exclusão exige decisão backend de uso único.
2. **Perfis:** JSON e ativação/autocorreção multi-arquivo não têm commit atômico
   com versão esperada. Mudanças podem afetar permissões efetivas; precisam
   também invalidar/revalidar capability epoch. Confirmação só na UI não basta.
3. **Terminal:** criar/encerrar sessões ainda usa os serviços existentes;
   interromper não fecha essa lacuna. Fechar requer lifecycle e resultado
   autoritativo do encerramento, distintos de escrever Ctrl+C.

Não se marca migração completa desses grupos nem se amplia o catálogo com
handlers que apenas chamariam serviços legados sem as garantias acima.

Frontend consolidado (20/09/2026): **2490 testes / 155 arquivos PASS (32,10 s)**,
incluindo registry real, Topbar pelos quatro ingressos, bridge, páginas e
store. TypeScript e ESLint do recorte aprovados. Backend de comandos:
**650 casos/subcasos PASS (159,091 s)** antes do ajuste final de handoff;
revalidação desse ajuste registrada abaixo.

Revalidação final integrada (20/09/2026): testes de interrupção, preparação
sem autoridade, snapshots, handoff, cancelamento pré-write, replay e lifecycle
**PASS** (App 21,005 s; workspace 0,535 s; terminal 1,817 s). PTYs simulados,
sem processo real. Métodos novos sem consumidor produtivo foram removidos;
testes exercitam Prepare/Execute/Cancel usados pelo App.
Detector Go `-race` indisponível neste ambiente sem compilador C/gcc;
testes de concorrência com PTY fake não substituem essa qualificação.

Não executados Wails, ACP, binários de diagnóstico, banco pessoal ou hardware.
`git diff --check` do escopo passou; a checagem global encontra whitespace
preexistente em `frontend/wailsjs/go/models.ts`, não alterado neste lote.

## 100. Mutações de listas e lifecycle de sessões — 20/09/2026

Listas e sessões integradas e verificadas automaticamente; perfis permanecem
pendentes. Não representa fechamento automático de R07 nem alteração
da baseline reconciliada **58 I / 24 P / 2 N**.

- [x] Listas: leitura de conteúdo e fingerprint no mesmo snapshot, preparo
  imutável e commit transacional com owner e comparação de versão.
- [x] App: criar, atualizar, duplicar e excluir pelo executor central; exclusão
  exige decisão backend. Resultado privado efêmero, sem conteúdo no ledger.
- [x] Testes App com banco temporário: criação, atualização, duplicação, CAS
  após preparo, exclusão após decisão e recusa de replay.
- [x] Tela de listas: revisão de rascunho/seleção, apresentação após
  sucesso, proteção contra respostas atrasadas e regressão consolidada.
- [x] Terminal: integração e revisão de criação/vínculo e
  fechamento/desvínculo; teardown continua após desvínculo persistido mesmo
  com cancelamento do caller, e falha posterior é resultado indeterminado.
  Criar preserva a sessão anterior e reutiliza a aba; fechar exige decisão.
  Botão, paleta, teclado e Deck compartilham preparação e commit.
- [x] Perfis: base de journal e fingerprint, serialização de leitores e
  escritores; mutações multi-arquivo deixam intenção durável.
- [x] Perfis: recuperação roll-forward do journal para estados anterior/posterior
  conhecidos; conflito externo é recusado. Falha de rename preserva o destino,
  sem fallback de apagar o arquivo; paths restritos a JSON e raízes resolvidas.
- [x] Perfis: contrato de commit coordenado com grants e capability epochs.
  Resolvido nas seções101–102, com claim específico e revalidação; não houve
  remoção indiscriminada da proteção de `MutatesEffectiveCapability`.
- [x] Gate automatizado consolidado de listas e sessões (não inclui migração
  de perfis, ainda pendente).
- [ ] Gate manual acumulado e geração oficial dos bindings pelo usuário.

Lacuna identificada neste recorte, resolvida nas seções101–102: o journal dos JSON não torna atômica a
alteração conjunta de arquivo, autorizações no banco e epochs. A integração
precisa derivar no backend os perfis afetados e o impacto de capacidade,
coordenar invalidação/revogação e somente depois publicar callbacks/eventos.
O legado não deve ser contado como comando migrado por ganhar um wrapper.

Restrições mantidas: nenhum Wails, ACP, binário de diagnóstico, PTY real,
hardware ou banco pessoal usado na validação deste bloco.

Evidência intermediária antes da última integração UI do terminal:
frontend **2514 testes / 159 arquivos PASS** (36,55 s); backend App
**656 casos/subcasos PASS** (155,93 s, filtro TestCommand|TestTerminalSession).
Domínios tasklist/profiles/configdir e recorte database tasklist command PASS;
go vet dos três domínios PASS. Testes de lifecycle terminal/workspace com
simulações PASS. TypeScript PASS antes da extensão final da UI terminal.
O gate final abaixo deve revalidar essa extensão, não reaproveitar os números
intermediários como evidência de código ainda não testado.

Gate final (20/09/2026): **2593 testes frontend / 161 arquivos PASS (34,12 s)**;
TypeScript e ESLint do recorte produtivo PASS. Inclui oito combinações de
ingresso para criar/fechar sessão, recusa Deck sob modal, binding órfão,
respostas atrasadas/ABA nos stores e evento versionado de vínculo do workspace.
O gate manual permanece aberto; não houve execução do aplicativo nem PTY real.

Revalidação backend final: `go test ./internal/app -run
'TestCommandPageMutation|TestTerminalSession|TestCommandTerminalInterrupt|TestCreateTerminalSessionAndCommit'
-count=1` PASS (3,322 s). Abrange o recorte alterado depois da rodada App
consolidada de 656 casos/subcasos; não é nova execução integral do backend.

## 101. Coordenação produtiva das mutações de perfis — 20/09/2026

Não adiciona comandos ao catálogo: **137 comandos / 60 locais / 66 defaults**
e baseline **58 I / 24 P / 2 N** mantidos. Este bloco remove a lacuna de
coordenação dos writers da tela antes do handoff de comandos de capability.

- [x] Impacto derivado do plano no backend, CAS final antes de autorizações,
  token consumido após início da escrita e resultado indeterminado explícito.
- [x] Exclusão: intenção durável antes do arquivo; grants revogados depois;
  restauração journalizada se a revogação falha, conforme AEP-0101.
- [x] Journal/rollback incerto conserva intenção e falha fechado; notificações
  de jobs fora dos locks do Manager e do serviço, sem repetição de publicação.
- [x] Controller/Wails de criar, editar, duplicar, excluir e ativar usa o
  coordenador; não existe fallback sem coordinator para escritas da tela.
- [x] App invalida epochs e retira mapa antes da escrita, cerca callbacks e
  revalida sessão; reconstrução não repete efeito nem reabre mapa antigo.
- [x] Testes automatizados de domínio, controller, adapter e App.
- [x] Migração das cinco operações para catálogo/ledger/handoff e ingressos
  paleta/teclado/Deck. Resolvida na seção102, com claim de commit específico
  para a transição que invalida a própria admissão antiga.
- [ ] Validação manual acumulada de perfis e mapa após alteração.

Os métodos nativos existentes não são contabilizados como comandos migrados.
Importação e writers internos fora do controller conservam seu escopo; não se
declara atomicidade distribuída entre SQLite e filesystem. A segurança decorre
da intenção durável, bloqueio de grants e recuperação/compensação explícitas.

Evidências (20/09/2026): regressão App `TestCommand|TestTerminalSession|
TestProfileMutationApp`: **664 casos/subcasos PASS** (183,949 s). Suites completas
profiles/profileaccess/jobprofilegrant/configdir PASS (4,452 / 2,939 / 0,555 /
0,492 s); recorte controllers e Wails `TestProfiles|TestUpdateProfileMediaSupport`
PASS (1,541 / 2,003 s). `go vet` dos seis pacotes alterados PASS. Frontend de
perfis: **28 testes / 3 arquivos PASS** (4,39 s), sem mudanças de interface.

Cobertura inclui rollback com preservação de bytes, conflito externo sem
sobrescrita, intenção anterior não substituída, CAS/ABA e decisão pendente,
cancelamento, notificação de jobs fora dos locks e exatamente uma vez,
transição de epochs durante publicação e rebuild do mapa real. A leitura de
perfil ativo para MediaSupport usa snapshot único, sem auto-heal na preparação.
Não há wrapper de commit sem consumidor produtivo nem fallback de escrita
sem coordenador no controller. Bindings Wails públicos não mudaram; nenhum
gerador foi executado. Não executados ACP, Wails, PTY real ou banco pessoal.

Após estabilização dos três workers, `go test ./internal/app -run
'^TestProfileMutationApp' -count=1` PASS (20,928 s), e `git diff --check`
do escopo passou. O total consolidado de 664 acima não foi recontado nesta
última execução focada.

## 102. Cinco mutações de perfis integradas — 20/09/2026

Catálogo v35: **142 comandos / 60 locais / 66 defaults**. Fecha a migração
pendente da seção101; não promove critérios globais por contagem de comandos.
Baseline **58 I / 24 P / 2 N** permanece sem nova reconciliação.

- [x] `profiles.create`, `.update`, `.duplicate`, `.delete`, `.activate`:
  definições backend contextuais nas três línguas; nenhum atalho padrão novo.
- [x] Botões existentes, paleta, teclado pessoal e Stream Deck usam o pipeline
  compartilhado; captura de página/formulário, seleção, owner, workspace,
  draft e modal topmost. Não operam globalmente sobre último perfil visitado.
- [x] Conteúdo/fingerprint lidos juntos; formulário conserva o snapshot da
  abertura, payload selado antes da decisão; CAS repetido na gravação.
- [x] Controller nativo e catálogo compartilham plano/publicação, sem writer
  alternativo. Hook de edição admite apresentação sem portas falsas de CRUD.
- [x] Claim efêmero de commit sob epoch/sessão/contexto exatos, antes da
  auto-invalidação. Cancelamento anterior impede claim; espera posterior é
  limitada a 35 s e exclusiva de handlers backend que alteram capabilities.
- [x] Exclusão exige decisão backend e usa intenção durável/compensação de
  grants. Nenhuma confirmação paralela da página; replay não repete efeito.
- [x] Rebuild somente depois do ledger terminal. Consulta de resultado pode
  sobreviver a mapa indisponível, sem dispensar autenticação do mesmo dono
  nem liberar Begin/Take/Commit; resultado incerto conserva mapa fechado.
- [x] Ledger não persiste nome, descrição ou conteúdo do perfil; resultado
  privado existe apenas na reserva autenticada e expira com ela.
- [x] Regressores de mocks antigos corrigidos sem alterar expectativas:
  contextos de roteamento preservados e eventos Wails simulados em teste.
- [x] Auditoria AEP-0058: removidas live regions concorrentes da captura Deck,
  configuração e importação; feedback usa announcer global, incluindo erros.
- [ ] Aceite manual acumulado de criar/editar/duplicar/excluir/ativar por
  ingressos e NVDA; instruções em `docs/content/recursos/COMANDOS.md`.

Evidências backend: App consolidado `TestCommand|TestTerminalSession|
TestProfileMutationApp` PASS (143,618 s); regressão final de perfis inclui
publicação incerta, exclusão negada e redação do ledger, PASS (25,256 s).
Suites completas controllers/profiles/profileaccess/jobprofilegrant/
commandsecurity/commandexecution/commandui PASS. `go vet` do App, controller,
security e execution PASS. Detector de corrida não executado: falta `gcc`.

A primeira execução integral frontend expôs 43 falhas em seis arquivos;
35 eram mocks de router/runtime incompletos, sete no fixture legado da
Topbar e uma auditoria de 15 live regions. Os recortes de correção passaram:
41 testes dos quatro mocks; 49 de configuração/importação/auditoria/Topbar.
A rodada integral posterior é registrada abaixo, sem substituir falhas por
exclusões de testes. Não executados Wails/build/generate, ACP, PTY real,
hardware ou banco pessoal; não houve commit, push ou PR.

Gate final frontend: **5059 testes / 420 arquivos PASS (75,05 s)**, execução
integral sem excluir os seis arquivos que haviam falhado. TypeScript
`tsc --noEmit`, ESLint do recorte produtivo e `git diff --check` PASS.

Revisão final de concorrência: preflight que consulta portas fica fora do
gate; claim relê owner/source e cerca versões publicadas do host e snapshot
do workspace sob locks de memória, além da revalidação de epochs. Três testes
adversariais mudam host, workspace ou fonte entre preflight e claim e provam
recusa sem escrita. Recorte final App PASS (27,127 s), security PASS (3,852 s)
e execution PASS (5,093 s), incluindo limites determinísticos do deadline.

## 103. Reconciliação de migração e ingressos residuais — 20/09/2026

Auditoria somente leitura de código por dois agentes Luna, com conferência
dos achados pelo agente principal. Alterações deste lote são documentais;
nenhum teste, executável, Wails, ACP, hardware ou banco pessoal executado.
Catálogo permanece v35 **142/60/66**. Não houve recontagem dos 84 critérios.

### Reconciliação concluída

- [x] Corrigir resumo de catálogo da seção1 (46 era a fotografia da seção81).
- [x] Identificar percentuais 58/24/2 como baseline antiga, não medição atual.
- [x] Encerrar pendências históricas do broker de arquivos (82 → 83), contrato
  de perfis (100 → 101/102) e cinco ingressos de perfis (101 → 102).
  Evidências automatizadas permanecem nas seções de implementação citadas;
  os checkboxes de aceite manual não foram promovidos.
- [x] Separar tabelas iniciais `candidate.*` do inventário vigente. Afirmações
  antigas de catálogo ausente, Ctrl+N sem contrato ou Stream Deck só protótipo
  não são diagnóstico atual.

### Residuais comprovados e próximo lote

- [x] **R07 — Histórico:** implementado na seção104; o levantamento encontrou
  `HistoryPage.tsx`, `handleKeyDown`, tratando
  Ctrl+N diretamente. Apesar do nome `handleNewConversation`, seu efeito é
  somente `navigate('/')`; não cria conversa nem aba. Migrar preservando esse
  efeito, pelo comando de navegação apropriado, com precedência contextual,
  modal/IME/repeat e consumo único. Não substituir por criação de chat.
- [x] **R07 — Lista aberta no workspace:** implementado na seção104; o
  levantamento encontrou `TaskListView.tsx`, `onKeyDown`, chamando diretamente
  limpar lista (Ctrl+L), abrir criação de tarefa (N)
  e clonar lista (D). A montagem é real, via `TaskListWorkspacePanel`.
  Não confundir tarefa dentro da lista com criar lista na `TaskListsPage`,
  nem limpar conteúdo com excluir lista. Reutilizar contratos aplicáveis e
  completar alvo contextual/efeito faltante antes de retirar o listener.
- [ ] **R07/R11 — Voz por perfil:** `HotkeysController.RegisterActiveProfileHotkeys`
  registra no manager nativo e emite `interaction:hotkey:triggered`;
  `useInteractionProfile.ensureHotkeyListener` processa por listener singleton.
  Há throttle backend e frontend, sem ingresso canônico nesse percurso.
  Migrar com identidade de perfil/trigger, escopo de voz e foreground
  explícitos; CRUD de perfis já migrado não resolve este acionador.
- [ ] **R07/R11 — Hotkey de job:** `jobs.Manager.registerJobHotkey` registra
  callback nativo, avalia `when` e chama `executeJob` diretamente. Infraestrutura
  de proveniência de jobs não equivale à migração desse ingresso. Integrar
  ao adapter `keyboard.global`, preservando `job.when` no runtime de jobs.

Ordem prática: fechar Histórico + lista do workspace; em seguida adapter
global compartilhado + consumidores voz/jobs. Gate do primeiro bloco:
tecla, botão, paleta e Deck convergem quando elegíveis; alvo/aba/modal corretos,
nenhum efeito duplicado ou ação em contexto errado; testes reais de cada
componente e contrato backend para mutações, sem substituições semânticas.
Gate global: colisão/unregister, sessão/geração antiga, lock/logout, foreground,
deduplicação e `when` falso cobertos antes de remover os callbacks legados.

### O que não deve gerar migração artificial

Tab/focus trap de modal, navegação ARIA em picker/menu/grid, seleção e digitação
nativas no editor não são, por si, comandos de aplicação. Listeners de
mensagem que delegam aos callbacks de `ChatSessionView`, por sua vez, já
chegam a `requestMessaging`/`requestChatNavigationCommand`; Mermaid e terminal
também possuem ingressos roteados. A presença de `onKeyDown` não prova legado.
Esta inspeção não certifica todos os controles nem transforma toda navegação
local em exclusão: conferir o efeito final e o contrato, não só a tecla.

### Fora do próximo lote, ainda no AEP

R11.1/R11.2 continuam exigindo tools públicas `command_catalog`/`command_config`
e CLI de comandos (não encontradas em `internal/tools`/`cmd/asst`). R09/R10/R12
mantêm suas obrigações de produto/qualificação. Bindings oficiais, NVDA e matriz
física continuam pendentes nos recortes registrados. Import/export avançado
e gesto longo conservam a prioridade adiada pelo usuário. Não declarar
"restam apenas testes manuais" nem porcentagem nova a partir desta auditoria.

## 104. Histórico e ações da lista no workspace — 20/09/2026

Pacote conjunto aprovado pelo usuário. Catálogo
`product-v36-history-tasklist-batch104`: **144 comandos / 61 locais / 67 defaults**.
Não recertifica os84 critérios; baseline geral e gates físicos permanecem abertos.

- [x] Histórico: retirar listener Ctrl+N; adicionar default contextual para
  `navigation.workspace.open`. Preservar retorno a `/`, sem criar conversa/aba.
  Supressão não recupera listener legado; outras páginas conservam seu Ctrl+N.
- [x] Botão do Histórico solicita dispatcher local por evento fechado, com
  catálogo/mapa/sessão/foco/modal e IME, não chama handler de rota diretamente.
- [x] `tasklist.task.create.open`: apresentação local do formulário de tarefa
  na lista ativa; sem escrita/ledger nem criação de outra lista.
- [x] Duplicação da lista aberta reutiliza `tasklists.duplicate`, preparo
  atômico de alvo/fingerprint e commit existente; configuração copiada sem tasks.
- [x] `tasklists.clear`: efeito destrutivo backend com decisão, payload selado,
  CAS final de metadados/workflow/tasks/notas e transação que conserva lista e
  workflow. Replay, cancelamento, owner incorreto e concorrência recusados.
  Eventos `taskList:cleared`/`tasklist.list.cleared` somente após commit.
- [x] Botões, paleta, combinação pessoal e Deck usam os mesmos registries;
  fonte da lista vinculada à aba ativa, identidade da lista e estado capturado.
  Limpeza considera tarefas fora da página carregada; alvo anterior não é retomado.
- [x] N/D/Ctrl+L preservados como **gestos fixos locais de controles**, agora
  solicitando os comandos comuns sem writers diretos. Não ampliam a gramática
  de bindings simples para letras soltas nem viram defaults globais. Exigem
  foco no root, ausência de modal/repeat/IME/consumo anterior; N/D não escrevem
  sobre campos. Binding consumido tem precedência; supressão de um binding não
  equivale a desativar gesto nativo. Este limite está documentado ao usuário.
- [ ] Aceite manual acumulado do pacote, com NVDA/Deck: roteiro em
  `docs/content/recursos/COMANDOS.md`, “Histórico e lista aberta no workspace”.

Evidências: `go test ./internal/app -run
'TestCommand|TestTerminalSession|TestProfileMutationApp' -count=1 -timeout=300s`
PASS (170,304 s); recorte `TestCommandTaskListClear|TestCommandPageMutation`
PASS (23,252 s). Domínio e transação SQLite têm testes focados de limpeza,
CAS, owner, cancelamento e rollback. `go vet ./internal/tasklist ./internal/app`
PASS. TypeScript e ESLint do recorte PASS.

A primeira regressão frontend teve **5081 PASS e uma falha**: expectativa
histórica da allowlist local ainda60, agora61. Expectativa atualizada mantendo
assertions explícitas de que abertura é local e limpeza não é. Recorte posterior
de allowlist/componente/registry: **33 testes PASS**. Gate integral final:
**5083 testes / 420 arquivos PASS (79,67 s)**, sem exclusões. TypeScript
reexecutado após os últimos testes PASS; ESLint do recorte e diff-check PASS.

Persistência: uma execução ampla durante o trabalho paralelo falhou com
`SQLITE_BUSY` em `TestWithSQLiteBusyRetryWaitsForTransientWriterLock` e
`TestListTasksPageWithContext_RetriesMetadataReadDuringTransientLock`. Não foi
atribuída preexistência sem baseline. Os dois casos mais a regressão de
transações passaram isoladamente (0,816 s). Reexecução final das suites
**completas** `go test ./internal/tasklist ./internal/database -count=1
-timeout=180s` PASS (0,436 s / 14,363 s), sem excluir testes ou afrouxar limites.

Nenhum Wails/build/generate, ACP, executável avulso, hardware ou banco pessoal
utilizado. Não houve commit/push/PR. Não engloba persistência do formulário de
tarefa, outros botões da lista, hotkeys de voz/jobs nem tools/CLI.

## 105. Preparação dos ingressos de voz e jobs — 21/09/2026

Escopo aprovado: substituir o ingresso hotkey → ação, preservando os serviços
de voz e o runtime de jobs. Esta rodada não declara essa substituição concluída.
Catálogo v36 **144 comandos / 61 locais / 67 defaults** inalterado.

- [x] Controller de voz aposenta a geração anterior antes de ler novo perfil;
  callbacks antigos não emitem após reload/Stop, e novo registro não herda o
  throttle do trigger anterior. Input desabilitado não registra hotkeys.
- [x] Testes sem hardware para geração antiga, Stop, entrega em andamento,
  throttle, ausência de dependência durante reload e input desabilitado.
- [x] Modificadores do registro nativo usam as constantes reais de Windows,
  Linux e macOS. Parser recusa modificadores desconhecidos/duplicados e tokens
  vazios, em vez de registrar silenciosamente outra combinação.
- [x] Listener termina ao fechar o canal, descarta eventos cancelados ou de
  inscrição aposentada e captura o canal antes de publicar a inscrição.
  Callback e unregister nativo não mantêm o mutex do manager. Callbacks já
  admitidos ainda podem concluir; não se afirma join nativo ou no-repeat.
- [x] Suite `internal/hotkey` repetida30 vezes PASS (1,662 s), sem hardware.
- [x] Voz: receptor registrado por lifecycle, seleção de uma única superfície
  elegível, cleanup de listener e recusa com modal. Sem singleton que aponta
  para a última instância montada e sem o segundo throttle frontend de1s.
  VoiceButton reutiliza o gate STT existente. Inicialização pendente é
  invalidada por desativação/reativação, unmount, cancelamento e mudança de
  perfil; abrir modal antes de concluir a inicialização também impede início.
  Carregamento de perfil tem geração própria e recusa respostas atrasadas.
- [x] Jobs: snapshot privado do trigger exige owner, UUID físico, fingerprint,
  keys/when e habilitação atuais; callback não entrega uma cópia rasa antiga.
  A borda `executeJob` revalida antes/depois de aplicar owner ao contexto e
  antes do executor; trocar somente o registry não troca silenciosamente o
  alvo preparado. `when`, proveniência e runtime existentes são preservados.
  Unregister aposenta o lifetime, inclusive após registrar o mesmo job novamente.
- [x] Suite completa `go test ./internal/jobs -count=1 -timeout=180s` PASS
  (6,095 s). `go vet ./internal/jobs ./internal/hotkey ./controllers` PASS.
- [ ] Gate de migração: publicar gramática/projeção `keyboard.global` com
  ownership exclusivo sobre a combinação, inclusive quando a janela tem foco.
- [ ] Gate de migração: converter os triggers de perfil/jobs em bindings
  autoritativos, com conflitos explícitos e uma única inscrição por combinação.
- [ ] Gate de migração: ligar voz à captura e revalidação de perfil efetivo,
  surface e geração, inclusive quando a janela está em segundo plano.
- [ ] Gate de migração: ligar o trigger de jobs ao executor comum, preservando
  `when`, grants, proveniência e a definição vigente; remover ingresso direto
  apenas quando houver cobertura equivalente.
- [ ] Gate nativo (código): substituir/completar a borda Windows para garantir
  no-repeat e encerramento que não aguarde a tecla ser solta. A biblioteca
  atual usa `Modifier uint8`,
  não transporta `MOD_NOREPEAT` (0x4000), e processa mensagens `WM_HOTKEY`
  enfileiradas depois de observar release por polling. Deduplicação dos canais
  isoladamente não prova uma única ocorrência por pressão física.
- [ ] Gate físico: validar o backend corrigido com tecla mantida durante
  reload/Stop, lock/unlock e combinação compartilhada por observadores.
  Não é a única pendência: o gate nativo acima exige implementação antes.

Evidência inicial: `go test ./controllers -run TestHotkeysController -count=1
-timeout=120s` PASS; `go vet ./controllers` PASS com acesso ao cache normal.
Não foram executados Wails, ACP, testes físicos ou banco pessoal.

Na revisão, testes expuseram duas falhas da primeira implementação frontend:
cancelamento deixava `isLoading` preso por compartilhar geração com o fetch de
perfil; modal aberto enquanto `initSTT` aguardava ainda permitia gravação.
Ambas corrigidas com regressões, sem remover assertions. Recorte inicial
integrado:18 testes PASS. TypeScript e ESLint do recorte PASS.

Gate final: **5097 testes / 422 arquivos frontend PASS** (82,42 s), sem
exclusões. Recorte final de voz (hook, routing e componente):20 testes PASS,
incluindo callback de listener aposentado e receptor removido durante seleção.
Diff-check do recorte PASS. Os gates de migração acima continuam abertos:
não foi publicado `keyboard.global` nem substituído o ingresso pelo executor.

## 106. Borda nativa Windows cancelável — 21/09/2026

Fecha o gate de implementação nativa da seção105; os gates de migração de
voz/jobs continuam abertos. Catálogo v36 **144 comandos / 61 locais / 67 defaults**
inalterado, sem recontagem dos84 critérios.

- [x] Windows usa RegisterHotKey com modificadores uint32 e MOD_NOREPEAT;
  não trunca 0x4000 para o tipo uint8 da dependência nem tenta fallback repetível.
- [x] Registro, leitura da fila e remoção pertencem à mesma thread nativa.
  Ao terminar, a thread dedicada não volta ao pool com mensagens residuais.
- [x] Mensagens de outro ID não são entregues; entrega pendente é cancelável,
  sem polling de release. Espera ociosa tem teto de50ms para observar stop;
  input acorda a espera, portanto esse teto não é atraso fixo por tecla.
- [x] Encerramento aguarda a remoção nativa, não o callback em andamento.
  Erros nativos são registrados e propagados por Unregister; chamadas
  concorrentes removem uma única vez. Uma instância encerrada não é reutilizada.
- [x] Testes por porta Win32 controlada, incluindo integração com Manager,
  callback bloqueado, evento pendente, erro de limpeza e afinidade de thread.
- [x] `go test ./internal/hotkey -count=30 -timeout=120s` PASS (1,732s).
- [x] `go test ./internal/jobs -count=1 -timeout=180s` PASS (6,086s).
- [x] Recorte `TestHotkey|TestPreparedHotkey` em jobs/controllers PASS
  (11,307s / 10,617s).
- [x] `go vet ./internal/hotkey ./internal/jobs ./controllers` PASS; exigiu
  acesso ao cache normal após uma tentativa limitada por permissão.
- [ ] Publicar projeção/resolução keyboard.global e migrar os bindings
  autoritativos de voz/jobs ao executor comum, conforme seção105.
- [ ] Aceite físico: manter tecla durante reload/Stop, soltar e pressionar
  novamente, validar lock/unlock e conflitos reais entre combinações.

As primeiras execuções dos testes novos expuseram sincronização incorreta nos
próprios testes (esperar fechamento antes de pedir stop e cancelar antes de
observar Wait). Corrigidas usando sinais de conclusão, sem remover os casos.
O filtro de ID é observado antes de cancelar, para teardown não esconder uma
entrega indevida. Nenhuma inscrição física foi usada nos testes.

Contrato Win32: [RegisterHotKey / MOD_NOREPEAT](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-registerhotkey)
e [espera por mensagens](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-msgwaitformultipleobjectsex).
Não houve Wails/build/generate, ACP, executável avulso ou acesso ao banco pessoal.
Não houve commit/push/PR. AEP permanece In Progress.

## 107. Gramática global, reserva nativa e semântica dos jobs — 21/09/2026

Continuação da migração aprovada de voz/jobs. **Não é migração concluída**:
os callbacks produtivos ainda chegam aos serviços existentes pelo ingresso
anterior. Catálogo v36 **144/61/67**, sem recontagem dos84 critérios.

- [x] `KeyboardGlobalTriggerPort` normaliza acorde simples v1, rejeita
  sequências e preserva namespace global. Local continua restrito ao local.
- [x] Porta global incluída em `commandProductTriggerPorts` para projeção.
  `ProjectComplete` continua exigindo porta explícita e `AllowedSources`
  compatível; normalização não equivale a ocorrência física autenticada.
- [x] Teste do App recusa pedido com origem global e acorde válidos, mas sem
  ocorrência nativa registrada. Não existe endpoint que promova UI a hotkey.
- [x] Manager reserva a combinação nativa canônica uma única vez, independentemente
  da ordem dos modificadores. Registro concorrente conflitante falha antes
  de chamar o backend nativo. Retirada mantém reserva até remoção confirmada.
- [x] Falha de remoção mantém reserva e erro; Stop aguarda remoções já em
  andamento fora do mutex, sem esperar conclusão de callbacks de ações.
- [x] Snapshot de ownership é tipado no espaço nativo e não compartilha memória.
  Não se converte VK para DOM `code` por suposição sobre layout de teclado.
- [x] Handler de jobs preserva `TriggerHotkey`, `keys` e `when` a partir de
  preparação privada, revalidada contra owner/UUID/fingerprint e configuração.
  Uma origem declarada no envelope não substitui essa preparação.
- [ ] Publicar bindings autoritativos de perfil/jobs, com fingerprints e
  aposentadoria por geração, no catálogo/resolvedor do produto.
- [x] Sincronizar ownership nativo Windows com o DOM antes de liberar a nova
  geração física — implementação na seção108; aceite físico continua separado.
- [ ] Montar ingresso global autenticado no executor e handoff de voz com
  perfil/surface vigentes; substituir os callbacks diretos após equivalência.
- [ ] Aceite físico consolidado de voz/jobs, tecla mantida, reload, segundo
  plano e lock/unlock. Não é a única pendência: os três itens acima são código.

Regressões já verificadas: suíte completa `internal/commandconfig` PASS
(7,031s); recorte do App de origem forjada, settings e resolução persistida
PASS (22,636s). As falhas de compilação durante integração foram snapshots
intermediários enquanto os agentes ainda editavam; não foram atribuídas à main.

Validação integrada: hotkeys repetidos30 vezes PASS (1,413s); controller de
voz PASS (10,209s); suíte completa de jobs PASS após a revisão final (16,127s).
Bindings, catálogo, contrato e executor comuns PASS (0,171s / 0,174s /
0,185s / 11,869s). `go vet` em hotkey/config/controllers/jobs/App PASS;
diff-check do recorte PASS. Race não certificado nesta rodada (indisponibilidade
de CGO/cache reportada pelo agente), sem alegar cobertura equivalente por repetição.
Testes cobrem troca de owner durante autorização de retry sem depender da
contagem interna de chamadas de autorização. Trabalho delegado a três agentes
Luna, integrado e revisado pelo agente principal. Sem Wails/build/generate,
testes ACP, executáveis avulsos, hardware, banco pessoal, commit/push/PR.

## 108. Coordenação produtiva entre hotkeys Windows e DOM — 21/09/2026

Entrega ligada aos registradores existentes de voz/jobs, não apenas um contrato
isolado. **Não conclui a migração desses ingressos para o executor comum**.
Catálogo v36 **144/61/67**, sem acréscimo ou recontagem dos84 critérios.

- [x] Manager publica o conjunto completo de reservas e aguarda confirmação
  exata de instância/revisão antes de `RegisterHotKey`. Propostas concorrentes
  são serializadas; ACK não autentica ocorrência nem executa comando.
- [x] Retirada publica a liberação somente após a remoção nativa; falha nativa
  mantém a reserva. Falha de registro não autoriza executar pelo caminho local
  antes da restauração do conjunto anterior.
- [x] Ponte compartilhada pela raiz do app e Topbar, fora de AuthGate, evitando
  depender do término do login para confirmar registros iniciados no login.
  Bootstrap assina eventos antes de consultar snapshot e fixa a instância pelo
  getter; eventos antigos não podem escolher a instância.
- [x] DOM compara virtual-key e modificadores do Windows/WebView2, não converte
  VK em posição física por suposição de layout. A mesma tecla aceita reservas
  distintas como Ctrl+A e Alt+A. A exclusão antecede down/repeat/up locais.
- [x] Mudança de reserva invalida a sequência e o estado de teclas locais;
  repetição mantida não se converte em novo pressionamento após a troca.
  Consulta por tecla permanece síncrona/em memória, sem chamada ao backend.
- [x] Falha transitória no bootstrap é repetida enquanto a ponte estiver
  montada. Desmontagem cancela novas consultas/ACKs; Shutdown libera esperas
  antes de aguardar encerramento dos controllers.
- [x] Registro de voz no startup não bloqueia a criação da WebView necessária
  para confirmar a publicação.
- [x] Inscrição Windows iniciada com tecla mantida descarta notificações até
  observar release; registro e retirada não aguardam esse release. Depois de
  admitida, a inscrição preserva presses rápidos enfileirados e deixa a política
  de repetição com `MOD_NOREPEAT`. A amostragem inicial não certifica lock/unlock.
- [x] ACK posterior ao cancelamento/deadline é recusado mesmo se o publisher
  ainda estiver retornando. Falha de proposta também tenta restaurar o conjunto
  anterior, sem chegar ao registro nativo.
- [ ] Publicar bindings autoritativos de perfil/jobs com fingerprints e
  aposentadoria por geração no catálogo/resolvedor do produto.
- [ ] Montar ingresso global autenticado no executor e handoff de voz com
  perfil/surface atuais; substituir callbacks diretos após equivalência.
- [ ] Aceite físico: colisão global/local, troca de perfil com tecla mantida,
  segundo plano, reload, lock/unlock. Outros sistemas operacionais não recebem
  certificação desta coordenação Windows.

Validação integrada e evidências finais registradas abaixo. Os testes usam
portas nativas controladas; nenhum registro físico, Wails/build/generate,
teste ACP, executável avulso ou banco pessoal foi usado. Os métodos Wails novos
não tiveram arquivos gerados editados à mão; a próxima execução normal de
Wails deve regenerá-los. Não houve commit/push/PR.

Evidências: suíte frontend completa **425 arquivos / 5.119 testes PASS**
(75,05s), seguida de **23 testes focados PASS** após o último ajuste de
isolamento dos consumidores compartilhados. TypeScript e ESLint do recorte
PASS. `go test ./internal/hotkey -count=30 -timeout=120s` PASS (5,227s);
suíte completa de jobs PASS (15,573s); recorte de voz/controllers PASS
(10,309s); App (`TestAppGlobalCommandOwnership` e
`TestCommandProductRejectsUnregisteredGlobalOccurrence`) PASS (21,868s).
`go vet` em hotkey/App/jobs/controllers PASS; diff-check do recorte PASS.
Não foi executada a suíte Go inteira nem certificado race nesta rodada.

Revisão principal corrigiu a identidade de combinações com mesmo VK/modificadores
diferentes, confirmação tardia, teardown mantendo a tecla pressionada e a
tentativa indevida de rearmar a inscrição a cada press (que perderia taps rápidos).
Três agentes Luna implementaram fatias separadas; integração e revisão no mesmo
worktree. Falhas iniciais de sandbox/esbuild/cache exigiram execução normal
autorizada; uma compilação intermediária viu a interface nativa ainda em edição
e passou depois da integração, sem atribuir o problema à main.

Referências do contrato nativo: [GetAsyncKeyState](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getasynckeystate)
(somente bit alto) e [KeyboardEvent do Chromium](https://chromium.googlesource.com/chromium/blink/+/master/Source/core/events/KeyboardEvent.cpp)
(`keyCode` de keydown/up no espaço de virtual-key do Windows).

## 109. Voz e jobs ligados ao executor comum — 21/09/2026

Catálogo v37: **146 comandos / 61 locais / 67 defaults locais**. Os novos
`voice.input.activate` e `job.run` aceitam exclusivamente `keyboard.global`.
As combinações existentes continuam configuradas nos perfis/jobs; sua projeção
autoritativa não cria uma segunda fonte editável nem novos atalhos locais.
Não há recontagem dos 84 critérios nesta entrega.

- [x] Callback nativo entrega ocorrência opaca, com geração/lifetime e
  fingerprint. Ocorrência vazia, registro aposentado, owner trocado ou trigger
  alterado não concedem autoridade. Dispatcher ausente recusa, sem fallback.
- [x] Publicação de bindings de perfil/job no resolvedor comum. IDs respeitam
  a gramática do contrato; metadados parametrizados têm política explícita.
  Configuração de hotkey inválida é isolada sem bloquear os demais comandos.
- [x] Executor global autentica sessão, fonte privada, configuração e alvo;
  entrada Wails não pode fabricar ocorrência física nem substituir argumentos.
- [x] Voz usa reserva/Take/Complete do broker existente. Evento apenas orienta
  a captura; perfil, trigger e bring-to-front são conferidos contra o handoff
  autoritativo. Troca/reload de perfil, superfície desmontada, ambiguidade ou
  modal invalidam a entrega; não há escolha de outro destinatário após esperar.
- [x] Job passa pela admissão da interface e decisão interativa antes de usar
  o handler/runtime existente, mantendo owner, trigger, `when` e alvo fixo.
  Nenhum novo caminho executa a ferramenta diretamente no callback nativo.
- [x] Consolidar regressão final do lote e evidência ponta a ponta de jobs:
  callback nativo simulado → dispatcher real → admissão da UI → questionário
  real → executor/serviço de ferramenta. Zero chamadas antes de confirmar,
  exatamente uma depois; ledger único `keyboard.global/succeeded`, run único,
  origem `user_hotkey` vinculada à invocation e trigger original preservado.
- [x] Preservar objeto de inputs vazio ao copiar a definição de job: `{}` não
  pode virar `null` e produzir fingerprints diferentes entre projeção e
  ocorrência nativa. Regressão verifica fingerprint e isolamento da cópia.
- [x] Aposentar ocorrências globais imediatamente no encerramento do runtime,
  antes de aguardar outros executores; orçamento de drenagem esgotado não
  mantém uma admissão nativa válida.
- [x] Reconciliação de ativações sem leases não avança a revisão nem invalida
  comandos concorrentes sem motivo. Lotes não vazios continuam invalidando
  antes da revalidação/mutação, inclusive se houver falha ou rollback.
  Teste integrado de job passou 12 vezes consecutivas após a correção.
- [x] Equivalência para jobs com perfil calculado por expressão: implementada
  na seção110, com alvo fixado e grant exato, sem herança implícita.
- [x] Equivalência para jobs longos: seção110 separa admissão e runtime sem
  desligar cancelamento, epochs, lifecycle ou deadline do chamador.
- [ ] Aceite físico consolidado: voz com perfil efetivo, segundo plano,
  bring-to-front, colisão DOM/nativo, tecla mantida durante reload, modal,
  job confirmado/cancelado, troca de sessão e lock/unlock.

O envelope de voz aceita apenas identidade de perfil/trigger e bring-to-front;
nunca áudio ou transcrição. Comandos locais de navegação continuam no caminho
em memória, sem nova consulta ao backend por tecla. Bindings Wails gerados não
foram editados manualmente nem regenerados nesta rodada.

Sem Wails/build/generate, testes ACP, executáveis avulsos, hardware ou banco
pessoal. Não houve commit/push/PR. Três agentes Luna trabalharam em adapters,
frontend e testes; integração/revisão e correções de contrato pelo principal.

### Evidências finais da seção109

- `go test ./internal/app -run '^(TestCommand|TestAppCommand|TestAppGlobalCommandOwnership)' -count=1 -timeout=300s`:
  **PASS, 162,909 s**. Inclui os testes globais, catálogo, lifecycle e ingressos.
- Integração de job acima repetida com `-count=12`: **PASS, 30,464 s**.
- Suítes completas `internal/jobs`, `controllers`, `internal/hotkey` e
  `internal/commandjobactivation`: **PASS**; incluem ausência de dispatcher,
  owner/stale/when com controles positivos e revisão conservadora em falhas.
- `internal/wailsapi`, recorte `^Test(Hotkeys|CommandCatalog)`: **PASS**.
- `go vet ./internal/app ./internal/jobs ./controllers ./internal/commandjobactivation`:
  **PASS**.
- Frontend: **427 arquivos / 5.133 testes PASS**, TypeScript sem emissão e
  ESLint do recorte **PASS**; regressões focadas de voz/handoff: **27 testes**.

Falhas encontradas e corrigidas antes do fechamento: política de argumentos
da voz incompatível com o binding parametrizado, ID de layer fora da gramática,
fingerprint divergente após cópia de inputs vazios e invalidação concorrente
por reconciliação sem leases. Não se introduziu retry automático de comando,
espera artificial ou retorno à execução direta para fazer os testes passarem.
Não é certificação de `go test ./...`, `-race` ou dos aceites físicos pendentes.

## 110. Perfis dinâmicos e duração de jobs — 21/09/2026

Fechamento das duas lacunas de código da seção109. Catálogo v38
`product-v38-job-runtime-profiles`, **146 comandos / 61 locais / 67 defaults**;
sem novos comandos ou combinações. A contagem global dos critérios não é
recalculada por este lote.

- [x] Preparar o slug de profile pela definição autoritativa e pelo resolvedor
  existente, sem resolver antecipadamente os demais inputs do job.
- [x] Vincular slug resolvido ao fingerprint da **expressão** e geração exata
  do grant. Ausência, revogação/regrant, resultado vazio ou inválido recusam;
  confirmação não cria autorização nem transforma template vazio em herança.
- [x] Conferir o alvo efetivamente resolvido antes da ferramenta em cada
  tentativa. Mudança de secret/tempo/evento não redireciona uma confirmação;
  recusa é permanente, sem retry para contornar a autorização.
- [x] Separar o orçamento de preparação/decisão/fila do lifetime de execução
  somente para handlers `job` explicitamente configurados pelo host.
- [x] Preservar deadline/cancelamento do chamador, lifecycle e epochs. Fila
  vencida não chama Start; depois do handoff, desfecho inconclusivo mantém
  `outcome_unknown`, sem repetir o efeito.
- [x] Validar prazo real recebido pela ferramenta no teste nativo integrado,
  sem esperar minutos nem criar relógio ou exceção de produção para teste.
- [x] Consolidar regressões dos domínios e do App, análise estática e docs.
- [ ] Aceite físico acumulado, separado das evidências automatizadas.

Hotkey/comando direto não possui payload de evento externo. Templates que
dependem de dados ausentes e resolvem para vazio continuam sendo recusados;
não se inventa evento, não se oferece wildcard e não se altera a configuração.
O runtime de jobs mantém seus próprios limites de ferramenta e política de
retry; duração maior não significa desligar cancelamento ou iniciar trabalho
desvinculado do comando.

### Evidências da seção110

- Recorte amplo do App: `go test ./internal/app -run '^(TestCommand|TestAppCommand|TestAppGlobalCommandOwnership)' -count=1 -timeout=300s`:
  **PASS, 192,400 s**.
- Integrações finais `^(TestCommandGlobal|TestAppCommandJobDynamic)`, três
  repetições: **PASS, 47,119 s**. Incluem ferramenta com orçamento próprio
  maior que cinco minutos, encerramento durante o efeito sem repetição,
  grant ausente, revogação e regrant que não revalida handler antigo.
- Suítes completas `internal/commandexecution` e `internal/jobs`: **PASS**
  (10,324 s / 6,834 s). Domínios `internal/jobprofilegrant` e
  `internal/profileaccess`: **PASS**. Testes de alvo dinâmico exercitam
  secrets, recusa permanente antes da primeira tentativa e antes do retry,
  controle positivo, owner e ausência de vazamento no erro de preparação.
- `go vet` de App, jobs, commandexecution, profileaccess e jobprofilegrant:
  **PASS**. Frontend e bindings gerados não foram alterados neste lote.

Sem Wails/build/generate, testes ACP, executáveis personalizados ou banco
pessoal. Três agentes Luna contribuíram em prazos e testes; integração,
preparação do alvo, revisão e evidências pelo principal. Não houve commit,
push ou PR. Não representa nova contagem global nem aceite físico do AEP.

## 111. Configuração por escopo e regras — 21/09/2026

Fechamento do lote de configuração operacional. Não representa uma
nova contagem dos 84 critérios e não certifica o AEP inteiro.

- [x] Editor global/workspace, CRUD de camadas, prioridades e restauração
  limitada ao escopo selecionado, sem apagar personalizações herdadas.
- [x] Edição de bindings e regras tipadas, overrides reversíveis e revisão
  de defaults, sempre pelo serviço confirmado de mutação.
- [x] Snapshot de edição vinculado ao commit; mudança de usuário, workspace
  ou configuração recusa uma edição antiga em vez de sobrescrever outra.
- [x] Regras síncronas no resolvedor em memória, com união aditiva; claims
  manuais e de eventos continuam exigindo prova autenticada do host.
- [x] Publicação preserva hotkeys globais de voz/jobs e notifica o mapa local.
- [x] Diagnósticos explícitos para combinações ainda não publicáveis pelos
  adapters; salvar configuração não é evidência suficiente de execução.
- [x] Regressão integrada backend/frontend, tipos, lint e análise estática.
- [x] Geração oficial de bindings, concluída na seção112.
- [ ] Aceite manual acumulado no aplicativo/NVDA.

Não ampliar este gate para imagens/estados do Stream Deck, tools de chat,
CLI ou portabilidade avançada: são frentes separadas. Limitações encontradas
na composição devem permanecer explícitas, sem transformar testes de domínio
em certificado de funcionalidade da interface.

### Evidências da seção111

- Recorte amplo do App (`TestCommand`, `TestAppCommand`, ownership global,
  mapas locais/completos e desktop): **PASS, 266,508 s**.
- Suítes completas de commandbindings, commandactivation e commandconfig:
  **PASS**. Golden de fingerprints dos padrões preservado sem alterar o golden.
- Frontend completo: **430 arquivos / 5.151 testes, PASS**. Teste adicional
  posterior de argumentos inválidos: recorte de dois arquivos, **11 testes PASS**.
  Recorte final de configuração: **59 testes PASS**; após ajuste do indicador
  de ativação contextual, as duas suítes da tela passaram (**42 testes**).
- TypeScript, ESLint do recorte, Stylelint dos estilos alterados e `go vet`
  de App/bindings/activation/config: **PASS**.
- Testes cobrem restauração restrita ao workspace, recusa de edição obsoleta
  inclusive alteração externa durante confirmação, publicação contextual,
  ativação manual autenticada, herança read-only e argumentos tipados.

Limites: novos ciclos de regras pela UI são persistentes; eventos usam o
produtor interno de jobs autorizado, com habilitação/regrant explícito.
O mapa local contextual cobre os handlers locais e fatos suportados pelo
adapter; outras combinações recebem diagnóstico, não promessa de execução.
Apresentação existente é preservada, mas não há editor de imagens/estados
do Deck neste lote. Bindings Wails não foram editados manualmente nem gerados;
a geração oficial e o aceite integrado continuam pendentes. Sem testes ACP,
Wails/build, banco pessoal, commit, push ou PR.

## 112. Integração com os bindings oficiais — 21/09/2026

- [x] Executar `wails generate module` e disponibilizar as quatro operações
  escopadas na API gerada, sem edição manual de `frontend/wailsjs`.
- [x] Conferir leitura, mutação, preparação e ativação através dos wrappers
  reais gerados; teste substitui somente `window.go`, não o módulo App.js.
- [x] Preservar o campo oficial `activeKnown` sem conversão especulativa
  de `active_known`, inexistente no contrato Go.
- [x] Ajustar o envio compartilhado de mensagens para construir `llm.ChatParams`
  oficial, mantendo correlação e fluxo único de envio/retry. A geração revelou
  a necessidade de `convertValues` após inclusão da estrutura de comando.
- [x] TypeScript e ESLint do recorte PASS; frontend completo **431 arquivos /
  5.154 testes PASS**. Recorte de configuração **54 testes PASS**.
- [x] Build do frontend (`npm run build`, TypeScript + Vite) PASS, sem build
  desktop. Regressão final de envio/retry com serialização: **64 testes PASS**.
- [x] Contratos backend de configuração PASS (**6,257 s**): escopo,
  configuração obsoleta, regra contextual e ativação manual.
- [ ] Aceite integrado no aplicativo e NVDA pelo usuário, seguindo o guia
  `docs/content/recursos/COMANDOS.md`; testes automatizados não substituem esse gate.

O gerador oficial compila/executa seu auxiliar temporário `wailsbindings.exe`;
não se iniciou a interface desktop, não se executaram testes ACP nem se
abriu o banco pessoal. Não houve commit, push ou PR. O AEP permanece
**In Progress**; este lote não altera o catálogo nem fecha gates físicos.

## 113. Execução contextual de teclado e sequências — 21/09/2026

- [x] Publicar bindings v1/v2 condicionados por tipo de superfície para
  handlers backend/auditados elegíveis em chat, editor, terminal e tasklist.
  Outras páginas permanecem limitadas às ações locais já suportadas.
- [x] Capturar lease/foco na UI e transportar apenas a observação de ID/tipo
  da superfície. O host confere a aba canônica, captura seu snapshot e
  revalida antes da execução/handoff, sem aceitar identidade ou alvo inventado.
- [x] Manter origem entre os passos da sequência, sem redirecionar após troca
  de aba/foco, modal ou mudança de mapa. Vários finais podem compartilhar
  prefixo; suppress de uma sequência não bloqueia finais independentes.
- [x] Preservar resolução sem fallback após falha, ownership global, IME,
  controle de repetição e deduplicação. Navegação local permanece sem
  roundtrip e sem auditoria por tecla.
- [x] Revalidar lease também entre Begin e commit de mutações do workspace.
  Abrir o menu de criação somente para sequências de criação, não para
  consultas backend que compartilham a infraestrutura.
- [x] Corrigir validação central de `trigger_spec`: somente keyboard.local
  admite v2; envelope/proveniência e demais origens permanecem v1. Decimais,
  strings e versões desconhecidas continuam recusados; gramática específica
  permanece na porta canônica de acionador.
- [x] Regenerar APIs Wails oficialmente; wrappers novos não fazem fallback
  para ingresso sem contexto. Corrigir clone de v2 para preservar campos
  ausentes, sem criar `modifiers: []` na raiz do documento.
- [x] Distinguir **Aba de lista de tarefas** de **Listas de tarefas** na
  condição da tela, com rótulos nos três idiomas e teste de persistência.
- [x] Exercitar configurar → ativar → executar → restaurar com serviços reais,
  consulta v1/v2, mutação UI, snapshot alterado, replay e matriz das quatro abas.
- [ ] Validar no aplicativo/NVDA junto aos demais lotes acumulados.
- [x] Implementar gravador de sequências na tela — entregue na seção114.

### Evidências e limites da seção113

- App, recorte amplo de comandos: **PASS, 237,959 s**. Matriz contextual
  final com os quatro tipos de aba: **PASS, 17,600 s**.
- Suítes completas commandcontract, commandexecution, commandconfig,
  commandbindings e commandactivation: **PASS**.
- Frontend completo: **431 arquivos / 5.165 testes PASS**.
- Recorte integrado final após estabilizar o dispatcher: **8 arquivos /
  345 testes PASS**. Build somente do frontend (TypeScript + Vite): **PASS**.
- TypeScript, ESLint dos arquivos alterados e `go vet` de
  App/commandcontract/commandexecution: **PASS**.
- Uma execução anterior do teste de jobs simultâneos falhou com SQLITE_BUSY
  durante o grant; a repetição dirigida e o recorte amplo final passaram.
  A investigação identificou escrita concorrente possível pelo heartbeat do
  job já iniciado. Isso **não certifica correção** dessa intermitência; não
  foram adicionados sleeps/retries para escondê-la.

Revisão paralela Luna integrou dispatcher, testes backend e revisão de
revalidação do commit. Sem ACP, startup desktop, banco pessoal, commit,
push ou PR. Whitespace emitido pelo gerador Wails foi preservado; arquivos
não gerados passam `git diff --check`. Catálogo inalterado e AEP
**In Progress**, sem nova porcentagem ou encerramento automático de gates R.

## 114. Gravador de sequências nas configurações — 21/09/2026

- [x] Reutilizar `ShortcutCapture` com seleção explícita de combinação simples
  ou sequência de duas etapas; habilitar v2 somente para teclado local,
  independentemente do escopo global/workspace da configuração.
- [x] Gravar prefixo com Control/Alt/Meta, exigir soltura da tecla do prefixo
  e aceitar a segunda tecla sem modificadores. Publicar o documento somente
  após os dois passos, usando a gramática e serialização compartilhadas.
- [x] Manter v2 ao reabrir o editor e alterar condições/prioridade. Trocar
  o tipo de gravação sozinho não apaga nem converte o documento existente.
- [x] Bloquear Salvar durante a captura e cancelar por Escape, Tab, blur,
  ocultação, desabilitação ou desmontagem, preservando o valor anterior.
  Repeat, IME e AltGraph não produzem etapas acidentais.
- [x] Oferecer operação por mouse, Enter/Espaço e clique sintetizado por
  tecnologia assistiva; anunciar etapas/resultado e associar instruções ao
  botão. Traduções pt-BR/en/es e componentes Select/Button compartilhados.
- [x] Não impor tempo limite na **gravação**. O timeout operacional de 1,5s
  do executor continua inalterado. Tab/Escape mantêm saída acessível da captura.
- [x] Cobrir captura → documento v2 → dispatcher, gravação pela página nos
  dois escopos e edição sem conversão para v1.
- [ ] Aceite manual/NVDA junto aos lotes acumulados; roteiro atualizado em
  `docs/content/recursos/COMANDOS.md`, com exemplo Ctrl+Shift+Y, L e resultados.

Este lote não altera backend, schema de banco, catálogo ou APIs Wails.
Sem geração de executáveis Go, ACP, inicialização desktop ou banco pessoal;
sem commit/push/PR. Não encerra automaticamente R09 ou o AEP inteiro.

### Evidências da seção114

- Frontend completo: **431 arquivos / 5.181 testes PASS**, 115,06 s.
- Recorte final captura + configuração: **3 arquivos / 67 testes PASS**.
- TypeScript, ESLint dos arquivos alterados, Stylelint do controle e
  `git diff --check` dos arquivos não gerados: **PASS**.
- Build somente frontend (TypeScript + Vite): **PASS**, Vite 52,81 s.
- A primeira regressão identificou uma corrida no novo teste de troca de
  escopo: chamada da leitura não significava conclusão do carregamento.
  O teste passou a aguardar o botão habilitado, sem sleeps ou aumento de
  timeout; a suíte completa foi repetida e passou.
- Testes automáticos de acessibilidade não substituem NVDA ou avaliação de
  contraste/layout real. O aceite manual permanece aberto.

## 115. Ciclo manual de camadas no aplicativo — 21/09/2026

Escopo: parte de R02.4/R09.2, ligando o ciclo de claims à configuração real.
Não é uma reconciliação nova dos 84 critérios nem encerramento integral de R02/R09.

- [x] Ação explícita por regra: fixar, alternar e desativar, sem escolher
  arbitrariamente entre várias regras da mesma camada.
- [x] Voltar à última ativação manual da mesma origem e escopo exato, com
  ordenação determinística; camada desabilitada ainda pode ter sua claim retirada.
- [x] Editor de regras persistentes/de sessão/temporárias; duração validada
  no host entre 1 e 86400 segundos, estado e prazo apresentados separadamente
  de habilitação da regra.
- [x] Scheduler pertencente ao runtime autenticado: expira sem tela aberta,
  cancela no shutdown e republica o mapa. Deadline também no teclado/paleta
  locais, sem banco/ledger/round-trip por tecla de navegação.
- [x] Expiração idempotente, bump separado global/workspace, rollback e
  isolamento das claims de jobs/eventos.
- [x] Edição não elimina nem renova o deadline; ativação já fixada é idempotente.
- [x] Troca de workspace preserva claims efêmeras válidas da sessão atual;
  restart encerra sessão/temporárias e mantém restauração persistente autenticada.
- [x] Projeção exige gerações de autenticação/segurança atuais; guard em
  memória via WatchSecurityEpoch impede republicação de claims antigas por
  rebuild comum. Só restauração autenticada explícita rebindará persistentes.
- [x] Bindings Wails gerados oficialmente, três idiomas e roteiro manual atualizado.
- [ ] Aceite manual/NVDA do ciclo novo; acumulado com os demais lotes.
- [ ] Extensão de condições além de `surface.type`/`app.focused` e composição
  nas demais origens previstas. Continuam pendências do item amplo R02/R09;
  campos diagnosticados como não operacionais não foram promovidos a suportados.

### Evidências e limites

- `go test ./internal/commandactivation ./internal/commandbindings ./internal/commandconfig -count=1`: PASS.
- Recorte App `TestCommandManual|TestApplyCommandLayerAction|TestCommandSettings|TestCommandLifecycle`: PASS no estado final (22,623 s), incluindo a regressão de epochs.
- `go vet` nos quatro pacotes acima: PASS.
- Frontend completo: **432 arquivos / 5.193 testes PASS**, repetido no estado final (109,58 s).
- TypeScript, lint focado e diff-check não gerado: PASS.
- Build somente frontend: PASS (Vite 56,82 s).
- A revisão corrigiu deadlock de captura de epoch dentro do gate, preservação
  de sessão sem deadline e seleção de back após layer_disable; testes não foram removidos.
- A regressão também detectou uma restrição nova excessiva aos ciclos de jobs;
  a restrição foi retirada e o teste existente de criação/edição/regrant passou.
  Duração manual não redefine o contrato independente das regras de eventos.
- A regressão App ampliada terminou com exit -1 sem relatório conclusivo;
  não é contada como aprovação. Uma execução focada anterior passou os testes,
  mas falhou ao limpar `app.test.exe` temporário por arquivo em uso; sem tentativa
  de contornar a proteção ou alterar o executável. O recorte repetido acima retornou exit 0.
- Não executados ACP, startup desktop ou banco pessoal. Sem commit/push/PR.

## 116. Condições operacionais e provas de contexto — 21/09/2026

Continuação de R02.4/R09.2. Não substitui a reconciliação dos 84 critérios.

- [x] `surface.id` junto de `surface.type` no teclado: mapa imutável com
  branches exatos, fallback por tipo e barreiras explícitas de conflito,
  supressão e revisão; combinações simples e sequências usam o mesmo contrato.
- [x] Backend valida a identidade contra o workspace canônico e a conserva
  até o efeito; outra aba do mesmo tipo não satisfaz a condição.
- [x] Configuração por título da aba, com tipo associado automaticamente;
  referência ausente e campos desconhecidos não são convertidos silenciosamente.
- [x] Condições de camadas compõem por união entre regras; cláusulas da mesma
  regra continuam conjunções. Teste via API real demonstra ambas.
- [x] Perfil efetivo do workspace na resolução da paleta backend/auditada e
  hotkey global. Mudança de perfil não depende de rebuild da configuração.
- [x] Versão contextual transitória do resolvedor preservada na preparação e
  comparada antes do efeito. Não é argumento de cliente, campo editável nem
  nova tabela/registro por navegação. Testes cobrem mudança, adição e remoção
  da prova; versão estável permite executar.
- [x] Diagnóstico considera condições do binding e da camada e distingue
  fonte operacional por origem. Ações somente locais da paleta/Deck não são
  promovidas a suporte de perfil sem prova na entrega.
- [x] Stream Deck resolve o perfil a cada pressionamento e revalida a prova
  antes da entrega auditada; descoberta considera dispositivos configurados
  mesmo quando a condição inicia falsa, sem reabrir HID por troca de perfil.
- [x] Atualização visual elimina teclas removidas do frame, sem deixar rótulos
  antigos; o renderer continua enviando somente diferenças.
- [x] Ausência de binding contextual permite sequência independente com o
  mesmo prefixo; supressão, revisão e conflito continuam barreiras explícitas.
- [x] Editor de condições agrupa cláusulas por posição/campo para leitores de
  tela e preserva valores indisponíveis. Traduções nos três idiomas.
- [ ] Aceite manual/NVDA das condições novas, acumulado no guia do usuário.
- [ ] Ainda não encerrados: fatos de programa em primeiro plano/dispositivo,
  perfil no teclado local, condições visuais nas outras origens e comandos
  de ativação manual expostos pelo catálogo nas origens restantes.

### Verificação desta seção

- Frontend completo após os refinamentos finais: 434 arquivos / 5.215 testes
  PASS (93,34 s). Recorte de tela/editor/contrato: 4 arquivos / 70 testes PASS.
- Build frontend, TypeScript e ESLint dos arquivos alterados: PASS.
- Domínios `commandactivation`, `commandconfig`, `commandbindings`,
  `commandexecution` e `commanddeck`: PASS.
- App dirigido: execução real por aba, recusa de aba irmã, projeção por regra,
  diagnósticos por origem e paleta por perfil sem rebuild: PASS.
- Regressão App de teclado, configurações, ativação manual e perfil global:
  PASS (48,573 s). `go vet` dos cinco domínios: PASS.
- Regressão App ampliada de Deck, hotkeys globais, providers, paleta, produto,
  lifecycle e fronteira de ingresso: PASS (52,465 s), incluindo os novos
  cenários físicos simulados de perfil A/B/A e frame sem reconexão.
  `go vet ./internal/app`: PASS.
- A primeira execução real encontrou uma validação antiga na gravação de
  binding que recusava `surface.id`; corrigida na API, sem relaxar teste.
- A regressão ampliada encontrou esperas antigas por `p.workers.Wait()`
  durante o funcionamento do produto: esse grupo também contém serviços
  permanentes. A espera dos testes agora observa as invocações pendentes,
  removidas somente depois de execução e `AcceptResult`, sem goroutine
  órfã nem desligamento artificial do produto. As verificações de ledger,
  supressão, replay, ownership e liberação de ocorrência foram preservadas.
- Bindings Wails gerados oficialmente. Sem ACP, startup desktop, banco pessoal,
  commit, push ou PR.

Comandos de regressão desta seção (no worktree; frontend em `frontend/`):

```text
go test ./internal/commanddeck ./internal/commandexecution ./internal/commandbindings ./internal/commandactivation ./internal/commandconfig -count=1 -timeout=240s
go test ./internal/app -run '^(TestCommandKeyboard|TestContextualKeyboard|TestCommandSettings|TestApplyCommandLayerAction|TestCommandManual|TestCommandGlobalProfile)' -count=1 -timeout=240s
go test ./internal/app -run '^(TestCommandDeck|TestCommandGlobal|TestCommandOrigin|TestCommandPalette|TestCommandProduct|TestCommandLifecycle|TestCommandIngress)' -count=1 -timeout=240s
npm test -- --reporter=dot
npm run build
```

## 117. Perfil no teclado local e apresentação coerente — 21/09/2026

Continuação de R02.4/R09.2/R09.3. Fecha a lacuna de perfil no teclado local
registrada na seção116, sem promover todas as origens/fatos a suporte completo.
Gate desta rodada: configuração real → mapa → seleção local ou execução
auditada → revalidação, com ajuda e editor refletindo o mesmo contrato.

- [x] Projeção imutável por perfil e fallback não listado, composta com tipo
  e identidade da aba. Supressão/revisão/conflito preservados; v1 e v2.
- [x] Perfil observado comparado com a fonte canônica no backend, incluindo
  override da aba; ausência/forja/troca de perfil não chegam ao efeito.
- [x] Topbar usa perfil do frame real e revisão transitória de mudanças:
  sequência e commit antigo recusados em A-B-A. Testes integrados de seis
  cenários passaram, sem novo IPC para navegação.
- [x] Ajuda de atalhos e prefixos reutiliza seletor puro do dispatcher:
  perfil/aba/NoMatch/barreiras refletem o mapa aceito. Testes dirigidos passaram.
- [x] Editor escolhe perfil pelo nome, preserva referência ausente e trata
  falha de carga/resposta antiga sem perder a configuração.
- [x] Regressão integrada e bindings Wails oficiais.
- [ ] Aceite manual/NVDA acumulado em `docs/content/recursos/COMANDOS.md`.

Limites mantidos: perfil no teclado local requer uma das quatro abas canônicas
do workspace; páginas administrativas não herdam surface nem perfil da última
aba. Ainda faltam programa em primeiro plano/dispositivo, condições visuais
nas demais origens, ativação manual pelo catálogo nas origens restantes e
outros gates da reconciliação. Esta seção não declara o AEP concluído.

### Verificação desta seção

- Frontend completo final: 436 arquivos / 5.232 testes PASS (101,24 s).
- A primeira rodada encontrou uma live region local no erro de carga de
  perfis. Corrigida para usar exclusivamente o anunciador global; teste de
  erro/retry e auditoria de live regions passaram, sem exceção na auditoria.
- Perfil na API real: camada, binding, regra, mapa e execução backend;
  override da aba prevalece sobre workspace; perfil forjado/ausente recusado;
  Begin/Commit positivo e reserva invalidada por A-B-A: PASS.
- Diagnóstico de perfil com página administrativa: PASS; não anuncia
  suporte apenas porque a condição foi persistida.
- Domínios `commandbindings`, `commandexecution` e `commandconfig`: PASS;
  `go vet` desses domínios e do App: PASS.
- TypeScript após geração oficial dos bindings Wails e ESLint dos arquivos
  alterados: PASS. Sem ACP, startup desktop, banco pessoal, commit ou push.
- Regressão App de teclado/configurações/ativação manual: PASS (42,508 s).
- Regressão App de Deck, globais, providers, paleta, produto, lifecycle e
  fronteira de ingresso: PASS (39,074 s).
- Build frontend final após regeneração e correções: PASS.

## 118. Condições físicas e ações de camada — 21/09/2026

Continuação de R02/R09/R10, com regressão automatizada concluída neste lote. Não altera a baseline de 84 critérios
nem promove os gates sem suas evidências. Escopo: programa/dispositivo nas
origens físicas, ações de camada pelo catálogo e requalificação de defaults.

- [x] Editor de condições filtra campos pelo suporte da origem; dispositivo
  é selecionado pelo nome/modelo, sem serial exposto. Referência ausente é
  preservada, e respostas de uma sessão/workspace anterior não são reutilizadas.
- [x] Editor específico de ações de camada usa nome, ciclo de vida e duração;
  não pede owner, identidade de sessão ou chave da pilha ao usuário.
- [x] Disponibilidade da paleta valida o binding efetivo e a regra-alvo pelo
  mesmo predicado da execução, nos escopos global e workspace. Remover a regra
  ou o binding torna a ação indisponível; não há argumentos padrão implícitos.
- [x] Interface encaminha ativar/alternar/voltar pelo executor backend comum,
  sem efetuar a mutação no frontend. Cobertura de teclado e paleta integrada.
- [x] Captura física imutável anterior à mudança de foco, com prazo, autoridade
  da ocorrência e dispositivo; ausência da captura nunca provoca recaptura
  tardia. Validador do domínio e guards do App cobertos; aceite físico pendente.
- [x] Ações de camada duráveis no backend, com isolamento da pilha por origem,
  publicação do mapa e tratamento da invalidação causada pela própria ação.
  Teste de teclado executa uma ação da camada recém-ativada e verifica sua
  remoção após voltar. Teste de transporte Deck cobre reabertura, tecla mantida,
  liberação/novo pressionamento, ativação idempotente e dois dispositivos.
- [x] Defaults global/workspace: upgrade, needs_review, rebase e restauração
  por API real, sem preparar personalização diretamente no banco.
- [ ] Aceite manual/NVDA acumulado; condições visuais nas demais origens e
  demais obrigações R02/R09/R10 continuam explícitas, não presumidas completas.

### Evidências da integração

- Frontend completo final: 437 arquivos / 5.249 testes PASS (126,01 s).
- TypeScript e ESLint dos arquivos frontend alterados: PASS.
- Build frontend: PASS. A primeira tentativa foi bloqueada pelo sandbox
  ao iniciar esbuild; repetição autorizada do mesmo comando passou.
- Domínios `commandactivation`, `commandconfig`, `commandbindings`,
  `commandexecution`, `commanddeck` e `commandforeground`: PASS.
- Regressão App de teclado, configurações, ciclo manual e perfil global:
  PASS (121,298 s). Inclui cinco novos testes de defaults por API: upgrade/rebase
  nos dois escopos, restauração preservando o global herdado, decisão negada
  sem persistência e restart com overrides e supressão preservados.
- Origem/diagnósticos e device-only sem captura nativa: PASS (20,015 s).
- Cancelamento concorrente e commit limitado da ação de camada: três testes
  PASS, complementados pela prova integrada do mapa após publicação.
- Regressão App final de camadas, Deck, globais, origem, paleta, produto,
  lifecycle e ingresso: PASS (68,165 s).
- Regressão App final de configurações, ciclo manual e teclado:
  PASS (109,862 s), incluindo requalificação dos defaults por API.
- `go vet` do App e dos seis domínios de comandos: PASS.
- Transporte Deck de camadas, reconexão e captura: três repetições PASS.
- Teclado de camadas, paleta e comandos globais: três repetições PASS,
  preservando a recusa de argumentos forjados no ingresso.
- As primeiras regressões detectaram recusa de argumentos persistidos pelo
  resolvedor e repetição de tecla na reabertura do Deck. Corrigidos: ingresso
  continua aceitando somente `{}`, argumentos vêm do binding; o estado da tecla
  é preservado na troca de mapa e liberado na desconexão física confirmada.
  Nenhuma recusa foi mascarada com retry do comando ou remoção de teste.
- Sem testes ACP, startup desktop, banco pessoal, commit, push ou PR.

## 119. Condições visuais na paleta de apresentação local — 21/09/2026

Lote implementado, continuação do item 1 de R02/R09/R10. A projeção de
configuração não transforma observações visuais em autorização backend.

- [x] Editor permite foco, tipo de tela, aba pelo nome e perfil para comandos
  locais da paleta; mantém restrições das demais classes/origens.
- [x] Projeção pelo resolvedor comum, respeitando supressões, ambiguidades,
  condições de camada e referências específicas sem fallback permissivo.
- [x] Paleta usa a superfície capturada antes da busca e revalida contexto,
  owner, mapa e prazo antes do efeito; picker não vira superfície de origem.
- [x] Diagnósticos por classe/origem coerentes com suporte real.
- [x] Bindings oficiais e regressão integrada backend/frontend.
- [x] Editor preserva a seleção original ao editar prioridade e condições de
  uma supressão da paleta; regressão cobre o payload persistido.
- [ ] Aceite manual/NVDA acumulado. Condições visuais do Stream Deck e de
  comandos duráveis continuam fora deste lote; não declarar item 1 concluído.

Evidências automatizadas: frontend completo com 438 arquivos e 5.269 testes
aprovados, incluindo seis cenários da paleta com superfície real, perfil,
contexto divergente e transição de ida e volta. Backend: projeção canônica
com supressão específica, clone profundo e API real sem ledger passou três
vezes (`go test ./internal/app -run '^TestLocalPaletteConditions' -count=3`).
Regressão ampliada de paleta, teclado, camadas, Deck e diagnósticos aprovada;
`go vet ./internal/app ./internal/commandbindings ./internal/commandconfig`
aprovado. TypeScript, ESLint dos arquivos alterados e build frontend aprovados.
Geração oficial dos bindings Wails realizada. Não foram executados
testes ACP, aplicativo desktop nem testes sobre o banco pessoal.

## 120. Condições visuais no Stream Deck para apresentação local — 21/09/2026

Lote implementado e qualificado automaticamente. Continuação do item 1 de R02/R09/R10, sem ampliar a
autoridade do backend sobre foco ou superfície visual.

- [x] Projetar as decisões do resolvedor comum para cada tecla física,
  considerando foco, tipo de tela, aba e perfil; preservar supressões,
  ambiguidades e condições herdadas da camada.
- [x] Entregar a projeção apenas no ingresso físico autenticado, preservando
  geração, sessão, captura de configuração e bloqueio de tecla mantida.
- [x] A UI escolhe exatamente um comando local usando providers reais e
  revalida o contexto sincronamente antes do efeito, sem ledger ou fallback
  para comandos duráveis.
- [x] Editor por origem e diagnósticos expõem somente o suporte implementado;
  seleção de aba permanece pelo nome e configuração não exige serial digitado.
- [x] Regressão por API real com dois comandos na mesma tecla e condições
  disjuntas, testes de integração UI e regressões backend/frontend.
- [ ] Aceite físico/NVDA acumulado. Condições visuais em ações duráveis
  permanecem pendentes; este lote não conclui o item 1 inteiro.

### Evidências e limites

- Frontend completo final: 440 arquivos / 5.286 testes PASS (79,62 s).
- Integração UI com provider real: 12 casos PASS, incluindo fallback explícito,
  ausência de provider, IME, blur, owner, geração, expiração, ambiguidade,
  payload inválido e getter reentrante que muda owner/foco.
- TypeScript, ESLint, build frontend e `git diff --check`: PASS.
- Regressão App de Deck e diagnósticos de origem/paleta/processo: PASS
  (101,835 s), incluindo a matriz canônica e as configurações mistas.
- Regressão adicional de teclado, paleta e comandos globais: PASS (24,669 s).
- Teste de API real: três repetições PASS; mesma tecla com dois comandos
  locais, perfil herdado da camada, título apresentado, tecla mantida,
  invalidação após desativar camada e ausência de ledger.
- Matriz canônica: três repetições PASS; supressão perfil+tipo+aba exata,
  fallbacks, perfil-only, foco, ambiguidades, revisão pendente e recusa de
  ramos duráveis ou fatos físicos no percurso local.
- Domínios commandbindings, commandconfig, commanddeck e commandexecution:
  PASS. `go vet` do App, bindings, config e Deck: PASS.
- O evento leva as alternativas locais; a UI não envia seus fatos visuais
  para autorizar ações no backend. Títulos mostram alternativas potenciais,
  com estado `conditional`, não uma afirmação de disponibilidade visual.
- Misturar ação durável e apresentação local condicionada na mesma tecla
  permanece indisponível para a ação durável e recebe diagnóstico explícito.
  Supressões Deck sem alvo confiável não são anunciadas como suportadas.
- Nenhum teste ACP, startup desktop, banco pessoal ou executável customizado.
  Não houve alteração de assinatura Wails: o novo campo pertence ao evento.
- Roteiro físico/NVDA acumulado em `docs/content/recursos/COMANDOS.md`.

## 121. Paleta contextual com commit durável no workspace — 21/09/2026

Continuação do item 1; não encerra todas as origens nem recertifica os 84
critérios. O suporte abrange exatamente `workspace.tab.chat.create`,
`workspace.tab.editor.create`, `workspace.tab.tasklist.create`,
`workspace.tab.terminal.create`, `workspace.tab.close`, `workspace.create`,
`workspace.chat.open`, `editor.mode.markdown`, `editor.mode.rich` e
`editor.mode.view`.

- [x] Projeção canônica separada para condições da paleta durável, reutilizando
  a matriz de resolução local, incluindo camadas, supressões e ambiguidades.
- [x] Editor e diagnósticos liberam os quatro campos visuais somente para
  estes comandos e para apresentação local já suportada; Deck não é ampliado.
- [x] Ingresso `BeginContextualPaletteUICommand` vincula geração e observação
  ao snapshot canônico do workspace, sem escrever em Begin ou Take.
- [x] Revalidar sessão, geração e alvo até o commit, inclusive perfil A-B-A;
  conservar escrita única e o registro de execução durável existente.
- [x] Gerar bindings oficiais Wails para a nova API e projeção.
- [x] Qualificação integrada do frontend e revisão cruzada deste lote:
  integração real do provider com 14 cenários e teste da bridge sem fallback;
  revisão estática independente do ingresso, resolução e commit backend.
- [ ] Aceite manual acumulado da paleta contextual e NVDA.
- [ ] Restante do item 1: condições visuais nos outros handlers duráveis e
  no ingresso durável do Stream Deck, com seus próprios protocolos de alvo.

Evidência backend: testes de nova reserva, Take/Commit, escrita única,
contexto forjado, rota antiga sem proof e perfil A-B-A passaram. Os domínios
`commandbindings`, `commandconfig`, `commandexecution`, `commandui` e
`workspace` passaram. A projeção cobre foco, perfil, tipo/aba, barreiras,
ambiguidade, argumentos e API real sem ledger durante a consulta.
Nenhum teste ACP, startup desktop, banco pessoal ou executável customizado.

TypeScript, build frontend, ESLint dos arquivos alterados e `go vet` do App,
bindings, configuração e UI: PASS. Os bindings foram produzidos pelo gerador
oficial, com remoção mecânica de whitespace final; `git diff --check`: PASS.
Frontend completo, após encerradas as alterações dos agentes: **441 arquivos /
5.310 testes PASS** (93,48 s). Recorte final de integração, bridge e clone:
**3 arquivos / 29 testes PASS**.
A regressão ampliada encontrou uma expectativa obsoleta no teste de bootstrap
com banco ocupado: ações de camada sem binding devem constar do catálogo,
mas indisponíveis com motivo. O teste passou a exigir esse contrato, incluindo
a presença das três ações; produção não foi alterada por essa correção.

A mesma regressão revelou um defeito real na elegibilidade do Stream Deck:
os 11 comandos de navegação de abas não constavam da seleção permitida e o
mapa perdia seus rótulos. Incluídos pelo resolvedor fechado existente, sem
abrir uma categoria genérica de comandos. O teste verifica os 11 IDs, o mapa
com título e a entrega nativa simulada. Reprodução: 10/10 falhas antes e
10/10 PASS após a correção, sem alterar timeout. Navegação continua local,
sem ledger por acionamento; isto não habilita condições visuais duráveis no Deck.
Os novos testes de paleta contextual passaram em três repetições (28,273 s).
Regressão ampliada final do App (paleta, teclado, abas/workspace, modos do
editor, configurações, Stream Deck e UI): **PASS (70,834 s)**. Domínio
`commanddeck`, `go vet` final do App/Deck e `git diff --check`: **PASS**.

## 122. Paleta contextual para protocolos de mensagens, terminal e editor — 21/09/2026

Ampliação da seção121 para comandos da superfície do workspace. Não significa
novos atalhos padrão ou novos handlers: conecta as condições visuais da paleta
aos protocolos produtivos já existentes, sem converter efeitos auditados em
apresentação local nem redirecioná-los ao executor genérico de abas.

Contagem verificada no registro e na projeção: **72 comandos**, dos quais
**23 Durable e 49 AuditedUI** — 10 de abas/workspace/modos, 1 de limpeza,
3 de terminal, 10 de mensagens, 45 de formatação e 3 de arquivos. São **62
comandos adicionais** ao recorte da seção121, não 62 critérios novos do AEP.

- [x] Seleção fechada de comandos backend e UI auditada; composição pela
  mesma matriz canônica, preservando supressão, ambiguidade e condições de camada.
- [x] Editor de condições e diagnósticos acompanham a seleção implementada;
  rejeitam suporte não implementado para páginas, ações de camada e Deck.
- [x] Paleta captura a origem anterior ao picker e transporta a preparação
  contextual para mensagens, limpeza, terminal e formatação, revalidando até
  o efeito sem fallback para a reserva incondicional.
- [x] Arquivos: fonte original válida até a preparação nativa; continuação
  vinculada ao mesmo alvo depois do blur, preservando validação canônica,
  sessão/configuração, prazo, confirmação de sobrescrita e escrita única.
- [x] Qualificação integrada final e revisão cruzada.
- [ ] Aceite manual acumulado dos novos grupos, incluindo NVDA e diálogo nativo.
- [ ] Restante: condições visuais nas mutações de páginas de listas/perfis,
  ações de camada e ingresso durável do Stream Deck. Essas páginas possuem
  identidade visual e alvo editado próprios; a aba ativa não pode substituí-los.

Evidência inicial: testes backend das três operações de arquivo com reset do
mapa após preparação, duplicação recusada, resultado no ledger e proteção contra
reset prematuro, perfil A-B-A, cofre bloqueado, prazo expirado e cancelamento:
PASS em três repetições (40,319 s). Testes do coordenador frontend de arquivo:
12 PASS. TypeScript e build frontend PASS. Sem alteração de assinatura Wails
ou DTO exportado neste lote. Sem testes ACP, startup desktop, banco pessoal
ou executáveis customizados.

Testes backend de protocolos contextualizados: PASS (31,012 s), incluindo
efeitos de chat/mensagem, editor auditado, recusa do ingresso antigo sem prova,
perfil A-B-A, confirmação destrutiva e preparação de terminal com commit
obsoleto recusado (sem iniciar processo terminal). Projeção/diagnósticos:
PASS (23,911 s), com matriz de 72 IDs e negativos separados para comandos
registrados sem suporte e ID desconhecido. Revisão independente da continuação
nativa de arquivos não identificou falha concreta. Frontend de configuração:
73 testes PASS.

Regressão ampliada final do App: **PASS (93,881 s)**, incluindo paleta,
teclado, abas, editor, chat, terminal, configurações, Stream Deck e UI.
Domínios `commandbindings`, `commandconfig`, `commandexecution`, `commandui`
e `workspace`: PASS. `go vet` do App/bindings/config/UI e `git diff --check`:
PASS.

Integração frontend final: **42 cenários** com provider e registries de alvos,
incluindo decisão destrutiva, mensagens, terminal, formatação e três operações
de arquivo. Recorte final conjunto com portas e coordenador de arquivo:
**4 arquivos / 67 testes PASS** (15,02 s). Continuação nativa consumível uma
vez; preparação recusa IME iniciado durante a espera; blur/reload permitido
somente na continuação de arquivo, nunca para trocar a origem.

Suíte frontend completa após todas as alterações: **441 arquivos / 5.348
testes PASS** (89,82 s). ESLint dos arquivos deste lote: PASS.

## 123. Paleta contextual nas páginas de listas e perfis

Escopo (21/09/2026): seis IDs — `tasklists.duplicate`, `tasklists.delete`,
`tasklists.clear`, `profiles.duplicate`, `profiles.delete`, `profiles.activate`.
São **78 comandos contextuais na paleta: 29 Durable e 49 AuditedUI**,
não seis critérios novos dos 84. O AEP permanece **In Progress**.

- [x] Reserva contextual de página com foco, tipo de tela e perfil; domínio
  visual explícito, perfil canônico e alvo validado separadamente em Prepare.
- [x] Sem usar a aba de fundo como identidade visual da página. A lista
  aberta no workspace preserva o provider de aba existente.
- [x] Editor, projeção e diagnósticos para os seis IDs; `surface.id` direto,
  herdado ou em supressão não é admitido neste grupo.
- [x] Alvo capturado, confirmação destrutiva, commit único e resultado
  preservados; sem fallback ao ingresso incondicional após contexto obsoleto.
- [x] Testes backend: efeitos reais dos seis comandos, cancelamento, alvo
  estrangeiro, fingerprint, perfil A-B-A, mapa, prazo e sessão. Ativação de
  perfil conclui com auditoria e renovação do mapa.
- [x] Bindings regenerados pelo Wails; TypeScript/build e análise estática.
- [x] Integração final de limpar lista na página real e regressão final.
- [ ] Aceite manual acumulado, incluindo NVDA.
- [ ] Criar/atualizar listas e perfis em formulários modais: requer desenho
  de ingresso compatível com o bloqueio atual da paleta em modal. Não exposto
  artificialmente neste lote. Ações de camada e Deck durável continuam separados.

Evidências: backend focado **PASS (25,144 s)**; projeção/diagnósticos e API
real de configuração **PASS (25,121 s)**; frontend de configuração **90 PASS**.
Integração inicial da Topbar/porta: **40 PASS**; regressão frontend inicial
**443 arquivos / 5.396 testes PASS (95,83 s)**. `go vet` dos pacotes alterados
e `git diff --check` PASS. Sem testes ACP, startup desktop ou banco pessoal.

Qualificação final: regressão ampliada do App **PASS (144,932 s)** e seis
domínios (`commandbindings`, `commandconfig`, `commandexecution`, `commandui`,
`tasklist`, `workspace`) PASS. Build do pacote App sem executável PASS.
Suíte frontend **443 arquivos / 5.410 testes PASS (93,74 s)** após integração
da página real. Verificação posterior do refresh do cache e espera assíncrona
do teste ABA: **53 testes PASS (7,29 s)**; nenhuma asserção removida.
TypeScript/ESLint finais PASS. A limpeza na página real recusa lista vazia,
troca de seleção e drift durante leitura; cancelar a decisão não escreve;
sucesso atualiza o cache e anuncia limpeza, não exclusão.

## 124. Ações de camada na paleta contextual do workspace

Escopo (21/09/2026): `layer.activate`, `layer.toggle`, `layer.back` com
foco, tipo de tela, aba e perfil. **81 IDs contextuais na paleta**:
72 comandos de workspace, seis de páginas e três de camada. Essa contagem
não representa critérios adicionais dos 84 itens históricos.

- [x] Projeção e editor aceitam as quatro condições no grupo fechado de
  camada, preservando argumentos persistidos validados pelo schema.
- [x] Readiness resolve condição no snapshot canônico e valida o alvo manual
  configurado; a projeção não concede autorização para executar.
- [x] Ingresso específico com prova privada vinculada à invocação, mapa,
  sessão e snapshot; não recebe argumentos de alvo, regra ou owner da UI.
- [x] Submissão síncrona após guarda visual final; sem reserva UI artificial,
  fallback ao ingresso antigo ou repetição automática.
- [x] Revalidação da prova imediatamente antes de ClaimOwnership; comparação
  em memória das versões publicadas, sem projection guard com I/O dentro da
  transação. Teste comprova recusa de perfil A-B-A antes do claim.
- [x] Publicação da própria camada pode invalidar o mapa sem apagar sucesso
  já confirmado; retorno não aplica efeito visual em outra rota/owner.
- [x] Bindings oficiais Wails regenerados; documentação de usuário atualizada.
- [x] Regressão ampla final e revisão cruzada.
- [ ] Aceite manual acumulado, incluindo NVDA.
- [ ] Condições visuais de camada em páginas independentes e nas demais
  origens não são habilitadas neste lote. Formulários modais e Stream Deck
  durável continuam como pendências separadas.

Evidências iniciais: projeção/settings **PASS (23,020 s)**; frontend de
configuração **20 PASS**. Backend contextual de camadas após correção do
claim **PASS (40,858 s)**. Frontend focado **219 testes / seis arquivos PASS**,
TypeScript/ESLint PASS. Sem startup desktop, banco pessoal ou testes ACP.

Qualificação final de concorrência e regressão das ações de camada nas origens
existentes: **PASS (57,487 s)**. Comparação de workspace e claim protegidas
juntas pelo lock do snapshot, dentro da validação de versões publicadas.
Controle sem mudança deve ativar e renovar mapa; perfil A-B-A deve recusar.
Suíte frontend completa: **444 arquivos / 5.442 testes PASS (108,33 s)**.
Build frontend, build do pacote App, `go vet` e `git diff --check` PASS.
Domínios activation/bindings/config/execution/UI/workspace PASS. Revisão
cruzada final sem novo bloqueador; aceite manual permanece aberto.

A regressão ampliada do App encontrou uma falha no encerramento de claim de
camada desabilitada (`TestApplyCommandLayerActionBackAcrossLayersAndDisabledLayer`).
Corrigido o predicado compartilhado: `deactivate` permite encerrar a claim;
`pin` e `toggle` continuam proibidos para camada desabilitada. O teste foi
mantido e ampliado com esses dois negativos. A rodada ampliada anterior
(268,257 s) não foi verde; a revalidação após a correção é registrada abaixo.

Revalidação do teste em **cinco repetições PASS (5,986 s)**; regressão ampliada
de camadas/configuração **PASS (74,952 s)**. Análise estática final PASS.

Reexecução da mesma bateria ampliada do App após a correção: **PASS
(152,497 s)**. Teste adicional força perfil A-B-A entre a validação otimista
e a claim protegida: controle positivo e dois negativos **PASS (22,780 s)**.
Não restam falhas automatizadas conhecidas deste lote; não encerra os aceites
manuais nem o escopo restante do AEP.

## 125. Stream Deck contextual para comandos do workspace

Escopo de 21/09/2026: união das apresentações LOCAL_UI já suportadas com
**70 comandos de workspace**, isto é, os 72 da paleta menos
`editor.mermaid.apply` e `editor.mermaid.remove`. O helper da paleta permanece
inalterado. Esta ampliação não inclui ações de camada, mutações de páginas
nem o protocolo especial Mermaid; não altera as contagens da paleta.

- [x] Projeção comum com quatro campos: `app.focused`, `surface.type`,
  `surface.id` e `profile`, nas superfícies canônicas do workspace.
  O editor oferece a aba pelo nome; não exige identificador opaco digitado.
- [x] Preservar `localDeckUIConditions` estritamente LOCAL_UI e oferecer a
  união em `contextualDeckUIConditions`, com elegibilidade fechada por ID e
  classe Durable/AuditedUI, sem resolver separadamente e concatenar vencedores.
- [x] Um único evento físico resolve um único ramo, inclusive na tecla mista
  LOCAL_UI/backend condicionado. Empate, supressão, revisão pendente ou falta
  de contexto recusam sem fallback nem repetição do evento.
- [x] Ramo local executa na UI sem IPC de execução de retorno nem ledger;
  isso não elimina o evento inicial host→UI. Ramo durável usa oferta opaca
  vinculada ao evento físico do host e ao snapshot canônico, consumível uma
  única vez, com TTL de **10 segundos**. Não aceita comando/argumentos
  substituídos pelo cliente e preserva os protocolos próprios do alvo.
- [x] Editor e diagnósticos acompanham a matriz; processo/dispositivo não
  entram nesta união visual. Supressão Deck sem alvo confiável falha fechado;
  comandos backend fora da lista continuam indisponíveis na combinação mista.
- [x] Testes focados cobrem contagem real de 70 IDs registrados, matriz,
  união de classes, conflito, supressão/revisão, argumentos inválidos,
  escopo e negativos de processo/dispositivo; provas LOCAL_UI preservadas.
- [x] AEP, índice e guia de comandos atualizados neste lote.
- [x] Suíte frontend completa e build frontend, incluindo TypeScript;
  build/vet de `internal/app` e verificação de whitespace aprovados.
- [x] Regressão backend ampla após correções: PASS (194,531 s).
- [ ] Aceite manual com Stream Deck físico e NVDA, incluindo tecla mista,
  mudança de perfil/aba e invalidação da oferta; demais gates do AEP mantidos.

Evidências executadas no recorte de projeção/editor:

- `go test ./internal/app -run '^(TestCommandDeckConditions|TestCommandDeckContextualConditions|TestCommandSettingsContextual|TestCommandSettingsOrigin|TestCommandSettingsPaletteLocalUI|TestCommandSettingsDeck)' -count=1 -timeout=240s`:
  **PASS (19,101 s)**.
- `src/lib/commandSettingsConditions.test.ts`: **28 testes PASS**.
- ESLint dos arquivos frontend alterados: **PASS**; `gofmt -d` dos arquivos
  Go alterados sem diferenças.
- Backend do ingresso contextual, resultado informado pela integração
  principal: **PASS (37,321 s)**. Não equivale à suíte completa.

Consolidação final informada pela integração principal:

- Suíte frontend completa: **445 arquivos / 5480 testes PASS (94,27 s)**.
- `npm run build`, incluindo TypeScript: **PASS**; Vite **41,98 s**.
- `go build ./internal/app`: **PASS**.
- `go vet ./internal/app`: **PASS**.
- `git diff --check`: **PASS**.

O build final substitui a pendência transitória de TypeScript da integração.
A primeira regressão backend ampla terminou com **FAIL (312,203 s)**;
a saída retornada foi truncada e não permitiu atribuir a falha. A repetição
com diagnóstico filtrado identificou duas regressões de perfil (**FAIL,
227,175 s**): `TestCommandDeckControllerInputProfileABAAcceptsWithoutRebuild`
e `TestCommandDeckProfileRefreshesFrameWithoutReopeningPhysicalConnection`.
Corrigido o roteamento: condição somente de perfil continua canônica no
backend quando não há ramo LOCAL_UI; a união visual permanece necessária
para teclas mistas e para foco/tipo/aba. As asserções anteriores foram mantidas.
Teste novo para perfil nativo versus tecla mista **PASS (18,961 s)**.
Na revisão cruzada foi corrigida uma retenção de ocorrências Deck quando a
reserva terminava antes de executar (preparação cancelada/recusada). A limpeza
agora acompanha o término da reserva, nos ingressos contextual e anterior.
Regressões de limpeza em mensagem/terminal e sucesso: **PASS (21,426 s)**,
sem ledger nem processo de terminal nos cancelamentos anteriores ao preparo.
Backend contextual com expiração do mapa e blur após Begin: **PASS (108,309 s)**.
Regressão completa do recorte Stream Deck: **PASS (72,903 s)**.
Requalificação ampla do App após essas correções: **PASS (194,531 s)**,
incluindo paleta, teclado, workspace, editor, chat, terminal, configurações,
camadas e Deck. `go vet` e `git diff --check` finais PASS. As rodadas anteriores
que falharam permanecem registradas acima; nenhuma asserção foi removida.
Não houve execução de ACP, startup desktop ou executáveis customizados.
O lote não encerra aceites manuais, BASE-PRONTA ou o restante do AEP, que
permanece **In Progress**.

## 126. Camadas no Stream Deck contextual

Escopo de 22/09/2026: **73 IDs elegíveis**, os 70 comandos de workspace da
seção125 mais `layer.activate`, `layer.toggle` e `layer.back`. Apresentações
LOCAL_UI permanecem na união, sem serem reclassificadas como duráveis.
Mutações de páginas e o protocolo especial Mermaid continuam pendentes.

- [x] Projeção e editor admitem `app.focused`, `surface.type`, `surface.id`
  e `profile` nas superfícies canônicas do workspace. Processo/dispositivo
  não podem ser combinados com esta matriz visual.
- [x] Perfil isolado em ramo backend continua no caminho nativo, salvo a
  necessidade de seleção única em tecla mista com LOCAL_UI.
- [x] Preservadas condições nativas de processo/dispositivo dos três comandos
  de camada, isoladas ou com perfil. Diagnósticos diretos e efetivos recusam
  sua combinação com foco/tipo/aba, inclusive fatos herdados de camadas,
  com `unsupported_origin_condition`. Não amplia os outros 70 comandos.
- [x] Um evento físico gera uma única oferta opaca, de uso único e TTL de
  10 segundos, vinculada ao host/dispositivo e snapshot. O ramo de camada
  usa `ExecuteContextualDeckLayerCommand` diretamente no backend, sem reserva
  UI; não permite à UI substituir comando, argumentos, regra ou escopo.
- [x] Ações de camada exigem classe durável e schema de argumentos
  persistidos válido. Regra/escopo são resolvidos pelo host; as teclas do
  mesmo dispositivo compartilham a mesma pilha de camadas, não pilhas por tecla.
- [x] Prova contextual segue até a claim: predicado em memória dentro de
  `GenerationTx`, com comparação de versões publicadas/snapshot protegida
  até a claim, sem I/O de projeção dentro da transação. Perfil A-B-A não
  valida ocorrência antiga.
- [x] Publicação causada pela própria ação pode invalidar o mapa de origem
  sem converter em falha o sucesso já confirmado; não há retry do evento.
- [x] Testes focados de projeção/configuração e negativos de argumentos;
  documentação técnica, índice e guia de uso atualizados.
- [x] Regressão ampla integrada do lote126: App PASS (249,874 s);
  frontend 447 arquivos/5529 testes PASS (87,59 s), builds e análise estática.
- [ ] Físico/NVDA — **Ativar**: configurar regra e condições de aba/perfil;
  confirmar uma ativação e anúncio, sem efeito em contexto incompatível.
- [ ] Físico/NVDA — **Alternar**: pressionar, soltar e pressionar novamente;
  confirmar alternância uma vez por evento, sem repetição ao manter pressionado.
- [ ] Físico/NVDA — **Voltar**: ativar por uma tecla e voltar por outra do
  mesmo dispositivo; confirmar a pilha compartilhada e o mapa resultante.
- [ ] Tecla mista LOCAL_UI/camada: perfis distintos selecionam um único ramo;
  contexto incompatível ou empate não executa nenhum fallback.
- [ ] Perfil A-B-A: uma oferta capturada antes da transição não deve executar
  após retornar ao perfil A; novo pressionamento deve usar o contexto atual.
- [ ] Reconexão do dispositivo: oferta anterior não deve ser reaproveitada;
  após reconectar, novo pressionamento deve respeitar o mapa vigente.

Checks executados no recorte de projeção/editor:

- `go test ./internal/app -run '^(TestCommandDeckConditions|TestCommandDeckContextual|TestCommandSettingsContextual|TestCommandSettingsOrigin|TestCommandSettingsPaletteLocalUI|TestCommandSettingsDeck)' -count=1 -timeout=240s`:
  primeira rodada **PASS (23,287 s)**; rodada final, incluindo diagnósticos
  herdados dos três comandos de camada, **PASS (16,865 s)**.
- `src/lib/commandSettingsConditions.test.ts`: **25 testes PASS**.
- `npx tsc --noEmit`, ESLint dos dois arquivos frontend e
  `git diff --check` do recorte: **PASS**.

Evidências adicionais informadas pela integração principal:

- Backend focado `TestContextualDeckLayer`: **PASS (59,409 s)**.
- Regressão de claim/perfil A-B-A da paleta: **PASS (23,835 s)**.
- `go vet`: **PASS**; bindings Wails gerados.

Rodada ampla informada pelo main em 22/09/2026:

- Backend amplo: **PASS (358,565 s)** na base anterior à última guarda de
  claim do teclado e à correção de condições nativas; essas alterações têm
  qualificações focadas posteriores, não estão implicitamente cobertas por
  essa rodada ampla.
- Build frontend, incluindo TypeScript: **PASS**; Vite **1 min 3 s**.
- Suíte frontend: **5528 PASS / 1 FAIL**, rodada **não verde**. O teste de
  deadline capturava `Date.now` antes da fixture e a criação acrescentava
  mais de 1 ms até calcular a expiração. O main antecipou o mock do relógio
  para antes da fixture, mantendo a asserção de expiração exata.
- Reexecução frontend: **447 arquivos / 5529 testes PASS (87,59 s)**.

A proteção final também mantém o lock do mapa até a claim, antes do lock
de snapshot, seguindo a ordem usada pelo teclado. O watcher de cancelamento
não é a garantia de exclusão. Testes sem watcher forçam reset e expiração
depois da validação otimista: negativos **PASS (23,378 s)**; claim e ações com
autopublicação **PASS (49,334 s)**. Revisão cruzada não identificou inversão
nos caminhos examinados; os locks são liberados antes de republicar o mapa.

Qualificação final da base integrada: regressão ampla do App **PASS
(249,874 s)**, incluindo as guardas finais de mapa e condições nativas.
Build do pacote App, `go vet`, TypeScript/build frontend, ESLint do recorte
alterado e `git diff --check` PASS. As rodadas anteriores permanecem acima
como histórico; nenhuma asserção foi removida para aprovar.

Correção da regressão de condições nativas de camada: recorte Go de
projeção/configuração **PASS (18,277 s)** e frontend **25 testes PASS**.
Mantidas as asserções de recusa da combinação visual/física.

Uma compilação intermediária encontrou import não usado, corrigido na integração.
Os gates automatizados acima passaram; não se declara aceite manual.
Sem ACP, startup desktop, executáveis
customizados ou commits. AEP **In Progress**: este lote não encerra os 84 itens
históricos, BASE-PRONTA ou os demais gates.

## 127. Páginas de listas e perfis no Stream Deck contextual

Escopo de 22/09/2026: **79 IDs elegíveis**, os 73 da seção126 mais
`tasklists.duplicate`, `tasklists.delete`, `tasklists.clear`,
`profiles.duplicate`, `profiles.delete` e `profiles.activate`.

- [x] Projeção de páginas restrita à classe durável e à origem Stream Deck;
  reutiliza `deckPageCommandSurface`, compartilhado com o ingresso.
- [x] Apenas `app.focused`, `surface.type` e `profile`. Perfis usam somente
  `profiles`; listas usam `tasklists`, com `tasklist` admitida apenas para
  duplicar/limpar. Sem `surface.id`, identificador de aba fictício ou fallback
  de página para uma superfície incompatível.
- [x] União de fatos da identidade com `surface.id`, processo ou dispositivo
  impede publicar células de página. A barreira de ID não apaga ramos
  compatíveis de workspace/LOCAL_UI. Preservados os caminhos nativos e as
  condições de processo/dispositivo das três ações de camada da seção126.
- [x] Editor e diagnósticos diretos/efetivos acompanham os três campos,
  inclusive condições herdadas. Supressão Deck sem alvo confiável continua
  falhando fechado, sem deduzir classe/ID pelo trigger.
- [x] Testes contam 79 comandos reais e cobrem superfícies válidas/inválidas,
  perfil/fallback, barreiras de união, argumentos inválidos, classe e escopo;
  testes anteriores de camadas e LOCAL_UI preservados.
- [x] Ingresso `BeginContextualDeckPageUICommand(offerID, generation,
  surfaceType, profile)` consome oferta física única, sem receber comando,
  alvo, ID de superfície ou serial da UI. O domínio da página não é inferido
  da aba ao fundo; alvo/versão/proprietário continuam no protocolo Prepare.
- [x] Seis operações reais, decisões/cancelamento, identidade física no
  ledger, replay, expiração, perfil ABA, página incompatível, alvo estrangeiro
  ou alterado, desconexão, cleanup e autopublicação: **29 cenários PASS
  (53,166 s)**. Guardas adicionais de fatos/expiração ou substituição do mapa
  antes do commit: **PASS (18,763 s)**.
- [x] Consumidor UI captura o alvo da página antes de aguardar, reutiliza
  Prepare/Take/Commit e não recorre à paleta ou à aba de fundo. Testes com
  `DecisionDialog` real e Take pendente comprovam restauração de foco e
  recusa após troca de controle, rota, seleção ou IME. Ativação de perfil
  conserva sucesso confirmado após invalidar seu próprio mapa.
- [x] Frontend completo: **448 arquivos / 5.578 testes PASS (96,14 s)**;
  TypeScript/Vite e ESLint dos arquivos alterados **PASS**.
- [x] Barreira final da fonte mantém mapa, instância física e geração
  estáveis até a admissão das listas ou claim de perfis, sob versões
  publicadas e snapshot canônico. O callback é apenas de memória; a escrita
  permanece fora do gate. Invalidação posterior não desfaz efeito já admitido.
- [x] Regressão determinística da janela preflight→claim: **12 cenários PASS
  (32,219 s)** em listas e perfis, com reset/substituição/expiração do mapa,
  desconexão e remoção da ocorrência. Nenhum negativo persiste; controles
  válidos mantêm efeito e autopublicação.
- [x] Regressões amplas da base final: App **PASS (273,020 s)**; frontend
  **448 arquivos / 5.578 testes PASS**. Build/vet de `internal/app`,
  TypeScript/Vite, ESLint focado, gofmt e diff-check **PASS**.
- [ ] Aceite físico/NVDA dos seis comandos, troca de página/perfil,
  cancelamento das confirmações, tecla mista e reconexão sem replay.

Checks focados de projeção/configuração: Go **PASS (23,195 s)**, com
reexecução final **PASS (22,164 s)**;
`commandSettingsConditions.test.ts` **25 testes PASS**; TypeScript e ESLint
dos arquivos frontend alterados **PASS**. Teste determinístico da claim
reexecutado na base integrada: **12 cenários PASS (13,025 s)**. Qualificação
automatizada integrada concluída conforme os gates acima; não é aceite manual.
Sem ACP, startup ou executáveis customizados.
Mermaid e demais gates permanecem pendentes; AEP **In Progress**, sem encerrar
os 84 itens históricos, BASE-PRONTA ou aceites manuais.

## 128. Mermaid no Stream Deck contextual

Escopo de 22/09/2026: **81 IDs elegíveis**, os 79 anteriores mais
`editor.mermaid.apply` e `editor.mermaid.remove`. Sem ampliar a paleta.

- [x] Projeção/settings oferecem foco, tipo de superfície, ID da aba e perfil
  para as duas mutações. Células Mermaid somente em `editor`, nunca em chat,
  terminal, listas ou perfis; schema persistido continua validado.
- [x] Condição somente de perfil também publica matriz UI restrita a editor,
  sem fallback nativo backend para esse ramo. Binding sem condições preserva
  o caminho incondicional existente. Ramos locais mistos e demais comandos,
  incluindo condições nativas de camadas, permanecem preservados.
- [x] Diagnósticos diretos/herdados seguem o helper ampliado; processo e
  dispositivo não são admitidos na matriz visual Mermaid.
- [x] Testes verificam contagem real de 81 IDs, perfil isolado/misto,
  superfícies incompatíveis, quatro fatos, argumentos/escopo inválidos e
  preservação da resolução incondicional.
- [x] Ingresso físico pelo percurso existente `BeginContextualDeckUICommand`,
  restrito ao editor canônico, preservando dispositivo/evento originais e
  mapa de teclado fixado até o handoff. Oferta consumida uma única vez antes
  da preparação; a confirmação não reutiliza a oferta de dez segundos.
- [x] Backend: 29 cenários de aplicar/remover, conclusão/cancelamento,
  oferta/handoff forjados e replay, expiração, mapa resetado, desconexão,
  contexto obsoleto e reserva que sobrevive ao prazo da oferta. Ledger sem
  código do diagrama; cancelamento após handoff sem confirmação mantém
  `outcome_unknown`, não inventa ausência de efeito.
- [x] Integração frontend com modal Mermaid real, confirmação e restauração
  do foco; nenhuma execução atrás de modal alheio. Leitura de metadados do
  editor de origem não autoriza efeito enquanto sua raiz estiver inativa.
- [x] Regressões amplas backend/frontend e gates estáticos da base integrada:
  App **PASS (464,744 s)**; frontend completo **450 arquivos / 5.617 testes
  PASS (88,37 s)**; build/vet Go, TypeScript/Vite, ESLint focado, gofmt e
  `git diff --check` aprovados.
- [ ] Aceite físico/NVDA de aplicar/remover, cancelamento, troca de
  aba/perfil, documento obsoleto e reconexão sem repetição do evento.

Checks focados: Go projeção/configuração **PASS (22,069 s)**, reexecução final
**PASS (15,970 s)**;
`commandSettingsConditions.test.ts` **25 testes PASS**; ESLint dos arquivos
frontend alterados **PASS**. TypeScript encontrou erros transitórios em
`Topbar.tsx` durante a integração concorrente; reexecução de `tsc --noEmit`
após a atualização desse arquivo pelo responsável: **PASS**. Diff check
do recorte **PASS**. Sem ACP, startup ou executáveis customizados.
AEP **In Progress**, sem alterar a baseline A/I/P/N, percentuais dos 84 itens,
BASE-PRONTA ou aceites manuais.

Qualificação final integrada: backend Mermaid **29 cenários PASS (53,887 s)**;
regressões anteriores Mermaid **PASS (15,517 s)**. Frontend inclui o hook
real com raiz `#root` inativa pelo modal, confirmação real, foco restaurado,
negação de modal alheio/ramos locais/camadas e invalidação por IME, blur,
perfil, mapa e registro da superfície. A leitura especial de metadados
mantém a identidade do registro e não enfraquece leituras normais; testes
cobrem raiz escondida, restrições aninhadas e mudanças reentrantes.
A primeira suíte frontend detectou um relógio não determinístico no novo
teste de expiração; corrigido para avançar após criar a oferta, mantendo o
prazo de produção. Reexecução completa aprovada conforme o gate acima.

A integração adicional com TipTap real revelou recusa indevida na restauração
do foco: o tracker de composição passa a `unknown` ao sair do modal para o
editor. A continuação física agora exige prova do alvo já preparado e foco
dentro da instância capturada, após a preparação e antes do efeito. Apenas
essa continuação tolera `unknown`; estado inicial desconhecido, blur de
janela, composição iniciada e foco em outro controle continuam bloqueados.
Não há alteração do tracker global nem dispensa das demais validações.

## 129. Reconciliação integral do acompanhamento — 22/09/2026

Revisão documental do estado após a seção128, por leitura do código, testes e
evidências registradas. Três frentes Luna conferiram C01–C42, C43–C84 e as
48 saídas R; integração e contrapontos pelo agente principal. Não houve
implementação, execução nova de testes, startup, ACP, consulta ao banco
pessoal, commit, push ou PR. O snapshot continua sendo HEAD
`7e88945ec585e4352c4548aad8cd14e4db0134c1` **mais alterações locais**: o SHA
sozinho não reproduz esta entrega. Resultados de teste citados são históricos,
não execuções desta reconciliação.

### O que a reconciliação distingue

- **I:** implementação identificada no texto daquele critério, não aceite
  integral nem garantia de ausência de bugs. **P:** lacuna funcional ou prova
  composta ainda insuficiente; **N:** entrega de produto ausente.
- A matriz C mantém 84 IDs únicos e o texto normativo; os 84 Ixx históricos
  são outro conjunto. Não converter a recontagem C em aceite BASE-PRONTA.
- Os 149 comandos do catálogo v39, 61 apresentações locais, 67 defaults
  locais e 81 IDs do Deck contextual são inventário de produto, não progresso
  dos 84 critérios. Nenhuma dessas contagens se soma à outra.
- Aceites manuais anteriores continuam válidos para os recortes relatados.
  Não há novo aceite de NVDA, unplug/replug, lock/unlock ou multidispositivo.
- A seção76 e os demais lotes conservam suas contagens históricas. A seção1
  e as linhas C/R da matriz são o estado vigente desta reconciliação.

### Δ18 — C22: varredura de catálogo no caminho físico do Deck

**Identificado na seção129 e corrigido na seção130; obrigação já existente em D12/C22, não ampliação de escopo.** O diagnóstico abaixo preserva o estado encontrado na reconciliação.
`commandProductRuntime.resolveDeckPress` chama `contextualDeckUIConditions`
por pressionamento. `deckUIConditions` percorre `registry.List()` para decidir
as superfícies elegíveis; `Registry.List` ordena IDs e clona as definições.
Isso ocorre antes mesmo de recusar um trigger sem condições. O núcleo
`Configuration.Resolve` continua indexado e sem SQLite, mas não basta para
declarar C22 integral no percurso produtivo. C22 passa de I para P.

Chegada objetiva: preparar/publicar a projeção e índices necessários por
geração/configuração, sem percorrer o catálogo no pressionamento; preservar
resolução única dos ramos locais/duráveis, invalidação, TTL e fonte física.
Adicionar prova que aumentar comandos não candidatos não aumenta o trabalho
do pressionamento e medir o percurso representativo em R06.3. Não se
demonstrou lentidão perceptível nem impacto no Ctrl+Tab; esta é uma lacuna de
contrato/custo identificada estaticamente, não um benchmark.

### Resultado da recontagem e justificativa das mudanças

- **C: 64 I / 18 P / 2 N = 84; 76,2% com implementação identificada.**
  Antes: 58/24/2. P→I em C11 (defaults), C18 (restore exposto), C23
  (ativação contextual), C28 (outbox/retenção/startup), C37 (configuração),
  C44 (foreground fixado) e C45 (executor fora de foco). C22 I→P por Δ18.
- **R: 11 A / 10 I / 24 P / 3 N = 48; A+I = 21/48 (43,8%).**
  R02.1–R02.4 e R09.2 passam de P para I; não recebem aceite automaticamente.
  R07.3 não foi promovido integralmente: migração Windows concluída, mas a
  garantia de ownership fora do Windows não está fechada.
- **Gates: 1/12 aceito, R04.** Nenhum novo aceite manual, final C ou gate.
  R06.1 permanece responsável pela recertificação dos 84 I históricos.

C28 foi julgado pelo próprio texto: retenção de runs preservando a outbox e
startup recuperando/reprocessando antes da retenção têm código e testes reais.
Mantê-lo parcial apenas porque R03.4 inclui outras obrigações seria incorreto.
O mesmo cuidado separa C11 (invariante dos defaults) da matriz completa de
migração R07 e C23 (ativação determinística) da prova Windows específica C43.

Os vinte critérios não integrais se agrupam assim, sem contagem duplicada:

- **Tools/CLI e agente:** C02, C09, C35, C71, C73 parciais; C34/C36 ausentes.
- **Garantias globais/plataforma/diálogo:** C04, C06, C77, C78, C79 e C83.
  Windows tem ownership e no-repeat. Fora dele, o registro ainda pode ocorrer
  sem barreira equivalente; qualificar ou recusar explicitamente o suporte.
  Falta composição/prova dos invariantes do diálogo frente a hotkey nativa
  conflitante e de lock/unlock global com rejeição de callbacks antigos.
- **Resolução e cache:** C22 e C62 — varredura por tecla e cache sem caller
  produtivo identificado são lacunas distintas, não novas funcionalidades.
- **Configuração acessível completa:** C38 — edição textual da apresentação
  Deck ainda ausente, além do aceite NVDA pendente.
- **Prova integrada por programa Windows:** C43 — provider e diagnóstico
  existem; falta a composição positiva de camada/captura/execução.
- **Queda abrupta:** C51 — falta prova de crash de processo, não o recovery
  transacional já implementado.
- **Identidade externa no produto:** C65 e C70 — biblioteca/mapeamento
  existem; montagem administrativa e ingresso continuam pendentes.

### Evidência automatizada disponível e seus limites

A última qualificação registrada é a seção128: recorte amplo do App
**PASS 464,744 s**; frontend completo **450 arquivos / 5.617 testes PASS**;
build/vet de `internal/app`, TypeScript/Vite, lint focado e diff check.
Voz/jobs e ownership globais possuem a qualificação específica das
seções108–110; não presumir que toda suíte histórica foi reexecutada no128.
Isso não equivale a `go test ./...` atual, race, lint global, Bugbot, CI/review
remota ou teste físico. Pendências desses gates permanecem explícitas.

### Frentes restantes e ordem prática

1. **Resolver Δ18 e qualificar resolução/cache (C22/C62, R06.3):** corrigir o
   caminho por tecla e verificar a montagem produtiva do cache previsto,
   preservando a navegação local sem ledger. Não recriar o resolvedor.
2. **Fechar composição global/modal e apresentação do Deck (R07/R09/R10):**
   garantir ownership/repeat nas plataformas suportadas ou recusar suporte;
   reservar invariantes do diálogo frente a hotkeys nativas e provar
   lock/unlock; concluir editor textual de imagem/título/estados e diagnóstico
   acessível de capacidades/disputa/reconexão. CRUD e pin/toggle/back já existem.
3. **Completar D9 (R08.2):** atalho efetivo na própria paleta,
   recentes/favoritos, coleta genérica de argumentos e acesso direto à
   configuração. Não substituir o picker compartilhado nem os preparadores
   de domínio que já funcionam.
4. **Entregar tools/CLI e suas fronteiras (R05/R11):** `command_catalog`,
   `command_config` e CLI list/describe/execute/retry com identidade,
   autorização, decisões e correlação do executor comum. Fachada Wails não
   equivale a tool de chat. Voz e jobs globais já migrados não são refeitos.
5. **Completar provas compostas e encerramento (R03.4/R06/R12):**
   crash/replay/anti-loop, identidade externa, requalificação individual I,
   desempenho, suíte global em ambiente aprovado e review/CI; consolidar
   inventário de equivalência e avaliação dos adapters da fase7.
6. **Preservar prioridade adiada da portabilidade avançada (R05.4/R11.3):**
   fluxo comum de import/export já existe; export sensível e composição com
   credenciais/tools não são descartados nem retomados silenciosamente.

Em paralelo, o roteiro manual acumulado pode validar os percursos já prontos.
Ele não exige repetir implantação de código concluído e não pode certificar
recursos ausentes. Não há estimativa de horas derivada da contagem de itens.

## 130. Deck: projeção estática fora do pressionamento — 22/09/2026

Correção de Δ18/C22, sem ampliar o catálogo nem alterar atalhos, classes de
execução ou política de auditoria. Branch/HEAD são os da seção1, com mudanças
locais; esta rodada não cria commit, executável customizado ou startup real.

### Implementação

- `deckTriggerIdentities` prepara `commandDeckCompiledTrigger` por dispositivo
  e tecla antes da época física: identidade, configuração imutável, registry,
  gerações, fatos requeridos e a matriz única de condições locais/duráveis.
- `resolveDeckPress` consulta apenas as entradas daquela tecla. Revalida o
  snapshot pronto e recusa configuração, registry ou gerações diferentes,
  inclusive uma projeção antiga apresentada com versões novas. Não chama
  `registry.List()` nem recompõe a matriz contextual.
- Perfil e foreground nativos continuam capturados no acionamento, não
  armazenados como resultado da compilação. Perfil A→B→A sem rebuild mantém
  o contrato anterior. Preview/render continuam fora do caminho de entrada.
- `runDeckEpoch` exige que o mapa de apresentação e o mapa de entrada tenham
  as mesmas versões antes de abrir o dispositivo. A borda de eventos continua
  clonando condições; consumidores não recebem os mapas internos reutilizados.
- Oferta física, fonte, seleção única, modal/foco, TTL, held-key, lock e
  revalidação no take/commit não foram relaxados.

A qualificação ampla encontrou três testes com expectativas anteriores à
evolução do produto. `TestCommandCatalogRefreshesStaleWorkspaceProjection`
e `TestCommandOSObservationRecoversBootstrapBeforeFirstObservation` exigiam
ações de camada disponíveis sem binding, contrariando o contrato de
`commandLayerPaletteReady`; agora exigem recusa explícita com motivo e
preservam a prova positiva nos testes de readiness/execução configurada.
`TestMountCommandProductPreservesLockedExistingHost` consultava a projeção
anterior à troca do gerenciador de credenciais; agora verifica o mesmo host,
segurança ainda fechada, recusa stale da projeção e ausência de resolução.
Não foram removidos testes nem alteradas permissões de produção para fazê-los
passar. Esse último cenário foi repetido cinco vezes após a correção.

Arquivos: `internal/app/app_command_deck.go`,
`internal/app/app_command_deck_profile_test.go`,
`internal/app/app_command_deck_projection_test.go`.
Verificações de regressão ajustadas:
`internal/app/app_command_catalog_projection_test.go`,
`internal/app/app_command_os_bootstrap_test.go`,
`internal/app/app_command_product_remount_test.go`.

### Validação

- Testes existentes `go test ./internal/app -run TestCommandDeck -count=1
  -timeout=180s`: PASS, 49,833 s.
- `go test ./internal/commandbindings ./internal/commanddeck
  ./internal/commandexecution -count=1 -timeout=5m`: PASS nos três pacotes.
- `go build ./internal/app`, `go vet ./internal/app` e `git diff --check`:
  PASS. O primeiro vet exigiu liberar escrita no cache normal do Go; repetido
  com sucesso, sem alterar a proteção do antivírus.
- `TestCommandDeckResolveAllocationsDoNotScaleWithNonCandidates` e
  `BenchmarkCommandDeckResolvePressCatalogCardinality`, 200 ms por amostra:
  PASS. Catálogo base 149 comandos: 2.656 ns/op, 2.280 B/op, 20 allocs/op;
  catálogo com 1.024 não candidatos (1.173 total): 2.564 ns/op,
  2.280 B/op, 20 allocs/op. Resolvedor isolado, não HID/ledger/render;
  diferenças de nanosegundos não demonstram ganho, mas o custo de alocação
  não cresce com o catálogo. Teste não impõe limiar frágil de tempo.
- Provas novas: equivalência de matriz mista, snapshot aposentado,
  configuração/registry trocados apesar de versões atuais, geração física
  aposentada e guarda estrutural contra recomposição/listagem no press.
  As seis provas foram executadas juntas: PASS, 3,658 s.
- Testes das duas correções de catálogo/bootstrap: PASS, 22,338 s;
  remontagem bloqueada: PASS em cinco repetições, 17,266 s.
- A primeira suíte ampla falhou (466,845 s); a repetição diagnóstica
  (464,946 s) isolou os três testes descritos acima. Nova execução completa
  após as correções: **PASS, 435,082 s**, resultado recuperado após a interrupção
  da conversa. Comando: `go test -json ./internal/app -count=1 -timeout=12m`.
- A seleção ampliada `Deck|StreamDeck|...`, com limite de 180 s, terminou
  sem PASS (201,721 s) e não conta como validação. A execução completa usa
  limite de 12 minutos, sem remover casos nem reduzir assertivas.

### Placar e fronteira de chegada

C22 P→I: **65 I / 17 P / 2 N = 84 (77,4%)**. Saídas maiores permanecem
**11 A / 10 I / 24 P / 3 N = 48**; gates **1/12**, sem novo aceite final.
Δ18 deixa de ser lacuna de implementação. C62 não é promovido: matriz
estática não é cache de resultado contextual. R06.3 continua exigindo
percentis integrados de resolução, fila/gate, ledger, handoff e render.
Benchmarks isolados não são latência de hardware nem prometem ganho visível.

Próxima frente: qualificar resolução/cache e fechar as garantias de
composição global/modal/plataforma já enumeradas na seção129; não reabrir a
migração de voz/jobs ou o CRUD de configuração que já estão implementados.

## 131. Cache de seleção no executor produtivo — 22/09/2026

C62 deixa de depender de uma biblioteca sem consumidor. O resultado final
da suíte do lote130 foi recuperado após interrupção da conversa: App completo
PASS em 435,082 s. Não foi necessário repetir nem alterar o teste manual.

### Implementação e limites de autoridade

- Cada `commandProductRuntime` monta um `ResolutionCache` LRU de 256 entradas,
  exclusivo daquele usuário/sessão/workspace. `resolvePersistedTrigger` usa
  `resolveCachedBinding` para a seleção pura de bindings; resultados de
  execução, autorização, preflight e decisões não entram no cache.
- A chave D12 mantém usuário, workspace nullable tipado, trigger normalizado,
  origem, registry e gerações global/workspace/camadas. O contexto composto
  inclui `context_version` nullable, fatos efetivos tipados, versão da origem
  canônica e versão do foreground físico. Teclado local inclui o snapshot do
  workspace da ocorrência, preservando mudanças A→B→A.
- Chave incompleta não é inventada: o percurso resolve sem cache. Workspace
  divergente não é cacheado. A captura do envelope produtivo fornece a união
  global/workspace publicada pelo host, como já fazia antes deste lote.
- Outra configuração imutável descarta entradas positivas e negativas do
  runtime; mudanças de contexto/gerações nunca encontram uma entrada de
  chave anterior. O LRU limita retenção de contextos antigos. Shutdown limpa
  as entradas e impede repopulação, inclusive por chamada tardia.
- `BindingIDs` e `LayerRefs` são copiados em Put/Get. A biblioteca antes
  copiava apenas BindingIDs; teste novo prova que mutar o input ou resultado
  devolvido não modifica a proveniência armazenada.
- Verificação de fonte/ocorrência, snapshot pronto, cofre, foco/modal/IME,
  TTL, reservas UI, contexto nativo, autorização e commit continuam a cada
  invocação. Acerto de cache não contorna nenhum desses gates.
- LOCAL_UI no frontend, repetição de navegação e matriz estática do Deck
  mantêm seus percursos. Não foi adicionado IPC, SQLite ou ledger por tecla
  de navegação; não foi alterado o inventário de comandos/defaults.

Arquivos: `internal/app/app_command_product.go`,
`internal/app/app_command_resolution.go`,
`internal/app/app_command_resolution_cache.go`,
`internal/app/app_command_resolution_cache_test.go`,
`internal/commandcontext/cache.go`, `internal/commandcontext/cache_test.go`.

### Evidências

- Cache montado pelo produto real: duas execuções de paleta populam o LRU;
  delta suppress invalida o positivo, remoção/rebuild invalida o negativo;
  cofre bloqueado continua recusando; shutdown limpa e impede nova inserção.
- Testes de chave variam usuário, workspace, origem, trigger, fatos,
  `context_version`, foreground, origem ABA e cada geração; ausência de
  geração workspace e workspace estrangeiro não criam entrada.
- Acerto positivo/negativo, invalidação por snapshot e cópia dos dois slices
  têm provas próprias, sem hooks de observabilidade adicionados à produção.
- Testes de cache e resolução persistida: PASS (21,622 s); testes finais de
  cache/origem física/foreground: PASS (19,563 s). Bibliotecas
  `commandcontext`, `commandbindings`, `commandexecution`: PASS sem cache de
  resultados de teste (`-count=1`). Build/vet e diff-check: PASS.
- Suíte completa do App desta integração: **PASS, 441,480 s** com
  `go test -json ./internal/app -count=1 -timeout=12m`. Executada sobre a
  integração inicial; o refinamento posterior da chave com versão foreground
  e seus casos adicionais foi validado pelo lote focado final de 19,563 s,
  além de build/vet. O PASS de 435,082 s pertence ao lote130.

### Placar e próximo passo

C62 P→I: **66 I / 16 P / 2 N = 84 (78,6%)**, sem novo aceite final.
Saídas R permanecem **11 A / 10 I / 24 P / 3 N = 48**; gates **1/12**.
R06.3 continua parcial: cache instalado não prova percentis de latência,
ganho de desempenho nem custo total de ledger/handoff/render. A próxima
frente funcional é composição global/modal/plataforma (C04/C06/C77–C79/C83),
com qualificação integrada de desempenho mantida no roteiro de R06.

## 132. Hotkeys globais: plataforma qualificada e sessão — 22/09/2026

### Implementação

- `hotkey.IsSupported()` só anuncia Windows. O App não inicializa o controller
  nativo fora dessa plataforma e não entrega registrador sem barreira de ownership.
- O adapter de outras plataformas recusa com `ErrNativeUnsupported`, inclusive
  para callers diretos do Manager; não publica eventos nem adquire ownership.
  Não existe fallback silencioso para a biblioteca sem no-repeat qualificado.
- A documentação do usuário diferencia escopo global de configuração de
  captura global do SO. Teclado local e paleta não foram desativados.
- `app_command_global_session_test.go` cria um perfil persistido em diretório
  isolado e atravessa lock/unlock, ingresso global, bootstrap produtivo,
  handoff e commit no ledger. Não recoloca configuração em memória depois do
  unlock. Duas ocorrências anteriores não revivem: uma é tentada antes e a
  outra somente depois do rebuild; nova ocorrência completa após revalidação.

### Evidências e limites

- Testes de `internal/hotkey`: PASS, 1,906 s, `-count=1`, incluindo duas
  recusas consecutivas sem geração/ownership residual e suporte da plataforma.
- Testes `TestAppGlobalCommand*`: PASS, 20,785 s, incluindo recusa do
  registrador sem barreira e ACK exato com snapshot copiado.
- Build/vet de `internal/hotkey` e `internal/app`: PASS.
- Regressões do controller de hotkeys: PASS, 9,893 s; frontend de ownership,
  bridge e repetição: **30 testes PASS em 4 arquivos**. A primeira execução
  npm encontrou `spawn EPERM` no esbuild; a repetição autorizada fora da
  restrição de subprocessos passou, sem alterar código ou gerar binário customizado.
- `native_other_test.go` verifica a factory padrão em build não Windows;
  esse teste não foi executado nesta máquina Windows. Não alegamos execução
  ou certificação nativa em Linux/macOS.
- Integração final `Test(AppGlobalCommand|CommandGlobal|CommandHostObserver|
  CommandHostMonitor|CommandOSBootstrap)`: **PASS, 7,159 s**, sem cache.
  Inclui segurança com sessão revogada/cofre bloqueado. O teste novo isolado
  também passou (21,173 s). Suíte completa de hotkey/controller: PASS
  (0,396 s / 5,674 s). Vet final: PASS; diff-check: PASS.
- Uma execução intermediária falhou no teste novo porque a fixture anterior
  só criava binding em memória; foi substituída por perfil persistido e
  recarga produtiva. Não houve relaxamento do guard de produção. A suíte
  completa do App não foi repetida neste lote; o PASS amplo anterior é da seção131.

### Placar

C04/C06/C77 P→I: **69 I / 13 P / 2 N = 84 (82,1%)**. Nenhum novo aceite final.
Saídas R: **11 A / 10 I / 24 P / 3 N = 48**; gates **1/12**.
C78/C79 seguem parciais: falta a reserva nativa temporária de Ctrl+Shift+R
no DecisionDialog, incluindo precedência sobre registro global conflitante.

## 133. Reserva temporária de repetição da decisão — 22/09/2026

### Contrato e integração

- O atalho reservado é somente `Ctrl+Shift+R`. O evento nativo pede ao
  `DecisionDialog` existente que repita seu anúncio; não confirma decisões,
  não invoca comandos configuráveis e não cria outro apresentador.
- A reserva tem prioridade sobre o registro configurável da mesma combinação.
  A retirada do diálogo não pode ressuscitar perfil/job removido enquanto a
  reserva estava ativa. Nunca há duas capturas nativas simultâneas da combinação.
- O App identifica conexão do renderer, revisão e ativação opaca do diálogo.
  Eventos/revisões antigos são recusados; close da conexão antiga não fecha a
  atual. A lease de 30 segundos evita reserva órfã após perda do renderer.
- A UI usa a stack real de modais e o ownership compartilhado do teclado;
  topmost sem decisão bloqueia fallback. A reserva não adiciona IPC, auditoria
  ou trabalho ao caminho de navegação local.

### Validação desta integração

- Frontend completo: **452 arquivos / 5.632 testes PASS**, zero testes
  pulados ou falhos. Inclui integração `Modal + DecisionDialog` reais com
  evento/API controlados: registra o topo, repete sem confirmar e encerra
  a sessão no unmount. `tsc --noEmit` e lint dos arquivos tocados: PASS.
- Hotkey completo: **PASS, 1,953 s**; controllers completos: **PASS, 14,109 s**.
  Cobertura de conflito, remoção/troca do inferior, ACK, falhas nativas,
  lease, revisão, teardown e dois Stop concorrentes. O estado redundante
  `owned` e campos sem leitores foram removidos; asserts antigos preservados.
  Os cenários de reserva temporária também passaram **20 execuções consecutivas**
  (`TestManager.*Temporary`, 3,592 s), sem substituir detector de corrida.
- `internal/commandexecution` completo: **PASS, 11,328 s**. A prontidão para
  repetir uma decisão exige sessão SO conhecida/desbloqueada e host ativo,
  mas não desbloqueia cofre nem concede autorização de comandos.
- App focado final: **PASS, 25,048 s**, cobrindo reserva/ownership/ingresso
  global e restart de manutenção. Job global com decisão/tool real:
  **5/5 PASS, 18,727 s**. Build das bibliotecas e vet: PASS.
- As regressões amplas detectaram e levaram à correção de cleanup frontend
  e da fixture SQLite: o banco isolado agora usa WAL, busy_timeout por conexão
  e pool como produção. Preserva auto_vacuum=NONE para exercitar migração de
  bancos antigos; não houve alteração no store de decisões ou no banco real.
  Execuções intermediárias do App falharam em lock SQLite (662,640 s) e em
  dois asserts de migração de manutenção (430,929 s); os cenários corrigidos
  passaram no lote focado acima. A repetição **completa final do App passou,
  372,447 s**, com `go test -json ./internal/app -count=1 -timeout=15m`,
  já incluindo a fixture corrigida e a versão final do gerenciador nativo.
- `go test -race` não foi executado: CGO desabilitado neste ambiente.
  Sem aceite físico/NVDA novo, execução Linux/macOS, inicialização de Wails,
  binários customizados ou testes dos pacotes ACP.

### Placar

C78/C79 P→I: **71 I / 11 P / 2 N = 84 (84,5%)**. Nenhum novo aceite final.
R07.3 P→I, eliminando a justificativa desatualizada de plataforma não Windows
que a seção132 já fechou e incluindo a precedência nativa desta integração:
**11 A / 11 I / 23 P / 3 N = 48**; gates **1/12**. C83/R06 permanecem
parciais pela qualificação transversal; isto não declara BASE-PRONTA.

Próxima frente implementável sem apoio manual: tools públicas de catálogo e
configuração (C34/C36/R11.1), reutilizando autorização/decisões e serviços
existentes; não acrescentar executores paralelos. Latência integrada e matriz
de equivalência por família continuam na fila R06/R07.

### Aceite manual acumulável com os demais lotes

- [ ] Com uma decisão aberta, Ctrl+Shift+R repete a pergunta sem confirmar.
- [ ] Após Alt+Tab para outro aplicativo, o mesmo atalho repete aquela decisão.
- [ ] Manter a tecla pressionada não produz repetição automática.
- [ ] Com foco em campo editável do Assistente, não há anúncio de repetição.
- [ ] Uma decisão inferior não responde quando existe outro modal acima.
- [ ] Ao fechar a decisão, o atalho anterior só volta se ainda estiver configurado.

## 134. Tools públicas de chat e configuração confirmada — 22/09/2026

### Entrega e limites

- `command_catalog` e `command_config` registradas no bootstrap comum de tools,
  respeitando a seleção do perfil. List/describe apresentam IDs reais, origens
  permitidas e schema dos argumentos. Execute usa `CommandExecutionService`,
  ActorAgent e ID da invocação comum para correlação e replay; não há executor
  paralelo nem conversão do agente em usuário.
- O ingresso GUI captura a sessão antes do turno. A autoridade revalida sessão,
  proprietário, conversa local, tool builtin em execução, turno, estado do host
  e papel do usuário. Contexto sem esse ingresso, sessão substituída, canal,
  CLI, job, dry-run ou invocação encerrada não toma emprestada a sessão desktop.
- As 18 ações D10 incluem 13 mutações persistentes: CRUD, restore e importação
  reutilizam o applier, diff, decisão explícita, CAS, persistência e rebuild
  existentes. A revalidação no commit usa a própria conexão da transação;
  teste com pool de uma conexão protege contra deadlock e leitura externa.
- Portabilidade usa o serviço comum, nos modos manter/substituir/cópia, com
  destino global ou workspace atual autorizado. Exportação não inclui segredos,
  claims ou grants. JSON e campos incompatíveis são rejeitados; erros conhecidos
  retornam códigos sanitizados, sem expor detalhes internos ao modelo.
- Catálogo `product-v40-agent-commands`: permanecem 149 comandos, 61
  apresentações e 67 defaults locais. Chat executa apenas `workspace.list`,
  `layer.activate`, `layer.toggle` e `layer.back`, segundo AllowedSources e os
  gates comuns. Não torna ações visuais globais. Ativar camada exige regra
  manual preparada; habilitar uma camada não equivale a ativá-la.
- Guia do usuário e checklist acumulável em `docs/content/recursos/COMANDOS.md`.
  Nenhum aceite manual foi presumido; não houve inicialização de Wails,
  geração de executáveis customizados ou execução dos pacotes ACP.

### Evidências automatizadas

- `go test ./internal/app -run 'TestCommandAgent' -count=1`: PASS, 21,805 s,
  incluindo CRUD/restore, negação sem escrita, stale/revogação, import/export,
  recibo real, replay, payload divergente e identidade do ingresso.
- Testes de schema e validação em `internal/tools/command`; regressões dos
  serviços comuns de execução, configuração e portabilidade mantidas.
- Repetição final dos pacotes: `internal/tools/command` PASS (0,455 s),
  `internal/tools` PASS (1,527 s), `internal/commandexecution` PASS (11,486 s),
  `internal/commandconfig` PASS (8,659 s) e `internal/commandportability`
  PASS (2,581 s), todos com `-count=1`.
- `go vet ./internal/app ./internal/tools/command ./internal/wailsapi`: PASS.
- Regressão Wails de chat/contexto: PASS, 13,675 s. Nenhuma mudança frontend.
- A primeira regressão completa detectou asserts presos à versão v39 do
  catálogo; foram atualizados para v40, mantendo contagens e demais condições.
  Uma rodada intermediária também compilou fixtures anteriores à exigência do
  ingresso GUI. As fixtures passaram a usar a autoridade real, sem relaxar
  produção. Resultado da repetição completa final registrado abaixo.
- **Regressão completa final do App: PASS, 355,605 s**, com
  `go test -json ./internal/app -count=1 -timeout=15m`, após estabilizar os
  três trabalhos paralelos e as fixtures de ingresso. `git diff --check`
  também passou. A contagem documental foi conferida: 74 I e 10 P.

### Placar e próxima frente

C34/C36 N→I e C35 P→I: **74 I / 10 P / 0 N = 84 (88,1%)**.
R11.1 N→I e R11.3 P→I: **11 A / 13 I / 22 P / 2 N = 48**,
A+I **24/48 (50,0%)**. Gates **1/12**, sem novo aceite final.
Os dez critérios parciais e a CLI pública continuam abertos; zero N nos
critérios C não significa implementação integral do AEP.

Próxima frente independente do usuário: R11.2, ingresso CLI de comandos com
list/describe/execute/retry, request ID e consulta autorizada, preservando
recusa de decisões impossíveis em headless. Qualificação transversal e
aceites manuais permanecem separados da implementação.

## 135. CLI pública de comandos — 22/09/2026

### Entrega

- `asst commands list/describe/execute/retry/status`, JSON com IDs reais,
  disponibilidade e motivo. Execute cria UUIDv7; request ID fornecido pelo
  caller só reapresenta uma invocação existente no retry autenticado.
- `internal/commandcli` delega ao `CommandExecutionService`, sem handler
  alternativo. Replay preserva ID/correlação e compara argumentos no ledger;
  consulta é redigida e não recupera output vivo. Estados terminais recusados
  produzem erro de saída sem perder o ID da solicitação.
- `NewCommandCLI` restaura a sessão local pelo fluxo comum, captura principal,
  revalida usuário/papel/sessão/cofre/SO e bloqueia consulta de outra origem.
  Não monta presenter nem cria confirmação textual alternativa. A publicação
  inicial assíncrona é aguardada por até cinco segundos, sempre fail-closed.
- `NewCommandCLIApp` seleciona o modo do entrypoint antes do startup: não
  reativa jobs, canais, autoconexão MCP, monitor LLM ou servidor HTTP. Startup
  sem ingresso GUI não captura hotkeys nativas nem abre o Stream Deck.
  Erros de RunE também drenam o App e liberam a instância antes de sair.
- JSON Schema de descoberta foi extraído para `commandcatalog.JSONSchema`,
  compartilhado com chat, sem wrapper de compatibilidade. Teste preserva
  isolamento de enums mutáveis.
- A compilação da CLI revelou cinco callers antigos de mutações de perfis.
  Foram migrados para os métodos Context do mesmo controller/coordenador,
  com `AuthenticatedContext`, sem recolocar escritores antigos.

### Limites explícitos

O catálogo continua `product-v40-agent-commands`, com 149 comandos e **zero
AllowedSources CLI**. D14 proíbe levar workspace/editor/foco e decisões ao
terminal. List/describe funcionam; execute desses comandos é recusado. Nenhum
comando sentinela foi acrescentado ao produto para produzir um teste verde.
A prova positiva de execução/replay usa registro isolado de teste e o executor,
autenticação e ledger reais. C02 continua parcial pela qualificação de
convergência por família; a borda CLI não é mais uma lacuna de implementação.

O runtime atual exige o observador de sessão do Windows e exclusão de instância:
fechar o outro processo antes de usar a CLI com o mesmo diretório de dados.
Não houve teste manual, inicialização de Wails, acesso ao banco real, build de
executável customizado ou execução dos pacotes ACP. Documentação do usuário:
`docs/content/downloads/cli.md` e `docs/content/recursos/COMANDOS.md`.

### Evidências

- App CLI focado: PASS, 21,010 s; descoberta, recusas sem diálogo/efeito,
  consultas da paleta proibidas, sessão/papel/OS/cofre, espera inicial e
  bootstrap sem hardware. A fixture de contexto vivo usa observador controlado,
  sem corrida com o monitor nativo.
- `internal/commandcli` com integração real: PASS, 4,479 s; execução, replay
  canônico sem novo efeito, argumentos/comando alterados, ID desconhecido,
  isolamento entre sessões e consulta sem output bruto.
- CLI completa: PASS em rodada anterior, 19,498 s. Uma repetição teve todos
  os testes aprovados (18,719 s), mas o Go retornou erro ao limpar o executável
  temporário `asst.test.exe`, ocupado por outro processo; não é falha de assert.
  Não houve tentativa de contornar o bloqueio ou remover o arquivo à força.
- Controllers: PASS, 15,188 s; executor: PASS, 17,601 s; catálogo: PASS,
  1,696 s; tools de comandos: PASS, 0,201 s.
- `go vet ./cmd/asst ./internal/app ./internal/commandcli ./internal/commandcatalog`:
  PASS. Resultado da regressão completa final do App registrado abaixo.
- **App completo: PASS, 361,107 s**, com
  `go test -json ./internal/app -count=1 -timeout=15m`.
- Repetição final da CLI completa: **PASS, 1,975 s**, incluindo cleanup normal
  do Go; o bloqueio temporário não se repetiu.
- Contagem das linhas C conferida por script: **76 I / 8 P**. O verificador
  do índice apontou inventário global antigo (103/102); foi reconciliado com
  os documentos já presentes (104 documentos/103 números), sem alterar seus
  estados. Não houve commit, push, PR ou aceite manual nesta rodada.
- Verificação final combinada de chat/CLI no App: **PASS, 28,165 s**;
  `internal/commandcli` **PASS, 1,002 s**, catálogo **PASS, 1,589 s** e
  cleanup CLI isolado **PASS, 18,201 s**. Verificador de status dos AEPs:
  **PASS**, 104 documentos/103 números e estados sincronizados.

### Placar e próximos passos

C71/C73 P→I: **76 I / 8 P / 0 N = 84 (90,5%)**.
R11.2 N→I: **11 A / 14 I / 22 P / 1 N = 48**, A+I **25/48 (52,1%)**.
Gates **1/12**, sem marcar novos aceites finais.

Próximo bloco sem apoio manual: qualificação integrada R06/R07/R11.4 por
família/origem permitida e ciclo reativo, consolidando gaps C02/C09/C83.
Apresentação Deck, identidade externa mapeada e demais parciais mantêm seus
gates específicos; não são substituídos por mais pacotes genéricos de base.

## 136. Qualificação integrada e concorrência de confirmações — 22/09/2026

### Correção produtiva

A regressão dos pacotes de comandos revelou `SQLITE_BUSY`/`BUSY_SNAPSHOT`
em `TestConcurrentConfirmedGrantHasSingleWinner`. Repetição antes da correção:
**11 falhas em 30 execuções**. O consumo de receipt começava por SELECT e
tentava promover o snapshot leitor a escritor enquanto outra transação
confirmava uma autorização.

`commanddecision.consumeBatch` agora começa pelo CAS de consumo com todos os
predicados anteriores: decisão, assunto, usuário, sessão, fingerprint, gerações,
expiração, tipo de autenticação, tipo de assunto, ações permitidas, resposta
afirmativa registrada e ausência de consumo prévio. Não há retry de callback,
mutex global novo nem dispensa de autorização. Receipt, auditoria, grant e
regra continuam na mesma transação; falha em qualquer receipt do lote desfaz
as alterações anteriores antes de chamar o efeito.

O teste concorrente de receipts foi fortalecido: não tolera mais SQLITE_BUSY
como resultado alternativo nessa disputa controlada. Novas provas recusam
aceitação incompleta/divergente e verificam rollback quando a última receipt
do lote não corresponde ao fingerprint esperado.

### Qualificação de navegação e medição

- `app_command_job_reactive_integration_test.go`: dois testes integrados novos
  verificam que o descendente do mesmo job termina com erro de anti-loop antes
  das tools e que revogar o grant cross-profile enquanto o diálogo está aberto
  impede criar o run alvo ou executar sua tool. Usam catálogo controlado de
  teste com binding persistido, projeção, consumer, sessão, diálogo, handler e
  runtime reais. Não acrescentam comando sentinela ao catálogo produtivo nem
  comprovam sozinhos toda a matriz C09/R11.4.
- `Topbar.palette.integration.test.tsx`: mesma navegação `navigation.history.open`
  por evento DOM, picker da paleta e evento de ingresso Deck, usando controller
  e executor frontend reais. Confere três navegações, input focado, descarte de
  repeat/IME e ausência de handoff durável. A ponte Wails/hardware é controlada;
  não representa um novo aceite físico.
- `TestCommandDeckAppLatency`: opt-in `COMMAND_APP_LATENCY=1`, SQLite temporário,
  catálogo/configuração/controlador reais, comando `navigation.settings.open`.
  Mede resolução e Input→Emit em janelas independentes; também mede a sequência
  desativar/reativar camada e reconstruir mapa. A projeção aposentada é recusada
  e 110 acionamentos por cenário não criam invocações de navegação.
- Amostra Windows/amd64, Go1.26.2, GOMAXPROCS22, warmup10/n100, com outras
  suítes em execução: reconstrução após mutação p50 **69,9304 ms**, p95
  **103,9234 ms**, p99 **112,3231 ms**. Isso é alteração de configuração, não
  latência por tecla. Input→Emit teve 96/100 amostras zero no cenário estável
  e 87/100 após mutação; resolução teve 100/100 e 99/100 zeros. O harness
  registra essa censura pelo relógio explicitamente: **não comprova p95 <1ms**.
  Não mede HID, transporte Wails, renderização, foco real nem separa internamente
  todos os gates. R06.3 permanece parcial.

### Verificação

- `go test ./internal/commanddecision ./internal/commandautomation -count=30`:
  PASS, **69,466 s / 13,148 s**. Nenhuma falha da disputa foi repetida.
- Regressão final `go test ./internal/command... ./internal/tools/command
  -count=1 -timeout=10m`: PASS em todos os pacotes, incluindo os novos testes
  de predicados e rollback. Execuções físicas opt-in não foram habilitadas.
- Frontend completo `npm test -- --maxWorkers=2`: **452 arquivos / 5632 testes
  aprovados**, 518,78 s. Teste novo de convergência também passou isoladamente;
  a suíte completa já tinha iniciado antes de sua inclusão. Warnings de act,
  refs e AudioContext em jsdom continuam registrados, sem ocultá-los.
- `npx tsc --noEmit`: PASS. `go vet ./internal/commanddecision
  ./internal/commandautomation ./internal/app`: PASS.
- Harness App opt-in: PASS, **25,249 s**, sem inferir aprovação temporal a partir
  de zero ou usar as médias de microbenchmark como percentis de produto.
- Novos testes reativos após revisão: PASS, **21,658 s**. ESLint do arquivo
  frontend alterado e nova execução de TypeScript: PASS.
- Jobs: PASS, **15,707 s**; controllers: PASS, **14,207 s**. Repetição final
  de vet nos pacotes alterados e App, `git diff --check` e verificador do índice
  AEP: PASS (104 documentos/103 números). A tentativa inicial de controllers
  usou caminho inexistente `internal/controllers`; o pacote real `./controllers`
  foi executado em seguida, sem mudança de código para esse erro de comando.
- Revisão independente Luna do CAS: sem defeito concreto encontrado; conferiu
  equivalência dos predicados e repetiu os dois testes concorrentes 30 vezes.
  Isso não substitui Bugbot nem fecha R06.4. As fixtures concorrentes usam
  busy_timeout de 5 s, enquanto o produto usa 100 ms; a correção elimina a
  promoção de snapshot da disputa reproduzida, não promete sucesso diante
  de qualquer contenção longa nem introduz retry automático de efeitos.
- App completo: a primeira rodada terminou em FAIL, **385,624 s**; a saída
  volumosa foi truncada e o diagnóstico nominal da falha não ficou preservado.
  Nova execução completa com coleta JSON por teste: **PASS, 328,489 s**.
  Não houve correção entre essas duas rodadas para explicar a divergência;
  portanto permanece uma intermitência sem causa isolada, não um defeito
  presumidamente resolvido. R06.2 não é promovido por essa repetição verde.

### Próximo fechamento independente de validação manual

Isolar a intermitência do App preservando os diagnósticos por teste; continuar
a matriz de convergência/delegação por família ainda pendente em C02/C09.
Para R06.3, qualificar transporte/render/foco e relógio apropriado antes de
comparar o percurso local com o orçamento experimental de 1 ms. A instrumentação
de backend desta seção não substitui essas janelas.

### Placar

**76 I / 8 P / 0 N = 84**, saídas **11 A / 14 I / 22 P / 1 N**, gates **1/12**.
Esta rodada corrige uma falha concreta e amplia provas, mas não encerra a
matriz completa C02/C09/C83 nem converte validação parcial em critério fechado.
Sem banco pessoal, ACP, Wails, executável customizado ou novo aceite manual.

## 137. Convergência de origens e qualificação rastreável — 22/09/2026

### Escopo e evidência

- `app_command_layer_origin_convergence_test.go`: configuração persistida pelo
  contrato real da tela, com os três comandos produtivos `layer.activate`,
  `layer.toggle` e `layer.back`. Paleta e teclado percorrem APIs reais do App;
  Deck percorre seu controlador com hardware controlado. A claim do chat
  permanece isolada enquanto as outras origens ativam, alternam e voltam.
  Confere efeitos no SQLite e invocações concluídas, não apenas metadados do
  catálogo. Uma prova complementar percorre activate/toggle/toggle/back pelo
  chat, na mesma conversa e com nova tool invocation para cada ação; replay
  conserva resultado e estado, sem nova decisão ou invocação. Owner estrangeiro
  e sessão revogada são recusados sem claim/execução bem-sucedida. Não é aceite
  HID nem prova de replay físico da mesma ocorrência de Deck.
- `app_command_dialog_global_integration_test.go`: callbacks de repetição de
  pergunta e job global passam pelo ciclo real de decisão e executor. Repetir
  não confirma; o job exige sua própria confirmação, termina com proveniência
  `user_hotkey` e uma execução da tool. Callback de repetição já liberado não
  emite novo anúncio. O stub limita-se à borda de captura: não implementa a
  disputa entre registros. Portanto esta prova **não fecha isoladamente** a
  integração nativa diálogo × hotkey conflitante de C83.
- Arbitragem real permanece coberta em
  `internal/hotkey/manager_temporary_test.go`, incluindo prioridade, restauração
  de um único slot e substituição/remoção do registro inferior. Não foi criada
  API pública de produção só para injetar um gerenciador em testes do App.
- Inventário de migração reconciliado com v40: tools e CLI não são mais
  descritas como ausentes. CLI continua sem comando produtivo autorizado;
  não se amplia origem para satisfazer artificialmente uma matriz de testes.

### Diagnóstico preservado

App completo: `go test -json ./internal/app -count=1 -shuffle=on -timeout=15m`
passou em **398,364 s**, seed **1790110328067208500**. Saída integral em
`app-qualification-20260922-run1.log` no worktree (arquivo ignorado pelo Git).
O cenário histórico
`TestCommandJobMultiSourceRejectsDivergentChainsAndKeepsSurvivor` passou em
**20 repetições / 41,001 s**, com saída em
`app-qualification-20260922-multisource.log`.

Não houve correção de produção nesta rodada que explique a falha sem nome
da seção136. Ela **não foi reproduzida**, o que não significa causa resolvida.
Na próxima ocorrência, preservar nome, seed, saída e stack antes de corrigir;
não atribuir por suposição a falha ao cenário multisource ou ao antivírus.

Recorte frontend de contexto, repetição, teclado contextual, ownership global,
Topbar e paleta: **6 arquivos / 377 testes PASS**, 30,69 s. Log integral em
`frontend-qualification-20260922.log`. Hotkey real: `go test ./internal/hotkey
-count=10 -timeout=5m` PASS, **2,984 s**. `go vet ./internal/app
./internal/hotkey`: PASS. O App completo iniciou antes da versão final dos
testes novos; sua aprovação não é apresentada como execução desses testes.

Validação dos novos testes após revisão:

- Convergência de camadas e recusas: **3 repetições PASS, 44,005 s**, log
  `app-qualification-20260922-origins-final.log`.
- Ciclo chat com toggle/back e replay: **3 repetições PASS, 27,527 s**.
- Diálogo/job: **10 repetições PASS, 4,733 s**, log
  `app-qualification-20260922-dialog-final.log`. A primeira versão do teste
  falhou porque aguardava o callback síncrono terminar antes de responder à
  decisão que o bloqueava. A correção foi na ordem do teste: responder e depois
  aguardar; não se aumentou timeout nem se alterou o contrato de produção.
  A prova final também aguarda o ledger terminal, em vez de inferi-lo da entrada
  na tool. Essa falha nova de fixture é distinta da intermitência da seção136.
- Rodada final conjunta dos quatro testes novos: PASS, **12,940 s**.
  Vet App/hotkey, `git diff --check` e verificador do índice AEP: PASS
  (104 documentos principais / 103 números ocupados e status sincronizados).

### Limites e próximos fechamentos

1. C02: completar a equivalência por família/origem permitida; as provas de
   camadas desta rodada não equivalem à matriz dos 149 comandos. Próximos
   casos não reconciliados no inventário: `navigation.about.open` pelo
   teclado/paleta/Deck até `/about`, `editor.format.code_block` pelo default
   Ctrl+Alt+C até a transformação e `chat.message.thread.collapse` pela origem
   UI até o alvo. Primeiro conferir a cobertura distribuída existente;
   acrescentar provas só onde faltar, sem exigir um teste monolítico de 149 IDs.
2. C09: separar recusas de identidade já comprovadas de combinações de
   delegação ainda sem efeito final verificado. Não criar outro núcleo de
   autorização nem manter a pendência apenas com o rótulo “matriz completa”.
3. C83: a prova de callbacks do App e a prova do Manager real são
   complementares, não uma única execução integrada nativa. Essa fronteira
   permanece explícita; a ausência de aceite manual não é o único motivo.
4. R06.3: relógio e transporte/render/foco continuam necessários para qualquer
   afirmação de orçamento ponta a ponta; resultados censurados em zero não
   qualificam a latência.

Placar preservado: **76 I / 8 P / 0 N = 84**; saídas **11 A / 14 I / 22 P /
1 N**, gates **1/12**. Sem novo aceite manual, banco pessoal, pacote de testes
ACP, Wails ou executável customizado. Mudanças desta rodada são de testes e
rastreabilidade; não anunciam nova funcionalidade ao usuário.

## 138. Integração da main e regressão cruzada — 22/09/2026

### Preservação e integração

- Checkpoint local `c9bead64c` preserva código, testes, documentação e bindings
  do AEP antes da integração. Caches `.gocache*`, `work/go-cache`, `work/go-tmp`,
  logs e bancos não foram incluídos. Não houve limpeza desses arquivos.
- `origin/main` atualizado por fetch: `714a47c4e`, 154 commits exclusivos da
  main no início da integração. Merge local, sem rebase nem alteração da main.
- Foram encontrados 14 arquivos conflitantes. A resolução combina comandos,
  capturas e guards do AEP-0103 com retorno de aceitação do envio, cronologia e
  apresentação de tools, cancelamento por execução, limite de concorrência e
  cache de jobs da main; não escolhe um lado integralmente.
- `frontend/wailsjs` regenerado com `wails generate module`, explicitamente
  autorizado pelo usuário. Somente normalização mecânica de espaços finais
  após geração; nenhum tipo foi escrito à mão. O aplicativo não foi aberto.
- Índice AEP reconciliado: 107 documentos / 106 números ocupados. AEP-0103
  permanece In Progress; os AEPs novos da main foram mantidos. O verificador
  interpretava a palavra “rascunho” no complemento do status Done do AEP-0057
  como Draft; a redação desse complemento foi desambiguada, sem trocar status.

### Validação da integração

- Suíte completa de `internal/app`: PASS (376,234 s). Pacotes de comandos,
  tools de comandos, controllers, wailsapi, tools e jobs: PASS. Também passaram
  chat e toolinvocations. `go vet ./...`: PASS. Testes ACP não foram executados.
- Frontend completo: 462/463 arquivos e 5.743/5.744 testes passaram. A única
  falha usou a versão anterior do teste de exclusão durante a resolução; ele
  aguardava o handler legado, que não é executado pelo pipeline do AEP.
  O teste final verifica preparação real, troca de superfície e cancelamento
  antes do commit, sem excluir cobertura nem restaurar o handler antigo.
- Regressão final após a resolução: **41 arquivos / 744 testes PASS**, incluindo
  chat, terminal, modal de workspace, store, serviços de chat e paleta. A suíte
  completa não foi repetida após essa correção; os resultados são distintos.
- TypeScript e ESLint dos dez arquivos de frontend resolvidos: PASS.
  Verificador AEP: PASS (107 documentos / 106 números); `git diff --check`: PASS.
- Logs locais ignorados: `merge-main-app-20260922.log`,
  `merge-main-command-regression-20260922.log`,
  `merge-main-frontend-all-20260922.log` e
  `merge-main-frontend-focused-final-20260922.log`.
- Nenhum aplicativo foi aberto, banco pessoal acessado ou push realizado.
  O merge é integração local, não aprovação integral para publicação.

### Falha preexistente identificada durante a regressão

`TestMessageCommandPinRevisionDetectsABA` falhou no pacote database e em
**4/30** repetições locais; revisão independente repetiu **2/30** falhas.
`message_repository.go`, `message_command_revision.go` e seu teste estão
idênticos ao checkpoint anterior ao merge. A assinatura calcula hash da
mensagem, e fixar/desafixar volta o campo pinned ao valor original; colisão
de updated_at torna a revisão indistinguível. A investigação aponta dependência
da resolução do relógio como causa provável, não falha de integração da main.

Esse achado fica **aberto**, sem sleeps no teste, relaxamento de asserções ou
alegação de suíte database verde. Próxima correção deve tornar a revisão
durável e independente de coincidências de timestamp, qualificando os writers
de mensagem. Não se introduziu migração de banco para esconder essa pendência
durante o merge. Logs: `merge-main-backend-domains-20260922.log` e
`merge-main-pin-repeat-20260922.log` (locais, ignorados pelo Git).

Placar preservado: **76 I / 8 P / 0 N**, saídas **11 A / 14 I / 22 P / 1 N**,
gates **1/12**. Integrar main não é fechar critério ou conceder aceite manual.

## 139. Revisão durável de mensagens contra ABA — 22/09/2026

### Correção

- A falha aberta na seção138 não é tratada com sleeps nem relaxamento de
  asserções. A v30 `chat_message_durable_revisions` adiciona revisão interna
  por mensagem, independente da precisão de `updated_at`.
- Triggers SQLite de INSERT/UPDATE/DELETE mantêm `chat_message_revisions`
  atomicamente. O nonce muda mesmo em SQL direto, UpdateColumn, bulk e troca
  de ID; exclusão limpa o metadado e reinserção não ressuscita a revisão.
  Não é auditoria por tecla, credencial ou histórico acumulativo.
- Snapshot de mensagem combina payload e revisão persistida. Snapshot de
  conversa inclui revisões ordenadas, preservando a consulta de ownership e
  a comparação/escrita dentro do BEGIN IMMEDIATE existente. Metadados
  ausentes recusam preparação/commit, sem fallback para timestamps.
- Migração transacional e idempotente faz backfill sem alterar payloads.
  A v30 aguarda o cutover v19 quando adiado, pois ele recria a tabela de
  mensagens; instalar triggers antes dele os perderia na retomada. O teste
  0.1.9 cobre adiamento, adoção do owner e aplicação após a reconstrução.
  Nenhum campo público ou binding Wails muda. Fixtures do App executam a
  mesma migração registrada em produção, sem mecanismo alternativo de teste.
- O escopo é ABA de linhas de mensagem: não declara resolvida toda possível
  sequência ABA de outras tabelas, nem restauração física com capturas vivas.

### Evidências e limites

- Testes determinísticos de relógio congelado, SQL/bulk, rollback, reinserção,
  metadados ausentes e dois handles de banco em arquivo estão em
  `internal/database/message_revision_storage_test.go`.
- Upgrade das fixtures 0.1.9–0.5.0 e segundo boot real preservam tokens,
  payload e timestamps; pin ABA invalida a revisão após o upgrade.
  `TestMessageRevisionsPublishedUpgradesAndSecondBoot` e o teste original
  `TestMessageCommandPinRevisionDetectsABA` passaram em **30 repetições**.
- Regressão completa `internal/database`: **PASS, 20,562 s**, incluindo
  o caminho adiado v19→v30 e rollback da migração. Log local ignorado:
  `message-revision-database-final-20260922.log`.
- Suíte completa `internal/app`: **PASS, 325,209 s**; recorte final de comandos
  de mensagem, editor e limpeza: **PASS, 30,620 s**, incluindo a prova no App
  de pin ABA com `updated_at` deliberadamente inalterado. Logs locais:
  `message-revision-app-all-20260922.log` e
  `message-revision-app-focused-final-20260922.log`.
- Upgrade, rollback/retry de migração e falha original: **30 repetições PASS,
  18,129 s** (`message-revision-upgrade-repeat-final-20260922.log`).
- Agente Luna implementou a suíte adicional; revisão principal exigiu separar
  pin/texto, verificar commits recusados e eliminar Skip de concorrência.
  Suíte final `Test(MessageRevisionStorage|MessageCommand|ConversationContent)`:
  **30 repetições PASS**. `go vet` de database/App, verificador AEP e
  `git diff --check`: PASS. Sem alteração de frontend nesta rodada.
- Revisão independente por outro agente Luna: nenhum bloqueio produtivo no
  escopo ABA; apontou fixture parcial do controller sem a migração, corrigida
  para usar o mesmo instalador canônico. A migração não é reparador de schema
  adulterado: `IF NOT EXISTS` não valida triggers previamente substituídos.
  O nonce é identidade interna, não segredo; o runtime valida presença e
  comprimento, e o gerador SQLite produz hexadecimal. A reinserção com mesmo
  ID/payload/datas foi qualificada diretamente; não se declara certificado
  todo o fluxo de restore físico a partir desses testes.
- Regressão adicional: **controllers PASS (15,867 s)** e **portability PASS
  (14,625 s)**, log `message-revision-controller-portability-20260922.log`.

Placar preservado: **76 I / 8 P / 0 N**, saídas **11 A / 14 I / 22 P / 1 N**,
gates **1/12**. Correção de segurança de concorrência não equivale a aceite
manual ou conclusão integral de C02/C09/C83. Sem banco pessoal, app aberto,
teste do pacote ACP, executável diagnóstico customizado ou push.

## 140. Convergência por família e fechamento das cadeias pendentes — 22/09/2026

### Escopo finito de C02

A revisão independente restringe a qualificação às famílias e origens
permitidas no catálogo v40, sem exigir 149 vezes todas as origens e sem
ampliar `AllowedSources`. Três ligações permaneciam não reconciliadas no
inventário: navegação Sobre por P/K/D até `/about`; default Ctrl+Alt+C até
transformação do editor; e evento UI de recolher thread até a mensagem real.
Os testes de catálogo ou de handlers diretos, isoladamente, não fecham essas
ligações. A conclusão desta seção depende de provas atravessando-as.

### Backend e fronteira de UI

- `TestCommandLocalFamiliesDeckProjectionAndSingleDispatch`: nove comandos
  de rota e sete de navegação do chat percorrem configuração persistida,
  projeção e adapter Deck até evento local autenticado. Pressionamento mantido
  e repeat não duplicam; nova pressão após keyup funciona; desconexão recusa.
  Os mesmos IDs constam da paleta local, sem invocação/auditoria persistida.
- `TestCommandEditorFormatFamilyContextualDeckUsesCommonHandoff`: todos os
  **45 IDs** de formatação/diagramas/slides atravessam oferta contextual,
  Begin/Take/Complete e ledger com origem `streamdeck.key`, uma invocação e
  recusa de replay de oferta/handoff. O ACK simula a resposta da UI, não prova
  transformação do documento; essa prova cabe aos testes frontend abaixo.
- `TestCommandEditorFormatDefaultsUseCommonHandoffWithoutRepeat`: os **14
  defaults** de formatação, inclusive Ctrl+Alt+C, percorrem a entrada real
  de teclado, sem repetição de mutação, com proveniência `keyboard.local`.
  Complementa a matriz existente de 45 comandos por paleta.
- Recorte Go dessas provas e da matriz de paleta: **PASS, 58,043 s**, log
  `command-family-backend-qualified-20260922.log`. O dispositivo físico não
  foi aberto; a entrada começa na fronteira confiável do adapter.

### Frontend, revisão e resultado

- `Topbar.palette.integration.test.tsx` e `Topbar.test.tsx`: as 11 rotas
  do mapa de navegação, inclusive `/about`, partem da paleta Combobox real,
  teclado configurado e evento Deck para o dispatcher produtivo. O mapa
  literal é verificado separadamente em `commandNavigation.test.ts`.
  Cobrem duplicação, modal, geração e identidade obsoletas, sem ledger.
- `Topbar.editorMode.integration.test.tsx`: Ctrl+Alt+C percorre o binding
  até o editor TipTap real, produzindo `codeBlock` e preservando conteúdo.
  Paleta, solicitação do menu e reserva Deck chegam à mesma transformação.
  Readonly e alvo invalidado durante Take não alteram o documento; Deck
  consome a reserva existente, sem abrir uma segunda invocação.
  `commandEditorFormatting.expanded.test.ts` verifica tipo de nó e consumo
  único do alvo, não apenas que uma função foi chamada.
- App completo: `go test ./internal/app -count=1 -timeout=12m -json`,
  **PASS, 322,682 s**, log `command-family-app-all-20260922.log`.
  São **1.190 testes de topo PASS**, com `TestCommandDeckAppLatency`
  **SKIP** (opt-in); a rodada não qualifica latência física integrada.
  `go vet ./internal/app` e ESLint dos cinco arquivos frontend alterados:
  **PASS**. Sem execução de pacote ACP, Wails ou banco pessoal.

- `ChatNavigation.origins.integration.test.tsx` monta `Topbar`,
  `ChatSessionView` e `MessageNode` reais. Paleta, teclado, evento Deck e
  clique no botão da mensagem chegam à expansão/contração real da thread:
  `aria-expanded`, presença/ausência do filho no DOM e outra mensagem
  preservada. Não substitui o dispatcher por um listener de teste.
  Ausência de chamadas Begin/Take/Complete é comprovada no frontend;
  ausência de ledger é comprovada separadamente no backend.
- Revisão independente: **Boyle (Luna)**, três rodadas read-only, sem
  pendências após corrigir a contagem de 44 para **45** IDs e explicitar
  as fronteiras dos testes. A fronteira Wails é simulada no frontend;
  o Go testa o produtor do evento, não HID físico. Essa composição não é
  anunciada como E2E de transporte/hardware. O teste literal de rotas
  impede uma expectativa circular derivada apenas do mapa de produção.
  A última rodada também revisou as correções de isolamento/modal, o
  forwardRef do mock e a coerência dos quatro documentos, sem novos achados.

- Recortes frontend: navegação **391 testes PASS**; editor e helpers
  **182 testes PASS**, com o recorte C02 repetido três vezes; chat **51
  testes PASS**. Após estabilizar o arquivo novo, chat integrado e mapa
  literal de navegação foram executados novamente: **11 testes / 2 arquivos
  PASS**, log `command-family-chat-final-20260922.log`.
- `npx tsc --noEmit`: **PASS**. ESLint completo: **zero erros**, quatro
  warnings preexistentes de `no-explicit-any` em `ProfilesPage.test.tsx`.
  O mock novo de MenuButton usa forwardRef, sem o aviso de ref do React.
- Verificador AEP: **PASS**, 107 documentos principais / 106 números,
  status sincronizados. Contagem independente das linhas C: **77 I / 7 P**.
  `git diff --check`: **PASS**.

Primeira regressão frontend completa: **461 arquivos PASS / 3 FAIL**, **5.790
testes PASS / 5 FAIL**, 430,41 s, log `command-family-frontend-all-20260922.log`.
Três falhas carregaram a versão intermediária do teste de chat (antes de
aguardar montagem/transições e ajustar o nome acessível da opção), já corrigida
e revalidada. Chat e mapa de navegação: **58 testes / 4 arquivos, PASS em
três execuções consecutivas**, logs `command-family-chat-stability-{1,2,3}.log`.
As outras duas falhas eram de fixture/isolamento: o registro de modal sem
overlay era removido pela reconciliação produtiva do DOM; o store global de
ajuda permanecia aberto entre casos após unmount. O teste agora mantém o
overlay e verifica a stack antes/depois do acionamento; teardown fecha o
store de ajuda e o caso de foco verifica sua pré-condição. Não se removeu
assertion nem se alterou produto para tornar a suíte verde.
Esses dois arquivos passaram **391/391 testes em três execuções consecutivas**
após a correção, com ESLint e diff-check aprovados.
Regressão completa final, concluída em **23/09/2026**: `npm test --
--maxWorkers=2 --reporter=dot`, **464 arquivos / 5.795 testes PASS**, 416,00 s,
log `command-family-frontend-final-20260922.log`. A primeira execução e seu
log foram preservados como diagnóstico, não reclassificados como PASS.
TypeScript e ESLint dos arquivos alterados passaram novamente com as fontes
finais. Alterações reunidas no commit temático desta seção; sem push/PR.

Resultado: **C02 P→I; 77 I / 7 P / 0 N = 84 (91,7%)**. Permanecem
**11 A / 14 I / 22 P / 1 N = 48** e **1/12 gates aceito**. Checkboxes C
continuam sem novo aceite. Sem nova funcionalidade, tecla padrão, origem
ampliada, handler alternativo ou alteração de produto para satisfazer testes.
Próximo passo técnico: delimitar e fechar as combinações produtivas de
delegação ainda pendentes em C09, sem reimplementar seu núcleo de identidade.

## 141. Autoridade de agentes e automações — 23/09/2026

Escopo: C09, preservando D2 e AEP-0101. Não habilitar origens proibidas,
emprestar sessão desktop a headless ou criar um comando artificial para
simular cobertura de produto. Aceites manuais permanecem separados.

- `app_command_agent_admission_test.go`: 18 casos nas portas reais
  `command_catalog.execute` e `command_config.layer_create`. O caller local
  é inicialmente válido; então origem persistida de job/tool/comando,
  conversa de subagente/canal, outro proprietário, turno divergente,
  invocação encerrada ou usuário desativado recusam o ingresso, mesmo com
  stamp desktop presente. Nenhuma decisão, invocação de comando ou camada
  solicitada é criada. SQLite e autenticação reais da fixture isolada;
  não se executa LLM, canal externo ou job físico nessa matriz.
- Regressão das dependências `commandidentity`, `jobprofilegrant`,
  `commandtoolbridge` e `jobs`: **PASS**, log
  `command-c09-dependencies-20260923.log`. Testes normais, sem ACP.
- `app_command_agent_config_delegation_test.go`: cinco invalidações entre
  abertura e confirmação de `layer_create` (caller encerrado, job, canal,
  subagente e usuário desativado) retornam recusa sem camada persistida.
  Mais um caso responde à confirmação e cancela o contexto enquanto uma
  consulta está suspensa: não afirma qual tabela ou fronteira exata foi
  interceptada. `TestCommandAgentConfig*`: **PASS, 26,6 s**.
- `TestCommandAgentLayerActivationRevalidatesCallerAfterDecision`: caller
  encerrado/origem trocada para job durante a decisão não ativa a camada.
  O contrato do executor registra `cancelled_stale` depois da admissão;
  não é a recusa de ingresso `ErrDenied`. As duas primeiras expectativas
  do teste confundiam esses estágios e foram corrigidas sem alterar produto.
  Recorte final de admissão/ativação: **PASS, 23,643 s**, log
  `command-c09-admission-final-20260923.log`.
- Integração App de jobs: profile dinâmico com grant, ausência de grant,
  revogação e regrant (handler antigo recusado, novo autorizado), e identidade
  `job_service` com Manager real: **PASS**, junto da admissão, 23,966 s,
  log `command-c09-admission-jobs-20260923.log`. O último usa definição probe
  isolada para observar identidade; não é um novo comando de produto.
- `go vet` de App e das quatro dependências acima: **PASS**.
- Admissão, ativação durante decisão e configuração revogada/cancelada:
  **PASS em três execuções consecutivas**, 32,676 s, log
  `command-c09-stability-20260923.log`. Não houve alteração de comportamento
  produtivo nesse recorte; foram completadas provas de fronteira.
- App completo antes do wire adicional: **PASS, 352,262 s**, 1.194 testes de
  topo aprovados; `TestCommandDeckAppLatency` opt-in pulado. Log
  `command-c09-app-all-20260923.log`. Isso não é qualificação de latência.

### Ligação final e limites

`app_command_subagent_wire_test.go` fecha a ligação apontada pela revisão:
registry App → tool subagent real → profileaccess → Manager/SQLite reais.
Chat cross-profile aprovado e job com grant exato produzem uma subconversa
e um run com owner, vínculo parental, resultado e término persistidos.
`SendParams.ProfileSlug` recebe o profile alvo autorizado no mesmo Manager;
não existe coluna ProfileSlug no run, portanto não se alega sua persistência.
Sem grant: `authorization_not_granted`, nenhum run/subconversa/Send.

Três cenários **PASS, 22,046 s**. Send e disponibilidade do provider são
controlados; não se executam LLM/SendMessageUseCase. O caso job entra pelo
contexto canônico com grant persistido, não pelo scheduler/executor inteiro.
As provas App de jobs/regrant qualificam separadamente essa borda anterior.
Não há nova origem nem comando artificial no catálogo de produto.

Repetição do wire: **três execuções consecutivas PASS**, 3,385 s, log
`command-c09-wire-stability-20260923.log`. Dependências `profileaccess`,
`tools/subagent` e `subagent`: **PASS**, log
`command-c09-subagent-dependencies-20260923.log`; vet também aprovado.

Revisão independente: **Rawls (Luna)** revisou as matrizes, pediu a ligação
final acima e aprovou seu escopo após inspeção. Corrigida precisão do teste
de cancelamento e exigência de valor nulo na recusa de admissão. Sem achado
pendente no recorte. `newCommandToolHandler` sem consumidor permanece lacuna
separada de R05.2; fronteira externa continua em R05.1.

Resultado: **C09 P→I; 78 I / 6 P / 0 N = 84 (92,9%)**. Saídas preservadas:
**11 A / 14 I / 22 P / 1 N = 48**; **1/12 gates aceito**. Nenhum checkbox
de aceite manual promovido. Regressão App final com o wire aprovada,
registrada abaixo. Próximo bloco: edição textual acessível da apresentação
do Stream Deck (C38), sem exigir hardware para construir a UI.

Regressão final: `go test ./internal/app -count=1 -timeout=12m -json`,
**PASS, 334,970 s**, **1.195 testes de topo aprovados**; somente
`TestCommandDeckAppLatency` opt-in pulado. Log
`command-c09-app-final-20260923.log`. Verificador de AEP e diff-check PASS.
Somente testes e documentação alterados; frontend não foi modificado nesta
rodada. Sem Wails, pacote ACP, banco pessoal, push ou PR. Revisão independente
também verificou a reconciliação documental final, sem achados pendentes.

## 142. Títulos personalizados do Stream Deck — 23/09/2026

Continuação de C38, sem promover o critério inteiro. O editor textual oferece
títulos opcionais para pt-BR, en e es, com labels, validação de 256 codepoints,
recusa de NUL e preservação dos demais metadados. Vazio remove a personalização
daquele idioma; o Deck usa o nome localizado do comando como fallback, sem
emprestar o título de outro idioma. Não altera o nome na paleta.

A projeção transporta os títulos por ID do binding materializado em snapshot
imutável, separado de Candidate e da identidade de execução. Deltas conservam
seu próprio título; restore remove sua apresentação sem mutar o snapshot anterior.
O renderer consulta apenas seleções elegíveis. Em empates equivalentes ou ramos
visuais do mesmo comando, títulos divergentes usam o nome localizado; vários
comandos potenciais continuam separados por barra. Nenhuma leitura de banco
ou imagem foi adicionada ao caminho de pressionamento do teclado.

O teste de persistência descobriu um defeito real: o diff de confirmação omitia
Presentation, tratando uma edição só de título como mudança vazia e recusando-a.
O campo agora aparece no antes/depois em três idiomas, sem expor o identificador
físico do dispositivo. Testes App percorrem criação, atualização, remoção,
confirmação, SQLite isolado e reconstrução da configuração. Entradas inválidas
são recusadas antes da confirmação e não persistem bindings.

Evidências iniciais:

- `app_command_settings_presentation_test.go`: persistência/releitura,
  remoção por idioma, recusa de entradas inválidas e diff localizado.
- `commandbindings/presentation_test.go`: isolamento de aliases, títulos
  por locale exato, empate e restore, preservando resolução de execução.
- `commandconfig/projection_complete_presentation_test.go`: IDs de deltas,
  fallback de binding desabilitado e publicação de mudança só de título.
- Frontend completo: **465 arquivos / 5.798 testes PASS, 105,45 s**, log
  `frontend/command-c38-frontend-all-20260923.log`.
- `commandbindings`, `commandconfig` e `commanddeck`: **PASS**, log
  `command-c38-domains-20260923.log`.

Durante a edição, houve interrupção dos workers e dois arquivos Deck ficaram
incompletos. Foram recuperados do conteúdo versionado e os patches reaplicados;
a validação final precisa cobrir os arquivos recuperados, não só resultados
anteriores à interrupção.

**Contagem preservada: 78 I / 6 P / 0 N = 84 (92,9%).** C38 continua P por
imagem/ícone e variantes de estado, além do aceite NVDA pendente. Saídas R e
gates permanecem **11 A / 14 I / 22 P / 1 N**, **1/12 aceito**. Roteiro manual
acumulado em `docs/content/recursos/COMANDOS.md`. Sem migração de banco, Wails,
testes ACP, executáveis diagnósticos personalizados, banco pessoal, push ou PR.

Complementos da validação:

- Editor/página: **86 testes em 3 arquivos PASS**, incluindo Tab/Shift+Tab,
  digitação, axe, salvar/reabrir após reload e alternância de origem sem perda
  de metadados. TypeScript e ESLint dos arquivos alterados: **PASS**.
- `app_command_deck_presentation_test.go`: configuração confirmada chega ao
  mapa contextual e ao renderer real; edição gera diff de uma tecla, frame
  inalterado não gera update, locale ausente restaura nome do catálogo.
  Matriz de equivalentes/branches inclui bindings desabilitados, inativos e
  de contexto inelegível sem vazamento de títulos.
- Recorte App final: testes **PASS, 18,015 s**; o comando terminou com código
  1 na limpeza do temporário padrão `app.test.exe`, em uso por outro processo.
  Não é falha de asserção e não se atribui a causa ao antivírus sem evidência.
  Log `command-c38-presentation-final-20260923.log`; sem executável customizado.
- `go vet` de App, commandbindings, commandconfig e commanddeck: **PASS**.
  Verificador dos AEPs, diff-check e varredura dos arquivos alterados por NUL:
  **PASS** após recuperação.

Frontend final consolidado, incluindo os testes adicionais de integração:
**465 arquivos / 5.801 testes PASS, 106,49 s**, log
`frontend/command-c38-frontend-final-20260923.log`.

Regressão completa App: **PASS, 491,599 s, 1.198 testes de topo aprovados**,
comando encerrou com código zero. Log `command-c38-app-all-20260923.log`.
Essa execução cobre o código produtivo final recuperado e os testes de settings;
os três testes de topo adicionais do renderer foram acrescentados depois da
compilação e estão no recorte final citado acima. Não se conta esse recorte como
aceite físico de hardware ou qualificação NVDA.

Revisão independente: **Lagrange (Luna)**, diff deste lote. Um apontamento
inicial sobre as APIs simplificadas Save/Delete foi retirado após verificar o
contrato de recusa deliberada de bindings ricos: protege metadados e não é a
porta usada pela tela, que usa MutateCommandSettings. Os wrappers simplificados
não têm consumidores produtivos no frontend. Nenhum P1/P2 ou achado pendente
após reavaliação. Implementação delegada a **Noether e Erdos (Luna)**, com
integração, correção do diff de confirmação e regressões conduzidas pelo main.

## 143. Ícones locais do Stream Deck — 23/09/2026

Continuação de C38/D13. Campo textual Ícone no editor existente, com sete opções
localizadas: configurações, conversa, pasta, reproduzir, parar, voltar e estrela.
Sem ícone remove somente esse campo; títulos e outros metadados são preservados.
Tokens desconhecidos previamente salvos permanecem disponíveis para inspeção e
substituição, sem tornar a configuração inválida nem carregar arquivos/URLs.

O campo persistido `presentation.icon` já existia; agora é projetado mesmo sem
títulos personalizados. O snapshot clona títulos e conserva o token por binding
materializado. Somente IDs selecionados/branches elegíveis são consultados;
ícone ausente ou divergente entre candidatos resulta em apresentação textual.
Deltas e restore usam os mesmos IDs e não alteram a identidade de execução.

Sete formas locais rasterizadas complementam o título acima do texto. Não há
fontes externas, SVG de usuário, rede ou caminhos interpretados. Token desconhecido
usa exatamente a apresentação sem ícone. Geometria pequena prioriza o texto.
Título, anúncio e estado são preservados; editar somente o ícone muda o frame,
e frames sem alteração não são reenviados. O caminho de pressionamento de tecla
não recebe decodificação de imagem nem consultas extras.

Imagem personalizada continua pendente: documentos do protocolo têm limite de
64 KiB, inclusive a confirmação que contém os snapshots antes/depois. Embutir
imagens nesses documentos faria o limite depender da quantidade de teclas e
poderia bloquear edições posteriores. Este lote não aumenta limites nem introduz
upload sem armazenamento real. Próximo trabalho: armazenamento separado e
referência segura de imagens, seguido das variantes de estado e de seu ciclo
real de execução. Não basta acrescentar controles sem consumidores.

**Contagem preservada: 78 I / 6 P / 0 N = 84 (92,9%).** C38 permanece P por
imagem personalizada e variantes de estado, além do aceite NVDA integral.
Saídas R: **11 A / 14 I / 22 P / 1 N = 48**; gates **1/12 aceito**.
Roteiro físico/NVDA acumulado no guia de comandos. Sem migração de banco,
Wails, ACP, executáveis diagnósticos personalizados, banco pessoal, push ou PR.

Evidências do lote143:

- `app_command_deck_icons_test.go`: formas distintas em 40/72/96 pixels,
  fallback em geometria pequena e token desconhecido, preservação de texto,
  anúncio e estado; acordo entre branches e descarte de binding desabilitado.
  Configuração confirmada → rebuild → mapa → renderer → diff/remoção/releitura
  usa SQLite isolado e o renderer produtivo, sem HID físico.
- Recorte App de ícones e títulos: **PASS, 20,764 s**, log
  `command-c38-icons-focused-20260923.log`.
- `commandbindings`, `commandconfig` e `commanddeck`, com `-count=1`:
  **PASS**, log `command-c38-icons-domains-20260923.log`.
- Frontend completo: **465 arquivos / 5.803 testes PASS, 103,51 s**, log
  `frontend/command-c38-icons-frontend-20260923.log`. Editor/página focados:
  **16 testes PASS**; TypeScript e ESLint dos arquivos alterados **PASS**.
- `go vet` de App, commandbindings, commandconfig e commanddeck: **PASS**.
  Verificador AEP, diff-check e integridade sem bytes NUL: **PASS**.

Revisão independente do lote: **Carver (Luna)**, somente leitura do diff, sem
P1/P2 ou pendências. O relatório inicial foi corrigido para não antecipar o
resultado do App completo e para distinguir empate equivalente (mesmo ícone
preservado) de ausência/divergência (fallback textual). A revisão não substitui
os testes físicos/NVDA. Implementação de UI por **Cicero (Luna)** e projeção por
**Hegel (Luna)**; integração, rasterização, testes App e documentação pelo main.

Regressão completa final do App: **PASS, 395,779 s, 1.204 testes de topo**,
com código zero. Apenas `TestCommandDeckAppLatency`, opt-in, foi pulado;
não se alega nova medição de latência. Log `command-c38-icons-app-20260923.log`.
Frontend, domínios, vet, TypeScript, lint focado e documentação validados;
sem achados pendentes na revisão local, sem push ou PR.

## 144. Imagens personalizadas do Stream Deck — 23/09/2026

Continuação de C38/D13. Seleção acessível de arquivo PNG/JPEG, substituição e
remoção no editor existente, com mensagens nos três idiomas. Títulos e ícone
permanecem preservados; a imagem tem precedência visual sobre o ícone e não
substitui texto/anúncio. Salvar aguarda a leitura do arquivo. A leitura não
persiste nada e cancelar a confirmação não deixa assets no banco.

`image_upload` é campo transitório do transporte da UI, extraído antes da
canonicalização. O backend limita a entrada a 1 MiB, 4.096 pixels por eixo e
4 milhões de pixels, verifica o conteúdo PNG/JPEG antes da decodificação e
normaliza para até 128 × 128 em RGBA de 8 bits, preservando proporção e removendo
metadados. PNGs de 16 bits também são convertidos, sem reter profundidade que
possa exceder o limite de armazenamento; normalização idempotente testada.
O documento persistido contém apenas `image_ref`, SHA-256 dos bytes normalizados.
O limite de 64 KiB de documentos/confirmações continua inalterado. O hash é
ocultado na exibição da confirmação, sem alterar a comparação nem o receipt.

Tabela aditiva `command_image_assets`, por usuário e digest, com quota de
16 MiB por usuário e até 80 KiB por PNG normalizado. O hook de configurações
grava o asset na mesma transação do binding/receipt, depois de revalidar a
autoridade. O commit comum remove assets sem referências desse usuário,
considerando todos os workspaces. Troca, remoção, exclusão/restore e falhas
preservam as garantias transacionais. Não há URL, caminho ou owner fornecido
pela UI para carregar imagens. O banco pessoal não foi aberto pelos testes.

Projeção imutável exige acordo entre os bindings elegíveis. Carregamento e
decodificação acontecem no render de frames alterados/reconexão, fora
do key-down; não há cache de bytes entre sessões. Ausência/corrupção do asset
usa ícone/título, sem mudar autorização nem execução. Falha transitória de
leitura agenda retry do frame a cada cinco segundos; ausência/corrupção estáveis
não geram polling de banco. Exportações de configuração
carregam referências, não os arquivos: noutra base/usuário é necessário selecionar
a imagem novamente. A documentação e o editor explicitam esse limite.

**Contagem preservada: 78 I / 6 P / 0 N = 84 (92,9%).** C38 segue parcial por
variantes ligadas ao ciclo real de execução e aceite NVDA integral. Saídas R:
**11 A / 14 I / 22 P / 1 N = 48**; gates **1/12 aceito**. Nenhum checkbox
manual foi promovido. Roteiro físico/NVDA acumulado no guia de comandos.

Evidências verificadas até aqui:

- App focado após revisão: **PASS**, incluindo upload maior que 64 KiB → decisão
  → persistência → rebuild → frame, preservação de título/anúncio e remoção;
  negação, sessão revogada durante decisão e falha da auditoria após escrita
  não deixam asset nem binding. Teste adicional força erro de leitura recuperável
  e verifica recuperação no mesmo mapa. Log `command-c38-images-focused-review-20260923.log`.
- `commandbindings`, `commandconfig`, `commanddeck`, `commandportability`:
  **PASS**, log `command-c38-images-domains-final-20260923.log`.
- `commandimage`: normalização, metadados, formato/dimensões/tamanho inválidos,
  integridade, quota, idempotência e isolamento/limpeza entre owners/workspaces.
  Testes e vet do pacote **PASS**.
- Vet de App e domínios alterados: **PASS**, log
  `command-c38-images-vet-20260923.log`.

Revisão independente: **Avicenna (Luna)**, três rodadas. Um P2 corrigido:
falha transitória no carregamento agora agenda retry do frame, em vez de exigir
reconexão/edição. Segunda rodada sem P1/P2 ou pendências; terceira conferiu a
normalização final de PNGs de 16 bits, sem novos achados. Revisão adicional do
frontend eliminou anúncio duplicado de erro: o `Input` anuncia uma única vez;
seleção/remoção continuam anunciadas pelo editor. **29 testes focados PASS**,
TypeScript e ESLint focado **PASS**. Implementação dividida entre **Sagan**
(codec/store), **Socrates** (projeção) e **Russell** (editor), todos Luna;
integração, renderer, testes App e documentação pelo main.

Qualificação sem ocultar tentativas anteriores:

- Primeira suíte frontend: duas falhas novas do seletor, corrigidas sem excluir
  ou enfraquecer testes. Segunda: imagens passaram, mas dois cenários de handoff
  da paleta falharam; **Topbar isolado passou 97/97**. Causa dessas duas falhas
  não comprovada; concorrência/carga permanece hipótese, não diagnóstico fechado.
- Primeira suíte App atingiu timeout agregado de dez minutos (602,490 s), sem
  asserção falha anterior e com teste ativo havia zero segundos. Reexecução com
  orçamento agregado de quinze minutos, sem alterar deadlines de produto ou
  limites individuais dos testes. Logs das tentativas anteriores preservados.

App completo: **PASS, 487,480 s, 1.208 testes de topo**, log
`command-c38-images-app-final-20260923.log`. Somente o opt-in
`TestCommandDeckAppLatency` pulado. Ajustes posteriores de retry/normalização
foram qualificados novamente no recorte App, nos domínios e na revisão
independente. Não se alega nova medição de latência nem aceite físico.

Frontend completo final com `--maxWorkers=2`: **465 arquivos / 5.816 testes
PASS, 437,74 s**, log `frontend/command-c38-images-frontend-qualified-20260923.log`.
Nenhum teste foi removido; as duas falhas anteriores da paleta não se repetiram,
mas não se afirma causa resolvida. TypeScript, ESLint focado, vet, verificador
AEP, diff-check e integridade sem bytes NUL **PASS**.
Sem Wails, suíte do pacote ACP, executáveis diagnósticos personalizados,
banco pessoal, push ou PR.

## 145. Feedback real de execução no Stream Deck — 23/09/2026

O acompanhamento temporário é específico do executor Stream Deck. Não modifica
o caminho rápido de navegação nem cria um segundo ledger: guarda somente o
estado mais recente de cada acionador em memória, com capacidade limitada.
Espera precede a chamada ao executor; execução começa na entrada do handler;
conclusão depende do resultado terminal retornado pelo pipeline comum.
Falha, recusa, cancelamento, timeout e resultado desconhecido não viram sucesso.

O renderer consulta esse estado fora do key-down e incorpora as mudanças ao diff
de frames. O feedback pertence à instância física, geração e versões do mapa;
conclusões antigas não sobrescrevem novas execuções. Reconexão, bloqueio,
captura e reconstrução invalidam a apresentação anterior. Um resultado final
é transitório; não representa estado persistente de camada ou de ferramenta.

Texto localizado em pt-BR/en/es acompanha a apresentação física. Após uma
escrita bem-sucedida do frame, o anúncio acessível exige sessão revalidada,
mapa local correspondente e instância ainda viva. O consumidor usa o announcer
compartilhado, valida owner/workspace/geração/expiração, elimina duplicatas e
recusa regressão de estado. O evento não contém serial nem autoriza ações.

Limites mantidos explícitos: eventos locais rápidos não recebem confirmação
artificial nem nova auditoria; estados persistentes ligado/desligado e imagens
personalizadas por estado continuam pendentes. O aceite físico/NVDA permanece
manual. **78 I / 6 P / 0 N = 84; C38 continua P.** Saídas R e gates permanecem
**11 A / 14 I / 22 P / 1 N = 48; 1/12 aceito**. Nenhum checkbox foi promovido.

Evidências da seção145:

- Recorte final de feedback: **17 testes de topo PASS, 25,983 s**, log
  `command-c38-feedback-reviewed-20260923.log`. Inclui driver simulado →
  executor real → frame/announcer → retorno ao estado normal; conclusão real
  com sucesso, falha e cancelamento; reconexão, geração, sobreposição,
  expiração, limite, Unicode e pixels do título em 16×16.
- `commanddeck`, `commandexecution` e `commandui`: **PASS**, log
  `command-c38-feedback-domains-20260923.log`.
- Frontend completo: **466 arquivos / 5.824 testes PASS, 440,84 s**, com
  `--maxWorkers=2`; log `frontend/command-c38-feedback-frontend-full-20260923.log`.
  TypeScript e ESLint focado **PASS**. Vet App/Deck/executor/broker **PASS**,
  log `command-c38-feedback-vet-final-20260923.log`.
- Tentativas intermediárias preservadas: fixture de reconexão sem cancelador
  provocou panic; teste de expiração em edição reteve mutex e atingiu timeout.
  Ambos foram corrigidos nos testes, sem remover cenários ou relaxar asserções;
  o recorte final acima passou. Vet iniciado durante edição encontrou método
  ainda não escrito; a execução final também passou.

Implementação paralela: **Zeno** (executor), **Hubble** (renderer) e **Boyle**
(frontend), todos Luna. Main integrou os anúncios, provas App e documentação.
App completo: **1.223 testes de topo PASS, 496,600 s**, log
`command-c38-feedback-app-full-20260923.log`. Refinamentos finais do renderer,
testes de falha/cancelamento e preservação do contexto original das ações de
camada foram requalificados no recorte Deck completo: **113 testes de topo
PASS, 199,164 s**, log `command-c38-feedback-deck-final-20260923.log`.
O teste opt-in de latência física permanece pulado; não há nova medição HID.
Revisão independente **Hume (Luna)**: três verificações, incluindo conferência
final dos 21 arquivos. P2 do título ausente em raster 16×16 corrigido com prova
de pixels; restauração do contexto original das ações de camada conferida.
Resultado final: **sem P1/P2 ou pendências adicionais**. Diff-check, integridade
sem bytes NUL e verificador de status AEP também passaram.
Não houve Wails, pacote ACP, executáveis diagnósticos personalizados, acesso
ao banco pessoal, push ou PR. Aceite físico/NVDA continua separado.

## 146. Apresentações por estado e estado efetivo de camada — 23/09/2026

Continuação de C38/D13: o formulário existente edita padrão e dez variantes
textuais, de ícone ou imagem. Campos ausentes herdam o padrão, por idioma nos
títulos. Não há configuração técnica de serial/digest nem grade obrigatória.
A troca de variante cancela leitura de arquivo pendente e limpa o erro daquela
seleção; não aplica um arquivo atrasado na variante seguinte.

O contrato `presentation.states` rejeita estados/campos desconhecidos e variantes
aninhadas. A projeção é imutável; consenso entre bindings é calculado depois da
herança. Imagens base e de estados usam normalização/owner/quota existentes,
com até onze uploads de 1 MiB por solicitação. A confirmação vincula os digests
sem lê-los para o usuário; gravação e remoção de assets não referenciados são
transacionais. O limite canônico de 64 KiB permanece, sem bytes de imagem.

Ligado/desligado para Ativar/Alternar camada lê metadados da mesma configuração
publicada que resolve os comandos. Uma primeira abordagem que consultava regras
do SQLite separadamente foi substituída antes da entrega: um commit ainda não
publicado não pode se combinar com versões antigas do host. O estado inclui
ativações de outras origens e regras Always; contexto não provado retorna
desconhecido, nunca desligado arbitrário. Alvo ausente, escopo incorreto e mapa
obsoleto não produzem indicador. Clones e restauração preservam os metadados.

Feedback transitório tem prioridade e retorna à apresentação atual. As mudanças
persistentes usam evento próprio, sem fingir invocação ou sucesso; baseline
inicial silenciosa, deduplicação em memória limitada e guards de owner, mapa,
sessão e expiração. Nenhum trabalho de imagem/estado foi colocado no key-down,
nem acrescentada auditoria à navegação.

**78 I / 6 P / 0 N = 84; C38 continua P**, agora com variantes e estado de
camada implementados, mas sem aceite integral físico/NVDA. Não se afirma
conclusão do AEP inteiro. Saídas R e gates preservados:
**11 A / 14 I / 22 P / 1 N = 48; 1/12 aceito**. O roteiro cumulativo de usuário
inclui edição/reabertura, herança, resultado temporário, mudança por outra
origem e navegação NVDA. Nenhum checkbox manual foi promovido.
Este lote não encerra as pendências dos outros cinco critérios parciais:
C43, C51, C65, C70 e C83. A implementação das variantes não é evidência de
integração externa, crash de processo ou qualificação nativa desses critérios.

Qualificação inicial: recorte App **13 testes de topo PASS, 22,382 s**, log
`command-c38-states-focused-20260923.log`; frontend de anúncios/Topbar
**303 testes PASS**. Domínios `commandbindings`, `commandconfig`,
`commandimage`, `commanddeck`, `commandportability` **PASS**, log
`command-c38-states-domains-20260923.log`. TypeScript, ESLint focado, vet desses
domínios e App, diff-check e verificador de status AEP passaram.

Implementação paralela com Luna: Hooke (contrato e provas de apresentação),
Schrodinger (assets/confirmação), Bacon (editor/i18n), Hypatia (estado publicado).
Main integrou renderer, anúncios, provas adicionais e documentação; Euler faz
a revisão independente.

Evidências finais do backend:

- App completo: **1.235 testes de topo PASS, 555,575 s**, log
  `command-c38-states-app-full-20260923.log`. Somente o teste opt-in de latência
  física foi pulado; não há medição nova de HID.
- Refinamentos posteriores da projeção e provas de mapa/imagem foram
  requalificados: **31 testes de topo PASS, 30,818 s**, log
  `command-c38-states-reviewed-20260923.log`. Inclui criação confirmada de
  acionador → mapa Deck → variante off/on após ativação pelo App, imagens de
  estado persistidas → pixels do renderer, guards e resultados do executor.
- Cinco domínios **PASS** em `command-c38-states-domains-final-20260923.log`;
  projeção/clones finais também **PASS** em
  `command-c38-states-projection-reviewed-20260923.log`. Provas de Always com
  condição não vazia e Context com cláusulas vazias confrontam o próprio
  resolvedor, evitando duplicação divergente de semântica.
- Vet final **PASS**, log `command-c38-states-vet-final-20260923.log`.

Tentativa frontend intermediária preservada: **5.831 testes passaram**, mas a
suíte terminou com erro ao carregar o teste de um helper novo sem consumidor,
removido depois da coleta do Vitest. O helper duplicava a redação já produtiva
no backend; sua remoção não corrigiu uma asserção falha. A qualificação final
foi reiniciada com os arquivos estabilizados. Um recorte de agente também
encontrou erro de acesso ao lock do cache Go; o recorte final do main acima
executou suas provas de assets. Não houve Wails, pacote ACP, executáveis
diagnósticos personalizados, banco pessoal, push ou PR.

Fechamento da seção146:

- Frontend completo final: **466 arquivos / 5.831 testes PASS, 498,36 s**,
  log `frontend/command-c38-states-frontend-final-20260923.log`.
- Revisão independente **Euler (Luna)**, três verificações. A corrida entre
  banco e mapa foi eliminada pela projeção imutável; a hipótese de divergência
  de Always/lifecycle foi confrontada com o resolver, e a derivação passou a
  reutilizar sua projeção efetiva. Dois P2 adicionais de assets foram corrigidos:
  poda de imagens substituídas antes de avaliar quota e preservação de uma
  referência importada ausente ao editar o mesmo binding. Isso não aceita nova
  referência sem asset próprio, não libera bytes de outro owner e não ignora
  corrupção. Resultado final da revisão: **sem achados pendentes**.
- Após essas correções, recorte amplo de configurações: **66 testes de topo
  PASS, 40,837 s**, log `command-c38-states-settings-final-20260923.log`.
  Recorte final de imagens: **9 testes de topo PASS, 25,506 s**, log
  `command-c38-states-images-final-20260923.log`. Inclui API confirmada com
  referências indisponíveis e 14 subcasos de quota, rollback e fronteiras de
  binding/owner. O App completo acima precede esses refinamentos; estes são
  qualificados pelos recortes finais, não por uma segunda suíte App completa.
- Vet pós-review **PASS**, log `command-c38-states-vet-delivery-20260923.log`.
  Sem alterações nas assinaturas públicas do Wails ou nova migração de banco.

## 147. Camada por programa: integração e resumo de auditoria — 23/09/2026

A lacuna de prova integrada descrita em C43 foi fechada no App:
configuração confirmada de camada com `foreground.process=editor.exe`,
acionador Stream Deck de **Ativar camada**, captura no ingresso, executor
durável real, claim persistida e publicação dos comandos da camada-alvo.
Processo divergente e erro de captura não criam claim nem invocação executável.
A troca de foco depois da captura não muda a ocorrência nem provoca recaptura.

No atalho global, a configuração confirmada cria uma supressão contextual do
default real de um job. O callback registrado recusa o job no programa que
ativa a camada; fora dele, percorre admissão, decisão e executor produtivos,
usando uma tool controlada, com run e ledger persistidos. A mudança de foco após a captura não troca essa
decisão, e a indisponibilidade impede admissão/execução. Registrador e leitor
do SO são controlados; não há nova prova nativa ou execução de job pessoal.

Essa prova revelou uma segunda divergência: a autoridade de configurações
usava uma projeção sem os defaults globais de voz/jobs, embora o runtime já
os publicasse. Settings e supressão de defaults passam a usar a mesma projeção
completa; o fingerprint inclui esses defaults. A apresentação mantém a origem
`keyboard.global` e o acorde canônico, com nome de camada localizado. A tela
permite consulta/supressão/restauração, sem usar o editor genérico para alterar
o registro nativo, que continua pertencendo ao perfil de voz ou job.

O adapter Windows passa a construir o snapshot pela mesma fábrica validada
usada pelos leitores controlados dos testes. Não há `unsafe`, identidade
exportada, endpoint de injeção, captura Win32 necessária à suíte ou desvio do
executor. PID, HWND, lifetime e estabilidade do foco continuam verificados
pelo adapter antes da construção; timestamp e versão continuam internos.

A prova integrada revelou um defeito: `foreground_summary` era sempre o
marcador de redação, inclusive para o resumo allowlisted produzido pelo App.
O ledger agora admite exatamente executável normalizado sem caminho, classe
da janela e versão conhecida do provider. Documento inesperado, campos extras,
duplicatas, controles, excesso de tamanho e caminhos recebem redação integral.
O snapshot/identidade nativa permanece transitório. Não há auditoria nova para
navegação, migração de banco ou alteração de binding Wails.

**C43 P→I; 79 I / 5 P / 0 N = 84 (94,0%).** Restam parciais C38, C51,
C65, C70 e C83. A contagem é de implementação identificada, não de aceite.
Nenhum checkbox final ou gate foi promovido; saídas R permanecem
**11 A / 14 I / 22 P / 1 N**, com **1/12 gates aceito**.

Qualificação já executada neste recorte:

- Domínios `commandbindings`, `commandconfig`, `commandforeground` e
  `commanddeck`: PASS. `commandforeground` também passou com `-count=10`.
- `commandledger` completo após a correção: PASS, 6,841 s; inclui persistência
  do resumo no SQLite e recusas de documentos inesperados.
- App: recorte `Test(Command(Foreground|Global|Deck|Physical|Occurrence)|ContextualDeckLayer)`
  PASS, 119,830 s. Não corresponde à suíte App inteira nem inclui refinamentos
  posteriores dos novos testes desta seção.
- `go vet` de `commandforeground`, `commandledger` e App: PASS.
- Integração global final: PASS, 3,693 s, em
  `command-c43-global-final-20260923.log`. Teste Deck final, com cinco cenários
  separando preview e pressão: PASS, 6,515 s. Apresentação global final:
  PASS, 20,587 s. Configurações do App: PASS, 20,443 s, em
  `command-c43-settings-final-20260923.log`. As execuções finais corrigem erros
  dos rascunhos de fixtures (supressão sem referência, expectativa de ID após
  override e ordem de modificadores); essas falhas não foram tratadas como
  falhas do produto.

Limites: SO controlado nos testes de App; não foi executada captura Win32/HID
física nem a suíte `!windows`. A recusa sem adapter é coberta pelo contrato
existente e a propagação de indisponibilidade pelo App foi exercitada com
leitor controlado. Sem pacote ACP, Wails, executáveis diagnósticos, banco
pessoal, push ou PR. Roteiro manual continua acumulado no guia de comandos.

Fechamento técnico da seção147:

- Após revisão, snapshot exige identidade completa, provider conhecido, resumo
  canônico e hash consistente. `CaptureBeforeShow` usa a mesma validação. Isso
  verifica consistência, não atesta origem do SO: o leitor continua montado no
  backend confiável, sem endpoint de seleção/injeção pelo cliente.
- Classes com caminho absoluto/relativo a drive e textos com controles Unicode
  de formatação são recusados/redigidos. Classes com dois-pontos sem caminho,
  como `ATL:00012345`, continuam válidas. Casos negativos e positivos cobertos.
- App pós-hardening: PASS, 47,510 s,
  `command-c43-app-reviewed-20260923.log`; recorte final após os últimos guards:
  PASS, 29,406 s, `command-c43-app-final-20260923.log`.
- Domínios finais `commandforeground` e `commandledger`: PASS, 1,600 s e
  6,947 s. TypeScript e ESLint focado: PASS; vet final do App e domínios: PASS.
- Frontend: `CommandSettingsPage.test.tsx` e `CommandSettingsPage.advanced.test.tsx`,
  **86 testes PASS**, incluindo supressão com argumentos vazios e restauração
  pelo ID persistido do override global. Avisos `act(...)` em teste existente
  não impediram a execução; frontend completo não foi repetido nesta seção.
- Implementação paralela Luna: Newton (Deck e UI), Laplace (integração global).
  Revisão independente de backend por Leibniz; UI revisada por Laplace e
  integração global revisada por Newton. Os achados de validação/redação foram
  corrigidos e requalificados; a fronteira de confiança do Reader foi
  explicitada, sem criar autenticação fictícia dentro do processo.
- Revisão final sem achados de produção pendentes. A prova global usa
  registrador/tool controlados e sincroniza o despacho por canal; cancelamento
  em timeout não é uma prova de encerramento físico do callback do SO.

## 148. Diálogo e job concorrentes no mesmo gerenciador — 23/09/2026

A lacuna de C83 é tratada com o `hotkey.Manager` produtivo compartilhado entre
o job e a reserva de Ctrl+Shift+R. A fronteira substituída é somente a captura
do SO: o teste não escolhe callbacks nem reimplementa prioridade. A decisão,
admissão, executor, tool de teste e persistência do job continuam no App real.

`NativeHotkey`, `NativeFactory` e `NewManager` tornam explícita a fronteira de
captura já existente. O singleton usa o mesmo construtor; não há caminho de
execução alternativo, modo de teste no runtime ou alias de compatibilidade.
Jobs e diálogos devem compartilhar a mesma instância, como na montagem atual.
A revisão identificou empréstimo indevido da fatia de modificadores à fábrica:
agora ela recebe uma cópia, com prova de preservação até a restauração.

Matriz automatizada transversal desta rodada:

- Defaults, sobreposição, múltiplas camadas e barreira de diálogo:
  suíte `internal/commandbindings`, incluindo fallback e topmost.
- Inputs/IME, repetição, foco, contexto vivo e paleta: 14 arquivos frontend,
  **233 testes PASS**, log `frontend/command-c83-frontend-20260923.log`.
- Abas, alvo e providers de foco: quatro arquivos adicionais,
  **50 testes PASS**, log `frontend/command-c83-context-20260923.log`.
- Reconexão/isolamento de dispositivos: suíte `internal/commanddeck`.
- Prevenção de execução duplicada/replay: `internal/commandexecution` e
  `internal/commandledger`. Os cinco domínios, incluindo hotkey, passaram.
- Recorte App de decisão/global/Deck/foreground: **PASS, 101,306 s**,
  log `command-c83-app-20260923.log`. Essa execução precede a estabilização
  do teste integrado novo e não é citada como evidência dele.
- Hotkey após correção da cópia: **dez repetições PASS, 4,609 s**.

A primeira chamada de backend continha o nome inexistente `commandresolution`
e terminou com erro de setup; o resolvedor pertence a `commandbindings` e a
chamada corrigida passou. Vitest inicialmente não iniciou esbuild no sandbox
(`EPERM`); os dois recortes normais passaram com permissão de execução. Não se
atribui nenhum desses erros ao produto ou ao antivírus.

`go test -race ./internal/hotkey -count=1` não executou: o ambiente está com
CGO desabilitado. Não foi alterada a toolchain nem contornada essa restrição;
os testes comuns não substituem qualificação com detector de corrida.

C38 foi reconciliado para não listar variantes/estado persistente como código
faltante: foram entregues na seção146. Seu aceite integral de teclado/NVDA e
dispositivo continua pendente. A integração externa de C65/C70 ainda exige
migração coordenada de middleware/bootstrap/AEP-0052; não foi habilitada por
inferência nem pelo simples fato de existir tabela ou autenticador de biblioteca.

Fechamento:

- Integração nova: **três repetições PASS, 22,234 s**, log
  `command-c83-manager-final-20260923.log`. Aguarda retorno do dispatch e
  ledger terminal; não confunde o envio ao canal com conclusão do efeito.
  Após acrescentar conferência final de contadores, outras **três repetições
  PASS, 21,327 s**, log `command-c83-manager-delivery-20260923.log`.
- Implementação paralela **Pasteur (Luna)**, integração/revisão do main e
  revisão independente **Banach (Luna)**. Achado da fatia compartilhada
  corrigido e requalificado. Sem achados bloqueantes remanescentes.
- Limite explícito da asserção negativa de evento antigo: usa observação
  limitada de 40 ms, seguida por nova conferência após o job terminar; não
  prova descarte sob atraso arbitrário de escalonamento. Não foi criado hook
  de teste no runtime para forçar essa prova. Lifecycle/cancelamento seguem
  cobertos separadamente no pacote hotkey; aceite do SO permanece manual.
- `go vet ./internal/hotkey ./internal/app`, diff-check e verificador de
  status AEP **PASS**. Não houve ACP, Wails, executável diagnóstico próprio,
  acesso ao banco pessoal, push ou PR.

**C83 P→I; 80 I / 4 P / 0 N = 84 (95,2%).** Restam C38, C51, C65 e C70.
Não se promove nenhum checkbox de aceite, saída R ou gate:
**11 A / 14 I / 22 P / 1 N = 48; 1/12 aceito**. O fechamento agregado de R12
inclui hardware/NVDA, isolamento/desempenho e review/CI, independentemente da
classificação de implementação dos critérios.

## 149. Cadastro administrativo externo montado — 23/09/2026

Primeiro percurso produtivo de preparação de C65/C70/R05.1, sem liberar o
executor externo antes de seus pré-requisitos:

- `external.identity_admin_scopes` opt-in, sem novo segredo. Issuer configurado
  e todos os scopes exigidos pelo serviço; role `admin` não é substituto.
- `POST /auth/external/identities/bootstrap` aceita apenas `{}`; deriva ator
  e alvo do `sub` assinado, que precisa coincidir com usuário local ativo.
- `POST /auth/external/identities` exige administrador já vinculado/ativo e
  bootstrap registrado, para vincular explicitamente outro subject do mesmo
  issuer a usuário existente. Sem JIT, upsert silencioso ou usuário atual.
- A v31 cria auditoria com ator/alvo/ação/horário, sem token, com FKs e índice
  parcial que permite somente um bootstrap por issuer. Auditoria é a primeira
  escrita da transação; autoridade e alvo são relidos sob a mesma transação
  antes do vínculo. Falha desfaz ambos. Nenhuma DDL ocorre na solicitação.
- Corpo limitado, campos desconhecidos rejeitados, rate limit e respostas
  sem detalhes de contas/SQL. Fora do modo externo, rotas retornam 404;
  sem configuração administrativa, 503. Sucesso é 201 e conflito é 409.
- `startHTTPAPI` usa `newHTTPAPIHandler`, exercitado por teste de montagem do
  App com JWT/JWKS e SQLite reais, sem abrir o listener público ou o app.

Não há cutover de `/auth/me`, readiness administrativa publicada, ingresso de
comandos ou revogação remota nova neste lote. D6 da AEP-0052 segue canônica.
A divergência já existente entre auth_context_id por token (AEP) e por conta
(biblioteca) foi apresentada ao usuário; não foi alterada sem essa decisão.
Documentação de usuário explica explicitamente a limitação e o opt-in.

Qualificação inicial:

- HTTP com JWT assinado/JWKS local, recusas de issuer/scope/subject, payload,
  administrador revogado, duplicidade e executor ainda indisponível: **PASS**.
- Montagem do App: **PASS, 22,409 s**,
  `command-external-enrollment-app-20260923.log`.
- Suítes auth/httpapi/commandidentity/config: **PASS**,
  `command-external-enrollment-domains-20260923.log` (antes dos novos testes
  finais de enrollment; não é evidência antecipada desses testes).
- Migração/schema/FKs/rollback e cinco fixtures publicadas com segundo boot:
  **PASS, 4,938 s**, `command-external-enrollment-migration-final-20260923.log`.
  A primeira tentativa revelou uma referência de tipo no pacote errado no
  teste novo; corrigida para usar a migração real v26 e SQL da tabela canônica.

**80 I / 4 P / 0 N = 84**, sem promoção de C65/C70. Saídas/gates preservados:
**11 A / 14 I / 22 P / 1 N = 48; 1/12 aceito**. Próxima entrega de C65/C70:
identidade por token e revogação coordenada, seguida de readiness/migração
explícita do middleware e ingresso produtivo no executor, com a matriz de
isolamento/revogação. Não são novos critérios ou aceite manual deste cadastro.

Fechamento técnico da seção149:

- Suítes finais auth/httpapi/commandidentity/config **PASS** em
  `command-external-enrollment-domains-final-20260923.log`. O teste corrigido
  distingue administrador sem mapping de mapping legado sem bootstrap e
  mantém a cobertura das duas recusas e de rollback.
- Concorrência em duas conexões SQLite: **20 repetições PASS, 2,605 s**,
  `command-external-enrollment-concurrency-20260923.log`.
- HTTP após recusar também `null` e arrays: **PASS, 6,012 s**.
- Regressão de migrations/registry/upgrades/schema: **PASS, 2,819 s**,
  `command-external-enrollment-schema-regression-20260923.log`. Inclui a
  restrição final de igualdade entre subject legado e IDs no bootstrap.
- `go vet` dos cinco pacotes alterados, diff-check e verificador de status
  dos AEPs **PASS**. Frontend/Wails não foram alterados nem executados.
- Implementação paralela **Anscombe (Luna)** em auth e **Epicurus (Luna)**
  em schema; main integrou rotas, configuração, montagem produtiva, testes
  HTTP/App e documentação. Revisão independente **Turing (Luna)** em duas
  conferências: **sem achados acionáveis pendentes**, incluindo parser final,
  atomicidade e guarda de bootstrap. Testes continuam evidência separada.
- Sem pacote ACP, executável diagnóstico próprio, banco pessoal, push ou PR.

## 150. Identidade individual de tokens e revogação do vínculo — 23/09/2026

Com a decisão autorizada pelo usuário, corrigida a divergência registrada na
seção149: `auth_context_id` combina issuer, subject e fingerprint SHA-256 do
token, sem guardar o JWT. Tokens diferentes não compartilham identidade,
mesmo pertencendo ao mesmo vínculo administrativo.

- O contexto capturado guarda um grupo interno do vínculo no próprio mapa
  do EpochService; não há store, mutex ou gate paralelo.
- Revogação pelo serviço de identidade invalida todos os tokens daquele
  vínculo e cancela suas esperas sob o gate antes da alteração persistida.
  Outros vínculos do usuário são preservados. Falha da alteração mantém a
  invalidação conservadora; efeito já iniciado não é desfeito.
- Reativação exige captura nova: snapshots anteriores continuam obsoletos.
- Validação inicial pode aquecer JWKS fora do gate; a captura revalida token
  com cache e relê vínculo/usuário dentro do gate, fechando a janela entre
  autenticação inicial e revogação. O gate não faz busca de rede.
- Sem nova migração, segredo ou armazenamento de JWT; sem mudança da D6,
  readiness, middleware ou novo endpoint remoto de revogação.

Qualificação: suítes `commandidentity`, `commandsecurity` e `commandexecution`
PASS; testes externos de identidade com 20 repetições PASS. `go vet` dos
quatro pacotes auth/identity/security/execution e `git diff --check` PASS.
O teste integrado usa repositório SQLite, autenticador, adaptador de epochs
e gate reais; verifica cancelamento de dois tokens, recusa após revogação e
impossibilidade de admitir provas antigas após reativação. Outro teste força
revogação exatamente entre autenticação e captura. Sem teste ACP ou Wails.

**80 I / 4 P / 0 N = 84** permanece: C65/C70 ainda dependem do cutover
coordenado do middleware, readiness e ingresso produtivo no executor.
Saídas/gates: **11 A / 14 I / 22 P / 1 N = 48; 1/12 aceito**. Este lote
fecha a identidade/revogação interna, não o acesso externo completo.

Implementação paralela: Erdos (Luna) em autenticação, Turing (Luna) em grupos
de epochs; integração e testes ponta a ponta da biblioteca pelo agente principal.
Revisão independente Kant (Luna): sem achados acionáveis. Suíte auth também
PASS; testes de grupo incluem concorrência, isolamento e falha conservadora.
Verificador dos status AEP PASS. Detector de corrida não executado neste lote;
testes concorrentes comuns não substituem `-race`. Sem push ou PR.
Regressão final conjunta com `-count=1`: auth, commandidentity,
commandsecurity, commandexecution e httpapi **PASS**; vet e diff-check finais
também **PASS**.

## 151. Middleware HTTP externo adota vínculos administrativos — 23/09/2026

O principal de `/auth/me` deixa de usar `sub` diretamente. Após validar JWT,
JWKS, issuer, audience e scopes, exige schema v26/v31, registro de bootstrap
do issuer e vínculo exato habilitado para usuário local ativo. Sem prontidão
administrativa retorna 503; sem vínculo/alvo ativo retorna 401. Não há JIT,
fallback para UUID local nem cache de mapping. A role continua vindo do IdP,
e não da role local. Sessão local não é inventada para o JWT externo.

A montagem produtiva `App.newHTTPAPIHandler` fornece o mesmo repositório ao
cadastro e ao middleware. As rotas de bootstrap continuam independentes do
middleware para permitir a primeira migração. Desabilitar novos cadastros
administrativos depois do bootstrap não remove o acesso das contas migradas.
O parsing exige Bearer e `/auth/me` recebe `Cache-Control: no-store`.

D6/D8 da AEP-0052 e o contrato de ingresso desta AEP foram atualizados no mesmo
ciclo. Documentação orienta a migração de instalações externas: bootstrap e
vínculo de cada conta antes de usar a API. O modo local não exige esses passos.
Readiness do executor continua ausente: este lote não habilita execução externa,
novas rotas de revogação ou dispositivos físicos externos.

**80 I / 4 P / 0 N = 84**: C65/C70 continuam parciais enquanto faltam montagem
do executor externo e matriz de isolamento/revogação do ingresso produtivo.
Saídas/gates mantidos em **11 A / 14 I / 22 P / 1 N = 48; 1/12 aceito**.

Validação e revisão:

- Montagem produtiva do App com JWT/JWKS assinados e SQLite de teste: PASS,
  incluindo 503 pré-bootstrap, cadastro HTTP de subject não UUID, userId local,
  revogação sem cache e continuidade após desabilitar novos cadastros.
- Suítes auth, commandidentity e commandsecurity PASS; suíte HTTP completa
  PASS após atualizar as fixtures para fornecer o repositório de vínculos.
  Duas tentativas intermediárias encontraram edição de fixture ainda incompleta
  (campo/import e injeção do repositório), corrigidas sem remover cobertura.
- `go vet` de auth/httpapi/commandidentity/app, diff-check e status AEP PASS.
- Epicurus (Luna): prontidão por issuer; Euler (Luna): matriz HTTP assinada;
  agente principal: montagem produtiva, middleware, testes App e documentação.
- Carson (Luna), revisão independente em duas rodadas: corrigido P2 que
  tratava falha de banco como 401. Ausência/revogação permanecem 401; falha
  operacional retorna 503 genérico e mantém o erro original no log. Sem outro
  achado acionável após a correção.
- Sem banco pessoal, ACP, Wails, executável diagnóstico, push ou PR.
- Testes finais específicos de falha de consulta (503 redigido) e subject igual
  ao UUID local sem vínculo (401, sem JIT): **PASS, 5,869 s**. App final:
  **PASS, 19,658 s**; HTTP completo **PASS, 8,124 s** antes desses dois casos
  adicionais. Auth/identity/security também passaram na repetição final.

## 152. Política externa usa roles do JWT — 24/09/2026

Após confirmação explícita do usuário, corrigida a divergência entre D8 da
AEP-0052 e `commandidentity.authorizeFresh`: tokens externos eram comparados
com a role local do usuário vinculado. Agora a regra compara exclusivamente
a lista de roles das claims revalidadas, inclusive roles posteriores à primeira.
`RequiredRoles` preserva alternativas (qualquer uma); `RequiredScopes` continua
exigindo todos os scopes. Sem roles no token não há fallback para admin local.

A identidade é revalidada antes da política; a comparação da projeção continua
recusando role/scopes forjados ou claims alteradas. Sessões locais e jobs mantêm
a política de role local. Não há mudança de schema, segredo, middleware ou
habilitação do ingresso externo neste lote.

**80 I / 4 P / 0 N = 84** e **11 A / 14 I / 22 P / 1 N = 48; 1/12 aceito**
preservados. C65/C70 continuam dependendo da integração do ingresso produtivo
ao executor; esta correção remove um defeito da autorização que a precede.

Qualificação: suites commandidentity/auth/httpapi/commandexecution PASS,
incluindo nova matriz roles locais versus JWT, alternativas, roles ausentes,
scopes parciais, alteração de claims e projeções adulteradas. Nos casos de
adulteração há baseline autorizado e os requisitos continuam satisfeitos,
isolando a recusa por divergência da projeção. Repetição final de identity
após reforço dos testes PASS; vet, diff-check e status AEP PASS.
Kierkegaard (Luna) implementou os testes; Turing (Luna) revisou código e
casos finais independentemente, sem achados acionáveis. Sem ACP, Wails,
banco pessoal, executável diagnóstico, push ou PR.

## 153. Ingresso externo no executor integral — 24/09/2026

`commandexecution.NewExternal` conecta o autenticador e a política externa ao
pipeline integral existente (execução, resultado efêmero, replay e consulta).
Não duplica fila, ledger, gate ou executor. O token pertence ao contexto privado
da solicitação, não ao estado global do serviço, envelope ou persistência.

- Aquecimento JWKS fora do gate, limitado por prazo e lifecycle; revalidação
  cached dentro do gate. Shutdown também cancela o aquecimento inicial.
- Ownership e grupo de revogação derivados do JWT e mapping, sem sessão local
  inventada. Política de roles/scopes soma-se à autorização de recursos do host.
- Execução e consulta passam pelo mesmo controle de identidade; revogação do
  vínculo usa o mesmo EpochService e cancela esperas existentes.
- Origem fixa Palette/UI/Chat; fontes físicas, system e demais origens não
  suportadas são recusadas na construção. Sem broker externo, handlers UI e
  decisões interativas são recusados, sem aprovação implícita.

Limite explícito: trata-se de integração da biblioteca ao executor, não de
montagem HTTP/App concluída. O runtime de produto ainda depende da sessão
desktop local; não foi reutilizado como identidade de outro usuário. Faltam
snapshot/resolução/handlers isolados por usuário externo e sua publicação
produtiva. O construtor não publica readiness nem abre endpoint.

Validação do bloco: testes dos pacotes commandexecution, commandidentity, auth e
httpapi passaram, assim como go vet desses pacotes e git diff --check. Os testes
novos usam SQLite e executor reais com verificador sintético de claims; cobrem
isolamento por token, replay, recusa por roles/scopes, revogação de execução em
fila, usuário mapeado substituindo o contexto do transporte e shutdown durante
autenticação normal/administrativa. Panic no aquecimento não vaza operação do
lifecycle. Administração é obrigatória na construção e participa do shutdown.
A recusa da política externa é traduzida para ErrDenied do executor, preservando
o estado denied no ledger em vez de classificá-la como cancelamento.
Revisão independente realizada; sem execução de ACP, Wails ou app.

**80 I / 4 P / 0 N = 84; 11 A / 14 I / 22 P / 1 N = 48; 1/12 aceito**
inalterados; C65/C70 permanecem parciais.

## 154. Transporte HTTP externo e fechamento da revisão de segurança (24/09/2026)

O transporte de commandexecution.ExternalService oferece execução, consulta
restrita ao contexto autenticado e revogação administrativa. As três origens
permitidas são palette, ui e chat; fontes físicas não são expostas. O corpo
contém somente EnvelopeCandidate, limitado a 8 KiB, sem campos desconhecidos.
Bearer é estrito, respostas usam no-store e as operações têm rate limit.
Consulta retorna apenas ID, estado e resumo; resultado efêmero só pode sair
na resposta da execução original, nunca por consulta/replay.

A composição HTTP copia o mapa fornecido e verifica o pareamento origem/serviço
e a mesma autoridade (autenticador/admin/epochs) em todas as entradas. Falha de
composição desabilita o conjunto inteiro. O administrador requer issuer
explícito, e operações de vínculo não aceitam issuer diferente do configurado.

**Limite de entrega:** transporte não equivale à montagem produtiva. O App ainda
não fornece ExternalCommands; por isso retorna 503 nessas rotas em modo externo.
Continuam pendentes o contexto do cliente autenticado e os handlers apropriados,
sem inferir autoridade a partir da sessão desktop. Não há promoção de C65/C70.

**Decisão de escopo do mantenedor:** manter workspaces como estão. Migração para
banco relacional com user_id e sem mapas JSON será uma iniciativa separada,
não requisito geral nem bloqueio automático do AEP-0103. A pesquisa de issues
abertas/fechadas encontrou a #118 (Vincular perfil à conversa no banco), que
menciona essa evolução, mas não uma issue específica para migração completa de
workspaces. Nenhuma issue foi criada ou alterada nesta verificação.

Contagens mantidas: **80 I / 4 P / 0 N = 84; 11 A / 14 I / 22 P / 1 N = 48;
1/12 aceito**. Dependência de contexto do workspace não implica dependência de
migração do seu armazenamento.

Validação: go test dos pacotes auth, commandidentity, commandexecution e httpapi
passou; TestHTTPAPI* de internal/app também passou após o ajuste do issuer.
go vet nos quatro pacotes, git diff --check e verificador de status dos AEPs
passaram. A matriz HTTP usa SQLite e executor reais com claims sintéticas;
o teste App de cadastro usa JWT assinado e JWKS temporário. Não foram executados
testes ACP, Wails, dispositivos físicos ou migração de dados pessoais.

Revisão independente: Godel revisou o diff e encontrou uma classificação
incorreta de cancelamento na autorização cached. Corrigida preservando ctx.Err()
antes de traduzir recusas; teste cobre cancelamento tanto na resolução quanto
na autorização. Releitura confirmou o achado fechado. Os achados anteriores de
issuer e composição HTTP também foram corrigidos. commandsecurity e
commandledger passaram em regressão adicional.

## 155. Ferramentas na paleta e fechamento da interface de configuração (24/09/2026)

Checkpoints: `03ef8f0a4` remove a revogação externa fora do gate;
`6f516326a` entrega diagnóstico seguro por dispositivo; `f59f7d6c9` qualifica
versões de acionadores e latência; `2a9049481` liga ferramentas ao executor;
`79b385168` entrega argumentos/preferências da paleta e configuração acessível.

### Implementação entregue

- Ferramentas elegíveis do catálogo runtime entram como comandos de alvo fixo,
  somente ad hoc pela paleta. JSON de argumentos é validado contra o schema real
  antes da decisão; execução revalida sessão, owner, alvo, geração e schema.
  A bridge mantém correlação e redação; argumentos e resultados brutos não
  viram bindings nem resposta desktop persistente. A resposta mostra estado,
  não o payload bruto da ferramenta. D11 continua vedando persistência sensível.
- Mudança no catálogo MCP reconstrói rotas e invalida versões antigas. Catálogo
  limitado a 4.096 entradas; orientação na UI é paginada, limitada e cancelável,
  nunca fonte de autorização. `json.RawMessage` chega como objeto JSON pelo
  serializador; a UI também tolera bytes/string, sem editar bindings gerados.
- Picker compartilhado mostra atalho efetivo, favoritos/recentes isolados por
  usuário/workspace e acesso à configuração. O formulário efêmero interpreta
  o schema real do Go, limpa contexto obsoleto e não deixa resposta tardia de
  uma execução encerrar o formulário de outra. Tab libera alvos capturados sem
  restaurar foco; mudança programática de foco preserva a captura válida.
- Configuração esconde opções avançadas sob controle acessível e expande
  entradas inválidas para correção. Avisos usam announcer global. Dispositivos
  mostram modelo, teclas, estado e motivo seguro, sem serial/ID na interface.
  Falha genérica de abertura não diagnostica sozinha disputa pelo driver HID.
- Corpus compartilhado qualifica triggers v1/v2 e recusa v3. Medição opt-in
  com SQLite/executor reais e portas controladas: executor p50 3,0136 ms,
  p95 4,526 ms, p99 10,5724 ms; reserva+CAS p50 0,5456 ms, p95 1,2784 ms,
  p99 4,0336 ms. Não mede renderização/HID nem certifica orçamento de 1 ms.

### Verificação e revisão

`npm test -- --maxWorkers=4 --silent`: **469 arquivos e 5.862 testes PASS**,
303,91 s. As rodadas anteriores detectaram seletores desatualizados, isolamento
de preferências, anúncios locais e foco/cleanup; foram corrigidos sem remover
testes. `tsc --noEmit`, ESLint (zero erros, quatro avisos preexistentes),
Stylelint (zero erros) e tokens passaram. Godel revisou independentemente;
achados de schema, contexto tardio e limpeza no Tab foram fechados na releitura.
Após essa rodada, o teste de orientação foi parametrizado para objeto JSON
real do Wails, string e bytes: sete testes focados PASS, sem mudar produção.
O verificador de status dos AEPs passou (107 documentos, 106 números).

Backend: os 32 pacotes `./internal/command...`, excluídos testes manuais,
passaram. No App, grupos `^TestCommand`,
`^Test(AppCommand|AppDrain|AppShutdown|Contextual)` e ferramentas passaram;
`go vet` dos pacotes command/auth/httpapi e App passou. Bernoulli revisou a
ponte e a sanitização do nome da ferramenta; achado de caracteres de controle
foi corrigido. A revisão somente leitura dos grupos I01/I02/I05/I10/I11/I12/I14
não encontrou nova lacuna concreta, mas não substitui aceite R06 agregado.
Não foram executados ACP/acpregistry, Wails, app, hardware ou banco pessoal;
race/CI remoto e NVDA não foram certificados.

### Bloqueios reais preservados

R03.4 continua parcial. O novo diagnóstico
`TestProductJobRunEventCommandReplayEndToEnd`, ainda no worktree e falhando,
reproduz: publicar uma claim nova altera a configuração efetiva e cancela o
`job.run` que sustenta a própria claim. Banco/grant permanecem válidos; o run
perde elegibilidade pelo cancelamento. Preparar o binding antes do run não
resolve. Lagrange e Bernoulli confirmaram a causa independentemente. Alterar
essa política exige decisão explícita do mantenedor; não foi relaxada nem o
teste removido/ignorado para produzir PASS.

C65/C70 aguardam composição externa no App e decisão sobre controle de uma
interface conectada versus operações backend. Nenhuma identidade desktop foi
emprestada ao JWT externo. Export sensível e gesto longo permanecem adiados;
migração de workspace continua iniciativa separada.

**Contagens:** 80 I / 4 P / 0 N = 84, inalteradas; 11 A / 18 I / 18 P / 1 N = 48,
com R05.2/R08.2/R09.3/R10.3 promovidos somente a implementação identificada;
1/12 gate aceito. Nenhum checkbox de aceite final foi promovido. Não restam
somente testes manuais, e BASE-PRONTA/AEP-CONCLUÍDO não foram declarados.

## 156. Vínculo externo de interface e preservação seletiva de execução (24/09/2026)

O mantenedor aprovou as duas decisões após a explicação dos impactos:

- A API externa também controlará uma interface do Assistente explicitamente
  vinculada ao usuário autorizado, com destino inequívoco e sem herança de
  sessão desktop ou elevação de roles/scopes. Sem conexão, só operações backend
  compatíveis. Desconexão, contexto obsoleto e troca de usuário invalidam o
  trabalho dirigido à UI; decisões interativas usam o contrato comum.
- Uma mudança reativa de camada poderá preservar execuções já admitidas
  somente mediante prova por execução de equivalência de comando, alvo,
  binding, autorização e contexto. Adição de camada não é suficiente por si.
  Conflitos, dependência alterada ou impossibilidade de comprovação cancelam;
  revogações e barreiras de sessão/segurança permanecem vigentes.

Contratos atualizados no AEP principal (identidades e estado do host) e na
AEP-0052/D4. As decisões deixam de estar bloqueadas por aprovação do usuário.
O estado de implementação e as evidências consolidadas abaixo atualizam esta
seção; não constituem aceite de gate nem conclusão do AEP.

### Evidência intermediária — recibos externos

Commit `1aa9e7c6c`: receipts distinguem `local_session` de `external_token`,
preservam o ID canônico da credencial e recusam decisões externas de mutação de
configuração. Consumo e ledger conferem owner/tipo/contexto exatos; recovery
por drenagem inclui invocações externas sem convertê-las em sessões locais.
A migração 32 reconhece o DDL anterior conhecido, preserva linhas/índice e
reverte integralmente quando o carimbo falha. Revisão independente do
implementador realizada pelo agente principal, com correções de constraints,
formato canônico e fixture de escape antes do commit.

Validação repetida pelo principal: `go test ./internal/commanddecision
./internal/commandbootstrap ./internal/commandledger ./internal/database
-count=1` passou. Isso **não habilita sozinho** a API produtiva nem fecha C65/C70:
composição App/HTTP, vínculo, transporte da interface e revisão integrada ainda
estão em andamento. As contagens acima não foram promovidas por esta entrega.

### Evidência intermediária — publicação reativa de jobs

Commit `79f06ab6b`: a publicação de claims possui prova imutável por execução, comparando resolução,
binding, proveniência e baseline persistido. Somente ingressos sem dependência
contextual mutável recebem a prova; execuções sem prova continuam canceladas.
Revalidação de sessão e configuração ocorre sob o gate de publicação. Renovar
apenas o deadline de uma lease não avança a geração efetiva nem cancela watches.

O principal repetiu os testes de `commandbindings`, `commandconfig`,
`commandsecurity` e `commandexecution` com sucesso. A suíte App
`^(TestCommandJob|TestProductJobRunEvent|TestAppCommand.*Projection|TestCommandLifecycleJob)`
passou em 44,069 s, incluindo o cenário causal completo de job, evento e replay.
As regressões de heartbeat e suppress foram reproduzidas e corrigidas; a
Godel revisou independentemente a revalidação, a cobertura da regressão e o
caminho rápido; os achados foram corrigidos antes do commit. A
verificação adicional de geração persistida fica no ingresso público da paleta,
não no caminho comum dos atalhos contextuais.

### Reconciliação de implementação — 24/09/2026

- **R03.4 e C65/C70: P→I; R05.1: P→I.** Evidências, escopo e limitações
  estão nas linhas atuais de R03/R05/C65/C70 e no resumo desta tasklist. O E2E
  de R03.4 é App fixture: job iniciado pelo ingresso global de hotkey → run e
  outbox do runtime → Consumer App → claim/layer/binding comprovados →
  `workspace.list` downstream e replay sem duplicar efeito. Não é aceite de
  Wails/UI nem prova de acionamento por candidate de paleta `job.run`.
- **R03.4 — subrequisitos cobertos por provas existentes:** restart/recovery
  via SQLite reaberto e lease abandonada, sequência/fingerprint, regras em
  global e dois workspaces, count-cap mantendo fontes pending/processing,
  anti-loop no limite 16/17, falhas/retries limitados e dead-letter terminal.
  AEP-0048/0067 atualizadas no commit `6d0411dbb`. Não se usa subprocesso nem
  se declara kill abrupto: essa prova continua parcial em C51.
- **C38 e C51 continuam P; R05.4 continua adiada.** R06/qualificação geral,
  R12/CI e aceite final continuam abertos; não foram executados manuais,
  ACP/acpregistry, Wails dev/build nem o aplicativo; não houve acesso a
  hardware ou banco pessoal. `wails generate module` aprovado passou para
  regenerar os bindings. Não restam apenas validações manuais.
- **Validações já concluídas:** `go test ./internal/command... -count=1`
  PASS (32 pacotes, conforme resultado do principal); `go test
  ./internal/commandexecution` PASS (27,095 s); frontend `npm test` PASS
  (474/474 arquivos, 5.895/5.895 testes), `tsc --noEmit` e ESLint dos 15
  arquivos afetados PASS; `go vet` App/core/HTTP/jobs PASS; testes externos
  App PASS (3,965 s); Godel (hardening/fingerprint) e Franklin (docs) sem
  achados.
- A suíte App ampla terminou com os demais casos passando e uma falha isolada
  no golden do índice keyboard-defaults: o build de teste compilou uma revisão
  intermediária do hash; o arquivo-fonte foi corrigido e o rerun de
  keyboarddefaults/fingerprint/old-delta/external/HTTP/workspace passou em
  **21,938 s**. Não se declara a rodada ampla original como full PASS.
  Nenhum checkbox A/C ou gate foi promovido.
- **Entrega externa:** commit `0548492ab`, incluindo App/HTTP, vínculo
  efêmero, frontend, bindings gerados e guia de uso. O único ingresso externo
  produtivo é UI; sem vínculo a allowlist admite apenas `workspace.list`,
  com runtime do mesmo usuário ativo. Interface vinculada aceita somente as
  rotas e navegação de abas explicitadas no guia; nenhum efeito contextual
  adicional é implicitamente autorizado. Stylelint e verificador de status
  dos AEPs também passaram.

**Contagens vigentes:** 82 I / 2 P / 0 N = 84; 11 A / 20 I / 16 P / 1 N =
48; 1/12 gates aceito (R04). BASE-PRONTA e AEP-CONCLUÍDO permanecem abertos.

## 157. Queda real isolada e fechamento de lacunas de foco (24/09/2026)

Commits temáticos: `e96f343d0` (prova de queda) e `0afb2c0f7` (foco,
instruções acessíveis e roteiro NVDA).

### C51 — prova de processo, não apenas remontagem

Com autorização explícita do mantenedor, foi executado
`TestC51CrashReplayAfterCommittedHandlerEffectOptIn`, no pacote
`internal/commandexecution`. O teste exige
`ASSISTENTE_C51_PROCESS_CRASH_OPT_IN=1`; sem opt-in ele é ignorado. Reutiliza
o executável corrente de `go test` como filho, sem criar executáveis avulsos
nomeados, e usa exclusivamente SQLite em `t.TempDir()`.

A execução passa por `NewComplete`/`ExecuteEnvelope`, confirma o efeito do
handler com ledger e auditoria ainda `Running` e comprova que outra instância
não adquire o lock nativo enquanto o filho está vivo. Só então encerra esse
filho à força. Uma nova instância obtém a lease e o RestartProof reais;
`CoordinatorRecovery` converte o par para `outcome_unknown`, com resumo `{}`.
Consulta, segunda recuperação e duas reentregas não repetem o handler: o efeito
persistido permanece único. A política e o recovery de produção não foram
substituídos por implementações exclusivas de teste.

O `Start` retorna imediatamente; a escrita do efeito ocorre fora do gate, em
goroutine. Revisão independente de Godel identificou e encerrou esse ajuste;
também foram revisados limpeza da lease, sincronização do buffer/Wait e uso do
executor integral. Revisão final sem achados. O teste passou (5,703 s) e a
repetição independente `-count=3 -v -timeout 180s` passou (1,917 s).

**C51 P→I.** A prova é de interrupção abrupta do processo de teste no Windows,
não desligamento elétrico, morte do sistema operacional ou teste do app Wails.

### C38 — correções entregues; NVDA ainda exige aceite humano

Condições e argumentos agora preservam um foco útil ao adicionar/remover linhas:
entrada nova, vizinha ou Adicionar; condições legadas sem opções têm fallback
no grupo. A DataGrid reutiliza traduções nos três idiomas e anuncia somente
atalhos aplicáveis, com instrução de Shift+F10/Menu e separação entre frases.
Regressões exercitam Tab/Enter e o último item legado, além da descrição em
inglês. Franklin revisou código e roteiro, sem achados finais.

O roteiro `docs/content/guias/VALIDACAO_COMANDOS_NVDA.md`, vinculado no guia
de comandos, registra PASS/FALHOU/NÃO TESTADO por operação, incluindo fluxos
condicionais de importação, revisão de padrões e conexão externa. Nenhum fluxo
não testado recebe aceite implícito. **C38 permanece P**: testes DOM/teclado não
comprovam fala, modo de interação ou foco percebido no NVDA real.

### Validação e limites

- Frontend: **474 arquivos / 5.901 testes PASS** na suíte completa; 61 testes
  focados PASS; `tsc --noEmit` e ESLint dos arquivos afetados PASS.
- Uma rodada focada anterior falhou em `CommandSettingsPage > mantém a seção
  avançada do binding alcançável por teclado`: após Enter, `aria-expanded`
  permaneceu `false` (77/78 testes da página passaram; advanced 13/13). O rerun
  isolado passou (1 PASS/77 ignorados), assim como a suíte completa. A causa
  não foi demonstrada; não se declara a rodada original como PASS.
- Backend: `commandexecution` PASS (28,296 s; revisão final reexecutada em
  21,072 s); `commandledger` PASS (6,081 s),
  `commandsecurity` PASS (1,566 s), `commandinstance` PASS (1,150 s).
  `go vet ./internal/commandexecution` PASS. O crash foi qualificado
  separadamente com opt-in; o skip no teste padrão não conta como essa prova.
- Stylelint terminou com código 0: zero erros e 2.133 avisos em arquivos CSS
  não alterados nesta rodada. Verificador de status dos AEPs e diff-check PASS.
- Não foram executados ACP/acpregistry, Wails dev/build, app, NVDA ou hardware;
  não houve acesso ao banco pessoal nem relaxamento do antivírus.
- R06/qualificação agregada e R12/CI/review/aceites continuam abertos. Esta
  rodada não certifica todo o AEP nem promove checkboxes de aceite final.

**Contagens vigentes: 83 I / 1 P / 0 N = 84 (98,8%); 11 A / 20 I / 16 P /
1 N = 48; 1/12 gate aceito.** Apenas C38 permanece parcial na contagem C.
Históricos anteriores mantêm as contagens que eram válidas em suas rodadas.

## 158. Integração da main para publicação e validação em outro computador (24/09/2026)

Por solicitação do mantenedor, a branch incorpora `origin/main` em `5c9278082`
antes da abertura do PR. Conflitos de conteúdo resolvidos em seis arquivos:
índice AEP, TaskListView e seu teste, rename de painéis, store de tasklists e
service de tool invocations. A integração preserva o menu de configurações,
salvamento automático e filas da main, sem restaurar os caminhos antigos de
duplicação/limpeza de tasklists que o AEP-0103 já migrou.

O lifecycle comum de tool invocations vindo da main passa a transportar
catálogo canônico estrito, redação sensível e guard de admissão dos comandos;
`IsBoundTo` continua conferindo identidade do banco e registry. O adapter de
teste da bridge acompanha `CreateOptions`. Nenhum fallback de autorização foi
introduzido para resolver incompatibilidades do merge.

Os apontamentos de lint são corrigidos sem remover testes: cleanup explícito,
propagação do fechamento de diretório após sync, simplificações equivalentes e
remoção de helpers não utilizados. Testes de recusa de contexto nil e de
opacidade de snapshot mantêm suas asserções, com justificativa local da análise
estática quando o comportamento é intencional.

A validação manual será realizada pelo mantenedor em outro computador, sobre
cópia do banco. A publicação não promove C38 nem os gates gerais de aceite.
**Contagens preservadas: 83 I / 1 P / 0 N; 11 A / 20 I / 16 P / 1 N;
1/12 gate aceito.** AEP permanece In Progress.

**Varredura de atalhos e correções de revisão:** o inventário registra seis
criações de Configurações ainda fora do catálogo e os dois novos Ctrl+N locais
dos editores de workflow/ações customizadas. Não se declara migração integral.
Os hooks locais passam a recusar evento consumido, repetição, composição IME
e AltGraph, preservando painel ativo e modal topmost (15 testes focados PASS).
A paleta restaura a origem disponível ao cancelar, com fallback ao botão
Comandos somente no contexto ainda válido; callbacks de identidade antiga não
roubam foco da tela nova. O adapter externo aceita o timestamp zero emitido
pelo Go no status desconectado sem aceitar contexto extra nesse status.

Revisores independentes: Godel (integração backend/lifecycle), Franklin
(integração frontend), Lagrange (autorização, foco e adapter), Bernoulli
(limpeza App) e Ampere (hooks e regressões visuais). Achados reais de foco e
DTO foram corrigidos com testes. A sugestão de focar o botão após mudança de
identidade foi descartada com justificativa técnica: o callback antigo não
deve interferir no foco da nova tela; o revisor retirou o apontamento.

**Evidências da integração:** frontend completo 477 arquivos / 5.968 testes
PASS (409,56 s), mais o teste adicional de foco entre identidades PASS;
TypeScript, ESLint e Stylelint sem erros (quatro avisos existentes de `any`
em ProfilesPage.test.tsx). `go build ./...`, `go vet ./...` e golangci-lint
com zero apontamentos PASS. App completo PASS (630,977 s), após corrigir
cleanup que tentava rollback de transação já commitada; a primeira rodada
que encontrou essa falha não conta como PASS. O inventário de logging passa
com 774 formatos: a comparação com a main mostra saldo +12 (controllers +0,
app +2, httpapi +6, jobs +9, profiles -5), sem arquivos Go temporários na
contagem observada. O teste continua impedindo `logging.Printf` em produção.

Uma rodada frontend anterior falhou no teste KeyR; passou isoladamente, no
arquivo e nas duas suítes completas posteriores. A causa dessa falha isolada
não foi demonstrada. ACP/acpregistry não executados localmente por causa do
histórico de bloqueio pelo antivírus. Não foram usados Wails dev/build, app,
banco pessoal, NVDA ou hardware nesta integração. Status dos AEPs e diff-check
PASS; CI e validação manual não são substituídos por essas evidências locais.
Todos os demais pacotes Go, exceto ACP/acpregistry, passaram na rodada final
com `go test -p 2 -timeout 10m` (resultados cacheados quando aplicável).

**Primeira rodada remota — PR #833:** bindings e scripts PASS. O CI revelou
falha de compilação Darwin sem CGO, testes de paleta que não aguardavam o
foco/callback assíncrono, fixture E2E sem versão de snapshot do workspace e
falha de publicação de configuração sob race, seguida de timeout acumulado
do pacote App. Esta rodada não é CI verde; as correções estão em validação.

**Retificação da evidência física:** a revisão remota identificou que
`TestManualPhysicalEnvironment` fornecia sessão conhecida/desbloqueada de
forma fixa. A execução manual histórica NÃO comprova `OSSessionObservable`.
Esse ponto volta a exigir execução manual com consulta nativa autoritativa;
as evidências separadas de tecla física/foreground não são anuladas por isso.
Não há promoção de critérios ou gates por alterar o teste. O comentário sobre
FreeBSD foi encerrado com justificativa: não é alvo de produto declarado e
o parser dessa plataforma já estava ausente na main; não se adiciona suporte
funcional por meio de constantes ou keycodes fictícios.

**Correções locais da primeira rodada:** consulta nativa de sessão em
`62f41ea90`, compilação Darwin sem CGO em `f3cc4f7e8`, sincronização dos testes
de paleta em `c0b8755de` e contexto da fixture de configuração em `b8a401654`.
Os dois arquivos de paleta passaram juntos (177 testes); vet e lint dos
pacotes App/ossession/commandphysical/hotkey passaram sem apontamentos.
Essas evidências não substituem a nova execução remota.

O timeout do App permanece pendente: a rodada local completa sem race já
levou 630,977 s. Matrizes de integração repetem fixtures persistidas e
mutações confirmadas; não se atribui o custo a deadlock ou a uma corrida sem
evidência. Nenhum teste foi excluído, nem limite de CI aumentado. A divisão
da execução mantendo cobertura e detector de concorrência foi proposta ao
mantenedor, ainda sem decisão registrada.

**Segunda integração da main:** merge `f203757fe` incorpora `147274d15`
(AEP-0109/PR #832). Três conflitos resolvidos: índice AEP, TaskListView e
interfaces do serviço de tasklists. Preservados compare-and-swap, recarga após
conflito e os comandos/guards do AEP-0103, sem restaurar clonar/limpar legados.
Testes de banco e serviço de tasklists PASS; cinco arquivos Vitest de tasklists,
102 testes PASS; TypeScript, lint dos componentes/store envolvidos e Go build
PASS. Os bindings vieram da main sem edição manual; seu whitespace gerado não
foi alterado. CI desta integração ainda pendente.

**Decisão de foco aprovada pelo mantenedor:** Alt+3 em visualização passa a
significar somente devolver o foco à leitura, sujeito ao binding efetivo e ao
contexto autorizado. Não é replay de mudança de modo nem cria execução
persistida. D17 e guia de usuário atualizados; validação da implementação em
andamento, sem promoção de critérios/gates.

**Segunda rodada de CI (head `c293f60dc`):** backend, frontend, bindings e
scripts PASS. E2E FAIL; backend-race encerrou por timeout acumulado do pacote
App (600,318 s), sem diagnóstico de corrida nesse log. Não é CI verde.

**Correções de integração em validação local:** `9b8218eba` apresenta e anuncia
erros do envio auditado, preservando o rascunho e sem oferecer replay de resultado
incerto. Dois arquivos Vitest (81 testes) e os cenários E2E de erro/streaming PASS.
`971cad649` atualiza fixtures de snapshots, envio, abertura do chat contextual,
limpeza e leitura de perfil. A proxy Wails deixou de inventar `then`; o handoff
de envio é consumido antes de aguardar callbacks e erros de submissão resultam
em `outcome_unknown`. Esse adapter testa integração frontend, não substitui as
provas Go de autorização/isolamento de alvo. Revisores independentes Godel,
Franklin e Ampere: correções de resultado incerto, prova de ACK e concorrência
do mock verificadas; nenhum teste excluído.

A rodada E2E agregada após essas correções teve 164 PASS, 12 FAIL, seis skips
preexistentes, dois interrompidos e 50 não executados por limite de falhas.
Rodada adicional de configurações/workspace/mock: 43 PASS e uma falha de foco.
Há pendências reais ou de fixtures em editor, mensagens, perfis e restauração de
foco; os resultados parciais não promovem aceite. A main permite navegação de
abas dentro do Monaco, enquanto a exceção da seção63 o exclui; a escolha foi
submetida ao mantenedor antes de alterar essa guarda.

O mantenedor aprovou preservar a troca de abas a partir do Monaco em
24/09/2026. Exceção registrada na seção63, restrita à navegação de abas e
mantendo bloqueios de IME, modais e mapa efetivo. Implementação em validação;
não é liberação genérica dos comandos globais em editores.

**Retomada da validação (24/09/2026):** o lote direcionado de editor,
mensagens, perfis e paths executou 27 cenários: 15 PASS e 12 FAIL. Os dois
cenários de paths passaram após o teste aguardar o foco inicial do chat antes
de abrir a decisão; as duas restaurações de foco continuam sendo exigidas.
O menu de contexto já havia passado nos 12 cenários focados após trocar o
evento sintético sem foco pelo pressionamento real de Shift+F10. Editor,
mensagens e perfis ainda estão em correção; não há promoção de critérios.

Na segunda rodada direcionada (editor/mensagens/modal), 20 de 22 cenários
passaram: Alt+3 após F6, cópia simples/Markdown com clipboard real, exclusão
com efeito na lista e os sete cenários do modal. Continuam falhando navegação
de abas a partir do Monaco e recolhimento da thread por ArrowLeft. Perfis
ainda exige rodada própria. `05a28af31` corrige o ciclo de fechamento do modal
de tokens: 75 testes Vitest PASS, sete E2E PASS e revisão independente Franklin
sem achados. `a60d8e040` registra as correções de preparação de foco nos testes
de menu e paths. As mudanças ainda não foram publicadas no PR.

**Editor e navegação revalidados:** os 23 E2E de editor e abas passaram após
reconhecer o controle nativo EditContext do Monaco e ativar a sequência de
leitura ao voltar à aba em modo view. O foco pós-F6 por Alt+3 não grava outra
mudança de modo. A bateria Topbar/apresentação de páginas/navegação passou
371 testes; editor e seus consumidores passaram 222 testes, mais 23 de
EditorContentArea. TypeScript e ESLint focados PASS. Franklin revisou as
exceções; o achado de consumo de tecla durante IME ativo foi corrigido e
retestado antes da consolidação.

O mantenedor aprovou corrigir o layout de Perfis neste PR. `189623599` impede
que a dica de edição ocupe toda a altura e esconda a grade; seis E2E de
listagem/navegação/deep link PASS, além de ativar e duplicar com clique real.
O Ctrl+N inicial de Perfis foi validado no navegador e em 67 testes de
apresentação de página, incluídos nos 371 acima. Ainda há falhas em edição
inline, exclusão confirmada de perfil e navegação de thread; não é aceite
integral nem CI verde. As contagens dos 84 critérios permanecem inalteradas.

**Operações de Perfis revalidadas:** dez E2E de criação, edição, exclusão,
ativação, duplicação e validação passaram em 24/09/2026. O diagnóstico de
edição inline demonstrou dois callbacks no mesmo Enter (salvar seguido de
blur); a sessão da grade agora termina sincronamente antes do callback e da
restauração de foco. A exclusão encontrou uma ligação ausente entre o evento
real `tool:questionnaire` e o registro de diálogos. A factory compartilhada
deriva apenas o scope restritivo de decisão previsto no AEP; não substitui
a autorização do backend nem relaxa as guardas de mutação. TypeScript PASS.
A navegação de mensagens e as revisões dessas correções ainda estão em
validação; esses resultados não representam CI verde nem aceite manual.

Os 13 E2E de operações de mensagens também passaram: exclusão, cópia de texto
e Markdown, edição, raciocínio, menu, navegação e expansão/recolhimento. A
correção preserva a referência da mensagem em alterações apenas estruturais
da árvore, evitando invalidar o foco ao carregar filhos. Não usa igualdade
por serialização de conteúdo nem relaxa a identidade do alvo. A exclusão de
perfil foi repetida com clique real, sem `force` ou chamada DOM de clique:
um cenário PASS. Revisões finais ainda em andamento.

**Consolidação da rodada:** `b207df719` corrige a sessão de edição da grade;
Lagrange revisou e o achado de itens falsy foi corrigido, com 53 testes PASS.
`deaa1420c` corrige a identidade estrutural e o foco de mensagens; Beauvoir
revisou sem achados de implementação após remover a proposta de comparação
JSON. A regressão ampliada passou 281 testes em 11 arquivos; foi acrescentada
uma prova de atualização apenas canônica seguida da projeção visível
(20/20 testes de navegação PASS).

A rodada final de navegador passou 27 cenários: grade (14), questionários
(4, incluindo resposta atrasada de confirmação e cancelamento sem fechar ou
desfocar a próxima decisão), operações de perfis (7) e paths (2). A bateria
de scope/hook/host/restauração de foco passou 43 testes. O P2 de foco tardio
identificado por Franklin foi corrigido com clear por ID e revalidação no
frame, preservando o retorno ao modal de origem. TypeScript, ESLint focado
e `git diff --check` PASS; a revisão final dessa frente precede o push.
Nenhum ACP/acpregistry, Wails dev/build, hardware ou banco pessoal foi usado.

**Publicação e primeira verificação do lote:** `f4c632f64` foi enviado ao PR
#833, que passou a `MERGEABLE` com a main atual. No run `36061552474`, E2E,
bindings e scripts passaram. Frontend executou 6.034 testes: 6.031 PASS e três
falhas em `ChatMessage.test.tsx`, cuja fixture de histórico vazio não expunha
`getConversationMessages`, agora consultado pelo componente real. A fixture
foi alinhada sem mudar produção ou asserções; 58 testes dos três componentes
envolvidos e ESLint PASS. Godel revisou sem achados. Backend/race e nova
verificação remota do ajuste continuam pendentes; não é CI integral verde.

**Timeout do detector de corrida — ajuste autorizado:** a rodada seguinte,
`36062491617` em `b5302960c`, passou backend, frontend, E2E, bindings e scripts.
Somente `backend-race` falhou: `internal/app` atingiu o timeout padrão de Go
de dez minutos (600,182 s); o subteste em curso tinha quatro segundos.
O mantenedor aprovou testar `go test -race -short -timeout=20m ./...` antes
de dividir em grupos. Preservados todos os testes, `-race`, `-short` e o
limite total de 25 minutos do job. O timeout não foi desativado; o novo valor
permanece sujeito à comprovação no CI, sem promover critérios ou aceite.

**Divisão autorizada do detector de corrida:** no run `36065536526`,
`551c5e7ba` passou backend, frontend, E2E, bindings e scripts, mas App atingiu
novamente o timeout (1200,201 s). O subteste corrente tinha dois segundos;
o limite era acumulado do pacote, não o teto de 25 minutos do job.
Após concordância do mantenedor, App passa a oito grupos descobertos pelo
próprio Go, com distribuição determinística dos testes, exemplos e sementes
de fuzz. Os demais pacotes permanecem inteiros em um grupo separado.
Todos conservam `-race -short`, com `-count=1` e timeout de 20 minutos.
O check agregado `backend-race` mantém seu nome e exige sucesso de todos
os grupos; falha, cancelamento ou grupo ignorado não produzem aprovação.
A configuração não altera critérios de aceite nem substitui validação manual.

Validação local do script: PASS com Go simulado, verificando união completa e
disjunção dos oito grupos, nomes Unicode, exemplos/fuzz, flags, exclusão exata
de App e propagação de falhas de descoberta/execução. Sintaxe Bash, parse do
YAML e gate agregado conferidos. Nenhum Go, ACP, Wails ou banco real executado
localmente nesta mudança; a execução real dos grupos depende do próximo CI.
Revisão independente de Godel: nenhum achado bloqueante. Incorporada sua
observação preventiva para descobrir os demais pacotes com `go list -race`,
incluindo eventuais pacotes futuros condicionados à build tag `race`.

A execução real descobriu 1.271 testes top-level em App. O CI de scripts
apontou SC2251 na asserção negativa do mock; corrigida com falha explícita,
revalidada localmente e aprovada por Franklin. No run `36069643187`, o grupo
3 revelou dependência de ordem em `TestListProvidersWithStatus`: o teste
não preparava banco próprio e ignorava os erros de criação dos provedores.
Adicionado setup independente, verificação dos erros e cleanup do helper
(restauração do DB anterior e fechamento da conexão). Cinco testes de CRUD
PASS localmente; o caso original também PASS executado sozinho, sem cache.
Franklin revisou o diff e os callers sem achados de isolamento. Não foram
executados localmente testes de ACP, Wails nem acesso a banco pessoal.

O grupo 6 revelou `SQLITE_BUSY` na criação real de tasklist com jobs ativos.
A criação passa a usar a transação `IMMEDIATE` e o retry limitado já existentes,
com contagem do limite e verificação de slug sob o writer lock. Não foi adicionado
retry no teste nem reduzida a concorrência do fixture. Os dois testes negativos
de proveniência/sessão passaram cinco vezes localmente após a correção.

Por solicitação do mantenedor, os grupos numéricos foram substituídos por
domínios: configuração, dispositivos, contexto, execução, jobs, interface,
segurança, outros comandos, chat e demais testes de App; outros pacotes ficam
em `pacotes-gerais`. A classificação é total, determinística e mantém grupos
residuais para novos nomes. O mock verifica a união completa e disjunta; a matriz
foi conferida contra todos os grupos. Essas alterações aguardam a próxima
execução integral do CI, sem promoção de aceite manual.
Regressões com oito writers em WAL verificam a última vaga do limite e a
unicidade de slug, incluindo contagens finais de listas/workflows. Esses testes
e os casos existentes de criação/rollback passaram em cinco repetições locais.

## 159. Aceite manual e separação dos próximos PRs — 25/09/2026

Registro das decisões do mantenedor após a primeira rodada do checklist.
São pendências e decisões de escopo, não funcionalidades já implementadas.
O PR atual é #833; seu merge continua sendo decisão do mantenedor e não
representa, sozinho, conclusão do AEP. Não ampliar esse PR com as melhorias abaixo.

### Bloqueios e verificações do PR atual

- [ ] Investigar e corrigir a perda de navegação após a primeira ação.
  Relato: Alt+C e outros destinos (Jobs/Histórico) deixam de responder após
  uma ação. No Stream Deck, tecla 1→aba 1 funciona inicialmente; depois de
  Ctrl+Tab→aba 2, tecla 1 não volta. Duas teclas apontando às abas 1 e 2
  também bloqueiam uma à outra: a primeira utilizada funciona e a seguinte
  não. Sair da janela e voltar recupera o funcionamento no cenário relatado.
  Atualização de contexto/foco é hipótese, não diagnóstico confirmado.
- [ ] Cobrir a causa com regressão automatizada e validar a sequência real
  teclado→Deck, Deck→Deck e navegação entre páginas, sem exigir Alt+Tab como
  contorno. Preservar barreiras de modal, IME, sessão e autorização.
- [ ] Corrigir regressão de foco em conversa vazia. Relato do mantenedor:
  com foco no campo de mensagem e nenhuma mensagem na conversa, pressionar
  Seta para cima transfere o foco para a região vazia de mensagens; é preciso
  Escape ou Tab para retornar. Antes, essa transferência não ocorria.
  Esperado: sem mensagem navegável, Seta para cima mantém o foco no campo,
  preservando a edição e a movimentação normal do cursor. Não inferir que um
  comando explícito de focar a região vazia deva ser proibido: o defeito é a
  transferência por esse gesto do campo. Cobrir conversa nova e conversa
  após limpeza, preservar navegação quando há mensagens e a prioridade de
  menus/pickers do campo. Revalidar com teclado/NVDA; causa ainda não investigada.
- [ ] Investigar latência percebida na navegação pelo Stream Deck. Confirmar
  que trocar abas, mover foco e abrir páginas usam o caminho local sem auditoria
  persistida por pressão, independentemente da origem. Medir recebimento,
  resolução e entrega à UI antes de atribuir o atraso ao banco. Preservar
  verificações de sessão, contexto e modais; não remover proteção para acelerar.
  Relato de atraso não é evidência de que o caminho esteja gravando auditoria.
- [ ] Após a correção, solicitar revalidação curta ao mantenedor, atualizar
  evidências e conferir CI/review antes de recomendar merge. Sem merge automático.

### PR independente — ativação por página do aplicativo

- [ ] Acrescentar condição de ativação da camada por página: Workspace,
  Configurações, Jobs, Histórico, Perfis e demais páginas suportadas do app.
  Definir a lista a partir das rotas reais, sem nomes técnicos expostos.
- [ ] Distinguir página ativa, tipo de aba, aba específica e foco na barra
  de abas. Estar na página Workspace não exige foco na barra; trocar de aba
  dentro dela não desativa uma camada condicionada somente à página.
- [ ] Atualizar elegibilidade e apresentação do Deck automaticamente ao
  navegar, sem ativação manual. Combinar com as demais condições e preservar
  restrições do comando: camada ativa não autoriza execução atrás de modal.

### PR independente — destino de aba sem teto arbitrário

- [ ] Oferecer ação parametrizada **Ir para aba**, com destino por posição
  inteira positiva, sem limite fixo de 9, 32 ou 64. Não criar uma entrada de
  catálogo/paleta para cada número. Exibir nomes claros: Aba 1, Aba 2 etc.
- [ ] Posição inexistente fica indisponível: não criar aba nem escolher outra.
  Posição acompanha reordenação; manter Ctrl+1…9 como atalhos padrão não limita
  as posições configuráveis no comando.
- [ ] Oferecer alternativa **Aba específica**, selecionada pelo nome, que
  mantém a identidade ao reordenar. Aba fechada fica indisponível, sem trocar
  silenciosamente de destino. Especificar o comportamento ao trocar workspace.

### PR dependente do destino de aba — apresentação automática no Stream Deck

- [ ] Usar por padrão o nome atual da aba-alvo, abreviado apenas para caber,
  e ícone do tipo: chat, editor, terminal ou lista de tarefas.
- [ ] Acompanhar renomeação, reordenação, fechamento e troca de workspace,
  respeitando a diferença entre posição e identidade de aba.
- [ ] Título/ícone personalizados permanecem opcionais; não exigir que o
  usuário preencha informações já disponíveis. Indicar destino indisponível
  sem manter apresentação enganosa nem executar em outra aba.

### PR próprio — simplificação dos gerenciadores de configuração

- [x] Conferir e reutilizar o padrão existente nas opções/configurações das
  listas de tarefas, incluindo gerenciadores de workflows e ações personalizadas.
- [x] Tela principal centrada em uma lista de camadas, estado ativo/inativo,
  resumo de por que/quando está ativa e ações básicas de criar/renomear/excluir.
- [x] Menu **Configurações da camada** abre gerenciadores separados:
  **Comandos e acionadores** e **Regras de ativação**, cada um em seu modal,
  com grid e operações próprias. Preferência aprovada: separados, não painel
  de guias reunindo novamente os dois gerenciadores.
- [ ] Manter opções avançadas nos formulários correspondentes, navegação
  consistente por teclado/NVDA e retorno de foco à camada ao fechar.
  Implementação e regressão automatizada concluídas na seção 163; aceite
  com NVDA permanece manual, sem presumir aprovação pelo teste de navegador.

Condição de página e destino de aba podem avançar em paralelo após combinar
contratos. Apresentação automática depende do destino de aba. Reorganização
visual pode avançar em paralelo com coordenação sobre formulários compartilhados.
Antes de implementar cada extensão, atualizar seus contratos no AEP/inventário
e definir testes/gate do PR; não declarar implementação a partir deste registro.

### Rodadas manuais e resultados relatados

O [checklist consolidado](../docs/content/guias/VALIDACAO_MANUAL_COMANDOS.md)
continua com 48 casos. Ao conversar com o mantenedor, apresentar somente
**1 a 5 por rodada**, com passos curtos, mantendo IDs técnicos neste registro.
Corrigir ao final e reavaliar falhas/caminhos afetados, salvo o bloqueio básico
acima, cuja correção foi antecipada. A rodada de testes está pausada por ele.

- Primeiro lote: UI01 e UI02 passaram; UI03 não executado por falta de clareza
  para favoritar; UI04 passou inicialmente, mas foi reaberto pela falha de
  navegação; UI05 inconclusivo, repetir no final.
- Segundo lote (1–5): UI06, CF01, CF02 e CF05 receberam “ok”; CF03 adiado.
  Esses relatos cobrem os passos enviados, não variantes omitidas dos casos
  extensos. CF02 recebeu confirmação de gravação; confirmar o ingresso usado,
  pois o mantenedor relatou ter preferido capturar uma tecla do Deck.
- Relato adicional: SD04 falhou na continuidade de navegação descrita acima.
  Captura e primeira execução do Deck funcionaram após ativação manual; isso
  não aprova automaticamente todas as variantes de SD01/SD04.
- Terceiro lote não executado: CF04, CF06, CF07, CF08 e parte de CA02.
  Nenhum deve receber PASS por ausência de relato de falha.

Correção do placar conversacional: ao reabrir UI04, a conta antes informada
como 7 aprovados / 2 falhas / 39 pendentes o contava duas vezes. O registro
sem duplicação é **6 casos com aprovação relatada / 2 com falha / 40 pendentes**.
Isso não é aceite integral de todas as variantes nem altera C01–C84. Na retomada,
explicitar as variantes faltantes sem exigir repetição do que já foi observado.

## 160. Correções dos achados manuais antes do merge — 25/09/2026

Escopo acordado: corrigir regressões deste PR; manter as extensões da seção159
para PRs menores após o merge. Não houve mudança de contrato de autorização,
remoção de auditoria de ações persistentes ou aceite manual presumido.

- [x] Reproduzir em teste a navegação que para após o controle anterior perder
  foco. A transição de página remove o controle; a de aba oculta o painel e
  executa blur. O foco passa ao documento, mas o dispatcher exigia um controle
  sobrevivente até para abrir páginas/trocar abas. Ambos os testes falharam
  antes da correção e passaram depois dela.
- [x] Permitir exclusivamente a navegação local com foco no documento,
  preservando janela ativa, modal, IME, dono/sessão, mapa vigente e validação
  específica do alvo. Testes cobrem teclado→página, Deck→Deck, Ctrl+Tab→Deck
  e recusas por modal, janela sem foco, sessão, geração e validade do evento.
- [x] Restaurar foco de destino quando a nova navegação começa no documento;
  não roubar foco se o usuário selecionar outro controle durante a transição.
- [x] No campo de chat, consumir Seta para cima somente após focar uma mensagem
  existente. Conversa nova e conversa limpa não enviam foco à região vazia.
  Adaptar o outro consumidor de ChatInput, o terminal, ao mesmo contrato
  booleano e cobrir histórico vazio, limpo e tentativa de foco malsucedida.
- [x] Investigar o caminho de navegação do Deck: não passa pelo ledger.
  `TestCommandDeckAppLatency` passou em fixture temporária, incluindo a
  asserção de zero invocações persistidas de `navigation.settings.open`.
  Windows/Go 1.26.2, 10 aquecimentos e 100 amostras por cenário: input→emissão
  estável p95 0,526 ms / p99 1,011 ms; após mutação de camada p95 1,011 ms /
  p99 1,149 ms. Mutação e rebuild da camada (fora do caminho estável por tecla) p95
  96,533 ms. Amostras zeradas são inferiores à resolução do relógio, não
  execução instantânea. A revalidação de sessão permanece obrigatória.
  Execução focada desta rodada: `$env:COMMAND_APP_LATENCY='1'; go test
  ./internal/app -run '^TestCommandDeckAppLatency$' -count=1 -v`, encerrada
  com PASS em 11,403 s. Os logs integrais históricos que registram SKIP não
  são a evidência desta execução opt-in.
- [x] Revisão estática adicional do runtime: leitura de tecla é orientada a
  eventos; não espera o ticker de um segundo. Escrita HID e dispatch partilham
  mutex, de modo que renderização condicional pode atrasar uma tecla. Não há
  reenvio de imagens em cada tick estável. Não alterar esse contrato sem prova
  e testes de concorrência: esta análise não demonstra a causa da demora física.
- [x] Regressão frontend: 22 arquivos / **1.138 testes PASS**, incluindo todos
  os testes Topbar, navegação de abas, ChatInput, ChatSessionView e TerminalPage.
  TypeScript isolado e ESLint dos dez arquivos alterados passaram.
- [x] Revisor independente Lagrange: primeira rodada apontou o consumidor
  TerminalPage incompatível com o callback booleano; corrigido por Beauvoir.
  Segunda rodada sem pendências funcionais. Rótulo de teste `owner` ajustado
  para `session`, pois esse caso altera a sessão. Revisão não substitui testes.
- [ ] Repetir no app físico a sequência de páginas e de abas pelo teclado/Deck,
  sem Alt+Tab intermediário; conferir foco/NVDA em conversa vazia e populada.
- [ ] Reavaliar latência percebida no dispositivo. A medição acima termina
  na emissão do evento: não mede USB, transporte Wails nem renderização visual.
  Portanto não é evidência de que o atraso físico relatado foi resolvido.

Não há promoção de UI04/SD04 a PASS nem alteração de C01–C84. CI remoto e
review do novo commit devem ser conferidos antes de recomendar merge, que
permanece decisão do mantenedor. As pendências manuais da seção159 continuam.

## 161. Subdivisão do grupo de contexto no detector de corrida — 25/09/2026

A rodada `36133053604`, commit `a62642ad7`, passou frontend, E2E, bindings,
scripts, backend principal e dez dos onze grupos race. `comandos-contexto`
atingiu o limite acumulado do Go de 20 minutos (1200,137 s; job de 22m03s).
No encerramento, `TestContextualPagePaletteProfileOperations` rodava havia
8 s e o subteste `delete` havia 3 s. Não houve asserção falha nem alerta
DATA RACE antes do timeout. Copilot revisou o commit sem novos achados.

Com autorização explícita do mantenedor, o grupo foi dividido por domínio:

- `comandos-contexto-deck`: `TestContextualDeck*` (27 testes no inventário do CI).
- `comandos-contexto-paleta`: `TestContextualPalette*`, `TestContextualPagePalette*`
  e `TestContextualLayerPalette*` (26).
- `comandos-contexto-workspace`: `TestCommandWorkspace*` (52).
- `comandos-contexto-base`: famílias restantes de contexto, perfil e escopo (11).

Os 116 testes do grupo anterior permanecem representados; contagem não é
estimativa de duração, pois subtestes e custos de preparação variam. A medição
real dos novos grupos é responsabilidade da próxima rodada do CI.
Há agora 14 grupos ao todo (13 de App e um dos demais pacotes), com descoberta
automática de testes, exemplos e fuzz targets. Famílias específicas precedem
o residual; nomes futuros não são descartados. Subtestes ficam com o teste pai.

Não foram alterados `-race -short -count=1 -timeout=20m`, o teto de 25 minutos
por job, `max-parallel: 4`, `fail-fast: false` nem o agregador obrigatório
`backend-race`, que só aprova se todos os grupos passarem. Nenhum teste foi
removido ou marcado para pular. Não há mudança no backend produtivo, nas
correções de navegação/foco ou no aceite humano. AEP continua **In Progress**,
83 I / 1 P; revalidação manual e latência física continuam pendentes.

Validação local: teste do script com Go simulado PASS (atribuição exata por
família, residual futuro, nomes Unicode, matriz sem grupos ausentes/duplicados,
flags e propagação de falhas). Sintaxe Bash, parsing YAML, 14 grupos únicos,
limites/dependência do agregador e `git diff --check` conferidos. Nenhum Go
real/Wails foi executado para essa validação. Beauvoir implementou os testes;
Lagrange revisou o diff final sem achados. ShellCheck não está instalado
localmente e permanece no job de scripts do CI. Resultado remoto ainda deve
ser confirmado no commit publicado.

## 162. Diagnóstico nativo e continuidade do Deck — 25/09/2026

O relato de primeira tecla funcionar e as seguintes dependerem de Alt+Tab
foi reproduzido no Wails/WebView2 com frontend/backend reais, banco sintético
e driver HID simulado. A troca de aba envelhecia o guard de projeção de jobs.
O preview inicial/periódico encerrava a época antes de revalidá-la; no caso
concorrente, cancelava a atualização iniciada pelo pressionamento. Além disso,
a publicação equivalente substituía a identidade da configuração e notificava
o frontend para descartar o mapa que continuava válido.

- Refresh antes da entrada física e dos previews inicial e periódico.
- Configuração totalmente idêntica conserva o ponteiro, mas recebe o guard novo.
- Renovação de deadline pode trocar o snapshot sem mudar versões; alterações
  efetivas, registry, sessão e revogações continuam invalidando a resolução.
- Notificação é dispensada somente para mapa vivo, não expirado e exatamente
  correspondente ao snapshot e às versões atuais.
- Validação nativa: **18/18 trocas** entre tarefas, editor e chat em duas
  sequências, com intervalos de 500 ms e 300 ms, sem Alt+Tab/refoco manual.
  É prova do caminho integrado com HID simulado, não aceite do hardware/NVDA.
- Regressão `TestCommandDeck*`: PASS (65,721 s); pacote `commandexecution`:
  PASS (34,498 s); frontend de foco: 17/17 PASS; ESLint dos testes e `go vet`
  de App/commandexecution/commanddeck: PASS. O exit 1 de uma execução anterior
  de Deck não teve causa recuperável no output truncado; a repetição completa
  passou, sem apagar testes nem declarar a primeira execução bem-sucedida.
- Revisor independente Beauvoir: sem achados bloqueantes na produção;
  solicitou reforço de regressão do polling e dos mapas inválidos.
- Reforço concluído: o teste integrado cobre Input com guard stale e o
  polling posterior à troca mantendo o mesmo handle, sem reconectar; os
  testes de mapa cobrem ausência, cancelamento, expiração, deadline futuro e
  ponteiro equivalente distinto. Execução conjunta PASS (21,139 s).
- Reexecução final de `TestCommandDeck*` junto à projeção de teclado:
  PASS (44,350 s). Beauvoir revisou também o reforço final sem bloqueios.

Executáveis, cache e temporários do diagnóstico ficaram em `work/native-deck`
no worktree, fora do Temp do Windows. Credential Manager e perfil real não
foram utilizados; a instância diagnóstica foi encerrada. Nenhum teste ACP
foi executado. O artefato exploratório de E2E foi preservado em `work/`, fora
da suíte publicada: sua expectativa de foco exclusivamente interno ao editor
não representava o fallback autorizado ao botão de aba. Nenhum teste
preexistente foi removido. CI do novo commit e confirmação física permanecem
separados desta evidência. **In Progress, 83 I / 1 P / 0 N**.

## 163. Pós-merge: gerenciadores separados — 25/09/2026

O mantenedor confirmou que a navegação física do Stream Deck voltou a funcionar
e informou o merge do PR #833. Merge confirmado em
`b4046d953cb9e994b3bc04da127717cd47f8e899`; esse aceite fecha o relato de
continuidade do Deck da seção 162, não os demais casos manuais nem uma medição
de latência que não foi fornecida.

Primeiro PR independente: `feat/command-settings-managers`, baseado na main
já integrada. A página conserva a lista de camadas, resumo e ações básicas;
**Configurações da camada** abre **Comandos e acionadores** ou **Regras de
ativação** em modais separados, reutilizando MenuButton, Modal e DataGrid.
Os formulários, opções avançadas, permissões, escopos e contratos de mutação
são preservados. Não há migração de banco nem alteração do executor/backend.

- [x] Abertura independente de cada gerenciador, sem os dois grids na página.
- [x] Escape fecha primeiro o formulário e depois o gerenciador; foco retorna
  ao grid e à camada, respectivamente. Lista vazia retorna à ação de criação.
- [x] Falha de recarga invalida o gerenciador e foca Recarregar; recuperação
  não reabre conteúdo antigo. Identidade e escopo continuam invalidando editores.
- [x] Correção mínima do ciclo de montagem do DataGrid: StrictMode reativa a
  referência de montagem; o cleanup continua impedindo foco após unmount.
- [x] Playwright: 3/3 cenários com frontend real e ponte Wails simulada;
  regressão de StrictMode: 2/2 casos. Não constituem teste nativo ou NVDA.
- [ ] Aceite manual de navegação/anúncios no NVDA, incluindo camada pessoal,
  padrão e herdada; integrado ao UI01 do checklist, sem criar outra rodada.

A revisão independente de Beauvoir identificou a perda de foco ao falhar a
recarga; corrigida e coberta pelo cenário de recuperação acima. Condição de
página, destino de aba sem teto e apresentação automática no Deck continuam
nos PRs separados previstos na seção 159. **In Progress, 83 I / 1 P / 0 N**.

Validação local: 18/18 E2E de configurações; build frontend, TypeScript,
ESLint e Stylelint sem erros (avisos preexistentes no lint geral). A suíte
completa executou 6.062 testes: 6.061 passaram e a auditoria de live regions
detectou dois roles locais novos no gerenciador. Os roles foram removidos,
preservando `announce()` global já existente para erros e sucesso. Reexecução
da auditoria junto à página e ao DataGrid: **154/154 PASS**, incluindo axe dos
dois gerenciadores. A rodada completa anterior não é registrada como PASS;
o CI valida o conjunto final. Beauvoir revisou a correção e encerrou sem
bloqueios, após as rodadas de foco, documentação e arbitragem de anúncios.

PR #834: a primeira rodada remota passou frontend e E2E. Copilot identificou
ausência de `dialog.ruleTitle` em espanhol; incluída tradução explícita e
regressão das chaves dos gerenciadores nos três locales, sem fallback de
idioma, incluindo os placeholders das contagens. Os checks finais continuam
associados ao commit atualizado, não ao resultado da rodada anterior.

Ajuste solicitado pelo mantenedor durante a revisão: **Editar camada** e
**Configurações da camada** ficam na toolbar junto de **Nova camada**. O
checkbox de consentimento da API externa também foi confirmado para essa
barra; estado e autorização permanecem no componente de conexão, sem criar
fluxo alternativo. Explicação acessível e botão explícito de criar convite
permanecem na seção correspondente. Edição pela toolbar e pelo menu da linha
compartilham o mesmo handler e as restrições de camada padrão/herdada.

Regressão do complemento: **454/454 Vitest** (44 arquivos, incluindo os
consumidores de Toolbar, página, conexão externa e auditoria de anúncios),
**19/19 E2E de configurações**, TypeScript, ESLint e Stylelint PASS. A revisão
independente pediu que o checkbox integrasse o roving tabindex: implementado
no hook compartilhado, com setas/Home/End sem alteração de consentimento e
Espaço nativo. O roteiro manual foi ajustado: Tab entra na toolbar, setas
selecionam o controle. Beauvoir revisou a produção sem novo bloqueio.

## 164. Contrato do comando parametrizado de destino de aba — 25/09/2026

**Implementado; validação técnica focada concluída.** Esta frente independente
da seção 159 não inclui condição de página nem apresentação automática no
Stream Deck. Não representa aceite manual nem fechamento dos gates AEP.

- ID único: `workspace.tab.go_to`; não criar IDs por número nem catálogo de
  posições.
- Argumentos persistidos no binding: `workspace_id`, `target_mode` e o campo
  do modo selecionado. O formato é
  `{"workspace_id":"…","target_mode":"position","position":N}` ou
  `{"workspace_id":"…","target_mode":"specific","tab_id":"…"}`;
  exatamente um de `position` (inteiro positivo sem teto) e `tab_id` (ID
  estável) acompanha o modo. A interface apresenta **Por posição** e **Aba
  específica**; esta mostra nomes, mas salva o ID. O binding fica vinculado
  ao workspace em que foi configurado.
- Execução revalida workspace, autenticação/sessão, foco/modal/IME e lease
  local. Posição resolve a ordem viva; destino ausente ou aba fechada falha
  fechado, sem criar, redirecionar ou escolher outra aba. Com outro workspace
  ativo, o binding fica indisponível e não é retargetado; volta a valer apenas
  no workspace ao qual permanece vinculado.
- Os comandos existentes `workspace.tab.first`…`ninth` e seus defaults
  `Ctrl+1…9` permanecem inalterados. O comando novo compartilha o dispatcher
  local protegido; não altera ativação por página nem apresentação automática
  do Deck.
- Evidência: `internal/app/app_command_workspace_chat.go` registra o único ID,
  schema, política de persistência e origens; `internal/commandconfig/projection_complete.go`
  valida modo/alvo como regra de configuração (além do schema); `app_command_keyboard.go`,
  `app_command_palette_conditions.go` e `app_command_deck_conditions.go` projetam
  argumentos condicionais por ramo sem compartilhar payload mutável; `CommandSettingsPage.tsx` e
  `CommandWorkspaceTabTargetFields.tsx` editam o destino; o dispatcher local
  revalida alvo, ordem, sessão e lease em
  `commandWorkspaceTabNavigation.ts`/`Topbar.tsx`. Testes de regressão ficam
  em `app_command_workspace_tab_target_test.go`,
  `internal/commandconfig/projection_complete_test.go`,
  `commandWorkspaceTabNavigation.test.ts`, `commandLocalKeyboard.test.ts`,
  `commandLocalPaletteConditions.test.ts`, `commandLocalDeckConditions.test.ts`,
  `CommandWorkspaceTabTargetFields.test.tsx` e
  `Topbar.deckSurfaceFocus.integration.test.tsx`.
- Validação executada: `go test ./internal/app ./internal/commandconfig -run
  '^TestWorkspaceTabGoTo'` — rodada final `EXIT_CODE=0` (`internal/app` 20,577 s;
  `commandconfig` cached; log `work/logs/tab-target-go-focused-final.log`). As
  rodadas anteriores `tab-target-go-focused-r2.log` e `-r3.log` terminaram com
  `EXIT_CODE=1`; `tab-target-go-focused.log` também falhou no cleanup ao tentar
  remover `app.test.exe` ainda em uso. Não são a evidência de aprovação. Main
  confirmou `wails generate module` e `tsc --noEmit` pós-geração com `exit 0`;
  Vitest focado (**26/26**) e `git diff --check` também PASS. Os testes cobrem alvos distintos
  por ramo na projeção Palette/Deck e despacho Deck, além dos modos inválidos
  na fronteira de configuração. Aceite NVDA, interação física com Stream Deck e
  apresentação automática continuam fora do escopo/pendentes.

Revisão local independente: Beauvoir revisou o diff completo, incluindo os
arquivos novos. A primeira rodada encontrou perdas de argumentos na paleta
e no Deck condicional, validação incompleta do modo/alvo e problemas de
anúncio/documentação. Corrigidos com regressões; a segunda rodada não
identificou bloqueios funcionais. Bindings foram regenerados oficialmente,
e o TypeScript pós-geração passou. Base atualizada para `origin/main`
`d32fc990f`, sem conflitos nem alteração nas mudanças desta frente.

## 165. Apresentação automática de destinos de aba no Stream Deck — 25/09/2026

Implementação da continuação visual da seção159, sobre a base do contrato de
destino da seção164. O mapa de apresentação do Deck consulta uma cópia do
workspace ativo; isso não muda resolução, autorização nem despacho. `Ir para
aba` resolve a apresentação por posição viva ou ID estável; `primeira` a `nona
aba` acompanham as posições atuais. Renomear, reordenar, fechar ou trocar o
workspace é refletido na próxima atualização de render.

O título automático e o ícone local de tipo são fallbacks campo a campo. Título,
ícone e imagem configurados permanecem intactos, incluindo herança e overrides
por estado; título personalizado continua visível com um sufixo localizado de
indisponibilidade se o destino sumir. Posição/ID ausente, workspace diferente,
ou qualquer ramo condicional selecionável sem alvo mostra indisponibilidade,
sem usar a aba ativa ou outra posição como substituta. Se ramos condicionais
válidos apontarem para destinos diferentes, a apresentação diz que o destino
depende do contexto. Não há evento de tecla, execução, confirmação ou auditoria
adicionados para renderizar esses dados.

Ícones de conversa, editor, terminal e lista de tarefas usam formas locais do
renderer existente; nenhuma imagem, arquivo, fonte externa ou conteúdo da aba é
carregado. O guia de comandos documenta o comportamento. Evidências focadas em
`internal/app/app_command_deck_tab_visual_test.go` cobrem posição e identidade
após rename/reorder, destinos ausentes/workspace alheio, `first`/`second`/`ninth`,
ramo condicional, divergência, ícones rasterizados e preservação dos campos
base/estado. Os sete testes focados selecionados por
`go test ./internal/app -run '^TestWorkspaceTabDeck' -count=1` passaram.
`go build ./...` e `go vet ./...` também passaram. Uma tentativa do teste
integrado existente `TestCommandDeckPresentationSettingsReachContextualMapAndRenderer`
parou antes do `deckMap`: `settingsSecurityFixture` falhou ao montar o produto
App com `configuração de executor inválida`. O teste não relacionado
`TestCommandProductRefusesUnknownInvalidAndRevokedRequests` reproduziu a mesma
falha no `readyCommandProduct` (linha76 de `app_command_product_test.go`),
confirmando que a limitação ocorre na fixture/bootstrap comum, não nesta prova
visual. Portanto, a integração App não é declarada aprovada. Não se afirma
aceite físico/NVDA nem fechamento de critério/gate manual.

Revisão independente de Lagrange e do agente principal identificou dois casos
visuais: ramos mistos ocultavam a combinação de comandos, e títulos
personalizados ocultavam a ambiguidade do destino. Corrigidos preservando o
título composto/personalizado e acrescentando a informação de destino como
sufixo; regressões focadas passaram. O agente principal reavaliou o patch
sem novos bloqueios funcionais. A atualização do AEP principal acompanha a
tasklist e o índice, sem promover aceite manual.
