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

Cada workspace mantém seu próprio conjunto de abas **self-contained**: cada aba tem seu estado isolado e é preservado ao alternar workspaces, com suporte a **split view** para ver duas abas lado a lado:

- **Chat** — conversas com o assistente
- **Editor** — arquivos para edição
- **Terminal** — sessões de comando
- **Task List** — listas de tarefas

O tipo de cada aba é identificado automaticamente e restaurado ao reabrir o workspace.

### Modelo por aba de chat

Quando o perfil usa um provider HTTP nativo, a barra do chat oferece **Modelo
desta aba**. A primeira opção, **Modelo do perfil**, mantém o comportamento do
perfil; escolher outro item substitui o modelo apenas naquela aba e nos próximos
turnos.

A escolha é salva no workspace e sobrevive ao fechamento do aplicativo. Ela
não altera o perfil e não segue a conversa: se a mesma conversa for aberta em
outra aba, a outra aba usa a própria escolha. Ao trocar de perfil, o app remove
automaticamente um modelo que pertença a outro provider.

Agentes ACP têm controles próprios de modelo e modo da sessão. Para eles, use
os seletores **Modelo do agente** e **Modo do agente** na barra do chat.

## Perfil por Workspace

Cada workspace pode ter um perfil de interação override, que define:

- Instruções de sistema customizadas
- Modelo e provedor preferido
- Parâmetros de geração (temperatura, etc.)

Isso permite, por exemplo, usar GPT-4o para trabalho e Claude para projetos pessoais, alternando apenas o workspace.
