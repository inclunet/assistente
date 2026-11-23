---
title: "Terminal"
weight: 3
---

# Terminal Integrado

O Assistente possui um terminal integrado com sessões **efêmeras** (AEP-0089), permitindo executar comandos sem sair do aplicativo.

## Como Funciona

O terminal cria sessões PTY (pseudo-terminal) reais usando o shell do sistema:

- **Windows**: PowerShell
- **macOS/Linux**: Bash (ou shell padrão)

Cada sessão mantém:

- Diretório de trabalho atual
- Histórico de comandos (até 200 comandos por sessão no backend)
- Estado: Idle, Busy ou Closed
- Saída formatada

No Bash, no zsh e nos shells POSIX, o status é o da última operação do comando.
No Linux, sessões zsh efêmeras e sessões persistentes criadas pelo modelo
carregam o perfil do usuário e dispensam o completador global do Ubuntu. Os
processos iniciados nessas sessões herdam a variável `skip_global_compinit=1`.
Assim, a pergunta de configuração do completador não consome parte do comando.
Sessões abertas manualmente carregam o completador conforme a configuração do sistema.
Se o zsh fizer uma pergunta na abertura de uma dessas sessões, responda na aba
antes de pedir ao modelo que use aquele terminal pelo identificador.

No PowerShell, uma operação final bem-sucedida devolve zero, mesmo após um
programa anterior falhar. Uma falha de cmdlet devolve 1, e um programa
nativo identificado como a última operação conserva seu código de saída. Se o
PowerShell indicar falha e o terminal não conseguir identificar qual operação
falhou dentro de uma expressão composta, o status é 1. Isso pode ocorrer em um pipeline no qual
um programa termina com erro e envia a saída a um cmdlet como `Get-Item`, ou
quando um `return` condicional pode ter encerrado o comando antes da etapa final.
O terminal conserva o código nativo quando a saída passa apenas por `Out-String`
ou `Out-String -Stream`. Em `$()` e `@()`, o código de uma operação nativa no fim
de um `if` corresponde ao ramo que o PowerShell executou, inclusive se o ramo
retornar esse programa com `return`. Se o ramo retorna apenas texto ou termina
com `return` vazio, a operação anterior do ramo determina o status.

Você pode enviar uma nova entrada mesmo enquanto o terminal continua produzindo saída.
Quando o modelo envia um comando, a saída anterior ao despacho permanece
associada à última entrada manual, inclusive se o envio for cancelado. Ao digitar
um comando na aba, o início da nova entrada já passa a identificar sua saída. A saída
que chega depois da execução gerenciada pode ser relida como saída da sessão.
Isso também vale quando o comando manual de origem já saiu do histórico.
No histórico, **Saída da sessão** identifica esse texto, inclusive o prompt
interativo do shell. Na aba ativa, o leitor de telas pode falar o prompt porque o shell
o envia pelo mesmo fluxo da saída espontânea. Você pode relê-lo no item imediatamente anterior. Quando a
saída precede o primeiro comando, ela ganha um item próprio. Uma saída longa
continua no bloco seguinte a cada 50 KiB. O histórico guarda até 200 comandos
e 200 blocos da sessão. A prévia do contexto enviado ao chat prioriza os comandos
recentes e usa o espaço restante para a saída da sessão. O envio segue a ordem
do histórico e contém até 900 pontos de código, com até 40 itens de cada origem.
Quando um programa responde a uma entrada em branco, por exemplo ao aceitar
uma opção padrão, o contexto do chat identifica a resposta como “Entrada em
branco”. Uma entrada em branco sem resposta é omitida.
Quando uma saída é longa, a prévia mostra o comando e o fim da saída disponível,
onde costuma aparecer o diagnóstico. As reticências indicam o trecho omitido. Se o
histórico já atingiu o limite de 50 KiB do comando, esse fim corresponde à parte
guardada e inclui a marca de corte do histórico.
O orçamento de contexto do perfil pode abreviar novamente o texto entregue ao
modelo. Nesse corte, o Assistente também prioriza os comandos recentes e suas
linhas finais, inclusive quando há saída da sessão antes e depois do comando.
Se o prazo vencer ou o comando for cancelado enquanto o terminal entrega a saída
anterior, o comando fica no histórico com código -1. A sessão volta a ficar disponível
e a saída pendente conserva a identificação original. Se você havia enviado um
comando manual, você ainda pode interrompê-lo com Ctrl+C. Se a escrita de uma entrada manual ou de um comando
do modelo falhar, o Assistente encerra a sessão. Isso impede que a próxima entrada
complete por engano uma linha enviada pela metade.
A aba continua aberta e o campo de comando fica somente para leitura. Você pode
selecionar e copiar o rascunho. Quando você digita um comando na aba, o campo
só é limpo depois que o backend aceita o envio, mesmo se a sessão terminar
antes da resposta. Se você já editou o texto, a edição é preservada. O campo informa quando a lista
está carregando e quando a sessão vinculada já terminou. Para voltar a editar,
use a barra de ferramentas para escolher outra sessão ou criar uma nova.
Essa orientação continua disponível com texto no campo. Anexos já carregados
permanecem no rascunho. Se o editor entrar em modo de leitura enquanto um anexo
está carregando, esse carregamento é descartado e a aba ativa anuncia o cancelamento.
Você pode anexar o arquivo de novo quando voltar a editar.
A prévia falada inclui até 300 pontos de código Unicode do comando ao iniciar
e da saída ao concluir ou receber texto. Um emoji composto
pode ocupar vários desses pontos. Os espaços iniciais ficam fora da contagem.
O leitor só anuncia o corte se o trecho omitido
contiver algo além de espaços em branco ou quebras de linha. Essa prévia usa
a saída original mesmo quando o histórico só guarda parte dela.
A saída comum só é lida quando a aba da sessão está visível e ativa. Em
Configurações, as conclusões de comandos de sessões vinculadas a abas chegam como resumo. Se você trocar de
aba enquanto um comando estiver rodando, o anúncio de conclusão identifica a
sessão pelo nome disponível no momento da fala e informa o código de saída.
Enquanto a aba existir, a posição atual pode identificá-la. Se ela fechar durante
a espera, o resumo usa o nome da sessão ou o título registrado ao concluir o comando. Sem esses
dados, o leitor anuncia “Terminal” e o código de saída.
Quando chega um aviso de atividade em segundo plano sem resumo da saída,
o leitor identifica o terminal pelo título da aba, se disponível.
O leitor de telas anuncia o encerramento mesmo com a aba de terminal fechada.
Quando o nome da sessão está disponível, o anúncio o inclui. Se o foco estava
no campo de comando, ele passa ao seletor de sessões ou a Novo terminal quando
a última sessão termina.

No PowerShell, você pode iniciar o comando com `using namespace` para usar
nomes de tipos abreviados. Por exemplo, depois de `using namespace System.Text`,
`[StringBuilder]::new('texto').ToString()` devolve `texto`.

Para reler um comando na aba Terminal, pressione **Seta para cima** no início
do campo de comando. O foco vai para o último item do histórico do Assistente.
O PowerShell também pode manter um histórico próprio por meio do PSReadLine.
Quando esse módulo está ativo, ele pode guardar a linha que o Assistente envia
ao executar um comando gerenciado. Essa linha inclui várias instruções para
capturar a saída e o código de término. Ela pode aparecer ao recuperar comandos
em um console PowerShell que compartilhe o mesmo arquivo de histórico.
A preferência `HistorySaveStyle` do PSReadLine controla a gravação em disco.
Com `SaveNothing`, a linha ainda pode ser recuperada em memória na mesma sessão.

A saída registrada é textual. Programas que movem o cursor para redesenhar
linhas anteriores podem aparecer de forma diferente no histórico e na tela
interativa. Sequências de controle incompletas podem aparecer como texto ao
encerrar a leitura.

## Sessões

Múltiplas sessões podem existir simultaneamente (até 10 por padrão), cada uma em sua própria aba.
Uma sessão em encerramento sai da lista de sessões disponíveis. Ela libera a vaga
para abrir outro terminal assim que a limpeza termina.

| Propriedade | Valor Padrão |
|---|---|
| Máximo de sessões | 10 |
| Timeout padrão | 15 segundos |
| Histórico de comandos no backend | 200 comandos por sessão |
| Histórico da aba | 200 comandos e até 200 blocos adicionais de saída da sessão |

Uma falha de leitura, um cancelamento ou o fechamento explícito da sessão devolve erro com a saída parcial disponível. Se o leitor já falhou quando o prazo vence, o diagnóstico conserva as duas causas. A duração acompanha o resultado quando a sessão registrou o início e o fim da execução.

Quando o prazo vence, a ferramenta informa o limite configurado e, quando disponível, a duração medida.
Se houver saída parcial, o timeout devolve essa saída como resultado. Um cancelamento explícito devolve erro e inclui a saída que já chegou.
Se o prazo vencer depois do envio, o Assistente tenta interromper o comando com
`Ctrl+C` e recuperar a saída restante antes de encerrar uma sessão efêmera.

Se um comando encerrar o próprio shell, a ferramenta devolve o código do processo e a saída disponível. O histórico da sessão fica disponível enquanto ela está aberta. Fechar a sessão manualmente durante um comando interrompe a espera e indica que ele não terminou normalmente.

## Histórico de Comandos

A saída manual que já apareceu na tela é preservada quando o histórico termina de carregar.

O histórico distingue um comando em andamento de um comando que terminou, inclusive
quando o shell devolve um código negativo.

Abrir o histórico permite ler os comandos anteriores. O leitor de telas anuncia
o início de um comando quando ele chega ao terminal ativo.

Cada entrada no histórico registra:

- Comando executado
- Saída completa
- Código de saída
- Timestamps de início e fim
- Origem: `user` (digitado) ou `llm` (executado pelo assistente)

## Integração com IA

O assistente pode executar comandos no terminal como parte de suas ferramentas (tool calling). Quando o LLM usa a ferramenta de terminal:

- O comando aparece no histórico com origem `llm`
- A saída é capturada e retornada ao LLM
- O usuário pode acompanhar em tempo real

## Interface

A interface inclui:

- **Toolbar**: Nome da sessão, diretório atual, botão de interrupção
- **Histórico**: Lista de comandos e saídas
- **Input**: Campo para digitar novos comandos

O campo do terminal envia texto diretamente ao shell. `/` é um caractere
normal nesse campo e não abre o menu de skills do chat. Autocomplete, quando
disponível, é responsabilidade do shell ativo e de seus recursos nativos.

## Atalhos

| Atalho | Ação |
|---|---|
| `Enter` | Executar comando |
| `Ctrl + C` | Interromper comando em execução |
| Deep link | `assistente://terminal/{id}` |
