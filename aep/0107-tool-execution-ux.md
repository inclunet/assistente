# AEP-0107 — Experiência de execução de tools: estado, contexto e resultados acionáveis

**Status:** Done

## Resumo

Define uma camada de apresentação para invocações de tools no chat. Cada invocação comunica, de forma curta e localizada, se ainda está em execução, se concluiu, falhou ou foi cancelada. A interface preserva a auditabilidade por meio de detalhes técnicos sob demanda, sem transformar o histórico conversacional em uma lista de nomes internos de tools e payloads.

Tools nativas recebem descrições específicas porque o host conhece seus argumentos e semântica. Tools MCP permanecem plug and play: por padrão, a UI mostra apenas uma ação genérica associada ao nome público do provedor, sem inferir intenção a partir de nomes técnicos, parâmetros ou texto livre.

Resultados estruturados de busca podem ser apresentados em um modal acionável: arquivos abrem no editor e URLs abrem no navegador externo. Nesta proposta não se cria navegador de arquivos, navegador web embutido nem visualizador de diff.

## Motivação

O evento técnico bruto de uma tool não responde às perguntas que importam durante a execução:

- O que está acontecendo agora?
- A saída visível é parcial ou a execução terminou?
- A execução teve sucesso, falhou ou foi cancelada?
- Existe um resultado útil que eu possa abrir?

Quando a pessoa isola uma invocação para ler uma saída longa, esse estado não pode desaparecer. Exibir somente o nome interno da tool tampouco é suficiente para orientar alguém que não conhece o subsistema de tools. Por outro lado, resumir arbitrariamente uma tool MCP pela heurística do host cria afirmações imprecisas e impõe convenções novas a integrações de terceiros.

O AEP-0104 já estabelece `tool_invocations` como ledger canônico e prevê uma projeção leve na timeline com detalhes carregados sob demanda. Esta proposta define como a projeção deve ser compreensível e acionável na superfície de chat.

## Decisões

### D1 — Card de invocação com estado explícito e persistente

Toda invocação exibida na timeline tem um cabeçalho de estado que permanece visível quando a saída é rolada ou a mensagem é isolada.

| Estado do ledger | Apresentação | Regra |
| --- | --- | --- |
| Pendente/em execução | `Em execução` + indicador não textual + duração | Saída recebida nesse estado é marcada como parcial. |
| Sucesso | `Concluída` + resumo factual opcional | Só pode ser exibido após a transição terminal do executor. |
| Falha | `Falhou` + causa curta e segura | A causa completa fica nos detalhes técnicos. |
| Cancelada | `Cancelada` | Não é apresentada como falha nem como sucesso. |

Texto, ícone e semântica acessível comunicam o estado; cor isolada não é suficiente. A chegada de chunks de output nunca promove uma invocação a sucesso. Uma execução em andamento com saída deve indicar explicitamente `saída parcial`.

O estado vem da projeção canônica da invocação, não de interpretação do texto da tool. O card continua recebendo atualizações enquanto estiver isolado.

### D2 — Microcopy localizada por adaptador para tools nativas

O host mantém adaptadores de apresentação para as tools nativas que possuem argumentos tipados. Os adaptadores devolvem uma intenção de exibição e alvos tipados; os textos são internacionalizados pelo host em `pt-BR`, `en` e `es`.

| Operação | Estado em andamento |
| --- | --- |
| Ler arquivo | `Lendo {arquivo}…` |
| Editar/criar arquivo | `Editando {arquivo}…` / `Criando {arquivo}…` |
| Buscar arquivos | `Buscando arquivos em {escopo}…` |
| Acessar URL | `Acessando {domínio}…` |
| Busca web | `Buscando na web…` |
| Executar comando | `Executando comando…` |

O rótulo de arquivo mostra somente o nome base. Se houver ambiguidade no mesmo card, ele incorpora o menor prefixo relativo necessário, por exemplo `api/config.ts`. Paths absolutos não aparecem no texto primário. URLs mostram o domínio, não a URL completa. Termos de busca e argumentos potencialmente sensíveis não são copiados automaticamente para a microcopy.

Os adaptadores não fazem parte do contrato MCP e não tentam derivar linguagem natural de uma tool arbitrária.

### D3 — MCP plug and play com fallback semântico mínimo

Para tools expostas por MCP, a apresentação padrão é:

> `Consultando {nome público do provedor}…`

O verbo é localizado pelo host; o nome do provedor é dado de exibição e não é traduzido. A UI não deduz que um provedor é CRM, busca, armazenamento ou outro domínio a partir de `name`, `description`, schema ou argumentos. Quando não houver nome público confiável, o fallback é `Consultando uma integração conectada…`.

Anotações MCP existentes de somente leitura, ação destrutiva, idempotência e acesso externo continuam sendo fonte de sinalização de risco e aprovação. O rótulo amigável não pode reduzir, omitir ou contradizer um aviso de segurança.

Um MCP pode receber uma apresentação mais específica no futuro somente por um contrato explícito e versionado, não por parsing de texto livre. Isso está fora do escopo deste AEP.

### D4 — Links apenas para destinos já suportados

O adaptador pode fornecer um `displayTarget` tipado, resolvido pela UI sem construir URLs internas no conteúdo textual:

```ts
type DisplayTarget =
  | { kind: 'file'; path: string; line?: number }
  | { kind: 'url'; url: string };
```

Para `file`, o clique no rótulo abre o arquivo no editor pelo contrato de deep link existente. Para `url`, o clique abre o navegador externo pelo mecanismo seguro já adotado pela aplicação. Paths precisam pertencer ao escopo permitido da invocação e URLs precisam passar pela política normal de abertura externa.

Se não existir um destino resolvível, o rótulo não é link. A UI não deve criar links decorativos ou que possam falhar silenciosamente.

### D5 — Busca estruturada abre modal de resultados sob demanda

Uma tool de busca compatível pode devolver, além de sua saída técnica, uma projeção estruturada validável:

```ts
type SearchResultItem = {
  kind: 'file' | 'url' | 'text';
  title: string;
  snippet?: string;
  target?: DisplayTarget;
};

type SearchResultPresentation = {
  total?: number;
  items: SearchResultItem[];
};
```

Depois de concluir, o resumo `12 resultados encontrados` torna-se a ação que abre um modal de resultados; ele não abre automaticamente. Itens `file` abrem no editor, itens `url` no navegador externo e itens `text` são somente leitura. O modal mostra título, caminho relativo ou domínio, trecho e ação disponível.

A primeira fase adapta apenas tools nativas com resultados tipados. Uma saída MCP só é elegível quando for decodificada e validada contra esse contrato; a UI nunca extrai resultados de JSON ou Markdown arbitrários por heurística. Itens grandes, paginação e conteúdo técnico continuam sujeitos às regras de tamanho, proveniência e carregamento lazy do AEP-0102 e do AEP-0104.

### D6 — Detalhes técnicos permanecem disponíveis, mas não são o estado primário

Todo card oferece `Ver detalhes técnicos` no menu de contexto e em um controle equivalente acessível por teclado. O modal de detalhes inclui, conforme autorização e disponibilidade:

- provedor, tool e identificador da invocação;
- estado terminal, timestamps e duração;
- parâmetros sanitizados;
- sinais de política e permissões aplicadas;
- saída, erro e metadados técnicos; e
- paginação ou proveniência do resultado, quando houver.

O menu de contexto não pode ser a única via para a informação: há botão ou ação de teclado equivalente, foco previsível, Escape para fechar e retorno do foco ao originador. A saída extensa é carregada sob demanda, conforme AEP-0104.

### D7 — Separação entre status, resultado e auditoria

A timeline não usa output técnico como texto de status. O contrato visual é:

1. **Status:** o que está acontecendo e se a invocação chegou a estado terminal;
2. **Resultado:** resumo factual e, quando estruturado, ação para abrir itens;
3. **Auditoria:** entrada, saída e metadados completos sob demanda.

Essa separação evita que output parcial pareça sucesso e mantém o fluxo de conversa legível em execuções com muitos dados.

## Fases

### Fase 1 — Estado canônico na timeline

- [x] Mapear a máquina de estados do ledger para cabeçalho persistente do card.
- [x] Indicar saída parcial durante execução e manter atualizações em modo isolado.
- [x] Implementar estados de sucesso, falha e cancelamento com texto acessível.
- [x] Cobrir transições de estado, inclusive output antes da conclusão.

### Fase 2 — Apresentação amigável e detalhes

- [x] Criar adaptadores para as tools nativas prioritárias.
- [x] Internacionalizar microcopy e aplicar redução de paths e URLs no texto primário.
- [x] Definir e aplicar sanitização adicional para argumentos sensíveis antes de exibi-los nos detalhes.
- [x] Criar modal de detalhes técnicos lazy, com menu de contexto e equivalente por teclado.
- [x] Adotar fallback MCP por provedor.
- [x] Integrar sinais explícitos de segurança e aprovação à apresentação resumida.

### Fase 3 — Destinos e resultados estruturados

- [x] Permitir abertura segura de arquivos no editor e URLs no navegador externo.
- [x] Adaptar buscas nativas ao contrato `SearchResultPresentation`.
- [x] Implementar modal de resultados, limites e paginação.
- [x] Avaliar decodificadores explícitos para outputs MCP compatíveis, sem heurística sobre texto livre. Decisão: não introduzir decodificadores nesta fase; MCP permanece plug and play pelo provedor. Um contrato futuro deverá ser explicitamente opt-in e versionado pelo servidor.

## Não objetivos

- Criar um navegador de arquivos próprio.
- Criar navegador web embutido.
- Criar revisão de diff ou tentar simular um diff no card de tool.
- Alterar o protocolo MCP para exigir intenção, locale ou rótulo de UI.
- Gerar ou inferir rótulos semânticos para MCPs a partir de modelos, nomes ou argumentos.
- Remover a saída técnica, o ledger ou as políticas de aprovação existentes.

## Implementação

A implementação iniciou no frontend em `frontend/src/components/chat/ToolCallsSection.tsx` e `frontend/src/lib/toolPresentation.ts`. O adaptador é deliberadamente allowlist para tools nativas; MCP usa exclusivamente o rótulo público do provedor. O card mantém estado textual, identifica saída parcial e abre detalhes técnicos sob demanda. Arquivos nativos e URLs HTTP(S) viram alvos acionáveis pelo editor e navegador externo já existentes.

Os resultados de busca nativos usam um contrato estruturado versionado,
carregado sob demanda e paginado no modal. A avaliação de decodificadores MCP
foi concluída: eles não serão introduzidos nesta fase, conforme a Fase 3.

### Revisão de integração — PR #809

A revisão da série identificou sete correções necessárias antes do encerramento:

- [x] Propagar os indicadores de busca e segurança do ledger até o histórico e o patch do turno.
- [x] Sanitizar também os argumentos transitórios exibidos sem detalhes persistidos.
- [x] Reconhecer o estado transitório `done` como sucesso.
- [x] Atualizar estado e saída do modal aberto durante a execução.
- [x] Tornar explícito qualquer limite adicional aplicado aos resultados de busca.
- [x] Agregar decisões de segurança, com precedência de bloqueio sobre aprovação.
- [x] Preservar decisões já coletadas nos caminhos de cancelamento e timeout.

Evidências verificáveis:

- `internal/app/db_message_window_test.go` e `internal/agent/service_stats_test.go`
  exercitam o ledger real e os caminhos de histórico e patch; os bindings foram
  regenerados a partir dos tipos Go.
- `ToolCallsSection.test.tsx` cobre argumentos transitórios sanitizados, estados,
  foco, resposta atrasada, paginação e acessibilidade. O parser mantém um teto
  defensivo de 100 itens, explicitando quantos foram materializados e orientando
  restringir a busca; cada página contém até 20 itens.
- `ChatMessage.toolDialogs.test.tsx` cobre o modal aberto durante a transição
  ativo → segmento → patch terminal canônico e o isolamento na troca de usuário.
  O host `ToolInvocationDialogsProvider` permanece no nível da mensagem, sem
  mover os cards da sua posição cronológica.
- `internal/toolinvocations/projection_read_test.go` cobre bloqueio com
  precedência, formatos inválidos e versão estritamente inteira igual a 1.
- `internal/tools/executor_test.go` cobre preservação dos sinais em sucesso,
  panic, cancelamento e timeout, inclusive quando o worker ainda não retornou.

A validação local inclui a suíte completa Go e Vitest, build, vet, lints,
TypeScript e detector de corrida nos pacotes afetados. A revisão independente
local não tem pendências acionáveis. Não houve validação manual com NVDA nesta
rodada; a cobertura automatizada de acessibilidade não a substitui.

## Riscos

- **Promessa incorreta de sucesso:** mitigada ao derivar estado exclusivamente do lifecycle canônico e marcar output não terminal como parcial.
- **Exposição de dados sensíveis:** mitigada por adaptadores allowlist, sanitização e detalhes sujeitos a carregamento/autorização existentes.
- **Links inseguros ou quebrados:** mitigada por alvos tipados e pelos validadores de path/URL já existentes; ausência de alvo significa ausência de link.
- **Inconsistência entre integrações:** mitigada por fallback MCP único em vez de exigir que servidores de terceiros traduzam ou classifiquem suas tools.
- **Modal excessivamente pesado:** mitigado por limites do AEP-0102, paginação e carregamento lazy do AEP-0104.
- **Regressão de acessibilidade:** mitigada por estado textual, controle de teclado equivalente ao contexto, foco restaurado e anúncios adequados.

## Critérios de aceitação

1. Uma pessoa consegue distinguir, sem abrir detalhes, entre invocação em execução, concluída, falha e cancelada.
2. Output recebido antes da conclusão terminal é identificado como parcial.
3. O estado permanece perceptível ao isolar a invocação e ao rolar uma saída longa.
4. Tools nativas prioritárias mostram microcopy localizada sem paths absolutos ou URLs completas no texto primário.
5. Toda tool MCP funciona sem mudança no servidor e recebe fallback por provedor; nenhuma heurística classifica sua intenção.
6. Arquivos e URLs só são clicáveis quando possuem destino tipado e validado.
7. Uma busca nativa com resultados estruturados abre modal sob demanda e cada item navegável segue para o editor ou navegador externo correto.
8. Detalhes técnicos são alcançáveis por mouse e teclado, preservam foco e não bloqueiam o acesso aos dados necessários para auditoria.
9. Estados, modais e rótulos passam por cobertura de acessibilidade e pelos idiomas suportados.

## Relações

- **AEP-0104 — Tool invocations como ledger canônico:** fonte de estado, projeção leve e detalhes lazy da invocação.
- **AEP-0102 — Resultados grandes de tools:** limites, proveniência e paginação de output e resultados estruturados.
- **AEP-0053 — Degradação graciosa de MCP nativo no chat:** falhas MCP continuam sendo classificadas pelo runtime; esta proposta apenas as apresenta.
- **AEP-0023 — Deep Links Internos:** contrato existente para abrir arquivo no editor; a UI resolve alvos tipados sem expor URI interna no texto.
- **AEP-0091 — Diálogos de decisão unificados:** confirmações continuam sob o contrato de decisão; o card de status não substitui aprovações.
