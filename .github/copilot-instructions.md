# Instruções para o GitHub Copilot

As regras canônicas do projeto estão em `AGENTS.md` na raiz do repositório.
Aplique todas elas em code review, chat e coding agent. Destaques para review:

- Idioma: responda e comente em Português (pt-BR).
- Acessibilidade é requisito fundamental (NVDA/teclado): sinalize qualquer
  regressão de foco, ARIA, contraste ou navegação por teclado.
- Nada de cores/font-sizes hardcoded em CSS — apenas tokens de
  `frontend/src/theme.css`.
- Toda string visível ao usuário deve ser internacionalizada nos 3 locales
  (`pt-BR`, `en`, `es`).
- Mudanças devem respeitar os AEPs em `aep/` (decisões arquiteturais) e a
  arquitetura backend-driven de messaging (AEP-0040).
- Testes: nunca deletar ou simplificar testes para "resolver" falhas; toda
  mudança de comportamento precisa de cobertura.

## Execução (específico do Copilot no VS Code: preferir Tasks)
- Para lint/build/test, prefira rodar **VS Code Tasks** em vez de comandos via shell.
- Use `run_task`/`create_and_run_task` quando precisar executar algo repetível e seguro.
- Tasks disponíveis em `.vscode/tasks.json` (use estas primeiro):
  - `Frontend: lint`
  - `Frontend: test`
  - `Frontend: build`
  - `Go: test ./...`
  - `Check: all (go test + frontend lint+test+build)`
- Só use shell quando não existir Task equivalente ou para diagnóstico pontual.
