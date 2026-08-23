# Arquitetura de Threads v2

**Status:** Done

## Estrutura Implementada

```
m1 n0: User → "pergunta" (raiz da thread)
  │
  └─ m2 n0: Assistant → resposta final (parentID=null, nova raiz)
```

## Fluxo de Mensagens

1. **User envia mensagem** (m1, n0, parentID=null)
   - Frontend envia para backend
   - Backend salva no banco, emite `chat:messages_ready`

2. **Assistant gera resposta** (m2, n0, parentID=null)
   - `OnDone` salva nova mensagem no nível 0
   - Visível na conversa principal

## API do LLM

O `loadConversationHistory` retorna apenas mensagens de nível 0:
- User messages
- Assistant responses finais

As interações internas (n1, n2) ficam nas threads e não são enviadas à API.

## Frontend

- `showInternalMessages=true`: Mostra threads expansíveis
- `showInternalMessages=false`: Mostra apenas n0 (user/assistant)
- Threads são expandidas com seta direita, colapsadas com seta esquerda

