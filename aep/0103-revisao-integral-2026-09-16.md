# AEP-0103 — Revisão integral em 16/09/2026

**Fotografia histórica:** o progresso atual e o tratamento posterior dos
achados estão na [reconciliação da tasklist](0103-tasklist-conclusao.md#76-reconciliação-de-implementação-e-aceite--18092026).
Não usar as contagens ou ausências de produto descritas abaixo como status atual.

**Resultado:** implementação parcial, com uma base extensa de contratos e serviços testados, mas ainda sem a composição e os percursos necessários para concluir o AEP. Não é correto afirmar que faltam somente três validações manuais.

Repositório: `C:\Users\leonardo.gleison\dev\assistente-worktrees\aep-0103-comandos`.
Branch: `feat/aep-0103-comandos`.
HEAD: `7e88945ec585e4352c4548aad8cd14e4db0134c1`, incluindo as alterações locais preexistentes da Topbar, ajuda, Ctrl+T e tasklist.
Fonte normativa: [AEP-0103](0103-comandos-acionadores-e-camadas-contextuais.md).
Acompanhamento vigente: [tasklist de conclusão](0103-tasklist-conclusao.md).

## 1. Método, abrangência e limites

Nota posterior: a execução de correções após esta baseline está registrada na seção 10 da [tasklist vigente](0103-tasklist-conclusao.md). Os resultados abaixo descrevem a fotografia da revisão, não o estado posterior às correções.

Revisão das decisões D1–D16, fases 0–7, 83 critérios finais, 84 itens Ixx.n, 20 itens Pxx.n e inventário de atalhos. Conferência da montagem do App, callers de produção, fronteiras frontend/backend, persistência, segurança, configuração, jobs, outbox, manutenção, hardware e testes. Seis frentes independentes de subagentes foram reutilizadas; os achados principais foram conferidos pelo agente principal contra código e, onde indicado, repetição do teste.

A comparação local com main abrange 501 arquivos (88.536 inserções / 1.121 remoções na fotografia inicial), incluindo código e testes. Esse volume não é métrica de completude. Esta revisão cobre o escopo arquitetural e os caminhos críticos; não é prova formal de ausência de defeitos em cada linha.

A ferramenta especializada Bugbot não está disponível nesta sessão; a leitura das skills de revisão não foi apresentada como execução dela. Revisão assistida por subagentes não substitui o gate Bugbot exigido pelo repositório. Race, CI remoto, Wails/HID/NVDA integrados e suíte integral frontend/e2e não foram certificados nesta rodada.

Nenhum código de produção ou teste foi corrigido nesta rodada. Foram alterados somente documentos de acompanhamento, preservando mudanças anteriores. Não houve migração de banco pessoal, leitura de segredos, instalação de hotkeys, acionamento de hardware, push, PR ou merge.

## 2. Correção do progresso anteriormente informado — F08

A tasklist histórica contém 84 itens de infraestrutura: **53 checkboxes marcados e 31 abertos**. O apêndice contém 83 critérios finais, dos quais 28 receberam anotações “Fechado” por evidência local em rodadas posteriores.

O total narrado como 81/84 corresponde a **53 + 28**, somando itens I de infraestrutura com critérios C do AEP. As contagens 60/84, 70/84 e 81/84 misturam universos e não são uma medida válida de progresso. Além disso, a contagem de 4/15 pacotes completos não confirma a montagem integral da base: I13/I14 foram fechados com escopos qualificados/sentinela, enquanto integração mais ampla continua aberta.

Portanto:

- Retira-se a afirmação de que restam apenas três testes manuais.
- Preservam-se código, testes e relatos válidos como evidência do subconjunto exercitado.
- Os 53 checkboxes são contagem documental; não estão automaticamente recertificados pela revisão.
- Os 83 critérios finais precisam de aceite conforme o texto completo, com integração quando exigida.
- Os 12 gates novos reorganizam obrigações existentes; não aumentam o AEP nem zeram o trabalho implementado.

**Saída verificável F08:** nova tasklist mapeia integralmente I/P/C e a documentação principal aponta a ela. Correção documental efetuada nesta rodada; futuras contagens devem manter denominadores separados.

## 3. O que está bom e deve ser reaproveitado

- **Registro e resolvedor:** validação de contratos, schema, origens/efeito/contexto, busca localizada, precedência, deduplicação de equivalentes e índice por acionador. Evidência em `internal/commandcatalog`, `commandbindings`, `commandcontract`, `commandjson`. O resolvedor não faz leitura de SQLite por tecla.
- **Segurança de execução:** epochs, gate, ledger, fingerprints, CAS e recibos de decisão possuem testes de invalidação, replay, rollback, handoff e resultados incertos. Evidência em `commandexecution`, `commandsecurity`, `commandledger`, `commanddecision`. Essas garantias só valem no produto quando a montagem não as contorna.
- **Configuração:** CRUD transacional, projeção, diffs, diagnóstico/revalidação de conflito, defaults versionados, rebase/restore e `needs_review` têm serviços aproveitáveis. Não há razão para recomeçar essas bibliotecas.
- **Claims/grants:** refs builtin/user, isolamento global/workspace, revogação, terminalidade e restore têm cobertura relevante. `activation_hook.go` revoga grants e reconcilia efeitos na mesma transação.
- **Jobs e persistência:** timeline incremental e outbox durável preservam identidade/replay; serviços de lease/recovery/coordinator existem. Falta fechamento do consumo/manutenção na composição produtiva.
- **Hardware:** driver Go real, manager, renderer com cache/diff e frame completo, callbacks por geração e estado seguro já existem. O usuário validou um round-trip HID real em 15/09/2026.
- **Componentes compartilhados:** o picker aproveita a UI existente e corresponde à preferência do usuário; não é necessário voltar a um modal exclusivo. Ele precisa ganhar busca/execução/acessibilidade exigidas por D9.
- **Regressões locais:** atalhos legados e bloqueio por modal foram validados pelo usuário. É evidência de preservação do comportamento atual, não de migração ao executor novo.

## 4. Defeitos confirmados e evidências

Prioridade indica ordem de correção antes do gate correspondente. Nenhum achado abaixo é classificado como incidente crítico/exploração atual sem demonstração.

### F01 — P1: teste de colisão de migração usa versão errada

Local: `internal/database/command_migration_test.go:113` e `internal/database/command_migration.go:15`.

O teste injeta conflito em `schema_migrations.version=20`, mas `ApplyCommandStorageMigration` aplica versão 21. O registro central também reserva a v21 para `command_storage_initial`. O teste espera uma recusa que não corresponde ao alvo que preparou. A falha foi reproduzida isoladamente nos dois subcasos.

Isso é inconsistência de fixture/expectativa após renumeração, não evidência de corrupção de banco real. Corrigir a prova de colisão no alvo correto, preservando testes de rollback/no-op e compatibilidade; não alterar versão publicada só para deixar teste verde.

**Gate:** R06.1/R06.2, I01.1/I15.2. Teste de nome conflitante na versão canônica recusa antes do callback, sem alterar carimbo ou dados.

### F02 — P1: importação de credencial indisponível não desabilita binding

Local: `internal/commandportability/types.go:864` e `types.go:1030`; teste `types_test.go:407`.

`validateCredentialReferences` consulta pattern exato, mas para missing/foreign/ambiguous apenas acrescenta warning. Ela não devolve estado que faça `validateReferences` atribuir `binding.Enabled=false`. O teste existente verifica warning/pattern e não verifica estado habilitado. Isso contraria C48.

O defeito foi confirmado pela leitura do percurso e da cópia dos bindings de volta ao plano. Não se demonstrou uso de credencial estrangeira nem exploração do App, cujo import de comandos ainda não está exposto.

**Gate:** R05.4, I11.5, C48. Testar os três estados tanto em `Plan.Snapshot` quanto no commit aplicado; o binding deve permanecer desabilitado até referência válida e ação autorizada.

### F03 — P1: falha do consumer não aciona retry/dead-letter

Local: `internal/commandjobactivation/consumer.go:82` (RunPass, erro em Consume) e `maintenance_adapter.go:47`.

A passagem reclama o lote e retorna o erro de consumo sem chamar `Store.Retry` ou `DeadLetter`. O adapter também só propaga erro/More. A linha permanece processing até vencer a lease; `commandjobevents.Store.RequeueExpiredLeases` a torna pending sem aplicar limite de tentativas. As primitivas de retry/dead-letter existem, mas não têm caller nesse percurso produtivo futuro.

Uma ocorrência permanentemente inválida pode ser reclamada repetidamente e impedir progresso previsível; não há prova de saída terminal pela política de tentativas. A cadência produtiva ainda não está montada, portanto não se afirma loop em execução no App atual.

**Gate:** R03.4, I09.4/I09.6. Erro transitório retorna a pending com política explícita; erro permanente/limite atingido chega a dead_letter auditado; lote continua com semântica definida. Teste usa relógio/leases controlados, sem sleep frágil.

### F04 — P2: resposta atrasada reabre picker fechado

Local: `frontend/src/components/layout/Topbar.tsx:284`.

Depois de abrir o menu, o callback de `refreshCommandCatalog().then(...)` chama `openCommandMenu` novamente. Só verifica se o botão existe, não se aquela abertura ainda está válida. Escape durante o fetch fecha o picker, mas o retorno tardio pode reabri-lo e tomar foco; respostas de requisições anteriores também podem substituir estado mais novo.

**Gate:** R08.3, C33. Teste com promise controlada: abrir, Escape, resolver; menu continua fechado e foco preservado. Cobrir troca de sessão/locale, reabertura e resposta fora de ordem.

### F05 — P2: motivo real de indisponibilidade é ocultado

Local: `internal/wailsapi/command_catalog.go:130` e `frontend/src/components/layout/Topbar.tsx:231`.

A API calcula `available=false` quando `CheckReadiness` recusa e devolve `readinessReason`. A UI ignora esse campo e usa `availabilityReason || availabilityStatus`. É possível o status estático ser “available” enquanto o comando está desabilitado por origem/readiness, resultando em mensagem contraditória ou sem motivo útil.

Além disso, `commandcatalog/readiness.go:33` é explicitamente um preflight de protótipo read-only: recusa write/destructive/interativo/alvo mutável. Popular o catálogo sozinho não torna essas ações selecionáveis. Substituir pela disponibilidade adequada ao executor completo não significa dispensar autorização/decisão.

**Gate:** R08.1/R08.4, C32. Motivo derivado corretamente e traduzido; ação disponível/indisponível deve corresponder ao contexto atual, preservando revalidação na execução.

### F06 — P2: strings da palette não estão nos três locales

Local: `frontend/src/components/layout/Topbar.tsx:222` e demais chamadas `commandPalette.*`; `KeyboardShortcutsHelp.tsx:85`.

A busca nos arquivos `pt-BR.ts`, `en.ts` e `es.ts` não encontrou `commandPalette`. O uso de defaultValue mascara a ausência e deixa textos de fallback em português/inglês ao trocar idioma.

**Gate:** R08.4. Adicionar todas as chaves nos três idiomas e verificar UI/announcer sem depender de fallback para texto builtin.

### F07 — P2: teste concorrente do lifecycle é instável

Local: `internal/app/app_command_lifecycle_test.go:45` e `:1463`.

A suíte ampla do App passou numa rodada, mas `TestAppCommandLifecycleHooksSerializeConcurrentResetAndShutdown` falhou na repetição focada `-count=20`, em duas iterações, com “callback concorrente executou sob lock de autenticação”.

**Limite do diagnóstico:** o detector usa `authMu.TryLock()`. Em um teste com goroutines concorrentes, falhar TryLock não prova que a própria goroutine do callback reteve o lock; outra goroutine pode estar lendo o estado. `resetCommandHostSession` solta seu RLock antes de chamar o HostState. Portanto, há falha reproduzível da prova de concorrência, mas ainda não se conclui deadlock ou violação real do mesmo thread.

**Gate:** R01.4/R06.2. Investigar a garantia de serialização e corrigir implementação ou mecanismo de prova conforme causa demonstrada; nunca remover/afrouxar o teste só para passar. Repetição controlada e race devem validar a garantia correta.

### F09 — P2: indisponíveis não são exploráveis por setas no picker

Local: `frontend/src/components/ui/menu/Menu.tsx:51`, `:336`, `:367` e `Topbar.tsx:230`.

A Topbar marca indisponíveis como disabled; o componente Menu exclui disabled da navegação focável e renderiza botão desabilitado. O motivo inserido em ariaLabel não fica acessível no percurso normal de setas/foco da palette. Isso não demonstra impossibilidade de leitura em todos os modos do NVDA, mas falha em oferecer exploração completa no fluxo de teclado definido em D9.

O picker hoje usa role menu/menuitem; o AEP pede combobox/listbox para a busca. A escolha de picker visual pode ser preservada ajustando o contrato acessível compartilhado ou uma variante apropriada.

**Gate:** R08.4, C32/C33. Navegar até resultado indisponível, ouvir motivo e impedir execução; testes de teclado/axe e NVDA no fluxo final.

## 5. Riscos que exigem prova adicional, sem afirmar exploração atual

### H01 — Ctrl+K concorre com o editor

`RichTextEditor.tsx:122` abre diálogo de link e faz preventDefault, mas não stopPropagation. A Topbar em `:376` também trata Ctrl+K e não verifica defaultPrevented, repeat ou composição. Dependendo do momento em que o modal é registrado, pode abrir os dois fluxos. Falta teste conjunto; os testes atuais da Topbar não exercitam palette.

Gate R07.2/R08.3: precedência contextual explícita e um único efeito por pressão. Ctrl+K no editor preserva ação contextual; fora dele abre palette. Não escolher outro atalho silenciosamente nem quebrar editor.

### H02 — Bridge pública precisa de origem e executor garantidos na montagem

`app_command_bridge.go:63` / `:74` delegam ao Bridge. Capability vincula comando/geração/owner, mas não origem; `Invocation.Source` vem do payload e validInvocation verifica enum. A interface Port permite qualquer implementação de Dispatch.

Hoje o sentinela recusa tudo, então não há exploração demonstrada. Antes de montar capacidades reais, origem física precisa vir do adapter confiável e o port deve alcançar o CommandExecutionService. Saber um UUID de capability não pode permitir inventar source global/HID nem evitar normalização, ledger, autorização e decisão.

Gate R01.3: teste negativo pelo ingresso Wails público; payload escolhendo source/IDs físicos não obtém execução, e todo handoff real tem invocação autorizada.

### H03 — Workspace de import não resolvido e API de export ampla

`commandportability/types.go:807` retorna camada desabilitada com ID de workspace de origem quando não há destino. Não foi demonstrado um write não autorizado, pois gates posteriores podem recusar. Falta comprovar o destino efetivamente aplicado ou a recusa antes do writer.

`ExportFromStore` seleciona IDs por owner; `ExportScopeFromStore` oferece escopo exato. Não há caller produtivo demonstrando autorização de workspace no caminho amplo. Não se classifica isso como vazamento atual.

Gate R05.3: usar autorização/escopo exatos, testar workspace não resolvido e foreign_owner até o commit, nunca inferir acesso por nome/ID portátil.

### H04 — Restore e contratos de origem

`commandactivation/service.go:522` restaura claims manuais persistentes, mas comentário diz encerrar temporárias/session e o loop as ignora. Outro mecanismo pode encerrá-las; essa obrigação precisa de prova no startup completo. O App restaura apenas conjunto global no caminho atual e usa origem interna `ui_action/command-lifecycle`, que não demonstra rebind físico de dispositivo.

Gate R02.4/R04.3: teste misto global/workspace, sessão/temporária/persistente, origem removida, idempotência e atomicidade. Não tratar comentário como evidência de implementação.

### H05 — Espera por gate/SQLite e responsividade

O gate RWMutex não é cancelável durante aquisição. O pipeline faz reservas/CAS no banco durante trechos protegidos. Isso cria risco de cauda sob mutações concorrentes. Microbenchmark do resolvedor em nanossegundos não representa esse caminho.

Medição opt-in desta revisão (20 warmups, 200 amostras/cenário; SQLite temporário em arquivo, WAL, synchronous=1, quatro conexões):

- Serial: p50 4,3149 ms; p95 6,9961 ms; p99 9,0656 ms.
- Mutação SQLite sintética concorrente: p50 25,4587 ms; p95 143,9388 ms; p99 241,5488 ms.
- Cenário concorrente efetuou 136.767 mutações sintéticas; teste terminou PASS.

São amostras do ExecuteEnvelope até o estado terminal, não latência teclado→ação nem medição exata do orçamento interno experimental de 1 ms. Não há garantia de máquina ociosa durante uma revisão paralela. Não declarar regressão percebida pelo usuário nem SLO de produto a partir desses números.

Gate R06.3/R12.1: medir segmentos/carga representativa, resolver contenção e validar atalhos reais sem remover ledger/segurança para cumprir número.

## 6. Lacunas de composição e produto frente ao AEP

### L01 — Boot ainda é sentinela

`app_command_lifecycle_product.go:23` define `lifecycle.ready`; Resolve/Authorize recusam e o adapter retorna ErrCapabilityDenied. A projeção produtiva usa `ProjectLocalRead` (`:247`). A fábrica completa existe, mas bootstrap mínimo não equivale ao sistema integral. R01 fecha isso sem mover contratos para fixtures.

### L02 — Providers e configuração por workspace ainda incompletos

`app_command_context.go:177` monta FactBus de workspace/foreground; surface, diálogo/foco/perfil exigem portas reais e integração frontend. A biblioteca frontend de bridge tem exports/testes, mas isso não prova instalação do runtime nas telas. R01/R02 precisam demonstrar sessão, ownership, versões e publicação global + workspace.

### L03 — Consumer e coordinator não montados no App

`internal/app/app_jobs.go:41` cria o Manager sem ligar Consumer/ConfigureCommandMaintenance. A busca de callers produtivos das fábricas não encontrou a montagem. `manager.go:1689` preserva retenção legada quando coordinator é nil. A outbox existir no schema não garante entrega, heartbeat ou recovery em operação. R03/R04.

### L04 — Recovery de restart e purga da outbox

`app_command_lifecycle_runtime.go` mantém Recover sem reconciliação durável; o drain atual cobre geração fechada graciosamente. Crash/restart exige prova própria. `commandjobevents.Store.PurgeExpired` existe, mas não há chamada produtiva nem etapa de purga em OutboxPort/coordinator. Crescimento de terminais precisa de saída bounded com deadline/claim/lease preservados. R04.

### L05 — Catálogo e migração reais

Os handlers de teclado/profiles/jobs ainda executam o fluxo antigo. O catálogo do boot tem apenas sentinela. Logo, “todos os atalhos funcionam” não certifica C02/C03. R07 migra famílias com equivalência e remove o caminho paralelo depois da prova.

### L06 — Palette não executa e configuração está ausente

`Topbar.tsx:236` apenas anuncia descrição ao selecionar. O serviço frontend oferece list/describe, não execução. A API de list usa catálogo estático; no estado sentinela não há ações reais úteis. A lista vazia relatada pode envolver falha de bootstrap ou consulta transformada em lista vazia pelo catch; não foi observada a sessão/log do App para afirmar a causa exata desse vazio.

D9 exige execução, argumentos, aliases, atalho efetivo, recentes/favoritos e navegação para configuração. D15 exige explicitamente **Configurações → Comandos e acionadores**, lista/detalhe de camadas, captura, conflitos e restore. Não existe essa tela exposta hoje. R08/R09 são implementação pendente, não falha do usuário em encontrar uma opção.

### L07 — Stream Deck funcional no App

Driver, renderer e prova HID existem; não foi encontrada montagem produtiva que os conecte aos comandos reais. Faltam mapas/imagens/títulos/estados, navegação, disputa/reconexão na UI e testes com sessão/lock do App. R10.

### L08 — Chat/CLI/import-export finais

Não foram encontrados entrypoints das tools compostas `command_catalog` e `command_config` nem CLI de comandos integrada como D10/fase 6 exigem. Portabilidade tem biblioteca, mas fluxo geral do produto ainda precisa incluir commandLayers e relatórios. R05/R11.

## 7. Validações executadas e resultado correto

### Backend

- `go build ./...`: PASS, exit 0 (frente de qualificação).
- `go vet ./...`: PASS, exit 0.
- `go test -mod=readonly ./internal/command... ./internal/app ./internal/jobs ./internal/auth ./internal/database -count=1`: FAIL global; pacotes command, App, jobs e auth passaram nessa rodada; database falhou em F01.
- Repetição pelo agente principal: `go test -mod=readonly ./internal/database -run '^TestApplyCommandStorageMigrationRejectsConflictingV20Name$' -count=1`: FAIL, dois subcasos, exit 1.
- `go test -mod=readonly ./internal/app -run '^TestAppCommandLifecycleHooksSerializeConcurrentResetAndShutdown$' -count=20 -timeout=120s`: FAIL, duas iterações, exit 1. Portanto, App não fica qualificado pela passagem única anterior.
- `go test -mod=readonly ./internal/acp ./internal/acpregistry -count=1 -v`: FAIL, ambos encerram com `0xffffffff` sem nome de teste/stack trace nessa execução. Outras tentativas de agentes relataram negação de acesso; não são prova da mesma causa. Diagnóstico permanece aberto, sem atribuir automaticamente ao AEP ou ao sandbox.
- Pacotes adicionais `commandactivation`, `commandconfig`, `commandportability`, `wailsapi` foram validados nas frentes de leitura. Passagem de suíte não anula F02, porque o teste não verifica o estado desabilitado.
- `COMMAND_EXECUTION_LATENCY=1 go test -mod=readonly ./internal/commandexecution -run '^TestExecuteEnvelopeLatency$' -count=1 -v`: PASS da fixture/execução; métricas e limites em H05, não aceite de desempenho.

### Frontend

- `npx tsc --noEmit`: sem erros.
- `npm run lint`: PASS, exit 0 da sequência.
- `npm test -- --run src/lib/commandBridge.test.ts src/lib/commandBridgeContext.test.ts src/lib/commandBridgeDialogAdapter.test.ts src/lib/commandBridgeWails.test.ts src/lib/commandContextProviders.test.ts src/lib/commandContextSession.test.ts src/lib/modalRegistry.test.ts src/components/layout/Topbar.test.tsx src/components/ui/KeyboardShortcutsHelp.test.tsx src/hooks/useWorkspaceKeyboardShortcuts.test.ts src/services/commandCatalog.test.ts`: **12 arquivos / 124 testes PASS**. O filtro inclui os dois arquivos .test.ts/.test.tsx do hook.
- Primeira tentativa do Vitest falhou no startup com spawn EPERM; repetição autorizada fora do sandbox passou. Não é falha de asserção do frontend.
- Esses testes não cobrem abertura/execução da palette, fetch tardio, colisão Ctrl+K conjunta ou semântica dos indisponíveis.

Não foram certificados nesta rodada: `go test ./...` integral verde, race, golangci-lint, Stylelint, Vitest integral, E2E, regeneração Wails, CI/review remota, NVDA e hardware integrados. São gates explícitos, não resultados presumidos.

## 8. O que os testes manuais anteriores realmente provaram

- `TestManualStreamDeckPhysicalRoundTrip`: usuário recebeu frame e pressionou primeira tecla no Stream Deck de 15 teclas; evento key:0 recebido e PASS. Prova válida do round-trip Go/HID nesse aparelho.
- `TestManualPhysicalEnvironment`: capturou foreground real. Porém usa `hotkey.IsSupported`, fixa OSSessionProbe como Known/desbloqueado e recebe modelo/serial por variáveis de ambiente. Não registra/pressiona hotkey global, não observa ciclo real lock/unlock e não repete verificação HID.
- Relato no App: atalhos antigos e bloqueio por modal funcionam; Ctrl+T corrigido e usuário confirmou comportamento; picker abre mas não tem comandos.
- Não há relato que valide NVDA/execução da palette, tela de configuração inexistente, reconexão integrada, múltiplos dispositivos ou comando real pelo Stream Deck.

Preservar esses resultados com seu alcance. A próxima rodada manual deve ocorrer somente após o caminho estar implementado, com instruções exatas e checkboxes de resultado.

## 9. Ordem recomendada

1. Corrigir F01/F02/F03 e investigar F07 no escopo dos gates; não assumir “só ligar UI”.
2. Fechar R01 (composição real) e R02/R05 em paralelo; R03/R04 ligam automação/manutenção/recovery.
3. Qualificar R06 e aceitar BASE-PRONTA com todos os 84 itens reconciliados.
4. R07 migra/popula; R08–R11 entregam interfaces, dispositivos, chat/CLI e portabilidade.
5. R12 fecha os 83 critérios do AEP, documentação e revisão.

Os bugs da palette F04/F05/F06/F09 ficam rastreados em R08 e podem ser corrigidos antes quando houver uma frente independente. Isso não certifica integração nem troca a prioridade da infraestrutura definida pelo usuário.
