# AEP-0096 — Baseline operacional de tools por perfil

**Status:** ✅ Done

## Resumo

Definir um kit operacional previsível para perfis agênticos e materializar, nos
profiles builtin, políticas de tools coerentes com a função de cada perfil.

Esta AEP estende as AEPs 0077 e 0081. O `ToolSelectionPolicy` continua sendo a
fonte única de seleção, e a política tri-state continua usando `disabled`,
`on_demand` e `preloaded`. A novidade é separar:

- **baseline operacional**: tools que constituem a função do perfil e precisam
  estar disponíveis no primeiro turno;
- **extensões**: tools futuras, MCPs e integrações que permanecem descobríveis
  sob demanda;
- **capacidades incompatíveis**: tools desabilitadas por padrão.

## Motivação

Os profiles builtin `Padrão` e `Programação` usam `enabled_tools: null`. Quando
o catálogo existe, isso inicia o turno somente com o control-plane e deixa as
tools de domínio sob demanda. Assim, o perfil de programação precisa descobrir
e carregar leitura, busca, edição e shell antes de executar sua função básica.

Esse fluxo difere do contrato operacional consolidado por agentes de código:
leitura, glob, grep, edição e shell formam um cinto básico estável; catálogo e
MCP são extensões. O custo não é apenas latência: o modelo pode afirmar que não
tem acesso ao disco, alucinar antes da descoberta ou gastar iterações carregando
capacidades que definem o próprio perfil.

Uma `tool_policy` explícita também tem uma limitação: hoje toda tool não listada
fica `disabled`. Apenas enumerar o baseline fecharia o perfil para novas
builtins e MCPs. É necessário declarar a política das tools não especificadas.

## Decisões

### D1 — Default explícito para tools não listadas

`ChatConfig` recebe `tool_policy_default`, com valores:

- `disabled`: tool não listada fica bloqueada;
- `on_demand`: tool não listada fica descobrível, mas não nasce no payload.

Campo ausente preserva a semântica atual de uma `tool_policy` explícita:
`disabled`. Profiles legados baseados em `enabled_tools` não mudam.

Tools opt-in continuam `disabled` quando não forem listadas explicitamente,
mesmo com default `on_demand`. Isso impede que uma extensão de superfície como
`text_edit` seja elevada por um default aberto.

Quando existir ao menos uma tool `on_demand`, `tool_catalog` continua sendo
promovida a `preloaded`, conforme a AEP-0081.

### D2 — Baseline do perfil Programação

O perfil `Programação` pré-carrega:

- `read_file`;
- `search_files`;
- `grep_search`;
- `edit_file`;
- `write_file`;
- `run_command`;
- `update_plan`;
- `profile`;
- `subagent`.

Tools não listadas usam `on_demand`, exceto opt-ins. Assim, o primeiro turno já
consegue explorar, editar, validar e delegar código, enquanto operações
destrutivas, automação, web e MCP permanecem progressivas.

O perfil declara também `mcp/*: on_demand`. A regra explícita mantém todas as
tools MCP atuais e futuras fora do payload inicial, mas descobríveis e
carregáveis sem configuração manual por servidor.

### D3 — Baseline do perfil Padrão

O perfil `Padrão` pré-carrega capabilities gerais de baixo risco:

- `read_file`;
- `search_files`;
- `grep_search`;
- `web_search`;
- `web_fetch`;
- `collect_responses`;
- `profile`;
- `subagent`.

Tools não listadas usam `on_demand`, exceto opt-ins. Edição e shell ficam
disponíveis mediante carregamento, sem inflar o payload de toda conversa.

O perfil declara também `mcp/*: on_demand`, com a mesma semântica progressiva:
nenhuma MCP nasce no payload, e novas tools MCP passam a ser descobríveis assim
que entram no registry.

### D4 — Profiles restritos falham fechados

`Editor de Texto` mantém somente `text_edit` e `edit_file` preloaded, com
default `disabled`. O escopo da skill e do contexto de superfície permanece
autoritativo.

`Canais de comunicação` usa default `disabled` e nenhuma tool preloaded. Uma
capability externa só entra após decisão explícita no próprio profile.

### D5 — Baseline é contrato, não alias de nomes

Não serão registrados aliases como `Glob`, `Grep` ou `Bash`. Eles duplicariam
schemas e criariam duas formas de executar a mesma ação. O alinhamento com o
mercado é semântico: disponibilidade inicial, escopo e comportamento
previsíveis.

Testes dos profiles builtin são o contrato executável do baseline e impedem que
uma alteração futura volte silenciosamente para `enabled_tools: null`.

### D6 — Evoluções posteriores ficam separadas

Esta AEP reserva fases próprias para:

- uma tool canônica de patch multi-hunk, sem substituir `edit_file` de imediato;
- uma capability única de plano/progresso sobre o domínio de task lists;
- descoberta de profiles, delegação especializada e troca confirmada pelo
  usuário;
- tratamento explícito de `finish_reason=max_tokens` no loop.

Essas mudanças não entram no baseline inicial porque alteram contratos de
execução e interação além da seleção de tools.

## Fases

1. **Política e profiles builtin**
   - adicionar `tool_policy_default` ao backend e à edição de profiles;
   - aplicar os baselines definidos nas decisões D2–D4;
   - atualizar versões builtin e testes de seleção/UI.
2. **Patch canônico**
   - implementado pela [AEP-0099](0099-patch-canonico-multi-hunk.md);
   - projetar e implementar edição multi-hunk com erros localizados;
   - manter compatibilidade com `edit_file` e `write_file`.
3. **Progresso unificado**
   - implementado pela [AEP-0100](0100-progresso-unificado-por-conversa.md);
   - expor uma única capability de plano/progresso ao modelo;
   - reutilizar o storage e os eventos do Task List Manager.
4. **Adequação pedido↔perfil**
   - implementada pela [AEP-0101](0101-profiles-descobríveis-e-delegacao-autorizada.md);
   - expor descrições de profiles ao agente sem classificador auxiliar;
   - favorecer subagentes para especialização pontual;
   - confirmar toda delegação cross-profile e troca persistente;
   - nunca elevar tools ou privilégios silenciosamente.
5. **Continuação por limite de saída**
   - implementada pela [AEP-0098](0098-limite-de-saida-e-tool-calls-truncadas.md);
   - propagar o stop reason dos providers;
   - distinguir JSON malformado original de tool call truncada;
   - continuar ou orientar fatiamento sem retry cego.

## Riscos

- **Ampliação de capacidade:** `Programação` passa a receber shell e escrita no
  primeiro turno. As allowlists, confirmações e políticas de filesystem
  continuam valendo; preload não significa aprovação automática.
- **Sobrescrita de builtin instalado:** o incremento de `_builtin_version`
  aplica novos factory defaults. Estado de runtime continua preservado pelo
  merge existente.
- **Divergência backend/UI:** ambos precisam calcular o default da mesma forma.
  Testes cobrem tools conhecidas, novas tools e opt-ins.
- **Schemas demais:** os baselines são pequenos; o ToolPlanner e o budget da
  AEP-0077 continuam aplicáveis.
- **MCP aberto demais:** `on_demand` apenas torna a tool descobrível. Não ignora
  disponibilidade, allowlist, risco, confiança de rede nem confirmação. O
  wildcard também não eleva tools opt-in.

## Critérios de aceitação

- [x] `tool_policy_default` é validado e propagado em todos os caminhos de
  seleção.
- [x] Campo ausente mantém compatibilidade com a política explícita atual.
- [x] Default `on_demand` não eleva tools opt-in não listadas.
- [x] `Programação` inicia com leitura, busca, edição e shell.
- [x] `Padrão` inicia com leitura, busca, web e questionário.
- [x] `Padrão` e `Programação` declaram `mcp/*: on_demand`, cobrindo MCPs
  futuras sem colocá-las no payload inicial.
- [x] `Editor de Texto` e `Canais de comunicação` permanecem fail-closed.
- [x] Profiles builtin deixam de depender de `enabled_tools: null`.
- [x] UI representa corretamente o estado efetivo de tools não listadas.
- [x] Testes backend e frontend cobrem os quatro profiles e a nova semântica.
- [x] Fases 2–5 permanecem rastreadas em AEPs/entregas próprias.
