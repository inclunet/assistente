# AEP — Assistente Enhancement Proposals

Propostas de melhoria para o Assistente, inspiradas nos PEPs (Python) e RFCs.

Este diretório é o **repositório único** de decisões arquiteturais do projeto
(ver `CLAUDE.md`). Não criar outro diretório para AEPs — tudo fica em `aep/`.

> **Inventário (2026-09-22):** este índice contém **107 documentos principais
> para 106 números ocupados**. A diferença é a colisão histórica 0074, representada
> temporariamente como 0074-A e 0074-B até sua renumeração. Séries multi-arquivo
> têm o principal listado na tabela e os demais em
> [Apêndices por AEP](#apêndices-por-aep). Convenção em
> [Convenção de numeração e anexos](#convenção-de-numeração-e-anexos).
>
> Verificação reproduzível: `python .github/scripts/verificar-status-aeps.py`.

## Índice

AEP-0103, seção139 após reconciliação129 (22/09/2026): **In Progress**.
**76/84 critérios com implementação identificada (90,5%); 8 parciais e
0 ausentes**, sem novo aceite final. Saídas R: **11 A / 14 I / 22 P / 1 N**,
25/48 implementadas incluindo aceitas (52,1%); **1/12 gates aceito**.
C34/C35/C36 e R11.1/R11.3: tools registradas, CRUD/restore/import com decisão,
export não sensível e execução como agente no serviço comum; sessão fixada
no ingresso GUI e recusa de autoridade emprestada para headless. C71/C73 e
R11.2 implementados: CLI list/describe/execute/retry/status, IDs de solicitação,
consulta autorizada e executor comum, sem presenter nem serviços autônomos.
Nenhum comando produtivo foi promovido a CLI: catálogo atual informa todos
como indisponíveis para execução, preservando D14. Qualificação de convergência
por família e demais lacunas dentro dos critérios parciais continuam pendentes.
Seção136: corrigida disputa SQLite no consumo de receipts, com CAS completo,
sem retry de efeitos; qualificação integrada ampliada sem novos aceites.
Seção137: provas adicionais de camadas por origem e callbacks diálogo/job;
inventário reconciliado com v40. App completo passou com ordem aleatória e
log integral; a intermitência anterior não foi reproduzida nem encerrada.
Seção138: integração da main `714a47c4e`, com checkpoint local recuperável.
Falha preexistente de revisão ABA ao fixar/desafixar mensagem identificada
no merge e tratada na seção139: revisão durável por mensagem, triggers
transacionais e migração v30, sem depender de timestamps ou alterar atalhos.
Catálogo v40: 149 comandos / 61 locais / 67 defaults; 81 IDs do Deck são
outro denominador. Δ18 corrigido: matriz estática preparada antes da entrada
física, com recusa de snapshot stale e fatos nativos atuais por tecla.
C62 implementado: LRU de seleção integrado, isolado e invalidado, sem cache
de autorização. C04/C06 implementados por recusa explícita de hotkeys globais
fora do Windows, sem fallback sem ownership/no-repeat. C77 qualificado com
lock/unlock global, perfil persistido e bootstrap produtivo. C78/C79 agora têm
reserva nativa temporária de Ctrl+Shift+R, stack real de diálogos, precedência
sobre configuração e teardown, sem confirmar decisões. R07.3 tem implementação
identificada; seu gate agregado permanece aberto. Latência integrada,
aceite físico/NVDA e demais gates continuam pendentes;
[estados, evidências e próximos passos](0103-tasklist-conclusao.md#139-revisão-durável-de-mensagens-contra-aba--22092026).

AEP-0103, seção128 (22/09/2026): 81 IDs no Deck contextual (79 anteriores +
aplicar/remover Mermaid), quatro fatos visuais restritos ao editor, inclusive
com condição somente de perfil; caminho incondicional e paleta preservados.
Integração e regressões amplas PASS: App 464,744 s; frontend 450 arquivos /
5.617 testes; build/vet, TypeScript/Vite e lint focado. Manuais pendentes.
**In Progress**, sem nova contagem dos 84 critérios ou aceite BASE-PRONTA;
[evidências](0103-tasklist-conclusao.md#128-mermaid-no-stream-deck-contextual).

AEP-0103, seção127 (22/09/2026): Stream Deck contextual ampliado para 79 IDs,
incluindo seis mutações de páginas de listas/perfis. Três campos para páginas
(foco/tipo/perfil), sem ID de aba; superfícies restritas pelo helper comum
ao ingresso. Oferta física única, alvo preparado e barreira da fonte até a
admissão. Integração automatizada PASS: App 273,020 s; frontend 448 arquivos /
5.578 testes; build/vet, TypeScript/Vite e lint focado. Aceites físicos/NVDA
pendentes. **In Progress**, sem encerrar BASE-PRONTA;
[evidências e limites](0103-tasklist-conclusao.md#127-páginas-de-listas-e-perfis-no-stream-deck-contextual).

AEP-0103, seção126 (22/09/2026): Stream Deck contextual ampliado para 73 IDs
(70 workspace + ativar/alternar/voltar camada), com quatro campos visuais,
oferta física única e API backend sem reserva UI. Argumentos/regra/escopo
persistidos, pilha por dispositivo e predicado em memória dentro de
`GenerationTx`, com versões publicadas/snapshot protegidos até a claim.
Checks focados PASS, incluindo backend Deck (59,409 s) e claim A-B-A da
paleta (23,835 s); `go vet` PASS e bindings Wails gerados. Backend amplo PASS
(358,565 s), antes das últimas correções de claim do teclado/condições nativas.
Build frontend com TypeScript PASS (Vite 1 min 3 s); suíte frontend
5529 testes/447 arquivos PASS (87,59 s), após correção do relógio do teste.
Regressão backend final PASS (249,874 s), com guardas finais integradas;
aceites manuais pendentes.
Páginas/Mermaid continuam fora da ampliação. **In Progress**, sem encerrar
os 84 itens históricos ou BASE-PRONTA; evidências na
[tasklist de conclusão](0103-tasklist-conclusao.md#126-camadas-no-stream-deck-contextual).

AEP-0103, seção125: Stream Deck contextual para 70 comandos de workspace
(72 menos duas mutações Mermaid), com quatro campos visuais e seleção única
entre ramos locais e duráveis no mesmo evento físico. Ramo local sem roundtrip
de execução/ledger; durável com oferta opaca vinculada ao host e snapshot,
uso único e TTL de 10 segundos. Frontend completo: 445 arquivos/5480 testes
PASS; build com TypeScript, build/vet de `internal/app` e diff check PASS.
**In Progress**; regressão backend ampla PASS (194,531 s), aceites manuais e demais
gates pendentes. Evidências na
[tasklist de conclusão](0103-tasklist-conclusao.md#125-stream-deck-contextual-para-comandos-do-workspace).

AEP-0103, seção124: ativar/alternar/voltar camada na paleta contextual das
abas do workspace, preservando executor, argumentos persistidos e conclusão
após renovação do mapa. **In Progress**; outras origens, páginas sem aba e
aceites manuais permanecem separados. Evidências na tasklist de conclusão.

AEP-0103, seção123: seis mutações de listas/perfis na paleta contextual,
com origem visual de página e alvo validado separadamente. Foco, tipo de tela
e perfil; sem condição de ID de aba para este grupo. **In Progress**;
formulários modais, ações de camada, Deck durável e aceites permanecem separados.
Evidências na [tasklist de conclusão](0103-tasklist-conclusao.md).

AEP-0103, seções121–122: paleta contextual ampliada de ações de abas/workspace
para mensagens, terminal e editor, preservando os protocolos próprios e a
validação do alvo. Arquivos suportam a continuação controlada após diálogo
nativo. **In Progress**; mutações de páginas, demais origens e aceites manuais
continuam separados. Evidências na [tasklist de conclusão](0103-tasklist-conclusao.md).

AEP-0103, seção120: condições visuais para comandos locais no Stream Deck,
com projeção pelo resolvedor comum, revalidação na UI e editor por origem.
**In Progress**; não estende o suporte visual a ações duráveis nem substitui
o aceite físico/NVDA. Evidências na tasklist de conclusão.

AEP-0103, seção119: condições visuais na paleta de apresentação local
implementadas, com aceite manual pendente. **In Progress**; não amplia a autoridade visual do backend nem
o suporte de outras origens. Evidências na tasklist de conclusão.

AEP-0103, seção118: condições físicas e ações de camada no teclado, paleta e
Stream Deck; editor por origem e seleção por nome. Regressões backend/frontend
e defaults por API passaram. **In Progress**; condições visuais nas demais
origens e aceites manuais continuam abertos na tasklist de conclusão.

AEP-0103, seção117: perfil no teclado local das abas do workspace, combinado
com tipo/aba específica, seleção por nome e ajuda contextual. **In Progress**;
evidências e aceites pendentes na tasklist de conclusão.

AEP-0103, seção116: condições por aba específica no teclado e proteção da
versão de contexto na resolução por perfil. Configuração usa títulos de abas,
preserva referências ausentes e diagnostica fatos indisponíveis por origem.
Stream Deck confere perfil por pressionamento e atualiza a apresentação sem reconectar.
**In Progress**; resultados e limites na tasklist de conclusão.

AEP-0103, seção115: ciclo manual de camadas com pin/toggle/back,
sessão/temporárias, expiração automática e proteção do mapa local no prazo.
Troca de workspace preserva sessão válida; restart encerra ativações efêmeras.
**In Progress**: aceites manuais e fatos/origens restantes continuam separados.

AEP-0103, seção114: gravação de sequências de duas etapas na configuração
de teclado local, em ambos os escopos; captura atômica, anúncios, cancelamento
e preservação de v2 durante edição. **In Progress**: aceite manual/NVDA e
demais gates permanecem abertos. Evidências na tasklist de conclusão.
Frontend completo **431 arquivos/5.181 testes**, tipos, lint e build frontend PASS.

AEP-0103, seção113: teclado contextual para ações backend/auditadas em abas
canônicas e sequências v2 com origem revalidada. App e cinco domínios PASS;
frontend 431 arquivos/5.165 testes PASS. Gravador entregue depois na seção114;
aceite manual e demais gates permanecem abertos. Evidências e
limites na [tasklist de conclusão](0103-tasklist-conclusao.md).

AEP-0103, seção112: bindings oficiais da configuração gerados e testados,
incluindo integração de ChatParams. Frontend 431 arquivos/5.154 testes,
TypeScript e contratos backend PASS. **In Progress**: aceite no aplicativo
e NVDA permanece pendente; catálogo inalterado.

AEP-0103, seção111: configuração global/workspace, edição e restauração de
camadas/bindings/regras, revisão de defaults e confirmação vinculada ao
snapshot. Publicação preserva voz/jobs; regras síncronas ficam no resolvedor
em memória. **In Progress**: limites de adapters, ciclos avançados, apresentação
do Deck e aceite físico/NVDA continuam separados. Catálogo v38 **146/61/67**;
qualificação e pendências na tasklist.

AEP-0103, seção110: perfil dinâmico de job preparado e vinculado ao grant exato,
sem redirecionamento entre confirmação e efeito. Prazo de admissão separado
do runtime somente para jobs, preservando cancelamento e deadline do chamador.
Catálogo v38 **146/61/67**, sem novos atalhos. **In Progress**: aceites físicos
e demais gates do AEP continuam separados; evidências na tasklist.

AEP-0103, seção108: reserva Windows sincronizada com a interface antes do
registro nativo, compartilhada entre raiz do app e teclado local. **In Progress**:
bindings autoritativos e executor comum de voz/jobs ainda pendentes; aceite
físico não realizado. Catálogo v36 **144/61/67** inalterado.

AEP-0103, seção107: gramática/projeção global, reserva exclusiva de combinações
e preservação do trigger no handler de jobs. **In Progress**; ingresso físico,
ownership com DOM e migração efetiva de voz/jobs ainda pendentes. Catálogo
v36 **144/61/67** inalterado; detalhes na tasklist de conclusão.

AEP-0103, seção106: backend Win32 com no-repeat e encerramento cancelável,
sem aguardar release. **In Progress**: migração `keyboard.global` e aceite
físico ainda pendentes; catálogo v36 **144/61/67** inalterado.

AEP-0103, seção105: preparação e correções de lifecycle dos ingressos de
voz/jobs. **In Progress**; migração integral para `keyboard.global` ainda
pendente, catálogo v36 **144/61/67** inalterado. Gates na
[tasklist vigente](0103-tasklist-conclusao.md).

AEP-0103, seção104: Histórico e lista no workspace integrados; Ctrl+N contextual,
abertura de tarefa, duplicação e limpeza backend com decisão/CAS. **In Progress**,
catálogo v36 **144/61/67**, aceite manual acumulado na
[tasklist vigente](0103-tasklist-conclusao.md). N/D/Ctrl+L da lista são gestos
fixos dos controles, não novos defaults remapeáveis. Sem nova porcentagem global.

AEP-0103, seção103: reconciliação documental dos ingressos residuais e das
pendências históricas. Próximo lote: Histórico e lista no workspace; depois,
hotkeys de voz/perfil e jobs. **In Progress**, catálogo v35 **142/60/66**;
percentual dos 84 critérios não recontado. Gates na
[tasklist vigente](0103-tasklist-conclusao.md#103-reconciliação-de-migração-e-ingressos-residuais--20092026).

AEP-0103, seção102: cinco mutações de perfis integradas ao executor e aos
ingressos nativos/paleta/teclado/Deck, com ownership de commit e reconstrução
do mapa após ledger terminal. **In Progress**, catálogo v35 **142/60/66**;
baseline **58 I / 24 P / 2 N** não recontada. Evidências e gates manuais na
[tasklist vigente](0103-tasklist-conclusao.md). Entradas anteriores são histórico.

AEP-0103, seção101: coordenação de arquivos/grants/epochs nos writers nativos
de perfis; migração das cinco mutações para catálogo/handoff ainda pendente.
**In Progress**, catálogo 137/60/66 e baseline 58 I / 24 P / 2 N mantidos.
Gates e evidências na [tasklist](0103-tasklist-conclusao.md).

AEP-0103, seção100, listas e sessões integradas: catálogo v34, 137 comandos/60 locais/66
defaults; mutações transacionais de listas e lifecycle de sessões terminal.
Perfis ganharam base de journal/fingerprint, mas aguardam commit coordenado
com grants/epochs antes de migrar CRUD/ativação. **In Progress**, baseline
58 I / 24 P / 2 N preservada; gates na [tasklist](0103-tasklist-conclusao.md).

AEP-0103, seção99: interrupção contextual do terminal, com snapshot de sessão
e geração gerenciada, validação do alvo visível e efeito único no backend.
Produto v33: 131 comandos/60 locais/66 defaults. Corrigido salvar edição de
lista; demais mutações ainda exigem contratos de domínio. **In Progress**,
baseline 58 I / 24 P / 2 N preservada; detalhes e gates na
[tasklist](0103-tasklist-conclusao.md#99-interrupção-do-terminal-e-salvamento-real-de-listas--19092026).

AEP-0103, seção98: nove apresentações de tarefas, perfis e terminal;
Ctrl+N contextual nas páginas, sem executar também a sequência do workspace.
Produto v32: 130 comandos/60 locais/66 defaults. CRUD e processos continuam
fora desse recorte. **In Progress**, baseline 58 I / 24 P / 2 N preservada;
gates na [tasklist de conclusão](0103-tasklist-conclusao.md#98-apresentação-de-tarefas-perfis-e-terminal--19092026).

AEP-0103, seção97: ajuda e rótulos de menus/controles do workspace, chat e
editor derivados da projeção efetiva, com remapeamento/supressão e prefixos
de sequência. Sem ampliar catálogo/defaults. AEP **In Progress**, baseline
58 I / 24 P / 2 N; validação manual acumulada pendente. Gates na
[tasklist de conclusão](0103-tasklist-conclusao.md#97-ajuda-e-rótulos-do-mapa-efetivo--19092026).

AEP-0103, seção96: sete apresentações do chat implementadas — foco,
leitura, menu, raciocínio e threads — com mensagem capturada e sem ledger.
Produto v31: catálogo121/locais51/defaults64. Automação concluída; bindings
oficiais e aceite manual pendentes. AEP **In Progress**; gates na
[tasklist de conclusão](0103-tasklist-conclusao.md#96-apresentação-e-navegação-contextual-do-chat--19092026).

AEP-0103, seção95: navegação entre regiões implementada, com ações
locais de foco, F6/Shift+F6 no mapa central e Escape contextual preservado.
Produto v30: catálogo114/locais44/defaults64. Validação automatizada concluída;
bindings oficiais e aceite manual pendentes. AEP **In Progress**; gates na
[tasklist de conclusão](0103-tasklist-conclusao.md#95-navegação-entre-regiões-da-interface--19092026).

Atualização AEP-0103, seção94: edição Mermaid implementada no fluxo de comandos,
com alvo capturado, escopo próprio do modal e execução auditada após seu
fechamento. Bindings oficiais e aceite manual pendentes. O AEP permanece
**In Progress**; evidências e gates na
[tasklist de conclusão](0103-tasklist-conclusao.md#94-edição-de-diagramas-mermaid--19092026).

AEP-0103, seção93: `chat.message.send_to_editor` envia por padrão a mensagem
inteira em Markdown a um novo documento; menus preservam recortes, formatos
e destino capturados. Sucesso aguarda aplicação real no editor. Transição
protegida, sem retry automático; resultado desconhecido pode deixar aba criada
e exige conferência. Autosave de rascunhos sem arquivo é restaurado sem texto
padrão em rascunho vazio/inexistente. Produto v28: catálogo108/locais40/bindings
padrão62. Consolidada frontend final: 1.621 testes/96 arquivos PASS (27,62 s),
TypeScript e lint PASS; caminho legado de inserção enfileirada removido.
Evidências na tasklist; bindings oficiais e aceite manual pendentes; baseline
**58 I / 24 P / 2 N** inalterada, AEP In Progress.

AEP-0103, seção92: salvar edição de mensagem usa preparação da base original
e commit atômico pelo fluxo de comandos, preservando metadados e rascunho.
Catálogo107/locais40/defaults62. Regeneração oficial e aceite manual pendentes;
evidências na tasklist. A transferência chat → editor é tratada posteriormente
na seção93, com confirmação da inserção no destino. In Progress.

AEP-0103, seção91: ações sobre mensagem selecionada implementadas e validadas,
incluindo exclusão confirmada e fixação. Abrir edição é apresentação local,
não salvamento de conteúdo. Catálogo106/locais40/defaults62; 1.257 testes
frontend PASS e App consolidado PASS. Bindings oficiais e aceite manual
pendentes. Status In Progress, sem promoção automática de critérios.

AEP-0103, seção90: implementada e validada automaticamente a migração de envio, cancelamento e
nova tentativa do chat, preservando o pipeline único da AEP-0040. Gates
na tasklist de conclusão, sem promoção automática de critérios globais.
Status In Progress; regeneração oficial dos bindings aguarda autorização
e aceite manual permanece pendente. Regressão frontend: 1.103 testes em 52 arquivos.

AEP-0103, seção89: Mensagens fixadas e Estatísticas de tokens passam pela
apresentação local contextual, compartilhada por botão, paleta, teclado
configurável e Deck. Catálogo 97, locais 39, defaults 62; sem nova tecla
padrão ou auditoria por acionamento. Aceite manual acumulado e baseline
58 I / 24 P / 2 N preservada; AEP In Progress.

AEP-0103, seção88: Limpar conversa migra para comando destrutivo confirmado
no backend, com Ctrl+L configurável, botão, paleta e Stream Deck. Captura do
alvo e comparação/limpeza atômica impedem apagar conteúdo novo durante a
confirmação. Catálogo 95, locais 37, defaults 62. Aceite manual pendente;
baseline 58 I / 24 P / 2 N preservada; AEP In Progress.

AEP-0103, seção87: seis inserções Markdown usam captura Monaco e o executor
existente; onze templates de slides migram junto com a criação pela toolbar.
Catálogo 94, locais 37, defaults 61; nenhuma nova tecla padrão. Aceite manual
acumulado, baseline 58 I / 24 P / 2 N preservada; AEP In Progress.

AEP-0103, seção86: links, tabelas com dimensões, inserção de código e Mermaid
no fluxo auditado; próxima/anterior célula em apresentação local sem escrita.
Formulários compartilhados e seleção capturada; Ctrl+K exclusivo da paleta.
Catálogo 83, locais 37, defaults 61. Aceite manual acumulado; AEP
**In Progress**, baseline global sem promoção automática.

AEP-0103, seção85: 25 ações adicionais de blocos, links e tabelas entram
no executor comum; 11 defaults migram do TipTap, com Ctrl+Alt explícito
distinto de AltGr. Catálogo 77, locais 35, defaults 61. Aceite manual
acumulado; AEP **In Progress**, sem fechamento presumido dos critérios globais.

AEP-0103, seção84: negrito, itálico e tachado no executor UI auditado,
por teclado, paleta, menu e Deck, preservando a seleção capturada.
Catálogo 52, locais 35, defaults 50. Aceite manual pendente;
AEP **In Progress**, sem promoção da baseline global.

AEP-0103, seção83: integração de `editor.file.open`, `editor.file.save` e
`editor.file.save_copy`, com Ctrl+O/S/Shift+S, paleta, menu e Deck no broker.
Catálogo 49, locais 35, defaults 47. Validação e limitações na tasklist;
aceite manual pendente, AEP **In Progress**, sem promoção da baseline global.

AEP-0103, seção81: modos Markdown, rico e visualização migrados para
`editor.mode.*`, com Alt+1/2/3, paleta, menu e Deck pelo commit durável.
Catálogo 46, locais 35, defaults 44 (40 v1 + quatro v2), 43 combinações.
Estado global preservado: 58 I / 24 P / 2 N, R07 parcial; aceite manual
pendente. Salvar/abrir arquivos continuam fora deste lote.

AEP-0103, seção80: seis comandos de apresentação do editor, incluindo
`editor.menu.insert.open`. Alt+I resolve Inserir no editor apto e Importar
fora dele, sem fallback legado; ações persistentes permanecem fora da migração.
43 comandos, 35 locais, 41 defaults (37 v1 + quatro v2) e 40 combinações.
Frontend 456/11 PASS + `tsc` e ESLint PASS; resolvedor, regressão ampliada App
(104,529 s) e `go vet` PASS; aceite manual do editor pendente;
**In Progress**, sem promoção automática dos critérios globais.

AEP-0103, seção79: cinco comandos de apresentação do editor (menus
Arquivo/Formatar/Modo, slides e fullscreen), com Alt+S/F5 configuráveis.
Registro histórico preservado; a lacuna de Alt+I foi tratada na seção80.

AEP-0103, seção78: Ctrl+M/H/P migram para os comandos de abrir seletor de
modelo, histórico e perfil do chat, por teclado/paleta/Deck. Escopo fechado
de apresentação no chat modal topmost, sem ampliar decisões ou gravar ledger.
37 comandos, 29 locais, 38 bindings. Aceite manual pendente; **In Progress**,
sem promoção dos critérios globais reconciliados abaixo.

Reconciliação AEP-0103 após seção75, registrada na seção76: **58/84 critérios
finais com implementação identificada (69,0% por critério, não por esforço nem
aceite)**; 24 parciais e 2 sem a funcionalidade prevista. **11/48 saídas R
aceitas + 5 implementadas sem aceite**, 29 parciais, 3 não implementadas;
**1/12 gates aceito (R04)**. C84 identifica a cláusula local já aprovada na
seção68, sem renumerar C01–C83. Não confundir com os 84 itens I históricos.
Estado **In Progress**; fonte única: [tasklist reconciliada](0103-tasklist-conclusao.md).

Atualização AEP-0103, seção 75 **implementada e validada automaticamente**: chat contextual por
Ctrl+Shift+I/paleta/Deck/botões no executor comum, preparação local e
criação/vínculo autenticados com compensação. Reutiliza conversa; abrir não
envia mensagens. 34 comandos, 26 locais e 35 bindings. 690 testes frontend,
tipos, lint, App ampliado, workspace e vet PASS. Evidências na tasklist;
aceite manual agrupado pendente. Status In Progress.

Atualização AEP-0103, seção 74 **implementada e validada automaticamente**: criação de workspace pelo
executor contextual, Ctrl+Shift+N/paleta/Deck/menu, persistência validada sem
trocar workspace ativo. 33 comandos, 26 locais e 34 bindings; 589 testes frontend,
tipos, lint, App amplo, workspace e vet PASS. Aceite manual em lote e
Ctrl+Shift+I permaneciam pendentes ao fechar esse lote; migração na seção 75. Status In Progress.

Atualização AEP-0103, seção 73 **implementada e validada automaticamente**: F1 no mapa efetivo, com
exceção modal exclusiva de ajuda pelo teclado local; sem bypass para Deck,
paleta ou mutações. Naquela seção: 32 comandos, 26 locais, 33 bindings; Ctrl+Shift+N/I
aguardavam os contratos duráveis identificados na tasklist. Corrigida abertura
atrasada do chat após troca de contexto. 516 testes frontend, tipos, lint,
testes App focados/amplos e vet PASS. Aceite manual pendente; In Progress.

Atualização AEP-0103, seção 72 **implementada e validada automaticamente**: Ctrl+K, Alt+E e Alt+I
pelo mapa efetivo; comandos locais de paleta e abertura dos fluxos de Dados.
Aceite manual conjunto com a seção 71 adiado pelo mantenedor para testar um
lote maior. F1 e demais atalhos contextuais não migrados neste recorte.
32 comandos, 26 locais e 32 bindings; 474 testes frontend, tipos, lint,
regressões backend focadas e vet PASS. AEP completo **In Progress**.

Atualização AEP-0103, seção 71 **implementada e validada automaticamente**: sequência Ctrl+N →
C/E/R/T pelo mapa resolvido e menu de criação como paleta especializada.
Prefixo local sem ledger, conclusão no executor contextual; v1 preservado.
29 bindings (25 v1 + quatro v2). 449 testes frontend em 14 arquivos, tipos,
lint, regressão App/commandconfig e vet PASS. Aceite manual com NVDA pendente;
sem Wails, ACP, PTY real ou banco real. AEP completo **In Progress**.

Atualização AEP-0103, seção 69 implementada e aceita manualmente: fluidez e Stream Deck
confirmados pelo mantenedor. Correção de Ctrl+K em campos nativos e substituição
da paleta por Combobox compartilhado com o picker de modelos. 314 testes
frontend, tipos e lint focado PASS. Aceite do usuário: “pode continuar.
validamos. se aparecer algo errado arrumamos.”
AEP completo **In Progress**.

Atualização AEP-0103, seção 70: `workspace.tab.terminal.create` implementado
nas três origens previstas, com admissão contextual e invalidação de owner,
sessão e workspace obsoletos. Na seção 70, o catálogo tinha 29 comandos, 23 de
apresentação local e 25 bindings (29 bindings após a seção 71).
`session_created` é deduplicado, o histórico é preservado e uma surface
existente recarrega a sessão perdida sem recriá-la. Ctrl+N continua legado:
não há migração nem novo default nesta entrega; a sequência fica para a
próxima etapa. O backend usa manager real, preserva cwd, guards de
snapshot/auth, CAS e compensação; testes fake cobrem lifecycle e erros de join.
Frontend: 317 testes em 10 arquivos, TypeScript e lint PASS; pacote `workspace`
completo PASS (1,716 s), `go vet` de App/workspace PASS e regressão App PASS
(53,621 s). Fixture de readiness com manager ausente foi corrigido. Wails, ACP,
PTY real e banco real não foram executados; só o aceite manual do novo terminal
permanece pendente.

Atualização AEP-0103, seção 68 implementada e validada automaticamente:
23 comandos de apresentação local sem invocação/auditoria por acionamento;
catálogo de 28 comandos e 25 atalhos padrão. Navegação migrada com repetição
seletiva; última seleção persistida separadamente, com proteção das mutações
dependentes. 541 testes frontend, regressões backend focadas, tipos, lint e vet
PASS. Aceite manual NVDA/Deck e fluidez pendente; AEP completo **In Progress**.

Registro histórico AEP-0103, seção 67 substituída pela seção 68: permanece a
aprovação de repetição seletiva na navegação; a tentativa de fila auditada por
acionamento foi abandonada. Os gates atuais são os da seção 68.

Atualização AEP-0103, seção 66 validada no recorte: 11 comandos de navegação de abas,
destino explícito validado no backend e ordenação dos snapshots de transporte.
28 comandos, 12 atalhos padrão; os atalhos rápidos legados permanecem enquanto
os gates de ocorrências consecutivas e latência integrada estiverem abertos.
604 testes frontend, regressões backend e checks estáticos PASS. Medição backend
p95 27,74 ms, sem IPC/renderização. AEP In Progress.

Atualização AEP-0103, seção 65: fechar aba ativa nas três origens, com
Ctrl+W/Ctrl+F4 padrão, substituição da última aba na mesma gravação e foco
guardado. 17 comandos, 12 atalhos; regressões backend e 354 testes frontend
distintos PASS. Aceite manual pendente; AEP In Progress.

Atualização AEP-0103, seção 64: editor e lista de tarefas no mesmo commit
contextual de criação de chat. 16 comandos, dez atalhos padrão; frontend
312 testes PASS, regressões backend App/workspace e verificações estáticas PASS. Terminal e sequências
não migrados; aceite manual dos novos tipos pendente. AEP In Progress.

Atualização AEP-0103, seção 63: navegação por teclado e Stream Deck em campos
nativos de texto, sem ampliar o contexto autorizado. Composição ativa e modais
permanecem bloqueantes; comandos contextuais mantêm a política estrita.
218 testes frontend, TypeScript e ESLint PASS. Aceite manual da correção
pendente; AEP In Progress.

Atualização AEP-0103, seção 62: criação de aba de chat com alvo versionado e
commit backend de uso único; Ctrl+T padrão, paleta e Stream Deck. Frontend
263 testes PASS; regressão final backend PASS (App, 45,221s), aceite manual pendente.
Demais mutações/atalhos não migrados; AEP In Progress, sem promover gates.

Atualização AEP-0103, seção 61: comando contextual de foco no painel ativo,
com preparação e revalidação local nas três origens. Sem novos atalhos padrão
nem migração de mutações assíncronas. Matriz frontend 354 PASS e validações
backend/estáticas PASS; aceite manual pendente. In Progress.

Atualização AEP-0103, seção 60: clareza dos nomes Comandos padrão e Mapa de
teclado padrão. Migração de abas pausada nas lacunas de execução assíncrona
e teclado contextual, por pedido do usuário; sem novos comandos migrados.
Status permanece In Progress; detalhes na tasklist de conclusão.

Atualização AEP-0103, seção 58: captura física substitui seleção de dispositivo
e serial manual, conforme decisão do usuário. Tecla identifica o aparelho;
captura temporária é autenticada e não executa comandos. Aceite manual dessa
nova UX pendente. AEP permanece In Progress.

Atualização AEP-0103, seção 57: aceite físico básico confirmado pelo usuário.
Descoberta somente leitura de dispositivos alimenta a seleção por modelo e
serial, preservando entrada manual; não abre HID nem ativa bindings. A seleção
nova e os cenários manuais específicos continuam pendentes. In Progress.

Atualização AEP-0103, seção 56: usuário confirmou o funcionamento da paleta,
Alt+C e configurações. Alt+M entra no catálogo e inicia-se o ingresso Stream
Deck de navegação/ajuda com bindings pessoais; aceite físico básico posterior
na seção 57. O AEP permanece In Progress, sem fechar R10 integralmente.

Atualização AEP-0103, seção 55: log de 18/09 identifica contenção SQLite nas
duas tentativas de bootstrap. Retry transacional limitado para preparar
escopo/restaurar claims, fora do gate de segurança e com revalidação da
sessão; não repete comandos. Aceite manual ainda pendente; In Progress.

Atualização AEP-0103, seção 54: teste manual ainda falhou após a seção 53.
Corrigido o bootstrap anterior à primeira observação do SO e a reconstrução
após unlock, com cancelamento e aviso à UI somente após prontidão final.
Não se presume aceite manual nem encerramento integral do AEP.

Atualização AEP-0103, seção 53: corrigida indisponibilidade dos 11 comandos
causada por projeção contextual obsoleta, sem enfraquecer os gates.
Oito combinações de navegação passam à camada padrão de teclado local;
sem fallback legado para combinações suprimidas. Aceite manual pendente.

Atualização AEP-0103, seção 52: corrigida propagação das teclas de busca
para o menu pai na paleta/pickers; seta entra no primeiro resultado, Enter
executa uma vez e espaço não aciona comandos. Abertura anuncia quantidade
total/disponível. Teste com menu e hook reais cobre os 11 comandos. Aceite
NVDA no ambiente do usuário pendente; **In Progress**, sem novo gate.

Atualização AEP-0103, seção 51: teclado local ampliado aos nove destinos de
navegação, com mapa tipado e handoff compartilhado com a paleta. Origem
continua `keyboard.local`, sem chamada de paleta nem navegação direta antes
da autorização. O usuário confirmou o primeiro atalho com camada ativa;
aceite manual dos novos destinos pendente. **In Progress**, sem novo gate.

Atualização AEP-0103, seção 50: corrigida recusa no bootstrap de fingerprints
canônicos persistidos pelas confirmações. Preservados dados/chaves e checks
de integridade; diagnóstico de storage e erro de carga da UI explicitados.
**In Progress**, sem novo aceite integral; reinício real ainda a conferir.

Atualização AEP-0103, seção 49: teclado personalizado conectado ao mapa ativo
para `workspace.list`, com picker compartilhado, deduplicação down/up e
invalidação por geração. Sem migração dos atalhos antigos; modificador
Control/Alt/Meta obrigatório, campos editáveis e modais preservados.
Bindings Wails e aceite manual pendentes. **In Progress**, sem aceite integral
de teclado global, Stream Deck ou D15. Notas anteriores são históricas.

Atualização AEP-0103, seção 48: tela passa a preparar regras e ativar/desativar
camadas globais pela origem UI autenticada. Claims persistidas alimentam a
resolução da paleta e a reconstrução do mapa. Geração Wails, validação visual
e ingresso personalizado de teclado permanecem pendentes. **In Progress**.

Atualização AEP-0103, seção 47: editor inicial de camadas globais e acionadores,
com captura, confirmação de persistência e supressão/restauração da paleta.
Bindings Wails e aceite visual ainda pendentes; teclado personalizado e
ativação de camadas pessoais não se tornam operacionais por este recorte.
Status **In Progress**, sem novo aceite integral de D15 ou BASE-PRONTA.

Atualização AEP-0103, seção 46: prioridade aprovada passa ao uso cotidiano,
adiando extensões de importação/exportação. Paleta com nove novas ações reais
de navegação, busca por metadados e confirmação preservada após mudança de tela.
Editor de atalhos e ligação produtiva física continuam pendentes.
**In Progress — 11/48 saídas R, 1/12 gates**; sem novos aceites integrais.

Atualização AEP-0103, seções 44–45: importação e exportação comum de camadas
conectadas à sessão desktop e à tela de Dados. Roundtrip público como cópia
qualificado; exportação sensível e aceite integral R05 seguem pendentes.
**In Progress — 11/48 saídas R, 1/12 gates**. Notas abaixo são históricas.

Atualização AEP-0103, seção 43: relatório redigido do lote chega ao App com
IDs reais de cópias e avisos agregados; preservado após falha de rebuild.
Corrigida substituição de camada cujo timestamp era alterado pelo ORM após
confirmação. **R05.3/R05.4 parciais; 11/48 saídas R, 1/12 gates**.
Entrada desktop autenticada e API/UX pública ainda pendentes.

Atualização AEP-0103, seção 42: lote ligado ao applier interno do App,
com reconstrução única da união ativa e conferência pós-commit de todos
os escopos alterados. Falhas de reconstrução não repetem o commit.
**R05.3/R05.4 parciais; 11/48 saídas R, 1/12 gates**. Restam transporte/UX,
relatório público e export sensível; entradas anteriores são históricas.

Atualização AEP-0103, seção 41: lote interno global+workspaces com consumo
atômico de receipts, CAS, dados e auditoria; todas as decisões antecedem
o commit. Validação da união final e recusa de efeitos tardios de hooks.
**R05.3/R05.4 parciais; 11/48 saídas R, 1/12 gates**. Falta montar o lote
no App/UI e habilitar export sensível; o applier existente continua unitário.

Atualização AEP-0103, seção 40: importação de envelope pelo applier do App,
com portas reais de posse, nomes por usuário/escopo e credenciais por pattern
exato. Compartilha confirmação, auditoria e reconstrução; sem ler segredos.
**R05.3/R05.4 parciais; 11/48 saídas R, 1/12 gates**. Transporte público,
atomicidade multi-escopo e export sensível ainda não habilitados.

Atualização AEP-0103, seção 39: adapter `job_service` do Manager real com
contexto privado por run, lifetime, definição e geração de grant vinculados.
Wiring do store de grants no App; recorte inicial de profile literal.
**R05.1/R05.2 parciais; 11/48 saídas R, 1/12 gates**. Sem novo catálogo.
Evidências na [tasklist de conclusão](0103-tasklist-conclusao.md#39-identidade-de-automação-ligada-ao-run-real--17092026).

Atualização AEP-0103, seção 38: delegação a tools preserva a origem privada
de eventos, inclusive quando selecionada por camada reativa. O runtime
revalida a origem antes do efeito, sem transformar a tool em um job fictício.
**R05.2/R03.4 parciais; 11/48 saídas R, 1/12 gates**. Evidências e limites
na [tasklist de conclusão](0103-tasklist-conclusao.md#38-origem-de-comandos-nas-tools-e-eventos-descendentes--17092026).
As notas seguintes são históricas.

Atualização AEP-0103, seção 37: montagem interna local de tools usa o executor
comum, fixando catálogo/schema/geração e revalidando autorização antes do efeito.
Confirmação, replay, redação e recusas durante o diálogo têm prova integrada.
**R05.2 parcial; 11/48 saídas R, 1/12 gates**, sem publicação no catálogo.
Evidências na [tasklist de conclusão](0103-tasklist-conclusao.md#37-delegação-local-de-tools-pelo-executor-comum--17092026).
As notas seguintes são históricas.

Atualização AEP-0103, seção 36: origem reativa verificada chega ao novo job,
sem transformar eventos em clique manual nem transmitir autorização aos
descendentes. **11/48 saídas R, 1/12 gates**, sem novos aceites.
Catálogo produtivo e ciclo automático completo permanecem pendentes;
evidências e limites na
[tasklist de conclusão](0103-tasklist-conclusao.md#36-delegação-reativa-preserva-a-raiz-verificada--17092026).
As notas abaixo são históricas.

Atualização AEP-0103, seção 35: decisões desktop reais e montagem interna de
`App.newCommandJobHandler`, com autorização por owner, sessão, profile alvo,
fingerprint e geração fixa do grant. **R05.2 parcial; 11/48 saídas R,
1/12 gates, 0/83 C, 53/84 históricos I**. A decisão do comando não concede
nem substitui grant. Sem publicação de `job.run`; catálogo R07, raiz reativa e
origens ainda não compostas permanecem pendentes. Evidências e limites na
[tasklist de conclusão](0103-tasklist-conclusao.md#35-decisão-desktop-e-autorização-de-job-pelo-app--17092026).

Atualização AEP-0103, seção 34: delegação ao runtime de jobs e preservação
da identidade nas tools. **R05.2 parcial; 11/48 saídas R, 1/12 gates**.
Sem publicação de `job.run`; composição App, raiz reativa e catálogo R07
permanecem pendentes. Evidências e limites na
[tasklist de conclusão](0103-tasklist-conclusao.md#34-ponte-de-delegação-para-jobs-e-identidade-de-tools--17092026).
As notas abaixo são históricas.

Atualização AEP-0103, seção 33: **R03.2 aceito; 11/48 saídas R, 1/12 gates**,
In Progress. Ingressos autenticados e múltiplas fontes simultâneas no App
qualificados; validação de cadeia compartilhada. R03.4 depende da delegação
R05.2 e do catálogo R07 para o ciclo completo comando → novo job → evento →
comando. Evidências, limites e ocorrência de cleanup em
[tasklist de conclusão](0103-tasklist-conclusao.md#33-ingressos-autenticados-cadeias-e-fontes-simultâneas--17092026).
As notas abaixo são históricas.

Atualização AEP-0103, seção 32: ponte job → camada → envelope implementada,
revalidação de proveniência e limite anti-loop antes da reserva; auditoria
estrutural sem payload privado. **10/48 saídas R, 1/12 gates**, In Progress.
R03.4 mantém composição completa e multiorigem no App; R03.2 mantém ingressos
pendentes. As notas abaixo são históricas.

Atualização AEP-0103, seção 31: origem privada preservada em jobs encadeados;
retenção de runs com lease corrigida e recuperação transacional qualificada.
**10/48 saídas R, 1/12 gates**, In Progress. R03.4 ainda requer proveniência
claim → envelope do comando; R03.2 mantém ingressos pendentes. Notas abaixo
são históricas.

Atualização AEP-0103, seção 30: claims de jobs integradas ao resolvedor real
com validade/condições/runtime revalidados e refresh equivalente sem cancelar
comandos. **10/48 saídas R, 1/12 gates**, In Progress; R03 permanece aberto.
As notas seguintes são históricas.

Atualização AEP-0103, seção 29: **10/48 saídas R, 1/12 gates**, In Progress.
R03.1/R03.3 aceitos: autoridade viva separada da publicação de configuração,
condições reais e isolamento de ciclos. Projeção no resolvedor e fechamento
de R03 ainda pendentes. As contagens das atualizações abaixo são históricas.

A seção 28 da [tasklist AEP-0103](0103-tasklist-conclusao.md) aceita o **Gate
R04**: restart e recuperação multiusuário/system com manutenção real, limpeza
legada, compactação e job vivo preservado até conclusão. Corrigida a fronteira
UTC/local da outbox. **Contagem vigente: 8/48 saídas R, 1/12 gates**, In Progress.
Limites e testes estão na tasklist; as notas seguintes são históricas.

A seção 27 da [tasklist AEP-0103](0103-tasklist-conclusao.md) aceita R04.3 e
R04.4: recuperação multiusuário paginada e política dinâmica por passagem,
com rollback/retomada e diagnóstico. **Contagem atual: 8/48 saídas R, 0/12
gates**, In Progress. O gate R04 ainda exige teste conjunto de restart com
todos os domínios reais e trabalho vivo; as notas seguintes são históricas.

A seção 26 da [tasklist AEP-0103](0103-tasklist-conclusao.md) aceita R04.1:
montagem produtiva do Consumer e da manutenção antes de jobs.Start, sem segundo
timer, com prova viva dos runs e join no encerramento. Contagem atual:
**6/48 saídas R, 0/12 gates**, status In Progress. As notas abaixo são históricas.

A seção 25 da [tasklist AEP-0103](0103-tasklist-conclusao.md) corrige
inicialização/cancelamento da cadência, retomada de heartbeat e propagação de
falhas de manutenção. A montagem completa no App permanece pendente;
**5/48 saídas R e 0/12 gates**, status In Progress.

A seção 24 da [tasklist AEP-0103](0103-tasklist-conclusao.md) implementa
exclusão nativa, ownership persistido v29 e recovery de gerações registradas
no bootstrap, além do encerramento antes de reset do banco. Mantém gerações
desconhecidas intactas e não repete efeitos. AEP-0103 segue In Progress;
R04 completo e BASE-PRONTA não estão certificados. Contagem vigente: **5/48
saídas R**, com R01.4/R04.2 aceitos; **0/12 gates**, pois a qualificação global
permanece pendente pelas falhas ACP/acpregistry (`0xffffffff`).

A seção 23 da [tasklist AEP-0103](0103-tasklist-conclusao.md) conecta a troca
real de workspace à reconstrução do runtime e qualifica a composição visual.
O catálogo anterior é retirado antes da reconstrução; falhas não reaproveitam
seu mapa. R01.1 foi aceito pela composição das provas reais UI/backend;
Naquela rodada R01.4 permanecia aberto: eram 3/48 saídas R, ainda 0/12 gates.

A seção 22 da [tasklist AEP-0103](0103-tasklist-conclusao.md) registra o aceite
de R01.3: ingresso autenticado, correlação, isolamento de callbacks e handoff
após ledger/CAS, com prova real de rejected_stale e replay. São 2/48 saídas R;
R01 e BASE-PRONTA permanecem abertos, e o AEP continua In Progress.

A seção 21 da [tasklist AEP-0103](0103-tasklist-conclusao.md) conecta os
providers mínimos de editor/terminal/tasklist/chat ao escopo React e à
captura contextual de Ctrl+K. R01.1 e o AEP permanecem In Progress.

A seção 20 da [tasklist AEP-0103](0103-tasklist-conclusao.md) registra a
superfície produtiva da barra e a sessão compartilhada entre seus executores.
R01.1 permanece parcial; não representa registro de todas as telas.

A seção 19 da [tasklist AEP-0103](0103-tasklist-conclusao.md) registra o aceite
de R01.2 (montagem produtiva), proteção contextual contra ABA e IME/foco.
AEP-0103 permanece In Progress; R01 e BASE-PRONTA continuam abertos.

A seção 18 da [tasklist AEP-0103](0103-tasklist-conclusao.md) registra a
resolução persistida do handoff UI e sua revalidação antes da entrega;
R01 e BASE-PRONTA ainda não têm aceite integral.

A rodada de isolamento de sequências e invalidação de callbacks da AEP-0103
está registrada na seção 16 da sua [tasklist de conclusão](0103-tasklist-conclusao.md).
É qualificação dos adapters, não aceite da montagem física no App.
A seção 17 registra as correções de lifecycle do Stream Deck e o bloqueio
de cancelamento de release na dependência; o AEP permanece **In Progress**.

| AEP | Título | Status |
|-----|--------|--------|
| [0001](0001-jobs-event-driven-automation.md) | Jobs — Event-Driven Automation | 🚧 In Progress |
| [0002](0002-tool-calling.md) | Plano de Implementação — Tool Calling | ✅ Done |
| [0003](0003-chat-tabs.md) | Sistema de Abas de Chat | 🗄️ Superseded |
| [0004](0004-chat-refactor.md) | Refatoração do Chat (Svelte) | 🗄️ Superseded |
| [0005](0005-chat-refactor-v2.md) | Refatoração do Chat v2 | 🗄️ Superseded |
| [0006](0006-chat-architecture-fix.md) | Fix Arquitetura de Chat | 🗄️ Superseded |
| [0007](0007-chat-tabs-isolation.md) | Isolamento de Abas e Conversas | 🗄️ Superseded |
| [0008](0008-tab-management-backend.md) | Gerenciamento de Abas no Backend | 🗄️ Superseded |
| [0009](0009-chat-component-refactoring.md) | Componentização do Chat | 🗄️ Superseded |
| [0010](0010-streaming-architecture.md) | Arquitetura de Streaming | 🗄️ Superseded |
| [0011](0011-thread-hierarchy.md) | Thread Hierarchy v2 | ✅ Done |
| [0012](0012-llm-provider-manager.md) | Multi-Provider LLM Architecture | 🚧 In Progress |
| [0013](0013-llm-refactor.md) | LLM Client Refactor | 🗄️ Superseded |
| [0014](0014-credential-persistence.md) | Persistência de Credenciais | ✅ Done |
| [0015](0015-provider-auto-credential.md) | Auto-extração de Credenciais | 🚧 In Progress |
| [0016](0016-http-request-tool.md) | HTTP Request Tool | 🚧 In Progress |
| [0017](0017-http-request-security.md) | HTTP Request Security | 🚧 In Progress |
| [0018](0018-http-unified-client.md) | Cliente HTTP Unificado | 🚧 In Progress |
| [0019](0019-http-client-centralization.md) | Centralização do Cliente HTTP | 🚧 In Progress |
| [0020](0020-mcp-implementation.md) | Implementação do Model Context Protocol (MCP) — núcleo e extensões | 🚧 In Progress |
| [0021](0021-mcp-native-mode.md) | MCP Modo Nativo (revisão v7) | 🚧 In Progress |
| [0022](0022-welcome-wizard.md) | Welcome Wizard | ✅ Done |
| [0023](0023-deep-links.md) | Deep Links Internos (`assistente://`) | ✅ Done |
| [0024](0024-speech-architecture.md) | Arquitetura de Voz (TTS/STT) | 🚧 In Progress |
| [0025](0025-interaction-profiles.md) | Arquitetura de Perfis de Interação por Voz (v2) | 🗄️ Superseded |
| [0026](0026-credential-fixes.md) | Correções no Sistema de Credenciais | ✅ Done |
| [0027](0027-profiles-refactor.md) | Refatoração ProfilesPage | 🚧 In Progress |
| [0028](0028-componentization.md) | Componentização Frontend | 🚧 In Progress |
| [0029](0029-auto-update.md) | Sistema de Auto-Update | ✅ Done |
| [0030](0030-email-system.md) | Sistema de Email | 📋 Open |
| [0031](0031-email-refinements-security.md) | Email + Chat Security | 📋 Open |
| [0032](0032-editor-rico.md) | Editor Rico + Inline Chat | 🚧 In Progress |
| [0033](0033-mcp-oauth-autodiscovery.md) | MCP OAuth Auto-Discovery | ✅ Done |
| [0034](0034-unified-workspace.md) | Unified Workspace | ✅ Done |
| [0035](0035-split-view.md) | Split View | 📝 Draft |
| [0036](0036-plan-tasklistmanager.md) | Task List Manager Feature | 🚧 In Progress |
| [0037](0037-sdk-migration-chat-provider.md) | SDK Migration + ChatProvider Interface | 🚧 In Progress |
| [0038](0038-voice-model-refactor.md) | Refatoração do Modelo de Voz (por Role) | ✅ Done |
| [0039](0039-tool-calling-revamp.md) | Tool Calling — Revamp & Enhancements | 🚧 In Progress |
| [0040](0040-backend-driven-messaging.md) | Backend-Driven Messaging — Desacoplamento Frontend↔Mensagens; isolamento, progresso cronológico e anúncios arbitrados | ✔️ Accepted |
| [0041](0041-proactive-tts.md) | TTS Proativo (Backend-Driven) | 🚧 In Progress |
| [0042](0042-chat-surface-context.md) | Chat Surface Context | 🚧 In Progress |
| [0043](0043-tts-stt-voices.md) | Evolução TTS/STT: Vozes (Assistant + User) | 🗄️ Superseded |
| [0044](0044-profile-settings-revamp.md) | Profile Settings Revamp (Tabbed Panels) | 🚧 In Progress |
| [0045](0045-cli-interface.md) | Interface CLI como alternativa ao Wails | 🚧 In Progress |
| [0046](0046-uuid-migration.md) | Migração de IDs Sequenciais para UUIDv7 | ✅ Done |
| [0047](0047-import-export.md) | Importação e Exportação de Conteúdo | 🚧 In Progress |
| [0048](0048-jobs-database-migration.md) | Migração de Jobs para Banco de Dados | ✅ Done |
| [0049](0049-mcp-database-migration.md) | Migração de MCP Servers para Banco de Dados | 🚧 In Progress |
| [0050](0050-profiles-database-migration.md) | Migração de Profiles para Banco de Dados (adiada) | 📝 Draft |
| [0051](0051-skills-database-migration.md) | Migração de Skills para Banco de Dados | 📝 Draft |
| [0052](0052-multi-user-accounts.md) | Sistema de Contas de Usuário | 🚧 In Progress |
| [0053](0053-mcp-graceful-degradation.md) | Degradação graciosa de MCP nativo no chat | 🚧 In Progress |
| [0056](0056-workspace-self-contained-tabs.md) | Workspace com Abas Autocontidas | ✅ Done |
| [0057](0057-chat-session-identity.md) | Sessões de Superfície e Timeline de Chat — fan-out e rascunho PR3 | ✅ Done |
| [0058](0058-global-accessibility-voice-arbitration.md) | Arbitragem Global de Acessibilidade e Voz | ✅ Done |
| [0059](0059-long-conversation-performance.md) | Performance de Conversas Longas | 🚧 In Progress — janela, timeline, detalhes lazy, contagem indexada e memoização da lista entregues; demais conteúdos pesados seguem pendentes |
| [0060](0060-command-policy-parser.md) | Parser e Política de Comandos | ✅ Done |
| [0061](0061-credential-loss-incident-and-defenses.md) | Incidente de Perda de Credenciais e Defesas | ✔️ Accepted |
| [0062](0062-profile-application-and-local-provider-auth.md) | Aplicação de Perfil e Auth de Provider Local | ✅ Done |
| [0063](0063-tool-invocations-and-common-executor.md) | Tool Invocations e Executor Comum | ✅ Done |
| [0064](0064-streaming-recovery-explicito.md) | Recuperação explícita de resposta interrompida (continuação) e cancelamento de geração | ✅ Done — recuperação, retry ancorado com leitura limitada e persistência terminal |
| [0065](0065-llm-rate-limiting.md) | Rate Limiting nas Chamadas ao Provedor LLM | ✅ Done |
| [0066](0066-connection-status-indicator.md) | Indicador de Status de Conexão com a API LLM | ✅ Done |
| [0067](0067-tasklist-domain-events-and-custom-actions.md) | Eventos de Domínio de Tasklists e Custom Actions | 🚧 In Progress |
| [0068](0068-subagentes-segundo-plano.md) | Sub-agentes em segundo plano (tool de sub-conversas) | ✅ Done |
| [0069](0069-feed-read-tool.md) | Tool `feed_read` (RSS/Atom/JSON Feed/Podcast → JSON canônico) | ✅ Done |
| [0070](0070-web-search-tool.md) | Tool `web_search` (busca web → JSON canônico paginável) | ✅ Done |
| [0071](0071-structured-tool-output-size-policy.md) | Política canônica de tamanho para saídas estruturadas | ✅ Done |
| [0072](0072-skill-catalog-and-loading.md) | Skill Loading Runtime | ✅ Done |
| [0073](0073-tasklist-conversation-linking.md) | Vínculo de Tasks e Tasklists a Conversas | ✅ Done |
| [0074-A](0074-prompt-cache-e-contexto-dinamico.md) | Prompt Cache, Custo de LLM e Layout da Request ⚠️ | 🚧 In Progress |
| [0074-B](0074-database-compaction-and-retention.md) | Compactação e Retenção do Banco de Dados ⚠️ | ✅ Done |
| [0075](0075-context-providers.md) | Context Providers | ✅ Done |
| [0076](0076-schema-versioning-migrations.md) | Versionamento de Schema do Banco (schema_migrations) | ✅ Done |
| [0077](0077-tool-planner-and-tools-subsystem-evolution.md) | ToolPlanner e Evolução do Subsistema de Tools | ✅ Done |
| [0078](0078-deprecacao-toolcalls-em-mensagens.md) | Deprecação de `tool_calls` em Mensagens | ✅ Done |
| [0079](0079-editor-modo-apresentacao-reveal.md) | Modo Apresentação Reveal.js no Editor | ✅ Done |
| [0080](0080-surface-context-unificado.md) | SurfaceContext Unificado | 🚧 In Progress |
| [0081](0081-politica-tools-por-perfil-e-carregamento-sob-demanda.md) | Política de Tools por Perfil e Carregamento sob Demanda | ✔️ Accepted |
| [0082](0082-network-trust-allowlist.md) | Network Trust Allowlist | ✅ Done |
| [0083](0083-channels-database-migration.md) | Migração de Canais e Contatos para Banco de Dados | ✅ Done |
| [0084](0084-agentes-acp-como-providers.md) | Agentes de código ACP como providers LLM | ✅ Done |
| [0085](0085-i18n-de-dialogos-do-questionnaire.md) | i18n dos diálogos que o backend manda para a tela (questionnaire) | ✅ Done |
| [0086](0086-registro-acp-descoberta-e-instalacao-de-agentes.md) | Descoberta e instalação de agentes pelo registro ACP | ✅ Done |
| [0087](0087-tela-de-erro-acessivel-e-diagnosticavel.md) | Tela de erro acessível e diagnosticável | 🚧 In Progress |
| [0088](0088-strangler-fig-borda-wails-app.md) | Concluir migração Strangler Fig da borda Wails (`App`) | ✅ Done |
| [0089](0089-terminais-como-recursos-efemeros.md) | Terminais como recursos efêmeros | 🚧 In Progress |
| [0090](0090-ordem-botoes-dialogos.md) | Ordem de botões em diálogos (confirmação antes de cancelar) | ✅ Done |
| [0091](0091-dialogos-de-decisao-unificados.md) | Diálogos de decisão unificados e ilhas documentais (Windows + NVDA) | ✅ Done |
| [0092](0092-filesystem-path-trust-allowlist.md) | Allowlist escopável de paths fora do sandbox (filesystem trust) | ✅ Done |
| [0093](0093-leitura-documentos-como-markdown.md) | Leitura unificada de documentos como Markdown (`read_file` / busca) | ✅ Done |
| [0094](0094-navegacao-em-conteudo-renderizado.md) | Navegação em conteúdo renderizado | ✅ Done |
| [0095](0095-mermaid-acessivel-e-resiliente.md) | Mermaid acessível e resiliente | ✅ Done |
| [0096](0096-baseline-operacional-de-tools-por-perfil.md) | Baseline operacional de tools por perfil | ✅ Done |
| [0097](0097-capabilities-de-protocolo-por-provedor.md) | Capabilities de protocolo configuráveis por provedor | 🚧 In Progress |
| [0098](0098-limite-de-saida-e-tool-calls-truncadas.md) | Limite de saída e tool calls truncadas | ✅ Done |
| [0099](0099-patch-canonico-multi-hunk.md) | Patch canônico multi-hunk | ✅ Done |
| [0100](0100-progresso-unificado-por-conversa.md) | Progresso unificado por conversa | ✅ Done |
| [0101](0101-profiles-descobríveis-e-delegacao-autorizada.md) | Profiles descobríveis, delegação autorizada e grants específicos de jobs | ✅ Done |
| [0102](0102-resultados-grandes-de-tools.md) | Resultados grandes de tools | ✅ Done |
| [0103](0103-comandos-acionadores-e-camadas-contextuais.md) | Comandos, acionadores e camadas contextuais | 🚧 In Progress |
| [0104](0104-tool-invocations-como-ledger-canonico.md) | Tool invocations como ledger canônico; cronologia por rodada e conclusão terminal | ✅ Done |
| [0105](0105-reautorizacao-oauth-mcp-nativo.md) | Reautorização OAuth interativa para MCP nativo | 🚧 In Progress |
| [0106](0106-contencao-do-pipeline-de-jobs.md) | Contenção do pipeline de jobs (limite de concorrência + cache por slug) | 🚧 In Progress |
| [0107](0107-tool-execution-ux.md) | Experiência de execução de tools: estado, contexto e resultados acionáveis | ✅ Done |
| [0108](0108-sessao-acp-presa-diagnostico-e-recuperacao.md) | Sessão ACP presa: diagnóstico e recuperação | 🚧 In Progress |

Correções paralelas posteriores à revisão da AEP-0103 estão registradas nas seções 10 e 11 da tasklist de conclusão: bridge, importação, restore, outbox, lifecycle e a primeira rodada real de produto. O executor desktop usa `newCommandDesktopExecutor` e `commandconfig.ProjectComplete`; autenticação é por sessão local sem JWT, a bridge é assíncrona com shutdown/join, e recovery é preflight somente leitura fail-closed sem reconciliação interprocesso. A limitação inicial de entrega somente de resumo/status foi resolvida na seção 14: `workspace.list` entrega resultado efêmero no picker compartilhado. A seção 15 liga a seleção da paleta à resolução de configuração persistida, supressão e recusa sem fallback, inclusive na bridge. A falha histórica de `internal/acpregistry` não reapareceu nas suítes completas das seções 12–14, mas sua causa não foi certificada. Providers/transporte de UI, origens físicas, recovery R04 e o aceite BASE-PRONTA permanecem pendentes.

Acompanhamento vigente da AEP-0103 (16/09/2026): [tasklist de conclusão integral](0103-tasklist-conclusao.md) e [revisão técnica](0103-revisao-integral-2026-09-16.md). A [tasklist de infraestrutura](0103-tasklist-infraestrutura.md) permanece como histórico. Os 84 itens de infraestrutura (53 marcados/31 abertos no documento histórico) são distintos dos 83 critérios finais; a contagem 81/84 foi invalidada por misturar os dois conjuntos. A revisão preserva a base testada e explicita defeitos, montagem produtiva incompleta e interfaces/migração pendentes em 12 gates rastreáveis. O AEP continua **In Progress**, sem aceite de BASE-PRONTA ou alegação de que restam apenas testes manuais; a extensão de manutenção não altera o aceite da retenção legada da AEP-0074-B.

A seção 12 registra a divisão contextual aprovada: foco/modal/IME/surface são
revalidados na UI antes do efeito visual; providers locais de backend mantêm
sessão, versões e alvos autoritativos. Não existe reader de DOM por Wails sob
DispatchGate. A seção 13 registra o handoff produtivo para `help.shortcuts.show`,
com entrega única, confirmação correlacionada e desfecho no ledger. O picker
reutiliza o painel de ajuda existente por esse percurso; não houve migração
geral dos atalhos. R01 e o aceite BASE-PRONTA permanecem abertos.

A seção 14 atualiza a limitação histórica da listagem: `workspace.list` agora
entrega DTO efêmero ao picker pelo mesmo executor, somente após sucesso
persistido e reautenticação. Histórico/replay permanecem redigidos; resposta
obsoleta não abre UI. Isso não migra a operação de troca de workspace nem
fecha o gate completo de catálogo, configuração ou BASE-PRONTA.

> **Números livres:** 0054 e 0055 estão vagos (lacunas). Novos AEPs devem ser
> numerados sequencialmente a partir do **maior número existente** (0108 → próximo
> 0109), salvo decisão explícita de reaproveitar uma lacuna.

## Status Legend

- 📝 **Draft** — em discussão/design (inclui status "Proposto" e "Rascunho")
- 📋 **Open** — aprovada, aguardando implementação
- 🚧 **In Progress** — sendo implementada
- ✅ **Done** — implementada (inclui "Concluído"/"Implementado")
- ✔️ **Accepted** — aceita como contrato/decisão vigente, mesmo sem 100%
  implementada. `Accepted` é o status canônico; `Aceito` é apenas alias
  histórico de leitura e não deve ser usado no topo de documentos novos.
- 🗄️ **Superseded** — substituída/obsoleta (mantida como registro histórico)
- ❌ **Deprecated** — desprezada/cancelada

O status descreve o estado do **escopo aceito**, não a idade do documento.
Fases explicitamente adiadas para outra issue ou outro AEP não impedem `Done`
quando os critérios de aceitação do escopo entregue estão completos. Toda
mudança de status precisa estar apoiada por evidências no documento, como
checklists, caminhos de código, testes ou PRs.

Documento principal e índice são atualizados juntos no PR que implementa,
abandona ou substitui o trabalho. Entrega parcial permanece `In Progress` e
deve declarar o que ainda falta; uma decisão vigente que funciona como contrato,
sem exigir implementação integral, usa `Accepted`.

## Convenção de numeração e anexos

Para evitar ambiguidades como as resolvidas pela issue #263:

1. **Um número, um tema.** Cada número de AEP corresponde a **um único tema**. Dois
   documentos que se autodenominam "AEP-NNNN" para temas diferentes é uma colisão
   e deve ser resolvido (renumerar o intruso ou rebaixá-lo a apêndice do tema dono
   do número). A única exceção conhecida é a colisão 0074, ainda pendente e
   explicitamente contabilizada como dois documentos principais sob um número.
2. **Documento principal vs. apêndices.** Uma série multi-arquivo tem **um documento
   principal** (`NNNN-tema.md`, listado no índice) e **apêndices** nomeados
   `NNNN-tema-<subtópico>.md` (executive-summary, metrics, fases, quick-start,
   token-management, etc.). Apêndices **não** ganham linha própria no índice; ficam
   em [Apêndices por AEP](#apêndices-por-aep).
3. **Sem espaços nem extensões fora de `.md`.** Nomes de arquivo usam apenas
   `kebab-case` minúsculo (`0036-plan-tasklistmanager.md`). Nada de espaços ou `.txt`.
4. **Numeração sequencial.** Novos AEPs seguem o maior número existente. Lacunas
   (0054, 0055) ficam reservadas/documentadas.
5. **Status no topo do documento.** Todo AEP declara o `Status` logo após o título,
   alinhado com a legenda acima e com este índice.
6. **Status faz parte da entrega.** O PR que alterar o estado de implementação
   atualiza os checklists ou evidências do AEP, o status no topo e esta tabela.

## Apêndices por AEP

Documentos secundários que **pertencem** ao mesmo tema do AEP principal:

- **0012 — Multi-Provider LLM Architecture** (principal: `0012-llm-provider-manager.md`)
  - `0012-llm-provider-token-management.md`
  - `0012-llm-provider-token-ui.md`
  - `0012-llm-provider-test-coverage.md`
  - `0012-llm-provider-phase8-validation.md`
- **0016 — HTTP Request Tool** (principal: `0016-http-request-tool.md`)
  - `0016-http-request-examples.md`
- **0020 — Implementação do Model Context Protocol (MCP) — núcleo e extensões** (principal: `0020-mcp-implementation.md`)
  - `0020-mcp-improvements-summary.md` (histórico/superseded)
- **0024 — Arquitetura de Voz (TTS/STT)** (principal: `0024-speech-architecture.md`)
  - `0024-speech-system-status.md`
  - `0024-speech-tts-refactor.md` *(ex-"AEP-0028: Speech/TTS Refactor"; renumerado para
    cá pela issue #263 — ver [colisões resolvidas](#colisões-de-numeração))*
- **0028 — Componentização Frontend** (principal: `0028-componentization.md`)
  - `0028-component-architecture.md`
  - `0028-componentization-index.md`
  - `0028-componentization-overview.md` *(ex-`.txt`; convertido pela issue #263)*
  - `0028-componentization-executive-summary.md`
  - `0028-componentization-summary.md`
  - `0028-componentization-fases.md`
  - `0028-componentization-metrics.md`
  - `0028-componentization-quick-reference.md`
  - `0028-componentization-quick-start.md`
- **0029 — Sistema de Auto-Update** (principal: `0029-auto-update.md`)
  - `0029-auto-update-github-actions.md`
  - `0029-auto-update-portable-vs-installed.md`
  - `0029-auto-update-quickstart.md`

## Colisões de numeração

### ✅ Resolvida — "AEP-0028"
Existiam dois temas sob o número 0028: a **série de Componentização Frontend** (dona
do número) e um documento de **Speech/TTS Refactor** que se autodenominava
"AEP-0028". A issue #263 rebaixou o segundo a **apêndice do AEP-0024** (tema de voz
ao qual pertence), renomeando `0028-speech-tts-refactor.md` →
`0024-speech-tts-refactor.md` e atualizando as referências textuais (ex.: AEP-0041).

### ⚠️ Pendente — "AEP-0074"
Existem **dois temas distintos** sob o número 0074:

- `0074-prompt-cache-e-contexto-dinamico.md` (**Prompt Cache** — referenciado como
  "AEP-0074" pelas AEPs 0072 e 0075);
- `0074-database-compaction-and-retention.md` (**Compactação/Retenção do Banco** —
  referenciado como "AEP-0074" por código em `internal/` e por comentários, ex.:
  `internal/database/maintenance.go`).

A renumeração de um deles exige alterar **referências em código fora de `aep/`**, o
que está fora do escopo da issue #263 (apenas governança/docs). A colisão fica
**registrada aqui** e deve ser resolvida em uma issue/PR dedicada que também atualize
as referências no código. Até lá, o índice usa os rótulos 0074-A e 0074-B e conta
ambos como documentos principais: por isso há 102 documentos para 101 números
ocupados.
