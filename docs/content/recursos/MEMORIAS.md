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

Memórias entram no prompt por orçamento (~1200 caracteres), ordenadas por política e importância, filtradas por escopo e relevância ao texto atual. `core`/`pinned` sempre entram; `auto` só se houver sobreposição de termos com a mensagem.
