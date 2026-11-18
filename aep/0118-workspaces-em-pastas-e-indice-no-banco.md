# AEP-0118: Workspaces em pastas e índice no banco

Status: Done

## Resumo

Cada workspace vive em uma pasta escolhida pelo usuário. Seu documento permanece
em `<pasta>/.assistente/workspace.yaml`; o índice de pastas conhecidas e a última
aberta passam ao banco já selecionado pelo aplicativo. Esta decisão substitui a
organização de workspaces avulsos e do índice YAML descrita no AEP-0034.

## Motivação

A organização atual cria workspaces dentro de `~/.assistente/workspaces/`, além
dos vinculados a pastas. Isso torna pouco previsível onde o estado foi salvo.
O índice YAML também tem gravações independentes que podem falhar parcialmente.

## Decisões

- Na primeira abertura sem argumento, usar a home como pasta do workspace:
  `~/.assistente/workspace.yaml`. Se o documento já existir, preservá-lo.
- Nas seguintes, restaurar a última pasta registrada. Argumento posicional
  continua escolhendo explicitamente a pasta.
- Abrir pasta sem workspace cria o documento com o nome da pasta e uma aba de
  chat; criar pela interface exige nome e pasta. Pasta já vinculada não pode
  ter seu documento sobrescrito por uma criação.
- Importação de workspace também exige destino explícito.
- O banco mantém referências e último acesso; o YAML da pasta permanece a fonte
  de verdade para nome, abas e estado. Não mudar a seleção ou localização do banco.
- A conexão do índice acompanha a conexão vigente do aplicativo, inclusive após
  reset do banco; isso não altera a escolha do banco.
- O índice é estado do cliente desktop, conforme AEP-0052; ele precisa estar
  disponível antes do login. Seu escopo combina a identidade local `desktop-client-id` e o diretório de configuração
  local, sem atribuir autoridade sobre conversas ou dados de outra conta.
- A importação do índice antigo é transacional e idempotente. Não manter duas
  fontes de escrita. Remover o índice antigo apenas após confirmar a importação.
- Não apagar workspaces contidos na pasta antiga. A remoção da pasta
  `workspaces` exige que esteja vazia; arquivos desconhecidos são preservados.
  Workspaces avulsos existentes permanecem onde estão, conforme decisão do
  mantenedor. Não transferir nem apagar suas pastas automaticamente.
- Ausência do documento, inclusive em uma pasta acessível, não comprova exclusão:
  o volume pode estar desmontado. Preservar a referência e permitir remoção
  explícita pela lista, com confirmação; não introduzir sentinelas ou seleção de banco.
- Na lista, Delete pede confirmação para remover workspace não ativo; F2
  permite renomear. Integrar ao catálogo/resolvedor e preservar foco da lista.
- Falhas de leitura/gravação devem ser propagadas. Não apresentar sucesso nem
  substituir um documento ilegível por um workspace vazio.

- Criação recusa nomes vazios e a pasta ativa, inclusive se seu documento foi
  removido. O comando coleta pasta/nome antes da escrita e revalida o contexto.
- Depois de publicar o documento, falha no índice preserva o arquivo e informa
  seu caminho no erro. Abrir a pasta novamente recupera a referência. Não fazer
  exclusão que possa apagar uma edição/substituição de outro processo.
- A lista identifica o workspace aberto por texto localizado. Delete em campos
  de texto mantém a edição nativa. Retorno de foco exige o modal da lista no
  topo; lista vazia recebe foco na busca. Erros usam o anunciador global.

## Fases

- [x] Índice transacional no banco e importação idempotente do YAML antigo (#1090).
- [x] Primeira abertura na home e restauração da última pasta (#1090).
- [x] Criação/importação em pasta explícita, nome e revalidação do contexto (#1091).
- [x] Gestão acessível da lista, preservando avulsos antigos (#1091).
- [x] Testes, documentação, revisão independente e CI dos dois PRs.

## Riscos

- Banco e filesystem não compartilham transação. Falha após publicação mantém
  o documento recuperável e é reportada; não existe sucesso silencioso.
- Volumes desmontados e arquivos removidos podem produzir a mesma ausência.
  A lista conserva referências até remoção explícita.
- Documento inválido ou com identidade diferente é preservado, com erro.
- O índice pré-login não concede autoridade sobre conversas de outra conta.
- Esta organização não comprova a causa do bloqueio intermitente de atalhos
  relatado em outro computador. Não alterar seleção de banco para tratá-lo.
- Os testes verificam DOM, foco e contrato de comandos; não substituem o aceite
  manual com NVDA no aplicativo instalado.

## Critérios de aceitação

- [x] Primeira abertura em `~/.assistente/workspace.yaml`; restauração e argumento
  explícito cobertos pelos testes de startup do App.
- [x] Criação/importação exigem destino e preservam documento existente e pasta
  ativa: `internal/workspace/create_directory_test.go`.
- [x] Persistência e isolamento por cliente: `internal/workspace/index_test.go`.
- [x] Migração repetível, conflitos, falha transacional e origem alterada cobertos
  por `index_test.go` e `index_recovery_test.go`.
- [x] Writer do índice YAML, criação interna e APIs sem consumidores retirados;
  fixtures usam os caminhos produtivos com diretório explícito.
- [x] Limpeza somente do índice migrado idêntico; diretórios com conteúdo e links
  simbólicos preservados: `index_recovery_test.go`.
- [x] F2/Delete passam pelo catálogo/resolvedor; formulários/decisões reutilizam
  componentes padrão. `WorkspaceListDialog.test.tsx`, integração da Topbar e
  `TestCommandWorkspaceListBindingsAreScopedAndSuppressible` cobrem o percurso,
  supressão, modal proprietário, edição nativa, confirmação e retorno de foco.
- [x] Cancelamento e contexto alterado durante formulário não criam workspace:
  `app_command_workspace_create_test.go`.
- [x] Falha pós-publicação preserva edição/substituição concorrente:
  `TestCreationIndexFailurePreservesConcurrentDocument`.
- [x] Testes ACP exclusivamente no CI remoto; não executados localmente.

## Evidências de entrega

Ordem de merge: [#1090](https://github.com/inclunet/assistente/pull/1090), depois
[#1091](https://github.com/inclunet/assistente/pull/1091). Nenhum merge automático.

Build, vet e lint Go aprovados; TypeScript e ESLint aprovados; Stylelint sem
novos erros. Suíte workspace, regressões Go de comandos/controller/API, 140
integrações da paleta/Topbar, nove provas da lista, contrato Go/frontend,
auditoria de live regions e E2E específico aprovados localmente.

CI completo aprovado nos commits `96fde6a97` (#1090, execução
[44230253539](https://github.com/inclunet/assistente/actions/runs/44230253539)) e
`a26b051aa` (#1091, execução
[44232924346](https://github.com/inclunet/assistente/actions/runs/44232924346)),
incluindo ACP, race, bindings, frontend, E2E e instalador Windows.

Revisor independente `revisao_abertura_posicional`: sete rodadas no PR base e
oito na implementação da interface, com fechamento sem pendências. Onze threads
remotas do #1090 e quatro do #1091 corrigidas/respondidas e resolvidas. O commit
final de documentação também passa por revisão independente e CI.

### Segunda rodada de revisão do PR #1091

- Projeção de teclas por página/perfil aceita as superfícies `workspaces` e
  `allowlists`; fixture compartilhada compara o DTO real Go com o controlador
  frontend sem invalidar os demais atalhos.
- A lista atualiza a coleção autenticada ao abrir e usa a mesma coleção para
  ativar por ID. O indicador ativo vem do snapshot atual; falha de atualização
  retorna resultado explícito para anúncio. Removidos os callbacks de menu
  congelados e sua adaptação na Topbar.
- Ctrl+F respeita o modal proprietário, composição e evento já consumido.
  Restauração após seleção é agendada somente depois da remoção do modal,
  preservando troca assíncrona e navegação posterior.
- O handler de criação, a execução/Commit e o formulário têm janela de cinco
  minutos; o broker não expira a reserva durante o Commit.
- Falha de registro após publicar o documento emite `workspace:creation-recovery`
  tipado, efêmero e vinculado a usuário/sessão. O toast traduzido orienta reabrir
  a pasta; o caminho não é incluído no resultado persistível do comando.

Revisor independente `revisao_abertura_posicional`: duas rodadas adicionais de
implementação e uma conferência final de testes/documentação; duas pendências de foco/erro corrigidas, segunda rodada sem
pendências. A revisão de documentação anterior também foi concluída. As duas
threads e os quatro apontamentos no corpo da segunda review remota foram
tratados; validação e CI deste lote são registrados no PR.

### Casos complementares da revisão

Personalização somente por perfil (sem `app.page`) também projeta a superfície
`workspaces`; a fixture Go/frontend prova o padrão nos outros perfis e a recusa
fora da superfície. A importação utiliza o mesmo aviso efêmero de recuperação,
com usuário e sessão capturados juntos na admissão da chamada. O teste do
controller injeta falha de registro; o teste de contexto prova que trocar a
sessão do mesmo usuário antes da importação preserva o destinatário original.
Duas rodadas adicionais do revisor independente corrigiram a recaptura da
sessão; a última terminou sem pendências. O CI de `61ed0977e` passou completo
([44237718027](https://github.com/inclunet/assistente/actions/runs/44237718027));
o PR registra a validação remota deste complemento.
