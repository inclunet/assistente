---
title: "Comandos e acionadores"
weight: 23
---

# Comandos e acionadores

Abra **Configurações → Comandos e acionadores** para inspecionar os comandos
padrão e organizar configurações pessoais em camadas. O seletor **Escopo**
separa configurações **globais** das do **workspace atual**. Na visão do
workspace, itens globais herdados continuam visíveis, mas são editados no
escopo global. Restaurar o workspace não apaga a configuração global.

## Roteiro manual único — aceite funcional acessível

Use esta checklist como ponto de entrada para o aceite manual das funções já
implementadas. Os roteiros especializados abaixo continuam servindo de
detalhamento, mas não é necessário repetir cada validação acumulada para obter
uma evidência básica. Faça a execução numa cópia descartável da configuração
local, com nomes e conteúdo sintéticos (por exemplo, `Teste AEP 0103`); não
use contas, mensagens, arquivos, listas ou workspaces reais. Não altere o
escopo global da instalação habitual. A criação de dados de teste aqui não é
migração nem teste de isolamento de workspaces.

Registre para cada item: data/versão, sistema, resultado observado e, se
falhar, passo e mensagem anunciada. Os itens básicos não exigem hardware
físico, dados reais ou credenciais; o teste físico do Stream Deck é opcional.

- [ ] **Paleta e teclado:** abra o workspace de teste, escreva `rascunho de
  teste` no campo de mensagem e pressione **Ctrl+K**. Esperado: a busca abre e
  o rascunho permanece intacto. Use **Seta para baixo** e **Seta para cima**;
  o destaque avança/recua uma opção por tecla, o foco permanece na busca e o
  NVDA anuncia a opção destacada. Busque **Listar workspaces**, pressione
  **Enter** uma vez e confira que abre o menu **Workspaces** com os nomes e
  estado ativo dos workspaces disponíveis; não deve trocar de workspace sem
  selecionar um item. Pressione **Escape** para fechar. Ao fechar a paleta,
  esperado: o foco vai ao botão **Comandos**, inclusive quando a abertura foi
  feita por Ctrl+K no campo de mensagem (não volta ao campo de origem).
  Evidência/resultado: ____________________.
- [ ] **Camada e atalho local:** em **Configurações → Comandos e acionadores**,
  crie uma camada descartável no workspace de teste. Associe **Próxima aba** a
  **Ctrl+Shift+Y** somente se a combinação estiver livre; se não estiver,
  não substitua o atalho existente e marque este item como bloqueado. Prepare
  uma regra manual de sessão, ative a camada e mantenha duas abas abertas.
  Pressione **Ctrl+Shift+Y**: esperado, a seleção muda para a próxima aba.
  Desative a regra/camada e repita: o atalho pessoal não deve mais agir.
  Restaure/remova somente os dados descartáveis criados neste passo.
  Evidência/resultado: ____________________.
- [ ] **Decisão sem efeito ao cancelar:** em **Configurações → Comandos e
  acionadores**, selecione uma camada descartável e, na grade **Acionadores**,
  abra **Ações → Editar acionador** na linha de um binding editável. Mude o
  atalho e salve; na confirmação da alteração do binding, escolha **Cancelar**.
  Esperado: o atalho anterior permanece após recarregar. Repita a edição e
  escolha **Confirmar**; esperado: o novo atalho fica visível após recarregar.
  Não use **Restaurar tudo** no escopo global. Evidência/resultado: ________.
- [ ] **Navegação e leitor de tela:** percorra a tela de comandos por teclado,
  usando **Tab** para os controles, **Setas** nos pickers/listas, **Enter**
  para selecionar e **Escape** para sair/cancelar. Com NVDA, confira nome,
  estado e disponibilidade dos controles e anúncios de resultado/erro; ao
  fechar a paleta ou diálogo, confira a restauração do foco ao acionador.
  Evidência/resultado: ____________________.
- [ ] **Stream Deck físico (opcional; exige aparelho conectado):** siga o
  procedimento em [Validação manual do Stream Deck](../../operations/streamdeck-manual-validation.md).
  Esperado: o comando de teste termina em `PASS`, renderiza o frame de teste,
  recebe a tecla física e encerra o handle. Se não houver aparelho ou acesso
  HID, marque **não executado — hardware indisponível**, sem bloquear os itens
  de teclado/paleta. Não inclua número de série na evidência compartilhada.
  Evidência/resultado: ____________________.

**Fora deste aceite:** exportação/importação com conteúdo sensível e teste de
gestos longos continuam adiados; migração e isolamento de workspaces não fazem
parte deste roteiro. A validação automatizada de **R08.2** está concluída; a
conferência manual com NVDA permanece pendente. Use o item de paleta abaixo
para registrar esse aceite manual.

**Avaliação futura — Fase 7 (não implementada):** avaliar pedais USB,
controladores MIDI e dials conforme as capacidades observáveis de cada
dispositivo. Antes de propor suporte, registrar modos de entrada (evento
discreto/contínuo), resolução/velocidade/pressão quando disponíveis,
calibração, reconexão e identidade do dispositivo, cancelamento/repetição,
feedback e operação por teclado/leitor de tela. Nenhum suporte, comando ou
atalho para esses dispositivos é declarado por esta checklist.

## Mensagens alteradas durante um comando

Se uma mensagem for alterada após preparar uma ação, a ação antiga é recusada.
Isso vale mesmo quando alguém fixa e desafixa rapidamente, ou edita e volta
ao texto anterior. Confira o estado atual e execute o comando novamente;
a confirmação anterior não autoriza agir sobre uma nova revisão da mensagem.

## Gerenciar pelo chat

Para consultar o catálogo pelo terminal, veja o grupo
[`asst commands`](../../downloads/cli/#catálogo-de-comandos-e-acionadores).
A CLI informa quais origens cada comando permite; ela não controla a janela
do aplicativo nem substitui confirmações interativas.

O agente pode consultar comandos, camadas e atalhos usando `command_catalog`
e `command_config`. Se o perfil restringe ferramentas, habilite essas duas
no perfil da conversa. O gerenciamento funciona no chat local com sessão e
cofre desbloqueados; canais externos, jobs e subagentes não podem aproveitar
a sessão do desktop para confirmar mudanças.

Exemplos de pedidos:

- “Liste minhas camadas globais e mostre os atalhos da camada Teste.”
- “Crie uma camada global chamada Revisão.”
- “Mude o atalho desta camada para Ctrl+Shift+Y e mostre a alteração.”
- “Verifique conflitos entre os atalhos deste workspace.”
- “Exporte as camadas deste workspace, sem credenciais.”

Criar, editar, habilitar, desabilitar, excluir, restaurar e importar sempre
mostram as mudanças e pedem confirmação. Cancelar não grava. Se a configuração
mudar durante a operação, o agente precisa consultá-la novamente; a decisão
anterior não autoriza sobrescrever a nova versão. Exportação pelo chat nunca
inclui credenciais, sessões ou autorizações de ativação.

Consultar um comando não significa poder executá-lo pelo chat. O catálogo
informa as origens permitidas. Neste lote, o agente executa **Listar workspaces**
e as ações **Ativar camada**, **Alternar camada** e **Voltar camada**. Ações
de interface continuam restritas aos contextos e origens já suportados.
Ativar/alternar exige uma regra manual já preparada, exibida ao consultar
a camada, e uma nova confirmação. **Voltar camada** desfaz somente a ativação
feita por aquela conversa e sessão, não a feita pelo teclado ou Stream Deck.
Habilitar uma camada, por si só, não cria uma regra de ativação.

A importação usa o mesmo formato da tela Dados e oferece manter, substituir
ou copiar, com relatório. O destino é o escopo global ou o workspace atual;
um documento não pode escolher outro destino implicitamente. Esses controles
são executados na chamada do agente, não a cada tecla de navegação.

Validação manual acumulada:

- [ ] Pedir a lista de camadas e os detalhes de uma camada real. Resultado: ____.
- [ ] Pedir para criar uma camada; negar e conferir que ela não foi criada.
  Repetir e confirmar, conferindo a camada em Configurações. Resultado: ____.
- [ ] Pedir uma alteração de atalho; conferir o diff e testar após confirmar.
  Resultado: ____.
- [ ] Pedir para ativar uma camada com regra manual preparada; negar e depois
  confirmar em outra solicitação. Conferir o mapa resultante. Resultado: ____.
- [ ] Exportar uma camada de teste e reimportar como cópia; conferir o relatório
  e a confirmação, sem alterar configurações reais inadvertidamente. Resultado: ____.

## Autenticação externa

Para instalações com autenticação por provedor externo, o
[cadastro administrativo de identidades](../../guias/EXTERNAL_IDENTITIES/)
vincula o par `(iss, sub)` do provedor a uma conta local ativa. No modo
`auth.mode=external`, o executor HTTP de comandos usa esse vínculo; não cria
uma sessão local nem transforma o JWT externo em uma sessão do aplicativo.
Para executar comandos, o provedor deve emitir o escopo `commands:execute`,
esse escopo deve estar configurado entre os escopos externos obrigatórios da
instalação, e o token externo deve declarar o papel `user` ou `admin`.
A conta local mapeada precisa estar ativa; seu papel local não substitui os
papéis nem os escopos do token.
Cada comando ainda passa pela política de origem, autorização e auditoria
própria do executor.

### API externa vinculada à interface

Esta integração permite que um cliente externo acione um conjunto limitado de
ações na interface do Assistente que o próprio usuário conectou. A conexão é
um vínculo temporário de roteamento para um destino explícito; não concede
permissões, não substitui a autenticação do comando e não empresta a sessão
desktop. O usuário precisa aprovar a conexão na interface e consumir o convite
antes de usá-la. O convite expira em **2 minutos**; a conexão ativa expira em
**45 segundos** e é renovada enquanto a interface conectada permanece ativa.
A interface precisa estar em foco e com o destino pronto no instante da ação.
Esse vínculo não controla outras aplicações nem autoriza efeitos em segundo
plano sobre uma interface sem foco.

Em **Configurações → Comandos e acionadores**, localize a seção de conexão
externa, marque o consentimento e gere o convite. O destino é capturado da
interface atual, não digitado pelo cliente externo. Copie o convite para o
cliente autorizado e consuma-o uma única vez pelo endpoint abaixo. O convite
pendente não possui cancelamento próprio: expira em dois minutos. Depois de
conectado, use **Desconectar** nessa seção para encerrar o vínculo.

A ampliação das origens permitidas faz parte da assinatura semântica dos
comandos. Se você personalizou um **atalho padrão** afetado, sua alteração pode
ficar como **Precisa de revisão**: confira-a e confirme o rebase na configuração.
Isso não apaga a personalização nem muda as teclas padrão; evita aplicar
silenciosamente uma alteração sobre um contrato que mudou.

As chamadas abaixo usam `Authorization: Bearer <token externo>` e respostas
`Cache-Control: no-store`. O token deve ser obtido e guardado pelo cliente de
acordo com o provedor; não o inclua em logs, links ou registros de comando.

- `POST /auth/external/ui-connections/consume` — consome o convite aprovado.
  Corpo: `{"invitation":"<convite>"}`. A resposta fornece `connectionId`,
  `generation`, `targetSnapshotId`, `contextVersion` e `expiresAt`.
- `GET /auth/external/ui-connections/context?connectionId=…&generation=…` —
  lê os identificadores opacos do contexto atual do vínculo autenticado, não
  o conteúdo da tela nem os dados do destino. A consulta exige exatamente
  esses dois parâmetros; a resposta contém os mesmos stamps do vínculo e o
  prazo vigente. Mudanças de destino/contexto atualizam os stamps, portanto
  o cliente deve usar o conjunto mais recente.
- `POST /commands/ui/execute` — envia a intenção de comando. O corpo contém
  `invocation_id`, `correlation_id`, `arguments` e `command_id`.
  O ingresso produtivo atual aceita seleção direta do comando: não envie
  `trigger_type`, `trigger_spec` nem `workspace_id`. O destino vinculado é
  resolvido pelo host, não por um workspace arbitrário no pedido. Para
  uma ação vinculada à interface, inclua também o conjunto completo
  `connectionId`, `generation`, `targetSnapshotId` e `contextVersion`
  devolvido pelo endpoint de contexto; os quatro campos devem ser enviados
  juntos. Omiti-los permite somente comandos compatíveis com execução sem UI:
  leitura interna/backend sem decisão, contexto ou alvo mutável. Envie apenas
  argumentos definidos pelo comando. A resposta contém
  `invocationId`, `status`, `resultSummary` e, quando disponível, `result`.
  Esse resultado completo é efêmero: não é devolvido novamente em replay.
- `GET /commands/ui/invocations/{id}` — consulta autenticada da invocação,
  sujeita ao mesmo proprietário e às permissões vigentes. Retorna
  `invocationId`, `status` e `resultSummary`, mas não recupera `result`.

Na integração atual da interface, o subconjunto que o dispatcher local aceita
é navegação por rota — `navigation.workspace.open`, `navigation.history.open`,
`navigation.memories.open`, `navigation.tasklists.open`, `navigation.jobs.open`,
`navigation.profiles.open`, `navigation.settings.open`,
`navigation.data.export.open`, `navigation.data.import.open`,
`navigation.help.open` e `navigation.about.open` — e navegação entre abas do
workspace — `workspace.tab.next`, `workspace.tab.previous` e
`workspace.tab.first` até `workspace.tab.ninth`. Essa lista descreve o
dispatcher da interface conectada, não uma autorização geral para qualquer
comando. Comandos contextuais, decisões interativas e efeitos fora desse
subconjunto não são liberados por essa integração.

O estado `succeeded` confirma que o handler local aceitou o efeito. Para
navegação, isso **não** confirma que a nova rota terminou de renderizar.
Um comando como `workspace.list` também não deve ser tratado como execução
headless multiusuário: no estado atual, sua execução depende de um runtime de
produto ativo associado ao mesmo usuário local vinculado. Sem esse runtime,
ela não está disponível; o JWT, sozinho, não cria nem seleciona um workspace.

## Repetir uma decisão aberta

Enquanto um diálogo de decisão estiver no topo, **Ctrl+Shift+R** repete a
pergunta; não confirma, não nega e não executa o comando associado à mesma
combinação em uma camada. No Windows, a reserva temporária também permite
repetir a pergunta depois de **Alt+Tab**, com outro aplicativo em foco.
Fechar a decisão retira essa reserva e devolve o atalho anterior apenas se
ele continuar configurado. Um modal superior que não seja de decisão bloqueia
a repetição da decisão inferior.

Dentro do Assistente, o atalho não repete a pergunta quando o foco está em
campos editáveis, Monaco ou durante composição de texto. Manter a tecla pressionada não repete
o anúncio. Se a interface perder a conexão, a reserva nativa expira em até
30 segundos sem renovação; nenhum comando é confirmado por essa expiração.

Validação manual do conflito com um job (use somente um job de teste sem
efeitos destrutivos):

- [ ] Configure **Ctrl+Shift+R** no job de teste. Abra uma decisão bloqueante
  no Assistente e pressione a combinação: somente a pergunta deve ser
  repetida; o job não deve iniciar. Resultado: ____.
- [ ] Com a mesma decisão aberta, use **Alt+Tab** para outro programa e
  pressione **Ctrl+Shift+R**. A pergunta deve ser repetida sem executar o job.
  Resultado: ____.
- [ ] Volte ao Assistente e encerre a decisão. Pressione **Ctrl+Shift+R**
  novamente: o job deve retomar seu fluxo normal, incluindo sua própria
  confirmação quando exigida, sem execução duplicada. Resultado: ____.

## Editar e restaurar a configuração

Atalhos globais do sistema operacional estão disponíveis somente no Windows,
onde o aplicativo coordena a captura com o teclado local e impede repetição
ao manter uma tecla pressionada. Em Linux e macOS, o registro global é
recusado até existir um adaptador com essas garantias. Isso não desabilita
o teclado local nem a paleta: configuração de escopo **global** (compartilhada
entre workspaces) não significa captura de teclado fora do aplicativo.

Selecione uma camada na lista. O detalhe separa seus comandos das regras que
determinam quando fica ativa. O menu **Ações** permite editar, habilitar,
desabilitar, restaurar e, para camadas pessoais, excluir. Habilitar uma camada
não é o mesmo que ativá-la manualmente: prepare a ativação manual e depois
use **Ativar manualmente**. Desativar manualmente remove essa ativação; outras
regras elegíveis da mesma camada continuam valendo.

É possível personalizar um comando padrão sem modificar o padrão distribuído
pelo aplicativo. **Restaurar** remove a personalização. A restauração também
está disponível por camada e para todo o escopo selecionado. Antes de gravar,
o aplicativo mostra as mudanças e pede confirmação. Cancelar não grava nada.
Se a configuração mudar enquanto o editor ou a confirmação estiver aberto,
recarregue e revise: uma confirmação antiga não autoriza sobrescrever a nova.

As regras permitem ativação manual, sempre ativa, por condição e por evento
interno de jobs. Regra de evento é criada desativada; habilitá-la exige uma
decisão específica de autorização. Para editar uma regra de evento já ativa,
desabilite-a primeiro e confirme uma nova autorização depois da edição.
Se duas confirmações tentarem autorizar a mesma regra simultaneamente, apenas
uma pode prevalecer; a outra deve ser revista sobre a configuração atual.
A autorização e a alteração da regra são gravadas juntas, sem aplicação parcial.

### Ativação manual e duração

Na lista de **Regras**, uma regra manual ou de alternância pode durar:

- **Persistente:** permanece após reiniciar, desde que a regra e a origem continuem válidas.
- **Sessão:** termina no reinício ou na troca da sessão autenticada; trocar de workspace não é reiniciar.
- **Temporária:** informe a duração ao ativar, entre 1 e 86400 segundos. Termina no prazo ou no reinício, o que ocorrer primeiro.

As ações da regra permitem **Fixar camada**, alternar sua ativação e
**Desativar camada**. Fixar uma ativação que já existe não renova seu prazo.
**Voltar à última ativação** desfaz a ativação manual mais recente do escopo
selecionado, não necessariamente da camada aberta. Não apaga regras e não
encerra ativações de outra origem ou outro workspace. Cada regra é independente:
desativar uma regra não desativa a camada se outra ainda a mantiver ativa.

A lista mostra a ativação manual e o horário de término das temporárias.
A expiração funciona com a tela de configurações fechada. O teclado e a
paleta deixam de usar o mapa vencido mesmo que sua atualização atrase.
Editar a configuração não reinicia a contagem; habilitar novamente uma camada
não recupera uma ativação que já terminou.

Para validar este ciclo em uma camada de teste, associe **Próxima aba** a
**Ctrl+Shift+Y** e mantenha duas abas abertas:

- [ ] Crie uma regra manual de sessão; fixe-a e confira que Ctrl+Shift+Y troca de aba. Resultado: ____.
- [ ] Troque de workspace e volte; a ativação de sessão deve continuar. Resultado: ____.
- [ ] Desative a regra; a combinação pessoal deve parar, sem afetar os atalhos padrão. Resultado: ____.
- [ ] Use uma regra temporária de 15 segundos; ative e feche as configurações. O atalho deve funcionar antes do prazo e parar depois. Resultado: ____.
- [ ] Ative duas camadas no mesmo escopo; use **Voltar à última ativação** e confira que somente a mais recente é retirada. Resultado: ____.
- [ ] Reinicie com uma ativação persistente e uma de sessão/temporária; somente a persistente válida deve retornar. Resultado: ____.
- [ ] Restaure a camada de teste; confira que seus padrões permanecem disponíveis. Resultado: ____.

Condições são conjunções de igualdades: todas as cláusulas de uma regra devem
ser verdadeiras. Regras diferentes da mesma camada são alternativas. No
teclado local, foco do aplicativo, tipo de superfície, aba específica e perfil são os fatos disponíveis;
a seleção por superfície atende combinações simples e sequências de duas
teclas. Nas abas de chat, editor, terminal e lista de tarefas, os comandos
auditados elegíveis também podem usar essa condição. Ações de navegação
continuam locais; efeitos persistentes mantêm autorização e confirmação.
Uma sequência iniciada em uma aba não é transferida para outra: mudança de
contexto cancela a continuação. Páginas fora dessas abas e fatos adicionais
ainda podem produzir diagnóstico de configuração não executável.
**Aba de lista de tarefas** é a aba aberta no workspace; **Listas de tarefas**
é a página de gerenciamento. Não são o mesmo contexto. O gravador do teclado
local permite combinações simples e sequências de duas etapas, tanto na
configuração global quanto na do workspace.
Regras de eventos de jobs podem
usar o perfil efetivo; não recebem foco ou superfície presumidos do evento.
Observe os diagnósticos: configuração persistida não garante que um acionador
esteja disponível naquele contexto.

A condição de perfil também é conferida pela paleta para comandos executados
no backend ou com entrega auditada, pelos atalhos globais e pelo Stream Deck
para ações auditadas ou de alteração do workspace. O perfil efetivo é o da
aba, quando ela tem uma substituição, ou o do workspace. Alterar o perfil não
exige salvar a camada novamente. Uma solicitação preparada antes da troca não
é reativada ao voltar ao perfil anterior. Ações estritamente locais da paleta
e do Stream Deck ainda não aceitam essa condição; a configuração informa essa
limitação. No teclado local, perfil funciona nas abas de chat, editor, terminal
e lista de tarefas do workspace, inclusive para navegação estritamente local.
Pode ser combinado com tipo de superfície e aba específica. As páginas de
configurações e gerenciamento fora dessas abas não herdam esse contexto.
Condições visuais em outras origens ainda permanecem pendentes.

Escolha **Perfil** nas condições e selecione o nome na lista. A seleção não
troca o perfil ativo: apenas restringe o acionador. Um perfil removido permanece
como referência indisponível até você editar ou remover a condição. Se a lista
não carregar, recarregue os perfis; isso não apaga as condições já salvas.
Os atalhos e os rótulos da ajuda acompanham o perfil efetivo sem republicar o
mapa. A mudança de perfil entre etapas cancela uma sequência, mesmo voltando
ao perfil anterior antes de terminar. Atalhos locais simples não passam a
consultar o backend nem a gerar auditoria por causa dessa condição.

No Stream Deck, a seleção é conferida em cada pressionamento; os rótulos são
atualizados no ciclo de descoberta, em aproximadamente um segundo. Uma tecla
que perdeu a ação fica vazia. Não é preciso reconectar o dispositivo.

Para restringir um atalho a uma aba, adicione **Aba específica** nas condições
do acionador ou da regra da camada. Escolha a aba pelo título; o tipo é
associado automaticamente. A identidade interna não precisa ser digitada.
Trocar o tipo para outro incompatível retira a restrição à aba anterior;
confira o conjunto de condições antes de salvar. Uma aba que não está mais
aberta aparece como indisponível, sem ser substituída por outra. Editar a
prioridade não altera essa referência. A condição não cria abas nem as reabre.

- [ ] Abra duas abas de chat com títulos diferentes. Configure Ctrl+Shift+Y
  para **Próxima aba**, restrito por **Aba específica** à primeira. Na primeira
  deve avançar; na segunda, não deve executar esse binding. Resultado: ____.
- [ ] Repita com uma sequência de duas teclas; mudar de aba entre os passos
  deve cancelar a execução. Resultado: ____.
- [ ] Feche a aba escolhida e edite somente a prioridade: a referência deve
  continuar indisponível, não virar uma restrição a outra aba. Resultado: ____.
- [ ] Em uma camada de teste, associe **Criar aba de chat** no Stream Deck a
  uma condição de perfil. No perfil escolhido, a tecla deve criar a aba;
  em outro perfil, não deve executar esse binding nem manter seu rótulo.
  Volte ao perfil escolhido sem reconectar o dispositivo. Resultado: ____.
- [ ] Com NVDA, percorra duas cláusulas do editor de condições. Cada grupo
  deve anunciar sua posição e o campo, sem confundir os controles de valor
  e remoção de uma cláusula com os da outra. Resultado: ____.
- [ ] Em **Configurações → Comandos e acionadores**, na camada de teste,
  configure **Ctrl+Shift+Y** (se estiver livre) para **Abrir menu de navegação**
  e adicione **Perfil**, selecionando um perfil existente. Ative a camada e
  volte à aba de chat. No perfil escolhido, a combinação deve abrir o menu;
  feche com Esc. Em outro perfil, não deve executar esse binding. Resultado: ____.
- [ ] Na mesma configuração, acrescente **Aba específica**. Com o perfil
  correto, compare a aba escolhida com outra do mesmo tipo: as duas condições
  devem ser satisfeitas juntas. Confira também a indicação na ajuda de atalhos.
  Resultado: ____.
- [ ] Grave uma sequência de duas etapas para uma ação no perfil escolhido.
  Inicie a sequência, troque de perfil e tente a segunda etapa: não deve
  executar. Inicie novamente já no perfil correto; deve funcionar. Resultado: ____.

Prioridades, condições, argumentos e revisão de padrões ficam nos controles
avançados do editor de acionador; abra **Opções avançadas do acionador** para
exibi-los. Uma condição ou prioridade persistida que esteja inválida abre essa
seção automaticamente ao editar, para que o problema não fique oculto. Os
erros de validação continuam visíveis. Empates incompatíveis são apresentados
como conflitos, sem escolher uma ação arbitrariamente. Um padrão alterado pode
exigir revisão da personalização: restaure-a ou confirme sua adaptação à versão
atual. Alterar a configuração
republica o mapa; atalhos de navegação continuam sendo resolvidos em memória,
sem gravar cada tecla ou consultar o banco a cada pressionamento.

- [ ] Edite um acionador e abra **Opções avançadas do acionador** com o teclado;
  confira prioridade e condições sem perder mensagens de validação. Se a
  configuração já tiver uma condição/prioridade inválida, a seção deve abrir
  automaticamente. Resultado: ________.

### Gravar uma sequência de teclado

1. Na camada de teste, crie ou edite um acionador de **teclado local** e
   escolha o comando — por exemplo, **Listar workspaces** (`workspace.list`).
2. Em **Tipo de gravação**, escolha **Sequência de duas etapas**. A escolha
   vale para a próxima gravação; não substitui sozinha o atalho já salvo.
3. Vá até **Capturar atalho de teclado** e pressione Enter ou Espaço.
4. Pressione a primeira combinação com Control, Alt ou Meta — por exemplo,
   **Ctrl+Shift+Y**, se estiver livre. O gravador anuncia a primeira etapa.
5. Solte as teclas e pressione a segunda, sem modificadores — por exemplo,
   **L**. Só agora o campo recebe a sequência inteira. Salve e confirme.
6. Ative a camada, volte ao contexto apropriado e use **Ctrl+Shift+Y**, solte
   as teclas e pressione **L** em até 1,5 segundo. O prefixo sozinho não executa.

A gravação não tem limite de tempo entre etapas. O limite de 1,5 segundo vale
para **usar** a sequência, não para configurá-la. Escape cancela; Tab cancela
e move o foco. Perder o foco ou sair da tela também cancela sem substituir
o valor anterior. Escape e Tab são controles de saída da gravação, não
teclas finais graváveis. **Salvar** fica indisponível durante a captura.
Editar condições ou prioridade de uma sequência existente preserva seus
dois passos. Para substituí-la por uma combinação simples, escolha esse
tipo e grave novamente. Sequências não se aplicam a atalhos globais do SO.

### Validar no ambiente de desenvolvimento

Os bindings oficiais desta tela já foram gerados. Encerre uma instância de
desenvolvimento antiga antes de iniciar a versão atual. No CMD:

```bat
cd /d C:\caminho\do\worktree
npm run dev
```

Substitua o caminho pelo diretório do seu checkout. Use a cópia de banco
reservada aos testes: criar um workspace
descartável não isola o banco, e alterações no escopo global afetam todos os
workspaces desse banco. Não use **Restaurar tudo** no escopo global se quiser
preservar personalizações existentes.

Abra **Configurações → Comandos e acionadores**. Faça a validação abaixo
com uma camada e um workspace descartáveis; não marque um item apenas por
conseguir salvar, confira também o efeito indicado.

- [ ] Criar uma camada global e outra no workspace; conferir que a global
  aparece herdada e não pode ser alterada pela visão local. Resultado: ________.
- [ ] Associar **Próxima aba** a uma combinação pessoal e ativar a camada
  manualmente; testar, desativar e conferir que a combinação deixa de agir.
  Resultado: ________.
- [ ] Configurar a ativação por condição **Tipo de superfície = chat** para
  uma camada de navegação; conferir efeito no chat e ausência no editor.
  Resultado: ________.
- [ ] Personalizar um atalho padrão, restaurá-lo e depois restaurar a camada
  de teste. Conferir a diferença na confirmação. Resultado: ________.
- [ ] Restaurar somente o workspace de teste; conferir que a camada global
  permanece. Reabrir o aplicativo e conferir persistência. Resultado: ________.
- [ ] Percorrer os controles e confirmações com Tab, setas e NVDA; conferir
  rótulos, erros e retorno do foco. Resultado: ________.
- [ ] Em uma camada de teste com condição **Tipo de superfície = chat**,
  associar o comando de listar workspaces a uma combinação livre; conferir
  que abre a seleção no chat, mas não no editor. Resultado: ________.
- [ ] Gravar **Ctrl+Shift+Y, L** para **Listar workspaces** em uma camada de
  teste com condição **Tipo de superfície = chat**. Ativá-la e conferir:
  prefixo sozinho não executa; a sequência abre o seletor no chat, não no
  editor; mudar de aba entre passos cancela. Resultado: ________.
- [ ] Reabrir o acionador gravado, editar só a prioridade e conferir que os
  dois passos permanecem após salvar/recarregar. Resultado: ________.
- [ ] Iniciar nova gravação, pressionar apenas o prefixo e cancelar com
  Escape; repetir com Tab. O atalho anterior permanece, e nenhum prefixo
  parcial pode ser salvo. Conferir anúncios NVDA. Resultado: ________.

## Atalhos de voz e de jobs

Os atalhos definidos em perfis de voz e em jobs ainda usam suas configurações
atuais, projetadas no mecanismo comum. Não precisam ser recriados nas camadas
desta tela: sua edição continua nos perfis e jobs.
Trocar o perfil ou encerrar seu registro invalida os callbacks antigos; um
perfil com entrada de voz desabilitada não registra atalhos de voz.
O registro respeita os modificadores configurados: havia um mapeamento incorreto
de Ctrl/Alt/Shift no Windows, agora corrigido. Combinações com modificadores
desconhecidos ou duplicados são recusadas, não reinterpretadas.

A mesma combinação não pode ter dois registros globais simultâneos. Se um
perfil de voz e um job tentarem registrar a mesma combinação, o segundo
registro é recusado e o conflito é registrado no log; isso não escolhe uma
prioridade entre comandos. A reserva só é liberada depois da remoção nativa
confirmada. Se essa remoção falhar, a combinação permanece reservada por
segurança durante a sessão; não há uma segunda inscrição automática.

No Windows, manter uma hotkey global pressionada não deve repetir a ação:
o registro nativo usa supressão de repetição. Retirar o registro não precisa
esperar que a tecla seja solta, mesmo com uma entrega pendente. Uma ação já
iniciada segue seu próprio controle de cancelamento; retirar o atalho não
desfaz efeitos já executados. A repetição dos atalhos locais de navegação
(como Ctrl+Tab) não muda com essa alteração.

A hotkey de voz é entregue apenas ao painel ativo elegível, respeita modais
abertos e o mesmo controle de microfone do botão de voz. Uma inicialização
pendente não deve começar a gravar depois que você trocar de aba ou perfil.
Nos jobs, alterar/desativar a definição ou retirar o acionador impede o uso
do callback antigo. A condição `when` continua valendo.

A migração não substitui o reconhecimento de voz, a escuta por palavra de
ativação nem o runtime dos jobs. Ela unifica a resolução do acionador e os
controles de contexto/autorização antes de chamar esses serviços existentes.

O aceite físico das hotkeys globais continua pendente, especialmente tecla
mantida durante troca de configuração e encerramento. Isso é separado dos
atalhos locais e do Stream Deck já disponíveis nas camadas.

## Histórico e lista aberta no workspace

No Histórico, **Ctrl+N** executa **Abrir workspace**. Preserva o comportamento
anterior: volta à página inicial sem criar conversa nem aba. A combinação é
contextual; nas outras páginas, Ctrl+N mantém a ação correspondente. O botão
“Nova Conversa” do Histórico solicita a mesma navegação.

Na aba de lista de tarefas do workspace, os controles passam pelos comandos:

- **Abrir criação de tarefa na lista**: abre o formulário existente, sem criar
  outra lista. Salvar a tarefa continua sendo a operação do formulário.
- **Duplicar lista selecionada**: usa a lista aberta como alvo; copia suas
  configurações, não suas tarefas.
- **Limpar tarefas da lista aberta**: pede confirmação e remove tarefas e
  notas. Mantém a lista, o workflow e os metadados. Não pode ser desfeito.

Essas ações também podem ser escolhidas na paleta ou associadas a uma
combinação pessoal ou tecla do Stream Deck. Só ficam disponíveis com a lista
correta ativa. Se o conteúdo mudar durante a confirmação, a limpeza é
recusada: revise a lista e solicite novamente, sem repetição automática.

**N**, **D** e **Ctrl+L** continuam como gestos fixos dos controles da lista,
agora solicitando os mesmos comandos dos botões. Não são novos bindings
globais remapeáveis. Exigem foco dentro da lista; N/D não interceptam digitação
em campos. Uma combinação pessoal já consumida tem precedência sobre o gesto.
Alterar um binding pessoal não remove esses gestos nativos do componente.

Validação manual deste lote, em **lista descartável**, com tarefas de teste:

- [ ] No Histórico, pressionar Ctrl+N e clicar “Nova Conversa”: ambos voltam
  ao workspace sem criar aba/conversa. Resultado: ________.
- [ ] Na lista ativa, focar um controle e pressionar N: abre criação de tarefa.
  Cancelar e repetir pela paleta. Resultado: ________.
- [ ] Pressionar D e repetir pela paleta: cada ação cria uma cópia das
  configurações, sem tarefas. Resultado: ________.
- [ ] Pressionar Ctrl+L e negar: nada é apagado. Repetir e confirmar:
  tarefas/notas removidas, lista/workflow preservados. Resultado: ________.
- [ ] Configurar combinações pessoais e Deck para as três ações; conferir
  funcionamento na lista ativa e recusa em outra aba ou sob modal.
  Resultado: ________.
- [ ] Conferir anúncios pelo NVDA e que N/D digitam normalmente nos campos.
  Resultado: ________.

## Comandos padrão

Abra o botão **Comandos** na barra superior ou pressione **Ctrl+K**, inclusive
no campo de mensagem e em campos de texto nativos, sem alterar seu rascunho.
Modais, composição IME e atalhos próprios de Monaco/contenteditable continuam
protegidos. A lista não depende de criar uma camada pessoal nem
de atribuir teclas: o catálogo atual contém **146 comandos**, dos quais a paleta
mostra os permitidos nessa origem, incluindo abrir chat contextual, criar e listar
workspaces, mostrar a ajuda de atalhos, abrir o menu de navegação, abrir os nove destinos de navegação,
focar o painel ativo do workspace, criar abas de chat, editor, terminal e lista de tarefas,
fechar a aba ativa e navegar entre abas (próxima, anterior e posições de um a nove).
Isso ainda não representa todas as ações existentes no aplicativo.

A paleta usa o mesmo **Combobox** dos pickers de modelos. O foco permanece
na busca enquanto as setas mudam a opção destacada; o leitor de tela anuncia
a opção. Itens indisponíveis podem ser percorridos para consulta, mas não
executados. Enter executa a opção disponível destacada; Escape fecha o picker.

Para conferir a correção da paleta:

- [ ] Escreva um rascunho no campo de mensagem e pressione **Ctrl+K**:
  a busca abre e o rascunho permanece intacto.
- [ ] Percorra opções com **Seta para baixo/cima**, como no picker de modelos:
  uma opção por pressionamento, sem mudar o foco para botões da lista.
- [ ] Digite uma busca e pressione **Enter** na opção desejada: uma única ação.
- [ ] Em opção indisponível, **Enter** não executa e mantém a paleta aberta.
- [ ] Pressione **Escape** e confira o retorno do foco ao botão Comandos.

### Atalho efetivo, favoritos, recentes e configuração

Cada comando pode mostrar seu atalho efetivo na paleta. Com um item selecionado,
use a ação **Favoritar**/**Remover dos favoritos** para controlar sua posição;
favoritos aparecem antes dos demais itens. Comandos acionados recentemente
sobem na lista depois dos favoritos. Essas preferências são locais e separadas
por usuário e workspace deste navegador; não são sincronizadas como bindings.

A ação **Configurar** leva a **Configurações → Comandos e acionadores** para o
binding daquele comando, quando há configuração correspondente. Ela não grava
argumentos da execução. Se o comando não tiver binding localizado, a página
informa que não encontrou uma configuração para ele.

- [ ] Abra a paleta com **Ctrl+K**, selecione um comando e confira se o atalho
  efetivo aparece quando existe. Resultado: ________.
- [ ] Marque e desmarque **Favoritar** no item selecionado; confira o estado
  anunciado e que favoritos aparecem primeiro. Resultado: ________.
- [ ] Acione um comando disponível; reabra a paleta e confira sua posição entre
  os recentes. Resultado: ________.
- [ ] Use **Configurar** num comando com binding; confira o destino e que o
  binding permanece inalterado até ser editado e salvo nas configurações.
  Resultado: ________.
- [ ] Com NVDA, percorra as ações do item e confira nome, estado de favorito,
  atalho anunciado e destino de **Configurar**. A validação manual de NVDA
  continua pendente. Resultado: ________.

### Execução ad hoc de ferramentas

Ferramentas compatíveis podem ser executadas **ad hoc pela paleta**. Elas só
ficam elegíveis quando o catálogo as declara disponíveis para a origem Paleta
e a decisão exige interação. Busque pelo nome amigável; a janela de argumentos
mostra o schema disponível como orientação. Se os metadados do catálogo de
runtime não puderem ser carregados, a orientação pode ficar indisponível, mas
isso não concede autorização: o backend continua validando e decidindo a
execução. Ferramentas sem schema de argumentos que a janela consiga editar não
podem ser enviadas por esse fluxo.

Preencha os campos de acordo com o schema. Para `arguments_json`, informe um
objeto JSON válido como texto; a UI valida a sintaxe e o backend valida o
contrato real. Cada execução continua sujeita à decisão/confirmação exigida
pelo backend; cancelar não executa. A paleta anuncia o estado da execução, mas
não apresenta o payload de saída da ferramenta. Não use dados reais ou
segredos no teste.

Por D11, argumentos potencialmente sensíveis não podem ser gravados em bindings
persistentes. A configuração de comandos identifica ferramentas como somente
ad hoc; não associe uma ferramenta a atalho de teclado ou Stream Deck.

Checklist manual curto, usando uma ferramenta compatível e dados sintéticos:

- [ ] Abra **Comandos** com **Ctrl+K**, pesquise pelo nome amigável e confira
  que a janela exibe a descrição/schema disponível sem expor um ID técnico.
  Resultado: ________.
- [ ] Em `arguments_json`, tente JSON sintaticamente inválido. Esperado: a
  janela aponta o campo inválido e não envia a execução. Resultado: ________.
- [ ] Informe um objeto JSON de teste válido e envie. Confirme a decisão
  interativa; depois repita e cancele. Esperado: somente a execução confirmada
  prossegue. Resultado: ________.
- [ ] Confira que a interface anuncia apenas o estado, sem exibir o payload
  bruto de saída, e que a configuração não oferece persistir a ferramenta como
  binding de teclado/Stream Deck. Resultado: ________.

### Navegação não é histórico de teclas

Trocar abas/telas, abrir menus e a ajuda de atalhos, ou mover o foco são ações
locais da interface. Elas não gravam uma invocação de comando no banco e não
esperam uma confirmação de auditoria para aparecer na tela. Camadas, atalhos
personalizados, supressões, modais, foco e composição continuam sendo respeitados.

A última aba selecionada é salva separadamente. Quando você navega rapidamente,
seleções intermediárias ainda não enviadas são substituídas pela mais recente;
não é salvo um histórico de cada troca. Criar e fechar abas continuam protegidos
pelo fluxo de persistência e execução. Fechar logo depois de navegar aguarda a
seleção pendente e confere a aba escolhida antes de fechar.

Se a gravação da seleção falhar, o aplicativo reconcilia a aba ativa e anuncia
a falha. A recuperação de foco não deve retirá-lo de outro controle que você
já tenha escolhido. Escape cancela uma criação ou um fechamento que ainda
esteja aguardando essa gravação; não desfaz uma operação já efetivada.

### Comandos da interface ativa

Os comandos de navegação, como **Alt+C** (Configurações), **Alt+M** (menu) e
**Alt+J** (Jobs), também funcionam com foco no campo de mensagem. Uma tecla
do Stream Deck associada à mesma navegação segue a mesma regra. Isso não
torna comandos de uma tela disponíveis fora dela. Modais e composição IME
ativa continuam bloqueando; Monaco e contenteditable não recebem essa exceção.

Para conferir a navegação durante a edição:

- [ ] Escreva um rascunho e pressione Alt+M: deve abrir o menu sem enviar a mensagem.
- [ ] Retorne ao campo e teste Alt+C e Alt+J: devem abrir os destinos correspondentes.
- [ ] Foque o campo sem digitar e pressione a tecla do Deck associada ao menu:
  deve abrir o menu, sem exigir uma tecla do teclado antes.
- [ ] Repita com modal aberto: não deve executar a navegação atrás do modal.
- [ ] Com composição IME ativa, a navegação deve continuar bloqueada.

Anote passou/falhou e o anúncio do NVDA; esses casos exigem aceite manual.

### Listas de tarefas, perfis e terminal

Nas páginas **Listas de tarefas** e **Perfis**, a paleta permite abrir o
formulário de criação, abrir a edição do item selecionado e focar a busca.
**Ctrl+N** abre o formulário da página ativa, sem criar também uma aba.
No workspace, as sequências **Ctrl+N, C/E/R/T** continuam disponíveis conforme
o mapa configurado. Ctrl+N não substitui edição de texto em campos editáveis.

No terminal ativo, a paleta permite abrir o seletor de sessões, focar a
entrada e focar o histórico quando houver conteúdo. Essas nove ações também
aceitam associações pessoais de teclado ou Stream Deck. Somente as duas
aberturas de criação recebem Ctrl+N padrão; os outros sete comandos não
recebem atalhos arbitrários.

Editar usa o item selecionado antes de abrir a paleta. Se a seleção, página,
aba ou sessão mudar, a ação antiga é recusada. Abrir formulários não salva
registros. Salvar/excluir/duplicar/ativar perfis ou listas e iniciar,
interromper ou encerrar processos do terminal **ainda não foram migrados**
para esses comandos; os controles existentes continuam disponíveis.

Validação manual deste bloco:

- [ ] Em Listas, fora de um campo editável, Ctrl+N abre uma única criação.
- [ ] Em Perfis, repetir Ctrl+N: criação de perfil, nenhuma aba nova.
- [ ] Selecionar um item em cada página; abrir a paleta e executar a edição:
  o formulário deve corresponder ao item escolhido.
- [ ] Executar Focar busca em cada página, inclusive com filtro sem resultados.
- [ ] No terminal, testar pela paleta Abrir seletor de sessões, Focar entrada
  e Focar histórico; histórico vazio deve ficar indisponível.
- [ ] Associar uma dessas ações ao Deck e conferir o mesmo resultado;
  fora da página/aba correta ou atrás de modal, não deve executar.
- [ ] Remapear/suprimir Ctrl+N de uma página: respeitar o mapa, sem cair na
  criação de aba. No workspace, conferir Ctrl+N seguido de C/E/R/T.

### Menus e apresentação do editor

Na aba ativa de editor, a paleta oferece **Abrir menu Arquivo do editor**,
**Abrir menu Formatar do editor**, **Abrir menu Inserir do editor**,
**Abrir menu Modo do editor**,
**Abrir seletor de slides do editor** e **Alternar tela cheia da apresentação**.
Eles apenas abrem os controles existentes; escolher depois Salvar, um modo ou
uma formatação continua sendo uma ação separada. Formatar requer edição rica;
slides requerem um documento Reveal em edição rica; tela cheia requer Reveal
em visualização. Controles desabilitados não podem ser acionados pelo comando.

**Alt+S** abre slides e **F5** solicita tela cheia pelo novo mapa. Os quatro
menus podem ser abertos pela paleta, Stream Deck ou por uma associação pessoal.
**Alt+I** é contextual: no editor ativo, visível e apto abre Inserir; fora do
editor, abre Importar dados. Não há listener legado nem fallback silencioso.
Inserir apenas apresenta o menu: não seleciona nem escreve conteúdo. Ctrl+S/O
e Ctrl+Shift+S são comandos contextuais de arquivo; Alt+1/2/3 são os modos persistentes do
editor descritos na seção seguinte.

- [ ] Na aba do editor, abra a paleta pelo botão e procure os quatro menus;
  execute cada um, confira setas/Escape e retorno de foco. Resultado: ____.
- [ ] Com editor ativo e editável, pressione Alt+I: apenas Inserir deve abrir;
  nenhuma ação de conteúdo deve ser executada. Resultado: ____.
- [ ] Fora do editor, pressione Alt+I: apenas Importar deve abrir; cancelar
  não deve importar dados. Resultado: ____.
- [ ] Com editor ativo em readonly ou modo view, Alt+I não deve abrir Inserir
  nem cair em Importar dados. Resultado: ____.
- [ ] Com outra aba ativa, Alt+I não deve abrir Inserir no editor encoberto;
  deve seguir a ação efetiva dessa superfície. Resultado: ____.
- [ ] Com modal, overlay ou composição IME ativos, Alt+I não deve atravessar
  a superfície bloqueadora. Resultado: ____.
- [ ] Suprima o padrão Alt+I de Inserir: no editor, nem Inserir nem Importar
  devem abrir; fora do editor, Importar continua disponível. Restaure depois.
  Resultado: ____.
- [ ] Remapeie Inserir e confirme que a nova combinação abre o menu uma única
  vez e que a combinação suprimida não usa o listener antigo. Resultado: ____.
- [ ] Em Reveal no modo rico, teste Alt+S e a ação equivalente pela paleta
  e por uma tecla do Deck. Apenas o seletor deve abrir. Resultado: ____.
- [ ] Em Reveal no modo de visualização, teste F5 e a ação equivalente;
  confira entrada/saída de tela cheia. Resultado: ____.
- [ ] Repita com outro editor ativo, outro tipo de aba, um modal ou menu
  sobreposto: não deve acionar o editor encoberto. Resultado: ____.
- [ ] Remapeie/suprima Alt+S ou F5 e confirme que a tecla anterior não executa
  por um listener legado; mantenha a tecla pressionada e confira ausência
  de reaberturas/toggles repetidos. Resultado: ____.
- [ ] Com NVDA, confira anúncios dos controles e navegação dos menus.
  Resultado: ____.

### Modos persistentes do editor

A regra vigente é explícita: IME ativo sempre bloqueia. `UnknownIME` não
é IME ativo; nesse estado desconhecido, somente controles native/rich
suportados e reconhecidos por registry válido podem prosseguir.
Na aba de editor ativa, **Alt+1** aplica `editor.mode.markdown`, **Alt+2**
aplica `editor.mode.rich` e **Alt+3** aplica `editor.mode.view`. Os três são
operações duráveis: passam por **Begin/Take/Commit**, confirmação explícita e
alvo `workspace/active_tab` com `ExactVersion`. O backend usa CAS; replay/ABA,
inclusive repetir o mesmo modo, falha de persistência e concorrência falham
fechado e fazem rollback. Somente `Tab.State.displayMode` é alterado; conteúdo,
arquivos e os demais campos da aba são preservados.

Antes da confirmação, a UI faz flush do estado Rich. O novo modo só é aplicado
após a confirmação. Se workspace, aba, documento, foco ou versão mudarem,
descarta a operação e não rouba o foco do novo controle ativo. Composição IME
ativa sempre bloqueia. `UnknownIME` é estado distinto e só pode prosseguir
para controles `native`/`rich` suportados quando reconhecidos por registry
válido. Ctrl+S, Ctrl+O e Ctrl+Shift+S usam os comandos de arquivo descritos ao final.

Checklist manual:

- [ ] Em editor editável, teste Alt+1/2/3 por tecla, menu, paleta e Stream Deck;
  confirme anúncio, confirmação única e modo correto. Resultado: ____.
- [ ] Reinicie o aplicativo e confirme a persistência de cada modo sem alterar
  conteúdo ou arquivos. Resultado: ____.
- [ ] Em readonly/view, modal e IME ativo, confirme bloqueio. Com IME
  desconhecido (`UnknownIME`), teste somente controles native/rich suportados
  por registry válido; não trate esse estado como IME ativo. Resultado: ____.
- [ ] Remapeie e suprima Alt+1/2/3; confirme que só o binding efetivo executa,
  sem listener legado nem fallback. Resultado: ____.
- [ ] Mude aba/workspace/foco durante a confirmação e confirme cancelamento,
  ausência de roubo de foco e rejeição de replay/ABA. Resultado: ____.

Tela cheia usa a API existente do WebView. Alguns ambientes exigem um gesto
confiável de teclado/clique e podem recusar uma solicitação via Stream Deck;
nesse caso a apresentação permanece embutida. Esse comportamento físico
precisa de validação no aplicativo, não é garantido pelo teste automatizado.

### Pickers do chat: roteiro de validação

O lote de migração de **Abrir seletor de modelo**, **Abrir histórico do chat**
e **Abrir seletor de perfil** preserva **Ctrl+M**, **Ctrl+H** e **Ctrl+P**.
As ações apenas abrem os pickers existentes: não alteram a seleção, não
enviam a mensagem e não limpam a conversa. **Ctrl+L não faz parte deste lote.**

Roteiro para executar após a validação automática do lote:

- [ ] Na aba de chat, escreva um rascunho e teste Ctrl+M, Escape, Ctrl+H,
  Escape, Ctrl+P, Escape. Cada combinação abre apenas o picker correspondente;
  o rascunho permanece intacto. Resultado: ____.
- [ ] Abra Ctrl+K e procure cada um dos três comandos. Execute-os por Enter:
  deve abrir o picker do chat ativo, não outro chat. Resultado: ____.
- [ ] Em uma aba de editor, abra o chat contextual com Ctrl+Shift+I. No campo
  de mensagem desse modal, repita Ctrl+M/H/P. Resultado: ____.
- [ ] Abra outro modal sobre esse chat e repita as teclas: não podem abrir
  pickers no chat encoberto. Feche o modal superior e confira novamente.
  Resultado: ____.
- [ ] Em Configurações → Comandos e acionadores, associe cada comando a uma
  tecla do Stream Deck. Teste na aba de chat e no chat contextual, inclusive
  focando o campo sem digitar antes. Fora de um chat apto, não deve executar.
  Resultado: ____.
- [ ] Remapeie um dos atalhos na camada de teste: somente a combinação efetiva
  deve executar. Teste também sua supressão; o listener antigo não pode abrir
  o picker por fora da configuração. Resultado: ____.
- [ ] Mantenha a tecla pressionada: o picker não deve alternar repetidamente.
  Com composição IME ativa, o comando não deve interromper a digitação.
  Resultado: ____.
- [ ] Com NVDA, confira anúncio, busca, setas, Escape e retorno de foco nos
  pickers. Registre comando, resultado e anúncio se encontrar falha: ____.

A paleta superior não abre por cima do chat modal; nesse contexto os três
pickers são acionados pelo teclado, pelos seus botões ou pelo Stream Deck.

**Focar painel ativo** leva o foco ao conteúdo da aba atual do workspace.
Só executa na tela do workspace, com uma aba visível e seu controle pronto;
não abre o workspace nem procura outra aba quando acionado em Configurações
ou Histórico. Se a aba, sessão ou tela mudar durante a preparação, cancela.
Um painel ainda carregando não recebe um pedido atrasado de foco.

O comando pode ser escolhido na paleta ou associado a uma tecla personalizada
ou ao Stream Deck. Não foi atribuído um novo atalho padrão. As mesmas restrições
valem para todas essas origens: uma camada global não torna a ação global.
**Criar aba de chat** usa o novo mecanismo e tem **Ctrl+T** como atalho padrão.
Funciona no workspace, inclusive com foco no campo de mensagem. A criação é
gravada pelo backend no workspace capturado; troca de aba ou workspace durante
a preparação cancela a tentativa. Não executa atrás de modal, durante composição
IME, no editor Monaco ou fora do workspace. Paleta e Stream Deck usam a mesma
operação; no Deck, associe o comando à tecla da sua camada pessoal.

**Criar aba de editor** e **Criar aba de lista de tarefas** usam a mesma operação
protegida de criação no workspace atual. Estão na paleta e podem receber um
atalho pessoal ou uma tecla do Stream Deck em **Comandos e acionadores**.
Não receberam novas teclas padrão: Ctrl+T continua sendo exclusivo de chat.
Os nomes iniciais respeitam o idioma da interface.

**Fechar aba ativa** usa **Ctrl+W** ou **Ctrl+F4**, além da paleta e de uma tecla
configurada no Stream Deck. Fecha somente a aba capturada no workspace visível.
Se ela for a última, cria uma aba de chat vazia na mesma gravação; não deixa o
workspace sem abas. Não apaga a conversa/documento nem encerra uma sessão de
terminal: mantém a semântica anterior de remover apenas a aba.

Após o fechamento, o foco vai para o painel sucessor pronto ou para seu botão
de aba. Não há pedido atrasado para roubar o foco se você já navegou para outro
controle. Modais, composição IME e Monaco permanecem protegidos; o teclado
também respeita a prioridade do DataGrid.

**Próxima aba**, **Aba anterior** e **Primeira aba** até **Nona aba** estão na
paleta e podem ser associados ao Stream Deck ou a combinações pessoais livres.
Só atuam no workspace visível; uma posição inexistente fica indisponível.
Próxima/anterior percorrem circularmente as abas. Selecionar a aba que já está
ativa não cria nem fecha nada. Cada acionamento usa o estado atual da interface,
sem aguardar o próximo render ou uma transação de auditoria.

**Ctrl+Tab, Ctrl+Shift+Tab, Ctrl+PageUp/PageDown e Ctrl+1…9** usam os comandos
de navegação do mapa padrão, que agora contém **40 combinações**. Podem ser
personalizados ou suprimidos pelo novo mecanismo, sem um handler antigo que
execute por fora do mapa. Manter a tecla pressionada repete a navegação, mas
não repete criar/fechar abas. Grades e controles de abas internas preservam
sua própria navegação; setas sem modificadores não foram capturadas globalmente.

Para validar a navegação nova, use a paleta ou uma tecla livre do Deck:

- [ ] Abra três abas; execute **Próxima aba** e **Aba anterior** e confira o destino.
- [ ] Na última aba, execute **Próxima aba**: deve selecionar a primeira.
- [ ] Execute **Segunda aba** e confira o foco e o anúncio do NVDA.
- [ ] Com três abas, **Nona aba** deve estar indisponível na paleta.
- [ ] Com um modal aberto ou em Configurações, não deve trocar a aba do workspace.
- [ ] Associe **Próxima aba** a uma tecla livre do Deck; uma pressão deve navegar.
- [ ] Pressione Ctrl+Tab várias vezes rapidamente e mantenha-o pressionado:
  cada ocorrência aceita deve avançar, inclusive ao voltar da última à primeira.
- [ ] Repita com Ctrl+Shift+Tab, Ctrl+PageUp e Ctrl+PageDown.
- [ ] Teste Ctrl+1, Ctrl+2 e Ctrl+9: selecionam a posição existente; uma
  posição inexistente não deve trocar a aba nem mover o foco.
- [ ] Suprima temporariamente Ctrl+Tab no mapa padrão: não deve existir um
  atalho antigo executando por fora da configuração. Restaure após conferir.
- [ ] Navegue e pressione Ctrl+W imediatamente: deve fechar a aba escolhida,
  nunca a anterior por causa de uma gravação pendente.
- [ ] Reinicie pelo fluxo habitual e confira a aba ativa persistida.

Para cada item, registre **passou/falhou**, a aba de destino e o anúncio do
NVDA. A conferência manual desta revisão continua pendente.

### Criação de terminal

`workspace.tab.terminal.create` chegou ao frontend integrado nas três origens
previstas e também participa da sequência Ctrl+N. A admissão
contextual invalida owner, sessão ou workspace obsoletos e usa commit único.
O catálogo atual tem 94 comandos, 37 de apresentação local e 61 defaults
(57 v1 + quatro v2), com 60 combinações efetivas.
O evento `session_created` é deduplicado, o histórico é preservado e uma
surface existente recarrega a sessão perdida sem recriá-la.

Ctrl+N seguido de R usa o mesmo comando de criação. O lifecycle/compensação backend usa o
manager real e tem testes isolados de falhas e compensação; a regressão App e
`go vet` passaram. Wails, ACP, PTY real e banco real não foram executados. Só
o aceite manual do novo terminal permanece pendente.

Roteiro manual pendente:

- [ ] No workspace, pressione **Ctrl+K**, escolha **Criar aba de terminal** e
  confirme uma nova sessão.
- [ ] Configure uma tecla pessoal e uma tecla do Stream Deck para o mesmo
  comando; cada invocação deve criar uma nova sessão.
- [ ] Fora do workspace, confirme que o comando fica indisponível.
- [ ] Feche a aba e confirme que a sessão do terminal não é encerrada.

Abrir o chat contextual continua no caminho existente. Esta seção não declara
o terminal migrado nem substitui o roteiro de aceite manual do AEP-0089.

Para validar o fechamento (use uma cópia dos seus dados):

- [ ] Com pelo menos três abas, feche a do meio com Ctrl+W: a próxima deve ficar ativa.
- [ ] Feche a última da direita com Ctrl+F4: a anterior deve ficar ativa.
- [ ] Com apenas uma aba, feche-a: deve sobrar uma nova aba de chat vazia.
- [ ] Execute **Fechar aba ativa** pela paleta e pelo Deck: uma pressão fecha uma aba.
- [ ] Confira o foco e o anúncio do NVDA após cada fechamento, inclusive com
  editor/lista de tarefas como aba sucessora.
- [ ] Em Configurações ou com modal aberto, o comando não deve fechar abas ocultas.
- [ ] Reinicie pelo fluxo habitual e confira que o fechamento foi persistido.

Para validar editor e lista de tarefas (use uma cópia dos seus dados):

- [ ] No workspace, abra **Comandos**, execute **Criar aba de editor** e confira
  que aparece uma única aba do editor com nome legível.
- [ ] Execute **Criar aba de lista de tarefas**: deve abrir uma única aba desse tipo.
- [ ] Associe uma combinação pessoal livre a cada comando e teste no workspace,
  inclusive a partir do campo de mensagem; Ctrl+T deve continuar criando chat.
- [ ] Associe os novos comandos a teclas do Deck e execute no workspace.
- [ ] Em Configurações ou com modal aberto, os comandos não devem criar abas.
- [ ] Reinicie pelo fluxo habitual e confira que as abas continuam presentes.

Registre passou/falhou, o tipo de aba criado e o anúncio do NVDA.

Para validar a criação de chat (use uma cópia dos seus dados):

- [ ] No workspace, pressione **Ctrl+T** uma vez: deve surgir uma única aba de chat.
- [ ] Digite no campo de mensagem e pressione **Ctrl+T**: deve criar outra aba,
  sem enviar a mensagem da aba anterior.
- [ ] No botão **Comandos**, procure **Criar aba de chat** e pressione Enter:
  deve criar uma única aba, também após você já ter usado Ctrl+T.
- [ ] Abra um modal: Ctrl+T não deve criar aba atrás dele. Feche o modal e repita.
- [ ] Em Configurações, **Criar aba de chat** deve estar indisponível na paleta.
- [ ] Associe **Criar aba de chat** a uma tecla do Stream Deck: uma pressão deve
  criar uma aba no workspace; fora dele ou com modal aberto, não deve criar.
- [ ] Reinicie pelo fluxo habitual e confira que as abas criadas foram mantidas.

Anote para cada item: passou/falhou, resultado observado e anúncio do NVDA.

Para conferir o comportamento contextual:

- [ ] No workspace, com uma aba carregada, abra a paleta pelo botão **Comandos**,
  procure **Focar painel ativo** e execute. O foco deve ir ao conteúdo daquela aba.
- [ ] Repita em chat, editor, terminal e lista de tarefas já carregados.
- [ ] Abra Configurações e procure o mesmo comando: deve estar indisponível,
  sem levar o foco para uma aba escondida.
- [ ] Em sua camada pessoal, associe uma combinação livre ao comando e ative
  a camada. Teste fora de campos de edição: no workspace deve focar o painel;
  em Configurações não deve executar a ação.
- [ ] Associe uma tecla do Stream Deck ao mesmo comando. Com o aplicativo em
  primeiro plano, confira o mesmo resultado no workspace e a recusa fora dele.
- [ ] Com um modal aberto, confira que nenhuma dessas associações move o foco
  para a interface de fundo.

Na busca, digite o nome ou alias; **Seta para baixo** entra nos resultados
e **Enter** executa o selecionado. Sem pesquisa, todos os comandos do
catálogo devem aparecer. Comandos indisponíveis não podem ser executados.
Ao abrir, o leitor de tela anuncia a quantidade total e a quantidade
disponível. O campo de busca não deve executar comandos ao digitar espaço;
Enter dispara uma única seleção, e a seta para baixo entra no primeiro
resultado. Se nenhuma ação puder ser executada, confira o anúncio de
disponibilidade: uma lista carregada não é o mesmo que um runtime pronto.
Uma mudança de aba, perfil ou revisão de jobs não deve deixar todos os
comandos indisponíveis: a consulta da paleta atualiza a projeção contextual
antes de verificar disponibilidade, como já faz o caminho de execução.
Sessão revogada, cofre bloqueado e runtime encerrado continuam bloqueando.

Na inicialização, a observação da sessão do Windows pode chegar depois do
login. Os comandos aguardam essa observação e recompõem o mapa autenticado
quando ela confirma a sessão desbloqueada; o mesmo vale após bloquear e
desbloquear a estação. A barra recebe uma nova notificação quando o teclado
está pronto, não apenas quando a projeção foi carregada. Não é necessário
apagar camadas nem recriar chaves para recuperar essa sequência de startup.
A tela de configurações também tenta novamente uma carga inicial falhada
quando recebe esse aviso, sem fechar um editor nem descartar alterações.

Se o banco estiver momentaneamente ocupado na preparação das configurações
ou restauração das camadas, a inicialização repete essas transações com
espera limitada. Isso não executa nem repete comandos. Se a disputa persistir
ou a sessão deixar de ser válida, os comandos continuam indisponíveis;
o log registra a falha para diagnóstico.

A camada **Comandos padrão** disponibiliza comandos na paleta sem definir atalhos.
Seu conteúdo original é versionado pelo aplicativo: desativar um acionamento
cria uma personalização reversível, sem apagar o padrão. Restaurar remove essa
personalização. A alteração passa por uma confirmação antes de ser gravada.

Uma supressão confirmada e publicada já afeta a execução pela paleta. Por
exemplo, desativar **Listar workspaces** impede que esse acionamento abra a
lista; restaurá-lo permite usar o padrão novamente. A descoberta do comando
na paleta não é, por si só, prova de que seu acionamento está permitido.

## Mapa de teclado padrão

A camada **Mapa de teclado padrão** é integrada e sempre ativa. Ela associa
atalhos padrão a comandos.
Não é necessário criar ou ativar uma camada pessoal para estes atalhos:

- **Alt+W** ou **Alt+Backspace**: abrir workspace.
- **Alt+C**: abrir configurações.
- **Alt+M**: abrir o menu de navegação, também disponível na paleta.
- **Alt+H**: abrir histórico.
- **Alt+L**: abrir memórias.
- **Alt+T**: abrir listas de tarefas.
- **Alt+J**: abrir jobs.
- **Alt+P**: abrir perfis.
- **Ctrl+T**: criar aba de chat no workspace (também no campo de mensagem).
- **Ctrl+W** ou **Ctrl+F4**: fechar a aba ativa do workspace.
- **Ctrl+Tab** ou **Ctrl+PageDown**: próxima aba.
- **Ctrl+Shift+Tab** ou **Ctrl+PageUp**: aba anterior.
- **Ctrl+1…9**: selecionar a aba daquela posição.

Essas combinações passam exclusivamente pelo novo mecanismo de comandos,
que separa apresentação local de execução persistente. Suprimir um
padrão na tela de configurações impede esse acionamento; **Restaurar padrão**
o recupera. Uma combinação de camada pessoal ativa tem precedência sobre o
padrão correspondente. A prioridade de campos editáveis e modais continua
valendo; não há fallback para o tratamento antigo se o mapa estiver indisponível.
Alt+Backspace fora de campos editáveis continua impedindo o retorno nativo
do WebView, inclusive com modal aberto, sem navegar por trás do modal.

**Ctrl+K**, **Alt+E** e **Alt+I** agora pertencem ao mapa efetivo, sem um
listener legado executando por fora. **F1** também usa o mapa efetivo e abre
Ajuda. Pode ser suprimido, restaurado ou substituído por um atalho pessoal.
Somente abrir Ajuda pelo teclado conserva a exceção durante modais; associar
F1 a outra ação não permite executá-la por trás de um modal. Paleta e Stream
Deck continuam bloqueados durante modais. Sobre continua disponível na paleta
e aceita atalhos pessoais, sem nova combinação padrão.

### Conferência de F1 no próximo lote manual

- [ ] F1 abre Ajuda, inclusive com foco no campo de mensagem.
- [ ] Em Comandos e acionadores, suprima F1; ele não deve mais abrir Ajuda.
- [ ] Restaure o padrão e confira que F1 volta a funcionar.
- [ ] Associe Abrir ajuda a uma combinação pessoal: ela deve abrir a mesma tela.
- [ ] Com um modal aberto, confira Ajuda pelo teclado; Alt+C e demais ações
  comuns não devem navegar no fundo. Uma tecla do Deck também não deve fazê-lo.
- [ ] Manter F1 pressionado não deve repetir a navegação.

### Criar workspace

**Ctrl+Shift+N**, **Criar workspace** na paleta, uma tecla do Stream Deck
associada a esse comando e **Novo workspace** no menu usam a mesma operação.
O novo workspace aparece na lista, mas **o workspace atual não muda**.
Funciona também em outras telas, como Configurações, quando a sessão e o
workspace estão prontos. Modais, composição e controles com atalhos próprios
continuam protegidos. Manter a tecla pressionada não cria vários workspaces.

O atalho pode ser remapeado ou suprimido em Comandos e acionadores, sem um
atalho antigo executando por fora do mapa. Mudanças de contexto antes da
gravação cancelam a solicitação. Uma falha ao publicar o índice é reportada
e remove somente o workspace recém-criado; não apaga o índice anterior.

Conferência no próximo lote manual (anote resultado e anúncio do NVDA):

- [ ] Ctrl+Shift+N cria um workspace e mantém o atual aberto.
- [ ] Na paleta, execute Criar workspace; confira uma única nova entrada.
- [ ] No menu do workspace, execute Novo workspace; confira o mesmo resultado.
- [ ] Associe Criar workspace a uma tecla do Deck e confira uma única criação.
- [ ] Repita em Configurações: cria sem navegar para outra tela.
- [ ] Suprima ou remapeie Ctrl+Shift+N: a combinação antiga não deve criar.
- [ ] Mantenha a tecla pressionada: não deve repetir a criação.
- [ ] Reinicie o aplicativo: o workspace criado continua na lista, sem mudar
  qual workspace estava ativo antes de sair.

### Abrir chat contextual

**Ctrl+Shift+I**, **Abrir chat contextual** na paleta, uma tecla do Stream Deck
associada ao comando e os botões de chat contextual usam a mesma operação.
No editor, terminal e lista de tarefas, abre o chat do painel ativo, reutilizando
a conversa vinculada ou criando uma quando necessário. Na aba de chat, apenas
foca o campo de mensagem. **Abrir não envia mensagem nem executa instruções.**

A preparação do contexto permanece na interface; criação e vínculo passam
pelo backend autenticado. O editor somente leitura, outros modais e composição
IME bloqueiam a abertura. O comando funciona dentro do editor editável, sem
tornar as demais ações do aplicativo disponíveis nesse contexto.

Se mudar de aba, workspace, tela ou sessão durante a preparação, a solicitação
antiga não abre o chat sobre o contexto novo. Escape cancela uma abertura
pendente; não desfaz uma gravação que já tenha sido efetivada. Falha de vínculo
remove somente a conversa recém-criada por essa tentativa, não uma preexistente.

Conferência no próximo lote manual (anote passou/falhou e anúncio do NVDA):

- [ ] Em um editor editável, Ctrl+Shift+I abre o chat contextual sem abrir DevTools.
- [ ] Feche e reabra: a conversa anterior é reutilizada, sem envio automático.
- [ ] Repita no terminal e na lista de tarefas, também usando o botão de chat.
- [ ] Na paleta, execute Abrir chat contextual e confira o contexto do painel ativo.
- [ ] Associe o comando a uma tecla do Deck; teste com foco dentro do editor.
- [ ] Em uma aba de chat, Ctrl+Shift+I foca seu campo, sem criar outra conversa.
- [ ] Suprima ou remapeie o atalho: a combinação antiga não abre chat nem DevTools.
- [ ] Manter a tecla pressionada não abre repetidamente nem cria conversas extras.
- [ ] Com modal aberto ou editor somente leitura, não abre outro chat.
- [ ] Se a preparação demorar, Escape ou troca de aba impede a abertura atrasada.

## Camadas pessoais e captura de teclas

### Stream Deck: comandos disponíveis

O ingresso físico aceita navegação, ajuda e as operações de workspace já
migradas, inclusive Abrir chat contextual. As restrições de tela e painel
continuam valendo. Não envia teclas para outros programas nem executa macros,
shell, jobs ou ações de edição/chat ainda não migradas. `Listar workspaces`
continua na paleta/teclado, mas não neste ingresso físico.

1. Feche o software oficial da Elgato para liberar o dispositivo.
2. Abra **Comandos e acionadores**, selecione uma camada pessoal e crie um
   acionador do tipo **Stream Deck**.
3. Escolha o comando, por exemplo **Abrir configurações**, e clique em
   **Gravar acionador**. Aguarde a indicação de prontidão e pressione e solte
   a tecla desejada no Stream Deck.
   O aplicativo identifica o aparelho automaticamente, mesmo com vários
   conectados. Não há seletor de dispositivo, serial ou posição para digitar.
4. Confira a indicação amigável, como **Stream Deck — tecla 3**, salve,
   confirme a alteração e ative a camada. Cancelar a gravação preserva o
   acionador anterior; capturar uma tecla ainda não salva a alteração.
5. Feche o diálogo de edição, mantenha a janela do Assistente em foco e
   pressione a tecla. A ação deve ocorrer uma
   única vez por pressionamento; soltar a tecla permite o próximo acionamento.

Durante a gravação, aparelhos conectados são abertos temporariamente pelo
mesmo leitor usado na execução. Teclas capturadas não executam comandos e
permanecem suprimidas até serem soltas. A gravação termina por cancelamento,
perda de foco/sessão ou após 30 segundos. Sem aparelho conectado, conecte um
Stream Deck e tente gravar novamente. Fora da captura, somente dispositivos
com bindings efetivos são abertos. A reconexão reenvia
o frame completo; perda de sessão ou bloqueio remove o mapa e limpa os títulos.
Em **Configurações → Comandos e acionadores**, a seção **Dispositivos Stream
Deck** mostra por aparelho o modelo, quantidade de teclas, estado traduzido e,
quando houver, uma explicação legível da falha ou espera de reconexão. Ela não
exibe ID interno nem número de série; condições de dispositivo só oferecem
aparelhos conectados.
Os títulos usam o idioma selecionado na tela de configurações (inglês antes
da primeira seleção nesta sessão). A posição deve existir no modelo conectado;
posições fora da geometria não são executadas. Títulos, ícones, imagens e
apresentações por estado são configurados no formulário descrito abaixo.
As ações de camada permitem trocar o mapa pelo dispositivo; não há editor
visual de pastas nem animações.
Neste recorte, ações visuais são recusadas com modal aberto ou com a janela
sem foco; o dispositivo não traz o aplicativo para frente automaticamente.

O usuário confirmou a execução básica no aparelho em 18/09/2026. A nova captura
de teclas e os cenários específicos de desconectar/reconectar,
bloquear/desbloquear e reiniciar ainda exigem conferência manual.

#### Títulos, ícones e imagens das teclas

Ao criar ou editar um acionador **Stream Deck**, você pode preencher um título
para português, inglês e espanhol. Os campos são textuais e acessíveis por Tab;
não é necessário selecionar uma imagem nem operar uma grade visual. Cada título
aceita até 256 caracteres. Salve e confirme a alteração para aplicá-la.

Deixe um idioma vazio para usar o nome localizado do comando naquele idioma.
Apagar um título não apaga a associação, o comando ou os outros idiomas. O nome
do comando na paleta também não muda: o título pertence à tecla configurada.
Os títulos são recuperados ao reabrir o editor. O seletor de apresentação
permite editar o padrão ou personalizar um estado específico.

O campo **Ícone** oferece configurações, conversa, pasta, reproduzir, parar,
voltar e estrela. Escolha pelo nome, usando o teclado, e salve/ confirme a
alteração. **Sem ícone** restaura a tecla somente com texto. O ícone aparece
acima do título; não altera a ação nem substitui o texto ou o anúncio.
Em geometrias muito pequenas, o texto tem prioridade e o ícone não é desenhado.

Um ícone desconhecido já salvo é preservado e indicado como indisponível no
seletor. Ele não é carregado de arquivo ou da internet: a tecla usa só o título.
Você pode substituí-lo por uma opção disponível ou removê-lo.

Para uma **imagem personalizada**, use o seletor de arquivo no mesmo formulário.
Escolha PNG ou JPEG de até **1 MiB**, no máximo 4.096 pixels por eixo e
4 milhões de pixels no total. O aplicativo reduz a imagem para caber em
128 × 128 pixels, preserva a proporção e descarta metadados. Salve e confirme.
Enquanto o arquivo é lido, Salvar fica indisponível. Cancelar a confirmação
não grava a imagem. Nenhum caminho de arquivo ou endereço da internet é usado
posteriormente para carregar a tecla.

A imagem aparece acima do título e tem prioridade sobre o ícone. **Remover
imagem** mantém títulos, comando e ícone; salve e confirme para aplicar a remoção.
Em teclas pequenas, o texto tem prioridade. O formulário informa se há imagem
associada, sem exigir a leitura de identificadores técnicos.

As imagens ficam no banco local, separadas da configuração e isoladas por
usuário, com limite de 16 MiB de imagens normalizadas por usuário. Imagens iguais
são reutilizadas; quando nenhuma associação as utiliza, são removidas na mesma
transação da alteração. O arquivo original não é modificado.
**Exportar configurações não exporta esses arquivos.** Ao importar em outro
banco ou usuário, selecione a imagem novamente: se a referência não estiver
disponível, a tecla usa ícone/título e continua executando o mesmo comando.
Você pode editar outros campos mantendo essa referência importada indisponível;
isso não copia nem libera acesso à imagem de outro usuário. Para exibi-la,
selecione o arquivo novamente. A substituição de imagens também libera, na
mesma transação, o espaço das imagens antigas que deixaram de ser utilizadas.
Para voltar a uma versão sem suporte a imagens, use uma cópia anterior do
banco ou remova as imagens nesta versão antes.

Quando uma tecla pode executar comandos diferentes conforme o contexto visual,
ela continua mostrando os comandos potenciais, separados por barra. Um título
personalizado só substitui o nome de um comando se suas associações candidatas
concordarem no mesmo título para o idioma atual; em caso de divergência, aparece
o nome localizado do comando, evitando apresentar uma associação arbitrária.
O ícone também só aparece quando todas as associações elegíveis concordam na
mesma opção; se alguma não tiver ícone ou usar outro, permanece somente o texto.

#### Resultado da execução na tecla

Comandos do Stream Deck que passam pelo executor com acompanhamento de resultado
mostram um estado textual temporário: aguardando, executando, concluído, falhou,
negado, cancelado, tempo esgotado ou resultado desconhecido. O estado aparece
junto da apresentação da tecla e é anunciado pelo leitor de telas quando o
Assistente está em foco, na mesma sessão e no mesmo mapa de comandos.
O envio de uma ação para a interface não significa que ela foi concluída:
o indicador de conclusão depende da confirmação efetiva do executor.

A apresentação é atualizada em ciclos de aproximadamente um segundo; operações
rápidas podem mostrar apenas o resultado final. O resultado final fica disponível
por três segundos e depois a tecla retorna à apresentação normal. Uma nova
execução da mesma tecla substitui a anterior; uma conclusão atrasada não toma
seu lugar. Desconexão, troca de mapa ou bloqueio invalidam o feedback antigo.

Atalhos locais rápidos, como navegação, continuam sem auditoria adicional e não
exibem conclusão artificial: esses eventos não possuem confirmação de resultado
no backend. Os indicadores não criam registros adicionais no banco.

#### Apresentação por estado e indicação de camada ativa

No editor da tecla, escolha a apresentação padrão ou um estado: ligado,
desligado, aguardando, executando, concluído, falhou, negado, cancelado,
tempo esgotado ou resultado desconhecido. Cada estado usa os mesmos campos
de título por idioma, ícone e imagem. Campos não preenchidos herdam o padrão;
remover uma imagem específica restaura a imagem padrão, quando houver.
Salvar e confirmar aplica todas as variantes juntas. Trocar de estado durante
a leitura de um arquivo cancela aquela leitura, sem aplicar o arquivo em outra
variante. A personalização visual não muda quando o comando pode executar.
Versões anteriores sem suporte a `states` podem recusar essa configuração;
para testar uma versão antiga, use a cópia do banco feita antes dessas alterações.

**Ativar camada** e **Alternar camada** podem mostrar ligado/desligado conforme
o estado efetivo da camada-alvo. Não se trata do resultado da última pressão:
a camada pode permanecer ligada por outra regra ou origem. Quando não há prova
suficiente do estado, a tecla não inventa um indicador. **Voltar camada** e
comandos sem estado persistente não recebem esse indicador.

O resultado temporário de uma execução tem prioridade; depois, a tecla volta
à apresentação do estado persistente atual ou ao padrão. Mudanças conhecidas
de ligado/desligado são anunciadas usando o mesmo leitor de telas compartilhado,
com as mesmas verificações de sessão, mapa e foco. A primeira exibição não lê
todas as teclas em voz alta.

- [ ] Personalize o padrão e o estado concluído de uma tecla com títulos e
  imagens diferentes. Salve, reabra e confira a preservação; execute e confira
  o resultado temporário e o retorno ao padrão. Resultado: ________.
- [ ] Configure Alternar camada na camada de controle. Personalize ligado e
  desligado, ative/desative a camada-alvo e confira texto, imagem e anúncio.
  Confira também uma mudança feita pelas configurações. Resultado: ________.
- [ ] Remova somente a imagem de um estado e confirme que ele herda a imagem
  padrão, preservando as outras variantes. Resultado: ________.
- [ ] Com NVDA, percorra seletor de estados e campos por Tab; confira os nomes
  sem leitura de serial, digest ou identificadores técnicos. Resultado: ________.

Validação manual acumulada:

- [ ] Edite uma tecla, percorra os campos de título por Tab e confira seus
  rótulos com NVDA. Preencha português e inglês, salve e confirme.
- [ ] Reabra o acionador e confira os dois valores. Com a camada ativa, confira
  o título físico e troque o idioma das configurações para inglês.
- [ ] Apague o título em inglês, salve e confirme: a tecla deve voltar ao nome
  do comando em inglês, mantendo o título personalizado em português.
- [ ] Pressione a tecla antes e depois da edição: o comando e suas restrições
  de contexto devem permanecer iguais.
- [ ] Pelo teclado, escolha **Pasta** no campo **Ícone**, salve e confirme.
  Reabra o acionador: a seleção e os títulos devem estar preservados.
- [ ] Confira o ícone acima do título no dispositivo. Troque para **Conversa**
  e depois **Sem ícone**; confirme cada alteração. A ação deve continuar igual.
- [ ] Pelo teclado e NVDA, escolha uma imagem PNG/JPEG, salve e confirme.
  Confira a imagem acima do título e a execução da ação no dispositivo.
- [ ] Reabra o editor e confira a indicação de imagem associada. Troque a
  imagem, negue a confirmação e confira que a imagem anterior permanece.
- [ ] Remova a imagem, salve e confirme: o ícone escolhido deve reaparecer,
  mantendo título e comando. Reabra o aplicativo e confira a persistência.
- [ ] Escolha um arquivo maior que 1 MiB ou de outro formato: confira o erro
  anunciado e a preservação da associação anterior.
- [ ] Associe uma tecla a **Copiar mensagem**, abra uma conversa e selecione
  uma mensagem. Com o Assistente em foco e NVDA ligado, pressione a tecla:
  confira o resultado temporário e seu anúncio, sem repetição a cada atualização.
- [ ] Aguarde o retorno à apresentação normal. Desconecte e reconecte o
  dispositivo: o resultado anterior não deve reaparecer nem ser anunciado.
- [ ] Confira que teclas de navegação continuam rápidas e não anunciam uma
  conclusão inventada. Bloqueie/desbloqueie a estação: nenhum resultado antigo
  deve reaparecer após a reconstrução do mapa.

A captura não ativa camadas. Se outro programa estiver usando o aparelho,
libere o dispositivo antes de tentar novamente.

Crie uma camada, informe seu nome e salve. No detalhe, consulte **Quando esta
camada fica ativa** e **Comandos desta camada**. Habilitada e ativa são estados
diferentes: salvar ou habilitar uma camada não a ativa automaticamente.

No menu **Ações** da camada pessoal:

1. Escolha **Preparar ativação manual** e confirme a criação da regra.
   Esta etapa prepara a camada, mas ainda não a ativa.
2. Abra o menu novamente e escolha **Ativar manualmente**. Os acionadores
   elegíveis passam a participar da resolução dos comandos.
3. Use **Desativar ativação manual** para retirar a ativação desta origem.
   Uma camada pode continuar ativa se existir outra origem mantendo-a ativa.

A ativação manual é persistente. Desabilitar uma camada suspende seu efeito,
sem apagar a ativação; é possível retirar essa ativação mesmo desabilitada.
Preparar a regra exige confirmação; ativar/desativar usa essa regra já
preparada, sem criar uma nova confirmação de configuração a cada vez.

Ao configurar um acionador de teclado, escolha um comando compatível e use
o botão de captura. Pressione a combinação desejada; **Escape** cancela e
**Tab** sai da captura. A captura não executa o comando. Combinações com
AltGr, composição de texto e repetições não são convertidas em atalhos.

O teclado personalizado oferece **Listar workspaces** (`workspace.list`), que
abre o picker compartilhado, e os nove comandos de navegação: workspace,
histórico, memórias, listas de tarefas, jobs, perfis, configurações, ajuda e
sobre. Navegação, foco e ajuda usam a execução local da interface, sem registro
persistente por acionamento. A resolução respeita a configuração efetiva da
origem: suprimir uma entrada da paleta não desativa um teclado pessoal válido.
Use uma
combinação contendo **Control, Alt ou Meta**, em uma camada ativa, sem
conflito nem condições contextuais. Combinações sem esses modificadores não
entram no mapa operacional. Composição de texto ativa, AltGr e modais mantêm
prioridade. As exceções explícitas em input/textarea nativos são navegação e
criação/fechamento de abas no contexto autorizado; Monaco permanece protegido.

Além dos atalhos de navegação e criação/fechamento migrados acima, os demais
atalhos anteriores continuam por seus caminhos existentes. A sequência Ctrl+N
agora usa o mapa local. Esta ligação não habilita atalhos globais do Windows e não
configura o Stream Deck. Comandos apenas de paleta não ganham teclado
automaticamente. A tela informa o recorte operacional.

## Criar abas: Ctrl+N e menu Nova aba

### Lote de validação: paleta e Dados

Os comandos **Abrir paleta de comandos**, **Abrir exportação de dados** e
**Abrir importação de dados** podem ser associados ao teclado local e ao
Stream Deck. São ações de apresentação: não gravam uma invocação por tecla.
Os padrões são **Ctrl+K**, **Alt+E** e **Alt+I**, respectivamente. Podem ser
suprimidos ou substituídos por uma camada pessoal; sem o binding efetivo não
há execução pelo atalho antigo. O botão da paleta continua acessível.

Abrir importação inicia a seleção do arquivo e sua análise; a gravação dos
dados depende da confirmação no fluxo existente. Abrir exportação leva à
seção de exportação; não gera nem baixa um arquivo automaticamente.

Teste este lote junto dos cenários de Ctrl+N abaixo:

- [ ] Ctrl+K abre a paleta pelo picker compartilhado, inclusive no campo de
  mensagem, sem alterar o rascunho. Setas e Enter selecionam normalmente.
- [ ] Alt+E abre Dados na exportação sem gerar arquivo. Alt+I abre a seleção
  de arquivo; cancelar não importa dados.
- [ ] Suprimir o padrão Ctrl+K: a tecla deixa de abrir a paleta, mas seu botão
  continua funcionando. Restaurar recupera o atalho.
- [ ] Em camada pessoal ativa, associar Ctrl+K a Abrir histórico: a tecla
  abre somente Histórico. Desativar a camada recupera a paleta padrão.
- [ ] Associar uma tecla do Stream Deck a Abrir paleta de comandos e outra
  a Abrir exportação de dados: cada tecla executa apenas a ação configurada.
- [ ] Com modal aberto ou composição de texto ativa, esses atalhos não agem
  na interface de fundo. Conferir foco e anúncios com NVDA.

### Lote de validação: criação de abas

No workspace, pressione **Ctrl+N**, solte as teclas e pressione **C** (chat),
**E** (editor), **R** (terminal) ou **T** (lista de tarefas) em até 1,5 segundo.
O prefixo não cria nada: somente a escolha final executa o comando. Escape,
perda de foco da janela ou mudança de contexto cancelam a sequência.

O botão **Nova aba** usa os mesmos comandos pelo fluxo da paleta. É possível
escolher com as setas e Enter ou clicar. Ao começar a navegar pelo menu com
as setas, a espera pela segunda tecla termina; o menu continua disponível.
Campos especializados, grids, composição de texto e modais mantêm prioridade.
O Ctrl+N específico das telas de cadastro não foi substituído.

As quatro sequências aparecem em **Mapa de teclado padrão** e podem ser
suprimidas/restauradas individualmente. A captura de atalhos pessoais também
grava sequências; use **Tipo de gravação → Sequência de duas etapas**.

Conferência manual pendente (registrar passou/falhou para cada item):

- [ ] Ctrl+N, C cria exatamente uma aba de chat; repetir com E, R e T.
- [ ] Ctrl+N, Escape não cria aba; Ctrl+N, esperar mais de 1,5 segundo, C
  também não cria.
- [ ] Ctrl+N, seta para baixo, Enter executa somente o item anunciado pelo NVDA.
- [ ] Clicar em Nova aba e escolher Editor cria uma única aba de editor.
- [ ] Suprimir Ctrl+N, C no mapa padrão: a sequência não cria chat; restaurar
  recupera o comportamento. O botão Nova aba continua oferecendo chat.
- [ ] Abrir um modal ou trocar de aba durante a escolha cancela a operação.
- [ ] Em uma tela de cadastro com Ctrl+N próprio, confirmar que a criação
  continua pertencendo àquela tela, sem abrir uma aba do workspace.

## Conferência do teclado personalizado

### Padrões migrados e disponibilidade da paleta

- [ ] Reiniciar pelo fluxo habitual de desenvolvimento e abrir **Comandos**.
  Com sessão e runtime prontos, deve anunciar **43 comandos**. A quantidade
  disponível depende da interface: **Focar painel ativo** fica indisponível
  fora do workspace e é revalidado antes de executar.
- [ ] Trocar de aba ou perfil e abrir novamente a paleta: não deve ficar
  permanentemente em zero disponíveis. Selecionar **Abrir histórico** executa.
- [ ] Sem camada pessoal ativa, fora de campos editáveis e modais, testar
  **Alt+H**, **Alt+C** e **Alt+W**. Cada pressionamento navega uma única vez.
- [ ] Em **Comandos e acionadores → Mapa de teclado padrão**, suprimir
  o padrão **Alt+H** e confirmar. Fora do editor, Alt+H não deve navegar.
- [ ] Restaurar esse padrão e confirmar: Alt+H volta a funcionar sem criar
  uma camada pessoal. Com modal aberto ou campo em edição, não navega.
- [ ] Criar na camada pessoal um acionador **Alt+H → Abrir memórias**,
  salvar, confirmar e ativar a camada: abre Memórias. Desativar a camada
  restaura o destino padrão Histórico. Remover a personalização após o teste,
  se ela não for desejada.

### Combinações pessoais

Depois de atualizar os bindings pelo seu fluxo habitual de desenvolvimento:

- [ ] Criar uma camada pessoal habilitada e selecionar essa camada.
- [ ] Criar um acionador para **Listar workspaces**, escolher teclado local,
  capturar **Ctrl+Shift+K**, salvar e confirmar. Enquanto a camada estiver
  inativa, a combinação não deve abrir o picker.
- [ ] No menu Ações da camada, **Preparar ativação manual**, confirmar e
  depois **Ativar manualmente**. Sair do campo de edição e fechar os modais.
- [ ] Pressionar **Ctrl+Shift+K**. Esperado: picker de workspaces, pelo mesmo
  componente compartilhado do aplicativo; selecionar outro workspace troca
  para ele. A paleta de comandos não é uma etapa intermediária.
- [ ] Manter a tecla pressionada. Esperado: uma execução por pressionamento;
  soltar e pressionar novamente permite outra execução.
- [ ] Testar com campo de texto em foco e com modal aberto. Esperado: nenhuma
  abertura do picker por esse atalho.
- [ ] Desativar a ativação manual da camada. Esperado: o atalho deixa de abrir
  o picker, sem reiniciar. Reativar restaura o comportamento.
- [ ] Alternar para outra janela e voltar. Esperado: sem tecla presa ou
  abertura atrasada; um novo pressionamento funciona no contexto atual.
- [ ] Conferir foco, navegação e anúncios com NVDA no picker; confirmar que
  Ctrl+N e Ctrl+Tab funcionam pelo novo mapa.

Esses itens são aceite manual pendente; os testes automatizados não marcam
essa conferência como realizada.

## Conferência adicional da navegação

- [ ] Pelo fluxo habitual Wails, atualizar os bindings e abrir o app.
- [ ] Em **Configurações → Comandos e acionadores**, na camada de teste,
  criar um acionador de teclado local para `navigation.history.open`,
  capturando **Ctrl+Shift+H**; salvar e confirmar.
- [ ] Ativar manualmente a camada, sair do campo de edição e fechar modais.
  Pressionar **Ctrl+Shift+H**: deve abrir Histórico, sem abrir a paleta.
- [ ] Ir a outra página e repetir: deve voltar ao Histórico, sem execução
  duplicada nem erro causado pela própria troca de página.
- [ ] Desativar a camada e repetir: a combinação personalizada não deve
  navegar. Reativar e testar em campo editável e modal: não deve navegar.
- [ ] Repetir o teste com outro destino, por exemplo
  `navigation.settings.open` em **Ctrl+Shift+J**, usando uma combinação
  que não esteja reservada pelo ambiente. Conferir foco e anúncio com NVDA.

Esses novos cenários ainda dependem de aceite no aplicativo; a confirmação
anterior do usuário cobriu criação, ativação e execução do primeiro atalho.

## Confirmação, segurança e resultados

- Uma falha ao carregar esta página não significa que o teclado foi
  desabilitado: a disponibilidade só é exibida após carregar a configuração.
  Use **Recarregar** para tentar novamente. Se persistir após reiniciar,
  consulte o diagnóstico `app.commands.storage` no log; não apague camadas,
  recibos ou chaves para contornar o erro.
- Corrigida a recusa no startup de assinaturas canônicas gravadas pelas
  confirmações de configuração. Camadas e atalhos já salvos são preservados;
  não é necessário recriá-los. Chaves ausentes/incompatíveis continuam
  impedindo a execução por segurança.
- Alterações de configuração usam a confirmação real do aplicativo e a
  persistência autenticada. Negar a confirmação não aplica a alteração.
- Registros com opções avançadas não suportadas são somente leitura; o
  editor não apaga condições ou argumentos para conseguir editá-los.
- A mensagem de configuração salva, mas não publicada, significa que a
  gravação ocorreu e a reconstrução do mapa operacional falhou. Não repita
  automaticamente a operação; recarregue a tela para conferir o estado.
- Importação e exportação continuam na página de Dados. Extensões avançadas
  desses fluxos foram adiadas; não são requisito para inspecionar os padrões.

Durante o desenvolvimento, os novos métodos do backend exigem regenerar os
bindings pelo fluxo normal do Wails antes de abrir esta tela. Não edite os
arquivos gerados manualmente.

## Arquivos do editor — Ctrl+S, Ctrl+O e Ctrl+Shift+S

Os três comandos fazem parte do catálogo configurável e compartilham o
mesmo executor pelo teclado, paleta, menu Arquivo e Stream Deck:

- **Ctrl+O — Abrir arquivo:** seleciona a aba do arquivo se já estiver aberta
  ou cria outra. Não substitui o conteúdo nem as edições de uma aba existente.
- **Ctrl+S — Salvar arquivo:** grava o documento da aba capturada. No primeiro
  salvamento, pergunta o destino.
- **Ctrl+Shift+S — Salvar cópia:** grava em outro destino sem transformar a
  cópia no documento original nem limpar suas alterações pendentes.

São ações do editor ativo, não atalhos globais do sistema operacional.
Mudanças de aba, workspace ou sessão durante a preparação cancelam a ação;
o diálogo nativo pode retirar o foco sem trocar o alvo. Selecionar um destino
existente no primeiro salvamento ou em Salvar cópia exige confirmação de
sobrescrita. Cancelar não grava. O prazo total do diálogo é de cinco minutos.

A escrita verifica novamente o arquivo observado e usa um temporário antes
da substituição. Uma alteração detectada exige reiniciar a operação. Isso
não equivale a um bloqueio de todos os outros programas que editam arquivos.
Se o desfecho for incerto, confira o arquivo antes de repetir a gravação.
Edições locais posteriores ao conteúdo enviado continuam marcadas como pendentes.

### Aceite manual deste lote

- [ ] Num documento de teste, Ctrl+S grava; editar novamente mantém o indicador
  de alterações até o próximo salvamento.
- [ ] Num documento novo, Ctrl+S pergunta o caminho; cancelar não altera a aba.
- [ ] Ctrl+O abre outro arquivo e preserva o documento anterior; abrir de novo
  um arquivo já aberto não descarta suas edições.
- [ ] Ctrl+Shift+S cria uma cópia; o caminho e as alterações do original ficam
  intactos. Recusar sobrescrita preserva o destino.
- [ ] Repetir Abrir/Salvar/Salvar cópia pela paleta, menu e Stream Deck.
- [ ] Remapear ou suprimir um atalho e conferir que a tecla antiga não executa
  o listener anterior. Em chat/terminal, os defaults do editor não executam.
- [ ] Conferir foco, anúncios NVDA e confirmação acessível de sobrescrita.

Este aceite físico permanece pendente. Reinicie o desenvolvimento pelo fluxo
normal do Wails para carregar a nova API; arquivos gerados não foram editados
manualmente, e os testes automatizados não iniciam o aplicativo.

## Formatação do editor rico

Negrito (Ctrl+B), Itálico (Ctrl+I) e Tachado (Ctrl+Shift+X) usam o novo
mecanismo de comandos. Também podem ser acionados pela paleta, pelo menu
Formatar e por uma tecla configurada no Stream Deck. Os atalhos exigem foco
no editor rico; não formatam a partir do chat, terminal ou outro campo.
Paleta, menu e Deck usam a seleção do editor ativo. Se o documento ou a
seleção mudar durante a preparação, a ação é cancelada em vez de atingir
outro trecho. Sem seleção, a marca vale para a digitação seguinte.
O registro da ação não contém o texto do documento ou da seleção.

### Validação manual acumulada — formatação

- [ ] Selecionar texto no modo rico e alternar Ctrl+B, Ctrl+I e Ctrl+Shift+X;
  confirmar que apenas o trecho selecionado mudou, uma vez por acionamento.
- [ ] Selecionar texto, abrir a paleta e executar Negrito; conferir a seleção.
- [ ] Repetir Itálico/Tachado pelo menu Formatar e por uma tecla do Stream Deck.
- [ ] Remapear/suprimir Ctrl+B e confirmar que o atalho nativo antigo não atua.
- [ ] Em chat, Markdown, visualização e documento somente leitura, verificar
  que estes comandos não alteram o documento rico em segundo plano.
- [ ] Conferir foco e leitura pelo NVDA após fechar paleta/menu.

Aceite manual pendente. A expansão abaixo acrescenta outras ações de edição.

## Parágrafos, blocos e edição de tabelas

O novo mecanismo também oferece:

- Parágrafo (Ctrl+Alt esquerdo+0) e definir título H1 a H6
  (Ctrl+Alt esquerdo+1 a 6). Para voltar a texto normal, use Parágrafo.
- Citação (Ctrl+Shift+B), bloco de código (Ctrl+Alt esquerdo+C), lista
  numerada (Ctrl+Shift+7) e lista com marcadores (Ctrl+Shift+8).
- Limpar formatação de texto e remover link; ao posicionar o cursor dentro
  de um link, Remover link retira o vínculo completo, preservando o texto.
- Adicionar/remover linhas e colunas, alternar cabeçalhos de linha/coluna/célula,
  mesclar/separar células e apagar a tabela atual.

Todas podem ser executadas pela paleta, pelo menu Formatar ou por uma tecla
configurada no Stream Deck. Ações sem atalho padrão aceitam atalhos personalizados.
Comandos de tabela ficam indisponíveis fora de uma tabela; Mesclar/Separar
dependem da seleção e da estrutura. A paleta conserva a seleção de células.
Somente o editor rico ativo e editável recebe essas alterações.

Para os atalhos Ctrl+Alt, pressione Ctrl e **Alt esquerdo** antes da tecla
final. AltGr não é aceito como substituto. A gravação de atalhos usa a mesma
distinção. Após trocar de janela ou recarregar o mapa, solte e pressione os
modificadores novamente. Não há fallback para o keymap nativo removido.

### Validação manual acumulada — blocos e tabelas

- [ ] Em um documento de teste, usar Ctrl+Alt esquerdo+1 e depois +0 para
  aplicar H1 e retornar a parágrafo; conferir também H2–H6.
- [ ] Alternar citação, código e as duas listas pelos atalhos acima.
- [ ] Repetir ações por paleta, menu e Stream Deck; conferir seleção e NVDA.
- [ ] Em uma tabela existente, adicionar/remover linha e coluna, alternar
  cabeçalhos, selecionar células e mesclar/separar pela paleta.
- [ ] Em uma cópia de teste, apagar a tabela; fora dela, ações de tabela
  ficam indisponíveis e não alteram outro documento.
- [ ] Remover link com cursor dentro do vínculo e limpar marcas de um trecho;
  confirmar que o texto foi preservado.
- [ ] Remapear/suprimir um dos novos atalhos; o default antigo não deve atuar.
  AltGr e composição de texto não devem disparar esses comandos.

## Links, inserções e navegação entre células

No editor rico, a paleta oferece **Definir link**, **Inserir tabela**,
**Inserir bloco de código**, **Inserir diagrama Mermaid**, **Próxima célula**
e **Célula anterior**. Não há novas teclas padrão para essas ações: é possível
atribuir atalhos ou teclas do Stream Deck nas configurações de comandos.
Ctrl+K continua abrindo a paleta, inclusive dentro do editor.

Definir link usa o formulário compartilhado: com texto selecionado, aplica
o vínculo à seleção; com o cursor dentro de um link, altera seu endereço;
sem seleção nem link atual, permite informar o texto a inserir. Inserir tabela
pergunta linhas e colunas (de 2 a 6) e se deve haver cabeçalho. Pelo menu
Inserir, as dimensões já escolhidas são aproveitadas, sem perguntar novamente.
Cancelar o formulário não altera o documento. Mudar o documento, a seleção
ou o contexto durante a preparação impede a edição.

Próxima célula e Célula anterior apenas movem a seleção dentro da tabela
atual: não criam linhas e não registram navegação no banco. Isso é diferente
do Tab nativo do editor, que pode acrescentar uma linha no fim da tabela.
Ao manter um atalho personalizado de navegação pressionado, a repetição
percorre as células existentes e para no limite; ações de conteúdo não repetem.
As demais ações desta seção alteram conteúdo e usam o fluxo auditado, sem
incluir endereço do link, texto ou conteúdo do documento no registro.

### Validação manual acumulada — inserções e células

- [ ] Selecionar texto, abrir Ctrl+K e executar Definir link; preencher
  `https://example.com` e conferir que apenas o texto selecionado recebeu link.
- [ ] Posicionar o cursor dentro do link e repetir Definir link com outro
  endereço; conferir que o texto não foi duplicado. Cancelar uma terceira
  tentativa e confirmar que o endereço anterior foi preservado.
- [ ] Sem seleção nem link atual, inserir um link com texto informado.
- [ ] Executar Inserir tabela pela paleta: escolher 2 linhas, 3 colunas e
  cabeçalho; conferir a estrutura. Repetir pelo menu Inserir com 3 por 2.
- [ ] Executar Próxima célula e Célula anterior pela paleta e por teclas
  personalizadas; nos limites, conferir que nenhuma linha foi acrescentada.
- [ ] Inserir código e Mermaid pelo menu e pela paleta; conferir também
  listas e citação pelo menu Inserir.
- [ ] Repetir Definir link e Inserir tabela por uma tecla do Stream Deck;
  confirmar que cada confirmação produz apenas uma alteração.
- [ ] Conferir foco, cancelamento e leitura dos formulários pelo NVDA.

Aceite manual pendente. O lote seguinte também migra Markdown e templates,
conforme a seção abaixo.

## Inserções Markdown e templates de slides

No editor em modo **Markdown**, a paleta, o menu Inserir e os acionadores
personalizados agora compartilham os comandos de tabela, bloco de código,
Mermaid, lista com marcadores, lista numerada e citação. O editor precisa
estar pronto e editável; não existe uma inserção alternativa silenciosa
quando o Monaco ainda não montou. Tabelas Markdown sempre têm cabeçalho;
o formulário pede somente as dimensões (de 2 a 6 linhas/colunas).

Onze comandos **Inserir slide…** estão disponíveis: básico, título, duas
colunas, imagem à direita/esquerda, seção, agenda, citação, comparação,
código e diagrama. Eles acrescentam um slide ao **fim do documento completo**,
com separação Reveal, tanto no modo Markdown quanto no rico. O botão de
novo slide da barra executa o mesmo comando de slide básico.

Não há novas teclas padrão neste lote. É possível executar pela paleta ou
atribuir uma combinação/tecla do Stream Deck em Comandos e acionadores.
No Markdown, os atalhos já configurados para listas/citação também passam
a usar essas ações. A disponibilidade continua contextual: não afeta chat,
visualização, documentos somente leitura ou outro editor.

O alvo é capturado antes da paleta/formulário. Se a aba, sessão, documento,
modelo ou seleção mudar enquanto o comando aguarda execução, ele é cancelado,
sem editar a nova seleção. Ações de conteúdo não repetem automaticamente.
O texto do documento e os dados do formulário não são enviados como
argumentos da auditoria.

### Validação manual acumulada — Markdown e slides

- [ ] Em Markdown, selecionar texto e executar Lista com marcadores,
  Lista numerada e Citação; conferir que o alvo correto foi alterado.
- [ ] Inserir tabela de 3 linhas e 2 colunas pela paleta e pelo menu;
  conferir o cabeçalho obrigatório e desfazer a operação.
- [ ] Inserir código e Mermaid; verificar a sintaxe Markdown/renderização.
- [ ] Executar os onze templates Inserir slide pela paleta/menu; conferir
  que acrescentam ao fim sem apagar os slides anteriores.
- [ ] No modo rico de uma apresentação, alterar o slide atual e criar
  outro pela barra; conferir que a edição anterior foi preservada.
- [ ] Associar Lista com marcadores e Inserir slide básico a teclas do
  Stream Deck; confirmar execução única no contexto adequado.
- [ ] Cancelar o formulário de tabela, testar documento somente leitura e
  conferir foco e anúncios do NVDA. Aceite ainda pendente.

## Limpar conversa

O botão **Limpar conversa**, **Ctrl+L**, a paleta e uma tecla configurada do
Stream Deck agora executam o mesmo comando. Ctrl+L pode ser personalizado em
Comandos e acionadores; funciona também no campo de envio da conversa ativa.
Não atua sobre campos de outros formulários, terminais ou editores.

A limpeza pede confirmação pelo diálogo de decisão compartilhado. Confirmar
remove mensagens, resumo e registros de ferramentas daquela conversa; não
exclui a conversa nem a aba. Cancelar preserva o conteúdo. Use uma conversa
de teste: a limpeza confirmada não possui desfazer.

O alvo fica fixado ao iniciar a ação. Trocar de aba, sessão ou conversa não
redireciona a limpeza. Se o conteúdo mudar enquanto a confirmação está aberta,
a operação é recusada; reveja a conversa antes de tentar novamente. Uma
geração ativa ou pendente também impede a limpeza. No chat contextual de
outra aba, o modal de chat precisa estar aberto e ser o modal ativo.

### Validação manual acumulada — Limpar conversa

- [ ] Em uma conversa descartável com mensagens, pressionar Ctrl+L no
  campo de envio, cancelar e conferir que as mensagens permanecem.
- [ ] Repetir e confirmar; conferir a conversa vazia, aba preservada e foco
  no campo de envio. Resultado: ____________________.
- [ ] Repetir em conversas descartáveis pelo botão, pela paleta procurando
  **Limpar conversa**, e por uma tecla configurada do Stream Deck.
- [ ] Abrir o chat contextual de editor/terminal/tasklist e repetir; conferir
  que somente a conversa vinculada foi limpa.
- [ ] Durante uma resposta, conferir que a limpeza fica indisponível; conferir
  também que Ctrl+L não limpa a conversa ao focar outro campo/editor/terminal.
- [ ] Conferir anúncio, leitura e cancelamento do diálogo pelo NVDA.

Aceite manual ainda pendente; não exige executar os testes anteriores agora.

## Consultar mensagens fixadas e estatísticas de tokens

Na paleta, procure **Mensagens fixadas** ou **Estatísticas de tokens** para
abrir as mesmas telas acessíveis pelos botões da conversa. Os comandos são
`chat.pinned.open` e `chat.tokens.open`. Você também pode associá-los a uma
combinação em Comandos e acionadores ou a uma tecla do Stream Deck; este lote
não atribui novas teclas padrão.

Essas ações apenas abrem a consulta, sem confirmação destrutiva e sem gravar
uma invocação por tecla. A consulta pertence à conversa ativa: não funciona
fora de um chat disponível nem atravessa um diálogo sobreposto. O chat modal
contextual também é aceito quando é o modal ativo. Trocar de conversa, aba ou
sessão invalida a consulta, sem redirecioná-la para outro histórico.

Abrir Mensagens fixadas não fixa nem desfixa mensagens. As ações internas dos
modais mantêm seus contratos próprios e não foram migradas neste lote.

### Validação manual acumulada — consultas da conversa

- [ ] Na conversa de teste, abrir Mensagens fixadas pelo botão e pela paleta;
  conferir que ambos mostram a mesma conversa. Resultado: ______________.
- [ ] Abrir Estatísticas de tokens pelo botão e pela paleta; conferir os
  mesmos dados e a navegação das abas pelo NVDA. Resultado: ______________.
- [ ] Configurar uma tecla de teclado e uma tecla do Stream Deck para cada
  comando; conferir abertura única, sem repetição ao manter a tecla pressionada.
- [ ] Repetir no chat contextual e conferir retorno do foco ao fechar.
- [ ] Com outro diálogo sobreposto, conferir que os acionadores não abrem
  consultas no chat que ficou atrás. Aceite manual pendente.

## Enviar, cancelar e tentar novamente no chat

O lote90 conecta os controles do chat a três comandos:

- **Enviar mensagem** (`chat.message.send`): envia o rascunho da conversa,
  com seus anexos, pelo mesmo pipeline de envio já usado no aplicativo.
- **Cancelar resposta** (`chat.response.cancel`): solicita o cancelamento
  da resposta capturada; não deve cancelar uma resposta posterior.
- **Tentar mensagem novamente** (`chat.message.retry`): reutiliza uma
  mensagem de usuário já persistida. No menu de mensagem, o alvo é a mensagem
  escolhida; na paleta, o turno interrompido elegível daquela conversa.
  Sem alvo elegível, o comando fica indisponível.

Enter no campo de envio e Escape durante a resposta continuam sendo ações
do próprio campo. Shift+Enter mantém a quebra de linha; o menu de comandos
iniciado por `/` tem prioridade sobre o envio. Este lote não acrescenta
combinações globais padrão. É possível associar os comandos a uma combinação
personalizada ou a uma tecla do Stream Deck.

São operações com efeitos reais, com autorização e resultado no backend,
diferentemente de navegar entre abas. O texto e os anexos não são argumentos
salvos na auditoria de comandos. A conversa fica vinculada à solicitação;
trocar de contexto não deve redirecionar a ação para outro chat.

O rascunho é preservado em recusa. Se houver aviso de resultado desconhecido,
confira o histórico antes de reenviar: a mensagem pode ter sido aceita pelo
backend mesmo que a confirmação não tenha chegado à interface. O aplicativo
não repete automaticamente um envio incerto.

### Validação manual acumulada — envio, cancelamento e nova tentativa

Use uma conversa de teste e mensagens curtas. Enviar pode consumir o provedor
de IA configurado; não é necessário realizar esses testes antes dos demais
lotes. Aceite manual ainda pendente.

- [ ] Enviar pelo botão e por Enter; conferir uma única mensagem e a limpeza
  do rascunho confirmado. Resultado: ____________________.
- [ ] Digitar texto com Shift+Enter e escolher um item do menu `/`; conferir
  que essas interações não enviam prematuramente. Resultado: ____________.
- [ ] Deixar um rascunho com anexo e executar **Enviar mensagem** pela paleta;
  conferir texto, anexo e conversa de destino. Resultado: ______________.
- [ ] Configurar teclado e Stream Deck para os três comandos e testar;
  manter a tecla pressionada não deve duplicar o envio/retry.
- [ ] Durante uma resposta, usar Escape no campo e depois repetir usando
  **Cancelar resposta** pela paleta/Deck. Resultado: ____________________.
- [ ] No turno interrompido, executar **Tentar mensagem novamente**; conferir
  que não aparece outra cópia da mensagem de usuário. Testar também a ação
  de reenviar uma mensagem específica em seu menu. Resultado: __________.
- [ ] Repetir envio/cancelamento no chat contextual de outra aba. Abrir um
  diálogo sobreposto e conferir que não há envio no chat atrás dele.
- [ ] Trocar de aba/conversa durante uma solicitação pendente; conferir que
  ela não atinge a nova conversa e que o rascunho não enviado permanece.
- [ ] Conferir os anúncios e a navegação com NVDA, inclusive erro de validação
  e cancelamento. Resultado: ____________________.

## Ações sobre uma mensagem

O lote91 conecta estas ações à mensagem escolhida no chat:

- **Copiar mensagem**: copia o texto sem a marcação Markdown.
- **Copiar mensagem como Markdown**: preserva a marcação.
- **Ler mensagem em voz alta**: usa o serviço de voz já configurado.
- **Abrir edição da mensagem**: abre a edição existente. Abrir não salva alterações e
  não grava uma invocação na auditoria. Para persistir, use **Salvar edição da mensagem**,
  descrito abaixo.
- **Alternar fixação da mensagem**: fixa ou desfixa a mensagem capturada.
- **Excluir mensagem e respostas**: pede confirmação antes de remover a
  mensagem e as respostas abrangidas pela exclusão existente.

Para usar a paleta ou um acionador configurado, primeiro dê foco à mensagem
na lista. Pelo menu de contexto, o alvo é a própria mensagem desse menu.
Sem mensagem elegível, a ação fica indisponível: o aplicativo não escolhe a
última mensagem por conta própria. Uma mensagem em geração não é um alvo
elegível para estas ações. Não são acrescentadas combinações globais padrão.

A captura não é redirecionada para outra mensagem durante uma espera.
Exclusão e fixação são confirmadas pelo backend; texto, áudio e conteúdo da
área de transferência não são gravados como argumentos da auditoria. Se o
resultado de uma alteração for desconhecido, confira a conversa antes de
tentar novamente, principalmente ao alternar a fixação.

### Validação manual acumulada — ações sobre mensagens

Use uma conversa descartável, especialmente para a exclusão. A leitura em
voz alta pode usar o provedor TTS configurado. Gere os bindings oficiais
antes de validar este lote; autorização/geração ainda pendentes.

- [ ] Dar foco à mensagem A e abrir a paleta; executar **Copiar mensagem**.
  Colar em um editor e conferir que foi A. Resultado: ____________________.
- [ ] Repetir com **Copiar mensagem como Markdown** e conferir a marcação.
  Resultado: ____________________.
- [ ] Executar **Abrir edição da mensagem**; conferir que abre a edição de A e que
  cancelar não altera seu conteúdo. Resultado: ____________________.
- [ ] Executar **Ler mensagem em voz alta**. Trocar de conversa enquanto o
  áudio ainda está sendo preparado e conferir que não começa uma fala tardia.
  Resultado: ____________________.
- [ ] Executar **Alternar fixação da mensagem** duas vezes separadas,
  conferindo fixação e desfixação na mesma mensagem. Resultado: __________.
- [ ] Executar **Excluir mensagem e respostas**, cancelar a confirmação e
  conferir que nada foi removido. Repetir e confirmar só na conversa de teste.
  Resultado: ____________________.
- [ ] Repetir pelo menu de contexto, teclado configurável e Stream Deck;
  conferir uma execução por acionamento. Resultado: ____________________.
- [ ] Repetir no chat contextual, conferir anúncios NVDA e garantir que um
  diálogo sobreposto impede ações na mensagem que ficou atrás.
  Resultado: ____________________.

O envio ao editor tem contrato próprio, descrito em **Enviar mensagem para o
editor** abaixo, incluindo recortes de código/tabelas/links. Outras operações
internas desses blocos não são declaradas migradas por este lote.

## Salvar edição da mensagem

O comando `chat.message.edit.save` salva o texto da edição aberta na mensagem
selecionada. O botão **Salvar** e **Ctrl+Enter**, dentro dessa edição, usam o
mesmo fluxo da paleta e dos acionadores configurados. Ctrl+Enter continua
um controle local do formulário, não um novo atalho global padrão.

Na paleta, procure **Salvar edição da mensagem** depois de abrir a edição e
alterar o texto. Para teclado personalizado ou Stream Deck, atribua esse
mesmo comando em **Comandos e acionadores**. Sem edição aberta elegível, ele
fica indisponível; não edita a última mensagem nem o campo de mensagem nova.

A alteração só é concluída após confirmação do backend. Uma atualização
concorrente da mensagem impede sobrescrever sua base original. Texto vazio
e mensagem interna ou do assistente não são alvos deste comando. Durante
uma geração ativa na conversa, aguarde o término antes de salvar; o comando
não cancela a resposta em andamento. Texto e metadados da mensagem não são
copiados para os argumentos do histórico de comandos.

Se o resultado ficar desconhecido, confira a mensagem antes de repetir. O
aplicativo não repete o salvamento automaticamente. O aceite manual deste
lote e a geração oficial dos bindings permanecem pendentes.

### Validação manual acumulada — salvar edição

- [ ] Em uma conversa descartável, focar uma mensagem sua e pressionar **F2**
  (ou usar **Abrir edição da mensagem** na paleta). Alterar o texto e clicar
  em **Salvar**. Conferir o texto atualizado na lista. Resultado: __________.
- [ ] Reabrir a edição, alterar o texto e pressionar **Ctrl+Enter**. Conferir
  que salvou sem enviar uma nova mensagem. Resultado: __________.
- [ ] Reabrir, alterar o texto e usar **Salvar edição da mensagem** pela
  paleta (**Ctrl+K**). Repetir com teclado configurado e Stream Deck.
  Resultado: __________.
- [ ] Manter Ctrl+Enter pressionado e conferir que não produz salvamentos
  repetidos. Cancelar outra edição e conferir texto original intacto.
  Resultado: __________.
- [ ] Sem edição aberta, conferir que o comando não atua no campo de envio
  nem escolhe uma mensagem por conta própria. Resultado: __________.
- [ ] Repetir no chat contextual e com NVDA: anúncio de sucesso, foco de
  retorno e leitura do texto atualizado. Resultado: __________.

## Enviar mensagem para o editor

O comando `chat.message.send_to_editor`, pela paleta, teclado configurável
ou Stream Deck, envia a mensagem selecionada inteira em Markdown para um
novo documento. Não há uma nova tecla padrão. Sem uma mensagem elegível
capturada, não escolhe automaticamente a mensagem mais recente.

Nos menus da mensagem e dos blocos, continuam disponíveis os recortes de
código, tabelas e links, os formatos oferecidos e a escolha entre documento
existente e novo documento. A escolha preserva a mensagem original, o conteúdo,
o formato e o destino capturados ao abrir o menu, sem trocar de fonte ao clicar.

O sucesso depende da aplicação real no editor: criar/ativar a aba ou enfileirar
conteúdo não significa que a transferência terminou. A transição verifica a
origem e o destino; mudanças de contexto ou destino indisponível não redirecionam
o conteúdo para outro documento. Não há repetição automática em caso de falha.

Se o resultado for desconhecido, **confira as abas e o conteúdo do documento
antes de tentar novamente**. Uma aba já criada pode permanecer, e a ausência
de confirmação não prova que nada foi aplicado. Repetir sem conferir pode
duplicar conteúdo ou criar outra aba.

Ao reabrir um rascunho sem arquivo associado, o editor restaura o autosave.
Rascunho vazio ou inexistente abre vazio, sem texto de exemplo. Outras falhas
de leitura deixam o documento somente leitura para evitar sobrescrever o
conteúdo salvo.

### Validação manual acumulada — transferência para o editor

- [ ] Selecionar uma mensagem, executar pela paleta e conferir o Markdown
  integral em um novo documento. Resultado: __________.
- [ ] Repetir por botão/menu, associação pessoal de teclado e Deck; conferir
  uma única transferência por acionamento. Resultado: __________.
- [ ] Enviar código, tabela e link pelos menus, testando os formatos oferecidos
  e destinos novo/existente; conferir recorte e documento. Resultado: __________.
- [ ] Abrir um menu/paleta para A e alterar o contexto antes de executar:
  não deve transferir B nem escolher a mensagem mais recente. Resultado: __________.
- [ ] Repetir no chat contextual, com destino ainda montando e com destino
  somente leitura; conferir transição, recusa e anúncios do NVDA. Resultado: __________.
- [ ] Em falha ou resultado desconhecido, conferir que não há retry automático
  e inspecionar eventual aba criada antes de repetir. Resultado: __________.

Aceite manual e geração/validação dos bindings oficiais permanecem pendentes.
O lote93 não representa a conclusão integral da migração de comandos.

## Editar e remover diagramas Mermaid

Selecione um bloco Mermaid no editor Markdown ou rico, ou foque seu bloco
no preview editável. Na paleta (**Ctrl+K**), procure **Editar diagrama Mermaid**
(`editor.mermaid.open`). Os botões e gestos existentes do diagrama abrem o
mesmo editor. Abrir não gera um registro de alteração no histórico.

Dentro do modal, **Aplicar**, **Ctrl+S**, **Cmd+S** ou **Ctrl+Enter** aplicam
o rascunho pelo comando `editor.mermaid.apply`. **Escape** cancela. Aplicar
só fica disponível com uma alteração e com o próprio modal em primeiro plano;
a paleta não abre por trás dele. Fora desse modal, Ctrl+S continua salvando
o documento, sem aplicar um diagrama.

Esses três atalhos internos de confirmação são controles fixos do formulário,
não novas associações globais editáveis. É possível criar outras associações
pessoais para os comandos; isso não remapeia esses controles internos.

**Remover diagrama Mermaid** (`editor.mermaid.remove`) funciona para o bloco
selecionado ou pelo botão do modal e exige confirmação. Cancelar a confirmação
preserva o diagrama. No editor rico, **Delete**, **Backspace** e **Shift+Delete**
sobre o preview usam essa mesma confirmação; Shift+Delete não remove mais
diretamente. Para Stream Deck ou associação pessoal de teclado,
escolha esses mesmos comandos em **Comandos e acionadores**. Aplicar exige
uma sessão de edição aberta; abrir e remover exigem um bloco elegível.

Aplicar/remover fecham o próprio modal antes da execução auditada. O alvo
permanece o bloco capturado: mudanças no documento, na instância do editor,
na aba ou na sessão impedem aplicar em outro lugar. Documento somente leitura
não é elegível. O texto Mermaid não é copiado para argumentos do histórico.
Se houver resultado desconhecido, confira o conteúdo antes de repetir;
não há repetição automática do efeito.

### Validação manual acumulada — Mermaid

- [ ] Em documento descartável, inserir dois blocos Mermaid pelo menu
  **Inserir**; selecionar o segundo e executar **Editar diagrama Mermaid**
  pela paleta. Confirmar que abriu o segundo. Resultado: __________.
- [ ] Alterar o segundo diagrama e clicar **Aplicar**. Repetir com **Ctrl+S**
  e **Ctrl+Enter**; conferir que só o bloco escolhido mudou e que nenhuma
  ação é repetida ao manter a tecla pressionada. Resultado: __________.
- [ ] Repetir no editor rico e no preview editável; conferir retorno de foco
  e leitura com NVDA. **Escape** deve descartar a edição. Resultado: __________.
- [ ] Usar **Remover diagrama Mermaid** e cancelar a confirmação: conteúdo
  intacto e foco de retorno. Repetir confirmando: só aquele bloco removido.
  Resultado: __________.
- [ ] No preview do bloco do editor rico, repetir com **Delete**, **Backspace**
  e **Shift+Delete**: todos devem pedir confirmação antes de remover.
  Resultado: __________.
- [ ] Associar **Editar diagrama Mermaid** e **Aplicar diagrama Mermaid**
  a teclas do Stream Deck, selecionando os comandos pelos IDs acima se o
  rótulo variar com o idioma. Abrir, alterar e aplicar uma única vez.
  Resultado: __________.
- [ ] Fora do modal, conferir **Ctrl+S** salvando o documento normalmente.
  Em documento somente leitura ou sem bloco selecionado, conferir recusa
  sem alterar outro documento. Resultado: __________.

Geração/validação dos bindings oficiais e aceite manual permanecem pendentes.
Os testes automatizados do Monaco usam uma fixture da API, não substituem
o teste do editor real no aplicativo.

## Navegar entre regiões da interface

**F6** avança para a próxima região da tela e **Shift+F6** volta para a
anterior: por exemplo, barra de navegação, barra de ferramentas, abas e
conteúdo. A ordem depende da tela; regiões indisponíveis são puladas.
Essas duas teclas passam pelo mapa em **Comandos e acionadores** e podem
ser personalizadas. Manter a tecla pressionada não repete a navegação.

Na paleta (**Ctrl+K**), procure **Próxima região**
(`navigation.landmark.next`), **Região anterior**
(`navigation.landmark.previous`) ou **Região padrão**
(`navigation.landmark.default`). Também podem ser associados ao Stream Deck.
A paleta preserva a região que estava selecionada antes de abrir a busca,
inclusive quando aberta pelo botão com o mouse.

**Escape** continua contextual: controles, menus e diálogos têm prioridade.
Se ninguém consumir a tecla e o foco estiver em uma região conhecida diferente
da principal, ela aciona **Região padrão**. Escape não vira uma associação
global editável; personalizar outra tecla para esse comando não altera os
comportamentos próprios de cancelamento dos componentes.

Esses comandos só movem o foco e não gravam invocações no histórico. Um modal
bloqueia as regiões de fundo. O chat contextual pode navegar pelas próprias
regiões quando é o modal no topo; outro diálogo sobre ele bloqueia essa
navegação. A paleta não abre atrás de modais.

### Validação manual acumulada — regiões

- [ ] No workspace, pressionar **F6** e **Shift+F6**, soltando entre os
  acionamentos. Conferir ordem, retorno circular e anúncios do NVDA.
  Resultado: __________.
- [ ] Repetir em **Histórico**, **Configurações** e **Ajuda**: mover uma única
  região por acionamento, sem salto duplo. Resultado: __________.
- [ ] Partindo do conteúdo, abrir a paleta por **Ctrl+K** e executar
  **Próxima região**. Repetir clicando no botão da paleta: a referência deve
  continuar sendo a região original, não o campo de busca. Resultado: __________.
- [ ] Associar os três comandos a teclas pessoais ou ao Stream Deck e conferir
  o mesmo movimento de foco. Resultado: __________.
- [ ] Em outra região conhecida, pressionar **Escape** e conferir retorno à
  região principal. Com um menu ou edição que consome Escape, conferir que
  o cancelamento do componente continua tendo prioridade. Resultado: __________.
- [ ] No chat contextual, testar **F6/Shift+F6** entre suas regiões. Abrir outro
  diálogo sobre ele e conferir que nada atrás recebe foco. Resultado: __________.
- [ ] Manter F6 pressionado: não deve navegar repetidamente. Trocar de aba ou
  tela durante uma seleção pendente da paleta: não executar em outro contexto.
  Resultado: __________.

Aceite manual com NVDA e equipamento físico permanece pendente.

## Ações de apresentação do chat

Além dos seletores e das ações de mensagem, a paleta oferece:

- **Focar campo de mensagem**: leva o foco ao campo de envio, se habilitado.
- **Focar mensagens**: leva o foco à região da lista, inclusive quando vazia.
- **Abrir leitura da mensagem**: abre o modo de leitura do turno selecionado;
  não inicia TTS. **Escape** continua saindo desse modo.
- **Abrir menu da mensagem**: abre o menu existente da mensagem selecionada.
- **Alternar exibição do raciocínio**: mostra ou oculta o raciocínio quando
  a mensagem possui esse conteúdo.
- **Expandir respostas da mensagem** e **Recolher respostas da mensagem**:
  controlam a thread selecionada, respeitando a disponibilidade de filhos.

Não foram criadas novas teclas padrão para essas sete ações. Você pode
associá-las a combinações próprias ou a teclas do Stream Deck em
**Comandos e acionadores**. Enter, R, setas e a tecla de menu continuam sendo
gestos locais dos componentes, não atalhos globais que capturam texto digitado.

Para ações de uma mensagem, primeiro coloque o foco nela e depois abra a
paleta por **Ctrl+K** ou pelo botão. A busca guarda a mensagem de origem;
não escolhe a última mensagem nem muda de alvo se a conversa for trocada.
As ações ficam indisponíveis quando não há mensagem apta. Chats em modais
só operam enquanto aquele modal é o mais alto; diálogos de decisão e leitura
isolada não autorizam ações no chat de fundo.

São ações de foco/apresentação, sem histórico de invocações por tecla.
Expandir uma thread pode buscar filhos pelo carregador existente, mas não
envia mensagem nem cria uma nova execução do modelo.

### Validação manual acumulada — apresentação do chat

- [ ] Em um chat, abrir **Ctrl+K**, buscar **Focar mensagens** e executar.
  Repetir com **Focar campo de mensagem**. Conferir foco/anúncio com NVDA,
  incluindo uma conversa vazia. Resultado: __________.
- [ ] Focar uma mensagem, abrir a paleta pelo teclado e executar
  **Abrir leitura da mensagem**. Conferir leitura, links e saída por Escape.
  Repetir abrindo a paleta pelo botão com o mouse. Resultado: __________.
- [ ] Focar outra mensagem e executar **Abrir menu da mensagem**. Conferir
  que as ações e o retorno de foco pertencem à mensagem escolhida.
  Resultado: __________.
- [ ] Em mensagem com raciocínio, executar **Alternar exibição do raciocínio**
  pela paleta, por tecla pessoal e pelo controle existente. Conferir uma
  alternância por acionamento. Resultado: __________.
- [ ] Em mensagem com thread, executar **Expandir respostas da mensagem** e
  **Recolher respostas da mensagem**; conferir também as setas da árvore.
  Trocar de conversa durante carregamento: não receber foco atrasado.
  Resultado: __________.
- [ ] Associar as ações ao Stream Deck e repetir no chat ativo e no chat
  contextual. Abrir outro diálogo sobre ele: o chat de fundo não deve reagir.
  Resultado: __________.
- [ ] Com a paleta aberta sobre uma mensagem, trocar de aba/conversa e tentar
  executar: não aplicar a ação a outra mensagem. Resultado: __________.

Aceite manual acumulado ainda pendente; testes automatizados não substituem
os anúncios reais do NVDA nem a validação física do Stream Deck.

## Atalhos mostrados na interface

A ajuda de teclado, o menu principal e os controles migrados do workspace,
chat e editor mostram as combinações do mapa efetivo. Ao personalizar ou
suprimir um atalho, esses rótulos acompanham a configuração; não continuam
anunciando a combinação padrão. Durante recarregamento ou sem uma projeção
válida, o rótulo é omitido nos controles e a ajuda indica que não há atalho
efetivo disponível. Isso não desabilita o botão nem afirma que o comando
possa executar fora de seu contexto.

O menu de nova aba mostra o prefixo da sequência configurada; a ação pode
mostrar a sequência completa. Os gestos nativos (por exemplo, setas em uma
lista e Escape em diálogo) são identificados separadamente na ajuda.

Validação manual acumulada:

- [ ] Remapear a paleta e o menu principal numa camada ativa; conferir os
  rótulos e anúncios do NVDA, sem a combinação antiga. Resultado: ________.
- [ ] Remapear/suprimir modelo, histórico, perfil e limpeza do chat; conferir
  rótulos e abrir os pickers, inclusive modelo de agente. Resultado: ________.
- [ ] No editor, remapear salvar, abrir, inserir e apresentação; conferir
  menus e toolbar. Suprimir e confirmar a retirada do rótulo. Resultado: ________.
- [ ] Conferir a ajuda e o botão de nova aba com uma sequência personalizada;
  o botão deve mostrar o prefixo, não a ação final. Resultado: ________.
- [ ] Desativar a camada e conferir o retorno das combinações efetivas
  anteriores, sem precisar reiniciar a aplicação. Resultado: ________.

## Interromper comando do terminal

Na aba de terminal, a paleta oferece **Interromper comando do terminal**.
É possível associar essa ação a uma tecla pessoal ou ao Stream Deck. O botão
de interrupção e Ctrl+C sem seleção usam o mesmo efeito backend. Ctrl+C com
texto selecionado continua sendo cópia e o gesto só é tratado dentro do
terminal, nunca no campo de busca da paleta.

A ação envia a interrupção ao PTY, não fecha a aba ou o shell e não garante que
todo programa respeite Ctrl+C. Se a sessão/execução gerenciada mudar durante a
solicitação, ela é recusada. Não há repetição automática quando a resposta se
perde. Entrada/saída do terminal não são gravadas no ledger de comandos.

Validação manual acumulada (use apenas um processo descartável de teste, não
uma tarefa real de trabalho):

- [ ] No terminal PowerShell, executar `Start-Sleep -Seconds 60` e pressionar
  Ctrl+C sem seleção. Esperado: voltar ao prompt sem fechar sessão/aba.
  Resultado: ________.
- [ ] Repetir com o botão de interrupção e depois com **Ctrl+K → Interromper
  comando do terminal**. Esperado: o mesmo terminal é interrompido.
  Resultado: ________.
- [ ] Associar a ação a uma tecla pessoal e a uma tecla do Stream Deck.
  Repetir o processo de teste e conferir um acionamento por pressão.
  Resultado: ________.
- [ ] Selecionar texto no histórico/entrada e usar Ctrl+C. Esperado: copiar,
  não interromper. No campo de busca da paleta, Ctrl+C também não interrompe.
  Resultado: ________.
- [ ] Abrir um modal ou trocar de aba/execução com a paleta aberta. Esperado:
  ação anterior não é reaproveitada para outro terminal. Resultado: ________.
- [ ] Em listas de tarefas, editar título/descrição e salvar. Reabrir e
  confirmar persistência; em falha, o formulário não deve anunciar sucesso.
  Resultado: ________.

O lote de interrupção acima não migrava as demais mutações. O bloco seguinte
amplia esse escopo; CRUD/ativação de perfis permanecem pendentes.

### Listas de tarefas e sessões de terminal (bloco 100)

Na página **Listas de tarefas**, criar, salvar alterações, duplicar e excluir
usam o executor central. A exclusão exige confirmação do backend e identifica
a lista capturada. Se a lista mudar enquanto o formulário/decisão estiver
aberto, a operação é recusada: recarregue a lista antes de tentar novamente.
A duplicação preserva as configurações da lista, sem copiar suas tarefas.

As ações **Duplicar lista selecionada** e **Excluir lista selecionada** podem
ser usadas na paleta, em uma combinação pessoal ou no Stream Deck quando há
uma linha selecionada nessa página. **Salvar nova lista** e **Salvar alterações
da lista** pertencem ao formulário correspondente: não atuam em outros
formulários e não abrem uma segunda paleta por cima de um modal. Nenhum novo
atalho global foi atribuído automaticamente.

Validação acumulada de listas, com dados descartáveis:

- [ ] Criar lista com título/descrição, salvar e reabrir. Conferir persistência.
  Resultado: ________.
- [ ] Editar ambos os campos e salvar. Confirmar que a mesma lista foi alterada,
  sem criar outra. Resultado: ________.
- [ ] Selecionar uma lista, abrir Ctrl+K e executar **Duplicar lista selecionada**.
  Conferir nova lista com configurações, sem tarefas copiadas. Resultado: ________.
- [ ] Executar **Excluir lista selecionada**, cancelar e confirmar que ela
  permanece. Repetir e confirmar a exclusão. Resultado: ________.
- [ ] Associar duplicação/exclusão a teclado pessoal e Deck; fora da página
  de listas ou sem seleção, conferir que não atuam sobre uma lista antiga.
  Resultado: ________.

**Perfis:** criar, editar, duplicar, excluir e ativar usam o mesmo mecanismo
pelos controles da tela, paleta, combinação pessoal e Stream Deck. Não foram
adicionadas combinações padrão para essas cinco ações.
**Salvar novo perfil** e **Salvar alterações do perfil** exigem o formulário
correspondente aberto. Na página de perfis, selecione o alvo para **Duplicar
perfil selecionado**, **Excluir perfil selecionado** ou **Ativar perfil
selecionado**. As ações não operam sobre o último perfil usado em outra tela.
Excluir exige confirmação e não permite remover o perfil ativo.

A gravação é coordenada com autorizações e invalida o mapa anterior de
comandos antes da alteração. O mapa é reconstruído depois do registro do
resultado conhecido; uma falha incerta não anuncia sucesso nem repete a escrita.
Se outro processo alterar os perfis enquanto o formulário estiver aberto,
reabra o formulário para revisar o conteúdo atual antes de salvar.
Se excluir falhar na revogação das autorizações, o backend tenta restaurar o
arquivo original. Se a recuperação também falhar, as autorizações continuam
bloqueadas; não contorne a falha recriando o perfil para reutilizá-las.

Validação acumulada de perfis, usando cópias descartáveis:

- [ ] Criar, editar e duplicar; conferir persistência e atalhos do workspace
  depois de cada operação. Resultado: ________.
- [ ] Ativar a cópia e conferir que ela aparece como ativa e que o mapa de
  comandos volta a responder. Resultado: ________.
- [ ] Voltar ao perfil anterior e excluir a cópia inativa; verificar que
  excluir o perfil ativo continua recusado. Resultado: ________.
- [ ] Repetir as ações disponíveis pela paleta, por combinação pessoal e
  Stream Deck. Fora da página/formulário correspondente, não devem executar.
  Resultado: ________.
- [ ] Cancelar a confirmação de exclusão e conferir que o perfil permanece.
  Resultado: ________.

No terminal, **Criar sessão de terminal** cria e vincula uma sessão à mesma
aba, sem criar outra aba. Se já havia uma sessão vinculada, ela permanece no
seletor de sessões. **Fechar sessão de terminal** pede confirmação do backend,
encerra a sessão capturada e remove seu vínculo; a aba permanece aberta.
Essas ações não são o comando de interrupção: interromper envia Ctrl+C;
fechar encerra o processo da sessão.

Validação acumulada de sessões (somente sessões descartáveis):

- [ ] Criar sessão pelo botão da página e pela paleta. Conferir que a aba é
  a mesma e que a sessão anterior continua no seletor. Resultado: ________.
- [ ] Fechar sessão e cancelar a confirmação. Conferir que ela permanece
  utilizável. Repetir confirmando; conferir sessão encerrada e aba preservada.
  Resultado: ________.
- [ ] Associar criar/fechar a combinações pessoais e Stream Deck. Fora de
  uma aba terminal ativa, não devem atuar sobre a última sessão utilizada.
  Resultado: ________.
- [ ] Abrir a confirmação e mudar/cancelar o contexto da operação. O pedido
  antigo não deve fechar outra sessão. Resultado: ________.

Depois de o backend confirmar o início do fechamento, cancelar a interface
não desfaz esse efeito: o encerramento continua para não deixar um processo
vivo sem vínculo. Em resultado indeterminado, confira o estado da sessão;
não há repetição automática do comando.

Os avisos de configuração, captura do Stream Deck e importação usam o
anunciador global do aplicativo, evitando mensagens simultâneas de regiões
independentes para o leitor de telas. O texto continua visível na tela.

### Atalhos globais de voz e jobs no Windows

Quando uma combinação está registrada como atalho global de voz ou de um job,
ela fica reservada para esse acionador: o teclado local não executa também um
comando com a mesma combinação. A reserva é sincronizada antes do registro no
Windows e removida depois que o sistema confirma a retirada. Isso não exige
uma nova configuração e não acrescenta consultas ao backend a cada tecla.

A coordenação continua ativa durante o login e a troca de telas. Se a interface
não confirmar a reserva, a nova hotkey não é registrada. O aplicativo não
contorna esse erro ativando simultaneamente os dois caminhos.

Os atalhos continuam sendo configurados nos perfis e nos jobs. O aplicativo
projeta essas configurações no mecanismo comum; não é necessário recriá-las
em uma camada. A voz só é entregue à superfície elegível do mesmo perfil que
originou o atalho, sem trocar de destinatário durante a espera. Modal aberto,
troca de perfil e superfície desmontada impedem a entrega.

O atalho de job agora pede confirmação antes de executar no serviço de jobs
existente. Um modal já aberto impede iniciar essa solicitação. A condição
`when`, o proprietário e a configuração atual do job continuam sendo conferidos.
A preparação, confirmação e espera para iniciar têm prazo limitado. Depois
que o job começa, valem os limites do runtime de jobs e de suas ferramentas;
o tempo gasto na confirmação não reduz esse orçamento. Cancelamento,
encerramento do aplicativo e alterações de segurança continuam sendo
respeitados, assim como um prazo menor imposto pelo chamador.

Um profile calculado por expressão precisa resolver para um profile instalado
e previamente autorizado para aquele job. O alvo é fixado antes da confirmação
e conferido novamente antes de cada tentativa: se a expressão mudar de alvo,
o job é recusado, não redirecionado. Expressão vazia ou sem os dados necessários
não herda automaticamente o profile ativo. Um atalho não fornece payload de
evento externo. Confirmar a execução não concede um grant de profile.

A validação física acumulada deve conferir uma combinação global também
atribuída a um comando local, troca de perfil, tecla mantida durante a troca,
modal aberto, cancelamento da confirmação e aplicativo em segundo plano.

### Condições visuais na paleta — validação acumulada

Para comandos que apenas apresentam ou navegam na interface, o acionador da
paleta pode restringir a execução pelo foco do aplicativo, tipo de tela, aba
específica e perfil. Em **Comandos e acionadores**, edite o acionador de origem
**Paleta**, adicione a condição e selecione a aba pelo nome quando necessário.
Isso não atribui automaticamente um atalho de teclado ao comando.

A condição se refere à tela de origem, não ao campo de busca da paleta. Mudar
de aba, perfil, workspace ou sessão invalida a preparação: feche e abra a
paleta novamente no contexto desejado. Supressões continuam sendo respeitadas.
Esta ampliação não se aplica a efeitos backend. Para Stream Deck, consulte
o roteiro abaixo; o editor mostra somente as condições suportadas por cada
classe e origem.

### Condições visuais no Stream Deck — validação acumulada

Nos comandos de apresentação local, como abrir o menu, configurações ou
seletores do chat, uma tecla do Stream Deck pode depender do foco do
aplicativo, tipo de tela, aba específica e perfil. Grave a tecla pelo
pressionamento e escolha as condições no editor. A aba é escolhida pelo nome;
não é necessário digitar o serial do dispositivo.

Uma mesma tecla pode ter comandos diferentes em contextos distintos. Se mais
de um binding empatar, a execução é recusada. A interface verifica o contexto
atual antes de iniciar a ação, respeitando foco, modais e composição de texto.
Sem o contexto necessário, não há tentativa de executar em outra aba.

A tecla apresenta os nomes dos comandos potenciais; essa apresentação
não afirma qual tela está em foco nem que a ação está disponível naquele
instante. As condições são verificadas ao receber o pressionamento na UI.
Manter a tecla pressionada não repete a ação. Os comandos locais não criam
registros de invocação.

O suporte visual também abrange **70 comandos de workspace**: os 72 comandos
contextuais de workspace da paleta, exceto aplicar/remover Mermaid. Inclui
ações de abas, mensagens, terminal, formatação e arquivos, preservando suas
confirmações e restrições de alvo. Com ativar, alternar e voltar camada,
o grupo passa a **73 comandos** (lote126 de 22/09/2026). O lote127 acrescenta
seis mutações de páginas, com limites próprios e qualificação automatizada aprovada,
descritos abaixo. O lote128 acrescenta aplicar/remover Mermaid no editor,
com qualificação automatizada integrada aprovada, conforme o roteiro abaixo.
Os quatro campos são foco do aplicativo, tipo de tela, aba específica e
perfil; não combine condições visuais com processo ou dispositivo.

Se a tecla tiver apenas ações de backend condicionadas ao perfil, a seleção
continua sendo feita diretamente pelo backend, inclusive a atualização do
visor quando o perfil muda. Não exige um contexto visual adicional. Isso não
inclui as mutações de páginas nem aplicar/remover Mermaid: nesses casos,
mesmo uma condição apenas de perfil exige a tela de origem correspondente.

Uma mesma tecla pode combinar apresentação local e esses comandos de
workspace, desde que o contexto selecione um único binding. Cada
pressionamento produz um único evento e apenas o ramo selecionado executa.
O ramo local não faz chamada de execução de retorno ao backend nem cria
registro de invocação. O ramo durável usa uma oferta opaca vinculada ao
pressionamento físico recebido pelo host e ao snapshot do workspace:
vale por 10 segundos e só pode ser usada uma vez. Contexto inválido ou oferta
expirada não executam outro ramo como alternativa. Comandos backend fora
desse grupo continuam indisponíveis na combinação visual mista.

O lote anterior de 70 comandos tem testes focados e suíte frontend completa aprovados (445
arquivos/5480 testes), além do build frontend com TypeScript e build/vet do
App. A regressão backend ampla passou após as correções; o aceite físico/NVDA
continua pendente, sem encerrar o roteiro abaixo.

Para camadas, escolha a regra e o escopo na configuração: esses argumentos
ficam persistidos e são usados pelo host, não enviados livremente pela UI.
A oferta física é consumida pela API `ExecuteContextualDeckLayerCommand`,
sem reserva de execução UI. As teclas do mesmo dispositivo compartilham a
pilha: uma tecla pode ativar uma camada e outra voltar. O host verifica geração
e snapshot ao admitir a ação; a atualização do mapa causada pela própria
camada não transforma uma execução concluída em falha.

Nos três comandos de camada, condições nativas de processo/dispositivo
continuam disponíveis, isoladas ou com perfil. Não as combine com foco,
tipo de tela ou aba: essa combinação é diagnosticada como indisponível,
inclusive quando parte das condições é herdada da camada. Esta preservação
não amplia as condições dos outros 70 comandos de workspace.

O lote126 tem testes focados, TypeScript, ESLint e `go vet` aprovados, com
bindings Wails gerados. O backend de camadas Deck passou em 59,409 s e a
regressão de claim A-B-A da paleta em 23,835 s. O backend amplo passou em
358,565 s numa base anterior às últimas correções de claim do teclado e
condições nativas. Build frontend com TypeScript aprovado (Vite 1 min 3 s);
suíte frontend final com 5529 testes aprovados em 447 arquivos após corrigir
o relógio do teste de prazo. A regressão backend final também passou (249,874 s),
com as guardas finais integradas. Os seguintes aceites físicos/NVDA permanecem pendentes:

- [ ] **Ativar camada** condicionada à aba/perfil: um efeito no contexto
  correto, nenhum em outra aba/perfil. Resultado: ________.
- [ ] **Alternar camada**: um efeito por pressionamento completo, sem repetir
  enquanto a tecla fica pressionada. Resultado: ________.
- [ ] **Voltar camada** por outra tecla do mesmo dispositivo, verificando a
  pilha compartilhada e o anúncio do resultado. Resultado: ________.
- [ ] Tecla mista com apresentação local e camada em perfis distintos:
  apenas um ramo executa; empate não executa. Resultado: ________.
- [ ] Trocar perfil A→B→A durante uma oferta: a oferta antiga deve ser
  recusada; novo pressionamento usa o perfil atual. Resultado: ________.
- [ ] Desconectar/reconectar o Deck: não reaproveitar oferta anterior;
  novo pressionamento respeita o mapa atual. Resultado: ________.

- [ ] Associe **Abrir seletor de modelo** à tecla gravada, condicionada a uma
  aba de chat pelo nome. Com o foco no chat escolhido, pressione a tecla: o
  picker existente deve abrir. Resultado: ________.
- [ ] Repita em outra aba e com o aplicativo sem foco: não deve abrir o picker
  em nenhuma delas. Resultado: ________.
- [ ] Adicione condição de perfil e compare um perfil correspondente com
  outro. Somente o correspondente deve permitir a ação. Resultado: ________.
- [ ] Mantenha a tecla pressionada, solte e pressione novamente; confira que
  só o novo pressionamento dispara outra ação. Resultado: ________.
- [ ] Desative a camada, aguarde a publicação e pressione a tecla: a antiga
  configuração não deve executar. Confira também os anúncios do NVDA no
  editor e no picker. Resultado: ________.
- [ ] Na mesma tecla, configure apresentação local para um perfil e uma ação
  de workspace suportada para outro. Confira um único efeito por
  pressionamento e nenhuma execução no contexto incompatível. Resultado: ________.

### Páginas no Stream Deck — listas e perfis

O lote127 amplia a configuração para 79 comandos: acrescenta duplicar,
excluir e limpar listas, além de duplicar, excluir e ativar perfis. Para
esses seis comandos, use apenas foco do aplicativo, tipo de tela e perfil;
não há condição de aba específica. Ações de perfis usam a página Perfis;
ações de listas usam a página Listas de tarefas. Duplicar e limpar também
admitem uma lista aberta no workspace; excluir não ganha essa extensão.

Não combine esses comandos de página com condições de ID de aba, processo
ou dispositivo, inclusive herdadas. Uma tecla compartilhada não torna a
página executável em outra superfície. As condições nativas de camadas
continuam com os limites descritos acima; Mermaid não entra nesta ampliação.

As ações usam o item selecionado na página, não a aba ao fundo. As confirmações
de exclusão/limpeza continuam obrigatórias. Mudar o alvo ou invalidar o mapa
antes da admissão impede a alteração; uma execução já admitida não é repetida
nem desfeita pela renovação do próprio mapa.

Integração e regressões automatizadas do lote127 aprovadas. Aceite físico/NVDA
acumulado para validação manual:

- [ ] Conferir os seis comandos nas páginas correspondentes, sem executar
  em outra página ou perfil. Resultado: ________.
- [ ] Conferir duplicar/limpar na lista do workspace e a indisponibilidade
  de excluir nesse contexto. Resultado: ________.
- [ ] Cancelar confirmação, mudar página/perfil e reconectar o dispositivo;
  nenhuma oferta antiga deve repetir a ação. Resultado: ________.

### Mermaid no Stream Deck — validação manual pendente

Aplicar e remover Mermaid elevam o grupo contextual a **81 comandos**.
Admitem foco, tipo de tela, aba específica e perfil, mas somente um editor
canônico pode executar. Mesmo com condição apenas de perfil, o comando não
fica disponível em outra tela. A configuração sem condições mantém seu
percurso anterior; esta ampliação não altera a paleta.

Qualificação automatizada integrada aprovada: App 464,744 s; frontend
450 arquivos / 5.617 testes; build/vet, TypeScript/Vite e lint focado.
O pressionamento captura o editor/bloco original. Aplicar fecha o modal
Mermaid correspondente; remover pede confirmação. A oferta física é
consumida antes da confirmação e nunca reaproveitada. Modal alheio,
documento alterado ou perda do contexto impedem a alteração; o conteúdo do
diagrama não é gravado no registro de auditoria do comando.

- [ ] Aplicar/remover no editor esperado, preservando o alvo e as
  confirmações existentes. Resultado: ________.
- [ ] Repetir em outra aba/tela/perfil; cancelar e trocar o documento durante
  a preparação: nenhuma ação deve atingir outro alvo. Resultado: ________.
- [ ] Reabrir após reconexão e conferir anúncios NVDA, sem reutilizar oferta
  antiga nem repetir a ação. Resultado: ________.

### Roteiro da paleta contextual

Roteiro de aceite manual acumulado (regressão automatizada aprovada):

- [ ] Em uma camada ativa, configure **Abrir seletor de modelo** na paleta
  com condição de uma aba de chat específica, escolhida pelo nome. Abra a
  paleta a partir dessa aba e execute o comando; deve abrir o picker existente.
  Resultado: ________.
- [ ] Abra a paleta a partir de outra aba: o comando condicionado não deve
  ficar disponível. Repita com perfil correspondente e diferente.
  Resultado: ________.
- [ ] Com a paleta aberta, altere o contexto e tente a seleção antiga: ela
  não deve executar em outra tela. Reabra a paleta para obter a lista atual.
  Resultado: ________.
- [ ] Confira leitura e navegação por NVDA, sem depender de IDs de abas.
  Resultado: ________.

### Paleta contextual para ações de abas e workspace

As ações de criar abas (chat, editor, lista de tarefas e terminal), fechar a
aba, criar workspace, abrir chat e mudar o modo do editor também aceitam
condições de foco, tipo de tela, aba e perfil na paleta. A origem é a aba de
onde você abriu o picker, não o campo de busca. A execução é recusada se essa
origem mudar durante a preparação. As ações de mensagens, limpeza de conversa,
terminal, formatação e arquivos do editor também aceitam essas condições. Os
comandos mantêm suas confirmações e restrições normais: a condição não concede
acesso a outro alvo. Seis mutações de listas/perfis também aceitam condições,
com as diferenças descritas abaixo. Ativar/alternar/voltar camada também
aceitam essas condições na paleta das abas do workspace. No Stream Deck,
o grupo contextual tem 73 comandos (70 de workspace e três ações de camada),
com os limites descritos em **Condições visuais no Stream Deck** acima.

### Ações de camada condicionadas na paleta

Em **Comandos e acionadores**, configure um acionador de paleta para
**Ativar camada**, **Alternar camada** ou **Voltar camada**. Escolha a camada
manual pelo nome nos controles existentes. Você pode restringir por foco,
tipo de tela, aba ou perfil, a partir de chat, editor, terminal ou lista de
tarefas aberta no workspace. Não se aplica à página de configurações nem
às páginas independentes de listas/perfis.

A camada que contém o acionador precisa estar ativa para a seleção existir.
Para testar ativação de outra camada, deixe o acionador em uma camada de
controle sempre ativa. Voltar encerra a reivindicação manual mais recente
do escopo configurado. A aplicação atualiza o mapa após a operação; essa
atualização não deve repetir a ação nem anunciar falha após um sucesso.
Desabilitar uma camada impede novas ativações, mas não impede encerrar uma
ativação manual anterior pelos controles de desativação.

- [ ] Configure **Ativar camada** na paleta, limitado a uma aba escolhida
  pelo nome. Abra **Ctrl+K** nessa aba e execute. Confira a camada ativa.
  Resultado: ________.
- [ ] Configure **Alternar camada**, execute duas vezes reabrindo a paleta
  entre as seleções e confira ativação/desativação. Resultado: ________.
- [ ] Ative uma camada manual e execute **Voltar camada**; confira o estado
  anterior e o anúncio com NVDA. Resultado: ________.
- [ ] Em outra aba ou página, a seleção condicionada não deve ficar apta.
  Mudar a origem antes da submissão deve cancelar a seleção antiga.
  Resultado: ________.

### Paleta contextual nas páginas de listas e perfis

Duplicar, excluir e limpar listas; duplicar, excluir e ativar perfis aceitam
condições de foco do aplicativo, tipo de tela e perfil. Abra **Ctrl+K** a
partir da página com o item de teste selecionado. A ação opera nesse item,
não na aba que ficou aberta em segundo plano. Duplicar e limpar também
funcionam na lista aberta como aba do workspace.

Neste grupo não há condição por ID de aba: o item selecionado na página não
é uma aba. Criar e salvar pelos formulários modais continuam fora deste
suporte contextual; a paleta não se abre sobre esses formulários.

- [ ] Em uma camada de teste ativa, configure **Duplicar lista** na paleta
  com condição do tipo de tela de listas. Selecione uma lista descartável,
  abra **Ctrl+K** e execute. Confira a cópia. Resultado: ________.
- [ ] Configure **Excluir lista** ou **Limpar lista** e cancele a confirmação:
  os dados devem permanecer. Repita confirmando apenas sobre dados de teste.
  Resultado: ________.
- [ ] Na página de perfis, configure **Duplicar perfil** e **Ativar perfil**.
  Confira o perfil selecionado e depois o perfil ativo. Resultado: ________.
- [ ] Abra a paleta em outra tela: a condição da página não deve ser
  satisfeita pela aba de fundo. Trocar a origem durante a preparação deve
  cancelar a execução antiga. Resultado: ________.
- [ ] Confira anúncios, setas e retorno do foco com NVDA. Resultado: ________.

Roteiro acumulado, usando uma camada de teste ativa:

- [ ] Crie um acionador de paleta para criar uma aba de chat, condicionado
  à aba atual escolhida pelo nome. Abra a paleta com **Ctrl+K** nessa aba e
  execute a ação: deve criar exatamente uma aba. Resultado: ________.
- [ ] Abra a paleta em outra aba: essa seleção condicionada não deve ficar
  disponível. Volte à aba de origem e repita com condição de perfil,
  comparando perfil correspondente e diferente. Resultado: ________.
- [ ] Em uma aba de editor, configure uma ação de mudar modo na paleta,
  condicionada ao tipo editor. Execute e confira o modo. Abra a paleta no
  chat: a ação condicionada não deve ficar disponível. Resultado: ________.
- [ ] Se o contexto mudar com a paleta aberta, a seleção antiga não deve
  modificar outra aba. Reabra a paleta e confira a lista atual. Resultado: ________.
- [ ] Confira rótulos e navegação com NVDA; os seletores de aba e perfil
  devem continuar usando nomes legíveis. Resultado: ________.

### Validação adicional da paleta contextual nas abas

Use conversas e arquivos de teste:

- [ ] Configure uma ação de mensagem, como **Alternar fixação da mensagem**,
  na paleta, com condição da aba de chat escolhida pelo nome. Selecione uma
  mensagem e abra **Ctrl+K**; execute e confira a alteração uma vez.
  Repita em outra aba: a seleção condicionada deve ficar indisponível.
  Resultado: ________.
- [ ] Configure a limpeza de conversa na paleta em uma conversa descartável.
  A confirmação deve continuar aparecendo. Cancele e confira que o conteúdo
  permaneceu; só confirme exclusões que você realmente queira fazer.
  Resultado: ________.
- [ ] No editor, condicione uma ação de formatação à aba e ao perfil.
  Abra **Ctrl+K**, execute e confira o texto. Com outro perfil, a ação
  condicionada não deve executar. Resultado: ________.
- [ ] Configure **Salvar cópia** na paleta e escolha um arquivo novo de teste.
  A ida ao diálogo nativo e a volta à mesma aba devem funcionar; recusar a
  confirmação de sobrescrita deve preservar o arquivo existente.
  Resultado: ________.
- [ ] Na aba de terminal, configure uma ação de sessão na paleta. Confira
  que ela fica restrita à aba escolhida e mantém sua confirmação quando
  aplicável. Use somente sessões de teste. Resultado: ________.
- [ ] Confira nomes, disponibilidade, confirmação e navegação com NVDA.
  Resultado: ________.

### Condições físicas e ações de camada — validação acumulada

O editor de condições mostra somente os campos suportados pela origem e pelo
tipo de comando. Para ações backend do Stream Deck, o dispositivo é escolhido
pelo nome/modelo entre os conectados. Uma referência desconectada permanece
indicada como indisponível; não é substituída automaticamente por outro aparelho.
O programa em primeiro plano é identificado pelo nome do executável, como
`notepad.exe`, não por caminho completo, título de janela ou endereço de página.
Essa condição se refere ao programa no instante do acionamento físico, antes
de o Assistente trazer sua janela para frente.
Uma mudança de foco posterior não troca a condição daquela ocorrência.
Se o sistema não conseguir identificar o programa, acionadores que dependem
dessa informação não são executados. Isso não autoriza controlar o programa
externo: a ação continua sujeita às permissões próprias do comando.

Quando houver atalhos registrados nos perfis de voz ou nos jobs, a camada
**Atalhos globais de voz e jobs** permite consultá-los e suprimir ou restaurar
seus padrões. A combinação e o destino continuam sendo configurados no perfil
de voz ou no job correspondente, não no editor genérico de acionadores.
Suprimir impede a execução pelo resolvedor; não remove o registro da combinação
no sistema operacional. Restaurar devolve o comportamento do padrão vigente.

As ações **Ativar camada**, **Alternar camada** e **Voltar camada** usam o mesmo
executor dos demais comandos. Ao configurar um acionador, escolha a regra pelo
nome da camada e pelo ciclo de vida. Regras temporárias exigem uma duração em
segundos. A regra precisa estar habilitada e pertencer ao escopo editado;
uma referência removida precisa ser corrigida explicitamente.

O acionador que ativa outra camada precisa estar em uma camada já ativa: não
coloque o único comando de ativação dentro da própria camada desativada.
**Voltar camada** retorna na pilha da mesma origem e sessão, não desfaz uma
ativação feita por outro dispositivo ou pela tela de configurações.
Essas ações não recebem combinações padrão novas; você escolhe os acionadores.

Na paleta, a ação só fica disponível quando há um acionador configurado. Para
ativar ou alternar, também é necessária uma regra-alvo válida; **Voltar camada**
não pede regra-alvo. Remover a regra necessária ou o acionador torna a ação indisponível.
Uma tecla mantida no Stream Deck não executa novamente só porque a ativação
publicou um novo mapa. Solte e pressione de novo para uma nova execução.

Roteiro manual acumulado deste lote (regressão automatizada concluída):

- [ ] Criar duas camadas de teste, uma já ativa para os acionadores e outra
  com regra manual. Configurar **Ativar camada** na primeira, selecionando a
  regra da segunda pelo nome. Executar pelo teclado e conferir o novo mapa.
  Resultado: ________.
- [ ] Configurar **Alternar camada** e **Voltar camada** na camada de controle.
  Testar cada origem separadamente: teclado, paleta e Stream Deck. Conferir
  que voltar não remove a ativação de outra origem. Resultado: ________.
- [ ] No Stream Deck, manter a tecla de ativação pressionada durante a troca
  de mapa: deve ocorrer uma única execução. Soltar e pressionar novamente;
  desconectar/reconectar e conferir um novo pressionamento. Resultado: ________.
- [ ] Escolher regra temporária, informar duração e conferir expiração sem
  reiniciar. Repetir com regra de sessão e conferir encerramento da sessão.
  Resultado: ________.
- [ ] Em comando backend do Stream Deck, configurar condição de programa.
  Pressionar com o programa correspondente em foco e depois com outro programa.
  Somente a primeira situação deve atender à condição. Resultado: ________.
- [ ] Conferir com NVDA os nomes de camada/dispositivo e a indicação de
  referência indisponível, sem exigir leitura de IDs ou serial. Resultado: ________.
- [ ] Com um perfil de voz ou job de teste já configurado, consultar sua
  combinação na camada **Atalhos globais de voz e jobs**. Suprimir e restaurar
  o padrão, confirmando cada alteração; verificar que a execução respeita
  a supressão e que o editor genérico não altera a combinação. Resultado: ________.
