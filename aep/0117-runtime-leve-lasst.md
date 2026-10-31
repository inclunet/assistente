# AEP-0117 — Runtime leve lasst

**Status:** In Progress — PoC inicial; extração de núcleo compartilhado e review pendentes

## Resumo

Investigar um executável `lasst` efêmero, independente da inicialização do aplicativo e de banco, para executar tarefas com um provedor, skills em arquivos e ferramentas. O primeiro objetivo é avaliar a reutilização do código existente antes de integrar reviews de PR.

## Motivação

O `asst` inicia `internal/app` e usa conversas persistentes. Em CI e containers, uma execução autocontida deve ler instruções e configuração externas e manter o estado do turno em memória.

## Decisões

- O experimento não altera o contrato de mensagens do Assistente (AEP-0040) nem desabilita o ledger de chat (AEP-0104).
- Reutilizar tipos e implementações existentes de LLM, parsing de skills e ferramentas quando suas dependências permitirem. Não inicializar o gerenciador de credenciais persistentes, contas ou banco.
- O ciclo do `internal/agent` atual requer ledger quando executa ferramentas. Sua reutilização sem banco depende de extração de um núcleo com fronteiras explícitas; não contornar essa guarda no serviço existente.
- A prova inicial precisa comprovar uma chamada ao provedor, carregamento de uma skill e leitura local durante um ciclo com tool calling. MCP e publicação de reviews pertencem a uma etapa posterior.
- O uso de arquivos para MCP pelo novo executável ainda precisa ser definido explicitamente em relação ao AEP-0049, que estabelece banco como fonte de runtime do Assistente. Nenhuma alteração de contrato MCP é implementada nesta fase.
- Um workflow futuro pode ler skills e configuração do checkout do PR como dados não confiáveis. Binário, credenciais, autorização de MCP e limites de capacidade permanecem sob controle confiável; o workflow deve registrar o SHA analisado. Restrições de ferramentas reduzem o impacto, mas não eliminam prompt injection.

## Fases

- [x] Investigar a composição do `asst`, dos providers, skills, executor e ciclo de agente.
- [x] Demonstrar execução efêmera com provider OpenAI-compatible, skill e ferramenta de leitura: `cmd/lasst`, testes `TestCompleteRunWithHTTPProvider` e `TestRunUsesSkillAndReadFile`. O teste usa servidor local simulado; validação com serviço real permanece pendente.
- [ ] Avaliar MCP, publicação inline e sumário em PR de teste.
- [ ] Decidir se a solução justifica implementação e integração no workflow `/review`.

## Riscos

- O provider OpenAI-compatible recebia `credentials.Manager` concreto; a PoC adiciona uma factory com chave em memória e mesma política de redirecionamento. A validação com provedores reais continua pendente.
- O ciclo de agente atual depende de conversas e ledger. Extrair o núcleo pode exigir mudanças maiores do que o desejado; um loop separado pode duplicar invariantes importantes, incluindo limite de saída e cancelamento.
- Leitura de arquivos e instruções em PRs exigem políticas de confiança explícitas. A publicação deve ser vinculada ao commit analisado.

## Critérios de aceitação

- [x] O teste integrado `TestCompleteRunWithHTTPProvider` executa o ciclo sem inicializar `internal/app` ou banco, usando servidor HTTP local simulado.
- [x] Testes em `cmd/lasst` cobrem skill, tool calling, leitura, limite, cancelamento, escopo de acesso, redirecionamentos e autenticação efêmera.
- [ ] A decisão de prosseguir considera o diff, os testes e os acoplamentos remanescentes.

## Resultado da PoC inicial

A implementação reutiliza o provider OpenAI-compatible, o parser de skills, o registry, o executor e `read_file`. O serviço `internal/agent` não é reutilizado: exige ledger de invocações e persistência de chat. A PoC contém um loop efêmero reduzido; ainda não demonstra paridade com o ciclo do Assistente. Uma nova factory do provider permite chave em memória e preserva a política de redirecionamento de mesma origem. A CLI expõe apenas leitura no workspace, nega `.git/` e recusa skills com políticas ou recursos ainda não suportados.

Ainda faltam integração MCP, operações de review, avaliação de modelos reais, políticas completas de skills, distribuição e integração ao workflow. A continuidade e abertura de PR dependem da avaliação do experimento.

## Validação local da PoC

- `go build ./...` e `go vet ./...` passaram. `go test ./cmd/lasst`, `./internal/skills` e `./internal/tools/filesystem` passaram; `golangci-lint run ./cmd/lasst ./internal/llm` não apontou ocorrências.
- TypeScript (`tsc --noEmit`), ESLint e Stylelint passaram; Stylelint apresentou avisos preexistentes. Vitest passou em 493 arquivos e 6.062 testes, com arquivos identificados como relacionados a ACP excluídos.
- A suíte completa de Go não foi validada: uma tentativa anterior falhou ao iniciar binários de `internal/acp` e `internal/acpregistry`. Por orientação do mantenedor, testes ACP não devem ser executados neste ambiente.
- A review independente foi repetida após a correção de lint e não encontrou pendências acionáveis no escopo da PoC. Não houve teste com provedor remoto real.

O modo opcional `LASST_REASONING_CONTENT_MODE=replay_with_tools` preserva o reasoning transitório nas continuações com ferramentas, segundo o AEP-0097; por padrão permanece desabilitado.


## Reavaliação arquitetural — 31/10/2026

### Objetivo e situação

O `lasst` deve ser um irmão do desktop e do `asst`: os três compõem um único núcleo de agente, ferramentas, skills, provedores e MCP. O desktop e o `asst` usam o control-plane persistente; o `lasst` compõe adapters de arquivos, ambiente e estado efêmero sem inicializar `internal/app` ou SQLite. Não criar outro produto interno paralelo com protocolo, executor ou cliente MCP próprios. O objetivo é compartilhar comportamento efetivo, não apenas tipos ou bibliotecas.

O PR filho #969 permanece em rascunho após o rebase do pai e requer atualização antes de qualquer integração. A PoC deste PR é evidência de viabilidade, não a arquitetura final. `cmd/lasst.run` é um segundo loop e deve ser eliminado após a migração; sua factory específica de provider precisa ser reavaliada como fronteira compartilhada de autenticação. O PR empilhado #969 é experimental: sua descoberta de skills e `github_mcp.go` específicos não são o contrato final. Ele não deve ser integrado como solução completa de review; ajustar ou substituir seus commits conforme as fases abaixo. Nenhuma fase nova está concluída por esta análise.

### Inventário verificado na main `a2db8a59b`

- **Agente e ciclo:** `internal/agent/agentic_loop.go` já concentra streaming, retry, fallback MCP nativo para bridge, bloqueio/reformulação de tool calls truncadas, ordem e escopo de `load_skill`, execução e retry autorizados, reasoning, orçamento/contexto e encerramento. A PoC não tem paridade com todos esses caminhos. `agent.NewService` exige `TurnStore` e leitor de timeline; quando há executor de tools, exige também `toolinvocations.Service`. O loop está integrado a persistência, eventos e projeção.
- **Evolução recente da timeline:** AEP-0116 e `internal/agent/turn_execution.go` introduzem lifecycle de tentativa, lease, ownership, recuperação e CAS terminal. `internal/chat/live_projection.go` separa `LiveProjection`, `ExecutionProjection` e `StreamProjection`, mas sua admissão ainda é escopada por usuário/conversa/turno/execução e depende de `database.RequireUserID`. Ela é uma fronteira útil para separar apresentação e execução, não um motor genérico pronto nem motivo para reutilizar identidade fictícia. A recuperação coordenada adicionada por #973 em `internal/chat/timeline_recovery.go` também distingue projeção, confirmação persistida e versão terminal; deve permanecer responsabilidade do adapter de chat. O #974 desloca também a derivação do patch terminal para o representante explícito em `internal/chat/response_projection.go`; essa autoridade de apresentação deve permanecer no adapter de chat. A fase 2 da AEP-0116 continua em andamento.
- **MCP:** `internal/mcp.Manager` já gerencia sessão, discovery, transportes stdio/SSE/HTTP, reconexão, cancelamento, roots e encerramento; `internal/mcp/bridge.go` e `tools.Registry` já adaptam e registram ferramentas. O manager também depende de repository/catálogo, identidade, `credentials.Manager`, OAuth e política de rede. Uma implementação JSON do repository sozinha não remove esses acoplamentos.
- **Skills e tools:** `internal/skills.Manager`, `skills.Parse`, `skills.Invoke`, `ContextProvider` e `load_skill` já tratam descoberta, ativação, arquivos complementares e políticas. `internal/tools/filesystem` já fornece `read_file`, `list_directory`, `search_files` e `grep_search`; o app os registra em `internal/app/app_tool_registry.go`. A composição compartilhada experimental está nos commits locais do #969, ainda não entregue por este PR. `run_command`, `commandpolicy` e allowlist existem, mas dependem de terminal/PTY e confirmação. Não foi localizada uma ferramenta Git diff dedicada.
- **Resultados grandes:** `internal/tools/large_result_store.go`, `ContentForModel` e `read_tool_result` já implementam armazenamento e continuação, porém o owner atual exige identidade de usuário e incorpora o ID da conversa quando presente. O limite da PoC não deve ser resolvido duplicando esse mecanismo nem fabricando uma conversa.
- **Provedores:** `internal/llm` já fornece providers e `Streamer`; o construtor OpenAI-compatible histórico usa `*credentials.Manager`. A factory efêmera da PoC comprova uma alternativa, mas a interface de autenticação/transporte deve ser examinada para compartilhar os demais providers sem acoplar o núcleo a banco ou criar fábricas divergentes.

### Contratos-alvo

1. **Motor de iteração único.** Extrair incrementalmente o protocolo agêntico existente para um motor com portas explícitas de streaming, execução de tools, política/contexto e observação de lifecycle. Adaptar o serviço de chat ao motor mantendo `TurnStore`, lease, `OwnsStream`, CAS/terminal, projeção confirmada, ledger obrigatório antes de efeitos e eventos canônicos. O `lasst` usa o mesmo serviço de invocações com ledger em memória, efêmero e escopado por execução, conforme decisão abaixo. Nunca injetar IDs fictícios nem usar um no-op para contornar as guardas de chat/jobs dos AEPs 0040, 0063 e 0104. A extração só termina quando ambos os consumidores executarem o mesmo motor e o loop duplicado for removido. A PoC aplica denylist por componente `.git`, também aninhado e em caminhos resolvidos; a validação e a abertura de arquivos nas tools existentes ainda não são atômicas contra troca concorrente de symlink.
2. **MCP genérico compartilhado.** Extrair conexão, ciclo de vida, transporte, discovery, bridge e encerramento do control-plane persistente. O Assistente preserva repositório DB-only, user-scope, catálogo, OAuth e ownership dos AEPs 0049, 0105 e 0112; o `lasst` fornece configuração em arquivo, segredos por fontes explícitas e estado em memória. Reutilizar `ServerConfig` e validar seus defaults antes de aplicá-los em headless. GitHub, GitLab e outros são configurações/servidores, não branches hardcoded no motor. Nenhuma identidade falsa ou relaxamento global de `RequireUserID`.
3. **Capacidades são teto imposto pelo operador.** Skills/configurações do checkout podem orientar a tarefa e solicitar recursos, mas não autorizam comandos, subprocessos, destinos, credenciais ou tools. Em headless, pedido fora da política confiável falha fechado. MCP stdio precisa autorização explícita de executável, argumentos e ambiente segregado; o atual `buildEnv` herda ambiente e não é isolamento. Filtrar ferramentas MCP não impede efeitos de um servidor arbitrário. Bloqueio efetivo de escrita, processo e rede exige também isolamento do processo/runner e política de egress, não apenas instruções ao modelo.
4. **Investigação local e publicação.** O agente consulta diff e arquivos do checkout local; skills orientam estratégias e formato dos achados. O MCP configurado publica comentários/sumário e pode fornecer contexto remoto pontual. Avaliar `run_command` + `commandpolicy` em headless e, se inadequado, criar uma tool Git compartilhada limitada com argumentos estruturados, refs autorizadas, sem shell, ext-diff ou textconv. Não conectar `run_command` com `confirmFn=nil`: hoje `DecisionConfirm` nesse caso prossegue para execução; o adapter headless precisa negar confirmação indisponível e cobrir sintaxe composta, argumentos, diretório e ambiente. Não fixar GitHub nem o fluxo de review dentro do motor.
5. **Revisão correta e efeitos externos.** O workflow/adapter da plataforma fixa PR e SHA confiáveis e comprova a correspondência com o checkout; ao avançar o PR aplica política explícita de interrupção ou publicação na revisão analisada. Testar criação, comentários inline, sumário, falhas parciais, repetição e concorrência. Servidores que localizam a review pendente pela conta exigem controle verificável de identidade/concorrência: não afirmar associação segura só por um booleano em memória. Uma execução não pode terminar com sucesso afirmando publicação se ela não foi confirmada.
6. **Skills, providers e resultados:** extrair descoberta por raízes e candidatos em `internal/skills`, mantendo precedência exe < home < workdir do desktop; o lasst aplica política explícita de colisão e leitura confinada com `os.OpenRoot`. Compartilhar invocação, supporting files e políticas, intersectando-as com o teto confiável do runtime. Para resultados grandes, extrair store/owner tipado de execução efêmera, isolado do owner usuário/conversa do desktop; compartilhar limites e `read_tool_result`. Manter falha explícita até haver continuação utilizável.

### Plano incremental e gates

- [x] PoC inicial e inventário/reavaliação sobre a main `a2db8a59b`; evidências nos caminhos acima.
- [ ] **Rede de segurança:** fixtures determinísticas do provider e composição de testes de contrato em ambos os runtimes. Registrar casos atuais de retry, fallback, ordenação, reasoning, limites, cancelamento e ferramentas; incluir testes de segurança de paths e ausência de efeitos antes da autorização.
- [ ] **Ledger efêmero:** adaptar o mesmo `ToolInvocationService` e repository em memória, com origem standalone explícita, transições antes de efeitos e isolamento por execução; atualizar AEPs 0063/0104 no PR de implementação.
- [ ] **Motor único:** extrair em etapas pequenas e substituir os dois loops; comprovar equivalência de mensagens, chamadas, erros e resultados. Preservar testes de ledger fail-closed, tool calls truncadas, retry, projeção/ownership, leases e CAS da timeline. Não mexer nos testes ACP neste ambiente; o CI remoto mantém sua cobertura.
- [ ] **MCP compartilhado:** extrair conexões/transportes com ports e adapters para app e execução efêmera, sem reimplementar lifecycle. Testar SDK em memória e HTTP, dois servidores distintos, stdio autorizado, descoberta, encerramento, cancelamento/reconnect, redirects e isolamento de segredos. Preservar testes de user-scope, OAuth, catálogo e rede do Assistente.
- [ ] **Skills, ferramentas e resultados:** compartilhar descoberta/invocação/políticas e composição de tools locais; preservar semântica de precedência do desktop, colisões explícitas e symlinks do lasst; testar abertura segura dos alvos contra troca concorrente de symlink, pois as tools existentes validam pathname antes da abertura; integrar resultados grandes com owners separados e verificar paginação, JSON/raw e isolamento. Decidir e testar diff local.
- [ ] **Integração de review:** configurar fontes confiáveis, checkout e SHA; demonstrar leitura local mais publicação inline/sumário via MCP em PR de teste, com negativos para ações fora de escopo, concorrência, falhas de rede e repetição. Testar pelo menos dois adapters/servidores sem embutir fornecedor no core. Só depois substituir o workflow `/review`.
- [ ] **Distribuição e aceite:** validar provider e MCP reais, container/CI, segredos, logs e limites operacionais; atualizar documentação, índice/status e comprovar ausência de banco e de componentes duplicados no runtime leve.

### Critérios adicionais de aceite

- [ ] Os três produtos executam o mesmo ciclo e as mesmas implementações de tools/skills/MCP; adapters distintos não contêm cópias dos protocolos ou lifecycle.
- [ ] A continuidade de uma chamada de ferramenta, cancelamento, falha e retry têm cobertura de contrato comum; invariantes de ledger/timeline do desktop continuam protegidas.
- [ ] Review usa checkout local, MCP selecionado e autorização confiável, sem edição/commit por padrão; comentários e sumário correspondem ao SHA efetivamente analisado.
- [ ] Skills/configs do PR não podem autorizar processos, destinos, segredos ou capacidades; isolamento do runner é testado independentemente das instruções.
- [ ] O lasst usa o serviço compartilhado de invocações com ledger em memória, não inicializa banco ou gerenciador persistente, suporta resultados grandes de forma retomável e não depende de um fornecedor fixo.


### Decisão — ledger compartilhado com persistência efêmera

Decisão do mantenedor: **manter o serviço e o contrato de ledger de invocações compartilhados**, com armazenamento em memória e efêmero para o `lasst`. A PoC `cmd/lasst.run` chama `tools.Executor` diretamente; o sucessor `runWithTools` no PR empilhado mantém essa chamada. Ambos são experimentais e devem ser corrigidos para passar pelo caminho compartilhado antes do aceite. Não há exceção geral à AEP-0063 D3.

`toolinvocations.Service` já recebe uma interface de repository, mas ainda exige catálogo, IDs e transições antes do efeito. A implementação precisa criar uma origem de execução standalone explícita e um repository em memória, compartilhando validação, transições, resultados e regras de retry do serviço. Se o serviço atual não permitir isso sem acoplar ao banco, extrair suas fronteiras para um núcleo único utilizado pelos dois adapters; não introduzir um segundo serviço de invocação.

O ledger efêmero pertence à execução e desaparece com o processo. Ele não tem a durabilidade do ledger do desktop, não permite recuperar execução após queda e não resolve sozinho a incerteza de um efeito remoto aceito antes de falha de rede. Definir e testar ownership, catálogo, limites, concorrência, retenção e resultado incerto; nenhuma repetição automática de efeito externo quando o resultado for desconhecido. Não fabricar usuário ou conversa nem enfraquecer o fail-closed, ledger durável e ownership existentes de chat/jobs.

A mudança efetiva será entregue em PR de implementação com testes e atualização das AEPs 0063 e 0104 para formalizar a nova origem e a diferença de durabilidade, preservando as origens atuais. Até então, a PoC não está conforme ao contrato-alvo.

### Achado adicional no PR filho — skill por symlink

O `os.OpenRoot` usado na descoberta experimental do #969 impede que a leitura saia da raiz, mas permite que um symlink aponte para `.git` dentro da própria raiz, inclusive em repositório aninhado. Essa leitura não passa pela política de filesystem do executor. Antes de aproveitar o PR filho, fechar essa lacuna e testar symlinks para `.git` raiz/aninhado, troca concorrente e paths explícitos; não declarar proteção resistente a TOCTOU apenas por usar `OpenRoot` ou comparar caminhos antes de abrir. Avaliar uma abertura por componentes com handles e validação de identidade compartilhável pelo filesystem; definir os limites de renomeação concorrente, hardlinks e mounts e isolar o runner conforme o modelo de ameaça. A solução não deve alterar silenciosamente o contrato de symlinks do desktop.
