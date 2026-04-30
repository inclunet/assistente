# AEP-0055 — Cliente Web + Workspace Mirroring via Agent

**Status**: 📝 Draft  
**Criado em**: 2026-04-29  
**Depende de**: AEP-0054 (Split Servidor/Cliente + Agent Local), AEP-0052 (Multi-User Accounts), AEP-0042 (Chat Surface Context)  
**Relacionado**: AEP-0034 (Unified Workspace)

---

## Resumo

Adicionar um **cliente web** (SPA) que se conecta ao mesmo servidor definido na AEP-0054, com duas formas de uso:

1. **Modo padrão**: o web opera recursos remotos do servidor (chat/tasklists/jobs/etc), sem tools locais.
2. **Modo espelho (MirroredWorkspace)**: o web “entra” em um workspace que está aberto em um desktop (Agent Local) e passa a pilotar editor/filesystem/terminal via relay, como se estivesse na máquina.

O desktop continua sendo o **single-writer** do filesystem: toda alteração é aplicada pelo agent e refletida ao web.

---

## Motivação

- Permitir acesso ao Assistente em qualquer navegador sem instalar o desktop.
- Permitir “usar meu workspace em casa via web” quando a máquina está ligada.
- Manter segurança e controle: o web não recebe acesso direto ao disco; tudo passa pelo agent autenticado.

---

## Decisões

### D1. A UI web é um processo/container separado
A SPA web é construída e distribuída separadamente (ex.: CDN/nginx) e consome a Resource API + streaming do servidor.

### D2. MirroredWorkspace é a experiência principal de workspace no web
O conceito de workspace no web é, primariamente, um **espelho** de um workspace do desktop:
- Fonte de verdade do estado persistido: `.assistente/workspace.yaml` no desktop.
- O web renderiza e envia comandos; o agent executa.

Um `ServerWorkspace` 100% remoto (sem agent) é opcional e fica para fase futura.

### D3. Agent Directory (UI) e seleção explícita
O servidor mantém o registro de agents (AEP-0054). O web:
- lista agents disponíveis do usuário
- lista workspaces disponíveis por agent
- persiste “último agent/workspace usado” para reabrir ao entrar no web

### D4. Mirror sessions são relays WS via servidor
O servidor cria uma `mirror_session` associada a:
- `user_id`
- `agent_id`
- `workspace_id`

E atua como relay:
- **web → agent**: comandos (abas, editor actions, terminal input)
- **agent → web**: eventos (workspace atualizado, terminal output, notificações de mudança)

O agent mantém conexão outbound WS para funcionar atrás de NAT.

### D5. Edição é single-writer no agent (sem CRDT)
Não haverá merge/CRDT nesta fase.
- O web envia ações/ops.
- O agent aplica no disco e retorna ack/conteúdo.

Se necessário, evoluir para lock por arquivo por mirror session.

### D6. Auth e autorização seguem a AEP-0052
- O web autentica no servidor como qualquer cliente.
- O servidor só permite mirroring quando `user_id` do web coincide com `user_id` do agent.
- Revogação: o usuário pode revogar um agent e invalidar sessões/tokens.

---

## Fases

### Fase 1 — Web client padrão (sem mirroring)
- SPA web consumindo chat via Resource API + streaming.

### Fase 2 — Listar agents + abrir mirrored workspace
- Tela/listagem de agents + workspaces.
- Abrir mirrored workspace e refletir abas e estado.

### Fase 3 — Terminal espelhado
- Terminal input/output via relay.

### Fase 4 — Editor/filesystem espelhados
- Ler/editar arquivo via agent.
- Notificações de mudança e refresh.

---

## Riscos

- **Latência e UX**: espelho depende de rede; precisa de indicadores de conexão e reconexão.
- **Segurança**: mirror session é canal sensível; exige correlação forte e policy por tool.
- **Disponibilidade**: o agent pode cair; o web precisa lidar com offline.

---

## Critérios de aceitação

1. **Web padrão**: SPA web autentica e acessa chat via Resource API + streaming.
2. **Agent directory**: web lista agents disponíveis do usuário e seus workspaces.
3. **MirroredWorkspace**: web abre o último workspace usado (agent/workspace) e vê abas do desktop.
4. **Terminal**: web envia input; recebe output do terminal do desktop.
5. **Editor/filesystem**: web edita um arquivo e a alteração aparece no disco do desktop (aplicada pelo agent).
6. **Revogação**: usuário revoga um agent e sessões de mirror são encerradas.
