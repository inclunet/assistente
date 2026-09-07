---
title: "Deep Links"
weight: 5
---

# Deep Links

> **Em 2 linhas:** links `assistente://` abrem o app direto em uma conversa, arquivo ou tarefa — útil para automações e skills te levarem ao lugar certo com um clique.

O Assistente suporta deep links via protocolo `assistente://`, permitindo navegação programática entre recursos do aplicativo.

## URIs Suportadas

| URI | Ação |
|---|---|
| `assistente://conversation/{id}` | Abre uma conversa existente pelo ID |
| `assistente://conversation/new?message=...` | Cria nova conversa com mensagem inicial |
| `assistente://conversation/{id}/send?message=...` | Envia uma mensagem para uma conversa existente |
| `assistente://tasklist/{id}` | Abre uma lista de tarefas |
| `assistente://editor/{id}` | Abre um arquivo no editor |
| `assistente://terminal/{id}` | Abre uma sessão de terminal |
| `assistente://navigate/{route}` | Navega para uma rota da aplicação |

### Parâmetro opcional `profile`

As ações de conversa (`new`, abrir e `send`) aceitam o parâmetro opcional `profile={slug}`, que força a conversa-alvo a usar um perfil específico — sem alterar o perfil global. Útil para disparar uma conversa já com o perfil certo, por exemplo uma análise de suporte técnico ou uma tarefa de programação.

```
assistente://conversation/new?message=analise+este+ticket&profile=techsupport
assistente://conversation/{id}?profile=programacao
assistente://conversation/{id}/send?message=revise+este+código&profile=programacao
```

O perfil é aplicado como _override_ da aba (mesmo mecanismo do seletor de perfil da barra do chat). Se o `slug` informado não existir, um aviso é exibido e a conversa segue com o perfil padrão.

## Uso em Skills

Deep links são usados internamente por skills para navegar o usuário a recursos específicos. Por exemplo, uma skill pode:

```
Sua lista de tarefas foi criada! Acesse em assistente://tasklist/abc123
```

## Tool Calling

O assistente pode usar a ferramenta `open_deep_link` para abrir deep links programaticamente:

```json
{
  "name": "open_deep_link",
  "arguments": {
    "uri": "assistente://conversation/new?message=Olá"
  }
}
```

## Validação

- URIs devem começar com `assistente://`
- O formato é validado antes da navegação
- URIs inválidas retornam erro descritivo
