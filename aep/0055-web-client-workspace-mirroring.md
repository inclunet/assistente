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

### D7. Pareamento do agent sem prompt (mesma conta)
- O agent mantém uma conexão outbound (WebSocket) autenticada no servidor usando o mesmo modelo de auth/sessão da AEP-0052.
- Não há confirmação interativa no desktop para iniciar um mirror: a autorização é derivada do `user_id` e das policies.
- O servidor deve oferecer revogação (desconectar agent, invalidar sessão/token do agent) para cortar o acesso.

### D8. Lifecycle de `mirror_session`
- `mirror_session` é efêmera e existe enquanto:
	- o browser está conectado;
	- o agent está conectado;
	- e o servidor mantém a sessão válida.
- Queda do agent encerra (ou pausa) a sessão; o web deve tratar como offline e permitir reconectar.
- O servidor persiste por usuário o “último alvo” (ex.: `last_agent_id` + `last_workspace_id`) para reabrir o último workspace ao entrar no web.

### D9. Policy de execução: web não executa tools, apenas aciona agent quando espelhado
- No modo padrão (sem mirror): o web não possui ferramentas locais; apenas consome recursos e ações expostas na Resource API.
- No modo espelho: operações de filesystem/editor/terminal são executadas no agent, e o servidor atua como relay.
- O servidor aplica autorização por ação/tool e por `workspace_roots` anunciados pelo agent.

### D10. Escopo de filesystem (roots) é obrigatório no espelho
- O agent deve anunciar roots permitidas por workspace e rejeitar qualquer operação fora dessas roots.
- O servidor deve reforçar a policy (defesa em profundidade): não encaminhar requests claramente fora das roots.

### D11. Streaming e limites (backpressure)
- Terminal output e eventos de workspace devem suportar:
	- chunking/paginação;
	- limites de tamanho por mensagem;
	- backpressure (evitar crescimento ilimitado de buffers no servidor).
- Resultados grandes (ex.: leitura de arquivo) devem ter limite e estratégia explícita (ex.: truncamento + indicação de que foi truncado).

### D12. Surface Context no web permanece alinhado à AEP-0042
- Mesmo no espelho, o web deve continuar enviando `surfaceStateJson`/`surfaceContextJson` quando disparar chat.
- Quando o contexto envolver editor/terminal, o web não deve “inventar” acesso local: qualquer dado que dependa do ambiente do desktop deve ser obtido via agent.

### D13. O web suporta `auth.mode=external` e também `auth.mode=local`
- **Preferência**: `auth.mode=external` (OIDC/IdP) para ambiente gerenciado.
- **Obrigatório**: quando o servidor estiver configurado em `auth.mode=local`, o cliente web deve conseguir autenticar e operar normalmente.

### D14. Storage de sessão no browser (modo local)
No modo `auth.mode=local`, o web precisa de uma estratégia segura de sessão:
- **Access token**: mantido apenas em memória (não persistir em `localStorage`).
- **Refresh token**: armazenado em **cookie `HttpOnly` + `Secure` + `SameSite`** emitido pelo servidor.
- O endpoint de refresh usa o cookie e retorna um novo access token.

### D15. CORS e CSRF
- Como a SPA web roda em origem diferente da API, o servidor deve ter uma política de CORS **restritiva** (allowlist de origins do web).
- No modo `auth.mode=local` (refresh em cookie), o servidor deve proteger endpoints state-changing contra CSRF.
	- Recomendação: `SameSite` + token CSRF (double-submit ou equivalente) para chamadas que dependem de cookie.

### D16. Autenticação de streaming/mirroring (alto nível)
- Conexões de streaming (chat) e mirror devem estar autenticadas e associadas a `user_id`.
- O servidor deve rejeitar conexões sem credenciais válidas e encerrar sessões quando houver revogação.
- Não é objetivo desta AEP padronizar o schema completo das mensagens WS; apenas garantir autenticação, correlação e limites (ver D11).

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
- **Exfiltração acidental**: sem roots/policy, o espelho pode dar acesso amplo ao disco; roots e defesa em profundidade são obrigatórios.
- **Backpressure**: terminal output pode explodir memória; limites e chunking são mandatórios.
- **Session storage inseguro**: persistir tokens no browser (ex.: `localStorage`) aumenta o impacto de XSS; o modo local deve preferir refresh em cookie `HttpOnly`.
- **CORS/CSRF**: web separado exige políticas explícitas para evitar abuso cross-site.

---

## Critérios de aceitação

1. **Web padrão**: SPA web autentica e acessa chat via Resource API + streaming.
2. **Agent directory**: web lista agents disponíveis do usuário e seus workspaces.
3. **MirroredWorkspace**: web abre o último workspace usado (agent/workspace) e vê abas do desktop.
4. **Terminal**: web envia input; recebe output do terminal do desktop.
5. **Editor/filesystem**: web edita um arquivo e a alteração aparece no disco do desktop (aplicada pelo agent).
6. **Revogação**: usuário revoga um agent e sessões de mirror são encerradas.
7. **Roots enforced**: o agent recusa operações fora das roots; tentativa é auditável e não vaza dados.
8. **Limites**: terminal output e leitura de arquivos respeitam limites e sinalizam truncamento quando aplicável.
9. **Auth local no web**: em `auth.mode=local`, o web autentica e mantém sessão sem armazenar refresh token em JS.
10. **CORS/CSRF**: API rejeita origins não autorizadas e endpoints com cookie são protegidos contra CSRF.
