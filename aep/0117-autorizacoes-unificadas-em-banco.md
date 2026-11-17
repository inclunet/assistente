# AEP-0117 — Autorizações unificadas em banco

**Status:** Done

## Resumo

Rede, shell, filesystem e ACP usam um serviço de autorização no backend,
um modelo persistente no banco já aberto pelo aplicativo e uma experiência
comum de decisão e gestão. O serviço compartilha identidade, escopos,
precedência, consentimento, auditoria e revogação. Cada domínio mantém seus
validadores e sua semântica de recurso; uma autorização não atravessa domínios.

Este AEP substitui progressivamente a persistência em arquivos dos AEPs 0082,
0084 D9 e 0092 e concretiza o trabalho futuro de shell do AEP-0082 D8. Não muda
a seleção do banco, o ciclo de abertura dos workspaces nem a autenticação OAuth.
O contrato visual e de foco continua sendo o AEP-0091, com textos do AEP-0085.

## Motivação

- `nettrust` e `fstrust` compartilham `trustscope`, mas persistem JSONs separados.
- `allowlist` administra documentos de política de shell associados a perfis,
  enquanto a confirmação de execução não oferece os escopos comuns.
- `acptrust` grava classes de ação em arquivos por perfil e tem integração própria
  com o diálogo, a origem do pedido e a revogação.
- Várias telas e mecanismos dificultam entender o que está autorizado, em que
  contexto e por qual decisão. Erros de gravação precisam de feedback consistente.
- APIs, helpers e testes exclusivos dos caminhos substituídos devem sair junto
  com a migração, sem manter uma segunda implementação para satisfazer testes.

## Decisões

### D1. Serviço único, com adaptadores de domínio

O contrato central recebe pedido tipado com domínio, operação, recurso,
identidade autenticada, perfil efetivo, workspace de origem, conversa, invocação
e superfície. Os dados são capturados pelo backend no início da operação: trocar
de aba/perfil enquanto um diálogo está aberto não muda o destinatário da regra.
O frontend não decide política, identidade, escopo nem persistência.

O serviço resolve política e concessões, produz `allow`, `deny` ou `ask`, obtém
consentimento quando cabível e retorna efeito, motivo e escopo efetivo. A gestão identifica as regras por ID para revogação.
Erros de consulta, identidade ausente ou payload desconhecido não viram `allow`.
Bloqueios definitivos não apresentam uma opção que prometa contorná-los.

Adaptadores de rede, shell, filesystem e ACP normalizam recursos, validam regras,
avaliam correspondência e traduzem o resultado para o executor/protocolo. Não
mantêm cópias de stores, controle de sessão ou fluxos paralelos de consentimento.
Pontos de verificação junto à execução continuam necessários: decisão central
não substitui defesa no momento de abrir conexão/arquivo ou executar comando.

### D2. Cinco escopos

- `once`: só a operação apresentada; não cria regra reutilizável. O resultado
  fica vinculado à invocação, sem poder ser consumido por uma chamada concorrente.
- `session`: durante a conversa, em memória. Identidade inclui usuário, conversa
  e geração da conversa no processo. Reiniciar, limpar, excluir ou reciclar a
  conversa invalida as decisões; IDs reutilizados não herdam autorizações.
- `workspace`: persistente, somente para a identidade canônica do workspace da
  operação. O caminho serve para migração/identificação quando necessário, não
  como localização de outro banco nem como escolha feita pela interface.
- `profile`: persistente, para o perfil efetivamente usado pela operação,
  incluindo override por conversa/aba. Não existe perfil global do workspace.
- `global`: persistente, para os contextos daquele usuário no banco atual.

Todos os domínios usam este vocabulário. Somente escopos cuja identidade exista
e cuja superfície permita consentimento são oferecidos. Sem conversa não existe
`session`; ausência de workspace/perfil não é convertida silenciosamente em
`global`. Autorizações de um usuário nunca passam a outro no mesmo banco.

### D3. Precedência e limites

1. Restrições obrigatórias do domínio e `deny` explícito aplicável prevalecem.
2. Sem negativa, uma concessão compatível ou política `allow` pode liberar.
3. Sem concessão, `ask` exige consentimento; o padrão específico do domínio é
   preservado. Uma política com default deny continua negando o que não liberou.
4. Cancelamento, timeout e ausência de interlocutor quando seria preciso perguntar
   nunca são interpretados como aprovação.

Não se encerra a busca no primeiro `allow` sem verificar os `deny` aplicáveis.
Entre permissões equivalentes, a ordem de explicação é sessão, perfil, workspace,
global, com desempate estável por identidade da regra. Ordem não permite que
regra mais específica contorne proibição. `ask` não é uma proibição explícita:
uma concessão válida evita repetir a pergunta, conforme AEP-0082 D8.

Permissões temporárias e persistentes usam a mesma avaliação. Revogar ou mudar
uma política invalida caches antes de responder sucesso; pedidos ainda pendentes
reavaliam negativas e identidade antes de liberar. As regras persistentes dos escopos aplicáveis são lidas por um SELECT único após preparar todas as origens. Permissões e negativas de filesystem usam esse mesmo snapshot. A leitura é o ponto de linearização; uma revogação posterior não desfaz retroativamente a decisão. Uma operação já executada
não é desfeita pela revogação, e a UI não promete isso.

### D4. Modelo e tabelas comuns

O repositório usa as conexões, transações e migrações versionadas existentes
(AEP-0076). Nenhum banco paralelo é criado. Modelo proposto:

- `authorization_policies`: conjuntos nomeados de política, com ID, proprietário,
  nome/slug, descrição, domínio, default effect e revisão. Preserva documentos de
  shell reutilizados por vários perfis; não duplica regras para cada associação.
- `authorization_policy_bindings`: associações de política a usuário e contexto,
  incluindo o perfil que já seleciona uma allowlist e eventuais variantes de
  origem de workspace. Uma importação preserva o alcance original da associação.
- `authorization_rules`: ID, proprietário, domínio, efeito (`allow`, `deny`,
  `ask`), origem (`policy`, `consent`, `migration`), política opcional, escopo e
  chave do escopo, operação, seletor normalizado, versão do seletor, autor,
  criação/atualização, revogação e expiração opcional.
- `authorization_imports`: origem canônica, proprietário, versão/hash importado,
  estado e diagnóstico da migração. Evita reimportar regras revogadas ou apagadas.

Origens administrativas usam versão zero, usuário vazio e AssignedTo como proprietário atribuído. Elas não são marcadores de importação de um usuário; o marcador só nasce na transação que grava as regras. A atribuição exige administrador autenticado e usuário ativo, aceita apenas origens registradas pelo backend e não transfere uma origem já atribuída.

A adoção automática também grava essa titularidade transacionalmente. Marcadores
de importações anteriores fixam o dono antes de qualquer nova atribuição;
proprietários conflitantes bloqueiam a adoção.

O seletor tem schema tipado por domínio e versão, validado no backend. JSON no
banco é representação de structs conhecidos, nunca expressão executável ou
objeto arbitrário do frontend. Campos comuns de consulta ficam em colunas;
índices por usuário, domínio, escopo/chave e política evitam varrer todas as regras.
Identidade normalizada torna upsert idempotente e deduplica concessões concorrentes.

`once` e `session` não são persistidos como regras ativas. O mesmo modelo de
decisão pode ser registrado em diagnóstico sem tornar a concessão reutilizável.
Eventos de concessão, negativa, revogação e erro têm correlação e motivo usando
o logging existente; não se cria um ledger ilimitado de cada correspondência.
Comandos brutos, tokens, headers e conteúdo de arquivos não são gravados nos logs.

### D5. Recursos específicos

**Rede:** host, porta e evidências de categoria/IP continuam tipados. Preservar
portas default, wildcard que não inclui apex, revalidação de DNS/categoria,
redirecionamentos e autorização por IP:porta no transporte (AEP-0017/0082).
Uma permissão de host não libera outros destinos ou portas por efeito colateral.

**Filesystem:** caminho canônico real, operação e alcance exato/subárvore.
Preservar resolução de symlinks, regras de arquivos sensíveis, negativas dentro
do workspace e comportamento de walks do AEP-0092. Autorizar leitura não permite
escrita; permissões de origem/destino em move/copy continuam independentes.

**Shell:** usar `commandpolicy.Parse` e a avaliação de comandos compostos.
Políticas antigas mantêm seus padrões e regras estruturadas com a semântica
original. Novas concessões não guardam a linha bruta: guardam programa e uma
estrutura de operação validada, com alcance exibido antes do consentimento.
Não reduzir comandos desconhecidos a prefixos amplos. Se não for possível
representar a operação com segurança sem persistir segredos, oferecer somente
`once`. Pipelines, encadeamentos, redirecionamentos e subprocessos não podem
ganhar autorização por apenas um fragmento aprovado. Negativas existentes
continuam prevalecendo sobre concessões de runtime.

O executor recebe o diretório canônico apresentado e recusa mudança de destino
durante a pergunta. A execução efêmera restabelece esse diretório após a
inicialização do shell e não executa o comando se a mudança de diretório falhar.

**ACP:** recurso é a classe de ação declarada pelo agente e sua identidade
disponível no backend, nunca um comando inferido do título enviado pelo agente.
Não assumir que a permissão ACP prova que o aplicativo inspecionou o shell,
arquivo ou rede usados internamente pelo processo externo. Regras deste domínio
não liberam ferramentas nativas, nem o inverso.

O adaptador respeita os `optionId` oferecidos e os desfechos do protocolo: não
inventa opção positiva; cancelamento mantém a resposta negativa/cancelled própria
do método. Concessões locais revogáveis exigem resposta pontual a cada pedido,
inclusive para escopos `profile` e `global`: a memória permanece no aplicativo.
Uma opção remota `allow_always` não representa os cinco escopos locais e não é
enviada para materializar uma concessão deste serviço. Sem opção pontual, o
adaptador não oferece autorização local que prometa revogação e devolve o
desfecho negativo/cancelled do protocolo com motivo explícito. Suporte futuro a
revogação remota exige contrato e prova específicos antes de liberar esse fluxo.
Importar regras antigas não apaga autorizações que o agente já tenha memorizado:
a gestão informa esse limite e orienta revogá-las na configuração do agente,
sem afirmar que remover a regra local ou reiniciar o app necessariamente basta.
Perguntas e planos `cursor/*` continuam sendo decisões do protocolo, não regras
de execução aprendidas a partir do seu texto.

Preservar a restrição vigente: concessões persistentes ACP são criadas e usadas
somente na superfície desktop. Canais não herdam permissões concedidas no desktop;
sem interlocutor, o fluxo ACP continua negando sem esperar. Essas restrições são
capabilities do adaptador/superfície, não stores ou diálogos separados.

### D6. Consentimento e gestão

Um gerador comum de pedidos usa o roteador e o `DecisionDialog`: quem pede,
recurso/operação, motivo, alcance e botões por escopo disponível, além de negar.
IDs estáveis resolvidos no backend; rótulos traduzidos em pt-BR/en/es. Preservar
leitura integral do comando/caminho, repetição da pergunta, foco após perguntas
consecutivas e ausência de anúncios duplicados (AEP-0091).

Falha ao salvar uma autorização não pode aparecer como concessão persistente.
Uma aprovação já concedida para a operação atual pode prosseguir como `once`,
com aviso não bloqueante e log informando que não foi lembrada. Uma negativa
continua negando a operação mesmo se sua persistência falhar. Falhas ao ler
políticas não são tratadas como lista vazia, pois isso poderia apagar uma negativa.

Configurações oferece uma área **Permissões**, com filtros por domínio e escopo,
origem, recurso, efeito, data e revogação. Políticas nomeadas e suas associações
podem ser geridas ali sem perder reutilização. Sessões são identificadas como
temporárias; a listagem não finge mostrar permissões de outras conversas que não
consultou. Usar DataGrid, Toolbar e contratos de confirmação existentes.

Rede conserva a criação por consentimento sobre destino concreto; unificar a
tela não autoriza criar exceção de rede sem suas evidências. Gestão manual de
filesystem e políticas shell mantém suas validações. Exportação/importação é
explícita, validada e não cria uma segunda fonte ativa de política.

### D7. Migração e corte de arquivos

A migração é idempotente e transacional por conjunto de origem: valida o arquivo,
grava regras/associações e marcador de importação na mesma transação. Um erro
mantém a origem pendente, diagnosticada e sem concessão implícita; não impede
inicialização do chat/comandos, mas bloqueia operações dependentes da política
que não pôde ser carregada. Não marcar arquivo inválido como importado.

O corte de cada domínio remove seus escritores legados antes de disponibilizar
seus consumidores: o binário novo nunca instancia o store antigo para escrita.
Um gate exclusivo por conjunto de origem serializa snapshot, importação e
publicação do estado pronto; chamadas concorrentes aguardam esse gate ou falham
com diagnóstico, sem escrever JSON nem observar um conjunto parcialmente pronto.
Revalidar o conteúdo da origem antes do commit detecta edição durante o snapshot
e aborta a importação, preservando-a pendente. Executar versões antigas em
paralelo sobre a mesma configuração durante o upgrade não é suportado; a
exclusividade de instância do desktop continua obrigatória. Escritas externas
depois do corte são edição de backup, não alterações de política ativa, e exigem
importação explícita. A implementação testa a concorrência entre todos os
escritores suportados, incluindo CLI e integrações no processo novo.

Importar os JSONs de rede/FS, documentos de shell e autorizações ACP, preservando
escopos, efeitos, datas e variantes de resolução home/workspace. Documentos shell
que compartilham slug mas diferem por origem não podem ser fundidos. Regras ACP
antigas por perfil/classe mantêm esse alcance, identificadas como legadas; não
inventar vínculo com agente que não existia na origem. Novas concessões podem
ser mais restritas e mostram explicitamente a identidade e classe autorizadas.

Importação global ocorre somente com dono autenticado identificado. Em modo
externo/multiusuário, origem sem atribuição inequívoca não é distribuída para
todos os usuários nem entregue ao primeiro login: permanece pendente para
atribuição administrativa explícita. Workspace ainda não conhecido é importado
na primeira abertura autenticada, antes de autorizar suas operações. Não varrer
discos externos nem trocar o banco para procurar arquivos.

O conjunto aplicável é capturado por operação: rede/FS incluem global, perfil
efetivo e workspace; shell inclui a política selecionada e suas variantes de
origem; ACP inclui as regras da identidade/perfil a que o pedido pertence.
Somente após todas essas origens serem importadas (ou registradas como ausentes)
o consumidor pode usar o conjunto exclusivamente no banco. Enquanto houver
origem aplicável pendente, consultas e mutações dependentes falham fechado,
inclusive se outra origem já possuir um `allow`. Arquivos legados
permanecem como fonte de recuperação, sem exclusão automática e sem leitura de
fallback; o marcador impede ressuscitar uma autorização revogada. Nova importação
de arquivo modificado requer ação explícita, com prévia/validação. Não manter
dual-write, monitor de arquivos ou backend antigo selecionável para testes.

### D8. Limpeza e verificação

Reutilizar o que tem responsabilidade real; remover stores em arquivo, métodos,
bindings, rotas, estilos e traduções exclusivos do caminho substituído quando
seu último consumidor for migrado. Leitores legados ficam apenas na fronteira
de importação com testes de upgrades reais. Não preservar construtores ou
métodos de produção cuja única utilidade seja sustentar testes antigos.

Testes novos provam comportamento: isolamento, precedência, cancelamento,
persistência, migração, revogação e execução via adaptadores. Testes que cobrem
comportamento mantido migram junto; testes exclusivos de funcionalidade removida
saem. Falhas não são contornadas apagando cobertura relevante.

**Não executar testes ACP localmente**, incluindo testes frontend e integrações
que inicializam agentes/helpers. O CI remoto cobre ACP e seus fluxos; testes
locais do núcleo usam dados tipados e doubles sem executar processos ACP.

## Fases

A entrega tem um PR documental e dois PRs de implementação empilhados, sem
micro-PRs de infraestrutura sem consumidor. Se o inventário exigir ajustar essa
divisão, atualizar este plano sem fragmentar por arquivo ou pacote.

- [x] PR documental: contrato, escopos, banco, ACP, migração e limites definidos.
- [x] PR 1, base no PR documental: persistência unificada e migração dos quatro
  domínios, conectadas aos fluxos existentes, incluindo gestão/revogação; retirar
  os stores de arquivo substituídos. Entrega útil: decisões existentes passam a
  sobreviver no banco com isolamento e origem preservados.
- [x] PR 2, base no PR 1: serviço/orquestração comum, consentimento escopado de
  shell/ACP quando representável, área unificada de Permissões e remoção das
  interfaces/APIs redundantes. Entrega útil: autorização e gestão consistentes.

Cada PR atualiza este status, o índice e os AEPs afetados com evidências. Durante
a transição, adaptadores migrados não voltam aos arquivos; os ainda pendentes
são explicitamente enumerados. Nenhum PR é mergeado pelo agente.

### Evidências da primeira implementação

- `internal/authorization` e os modelos `AuthorizationRule`,
  `AuthorizationPolicy`, `AuthorizationPolicyBinding` e `AuthorizationImport`
  usam o banco aberto e exigem identidade de usuário. Importação, marcador e
  substituição de política são transacionais.
- `trustscope`, `allowlist` e `acptrust` substituem os escritores JSON; os
  leitores antigos existem apenas na fronteira de importação. As telas atuais
  consultam e revogam pelo banco. Bindings Wails foram regenerados para remover
  a consulta de caminhos de arquivos, sem consumidores de UI.
- Testes de `authorization`, `trustscope`, `nettrust`, `fstrust`, `allowlist` e
  `tools/shell` cobrem rollback, revogação, usuário, arquivo inválido, escopos,
  negativas e erros de consulta. O controller captura o workspace antes de
  enfileirar envio/retry; testes cobrem operação de A com interface já em B e
  canais sem herança do workspace desktop.
- ACP preserva a decisão local e devolve `allow_once`, incluindo a primeira
  resposta. Seus testes foram adaptados para contexto autenticado e banco
  isolado; execução exclusivamente no CI remoto.
- CI remoto do PR #1088 concluído com sucesso no run `44004637706`, incluindo ACP e detector de corrida. Revisão independente local concluída sem pendências; achado remoto sobre criação concorrente de vínculo corrigido com regressão que reproduz SQLITE_BUSY antes do fix e passa após reexecutar a transação inteira.
- Revisão de origem filesystem: resolução relativa, sandbox, walks e padrões de skills recebem o contexto capturado. O teste TestReadFileMantemWorkspaceDaOrigemDepoisDeTrocarATela executa leitura real após A→B e verifica que canal sem workspace não herda UI/cwd.

### Evidências da segunda implementação

- `authorization.Resolve` concentra avaliação, pergunta, revalidação, gravação e degradação explícita para uma operação. Rede, filesystem, shell e ACP o utilizam; `questionnaire.AuthorizationActions` fornece IDs e rótulos comuns.
- `shelltrust` representa operações reconhecidas sem persistir comandos livres; testes cobrem argumentos secretos/compostos, diretório exato, negativas e conversa limpa durante pergunta. O perfil é capturado antes de aguardar consentimento.
- Terminais persistentes não consultam nem criam concessões reutilizáveis: o diretório inicial não comprova o diretório após um `cd`. A regressão executa chamadas via `RunCommand`, confirma cada operação e preserva a concessão para execução efêmera.
- ACP associa novas regras a agente/classe, retorna apenas a opção pontual oferecida e preserva restrição de superfície. Novos testes de falha de gravação e isolamento entre agentes ficam para execução exclusiva no CI remoto.
- `PermissionsController` e `PermissionsPage` substituem as três telas de grants. Gestão exibe sessões de todas as conversas em memória daquele usuário, regras persistentes e revogadas; políticas e associações reutilizam os editores existentes. APIs, DTOs, bindings, estilos e traduções sem consumidor foram retirados.
- Origens ambíguas têm atribuição administrativa explícita no mesmo modelo de importações. Testes cobrem usuário não administrador, proprietário inexistente, reassignment e distinção entre atribuição e importação concluída.
- A regressão de titularidade cobre adoção automática seguida de segunda conta e marcadores anteriores ao registro administrativo, impedindo reatribuição e duplicação de concessões.
- Revisão remota da interface: recarga preserva o DataGrid e sua restauração por identidade, com fallback para a toolbar após remover a última linha. Associação navega para a rota de perfis, sem contornar guardas de comandos/modal. Regressões de foco e E2E de abertura do editor real passaram.
- Revisão remota de diretório shell: regressões de troca de symlink durante consentimento, caminho literal com aspas e falha de mudança de diretório. O teste de startup bash com `cd` passou no CI Linux (job 133817079775); o wrapper PowerShell foi exercitado localmente.
- Diagnósticos de migração usam `PermissionDiagnostic` com código, domínio e perfil, traduzidos nos três idiomas. Detalhes técnicos ficam no log; testes do controller preservam regras conhecidas junto da pendência e verificam os códigos.
- Avaliação da ressalva final do Copilot: concessões shell exigem forma lexical literal, sem colapsar aspas/escapes entre interpretadores. O wrapper de diretório preserva aliases definidos em linhas anteriores e argumentos literais, com teste Bash que verifica também o status de saída.
- Compatibilidade POSIX: a avaliação do literal usa espaço inicial em vez de `eval --`, que dash interpreta como comando. A regressão de `/bin/sh` acompanha as de Bash e PowerShell.
- Testes de UI cobrem axe, filtros, erro de leitura, pendências e confirmação de revogação. Testes de concorrência cobrem geração de sessão e snapshot consistente dos escopos. Build, vet, golangci-lint, TypeScript e ESLint passaram; Stylelint sem erros. Dez pacotes Go, 174 testes de regressão frontend e dois E2E de arquivos/foco passaram. Revisão independente concluída sem pendências. CI remoto completo aprovado no run 44016935185, incluindo ACP, startup Bash e detector de corrida. Quatro rodadas do Copilot tiveram seus achados corrigidos; a quarta pediu cobertura direta do formulário de atribuição, atendida por três testes de comportamento aprovados e revisados independentemente. A tentativa de excluir ACP na suíte frontend ampla não foi respeitada pelos projetos Vitest e incluiu testes de UI com mocks; não houve execução de agentes nem testes Go ACP. As validações locais seguintes usam arquivos explicitamente selecionados.
O formulário de atribuição tem testes reais de seletores, IDs enviados, bloqueio durante gravação, sucesso confirmado, falha com retry e cancelamento. A revisão independente foi repetida a cada lote, sem pendências ao publicar. Ordem de merge pelo mantenedor: #1087 → #1088 → #1089; nenhum merge foi realizado pelo agente.

## Riscos

- Ampliar permissões ao normalizar comandos ou migrar documentos homônimos:
  comparar comportamento antigo/novo com fixtures de upgrade e negativas.
- Vazamento entre usuários, perfis ou workspaces: capturar identidade da operação,
  exigir usuário no repositório e testar mudanças de contexto durante consentimento.
- Ressuscitar concessões após revogação: marcador transacional e ausência de fallback.
- Contenção do banco durante streaming: transações curtas, índices e sem lock de
  banco aberto enquanto espera usuário; seguir políticas do AEP-0074.
- Confundir aprovação ACP com controle do processo externo: preservar classe,
  origem, capabilities e limites do protocolo na UI e na documentação.
- Regressão de acessibilidade: testes de foco/anúncios e validação manual NVDA,
  sem afirmar que testes de DOM comprovam o resultado auditivo.

## Critérios de aceitação

- [x] Rede, shell, FS e ACP persistem no banco atual, sem dual-write/fallback.
- [x] Os cinco escopos têm identidade e duração consistentes; escopo indisponível
  não é oferecido nem convertido em outro mais amplo.
- [x] Negativas e barreiras de cada domínio prevalecem; isolamento por usuário,
  perfil, workspace, conversa e superfície é exercitado por testes.
- [x] Migração repetida/interrompida, arquivos inválidos, origens homônimas e
  revogação após importação não perdem negativas nem ampliam autorizações.
- [x] Consentimento concorrente, troca de aba/perfil e revogação durante pergunta
  não autorizam outro contexto nem ignoram uma negativa recém-criada.
- [x] ACP usa o mesmo serviço sem inventar opções, herdar permissão de desktop
  em canal ou confundir `allow_always` remoto com escopos locais.
- [x] Gestão mostra origem/escopo/efeito e revoga pelo mesmo repositório da execução.
- [x] Falhas de consulta/gravação têm erro e aviso coerentes, sem sucesso falso.
- [x] Código morto e testes exclusivos dos fluxos removidos foram eliminados;
  cobertura relevante e documentos de usuário acompanham os consumidores migrados.
- [x] Checks verdes e reviews resolvidas em cada PR; validação completa de ACP concluída no CI remoto. A exceção da execução inicial de testes de UI com mocks está registrada acima; nenhuma execução local de agentes ou testes Go ACP ocorreu.
