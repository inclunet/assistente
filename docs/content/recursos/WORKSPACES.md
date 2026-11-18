---
title: "Workspaces"
weight: 1
---

# Workspaces

Workspaces permitem organizar conversas, abas de editor, terminais e listas de tarefas em espaços de trabalho separados — ideal para separar contextos como projetos, clientes ou áreas de trabalho.

## Conceito

Um workspace é um container que agrupa:

- **Abas de chat** — conversas relacionadas ao contexto
- **Abas de editor** — arquivos abertos
- **Abas de terminal** — sessões de terminal
- **Listas de tarefas** — tarefas do projeto
- **Perfil por conversa/aba** — usa o perfil padrão do aplicativo, com seleção específica quando necessário

## Abertura pelo executável

- `assistente.exe` restaura o último workspace usado. A pasta definida pelo
  atalho ou pelo atualizador não é tratada como uma escolha de workspace.
- `assistente.exe .` abre a pasta atual como workspace.
- `assistente.exe "C:\Meus projetos\Projeto"` abre a pasta indicada.

A pasta indicada precisa existir. Se ainda não houver
`.assistente/workspace.yaml`, o aplicativo cria o workspace com uma aba de chat.
Sem histórico de uso, a abertura sem argumentos cria o workspace padrão em
`~/.assistente/workspace.yaml`. Um documento existente inválido é preservado
e causa um erro explícito, sem ser substituído por um workspace vazio.
O menu **Abrir Assistente Aqui** do Windows usa a mesma sintaxe. A opção
`--workdir` não é suportada.

Se a inicialização do workspace falhar, o aplicativo informa o erro e encerra
essa abertura; não continua com uma interface sem contexto para comandos.
Os logs habilitados também recebem o erro. Se já houver uma instância usando
esse banco, a abertura apresenta a janela existente, sem trocar seu workspace.

## Localização dos Dados

Os workspaces são armazenados em:

- **Workspace padrão**: `~/.assistente/workspace.yaml`
- **Workspace avulso antigo**: `~/.assistente/workspaces/<id>/.assistente/workspace.yaml` (preservado)
- **Workspace de diretório**: `<diretório>/.assistente/workspace.yaml`
- **Config global**: `~/.assistente/` (home directory)

A lista de workspaces conhecidos e o último aberto são armazenados no banco.
O índice YAML antigo é importado uma vez e removido após confirmação; se seu
conteúdo tiver mudado desde a importação, o arquivo é preservado. A pasta
`workspaces` permanece enquanto contiver workspaces antigos ou outros arquivos.

A lista preserva referências cujo documento está ausente: uma pasta acessível
também pode ser um volume temporariamente desmontado. A remoção da referência
exige uma ação explícita; ausência e erros de acesso não apagam o histórico.
O arquivo local `desktop-client-id` identifica a instalação para separar seus
recentes dos de outro desktop; ele não concede acesso às conversas.

## Operações

| Ação | Como |
|---|---|
| **Criar workspace** | Menu → Novo Workspace |
| **Alternar workspace** | Lista de workspaces na toolbar → selecione e pressione Enter |
| **Renomear** | Lista de workspaces → F2 no item desejado |
| **Remover** | Lista de workspaces → Delete e confirmação (somente não ativo) |

Para criar, use **Ctrl+Shift+N**, **Criar workspace** na paleta ou **Novo
workspace** no menu. Uma tecla do Stream Deck também pode receber essa ação.
Todas usam o mesmo comando protegido: a lista é atualizada, mas o workspace
ativo não muda. O atalho pode ser personalizado em Comandos e acionadores.
Primeiro escolha uma pasta existente, depois informe o nome. Cancelar qualquer
etapa não cria o workspace. Uma pasta com documento existente não é sobrescrita;
a pasta do workspace ativo também não pode ser destino de uma nova criação.
A importação de YAML pede uma pasta de destino e preserva documentos existentes.

A lista tem busca por nome ou pasta. Use setas e Enter para abrir, F2 para
renomear e Delete para remover, com confirmação. A remoção não apaga conversas,
banco ou outros arquivos da pasta: remove somente o documento de workspace e
sua referência na lista. O workspace ativo precisa ser fechado pela troca para
outro antes de poder ser removido. Avulsos antigos continuam em suas pastas.
Se o documento for criado mas o registro no banco falhar, ele é preservado e
o erro informa seu caminho. Abra essa pasta novamente para recuperar a referência.


No picker de comandos, selecione **Listar workspaces** (`workspace.list`).
O comando consulta o backend pelo executor autenticado e abre o picker com
os workspaces retornados. Use as setas e Enter para escolher um workspace;
essa escolha usa a operação de troca já existente. Listar não troca o workspace
automaticamente. A resposta não contém caminhos locais e os nomes não são
gravados no histórico de comandos.

Se o foco, aba, workspace ou sessão mudar antes de a lista chegar, ela não é
aberta no novo contexto. Escape cancela a apresentação pendente, não desfaz
uma consulta já executada. Falha ou perda de resposta não causa repetição
automática. Os atalhos legados permanecem inalterados enquanto os demais
providers, transporte e fluxos de trigger/suppress são integrados.

Ao abrir o picker, o catálogo é consultado antes de exibir o menu. Se você mudar
de foco, aba, workspace ou sessão durante essa consulta, a resposta antiga é
descartada: ela não deve abrir o menu no novo contexto. Acione o botão novamente
para consultar no contexto atual. O botão sinaliza quando está carregando.
Escape cancela a abertura pendente. Use o botão da barra ou a opção no menu
principal; Ctrl+Shift+P também abre a paleta em campos de edição, quando não
há modal nem composição de texto em andamento.

O comando **Mostrar atalhos de teclado** (`help.shortcuts.show`) já pode ser
selecionado no picker. Ele usa o mesmo painel de ajuda existente, depois que o
backend autoriza e registra a invocação. O recebimento do pedido não significa
sucesso: o resultado só é confirmado após a abertura do painel pela UI. Se o
contexto mudar antes da abertura, o efeito é recusado. Se a confirmação se
perder, a aplicação não repete automaticamente a ação. Os atalhos antigos e
a opção de ajuda no menu mantêm seus caminhos atuais.

### Usar a paleta para navegar pelo aplicativo

O botão **Comandos**, na barra superior do aplicativo,
permite abrir Workspace, Histórico, Memórias, Listas de tarefas, Jobs, Perfis,
Configurações, Ajuda e Sobre. Esses comandos abrem as telas existentes; não
criam, apagam ou alteram seus dados. A paleta também mantém **Listar workspaces**
e **Mostrar atalhos de teclado**.

1. Abra **Comandos** pelo botão ou por **Ctrl+Shift+P**, inclusive em um campo de edição.
2. Digite parte do nome, descrição, categoria ou alias do comando.
3. Use as setas para selecionar e **Enter** para executar. **Escape** fecha
   sem executar e devolve o foco ao botão.

A navegação só acontece depois da autorização do backend. Comandos indisponíveis
não são executados. Mudança de contexto antes da autorização cancela o efeito;
uma navegação já realizada não é repetida se sua confirmação se perder.

Em **Configurações → Comandos e acionadores**, o [editor de camadas](../COMANDOS/)
permite inspecionar padrões, desativar/restaurar acionamentos da paleta e salvar
configurações pessoais. **Limite atual:** o teclado personalizado e a ligação
produtiva do Stream Deck continuam pendentes. Os atalhos já existentes mantêm
seu comportamento; eles não foram transferidos para a paleta nem ganharam
consultas ao backend a cada tecla.

Ao selecionar **Listar workspaces**, a paleta também respeita a configuração
de comandos carregada para sua sessão. Se esse acionamento estiver suprimido,
a lista não abre e o leitor de tela anuncia que ele está desativado. Uma
personalização pendente de revisão não é ignorada para executar o padrão.
Essa supressão pode ser alterada no editor da camada **Padrão do aplicativo**;
os atalhos de teclado existentes continuam usando seus caminhos atuais.

## Abas por Workspace

Cada workspace mantém seu próprio conjunto de abas **self-contained**: cada aba tem seu estado isolado e é preservado ao alternar workspaces, com suporte a **split view** para ver duas abas lado a lado:

- **Chat** — conversas com o assistente
- **Editor** — arquivos para edição
- **Terminal** — sessões de comando
- **Task List** — listas de tarefas

O tipo de cada aba é identificado automaticamente e restaurado ao reabrir o workspace.

### Modelo por aba de chat

Quando o perfil usa um provider HTTP nativo, a barra do chat oferece **Modelo
desta aba**. A primeira opção, **Modelo do perfil**, mantém o comportamento do
perfil; escolher outro item substitui o modelo apenas naquela aba e nos próximos
turnos.

A escolha é salva no workspace e sobrevive ao fechamento do aplicativo. Ela
não altera o perfil e não segue a conversa: se a mesma conversa for aberta em
outra aba, a outra aba usa a própria escolha. Ao trocar de perfil, o app remove
automaticamente um modelo que pertença a outro provider.

Agentes ACP têm controles próprios de modelo e modo da sessão. Para eles, use
os seletores **Modelo do agente** e **Modo do agente** na barra do chat.

## Ajuda de atalhos pela paleta

O comando `help.shortcuts.show` respeita a configuração persistida da paleta.
Se estiver suprimido ou pendente de revisão, não abre a ajuda. Se a configuração
ou a sessão mudar enquanto ele aguarda autorização, a tentativa anterior não
é entregue à interface; selecione novamente após estabilizar a sessão.
A tela de edição geral de comandos e acionadores ainda não está disponível.

Na integração de camadas com jobs, ainda em desenvolvimento, o comando
selecionado conserva a origem da automação. Históricos incompatíveis são
recusados, assim como repetição de um comando na mesma cadeia ou mais de 16
comandos encadeados. Quando o job termina, sua ativação deixa de influenciar
novas seleções. A auditoria registra identificadores da cadeia, não os
payloads privados do job. Isso ainda não disponibiliza um editor de regras.

Uma preparação visual antiga é descartada quando o usuário, a sessão ou o
workspace muda, mesmo que você volte ao anterior. Durante composição de texto
(IME), eventos de outro campo não liberam a execução no campo atual; se não
for possível confirmar o estado da composição, o comando visual é recusado.
As versões internas de contexto distinguem também mudanças de aba/perfil que
voltam ao valor anterior. Isso não altera o formato salvo do workspace.

Ao navegar para outra página enquanto a paleta carrega ou um comando aguarda,
a preparação visual anterior é descartada. Abra a paleta novamente na página
atual. Isso também vale para mudanças de parâmetros da página; uma resposta
atrasada não deve abrir a ajuda ou um picker na nova navegação.

Ao usar Ctrl+Shift+P em um painel de chat, editor,
terminal ou tarefas, a consulta da paleta também acompanha o contexto desse
painel. Se o recurso mudar ou deixar de estar disponível durante a consulta,
a resposta antiga não abre a paleta. Você pode tentar novamente após a
mudança ou usar o botão na barra superior. O atalho também funciona em campos editáveis, preservando o rascunho.

## Comandos por workspace

Ao trocar de workspace, o catálogo de comandos é reconstruído com a
configuração do destino. Comandos visuais ainda pendentes do workspace
anterior são descartados, inclusive se você voltar a ele. Se a configuração
não puder ser carregada, a troca de workspace é preservada, mas o catálogo
fica indisponível em vez de usar as configurações anteriores. Após resolver
a falha, selecionar novamente o workspace tenta reconstruir o catálogo.
Essa reconstrução não acontece na troca comum de abas (Ctrl+Tab).

## Barras e perfil padrão

A barra superior reúne o menu principal, a paleta de comandos e o título.
A lista de workspaces fica na barra do workspace, junto das ações de abas e
opções do workspace. Use as setas na toolbar, Enter para abrir a lista e
Escape para fechar e retornar ao botão.

Não existe mais um perfil global do workspace. O perfil definido como padrão
em **Perfis** é usado quando a conversa/aba não tem um perfil próprio. A seleção
individual continua disponível; trocar de workspace não troca o padrão do app.
Configurações antigas de perfil global no arquivo do workspace são ignoradas.

**Ctrl+Shift+P** é o atalho padrão da paleta, inclusive em campos de texto.
Comandos de chat indisponíveis em Configurações explicam que é necessário voltar
ao chat. O leitor de telas recebe a opção ativa, sua ajuda e seu estado pelo
controle da lista, sem repetir esse mesmo texto em um anúncio separado.
Se o documento for salvo, mas o registro na lista falhar, o aplicativo mostra
um aviso não bloqueante com o caminho preservado. Abra novamente a pasta
escolhida para recuperar o workspace. A lista é atualizada a cada abertura;
se essa atualização falhar, o aplicativo informa o problema.
