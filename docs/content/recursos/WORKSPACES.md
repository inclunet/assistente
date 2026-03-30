---
title: "Workspaces"
weight: 1
---

# Workspaces

Workspaces permitem organizar conversas, abas de editor, terminais e listas de tarefas em espaços de trabalho separados — ideal para separar contextos como projetos, clientes ou áreas de trabalho.

## Conceito

Um workspace é um container que agrupa:

- **Abas de chat** — conversas relacionadas ao contexto
- **Abas de editor** — arquivos abertos
- **Abas de terminal** — sessões de terminal
- **Listas de tarefas** — tarefas do projeto
- **Perfil override** — cada workspace pode usar um perfil de interação diferente

## Localização dos Dados

Os workspaces são armazenados em:

- **Workspace avulso**: `~/.assistente/workspaces/<id>/workspace.yaml`
- **Workspace de diretório**: `<diretório>/.assistente/workspace.yaml`
- **Config global**: `~/.assistente/` (home directory)

## Operações

| Ação | Como |
|---|---|
| **Criar workspace** | Menu → Novo Workspace |
| **Alternar workspace** | Menu → selecione o workspace desejado |
| **Renomear** | Menu → Renomear Workspace ativo |
| **Deletar** | Menu → Deletar Workspace |

## Abas por Workspace

Cada workspace mantém seu próprio conjunto de abas, com estado (ativa + lista) preservado ao alternar:

- **Chat** — conversas com o assistente
- **Editor** — arquivos para edição
- **Terminal** — sessões de comando
- **Task List** — listas de tarefas

O tipo de cada aba é identificado automaticamente e restaurado ao reabrir o workspace.

## Perfil por Workspace

Cada workspace pode ter um perfil de interação override, que define:

- Instruções de sistema customizadas
- Modelo e provedor preferido
- Parâmetros de geração (temperatura, etc.)

Isso permite, por exemplo, usar GPT-4o para trabalho e Claude para projetos pessoais, alternando apenas o workspace.
