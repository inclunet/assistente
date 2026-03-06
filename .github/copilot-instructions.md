# Instruções para GitHub Copilot (assistente)

## Logs (performance + ruído)
- Evite adicionar `console.log` em código de produção.
- Ao mexer em um recurso/arquivo, remova `console.log` existentes no trecho tocado.
- `console.warn` e `console.error` são permitidos quando representam condição anômala real.
- Se for indispensável logar para diagnóstico, prefira:
  - log temporário removido antes do merge; ou
  - log protegido por flag (ex.: `import.meta.env.DEV` + toggle em `localStorage`).

## Escopo de mudanças
- Não faça “varreduras globais” para remover logs em arquivos grandes sem necessidade do PR; aplique a limpeza incrementalmente quando o recurso for alterado.

## Execução (preferir VS Code Tasks)
- Para lint/build/test, prefira rodar **VS Code Tasks** ao invés de comandos via shell/terminal.
- Use `run_task`/`create_and_run_task` quando precisar executar algo repetível e seguro.
- Tasks já disponíveis em `.vscode/tasks.json` (use estas primeiro):
  - `Frontend: lint`
  - `Frontend: test`
  - `Frontend: build`
  - `Go: test ./...`
  - `Check: all (go test + frontend lint+test+build)`
- Só use shell/terminal quando não existir Task equivalente ou quando for necessário diagnosticar algo pontual.

## Testes (não deixar feature descoberta)
- Sempre verificar se a mudança/feature está coberta por testes.
  - Frontend: preferir **Vitest** para utilitários, hooks e componentes.
  - Backend: preferir `go test ./...` com testes de unidade/integração onde fizer sentido.
- Se não houver teste cobrindo o comportamento alterado (ou se a cobertura estiver fraca), criar/atualizar testes junto com a mudança.
- Ao adicionar testes, manter o escopo focado na feature tocada (evitar “mass refactor” de testes).
- Antes de concluir, rodar as tasks de teste relevantes (preferência por `Frontend: test` e/ou `Go: test ./...`).
