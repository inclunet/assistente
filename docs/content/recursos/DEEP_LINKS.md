---
title: "Deep Links"
weight: 5
---

# Deep Links

O Assistente suporta deep links via protocolo `assistente://`, permitindo navegação programática entre recursos do aplicativo.

## URIs Suportadas

| URI | Ação |
|---|---|
| `assistente://conversation/{id}` | Abre uma conversa existente pelo ID |
| `assistente://conversation/new?message=...` | Cria nova conversa com mensagem inicial |
| `assistente://tasklist/{id}` | Abre uma lista de tarefas |
| `assistente://editor/{id}` | Abre um arquivo no editor |
| `assistente://terminal/{id}` | Abre uma sessão de terminal |
| `assistente://navigate/{route}` | Navega para uma rota da aplicação |

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
