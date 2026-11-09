---
title: "Memórias"
weight: 7
---

# Memórias

Memórias são fatos duráveis que sobrevivem entre conversas — preferências, decisões e convenções. A IA as usa como contexto quando relevantes.

## Conceito

Cada memória tem conteúdo obrigatório, resumo, política de carga (`core`, `pinned`, `auto`, `retrievable`, `archived`), tipo, escopo (`global`, `user`, `workspace`, `project`, `conversation`) e importância.

## Gestão

Em **Memórias**: grid paginado com busca, filtro por política e arquivamento; crie ou edite com tipo/escopo/tags e importância. Arquivar preserva a política anterior para restaurar.

## Uso pela IA

Quando o provedor está habilitado e há usuário autenticado, memórias elegíveis podem entrar no prompt dentro do orçamento configurado (1200 caracteres por padrão). `core` e `pinned` têm prioridade, mas o orçamento pode limitar ou truncar seu conteúdo. `auto` depende de sobreposição de termos com a mensagem e do escopo; `retrievable` fica disponível para busca pela ferramenta, e `archived` fica fora do fluxo normal.

O assistente é orientado a buscar memórias quando a resposta depende de informações anteriores e o contexto já carregado não basta. Também deve considerar fatos duráveis apresentados em explicações, buscar registros relacionados antes de gravar e atualizar uma memória existente quando houver uma correção. Essas decisões dependem do modelo e da disponibilidade da ferramenta no perfil; a aplicação não garante que toda informação relevante será lembrada ou recuperada. Você pode conferir e corrigir os registros em **Memórias**.
