# AEP-0101 — Adequação entre pedido e perfil

**Status:** ✅ Concluído

## Resumo

Adicionar um preflight backend-driven ao primeiro turno de conversas interativas.
Antes de persistir a mensagem do usuário ou montar o payload do turno, o app
classifica quais tools são indispensáveis para executar o pedido e compara esse
conjunto com a política efetiva do perfil atual e, quando outro perfil oferece
cobertura inicial estritamente melhor, pede confirmação para trocar o perfil da aba.

A classificação e a recomendação não conhecem slugs de profiles, endpoints ou
provedores específicos. O classificador recebe o catálogo de tools disponível e
devolve nomes desse catálogo; o matcher deriva candidatos dos profiles instalados
e de `ToolSelectionPolicy.ResolveEffectiveToolPolicy`. A troca só ocorre após uma
ação afirmativa no `DecisionDialog`.

Esta AEP implementa a fase 4 da AEP-0096.

## Motivação

Os baselines da AEP-0096 tornam os profiles previsíveis, mas ainda dependem de a
pessoa escolher o profile adequado antes do primeiro pedido. Um pedido de
programação enviado pelo profile Padrão, por exemplo, começa sem edição, shell e
plano no payload inicial. O modelo pode gastar iterações descobrindo essas tools,
concluir incorretamente que não tem acesso a elas ou tentar compensar o profile
inadequado.

Carregar tools automaticamente não é uma solução aceitável: `on_demand` não
autoriza o harness a ampliar a superfície do turno sem uma decisão explícita, e
uma tool `disabled` é uma negação forte. Também não é aceitável codificar pares
como “pedido de código → slug `programacao`”, pois profiles podem ser
customizados, removidos ou substituídos.

## Decisões

### D1 — Gate único no pipeline canônico

O preflight ocorre no `SendMessageUseCase`, depois de `PrepareContext` resolver o
profile e antes de `RecordUserMessage`.

Assim:

- não existe um segundo fluxo de envio;
- a mensagem ainda não foi persistida quando a decisão abre;
- o profile e a superfície efetivos já são conhecidos;
- o payload efetivo do turno ainda não foi montado;
- confirmar a troca permite refazer `PrepareContext` e seguir o mesmo pipeline.

O gate só roda para mensagem nova no primeiro turno de uma conversa desktop.
Retry, canais, jobs, subagentes e conversas que já tenham mensagens não abrem
uma decisão de troca. Superfícies sem interlocutor continuam com o profile
recebido e sem elevação.

### D2 — Classificação por tools do catálogo

Um classificador auxiliar, sem tools e com timeout curto, recebe:

- o texto do pedido;
- metadados estruturados da superfície;
- os nomes, descrições truncadas, classes, pacotes e riscos das tools que
  aparecem no plano inicial do profile atual ou de ao menos um candidato.

Sua única saída aceita é JSON com:

- `required_tools`: nomes exatos do catálogo indispensáveis para realizar o
  pedido no primeiro turno;
- `confidence`: número entre 0 e 1.

Nomes desconhecidos, respostas inválidas, baixa confiança, timeout e provider
sem papel auxiliar resultam em “sem recomendação”. O pedido segue no profile
atual. A resposta do classificador nunca executa tools, altera policies nem
escolhe diretamente um profile.

O transporte é a interface genérica `llm.ChatProvider.SimpleChat`. Não há
detecção por URL, endpoint, nome de provider ou família de modelo. Providers
que recusam papéis auxiliares, como ACP, simplesmente não executam o preflight.
O classificador usa um bucket de rate limit auxiliar, com o mesmo teto
configurado, mas separado do bucket do turno principal; assim nunca consome o
último token reservado ao envio que motivou o preflight.

### D3 — Matching derivado da política efetiva

Para cada profile instalado, o backend resolve a política com a mesma
`ToolSelectionPolicy` usada pelo turno.

O profile atual é considerado adequado quando todas as tools requeridas estão
`preloaded`. Uma tool apenas `on_demand` continua permitida, mas não satisfaz a
necessidade de disponibilidade inicial que motivou esta AEP.

Um profile candidato precisa:

- ser diferente do atual;
- não desabilitar nenhuma tool requerida;
- oferecer mais tools requeridas como `preloaded` que o profile atual.

Vence a maior cobertura que permanece no plano inicial depois de
`ToolSchemaBudgetBytes` e `PreferredToolPackages`. Empate não gera recomendação,
para evitar uma escolha arbitrária. Nenhum slug, nome de profile ou matriz fixa
de capabilities participa do algoritmo.

### D4 — Troca sempre confirmada

Quando houver candidato inequívoco, o backend usa o contrato `kind=decision` da
AEP-0091, com ações nesta ordem:

1. trocar para o profile recomendado;
2. continuar com o profile atual;
3. cancelar o envio via ESC.

Confirmar:

- persiste `profile_override.slug` apenas na aba de origem;
- valida novamente que `surfaceTabId` pertence à conversa antes de persistir;
- invalida tools carregadas da conversa;
- refaz a resolução do profile para o mesmo turno;
- atualiza o workspace no frontend por evento.

Continuar preserva integralmente o profile e o conjunto de tools atual. Não chama
`tool_catalog`, não altera `tool_policy` e não carrega tools sob demanda.
Cancelar encerra o envio antes da persistência da mensagem.
O `questionnaire.Manager` serializa decisões backend para que conversas
concorrentes não sobrescrevam o único diálogo visível.

### D5 — Falhas são conservadoras

Falha de classificação ou ausência de candidato não bloqueia o pedido, porque
nenhuma capacidade foi ampliada. Já uma falha ao persistir uma troca confirmada
interrompe o envio: seguir com um profile diferente do que o diálogo prometeu
seria enganoso.

O classificador é recomendatório e sujeito a prompt injection. A validação
estrita limita sua autoridade a selecionar nomes conhecidos de tools para uma
pergunta ao usuário. Ele não recebe credenciais, não executa chamadas e não
produz policy.

## Fases

1. **Contrato e classificador**
   - implementar resposta JSON estrita e timeout;
   - listar tools registradas sem expor argumentos ou segredos;
   - cobrir provider auxiliar indisponível e saída inválida.
2. **Matcher de profiles**
   - listar profiles instalados;
   - resolver policies pelo contrato central;
   - recomendar somente melhoria inequívoca de cobertura.
3. **Gate e decisão**
   - inserir o preflight antes de persistir a mensagem;
   - usar `DecisionDialog`;
   - persistir o override da aba somente após confirmação.
4. **Sincronização e fechamento**
   - atualizar o workspace no frontend;
   - garantir que recusa não carrega tools;
   - validar backend e frontend, inclusive acessibilidade.

## Riscos

- **Latência no primeiro turno:** limitar o classificador por timeout e executá-lo
  somente em conversa desktop vazia com mais de um profile elegível.
- **Falso positivo:** exigir confiança mínima, melhoria estrita e candidato
  inequívoco; a pessoa pode continuar sem trocar.
- **Falso negativo:** falhar conservadoramente mantém o comportamento anterior e
  nunca eleva capacidade.
- **Prompt injection:** validar apenas nomes conhecidos e ignorar todo conteúdo
  extra da resposta.
- **Profile customizado:** derivar matching da policy instalada, sem slugs fixos.
- **Divergência de UI:** a persistência é backend-driven e um evento com o
  workspace atualizado sincroniza o store.
- **Concorrência:** o preflight participa da serialização por conversa, a fila de
  questionários mostra uma decisão backend por vez e a troca revalida o vínculo
  aba↔conversa antes de persistir.

## Critérios de aceitação

- [x] O preflight roda antes da primeira mensagem ser persistida.
- [x] Retry, canal, job, subagente e conversa não vazia não abrem troca de profile.
- [x] Classificação usa a interface genérica do provider e catálogo dinâmico.
- [x] Nenhum endpoint, provider, modelo ou slug de profile é hardcoded.
- [x] Matching usa `ToolSelectionPolicy.ResolveEffectiveToolPolicy` e o plano orçado.
- [x] Empate ou baixa confiança não escolhe profile arbitrariamente.
- [x] Troca só ocorre após ação afirmativa no `DecisionDialog`.
- [x] Continuar não carrega tools nem altera policy.
- [x] Cancelar não persiste a mensagem.
- [x] A troca atualiza `profile_override` da aba e o estado visível do workspace.
- [x] Testes cobrem classificação, matching, confirmação, recusa, cancelamento,
      falhas e ausência de interlocutor.
- [x] Strings visíveis existem em pt-BR, inglês e espanhol.
