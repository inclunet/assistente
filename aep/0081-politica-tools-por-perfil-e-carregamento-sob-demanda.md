# AEP-0081 — Política de Tools por Perfil e Carregamento sob Demanda

Status: Draft
Criado em: 2026-07-01
Relacionado: AEP-0021, AEP-0049, AEP-0050, AEP-0063, AEP-0071, AEP-0072, AEP-0075, AEP-0077, AEP-0080

## Resumo

Esta AEP define a nova politica de tools por perfil: cada tool passa a ter um estado efetivo tri-state no perfil, mantendo uma unica lista/grid na UI do Profile Manager.

Estados:

- `disabled`: a tool fica bloqueada para o perfil, nao aparece em descoberta via `tool_catalog` e nao pode ser carregada.
- `on_demand`: a tool pode ser descoberta e carregada via `tool_catalog`, mas nao nasce disponivel no inicio do turno/conversa.
- `preloaded`: a tool nasce disponivel no inicio do turno/conversa para aquele perfil.

O `tool_catalog` passa a ser a unica interface de control-plane para descoberta e gestao de tools sob demanda. Ele recebe um campo opcional `action`; quando `action` for omitido, o comportamento padrao continua sendo busca/listagem para compatibilidade. Acoes iniciais: `search`, `load`, `unload` e `list_loaded`.

Tools carregadas por `tool_catalog action=load` podem persistir por conversa/sessao em turnos futuros, ate restart, TTL, mudanca de `schema_hash`, estouro de budget ou `unload`, sempre respeitando allowlist/estado do perfil.

## Motivação

AEP-0049 introduziu o `tool_catalog` como catalogo persistido de builtins e MCP. AEP-0077 concluiu a centralizacao da selecao via `ToolSelectionPolicy` e `ToolPlanner`, com budget de schema, ranking e resolucao bridge/native. Ainda falta representar, no perfil e na UI, uma politica clara entre "bloqueado", "descobrivel sob demanda" e "sempre disponivel".

O contrato atual de `enabled_tools` e binario demais:

- `nil` significa default aberto;
- `[]` significa nenhuma tool;
- lista explicita significa allowlist de tools habilitadas.

Esse modelo nao distingue tools que podem ser descobertas de tools que devem ser pre-carregadas. Como MCP bridges podem expor muitas tools e schemas grandes, carregar tudo por padrao aumenta custo, contexto e risco. Criar varias meta-tools para gerenciar tools tambem aumentaria superficie e confusao; por isso `tool_catalog` deve concentrar busca, load, unload e listagem de carregadas.

## Decisões

### D1. Profile Manager usa um unico grid tri-state

A UI de tools no Profile Manager deve continuar sendo uma unica lista/grid. Nao deve haver grids separados para "desabilitadas", "sob demanda" e "pre-carregadas".

Cada linha exibe a tool e seu estado. A interacao por teclado deve permitir alternar o estado com `Space`, em ciclo previsivel:

```text
disabled -> on_demand -> preloaded -> disabled
```

Controles adicionais podem existir para edicao em massa, mas a experiencia principal permanece no grid unico. O estado deve ser anunciado por leitor de tela quando mudar, usando o announcer global. A informacao visual deve usar texto/badges, nao apenas cor.

### D2. Semantica dos estados

O estado efetivo de cada tool combina configuracao do perfil, disponibilidade do catalogo e politica global de tool calling:

| Estado | Pode executar? | Aparece no `tool_catalog search`? | Entra no payload inicial? |
|---|---|---|---|
| `disabled` | Nao | Nao | Nao |
| `on_demand` | Sim, apos `load` | Sim | Nao |
| `preloaded` | Sim | Sim | Sim |

Regras:

- `disabled` e denylist forte no escopo do perfil. Nem `tool_catalog`, nem MCP nativo, nem expansao dinamica podem elevar essa tool.
- `on_demand` autoriza descoberta/carregamento, mas nao consome budget inicial de schema.
- `preloaded` autoriza e solicita que a tool entre no conjunto inicial planejado, sujeita a availability, budget e conflitos native/bridge.
- `disable_tools=true` no perfil vence todos os estados: nenhuma tool de modelo e exposta, exceto caminhos de bootstrap/control-plane que a implementacao declarar explicitamente necessarios e seguros.

### D3. Compatibilidade com `enabled_tools`

Enquanto o schema persistido ainda for `enabled_tools`, a migracao semantica deve ser deterministica e sem perda:

- `enabled_tools` ausente ou `null`: perfil legado aberto. O runtime usa defaults do ToolPlanner; tools elegiveis entram como `on_demand`, e apenas o conjunto minimo/control-plane entra como `preloaded`.
- `enabled_tools: []`: selecao explicita vazia. Todas as tools ficam `disabled`, exceto bootstrap/control-plane obrigatorio quando permitido pela politica.
- `enabled_tools: ["a", "b"]`: allowlist explicita. As tools listadas ficam inicialmente `preloaded` para preservar comportamento anterior; tools ausentes ficam `disabled`.

Quando houver novo schema de perfil, a representacao recomendada e um mapa por tool:

```json
{
  "tool_policy": {
    "read_file": "preloaded",
    "web_search": "on_demand",
    "run_command": "disabled"
  }
}
```

Uma forma ordenada tambem e aceitavel se a UI precisar preservar prioridade:

```json
{
  "tools": [
    { "name": "read_file", "state": "preloaded" },
    { "name": "web_search", "state": "on_demand" },
    { "name": "run_command", "state": "disabled" }
  ]
}
```

AEP-0050 deve tratar `profile_tools` como politica tri-state quando a migracao de profiles para banco for retomada. A tabela binaria `profile_tools(profile_id, tool_name)` nao e suficiente para o contrato novo.

### D4. `tool_catalog` e a interface unica de control-plane

Nao criar varias meta-tools para gerenciar tools. O `tool_catalog` concentra as operacoes de descoberta e carregamento:

```json
{ "action": "search", "query": "web", "risk": "network" }
{ "action": "load", "tools": ["web_search"] }
{ "action": "unload", "tools": ["web_search"] }
{ "action": "list_loaded" }
```

`action` e opcional. Se omitida, o comportamento e `search`, preservando chamadas antigas que apenas filtravam/listavam o catalogo.

Semantica das acoes:

- `search`: lista somente tools visiveis para o perfil e disponiveis ao usuario/sessao atual. Tools `disabled` nao aparecem.
- `load`: tenta tornar tools `on_demand` disponiveis para a conversa/sessao atual, respeitando allowlist, availability, risco, budget de schema e conflito MCP native/bridge.
- `unload`: remove tools carregadas sob demanda quando nao forem bootstrap/control-plane nem preloaded obrigatorias.
- `list_loaded`: retorna tools efetivamente carregadas para a conversa/sessao, distinguindo `preloaded`, `loaded_on_demand` e `control_plane`.

Nomes alternativos como `activate`/`deactivate` foram considerados, mas `load`/`unload` refletem melhor o efeito real: disponibilizar ou remover schema/capacidade do conjunto efetivo, nao executar a tool de dominio.

### D5. Persistencia por conversa/sessao

Tools carregadas por `tool_catalog action=load` podem permanecer disponiveis em turnos seguintes da mesma conversa/sessao.

Escopo inicial:

- persistencia in-memory por conversa/sessao;
- reset em restart do app;
- TTL configuravel ou default conservador;
- invalidacao quando `schema_hash` mudar ou a tool ficar `unavailable`;
- invalidacao quando o perfil efetivo mudar, `disable_tools` for ativado, ou a allowlist deixar de permitir a tool;
- remocao explicita por `unload`;
- replanejamento quando o budget de schema for excedido.

A persistencia de loaded tools nao substitui o `tool_catalog`. Ela e um cache de disponibilidade efetiva para o agentic loop, derivado da politica do perfil e do catalogo.

### D6. Budget, schema hash e governanca

O budget de schema bytes do ToolPlanner continua sendo a barreira de entrada. `preloaded` e preferencia forte, nao garantia absoluta quando o budget for finito.

Regras:

- o planner deve contabilizar schemas preloaded e loaded-on-demand no conjunto acumulado da conversa/sessao;
- ao exceder budget, tools `on_demand` menos recentes ou menos prioritarias podem ser descarregadas antes de remover preloaded;
- mudanca de `schema_hash` invalida a tool carregada e exige novo `load`;
- o resultado de `load` deve informar quais tools entraram, quais foram rejeitadas e por qual motivo (`disabled_by_profile`, `unavailable`, `budget_exceeded`, `risk_blocked`, `schema_changed`, `native_bridge_conflict` etc.).

### D7. Segurança e tools destrutivas

O estado `on_demand` ou `preloaded` nao remove politicas de seguranca da execucao.

Regras:

- tools com risco `destructive`, `shell`, `write` ou HTTP mutavel continuam exigindo allowlist, confirmacao ou politica especifica quando aplicavel;
- `tool_catalog search` deve filtrar ou marcar risco de forma textual e estruturada;
- `tool_catalog load` nao pode carregar tools fora do escopo do perfil;
- MCP servers com muitas tools devem respeitar budget por schema e ranking; nenhum servidor deve despejar todo o catalogo por estar conectado;
- MCP native continua obedecendo AEP-0021: a politica native/bridge e resolvida pelo perfil/provider, e a allowlist enviada ao provider nativo deve ser derivada do mesmo estado tri-state.

### D8. Bootstrap e control-plane

`tool_catalog` e `load_skill` sao capacidades de bootstrap/control-plane. Para nao quebrar descoberta:

- `tool_catalog` deve permanecer disponivel como control-plane quando tool calling estiver habilitado e o perfil permitir descoberta de tools;
- `load_skill`, quando existir como tool, segue a politica da AEP-0072: so e exposta quando tool calling estiver habilitado e skills `on_demand` puderem ser autoativadas pelo modelo;
- a UI pode mostrar essas capacidades como sistema/read-only ou sempre disponiveis conforme a politica final;
- o usuario nao deve precisar habilitar manualmente `tool_catalog` para conseguir descobrir tools em um perfil que permite tools sob demanda.

Se `disable_tools=true`, o runtime pode remover ate mesmo `tool_catalog` do modelo. Nesse modo, `/skill` explicito do usuario continua sendo carregamento backend-driven, conforme AEP-0072, mas autoativacao por modelo nao ocorre.

### D9. Relacao com Context Providers e SurfaceContext

Context Providers podem registrar tools de acao/consulta, mas a exposicao dessas tools ao modelo passa pela mesma politica tri-state e pelo ToolPlanner.

SurfaceContext (AEP-0080) pode influenciar ranking e pacotes preferenciais, mas nao pode elevar uma tool `disabled`. Actions sobre surface continuam exigindo alvo estruturado e validacao de staleness quando aplicavel.

## Fases

### Fase 1 — AEP e contrato

- Aprovar esta AEP e atualizar referencias em AEPs relacionadas.
- Definir nomes finais dos estados (`disabled`, `on_demand`, `preloaded`) e labels localizadas para UI.
- Definir schema de request/response de `tool_catalog` com `action` opcional.
- Nenhuma mudanca funcional no runtime neste PR documental.

### Fase 2 — Politica efetiva no backend

- Criar resolver de politica tri-state a partir de `enabled_tools` legado.
- Integrar a politica ao `ToolSelectionPolicy`/ToolPlanner sem mudar comportamento legado alem do necessario.
- Garantir que `disabled` bloqueia search, load, MCP native allowlist e expansao dinamica.
- Cobrir `nil`, `[]`, lista explicita e `disable_tools=true` por testes.

### Fase 3 — `tool_catalog` com acoes

- Estender schema da tool com `action`.
- Implementar `search` como default quando `action` estiver ausente.
- Implementar `load`, `unload` e `list_loaded` com respostas estruturadas e motivos de rejeicao.
- Manter compatibilidade com callers antigos de listagem.

### Fase 4 — Persistencia por conversa/sessao

- Criar store in-memory de loaded tools por conversa/sessao.
- Aplicar TTL, restart reset, invalidacao por `schema_hash`, availability, perfil e budget.
- Registrar eventos/telemetria de load/unload para auditoria de turno.

### Fase 5 — UI do Profile Manager

- Manter grid unico de tools.
- Trocar checkbox/binario por controle tri-state acessivel.
- `Space` alterna estado; setas continuam navegando o grid.
- Anunciar mudancas de estado via announcer.
- Adicionar textos i18n nos tres idiomas e usar apenas tokens de tema.

### Fase 6 — Governanca MCP e risco

- Validar servidores MCP com muitas tools sob budget finito.
- Garantir que tools destrutivas/write/shell continuam bloqueadas por allowlist/confirmacao.
- Testar MCP native/bridge com allowlist derivada do tri-state.

## Riscos

| Risco | Impacto | Mitigacao |
|---|---|---|
| Perfis legados mudarem comportamento | Alto | Migracao semantica preserva lista explicita como `preloaded` e `[]` como tudo bloqueado. |
| `tool_catalog` virar meta-tool poderosa demais | Alto | Filtrar por perfil, risco, user scope e budget; `load` nao executa tools de dominio. |
| Budget descarregar tool necessaria | Medio | Resposta estruturada com motivo, ranking deterministico e possibilidade de recarregar. |
| MCP bridge com centenas de tools inflar contexto | Alto | `on_demand` como default para catalogo amplo, schema budget e omissao explicita. |
| UX tri-state confundir usuarios | Medio | Grid unico, labels textuais, ajuda curta e anuncio de estado. |
| Bootstrap quebrar descoberta | Alto | `tool_catalog` tratado como control-plane sempre disponivel quando tools sob demanda forem permitidas. |
| Tool carregada ficar obsoleta apos schema change | Medio | Invalidacao por `schema_hash` e availability. |

## Critérios de aceitação

- Existe uma politica tri-state por tool no perfil: `disabled`, `on_demand`, `preloaded`.
- O Profile Manager usa um unico grid/lista de tools, com controle tri-state acessivel e alternancia por `Space`.
- `enabled_tools` legado tem migracao/compatibilidade documentada e testada.
- `tool_catalog` aceita `action` opcional e assume `search` quando omitida.
- `tool_catalog` implementa `search`, `load`, `unload` e `list_loaded` sem criar meta-tools paralelas.
- `search` nao revela tools `disabled` para o perfil.
- `load` nao carrega tools fora da allowlist/estado do perfil e retorna motivos estruturados de rejeicao.
- Tools carregadas sob demanda persistem por conversa/sessao ate restart, TTL, `unload`, mudanca de `schema_hash`, indisponibilidade, mudanca de perfil ou budget.
- `tool_catalog` e `load_skill` sao tratados como bootstrap/control-plane conforme a politica de tool calling e AEP-0072.
- MCP bridge/native, tools destrutivas e budgets de schema respeitam a mesma politica central.
- Nenhum fluxo alternativo de envio de mensagens e criado.
