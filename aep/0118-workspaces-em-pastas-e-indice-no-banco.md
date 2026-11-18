# AEP-0118: Workspaces em pastas e índice no banco

Status: In Progress

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

## Fases

- [x] Repositório do índice no banco e importação idempotente do YAML antigo.
  Testes de migração, revisão concorrente e preservação de origem alterada em `internal/workspace/index_test.go`.
- [x] Resolução inicial na home e criação/importação em pasta explícita.
- [x] Interface acessível de nome e destino, usando os comandos existentes.
- [x] Preservação dos avulsos antigos, remoção e renomeação pela lista.
- [ ] Testes, documentação do usuário, revisão independente e CI.

## Entrega da interface (PR empilhado)

Criação continua no comando `workspace.create`: seletor nativo de pasta seguido
pelo formulário de nome. Cancelamento não escreve; o commit revalida sessão,
configuração e snapshot após a resposta. Importação também exige destino.
O gerenciador recusa a pasta ativa, inclusive se seu documento foi removido.

A lista usa Modal, Toolbar e DataGrid. F2 e Delete são apresentações registradas
no catálogo, disponíveis somente no modal proprietário. Renomear não troca o
workspace; Delete usa ConfirmDialog e recusa o ativo também no backend.
O filtro preserva a busca como destino de foco quando a lista fica vazia.
A remoção apaga apenas o documento e a referência, preservando banco, conversas
e demais arquivos. Não remove recursivamente pastas antigas.

Foram retirados CreateWorkspace, Manager.Create e o caminho de criação de
avulsos; fixtures usam o comando produtivo com diretório explícito. Mocks e
assertions exclusivos da API removida também saem. Não há API de produção
mantida apenas para testes.

Evidências: create_directory_test.go (destino, não sobrescrita, pasta ativa),
app_command_workspace_create_test.go (paleta/teclado/Deck e cancelamento),
WorkspaceListDialog.test.tsx (binding, confirmação, ownership, foco), além das
regressões de integração da Topbar. Revisão e CI finais ainda em andamento;
status geral permanece In Progress até a conclusão das verificações.

## Riscos

Entrega inicial: build, vet estático e lint sem issues; suíte de workspace e
testes nomeados de startup/comandos, controller e Wails API passaram. O teste
de publicação concorrente da identidade local passou em três repetições.
Revisor independente `revisao_abertura_posicional`: três rodadas, corrigidas
preservação de documento inválido, identificação da origem importada e
publicação atômica da identidade; rodada final sem pendências. ACP não foi
executado localmente. Interface de criação por pasta, Delete e F2 permanecem
pendentes para o PR empilhado; status geral continua In Progress.

Revisão remota: publicação atômica sem substituição nos três sistemas,
proteção da identidade antes de migrar, de-duplicação de referências da mesma
pasta, conexão vigente após reabertura do banco e rollback de renomeação
cobertos por `internal/workspace/index_recovery_test.go`. No Windows,
`publish_document_windows_test.go` verifica caminho maior que 300 caracteres.
Ausência de arquivo não remove referência automaticamente: não distingue um
volume desmontado de uma exclusão. A remoção explícita continua disponível.
Segunda revisão local: três rodadas do mesmo revisor independente (hardlinks,
caminhos longos, rodada final limpa). Suíte workspace e startup App passaram;
build, vet e lint do recorte sem pendências. ACP permanece exclusivo do CI.


- Banco e filesystem não compartilham uma transação: migração e criação exigem
  publicação recuperável, sem excluir a única cópia depois de falha parcial.
- Pastas movidas, arquivos inválidos e nomes/IDs conflitantes precisam de erro
  explícito, sem sobrescrita silenciosa.
- O índice pré-login não concede acesso aos conteúdos referenciados nas abas.
- A mudança de organização não comprova nem substitui o diagnóstico do bloqueio
  intermitente dos atalhos relatado em outra máquina.

## Critérios de aceitação

- [x] Primeira abertura persiste o workspace na `.assistente` da home (`TestWorkspaceStartupFirstLaunchUsesHomeDefault`).
- [x] Reabertura restaura a última pasta; abertura explícita prevalece (testes de startup do App).
- [ ] Criação e importação pela interface exigem pasta e preservam documentos existentes.
- [ ] Índice e seleção sobrevivem a reinício, com isolamento entre diretórios locais.
- [ ] Migração tolera repetição e falhas sem perda de abas ou referências.
- [ ] Não há escrita de índice YAML nem criação de novos workspaces em pastas internas.
- [ ] Limpeza remove somente legado migrado; pasta com conteúdo é preservada.
- [ ] Atalhos permanecem no catálogo/resolvedor do AEP-0103; formulários seguem AEP-0091.
- [ ] Testes ACP são executados exclusivamente no CI remoto.

Revisão remota adicional: nomes vazios são recusados antes da persistência e o
rollback da exclusão usa publicação sem substituição. Se outra instância criar
um documento no intervalo, ambos são preservados e o erro identifica o backup.
Contraprovas em `TestWorkspaceRejectsBlankNameBeforePersisting` e
`TestDeleteRollbackPreservesConcurrentDocument`. Revisor independente, sétima
rodada: sem defeitos de produção; ajuste errcheck no teste aplicado.
