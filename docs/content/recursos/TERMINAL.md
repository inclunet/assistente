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
- Histórico de comandos (até 200 entradas por sessão)
- Estado: Idle, Busy ou Closed
- Saída formatada

No Bash, no zsh e nos shells POSIX, o status é o da última operação do comando.
No Linux, sessões zsh efêmeras e sessões persistentes criadas pelo modelo
carregam o perfil do usuário e dispensam o completador global do Ubuntu.
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
ou `Out-String -Stream`.

Se a escrita de um comando enviado pelo modelo falhar, o Assistente encerra a sessão
para impedir que o próximo comando complete a linha anterior. Isso também vale
quando você abre um terminal persistente e pede ao modelo que use essa sessão.
A aba continua aberta e o campo de comando fica somente para leitura. Você pode
selecionar e copiar o rascunho. Quando você digita um comando na aba, o campo
só é limpo depois que o backend aceita o envio, mesmo se a sessão terminar
antes da resposta. Se você já editou o texto, a edição é preservada. O campo informa quando a lista
está carregando e quando a sessão vinculada já terminou. Para voltar a editar,
use a barra de ferramentas para escolher outra sessão ou criar uma nova.
A saída comum só é lida quando a aba da sessão está visível e ativa. Em
Configurações, as conclusões de comandos de sessões vinculadas a abas chegam como resumo. Se você trocar de
aba enquanto um comando estiver rodando, o anúncio de conclusão identifica a
sessão pelo nome disponível no momento da fala e informa o código de saída.
Enquanto a aba existir, a posição atual pode identificá-la. Se ela fechar durante
a espera, o resumo usa o nome da sessão ou o título registrado ao concluir o comando. Sem esses
dados, o leitor anuncia “Terminal” e o código de saída.
O leitor de telas anuncia o encerramento mesmo com a aba de terminal fechada.
Quando o nome da sessão está disponível, o anúncio o inclui. Se o foco estava
no campo de comando, ele passa ao seletor de sessões ou a Novo terminal quando
a última sessão termina.

No PowerShell, você pode iniciar o comando com `using namespace` para usar
nomes de tipos abreviados. Por exemplo, depois de `using namespace System.Text`,
`[StringBuilder]::new('texto').ToString()` devolve `texto`.

A saída registrada é textual. Programas que movem o cursor para redesenhar
linhas anteriores podem aparecer de forma diferente no histórico e na tela
interativa. Uma sequência de controle incompleta de até 4 KiB pode aparecer
como texto ao encerrar a leitura. Acima desse limite, o terminal descarta
a cópia literal e volta a mostrar texto quando a sequência termina ou é cancelada.

## Sessões

Múltiplas sessões podem existir simultaneamente (até 10 por padrão), cada uma em sua própria aba.

| Propriedade | Valor Padrão |
|---|---|
| Máximo de sessões | 10 |
| Timeout padrão | 15 segundos |
| Histórico por sessão | 200 entradas |

Uma falha de leitura, um cancelamento ou o fechamento explícito da sessão devolve erro com a saída parcial disponível. Se o leitor já falhou quando o prazo vence, o diagnóstico conserva as duas causas.

O timeout pode ser configurado em cada chamada da ferramenta. Quando ele é
atingido, o Assistente envia `Ctrl+C`, aguarda e drena a saída restante e só
então encerra uma sessão efêmera, evitando perder as últimas linhas do comando.

Se um comando encerrar o próprio shell, a ferramenta devolve o código do processo e a saída disponível. O histórico da sessão fica disponível enquanto ela está aberta. Fechar a sessão manualmente durante um comando interrompe a espera e indica que ele não terminou normalmente.

## Histórico de Comandos

A saída manual que já apareceu na tela é preservada quando o histórico termina de carregar.

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
