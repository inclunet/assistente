---
title: "Chat"
weight: 3
---

# Chat

## Fixar mensagens

Para guardar uma mensagem importante dentro da conversa:

1. Foque a mensagem e pressione `Shift+F10` ou a tecla Menu de Contexto.
2. Escolha **Fixar mensagem**.
3. Para remover a fixação, abra o mesmo menu e escolha **Desafixar mensagem**.

O estado é salvo no banco e permanece após reiniciar o aplicativo. A indicação
**Mensagem fixada** aparece junto ao cabeçalho da mensagem, com texto e ícone,
sem depender apenas de cor.

O botão **Mensagens fixadas** da barra do chat abre uma lista acessível de todas
as fixadas naquela conversa. A lista também inclui mensagens persistidas de
threads e ferramentas. Itens técnicos transitórios exibidos durante streaming
não podem ser fixados porque ainda não correspondem a uma mensagem persistida.

Fixações pertencem à conversa e ao usuário autenticado. Outros usuários da
mesma instalação não conseguem listar nem alterar essas mensagens.

## Respostas em andamento e fila

Cada conversa executa um turno por vez. Se você enviar outra mensagem ou tentar
novamente enquanto a resposta atual ainda está em andamento, a ação entra na
fila daquela conversa e o chat informa quantos turnos aguardam.

Cancelar a geração interrompe somente a resposta atual; não apaga os itens já
enfileirados. Conversas diferentes continuam respondendo em paralelo. Ao
terminar um turno com ferramentas, o chat atualiza apenas a resposta daquele
turno, preservando a posição e a janela de histórico que você estava lendo.

## Limite do texto

O texto de cada nova mensagem pode ocupar até **512 KiB em UTF-8**. Letras
acentuadas, caracteres combinantes e emojis podem usar mais de um byte. Se o
texto exceder esse limite, o chat anuncia o erro antes de iniciar o envio e
mantém o rascunho e o foco para correção.

Anexos não entram nessa contagem: mídia e arquivos têm validação própria. O
limite protege a comunicação interna, a serialização e o uso de memória.
