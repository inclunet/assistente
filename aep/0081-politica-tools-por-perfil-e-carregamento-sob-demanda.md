# AEP-0081 — Política de Tools por Perfil e Carregamento sob Demanda

Status: Draft
Criado em: 2026-07-01
Relacionado: AEP-0021, AEP-0049, AEP-0050, AEP-0063, AEP-0071, AEP-0072, AEP-0075, AEP-0077, AEP-0080

## Resumo

Esta AEP define a nova política de tools por perfil: cada tool passa a ter um estado efetivo tri-state no perfil, mantendo uma única lista/grid na UI do Profile Manager.

Estados:

- `disabled`: a tool fica bloqueada para o perfil, não aparece em descoberta via `tool_catalog` e não pode ser carregada.
- `on_demand`: a tool pode ser descoberta e carregada via `tool_catalog`, mas não nasce disponível no início do turno/conversa.
- `preloaded`: a tool nasce disponível no início do turno/conversa para aquele perfil.

O `tool_catalog` passa a ser a única interface de control-plane para descoberta e gestão de tools sob demanda. Ele recebe um campo opcional `action`; quando `action` for omitido, o comportamento padrão continua sendo busca/listagem para compatibilidade. Ações iniciais: `search`, `load`, `unload` e `list_loaded`.

Tools carregadas por `tool_catalog action=load` podem persistir por conversa/sessão em turnos futuros, até restart, TTL, mudança de `schema_hash`, estouro de budget ou `unload`, sempre respeitando allowlist/estado do perfil.

## Motivação

AEP-0049 introduziu o `tool_catalog` como catálogo persistido de builtins e MCP. AEP-0077 concluiu a centralização da seleção via `ToolSelectionPolicy` e `ToolPlanner`, com budget de schema, ranking e resolução bridge/native. Ainda falta representar, no perfil e na UI, uma política clara entre "bloqueado", "descobrível sob demanda" e "sempre disponível".

O contrato atual de `enabled_tools` é binário demais:

- `nil` significa default aberto;
- `[]` significa nenhuma tool;
- lista explícita significa allowlist de tools habilitadas.

Esse modelo não distingue tools que podem ser descobertas de tools que devem ser pre-carregadas. Como MCP bridges podem expor muitas tools e schemas grandes, carregar tudo por padrão aumenta custo, contexto e risco. Criar várias meta-tools para gerenciar tools também aumentaria superfície e confusão; por isso `tool_catalog` deve concentrar busca, load, unload e listagem de carregadas.

## Decisões

### D1. Profile Manager usa um único grid tri-state

A UI de tools no Profile Manager deve continuar sendo uma única lista/grid. Não deve haver grids separados para "desabilitadas", "sob demanda" e "pre-carregadas".

Cada linha exibe a tool e seu estado. A interação por teclado deve permitir alternar o estado com `Space`, em ciclo previsível:

```text
disabled -> on_demand -> preloaded -> disabled
```

Controles adicionais podem existir para edição em massa, mas a experiência principal permanece no grid único. O estado deve ser anunciado por leitor de tela quando mudar, usando o announcer global. A informação visual deve usar texto/badges, não apenas cor.

### D2. Semântica dos estados

O estado efetivo de cada tool combina configuração do perfil, disponibilidade do catálogo e política global de tool calling:

| Estado | Pode executar? | Aparece no `tool_catalog search`? | Entra no payload inicial? |
|---|---|---|---|
| `disabled` | Não | Não | Não |
| `on_demand` | Sim, após `load` | Sim | Não |
| `preloaded` | Sim | Sim | Sim |

Regras:

- `disabled` é denylist forte no escopo do perfil. Nem `tool_catalog`, nem MCP nativo, nem expansão dinâmica podem elevar essa tool.
- `on_demand` autoriza descoberta/carregamento, mas não consome budget inicial de schema.
- `preloaded` autoriza e solicita que a tool entre no conjunto inicial planejado, sujeita a availability, budget e conflitos native/bridge.
- `disable_tools=true` no perfil vence todos os estados: nenhuma tool de modelo é exposta, exceto caminhos de bootstrap/control-plane que a implementação declarar explicitamente necessários e seguros.

### D3. Compatibilidade com `enabled_tools`

Enquanto o schema persistido ainda for `enabled_tools`, a migração semântica deve ser determinística e sem perda:

- `enabled_tools` ausente ou `null`: perfil legado aberto. O runtime usa defaults do ToolPlanner; tools elegíveis entram como `on_demand`, e apenas o conjunto mínimo/control-plane entra como `preloaded`.
- `enabled_tools: []`: seleção explícita vazia. Todas as tools ficam `disabled`, exceto bootstrap/control-plane obrigatório quando permitido pela política.
- `enabled_tools: ["a", "b"]`: allowlist explícita. As tools listadas ficam inicialmente `preloaded` para preservar comportamento anterior; tools ausentes ficam `disabled`.

Quando houver novo schema de perfil, a representação recomendada é um mapa por tool:

```json
{
  "tool_policy": {
    "read_file": "preloaded",
    "web_search": "on_demand",
    "run_command": "disabled"
  }
}
```

Uma forma ordenada também é aceitável se a UI precisar preservar prioridade:

```json
{
  "tools": [
    { "name": "read_file", "state": "preloaded" },
    { "name": "web_search", "state": "on_demand" },
    { "name": "run_command", "state": "disabled" }
  ]
}
```

AEP-0050 deve tratar `profile_tools` como política tri-state quando a migração de profiles para banco for retomada. A tabela binária `profile_tools(profile_id, tool_name)` não é suficiente para o contrato novo.

### D4. `tool_catalog` e a interface única de control-plane

Não criar várias meta-tools para gerenciar tools. O `tool_catalog` concentra as operações de descoberta e carregamento:

```json
{ "action": "search", "query": "web", "risk": "network" }
{ "action": "load", "tools": ["web_search"] }
{ "action": "unload", "tools": ["web_search"] }
{ "action": "list_loaded" }
```

`action` é opcional. Se omitida, o comportamento é `search`, preservando chamadas antigas que apenas filtravam/listavam o catálogo.

Semântica das ações:

- `search`: lista somente tools visíveis para o perfil e disponíveis ao usuário/sessão atual. Tools `disabled` não aparecem.
- `load`: tenta tornar tools `on_demand` disponíveis para a conversa/sessão atual, respeitando allowlist, availability, risco, budget de schema e conflito MCP native/bridge.
- `unload`: remove tools carregadas sob demanda quando não forem bootstrap/control-plane nem preloaded obrigatórias.
- `list_loaded`: retorna tools efetivamente carregadas para a conversa/sessão, distinguindo `preloaded`, `loaded_on_demand` e `control_plane`.

Nomes alternativos como `activate`/`deactivate` foram considerados, mas `load`/`unload` refletem melhor o efeito real: disponibilizar ou remover schema/capacidade do conjunto efetivo, não executar a tool de domínio.

### D5. Persistência por conversa/sessão

Tools carregadas por `tool_catalog action=load` podem permanecer disponíveis em turnos seguintes da mesma conversa/sessão.

Escopo inicial:

- persistência in-memory por conversa/sessão;
- reset em restart do app;
- TTL configurável ou default conservador;
- invalidação quando `schema_hash` mudar ou a tool ficar `unavailable`;
- invalidação quando o perfil efetivo mudar, `disable_tools` for ativado, ou a allowlist deixar de permitir a tool;
- remoção explícita por `unload`;
- replanejamento quando o budget de schema for excedido.

A persistência de loaded tools não substitui o `tool_catalog`. Ela é um cache de disponibilidade efetiva para o agentic loop, derivado da política do perfil e do catálogo.

### D6. Budget, schema hash e governança

O budget de schema bytes do ToolPlanner continua sendo a barreira de entrada. `preloaded` é preferência forte, não garantia absoluta quando o budget for finito.

Regras:

- o planner deve contabilizar schemas preloaded e loaded-on-demand no conjunto acumulado da conversa/sessão;
- ao exceder budget, tools `on_demand` menos recentes ou menos prioritárias podem ser descarregadas antes de remover preloaded;
- mudança de `schema_hash` invalida a tool carregada e exige novo `load`;
- o resultado de `load` deve informar quais tools entraram, quais foram rejeitadas e por qual motivo (`disabled_by_profile`, `unavailable`, `budget_exceeded`, `risk_blocked`, `schema_changed`, `native_bridge_conflict` etc.).

### D7. Segurança e tools destrutivas

O estado `on_demand` ou `preloaded` não remove políticas de segurança da execução.

Regras:

- tools com risco `destructive`, `shell`, `write` ou HTTP mutável continuam exigindo allowlist, confirmação ou política específica quando aplicável;
- `tool_catalog search` deve filtrar ou marcar risco de forma textual e estruturada;
- `tool_catalog load` não pode carregar tools fora do escopo do perfil;
- MCP servers com muitas tools devem respeitar budget por schema e ranking; nenhum servidor deve despejar todo o catálogo por estar conectado;
- MCP native continua obedecendo AEP-0021: a política native/bridge é resolvida pelo perfil/provider, e a allowlist enviada ao provider nativo deve ser derivada do mesmo estado tri-state.

### D8. Bootstrap e control-plane

`tool_catalog` e `load_skill` são capacidades de bootstrap/control-plane. Para não quebrar descoberta:

- `tool_catalog` deve permanecer disponível como control-plane quando tool calling estiver habilitado e o perfil permitir descoberta de tools;
- `load_skill`, quando existir como tool, segue a política da AEP-0072: só é exposta quando tool calling estiver habilitado e skills `on_demand` puderem ser autoativadas pelo modelo;
- a UI pode mostrar essas capacidades como sistema/read-only ou sempre disponíveis conforme a política final;
- o usuário não deve precisar habilitar manualmente `tool_catalog` para conseguir descobrir tools em um perfil que permite tools sob demanda.

Se `disable_tools=true`, o runtime pode remover até mesmo `tool_catalog` do modelo. Nesse modo, `/skill` explícito do usuário continua sendo carregamento backend-driven, conforme AEP-0072, mas autoativação por modelo não ocorre.

### D9. Relação com Context Providers e SurfaceContext

Context Providers podem registrar tools de ação/consulta, mas a exposição dessas tools ao modelo passa pela mesma política tri-state e pelo ToolPlanner.

SurfaceContext (AEP-0080) pode influenciar ranking e pacotes preferenciais, mas não pode elevar uma tool `disabled`. Actions sobre surface continuam exigindo alvo estruturado e validação de staleness quando aplicável.

## Fases

### Fase 1 — AEP e contrato

- Aprovar esta AEP e atualizar referências em AEPs relacionadas.
- Definir nomes finais dos estados (`disabled`, `on_demand`, `preloaded`) e labels localizadas para UI.
- Definir schema de request/response de `tool_catalog` com `action` opcional.
- Nenhuma mudança funcional no runtime neste PR documental.

### Fase 2 — Política efetiva no backend

- Criar resolver de política tri-state a partir de `enabled_tools` legado.
- Integrar a política ao `ToolSelectionPolicy`/ToolPlanner sem mudar comportamento legado além do necessário.
- Garantir que `disabled` bloqueia search, load, MCP native allowlist e expansão dinâmica.
- Cobrir `nil`, `[]`, lista explícita e `disable_tools=true` por testes.

### Fase 3 — `tool_catalog` com ações

- Estender schema da tool com `action`.
- Implementar `search` como default quando `action` estiver ausente.
- Implementar `load`, `unload` e `list_loaded` com respostas estruturadas e motivos de rejeição.
- Manter compatibilidade com callers antigos de listagem.

### Fase 4 — Persistência por conversa/sessão

- Criar store in-memory de loaded tools por conversa/sessão.
- Aplicar TTL, restart reset, invalidação por `schema_hash`, availability, perfil e budget.
- Registrar eventos/telemetria de load/unload para auditoria de turno.

### Fase 5 — UI do Profile Manager

- Manter grid único de tools.
- Trocar checkbox/binário por controle tri-state acessível.
- `Space` alterna estado; setas continuam navegando o grid.
- Anunciar mudanças de estado via announcer.
- Adicionar textos i18n nos três idiomas e usar apenas tokens de tema.

### Fase 6 — Governança MCP e risco

- Validar servidores MCP com muitas tools sob budget finito.
- Garantir que tools destrutivas/write/shell continuam bloqueadas por allowlist/confirmação.
- Testar MCP native/bridge com allowlist derivada do tri-state.

## Riscos

| Risco | Impacto | Mitigação |
|---|---|---|
| Perfis legados mudarem comportamento | Alto | Migração semântica preserva lista explícita como `preloaded` e `[]` como tudo bloqueado. |
| `tool_catalog` virar meta-tool poderosa demais | Alto | Filtrar por perfil, risco, user scope e budget; `load` não executa tools de domínio. |
| Budget descarregar tool necessária | Médio | Resposta estruturada com motivo, ranking determinístico e possibilidade de recarregar. |
| MCP bridge com centenas de tools inflar contexto | Alto | `on_demand` como default para catálogo amplo, schema budget e omissão explícita. |
| UX tri-state confundir usuários | Médio | Grid único, labels textuais, ajuda curta e anúncio de estado. |
| Bootstrap quebrar descoberta | Alto | `tool_catalog` tratado como control-plane sempre disponível quando tools sob demanda forem permitidas. |
| Tool carregada ficar obsoleta após schema change | Médio | Invalidação por `schema_hash` e availability. |

## Critérios de aceitação

- Existe uma política tri-state por tool no perfil: `disabled`, `on_demand`, `preloaded`.
- O Profile Manager usa um único grid/lista de tools, com controle tri-state acessível e alternância por `Space`.
- `enabled_tools` legado tem migração/compatibilidade documentada e testada.
- `tool_catalog` aceita `action` opcional e assume `search` quando omitida.
- `tool_catalog` implementa `search`, `load`, `unload` e `list_loaded` sem criar meta-tools paralelas.
- `search` não revela tools `disabled` para o perfil.
- `load` não carrega tools fora da allowlist/estado do perfil e retorna motivos estruturados de rejeição.
- Tools carregadas sob demanda persistem por conversa/sessão até restart, TTL, `unload`, mudança de `schema_hash`, indisponibilidade, mudança de perfil ou budget.
- `tool_catalog` e `load_skill` são tratados como bootstrap/control-plane conforme a política de tool calling e AEP-0072.
- MCP bridge/native, tools destrutivas e budgets de schema respeitam a mesma política central.
- Nenhum fluxo alternativo de envio de mensagens é criado.
