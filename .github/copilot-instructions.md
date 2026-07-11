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
