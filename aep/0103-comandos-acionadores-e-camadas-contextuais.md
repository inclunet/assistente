# AEP-0103: Comandos, acionadores e camadas contextuais

**Status:** In Progress

**Apresentação automática de abas — seção165 da tasklist:** o Stream Deck
deriva título e ícone do tipo da aba-alvo para `workspace.tab.go_to` e
`workspace.tab.first`…`ninth`, acompanhando renomeação, ordem e fechamento.
Campos personalizados e variantes de estado continuam tendo precedência.
Destino ausente ou de outro workspace é apresentado como indisponível, sem
redirecionamento. A derivação visual não concede autorização, altera o
despacho nem acrescenta auditoria por tecla. Evidências automatizadas e
limitações de integração ficam na tasklist. O título visível é abreviado para
caber na tecla quando necessário, enquanto o anúncio mantém o título completo;
aceite físico/NVDA segue pendente.

**Destino parametrizado de abas — seção164 da tasklist:** `workspace.tab.go_to`
é um único `local_ui` com binding vinculado a `workspace_id` e alvo por
posição inteira positiva (sem teto configurável) ou `tab_id` estável. Os
argumentos persistidos registram também `target_mode`: `position` junto a
`position`, ou `specific` junto a `tab_id`, sempre com `workspace_id`. Posição
usa a ordem corrente; aba ausente/fechada e workspace divergente ficam
indisponíveis sem fallback ou retarget. Ctrl+1…9 e a apresentação automática
do Deck não mudam nesta extensão. Evidências de dispatch e configurador ficam
na tasklist.

**Extensão de apresentação pós-PR #833 — gerenciadores separados:** conforme
decisão registrada na seção159 da tasklist, a tela principal organiza camadas
e seus estados. Na toolbar de Nova camada, Editar camada atua na seleção e
o menu Configurações da camada abre, separadamente, os
gerenciadores Comandos e acionadores e Regras de ativação, usando MenuButton,
Modal e DataGrid compartilhados. Formulários continuam pertencendo ao
gerenciador de origem; Escape fecha somente o modal superior e o retorno
restaura o foco no gerenciador ou na camada correspondente. Identidade,
escopo e camada delimitam a instância aberta: trocar usuário/sessão/workspace
ou remover a camada invalida a apresentação anterior. Camadas padrão e
herdadas continuam consultáveis com suas restrições existentes. Esta extensão
não altera resolução, ativação, autorização, persistência nem contratos de
execução. Gates: CRUD existente preservado, gerenciadores mutuamente
exclusivos, isolamento de identidade, foco/teclado e acessibilidade automatizada.
Aceite NVDA permanece manual; o AEP não passa a Done por esta reorganização.
O consentimento de controle pela API externa também fica nessa toolbar por
solicitação do mantenedor. Apenas sua apresentação muda: continua opt-in,
separado de teclado/Deck, sem criar convite/conexão ao marcar e com explicação
associada. A criação de convite mantém os guards e a ação explícita existentes.

**Diagnóstico nativo — seção162 (25/09/2026):** reproduzida a interrupção do
Deck após troca de aba no Wails/WebView2, com backend real, banco descartável
e somente o HID simulado. A atualização do guard deve preceder a resolução
e os previews do dispositivo; atualizar apenas o guard preserva a identidade
da configuração e o mapa local vigente. Duas sequências de nove trocas
passaram após a correção, sem Alt+Tab. Não substitui hardware físico/NVDA
nem altera a contagem **83 I / 1 P / 0 N**.

**Qualificação no CI — seção161 (25/09/2026):** após timeout acumulado,
o grupo race de contexto foi subdividido em Deck, paleta, workspace e base,
preservando todos os testes, flags, limites e agregador obrigatório. A nova
rodada deve confirmar a conclusão dos grupos; não promove o aceite manual.

**Correções da validação — seção160 (25/09/2026):** navegação local pode
continuar após a transição deixar o foco no documento, sem dispensar sessão,
mapa vigente, janela ativa, modal ou prova de alvo de ações contextuais.
Seta para cima no campo de chat/terminal só é consumida quando consegue
focar uma mensagem/item existente. Regressões automatizadas e investigação
de latência estão na tasklist; revalidação física/NVDA permanece pendente.
Não altera a contagem 83 I / 1 P nem promove o AEP a Done.

**Integração para PR — seção158 (24/09/2026):** atualização com `origin/main`
em `147274d15`, preservando autosave e concorrência de tasklists e lifecycle unificado de tools,
com os guards e a redação de comandos. A publicação prepara testes em outro
computador; não promove C38/NVDA nem os gates finais. Contagens abaixo mantidas.
Na revalidação local, 23 E2E de editor/abas, dez de operações de perfis e 13
de ações de mensagens passaram. A regressão dos consumidores da grade e da
árvore de mensagens passou 281 testes. Os achados de integração e revisões
ficam rastreados na seção158; CI remoto e aceite humano continuam separados.

**Acompanhamento vigente — seção157 (24/09/2026):** dos 84 critérios finais,
**83 têm implementação identificada (98,8%), 1 é parcial e 0 ausentes**.
C51 passa a I após prova autorizada de queda abrupta de processo filho de teste,
lease nativa, recuperação para `outcome_unknown` e replay sem repetir o efeito.
O teste isolado passou e sua repetição `-count=3` passou (1,917 s).
C38 permanece P: foco e instruções de teclado foram corrigidos e cobertos por
testes; o aceite integral com NVDA depende de validação humana pelo roteiro
`docs/content/guias/VALIDACAO_COMANDOS_NVDA.md`.
Saídas/gates preservados: **11 A / 20 I / 16 P / 1 N = 48; 1/12 gate aceito**.
R06/qualificação agregada e CI/review final continuam abertos. Não é AEP Done
nem afirmação de que só resta teste manual em todo o projeto.

**Histórico — seção156 (24/09/2026):** dos 84 critérios finais,
**82 têm implementação identificada (97,6%), 2 são parciais e 0 ausentes**.
Saídas maiores: **11 A / 20 I / 16 P / 1 N = 48; 1/12 gate aceito**. C65/C70,
R05.1 e R03.4 passam a I por código e provas existentes, sem aceite final.
R03.4 combina no App fixture o job, fato/outbox, Consumer, claim/projeção e
comando downstream/replay; suas provas complementares cobrem recovery via
SQLite reaberto, sequence/fingerprint, escopos, retenção, anti-loop e retry até
dead-letter. Não é kill abrupto de processo (C51 permanece P), nem aceite
Wails/UI ou prova de acionamento pela paleta de `job.run`.

A seção156 completa a composição externa App/HTTP/frontend: mapeamento
administrativo `(iss, sub)`, autoridade por JWT, executor e revogação por token,
execução/consulta HTTP, conexão de UI explicitamente vinculada e leitura
backend `workspace.list` por usuário (commit `0548492ab`). O ingresso externo
produtivo aceita somente UI, em `/commands/ui/execute`;
não se empresta sessão desktop nem se habilitam adapters físicos. Testes
externos App PASS (3,965 s); `go test ./internal/command... -count=1` PASS
(32 pacotes), `commandexecution` PASS (27,095 s), frontend PASS (474 arquivos /
5.895 testes), `tsc --noEmit`, ESLint dos 15 arquivos afetados e `go vet`
App/core/HTTP/jobs PASS. Godel (hardening/fingerprint) e Franklin (docs) sem
achados. A regressão App ampla terminou em 359,955 s com uma única falha de
golden compilado de revisão intermediária; o valor foi corrigido e a repetição
dos grupos keyboard-defaults/fingerprint/deltas/external/HTTP/workspace passou
em 21,938 s. Não se declara a execução ampla original como PASS.
R06/qualificação geral, CI/review final e aceites continuam abertos.
C38 e C51 permanecem parciais; R05.4 sensível permanece adiada. Não houve
validação manual/NVDA, ACP/acpregistry, Wails dev/build, execução do app,
hardware ou acesso ao banco pessoal. Apenas `wails generate module` foi
executado com aprovação, para gerar os bindings. Não restam somente validações
manuais. Detalhes e limitações: seção156 da tasklist e evidências desta AEP.

A migração de workspaces para banco permanece iniciativa independente, não
requisito geral dos comandos nem justificativa para ampliar este AEP.

**Histórico anterior à seção156:** as restrições de ingresso descritas abaixo
registram o estado de cada rodada, não substituem o estado vigente acima.

A seção152 corrige a política de roles externas: usa exclusivamente as roles
do JWT revalidado, sem fallback para role local. A política local e a exigência
de todos os scopes continuam inalteradas; ingresso externo ainda não habilitado.
A seção151 migra o middleware HTTP para resolver exclusivamente vínculos
externos explícitos, com bootstrap administrativo e alvo ativo. Atualiza D6
da AEP-0052 no mesmo ciclo; não habilita ainda o executor externo.
A seção150 individualiza tokens por fingerprint e revoga todos os contextos
do vínculo sob o mesmo gate, incluindo esperas pendentes. A captura revalida
JWT e vínculo sem rede dentro do gate. Não habilita o middleware externo.
A seção149 entrega cadastro administrativo externo na API HTTP: bootstrap
único do próprio subject legado, criação posterior com administrador mapeado
e scopes explícitos, auditoria e vínculo atômicos pela v31. Não publica
readiness nem muda o middleware, D6 da AEP-0052 ou o contexto por token;
C65/C70 seguem parciais até fechar esse ingresso e sua revogação.
A seção148 promove C83 a implementação identificada: integra diálogo e job conflitante no mesmo Manager produtivo,
mantendo apenas a captura do SO controlada. O construtor nativo é comum ao
singleton e à composição independente; o adapter recebe cópia dos modificadores
para não alterar o registro preservado durante a restauração. A matriz cobre
defaults/camadas, inputs, abas/foco, reconexão e replay, sem substituir NVDA,
hardware ou fechamento agregado de R12. C65/C70 seguem sem ingresso externo;
o override da AEP-0052 não foi ativado nesta rodada.
A seção147 fecha a lacuna automatizada de C43: camada por programa atravessa
configuração confirmada, captura por evento, executor real e publicação do mapa
Deck. Corrige a auditoria que descartava o resumo permitido em D14; somente
executável, classe e versão do provider são persistidos, com redação integral
de documentos inesperados. SO controlado nos testes; aceite físico separado.
Inclui prova global de supressão por programa até o executor real de jobs.
Configurações passam a reconhecer os defaults globais já publicados pelo
runtime, com apresentação fiel e consulta/supressão/restauração na tela;
combinações nativas continuam geridas pelos perfis de voz e jobs.
A seção136 corrige o consumo concorrente de receipts com CAS completo como
primeira operação da transação, preservando autorização, auditoria e rollback.
Na seção137, convergência de camadas e callbacks de diálogo/job recebem provas
adicionais; inventário atualizado para v40. App completo com ordem aleatória
passou e preservou log integral, sem reproduzir ou explicar a intermitência
anterior. Latência ponta a ponta e gates seguem abertos.
Seção138 integra a main `714a47c4e`, preservando o checkpoint `c9bead64c`.
Regressão revelou falha preexistente de revisão ABA ao fixar/desafixar mensagem.
A seção139 substitui a dependência exclusiva de conteúdo/timestamp por nonce
durável por mensagem, renovado transacionalmente pelos writers SQLite. A v30
faz backfill sem alterar mensagens; os digests de comandos e conversa incluem
essa revisão. Ausência de metadados recusa a operação, sem fallback inseguro.
Evidências e limites estão registrados na tasklist; sem novo aceite manual.
A seção140 fecha C02: as ligações restantes do inventário chegam a `/about`,
ao `codeBlock` no TipTap e ao recolhimento da thread no DOM real. Backend
produtor e frontend consumidor têm provas distribuídas; não é teste físico
HID/Wails ponta a ponta. Não há nova origem, default ou handler de produção.
A seção141 qualifica a autoridade de agente/job e a seção155 acrescenta o
consumidor produtivo command→tool pela paleta, com executor comum, identidade
local, confirmação e correlação/auditoria. O Send/provider e o ciclo de job
por contexto canônico continuam controlados, não scheduler/executor/LLM E2E.
R05.2 passa a implementação identificada, sem aceite agregado. Ingresso
externo de C65/C70 permanece separado e pendente da decisão de UI; R03.4 não
é promovido.
A seção142 implementa edição textual de títulos do Stream Deck por idioma,
persistência confirmada e renderização com fallback localizado. Corrige também
o diff de confirmação que omitia apresentação e recusava mudanças só de título.
C38 permanece parcial: imagem/ícone, variantes de estado e aceite NVDA integral
não foram encerrados. Não há nova migração de banco nem alteração de atalhos.
A seção143 acrescenta seleção textual e renderização de sete ícones locais do
Deck. Remoção restaura texto; divergência entre bindings elegíveis e tokens
desconhecidos usam fallback textual. C38 continua parcial por imagens próprias,
variantes de estado e aceite NVDA. O limite de 64 KiB não foi aumentado para
embutir imagens no protocolo de comandos.
A seção144 adiciona seleção de PNG/JPEG e armazenamento local separado por
usuário, confirmado na mesma transação do binding. A configuração contém só
o digest; a imagem é aplicada nos frames alterados, nunca no pressionamento.
Limites de arquivo/dimensões/quota, remoção de assets sem referências e fallback
textual/ícone mantêm o protocolo de 64 KiB. Exportações de configuração não
transportam os arquivos de imagem. C38 segue parcial por variantes e aceite NVDA.
A seção145 liga feedback transitório do Deck ao resultado real do executor,
com texto localizado e anúncio acessível, sem auditoria adicional da navegação.
Reconexão, geração e sessão invalidam resultados antigos. Estados persistentes
ligado/desligado, personalização por estado e aceite físico/NVDA ainda não foram
encerrados; C38 e as contagens permanecem parciais, sem promoção de gates.
A seção146 acrescenta variantes de título/ícone/imagem por estado, com herança
do padrão e confirmação atômica das imagens. A indicação ligado/desligado das
ações de camada consulta metadados da mesma projeção imutável publicada pelo
host; não lê regras novas do banco junto de um mapa antigo. Estado não provado
não é apresentado como desligado. Anúncios usam o canal compartilhado e a
primeira apresentação permanece silenciosa. Aceite físico/NVDA continua aberto;
C38 e as contagens não são promovidos apenas por essa implementação.
Saídas maiores: **11 A / 14 I / 22 P / 1 N = 48**; 25/48 com implementação
identificada incluindo aceitas (52,1%). Gates: **1/12 aceito, R04**.
C34/C35/C36 e R11.1/R11.3 têm implementação identificada: tools públicas de
chat, CRUD/restore/import com diff e decisão, exportação sem credenciais e
execução com ActorAgent no executor comum. A sessão é capturada no ingresso
GUI e revalidada; canais, jobs, subagentes e CLI não herdam essa autoridade.
O catálogo expõe 149 IDs; quatro permitem execute pelo chat neste lote.
CLI list/describe/execute/retry/status implementada: C71/C73 e R11.2 chegam a I.
Sessão e lookup autorizados, UUIDv7 no ingresso e replay pelo ledger comum;
sem confirmação textual alternativa ou inicialização de hardware/serviços
autônomos. Nenhum dos 150 comandos atuais permite CLI: discovery é funcional,
ações visuais/interativas continuam indisponíveis conforme D14. C02 tem
implementação identificada; não houve ampliação de origens para fechá-lo.
C22 volta a I após a correção de Δ18: condições do Deck são preparadas por
snapshot antes da entrada física, sem varrer catálogo por pressionamento.
Fatos nativos continuam atuais; configuração/registry/gerações stale recusam
a entrada. C62 agora tem LRU de seleção montado no executor produtivo, com
chave contextual completa, invalidação e cópias de proveniência. Autorização
e guards continuam por invocação. R06.3 (latência integrada) segue parcial.
C04/C06 fecham a borda de plataforma por recusa explícita de captura global
fora do Windows, desde suporte anunciado e bootstrap até o adapter nativo.
Isso não implementa hotkeys Linux/macOS; teclado local e paleta são mantidos.
C77 possui prova global de lock/unlock com perfil persistido: ocorrências
antigas não revivem e a retomada passa pela revalidação/bootstrap produtivos.
C78/C79 possuem reserva nativa temporária de Ctrl+Shift+R para o
DecisionDialog topmost, prioritária sobre bindings configuráveis, com lease,
identidade de conexão/revisão e liberação sem ressuscitar registros removidos.
Repetir o anúncio não confirma a decisão; inputs/IME e callbacks antigos
são filtrados. Aceite físico/NVDA continua pendente.
R07.3 passa a implementado após a composição de ownership e a recusa explícita
fora da plataforma qualificada; o gate R07 continua aberto pelas outras saídas.
Catálogo v41: **150 comandos / 61 locais / 67 defaults locais**; 81 IDs no
Deck contextual não significam 81 critérios concluídos. Apresentação Deck
e qualificações finais permanecem.
Reconciliação documental na seção129; implementação e testes de Δ18 na
seção130, cache e testes na seção131, plataforma na seção132 e reserva do
diálogo na seção133; tools e portabilidade do agente na seção134; CLI na seção135.
Estado individual e próximos passos na
[tasklist atualizada](0103-tasklist-conclusao.md#141-autoridade-de-agentes-e-automações--23092026).

**Seção128 — Mermaid no Stream Deck contextual (22/09/2026):** 81 IDs
(79 + aplicar/remover Mermaid), com foco/tipo/aba/perfil e células somente
no editor. Perfil isolado exige a matriz UI; binding incondicional mantém
seu percurso anterior. Paleta inalterada. Integração e regressões amplas
PASS: App 464,744 s; frontend 450 arquivos / 5.617 testes; build/vet,
TypeScript/Vite e lint focado. Modal de origem, confirmação, foco e oferta
única qualificados automaticamente; aceites físicos/NVDA pendentes.
Camadas já qualificadas na seção126; sem alterar a baseline dos 84 critérios
ou encerrar BASE-PRONTA. AEP permanece **In Progress**.

**Seção127 — páginas no Stream Deck contextual (22/09/2026):** 79 IDs
elegíveis (73 anteriores + seis mutações de listas/perfis). Páginas admitem
foco, tipo de tela e perfil, nunca ID de aba; listas usam `tasklists`, perfis
usam `profiles`, e duplicar/limpar lista também admitem `tasklist`.
Projeção e ingresso compartilham `deckPageCommandSurface`; barreiras herdadas
e caminhos nativos anteriores são preservados. Oferta física única e alvo
preparado usam o protocolo existente; mapa/conexão/geração ficam estáveis até
a admissão, sem I/O sob o gate. Qualificação automatizada integrada PASS:
App 273,020 s; frontend 448 arquivos / 5.578 testes; build/vet, TypeScript/Vite
e lint focado. Aceites físicos/NVDA pendentes na seção127.
Mermaid continua fora; AEP permanece **In Progress**.

**Seção126 — camadas no Stream Deck contextual (22/09/2026):**
73 IDs elegíveis: 70 comandos de workspace e `layer.activate`, `layer.toggle`
e `layer.back`, além das apresentações LOCAL_UI já suportadas. Quatro campos
visuais nas superfícies do workspace; perfil isolado sem ramo local conserva
o caminho nativo. Uma oferta física de uso único chega à API
`ExecuteContextualDeckLayerCommand`, sem reserva UI; argumentos, regra e
escopo vêm do host. A pilha é compartilhada pelas teclas do mesmo dispositivo.
Um predicado em memória dentro de `GenerationTx`, com comparação de versões
publicadas/snapshot protegida até a claim, protege a admissão; a própria
publicação que invalida o mapa não apaga sucesso confirmado. Checks focados
PASS; backend focado de camadas Deck PASS (59,409 s), regressão de claim A-B-A
da paleta PASS (23,835 s), `go vet` PASS e bindings Wails gerados.
Backend amplo PASS (358,565 s), em base anterior às últimas correções de
claim do teclado/condições nativas. Build frontend com TypeScript PASS
(Vite 1 min 3 s); suíte frontend completa PASS (447 arquivos/5529 testes,
87,59 s), após tornar determinístico o relógio do teste de prazo.
Regressão backend final PASS (249,874 s), incluindo guarda do mapa até a claim
e preservação de condições nativas. Páginas/Mermaid e aceites manuais pendentes.
Não encerra o AEP, os 84 itens históricos ou BASE-PRONTA.

**Seção125 — Stream Deck contextual do workspace (21/09/2026):**
70 comandos de workspace (72 da paleta menos as duas mutações Mermaid)
admitem foco, tipo de tela, aba e perfil, em união com apresentações LOCAL_UI.
Um único evento físico seleciona um único ramo: o local executa na UI sem
roundtrip de execução/ledger; o durável consome oferta opaca vinculada ao
evento físico do host e ao snapshot, de uso único e TTL de 10 segundos.
Camadas, mutações de páginas e Mermaid ficam fora desta ampliação; condições
visuais não se misturam a processo/dispositivo. Testes focados e backend
registrados na seção125. Suíte frontend completa (445 arquivos/5480 testes),
build frontend com TypeScript, build/vet de `internal/app` e diff check PASS;
regressão backend ampla PASS (194,531 s); aceites manuais pendentes.
O AEP permanece **In Progress**, sem encerramento dos demais gates.

**Seção124 — ações de camada na paleta contextual (21/09/2026):**
ativar, alternar e voltar camada admitem foco, tipo de tela, aba e perfil a
partir das quatro superfícies do workspace. A seleção conserva os argumentos
persistidos e o executor de camada existente; não aceita regra ou alvo da UI.
A UI valida a origem antes da submissão; o backend revalida mapa, sessão,
snapshot e alvo. A publicação que invalida o próprio mapa não apaga o sucesso
já confirmado. Não amplia condições visuais do teclado/Deck nem habilita
camadas contextuais nas páginas sem aba. Qualificação na seção124; In Progress.

**Seção123 — paleta contextual nas páginas (21/09/2026):** duplicar, excluir
e limpar listas; duplicar, excluir e ativar perfis passam a admitir condições
de foco, tipo de tela e perfil. A origem visual é a página real, não a aba
de fundo. Duplicar/limpar também admitem a lista aberta no workspace.
O alvo continua capturado e validado pelo protocolo de mutação existente.
Não admite `surface.id` neste grupo nem expõe criação/edição dentro de modais.
Qualificação e pendências na seção123 da tasklist; status **In Progress**.

**Seção122 — paleta contextual nas abas (21/09/2026):** extensão das condições
visuais para ações de mensagens, limpeza de conversa, terminal, formatação e
arquivos do editor. Cada grupo mantém preparação, decisão e conclusão próprias.
Arquivos admitem continuação após diálogo nativo somente para o alvo já
preparado, com sessão, gerações, prazo e snapshot canônico ainda válidos.
Não habilita condições visuais para mutações das páginas de listas/perfis,
ações de camada ou ações duráveis do Stream Deck. Qualificação e aceites na
seção122 da tasklist de conclusão; o AEP permanece In Progress.

**Seção121 — paleta contextual com efeito durável (21/09/2026):**
condições de foco, tipo de tela, aba e perfil para as dez ações de criação e
fechamento de abas, criação de workspace, abertura de chat e modos do editor
que usam o coordenador de commit do workspace. A UI preserva a origem anterior
ao picker; o backend valida o alvo canônico e continua sendo o único escritor.
Não amplia o suporte visual dos demais handlers duráveis nem do Stream Deck.
Qualificação e pendências na seção121 da tasklist de conclusão.

**Seção120 — condições visuais no Stream Deck (implementado, 21/09/2026):**
projeção canônica dos comandos de apresentação local por tecla, com decisão
visual síncrona na UI, editor por origem e validação de contexto. O dispositivo
apresenta os comandos potenciais, sem inferir foco no backend. Ações duráveis
com condições visuais e o aceite físico/NVDA continuam pendentes. Evidências
de qualificação na seção120 da tasklist.

**Seção119 — condições visuais na paleta (implementado, 21/09/2026):**
projeção de condições para comandos de apresentação local, com decisão de
camadas no resolvedor comum e revalidação visual na UI. Não habilita condições
visuais para efeitos backend nem para o Stream Deck por inferência. Evidências
e pendências registradas na seção119 da tasklist; aceite manual permanece aberto.

**Seção118 — condições físicas e ações de camada (21/09/2026):**
editor por origem, dispositivo pelo nome e ações `layer.activate`, `layer.toggle`
e `layer.back` no fluxo comum de teclado, paleta e Stream Deck. Regressões de
backend, frontend e defaults por API passaram. Permanecem condições visuais nas
demais origens e aceite manual; não declara R02/R09/R10 encerrados. Evidências
e pendências na seção118 da tasklist de conclusão.

**Seção117 — perfil no teclado local (21/09/2026):** projeção por perfil em
memória nas quatro abas do workspace, composta com tipo/identidade da aba;
observação validada pelo backend e lease local invalidada por transições A-B-A.
Seleção de perfil pelo nome e ajuda de atalhos usam o contexto efetivo. Evidências
e limitações na seção117 da tasklist; demais fatos/origens e aceites continuam abertos.

**Seção116 — condições operacionais (21/09/2026):** teclado por aba específica,
com seleção pelo título, composição com regras e barreiras explícitas; o host
valida tipo/identidade e mantém a revalidação até o efeito. Resolução por perfil
nas origens suportadas usa a fonte do workspace e uma versão transitória
preservada entre preparação e revalidação, sem nova tabela ou auditoria por
tecla local. Stream Deck reavalia por pressionamento e atualiza/limpa rótulos
sem reconectar ao trocar de perfil. Diagnósticos distinguem fatos suportados por origem. Evidências,
limites e aceites restantes na seção116 da tasklist; não encerra todo R02/R09.

**Seção115 — ciclo manual de camadas (21/09/2026):** configuração e execução
de regras persistentes, de sessão e temporárias; fixar, alternar, desativar e
retornar à última ativação do escopo. Expiração host-side com deadline também
no mapa local, preservação do prazo ao editar e distinção entre troca de
workspace e restart. Roteiro manual acumulado em `docs/content/recursos/COMANDOS.md`.
Não fecha por extensão todos os fatos contextuais, origens ou gates R02/R09.

**Seção114 — gravador de sequências (21/09/2026):** o controle compartilhado
agora grava duas etapas para teclado local nas configurações global e do
workspace. Captura atômica, anúncios acessíveis e cancelamento preservam o
valor anterior; edição mantém v2. O gravador não tem timeout; o executor
mantém 1,5s. Roteiro manual atualizado, aceite NVDA acumulado com os demais
lotes. Backend, catálogo e banco inalterados; evidências na tasklist.
Frontend completo **431 arquivos/5.181 testes**, recorte de integração,
TypeScript, lint e build frontend **PASS**.

**Seção113 — teclado contextual e sequências (21/09/2026):** implementação
validada automaticamente. Regras por tipo de superfície passam a contemplar
ações backend/auditadas em abas canônicas, além das ações locais de interface.
O host revalida a aba; foco e lease continuam sob autoridade da UI. Sequências
v2 preservam a origem entre os passos e usam a mesma resolução. O aceite
manual será acumulado com os lotes anteriores; nenhuma nova contagem global.
App, cinco domínios e frontend completo (431 arquivos/5.165 testes) PASS.
A limitação do gravador a combinações simples foi removida na seção114;
demais limites e evidências estão na tasklist.

**Seção112 — integração oficial (21/09/2026):** bindings de configuração
escopada gerados pelo Wails e validados através dos wrappers reais. TypeScript,
frontend completo (431 arquivos/5.154 testes) e contratos backend passaram.
Envio compartilhado usa a classe oficial ChatParams, preservando correlação.
Aceite no aplicativo/NVDA continua pendente; nenhuma nova contagem global.

**Seção111 — configuração por escopo e regras (21/09/2026):** editor global e
do workspace, personalizações reversíveis, prioridades, regras e revisão de
defaults ligados ao serviço confirmado. O snapshot de edição é revalidado
antes do commit; publicação preserva voz/jobs e atualiza o mapa local. Regras
síncronas participam do resolvedor em memória sem ampliar a especificidade
do binding. Diagnósticos deixam explícitos conflitos e limites de adapters.
Catálogo v38 **146/61/67** inalterado. Evidências e limites na seção111 da
tasklist; isso não encerra imagem/estados do Deck, portabilidade avançada,
todos os ciclos de ativação ou o aceite físico/NVDA do AEP.

**Seção110 — perfis dinâmicos e duração de jobs (21/09/2026):** o alvo de
profile é preparado pelo resolvedor existente, vinculado ao grant por expressão
e geração, e conferido novamente contra os inputs reais antes de cada tentativa.
Mudança de alvo, revogação/regrant ou template vazio recusam sem herança implícita.
O handler de job pode delegar o prazo após o handoff ao runtime existente;
preparação/decisão/fila permanecem limitadas, e cancelamento, lifecycle, epochs
e deadline do chamador continuam válidos. Outros handlers não recebem essa
exceção. Catálogo v38 **146/61/67**, sem novos atalhos; gates e evidências na
seção110 da tasklist. Aceites físicos permanecem separados.

**Seção109 — ingresso comum de voz e jobs (21/09/2026):** os callbacks
nativos passam a entregar ocorrências privadas ao resolvedor/executor, com
bindings derivados da configuração autoritativa de perfil e job. Voz recebe
handoff único vinculado a perfil/superfície; jobs usam o runtime existente após
admissão da interface e confirmação. Catálogo v37 **146/61/67**: os dois novos
comandos são exclusivos de `keyboard.global`, sem novos atalhos locais.
Na entrega109, perfil dinâmico e orçamento compartilhado de cinco minutos
ainda eram lacunas; sua evolução está registrada na seção110. Evidências
da entrega original permanecem na seção109 da tasklist.

**Seção108 — reserva Windows compartilhada com o teclado local (21/09/2026):**
os registros produtivos de voz/jobs aguardam a exclusão da combinação no DOM
antes de registrar no sistema operacional. Retirada libera o DOM somente
depois da remoção nativa. A ponte permanece ativa fora do login e é compartilhada
com a barra de ferramentas, sem consulta ao backend por tecla. Isso implementa
a coordenação dos observadores, **não** a publicação dos bindings nem a troca
dos callbacks de voz/jobs pelo executor comum. Catálogo v36 **144/61/67**
inalterado; aceite físico e demais gates na seção108 da tasklist.

**Seção107 — contrato global e exclusividade de registro (21/09/2026):**
gramática de acorde `keyboard.global` v1 disponível na projeção; o registro
nativo reserva cada combinação exclusivamente durante registro e retirada.
O handler de jobs exige preparação confiável para preservar o trigger de
hotkey e sua condição. Isso ainda não liga voz/jobs ao executor no produto:
faltam publicação autoritativa dos bindings e sincronização de ownership
com o teclado local. Catálogo v36 **144/61/67** inalterado; gates na tasklist.

**Seção106 — backend Windows de hotkeys (21/09/2026):** registro Win32 próprio
transporta `MOD_NOREPEAT` sem truncamento, cancela entrega pendente e encerra
a inscrição sem esperar release. Operações nativas usam a mesma thread dedicada.
Isso fecha a implementação da borda nativa da seção105, não a migração dos
ingressos voz/jobs para `keyboard.global`. Validação física permanece pendente;
catálogo v36 **144/61/67** inalterado. Evidências na seção106 da tasklist.

**Seção105 — preparação dos ingressos de voz/jobs (21/09/2026):** correções
dos caminhos existentes precedem a troca para o executor comum. Callbacks de
perfil têm geração e são aposentados antes de recarregar a configuração;
desativar input de voz impede registro. A publicação de `keyboard.global`,
projeção dos triggers existentes e substituição integral dos ingressos ainda
não estão concluídas. Catálogo permanece v36, 144/61/67; sem recontagem dos84.
Gates e evidências na seção105 da tasklist.

**Seção104 — Histórico e lista no workspace (20/09/2026):** catálogo v36
**144/61/67**. Ctrl+N do Histórico usa binding contextual de retorno ao workspace;
abertura de tarefa, duplicação e limpeza da lista aberta convergem aos comandos
comuns. Limpeza é backend destrutivo com decisão e transação/CAS. N/D/Ctrl+L
são gestos fixos dos controles com requests comuns, não novos defaults globais.
Tasklist seção104 registra evidências e aceite manual pendente; sem recontagem
global dos84 critérios. As entradas abaixo são históricas.

**Seção103 — reconciliação de migração (20/09/2026):** inventário histórico
separado das pendências vigentes; broker de arquivos e mutações de perfis
reconciliados. Residuais comprovados: Ctrl+N do Histórico, Ctrl+L/N/D da lista
aberta no workspace e hotkeys de voz/perfil e jobs. Plano e gates na seção103
da tasklist. Catálogo v35 permanece 142/60/66; sem nova contagem dos 84 critérios
ou aceite manual. Esta rodada só altera documentação.

**Seção102 — mutações de perfis no executor (20/09/2026):** catálogo v35,
**142 comandos / 60 locais / 66 defaults**. Criar, editar, duplicar, excluir
e ativar perfis usam preparação contextual, fingerprint e commit coordenado
por botões, paleta, teclado pessoal e Stream Deck. O backend reivindica o
commit sob validação exata de epochs antes de invalidar a própria admissão;
o executor aguarda o desfecho por prazo limitado, sem ignorar cancelamentos
anteriores. A reconstrução do mapa ocorre depois da finalização do ledger.
Exclusão exige decisão e mantém compensação/grants da seção101. Conteúdo do
perfil não é argumento persistido nem resultado do ledger. Sem novos atalhos
padrão; baseline **58 I / 24 P / 2 N** não recontada. Gates na seção102 da
tasklist; aceite manual continua acumulado. As seções abaixo são históricas.

**Seção101 — coordenação de perfis (20/09/2026):** writers nativos do
controller passam por mutação preparada e coordenação de arquivos/grants.
O App cerca a escrita e publicação com invalidação de epochs, retira o mapa
anterior e reconstrói somente após resultado conhecido. Falhas de exclusão
preservam a compensação da AEP-0101; resultado incerto conserva a intenção.
Esta integração **não** adiciona as cinco mutações de perfis ao catálogo:
handoff específico de capability e ingressos de comandos continuam pendentes.
Catálogo **137 / 60 / 66**, baseline **58 I / 24 P / 2 N** preservados;
evidências e gates na seção101 da tasklist.

**Seção100 — mutações e lifecycle (20/09/2026):** catálogo v34
com **137 comandos / 60 locais / 66 defaults**. Quatro operações de listas
ganham preparo, CAS e transação backend; exclusão usa decisão de uso único.
Duas operações de sessão terminal ganham criação/vínculo e fechamento com
teardown protegido. UI integrada e regressão frontend de 2593 testes aprovada;
gate manual aberto na seção100 da tasklist. Base de persistência de perfis
recebeu journal e fingerprint, mas
CRUD/ativação de perfis **não** estão migrados: falta coordenar arquivo,
grants e capability epochs. Baseline **58 I / 24 P / 2 N** inalterada.

**Seção99 — interrupção contextual do terminal (19/09/2026):**
`terminal.command.interrupt` passa por reserva, comparação do alvo visível,
handoff e commit backend. Paleta, tecla pessoal, Deck e controles nativos
compartilham o efeito; Ctrl+C permanece gesto exclusivo da superfície, com
cópia de seleção preservada. Snapshot opaco prende sessão e geração de input
gerenciado, não PID de subprocesso natural; não persiste texto do terminal.
Produto v33: **131 comandos / 60 locais / 66 defaults**. Não migra criar/fechar
processos nem CRUD/ativação de perfis/listas. Corrigido separadamente o salvar
edição de lista que anunciava sucesso sem persistir. Baseline **58 I / 24 P /
2 N** preservada; gates e lacunas de domínio na seção99 da tasklist.

**Seção98 — apresentação de tarefas, perfis e terminal (19/09/2026):**
nove ações compartilham paleta, teclado local e Stream Deck: abrir criação,
abrir edição e focar busca nas páginas de listas/perfis; abrir seletor de
sessões, focar entrada e histórico no terminal. Ctrl+N passa pelo mapa
contextual nas duas páginas, preservando sequências do workspace fora delas.
Produto v32: **130 comandos / 60 locais / 66 defaults**. Não migra CRUD,
ativação de perfil nem criação/interrupção/encerramento de processos.
Baseline **58 I / 24 P / 2 N** inalterada; gates na seção98 da tasklist.
Automação consolidada aprovada: 2443 testes frontend/151 arquivos, suíte
backend de comandos, TypeScript e ESLint. Aceite manual e geração oficial
de bindings continuam pendentes; não foi executado Wails neste lote.

**Seção97 — rótulos derivados do mapa efetivo (19/09/2026):** ajuda,
menu principal, paleta, workspace, chat e editor consultam a projeção aceita
pelo teclado, sem defaults artificiais durante supressão/invalidação.
Rótulos não concedem disponibilidade: os guards de execução continuam iguais.
Sequências anunciam prefixo no menu e combinação completa na ação. Gestos
próprios de componentes permanecem distintos dos bindings configuráveis.
Sem novos comandos/defaults; produto v31 e baseline **58 I / 24 P / 2 N**
inalterados. Evidências e aceite manual na seção97 da tasklist.

**Seção96 — apresentação e navegação contextual do chat (19/09/2026):**
implementadas sete ações de foco, leitura, menu, raciocínio e threads que
compartilham o dispatcher local, sem ledger. A mensagem é capturada antes
da paleta ou menu, com isolamento de conversa/sessão/superfície; abertura de
leitura não é TTS. Sem novos defaults nem captura global de setas/Enter/R.
Produto v31: catálogo121/locais51/defaults64. Gates e evidências na seção96
da tasklist; baseline global **58 I / 24 P / 2 N** inalterada. Automação
concluída, com geração oficial de bindings e aceite manual pendentes.

**Seção95 — navegação entre regiões (19/09/2026):** implementada,
`navigation.landmark.next`, `.previous` e `.default` são apresentação local
sem ledger. F6/Shift+F6 pertencem ao mapa central; Escape permanece gesto
contextual após os componentes. A paleta captura a região de origem antes
de mover o foco, e modal só admite navegação das regiões de sua instância
topmost habilitada, sem fallback para a página de fundo. Não amplia os
atalhos de decisão nem a allowlist de repetição automática. Baseline global
**58 I / 24 P / 2 N** inalterada; evidências e gates na seção95 da tasklist.
Produto v30: catálogo114/locais44/bindings padrão64. Validação automatizada
concluída; geração oficial de bindings e aceite manual acumulado pendentes.

**Seção94 — edição de diagramas Mermaid (19/09/2026):** implementadas
abertura local e aplicação/remoção auditadas. O bloco e a
instância do documento são capturados antes da espera; confirmar não procura
outro editor. O modal Mermaid possui escopo visual fechado, distinto de
`DecisionDialog`: aplicar/remover capturam a sessão topmost e fecham somente
essa instância antes do handoff normal. Isso não amplia `DialogCommandProof`
nem declara que o backend autentica fatos do DOM. Paleta permanece fora dos
modais. Teclado e Deck preservam sua origem/reserva. Evidências e pendências
na seção94 da tasklist; baseline global **58 I / 24 P / 2 N** inalterada.
Bindings oficiais e aceite manual continuam pendentes. Os três atalhos de
confirmação são controles invariantes do formulário, não novos defaults
globais remapeáveis; associações pessoais aos comandos continuam disponíveis.
Produto v29: catálogo111/locais41/bindings padrão62.

**Seção93 — transferência chat → editor (19/09/2026):**
`chat.message.send_to_editor` transfere por padrão a mensagem selecionada
inteira em Markdown para um novo documento. Menus mantêm recortes de
código/tabela/link, formatos e destinos explícitos, com identidade e conteúdo
original da fonte capturados. A admissão prepara antes de Take; sucesso exige
ACK da aplicação real no editor, não mero enfileiramento. A transição autorizada
preserva as guardas de origem/destino e não admite retarget nem retry automático.
Falha após Take usa cancelamento e reconciliação como resultado desconhecido;
uma aba já criada pode permanecer e deve ser conferida antes de nova tentativa.
Rascunhos sem arquivo restauram o autosave; vazio/rascunho inexistente não
recebe texto padrão, e outras falhas impedem sobrescrita. Produto v28:
catálogo108/locais40/bindings padrão62, sem nova tecla padrão.
Evidências focadas e limites na seção93 da tasklist;
bindings oficiais e aceite manual pendentes. Baseline **58 I / 24 P / 2 N**
inalterada, sem conclusão integral do AEP.

**Seção92 — salvar edição de mensagem (19/09/2026):** `chat.message.edit.save`
migra o salvamento do formulário existente para preparação efêmera e commit
backend de uso único. Botão/Ctrl+Enter local, paleta, teclado configurável e
Deck preservam mensagem, base original e rascunho capturados. O commit compara
a revisão na transação, preserva metadados e recusa geração ativa sem cancelá-la.
O evento tipado atualiza somente a conversa correspondente. Texto novo durante
a espera não é descartado; resultado desconhecido não dispara retry.
Catálogo107/locais40/defaults62, sem novas teclas padrão. Evidências e gates
na seção92 da tasklist; bindings oficiais e aceite manual ainda pendentes.
O envio ao editor, ainda não migrado na seção92, é tratado na seção93 acima.
Baseline global inalterada, sem declaração de conclusão integral do AEP.

**Seção91 — ações sobre a mensagem selecionada (19/09/2026):** implementadas
e validadas automaticamente: copiar texto/Markdown, falar, abrir edição, fixar/desfixar e
excluir. Abrir edição é apresentação local sem ledger; não migra salvar a
edição. Demais efeitos exigem admissão com a mensagem capturada, sem conteúdo
no ledger; exclusão exige a decisão backend de uso único. Não migra envio ao
editor nem comandos internos de blocos. Catálogo 106, locais 40, defaults 62.
Regressão frontend 1.257 testes/62 arquivos PASS; App consolidado, vet,
TypeScript e lint PASS. Geração oficial de bindings e aceite manual pendentes.
Gates na tasklist, seção91; baseline global inalterada.

**Seção90 — envio, cancelamento e nova tentativa (19/09/2026):** migração
implementada e validada automaticamente de `chat.message.send`, `chat.response.cancel` e
`chat.message.retry`. São operações auditadas, não apresentação local.
O contrato mantém o pipeline único da AEP-0040 e exige correlação de uso
único, alvo capturado e proteção contra reenvio em resultado desconhecido.
Gates e evidências em `0103-tasklist-conclusao.md`, seção90. Não declarado
concluído: regeneração oficial dos bindings aguarda autorização; baseline
global e aceite manual permanecem inalterados.

**Seção89 — consultas da conversa (19/09/2026):** `chat.pinned.open` e
`chat.tokens.open` abrem os modais existentes de mensagens fixadas e
estatísticas de tokens por botão, paleta, teclado configurável e Stream Deck.
São apresentação local, sem ledger por acionamento e sem novas teclas padrão.
O alvo é a conversa ativa e permanece fixado; mudança de contexto invalida a
apresentação. Catálogo 97, locais 39, defaults 62. Ações internas dos modais
não são declaradas migradas. Aceite manual acumulado e baseline global
preservada; AEP In Progress.

**Seção88 — Limpar conversa (19/09/2026):** botão, Ctrl+L configurável,
paleta e Stream Deck passam por `chat.conversation.clear`, comando destrutivo
com decisão interativa no backend. A conversa é capturada antes da confirmação;
conteúdo alterado, geração ativa ou contexto obsoleto impedem a limpeza.
Mensagens, resumo e registros de ferramentas são apagados atomicamente.
Catálogo 95, locais 37, defaults 62. Não inclui envio/retry, exclusão de
mensagens individuais ou outras ações de chat. Aceite manual acumulado;
baseline 58 I / 24 P / 2 N preservada, sem conclusão integral do AEP.

**Seção87 — Markdown e templates de slides (19/09/2026):** as seis
inserções Markdown (tabela, código, Mermaid, listas e citação) reutilizam
os comandos auditados existentes, agora com captura de modelo, versão e
seleção Monaco. Onze templates `editor.slide.insert.*` entram no catálogo;
menu e criação de slide na toolbar usam o mesmo executor. Slides acrescentam
conteúdo ao documento completo; não substituem a seleção do slide rico.
Catálogo 94, locais 37, defaults 61, sem novas teclas padrão. Tabelas Markdown
fixam cabeçalho; conteúdo e parâmetros de formulário continuam efêmeros.
Aceite manual acumulado e baseline 58 I / 24 P / 2 N preservada.
Evidências e limites na seção87 da tasklist; o AEP não está concluído.

**Seção86 — inserções e células (19/09/2026):** quatro comandos de conteúdo
(`editor.format.link.set`, `.table.insert`, `.code_block.insert`,
`.mermaid.insert`) e dois de navegação local (`editor.table.cell.next`,
`.previous`). Links e tabelas usam preparação efêmera pelo formulário
compartilhado, captura da seleção e validação antes do commit. Navegação não
cria linhas nem grava ledger. Ctrl+K permanece exclusivo da paleta. Catálogo
83, locais 37, defaults 61; não há novas teclas padrão. Aceite manual
acumulado, baseline global preservada; evidências e limites na seção86 da
tasklist. Markdown, templates de slides e edição Tab nativa não são
declarados migrados por este lote.

**Seção85 — blocos e tabelas (19/09/2026):** mais 25 ações no executor
de formatação: parágrafo, H1–H6, listas, citação, código, limpar marcas,
remover link e 12 alterações de tabela. Paleta, menu e Deck preservam o alvo
capturado, incluindo CellSelection. Onze defaults existentes do TipTap migram
para bindings configuráveis; Ctrl+Alt exige observação explícita de Ctrl e
Alt esquerdo, sem liberar AltGr ou eventos ambíguos. Catálogo 77, locais 35,
defaults 61. Inserção/edição de link com diálogo, criação de tabela com
dimensões e navegação de células continuam fora deste lote. Aceite manual
acumulado, sem alteração da baseline global; evidências na seção85 da tasklist.

**Seção84 — formatação rica (19/09/2026):** negrito, itálico e tachado
entram pelo executor UI auditado, com Ctrl+B/I/Shift+X, paleta, menu e Deck.
A seleção e a instância são capturadas antes da espera; mudança de documento,
seleção ou contexto cancela, sem retarget. Conteúdo não entra na auditoria.
Catálogo 52, locais 35, defaults 50. Testes e aceite manual separado na
seção84 da tasklist; baseline global preservada.

**Seção83 — arquivos do editor (19/09/2026):** `editor.file.open`,
`editor.file.save` e `editor.file.save_copy` usam preparação efêmera e commit
no broker, com os defaults Ctrl+O, Ctrl+S e Ctrl+Shift+S no contexto editor.
Paleta, menu e reserva Stream Deck compartilham o mesmo percurso. Conteúdo
não integra argumentos auditáveis nem resultado persistido. O diálogo tem
prazo total de cinco minutos; o commit mantém limite próprio de 35 segundos.
A perda de foco causada pelo diálogo não invalida um handoff de teclado já
tomado; sessão, configuração, epoch e snapshot do workspace continuam exigidos.
O receipt valida identidade e bytes antes da substituição, sem prometer CAS
interprocessos contra a janela entre validação e rename. Abertura seleciona
uma aba existente do mesmo arquivo ou cria outra, sem substituir o documento
fonte. Catálogo 49, locais 35, defaults 47. Evidências e pendências na seção83
da tasklist; aceite manual e baseline global não são promovidos por contagem.

### Estado reconciliado — 18/09/2026

**Reconciliação após a seção 75 (registro 76):** 58/84 critérios finais com
implementação identificada (**69,0% por critério, não por esforço ou aceite**),
24 parciais e 2 sem a funcionalidade de produto prevista. Saídas maiores:
11/48 aceitas, 5 implementadas sem aceite, 29 parciais e 3 não implementadas.
A+I = 16/48 (33,3%); **1/12 gates aceito (R04)**. Os estados e evidências por
item estão na [tasklist vigente](0103-tasklist-conclusao.md).
Nenhum aceite final C foi presumido pela revisão documental; implementação,
validação e entrega integral permanecem medidas distintas. Os 53/84 itens
I marcados são somente históricos e não entram nessa porcentagem.
C84 é a cláusula já aprovada de apresentação sem auditoria por tecla; não
renumera C01–C83 nem acrescenta escopo novo. **AEP In Progress**.

Lote79: migração dos menus Arquivo/Formatar/Modo, seletor de slides e
fullscreen do editor. Alt+S/F5 no mapa configurável; cinco comandos locais,
com instância do editor revalidada e sem ledger de abertura. Na fotografia
histórica da seção79, eram 42 comandos e 34 locais. Alt+I permaneceu
legado por colisão contextual com importação; salvar/abrir arquivos, mudar
modo e editar conteúdo não foram classificados como apresentação. Evidências
e aceite manual do lote na seção79 da tasklist; contagem global preservada.
Validação automática: 481 testes frontend, tipos, lint, App ampliado e vet
PASS. Aceite físico/manual pendente, sem fechamento integral de R07.

Registro histórico da seção80: `editor.menu.insert.open` foi incorporado à apresentação contextual
do editor. Alt+I abre Inserir somente quando o editor ativo, visível e apto é
capturado; fora dele, `navigation.data.import.open` continua sendo a ação de
Importar. Não há fallback para listener legado nem seleção/escrita de conteúdo
por esse comando. Catálogo: 43 comandos, 35 locais, 41 defaults (37 v1 +
quatro v2) e 40 combinações efetivas. Frontend 456/11 PASS, `tsc` e ESLint PASS;
Backend: suíte do resolvedor, regressão ampliada de comandos App (104,529 s)
e `go vet` PASS. Aceite manual do editor permanece pendente; salvar/abrir
arquivos, mudar modo e editar conteúdo ainda não haviam sido migrados.

Seção81: `editor.mode.markdown`, `editor.mode.rich` e `editor.mode.view`
migram Alt+1/2/3 e as escolhas do menu para o commit durável, também usado
pela paleta e pelo Deck. Somente `displayMode` é persistido, com alvo
versionado, CAS e aplicação visual após confirmação. Catálogo 46, locais 35,
defaults 44 (40 v1 + quatro v2), 43 combinações. Evidências e checklist manual
na seção81 da tasklist; sem promoção automática de critérios globais.

Lote seguinte (tasklist, seção 78), autorizado após a inspeção 77:
`chat.model.open`, `chat.history.open` e `chat.profile.open` usam teclado,
paleta e Deck, com defaults Ctrl+M/H/P e os pickers existentes. O chat modal
tem escopo fechado de apresentação topmost, separado do scope de decisão.
A paleta captura a instância antes da busca; nenhum comando escolhe modelo,
perfil ou conversa nem grava ledger pela abertura. Catálogo: 37 comandos,
29 locais e 38 bindings (34 v1 + quatro v2). Ctrl+L permanece legado.
Aceite manual pendente; contagem global acima preservada.

**Seção 75 implementada e validada automaticamente:** `workspace.chat.open` migra Ctrl+Shift+I, paleta,
Stream Deck e botões existentes para o executor contextual. Preparação dos
adapters permanece local; criação/vínculo passam pela fronteira autenticada
com snapshot versionado e validação de proprietário. Reutiliza conversa
existente; falha ao persistir vínculo compensa somente a conversa recém-criada.
Em aba de chat, só foca a entrada. Não envia mensagens automaticamente.
Apresentação ocorre após sucesso e releitura do snapshot pela API existente,
sem transportar seleção/conteúdo em argumentos ou resultados persistidos.
Readonly, composição, cancelamento e troca de contexto bloqueiam abertura
indevida; Ctrl+Shift+I permanece reservado contra DevTools mesmo suprimido.
Catálogo atual: 34 comandos, 26 locais e 35 bindings (31 v1 + quatro v2).
690 testes frontend em 22 arquivos, tipos, lint, App ampliado, workspace e vet
PASS. Evidências na seção 75 da tasklist; aceite manual em lote pendente.
Não promete atomicidade SQLite/YAML em queda do processo. AEP In Progress.

**Seção 74 implementada e validada automaticamente:** `workspace.create` migra Ctrl+Shift+N, paleta,
Stream Deck e a ação Novo workspace do menu para uma escrita contextual.
O snapshot versionado do workspace/aba protege a **origem**, não designa o
workspace a criar. A ação pode partir de qualquer tela com sessão/workspace
prontos; mudanças de contexto antes do commit cancelam a solicitação.
Reutiliza admissão, ledger e handoff das escritas de workspace; o nome legado
da porta de transporte `CommitWorkspaceTabCommand` não escolhe a operação.
O novo workspace e seu índice são persistidos com erros explícitos e
compensação em falha anterior à publicação. Não troca o ativo nem `LastOpened`.
Não há promessa de transação entre arquivos resistente a queda do processo.
Índice ausente/inválido recusa a criação; falha de publicação preserva os bytes
anteriores e compensa somente o diretório novo. Os dois menus de workspace
aguardam fechamento e restauração de foco, preservando a origem da seleção.
Catálogo atual: 33 comandos, 26 locais e 34 bindings (30 v1 + quatro v2).
Regressão: 589 testes frontend em 17 arquivos, TypeScript, ESLint,
App amplo (37,780 s), workspace e vet PASS. Aceite manual pendente em lote.
Ctrl+Shift+I ficou fora da seção 74 e foi migrado na seção 75; AEP integral continua In Progress.

**Seção 73 implementada e validada automaticamente:** F1 passa ao mapa efetivo de `navigation.help.open`.
O contrato de apresentação preserva uma exceção fechada: somente ajuda acionada
pelo teclado local pode navegar durante um modal, inclusive quando remapeada.
Não vale para outro comando associado a F1, paleta, Stream Deck ou execução
backend; não altera a prova de `decision.respond` na bridge. Autenticação,
proprietário, sessão, foco e composição continuam obrigatórios. Sem ledger.
F1 sem modificadores é admitido; outras teclas sem modificadores continuam
recusadas no ingresso simples. Supressão não tem fallback legado.
Catálogo permanece com 32 comandos e 26 locais; mapa passa a 33 bindings
(29 v1 + quatro v2). Ao concluir a seção 73, Ctrl+Shift+N/I ainda não eram comandos migrados: criação
de workspace e criação/vínculo de conversa exigem fronteira durável própria.
Corrigida também a abertura atrasada do chat: troca de workspace/aba/modal,
substituição de adapter ou nova solicitação invalida a apresentação pendente,
inclusive após sair e voltar ao contexto anterior. Isso não desfaz escrita
backend já iniciada. Regressão: 516 testes frontend em 15 arquivos PASS;
testes App focados/amplos, tipos, lint e vet PASS. Aceite manual pendente.

**Seção 72 implementada e validada automaticamente:** Ctrl+K, Alt+E e Alt+I migram para defaults
do mapa efetivo e comandos de apresentação local, sem auditoria por tecla.
Abrir a paleta e os fluxos de Dados não autoriza importação/exportação de dados.
O aceite manual das seções 71–72 será feito em lote, por escolha do mantenedor.
F1 e demais atalhos contextuais ficam explicitamente fora deste recorte.
Catálogo atual: 32 comandos, 26 locais, 32 bindings (28 v1 + quatro v2).
474 testes frontend em 14 arquivos, TypeScript, ESLint, regressões backend
focadas e `go vet ./internal/app` PASS. Sem Wails, ACP, PTY real ou banco real.

**Seção 71 implementada e validada automaticamente:** migração de Ctrl+N seguida de C/E/R/T e do
menu de criação. Sequências de dois passos usam documento `keyboard.local`
v2 (`version: 2`, `steps` com dois objetos `code`/`modifiers`); atalhos v1 e
seus fingerprints permanecem inalterados. O prefixo exige Control, Alt ou Meta;
o segundo passo não tem modificadores e Escape é reservado para cancelar.
O prefixo mantém apenas estado local por 1.500 ms, sem invocação/ledger.
O segundo passo resolve um binding completo e passa pelo executor contextual.
Seleção pelo botão/menu é uma paleta especializada, não um evento de teclado
sintético. Após a seção 71, o mapa padrão continha 29 bindings: os 25 v1 preservados e quatro
sequências v2. Ao usar setas/Enter, o menu compartilhado assume a seleção;
a segunda tecla física continua usando o ingresso keyboard.local.
Evidências: 449 testes frontend em 14 arquivos, TypeScript, ESLint focado,
regressão App/commandconfig e `go vet ./internal/app` PASS. Sem Wails, ACP,
PTY real ou banco real. Aceite manual com NVDA pendente; roteiro e gates na
seção 71 da tasklist e em `docs/content/recursos/COMANDOS.md`.

**Seção 69 implementada e aceita manualmente:** o mantenedor confirmou a fluidez dos comandos
e o Stream Deck do recorte 68. A paleta passa a usar o Combobox compartilhado
com o picker de modelos, em substituição ao Menu com busca, e Ctrl+K admite
campos de texto nativos. Aceite do usuário: “pode continuar. validamos. se
aparecer algo errado arrumamos.” O relato não é uma nova medição de latência
nem aceite integral dos 84 itens.
Validação desta correção: 314 testes frontend em 15 arquivos, TypeScript e
ESLint focado PASS. Pickers existentes preservados; sem execução Wails/Go.

### Seção 70 — `workspace.tab.terminal.create` implementado

`workspace.tab.terminal.create` é o recorte implementado e pré-requisito para
tratar Ctrl+N como uma migração segura. O frontend integrado implementa as três origens previstas,
com admissão contextual e as proteções de owner, sessão e workspace:
`terminalId` identifica a sessão viva. Na seção 70, o catálogo tinha 29 comandos, 23 de
apresentação local e 25 bindings (29 bindings após a seção 71). O evento `session_created` é deduplicado sem perder o histórico; uma
surface existente recarrega a sessão quando perde o evento, sem recriá-la. A
criação frontend usa commit único e invalida owner, sessão ou workspace
obsoletos. O backend usa manager real, preserva o cwd, aplica guards de
snapshot/auth, CAS e compensação; testes fake cobrem lifecycle e erros de join.
Ctrl+N ficou fora da seção 70; sua migração está na seção 71. O aceite manual do terminal permanece
pendente; PTY real e end-to-end físico das três origens não foram testados.

Evidência atual: frontend **317 testes em 10 arquivos PASS**, TypeScript e
lint PASS; pacote `workspace` completo PASS (1,716 s) e `go vet` de App/workspace
PASS. Regressão App final: PASS (53,621 s) no recorte de comandos; fixture de
readiness com manager ausente foi corrigido. Não foram executados Wails, ACP,
PTY real ou banco real. Só o aceite manual do novo terminal permanece pendente.

**Revisão aprovada pelo mantenedor: política por efeito (seção 68 implementada
e validada automaticamente; aceite manual pendente).** Navegação/apresentação local deixa de exigir invocação,
ledger, auditoria ou handoff persistente por acionamento. Esta decisão
substitui, para essa classe explícita, as exigências universais anteriores.
Persistir a última seleção não equivale a registrar cada tecla. Operações de
domínio e efeitos sensíveis mantêm suas validações e proteções próprias.

Na seção 68: 28 comandos, sendo 23 de apresentação local; mapa padrão com
25 combinações. Ctrl+Tab/Shift+Tab/PageUp/PageDown/1…9 agora usam o novo mapa,
sem fallback legado concorrente, com repeat apenas na navegação. A última
seleção é persistida de forma coalescente; criar/fechar aguarda essa seleção e
revalida o alvo, sem retargeting. Testes cobrem falha/reconciliação, logout com
gravação pendente, Escape e recuperação de foco sem roubar foco externo.
541 testes frontend (25 arquivos), regressões backend focadas, TypeScript,
ESLint e vet PASS. Não constitui medição de latência com IPC/render nem aceite
NVDA/Stream Deck. Critérios e evidências atuais: seção 68 da tasklist.

Seção 67 substituída pela seção 68: permanece a aprovação de repetição
automática seletiva para navegação (D3), mas não a tentativa de processar cada
ocorrência pelo ledger. As evidências abaixo são históricas; o fechamento da
solução local e seu gate manual atual estão na seção 68 da tasklist.

Seção 66, entrega parcial validada: família de 11 comandos de navegação de abas nas três
origens e ordenação de snapshots de transporte. Catálogo 28, mapa padrão 12.
Alvos explícitos são comparados à resolução autoritativa; os atalhos rápidos
legados não foram substituídos. Gate de ocorrências consecutivas e latência
integrada ainda aberto. Frontend 604 testes (17 arquivos), regressões backend,
TypeScript, ESLint e vet PASS. Amostra backend: mediana 26,28 ms, p95 27,74 ms,
sem IPC/renderização; não comprova orçamento de 1 ms. Atualizações locais
legadas fora dos snapshots permanecem explícitas na tasklist. AEP In Progress.

Seção 65: `workspace.tab.close` com Ctrl+W/Ctrl+F4, paleta e Deck. Alvo ativo
versionado, fechamento/substituição da última aba em uma gravação, sem criar
conversa ou excluir recursos. Foco pós-sucesso limitado à sucessora prevista;
sem fila tardia. Catálogo 17, mapa padrão 12. Regressões App/workspace PASS;
frontend 351 testes integrados e mais três casos de foco PASS (354 distintos).
Aceite manual pendente; troca de abas, terminal e sequências não migrados.
AEP permanece In Progress.

Seção 64: criação contextual de editor e lista de tarefas acrescentada ao
contrato de chat. Catálogo de 16 comandos; dez atalhos padrão preservados.
Novos tipos usam paleta e bindings pessoais de teclado/Deck, sem aceitar tipos
arbitrários. Handoffs concorrentes são isolados por ID. Frontend: 312 testes
em 12 arquivos PASS; regressões backend App/workspace, TypeScript, ESLint e
go vet PASS. Terminal, sequências
Ctrl+N e fechamento/troca de abas não migrados; aceite manual dos novos tipos
pendente. AEP In Progress.

Seção 63: navegação conhecida permite input/textarea nas entradas de teclado e
Stream Deck. Exceção explícita à exigência de composição `inactive`: somente
esses comandos de navegação podem aceitar `unknown` em campo nativo, sem
reinterpretá-lo como `inactive`. Composição `active` continua recusada; contexto,
owner, foco, modal, superfície e revalidação continuam obrigatórios. Não se aplica
a mutações, comandos contextuais, IDs futuros, Monaco ou contenteditable.

Exceção posterior aprovada pelo mantenedor em 24/09/2026: a navegação local
entre abas do workspace também pode partir do editor Monaco ativo, preservando
Ctrl+Tab/Ctrl+Shift+Tab e Ctrl+PageUp/PageDown da main. Essa autorização é
restrita à família de troca de abas; não amplia a navegação global, mutações
nem comandos futuros. Composição IME ativa, modais, contexto obsoleto e
bindings suprimidos continuam bloqueando. O mapa efetivo e remapeamentos
continuam autoritativos, sem listener legado paralelo.
O usuário confirmou o funcionamento da criação de chat e a restrição ao contexto
autorizado; reportou o bloqueio de navegação em texto tratado nesta seção.
Validação: 218 testes frontend (9 arquivos), TypeScript e ESLint PASS.
Aceite manual desta correção pendente; AEP permanece In Progress.

Seção 62: criação de aba de chat com alvo versionado, commit de escrita backend
e Ctrl+T no mapa padrão. Paleta e Deck compartilham o contrato. Frontend 263
testes PASS; regressão final integrada das origens backend PASS (App, 45,221s).
Aceite manual pendente; demais tipos de aba/atalhos não migrados. In Progress.

Seção 61: integração contextual de `workspace.panel.focus` com alvo visual
capturado e revalidado para paleta, teclado pessoal e Stream Deck. Somente
foco imediato em painel pronto; criação/fechamento de abas e chat contextual
assíncrono continuam pendentes. Matriz frontend 354 PASS, testes backend
focados, TypeScript, ESLint e vet PASS; aceite manual pendente. In Progress.

Seção 60: nomes das camadas passam a Comandos padrão e Mapa de teclado padrão,
sem mudança de identidade. Migração de workspace/abas parada, conforme pedido,
nas lacunas de execução assíncrona e teclado contextual descritas na tasklist.
Nenhum comando adicional declarado migrado; AEP permanece In Progress.

Seção 58: por decisão explícita do usuário, a configuração física passa a
capturar a tecla pressionada, sem seletor de aparelho, serial ou entrada manual.
Identidade permanece interna à persistência; captura temporária não executa
comandos, expira e é invalidada pela sessão. Substitui a UX da seção 57.
Aceite manual da nova captura pendente; AEP permanece In Progress.

Seção 57: usuário confirmou sucesso do fluxo físico básico no App. A tela
passa a receber descoberta read-only de modelos/seriais/geometrias, inclusive
sem bindings, preservando entrada manual. A seleção detectada e os cenários
manuais de reconexão/lock/restart não são presumidos aceitos. In Progress.

Seção 56: aceite manual da seção 55 confirmado pelo usuário (paleta,
Alt+C e configurações funcionando). Integração de `navigation.menu.open`
com Alt+M e primeiro ingresso de Stream Deck para navegação/ajuda, por
bindings globais de camada pessoal. Driver e executor compartilham a sessão
autenticada; não há injeção de teclas nem execução física disfarçada de paleta.
Recorte e validações na tasklist; aceite básico posterior na seção 57.

Seção 55: o novo log registra `SQLITE_BUSY` nos dois bootstraps, durante
restauração de claims e preparação do escopo. Essas transações passam a
usar retry limitado da política SQLite existente. A restauração espera fora
do gate, revalida a mesma sessão e captura epochs novos por tentativa;
cancelamento e falha persistente permanecem fechados. Não há retry de
execução de comandos. Evidências na tasklist; aceite manual ainda pendente.

Seção 54: o aceite manual da seção 53 falhou (paleta 11/0, teclado e
configurações indisponíveis). Reproduzida a corrida entre autenticação e a
primeira observação do SO: a recusa inicial permanecia após o unlock porque
o monitor descartava mapas sem reconstruí-los. Reconstrução autenticada
cancelável fora do recebimento de eventos e notificação após readiness
restabelecem o percurso; lock/falha do monitor continuam fechados.
Evidências e limites na seção 54 da tasklist; aceite real ainda pendente.

Seção 53: reproduzida a paleta com 11 comandos e nenhum disponível quando
a projeção contextual fica obsoleta. A consulta agora usa a mesma atualização
de projeção da execução, preservando os bloqueios de sessão e cofre.
Migração de oito combinações de navegação para a camada padrão de teclado,
sem fallback legado; supressão/restauração e prioridade de camada pessoal.
F1, menu, importação/exportação e atalhos de abas não migram nesta rodada.
Aceite manual continua pendente; evidências na seção 53 da tasklist.

Seção 50: correção do bootstrap após salvar configurações. A verificação
de prontidão reconhece os formatos reais de fingerprint: digest hexadecimal
canônico (versão vinculada ao domínio HMAC) e `vN:digest`. Todas as versões
de chave registradas continuam obrigatórias; nenhuma chave ou receipt é
recriada/reescrita, nem uma decisão é autorizada por essa leitura. Diagnóstico
fechado de falha e UI que distingue erro de carga de teclado indisponível.
Reprodução e regressão em bancos de teste; reinício no banco do usuário
ainda requer aceite. Não se amplia o escopo funcional da seção 49.

Seção 49: primeiro ingresso de teclado local conectado ao mapa ativo e ao
picker compartilhado, para `workspace.list`. Resolução em memória, origem
`keyboard.local` no executor comum e invalidação de gerações; nenhuma chamada
ao backend para combinações ausentes do mapa. Exige Control/Alt/Meta, fora
de campos editáveis e modais, sem conflitos nem condições contextuais.
Não migra os atalhos legados nem habilita teclado global/Stream Deck.
Geração Wails e aceite manual continuam pendentes; D3/D7/D15 não estão
integralmente aceitos. Evidências e limites na seção 49 da tasklist.

Seção 48 (histórico): ativação manual das camadas globais ligada à tela, usando regras
persistentes e claims do serviço existente. Preparar não ativa; Pin/Back
atuam na origem UI autenticada e publicam novamente o mapa. A ativação
alimenta a resolução produtiva da paleta. Teclado personalizado e aceite
visual continuam pendentes; não se presume fechamento integral de D15.

Seção 47: primeiro recorte do editor D15 em **Configurações → Comandos e
acionadores**, com camadas globais, captura de teclado, persistência confirmada
e supressão/restauração de padrões da paleta. Geração de bindings Wails e
validação visual ficam a cargo do usuário nesta máquina. Configuração salva
não significa teclado operacional: ingresso físico e ativação de camadas
pessoais ainda não estão ligados por esta tela. D15 permanece parcial.

Seção 46: por decisão do usuário, novas extensões de portabilidade ficam para
depois do uso cotidiano. A paleta ganha nove comandos de navegação para telas
existentes, pelo handoff UI autenticado, além dos dois comandos anteriores.
Busca inclui descrição, categoria e aliases; desmontagem causada pela própria
navegação não cancela sua confirmação. Não há migração dos atalhos legados,
editor de bindings ou ingresso produtivo de Stream Deck entregue neste recorte.
O AEP permanece In Progress, sem novos aceites integrais de R/C/I.

Seção 45: exportação comum desktop conectada ao painel de Dados e às APIs
existentes, com leitura transacional consistente, autorização de escopo e
revalidação de sessão/época antes de liberar o JSON. Roundtrip público como
cópia preserva originais e cria IDs novos após confirmação. A ação sensível
continua separada e indisponível; IncludeCredentials/senha são recusados no
export comum conforme D10. R05.3 em validação integral; R05.4 parcial.

Seção 44: importação desktop conectada à fachada `ExportImport` e à tela
de Dados. O host deriva usuário/sessão, autoriza workspaces e compõe projeção
e reconciliação de ativação no commit real. Políticas manter/substituir/copiar,
renomeação e remapeamento são escolhas explícitas; o relatório distingue
persistência de publicação. Não há geração manual de bindings ou writer
paralelo. Exportação pública/sensível e aceite integral R05.3/R05.4 permanecem
pendentes. As notas anteriores abaixo preservam o histórico.

Seção 43: relatório redigido propagado do plano realmente aplicado até o
applier do App. Cópias retornam seus IDs persistidos, sem replanejamento;
Keep retorna no-op explícito e falha de rebuild preserva o relatório do
commit. Avisos são agregados por código, sem patterns ou conteúdo privado.
Corrigido o writer de camadas existentes para preservar o timestamp do
preview confirmado. A entrada desktop ainda precisa compor autenticação
local, autorização de workspace e hook de ativação; API/UX pública não foi
habilitada. R05.3/R05.4 permanecem parciais.

Seção 42: o applier interno do App compõe o lote global+workspaces com
uma reconstrução do mapa do workspace ativo após o commit. A publicação
confere a auditoria e a geração exata de cada escopo alterado, inclusive os
não ativos. Falha pós-commit retorna `Committed=true, Rebuilt=false`, sem
repetir a importação. Transporte/UX pública, relatório público e exportação
sensível continuam pendentes; R05.3/R05.4 seguem parciais. As notas abaixo
registram o estado histórico de cada rodada.

Seção 41: o backend de importação passa a aceitar lote global+workspaces
numa única transação por `ApplyCommandEnvelopeBatch`. Todos os escopos são
autorizados antes da leitura; cada diff alterado exige confirmação antes de
qualquer escrita. Recibos, CAS, dados e auditorias revertem juntos. A união
final global+workspace é validada além dos previews individuais. O applier
do App permanece unitário; montagem multi-escopo no App/UI e export sensível
ainda faltam. R05.3/R05.4 permanecem parciais, sem novo aceite integral.

Seção 40: importação interna de envelope ligada ao applier do App, com
owner derivado do token e portas SQL reais para UUIDs, nomes e patterns de
credenciais. O caminho compartilha confirmação, commit auditado, suspensão
e reconstrução das mutações comuns. Credenciais são consultadas somente por
metadados, sem descriptografia. R05.3/R05.4 permanecem parciais: transporte
público, lote multi-escopo e export sensível continuam pendentes.

Seção 39: `job_service` ganha adapter do Manager real, ligado ao contexto
privado e ao lifetime do run, com definição e grant congelados/revalidados.
O App fornece seu store de grants. O recorte é subagent com profile literal;
isso não publica uma entrada de comandos de automação nem conclui R05.1/R05.2.
Evidências e limites na seção 39 da tasklist; contagens permanecem inalteradas.

Seção 38: tools passam a transportar a origem privada de eventos do runtime,
sem criar um job intermediário. O executor inaugura a cadeia para delegações
locais diretas; camadas reativas exigem a origem verificada da projeção.
A montagem revalida essa prova antes da tool, após persistir a invocação.
Qualificação e limites estão na seção 38 da tasklist; R05.2/R03.4 continuam
parciais, sem publicação de catálogo nem novos aceites.

Seção 37: montagem interna de comando local → tool pelo executor comum,
com ID canônico, schema e geração fixados; autorização revalidada no worker
antes do efeito, depois de persistir a invocação. Teste integrado cobre
confirmação, replay sem efeito duplicado, redação e mudanças durante o diálogo.
Não publica catálogo nem habilita delegação reativa, de agents ou de profiles.
**R05.2 parcial; 11/48 R, 1/12 gates, 0/83 C, 53/84 históricos I**.
Evidências e limites na seção 37 da tasklist. Notas seguintes são históricas.

Seção 36: delegação local reativa preserva raiz e cadeias verificadas na
outbox. O App relê fontes autorizadas antes das tentativas; raiz divergente
ou fonte encerrada não vira clique manual. Dispatch não é herdado por jobs
descendentes. Teste integrado com catálogo controlado não publica `job.run`
nem fecha R05.2/R03.4. **11/48 R, 1/12 gates, 0/83 C, 53/84 históricos I**.
Evidências e limites na seção 36 da tasklist. As notas abaixo são históricas.

Seção 35: decisões desktop reais são ligadas à montagem interna do App para
jobs. `App.newCommandJobHandler` fixa owner, sessão, definição e alvo; para
`subagent` literal, valida profile/provider e exige o grant exato com seu
fingerprint e geração fixa antes da fila e em cada tentativa. Regrant
posterior não valida um handler antigo, e a decisão do comando não concede nem
substitui grant. Profile omitido mantém a herança da AEP-0101; templates
dinâmicos permanecem fora desta montagem fixa. **R05.2 parcial; 11/48 saídas
R, 1/12 gates, 0/83 C, 53/84 históricos I**. Sem publicação de `job.run`;
catálogo R07, raiz reativa e origens ainda não compostas permanecem fora desta
seção. As notas da AEP-0048 sobre `PrepareCommandJob`/`CommandHandler` e da
AEP-0101 sobre grants exatos continuam vigentes.

Seção 34: ponte comando → runtime real de jobs implementada para usuário
local, com alvo fixado pelo bootstrap, confirmação interativa obrigatória,
revalidação por tentativa e resultado durável. Ponte de tools preserva o
profile de origem e a correlação, sem promover o alvo a chamador.
**R05.2 parcial; 11/48 saídas R, 1/12 gates, 0/83 C, 53/84 históricos I**.
Faltam composição de autorização no App, raiz reativa completa e entradas
agent/job_service/catálogo R07. Evidências e limites na seção 34 da tasklist.
As notas abaixo são históricas.

Seção 33: **R03.2 aceito; 11/48 saídas R, 1/12 gates, 0/83 critérios C,
53/84 históricos I**. Tasklists reais produzem origem interna autenticada;
hotkey passa pelo callback registrado pelo Manager. Cadeias têm validação
compartilhada e múltiplos jobs simultâneos estão qualificados no App.
R03.4 segue aberto pelo ciclo comando → novo job → evento → comando,
dependente de R05.2/R07. App e nove pacotes de infraestrutura passaram;
limites e ocorrência de cleanup do teste Go estão na tasklist. As notas
abaixo são históricas.

Seção 32: proveniência verificada da outbox acompanha as camadas selecionadas
até o envelope; o executor revalida a origem, acrescenta o comando e recusa
loops/limite antes da reserva. Auditoria conserva somente metadados estruturais.
**10/48 saídas R e 1/12 gates**, sem novo aceite C/I: ainda falta qualificar
a composição comando → job → evento → comando e multiorigem no App (R03.4),
além dos ingressos restantes de R03.2. Notas abaixo são históricas.

Seção 31: herança privada de origem/cadeia entre jobs integrada ao Manager;
retenção de runs protegidos corrigida e recuperação/rollback da outbox
qualificados. **10/48 saídas R e 1/12 gates**, sem novo aceite C/I. R03.4
continua aberto especificamente pela ponte de proveniência da claim de job
ao envelope do comando resolvido; testes isolados do limite 16 não a
substituem. R03.2 ainda requer ingressos restantes. Notas abaixo são históricas.

Seção 30: claims autorizadas de jobs agora entram no mapa efetivo do resolvedor
do App, com guard de lease/runtime/condição e refresh sem depender de
notificação. Renovação equivalente preserva comandos em andamento; caminho
estável da projeção não consulta SQLite. **10/48 saídas R, 1/12 gates, 0/83
critérios C e 53/84 históricos I**, sem novo aceite de gate. R03.2/R03.4 ainda
exigem ingressos/replay e demais provas integradas. Notas abaixo são históricas.

Seção 29: **R03.1/R03.3 aceitos; 10/48 saídas R, 1/12 gates, 0/83 critérios
finais C e 53/84 históricos I**. A fonte viva de jobs agora sobrevive a
publicações de configuração sem perder invalidação de segurança; condições,
heartbeat e ciclos independentes têm provas complementares. R03 permanece
aberto: projeção das claims no resolvedor, ingressos e replay ainda pendentes.
As contagens das notas seguintes são históricas.

Seção 28: **Gate R04 aceito**, com restart de App/core/Manager sobre SQLite
temporário, recuperação multiusuário/system, limpeza legada, compactação e
proteção de job vivo além do TTL inicial até sua conclusão real. O cenário
passou três repetições e revelou/corrigiu comparação UTC/local na outbox.
Contagem vigente: **8/48 saídas R, 1/12 gates, 0/83 critérios finais C e
53/84 históricos I**. Não certifica crash de processo nem encerra R03/R01
ou o AEP. Evidências e limites estão na tasklist. As notas abaixo são históricas.

Seção 27: **R04.3 e R04.4 aceitos**, com recuperação paginada multiusuário,
rollback/retomada, política imutável por passagem e diagnóstico de etapas.
Contagem atual: **8/48 saídas R, 0/12 gates, 0/83 critérios finais C e 53/84
históricos I**. R04 tem quatro saídas aceitas, mas o gate aguarda qualificação
conjunta de restart no App com todos os domínios e trabalho vivo. R03 e a
projeção das mudanças de claims permanecem abertos. As notas seguintes são
históricas; não prevalecem sobre esta contagem.

Seção 26 da tasklist: R04.1 aceito. Consumer e adapters reais são montados
antes de jobs.Start, na única cadência existente. Identidade de jobs exige
registro vivo no Manager, watch do epoch e linha persistida; terminalidade
não é prova de vida. Shutdown aguarda o join da manutenção. Contagem vigente:
**6/48 saídas R, 0/12 gates R, 0/83 critérios finais C, 53/84 históricos I**.
R03.1 e R04.3/R04.4 continuam parciais: contexto completo, projeção de claims,
qualificação multiusuário e atualização dinâmica da política ainda faltam.
Os parágrafos seguintes registram as rodadas anteriores, não a contagem atual.

Seção 25 da tasklist: manutenção inicial do coordenador migra para sua única
goroutine cancelável, fora dos locks de Start; heartbeat retoma o prefixo
confirmado após erro, e falhas transitórias não são confundidas com rejeições
definitivas de claims. Retenção informa continuação após rollback. Montagem
do Consumer com portas reais no App e ligação antes de jobs.Start continuam
pendentes; contagem permanece 5/48 saídas R, sem novo gate certificado.

Seção 24 da tasklist: recovery registrado está ligado ao bootstrap. A nova
migração v29 associa startups privados à identidade física do banco sob lock
nativo. Somente namespaces anteriores comprovados podem ser reconciliados;
geração atual, desconhecida ou de outra cópia permanece fora da prova. O App
recupera receipts e ledger/audit antes de publicar, sem reexecutar efeitos.
O reset do banco encerra comandos antes do fechamento/remoção. A qualificação
desta rodada e seus limites estão na tasklist; a cadência completa de R04 e
BASE-PRONTA ainda não estão concluídas. **Contagem vigente: 5/48 saídas R**
(R01.1–R01.4 e R04.2), **0/12 gates R, 0/83 critérios finais C e 53/84 itens
históricos I**. Testes dos pacotes afetados e vet passaram; ACP/acpregistry
falharam na suíte global e isoladamente com `0xffffffff`, causa não determinada.
O gate R01 permanece em validação de regressão, com suas quatro saídas aceitas.

Atualização da seção 23 da tasklist: troca produtiva de workspace retira a
publicação anterior, aposenta o runtime e reconstrói a configuração do destino
antes do evento à UI. Falha de leitura deixa comandos indisponíveis e permite
nova tentativa, sem desfazer a troca ou reutilizar deltas antigos. A composição
visual recebeu testes integrados. **R01.1 aceito** com ownership real,
notificação perdida, mutação concorrente e provider ausente cobertos pela
composição de provas UI/backend. Contagem naquela rodada: **3/48 saídas R**, ainda
**0/12 gates R, 0/83 critérios finais C e 53/84 itens históricos I**. R01.4
continuava aberto: faltava recuperação interprocesso segura, agora na seção 24.

Atualização da seção 22 da tasklist: **R01.3 aceito**, com ingresso público
pela composição real, suppress/rejected_stale duráveis, replay e confirmação
de ledger/CAS anteriores ao handoff visual. A rodada corrige validação antes
do tracker, correlação de ocorrência e referências mutáveis entre callbacks.
Contagem naquela rodada: **2/48 saídas R**, mantendo **0/12 gates R, 0/83 critérios
finais C e 53/84 itens históricos I**. R01.1/R01.4 e BASE-PRONTA seguem abertos;
isso não habilita listeners físicos nem condições sem autoridade montada.

Atualização da seção 19 da tasklist: R01.2 tem aceite da montagem produtiva do
executor completo/projeção persistida, com diagnósticos de dependências e
execução dos handlers reais. Naquela rodada eram **1/48 saídas R**, não o
fechamento de R01 ou dos 84 itens históricos. A rodada também endurece
contexto contra ABA de aba/perfil, leases visuais aposentadas e evidência IME
de outro elemento; providers/acionadores/recovery ainda têm trabalho restante.

Execução posterior à revisão: seis frentes paralelas corrigiram importação de credenciais/escopos, retries/dead-letter, fronteira pública da bridge, restore de claims, atomicidade da montagem e invalidação na recarga. A rodada real de produto agora usa `newCommandDesktopExecutor` e `commandconfig.ProjectComplete`, com autenticação pela sessão local sem JWT, bridge assíncrona e shutdown com join. Naquela rodada, o catálogo gerava a lista internamente sem entregá-la à UI; essa limitação foi resolvida na seção 14. O `Recover` do lifecycle faz preflight global somente leitura e fail-closed; sem prova interprocesso, não reconcilia pendências. A qualificação histórica dessa rodada teve frontend com 315 arquivos e 2.966 testes PASS, mas backend com 112 pacotes PASS, 8 sem testes e `internal/acpregistry` FAIL em 8.747s (`exit ffffffff`); o binário compilou, mas sua execução direta também reprovou sem output, com causa não certificada. A rodada posterior da seção 12 passou com 3.000 testes frontend e suíte Go completa; isso não explica a falha intermitente de acpregistry. Evidências e limitações históricas estão nas [seções 10 e 11 da tasklist](0103-tasklist-conclusao.md#11-rodada-real-de-produto--16092026). Esses fechamentos locais não certificam BASE-PRONTA.

A [revisão integral](0103-revisao-integral-2026-09-16.md) e a [tasklist de conclusão](0103-tasklist-conclusao.md) são o acompanhamento vigente. A implementação já tem uma baseline real de executor desktop e projeção completa, mas ainda faltam a generalização dos providers/transporte de UI, automação/manutenção/recovery, ingressos físicos e migração dos comandos. O picker executa `workspace.list` e apresenta a lista autenticada sem persistir seu conteúdo (seção 14); `help.shortcuts.show` e os nove comandos de navegação usam handoff registrado com confirmação de resultado (seções 13 e 46). Os atalhos legados permanecem inalterados. A seção 47 registra o primeiro editor de configurações globais D15, ainda dependente da geração de bindings para uso no aplicativo e sem ingresso produtivo de teclado personalizado. A consolidação corrigiu desbloqueio presumido, reutilização de dependências obsoletas e disponibilidade de origens desconectadas; os cenários estão documentados na seção 11.

Os **84 itens de infraestrutura** do plano histórico são distintos dos **83 critérios de aceitação** ao final deste AEP. Os checkboxes históricos somam 53 marcados/31 abertos; a contagem posterior 81/84 misturou 28 fechamentos locais de critérios C com itens I e foi retirada como medida válida. Evidência de primitivas/sentinela não encerra os percursos integrados. As atualizações antigas abaixo são histórico do subconjunto entregue, não substituem o aceite integral. A divisão contextual D2/D8 foi atualizada com aprovação explícita do usuário; as demais decisões permanecem vigentes.

### Teclado local de navegação — 17/09/2026

A seção 52 registra correção do teclado do picker compartilhado: eventos
do campo de pesquisa não são novamente processados pelo menu pai. A lista
da paleta independe de atalhos personalizados e anuncia totais/disponibilidade;
os novos destinos aceitam configuração de teclas, mas não receberam novas
combinações padrão. O aceite NVDA da correção ainda depende do usuário.

A seção 51 da tasklist amplia o teclado personalizado aos nove comandos
`navigation.*.open`, além de `workspace.list`. O host publica combinação,
comando e tipo de handler; a UI não escolhe o comando no ingresso, que
resolve novamente o binding real com origem `keyboard.local`. A navegação
reutiliza reserva, autorização, handoff e confirmação de resultado da paleta.
O mapa é revalidado antes do handoff; a troca de página do próprio efeito
não cancela sua confirmação. Atalhos legados e prioridade de campos/modais
permanecem. O usuário confirmou o primeiro atalho com camada ativa; os novos
destinos ainda exigem aceite no app. Não certifica D3/D7/D15 integralmente.

### Resolução produtiva da paleta — 16/09/2026

Atualização da seção 18 da tasklist: `help.shortcuts.show` também resolve o
default/delta persistido, com reserva UI obrigatória e sem redirecionamento
para handler backend. Antes de entregar o handoff, o App revalida a admissão
após a espera. Supressão/recusa não executam o efeito visual. Essa integração
não encerra os providers, os acionadores físicos ou o recovery de R01.

A seleção backend da paleta agora resolve o snapshot real de `commandconfig`
publicado pelo lifecycle, incluindo defaults, deltas de supressão e pendências
de revisão. A bridge usa o mesmo percurso, sem bypass direto do binding. A
leitura de configuração/camadas/versões é atômica em `HostState`; as camadas
contribuintes vêm dos bindings efetivos. O dispatcher conserva autorização,
CAS e ledger existentes. A projeção global+workspace é publicada como unidade;
seu stamp volátil invalida ambos os escopos quando qualquer parte muda.

O primeiro default produtivo é a seleção `palette:workspace.list`; seu
fingerprint inclui a semântica executável e exclui apresentação visual. Essa
entrega não migra atalhos nem promove a paleta a origem física. Condições com
autoridade ainda ausente são recusadas sem fallback. Handoff de UI e origens
físicas continuam com seus contratos próprios; R01 e BASE-PRONTA seguem abertos.
Evidências da rodada ficam na seção 15 da tasklist de conclusão.

### Gate de montagem do App — 15/09/2026

A rodada de lifecycle físico de 16/09 (seção 17 da tasklist) corrigiu rollback
de abertura, falha de frame, callbacks obsoletos, concorrência de handles e
liberação após erro do driver. Ela também identificou um limite na dependência
fixada: a espera por release não possui cancelamento explícito no fechamento.
O aceite de shutdown físico e a montagem no App permanecem abertos; a prova
manual histórica não cobre remoção/fechamento com tecla pressionada.

Atualização de robustez (16/09): o controller de entrada isola prefixos por
fonte, correlaciona release/repeat com a tecla composta e invalida resoluções
em andamento nas transições de lifecycle. A identidade do evento é emitida
pelo adapter, não preservada do resolvedor. A seção 16 da tasklist registra
as regressões e seus limites: isso não monta listeners físicos no produto
nem fecha R01/BASE-PRONTA.

Após a validação física de I13.5/I13.6, I13 passa a ser contabilizado como o
terceiro pacote completo da infraestrutura. Em I14.2, `commandruntime` agora
possui `MountSpec`/`MountDependency`, exigindo manifesto explícito para
catálogo, defaults, políticas, stores, presenter, providers, dispatcher e
adapters antes de criar o controller. O App ganhou
`ConfigureCommandLifecycleMountSpec`, que falha fechado sem instalar runtime
quando alguma dependência está ausente, duplicada ou nil. Em complemento,
`ConfigureCommandLifecycleForApp` já preenche esse manifesto a partir das
dependências reais preparadas pelo bootstrap confiável: registry/handlers/store,
política/envelope, `HostState`, `commandbridge.Bridge`,
`commandcontext.FactBus`, presenter de decisão e adapter físico/entrada. O App
também tem portas padrão reais para o `commandruntime.Config`: autenticação na
sessão local atual, geração privada vinculada ao `EpochService`, commit pelo
gate de segurança, projeção a partir de `HostState.Snapshot`, enable/readiness
em memória e falha fechada quando host/config/camadas/unlock não estão prontos.
`ensureCommandLifecycleMountedForCurrentUser` liga, pós-auth, o executor desktop
real por `newCommandDesktopExecutor`, o catálogo produtivo e a projeção completa
por `commandconfig.ProjectComplete`, com providers e adapter fail-closed;
Login/RefreshAuth tentam montar sem quebrar a autenticação se alguma dependência
ainda estiver indisponível. I14.2 fica encerrado localmente.

Atualização de I14.3: o pós-auth agora também tenta reconstruir a configuração
inicial com `HostState.RebuildUserConfiguration` antes do Bootstrap. Quando há
geração local válida, ele carrega `commandconfig.Store`, projeta o subconjunto
local `keyboard.local`/read/none com `ProjectLocalRead`, revalida
`Store.CheckCurrent` dentro da publicação e só então troca o mapa no `HostState`.
Quando ainda não há geração base, mantém o host fechado sem publicar mapa; quando
há geração inválida/obsoleta, falha fechado e não mascara como mapa vazio. Ainda falta
recovery/reconciliação durável de restart e claims persistentes.

Atualização seguinte de I14.3/I14.5: `SetupVault`/`UnlockVault` agora finalizam
a barreira de auth antes de relançar o lifecycle para a sessão atual, então o
desbloqueio do cofre pode reprojetar/publicar comandos sem novo Login/RefreshAuth.
No shutdown/drain, o App usa a prova de `CloseAndDrain` para rodar recovery
bounded de receipts e invocações pendentes antes de destruir dependências,
sem inferir encerramento por idade ou restart.

Atualização de claims persistentes: quando há geração base de configuração, o
rebuild produtivo chama `commandactivation.RestorePersistent` com owner/epoch
derivados do `HostState`, recarrega o snapshot e deriva `ActiveUserLayerIDs`
somente de claims manuais, ativas, persistentes, da sessão atual, não expiradas,
com regra ativa e camada de usuário habilitada. A lista derivada é publicada no
`HostState` junto com a configuração; instalação nova sem geração base permanece
fechada sem alterar claims.

Fechamento qualificado de I14.3: restart de processo agora foi coberto pelo
contrato local do lifecycle. O App reabre carregando/projetando/publicando uma
configuração válida quando sessão, cofre e SO estão prontos, mas não transforma
invocações ou receipts pendentes em recuperados apenas por inferir que houve
restart; reconciliação continua exigindo prova de drain ou manutenção posterior.
Contagem atual: **50/84 critérios locais, 34 abertos; 3/15 pacotes completos**.

Avanço de I14.4: a publicação de configuração persistida agora fica vinculada
ao `userID/sessionID` capturado no load. Se a sessão local muda antes do commit,
o publish é rejeitado, evitando que rebuild atrasado de uma sessão anterior
sobrescreva o estado volátil da sessão atual.

Avanço adicional de I14.4: resets de login/refresh/logout agora também removem
a configuração volátil do usuário atual no `HostState`; durante transição de
auth, o executor deixa de ver o mapa antigo até que um rebuild autenticado
publique uma nova configuração.

Fechamento de I14.4: o runtime produtivo também revalida a projeção contra o
`HostState` atual no `Publish` e antes de `SetEnabled`, recusando configuração,
camadas ou unlock antigos entre etapas. Somado aos testes de reset/readiness,
reset/shutdown concorrentes e falha/cancelamento do monitor de SO, I14.4 fica
encerrado localmente. Contagem atual: **51/84 critérios locais, 33 abertos;
3/15 pacotes completos**.

Avanço de I14.5: shutdown do lifecycle também remove a configuração volátil do
usuário atual no `HostState` antes de aguardar o worker parar, evitando mapa
consultável enquanto bridge, drain e demais dependências ainda estão encerrando.

Fechamento de I14.5: shutdown integrado agora tem prova de que encerra lifecycle,
drena o domínio de executores, fecha a bridge, bloqueia novas admissões/remontagens
e preserva dependências quando o drain falha. Contagem atual: **52/84 critérios
locais, 32 abertos; 3/15 pacotes completos**.

Fechamento de I14.6/I14: a matriz local agora cobre instalação nova sem publicação,
persistência, claims, restart com pendência sem falsa reconciliação, falhas antes
de readiness, retomada após erro, monitor de SO e shutdown integrado. I14 fica
completo localmente. Contagem atual: **53/84 critérios locais, 31 abertos;
4/15 pacotes completos**.

Avanço de I15.2: a qualificação ampla foi repetida com temporários externos ao
repo. `go vet ./...`, frontend lint e frontend build passam; `go test ./...`
passa na maior parte da árvore, mas ainda falha em `internal/acp` e
`internal/acpregistry` por encerramento de processo `0xffffffff`. Race, NVDA,
hardware e review permanecem pendentes.

Diagnóstico seguinte de I15.2: `internal/acp` e `internal/acpregistry` encerram
antes de listar testes ou emitir `GODEBUG=inittrace`; os binários de teste
compilados retornam `-1` diretamente. `golangci-lint` v2.11.4 carrega config e
pacotes, mas termina antes da análise com `no go files to analyze`.

### Stream Deck real validado — 15/09/2026

`internal/commanddeck` iniciou a base testável de I13.5/C41/C42: o renderer
valida geometria, controla estado por dispositivo, força frame completo após
abertura/reconexão, aplica diff incremental e cacheia hashes de imagem por
modelo/tamanho/conteúdo. O `Manager` sem HID acrescenta posse exclusiva lógica,
estado seguro na abertura, ativação por geração, lock/logout seguro, rejeição de
geração obsoleta e backoff de reconexão. A borda `DeviceAdapter` liga eventos
normalizados do futuro driver HID ao controller físico comum, bloqueia teclas em
estado seguro e força frame completo seguro em lock/logout. `Driver`/`Handle` e
`Runtime` deixam a biblioteca HID real plugável depois: descoberta não derruba o
app em falha individual, abertura escreve frame seguro antes de ativar, leitura
com erro desconecta com backoff, e shutdown fecha handles após frame seguro. A
base também registra um diagnóstico testável de biblioteca/modelos: o candidato
`rafaelmartins.com/p/streamdeck` ficou explicitamente `not ready` até validação
de licença, manutenção, Windows/Wails, modelos e HID físico. Descoberta
detalhada agora reporta falhas parciais por dispositivo, e testes multi-device
garantem isolamento de render/desconexão.

Atualização de preparação física: `rafaelmartins.com/p/streamdeck` foi integrado
como dependência real e o `StreamDeckDriver` concreto compila atrás da interface
existente. O diagnóstico local agora confirma licença BSD-3-Clause, pure Go/sem
CGO, suporte multiplataforma declarado e modelos básicos; antes do teste físico,
restava somente `physical-hid-unverified`. O teste manual opt-in
`TestManualStreamDeckPhysicalRoundTrip` e o runbook operacional reduzem o
fechamento de I13.5 à execução com o Stream Deck conectado.

Validação física executada em 15/09/2026: Stream Deck serial `AL28K2C54852`,
modelo `Stream Deck`, 15 teclas. A primeira tecla recebeu o frame vermelho, o
evento `streamdeck.key:AL28K2C54852` / `key:0` foi recebido e o teste terminou
com `PASS`. I13.5 fica encerrado localmente. A contagem sobe para **47/84
critérios locais, 37 abertos; 2/15 pacotes completos**. Mapas reais de produto e
UI do Stream Deck continuam em P04; validação de teclado/foco/janela e ambiente
ampliado seguem em I13.6.

### Ambiente físico de teclado/foco/janela validado — 15/09/2026

I13.6 foi encerrado localmente com `internal/commandphysical`: foreground nativo,
hotkey global suportado, sessão interativa conhecida/desbloqueada e Stream Deck
físico validado são consolidados em relatório fail-closed. A execução manual no
PowerShell visível capturou foreground `windowsterminal.exe`, classe
`CASCADIA_HOSTING_WINDOW_CLASS`, confirmou hotkey global suportado e reutilizou o
Stream Deck serial `AL28K2C54852` modelo `Stream Deck`; o teste terminou com
`PASS`. Com I13.5 e I13.6 fechados, I13 é consolidado como pacote completo; a
contagem passa a **48/84 critérios locais, 36 abertos; 3/15 pacotes completos** naquele momento.
A migração de mapas reais e UI continua nos pacotes P01/P04/I14.

### Prova de escopo de diálogo — 15/09/2026

I13.3 foi encerrado localmente como infraestrutura de reserva/invariante: o
bridge transporta `DialogCommandProof`/`DialogProof` e exige round-trip exato no
resultado. A composição autenticada do frontend só deixa `decision.respond`
atravessar uma barreira modal quando a prova corresponde ao `DialogCommandScope`
topmost atual, com origem `keyboard.local` e ownership local; prova ausente,
stale, de outro diálogo ou global permanece bloqueada antes de bindings de
fundo. **Contagem anterior: 46/84 critérios locais, 38 abertos; 2/15 pacotes completos**.

### Ocorrências físicas e sequências — 15/09/2026

I13.2 foi encerrado localmente como contrato de infraestrutura: a ponte aceita
`sourceEventId` UUIDv7 e o adapter gera esse ID somente para o primeiro `down`
aceito com binding; repeat, release e entrada sem binding não criam ocorrência.
O adapter também modela sequências como `Ctrl+N` com timeout e limpeza em
blur/troca de geração. **45/84 critérios locais, 39 abertos; 2/15 pacotes
completos**. Listeners reais de SO, layout físico, HID/Stream Deck e comandos de
produto seguem fora desta etapa.

### Lifecycle de adapters físicos — 15/09/2026

I13.4 foi encerrado localmente com `internal/commandadapter`: listeners físicos
futuros entregam callbacks a um controller que anexa sessão/owner/geração e faz
handoff por `commandbridge.Input`, sem chamar handler final nem conhecer catálogo
de produto. Lock/logout/shutdown suspendem entradas e conclusões atrasadas de
lifecycle não regredem geração. **Contagem histórica: 44/84 critérios locais,
40 abertos; 2/15 pacotes completos**. Teclado global real, HID/Stream Deck e
comandos de produto continuavam fora desta etapa.

### Ponte UI/backend — 15/09/2026

I13.1 foi encerrado localmente: o App monta uma `commandbridge.Bridge` privada,
expõe métodos Wails para invoke/input/result/cancel/lifecycle e revalida
usuário/sessão autenticados antes de aceitar payload da UI. O frontend ganhou
adapter tipado sobre `window.go.app.App`, sem edição manual de bindings gerados.
**Contagem histórica: 43/84 critérios locais, 41 abertos; 2/15 pacotes
completos**. Isso ainda não habilitava listeners físicos, ownership local/global
real, Stream Deck/HID, providers autoritativos de contexto ou comandos de
produto.

### Quinze pacotes existentes — 15/09/2026

Rodada I01–I15 com seis agentes Luna, revisão cruzada e correções centrais.
Inclui regressão dos fundamentos já entregues, não quinze pacotes novos ou
quinze conclusões. **Contagem histórica: 42/84 critérios locais, 42 abertos; 2/15 pacotes completos**.
I11.1 foi encerrado com envelope v1/v2 entre bancos distintos, bindings/regras,
deltas/needs_review e remapeamento de workspace. Montagem pública, multi-escopo
e export sensível permanecem pendentes. Legados sensíveis em condição e
apresentação agora são recusados pela validação compartilhada.

Identidade de job reconsulta o grant real por geração exata, sem manutenção
na leitura final. Mutação global do App reconstrói o mapa do banco e distingue
commit de publicação; não repete efeito quando só o rebuild falha. Recuperação
de receipts e invocações compartilha o coordinator e prova do core atual, não
prova de restart. Teclado local interno revalida foco/surface e respeita barreira
modal, sem instalar listener ou migrar comandos de produto.

Qualificação encontrou contenção SQLite, corrigida com retry central de reserva
e CAS, nunca do handler. Cada tentativa relê o relógio sem renovar deadlines.
Desempenho integrado continua pendente: medição sintética não qualifica a meta
experimental nem a experiência de teclado/UI. A tasklist registra as evidências
atuais e os limites; contagens abaixo são históricas. BASE-PRONTA não atingido.

### Dez frentes existentes — 15/09/2026

I03/I04/I05/I06/I08/I09/I11/I12/I13/I14 avançaram com seis agentes Luna e
revisão central, sem habilitar comandos de produto. Regrant passou a usar a
decisão exata e o writer comum, com rollback e invalidação do mapa antigo antes
do commit. A fábrica do App usa sessão/JWT e presenter reais; ainda não é
montagem produtiva. Preview permanece leitura, nunca autoridade executável.

Heartbeat integra a mesma passagem do coordinator; o Manager configurado usa
seu único timer e drena a manutenção antes do teardown. Recuperação pagina
usuários/system com prova real do core atual, sem inferir prova de restart.
Ponte TS de contexto/dispatch filtra sessão/geração, e o round-trip interno de
importação cobre deltas/needs_review e remapeamento de cópias por workspace.
Falha de snapshot só ganha recusa durável quando mantém as gerações obrigatórias
autoritativas; nenhum requisito de nulabilidade foi flexibilizado.

Contagem atual: **41/84 critérios locais, 43 abertos; 2/15 pacotes completos**.
I08.2 encerrado no serviço interno; os demais pacotes compostos ainda têm gaps.
Restart, transporte autenticado UI/backend, montagem dos domínios no App,
portabilidade pública/multi-escopo e qualificação de desempenho/hardware
continuam pendentes. A tasklist é a referência atual; as contagens abaixo são
históricas. Nenhum pacote adicional de infraestrutura foi inventado.

### Ligação do core — 15/09/2026

O core agora fecha novas admissões e drena obrigatoriamente todos os executores
registrados antes de emitir prova de suas gerações. O ledger consome essa prova
no writer de recuperação existente; finalização bloqueada ou com falha foi
exercitada com executor e SQLite reais, sem reexecução de efeito. App.Shutdown
preserva dependências se a drenagem falhar. Isso não prova encerramento de um
processo anterior: restart e montagem da manutenção continuam pendentes.
Adapters reais ligam outbox/reconciliação ao coordinator e preservam continuação;
o scope de decisão está no stack real de Modal, sem habilitar despacho físico.
Importação interna distingue `ErrNoChanges` de entrada inválida e testa regra
de evento desabilitada, sem concessões. Nenhum atalho de produto foi migrado.

A fábrica interna completa do App compõe sessão, HostState e FactBus reais;
decisões interativas usam o presenter real e a mesma raiz SQL do ledger. Recusa
transações/DB estrangeiro, cofre fechado, SO desconhecido e sessão sem mapa
reconstruído. Não é chamada pelo startup produtivo e não cria fallback para
providers UI ausentes. Build/vet/lint e 31 pacotes envolvidos passaram após revisão;
a suíte global da rodada teve 108 pacotes aprovados e falhas de processo em
ACP/acpregistry. A validação completa permanece pendente.

Contagem atual: **40/84 critérios locais, 44 abertos; 2/15 pacotes completos**.
I11.4 encerrado com serialização negativa e preservação de grants/claims de regra
intocada no destino. Não equivale à importação pública habilitada nem à conclusão
da infraestrutura. As contagens abaixo registram rodadas anteriores.

### Continuação local de 15/09/2026 — mesmas cinco frentes

Passagens de consumo e heartbeat agora usam lotes/cursor e TTL atual sem criar
uma cadência nova. Jobs/tools paginam usuários e informam continuação; as limpezas
internas por usuário ainda não têm limite de linhas. Escopo restritivo de decisão
acompanha a fila da UI, mas não está registrado no dispatcher de Modal/Wails.
Shutdown fecha a montagem do App e sinaliza o worker mesmo com contexto cancelado;
rebootstrap limpa a geração anterior. Importação exercita no-op, referência ausente,
rollback e recusa de lote multi-escopo. A prova produtiva de geração drenada continua
pendente e não ganhou um substituto baseado em callback. Contagem permanece 39/84,
2/15 pacotes completos; entradas produtivas seguem desabilitadas.

### Integração local de 15/09/2026 — I09/I11/I12/I13/I14

O writer interno de importação usa decisão/CAS comuns e revalida referências
antes do commit; a migração 27 preserva auditorias anteriores e admite
`config_import`. A purga de fatos protege leases vivas. Retenção de ativações
e adapters reais de manutenção cobrem idade/caps, usuários/system e preservação
de ledger; a cadência única ainda não está montada e o adapter legado não é paginado.
Shutdown do executor espera finalização durável e impede novo Start após fechamento,
mas não constitui por si só prova de geração encerrada para recuperação.
O adapter de decisão alcança a fila real da UI, e o App ganhou hooks de autenticação
e encerramento com join do worker. Providers/transporte produtivos, heartbeat,
importação pública e hardware permanecem pendentes. Nenhum atalho foi migrado.
Tasklist: 39/84 critérios locais, 2/15 pacotes completos; não é percentual de esforço.

**Data:** 2026-09-12

**Acompanhamento da implementação:** `0103-tasklist-infraestrutura.md` contém
os pacotes e evidências locais. I02 implementa os contratos completos de
catálogo/envelope/JCS/defaults; detalhes de compatibilidade em
`0103-contratos-versionados.md`. Isso não encerra os critérios finais nem
habilita adapters ou migra comandos do produto. Status permanece In Progress.

I03 acrescenta FactBus escopado, gerações/caches isolados, snapshots do Manager
real, captura nativa de foreground e leitores de superfície/foco/diálogo na UI.
A ponte autenticada UI/backend e o registro das superfícies reais permanecem
pendentes em I03.1/I14; snapshots enviados pela UI não concedem autoridade.
Evidências e limitações de validação estão na tasklist, sem marcar critérios
finais como concluídos por testes isolados.

I04 amplia o mesmo Service/ledger com argumentos, resolução direta/trigger,
gates de leitura/escrita/destrutivo e consumo transacional de receipt de
invocação (I06.1 antecipado). Migração central v21 preserva o schema v20
conhecido. O fluxo de sessão local tem testes com handlers controlados;
contextos externo/job/system (I10), projeção completa (I05), recusas anteriores
ao snapshot assinável e montagem no App permanecem pendentes. Não há ativação
do novo executor nem migração dos atalhos nesta entrega.

**Relacionados:** AEP-0001, AEP-0023, AEP-0045, AEP-0046, AEP-0047,
AEP-0048, AEP-0052, AEP-0058, AEP-0060, AEP-0063, AEP-0067, AEP-0074-B,
AEP-0080, AEP-0091, AEP-0101

## Resumo

Esta AEP propõe um sistema único e configurável para executar comandos do
Assistente a partir de teclado local, hotkeys globais, Stream Deck conectado
diretamente por USB/HID, Command Palette, chat, CLI e eventos.

O sistema separa cinco conceitos:

1. **comando**: capacidade identificada por um ID estável;
2. **acionador**: gesto ou evento que solicita um comando;
3. **binding**: associação entre acionador, comando e argumentos;
4. **camada**: conjunto combinável de bindings;
5. **regra de ativação**: condição ou evento que ativa uma camada.

Camadas padrão fornecidas pelo aplicativo permanecem ativas e contribuem com
seus bindings. Camadas adicionais são combinadas sobre elas, sem substituir o
mapa completo. Um binding mais específico pode sobrescrever apenas o mesmo
acionador no contexto em que ambos concorrem. Assim, ativar uma camada de chat
não desabilita comandos padrão como `Ctrl+N`.

O Stream Deck não dependerá do software oficial da Elgato. O backend Go será o
proprietário do dispositivo, receberá seus eventos e renderizará diretamente as
teclas. Imagens e estados físicos serão uma apresentação dos mesmos comandos e
camadas usados pelos demais acionadores.

## Motivação

Os atalhos atuais estão distribuídos entre hooks e componentes do frontend,
constantes locais e um gerenciador separado de hotkeys globais. Esse desenho
torna difícil:

- personalizar atalhos sem alterar código;
- detectar conflitos entre handlers registrados em lugares diferentes;
- preservar corretamente escopo, foco e precedência de diálogos;
- descobrir e executar ações por uma Command Palette;
- configurar as mesmas ações por teclado, chat ou dispositivos físicos;
- reagir à tela, aba, workspace ou programa em primeiro plano;
- expor jobs e tools MCP sem criar um caminho de execução diferente;
- restaurar padrões de forma seletiva;
- oferecer gerenciamento acessível de um Stream Deck sem usar o software
  oficial, que não atende ao fluxo com leitor de telas.

O projeto já possui peças relacionadas, mas ainda independentes:

- a AEP-0001 prevê hotkeys como triggers de jobs;
- a AEP-0063 fornece execução comum para tools nativas e MCP;
- a AEP-0080 define contexto de surfaces;
- `internal/hotkey` registra hotkeys globais;
- o frontend possui handlers locais em hooks e componentes;
- a CLI e os deep links já expõem algumas ações por interfaces alternativas.

Sem um registro canônico de comandos e um resolvedor comum, acrescentar Stream
Deck, Command Palette e configuração por chat multiplicaria fluxos paralelos e
os mesmos problemas de escopo já observados nos atalhos locais.

## Decisões

### D1 — Arquitetura orientada a comandos

O núcleo seguirá o fluxo:

```text
Acionador → resolvedor de bindings e camadas → registro de comandos → executor
```

O teclado não é o núcleo do sistema. Ele é um adapter de entrada, assim como
Stream Deck, Command Palette, chat e CLI.

O nome conceitual do subsistema será **Gerenciador de Comandos e Acionadores**.
O gerenciador de teclado atual será integrado gradualmente como adapter, e não
expandido para conhecer layouts físicos, regras de janela ou execução de tools.

### D2 — Registro canônico de comandos

Todo comando terá:

- ID estável e namespaced, como `workspace.tab.new`, `chat.model.select`,
  `job.run` ou `layer.toggle`;
- nome, descrição, categoria e aliases versionados nos três locales;
- schema tipado de argumentos;
- escopos e contextos em que pode executar;
- `context_policy` com fatos obrigatórios e regra de staleness;
- `allowed_source_types`, usando exatamente a taxonomia de D3, inclusive a
  origem interna reservada `system`;
- paths sensíveis de input/output e política de persistência;
- classificação de risco;
- `effect_class`, com `read`, `write` ou `destructive`;
- `decision_requirement`, incluindo `none` ou diálogo interativo;
- `mutates_effective_capability`, derivado do contrato do handler;
- estado de disponibilidade e motivo quando indisponível;
- apresentação padrão opcional, incluindo ícone e estados;
- handler ou rota de execução.

IDs apresentados pelo catálogo são canônicos. Chat, importação e UI não podem
inventar IDs nem executar handlers por nome aproximado.

Aliases pertencem ao registro, em mapa versionado `locale → string[]`, e passam
pela mesma normalização de busca da Command Palette. A UI não mantém listas
paralelas.

`context_policy` lista fatos por provider e modo: `exact_version`, `max_age_ms`,
`event_snapshot` para captura confiável no instante do acionamento, ou `none`
somente para leitura que não depende de alvo atual. `event_snapshot` exige
`max_age_ms > 0` e timestamp autenticado do provider.
`internal/commandcontext.VersionService` registra providers e monta
`context_version` como fingerprint dos pares `(fact_name, fact_version)`.
Providers iniciais, respeitando a divisão de autoridade abaixo (os providers
visuais não são registrados no `VersionService` Go como espelhos da UI):

- surface: consulta `surfaceId`/`snapshotVersion` pelo contrato da AEP-0080;
- diálogo: geração do stack topmost;
- foco/controle e janela/processo: geração dos adapters de UI/SO;
- workspace/aba: versão do store canônico;
- camadas ativas: `active_layers_generation` do resolvedor;
- job: `run_id` + último `job_run_events.sequence`;
- sessão/lock: epochs do `EpochService`.

Na revalidação em seu domínio, cada provider compara a versão atual ou a idade exigida pelo
comando. Provider ausente, versão incomparável ou TTL vencido torna o comando
indisponível/falha fechado; não há heurística comum aplicada a contextos
diferentes.

O registro rejeita `context_policy = none` quando `effect_class` não for
`read`, quando houver alvo mutável ou quando o comando alterar capacidade
efetiva. `CommandExecutionService` revalida essa invariância; metadata de comando
não pode optar por escapar de staleness.

`effect_class = destructive` exige `decision_requirement = interactive`,
segue AEP-0091 e exige
`allowed_source_types ∩ {cli, event, system} = ∅`, mesmo quando a coleção
contém outras origens. O registro rejeita origens inerentemente headless e o
executor rejeita qualquer chamada sem um `DecisionPresenter` interativo
autenticado. Combinação
`destructive + none` é inválida no registro e recusada novamente pelo executor.

`DecisionPresenter` é uma porta interna injetada pelo bootstrap, nunca um campo
ou capability declarada pelo chamador. Na v1, somente a sessão Wails local
registra uma implementação; ela envia `kind: decision` ao
`questionnaire.Manager` e o frontend apenas renderiza o `DecisionDialog` da
AEP-0091. A porta recebe `DecisionRequest` com `decision_id` UUIDv7,
`invocation_id` ou ID da mutação de configuração, usuário/sessão, fingerprint
da solicitação, gerações de autenticação/segurança, ações permitidas e
`expires_at`. O `DecisionDialog` continua produzindo o corpo da AEP-0091:
`{ actionId }` ou `{ cancelled: true }`. O host confiável anexa o
`decision_id` da solicitação backend à correlação, resultando em
`{ decision_id, actionId }` ou `{ decision_id, cancelled: true }` para a porta;
ESC, fechamento e cancelamento explícito fazem o CAS para `cancelled`
imediatamente, sem aguardar expiração.

`DecisionReceiptService` valida a resposta contra a solicitação criada no
backend e faz CAS único de `pending` para `accepted`, `denied`, `cancelled` ou
`expired`; resposta duplicada ou ação fora do conjunto falha fechado. Para
despacho destrutivo, o `CommandExecutionService` exige receipt `accepted`
correspondente a usuário, sessão, invocação, fingerprint, ação afirmativa e
gerações atuais. Sob o `DispatchGate`, consome a receipt e efetiva o CAS de
`evaluating` para `queued` na mesma transação; replay, receipt já
consumida ou geração alterada não executa. `authorization_decision_id` é o
`decision_id` consumido.
Mutação confirmável de configuração usa o mesmo contrato com
`subject_type = config_mutation` e consome a receipt ao gravar a mudança.
Origem sem presenter registrado nunca cria receipt afirmativa.

#### Divisão de autoridade contextual UI/backend — aprovada em 16/09/2026

A seção 21 da tasklist registra os providers mínimos das quatro páginas de
workspace, compartilhados pelo escopo React e consumidos pela preparação
contextual do catálogo. Não habilita seleção/conteúdo, migração de atalhos ou
condições visuais como autoridade backend; R01.1 continua parcial.

A seção 20 da tasklist registra a integração da superfície explícita da
Topbar com o catálogo e os executores visuais. A sessão é descartada em
mudanças de navegação/owner; isso não generaliza os providers para outras
telas nem encerra R01.1. O AEP permanece In Progress.

Implementação e evidências desta mudança são acompanhadas nas seções 12 e 13
da [tasklist de conclusão](0103-tasklist-conclusao.md). O handoff local de
execução registrada é montado para `help.shortcuts.show` (palette); isso não
constitui migração geral dos comandos ou fechamento integral de R01.

### Política de execução por efeito — revisão de 18/09/2026

Uma entrada no catálogo não implica necessidade de auditoria persistente.
A classificação é confiável, declarada pelo produto, não escolhida pelo
binding, payload ou frontend:

- **Apresentação local:** navegar entre telas/abas, abrir menu/ajuda e mover
  foco. Executa na UI, sincronamente após validar owner, rota, superfície,
  modal, foco e composição. Não cria `command_invocations`, chave de
  idempotência nem histórico por tecla. No recorte atual: `navigation.*`,
  `help.shortcuts.show`, `workspace.panel.focus` e os onze comandos de
  navegação `workspace.tab.next/previous/first` até `ninth`.
- **Pickers de chat (ampliação autorizada em 18/09/2026):**
  `chat.model.open`, `chat.history.open` e `chat.profile.open` apenas abrem
  os pickers existentes; não selecionam modelo/perfil/conversa, não enviam
  mensagem e não limpam conteúdo. São apresentação local sem ledger. Os
  defaults Ctrl+M/H/P são configuráveis; a superfície ativa e suas capacidades
  são revalidadas na UI. Um binding global não torna o comando disponível
  fora de um chat apto. Ctrl+L fica fora dessa classificação.
- **Consultas do chat (lote89):** `chat.pinned.open` e `chat.tokens.open`
  abrem os modais existentes da conversa capturada, sob as mesmas restrições
  de owner/sessão, aba ativa, superfície visível, foco, composição e modal
  topmost. Não fixam/desfixam mensagens, não limpam conteúdo e não alteram o
  contexto do modelo. São apresentação local sem ledger e sem novos defaults;
  a configuração pode associar teclado e Stream Deck. O escopo de chat permite
  esses dois IDs explicitamente, sem permitir comandos arbitrários ou limpeza.
- **Apresentação do editor (lotes79–80):** `editor.menu.file.open`,
  `editor.menu.format.open`, `editor.menu.insert.open`,
  `editor.menu.mode.open`, `editor.slides.open` e
  `editor.presentation.fullscreen` abrem controles existentes ou solicitam
  fullscreen do renderer, sem salvar, formatar, inserir conteúdo ou persistir
  modo. Inserir apenas abre o menu; não insere conteúdo. Apenas a instância
  visível do editor ativo é elegível; modal, overlay, composição e contexto
  obsoleto bloqueiam. Readonly ou modo view bloqueiam especificamente Inserir;
  Formatar requer editor rico editável, slides requerem Reveal rico e
  fullscreen requer Reveal em visualização. Arquivo e Modo seguem suas
  capacidades próprias. Alt+I é contextual: no editor apto resolve Inserir;
  fora dele resolve Importar conforme o mapa efetivo, sem fallback legado.
  Alt+S/F5 são defaults configuráveis. Fullscreen depende do suporte/permissão
  do WebView; não contorna exigências de gesto confiável.
- **Apresentação de páginas (lote98):** `tasklists.create.open`,
  `tasklists.edit.open`, `tasklists.search.focus`, `profiles.create.open`,
  `profiles.edit.open`, `profiles.search.focus`, `terminal.sessions.open`,
  `terminal.focus.input` e `terminal.focus.history` apenas abrem controles
  existentes ou movem foco. Capturam página/instância e seleção/sessão antes
  da paleta; mudança de identidade, alvo, rota, modal ou composição invalida
  a captura. Edição não salva; abrir formulário não cria registro. Não se
  estende essa classificação a CRUD, ativação ou processos do terminal.
- **Operação de domínio:** consultas e alterações comuns possuem autenticação,
  validação e persistência adequadas à operação. Não herdam automaticamente o
  protocolo completo porque aparecem no catálogo. A seleção da última aba é
  uma operação explícita de workspace/aba, não uma invocação auditada. Outros
  handlers existentes só mudam mediante revisão concreta de seus efeitos;
  esta decisão não elimina suas proteções por inferência.
- **Execução com consequências:** envio, processos, exclusão, efeitos externos
  e demais comandos classificados para execução durável continuam no executor
  com autorização, prevenção de duplicidade, resultado e auditoria exigidos
  pelo domínio. Criar e fechar abas permanecem nesse percurso neste recorte;
  não são presumidos puramente visuais.

O mapa local é uma projeção autenticada dos bindings resolvidos pelo host,
incluindo supressões e camadas; não é uma autorização para efeitos de domínio.
Na projeção contextual da seção80, o host publica `contextualBindings` para
combinações simples de apresentação que dependem somente de `surface.type`.
Cada entrada contém resoluções `bySurface` e um `fallback` para outras
superfícies conhecidas. `null` é uma barreira explícita, inclusive em
supressão, conflito ou revisão pendente; nunca autoriza experimentar o
comando global. A UI lê o provider vigente a cada acionamento e recusa
contexto desconhecido, sem consultar banco ou IPC por tecla. Não resolve
prioridades nem traduz IDs de comandos no cliente. A seção113 amplia essa
projeção para sequências v2 e operações auditadas nas surfaces reais das abas
chat/editor/terminal/tasklist. A UI captura uma lease de contexto e envia
somente a observação de tipo/ID da surface; o host não aceita command ID,
owner ou argumentos de execução nesse ingresso. A observação deve coincidir
com a aba canônica, e o snapshot de workspace capturado é revalidado durante
a resolução/admissão e antes do handoff. Não se transforma a aba em provider
visual: foco/modal/IME e a lease da surface continuam sendo conferidos na UI.
Outros fatos e operações contextuais em páginas sem essa correspondência
continuam explicitamente indisponíveis, sem fallback para execução global.
As sequências fixam o contexto no prefixo e recusam uma continuação obsoleta;
barreira de uma sequência não suprime outras identidades com o mesmo prefixo.
No lote98, um prefixo simples contextual pode coexistir com sequências v2
já existentes. O host publica `fallbackToSequences: true` somente quando a
resolução da superfície alternativa é `NoMatch`. Na UI isso permite continuar
para as sequências apenas em superfície conhecida sem ramo próprio e sem
fallback simples. Ramo explícito `null`, supressão/conflito, superfície
desconhecida ou flag ausente continuam barreiras; nunca se tenta a sequência
após falhar um comando contextual. Ajuda e rótulos seguem a mesma prioridade.
Somente a lista fechada de apresentação pode usar o caminho local. Atualização
de mapa, logout, mudança de owner e perda de foco invalidam a projeção local
antes de recarregá-la. Não se promete revalidação síncrona do estado remoto
por tecla: a política de apresentação usa a última projeção válida publicada
na UI. Ela não permite executar efeitos privilegiados com uma autorização
antiga. A paleta também respeita a resolução efetiva, não apenas o catálogo.

Stream Deck mantém descoberta/captura/normalização e checagens de geração,
owner e configuração no host; apresentação local usa evento efêmero tipado,
sem reserva no ledger. A UI rejeita geração/owner/contexto divergentes. Eventos
visuais não são reexecutados após reconexão. Teclado global e origens headless
não ganham esse caminho por inferência.

A aba visual muda imediatamente. A última seleção é persistida separadamente,
com alvo explícito e coalescência das seleções ainda não enviadas: não há fila
durável de cada troca. Falha reconcilia o estado sem aplicar resposta de outra
sessão/workspace. Antes de uma operação dependente da aba canônica, aguarda-se
a seleção pendente e revalida-se o alvo originalmente capturado, sem retarget.
Foco/anúncio visual não afirma que houve uma transação de auditoria.

As regras abaixo de reserva, `CommandInvocation`, handoff e ledger aplicam-se
ao percurso durável, não aos comandos de apresentação local acima. A exceção
também se aplica a supressão local: combinação suprimida não dispara IPC nem
grava uma chave por tecla. A infraestrutura auditada continua válida para
seus consumidores e não deve ser usada como condição de navegação básica.

O contexto é revalidado no domínio que possui sua fonte autoritativa. Esta
decisão substitui a interpretação de que o backend consulta DOM/foco por Wails
dentro do `DispatchGate`; não substitui autenticação, autorização ou ledger.

- **Backend:** sessão, cofre, gerações de segurança/configuração, identidade e
  versão do alvo persistido, workspace/aba/perfil canônicos e fatos nativos
  pertencem aos providers locais do backend. O gate continua curto e não
  aguarda Wails, resposta da UI ou conclusão de efeitos.
- **UI:** foco e capacidades do controle, superfície visual explícita, modal
  topmost e composição IME são relidos sincronamente na UI imediatamente
  antes do efeito visual. Uma observação enviada por Wails não é provider
  autoritativo do backend, mesmo que tenha versão ou TTL recente.
- Uma preparação visual é local, opaca, de uso único e vinculada à instância
  UI, usuário/sessão, workspace/aba e contexto capturado. A aplicação consome
  a preparação e relê as fontes; só inicia o efeito síncrono se ainda forem
  equivalentes. Não há `await` entre essa revalidação e o efeito. Ausência de
  fonte exigida, blur, composição ativa/desconhecida, logout, descarte da
  instância ou divergência recusa o efeito; nunca procura outro alvo.
  A exceção de navegação da seção 63 admite somente composição `unknown`
  nos campos nativos especificados; não flexibiliza composição `active`.
- Notificações invalidam preparações/caches e ajudam a observar transições,
  mas não autorizam o efeito. A checagem final consulta novamente DOM,
  registries e stores, inclusive quando uma notificação não foi entregue.
  Mudança de aba é verificada mesmo se o perfil permanecer igual. IME usa
  estado explícito `active | inactive | unknown`; uma leitura sem evidência
  suficiente não transforma `unknown` em `inactive`.
- **Comandos duráveis que atravessam os dois lados:** o backend autentica, autoriza,
  reserva a invocação e entrega somente um handoff tipado, correlacionado e de
  uso único para a sessão/instância admitidas. A UI valida o contexto local
  novamente antes do efeito e devolve o resultado correlacionado. Ack de
  recebimento não significa efeito concluído. Recusa visual antes do efeito
  não vira sucesso; perda de confirmação após handoff continua sujeita a
  `outcome_unknown`, sem reexecução automática. Enquanto esse percurso não
  estiver montado para um comando, ele permanece indisponível.
- Comando de backend recebe um **alvo explícito**, autenticado e revalidado
  no backend, nunca “o controle/aba que estiver ativo quando a resposta
  chegar”. Troca de foco depois da submissão não desfaz um efeito backend já
  admitido; seu retorno não pode aplicar efeito visual em outro contexto.
  Operação que exigir foco ainda atual no instante do efeito deve executar
  essa parte no domínio UI ou permanecer indisponível; TTL/cache não simulam
  essa garantia.

A proteção local de apresentação (por exemplo, aplicar o resultado da
consulta do catálogo no picker) não concede permissão para comandos nem
substitui o ponto único de execução abaixo. Ela também não transforma o
catálogo de consulta em um executor alternativo.

Comandos de UI podem ser executados no frontend por uma ponte tipada. Comandos
de backend são enviados ao serviço correspondente. Jobs usam o runtime de jobs;
tools internas e MCP usam o executor comum da AEP-0063. O registro de comandos
não duplica o `tool_catalog`: ele pode expor um comando parametrizado que delega
a uma entrada existente do catálogo.

### D2.1 — Envelope e ponto único de execução

Nenhum adapter chama diretamente o handler final. Toda solicitação converge
para `CommandExecutionService`, inclusive comandos de UI. O serviço recebe um
envelope versionado:

```text
CommandInvocation
  version, invocation_id, command_id?, arguments?
  observed_trigger_type?, trigger_type?, trigger_spec?
  user_id?, auth_context_type, auth_context_id, auth_generation
  session_id?, security_generation
  actor_type, actor_id
  source_type?, observer_type?, source_instance_id?, source_event_id?
  source_occurred_at?, source_replay_policy_generation?
  source_replay_deadline?
  workspace_id?, binding_ids?, registry_version
  global_config_generation?, workspace_config_generation?
  active_layers_generation?
  foreground_snapshot?
  conversation_id?, turn_id?, surface_type?, surface_id?
  surface_snapshot_version?, context_version?
  context_captured_at_by_provider?
  source_profile_slug?, target_profile_slug?
  authorization_decision_id?, delegation_fingerprint?, grant_generation?
  job_id?, job_slug?, job_definition_fingerprint?, run_id?
  provenance?, correlation_id, request_fingerprint_version?
  request_fingerprint?, client_requested_at?, received_at
```

`source_type` distingue teclado local/global, Stream Deck, palette, chat, CLI,
evento e sistema. `source_instance_id` é um UUIDv7 novo para cada abertura,
reconexão ou geração física do adapter; a identidade estável do dispositivo
permanece em `trigger_spec`. `source_event_id` identifica uma ocorrência única
de adapter físico/de evento e é UUIDv7 gerado pela borda confiável. Execuções
diretas o omitem. Contador/ID nativo do protocolo pode ficar em metadata
redigida, mas não substitui a identidade canônica.
`actor_type` distingue usuário, agente e automação.

`source_type` é a origem lógica vencedora que governa binding e
`allowed_source_types`; `observer_type` e `observed_trigger_type` registram quem
observou o evento físico. Em solicitação por trigger, `source_type` fica nulo até
a resolução escolher o candidato; em execução direta, é fixado na borda.

`actor_type` e `actor_id` são derivados no backend do principal e da origem
autenticados; valores recebidos de adapter/cliente são ignorados e divergência
falha fechado. O mesmo vale para `user_id`. O payload nunca escolhe a identidade
que será usada por autorização ou auditoria.

`auth_context_*`, `source_type`, `observer_type`, `source_instance_id` e
`source_event_id` também são derivados ou validados pela borda autenticada e
pelo registro do adapter. Cada endpoint fixa as origens que pode produzir;
cliente não promove a si próprio a `system`, hotkey ou dispositivo.
`trigger_spec` é normalizado pelo dispatcher e validado contra a capacidade do
adapter antes de chegar ao resolvedor.

`context_captured_at_by_provider` é mapa `provider_id → RFC3339 com timezone`
e preserva separadamente o instante informado por cada provider confiável. O
ingresso nunca substitui timestamp ausente por `received_at`, pois isso faria
snapshot antigo parecer novo; valor de cliente não confiável é ignorado.
Política `max_age_ms` ou `event_snapshot` exige entrada para cada provider e
falha fechado quando alguma faltar. `exact_version` pode omitir a entrada
daquele provider, pois reconsulta sincronamente a versão autoritativa em seu
próprio domínio e exige igualdade antes do despacho/efeito correspondente.
O mapa de providers do envelope backend não aceita um espelho de DOM/foco
como autoridade; as precondições visuais pertencem à preparação local descrita
na divisão UI/backend. `none` não declara provider backend e omite o mapa e
`context_version`. Versões/epochs monotônicos ficam em `context_version`, não
são serializados como timestamp.

Invocação com usuário exige `global_config_generation` e
`active_layers_generation`; workspace exige também
`workspace_config_generation`. `system` sem usuário omite as três, pois não
acessa bindings/camadas. Policy que declara provider exige `context_version`;
`none` o omite. Não há string vazia, zero ou sentinel para esses casos.

Adapters físicos e de evento exigem `source_instance_id` e `source_event_id`.
Adapter de evento exige ainda `source_occurred_at`, obtido do timestamp
autenticado e persistido do produtor; adapter físico o omite. Palette,
`ui.action`, chat, CLI e `system` omitem os três e deduplicam pela PK
`invocation_id`. A borda Wails cria o UUIDv7 de cada clique/formulário antes do
serviço. Após resolução, `binding_ids` é sempre materializado como lista, ainda
que vazia.
`source_replay_policy_generation` e `source_replay_deadline` são derivados
internamente depois de validar a fonte; valor recebido de cliente é ignorado.

Na primeira tentativa, IDs são gerados em borda confiável: Wails para
palette/UI, contexto persistido da tool call para chat, processo backend para
`system`, serviço CLI para terminal e cada adapter de teclado, Stream Deck ou
evento para sua ocorrência. Todos geram `invocation_id` UUIDv7. Somente
adapters físicos/de evento também
geram `source_event_id`, usado para deduplicar a ocorrência; palette, UI, chat,
CLI e `system` o omitem. A CLI imprime/devolve o ID e aceita `--request-id`
apenas em retry autenticado; chat reutiliza o ID associado ao mesmo tool call.
Valor reapresentado nunca troca ownership e sempre passa pelo fingerprint/ledger.

No envelope de ingresso, `request_fingerprint_version` e
`request_fingerprint` ficam ausentes: adapters não os calculam nem podem
fornecê-los. `authorization_decision_id` também fica ausente e só é preenchido
pelo `DecisionReceiptService` após resposta válida. Depois da normalização e,
para trigger, após fixar o candidato vencedor, o backend calcula o HMAC por
JSON Canonicalization Scheme (RFC 8785).
Ambos se tornam obrigatórios antes da reserva no ledger. O fingerprint inclui
schema, comando ou trigger, argumentos, IDs/versões de contexto, usuário/ator
derivados, tipo/ID do contexto autenticado, origem/observador, workspace,
`source_event_id`, `source_occurred_at`, geração e deadline de replay quando
presentes;
`source_instance_id` somente quando não houver `source_event_id`; versões de
catálogo/configuração, profiles de origem/destino,
política/requisito de decisão, `delegation_fingerprint`, `grant_generation`,
`job_id`, `job_slug`, `job_definition_fingerprint`, `run_id` e proveniência.
Exclui token bruto, `authorization_decision_id`, `auth_generation` rotativa e
timestamps. O ID da decisão é resultado da interação, não entrada semântica da
solicitação; a receipt o vincula ao `invocation_id` e ao fingerprint já
calculado. É calculado no backend e persistido
sem revelar segredos. Reentrega com o mesmo `invocation_id` só é aceita se o
fingerprint for idêntico; divergência é conflito e falha fechado.

A chave `command-request-hmac:v1` vive no secret manager e permanece disponível
por pelo menos a maior expiração dos ledgers. O ledger guarda
`request_fingerprint_version`. Rotação cria versão nova para requests novos e
mantém chaves antigas até seus ledgers expirarem. Chave esperada indisponível
faz a reentrega falhar fechado, sem executar novamente.

`provenance` é um documento versionado e redigido com `_source`,
`_source_job_id`, `_chain_id` e `_chain_history` da AEP-0067, além de
`command_chain_history` separado, quando a solicitação vier de cadeia reativa.
O dispatcher o copia sem reconstruir por heurística.

Uma solicitação informa `command_id` para execução direta ou
`trigger_type`/`trigger_spec` para resolução de binding. Depois da resolução,
solicitação por trigger contém também o `command_id` vencedor; solicitação
direta mantém `trigger_*` nulo. A única exceção é a resolução terminal
`effect = suppress` da D4: ela preserva `command_id` nulo, registra o marcador
`suppressed` no ledger e não cria `CommandInvocation`. Fora dessa exceção,
ausência do campo exigido por cada modalidade ou combinação incoerente falha
antes de qualquer efeito.

Se qualquer um entre `surface_type`, `surface_id` e
`surface_snapshot_version` estiver presente, os três tornam-se obrigatórios e
não vazios. Comando direcionado a surface sem o trio completo falha fechado,
conforme AEP-0080.

Na borda, o mapeamento é único e explícito:
`SurfaceContext.surfaceType → surface_type`, `surfaceId → surface_id` e
`snapshotVersion → surface_snapshot_version`. O envelope e o SQLite usam
snake_case; `context_version` é o fingerprint composto do `VersionService`, não
um alias de `snapshotVersion`.
Esses três valores recebidos são candidatos, não autoridade. O backend localiza
a surface no registro canônico do usuário/contexto autenticado, reconsulta o
snapshot provider da AEP-0080 e exige igualdade de tipo, ID e versão antes de
materializá-los no envelope interno. Surface ausente, pertencente a outro
usuário ou divergente falha fechado; cliente não atesta a própria atualidade.

O serviço, nessa ordem:

1. autentica o principal e valida usuário/proveniência mínima;
2. normaliza e resolve o binding/candidato num snapshot sem efeitos, capturando
   versões e bindings contribuintes; falha de resolução também produz resultado
   determinístico;
3. calcula o fingerprint canônico da solicitação resolvida ou recusada;
4. para `suppress`, adquire `DispatchGate`, revalida autenticação, segurança,
   staleness e gerações e só então reserva o ledger terminal enquanto mantém o
   gate; se stale, não grava `suppressed`, mas reserva uma recusa terminal
   `rejected_stale` para os mesmos IDs/ownership e registra o evento no log de
   segurança. Tanto `suppressed` quanto `rejected_stale` encerram o
   processamento sem seguir aos passos seguintes e sem criar
   `CommandInvocation`. Nos demais casos, reserva atomicamente ledger e
   auditoria como `evaluating`;
5. se a resolução falhou, conclui `denied`; caso contrário valida origem
   permitida, disponibilidade, argumentos, contexto e política;
   falhas após autenticação terminam a tentativa como `denied`. Quando a
   política exige interação, obtém uma receipt pelo presenter interno antes de
   continuar; não mantém `DispatchGate` aberto enquanto aguarda o usuário;
6. revalida contexto de autenticação, geração de segurança, staleness e
   autorização imediatamente antes do despacho, incluindo e consumindo a
   receipt no CAS para `queued`; cancela a invocação se qualquer gate estiver
   obsoleto;
7. ao retirar da fila, revalida novamente os mesmos gates e faz CAS atômico de
   `queued` para `running`; se o contexto mudou, grava `cancelled_stale` sem
   chamar o handler. Também compara `registry_version`,
   `global_config_generation` e `workspace_config_generation` atuais; mudança de
   comando, camada, binding ou prioridade cancela como stale. Compara também
   `active_layers_generation`; claim ativada/desativada desde a resolução
   cancela a invocação. Se a transição vencer, encaminha ao handler;
8. conclui a trilha como `succeeded`, `failed`, `cancelled`, `timed_out` ou
   `outcome_unknown`; `denied` e `cancelled_stale` já encerram nos gates
   anteriores.

`registry_version` identifica o catálogo/defaults carregado.
Cada usuário possui `global_config_generation`; cada workspace possui
`workspace_config_generation`. Mutação global incrementa a primeira e invalida
invocações em todos os workspaces; mutação local incrementa somente a segunda.
A invocação captura `workspace_id` e o par de gerações na resolução. Sem
workspace, captura apenas a global.

O resolvedor mantém contador global por usuário e contador por
usuário+workspace. `active_layers_generation` é fingerprint composto somente
dos dois contadores aplicáveis ao `user_id`/`workspace_id` da invocação.
Mudança de claim global incrementa o contador do usuário e afeta seus
workspaces; mudança local incrementa apenas o workspace. `layer.back`,
expiração, pin e eventos seguem o mesmo escopo. Mudança de outra conta ou
workspace não cancela a invocação.

`internal/commandsecurity.DispatchGate` serializa admissão com mudanças de
segurança/configuração. Logout, lock, troca de principal e mutações de mapa
adquirem o gate exclusivo antes de incrementar gerações. O worker adquire o gate
compartilhado, revalida, faz CAS para `running` e entra sincronamente em
`handler.Start(ctx)` antes de liberar o gate. `Start` é obrigatoriamente
não bloqueante: apenas aceita o handoff, enfileira/inicia o worker e devolve um
handle/ack de início; nunca espera job, tool ou UI terminar e não readquire o
gate. Adapter de API síncrona precisa envolvê-la em fila/goroutine controlada
antes de anunciar suporte; caso contrário o comando fica indisponível. Assim
não existe janela entre CAS e início, nem lock mantido durante trabalho longo.
Invalidação posterior cancela o contexto quando suportado; efeito já admitido
não é retroativamente desfeito. Testes cobrem ack limitado, logout concorrente e
ausência de deadlock.

`Start` devolve `ExecutionHandle` com ID, canal/future `Done` e `Cancel`.
`CommandExecutionService` aguarda fora do gate e faz CAS terminal único para
`succeeded`, `failed`, `cancelled`, `timed_out` ou `outcome_unknown`,
atualizando auditoria e ledger na mesma transação. Antes de entrar no handler,
erro/panic comprovadamente sem
handoff vira `failed`; depois de entrar em `Start`, panic sem rejeição
conclusiva, canal fechado sem outcome e perda do prazo sem cancelamento
confirmado viram `outcome_unknown`, pois o efeito pode ter ocorrido. `failed`
pós-handoff exige outcome explícito do `ExecutionHandle`. Outcome tardio após
terminal é ignorado. Handler síncrono curto usa handle já concluído.

`timed_out` só vale antes do handoff ou quando o handler confirma cancelamento
sem efeito. Depois do ack, prazo vencido, canal perdido ou cancelamento não
confirmado vira `outcome_unknown`, nunca `timed_out`; retry automático é
proibido e reconciliação explícita consulta o executor/recurso antes de nova
invocação. Outcome tardio verificável pode reconciliar
`outcome_unknown → succeeded|failed` por CAS auditado; não dispara novo efeito.

A reserva de execução é uma transação que cria a chave no ledger de
idempotência e a linha de auditoria `evaluating` antes do handler; supressão
segue a exceção terminal sem auditoria definida em D4. O ledger usa PK `id`, `key` UNIQUE,
`invocation_id` UNIQUE e índice parcial UNIQUE de `source_event_id` quando
presente. Replay com usuário/origem/fingerprint divergente é conflito, não nova
execução. Reentrega recebe o resultado
existente ou falha como
duplicada, nunca chama novamente o handler. Palette, chat e CLI fornecem um
`invocation_id` UUIDv7 idempotente por solicitação. Eventos físicos recebem
`source_event_id` no único adapter que os possui. Para eles, a garantia de
deduplicação é limitada à sessão física ou até `expires_at` do ledger; depois
disso, nova observação física é nova solicitação.

Evento durável não segue essa regra curta. Na v1, o adapter relê
`command_job_activation_outbox` por PK/ownership, exige igualdade de
`source_event_id` e `source_occurred_at`. A linha nasce na mesma transação do
`job_run_events` de origem e não possui cascade com `job_runs`.
`command_event_replay_policy_epochs` registra cada
mudança de `maintenance.job_retention_hours` com geração, `effective_at` e
horizonte; o evento usa o epoch vigente em `source_occurred_at` para calcular
um `source_replay_deadline` imutável. O ledger recebe `expires_at` nunca
anterior a esse deadline.

Antes de remover um ledger de evento, a manutenção exige que o deadline tenha
vencido. Se não houver ledger, evento após seu `source_replay_deadline` é
rejeitado como `rejected_stale` antes da resolução, nunca aceito como novo.
Aumentar retenção cria epoch apenas para ocorrências seguintes e não reabre
eventos cujo deadline anterior venceu; diminuir também não encurta ledgers já
reservados. Não se infere idade do UUIDv7.

`received_at` é sempre atribuído pelo backend ao receber o envelope e governa
retenção/idade das origens não duráveis. Para evento durável,
`source_occurred_at` autenticado governa a admissibilidade e o piso de retenção
do ledger. `client_requested_at`, quando fornecido, é apenas metadado validado
e nunca altera expiração, ordenação de segurança ou caps.

Reentrega consulta primeiro o ledger pelo ID e recebe status, `result_summary`
e `result_ref` redigidos por `CommandExecutionService.GetInvocation`; não
repete o handler mesmo se a auditoria detalhada já tiver sido compactada. A
consulta exige o mesmo contexto autenticado, filtra por
`(user_id, invocation_id)` e reaplica autorização do ator; buscar somente pela
PK é proibido. A API pública não consulta invocações `system`. No startup, um
método interno e não exposto, `ReconcileSystemInvocations`, opera sob capability
privilegiada da instância e seleciona apenas `user_id IS NULL AND
auth_context_type = 'system'` de epochs anteriores nos estados recuperáveis;
não exige que `auth_context_id` antigo seja igual ao epoch atual e não aceita ID
fornecido externamente. Assim,
registros `evaluating`, `queued` ou `running` de uma geração encerrada viram
`outcome_unknown` em `command_invocations` e
`command_idempotency_keys` na mesma transação, nunca são reexecutados
automaticamente. O usuário ou fluxo
chamador precisa consultar o efeito e criar uma nova invocação explícita. Esse
tratamento reconhece que exatamente-uma-vez não é garantível para todo handler
de UI ou sistema após queda entre efeito e commit.

Recusas anteriores à autenticação — sem usuário confiável ao qual associar a
linha — vão para o log de segurança, não para uma conta indicada pelo payload.
Toda recusa posterior é persistida como `denied`, com código redigido.

`allowed_source_types` usa os valores exatos `keyboard.local`,
`keyboard.global`, `streamdeck.key`, `palette`, `ui.action`, `chat`, `cli`,
`event` e `system`. `actor_type` é ortogonal: por exemplo, uma tool chamada pelo agente
tem `source_type = chat` e `actor_type = agent`. Não existe categoria implícita
`desktop`; cada comando declara explicitamente quais entradas aceita.

**Override da AEP-0052 e ingresso externo — seções151–156:** D6 distingue
sessões locais (`sub = user_id`) de JWTs externos. APIs externas resolvem
exclusivamente `(iss, sub) → users.id` por vínculo administrativo habilitado e
usuário ativo; sem bootstrap/vínculo, falham fechado, sem fallback legado.
O executor externo e as rotas HTTP estão compostos no App. Readiness por
origem e revogação continuam exigidas; o ingresso externo não inventa sessão
desktop nem habilita adapters físicos. Os limites e os testes pendentes de
qualificação/review estão no acompanhamento vigente acima.

Os contextos de autenticação são:

- `local_session`: o `SessionService` da AEP-0052 fornece somente `user_id` e
  `session_id`; o `EpochService` desta AEP fornece as gerações. Desktop e CLI
  recebem esses dados do backend; `auth_context_id = session_id` e
  `auth_generation` é mantida por esse session ID, não por usuário; IDs vindos
  como argumentos são ignorados;
- `external_token`, com ingresso condicionado à prontidão acima: JWT validado fornece `sub`, scopes e um
  `auth_context_id` derivado de `iss` + `sub` + `jti` ou fingerprint do token;
  `(iss, sub)` sempre precisa resolver por mapeamento administrativo explícito.
  `sub` isolado nunca é aceito como `users.id`, pois não é global entre issuers.
  Não há provisionamento automático nem fallback para usuário atual;
  ausência/ambiguidade falha fechado. Antes de habilitar comandos nesse modo, o
  upgrade exige que o administrador migre identidades usadas pelo middleware
  vigente para `external_identity_mappings`. JWT/scopes são revalidados;
- `job_service`: automação usa o usuário proprietário, ID e versão persistida do
  job, representada por `job_definition_fingerprint`, além dos grants exatos
  aplicáveis; o gate final relê a definição e compara o fingerprint. Não pode
  abrir diálogo nem executar comando que exija interação;
- evento externo: só vira um dos contextos acima após autenticação do ingress e
  mapeamento inequívoco para usuário; caso contrário falha fechado;
- `system`: contexto interno criado pelo processo, sem `user_id`, restrito a
  comandos explicitamente seguros que não acessam dados ou bindings de usuário.

Hotkeys e Stream Deck exigem um contexto autenticado atualmente ativo. Contexto
`system` sem usuário só executa comandos internos que declarem essa origem e não
acessa bindings ou dados de usuário.
Em `auth.mode=external`, teclado físico global e Stream Deck ficam
indisponíveis: não existe sessão desktop autoritativa à qual vincular o evento.
Palette/UI/chat continuam usando o principal do JWT da própria requisição.
Habilitar adapters físicos nesse modo exige AEP posterior para um broker local
que vincule e revogue explicitamente um principal externo ativo; “último token”
ou usuário inferido nunca é aceito.

**Adendo aprovado pelo mantenedor em 24/09/2026 — interface externa vinculada:**
o ingresso JWT também poderá acionar comandos na interface do Assistente
explicitamente conectada ao usuário autorizado. Identidade da chamada e
identidade/conexão do destino são verificadas separadamente; a conexão não
concede roles/scopes nem transforma JWT em sessão local. O destino precisa ser
inequívoco, sem escolher a última janela ativa ou herdar a sessão desktop.
Comandos contextuais fixam e revalidam aba, seleção e versões da interface;
desconexão, substituição da conexão, mudança de usuário ou contexto obsoleto
recusam trabalho pendente, sem redirecionar para outra janela. Decisões
interativas passam pelo contrato comum e ficam vinculadas ao solicitante,
destino e invocação exatos. Sem interface conectada, somente comandos de
backend explicitamente compatíveis podem executar. A implementação deve
reutilizar os handlers e o pipeline comuns, incluindo auditoria e replay;
não cria executor de efeitos paralelo, não habilita dispositivos físicos no
modo externo e não depende de migração do armazenamento de workspaces.

Bootstrap do modo externo é pré-requisito explícito: endpoint administrativo
fora do command manager, protegido por issuer configurado + scope admin, cria
`(iss, sub) → users.id` para usuário local existente. O primeiro admin só pode
vincular o próprio token legado quando `sub` já coincide com esse `users.id`;
demais vínculos são escolhas explícitas auditadas. Concluído o lote, uma flag de
readiness faz o middleware resolver todos os principals pelo mapa. Até então,
CommandExecutionService fica desabilitado em `auth.mode=external`. Não há JIT.

As gerações não são atribuídas à AEP-0052.
`internal/commandsecurity.EpochService`, definido aqui, cria um epoch aleatório
novo no startup, mantém `auth_generation` por contexto autenticado e uma
`security_generation` do processo. Logout, revogação observada, troca de usuário
e substituição do principal invalidam o epoch de autenticação; lock/unlock e
mudança de principal incrementam o epoch de segurança. O serviço consulta a
validade real da sessão/JWT/job na revalidação, portanto o epoch é proteção
adicional contra trabalho obsoleto, não uma nova autoridade de autenticação.
Para `system`, o epoch aleatório do processo preenche `auth_context_id` e
`auth_generation`; restart o invalida e o gate continua limitado aos handlers
puros definidos acima.

Comandos que delegam para tools passam então pelo executor da AEP-0063 e
correlacionam `command_invocations.invocation_id` com `tool_invocations`. Jobs passam pelo
runtime de jobs; ações de frontend recebem da ponte somente um despacho já
autorizado, vinculado ao `invocation_id`. Command Palette, chat e CLI podem
selecionar diretamente um `command_id`, mas não ignoram validação, autorização
ou auditoria.

Contexto `system` sem `user_id` não pode delegar para tool, MCP ou job, pois
esses contratos exigem proprietário. Ele fica restrito a handlers puros e
internos sem dados de usuário; tentativa diferente falha fechado.

Argumentos sensíveis são redigidos ou resumidos na auditoria conforme a política
do comando. Erro, status, origem, ator, comando e correlação permanecem
diagnosticáveis.

Se o comando delegar a uma tool, paths sensíveis do registro são propagados ao
`ToolInvocationService`: o valor bruto existe somente em memória para execução,
enquanto input/output persistidos usam placeholders e fingerprint do conteúdo
redigido. Essa extensão deve atualizar a AEP-0063 no mesmo PR que a implementar.
Até esse suporte existir, comando com paths sensíveis é indisponível para
delegação a tools e falha fechado.

Quando `actor_type = agent`, o envelope preserva conversa, turno,
`surface_type`, `surface_id` e `surface_snapshot_version`; esses campos vêm dos
nomes camelCase do `SurfaceContext` pelo mapeamento de borda já definido, além
dos profiles de origem e destino. `CommandExecutionService` não trata o agente como
o usuário autenticado: delegação cross-profile interativa exige
`DecisionDialog` e registra `authorization_decision_id`; origem sem interlocutor
falha fechado.

Jobs cross-profile transportam `job_id`, `target_profile_slug`,
`delegation_fingerprint` e `grant_generation`. O serviço relê o grant pela chave
`(user_id, job_id, target_profile_slug, delegation_fingerprint)` e pela geração
exata da AEP-0101 imediatamente antes do handler. `job_id` é UUID; o profile
alvo permanece identificado pelo slug canônico, como exige aquela AEP. O
envelope transporta a decisão, mas não cria nem amplia grants.

### D3 — Acionadores são adapters, não comandos

Tipos iniciais de acionador:

- `keyboard.local`: combinação recebida dentro da janela do Assistente;
- `keyboard.global`: hotkey registrada no sistema operacional;
- `streamdeck.key`: tecla física, incluindo dispositivo e posição;
- `palette`: escolha na Command Palette;
- `ui.action`: clique, formulário ou ação direta da UI autenticada;
- `chat`: execução estruturada solicitada pelo agente;
- `cli`: execução solicitada pelo entrypoint de terminal;
- `event`: na primeira versão, somente fato de `job_run_events` espelhado
  transacionalmente em `command_job_activation_outbox`;
  outros produtores exigem outbox antes de entrar na taxonomia operacional.

`system` é uma origem interna reservada para execução direta pelo processo. Não
é acionador configurável, não aparece em bindings do usuário e obedece às
restrições sem proprietário da D2.1. Assim, a taxonomia completa de
`allowed_source_types` é a lista acima mais essa origem interna explícita.

Adapters normalizam a entrada para uma identidade de acionador e nunca executam
diretamente a ação final. Uma entrada física gera no máximo uma execução, mesmo
quando mais de um observador puder enxergá-la.

Cada adapter físico normaliza primeiro uma transição `up → down` e só depois
gera `source_event_id`. Por padrão, em `keyboard.local`, `keydown` com
`KeyboardEvent.repeat = true` é descartado e `keyup` libera a combinação; perda
de foco, blur ou troca de geração limpa o estado pressionado sem disparar ação.

**Exceção aprovada pelo mantenedor em 18/09/2026:** comandos explícitos de
navegação de abas podem aceitar repetição automática local. A política é uma
lista fechada de comandos, conferida também no host; criar/fechar abas e outros
efeitos continuam sem repetição. Um repeat só é elegível após down observado
da mesma combinação e geração ainda pressionada, nunca isoladamente. Cada
repeat aceito produz uma ação local distinta, sem deduplicar pressionamentos
legítimos e sem persistir invocação/ledger da apresentação. Não reutiliza
autorizações para efeitos de domínio. A seleção visual usa o estado síncrono
atual; somente sua persistência pendente pode ser coalescida. Setas futuras devem
respeitar ownership do controle e só aderem mediante suporte explícito; esta
decisão não captura setas globalmente nem altera teclado global ou Stream Deck.

O adapter global solicita a opção nativa de no-repeat quando disponível e
mantém a mesma máquina de estado de pressão/liberação; plataforma que não
consiga garantir essa borda não anuncia suporte ao binding global. Stream Deck
deduplica callbacks até o release correspondente. Testes mantêm a tecla
pressionada, simulam repeat/reconexão e provam uma única ocorrência.

Eventos de teclado têm ownership exclusivo. Uma combinação registrada como
`keyboard.global` pertence ao adapter do sistema operacional inclusive quando o
Assistente está em foco; o adapter DOM recebe a lista correspondente e não emite
`keyboard.local` para ela. No ingresso, o adapter global preenche somente
`observer_type`/`observed_trigger_type = keyboard.global`; após a resolução,
`source_type`/`trigger_type` também ficam `keyboard.global` em qualquer foco.
Binding local da mesma combinação fica
marcado como shadowed/conflitante e não participa; para variar a ação dentro do
Assistente, o binding global usa condições/camadas de foco e surface. O adapter
local possui somente combinações não registradas globalmente. Alterações de
registro são aplicadas por geração antes de publicar o novo mapa. Stream Deck
possui um único listener por dispositivo. Essa exclusão evita dupla execução e
define uma única observação física.

Pressão normal, pressão longa, alternância e dial podem ser acrescentados como
gestos normalizados quando o dispositivo oferecer esses sinais. Pressão longa
exige máquina de estado própria que emita exatamente um gesto normal ou longo
por ciclo físico, nunca reaproveita os `keydown.repeat` descartados. Capacidade
não detectada não deve ser simulada de forma ambígua.

### D4 — Bindings associam acionadores a comandos

Um binding contém:

- camada;
- tipo e especificação normalizada do acionador;
- `command_id` para `execute`;
- argumentos validados pelo schema do comando para `execute`;
- condição tipada opcional;
- `resolution_priority`;
- `effect`, com `execute` ou `suppress`;
- `replaces_default_id`, `replaces_default_version` e
  `replaces_default_fingerprint` para delta de default;
- `review_status`, com `active` ou `needs_review`;
- estado habilitado/desabilitado;
- origem: padrão do aplicativo ou personalização do usuário;
- metadados de apresentação específicos do acionador.

`effect = suppress` exige os três campos `replaces_default_*`, mantém
`command_id` nulo e argumentos vazios; o default alvo é a única autoridade.
Override executável de default também exige o trio; binding inteiramente novo os
mantém nulos. `needs_review` sempre bloqueia delta e default, sem fallback.

O mesmo comando pode ter vários bindings. O mesmo acionador pode aparecer em
várias camadas. Reutilização não é conflito enquanto as condições ou camadas
não puderem estar ativas simultaneamente.

Quando candidatos simultâneos possuem o mesmo `command_id`, argumentos
normalizados e escopo de execução, o resolvedor os deduplica e produz uma única
invocação, preservando a proveniência de todos os bindings equivalentes.
Diferença de comando, argumentos ou escopo continua sendo conflito e falha
fechado se a precedência não escolher um único vencedor.

Bindings `effect = suppress` são aplicados antes dessa deduplicação. Eles
removem os defaults referenciados no contexto, consomem o acionador quando não
restar candidato e nunca criam `CommandInvocation`. Antes de retornar, o
serviço reserva `command_idempotency_keys` em estado terminal `suppressed`, com
`invocation_id`/`source_event_id`, ownership, fingerprint do request resolvido,
gerações e `expires_at`; não cria linha de auditoria de comando. Reentrega
encontra essa chave antes de qualquer despacho e retorna `suppressed` ou
conflito de fingerprint, mesmo se configuração posterior remover o tombstone.
Somente bindings `effect = execute` participam do agrupamento por
comando/argumentos.

Todo delta é materializado antes da tupla de D7: override substitui o candidato
default referenciado e herda seu `scope_rank`/especificidade base; condição do
usuário só pode estreitar o contexto. Tombstone remove o candidato. Portanto
delta nunca compete nem perde para o próprio default; vínculo sem default válido
vira `needs_review`.

### D5 — Camadas são conjuntos aditivos

Camadas agrupam bindings de qualquer tipo de acionador. Elas não são exclusivas
do Stream Deck e não representam cópias completas de um mapa de teclas.

Exemplo:

```text
Camada Chat
  Ctrl+M             → chat.model.select
  Ctrl+Tab           → workspace.tab.next
  Stream Deck tecla 1 → workspace.tab.previous
  Stream Deck tecla 2 → workspace.tab.next
```

Várias camadas podem estar ativas ao mesmo tempo. O mapa efetivo é uma
composição:

```text
Padrões do aplicativo
+ padrão da surface ativa
+ camadas ativadas pelo usuário
+ camada contextual do programa em foco
+ camada temporária de uma operação
```

Uma camada contribui apenas com os bindings que declara. Binding ausente cai
para a próxima camada aplicável; não significa "sem comando".

`enabled = false` apenas desliga uma personalização e, portanto, permite
fallback. Para desabilitar deliberadamente um default, o sistema cria um
binding `effect = suppress` que referencia o binding padrão e bloqueia o
fallback somente no contexto declarado. Esse tombstone participa da mesma
precedência dos bindings executáveis e pode ser restaurado.

### D6 — Estrutura padrão é permanente e versionada

O aplicativo fornecerá camadas padrão, no mínimo:

- aplicativo;
- workspace;
- chat;
- editor;
- tasklists;
- terminal;
- diálogos e decisões.

Elas são versionadas no código e ativadas automaticamente por seu contexto.
Personalizações ficam no banco como deltas. O usuário pode:

- substituir um binding padrão ou criar tombstone explícito para desabilitá-lo;
- restaurar um binding;
- restaurar uma camada;
- restaurar todas as personalizações.

Ativar uma camada adicional nunca desabilita implicitamente uma camada padrão.
Uma atualização pode adicionar novos defaults sem regravar nem apagar
personalizações existentes. Todo override ou tombstone de default armazena
`replaces_default_id` e `replaces_default_version`, permitindo detectar se o
default mudou desde a personalização.

No upgrade, se o fingerprint semântico do default referenciado não mudou, o
sistema apenas avança `replaces_default_version`. Se comando, trigger,
argumentos, escopo ou risco mudou, o delta vira `needs_review`: ele e o novo
default ficam bloqueados naquele acionador/contexto, sem fallback, até decisão
do usuário. Restaurar remove o delta e adota o novo default; rebase confirmado
atualiza versão/fingerprint. A UI anuncia e lista pendências.

O fingerprint usa RFC 8785 sobre toda semântica executável: comando, argumentos,
trigger, condição, effect, escopo, prioridades, origens permitidas,
`context_policy`, `effect_class`, risco, `decision_requirement`,
`mutates_effective_capability` e requisitos do adapter. Somente apresentação
puramente visual fica fora. Qualquer mudança desse conjunto exige review.

Atalhos obrigatórios da AEP-0091, incluindo `Ctrl+Shift+R` para repetir a
pergunta do `DecisionDialog`, são invariantes não suprimíveis. O usuário pode
adicionar uma alternativa, mas não remover a rota exigida pelo contrato. Outros
atalhos essenciais de acessibilidade só admitem substituição quando o contrato
correspondente permitir e houver alternativa equivalente validada.

### D7 — Resolução determinística de conflitos

Condições de bindings e de ativação usam predicados tipados; JavaScript, Go
templates e expressões de shell livres não são aceitos. Essa regra não altera
`when` e `emit_when` internos de jobs da AEP-0001: o binding de hotkey apenas
solicita o trigger identificado ao runtime de jobs, que continua avaliando seus
templates e políticas. Converter essas expressões de jobs exige decisão
separada e não faz parte desta AEP.

Contextos previstos incluem:

- diálogo no topo;
- controle focado e capacidade do controle;
- surface e instância ativas;
- aba e workspace ativos;
- janela do Assistente focada ou desfocada;
- processo e identidade da janela em primeiro plano;
- perfil e modo operacional;
- estado de job ou recurso;
- dispositivo de origem.

A precedência conceitual é:

1. diálogo ou decisão bloqueante;
2. controle focado;
3. surface/aba ativa;
4. workspace;
5. camadas explícitas ou temporárias;
6. programa em primeiro plano, somente quando o Assistente estiver sem foco;
7. aplicativo;
8. global padrão.

Dentro do mesmo nível, especificidade tipada vem antes da prioridade explícita;
empate não resolvido é conflito de configuração e não pode executar duas ações.

O procedimento operacional determinístico compara a tupla:

1. posição do escopo na lista acima;
2. especificidade do predicado, comparada por campos tipados — identidade exata
   antes de tipo e tipo antes de curinga;
3. `command_layers.resolution_priority`;
4. `command_bindings.resolution_priority`.

Comparação é lexicográfica: menor posição na lista de escopos vence (diálogo
antes de global), identidade exata vence tipo/curinga e, somente nos campos
`resolution_priority`, o inteiro maior vence. Prioridades são persistidas e
incluídas no export. Se comandos/argumentos diferentes ainda empatarem após a tupla, a
configuração é inválida e o evento falha fechado; IDs não são usados como
desempate oculto.

Predicados sobre campos diferentes são incomparáveis nessa etapa e, portanto,
empatam em especificidade; não existe precedência oculta entre dispositivo,
perfil, modo, estado ou processo. As prioridades persistidas resolvem esse caso.
Se também forem iguais e os destinos diferirem, aplica-se o conflito fail-closed
descrito acima.

Na v1, condição é somente conjunção de cláusulas `campo eq escalar`, com campos
de enum fechado e valores normalizados por tipo (string exata com case definido
pelo campo, booleano ou ID opaco). Ausência de cláusula é curinga; OR, NOT,
regex, intervalos e operadores de conjunto ficam fora. B domina A somente
quando contém todas as cláusulas idênticas de A e ao menos uma adicional; por
exemplo, `surface=editor ∧ process=code.exe` domina `surface=editor`. Valores
diferentes ou sem relação de subconjunto são incomparáveis e dependem de
prioridade explícita ou terminam em conflito.

O stack compartilhado de `Modal` registra no dispatcher um
`DialogCommandScope { dialog_id, kind, generation, allowed_command_ids,
allowed_trigger_specs }` ao abrir e remove ao fechar. Para `DecisionDialog`, a
ponte deriva o scope das `actions` reais da AEP-0091 e dos atalhos invariantes,
mapeando respostas para o comando fixo `decision.respond`; payload não injeta
command IDs arbitrários. O topo do stack é a fonte autoritativa e mudanças
incrementam `generation`.

Para os três pickers de chat, o escopo de apresentação modal é distinto do
escopo de decisão: só o chat pertencente à instância topmost pode abrir seus
pickers, com a allowlist fechada acima e handlers montados. Um modal sobre
ele bloqueia esses comandos; fechar/remontar/trocar a conversa invalida o
alvo anterior. Nenhum payload amplia a allowlist de `DecisionDialog`.
Teclado e Stream Deck reutilizam essa ação local; a paleta existente continua
fora dos modais e captura o chat de origem antes de abrir sua busca, sem
redirecionar uma seleção pendente para outra instância de chat. Não é uma
autorização genérica para abrir a paleta ou navegar atrás de um modal.

Diálogo bloqueante no topo é barreira, não apenas camada prioritária. Enquanto
existir, somente bindings declarados pelo `DialogCommandScope` topmost são
avaliados. Se não houver candidato permitido, o acionador é consumido ou
recusado sem cair para surface, workspace, aplicativo ou global. Isso vale
também para hotkey do SO e Stream Deck e preserva a AEP-0091.
Atalhos invariantes exigidos pela AEP-0091 integram implicitamente toda allowlist
do `DecisionDialog` e não podem ser omitidos nem bloqueados por configuração.
Enquanto houver diálogo topmost, o dispatcher reserva essas combinações antes
de consultar qualquer binding configurável ou ownership global. Assim,
`Ctrl+Shift+R` chega ao `DecisionDialog` mesmo se existir binding concorrente.
No caminho local, a reserva só ocorre depois dos guardas obrigatórios da
AEP-0091: evento não repetido, sem composição IME e fora de input, textarea,
contenteditable e Monaco. Se um guarda bloquear, o dispatcher ignora sem
capturar a digitação. No caminho global, o adapter confiável do SO registra
temporariamente `Ctrl+Shift+R` somente enquanto houver `DecisionDialog`
topmost, inclusive com outro programa em foco, e remove o registro ao fechar o
diálogo. Essa reserva intencional preserva o caso Alt+Tab da AEP-0091, não é
binding configurável e não permanece ativa fora do diálogo.

A UI deve detectar sobreposição possível no momento da edição, explicar em quais
contextos ela ocorre e pedir confirmação antes de criar uma substituição. Um
conflito confirmado sobrescreve somente o acionador concorrente naquele
contexto.

### D8 — Ativação de camadas

Uma camada pode ser:

- sempre ativa;
- ativa enquanto uma condição for verdadeira;
- ativada/desativada manualmente;
- alternada por um comando;
- ativada por evento e removida por evento correlato;
- temporária, com duração ou ciclo de vida definido.

Cada regra produz claims independentes. Uma camada habilitada fica ativa quando
ao menos uma claim válida está ativa; desativação encerra somente o
`activation_id` correspondente. Não há prioridade entre regras. Fixar uma
camada cria claim manual persistente que eventos automáticos não removem;
outras camadas continuam compondo o mapa. `layer.back` encerra a claim manual
mais recente da mesma `manual_stack_key`, ordenada por
`(activated_at, activation_id)`. A chave é derivada no backend da origem
normalizada e de sua sessão/dispositivo, nunca inventada pelo payload.

`layer_disable` alterna `command_layers.enabled` para falso, incrementa a
geração de configuração e, se a camada estava efetiva, o contador de claims do
escopo, tudo sob o `DispatchGate` e na mesma transação. Claims persistidas não
são encerradas, mas o resolvedor as ignora enquanto `enabled = false`.
`layer_enable` revalida ownership, expiração, autenticação e fonte de cada
claim, recalcula condições contextuais e só então torna a camada elegível;
claim expirada/stale não ressuscita. A geração efetiva é incrementada se o
conjunto ativo mudar.

Regras de surface, foco, controle e programa em primeiro plano são condições
síncronas dos context providers da D2, recalculadas em memória quando sua versão
muda. Elas não usam `LayerActivationEvent`, outbox ou ledger de evento. O
envelope abaixo é apenas para ativações event-driven; na v1, somente jobs.

O produtor entrega primeiro um candidato sem autoridade sobre regra, camada ou
escopo:

```text
LayerActivationCandidate
  version, event_name, source_type, source_instance_id, source_event_id
  source_correlation_id?, sequence, source_state, occurred_at, expires_at?
  source_job_database_id?, source_job_slug?, run_id?
  claimed_user_id?, claimed_workspace_id?, provenance?
```

O adapter autenticado valida produtor e ocorrência persistida, resolve o job e
deriva o usuário. `claimed_user_id`/`claimed_workspace_id`, quando um protocolo
legado os transportar, são apenas assertions: divergência é rejeitada e esses
campos nunca compõem chave, lookup ou ownership. O dispatcher localiza regras
e layers somente em catálogo/SQLite pelo owner derivado e produz um evento
normalizado por regra correspondente. Esse envelope interno, não aceito
diretamente de produtor/cliente, é:

```text
LayerActivationEvent
  version, activation_id, rule_ref_kind, rule_ref, user_id, workspace_id?
  source_type, source_instance_id, source_event_id
  source_correlation_id?, sequence
  state, occurred_at, expires_at?
  auth_context_type, auth_context_id, auth_generation, security_generation
  source_replay_policy_generation, source_replay_deadline
  source_job_database_id?, source_job_slug?, chain_id?, chain_history?
```

No evento normalizado, `rule_ref_*`, `user_id`, `workspace_id`,
`activation_id`, contexto de autenticação e gerações/deadline são todos
derivados ou revalidados pelo backend. Nenhum valor homônimo do candidato pode
substituí-los; divergência detectável falha fechado antes de reservar o ledger.

`activation_id` é UUIDv7 novo a cada ciclo; ativar e desativar o mesmo ciclo
reutiliza esse ID. `sequence` cresce dentro de
`(user_id, workspace_id, rule_ref_kind, rule_ref,
activation_id)`. Evento duplicado com mesma sequência é idempotente; sequência
menor é ignorada; mesma sequência com conteúdo diferente falha fechado. Uma
desativação atrasada só encerra seu próprio `activation_id`, nunca uma ativação
mais nova. Expiração local não fabrica `LayerActivationEvent`: um scheduler usa
a chave estável `(activation_id, expires_at)` e, em transação, faz CAS de claim
ativa para terminal `expired` quando o prazo persistido vence, grava
`terminal_reason = expiry` e incrementa a geração do escopo. Repetição ou
restart encontra o mesmo estado/PK e vira no-op idempotente.

Estado `deactivate` ou expirado é terminal. Depois de `deactivate`, o CAS aceita
apenas replay idempotente da mesma sequência/fingerprint; depois de expiração
local, somente repetir a mesma chave de expiração é no-op. Ambos rejeitam
qualquer `activate`, mesmo com sequência maior. Novo ciclo exige novo
`activation_id`; evento tardio não ressuscita claim encerrada.

No envelope genérico, `state` aceita somente `activate` ou `deactivate`.
Cada adapter mapeia seu domínio antes de publicá-lo; no caso de jobs,
`queued`/`started`/`retry_scheduled` viram `activate` e estados terminais viram
`deactivate`. `source_event_id` é sempre UUIDv7 estável emitido pelo produtor e
`occurred_at` é timestamp autenticado; contador ou ID opaco de protocolo não é
aceito nesse envelope.

Na primeira versão, somente fatos persistidos em `job_run_events` e espelhados
na outbox durável desta AEP podem produzir esse envelope. Replay/reconciliação
consome a outbox, não o EventBus best-effort nem a linha sujeita à cascade do
run.

Para eventos da AEP-0067 entrarem depois, uma atualização daquela AEP precisa
definir outbox durável e publicar `_event_id` UUIDv7, `_occurred_at`,
`_correlation_id` e `_sequence`, persistidos transacionalmente antes do
dispatch e reutilizados no replay. O produtor fornece correlação/ordem; o
adapter resolve `activation_id` por regra. Até esse contrato existir, eventos
legados continuam para seus consumidores atuais, mas são indisponíveis como
ativadores de camada.

O ledger de ativação persiste `workspace_id` e `event_fingerprint`. A chave
única por ocorrência usa dois índices parciais: para camada global,
`(user_id, rule_ref_kind, rule_ref, source_event_id) WHERE workspace_id IS
NULL`; para camada local, `(user_id, workspace_id, rule_ref_kind, rule_ref,
source_event_id) WHERE workspace_id IS NOT NULL`.
`source_correlation_id` serve somente para localizar o ciclo em
`command_layer_activation_state`. Há índices auxiliares não únicos, separados
para escopo global/local, sobre usuário, workspace quando aplicável,
`rule_ref_kind`, `rule_ref`, `source_type` e `source_correlation_id`; a consulta
considera também estados terminais e falha fechado se encontrar mais de um
ciclo. O dispatcher processa criação/avanço sob o `DispatchGate` exclusivo, de
modo que dois eventos do mesmo ciclo não podem inserir `activation_id`
concorrentes. Ele
nunca substitui o `source_event_id`, que permanece obrigatório e é a única
chave idempotente de cada transição. Na mesma transação, o estado de PK
`activation_id` avança por CAS sobre `sequence`; update exige o cursor anterior.
Mesmo número com fingerprint diferente grava conflito e não altera a camada.

A regra persiste `event_name` exato e `allowed_internal_producer_types`; na
primeira versão, `event_name` só aceita
`command-context.job-run-state.v1` e o producer type só aceita `jobs.runtime`.
Webhook, plugin e outro produtor externo são rejeitados e ficam fora do escopo
até uma AEP definir identidade de ingress e grants próprios.
Criar/habilitar essa regra é mutação confirmável e persiste
um grant próprio desta AEP em `command_layer_automation_grants`. Esse grant
não reutiliza os grants de delegação de jobs da AEP-0101. Sua chave natural é
o escopo canônico `(user_id, workspace_id, layer_ref_kind, layer_ref,
rule_ref_kind, rule_ref)`; índices parciais separados representam workspace
global e local. A linha vincula `authorization_decision_id`, fingerprint
imutável da regra, `event_name` e fingerprint dos produtores permitidos.

Cada ciclo de concessão recebe `automation_grant_generation` monotônica dentro
da chave natural. O fingerprint do grant cobre chave natural, fingerprint da
regra, fato, produtores e geração. Só pode existir uma linha ativa por chave;
criação e revogação serializam sob o `DispatchGate`, releem a maior geração e
fazem CAS da linha ativa. Revogar preenche `revoked_at`, `revoked_by` e
`revocation_reason`; reconceder insere nova linha com geração maior, preservando
o histórico. Excluir ou desabilitar regra/layer, alterar condição, lifecycle,
fato, produtor, owner ou workspace revoga o grant ativo. Importar, duplicar ou
restaurar configuração nunca cria nem transporta grant; habilitar novamente
exige nova decisão explícita.

`command_layer_activation_rules` referencia o grant ativo por ID, geração e
fingerprint. Cada evento relê, pela chave natural derivada no backend, tanto a
regra quanto a linha ativa e exige que ID, geração e fingerprints coincidam;
grant ausente, revogado, divergente ou concorrente desabilita a regra e falha
fechado. Resposta pendente de `DecisionDialog` também carrega a geração
observada e é rejeitada se ela mudou antes da persistência. Essa ativação
pré-autorizada ocorre somente pelo adapter D8, não executa
`layer.activate`/`layer.toggle` como comando headless.

Sem usuário, regra, autenticação ou identidade válida — correlação, ou o par
instância/evento quando a correlação for ausente — eventos internos não alteram
camadas. Antes de atualizar o estado, o serviço compara
`auth_generation` e `security_generation` atuais; evento de sessão anterior,
logout ou estação bloqueada é descartado.

`user_id` e `workspace_id` não são aceitos como autoridade do payload. O
dispatcher deriva o dono do principal autenticado e o escopo da layer/rule
resolvida; `workspace_id = NULL` representa camada global. Isso também vale
para refs `builtin`, que não dependem de FK para recuperar o escopo. Jobs não
possuem workspace na AEP-0048: o dispatcher relê
`source_job_database_id`/`source_job_slug`/`run_id` apenas no usuário e deriva
o workspace exclusivamente da layer/rule, validando o acesso do mesmo usuário.
O mesmo evento pode alimentar regras de workspaces distintos,
cada uma com estado/chave próprios. Divergência, workspace inacessível ou
ausência de ownership falha fechado.

`source_instance_id` identifica a geração do dispatcher somente para
proveniência. A chave de idempotência é a chave global/local por
`source_event_id` definida acima, portanto replay estável após reinício
continua duplicata sem atravessar workspaces.
`source_replay_policy_generation` e `source_replay_deadline` são derivados pelo
adapter do mesmo `command_event_replay_policy_epochs` de D2.1 e não são aceitos
como autoridade do payload. Estado e ledger de ativação persistem ambos; seu
`expires_at` nunca antecede o maior entre o deadline da fonte e a retenção
terminal de ativações.

Se ledger/cursor terminal já tiver sido removido, evento posterior ao deadline
imutável do epoch vigente em `occurred_at` é rejeitado antes do insert.
Aumentar a retenção depois não reabre ocorrência de epoch anterior; diminuir
não encurta ledger existente. A idade vem somente do timestamp autenticado,
nunca do UUIDv7. Assim, limpeza delimita a deduplicação sem permitir replay
antigo reativar uma camada.

Exemplos:

```text
surface = chat                         → ativa Chat
foreground.process = code.exe          → ativa Desenvolvimento
app.focused = false                    → ativa Global
job run em andamento                   → ativa Execução
streamdeck.key.5 → layer.toggle        → alterna Trabalho
```

Mudanças de estado notificam um `ContextFactBus` do respectivo domínio,
in-process e não durável,
separado de `LayerActivationEvent`, com
`{ provider_id, instance_id, version, captured_at }`. Produtores confiáveis são
os registries locais da UI e os providers backend/adapters do SO, sem promover
uma notificação remota a snapshot autoritativo; duplicata de versão é idempotente. A
notificação apenas invalida cache: antes de cada resolução, o `VersionService`
consulta o snapshot atual do provider, portanto perda/reordenação não conserva
camada incorreta. O monitor de janela em primeiro plano é adapter específico por
sistema operacional; no Windows, não depende do software do Stream Deck.

No backend, o consumidor do `ContextFactBus` adquire `DispatchGate` exclusivo antes de trocar
snapshot e recalcular claims. Troca de snapshot sempre altera a versão do
provider em `context_version`, mas só incrementa o contador global/workspace de
`active_layers_generation` quando o conjunto efetivo de claims mudar.
Resolução/admissão lê somente providers locais backend sob o gate compartilhado. Se a versão
autoritativa diferir da versão usada no último cálculo de claims, libera o gate
compartilhado, adquire o exclusivo, compara novamente, reconcilia
sincronamente snapshot/claims/gerações e reinicia a resolução. Assim,
notificação perdida ou mudança já observada não conserva camada antiga nem
atravessa o CAS/início com geração antiga. Para fatos visuais, a UI relê sua
fonte antes do efeito síncrono, conforme a divisão de autoridade da D2. Não há
roundtrip Wails sob o gate, nem alegação de atomicidade entre o DOM e o CAS
backend. Condições visuais de camadas não podem autorizar efeitos backend
usando apenas uma notificação espelhada; a projeção/entrada precisa respeitar
os dois domínios e continuar indisponível quando faltar essa composição.

Para jobs, a integração publica o fato contextual interno versionado
`command-context.job-run-state.v1`, cujo `event_name` é exatamente esse nome,
com `user_id` como assertion candidata, `job_database_id`, `job_slug`,
`run_id`, `run_event_id`, `sequence`, `state`, `occurred_at`,
`root_origin_type`, `root_origin_id`, `_source`, `_source_job_id`, `_chain_id` e
`_chain_history`. `_source` deve ser
`job` nesse fato; outro valor falha fechado. `run_event_id` é o UUIDv7 de
`job_run_events.id` e vira o `source_event_id` estável, inclusive em replay.
`job_slug` recebe `Job.ID`, que no modelo atual é o slug público usado por
`eventctx.SourceJobID` conforme AEP-0067; `job_database_id` recebe
`Job.DatabaseID`, UUID de `jobs.id` referenciado por `job_runs.job_id`.
O adapter resolve primeiro `(user_id, job_slug)`, exige que a linha encontrada
tenha exatamente esse `DatabaseID` e então usa o UUID em correlação e
autorização. Fato legado que envia `job.ID` no campo ambíguo `job_id` ou omite
`job_database_id` é rejeitado.

`run_id` é tratado nesta borda como ID opaco e precisa corresponder exatamente
a uma linha `job_runs.id` do mesmo usuário e job. O adapter não valida
prefixo/formato nem converte IDs. A divergência entre o formato UUIDv7
documentado na AEP-0048 e produtores atuais deve ser corrigida em PR próprio,
com status/evidência da AEP-0048 atualizados, mas não bloqueia lookup seguro por
PK/ownership nesta integração. O adapter D8 permanece desabilitado até o
produtor persistir/publicar `Job.DatabaseID` como `job_database_id` e a AEP-0048
ser atualizada no mesmo PR; não há fallback silencioso do slug para UUID.

Esta AEP é dona de `root_origin_type` e de sua normalização:
`manual → manual`, `cron → cron`, `interval → interval`,
`hotkey → user_hotkey`, `webhook → external_event`; trigger `event` herda a
raiz autenticada do payload/provenance e vira `internal_event` somente quando o
produtor for interno conhecido. Raiz ausente vira `unknown`. Na v1, `manual`,
`cron`, `interval`, `user_hotkey` e `internal_event` são elegíveis;
`external_event`/`unknown` não ativam camada, mesmo quando o run intermediário
tenha `_source = job`.
O futuro PR da integração adiciona e persiste esse campo no runtime/timeline,
atualiza a AEP-0048 no mesmo ciclo e só então habilita o adapter; inferência
retroativa a partir do trigger atual é proibida.

Persistência incremental exige migração prévia da AEP-0048:
`job_runs.status` passa a aceitar `queued`, `running`, `retrying`, `completed`,
`failed` e `skipped`; ganha `queued_at` NOT NULL e torna `started_at` nullable.
Linhas existentes recebem `queued_at = started_at`; estados terminais não
mudam. O run é inserido como `queued`, muda para `running` ao iniciar e só então
preenche `started_at`. Duração continua calculada desde `started_at`, não da
fila. `job_run_events` referencia a linha já criada.

Como pré-requisito do adapter, o executor persiste
`job_run_events.type = queued` antes do despacho, `started` antes da tool e
`retry_scheduled` antes do backoff. Em paralelo, `job_runs.status` usa
respectivamente `queued`, `running` e `retrying`; os enums não são
intercambiáveis. Sem essas transições incrementais o fato v1 fica desabilitado.

O runtime gera `run_event_id` ao criar cada `RunEvent`, persiste
todos os estados mapeados — inclusive `completed`, `failed` e `skipped` —
incremental e idempotentemente antes de publicar o fato, reutilizando o mesmo
UUID na linha. O `defer LogRun` final vira upsert pelo UUID e não duplica eventos
já persistidos. O PR dessa integração atualiza a AEP-0048 e seus testes no mesmo
ciclo.

Na mesma transação de cada `job_run_events` elegível, o runtime insere
`command_job_activation_outbox` com o fato normalizado, owner, IDs
job/run/event, provenance, epoch/deadline de replay e fingerprint. Só publica
após commit. A outbox não referencia `job_runs`/`job_run_events` por FK com
cascade; `CleanRunsExceedingCount` pode remover runs e timeline sem apagar uma
ocorrência ainda reprocessável. O dispatcher faz claim com lease, processa
todas as regras elegíveis e marca `delivered`; crash devolve `processing`
vencido para `pending`. Falha permanente auditada vira `dead_letter`.

Outbox `pending`/`processing` não é removida por idade ou count-cap. Linha
`delivered`/`dead_letter` só sai depois de `source_replay_deadline`; até lá,
replay encontra a mesma PK/fingerprint. O
`InstanceMaintenanceCoordinator` processa/reconcilia essa outbox antes da
limpeza de jobs. O PR de implementação atualiza AEP-0048 e AEP-0074-B para
substituir a cascade como fronteira de replay; até outbox, ordem de manutenção e
testes de count-cap existirem, o adapter D8 permanece desabilitado.

`state` aceita `queued`, `started`,
`retry_scheduled`, `completed`, `failed` e `skipped`. A chave de
correlação é `(user_id, run_id)`; `sequence` impede regressão por entrega fora
de ordem. Estados `queued`, `started` e `retry_scheduled` mantêm a regra ativa;
`completed`, `failed` e `skipped` a encerram. Esse fato deriva do
runtime e da timeline `job_run_events` da AEP-0048; não inventa nomes no event
bus público da AEP-0001.

Essa lista é allowlist exaustiva. `triggered`, `event_emitted`,
`event_received` e qualquer tipo desconhecido são ignorados idempotentemente e
não produzem `LayerActivationEvent`. Gaps de `sequence` são permitidos; CAS
aceita somente valor maior que o cursor, não exige contiguidade.

O adapter usa `run_id` como `source_correlation_id` e resolve ou cria um
`activation_id` distinto por
`(user_id, workspace_id, rule_ref_kind, rule_ref, run_id)`. Assim, duas regras
que observam o mesmo run mantêm ciclos independentes. Ele preserva
`job_run_events.sequence` e mapeia `job_runs.status = retrying` para o fato
`state = retry_scheduled`. A timeline é a fonte de ordem; o status do run serve
apenas para reconstrução no startup.

Claim derivada de job usa lease própria, renovada por heartbeat do runtime:
`maintenance.command_job_activation_lease_seconds` (padrão 180), com heartbeat
antes da metade do TTL. Runs não terminais que sustentam claim ficam excluídos
da política/ciclo de limpeza da AEP-0074-B somente enquanto a lease estiver
válida. Lease expirada marca a claim inativa e devolve o run órfão à retenção no
próximo ciclo. O PR dessa integração deve atualizar
AEP-0074-B e AEP-0048 no mesmo ciclo. No startup, fonte ausente, lease vencida
ou estado não autoritativo torna a claim inativa até confirmação nova do
runtime. A linha pode permanecer para auditoria, mas nunca mantém a camada
efetiva sem lease válida.

O adapter preserva a proveniência anti-loop da AEP-0067. Se um binding ativado
por esse ciclo iniciar job, tool que publica evento ou outro comando reativo, a
invocação herda `_chain_id`/`_chain_history` sem acrescentar namespaces que não
sejam jobs. `provenance.command_chain_history` é lista ordenada de
`{ command_id, invocation_id, layer_refs }`, persistida no mesmo documento
redigido do envelope. `CommandExecutionService` é dono do limite constante
versionado `CommandMaxChainDepth = 16`: antes da reserva, valida a lista, rejeita
repetição do mesmo `command_id` na cadeia e acrescenta a entrada atual. Ao
iniciar novo job, somente o runtime de jobs acrescenta o job à
`_chain_history` e chama `DetectLoop`/`MaxChainDepth`. Evento derivado de job sem
proveniência não pode habilitar comando capaz de ampliar a cadeia; falha
fechado.

Cancelamento não pertence ao enum vigente da AEP-0048 e, portanto, não é
inventado aqui. Se o runtime ganhar esse estado, a AEP-0048 deve ser atualizada
antes de ele entrar no fato contextual v1 ou em uma versão posterior.

Trocas rápidas passam por estabilização curta. O usuário pode fixar uma camada,
criando a claim manual persistente descrita acima; isso impede sua remoção por
automação, mas não congela as outras camadas. Se o contexto deixar de ser
confiável, o resolvedor retorna ao conjunto padrão seguro.

### D9 — Command Palette

A Command Palette usa o mesmo registro e deve permitir:

- buscar por nome, descrição, categoria e aliases localizados;
- executar comandos disponíveis;
- mostrar atalho efetivo no contexto atual;
- listar recentes e favoritos;
- solicitar argumentos por formulário quando faltarem;
- abrir diretamente a configuração do comando ou binding.

Comandos indisponíveis podem ser exibidos com o motivo, em vez de falhar
silenciosamente.

A palette abre com foco no campo de busca, usa o padrão acessível
combobox/listbox, permite setas para navegar e Enter para executar. Escape fecha
e restaura o foco ao elemento que a abriu. Quantidade de resultados, comando
indisponível, sucesso e erro são anunciados pelo announcer global. Formulários de
argumentos e confirmações seguem os componentes compartilhados; abrir uma nova
surface transfere o foco segundo o contrato dessa surface. Testes cobrem
teclado, focus trap, restauração de foco e axe, com validação manual por NVDA
antes de concluir a fase.

### D10 — Gerenciamento por chat

O agente gerencia o sistema por duas tools compostas, seguindo a convenção da
AEP-0048 para não inflar o catálogo:

- `command_catalog`, com ações `list`, `describe` e `execute`;
- `command_config`, com ações `layer_list`, `layer_get`, `layer_create`,
  `layer_update`, `layer_delete`, `layer_enable`, `layer_disable`,
  `layer_restore`, `binding_list`, `binding_check_conflict`,
  `binding_create`, `binding_update`, `binding_delete`, `binding_enable`,
  `binding_disable`, `binding_restore`, `config_import` e `config_export`.

Alterações destrutivas, conflitos e comandos sensíveis continuam sujeitos ao
contrato de decisão da AEP-0091. A resposta da tool inclui IDs reais e o efeito
resolvido; o modelo não edita tabelas diretamente.

Além disso, toda ação mutável de `command_config` solicitada por agente mostra
ao usuário o diff exato e exige decisão explícita, mesmo que não seja
destrutiva: criar ou habilitar um binding pode conceder capacidade futura.
Origem headless falha fechado. A confirmação autoriza somente aquela mutação e
não concede grant reutilizável para executar o comando configurado.

O registro marca `mutates_effective_capability` em comandos como
`layer.activate`, `layer.toggle`, `layer.back` persistente e registro de hotkey.
Para todo `actor_type != user`, `CommandExecutionService` exige decisão em
`command_catalog.execute` se esse flag estiver ativo ou se
`decision_requirement` exigir; comandos read-only com requisito `none` seguem
sem diálogo. Para os casos confirmáveis, origem headless falha fechado.
Automação de ativação usa exclusivamente a regra previamente confirmada da D8;
um binding `event` não pode chamar esses comandos mutáveis. Assim, não existe
segunda rota para alterar o mapa efetivo.

A marca não é declarada livremente pelo autor do comando.
`CommandHandler.EffectClass()` e `CommandHandler.Mutability()` fornecem,
respectivamente, `effect_class` e `mutates_effective_capability`; o registro
rejeita divergência com metadata e combinações inválidas de risco/decisão. Em
`command_config`, somente list/get/check_conflict e
`config_export` sem credenciais são leitura; create, update, delete, enable, disable, restore e
import são mutações de capacidade e sempre exigem o gate. Teste de catálogo enumera todas as ações/handlers para impedir que
um verbo novo nasça sem classificação.

`config_export_sensitive` existe apenas no registro/UI, não na tool
`command_config`: risco alto, `decision_requirement=interactive`, somente
`ui.action`, formulário de senha e criptografia/redaction da AEP-0047; chat e
origem headless são proibidos. `config_export` rejeita
`includeCredentials=true` em vez de promover silenciosamente.

### D11 — Persistência

Defaults ficam no código. SQLite guarda entidades do usuário e deltas:

```text
command_layers
  id, user_id, workspace_id nullable_for_global, name, description, enabled, source,
  resolution_priority, created_at, updated_at

command_layer_activation_rules
  id, user_id, workspace_id nullable_for_global,
  layer_ref_kind, layer_ref, rule_ref_kind, rule_ref,
  mode, condition, lifecycle, event_name,
  allowed_internal_producer_types, authorization_decision_id,
  automation_grant_id, automation_grant_generation,
  automation_grant_fingerprint, enabled, source,
  replaces_default_id nullable, replaces_default_version nullable,
  replaces_default_fingerprint nullable, review_status

command_layer_automation_grants
  id, user_id, workspace_id nullable_for_global,
  layer_ref_kind, layer_ref, rule_ref_kind, rule_ref,
  rule_fingerprint, event_name, producer_types_fingerprint,
  automation_grant_generation, automation_grant_fingerprint,
  authorization_decision_id, granted_at, granted_by,
  revoked_at nullable, revoked_by nullable, revocation_reason nullable

command_bindings
  id, user_id, workspace_id nullable_for_global, layer_ref_kind, layer_ref,
  trigger_type, trigger_spec, command_id nullable_for_suppress,
  arguments empty_for_suppress,
  condition, effect, enabled, source, resolution_priority, replaces_default_id,
  replaces_default_version, replaces_default_fingerprint, review_status,
  presentation

command_config_generations
  id, user_id, workspace_id nullable_for_global, generation, updated_at

command_config_mutations
  mutation_id PK UUIDv7, schema_version, user_id, session_id,
  scope, operation, binding_id, decision_id UNIQUE, request_fingerprint,
  auth_generation, security_generation, generation_id,
  before_generation, after_generation, before_enabled, after_enabled,
  occurred_at

command_event_replay_policy_epochs
  id, producer_type, generation, effective_at, replay_horizon_seconds,
  created_at

command_key_versions
  id PK UUIDv7, version UNIQUE, digest SHA-256, active

command_job_activation_outbox
  source_event_id PK UUIDv7, user_id, job_database_id, job_slug, run_id,
  sequence, state, occurred_at, root_origin_type, provenance,
  source_replay_policy_generation, source_replay_deadline,
  event_fingerprint, delivery_state, lease_owner nullable,
  lease_expires_at nullable, attempts, last_error_code nullable,
  created_at, delivered_at nullable

command_decision_receipts
  decision_id PK UUIDv7, user_id, auth_context_type, auth_context_id,
  auth_generation, security_generation, subject_type, subject_id,
  request_fingerprint, allowed_action_ids, accepted_action_id nullable,
  status, expires_at, responded_at nullable, consumed_at nullable

command_layer_activation_state
  activation_id PK UUIDv7, layer_ref_kind, layer_ref, rule_ref_kind, rule_ref,
  user_id, workspace_id nullable_for_global,
  auth_context_type, auth_context_id, auth_generation,
  security_generation, source_type, source_instance_id nullable,
  source_event_id nullable, source_correlation_id nullable, sequence nullable,
  source_job_database_id nullable, source_job_slug nullable,
  event_fingerprint nullable, source_replay_policy_generation nullable,
  source_replay_deadline nullable, state, terminal_reason nullable,
  provenance nullable, manual_stack_key nullable, activated_at,
  expires_at nullable, updated_at

command_activation_idempotency_keys
  id, key, user_id, workspace_id nullable_for_global,
  rule_ref_kind, rule_ref, source_type, source_instance_id, source_event_id,
  source_correlation_id nullable, sequence, event_fingerprint,
  source_job_database_id nullable, source_job_slug nullable,
  source_replay_policy_generation, source_replay_deadline,
  terminal_state, created_at, expires_at

external_identity_mappings
  id, issuer, subject, user_id, enabled, created_at, updated_at

command_invocations
  invocation_id PK UUIDv7, schema_version, user_id nullable_for_system,
  auth_context_type, auth_context_id,
  auth_generation, session_id nullable_for_non_local, security_generation,
  workspace_id nullable_without_workspace, registry_version,
  global_config_generation nullable_for_system,
  workspace_config_generation nullable_without_workspace,
  active_layers_generation nullable_for_system,
  command_id nullable_until_resolved, binding_ids nonnull_default_empty,
  observed_trigger_type nullable_for_direct,
  trigger_type nullable_for_direct, trigger_spec_snapshot nullable_for_direct,
  trigger_fingerprint nullable_for_direct,
  actor_type, actor_id, source_type nullable_until_resolved,
  observer_type nullable,
  source_instance_id nullable, source_event_id nullable,
  source_occurred_at nullable_for_non_event,
  source_replay_policy_generation nullable_for_non_event,
  source_replay_deadline nullable_for_non_event,
  arguments_summary, arguments_fingerprint,
  conversation_id nullable, turn_id nullable,
  surface_type nullable, surface_id nullable, surface_snapshot_version nullable,
  context_version nullable_for_none,
  context_captured_at_by_provider nullable_for_exact_version_or_none,
  context_summary nullable_for_none, foreground_summary nullable,
  source_profile_slug nullable, target_profile_slug nullable,
  authorization_decision_id nullable_until_decided,
  delegation_fingerprint nullable, grant_generation nullable,
  job_id nullable, job_slug nullable, job_definition_fingerprint nullable,
  run_id nullable, provenance nullable,
  correlation_id, request_fingerprint_version, request_fingerprint,
  risk, policy_decision,
  result_summary nullable_until_terminal, result_ref nullable,
  status, error_code nullable_for_succeeded_or_nonterminal,
  client_requested_at nullable, received_at,
  completed_at nullable_until_terminal

command_idempotency_keys
  id, key, invocation_id, user_id nullable_for_system,
  auth_context_type, auth_context_id,
  source_type nullable_until_resolved, source_instance_id nullable,
  source_event_id nullable, source_occurred_at nullable_for_non_event,
  source_replay_policy_generation nullable_for_non_event,
  source_replay_deadline nullable_for_non_event,
  request_fingerprint_version, request_fingerprint,
  status, result_summary, result_ref, received_at, expires_at
```

No ledger, `id` é PK UUIDv7, `key` é UNIQUE e vale
`invocation:<invocation_id>` para toda solicitação sem `source_event_id`,
direta ou resolvida por trigger, e `event:<source_event_id>` quando esse ID
existir. Portanto palette, `ui.action`, chat e CLI por trigger usam a primeira
forma. `invocation_id` também é UNIQUE e
`source_event_id` tem índice único parcial quando não nulo. Conflito relê
ownership e fingerprint antes de classificar como reentrega; divergência falha
fechado. `status` aceita os estados de invocação, `suppressed` e
`rejected_stale`; nos dois últimos, `invocation_id` continua obrigatório, mas
não é FK para
`command_invocations`, pois não há linha de auditoria e o ledger sobrevive à
compactação.

No ledger de ativação, `key` inclui escopo canônico:
`activation:<user_id>:global:<rule_ref_kind>:<rule_ref>:<source_event_id>` ou
`activation:<user_id>:workspace:<workspace_id>:<rule_ref_kind>:<rule_ref>:<source_event_id>`,
e é UNIQUE. Os dois índices parciais equivalentes são os definidos na D8.
Assim, o mesmo evento pode alimentar regras ou workspaces distintos sem
colisão e não reaplica a mesma regra no mesmo escopo.

Condições, argumentos, especificações e apresentação são documentos JSON
versionados e validados. Alterações relevantes mantêm auditoria suficiente para
desfazer.

`command_config_mutations` registra mudanças confirmadas de configuração, não
invocações de comandos. O subconjunto v1 é fechado em
`scope=global`/`operation=binding_enabled`: IDs e gerações vinculam a receipt,
os booleanos antes/depois preservam o efeito reversível, e o fingerprint
versionado identifica a proposta sem persistir seu texto ou documentos. Registro,
consumo da receipt e mudança de configuração pertencem à mesma transação.
Uma operação revertida não deixa linha de sucesso. Consulta interna é escopada
por usuário/sessão e UUID da mutação. Não há exclusão automática ou undo
automático; política de retenção e restauração confirmada continuam pendentes.

`command_bindings` usa a mesma referência polimórfica `builtin|user` do estado
de ativação. Ref `user` precisa apontar para `command_layers` do mesmo
usuário/workspace; ref `builtin` é validada no catálogo e só aceita delta com
`replaces_default_*`. O owner e o escopo ficam na própria linha para que
restore/import funcionem sem materializar defaults. Binding inteiramente novo
só pode referenciar layer `user`.

Campos marcados com `?` no envelope são nullable no SQLite; ausência vira
`NULL`, nunca string vazia. `command_id` fica nulo em `evaluating`/`denied`
somente quando a resolução prévia não encontrou comando; em toda reserva
resolvida executável é preenchido e imutável. Supressão terminal é a exceção
sem `CommandInvocation` da D4. `trigger_*` é nulo em execução direta;
`session_id` é nulo fora de `local_session`; campos de surface, conversa, job,
profile, workspace e decisão são nulos quando o contexto não se aplica.
`source_occurred_at` é obrigatório somente para
`source_type = event`, vem da fonte autenticada e fica nulo nas demais origens.
Nesse caso, `source_replay_policy_generation` e `source_replay_deadline`
também são obrigatórios e derivam do epoch de política, nunca do payload.
Campos obrigatórios do envelope e `binding_ids` (default `[]`) são NOT NULL.

Constraints condicionais validam os grupos: conversa e turno aparecem juntos;
surface exige o trio tipo/ID/versão ou todos `NULL`; delegation fingerprint e
generation aparecem juntos; `job_service` exige job ID/slug/fingerprint e
run ID, enquanto outros contextos os deixam `NULL`; policy com providers exige
`context_summary`, e a que declara foreground exige também
`foreground_summary`; origem job/evento reativa exige `provenance`. Ator agente
exige `source_profile_slug` e `target_profile_slug`; demais atores só os
preenchem quando houver delegação explícita. Combinação parcial falha antes da
reserva.

Todas as PKs persistidas criadas por esta AEP são UUIDv7 conforme AEP-0046.
FKs entre essas tabelas também usam UUIDv7. IDs de defaults que vivem no código
são strings namespaced estáveis; `replaces_default_id` referencia essa
identidade lógica, não uma linha SQLite.

`command_event_replay_policy_epochs` tem UNIQUE
`(producer_type, generation)` e `(producer_type, effective_at)`. A implantação
cria o epoch inicial antes de habilitar o adapter D8; alteração da retenção de
jobs persiste o novo epoch na mesma seção crítica que publica a configuração.
Lookup por `source_occurred_at` escolhe o maior `effective_at` não posterior ao
evento. Ausência ou ambiguidade falha fechado.

Regras persistidas e estado de ativação usam referências polimórficas
validadas, não FKs:
`layer_ref_kind`/`rule_ref_kind` aceitam `builtin` ou `user`; refs builtin são
IDs namespaced do catálogo em código e refs user são UUIDv7 que precisam
pertencer ao mesmo usuário. Isso permite ativar defaults sem copiá-los para
`command_layers` e mantém restore sob ownership do catálogo.

Em `command_layer_activation_rules`, `id` é a PK física da linha. Regra criada
pelo usuário usa `rule_ref_kind = user` e `rule_ref = id`; delta de regra
padrão usa `rule_ref_kind = builtin`, o ID namespaced do catálogo em `rule_ref`
e o trio `replaces_default_*`. A referência de layer é independente: tanto
regra user quanto delta builtin podem apontar para layer `builtin` ou `user`,
desde que o owner e o workspace canônicos coincidam. Default puro continua no
código e não exige linha SQLite. Pares de índices únicos parciais impedem duas
linhas com a mesma referência de regra no escopo global/local; `needs_review`
bloqueia a regra sem fallback, como nos bindings.
Habilitar uma regra builtin event-driven que exija grant materializa primeiro
esse delta confirmado; assim, grant, geração e revogação têm uma linha
autoritativa sem copiar a camada/default inteira.

`binding_ids` é uma lista JSON ordenada que registra todos os bindings
equivalentes considerados na deduplicação; fica vazia para execução direta.
O acionador efetivo também fica em snapshot normalizado/redigido e fingerprint,
portanto excluir o binding ou atualizar defaults não apaga sua origem histórica.
`status` aceita `evaluating`, `queued`, `running`, `succeeded`, `failed`, `denied`,
`cancelled`, `cancelled_stale`, `timed_out` e `outcome_unknown`. `result_summary` é redigido e
`result_ref` guarda somente referência estável e não sensível, como o ID de uma
aba ou run, permitindo consultar uma reentrega sem repetir efeitos.
Em `evaluating`, `queued` e `running`, `result_summary`, `result_ref`,
`error_code` e `completed_at` são `NULL`. Todo estado terminal exige
`result_summary` redigido — objeto vazio quando não houver retorno — e
`completed_at`; `result_ref` continua opcional. `error_code` é `NULL` em
`succeeded` e obrigatório nos demais terminais, com código estável inclusive
para cancelamento, staleness, timeout e outcome desconhecido.

`command_layer_activation_state` mantém o último cursor de cada ciclo. No
startup, regras `always` e contextuais são recalculadas e não ocupam essa
tabela; ciclo manual só é
restaurado se seu lifecycle for persistente e o usuário for autenticado
novamente; temporário expirado ou session-scoped termina; ciclo de evento/job é
reconciliado com a fonte. Todo estado sem autenticação válida fica inativo;
evento/job também exige provenance e fonte revalidáveis. Claim manual
persistente exige dono autenticado e lifecycle válido, mas não provenance de
job. Ciclos terminais permanecem pela mesma retenção curta das invocações para
deduplicar reentregas; ativos não são removidos pela idade.

Ao restaurar claim manual persistente após login/restart, o serviço revalida
ownership/layer e, numa transação, substitui auth/security generations e
`manual_stack_key` pelo novo contexto antes de ativá-la. Origem/dispositivo não
mais disponível deixa a claim inativa e visível para revisão; `layer.back` só
opera sobre claims já rebindadas.

Matriz de nulabilidade do estado: claims manuais exigem `manual_stack_key` e
podem deixar `source_instance`, `source_event`, correlação, sequence e
provenance nulos; claims de evento deixam `manual_stack_key` nula e exigem
instância, evento UUIDv7, sequence, fingerprint, geração/deadline de replay;
correlação é opcional; claim temporária exige `expires_at`; provenance é
obrigatória quando a origem for job.
Campos de auth/segurança e refs de layer/rule são sempre obrigatórios.
`workspace_id` é obrigatório para layer local e nulo somente para layer global;
é persistido também no ledger mínimo. Ausência vira `NULL`, nunca sentinel vazio.

`workspace_id` nulo identifica camada global do usuário; preenchido identifica
camada daquele workspace. A consulta efetiva carrega somente camadas globais do
usuário autenticado mais as do workspace atual. SQLite usa dois índices únicos
parciais: `(user_id, name) WHERE workspace_id IS NULL` para globais e
`(user_id, workspace_id, name) WHERE workspace_id IS NOT NULL` para workspaces.

Os grants de automação também usam pares de índices parciais para a chave
natural global/local. Outro par de índices únicos parciais, filtrado por
`revoked_at IS NULL`, garante no máximo um grant ativo por chave. Um índice
único adicional sobre chave natural mais `automation_grant_generation`
preserva a monotonicidade auditável; a transação sob `DispatchGate` relê a
maior geração antes do insert. `automation_grant_id` referencia exatamente a
linha ativa e nunca é inferido apenas pelo fingerprint.
Bindings herdam o escopo da camada, evitando misturar configurações.

`command_config_generations` usa os mesmos dois índices únicos parciais de
escopo: uma linha global por usuário e uma por usuário+workspace. Toda mutação
incrementa a linha aplicável na mesma transação dos dados alterados.

`external_identity_mappings` tem índice único `(issuer, subject)` e FK para
`users.id`. Só administração autenticada pode criá-lo; ele não é importado,
exportado nem inferido por login.

Abrir ou carregar um workspace não autoriza conteúdo controlado pelo workspace
— arquivos do projeto, metadados importados ou eventos emitidos por ele — a
criar ou habilitar bindings de shell, MCP, hotkeys globais ou ações externas.
Essas operações exigem ator autenticado e o fluxo explícito de configuração.

Exportação e importação integram o envelope versionado da AEP-0047 pela seção
`resources.commandLayers`. Cada camada inclui UUID, escopo portátil,
`activationRules` e `bindings`; overrides incluem ID, versão e fingerprint do
default para round-trip de `needs_review`. Defaults puros, invocações e todo
`command_layer_activation_state` — inclusive pin/claim manual persistente — não
são exportados. Camadas importadas começam sem claims manuais; regras
contextuais são recalculadas no destino. Referências internas
são remapeadas em conjunto e a importação é idempotente por UUID.
Para `activationRules`, refs builtin são validadas no catálogo e refs user são
remapeadas com a camada/lote; owner e workspace vêm do destino autenticado.
Regra event-driven importada permanece desabilitada sem grant e exige
confirmação local para habilitar.

Bindings persistentes não armazenam segredo bruto. Paths marcados como
sensíveis pelo comando aceitam somente referência ao cofre/credencial; quando
o schema não permite separar o segredo, aquele comando só admite execução
ad hoc e não pode virar binding persistente. O export inclui a referência, nunca
o valor. Se o usuário escolher `includeCredentials`, o segredo viaja apenas no
bloco criptografado definido pela AEP-0047. Exportação faz nova validação e
recusa o arquivo, com relatório, se encontrar configuração antiga que viole
essa regra.

Referência portátil de credencial usa `{ kind: "credential", pattern }`, nunca
UUID local. Na importação, credenciais do bloco criptografado são tratadas
primeiro pela AEP-0047; depois, cada binding resolve o `pattern` exato somente
no usuário de destino. Ausência, ambiguidade ou pattern pertencente a outro
usuário deixa o binding desabilitado e entra no relatório. Não há associação
automática por posição, nome aproximado ou ID da instância de origem.

Conflito de UUID com conteúdo diferente exige escolha explícita entre manter,
substituir ou importar como cópia com novos UUIDs. Referência a workspace,
comando, dispositivo ou default ausente fica desabilitada e entra no relatório
de importação; não é aproximada por nome. Grants, autorizações e ativações
temporárias nunca são exportados ou concedidos. A configuração importada só
entra no mapa efetivo após validação e confirmação dos conflitos.

Toda busca/upsert usa `(user_id autenticado, id)`. UUID já pertencente a outro
usuário retorna conflito `foreign_owner` sem revelar conteúdo, sobrescrever ou
associar referência. UUID ausente é criado para o usuário autenticado,
ignorando qualquer owner do arquivo. A opção “cópia” gera novos UUIDs e remapeia
somente relações internas validadas daquele lote.

`workspace_id` portátil nunca é aceito sem resolução. O import recebe mapa
explícito origem→workspace de destino; ID igual só é reutilizado após repository
confirmar ownership/acesso do usuário autenticado. Ausência, ambiguidade ou
destino não autorizado desabilita a camada e entra no relatório antes do
commit.

Se “cópia” colidir com nome único no mesmo escopo, exige novo nome explícito
antes do commit; a UI pode sugerir rótulo localizado, mas não persiste enquanto
ele não for único. Validação e insert ocorrem na mesma transação para fechar
corrida.

Essa seção só é habilitada depois que o mesmo PR atualizar a AEP-0047, registrar
`commandLayers` e incrementar a versão do envelope portátil com regras de
compatibilidade. Até lá, comandos/camadas são recurso não suportado pelo export:
o relatório avisa a omissão e a UI não promete backup deles. Importador antigo
continua ignorando seção desconhecida com warning, como define a AEP-0047.

`command_invocations` é auditoria técnica efêmera.
`internal/commandinvocations.MaintenanceService`, definido por esta AEP e
executado pelo novo `InstanceMaintenanceCoordinator`, opera em escopo
privilegiado da instância: enumera todos os usuários e também `user_id IS NULL`,
sem depender do usuário ativo. A implementação adiciona APIs de manutenção
separadas: uma enumera IDs de usuário; cada limpeza por usuário recebe contexto
interno com aquele `user_id` e preserva `RequireUserID`; registros system usam
métodos dedicados que exigem capability da instância. O coordenador nunca
remove o guard nem passa contexto sem usuário a APIs comuns. Ele absorve a
cadência hoje iniciada por `jobs.Manager.runRetention` e chama, por interfaces,
numa única goroutine e nesta ordem: reconciliação de leases da
`command_job_activation_outbox`; retenção de jobs;
`ToolInvocations.CleanOldDryRuns`; `CleanOrphanChat`; `CleanOldChat`; retenção
de invocações/ledgers de comandos; retenção de ativações; e compactação física
por `maybeCompact`. O futuro PR de implementação atualizará a AEP-0074-B e
moverá a responsabilidade sem perder nenhuma limpeza nem vacuum/compactação;
não criará loop paralelo. Este PR documental apenas registra esse requisito.

O serviço lê exclusivamente
`maintenance.command_invocation_retention_days` (padrão 30) e
`maintenance.command_invocations_per_user_keep` (padrão 10.000) de
`MaintenanceSettings`/`config.json`. Invocações internas sem usuário usam
`maintenance.command_invocations_system_keep` (padrão 1.000), além do mesmo
limite por idade. Para ciclos terminais, usa
`maintenance.command_activation_terminal_retention_days` (padrão 30) e
`maintenance.command_activation_terminal_keep_per_user` (padrão 10.000), além
de `maintenance.command_job_activation_lease_seconds` (padrão 180). Estados
ativos ficam fora da limpeza por idade/quantidade. As seis chaves
aparecem na mesma UI de manutenção. O PR que implementar esta fase deve
atualizar settings e UI no mesmo ciclo; não se cria configuração paralela.

O serviço remove registros antigos/acima do limite. Índices mínimos:
`(user_id, received_at)`, `(user_id, status, received_at)`, PK única por
`invocation_id` para chamadas diretas e índice único parcial
`(source_event_id) WHERE source_event_id IS NOT NULL AND source_event_id <> ''`
para eventos de adapter. Evento físico sempre tem usuário autenticado; contexto
`system` sem usuário não usa esse índice e possui
`(received_at) WHERE user_id IS NULL` para sua limpeza global.

Os limites de quantidade removem somente auditoria detalhada em
`command_invocations` e estado terminal em
`command_layer_activation_state`. Os dois ledgers mínimos não são removidos por
cap: permanecem até `expires_at`, calculado com a respectiva retenção
configurada. Para `source_type = event`, o piso adicional, a consulta da fonte e
a rejeição por `source_occurred_at` seguem D2.1; a chave não é removida enquanto
a fonte ainda puder reentregá-la. Só depois desses gates a chave pode ser
reutilizada. Status/resultado redigido e último
fingerprint/sequence são atualizados no ledger na mesma transação da mudança de
estado. `command_idempotency_keys.key` e
`command_activation_idempotency_keys.key` têm índices únicos. Assim, compactar
uma linha recente não reabre a execução nem a ativação.

Não se persiste `arguments` bruto nem o `SurfaceContext` completo:
`arguments_summary` é redigido; `arguments_fingerprint` e
`request_fingerprint` são HMACs domain-separated calculados no backend sobre os
valores canônicos ainda não redigidos e só o digest é persistido. Assim, valores
sensíveis diferentes não colapsam no mesmo placeholder. Identidades de
conversa/turno/surface, `surface_snapshot_version`, `context_version` e resumo
redigido permitem rastrear a origem sem copiar segredos. O conteúdo completo do `SurfaceContext` não é
reconstituível depois de expirar e esta AEP não promete reprodução integral.
Auditorias de decisão/grant que tenham retenção própria na AEP-0091 ou AEP-0101
não são substituídas por esta tabela.

Quando um comando delega para tool, `tool_invocations` precisa ganhar
`origin_type = command_invocation` e
`origin_id = command_invocations.invocation_id`. O primeiro PR dessa integração
atualiza a AEP-0063, enum, consultas e retenção no mesmo ciclo. Até isso existir,
comando que delega para tool fica indisponível; usar `system` como fallback é
proibido porque misclassificaria ação de usuário/agente como automação interna.
A origem completa permanece em `command_invocations` e a UI tolera o lado
técnico já expirado.

### D12 — Resolução eficiente

O resolvedor não percorre todas as camadas nem consulta o banco a cada tecla.

- configurações são carregadas e validadas na memória;
- bindings são indexados pela identidade normalizada do acionador;
- o conjunto de camadas ativas é mantido separadamente;
- para uma entrada, somente candidatos daquele acionador são avaliados;
- resultados frequentes podem ser cacheados pela tupla exata
  `(user_id, workspace_id, trigger_identity, source_type, context_version,
  registry_version, global_config_generation, workspace_config_generation,
  active_layers_generation)`;
- mudanças de contexto invalidam apenas entradas afetadas.

O cache positivo e o negativo exigem igualdade de todos os componentes; valor
nullable usa marcador tipado, nunca string vazia. Mudar catálogo, default,
binding, prioridade, camada ou claim incrementa a geração correspondente sob o
`DispatchGate` antes de publicar o novo snapshot, portanto entrada anterior não
é reutilizada nem para executar nem para repetir recusa stale. Usuário,
workspace e identidade normalizada do acionador nunca são inferidos apenas de
`context_version`.

Para o Stream Deck, a composição é recalculada quando camadas, contexto ou
estado visível mudam. O renderer compara o estado anterior e atual e envia ao
dispositivo somente teclas alteradas. Imagens redimensionadas ficam em cache.
Ao abrir ou reconectar um handle, invalida o estado renderizado daquele
dispositivo e força frame completo antes de voltar ao diff incremental.

### D13 — Stream Deck direto por Go

O Assistente controla o Stream Deck diretamente por USB/HID. Não há dependência
do aplicativo ou plugin oficial da Elgato.

O adapter deve:

- enumerar modelos suportados e suas geometrias;
- abrir e possuir exclusivamente cada dispositivo;
- receber eventos de tecla;
- renderizar imagem, título e estado;
- detectar remoção e reconectar com backoff;
- suportar mais de um dispositivo sem confundir identidade e posição;
- liberar os dispositivos no shutdown;
- degradar sem impedir a inicialização do Assistente.

Uma prova de conceito externa existente usa
`rafaelmartins.com/p/streamdeck` em Go. Antes de torná-la dependência do produto,
a implementação deve verificar licença, manutenção, modelos suportados,
reconexão, distribuição nos sistemas-alvo e compatibilidade com o build Wails.

Enquanto o Assistente possuir o dispositivo, outro processo, incluindo a prova
de conceito, pode não conseguir abri-lo. A UI deve informar essa disputa sem
encerrar o aplicativo.

O processo pode manter o dispositivo aberto antes do login, mas sem sessão
autenticada ele fica em estado seguro: imagem neutra ou apagada, sem bindings
ativos, e todo evento físico é rejeitado. No logout ou troca de usuário, o
gerenciador invalida atomicamente a geração da sessão, cancela despachos ainda
não iniciados, remove do estado em memória as camadas, bindings e caches do
usuário anterior — sem excluir sua persistência — e renderiza o estado seguro
antes de carregar outra conta. Callbacks carregam a geração da sessão e são
recusados se ficarem obsoletos. Somente depois de carregar e validar o novo mapa
ocorre nova renderização.

O estado `os.session_locked` faz parte do contexto de segurança. Ao bloquear a
estação, o adapter do sistema operacional incrementa a geração de segurança,
cancela invocações reservadas ainda não despachadas, renderiza o Stream Deck no
estado seguro e rejeita hotkeys globais e eventos de dispositivo. Não há
allowlist implícita durante o bloqueio. Ao desbloquear, bindings só voltam após
revalidar a sessão autenticada e reconstruir o mapa do usuário atual.

O estado visual de uma tecla é apresentação do binding efetivo. Pode ter título,
ícone padrão, imagem escolhida pelo usuário e variantes como ligado, desligado,
executando, concluído e erro. Texto, anúncio e estado não podem depender apenas
de imagem ou cor.

Apresentações builtin usam `title_key` e `status_label_keys` existentes em
pt-BR, en e es. Conteúdo personalizado pode fornecer `title_by_locale`; locale
ausente cai para o nome localizado do comando, nunca para string builtin
hardcoded. Anúncios usam as mesmas chaves/fallbacks.

Implementação da seção146: `presentation.states` contém variantes para `on`,
`off`, `waiting`, `running`, `succeeded`, `failed`, `denied`, `cancelled`,
`timed_out` e `outcome_unknown`. Cada variante aceita somente `title_by_locale`,
`icon` e `image_ref`; campos ausentes herdam o padrão, por idioma no caso dos
títulos. Não há variantes aninhadas. O consenso entre bindings elegíveis é
calculado depois da herança, sem escolher arbitrariamente uma imagem ou título.
Uploads transitórios são normalizados antes da confirmação; base e variantes
são gravadas na mesma transação, fora do documento canônico de 64 KiB.

Para `layer.activate` e `layer.toggle`, ligado/desligado apresenta o estado
efetivo conhecido da camada-alvo, não o sucesso da última execução nem uma
promessa de que a próxima alternância removerá ativações de outras origens.
Metadados de regras e camadas pertencem à mesma projeção publicada que resolve
os bindings. Caminhos contextuais não provados omitem o indicador, em vez de
inventar desligado. Feedback transitório do executor tem prioridade e expira
para o estado atual. Não se atribui estado persistente a comandos sem fonte
autoritativa, nem se acrescenta auditoria aos comandos locais de navegação.

Uma tecla que ativa outra camada oferece navegação semelhante a pasta, mas
continua usando o mecanismo genérico `layer.activate`, `layer.toggle` ou
`layer.back`. Teclado, chat ou outro dispositivo podem ativar a mesma camada.

### D14 — Contexto de programas externos

Quando o Assistente estiver sem foco, hotkeys globais e dispositivos físicos
podem usar camadas ativadas pelo programa em primeiro plano.

O adapter do SO captura processo, identidade da janela e versão em
`foreground_snapshot` no instante do evento, antes de qualquer
`WindowPort.Show`/bring-to-front. O resolvedor usa esse snapshot com
`context_policy = event_snapshot`; trazer o Assistente à frente não troca a
camada daquele evento. Snapshot só é aceito do adapter confiável e continua
sujeito a auth/security generation e limite de idade.

`foreground_snapshot` é transitório e não vai ao SQLite. A auditoria guarda
apenas `foreground_summary` allowlisted/redigido: identidade normalizada do
executável, classe da janela e versão do provider. Título, URL, documento,
caminho e texto da janela nunca são persistidos.

Exemplo:

```text
OBS em foco     → camada OBS
VS Code em foco → camada Desenvolvimento
nenhuma regra   → camada Global
```

Detectar foco não autoriza automaticamente controlar outro programa. Integrações
usam, em ordem de preferência:

1. comando ou API específica;
2. tool MCP autorizada;
3. job;
4. envio de teclas ao programa em primeiro plano, se uma capacidade privilegiada
   futura for explicitamente habilitada.

Injeção de teclado, shell e controle externo exigem política, confirmação e
auditoria próprias. Título de janela pode conter dados sensíveis e não deve ser
persistido ou enviado ao modelo sem necessidade.

Comandos de shell continuam obrigatoriamente passando pelo avaliador único
`internal/commandpolicy` definido na AEP-0060. `CommandExecutionService` não
implementa uma segunda allowlist nem transforma uma aprovação de camada em
autorização de shell.

A exposição na CLI não altera os non-goals da AEP-0045. A CLI pode listar e
descrever todo o catálogo, mas só executa comandos que declarem suporte à origem
`cli`, não dependam de runtime visual e tenham
`decision_requirement = none`. Comando perigoso ou que exija `DecisionDialog`
aparece indisponível com motivo e falha fechado no `CommandExecutionService`;
esta AEP não cria confirmação textual alternativa. Comandos de workspace,
editor ou foco também permanecem indisponíveis e esta AEP não leva essas
surfaces ao terminal.

### D15 — Interface de configuração por camadas

A entrada fica em **Configurações → Comandos e acionadores**. A tela inicial é
uma lista de camadas:

```text
Padrão do aplicativo   — sempre ativa
Chat                   — tela de chat
Desenvolvimento        — VS Code em foco
OBS                    — OBS em foco
Trabalho               — ativação manual
```

O detalhe de uma camada possui duas seções principais:

1. **Quando esta camada fica ativa**;
2. **Comandos desta camada**.

A lista de comandos mostra acionador, comando, condição e conflito. Opções
específicas aparecem somente quando relevantes. Teclado e Stream Deck oferecem
captura do acionador: o usuário pressiona a combinação ou a tecla física.
No Stream Deck, o próprio evento identifica o aparelho e a posição; serial e
IDs são internos, sem seletor ou configuração manual de aparelho desconectado.
A posição é exibida com nome amigável; imagem, título e estados continuam no
escopo específico do Stream Deck. Durante captura, não há despacho de comandos.

Camadas padrão podem ser inspecionadas. Editá-las cria overrides reversíveis,
sem alterar o default versionado.

Configuração avançada de prioridade e predicados permanece em divulgação
progressiva. O fluxo comum é encontrar um comando, capturar uma tecla e,
opcionalmente, definir quando funciona.

Toda operação deve funcionar em lista estruturada com teclado e leitor de telas.
Grade visual do Stream Deck é uma visualização opcional, nunca o único editor.
Reordenação oferece botões mover anterior/próximo e não depende de arrastar.

### D16 — Segurança e limites

- Deep links da AEP-0023 não ganham execução arbitrária de comandos.
- Acionadores não contornam permissões do comando executado.
- Tools MCP seguem catálogo, política e executor comum.
- Comandos perigosos fora de foco não são silenciosamente autorizados.
- Entrar em uma camada ou pasta não executa suas ações.
- Configurações importadas começam sem novos grants.
- Identidade de processo/janela é dado de contexto, não prova de confiança.
- Diálogos bloqueantes e foco obedecem AEP-0091.
- Eventos externos não ativam camadas nesta versão; suporte futuro exige
  contrato de identidade e grants em AEP própria.

### D17 — Modo persistente do editor no lote81

`editor.mode.markdown`, `editor.mode.rich` e `editor.mode.view` são comandos
duráveis, com `Alt+1`, `Alt+2` e `Alt+3` como defaults, respectivamente. O
contrato usa `Begin/Take/Commit`, alvo mutável `workspace/active_tab` com
`ExactVersion` e CAS no backend. O `Commit` só altera `Tab.State.displayMode`;
conteúdo, arquivos e os demais campos da aba permanecem intactos.

A UI prepara e faz flush do estado rico antes da confirmação, mas só aplica o
novo modo depois da decisão confirmada. Se workspace, aba, documento, foco ou
versão mudarem durante a preparação, a operação é descartada sem roubar o
foco do controle que o usuário passou a usar. Replays e ABA são recusados por
enumeração e versão, inclusive quando repetem o mesmo modo; falha de storage
restaura o estado e a versão anteriores.

**Distinção aprovada em 24/09/2026, durante a integração da main:** quando o
editor ativo já estiver em `view`, um novo acionamento autorizado de
`editor.mode.view` pelo teclado (Alt+3 no mapa padrão) apenas devolve o foco à
leitura. Esse efeito local não regrava `displayMode`, não cria execução
persistida e não reutiliza ticket ou handoff. A admissão continua dependendo
do binding efetivo e do contexto atual: supressão, remapeamento, modal e
mudanças de identidade/superfície não podem ser contornados por listener ou
fallback legado. A transição entre modos permanece durável e os replays de
execuções persistidas continuam recusados. A implementação e sua validação
são acompanhadas na seção158 da tasklist; esta aprovação não promove gates.

Composição IME ativa sempre bloqueia o comando. `UnknownIME` é um estado
distinto: somente pode prosseguir para controles `native`/`rich` suportados e
quando o registry válido os reconhece; não é exceção para IME ativo, não libera
modos fora desse registro e não permite execução sem alvo contextual atual.

## Fases

### Fechamento adicional de gaps I05/I09/I11/I12/I13 — 15/09/2026

O tasklist continua com 15 pacotes de infraestrutura e 38/84 critérios
encerrados. Esta rodada amplia cinco pacotes existentes; não encerra os seus
requisitos de montagem no App e não declara BASE-PRONTA.

Diagnóstico passa a considerar o conjunto dinâmico confiável, com stamp privado
e revalidação. O preview estrutural compara a prova ao estado persistido antes
de projetar a mutação, sem impedir disable/delete da própria camada ativa.
Não infere ativação contextual a partir de claims manuais expiradas nem antecipa
efeitos do hook. Simulação autoritativa completa de expiração/rebind continua aberta.

`VerifiedRuntimeFactTx` compartilha a validação de integridade/epoch com
`VerifiedFactTx`, mas distingue renovação de uma lease ainda viva de replay.
A ocorrência antiga continua recusada pelo ingresso. Fonte removida permanece
indisponível; heartbeat e reconstrução/preservação da fonte no runtime real
ainda precisam de montagem.

As seis chaves D11 agora estão em MaintenanceSettings, config.json e na mesma
tela de manutenção, com i18n e documentação. O caminho opcional do coordenador
relê a política por passagem e rejeita overflow; leitura inválida não é tratada
como arquivo ausente. Lotes pendentes impedem limpeza. Prova real de geração
encerrada, adapters e coordenação produtiva continuam pendentes.

As pontes Go/TS receberam shutdown idempotente, invalidação, drenagem dos
cancelamentos admitidos e liberação de recursos; `streamdeck.key` segue D3.
Isso não monta listener físico, transporte Wails ou handlers de produto.

Portabilidade interna ganhou contêiner explícito de deltas builtin por escopo,
com referências de layer/default/regra distintas e conflitos após remapeamento
ao destino. O round-trip preserva revisão e não transporta autoridade. Writer
confirmado e fluxo público continuam bloqueados; detalhes estão na AEP-0047.

ACP/acpregistry foram comparados à base f36c25ddb com o mesmo ambiente atual e
também encerraram com 0xffffffff nela, antes de testes. A causa raiz não foi
determinada; não retirar testes nem declarar qualificação global verde.

O [tasklist de infraestrutura e entrega](0103-tasklist-infraestrutura.md)
consolida a baseline de acompanhamento: pacotes, dependências, critérios de
saída e rastreabilidade dos critérios finais. Ele não altera os contratos desta
AEP nem declara concluídos os incrementos parciais abaixo. A infraestrutura e
a posterior migração/população de comandos possuem marcos separados.

### Rodada I05–I10 — consolidação em 15/09/2026 (parcial)

Esta rodada amplia a infraestrutura, não migra a população de comandos nem
habilita atalhos no produto. `commandconfig.ProjectComplete` valida catálogo,
argumentos, condições e portas de acionador; `CompleteMutationService` unifica
autenticação, preview privado, confirmação, revalidação, CAS e auditoria para
CRUD e upgrade/rebase de defaults. A auditoria `command_config_mutations`
schema v2 acrescenta scope/workspace e documentos antes/depois; schema v1 e
seus receipts existentes são preservados no upgrade v22. Os documentos
persistidos não podem conter valores em paths sensíveis: referências portáveis
de credenciais ainda dependem de I11 e são recusadas neste recorte.

`commandactivation` acrescenta regras, claims, pin/toggle/back, expiração,
restore autenticado e recomposição contextual. A tabela técnica
`command_layer_activation_generations` usa PK UUIDv7 e uma geração monotônica
por usuário/escopo global ou workspace; atualização e claims pertencem ao
mesmo TX, sem publicar incremento em memória antes de commit. Não substitui
os epochs de autenticação nem a geração da configuração.

`NewActivationMutationHook` compõe disable/delete de layers, revogação de
grants e reconciliação de claims no TX da receipt/configuração. Restore de
layer/conjunto continua explicitamente recusado nesse fluxo: o diff ainda
precisa incluir regras/grants/claims, não apenas bindings/layers. O restore
isolado do repository não representa restauração agregada pronta.

`commandautomation` mantém grants exclusivos de ativação, separados dos
grants de delegação de jobs, com fingerprints JCS/HMAC e consumo atômico de
receipt. CRUD completo de regras e consumo de cada evento ainda precisam
compor essas provas; existência de um grant ou de uma linha habilitada não
autoriza fallback. A migração v23 registra regras, claims, grants, gerações,
outbox e epochs de replay, sem habilitar os consumidores.

O runtime de jobs passa a gravar timeline incremental; v24 prepara
`queued_at` antes do AutoMigrate de bancos populados. A outbox independente
preserva ID de ocorrência, fingerprint e deadline imutável. O consumidor de
ativação permanece desabilitado: CAS por regra, lease/heartbeat de claim,
reconciliação e manutenção I12 ainda não estão ligados. Lease de entrega da
outbox não é lease da claim de job. O epoch inicial do produtor também deve
ser criado pelo bootstrap da política antes de habilitar produção/consumo.

O MESMO executor aceita portas internas de identidade local/external/job/system
e usa o MESMO `commandsecurity.EpochService`, sem relógio de segurança paralelo.
`commandidentity.CoreEpochs` adapta essa instância; política reconsulta a
origem, em vez de confiar em campos de uma projeção fornecida. A migração v25
prepara o vínculo administrativo externo/FK, mas a adoção pelo middleware e
readiness externo permanecem pendentes; não há JIT ou adapter físico externo.
`commandtoolbridge` delega ao executor comum de tools com origem
`command_invocation`, contrato tipado e redação propagada. Montagem dos
runtimes reais, ponte UI de I03, integração de grants AEP-0101 e qualificação
global continuam itens abertos no tasklist.

O executor valida `command_chain_history` separado do histórico de jobs,
limite versionado 16 e repetição de command ID antes da reserva; referências
de layers vêm da resolução confiável, nunca são inferidas de binding IDs.
Testes e limites de validação desta rodada são registrados no tasklist após
a consolidação, sem converter contagem de arquivos/testes em porcentagem do
AEP nem declarar os seis pacotes inteiros concluídos.

### Rodada I11–I15 e pendências anteriores — 15/09/2026 (parcial)

Seis frentes Luna ampliaram portabilidade, manutenção, pontes, lifecycle,
qualificação e mutações agregadas. O principal revisou as interfaces e integrou
o consumidor transacional `commandjobactivation`. Isso não habilita o novo
executor no produto nem encerra BASE-PRONTA.

O consumidor relê fato/outbox, epoch e deadline originais, identifica job por
owner + DatabaseID + slug, e revalida regra, grant, fingerprints e autoridade
do runtime sob o gate compartilhado. Claim, sequência, ledger de evento e ack
ficam no mesmo TX. Reentrega, conflito de fingerprint e ciclo terminal são
distintos; remover o detalhe de um ciclo por retenção não permite reabri-lo.
Lease própria e renovação exigem runtime/sessão/gerações correspondentes;
reconciliação em lotes torna inativa a claim cuja fonte desapareceu. Faltam
montar o worker e heartbeat reais, o bootstrap do epoch e a política completa
de manutenção. `commandjobevents.Adapter` continua desabilitado.

Migração central diferida v26 acrescenta leases sem cascata, unicidade de
ocorrências/regras e suporte à manutenção. Reconhece o schema anterior completo
e amplia verbos da auditoria sem perder documentos. Testes cobrem upgrade v25,
segundo boot, preservação de auditoria e rejeição de drift. Jobs passam a
persistir somente proveniência estrutural autorizada, incluindo validação de
`command_chain_history` separado; payload arbitrário de trigger não é copiado.

`commandconfig` amplia o diff privado com regras/grants/claims, restore agregado,
CRUD de regras e diagnóstico autenticado/versionado. Revogações pertencem ao
mesmo commit da configuração e da receipt; criar regra event-driven não
significa conceder autoridade. Importação e concessão event-driven pelo serviço
comum ainda requerem composição; os serviços não foram publicados no App.

`commandportability` define DTO e planejamento seguro de `resources.commandLayers`
com catálogo/sensibilidade e referências exatas de credenciais. Não existe
writer alternativo: import genérico recusa esse recurso até o commit confirmado
comum existir. Deltas sobre builtin ainda não têm round-trip completo.

`commandmaintenance` fixa a sequência e exige portas/política explícitas antes
de efeitos; a cadência legada permanece sem coordenador montado. Retenção
preserva ledgers até seus deadlines e runs não terminais com claim/lease viva.
A prova efetiva de encerramento de gerações antigas e os seis settings/UI
continuam pendentes; marcador de banco, sozinho, não comprova drenagem.

`commandbridge` e `commandruntime` implementam contratos tipados e máquinas de
estado testadas: ack/resultado/cancelamento, geração, pressão e cancelamento da
inicialização. Não substituem transporte Wails, stack de diálogos, listeners
SO/HID nem validação física. Testes de App/config usam diretórios temporários.
Resultados globais e limites de qualificação estão no tasklist; testes com
portas controladas não provam montagem real ou latência integrada.

### Evidência I01 — armazenamento e chaves operacionais (baseline)

`internal/commandbootstrap` compõe as migrações de configuração, receipts e
ledger. A migração v21 `command_storage_initial` pertence ao registro central
da AEP-0076: fica adiada na abertura genérica do banco e é concluída pelo host
com dados e carimbo na mesma transação. O App chama a preparação após carregar
o cofre, tanto na inicialização quanto em sua reconfiguração. Não há executor,
mapa ou adapter habilitado por esse passo; prontidão de armazenamento é distinta
de prontidão de execução. Falha é retida e não impede autenticação legada.

O schema conhecido é validado antes de alteração e após migração, inclusive
quando já carimbado. Schema desconhecido/índice incompatível falha fechado e
preserva os dados. A comparação tolera somente reordenação de definições de
colunas/constraints de tabela; expressões e ordem de colunas dos índices são
preservadas. SQLite privado em memória fornece a referência a partir das mesmas
definições dos repositories, evitando manter uma segunda cópia do DDL.

`credentials.Manager.EnsureInstanceSecret` lê o armazenamento exato de instância
e usa insert-if-absent, nunca upsert destrutivo nem fallback user-scoped. Exige
persistência e cofre previamente validado. O vencedor de uma corrida é relido e
decifrado; entrada ilegível/vazia/tipo incorreto não é substituída.

`PrepareKeys` cria a chave de fingerprint apenas em armazenamento sem histórico
que dependa dela, valida base64 e vincula cada versão a digest não secreto em
`command_key_versions`. Ausência ou troca de chave já registrada falha fechado.
`RotateKeys` é uma porta interna de manutenção explícita com versão esperada e
CAS: conserva versões anteriores e não altera JWT, pepper ou DEK. Todas as
versões são conservadas indefinidamente nesta etapa, portanto nunca menos que
o horizonte dos ledgers; eventual coleta segura pertence a I12. Queda depois de
criar a próxima chave, mas antes de ativar a versão, permite retentar sem trocar
a chave. A futura composição do executor deve suspender admissões durante a
rotação e publicar a versão retornada; não existe endpoint público de rotação.

Testes usam bancos/DEKs sintéticos e cobrem primeira abertura, reabertura,
adoção do schema experimental, drift, rollback/carimbo, concorrência, cofre
indisponível, recuperação de prontidão e rotação. Validação integral de releases,
race, ferramentas de lint/review e integração final de execução continuam em
I15/I14; o AEP permanece In Progress.

### Evidência incremental — protótipo da Fase 0

O pacote `internal/commandbindings` inicia o experimento de seleção pura de
candidatos. Ainda não está conectado ao aplicativo e não executa handlers.
Não representa a conclusão da Fase 0 nem do registro/resolvedor da Fase 1.

- Implementado: índice por acionador, filtros de habilitação/ativação,
  precedência de escopos, conjunção de igualdades tipadas, conflito sem desempate
  por ID, agrupamento de destinos equivalentes e barreira de diálogo.
- A especificidade parcial é avaliada eliminando condições estritamente
  dominadas dentro do melhor escopo; prioridades comparam somente os máximos
  restantes. Não se usa ordenação com comparador parcial. O teste de todas as
  permutações cobre três candidatos com condições comparáveis e incomparáveis.
- No protótipo, condição por `surface.id` exige também `surface.type`,
  fornecido pelo normalizador confiável, para representar identidade antes de
  tipo por inclusão de cláusulas. Identidade sem tipo é recusada. Candidatos de
  diálogo possuem `DialogID` e somente os do topmost participam, inclusive na
  proveniência de equivalentes.
- Testes: `go test ./internal/commandbindings`; benchmark do índice:
  `go test ./internal/commandbindings -run '^$' -bench . -benchmem`.
  O benchmark distribui bindings por acionadores distintos; não mede a ponte
  Wails, persistência nem o pior caso de muitos candidatos na mesma tecla.
- Validação local inicial: testes (98,9% de cobertura) e `go vet` do pacote
  passaram, assim como build/vet do backend completo. A suíte geral não ficou
  verde: `config`/`wailsapi` falharam por acesso negado à pasta real `.assistente`
  e `acp`/`acpregistry` encerraram com `exit status 0xffffffff`. Esses pacotes
  não foram alterados neste incremento. `-race` ainda não foi validado: o ambiente Windows de teste
  está sem compilador C configurado e o Go exige cgo para esse detector.
- Limites: candidatos chegam já normalizados, com identidade de argumentos e
  alvo fornecida pelo chamador confiável. O protótipo não calcula fingerprint,
  não autentica e não valida o schema de argumentos do catálogo.
- Pendentes: completar catálogo e materialização de deltas/overrides/tombstones
  e `needs_review` (incrementos parciais descritos na Fase 1), claims,
  persistência, executor/ledger, providers autoritativos, reserva dos atalhos
  invariantes de diálogo, adapters físicos e migração de handlers.
  Um resultado `selected` é apenas seleção, nunca autorização de execução.

Pontos já identificados para o inventário, ainda não migrados:

O [inventário inicial de atalhos](0103-inventario-atalhos.md) detalha símbolos,
combinações, guards e testes existentes. É documento de apoio, não conclusão
da Fase 0; os IDs sugeridos ainda não estão registrados no produto.

- `frontend/src/hooks/useWorkspaceKeyboardShortcuts.ts`: abas e sequências
  `Ctrl+N` seguidas de letra; preservar cancelamento, timeout e foco.
- `frontend/src/hooks/useActivePanelShortcut.ts`: `Ctrl+N` do painel ativo.
- `frontend/src/components/chat/ChatToolbar.tsx`: seletores e ações do chat.
- `frontend/src/pages/useEditorMenus.tsx`: ações e modos do editor.
- `controllers/hotkeys_controller.go` e
  `frontend/src/hooks/useInteractionProfile.ts`: hotkeys de voz por perfil.
- `internal/jobs/manager.go`: hotkeys que solicitam triggers de jobs.

O próximo incremento deve completar o inventário e o catálogo de comandos,
definir os contratos de sequências/ponte de UI e validar os protótipos físicos.
Os critérios de aceitação finais permanecem abertos: testes do seletor isolado
não demonstram as garantias de execução, persistência e segurança do sistema.

### Fase 0 — Inventário e protótipos

- Inventariar atalhos locais, hotkeys de perfis, comandos de menu e ações
  executáveis existentes.
- Prototipar o registro de comandos e a resolução de camadas sem migrar handlers.
- Validar a biblioteca Go do Stream Deck, exclusividade, reconexão e modelos.
- Prototipar observação de janela em primeiro plano no Windows.
- Medir latência e estabilidade com muitas camadas e bindings.

### Fase 1 — Registro, defaults e resolvedor

Incrementos isolados adicionais (sem conexão ao dispatcher):

- `internal/commandcontext`: comparação de versões exatas e validade temporal
  dos fatos capturados. A autenticidade dos snapshots é pré-condição do host;
  `VersionService` registra providers injetados, captura fatos exigidos e
  calcula uma identidade contextual determinística de provider/fato/versão.
  Não implementa os providers reais do aplicativo, fingerprint de request
  RFC 8785, autorização ou revalidação atômica sob `DispatchGate`.
- `Configuration.WithoutDeltas`: restauração seletiva em um novo snapshot de
  memória, preservando as personalizações não removidas e seus ajustes de
  revisão. Não representa restore transacional no SQLite ou por camada.
- `internal/commandinput`: máquina de pressão/liberação com descarte de repeat
  e invalidação monotônica por geração. Ainda não registra hotkeys, não observa
  eventos DOM/SO/Stream Deck, não gera IDs de ocorrência e não arbitra ownership
  entre teclado local e global. O host deverá integrar esses contratos.
- `configuration_bench_test.go`: mede candidatos no mesmo acionador, além da
  distribuição entre buckets. A amostra local com 1000 candidatos na mesma
  tecla ficou em aproximadamente 4 ms e 2 MB/op; não constitui SLA e evidencia
  custo de materialização/seleção a otimizar antes de integrar o produto.

Validação conjunta desta rodada: testes de `commandbindings`, `commandcatalog`,
`commandcontext` e `commandinput` passaram, com coberturas de 92,9%, 99,2%, 96,0%
e 90,2%, respectivamente. Build e vet gerais também passaram. Os percentuais
medem instruções instrumentadas dos pacotes, não progresso do AEP.
Continuam pendentes a suíte geral verde, o detector de corrida com compilador C,
lint v2 e revisão Bugbot antes de push.

#### Integração inicial de diagnóstico (sem execução)

`internal/commandpreflight.Service.Inspect` conecta configuração de bindings,
catálogo e `VersionService` num fluxo interno restrito à origem teclado local.
O host injetado deve autenticar cada consulta e fornecer snapshots estáveis do
mesmo escopo de usuário/sessão dos providers. A entrada contém somente o
acionador normalizado; não escolhe usuário, origem, fatos ou permissões.
O serviço recusa contratos fora do subconjunto de leitura sem interação e sem
mutabilidade, revalida fatos, reconsulta o host e verifica novamente o TTL ao
final. Mudança de sessão, configuração ou contexto recusa o diagnóstico sem
resultado parcial.

`read_checks_passed` não é autorização nem token de despacho. `would_suppress`
é uma simulação que não consome a tecla nem cria marcador terminal. Não existe
rota de handler, invocação ou endpoint Wails nesta integração. Argumentos,
disponibilidade completa, autorização, receipts, ledger/auditoria duráveis e
gerações de segurança reais precisam existir antes de habilitar execução.
O teste de integração usa comando e providers de teste, não comando registrado
no produto. Os atalhos atuais permanecem intactos.

`internal/commandsecurity.DispatchGate` fornece o primitivo de admissão
compartilhada/mutação exclusiva, sem conectá-lo a logout ou handlers ainda.
O handoff não bloqueante deverá ocorrer dentro do gate; aguardar trabalho
longo ocorre fora. A espera pelo RWMutex não é cancelável imediatamente: o
cancelamento é observado antes e depois de adquirir o lock.

Os testes de integração cobrem leitura, recusa de escrita, contexto alterado,
logout/troca de usuário ou sessão, geração modificada, erro final sem resultado
parcial, snapshots equivalentes reconstruídos e TTL vencido durante a consulta
do host. Testes focados dos pacotes envolvidos, build e vet gerais passaram.
Essa evidência não substitui o ledger durável, validação NVDA, suíte geral verde
ou revisão Bugbot; nenhum critério de execução ponta a ponta está concluído.

#### Persistência inicial isolada (sem execução)

`internal/commandledger` inicia as tabelas `command_idempotency_keys` e
`command_invocations`, com migração explícita, sem bootstrap no aplicativo.
A API interna aceita somente solicitações diretas `local_session` de leitura
sem argumentos/contexto, nas origens `palette`, `ui.action` e `cli`. O chamador
confiável ainda deve autenticar, validar o contrato e produzir os HMACs; o
repositório compara fingerprints, mas não os autentica nem autoriza execução.

Reserva e auditoria são transacionais. A reentrega verifica usuário, sessão,
origem e fingerprint, sem renovar a expiração. Transições CAS atualizam ambos
os registros ou revertem a transação; terminais não são reiniciados pela API.
Os testes usam SQLite em arquivos temporários e incluem reabertura do banco,
rollback por falha da auditoria e isolamento de escopo.

`RecoverClosedGeneration` acrescenta recuperação interna explícita de uma
sessão local e geração de segurança encerrada: pares `evaluating`, `queued`
ou `running` passam atomicamente para `outcome_unknown`, sem novo efeito.
O host precisa comprovar o encerramento e impedir admissão da geração antes
da chamada; o repositório não deduz isso de relógio ou expiração. Gerações e
sessões fora do escopo e estados terminais são preservados. Testes cobrem
idempotência, replay, isolamento, rollback e par com fingerprint divergente.

`Reconcile` fornece o CAS interno `outcome_unknown → succeeded|failed` após
consulta por `OutcomeVerifier` confiável, fora da transação SQLite. A consulta
recebe uma cópia do registro escopado; falha, cancelamento e outcome inconclusivo
preservam a incerteza. O commit confere novamente identidade, fingerprint e
estado, atualiza ledger e auditoria juntos e não sobrescreve um terminal que
venceu a corrida. O subconjunto atual não produz retorno além de `{}`.
O host ainda precisa selecionar o verificador pelo comando, reaplicar
autenticação/autorização e consultar uma fonte conclusiva sem executar efeitos.
Nenhum verificador de produto ou endpoint está registrado; os verificadores
dos testes não constituem prova de reconciliação ponta a ponta.

O harness exclusivo de teste `pipeline_integration_test.go` combina catálogo,
ledger SQLite e DispatchGate para uma leitura direta de fixture: reserva,
validação estática, CAS de fila/running, handoff, conclusão e replay sem nova
chamada. A espera do resultado libera o gate para mutações. Autenticação,
autorização, identidade e handler são substitutos explícitos de teste, não
serviços de produto. O HMAC usa `SignLocalRead` com chave exclusiva de fixture.
O harness não é executor reutilizável e não
cobre o protocolo completo de cancelamento/panic/receipts. Essa evidência não
habilita comandos reais nem conclui a execução ponta a ponta exigida pelo AEP.

`SignLocalRead` implementa HMAC-SHA256 para a projeção fechada de leitura direta
local sem argumentos/contexto. Recebe somente envelope interno já derivado e
recusa fingerprints preenchidos no ingresso. A projeção JCS contém `version:1`,
IDs de invocação/correlação, comando, usuário/ator, contexto autenticado/sessão,
geração de segurança, origem, versões de catálogo/configuração/camadas e
constantes `effect:read`, `decision:none`, `context_policy:none`, `arguments:{}`
e `binding_ids:[]`. Omite auth_generation e timestamps; opcionais fora do
subconjunto permanecem ausentes. O HMAC de argumentos cobre `{}` separadamente.
As saídas são hexadecimal minúsculo. O serializador é restrito a essa projeção
(chaves ASCII fixas, strings Unicode válidas e constantes), não JSON genérico;
segue escapes e preservação de Unicode da RFC8785, sem normalização.

O provider injetado recebe `command-request-hmac:<versão>` e deve devolver
chave de pelo menos 32 bytes; não existe geração/fallback automático. Testes
comparam um documento canônico literal e exercitam alterações semânticas,
reentrega SQLite com versão retida e recusa quando a chave antiga desaparece.
O host ainda precisa derivar/autenticar a identidade, escolher a versão por
ledger escopado no retry, manter chaves no secret manager e validar a política
antes de assinar. `NewCredentialKeyProvider` conecta a leitura ao Manager já
inicializado: o nome lógico `command-request-hmac:vN` mapeia para o segredo
de instância `internal-auth:command-request-hmac:vN`, tipo `secret`, codificado
em base64url canônico sem padding. A consulta é exata e estritamente sem usuário,
sem o fallback legado de `GetInstanceSecret`. Chave ausente, curta, malformada ou
credencial user-scoped falha fechado. O adapter não cria nem substitui chaves,
não inicializa o cofre e não registra bootstrap. Testes usam Manager real com
criptografia em memória, sem keychain, e reserva SQLite em arquivo temporário.
Provisionamento, rotação, retenção operacionais e integração no host ainda faltam.

`SessionService.AuthenticateLocalAccess` adiciona consulta interna de identidade
para comandos locais. Reutiliza a verificação JWT existente e confere em uma
leitura SQLite a relação sessão/usuário, conta ativa e ausência de revogação;
confere também expiração da sessão e do access token ao final da consulta.
Devolve somente user_id/session_id UUIDv7, sem role, token ou gerações. Não
modifica `VerifyAccessToken`, middleware externo nem o login vigente. O teste
`TestAuthenticatedRequestUsesSessionIdentityAndRejectsLogout` usa sessão/JWT,
Manager e ledger reais em armazenamento de teste: deriva ownership no backend
e recusa nova reserva após logout mesmo com assinatura JWT ainda válida.
Esta leitura não é autorização nem elimina corridas após retornar: todas as
origens de revogação ainda precisam participar da coordenação do host.

`commandsecurity.EpochService` fornece gerações locais em memória sobre um
DispatchGate injetado: identidade aleatória de startup, contador sem reuso,
auth_generation por sessão e security_generation global. Capture recebe IDs
já autenticados; Admit compara snapshot, chama revalidação autoritativa e
handoff sob o mesmo gate compartilhado. Invalidação de sessão é local;
invalidação de principal altera sessão e segurança atomicamente; invalidação
global torna todos os snapshots anteriores obsoletos. Overflow falha fechado.
Testes verificam o lock durante revalidação/handoff e liberação após retorno.
O teste integrado de identidade usa essas gerações e consulta SessionService
sob Admit: logout recusa handoff mesmo antes da invalidação observada do epoch.
Capture
não autentica, não representa estado locked e não deve readquirir um gate já
detido. Lifecycle/limpeza de sessões fora das transições e suporte
system/external/job continuam pendentes; nenhuma execução real é
habilitada por este incremento.

`MutateSession`, `MutatePrincipal` e `MutateSecurity` coordenam uma mutação
autoritativa curta com a invalidação sob o mesmo gate exclusivo. Invalidam antes
do callback e mantêm a invalidação em erro/panic, pois pode ter ocorrido efeito
parcial; não prometem rollback do banco. IDs/escopo e callback são fornecidos pelo
host confiável, nunca por payload. Callbacks não podem readquirir o gate nem
aguardar interação, rede ou conclusão de handlers. O teste de sessão/ledger
agora também executa `SessionService.Logout` dentro de MutatePrincipal; snapshots
anteriores são recusados por staleness e o token revogado não reserva novamente.
Teste concorrente verifica exclusão durante a mutação e recusa do snapshot
antigo na admissão.

`BeginTransition` acrescenta uma barreira para operações longas: invalida todos
os snapshots e impede Capture/Admit enquanto houver transições abertas, sem
manter o gate durante I/O ou parada de runtimes. Encerramentos são idempotentes,
aninháveis e independentes do cancelamento da operação. Exaustão de gerações
desabilita novas admissões permanentemente naquela instância.
O App mantém uma instância inicializada sob demanda por sync.Once e chama a
barreira em Login, RefreshAuth, Logout, rollbackLoginState, SetupVault e
UnlockVault. A política inicial é conservadora: inclusive refresh e tentativas
de autenticação que falham invalidam snapshots de comandos. O retorno legado
dessas operações é preservado; falha de inicialização deixa comandos indisponíveis,
sem impedir o logout existente. A ordem é authSessionMu (quando aplicável),
gate e authMu; callbacks de admissão não podem adquirir authSessionMu.
Testes focados do App verificam os hooks em falhas precoces e Logout real com
sessão SQLite e keychain substituído por callbacks de teste. Permanecem pendentes
outras origens de revogação, bloqueio do SO, executor e refinamento do refresh.

`CaptureAuthenticated` fecha a janela entre autenticação local e captura de
gerações: o callback confiável consulta a identidade autoritativa sob o mesmo
gate exclusivo que publica o snapshot. Uma transição coordenada não pode entrar
entre essas etapas. Callback inválido, identidade inválida, erro ou cancelamento
não publicam snapshot nem sessão; transição aberta recusa antes da consulta.
O callback deve fazer somente consulta local curta e não pode readquirir o gate,
inicializar cofre, aguardar UI/rede ou executar handlers. A API anterior Capture
permanece primitiva de baixo nível, não autenticação para requests de produto.
O teste integrado de sessão/ledger deriva ownership e ambas as gerações da mesma
captura em cada reserva e compara a identidade exata ao revalidar a admissão.
Testes unitários verificam exclusão, cancelamento e liberação do gate em panic.
Isso não concede autorização nem substitui os gates finais de fila/despacho;
o executor de produto e a coordenação de todas as fontes de revogação continuam
pendentes.

Esta projeção não cobre comandos com argumentos, providers,
receipts, delegação ou eventos e não habilita o executor de produto.

Permanecem pendentes integração ao startup e ativação no produto,
comprovação de encerramento de geração, verificadores reais de reconciliação,
ampliação da projeção HMAC/RFC8785 e integração ao secret manager,
resultados, política de retenção, eventos,
supressão, identidades externas e constraints condicionais completas de D11.
As colunas futuras não tornam esses fluxos suportados. Nenhuma fase ou critério
de execução ponta a ponta é concluído por este incremento.

#### Executor interno de leituras diretas (ainda não exposto no produto)

`internal/commandexecution.Service` passa a integrar o caminho que antes existia
somente em harness: sessão local real, captura autenticada de gerações/catálogo,
HMAC, reserva durável, política, CAS de fila/running, Start e conclusão.
O ingresso recebe somente IDs de invocação/correlação e command_id; identidade,
origem (fixa no adapter), relógio, chave e versões são derivados pelo backend.
O construtor exige dependências explícitas e copia as rotas de handlers; recusa
contratos não read/none, alvos mutáveis, argumentos/providers (fora da API),
origens não suportadas e ausência de política. Nenhum handler é inferido por nome.

Autenticação da mesma sessão, lock, versões e autorização são reconsultados nos
dois gates: evaluating→queued e queued→running. Start ocorre sincronamente sob
o gate após o CAS, mas deve apenas devolver um handle não bloqueante. Espera e
Cancel ocorrem fora do gate. Falha/panic antes de Start é failed; erro/panic ao
entrar em Start, handle inválido, canal perdido ou cancelamento sem confirmação
produzem outcome_unknown. Outcomes explícitos succeeded/failed/cancelled são
persistidos com CAS; não há retry do handler. A finalização tem contexto e prazo
próprios, para não perder o registro ao cancelar o chamador. Falha de persistência
é devolvida e nunca tratada como autorização para nova execução.
O ledger grava policy_decision allowed/denied junto ao CAS correspondente e
admite queued→failed para falha conclusiva anterior ao handoff.

`NewLocalReadAuthorizer` fornece a política local inicial: allowlist de roles por
command_id do host e consulta da conta/sessão no banco a cada gate, sem confiar
na role do JWT. Comando/role não listado falha fechado, inclusive para admin.
Atualizações de role/revogação devem participar do mesmo gate; essa política
não torna automaticamente coordenados endpoints legados de mutação.
`App.newCommandReadExecutor` é fábrica interna sem rota Wails: injeta o serviço
de sessão, EpochService e provider de chave do Manager já carregado, exige a
identidade ativa exata e recusa troca de SessionService. Não inicializa, gera
ou grava segredos. Deve ser construído em bootstrap serializado.

Testes do serviço usam sessão/JWT, SQLite e HMAC reais com dados temporários;
testes do App exercitam a fábrica e logout enquanto o handler aguarda resultado,
sem keychain real. Políticas de teste são explícitas, não defaults de produção.
Evidências: `TestServiceConcurrentReplaysObserveRunningWithoutNewHandoff`
verifica seis reentregas concorrentes com um único Start;
`TestServiceFailedQueueCommitNeverStartsOrOverwritesLedger` cobre falha de CAS;
`TestServiceRealPolicyRejectsChangedRoleDespiteValidJWT` cobre a role vigente;
`TestServiceReplayUsesStoredKeyVersionWithoutFallbackForNewRequest` cobre a
versão de chave vinculada ao ledger; e
`TestCommandExecutionAppUsesInstanceKeyAndRejectsLogout` cobre a fábrica do App.

Limites deste recorte: somente palette/ui.action/cli, read/none sem workspace;
fila lógica com retirada imediata (sem scheduler compartilhado); sem payload de
resultado além do status/resumo vazio atual. Execute nunca retoma uma reserva
existente. GetInvocation reautoriza sem criar reserva nem chamar handler, mas
ainda exige request original e fingerprint reproduzível nas gerações atuais;
consulta após mudança de gerações pode falhar como conflito, não reexecutar.
Ainda faltam consulta histórica independente de versões, recusas auditadas para
todo envelope não suportado, log de segurança pré-autenticação, fontes completas
de eventos autoritativos de lock/versões,
provisionamento de chaves e registro de comandos/rotas de produto. Não se habilita
auth.mode=external, atalhos, UI, receipts ou efeitos de escrita neste bloco.

#### Estado do host e cancelamento por invalidação

`EpochService.AdmitExecution` associa um contexto de execução ao snapshot dentro
do mesmo gate compartilhado que revalida e entra em Start. A inscrição não tem
janela após o handoff. Invalidação de sessão cancela apenas os contextos daquela
sessão; mudança de segurança/principal e BeginTransition cancelam todos. A
invalidação fecha apenas contextos internos, nunca chama Cancel do handler sob
o gate. O executor observa esse cancelamento e chama Cancel fora do gate,
persistindo outcome_unknown quando não houve confirmação conclusiva. Release
idempotente remove a inscrição ao terminar; erro/panic de handoff também limpa
a inscrição. Resultado tardio não sobrescreve o terminal. Isso não desfaz um
efeito que já tenha começado.

`MutateUserConfiguration` publica sob gate exclusivo e cancela execuções apenas
do usuário afetado, sem alterar security_generation de outras contas. O
callback deve avançar as gerações de configuração aplicáveis. Erro ou tentativa
de publicação sem mudança efetiva pode cancelar conservadoramente execuções do
próprio usuário; não reativa contexto já cancelado.

**Exceção aprovada em 24/09/2026 — atualização reativa sem impacto:** uma
publicação derivada de claims de job pode preservar uma execução já admitida
somente quando o host comprovar, por execução, que comando, alvo, binding,
autorização e contexto relevantes permanecem equivalentes. Não basta ser uma
adição de camada: uma adição pode alterar precedência ou autorização. A prova
deve usar estado autoritativo imutável e validação atômica com a publicação;
falha, conflito, ausência de prova ou dependência alterada mantêm cancelamento.
Preparações e novas invocações não herdam essa exceção: usam as versões atuais
e revalidam normalmente. Revogação, remoção relevante, logout, lock e troca de
principal conservam suas barreiras; mutação real da configuração persistida
não é tratada como mero refresh de claim. Não há callbacks de efeito, rede ou
espera de UI sob o gate nem ressurreição de contexto já cancelado.

`commandexecution.HostState` substitui versões inventadas pelo chamador na
fábrica do App: mantém Configuration imutável, lista detached de camadas ativas e
gerações globais por usuário. Gerações usam startup UUIDv7 e contador monotônico;
overflow desabilita novas leituras do estado. ForgetUserConfiguration remove
somente o snapshot em memória e republicação nunca reutiliza gerações.
Snapshot usa mutex curto próprio, sem readquirir o DispatchGate. As mutações
passam pelo EpochService da mesma instância; nenhuma conta recebe gerações de
outra conta.

O estado começa com cofre fechado e sessão do SO desconhecida. Só sinaliza
Unlocked com cofre aberto E sessão do SO conhecida e desbloqueada; cofre aberto
sozinho não libera comandos. Setters recebem fatos de adapters confiáveis, não
de payload/UI. A fábrica exige HostState explícito com o mesmo EpochService e
não aceita que Config.Snapshot substitua esse estado. A instalação no App é
serializada e rejeita substituir a instância de HostState já instalada.

Login descarta o mapa anterior (inclusive tentativa que falha); logout e
rollback descartam mapa e marcam o cofre fechado. SetupVault/UnlockVault bem
sucedidos e SetupMasterPassword atualizam a observação de cofre; o caminho
legado de SetupMasterPassword também passa pela barreira de transição. Os
retornos legados são preservados e nenhum desses hooks consulta keychain sob
o gate. Testes de App verificam falhas precoces sem I/O real, remoção do mapa,
republicação sem reuso de gerações e logout com cancelamento do executor.

Reconstrução autenticada do mapa: `RebuildUserConfiguration` captura a sessão
local e suas gerações, constrói o snapshot fora do gate e reautentica a mesma
identidade antes de publicar sob gate exclusivo. Mudanças de segurança,
autenticação ou configuração durante o carregamento descartam o resultado.
A revisão de carregamento é conservadora e global nesta instância: mudanças de
outra conta também podem recusar um carregamento concorrente, sem invalidar
execuções já admitidas dessa outra conta. Não há retry implícito.

Cada observação do SO remove os mapas anteriores, inclusive unlock e estado
desconhecido. Publicação simples de configuração não torna bindings utilizáveis:
somente a reconstrução autenticada associa o mapa à sessão exata. O hook interno
do App autentica JWT e sessão persistida antes/depois do builder e confere a
identidade atual e as dependências da instância. Não guarda token, não consulta
keychain e não permite que um builder atrasado ressuscite um mapa removido.

Exaustão do contador de segurança também desabilita o EpochService e cancela
contextos admitidos, sem executar a mutação autoritativa nem reutilizar geração.
Assim, falha ao invalidar na parada do observador não preserva admissões antigas.

Adapter nativo `internal/ossession`: usa janela message-only e notificações
WTS da sessão do processo, consulta WTSInfoEx após registrar o observador e só
aceita desbloqueio com sessão ativa e flags conhecidas. Lock invalida mesmo
que uma consulta posterior já veja unlock. Desconexão, erro e encerramento
deixam o estado desconhecido/fechado. Plataformas não suportadas recusam a
observação; não há inferência por cofre ou JWT. Referências oficiais:
[WTSRegisterSessionNotification](https://learn.microsoft.com/en-us/windows/win32/api/wtsapi32/nf-wtsapi32-wtsregistersessionnotification),
[WM_WTSSESSION_CHANGE](https://learn.microsoft.com/en-us/windows/win32/termserv/wm-wtssession-change)
e [WTSINFOEX_LEVEL1_W](https://learn.microsoft.com/en-us/windows/win32/api/wtsapi32/ns-wtsapi32-wtsinfoex_level1_w).

A fábrica interna do executor instala um único monitor quando o ciclo de vida
do App já está iniciado e acompanha seu encerramento via bgWG/contexto de
shutdown. Erro do monitor mantém o host fechado, sem reinício silencioso.
O fim do startup também verifica um host previamente instalado. A fábrica
ainda não é chamada pelo startup de produto. Os testes do App
injetam um observador falso; validação manual com lock/unlock real do Windows
permanece pendente, sem bloquear a estação nem consultar segredos em testes.
O pump verifica cancelamento em esperas de até 100 ms; chamadas Win32 síncronas
não recebem cancelamento forçado. Um timeout de limpeza retorna erro e fecha
o host, mas a liberação nativa ainda depende de a chamada do SO retornar.
Falha terminal da fonte descarta eventos enfileirados obsoletos, sem reproduzir
unlock depois de perder a observação.

Evidências deste bloco: testes de reconstrução, publicação exclusiva e
invalidação por overflow; testes de decoder/eventos WTS com fontes falsas;
testes de App para sessão divergente, JWT inválido, remoção durante carregamento,
encerramento do monitor e cancelamento do executor. Build e vet do repositório
e testes focados dos pacotes afetados são a validação local; o teste nativo
interativo e a suíte completa do App permanecem fora desta evidência.

Leitura persistida inicial: `internal/commandconfig` fornece migração explícita
de `command_layers`, `command_bindings` e `command_config_generations`, sem
registrá-la no banco de produto. O carregador lê camadas, bindings e gerações
na mesma transação SQLite. Escopo global carrega só globais; workspace carrega
globais mais exatamente o workspace autenticado. Referências user precisam de
camada do mesmo owner/escopo; builtin exige delta e validação posterior no
catálogo. Ausência de geração não cria defaults nem inventa uma versão.

Snapshot conserva documentos JSON opacos, disabled e needs_review; não gera
Candidate nem ativa camadas. Documentos ainda exigem validação de versão/schema,
catálogo, defaults e referências a segredos pelo projetor confiável. Não existe
projetor permissivo de produto. Um stamp privado, independente dos campos
mutáveis entregues ao projetor, permite reconsultar as gerações antes de publicar.
Troca/remoção da geração e snapshot de outro Store falham fechado.

`rebuildPersistedCommandConfiguration` conecta esse carregador à reconstrução
autenticada do App, obrigando um projetor fornecido pelo bootstrap. O owner vem
do JWT/sessão local revalidada; a geração persistida é conferida novamente sob
DispatchGate. Essa borda aceita somente configuração global enquanto HostState
não isola workspaces. Nenhuma claim manual/evento é restaurada por esse caminho.
Escritores devem alterar dados e geração na mesma transação sob o gate;
escrita direta fora desse protocolo não é suportada. O carregador não oferece
CRUD/Wails ou migração automática.

Evidência do carregador: testes SQLite temporários exercitam constraints
isoladamente, rollback da migração, índices incompatíveis, ownership e
isolamento global/workspace. Um teste com duas conexões e commit concorrente
confirma snapshot consistente, seguido de recusa na revalidação. Testes do
stamp cobrem troca de ID sem avanço de geração e mutação de slices/pointers
entregues ao projetor. A integração do App cobre geração ausente e alterada
durante a projeção, usando somente banco e chaves de fixture.

#### Projeção estrita de leitura local (subconjunto interno)

`commandconfig.ProjectLocalRead` liga os documentos à configuração pura do
resolver. O subconjunto atual aceita somente escopo global de armazenamento,
`keyboard.local`, comandos de catálogo `read`, sem decisão, `Context.None`, sem
alvo mutável e sem alteração de capacidade. Uma allowlist confiável adicional
declara quais handlers não recebem argumentos; o catálogo atual ainda não
descreve esse contrato. Argumentos e apresentação devem ser objetos vazios.
Não há aceitação de argumentos secretos, templates ou referências a tools.

O formato **interno** v1 do acionador é
`{"version":1,"code":"KeyK","modifiers":["Control","Shift"]}`.
Usa códigos físicos fechados (letras, dígitos, F1–F24 e navegação básica), com
modificadores Control/Alt/Shift/Meta únicos, normalizados nessa ordem. Não é
ainda contrato de API pública nem adapter DOM/nativo. A condição usa
`{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":true}]}`:
conjunção dos campos tipados já suportados pelo resolver, sem campos repetidos.
Versões futuras, campos desconhecidos, chaves JSON duplicadas, tipos incorretos
e documentos excedendo limites são recusados. Até registros desabilitados são
validados; um documento inválido recusa o mapa inteiro, sem fallback permissivo.

Defaults e camadas builtin vêm exclusivamente do bootstrap, com fingerprint
semântico fornecido por ele, sem hash da projeção parcial. Este subconjunto só
aceita defaults de escopo Global. Deltas preservam supressões, invariantes e
`needs_review` do resolver; não persistem automaticamente ajustes de versão.
Referências builtin são conferidas contra as camadas declaradas e a camada do
default. Camadas novas usam precedência ExplicitLayer e só participam quando
habilitadas **e** explicitamente ativas no contexto confiável do chamador.

`rebuildPersistedLocalReadConfiguration` integra esse projetor ao caminho de
reauth/publicação/revalidação de geração do App. Nesse caminho, camadas de
usuário continuam inativas e qualquer tentativa de passar uma lista de claims
é recusada: restore autenticado de claims permanece pendente. Testes usam
apenas SQLite temporário e catálogo/handlers de fixture. Nenhum binding de
produto ou adapter físico é registrado por esse helper.

#### Escrita preparada de enabled (primitivas internas)

`PrepareBindingEnabled` prepara exclusivamente habilitar/desabilitar um binding
global existente. Valida o mapa antes/depois, recusa no-op, documentos fora do
subconjunto e qualquer ajuste/revisão pendente de defaults. A proposta mantém
dados e geração privados; `Diff` retorna cópias independentes, sem aceitar de
volta payload editado pelo cliente. Preparar não escreve nem ativa camadas.

`CommitBindingEnabled` é uma primitiva de repository, não autorização. Sob o
gate exclusivo do host, faz CAS do ID e valor da geração, compara o binding
integral com o estado preparado e troca somente `enabled`. Dados e incremento
da geração pertencem à mesma transação SQLite. Conflito, remoção, erro de
escrita ou cancelamento revertem ambos; replay não reaplica a proposta e
esgotamento de int64 não recicla a geração. Não há retry automático.

`HostState.ChangeUserConfiguration` captura sessão/segurança/revisão do host,
aguarda preparação/decisão fora do gate e revalida antes do commit. Cancela
execuções do usuário e remove seu mapa antes de chamar o escritor; erro/panic
do escritor nunca restaura cache antigo. Outras contas conservam seus mapas.
Uma reconstrução autenticada separada é obrigatória após a tentativa de commit.

O helper não exportado `changeCommandBindingEnabled` conecta esse caminho ao
JWT/sessão atual do App e exige um adapter **confiável** de confirmação do diff.
Exige também política autoritativa de escrita, reavaliada sob o gate antes da
preparação e do commit; ownership ou confirmação não substituem essa política.
Não aceita booleano de aprovação vindo de payload. Esse seam é testado com
decisões de fixture: ainda NÃO está ligado ao DecisionDialog da AEP-0091, ao
ledger/auditoria de comandos write ou a Wails/tools. Portanto não constitui
uma funcionalidade de configuração pronta para uso no produto.

Evidências: SQLite temporário cobre commit, replay, concorrência, ABA, rollback,
overflow e cópias de diff; testes do host/App cobrem negação, sessão inválida,
lock durante confirmação, geração alterada durante espera e descarte de mapa.
Não há migração automática, criação de bindings/camadas/gerações, restore,
rebase ou persistência de claims neste subconjunto.

#### Recibos de decisão e adapter local (subconjunto interno)

`commanddecision` implementa pedidos de `config_mutation`/`local_session` com
IDs UUIDv7, usuário/sessão, fingerprint fornecido pelo produtor confiável,
gerações e expiração. As ações deste subconjunto são fechadas: `apply` e
`deny`. `Decide` registra `pending` antes de invocar o presenter e faz CAS
único para accepted/denied/cancelled/expired, com evento na mesma transação.
IDs divergentes, ações desconhecidas, erros, panic e cancelamento não concedem
aprovação. O prazo absoluto inclui a espera na fila de diálogos.

`Consume` confere todos os vínculos atuais, ação afirmativa e prazo, e grava
consumo/evento na mesma transação do efeito SQLite fornecido pelo host. O
callback só pode usar a transação recebida; falha reverte as três partes.
Reentrega não consome novamente. O serviço não autentica nem adquire o gate:
essa revalidação deve ocorrer no executor/host confiável antes de consumi-lo.
O fingerprint não é calculado por esse repository, e Body/diff nunca é
persistido nos recibos ou nos eventos. Os timestamps desse subconjunto SQLite
são inteiros Unix em milissegundos. Migração continua explícita, fora do App.

`commandDecisionPresenter` usa o `questionnaire.Manager` existente com
`kind=decision`, ações com polaridade/escopo explícitos, corpo documental e
rótulos pt-BR/en/es. O ID curto do questionário serve só ao transporte da UI:
o adapter anexa o UUIDv7 backend à resposta. Não interpreta rótulos traduzidos
como ações e não aceita ID de decisão injetado nas respostas.

O caminho interno `changeCommandBindingEnabledWithDecision` agora compõe essas
peças com o escritor real de bindings. `ChangeUserConfigurationWithEpoch`
entrega à preparação as gerações capturadas junto da autenticação, sem nova
captura fora do gate. `ConfirmBindingEnabled` vincula UUIDv7 privado da proposta,
usuário/sessão, gerações de autenticação/segurança, ID/valor da geração global e
documentos completos antes/depois a um HMAC versionado. O produtor não recebe
fingerprint, ID de decisão ou aprovação da UI. O renderizador confiável recebe
cópias do diff; o texto apresentado não fica no comprovante interno retornado.

`SignConfigurationMutation` usa a chave já reservada
`command-request-hmac:vN`, com domínio/ação próprios para configuração global.
O envelope fechado é JCS de strings; a geração int64 é uma string decimal e
os dois documentos de snapshot são strings JSON opacas, assinadas byte a byte,
não JSON arbitrário recanonizado. Isso é deliberadamente conservador: nenhuma
mudança nos bytes do snapshot privado é ignorada. A leitura da chave e a espera
pelo diálogo ficam fora do gate; o commit não consulta o Credential Manager.
Este incremento não cria segredos nem reutiliza JWT/refresh pepper.

`CommitConfirmedBindingEnabled` usa `ConsumeForDatabase` para verificar que
configuração e recibos compartilham a mesma raiz `sql.DB`, recusando outro banco
ou transação pré-aberta. Uma única transação consome a receipt, registra o
evento de consumo, faz CAS da geração e grava `enabled`. Não há commit interno
ou savepoint independente no escritor. Prazo vencido após o UPDATE, conflito,
erro de escrita ou erro de evento revertem consumo e configuração juntos.
O host reautentica/reautoriza e verifica o epoch original sob o gate exclusivo
antes de invalidar o mapa e entregar o commit. O mapa não é restaurado em falha.

Testes de integração usam SQLite temporário e o presenter real do App com o
Manager de questionários: token inválido, negação, política revogada, lock
durante o diálogo e sucesso confirmado. Testes de repository cobrem replay,
propostas concorrentes, CAS obsoleto, vínculo de epoch, outro banco, rollback e
expiração precisamente após a escrita. O caminho anterior de callback simples
continua como seam interno de testes, sem entrypoint de produto.

Validação deste incremento: dez pacotes do núcleo, testes focados do App,
`go build ./...`, `go vet ./...` e verificador dos AEPs passaram. Concorrência
de propostas e expiração após UPDATE passaram dez repetições. Não se declara
suíte geral verde: sua tentativa encontrou testes legados de `internal/config`
tentando escrever a configuração real do usuário, recusados pelo sandbox.
Os novos testes usam bancos temporários; não há habilitação no produto.

O commit confirmado agora também grava `command_config_mutations`, com IDs,
sessão, fingerprint, receipt e valores antes/depois, sem texto sensível. Falha
nessa inserção reverte binding, geração, consumo e evento. A migração explícita
valida o schema esperado; tabela/view incompatível aborta toda a migração, sem
reescrever o objeto existente. Constraints recusam UUIDs inválidos, decisão
duplicada, no-op e incremento de geração incorreto. Testes cobrem esses casos,
consulta escopada, replay e rollback completo; o teste do App confirma a linha
de auditoria correspondente à única alteração bem-sucedida.

`EpochService.WatchEpoch` agora liga a preparação/decisão ao epoch capturado.
A inscrição ocorre sob gate com revalidação, portanto invalidação entre captura
e inscrição não é perdida. Lock/logout/invalidação cancela o contexto da espera,
sem chamar UI sob o gate. `HostState` libera essa inscrição antes da própria
publicação e mantém a revalidação atômica original para o commit. O teste do App
bloqueia a sessão com o diálogo aberto, não envia resposta da UI e exige
cancelamento, sem esperar o prazo do diálogo. Testes adicionais cobrem isolamento
entre sessões e liberação idempotente das inscrições.

Ainda falta registrar a invocação de write no ledger quando o comando de produto
for registrado: nem histórico de receipt nem auditoria de configuração são
substitutos da auditoria completa de execução.

`commanddecision.ReconcileSession` agora recupera recibos `pending`/`accepted`
da sessão autenticada: prazo vencido vira `expired`; gerações anteriores viram
`cancelled`. Recibos atuais válidos, estados encerrados e outras sessões/contas
permanecem intactos. Cada lote aceita até 128 linhas, em ordem de UUID, e devolve
`More` quando o snapshot contém outros candidatos. Estado e evento são gravados
na mesma transação; erro/cancelamento reverte o lote e não reporta progresso
parcial. O índice por usuário/sessão/status/ID evita varredura entre contas;
a migração recusa índice homônimo incompatível. Não há reapresentação, consumo,
efeito, exclusão ou reativação automática. O horário da resposta anterior é
preservado quando existente; o evento registra o instante do encerramento.

`recoverCommandDecisionSession` compõe a operação com JWT, sessão atual do App,
política, cofre e estado do SO revalidados sob o gate. Processa um lote por
chamada e invalida o mapa antes da tentativa, sem loop prolongado sob a trava.
O chamador deve terminar os lotes e reconstruir o mapa autenticadamente. Testes
do App recuperam a confirmação obsoleta sem repetir binding/auditoria nem abrir
diálogo, recusam token/política inválidos e comprovam idempotência. Testes de
repository cobrem limites, isolamento, recuperação de Store recriado,
concorrência e rollback por falha no evento.

`HostState.RebuildUserConfiguration` também inscreve o carregamento no epoch
capturado, com revalidação da janela entre captura e inscrição. Lock, logout
ou invalidação cancelam o contexto entregue ao builder/projetor, permitindo
interromper I/O cooperativo antes de terminar a leitura. A inscrição é liberada
em erro, configuração inválida, panic e sucesso, antes da publicação. O commit
continua usando o contexto original e revalidando sessão, epoch e revisão do
host: cancelamento não substitui essas verificações. Testes no host e na borda
autenticada do App cobrem cancelamento durante projeção, perda da observação do
SO e reconstrução posterior. Builders que ignoram contexto não são interrompidos
à força; seus resultados obsoletos continuam recusados. Não há trabalho novo
no caminho de resolução por tecla nem promessa de latência da trava/SQLite.

Permanecem pendentes bootstrap autenticado do presenter e chamada automática da
recuperação antes de publicar o mapa. Esta recuperação é estritamente da sessão
retomada: manutenção de recibos de outras sessões abandonadas, retenção e
recuperação completa de invocações ainda não estão integradas. Não há entrypoint Wails/tool novo ou alteração
de configuração acessível ao usuário por esse incremento.

Pendente: ampliar o projetor para contratos de produto, integrar o ledger
de write, ampliar escritores para CRUD/restauração confirmada,
persistência/restore de claims, ligação completa à recuperação pós-unlock e
estado de execução por workspace. O estado do SO não
é inferido da presença de uma sessão/JWT ou da disponibilidade do cofre; sem
observação válida, o host continua fechado. Não há atalhos nem comandos de
produto ativados.

Incremento inicial: `internal/commandcatalog` contém um snapshot imutável dos
contratos estáticos de comando, com IDs exatos e namespaced, efeitos,
mutabilidade, origens permitidas e políticas de contexto por provider/fato.
O registro recusa metadata divergente do descriptor confiável do handler,
escrita/ação com alvo mutável usando contexto `none`, políticas temporais sem
TTL positivo e qualquer origem `cli`/`event`/`system` em comando destrutivo.
`AllowsSource` consulta somente a declaração; não concede autorização.

Validação: `go test ./internal/commandcatalog ./internal/commandbindings`
passou (100% de cobertura no catálogo inicial, 98,9% no seletor); `go vet`
dos dois pacotes passou. Nenhum comando do produto está registrado ainda.
O descriptor do handler será obtido no bootstrap a partir de
`EffectClass()`/`Mutability()`, nunca de cliente. Permanecem pendentes schemas
de argumentos, integração de aliases/locales na UI, risco/redação, disponibilidade, versão do
catálogo, ponte e executor. Este incremento não conclui a Fase 1.

O catálogo agora possui apresentação opcional versionada neste estágio de
protótipo. Quando fornecida, exige nome, descrição e categoria nos três locales
(`pt-BR`, `en`, `es`), com aliases validados pela mesma normalização da busca.
`Registry.Search` localiza por ID e metadata, ordena por ID e devolve cópias
profundas; `Lookup` continua exigindo identidade canônica exata. A normalização
inicial uniformiza caixa e espaços, sem busca aproximada ou remoção de acentos.
Ainda falta exigir essa apresentação no bootstrap de comandos do produto e
ligar a Command Palette ao catálogo, sem listas paralelas no frontend.

Validação do incremento conjunto: testes dos dois pacotes passaram (92,5% de
cobertura em bindings e 96,9% em catálogo), assim como `go build ./...`,
`go vet ./...` e verificador de status dos AEPs. Testes adicionais cobrem
permutações da composição, conflitos entre overrides, proveniência,
restauração sem mutar snapshots e leituras concorrentes com retornos isolados.
O `golangci-lint` local não executou: binário v1 incompatível com configuração
v2 do repositório. A suíte geral e o detector de corrida continuam com as
limitações registradas na Fase 0; não se declara validação integral ou CI verde.

- Implementar registro tipado de comandos.
  - Incremento experimental adicional: `internal/commandbindings/defaults.go`
    materializa overrides do mesmo acionador antes da precedência, aplica
    tombstones antes da deduplicação e permite restaurar reconstruindo o
    snapshot sem o delta. Desabilitar a personalização preserva fallback.
    Testes cobrem condições restritas, versões, referências órfãs, imutabilidade
    e barreira de diálogo. Os dois pacotes passaram dez repetições dos testes;
    `go vet` focado também passou.
  - Não há persistência/restore transacional, remapeamento de acionador,
    fingerprint RFC 8785, ledger de supressão nem reserva de invariantes no
    dispatcher. Em mudança semântica, sem snapshot histórico completo, a
    pendência ainda bloqueia conservadoramente o acionador fora de diálogos;
    isso precisa ser refinado para o contexto exato antes de integrar ao produto.
- Definir schema versionado de contexto, acionadores, bindings e camadas.
- Criar camadas padrão no código e persistência de deltas no SQLite.
- Implementar resolução determinística, índice em memória e diagnóstico de
  conflitos.
- Cobrir precedência, fallback de defaults, restore e concorrência com testes.

### Fase 2 — Teclado local e hotkeys globais

- Migrar incrementalmente os atalhos do workspace e chat para comandos.
- Preservar comportamento de foco, input, menus, modais e abas.
- Integrar `internal/hotkey` como adapter global.
- Migrar hotkeys de perfis e o trigger `hotkey` de jobs para bindings que
  executam comandos canônicos.
- Remover handlers paralelos somente após equivalência automatizada.

### Fase 3 — Command Palette e configuração

- Implementar Command Palette acessível.
- Criar lista e detalhe de camadas.
- Implementar captura de teclado, regras de ativação e análise de conflitos.
- Implementar restauração por binding, camada e conjunto completo.
- Atualizar ajuda de atalhos para consultar o mapa efetivo.

### Fase 4 — Contexto e programas em foco

- Integrar contexto de surface, aba, workspace, foco e diálogos.
- Implementar adapter Windows para janela/processo em primeiro plano.
- Implementar fixação manual, estabilização e fallback seguro.
- Definir adapters equivalentes ou degradação explícita em Linux e macOS.

### Fase 5 — Stream Deck direto

- Implementar gerenciador Go de dispositivos.
- Mapear teclas físicas para acionadores normalizados.
- Renderizar bindings efetivos com cache e atualização diferencial.
- Implementar imagens, estados, navegação por camadas e múltiplos dispositivos.
- Adicionar reconexão, diagnóstico de disputa e desligamento limpo.
- Validar operação sem o software oficial instalado.

### Fase 6 — Chat, CLI e automações

- Expor catálogo, execução e gerenciamento estruturado ao chat.
- Expor listagem e execução na CLI somente para comandos que declarem essa
  origem e não dependam de surface visual.
- Integrar eventos e jobs sem criar executor paralelo.
- Integrar exportação/importação e auditoria.

### Fase 7 — Expansão de adapters

- Avaliar pedais USB, controles MIDI e outros dispositivos.
- Avaliar dial e gestos avançados conforme capacidades detectadas.
- Avaliar controle privilegiado de programas externos em AEP ou decisão de
  segurança específica.

#### Avaliação de expansão — 24/09/2026

A arquitetura permite novos adapters sem outro executor, mas não autoriza
tratar qualquer dispositivo como teclado nem atribuir capacidades por nome
de modelo. A avaliação desta fase não declara drivers adicionais entregues:

- **Pedais USB:** quando o próprio equipamento emite teclas, a entrada já
  pertence ao adapter de teclado e às suas regras de foco/ownership. Um pedal
  HID com protocolo próprio exigirá descoberta, captura e lifecycle próprios;
  não se deve prometer identificação individual para um pedal que o sistema
  operacional apresenta apenas como teclado.
- **MIDI:** note-on/note-off e controles contínuos precisam de normalização
  explícita, política para rajadas e cancelamento/reconexão. Control changes
  não devem gerar decisões destrutivas a cada amostra. O backend só poderá
  receber candidatos de um adapter autenticado, nunca identidade fornecida
  por mensagens MIDI.
- **Dials e gestos:** rotação, direção, quantidade e pressionamento são
  capacidades diferentes. Um adapter futuro deve declarar as suportadas e
  definir agregação/repetição antes de publicar bindings; não reutilizar
  implicitamente uma tecla discreta para um eixo contínuo. Gesto longo
  permanece adiado por decisão do mantenedor.
- **Controle de outros programas e broker externo:** exigem decisão de
  segurança específica sobre identidade, permissões e confirmação. Esta
  avaliação não habilita controle privilegiado nem dispositivos físicos
  para JWT externo.

Conclusão: manter os adapters atuais e exigir protótipo com hardware,
descoberta/captura acessível, isolamento de dispositivo/sessão e recuperação
testados antes de cada expansão. Não há dependência desses novos drivers
para concluir os comandos de teclado e Stream Deck desta entrega.

### Evidência parcial: custo de resolução e orçamento de latência

O resolvedor puro já usa snapshot imutável indexado por acionador: leitura de
SQLite, Credential Manager, receipts e reconstrução de configuração não fazem
parte de `Resolver.Resolve`. O caminho de um único candidato evita as listas
intermediárias de dominância/prioridade, preservando validação de fatos,
elegibilidade, bloqueio por diálogo e proveniência. Testes diferenciais contra
o caminho geral cobrem também candidatos desabilitados, camadas inativas,
foreground, contexto inválido e isolamento do resultado retornado.

Benchmarks reproduzíveis em `internal/commandbindings/resolver_benchmark_test.go`
cobrem volume de bindings, colisões e leitura concorrente do mesmo snapshot.
`internal/commandsecurity/gate_benchmark_test.go` mede admissão isolada,
admissões concorrentes e mistura com mutações exclusivas de callback vazio.
Os valores `ns/op` são médias de microbenchmark, não percentis de experiência
do usuário; callbacks vazios não representam contenção de SQLite ou de login.

Amostra local em Windows/amd64, Intel Core Ultra 7 155H, Go 1.26.2,
`-benchtime=200ms -count=1`: seleção de um candidato entre 1/100/1000 bindings
distintos ficou em aproximadamente 65–68 ns/op (16 B, uma alocação); colisões
de 10/100 candidatos no mesmo acionador ficaram em aproximadamente 3,2/33 µs.
São fixtures sintéticas e uma única rodada, não garantia ou limite de produto.
Para reproduzir: `go test ./internal/commandbindings -run '^$' -bench
BenchmarkResolve -benchmem -benchtime=200ms` e o equivalente em
`./internal/commandsecurity` com `-bench BenchmarkDispatchGate`.

Orçamento experimental para integração futura: p95 abaixo de 1 ms de
processamento interno para atalhos locais simples, sem incluir o trabalho da
ação. Essa meta ainda **não foi demonstrada ponta a ponta** e não deve virar
asserção temporal frágil em teste unitário. A validação de produto deve medir
separadamente resolução, fila/gate, autenticação/ledger, handoff e renderização,
com troca de abas, alteração de bindings, sessão bloqueada e carga concorrente.

Não foi introduzida exceção ao fluxo auditado de D4: distinguir ações
puramente visuais de comandos com efeitos exige classificação explícita e
revisão do contrato antes de qualquer dispensa de ledger/autorização. `Ctrl+N`
não é presumido visual/read-only: pode criar dados persistentes. Receipts de
configuração continuam exclusivos da mutação confirmada, não de cada uso do
binding. Uma otimização futura nunca pode reutilizar resultado de autorização
após invalidação nem remover a revalidação atômica para ganhar desempenho.

Risco ainda aberto: `DispatchGate` mantém exclusão durante a transação de
configuração; leitores podem esperar I/O de SQLite, e a aquisição do mutex
atual não é cancelável. Os microbenchmarks não resolvem nem limitam essa espera.
Antes de ativar atalhos no produto, medir caudas de latência com essa contenção
real e definir tratamento de indisponibilidade sem execução com mapa obsoleto.

## Riscos

- **Conflitos difíceis de compreender:** muitas camadas podem tornar o resultado
  surpreendente. Mitigação: resolução determinística, explicação do binding
  efetivo, simulação de contexto e confirmação de sobreposição.
- **Regressão de atalhos locais:** migração pode perder regras de foco ou modal.
  Mitigação: inventário, migração incremental e regressões por surface.
- **Camada personalizada ocultar defaults:** um mapa completo substituiria
  comandos como `Ctrl+N`. Mitigação: composição aditiva e overrides por binding.
- **Listeners duplicados:** frontend, hotkey global e dispositivo podem disparar
  duas vezes. Mitigação: identidade normalizada, ownership por adapter e
  deduplicação de evento físico e de candidatos equivalentes.
- **Troca excessiva de contexto:** foco rápido pode causar oscilação do Stream
  Deck. Mitigação: eventos, estabilização curta, fixação manual e cache.
- **Custo de renderização:** imagens podem consumir CPU e USB. Mitigação: cache,
  pré-processamento e diff por tecla.
- **Biblioteca ou modelo incompatível:** dependência Go pode não cobrir todos os
  aparelhos. Mitigação: capability detection e protótipo na Fase 0.
- **Disputa pelo dispositivo:** outro processo pode possuir o HID. Mitigação:
  erro recuperável e reconexão, sem falhar o aplicativo.
- **Ações perigosas fora de foco:** uma tecla física pode executar mutações sem
  contexto visível. Mitigação: política por comando, confirmação acessível,
  anúncio e grants explícitos.
- **Monitoramento invasivo de janela:** títulos podem conter informação
  sensível. Mitigação: usar identidade do processo por padrão e minimizar
  persistência/log.
- **Interface ainda complexa:** flexibilidade pode sobrecarregar a configuração.
  Mitigação: lista de camadas, detalhe com duas seções e divulgação progressiva.

## Critérios de aceitação

IDs estabilizados pela reconciliação de 18/09/2026 e relidos em 22/09/2026:
C01–C83 preservam a
baseline; C84 identifica a cláusula de apresentação local já acrescentada
na seção 68. Existem **84 critérios finais**, distintos dos 84 itens históricos
de infraestrutura. Estado de implementação e evidências de cada um estão na
[matriz vigente](0103-tasklist-conclusao.md#5-matriz-reconciliada-dos-84-critérios-finais).
Checkbox é aceite final, não ausência de código.

- [ ] C01 — Existe registro canônico e pesquisável de comandos com IDs, argumentos,
  disponibilidade, risco, aliases localizados e apresentação.
- [ ] C02 — Teclado local, hotkey global, Stream Deck, Command Palette, chat e CLI
  podem convergir para o mesmo comando sem handlers finais duplicados.
- [ ] C03 — Todo acionamento classificado para execução durável produz `CommandInvocation` e
  passa por `CommandExecutionService`, com sessão, proveniência, autorização,
  deduplicação e auditoria antes do handler final; `effect = suppress` é
  consumido sem criar invocação.
- [ ] C84 — Apresentação local utiliza a projeção efetiva e valida o contexto na UI,
  sem invocação/ledger por tecla; persistência da última seleção é separada.
- [ ] C04 — A reserva atômica por evento impede reentrega, e ownership exclusivo
  impede duplicidade entre teclado local/global e listeners de dispositivo.
- [ ] C05 — Solicitações diretas e triggers sem `source_event_id` usam
  `invocation:<invocation_id>`; eventos usam `event:<source_event_id>`.
- [ ] C06 — Manter uma tecla pressionada não repete comandos fora da lista explícita
  de navegação local da D3: repeats elegíveis têm ocorrência independente;
  demais repeats são descartados antes de gerar `source_event_id`. Testes
  cobrem identidade da combinação, release, blur, reconexão e saturação.
- [ ] C07 — Retirada de `queued` revalida todos os gates no mesmo CAS para `running`.
- [ ] C08 — Cada instância física usa geração própria e índice parcial de eventos;
  invocações diretas deduplicam somente pela PK UUIDv7.
- [ ] C09 — Execução por agente e automação preserva e revalida os gates da AEP-0101;
  origem headless não herda a identidade do usuário para autorizar mutações.
- [ ] C10 — Usuário e ator são derivados pelo backend; payload não escolhe identidade
  de autorização/auditoria.
- [ ] C11 — Camadas padrão do aplicativo e das surfaces permanecem ativas e um binding
  ausente em camada superior cai para o default.
- [ ] C12 — Overrides afetam somente o acionador e contexto declarados.
- [ ] C13 — Tombstone bloqueia o default no contexto declarado, enquanto
  personalização apenas desabilitada permite fallback.
- [ ] C14 — Tombstones são aplicados antes da deduplicação e nunca produzem invocação.
- [ ] C15 — Tombstone que consome um acionador grava marcador terminal no ledger;
  reentrega do mesmo evento não passa a executar um default após mudança de
  configuração.
- [ ] C16 — Acionamento stale não grava `suppressed`, mas recebe marcador terminal
  `rejected_stale`; o mesmo ID nunca executa em reentrega posterior.
- [ ] C17 — Override de default persiste ID e versão do default substituído.
- [ ] C18 — É possível restaurar um binding, uma camada ou todas as personalizações.
- [ ] C19 — Conflitos são detectados considerando a possível interseção de contextos,
  e empate não executa dois comandos.
- [ ] C20 — Escopo, especificidade e prioridades persistidas produzem resolução
  determinística após importação/restart; empate termina em conflito fail-closed.
- [ ] C21 — Bindings equivalentes por comando, argumentos e escopo produzem uma única
  invocação com proveniência preservada.
- [ ] C22 — O resolvedor não consulta SQLite nem percorre o catálogo completo a cada
  acionamento.
- [ ] C23 — Mudanças de surface, foco, workspace, janela externa e eventos podem
  ativar e desativar camadas de forma determinística.
- [ ] C24 — Desabilitar camada a remove imediatamente do mapa sem ressuscitar claims
  stale ao reabilitá-la; expiração local é idempotente após restart.
- [ ] C25 — Ativações por evento têm ID, sequência, correlação e deduplicação; evento
  atrasado não encerra ciclo mais novo.
- [ ] C26 — Claim e ledger de ativação preservam o escopo global/workspace, inclusive
  para refs `builtin`; eventos e replay de outro workspace falham fechado.
- [ ] C27 — A primeira versão aceita apenas fatos de `job_run_events` espelhados
  transacionalmente na outbox durável; EventBus best-effort e produtores
  externos falham fechado.
- [ ] C28 — Count-cap/cascade de runs não remove a outbox antes do deadline; startup
  recupera leases e reprocessa pendências antes da retenção de jobs.
- [ ] C29 — Estado de ativação persistido é reconciliado em modo seguro no startup e
  preserva autenticação, geração e proveniência anti-loop da AEP-0067.
- [ ] C30 — Claim de job sem lease e fonte autoritativa válidas fica inativa.
- [ ] C31 — Replay de ativação fora da retenção é rejeitado, e ownership vem do
  principal autenticado, não do payload.
- [ ] C32 — A Command Palette busca e descreve comandos disponíveis e indisponíveis
  com motivo, mas executa somente os disponíveis.
- [ ] C33 — A Command Palette tem navegação completa por teclado, anúncios e
  restauração de foco cobertos por testes e validação NVDA.
- [ ] C34 — A configuração por chat usa tools estruturadas, IDs reais e confirmações
  de segurança.
- [ ] C35 — Toda mutação persistente solicitada por agente mostra diff, exige decisão
  explícita e falha fechado sem interlocutor.
- [ ] C36 — `command_catalog.execute` aplica o mesmo gate a comandos que alteram
  capacidade efetiva, incluindo ativação de camada.
- [ ] C37 — A tela de configuração oferece lista de camadas, detalhe de ativação e
  bindings, captura de teclas e explicação do resultado efetivo.
- [ ] C38 — Toda configuração é operável por teclado e NVDA sem depender de grade,
  arrastar, imagem ou cor.
- [ ] C39 — O Assistente controla ao menos um modelo de Stream Deck diretamente por
  Go, sem software oficial, com reconexão e shutdown limpo.
- [ ] C40 — Sem sessão autenticada, e durante logout ou troca de usuário, o Stream
  Deck fica em estado seguro e rejeita callbacks de gerações anteriores.
- [ ] C41 — O Stream Deck atualiza somente teclas cujo conteúdo efetivo mudou e usa
  cache de imagens.
- [ ] C42 — Abertura/reconexão do Stream Deck invalida o diff e força frame completo.
- [ ] C43 — Camadas baseadas no programa em primeiro plano funcionam no Windows e
  degradam explicitamente em plataformas sem adapter.
- [ ] C44 — Contexto externo é capturado antes de bring-to-front e não muda no meio do
  acionamento.
- [ ] C45 — Comandos disparados fora de foco preservam permissões, decisões e
  auditoria do executor de destino.
- [ ] C46 — Exportação/importação preserva UUIDs e escopos, relata referências e
  conflitos e não transfere grants nem histórico de invocações.
- [ ] C47 — Binding persistente e export não contêm segredos brutos; delegação a tool
  propaga redação ou permanece indisponível.
- [ ] C48 — Referência importada de credencial resolve pattern exato no usuário de
  destino ou deixa o binding desabilitado.
- [ ] C49 — `command_invocations` tem payload redigido, origem rastreável, índices e
  retenção por idade e quantidade, sem prometer reconstruir o snapshot completo.
- [ ] C50 — `command_invocations.invocation_id` é a PK canônica da invocação,
  consulta e correlação com tools; o ledger tem PK própria `id` e referências
  UNIQUE explícitas.
- [ ] C51 — Reentrega dentro da janela retorna status/resultado redigido sem repetir o
  handler; invocações interrompidas por queda viram `outcome_unknown`.
- [ ] C52 — Evento durável preserva a chave pelo horizonte de replay da fonte e,
  depois dele, é rejeitado por `source_occurred_at` autenticado em vez de ser
  tratado como solicitação nova.
- [ ] C53 — Ativações por evento persistem o mesmo epoch/deadline imutável da fonte;
  aumentar retenção não reabre ocorrência antiga.
- [ ] C54 — Recuperação de startup atualiza auditoria e ledger para
  `outcome_unknown` na mesma transação.
- [ ] C55 — Reutilizar `invocation_id` com request fingerprint diferente falha
  fechado.
- [ ] C56 — Caps de auditoria não removem os ledgers antes de `expires_at`; compactar
  registro recente não permite nova execução ou ativação.
- [ ] C57 — Consulta de invocação aplica propriedade por usuário e autorização do
  ator, sem lookup cross-user apenas pela PK.
- [ ] C58 — Sessão, geração de segurança e staleness de contexto são revalidados
  imediatamente antes de todo handler.
- [ ] C59 — Policies `max_age_ms`/`event_snapshot` falham fechado sem timestamp de
  cada provider; ingresso não transforma snapshot sem `capturedAt` em contexto
  recém-capturado.
- [ ] C60 — `handler.Start` confirma handoff sem bloquear; logout/mutação concorrente
  não espera o trabalho longo nem entra em deadlock.
- [ ] C61 — Versões do catálogo e da configuração são revalidadas ao retirar da fila;
  binding alterado não executa resolução antiga.
- [ ] C62 — Cache de resolução inclui usuário, workspace, acionador, origem,
  `context_version` e todas as versões/gerações de catálogo, configuração e
  camadas ativas.
- [ ] C63 — Cada comando declara `context_policy`; nas policies que declaram
  providers, provider ausente ou versão/TTL inválido falha fechado.
- [ ] C64 — `context_policy = none` é rejeitado para qualquer comando não read-only.
- [ ] C65 — Contextos local, JWT externo, job e system têm fontes de identidade e
  revogação explícitas; `EpochService` invalida trabalho obsoleto.
- [ ] C66 — Ativação event-driven usa grants próprios de camada, com chave natural,
  geração monotônica, histórico de revogação e revalidação autoritativa por
  evento; não reutiliza nem amplia grants de delegação da AEP-0101.
- [ ] C67 — Adapter de jobs exige `job_slug = Job.ID` e
  `job_database_id = Job.DatabaseID`, confirma ambos por owner e permanece
  desabilitado para fatos legados ambíguos.
- [ ] C68 — Evento de ativação recebido é candidato sem autoridade; dispatcher
  deriva owner, workspace, regra, layer e epochs antes do envelope interno.
- [ ] C69 — Regras e layers builtin/user usam refs polimórficas consistentes no
  schema, grants, estado, ownership, importação e restore.
- [ ] C70 — Após o PR atualizar a AEP-0052, identidade externa só acessa usuário
  local por mapeamento administrativo exato de emissor e subject; antes disso,
  o command manager fica indisponível nesse modo.
- [ ] C71 — Cada comando declara origens permitidas e o serviço bloqueia origem não
  autorizada, incluindo comandos visuais solicitados pela CLI.
- [ ] C72 — `effect_class` e mutabilidade vêm do contrato do handler; metadata
  divergente impede o registro.
- [ ] C73 — CLI não executa comando que exija diálogo/decisão interativa.
- [ ] C74 — Comando destrutivo só avança com receipt de decisão criada no backend,
  vinculada à solicitação e consumida uma vez no CAS para `queued`.
- [ ] C75 — `cli`, `event` e `system` não registram/executam comando destrutivo;
  qualquer origem sem presenter interativo falha fechado.
- [ ] C76 — Em autenticação externa, adapters físicos permanecem indisponíveis até
  existir vínculo local explícito e revogável com um principal externo.
- [ ] C77 — Estação bloqueada suspende hotkeys globais e dispositivos físicos e
  apresenta estado seguro até revalidar a sessão após desbloqueio.
- [ ] C78 — Diálogo topmost bloqueia fallback para camadas inferiores e os atalhos
  obrigatórios da AEP-0091 não aceitam tombstone.
- [ ] C79 — Dispatcher reserva atalhos invariantes do diálogo antes de qualquer
  binding configurável.
- [ ] C80 — Shell continua passando exclusivamente por `internal/commandpolicy`.
- [ ] C81 — Manutenção em escopo de instância cobre todos os usuários e registros
  `system` em uma única cadência.
- [ ] C82 — Deep links e configurações importadas não concedem execução arbitrária.
- [ ] C83 — Testes cobrem fallback de defaults, sobreposição, múltiplas camadas,
  modais, inputs, múltiplas abas, troca de foco, reconexão de dispositivo e
  prevenção de execução duplicada.
