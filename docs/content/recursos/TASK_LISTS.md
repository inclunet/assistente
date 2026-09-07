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

### Filtrar e paginar tarefas

Para boards grandes e automações, a tool `task_list` aceita consultas
limitadas no banco. Informe `task_list_id` ou `task_list_slug` e combine:

- `status_id`: retorna apenas tarefas daquele status do workflow;
- `limit`: limita a página entre 1 e 100 tarefas (padrão 100);
- `sort`: usa `created_at:asc` para tarefas mais antigas primeiro ou
  `created_at:desc` para as mais novas primeiro;
- `cursor`: continua a partir do `next_cursor` retornado pela página anterior.

O resultado informa `has_more` e `next_cursor`. Enquanto `has_more` for
verdadeiro, envie o cursor seguinte mantendo a mesma lista, o mesmo
`status_id` e o mesmo `sort`. O cursor é opaco: não deve ser editado nem
reutilizado com outro filtro.

Exemplo para buscar as 20 tarefas mais antigas do status 1:

```json
{
  "task_list_slug": "noticias-tai",
  "status_id": 1,
  "limit": 20,
  "sort": "created_at:asc"
}
```

As páginas são planas para que `limit` seja um limite real. Subtarefas
aparecem como itens próprios com `parent_id`. Sem os parâmetros de filtro e
paginação, a leitura completa anterior continua disponível e preserva a
hierarquia.

### Ler e paginar notas

A tool `task_note` mantém as operações existentes de criação, atualização e
sincronização. Para ler notas sem alterar dados, envie `list: true`. A consulta
é feita diretamente no banco, retorna no máximo 100 itens e pode ser global ou
limitada a uma tarefa:

- `task_id`: UUID exato da tarefa;
- `task_code`: código da tarefa, opcionalmente desambiguado por
  `task_list_id` ou `task_list_slug`;
- `source`: origem externa exata, como `jira`;
- `type`: `1` interna, `2` cliente, `3` agente ou `4` sistema;
- `external_id`: identificador exato do comentário remoto;
- `external_parent_id`: identificador exato do pai ou da thread;
- `limit`, `sort` e `cursor`: mesmo contrato de páginas estáveis usado para
  tarefas.

Nos filtros externos, omitir o campo significa “não filtrar”. Enviar `null`
seleciona notas sem aquele valor; por exemplo,
`"external_parent_id": null` retorna notas no nível principal. Strings vazias
não são aceitas como filtro.

Exemplo para processar respostas de clientes mais antigas:

```json
{
  "list": true,
  "type": 2,
  "limit": 20,
  "sort": "created_at:asc"
}
```

O resultado contém `notes`, `has_more` e `next_cursor`. Para continuar, repita
`list: true`, os mesmos filtros, `limit` e `sort`, acrescentando o cursor
recebido. O cursor é opaco e não funciona com filtros diferentes. A ordem usa
`created_at` e o UUID da nota como desempate determinístico.

## Deep Links

Acesse uma lista diretamente via: `assistente://tasklist/{id}`
