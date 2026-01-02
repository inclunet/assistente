# Arquitetura de Threads v2

## Estrutura Implementada

```
m1 n0: User → "pergunta" (raiz da thread)
  │
  ├─ m2 n1: Assistant → "🔧 Chamando: file_manager" (parentID=m1)
  │    │
  │    ├─ m3 n2: Agent → request para tool (parentID=m2)
  │    │
  │    └─ m4 n2: Tool → resposta (parentID=m2)
  │
  └─ m5 n1: Agent → resposta final (parentID=m1)

m6 n0: Assistant → resposta final visível (parentID=null, nova raiz)
```

## Fluxo de Mensagens

1. **User envia mensagem** (m1, n0, parentID=null)
   - Frontend envia para backend
   - Backend salva no banco, emite `chat:messages_ready`

2. **Assistant decide chamar agente** (m2, n1, parentID=m1)
   - `OnToolCalls` salva mensagem com tool_calls
   - ParentID aponta para mensagem do usuário

3. **Agent executa tools** (m3, m4, n2, parentID=m2)
   - `createAgentMessageSaver` salva cada interação
   - ParentID aponta para mensagem de tool_calls do assistant

4. **Agent retorna resposta** (m5, n1, parentID=m1)
   - `OnToolResults` salva resposta do agente
   - ParentID aponta para mensagem do usuário

5. **Assistant gera resposta final** (m6, n0, parentID=null)
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

