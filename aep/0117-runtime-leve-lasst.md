# AEP-0117 — Runtime leve lasst

**Status:** In Progress — prova de conceito inicial implementada; MCP e review pendentes

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
- Um workflow futuro deve manter instruções, binário e configuração confiáveis separados do checkout do PR, registrar o SHA analisado e tratar o conteúdo do PR como dados não confiáveis.

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
