# AEP-0103 — Tasklist de infraestrutura e entrega
Documento de acompanhamento, não nova AEP nem alteração dos contratos.
Baseline v1: 14/09/2026 • código examinado: `11c10c578051c7276b7345cd608d6460a3b1803c`.
Branch: `feat/aep-0103-comandos`. AEP principal continua **In Progress**.

## 1. Objetivo e fonte de verdade

Encerrar a infraestrutura prevista antes de migrar/popular o novo sistema com os comandos existentes. Não reduzir a base para antecipar uma demonstração de atalhos.

Arquivo canônico de acompanhamento: `aep/0103-tasklist-infraestrutura.md` no worktree. A cópia entregue em `outputs/` é uma fotografia; atualizar a canônica e republicar a cópia nas revisões do plano.

A fonte normativa permanece `aep/0103-comandos-acionadores-e-camadas-contextuais.md`. Este arquivo é a fila operacional: IDs estáveis, dependências, critérios de saída e registro de mudanças. Em divergência, não mudar arquitetura silenciosamente; esclarecer e atualizar o AEP no mesmo ciclo autorizado.

Este plano substitui a estimativa informal de “35%” como instrumento de acompanhamento. Não há porcentagem total validada. Test coverage, linhas e commits não medem entrega do AEP.

### Fotografia atual — 14/09/2026, após I02

- 15 pacotes de infraestrutura; I01/I02 implementados e validados localmente (2/15); qualificação global I15 pendente.
- 75 itens restantes de infraestrutura, dos 84 da baseline; I01.1–I02.5 encerrados na implementação local.
- 4 marcos de infraestrutura, seguidos por 6 pacotes de migração/entrega.
- 83 critérios finais do AEP com responsáveis mapeados no apêndice.
- M1: I01/I02 entregues localmente, I03 em integração; M2–M4 ainda não encerrados. Contagem de pacotes não é porcentagem de esforço ou do AEP.
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

Evidência/limite atual: internal/commandcontext oferece freshness/VersionService; HostState atual publica configuração global por usuário; os providers reais completos ainda não estão ligados.

- [ ] I03.1 — Implementar providers de surface, diálogo, foco/controle, aba, workspace, perfil e janela; validar ownership na fonte, não confiar no snapshot enviado pela UI.
- [ ] I03.2 — Implementar ContextFactBus e reconciliação síncrona quando uma notificação se perder ou chegar fora de ordem.
- [ ] I03.3 — Separar gerações globais, por workspace e de camadas efetivas; mudança em outra conta/workspace não invalida trabalho independente.
- [ ] I03.4 — Completar políticas exact_version, max_age_ms e event_snapshot no percurso de admissão, com timestamps confiáveis por provider.
- [ ] I03.5 — Implementar captura de foreground antes de bring-to-front, redação de resumo e degradação explícita onde não houver adapter; não persistir títulos/URLs.
- [ ] I03.6 — Testar atualização de contexto concorrente, provider ausente e caches positivos/negativos com todas as dimensões de isolamento.

Critério de saída: Todo alvo/contexto usado pelo executor é reconsultável e versionado, inclusive com perda de notificações.

### I04 — Dispatcher, execução e ledger completos

Estado: **Parcial**. Esforço restante: **GG**.
Dependências: I02, I03.
Referências: D2.1, D3, D4, D11, D16.

Evidência/limite atual: internal/commandexecution/types.go restringe o executor a local_session/read direto, sem argumentos, workspace, providers ou decisão; ledger e handles já têm testes desse subconjunto.

- [ ] I04.1 — Unificar execução direta e por trigger: resolver, fixar origem vencedora, derivar ator e normalizar argumentos antes de assinar/reservar.
- [ ] I04.2 — Ampliar reserva e auditoria para argumentos, triggers, origem física/evento, workspace e contexto; preservar IDs canônicos e rejeitar fingerprint divergente.
- [ ] I04.3 — Persistir recusas pós-autenticação e marcadores terminais suppressed/rejected_stale; reentrega não pode passar a executar após alteração de configuração.
- [ ] I04.4 — Completar gates evaluating → queued → running para read/write/destructive, revalidando política, catálogo, mapa, contexto e decisão no ponto correto.
- [ ] I04.5 — Preservar handoff não bloqueante, finalização atômica, resultado redigido, consulta autorizada independente da versão atual e reconciliação auditada de outcome_unknown sem reexecução.
- [ ] I04.6 — Testar filas, duplicidade, perda de ack, panic, cancelamento, mudança de configuração e falhas transacionais com handlers controlados.

Critério de saída: O serviço único consegue aplicar os contratos do AEP a todos os tipos previstos de execução, sem depender de handlers de produto.

### I05 — Configuração completa, defaults e restauração

Estado: **Parcial**. Esforço restante: **G**.
Dependências: I02, I03.
Referências: D4–D7, D10, D11.

Evidência/limite atual: commandbindings tem composição/restore em memória; commandconfig tem schema de layers/bindings/gerações e writer global de enabled, não CRUD/restauração completos.

- [ ] I05.1 — Completar projeção de configuração global + workspace, condições, argumentos, tipos de acionador e apresentação; nenhuma leitura bruta vira autorização.
- [ ] I05.2 — Implementar criar/editar/excluir/habilitar/desabilitar bindings e layers com ownership, validação de referências e CAS de geração.
- [ ] I05.3 — Implementar restauração persistente por binding, camada e conjunto; não apagar defaults nem conceder grants.
- [ ] I05.4 — Completar upgrade de defaults: versão sem mudança semântica, needs_review no contexto exato e rebase confirmado; eliminar bloqueio excessivamente amplo do protótipo.
- [ ] I05.5 — Expor serviço interno único de conflito/diagnóstico e diff exato, reutilizável por UI/chat/importação; testar corrida entre checagem e commit.

Critério de saída: Todas as operações de configuração previstas têm uma implementação transacional comum, sem escritores paralelos.

### I06 — Decisões e mutações de capacidade

Estado: **Parcial**. Esforço restante: **G**.
Dependências: I04, I05.
Referências: D2, D10, D11; AEP-0091.

Evidência/limite atual: commanddecision e app_command_decision.go persistem/mostram decisões; commandconfig/decision.go consome receipt atomicamente apenas para binding_enabled global.

- [ ] I06.1 — Ampliar receipt para invocação e consumir decisão destrutiva na mesma transação do CAS para queued.
- [ ] I06.2 — Aplicar diff/receipt/auditoria a todo CRUD, restore, import e alteração de capacidade; incorporar a classificação obrigatória de cada verbo.
- [ ] I06.3 — Registrar presenter autenticado no ciclo apropriado; cancelamento, timeout, logout e resposta tardia não deixam autorização reutilizável.
- [ ] I06.4 — Revalidar gerações de configuração/grant entre apresentação e commit; negar origem headless onde o contrato exige interlocutor.
- [ ] I06.5 — Testar decisão manipulada, replay, rollback e correlação; validar contrato do diálogo compartilhado sem duplicar UI.

Critério de saída: Nenhuma nova rota de configuração ou comando mutável precisa construir um mecanismo próprio de confirmação.

### I07 — Claims e ativação manual/contextual/temporária

Estado: **Pendente — primitivas de contexto reaproveitáveis**. Esforço restante: **G**.
Dependências: I03, I05, I06.
Referências: D8, D11.

Evidência/limite atual: commandconfig/types.go exclui regras e claims do Snapshot; App recusa ActiveUserLayerIDs até existir restore autenticado.

- [ ] I07.1 — Implementar schema/repository de regras, estado e referências builtin/user com isolamento global/workspace.
- [ ] I07.2 — Implementar união de claims, pin, toggle/back e manual_stack_key derivada da origem; uma regra não encerra claim de outra.
- [ ] I07.3 — Implementar expiração idempotente e estados terminais que não ressuscitam, inclusive após restart.
- [ ] I07.4 — Implementar disable/enable com revalidação das claims e atualização atômica das gerações efetivas.
- [ ] I07.5 — Implementar restore autenticado de claims manuais persistentes; rebind de sessão/gerações/dispositivo e revisão quando a origem não existir.
- [ ] I07.6 — Testar ciclos concorrentes, condições recalculadas, isolamento e reinicialização sem reativação indevida.

Critério de saída: Ativação deixa de ser lista fornecida ao projetor e passa a ser estado autoritativo com ciclo de vida completo.

### I08 — Grants exclusivos de automação de camadas

Estado: **Pendente**. Esforço restante: **G**.
Dependências: I06, I07.
Referências: D8, D11; AEP-0101 como limite de separação.

Evidência/limite atual: O AEP especifica command_layer_automation_grants; não confundir com grants existentes de delegação de jobs.

- [ ] I08.1 — Persistir chave natural por owner/workspace/layer/rule, uma concessão ativa e histórico de gerações/revogações.
- [ ] I08.2 — Criar/habilitar regra event-driven somente com decisão vinculada ao fingerprint exato da regra e dos produtores.
- [ ] I08.3 — Revogar atomicamente ao alterar/excluir/desabilitar regra ou camada; reabilitação exige nova decisão.
- [ ] I08.4 — Revalidar ID, geração e fingerprints autoritativos a cada evento; import/cópia/restore nunca transportam concessão.
- [ ] I08.5 — Testar concessão/revogação concorrente, receipt atrasada e isolamento entre grants de delegação e ativação.

Critério de saída: Automação possui autoridade explícita e revogável, sem herdar implicitamente as permissões do usuário.

### I09 — Fatos de jobs, outbox e replay durável

Estado: **Pendente na integração de comandos**. Esforço restante: **GG**.
Dependências: I07, I08.
Referências: D2.1, D8, D11; AEP-0048, AEP-0067, AEP-0074-B.

Evidência/limite atual: Há runtime de jobs existente em internal/jobs/executor.go e repository.go, com LogRun final; internal/database/models_jobs.go ainda não representa a timeline incremental exigida. O protocolo command-context.job-run-state.v1 exige integração própria, não reaproveitamento direto do EventBus.

- [ ] I09.1 — Migrar timeline/status incremental e queued_at/started_at conforme AEP-0048; persistir Job.DatabaseID, slug, run_event_id e root_origin_type sem inferência retroativa.
- [ ] I09.2 — Inserir fato elegível e outbox na mesma transação; não usar cascade de runs como fronteira de replay.
- [ ] I09.3 — Implementar epochs de política de replay e deadline imutável por ocorrência, preservado quando a retenção mudar.
- [ ] I09.4 — Implementar consumo com lease, retry, delivered/dead_letter e processamento de cada regra/escopo por CAS de sequência; replay e conflito de fingerprint são distintos.
- [ ] I09.5 — Implementar lease/heartbeat da claim de job, reconciliação autoritativa e anti-loop com command_chain_history separado, limite versionado 16.
- [ ] I09.6 — Testar queda após commit, entrega duplicada/fora de ordem, count-cap, fonte perdida, raiz externa/unknown e fatos legados ambíguos; atualizar AEPs associados.

Critério de saída: Evento durável nunca perde sua barreira de replay pela limpeza do job e não concede capacidade por payload.

### I10 — Identidades, autorização e delegação entre runtimes

Estado: **Parcial — apenas sessão local no novo executor**. Esforço restante: **GG**.
Dependências: I02, I04, I06.
Referências: D2.1, D10, D14, D16; AEP-0052, AEP-0063, AEP-0101.

Evidência/limite atual: auth/command_principal.go autentica sessão local; commandexecution recebe Authorize do host; integrações externas/tools/jobs continuam condicionadas no AEP.

- [ ] I10.1 — Implementar política autoritativa de comandos para usuário/agente/job/system, sem tratar allowed_source_types ou confirmação como autorização suficiente.
- [ ] I10.2 — Completar contexto job_service e grants de delegação exatos da AEP-0101; reconsultar owner/definição/profile/grant no gate final.
- [ ] I10.3 — Implementar ponte tipada para tools/jobs com correlação command_invocation, propagação de sensibilidade e redação; preservar executor comum e commandpolicy para shell.
- [ ] I10.4 — Implementar modo system restrito sem usuário e APIs privilegiadas de manutenção; proibir acesso a bindings e delegação que exige owner.
- [ ] I10.5 — Implementar mapeamento administrativo (issuer, subject), migração/readiness externa e revogação em conjunto com AEP-0052; nunca JIT/último token. Adapters físicos externos continuam proibidos sem a AEP futura do broker.
- [ ] I10.6 — Testar fronteiras de ator, cross-profile, externa/local, origem headless e ausência de interlocutor; atualizar AEP-0063 quando alterar origem/redação.

Critério de saída: Cada runtime recebe somente autoridade comprovada; nenhuma integração posterior precisa inventar identidade ou bypass.

### I11 — Importação, exportação e referências sensíveis

Estado: **Pendente para commandLayers**. Esforço restante: **G**.
Dependências: I02, I05, I06, I08.
Referências: D10, D11; AEP-0047.

Evidência/limite atual: internal/portability/types.go e service.go ainda não incluem commandLayers. AEP-0103 exige versão/registro explícitos de resources.commandLayers.

- [ ] I11.1 — Versionar resources.commandLayers no envelope da AEP-0047 e implementar round-trip de deltas/needs_review e escopo portátil.
- [ ] I11.2 — Resolver UUIDs, refs builtin/user e mapa de workspaces no destino autenticado; conflito foreign_owner não revela conteúdo.
- [ ] I11.3 — Implementar manter/substituir/cópia com remapeamento transacional; nome conflitante exige escolha explícita.
- [ ] I11.4 — Excluir grants, claims, defaults puros e histórico; regra event-driven importada fica sem concessão e desabilitada.
- [ ] I11.5 — Validar referências exatas de credenciais por pattern, não IDs locais; separar export sem segredo e export sensível UI-only com criptografia/decisão da AEP-0047.
- [ ] I11.6 — Testar importação repetida, referência ausente, dados antigos com segredo bruto e rollback do lote; atualizar AEP-0047.

Critério de saída: Backup/restore de configuração não transfere permissões nem introduz segredos em bindings.

### I12 — Recuperação e manutenção de toda a instância

Estado: **Parcial**. Esforço restante: **G**.
Dependências: I04, I07, I09, I10.
Referências: D2.1, D8, D11; AEP-0074-B.

Evidência/limite atual: commanddecision/ReconcileSession recupera somente sessão atual em lotes; commandledger/RecoverClosedGeneration exige prova externa de encerramento; não é coordenador global. A cadência legada está em internal/jobs/manager.go (runRetention), com compactação em internal/database/maintenance.go.

- [ ] I12.1 — Definir prova de encerramento de geração e exclusão de execuções antigas antes de recuperar pendências, incluindo reinício e outros usuários/sessões.
- [ ] I12.2 — Reconciliar invocação+ledger atomicamente para outcome_unknown, incluindo system com capability interna; jamais reexecutar efeito.
- [ ] I12.3 — Integrar recuperação de receipts de sessões abandonadas, claims e leases com lotes, cancelamento e critérios de término.
- [ ] I12.4 — Migrar a cadência de retenção para um único InstanceMaintenanceCoordinator; preservar limpezas legadas e compactação, com outbox antes da retenção de jobs.
- [ ] I12.5 — Implementar idade/caps sem remover ledger antes do prazo, nem estado ativo; adicionar as seis settings previstas e UI/i18n correspondente.
- [ ] I12.6 — Testar múltiplos usuários/system, interrupção entre lotes, retenção alterada, compactação e ausência de dois loops; atualizar AEP-0074-B.

Critério de saída: Reinício e limpeza têm um único dono, cobrem toda a instância e não reabrem execução ou ativação antiga.

### I13 — Infraestrutura das pontes e adapters físicos

Estado: **Parcial — máquina de pressão e observador de sessão**. Esforço restante: **GG**.
Dependências: I03, I04, I06, I07.
Referências: D3, D7, D13, D14; AEP-0080, AEP-0091.

Evidência/limite atual: commandinput/press.go cobre bordas de pressão; ossession e hooks do App existem. Isso não constitui teclado integrado, ponte de UI ou gerenciador HID.

- [ ] I13.1 — Fechar ponte tipada de despacho UI com ack/resultado/cancelamento, sessão e invocation_id; registrar capabilities sem handlers reais migrados.
- [ ] I13.2 — Implementar ownership local/global por geração, ocorrências UUIDv7, repeat/release/blur/reconexão e contrato de sequências Ctrl+N do inventário.
- [ ] I13.3 — Integrar DialogCommandScope ao stack real e reservar invariantes de decisão antes de bindings/ownership, respeitando input/IME e registro global temporário.
- [ ] I13.4 — Implementar ciclo de vida genérico de adapter, callbacks com geração, suspensão por lock/logout e shutdown; nenhum listener chama handler final.
- [ ] I13.5 — Validar biblioteca/licença/build/modelos HID e implementar gerência de dispositivos com exclusividade, reconexão/backoff e estado seguro; renderer com cache/diff e frame completo após reabrir.
- [ ] I13.6 — Validar teclado/foco/janela e ao menos um Stream Deck real; falha de hardware não derruba App. Registrar explicitamente dependência de dispositivo e ambiente.

Critério de saída: As entradas e a ponte UI cumprem contratos do núcleo antes de receber a população de comandos do aplicativo.

### I14 — Montagem final no ciclo de vida do App

Estado: **Parcial — hooks e fábricas sem bootstrap de produto**. Esforço restante: **G**.
Dependências: I01, I02, I03, I04, I05, I06, I07, I08, I09, I10, I11, I12, I13.
Referências: D2.1, D8, D11, D13.

Evidência/limite atual: app_command_execution.go tem fábrica não chamada pelo startup; app_command_os_session.go tem observador; recovery/rebuild ainda são operações internas explicitamente chamadas.

- [ ] I14.1 — Construir esqueleto de bootstrap serializado e readiness observável após I01, sem expor novas rotas nem cadastrar comandos de produto; este subitem pode começar cedo.
- [ ] I14.2 — Montar catálogo/defaults de contrato, políticas, stores, presenter, providers, dispatcher e adapters com dependências explícitas; sem fallback permissivo.
- [ ] I14.3 — Orquestrar login/unlock/restart: autenticar → recuperar/reconciliar → carregar/projetar → publicar → habilitar entradas, revalidando cada transição.
- [ ] I14.4 — Impedir retomadas concorrentes e publicação de geração antiga; logout/troca de usuário/falha de monitor cancela trabalho e apaga somente estado em memória.
- [ ] I14.5 — Integrar shutdown, drenagem/cancelamento e manutenção sem goroutines órfãs, mutex durante UI/cofre ou cadências duplicadas.
- [ ] I14.6 — Testar instalação nova, upgrade, restart com pendência, falhas em cada etapa e retomada após erro; nunca mascarar indisponibilidade como mapa vazio pronto.

Critério de saída: A base completa nasce, funciona e encerra dentro do App, ainda sem migrar os comandos existentes.

### I15 — Qualificação e aceite da infraestrutura

Estado: **Parcial — testes focados e benchmarks existentes**. Esforço restante: **G**.
Dependências: I14.
Referências: Fase 0, D12, riscos e critérios transversais.

Evidência/limite atual: Há testes unitários/integração selecionada, build/vet e microbenchmarks. Não há comprovação integral de p95, hardware, suíte geral segura e review de entrega.

- [ ] I15.1 — Isolar os testes legados que escrevem configuração pessoal; disponibilizar suíte geral reproduzível em dados temporários.
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

- **R01 — Testes pessoais:** a suíte geral legada já tentou acessar a pasta real de configuração. I15.1 deve isolar isso; não rodar indiscriminadamente contra dados pessoais.
- **R02 — Detector de corrida/lint:** último registro aponta ausência de C/CGO configurado e lint v1 incompatível com a configuração v2. Revalidar o ambiente e resolver em I15.2.
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

Avançar em I02 e preparação de I15.1. Ao fechar I02, publicar primeira calibração de esforço usando I01/I02; ao fechar cada marco, revisar previsão do restante e confirmar se o escopo aumentou.

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
