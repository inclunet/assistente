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
- **Perfil override** — cada workspace pode usar um perfil de interação diferente

## Localização dos Dados

Os workspaces são armazenados em:

- **Workspace avulso**: `~/.assistente/workspaces/<id>/workspace.yaml`
- **Workspace de diretório**: `<diretório>/.assistente/workspace.yaml`
- **Config global**: `~/.assistente/` (home directory)

## Operações

| Ação | Como |
|---|---|
| **Criar workspace** | Menu → Novo Workspace |
| **Alternar workspace** | Menu → selecione o workspace desejado |
| **Renomear** | Menu → Renomear Workspace ativo |
| **Deletar** | Menu → Deletar Workspace |

Para criar, use **Ctrl+Shift+N**, **Criar workspace** na paleta ou **Novo
workspace** no menu. Uma tecla do Stream Deck também pode receber essa ação.
Todas usam o mesmo comando protegido: a lista é atualizada, mas o workspace
ativo não muda. O atalho pode ser personalizado em Comandos e acionadores.

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
principal; Ctrl+K também abre o picker fora de campos de edição, quando não
há modal nem composição de texto em andamento.

O comando **Mostrar atalhos de teclado** (`help.shortcuts.show`) já pode ser
selecionado no picker. Ele usa o mesmo painel de ajuda existente, depois que o
backend autoriza e registra a invocação. O recebimento do pedido não significa
sucesso: o resultado só é confirmado após a abertura do painel pela UI. Se o
contexto mudar antes da abertura, o efeito é recusado. Se a confirmação se
perder, a aplicação não repete automaticamente a ação. Os atalhos antigos e
a opção de ajuda no menu mantêm seus caminhos atuais.

### Usar a paleta para navegar pelo aplicativo

O botão **Comandos**, junto ao seletor de workspace na barra compartilhada,
permite abrir Workspace, Histórico, Memórias, Listas de tarefas, Jobs, Perfis,
Configurações, Ajuda e Sobre. Esses comandos abrem as telas existentes; não
criam, apagam ou alteram seus dados. A paleta também mantém **Listar workspaces**
e **Mostrar atalhos de teclado**.

1. Abra **Comandos** pelo botão ou por **Ctrl+K**, fora de um campo de edição.
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

Ao usar Ctrl+K fora de um campo editável em um painel de chat, editor,
terminal ou tarefas, a consulta da paleta também acompanha o contexto desse
painel. Se o recurso mudar ou deixar de estar disponível durante a consulta,
a resposta antiga não abre a paleta. Você pode tentar novamente após a
mudança ou usar o botão na barra compartilhada. Ctrl+K em campos editáveis
continua reservado ao próprio campo.

## Perfil por Workspace

Ao trocar de workspace, o catálogo de comandos é reconstruído com a
configuração do destino. Comandos visuais ainda pendentes do workspace
anterior são descartados, inclusive se você voltar a ele. Se a configuração
não puder ser carregada, a troca de workspace é preservada, mas o catálogo
fica indisponível em vez de usar as configurações anteriores. Após resolver
a falha, selecionar novamente o workspace tenta reconstruir o catálogo.
Essa reconstrução não acontece na troca comum de abas (Ctrl+Tab).

Cada workspace pode ter um perfil de interação override, que define:

- Instruções de sistema customizadas
- Modelo e provedor preferido
- Parâmetros de geração (temperatura, etc.)

Isso permite, por exemplo, usar GPT-4o para trabalho e Claude para projetos pessoais, alternando apenas o workspace.
