# AEP-0111 — Inicialização desktop exclusiva por banco

**Status:** In Progress

## Resumo

Uma segunda abertura do desktop que selecione o mesmo banco não inicializa
SQLite, migrações, jobs, comandos ou dispositivos. Solicita à primeira janela
que apareça e encerra normalmente. Nunca encerra a instância anterior à força.

## Motivação

Múltiplos processos chegaram à interface antes de descobrir a disputa pelo
lock de comandos. O usuário via uma janela aparentemente pronta, mas sem ações.
A exclusividade precisa anteceder os serviços; não substitui o tratamento de
contenção interna de uma única instância (AEP-0074 e AEP-0106).

## Decisões

- Resolver o banco pela precedência existente (workdir > home > executável),
  sem abrir SQLite; fixar o caminho canônico devolvido pela reserva para todo o
  startup desktop e para reset/reabertura, sem redescobrir o arquivo por config.json.
- Reservar a instância antes de construir/inicializar os serviços.
- Solicitação de ativação não executa comandos, não recebe argumentos de ação
  e não muda usuário, sessão, autorização ou contexto da primeira instância.
- Se a janela ainda não estiver pronta, reter uma solicitação de apresentação.
- Criar o worker de apresentação antes do runtime; cancelar e aguardar seu término
  antes do shutdown dos serviços. A espera cobre chamadas Go em andamento, não
  confirma a execução de mensagens já enfileiradas na UI nativa.
- Falhas de reserva/notificação não autorizam abrir uma instância concorrente.
  Comunicar o problema no mecanismo nativo de erro, nos três idiomas.
- Manter a reserva até terminar o encerramento. Não remover locks de processos
  vivos; o sistema operacional deve liberar a posse após a morte do processo.
  Como Shutdown pode retornar preservando serviços que não drenaram, depois de
  entrar no runtime Wails a reserva permanece viva até `os.Exit`, inclusive em
  falha. Isso também cobre um callback de startup ainda não agendado.
- Bancos separados podem ter desktops separados. Posse de Stream Deck e
  atalhos globais permanece sob os mecanismos existentes do AEP-0103.
- CLI e versões antigas não ganham exclusividade retroativamente. O lock de
  comandos existente permanece como defesa adicional.
- Reserva usa sidecar estável ao lado do banco, com lock nativo mantido durante
  o processo. Hardlinks existentes também usam identidade física no cache do
  usuário. Os sidecars persistem, mas a posse não; não guardar PIDs como prova.
- Notificação local usa loopback numérico, token aleatório por abertura e prazo
  limitado. O único efeito admitido é enfileirar apresentação da janela.
- Redes de arquivos, remoção maliciosa de sidecars e novos hardlinks criados
  durante a execução não fazem parte da garantia. O lock físico de comandos
  continua necessário. O reset, após drenagem e fechamento do SQLite, trunca
  o arquivo sem removê-lo: preserva a identidade física e os hardlinks existentes,
  além da reserva desktop. A limpeza de WAL/SHM e a reabertura usam o caminho fixado.
  Antes de fechar SQLite, o reset rejeita symlink/arquivo não regular por `Lstat`
  e compara a identidade com `f.Stat` após abrir sem truncamento; trunca somente
  o handle validado. Substituição hostil posterior do pathname (inclusive antes
  da reabertura) não está coberta por esta garantia local.
  `TestSettingsControllerResetDatabaseUsesFixedDatabasePath` verifica `os.SameFile`,
  remoção dos dados antigos e ativação da instância reservada pelo hardlink após reset.

## Fases

1. Resolução estável do banco e reserva antecipada.
2. Ativação da janela existente, diagnóstico e encerramento seguro.
3. Testes automatizados de concorrência, reabertura e falhas; validação física.

## Riscos

- A primeira janela pode não estar responsiva: a segunda não pode esperar
  indefinidamente nem iniciar serviços como fallback.
- Nomes alternativos do mesmo arquivo e reset do banco exigem atenção na
  identidade e no tempo de vida da reserva.
- Instância única não corrige contenção entre goroutines do mesmo processo.

## Critérios de aceitação

- [x] Segunda abertura não inicializa serviços nem SQLite (testes `TestRunSecondInstanceDoesNotStartDesktop` e `TestResolvePath*`).
- [x] Solicitação durante startup é enfileirada para atendimento após prontidão (`TestDesktopActivationQueuesBeforeReadyAndStops`).
- [x] Bancos distintos são independentes (`TestDifferentDatabasesAreIndependent`).
- [x] Falha de notificação não libera abertura concorrente (`TestSilentEndpointFailsClosedWithinDeadline` e teste do entrypoint).
- [x] Encerramento libera a reserva; reabertura funciona (`TestContentionActivationReopenAndUnchangedDatabase`).
- [x] Caminho canônico reservado permanece fixo mesmo após troca de symlink
  (`TestGuardCanonicalPathSurvivesSymlinkRetarget`); reset pela composição do App
  reabre esse arquivo, preservando o banco alternativo da resolução de configuração
  (`TestWireSettingsResetUsesDesktopDatabasePath`). O teste de symlink requer
  privilégio não disponível no Windows local e roda no CI Linux; os testes de
  reset/composição passaram localmente com bancos descartáveis.
- [x] Encerramento aguarda o worker Go de ativação e descarta pedidos antes da
  prontidão (`TestDesktopActivationStopDrainsInFlightCall` e
  `TestDesktopActivationShutdownBeforeReadyDiscardsRequests`).
- [x] Testes automatizados e revisão independente sem pendências (Averroes/Luna,
  duas rodadas; corrigida liberação antecipada no shutdown). Build, vet e lint
  passaram. As suítes emitiram PASS; duas execuções locais tiveram erro posterior
  ao remover o executável de teste por arquivo em uso no Windows, sem causa
  confirmada. Nenhum executável do app ou teste ACP foi iniciado.
  A revisão incremental de caminho canônico, reset e worker (Averroes e Noether,
  Luna) corrigiu restauração no Linux, wiring e isolamento das fixtures; rodada
  final sem achados. Testes locais de raiz, guard, database, controller e App
  passaram; o helper existente de fixture agora permite restaurar DB e caminho.
- [ ] Validação manual de foco/restauração da janela e NVDA.
