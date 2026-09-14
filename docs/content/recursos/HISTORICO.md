---
title: "Histórico e Conversas Longas"
weight: 15
---

# Histórico

O histórico guarda conversas por workspace com busca, filtros e retomada. Conversas longas usam sumarização automática e backend-driven messaging para manter o contexto sem perder o fio, com paginação e virtualização.

## Seleção e exclusão pelo teclado

- Use `Ctrl+Espaço` para marcar ou desmarcar a conversa focada, `Shift+Seta` para ampliar a seleção e `Ctrl+A` para selecionar todas as conversas visíveis.
- `Delete` e o botão **Excluir** da barra executam a mesma ação: havendo seleção, uma única confirmação exclui todas as conversas selecionadas; sem seleção, exclui apenas a conversa focada.
- Enquanto a confirmação ou a exclusão está em andamento, uma segunda exclusão não é iniciada.
- A exclusão em lote é atômica: se qualquer conversa não puder ser excluída, nenhuma delas é removida. A seleção permanece ativa e o leitor de telas anuncia a falha.
- Uma conversa com subagente, resposta do assistente ou entrega para canal ainda em andamento não é excluída. Cancele ou aguarde o trabalho terminar e tente novamente; as demais conversas do lote também permanecem intactas.
- Após excluir, o foco permanece na mesma posição lógica da grade (ou volta à linha anterior ao excluir a última). Se não restarem conversas, ele retorna a um controle utilizável da página.

## Detalhes de ferramentas

O histórico mostra inicialmente apenas o nome, o estado, a duração e uma prévia
estrutural de cada ferramenta. Use **Mostrar tudo** dentro da ferramenta para
carregar os parâmetros e o resultado completos. Esse carregamento ocorre
somente quando solicitado, funciona sem internet porque lê o banco local e não
grava o conteúdo técnico no navegador.

Os controles entram na ordem de Tab no modo de leitura da mensagem. Se a
política de retenção já removeu o payload, o app informa que os detalhes não
estão mais disponíveis. Excluir mensagens ou conversas também remove detalhes
associados e invalida cópias temporárias mantidas em memória.

## Exportação e importação

Ao exportar uma conversa, o arquivo JSON inclui o histórico completo das
ferramentas no bloco `toolInvocations`. As mensagens continuam limpas, sem
registros técnicos duplicados. Exportações em HTML, PDF e Markdown exibem as
chamadas e os resultados a partir desse mesmo histórico.

Ao importar um JSON antigo, o app converte automaticamente os registros de
ferramentas para o formato atual. Ferramentas que não existem na instalação de
destino permanecem legíveis no histórico, mas não são habilitadas para
execução.
