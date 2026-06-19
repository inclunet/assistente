# AEP-0074 — Prompt Cache, Custo de LLM e Layout da Request

Status: Proposta
Criado em: 2026-06-16
Atualizado em: 2026-06-18
Depende de: AEP-0075 (Context Providers), AEP-0072 revisada (Skill Loading Runtime)
Relacionado: AEP-0012, AEP-0059, AEP-0051, AEP-0039

## Resumo

O Assistente deve tratar prompt/context caching como uma capacidade arquitetural explícita da request inteira, não apenas do system prompt. Providers modernos reduzem custo e latência quando chamadas consecutivas compartilham um prefixo idêntico; por isso, a request deve ser montada sempre em uma ordem previsível: conteúdo estável primeiro, conteúdo de conversa depois, e conteúdo do turno atual no fim.

Esta AEP define:

- normalização simples das métricas de cache reportadas pelos providers;
- layout cache-friendly sempre ativo para system prompt, ferramentas, resumo, histórico e contexto dinâmico;
- configuração de cache controlada pelo perfil, não inferida magicamente pelo provider;
- uso dos controles de histórico e orçamento que já existem no código;
- uma sequência enxuta de PRs para implementar cache sem refazer o subsistema de contexto.

Ordem arquitetural atual:

1. AEP-0075 separou memória, workspace, tasklists e outros estados dinâmicos em Context Providers.
2. AEP-0072 revisada definiu skills como módulos estáticos de instrução/workflow, com modos `base`, `on_demand` e `disabled`.
3. Esta AEP otimiza cache sobre essa base: skills estáticas no prefixo, contexto dinâmico no sufixo, e controles ativos partindo do perfil.

## Estado atual do código

Esta AEP parte do estado atual do runtime, não de uma arquitetura hipotética.

Já existe:

- `profiles.Chat.ContextWindow`, que define a janela de contexto do modelo;
- `profiles.Chat.MaxContextMessages`, com default efetivo de 50 mensagens;
- `profiles.Chat.MinContextMessages`, com default efetivo de 10 mensagens preservadas após sumarização;
- resumo incremental persistido em `Conversation.Summary` e `SummaryUpToMessageID`;
- `HistoryLoader`, que remove do contexto mensagens já cobertas pelo resumo;
- truncamento por quantidade de mensagens, preservando turns a partir de mensagens `user`;
- `MediaHistoryLoader`, que remove mensagens antigas de `tool` e placeholders de tool calling para economizar tokens;
- trigger de sumarização baseado em orçamento: `context_window - max_tokens - margem de segurança`;
- proteção contra sumarizações concorrentes via `SummarizingInProgress`;
- estatística de uso da janela baseada no último usage reportado pelo provider;
- Context Providers com `ProviderBudgets`, incluindo `tasklist` com budget e truncamento.

Portanto, esta AEP não propõe reimplementar janela deslizante, trigger de resumo, mínimo de mensagens preservadas, nem budget básico de contexto. O foco é organizar e instrumentar o que já existe para favorecer cache.

## Motivação

DeepSeek, OpenAI, Anthropic, Gemini, xAI, Qwen, Mistral, Kimi, LiteLLM e OpenRouter oferecem alguma forma de prompt/context caching. O mecanismo varia, mas a regra prática converge: conteúdo repetido e idêntico no começo da request tende a ser cacheável; conteúdo dinâmico cedo demais reduz cache hits.

Hoje o Assistente já economiza tokens com resumo, janela de mensagens e remoção de tool messages antigas. Isso é correto e deve ser preservado. O problema restante é que a request inteira ainda precisa ficar mais previsível e observável:

- métricas de cache reportadas pelos providers ainda não são normalizadas;
- custo estimado ainda trata todos os prompt tokens como custo cheio;
- o layout do system prompt ainda mistura alguns blocos estáveis e dinâmicos;
- a ordem e o tamanho dos blocos devem ser fáceis de auditar;
- hints e cache control precisam partir do perfil, porque a decisão de política depende do uso pretendido daquele perfil.

## Providers com cache relevante

### Cache automático por prefixo

- DeepSeek: automático; usage com `prompt_cache_hit_tokens` e `prompt_cache_miss_tokens`.
- OpenAI: automático para prompts longos; usage com `prompt_tokens_details.cached_tokens`; algumas APIs aceitam `prompt_cache_key`.
- xAI/Grok: automático; recomenda `x-grok-conv-id` ou `prompt_cache_key`; usage com `cached_tokens`.
- Gemini 2.5+ / Vertex: cache implícito automático em modelos recentes; usage com `cachedContentTokenCount`.
- Qwen/DashScope: cache implícito por padrão; usage com `cached_tokens`.
- Moonshot/Kimi: prefix cache com `prompt_cache_key`; usage com `cached_tokens`.

### Cache explícito ou híbrido

- Anthropic: `cache_control` em blocos; usage com `cache_creation_input_tokens`, `cache_read_input_tokens` e `input_tokens`.
- Gemini/Vertex: `cachedContents` explícito com TTL, além do implícito.
- Qwen/DashScope: `cache_control`, session cache e cache implícito.
- Mistral: `prompt_cache_key`; usage com `prompt_tokens_details.cached_tokens`.

### Gateways

- LiteLLM: suporta/normaliza prompt caching para OpenAI, Anthropic, Gemini/Vertex, Bedrock, DeepSeek, xAI, DashScope/Qwen, OpenRouter e outros. Pode expor campos OpenAI-style ou campos específicos.
- OpenRouter: pode expor `cached_tokens`, `cache_write_tokens` e `cache_discount`, dependendo da rota.

## Decisões

### D1. Layout cache-friendly é arquitetura base

O prompt deve ser montado sempre em ordem cache-friendly, mesmo quando o perfil não habilita hints ou cache control explícito.

Isso não é uma feature toggle. É uma simplificação da arquitetura:

1. conteúdo estável primeiro;
2. contexto de conversa depois;
3. contexto dinâmico e turno atual no fim.

O perfil controla se o runtime envia hints, usa cache control explícito, coleta diagnósticos extras ou sobrescreve budgets. O perfil não deve controlar se a request é montada de forma organizada.

### D2. A unidade cacheável é a request inteira

O cache não depende só do system prompt. A request enviada ao provider inclui system, tools, histórico, resumo, contexto dinâmico e mensagem atual. A ordem alvo é:

```text
prefixo estável:
  - prompt base
  - instruções estáticas do perfil
  - skill base completa, quando houver
  - catálogo compacto de skills on_demand
  - definições estáveis de tools/MCP

prefixo de conversa:
  - resumo acumulado da conversa, quando existir
  - mensagens preservadas após summary_up_to_message_id

sufixo dinâmico:
  - context providers dinâmicos (memory, workspace, tasklist, surface)
  - conteúdo específico do turno
  - mensagem atual do usuário
```

Essa ordem deve respeitar os controles já existentes de `MaxContextMessages`, `MinContextMessages`, `ContextWindow` e sumarização. Esta AEP não substitui esses controles.

### D3. Skills são estáticas por contrato

Skills no runtime novo são Markdown estático e não usam templates. Não existe mais política de cache para templates de skills, nem classificação de skill dinâmica.

Regras:

- a primeira skill habilitada no perfil é `base` e pode entrar completa no prefixo estável;
- skills `on_demand` aparecem no catálogo compacto até serem carregadas;
- skills `disabled` não aparecem nem carregam;
- conteúdo completo de skill `on_demand` carregado por ferramenta entra no fluxo daquele turno, não no prompt inicial cacheável;
- argumentos de invocação explícita de skill pertencem ao turno atual, não ao corpo estável da skill;
- qualquer resquício de template em skill é bug e deve ser removido, não compatibilizado.

### D4. Context Providers são o lugar do dinamismo

Memória, workspace, tasklists, superfície ativa e outros estados do app entram por Context Providers. Eles ficam fora do prefixo estável.

Política:

- `memory` pode ter instruções estáveis curtas, mas records/memórias carregadas são dinâmicas;
- `workspace` e `surface` são dinâmicos;
- `tasklist` é dinâmico e respeita budget;
- providers devem produzir blocos pequenos, ordenados e com budget;
- budgets default são parte do runtime, e overrides podem vir do perfil.

### D5. Histórico atual é preservado

O sistema já controla quantidade de mensagens, budget de contexto e momento de sumarizar. Esta AEP não propõe uma nova estratégia de histórico como pré-requisito.

Decisão:

- manter `HistoryLoader` e `MediaHistoryLoader` como fonte canônica da janela enviada ao modelo;
- manter remoção de mensagens antigas de `tool` e placeholders de tool calling;
- manter `summary_up_to_message_id` como limite entre resumo e mensagens recentes;
- manter trigger de resumo por budget;
- só fazer ajustes pontuais se uma medição real mostrar problema.

### D6. Ordenação determinística é obrigatória

Qualquer lista que entra na request deve ter ordem estável:

- skill base e catálogo de skills;
- tools e MCP tools;
- context blocks;
- tasklists e tasks;
- mapas serializados em JSON que entrem no prompt;
- mensagens conforme ordem persistida pelo banco.

Se a ordem semântica importa, ela deve ser persistida. Se não importa, ordenar por chave estável.

### D7. Métricas de cache devem ser normalizadas

`llm.Usage` deve distinguir:

- prompt total reportado;
- completion tokens;
- cache hit/read tokens;
- cache miss tokens;
- cache write/creation tokens;
- provider shape original quando necessário para auditoria.

Mapeamento:

- OpenAI-style: `cached_tokens` é hit/read; miss = `prompt_tokens - cached_tokens`;
- DeepSeek: `prompt_cache_hit_tokens` e `prompt_cache_miss_tokens` são campos diretos;
- Anthropic: preservar `cache_creation_input_tokens`, `cache_read_input_tokens` e `input_tokens`;
- Gemini: `cachedContentTokenCount` é hit/read;
- Kimi/Qwen/OpenRouter/LiteLLM: best-effort por campos reportados.

O custo estimado deve usar tokens billable por classe quando o provider reportar os dados. Quando não reportar, o cálculo antigo permanece como fallback.

### D8. Controles ativos partem do perfil

Provider informa capacidade técnica; perfil decide política.

Configuração conceitual no perfil:

```json
{
  "chat": {
    "prompt_cache": {
      "mode": "off | observe | auto | explicit",
      "provider_hints": false,
      "explicit_cache_control": false,
      "context_budgets": {
        "memory": 1200,
        "workspace": 500,
        "tasklist": 4000
      }
    }
  }
}
```

Regras:

- `observe`: coleta métricas quando o provider reporta, sem enviar hints;
- `auto`: permite hints seguros quando provider suporta;
- `explicit`: permite cache control explícito em providers suportados;
- provider sem suporte deve ignorar/falhar de forma auditável, sem alterar a resposta;
- chaves de cache não podem conter conteúdo de mensagens, email, nomes de usuário, tickets ou secrets.

## Fases / PRs

### PR 1 — Atualizar esta AEP

Atualizar a AEP-0074 para refletir o estado atual após AEP-0072 e AEP-0075.

Entrega:

- remover premissas antigas sobre templates de skills;
- documentar controles existentes de contexto e resumo;
- definir o plano enxuto abaixo.

### PR 2 — Métricas básicas de cache

Estender `llm.Usage` e os providers para capturar métricas de cache quando reportadas.

Escopo:

- `CacheReadTokens`;
- `CacheWriteTokens`;
- `CacheMissTokens`;
- mapeamento OpenAI-compatible/DeepSeek, Anthropic e Gemini;
- testes com payloads de usage.

Não muda layout de prompt.

### PR 3 — Persistência e stats

Persistir e propagar as métricas novas.

Escopo:

- salvar métricas em mensagem ou estrutura JSON compatível;
- propagar nos eventos finais de resposta;
- atualizar estatísticas de token;
- manter compatibilidade com mensagens antigas.

### PR 4 — Reordenação cache-friendly do system prompt

Reorganizar o system prompt para deixar estável primeiro e dinâmico depois.

Escopo:

- prompt base e instruções estáveis primeiro;
- skill base e catálogo de skills no prefixo;
- resumo depois do prefixo estável;
- Context Providers dinâmicos depois do resumo;
- conteúdo específico do turno no fim;
- testes de regressão do builder.

Não muda a estratégia de histórico.

### PR 5 — Determinismo básico da request

Garantir ordenação estável do que já entra na request.

Escopo:

- skills e catálogo;
- tools e MCP tools;
- context blocks;
- serialização JSON que entra no prompt;
- testes focados em duas chamadas com mesmo estado produzindo prefixo estável.

### PR 6 — Configuração de cache no perfil

Adicionar configuração de cache no perfil.

Escopo:

- `prompt_cache.mode`;
- `prompt_cache.provider_hints`;
- `prompt_cache.explicit_cache_control`;
- `prompt_cache.context_budgets`;
- validação;
- defaults conservadores;
- tipos Wails/frontend quando necessário.

O layout cache-friendly continua sempre ativo.

### PR 7 — Budgets de Context Providers via perfil

Usar os overrides do perfil para preencher `contextprovider.BuildRequest.ProviderBudgets`.

Escopo:

- defaults por provider;
- overrides por perfil;
- aplicar sem substituir `ContextWindow`, `MaxContextMessages` ou `MinContextMessages`.

### PR 8 — Cache hints controlados pelo perfil

Enviar hints apenas quando perfil e provider permitirem.

Escopo:

- capability técnica no provider;
- `prompt_cache_key` ou header equivalente em OpenAI-compatible/LiteLLM/DeepSeek-like;
- chave segura derivada de provider, perfil e conversa, sem dados sensíveis;
- fallback silencioso quando não suportado.

### PR 9 — Cache control explícito

Implementar cache control para providers que exigem marcação explícita.

Escopo:

- Anthropic `cache_control` em blocos estáveis;
- Gemini/Vertex apenas se o caminho estiver claro;
- só quando `prompt_cache.mode=explicit` ou `explicit_cache_control=true`;
- nunca aplicar genericamente em gateways sem capability.

### PR 10 — Custo e UX básicos

Expor os resultados de cache para o usuário.

Escopo:

- tokens cache read/write/miss;
- hit rate;
- custo estimado quando possível;
- fallback claro quando provider não reporta cache;
- avisos simples quando cache está habilitado no perfil mas provider não suporta ou não reporta.

## Riscos

- Provider/gateway pode reportar métricas incompletas ou inconsistentes.
- Cache hints podem ser ignorados silenciosamente por gateways.
- `cache_control` explícito pode quebrar chamadas se aplicado em provider incompatível.
- Reordenar o system prompt pode alterar ligeiramente a prioridade percebida pelo modelo.
- Prefixo estável grande demais pode aumentar custo em providers sem cache efetivo; por isso layout cache-friendly não deve significar despejar mais conteúdo.

## Critérios de aceitação

- AEP reflete que skills são estáticas e sem templates.
- Métricas de cache são capturadas quando o provider reporta.
- Métricas são persistidas e aparecem nas estatísticas.
- System prompt é montado com conteúdo estável antes de conteúdo dinâmico.
- Controles de contexto existentes continuam funcionando: `ContextWindow`, `MaxContextMessages`, `MinContextMessages`, resumo e `summary_up_to_message_id`.
- Provider hints e cache control dependem do perfil.
- Nenhuma chave de cache contém conteúdo sensível.
- Pelo menos um provider compatível mostra cache read/hit em uso real ou manual.
