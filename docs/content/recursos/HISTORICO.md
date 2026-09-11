---
title: "Histórico e Conversas Longas"
weight: 15
---

# Histórico

O histórico guarda conversas por workspace com busca, filtros e retomada. Conversas longas usam sumarização automática e backend-driven messaging para manter o contexto sem perder o fio, com paginação e virtualização.

## Seleção e exclusão pelo teclado

- Use `Ctrl+Espaço` para marcar ou desmarcar a conversa focada, `Shift+Seta` para ampliar a seleção e `Ctrl+A` para selecionar todas as conversas visíveis.
- `Delete` e o botão **Excluir** da barra executam a mesma ação: havendo seleção, uma única confirmação exclui todas as conversas selecionadas; sem seleção, exclui apenas a conversa focada.
- A exclusão em lote é atômica: se qualquer conversa não puder ser excluída, nenhuma delas é removida. A seleção permanece ativa e o leitor de telas anuncia a falha.
- Uma conversa com subagente ainda em execução não é excluída. Cancele ou aguarde o run terminar e tente novamente; as demais conversas do lote também permanecem intactas.
- Após excluir, o foco permanece na mesma posição lógica da grade (ou volta à linha anterior ao excluir a última). Se não restarem conversas, ele retorna a um controle utilizável da página.
