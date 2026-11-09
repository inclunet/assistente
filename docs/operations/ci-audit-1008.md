# Conclusão da auditoria de testes e CI — #1008

Estado: auditoria consolidada; encerramento da issue condicionado ao merge deste
registro. Código e workflow examinados: `132a939ff` (merge do #1061).
Não há alteração de testes, workflow, permissões ou proteção da main neste recorte.
O AEP-0103 continua **In Progress**; concluir a auditoria não promove seus aceites.

O histórico, os métodos e as contraprovas de cada mudança permanecem em
[Custo de testes e CI](ci-test-cost.md). A auditoria encontrou desperdício real
em preparação, compilações, ambientes e esperas repetidas. Isso não demonstra
que a maioria dos testes seja inútil, nem permite tratar CI verde como ausência
de intermitências.

## Resultado e amostra final

Quatro runs de `pull_request`, todos na tentativa 1 e concluídos com sucesso:

- Baseline [42487226598](https://github.com/inclunet/assistente/actions/runs/42487226598):
  28 jobs; 23,10 min de parede; 158,18 min somados dos jobs; primeiro job começou
  26 s após a criação. O valor agregado foi recalculado dos timestamps em segundos
  da API; o registro inicial arredondado trazia 158,17.
- #1059 [42615717233](https://github.com/inclunet/assistente/actions/runs/42615717233):
  17 jobs; 22,65 min de parede; 88,67 min somados; primeiro job começou após 190 s.
- #1060 [42631434918](https://github.com/inclunet/assistente/actions/runs/42631434918):
  17 jobs; 14,13 min de parede; 90,73 min somados; primeiro job começou após 28 s.
- #1061 [42638825906](https://github.com/inclunet/assistente/actions/runs/42638825906):
  17 jobs; 13,52 min de parede; 88,12 min somados; primeiro job começou após 3 s.

Nos três runs finais, a mediana é 14,13 min de parede e 88,67 min somados dos
jobs. São revisões e runners diferentes, não um benchmark controlado: o run do
#1059 continua levando quase o tempo da baseline apesar de consumir muito menos
minutos de jobs. Soma de jobs não é faturamento. As medições locais pareadas
por seleção, com casos/resultados preservados, estão no histórico; não se
atribui todo o resultado a um único recorte.

No #1061, os logs de cobertura registram 9.049 testes Go top-level aprovados
em 136 pacotes. Vitest preserva 528 arquivos / 7.047 testes. E2E registra 242
passes e 6 skips preexistentes, sem flaky no resumo. A contagem de testes não é
um percentual de cobertura; exemplos, seeds, subtestes e skips devem ser
comparados por sua própria categoria. A baseline tinha 9.040 passes Go e
7.045 testes frontend; não houve redução dessas contagens para ganhar tempo.

### Preparação, compilação, execução e espera

No run final, os timestamps das etapas mostram:

- Compilação compartilhada do App com race: 28 s; publicação do binário: 5 s.
  O binário é reutilizado somente dentro do mesmo run/revisão, não como
  aprovação de outro commit.
- Etapas de execução das quatro partições do App: 329, 395, 567 e 672 s.
  Elas usam o binário já compilado. O maior grupo ainda determina o caminho
  crítico; redistribuir por duração só deve ser adotado com inventário
  determinístico e medição do custo de manutenção/setup.
- Preparação do grupo mais lento: dependências de sistema 19 s, setup Go/cache
  14 s e download do binário 4 s. Esses tempos são etapas observadas, não uma
  atribuição de cada segundo a CPU, disco ou rede.
- Etapa Go com cobertura: 479 s; os demais grupos race: 134, 150, 437 e 441 s.
  Essas etapas misturam compilação/descoberta e execução. A API de jobs não
  permite desagregá-las exatamente; não chamar tudo de tempo de teste.
- Frontend: instalação 29 s, TypeScript/Vite 51 s, Vitest 198 s. E2E: instalação
  45 s, dependências do sistema com cache do navegador 7 s, execução 412 s.
- Ambos os gates finais duraram 3 s. Seus `timeout-minutes` só limitam execução,
  não o tempo esperando começar. A diferença entre criação do run e início
  de um job inclui dependências do DAG, escalonamento e possível fila; não é
  medida isolada da fila de runners.

Reprodução: consultar `GET /repos/inclunet/assistente/actions/runs/{id}` e
`GET /repos/inclunet/assistente/actions/runs/{id}/jobs?per_page=100`, verificando
`run_attempt`, paginação e conclusão. Parede = maior `completed_at` dos jobs
menos `created_at` do run. Soma = soma de `completed_at - started_at` dos jobs;
duração de etapa = mesma diferença em `steps`. Converter para minutos somente
após somar segundos. Nos logs JSON Go, contar pares Package/Test com
`Action=pass` e sem `/` no Test; não somar novamente subtestes. Logs/artefatos
têm retenção limitada, por isso os resultados e limitações estão registrados aqui.

## Garantias e redundâncias examinadas

- Persistência, migração e portabilidade: bancos, conexões e dados continuam
  privados por cenário. `dbfixture.Open` compartilha somente bytes de schema
  vazio construído pelo bootstrap real. Provas de upgrade/migração continuam
  percorrendo migrações reais; não são substituídas por esse helper. Recortes
  #1043, #1044, #1050 e #1056, com nomes/resultados e medições no histórico.
- Configuração e execução de comandos: as fixtures contextual/Deck/catálogo
  deixaram de repetir edições confirmadas, jobs e restart como preparação.
  Edição confirmada, CAS, autorização, publicação e replay mantêm suas provas
  específicas; o mapa nominal está nas seções dos #1041, #1048 e #1054.
- Lifecycle: #1061 migra 18 chamadas de 14 testes para a montagem produtiva,
  retira duas entradas intermediárias e seu helper exclusivo, preserva os
  33 testes do arquivo e fortalece duas provas de autenticação. Contraprovas
  detectam falta da barreira de shutdown e do pós-auth do próprio Login.
- Temporização e transporte: #1055 mantém o comando realmente lento no serviço
  e prova encaminhamento do deadline na borda Wails; #1058 usa relógio controlado
  no retry OAuth mantendo manager/SQLite reais e as três variantes; #1059 leva
  provas de conexão ao serviço responsável e usa loopback para falha real de
  transporte. Não foram reduzidos prazos de produto para acelerar a suíte.
- Interface e acessibilidade: o projeto Node do Vitest contém apenas casos
  demonstrados sem DOM (#1053); componentes, teclado, foco, eventos e axe
  permanecem em jsdom/E2E. O `tsc` duplicado no mesmo job saiu; o typecheck dos
  bindings recém-gerados permanece, pois verifica outra entrada.
- Compilação, corrida e cobertura: #1042 compartilha compilação race, não
  resultados, e preserva descoberta/inventário, exemplos e seeds entre partições.
  A suíte normal com cobertura e a instrumentada com race protegem propriedades
  distintas. Compilar Windows e CLI Linux/macOS cobre destinos diferentes.
- Confiabilidade: #1051 restaura a política de conexão após compactação; #1052
  faz a fixture ABA usar aquisição real do writer; #1057 corrige classificação
  HTTP estruturada que causava retry indevido. Esses defeitos foram demonstrados
  por contraprovas, diferentemente das intermitências ainda não explicadas.

As redundâncias comprovadas foram preparação repetida, typecheck idêntico no
mesmo job, compilação do mesmo App em várias partições, espera real duplicada
em serviço/borda e um caminho produtivo mantido apenas pelos testes. Não se
classificou a repetição de uma garantia em fronteiras distintas como inútil
apenas por nomes parecidos ou pela quantidade de arquivos.

## Decisão sobre reexecução e checks

Consulta de 09/11/2026 à proteção de `main`: `strict=true`, checks obrigatórios
`backend`, `frontend`, `e2e`, `backend-race`, todos vinculados ao app 15368;
uma aprovação humana exigida e descarte de reviews obsoletos. O ruleset
`Copilot review automatico` está ativo e revisa pushes. É um retrato da
configuração consultada, não uma alteração feita por esta auditoria.

Mantemos a revalidação por head/integração e os quatro checks, inclusive em
PR documental e empilhado. O custo de árvores idênticas é real: a atualização
da pilha #1047–#1051 já produziu cinco heads novos com conteúdo idêntico, até
85 jobs. Porém, igualdade de árvore não constitui um check do novo commit na
proteção vigente. O caso histórico de dispatch verde com checks do PR ausentes
(#1048–#1051, runs `42534488570` a `42534500801`) também mostrou que validar o
conteúdo não equivale a satisfazer o check associado ao merge SHA.

Não ativamos seleção por impacto nem reaproveitamento de aprovação neste
fechamento. Para adotá-los futuramente, será preciso qualificar a identidade
da integração/base, workflow, toolchain, plataforma, dependências e contexto
relevante; recusar mudanças transversais; representar ausência de prova como
falha; e revisar explicitamente a proteção/contrato antes de mudar o significado
do verde. Isso é uma possibilidade de evolução, não uma impossibilidade técnica.
A política atual fica retida por essa lacuna de qualificação, sem acrescentar
um sistema de cache de aprovações apenas para encerrar a issue.

## Cancelamento e disponibilidade de diagnósticos

O #1060 qualifica o caso automático já demonstrado: dispatch redundante
cancela somente a si próprio, depois de comprovar CI efetivo do mesmo PR/head.
A prova real [42630695596](https://github.com/inclunet/assistente/actions/runs/42630695596)
criou somente `scripts`, sem iniciar trabalho pesado; preservou o CI do PR
`42630510631`. Não generalizar esse resultado para todos os cancelamentos.

Mantemos os gates com `always()`: eles conferem os resultados de suas
dependências e recusam falha, cancelamento ou skip. Trocar a condição para
ignorar o gate pode produzir check `skipped`, que o GitHub aceita como estado
permitido de status check obrigatório. Isso enfraqueceria a proteção pretendida.

Runs substituídos podem conservar gates em fila após cancelamento normal;
essa ocorrência foi registrada nos lotes #1057–#1059. Não adicionamos outro
autocancelador no job raiz: isso exigiria ordenar runs/reruns, validar PR/head,
resolver corridas entre consulta e cancelamento e tratar casos de mesma head
com base diferente. Acrescentaria uma dependência de API a todo CI, com ganho
não quantificado. A limitação permanece explícita e o tratamento é operacional:

1. Conferir PR, evento, head, base/integração, tentativa e existência da execução
   substituta. Não usar somente idade ou `head_sha`; rerun ou retarget pode
   preservar o mesmo SHA. Preservar a execução atual do PR e o CI da main.
2. Antes de qualquer cancelamento, coordenar com quem opera o PR para não
   iniciar reruns desse run até a confirmação do encerramento. O ID não muda
   no rerun e os endpoints de cancelamento não oferecem condição por
   `run_attempt`: reler o estado sozinho não elimina essa corrida. Sem essa
   coordenação, não solicitar cancelamento normal nem force-cancel.
   Registrar o run antigo e sua substituição; coletar diagnósticos já disponíveis.
   Revalidar identidade, tentativa e estado imediatamente antes de solicitar
   cancelamento normal apenas do run comprovadamente substituído.
3. Se ele continuar ativo, conferir os uploads Go/race após o cancelamento:
   aguardar os uploads em execução concluírem ou falharem e coletar os artefatos
   novos antes de escalar. Publicação de diagnóstico em andamento não prova
   que o run está preso. Os 2 min limitam a etapa em execução, não a espera
   para iniciá-la; upload sem progresso deve ser tratado como anomalia, sem
   prometer preservação de logs indisponíveis.
4. Mantendo a coordenação do passo 2, revalidar identidade, tentativa e estado
   imediatamente antes de usar
   `POST /repos/{owner}/{repo}/actions/runs/{run_id}/force-cancel` nesse run.
   Não cancelar em lote por branch, nem cancelar para transformar falha em verde.
5. Confirmar conclusão e conferir que a execução atual segue preservada antes
   de liberar reruns. `409` não significa sucesso por si só: reler o estado.
   Se continuar preso após as tentativas, preservar os IDs e acionar suporte
   GitHub, sem mudar gates.

Limites atuais: Go normal 15 min por processo de teste e job de 30 min;
race 20 min por processo, etapa de 23 min e job de 45 min; uploads de logs
Go/race com limite de 2 min. JSONL de cobertura e logs race têm upload em
`always()` e retenção de 7 dias; E2E mantém trace no primeiro retry e screenshot
na falha, com upload quando o workflow não foi cancelado. Force-cancel pode
interromper o próprio upload: não há promessa de recuperar artefato inexistente.
Os logs disponíveis devem ser coletados antes do encerramento forçado.

Referências: [cancelamento](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-cancellation),
[force-cancel](https://docs.github.com/en/rest/actions/workflow-runs#force-cancel-a-workflow-run),
[identidade de run e tentativa](https://docs.github.com/en/actions/reference/workflows-and-actions/variables),
[estados de checks obrigatórios](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches#require-status-checks-before-merging)
e [runs que permanecem presos](https://docs.github.com/en/actions/how-tos/troubleshoot-workflows#canceling-workflows).

## Pendências preservadas fora do encerramento

O acompanhamento e as capturas de diagnóstico estão em
[Investigação de polling e foco — #1062](intermittent-polling-focus-1062.md).

A [#1062](https://github.com/inclunet/assistente/issues/1062) reúne as ocorrências
sem causa demonstrada: polling do Deck após liberar a barreira, foco no E2E
após fechar editor e commit/foco da criação de terminal na Topbar. O retry HTTP
indevido tem correção específica no #1057 e não integra essa lista.
Uma reexecução verde ou a ausência de flaky nesta amostra não resolve a #1062.

Otimizações futuras — seleção por impacto, reuso entre heads, automação de
cancelamentos complexos e redistribuição por duração — não são pré-requisitos
para encerrar esta auditoria. Exigem nova evidência/qualificação e não autorizam
alterações silenciosas das garantias, skips ou prazos.

## Disposição dos critérios originais

- Diagnóstico reproduzível: baseline e amostra final identificadas por run,
  tentativa, job e fórmulas, com preparação/compilação/execução/espera delimitadas.
- Mapa de garantias e redundâncias: fronteiras, provas preservadas e duplicações
  demonstradas listadas acima e detalhadas por recorte no histórico.
- Plano incremental e métricas: entregas #1041–#1061, comparações pareadas e
  observações remotas; limitações de comparação explicitadas.
- Timeouts, cancelamento e diagnósticos: limites e uploads inspecionados,
  cancelamento redundante qualificado e exceções operacionais documentadas.
  Verificado não significa que o GitHub nunca deixará um run preso.
- Cobertura relevante: preservação de inventários/asserções registrada por
  recorte, duas provas fortalecidas no #1061, sem excluir casos para mascarar
  falhas. Instabilidades desconhecidas permanecem rastreadas na #1062.

O merge deste registro conclui a #1008 como auditoria com melhorias entregues
e decisões delimitadas; não declara ausência de defeitos, conclusão do AEP-0103
ou ganho garantido para toda execução futura. ACP não foi executado localmente.
