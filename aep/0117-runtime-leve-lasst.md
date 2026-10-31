# AEP-0117 — Runtime leve lasst

**Status:** In Progress — descoberta de skills e MCP experimental; validação de review pendente

## Resumo

Investigar um executável `lasst` efêmero, independente da inicialização do aplicativo e de banco, para executar tarefas com um provedor, skills em arquivos e ferramentas. O primeiro objetivo é avaliar a reutilização do código existente antes de integrar reviews de PR.

## Motivação

O `asst` inicia `internal/app` e usa conversas persistentes. Em CI e containers, uma execução autocontida deve ler instruções e configuração externas e manter o estado do turno em memória.

## Decisões

- O experimento não altera o contrato de mensagens do Assistente (AEP-0040) nem desabilita o ledger de chat (AEP-0104).
- Reutilizar tipos e implementações existentes de LLM, parsing de skills e ferramentas quando suas dependências permitirem. Não inicializar o gerenciador de credenciais persistentes, contas ou banco.
- O ciclo do `internal/agent` atual requer ledger quando executa ferramentas. Sua reutilização sem banco depende de extração de um núcleo com fronteiras explícitas; não contornar essa guarda no serviço existente.
- A prova inicial precisa comprovar uma chamada ao provedor, carregamento de uma skill e leitura local durante um ciclo com tool calling. A etapa seguinte experimenta GitHub MCP com escopo restrito; a publicação real ainda precisa de validação.
- O uso de arquivos para MCP pelo novo executável ainda precisa ser definido explicitamente em relação ao AEP-0049, que estabelece banco como fonte de runtime do Assistente. O lasst usa um conector separado e restrito, sem alterar o contrato MCP do Assistente.
- Um workflow futuro pode ler skills e configuração do checkout do PR como dados não confiáveis; binário, credenciais e política de capacidades permanecem sob controle confiável. O workflow deve registrar o SHA analisado e reconhecer que restrições de ferramentas não eliminam prompt injection.

## Fases

- [x] Investigar a composição do `asst`, dos providers, skills, executor e ciclo de agente.
- [x] Demonstrar execução efêmera com provider OpenAI-compatible, skill e ferramenta de leitura: `cmd/lasst`, testes `TestCompleteRunWithHTTPProvider` e `TestRunUsesSkillAndReadFile`. O teste usa servidor local simulado; validação com serviço real permanece pendente.
- [ ] Validar publicação inline e sumário com GitHub MCP real em PR de teste.
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

Ainda faltam validação de review com MCP real, avaliação de modelos reais, políticas completas de skills, distribuição e integração ao workflow. A continuidade e abertura de PR dependem da avaliação do experimento.

## Validação local da PoC

- `go build ./...` e `go vet ./...` passaram. `go test ./cmd/lasst`, `./internal/skills` e `./internal/tools/filesystem` passaram; `golangci-lint run ./cmd/lasst ./internal/llm` não apontou ocorrências.
- TypeScript (`tsc --noEmit`), ESLint e Stylelint passaram; Stylelint apresentou avisos preexistentes. Vitest passou em 493 arquivos e 6.062 testes, com arquivos identificados como relacionados a ACP excluídos.
- A suíte completa de Go não foi validada: uma tentativa anterior falhou ao iniciar binários de `internal/acp` e `internal/acpregistry`. Por orientação do mantenedor, testes ACP não devem ser executados neste ambiente.
- A review independente foi repetida após a correção de lint e não encontrou pendências acionáveis no escopo da PoC. Não houve teste com provedor remoto real.

O modo opcional `LASST_REASONING_CONTENT_MODE=replay_with_tools` preserva o reasoning transitório nas continuações com ferramentas, segundo o AEP-0097; por padrão permanece desabilitado.

## Descoberta de skills no repositório

O `lasst` aceita um nome simples e procura `SKILL.md` em `.assistente/skills`, `.github/skills`, `.agents/skills`, `.claude/skills` e `.cursor/skills` do diretório de execução. Nomes duplicados são recusados e exigem caminho explícito; links resolvidos fora do workspace não são aceitos na descoberta. Caminhos explícitos continuam disponíveis. A validação do fluxo de review com MCP real continua pendente.

## MCP GitHub de review

`.assistente/lasst-mcp.json` pode habilitar o conector GitHub remoto; o destino é fixo e a configuração do repositório não pode fornecer URL, executável, segredo ou escopo. O ambiente confiável fornece `LASST_GITHUB_TOKEN`, `LASST_GITHUB_REPOSITORY`, `LASST_GITHUB_PR`. O cliente utiliza o SDK MCP e a bridge existentes, com segredo em memória e redirecionamentos restritos.

Somente leitura do PR selecionado é exposta; owner, repositório e número vêm do ambiente confiável. Escrita e publicação de review ficam bloqueadas até resolver a identificação da review pendente e vinculação ao commit quando há concorrência. Ainda é necessário testar o servidor real e validar o fluxo em PR de teste antes de substituir o workflow. Skills e configurações do PR permanecem dados não confiáveis: permissões restritas reduzem impactos, mas não impedem comentários manipulados por instruções hostis. Esta CLI separada não muda o runtime MCP de banco do Assistente previsto no AEP-0049.
