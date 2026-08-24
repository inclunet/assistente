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

- A timeline principal renderiza mensagens raiz e indica quando há filhos.
- Os filhos são carregados/renderizados como thread expansível.
- Threads são expandidas com seta direita e recolhidas com seta esquerda.

## Fases e estado verificado

### Fase 1 — Modelo e persistência ✅

- [x] `chat.Message` e `database.ChatMessage` transportam `ParentID` opcional.
- [x] Raízes usam `parent_id = NULL`; descendentes referenciam mensagens por
  UUID.
- [x] A migração UUID preserva cadeias e referências futuras de `parent_id`.

### Fase 2 — Árvore e histórico ✅

- [x] O backend separa raízes e filhos e monta a árvore em ordem determinística.
- [x] A leitura de histórico para o LLM consulta somente mensagens raiz.
- [x] A consulta recursiva retorna hierarquias profundas sem atravessar o
  escopo do usuário.

### Fase 3 — Interface acessível ✅

- [x] A interface renderiza indicador apenas quando existem filhos.
- [x] Threads podem ser expandidas/recolhidas e expõem `aria-expanded`.
- [x] Navegação em modo de leitura usa setas para entrar na thread ou retornar
  ao pai.

## Critérios de aceitação

- [x] Mensagens raiz e internas permanecem distinguíveis por `ParentID`.
- [x] A resposta final do assistant pode permanecer no nível principal,
  associada ao turno sem virar filha artificial.
- [x] Interações internas não contaminam o histórico raiz enviado ao LLM.
- [x] Filhos são ordenados de forma estável e o carregamento preserva
  isolamento por usuário.
- [x] O controle de thread é operável por mouse e teclado e anuncia seu estado.

## Evidências

- tipos, árvore e janela por thread:
  `internal/chat/types.go`, `conversation_service.go`, `history.go` e
  `conversation_service_test.go`;
- persistência, ordenação e recursão:
  `internal/database/message_repository.go`, `ordering_test.go` e
  `message_tree_descendants_test.go`;
- preservação de hierarquia na migração:
  `internal/database/migration_uuid_test.go`;
- interface e acessibilidade:
  `frontend/src/components/chat/MessageNode.tsx`,
  `ThreadIndicator.tsx`, `MessageNode.test.tsx` e
  `ThreadIndicator.test.tsx`.

