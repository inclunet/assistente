---
title: "Ferramentas"
weight: 12
---

# Ferramentas

O assistente expõe 15 famílias de tools ao LLM: filesystem, shell, web, feed, http, memory, tasklist, mcpserver, history, job, questionnaire, skillloader, subagent e outras. Cada tool tem baseline operacional por perfil e é auditável no histórico do turno.

## Descoberta e carregamento

Tools fora do baseline do perfil ficam disponíveis pelo `tool_catalog`. A busca
aceita uma descrição da tarefa e filtros de origem, categoria, classe, pacote,
risco e disponibilidade. Os resultados priorizam relevância, pacotes
preferenciais do perfil e tools usadas recentemente na mesma conversa.

No primeiro turno, o assistente pode antecipar uma busca pequena e carregar até
três tools de leitura relevantes. Esse mecanismo nunca carrega automaticamente
tools de escrita, shell, rede ou operações destrutivas, nem habilita tools
desativadas ou opt-in.

O carregamento aceita nomes exatos e seletores como `mcp/atlassian/*` e
`package/history/*`. Cada seletor é limitado a 20 tools e continua sujeito à
política do perfil, disponibilidade, allowlists, confirmações e orçamento de
schemas. Para operações sensíveis, o carregamento apenas disponibiliza a
capacidade; ele não aprova sua execução.
