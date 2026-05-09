# AEP-0060: Parser e Política de Comandos

## Status: Draft

## Resumo

Criar um pacote interno dedicado para analisar linhas de comando antes da execução da ferramenta `run_command`, separando comandos compostos em unidades avaliáveis e aplicando uma política de segurança por programa, subcomando, argumentos e recursos de shell usados.

A implementação deve ser agnóstica de plataforma por contrato: em vez de tentar reproduzir integralmente Bash, PowerShell ou `cmd.exe`, o projeto define uma subset comum e conservadora. Comandos fora dessa subset não são autoaprovados.

## Motivação

A allowlist atual avalia a string inteira do comando. Isso causa dois problemas:

- comandos compostos seguros, como `git status && git diff`, pedem confirmação mesmo quando cada comando individual já é permitido;
- comandos parcialmente perigosos podem ficar difíceis de classificar, porque a política não distingue programa, subcomando e operadores.

Também há comandos em que o primeiro token não basta para decidir segurança. `kubectl get pods` costuma ser leitura, enquanto `kubectl delete pod x` é destrutivo e `kubectl patch deployment x` altera estado. O sistema precisa expressar essa diferença sem depender de heurísticas frágeis no ponto de execução.

## Decisões

### 1. Pacote dedicado

Adicionar `internal/commandpolicy` para concentrar:

- parsing da linha de comando;
- representação de comandos atômicos;
- detecção de features conservadoras;
- avaliação e agregação de decisões.

`internal/tools/shell/run_command.go` deve apenas solicitar a avaliação, decidir se pede confirmação e executar a string original no PTY quando permitido.

### 2. Subset cross-platform

O parser da primeira entrega reconhece uma subset comum:

- comandos simples com argumentos;
- aspas simples e duplas;
- escapes básicos **dentro de aspas duplas** (ver nota abaixo);
- separadores `;`, `&&`, `||` e newline;
- redirecionamentos `>`, `>>`, `2>`, `2>>`, `<`, `<<`;
- pipe `|`, background `&` e constructs suspeitos como features conservadoras.

Tokens de separação ou redirecionamento dentro de aspas são tratados como argumentos comuns.

#### Backslash (`\`) fora de aspas é literal

Em POSIX, `\` fora de aspas é caractere de escape. Esta subset diverge intencionalmente: `\` fora de aspas é **literal** (e o byte seguinte continua processado normalmente). Justificativas:

- Windows usa `\` como separador de path. Tratar como escape POSIX engole a barra (`C:\Windows` viraria `C:Windows`) e quebra paths reais e patterns da allowlist — o `AlwaysDeny` default `del /s /q C:\` deixaria de bater silenciosamente, degradando uma decisão de `deny` para `confirm`.
- O contexto de entrada é uma única linha de comando vinda de `a.Command` (não um script multi-linha), então `\` no fim de linha é mais provavelmente um path Windows do que continuação de linha.
- Dentro de aspas duplas, `\` continua funcionando como escape para `\"`, `\\`, `` \` `` e `\$` (ver `readQuoted`). Quem precisar de escape POSIX usa aspas duplas.

Qualquer sintaxe ambígua, incompleta ou fora da subset resulta em decisão `confirm`, nunca `approve`.

### 3. Avaliação por comando atômico

Uma linha composta é aprovada automaticamente apenas quando todos os comandos atômicos são aprovados e nenhuma feature conservadora exige confirmação.

A agregação segue precedência conservadora:

1. qualquer `deny` torna a linha `deny`;
2. qualquer `confirm` torna a linha `confirm`;
3. somente tudo `approve` torna a linha `approve`.

### 4. Regras estruturadas

Estender a allowlist com regras estruturadas, mantendo compatibilidade com `AutoApprove` e `AlwaysDeny`.

As regras estruturadas permitem diferenciar subcomandos:

```go
type CommandRule struct {
    Program     string   `json:"program"`
    Subcommands []string `json:"subcommands,omitempty"`
    Args        []string `json:"args,omitempty"`
    Decision    string   `json:"decision"`
    Description string   `json:"description,omitempty"`
}
```

`Description` é texto livre exibido na UI da allowlist e nas razões verbosas (`DetailedReasons`); nunca aparece em `Reasons` (LLM-bound) — ver §5b.

Exemplos esperados:

- `kubectl get *` pode ser `approve`;
- `kubectl describe *` pode ser `approve`;
- `kubectl delete *` deve ser `confirm` ou `deny`;
- `kubectl patch *` deve ser `confirm`.

Regras `deny` têm precedência sobre regras `confirm` e `approve`.

#### Precedência entre regras que casam o mesmo átomo

Quando mais de uma regra casa o mesmo `(program, subcommands, args)` para um átomo, a ordem de resolução é fail-closed e separada da agregação entre átomos compostos:

1. regra estruturada `deny`;
2. pattern legado `always_deny`;
3. regra estruturada `confirm` — **antes** de `approve`;
4. regra estruturada `approve`;
5. pattern legado `auto_approve`;
6. `default_action` da allowlist.

A intenção é permitir que o usuário adicione uma exceção restritiva (ex.: `kubectl get secret` com `decision=confirm`) sobre uma regra mais permissiva (`kubectl get *` com `decision=approve`) sem ter que reordenar manualmente. Para a agregação entre átomos compostos (linha `git status && rm -rf dist`), a precedência continua sendo `deny > confirm > approve`, conforme item 3.

#### Wildcard `*`

`*` em `Subcommands`/`Args` é interpretado como "casa o restante" e só é permitido na **última posição** da lista. Em qualquer outra posição é tratado como literal — uma regra como `Subcommands: ["pod", "*", "--force"]` jamais casa, porque `*` no meio nunca é wildcard. A validação no `Manager.save` rejeita explicitamente regras com `*` fora da última posição, mas o evaluator preserva o mesmo comportamento defensivo para regras carregadas de perfis legados ou editadas manualmente.

#### Validação no save

O `Manager.save` rejeita allowlists com regras estruturadas inválidas:

- `Program` vazio ou só whitespace;
- `Decision` ausente ou diferente de `approve|confirm|deny` (case-insensitive);
- `*` fora da última posição em `Subcommands` ou `Args`.

Em runtime, `parseRuleDecision` mantém o fail-closed: valor desconhecido vira `confirm`. A combinação garante que o problema seja detectado cedo (no save) e que perfis pré-existentes inválidos não exponham o sistema a aprovações indevidas.

### 5. Compatibilidade

As listas legadas `AutoApprove` e `AlwaysDeny` continuam funcionando para perfis e allowlists existentes. Elas passam a ser avaliadas contra cada comando atômico quando a linha puder ser parseada com segurança.

### 5b. Atribuições inline (`KEY=VALUE cmd ...`) e contrato de não-vazamento

Atribuições inline de env (`TOKEN=secret cmd ...`) são uma porta fácil para que valores sensíveis vazem em log, em `Reasons` e no `Content` devolvido ao LLM. O parser as trata explicitamente:

- tokens iniciais que casam o formato POSIX `[A-Za-z_][A-Za-z0-9_]*=...` são consumidos em `Command.EnvAssignments` antes de o `Program` ser definido;
- a presença desses prefixos adiciona `FeatureEnvAssignment`, que faz `RequiresConfirmation()` devolver `true` — env inline jamais é auto-aprovado mesmo quando o programa real está em `AutoApprove`;
- uma linha contendo apenas atribuições (`TOKEN=secret`) é tratada como sintaxe ambígua e não gera `Command`, com mensagem genérica para não revelar nem o nome nem o valor da variável.

`EvaluationResult` separa **dois** slices de motivos:

- `Reasons`: seguro para envio externo (LLM, telemetria). Contém apenas `program`, tipo da regra e índice de correlação (`rule[N]`, `always_deny[N]`, `auto_approve[N]`). Nunca interpola pattern bruto, `Subcommands`/`Args`/`Description` de regras estruturadas, nem o lado direito de uma atribuição que tenha escapado.
- `DetailedReasons`: uso **estritamente** ao vivo na UI do desktop, onde o usuário já enxerga o conteúdo da allowlist. Mantém a forma verbosa anterior. Nunca deve ser escrito em arquivo de log nem anexado a relatos de bug.

Como defesa em profundidade, o evaluator e o `redactCommandForLog` aplicam `redactProgramForReason`/`redactProgramSegment` em qualquer `Program` que contenha `=` (perfis legados, `Command` construído fora do parser): o lado direito vira `=<redacted>`. Logs locais usam `Reasons` (safe) e contagens/features — nunca `DetailedReasons`.

### 6. Migração de allowlists existentes

`Manager.EnsureDefaults` é executado no boot e historicamente só criava `padrao.json` quando o diretório de allowlists estava vazio. Isso impediria que usuários pré-existentes recebessem as novas `CommandRules` para `kubectl` introduzidas neste AEP.

Adicionamos uma migração idempotente, executada após o early-return:

- Se `padrao.json` existe, é carregado.
- Para cada `CommandRule` em `DefaultAllowlist()`, comparamos `Program` (case-insensitive) contra os programas já presentes em `al.CommandRules`.
- Programas ausentes recebem todas as regras default daquele programa.
- Se nada precisa ser adicionado, **nenhuma escrita** acontece. A leitura (`Exists` + `loadFromFile`) sempre é executada para descobrir o estado atual; o que evitamos quando não há regras a mesclar é apenas a re-serialização e o `Write` no disco.

A migração respeita personalizações: se o usuário já tem qualquer regra estruturada para `kubectl`, deixamos suas regras intactas. Apenas programas inexistentes nas regras estruturadas do usuário recebem o default.

## Fases

### Fase 1 — Documentação arquitetural

- Registrar esta AEP.
- Definir a primeira subset suportada e os comportamentos conservadores.

### Fase 2 — Package `commandpolicy`

- Criar AST mínima para comandos atômicos.
- Implementar parser da subset comum.
- Detectar separadores, redirecionamentos, pipes, background e sintaxe ambígua.
- Adicionar testes unitários do parser.

### Fase 3 — Allowlist estruturada

- Estender o modelo de allowlist com regras estruturadas.
- Manter compatibilidade com regras legadas.
- Cobrir precedência e subcomandos com testes.

### Fase 4 — Integração com `run_command`

- Substituir a avaliação direta de string por `commandpolicy.Evaluate`.
- Logar motivos de confirmação/bloqueio para facilitar diagnóstico.
- Preservar execução da string original no PTY.

### Fase 5 — Validação

- Rodar testes do pacote de política.
- Rodar testes de allowlist e shell afetados.
- Garantir que comandos compostos seguros sejam autoaprovados e comandos destrutivos exijam confirmação ou sejam bloqueados.

## Riscos

- Um parser permissivo demais pode autoaprovar comandos que deveriam pedir confirmação.
- Um parser restritivo demais pode gerar prompts extras, mas esse é o erro aceitável para a primeira entrega.
- PowerShell e `cmd.exe` têm gramáticas próprias; por isso a subset do projeto deve ser explícita e não vendida como parser completo dessas shells.
- Redirecionamentos e heredocs podem alterar arquivos ou esconder efeitos colaterais; na primeira entrega eles exigem confirmação.

## Critérios de aceitação

- `git status && git diff` pode ser autoaprovado quando ambos os comandos são permitidos.
- `git status && rm -rf dist` não pode ser autoaprovado.
- `kubectl get pods` pode ser autoaprovado por regra estruturada.
- `kubectl delete pod x` e `kubectl patch deployment x` não são autoaprovados por uma regra genérica de `kubectl`.
- `>`, `>>`, `2>`, `2>>`, `<`, `<<`, `|`, `$()` e backticks forçam confirmação na primeira entrega.
- Tokens especiais dentro de aspas não são tratados como operadores.
- Regras legadas continuam funcionando.
- Testes cobrem parser, agregação de decisões, regras estruturadas e integração com `run_command`.
- Atribuições inline `KEY=VALUE` antes do programa são consumidas em `Command.EnvAssignments`, forçam `confirm` e nunca aparecem em `Reasons`, log do `run_command` ou `Content` devolvido ao LLM.
