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
- O índice é estado do cliente desktop, conforme AEP-0052; ele precisa estar
  disponível antes do login. Seu escopo combina a identidade local `desktop-client-id` e o diretório de configuração
  local, sem atribuir autoridade sobre conversas ou dados de outra conta.
- A importação do índice antigo é transacional e idempotente. Não manter duas
  fontes de escrita. Remover o índice antigo apenas após confirmar a importação.
- Não apagar workspaces contidos na pasta antiga. A remoção da pasta
  `workspaces` exige que esteja vazia; arquivos desconhecidos são preservados.
  Workspaces avulsos existentes permanecem onde estão, conforme decisão do
  mantenedor. Não transferir nem apagar suas pastas automaticamente.
- Ao detectar documento removido, remover sua referência do banco. Acesso
  negado, falha de volume e documento inválido não comprovam exclusão.
- Na lista, Delete pede confirmação para remover workspace não ativo; F2
  permite renomear. Integrar ao catálogo/resolvedor e preservar foco da lista.
- Falhas de leitura/gravação devem ser propagadas. Não apresentar sucesso nem
  substituir um documento ilegível por um workspace vazio.

## Fases

- [x] Repositório do índice no banco e importação idempotente do YAML antigo.
  Testes de migração, revisão concorrente e preservação de origem alterada em `internal/workspace/index_test.go`.
- [ ] Resolução inicial na home e criação/importação em pasta explícita.
- [ ] Interface acessível de nome e destino, usando os comandos existentes.
- [ ] Preservação dos avulsos antigos, remoção e renomeação pela lista.
- [ ] Testes, documentação do usuário, revisão independente e CI.

## Riscos

Entrega inicial: build, vet estático e lint sem issues; suíte de workspace e
testes nomeados de startup/comandos, controller e Wails API passaram. O teste
de publicação concorrente da identidade local passou em três repetições.
Revisor independente `revisao_abertura_posicional`: três rodadas, corrigidas
preservação de documento inválido, identificação da origem importada e
publicação atômica da identidade; rodada final sem pendências. ACP não foi
executado localmente. Interface de criação por pasta, Delete e F2 permanecem
pendentes para o PR empilhado; status geral continua In Progress.

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
