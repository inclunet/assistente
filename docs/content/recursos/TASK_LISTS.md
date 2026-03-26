---
title: "Listas de Tarefas"
weight: 4
---

# Listas de Tarefas

O sistema de listas de tarefas permite gerenciar tarefas com workflows customizáveis, suporte a subtarefas e visualização em lista ou kanban.

## Conceito

Cada lista de tarefas possui:

- **Título e descrição**
- **Workflow**: Define os status possíveis e transições permitidas
- **Tarefas**: Itens com título, descrição e subtarefas
- **Modo de visualização**: Lista ou Kanban

## Operações

### Listas

| Ação | Descrição |
|---|---|
| **Criar** | Nova lista com título e descrição |
| **Editar** | Alterar título e descrição |
| **Clonar** | Duplicar lista com workflow e tarefas |
| **Deletar** | Remover lista completamente |

### Tarefas

| Ação | Descrição |
|---|---|
| **Criar tarefa** | Com título, descrição e tarefa-pai (subtarefas) |
| **Atualizar status** | Mover tarefa entre status (validado pelo workflow) |
| **Reordenar** | Arrastar tarefas dentro de um status |
| **Subtarefas** | Criar tarefas filhas para quebrar em partes menores |

### Workflow

Cada lista tem seu próprio workflow com:

- **Status**: Lista de status possíveis (ex: A Fazer, Em Progresso, Concluído)
- **Transições**: Regras de quais status podem avançar para quais
- **Reordenação**: Status podem ser reordenados

## Modos de Visualização

- **Lista**: Visualização linear com agrupamento por status
- **Kanban**: Colunas por status, com drag-and-drop

Alterne entre modos via botão na toolbar.

## Integração com IA

O assistente pode gerenciar listas de tarefas via tool calling:

- `list_task_lists` — listar todas as listas
- `create_task_list` — criar nova lista
- `create_task` — adicionar tarefa
- `update_task_status` — mover tarefa entre status

Isso permite pedir ao assistente: "crie uma lista de tarefas para o projeto X" ou "mova as tarefas concluídas".

## Deep Links

Acesse uma lista diretamente via: `assistente://tasklist/{id}`
