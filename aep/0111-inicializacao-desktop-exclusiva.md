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
  sem abrir SQLite; fixar esse caminho para todo o startup desktop.
- Reservar a instância antes de construir/inicializar os serviços.
- Solicitação de ativação não executa comandos, não recebe argumentos de ação
  e não muda usuário, sessão, autorização ou contexto da primeira instância.
- Se a janela ainda não estiver pronta, reter uma solicitação de apresentação.
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
  continua necessário. O reset no mesmo caminho mantém a reserva desktop.

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
- [x] Testes automatizados e revisão independente sem pendências (Averroes/Luna,
  duas rodadas; corrigida liberação antecipada no shutdown). Build, vet e lint
  passaram. As suítes emitiram PASS; duas execuções locais tiveram erro posterior
  ao remover o executável de teste por arquivo em uso no Windows, sem causa
  confirmada. Nenhum executável do app ou teste ACP foi iniciado.
- [ ] Validação manual de foco/restauração da janela e NVDA.
