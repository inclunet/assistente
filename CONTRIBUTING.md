# Contribuindo com o Assistente

## Antes de abrir um PR

### Rodar localmente

```bash
cd frontend
npm run lint        # ESLint + jsx-a11y
npm run lint:css    # Stylelint (tokens)
npx tsc --noEmit    # TypeScript
npm run test        # Vitest + axe-core
```

### Checklist obrigatório

#### Internacionalização (i18n)
- [ ] Toda string visível ao usuário usa `t('namespace.key', 'fallback')`
- [ ] Chave adicionada nos 3 locales: `pt-BR.ts`, `en.ts`, `es.ts`
- [ ] Nenhum texto hardcoded em português, inglês ou espanhol no JSX

#### Design System (CSS)
- [ ] Cores usam variáveis do `theme.css` (nunca `#hex`, `rgb`, `rgba`)
- [ ] Font-sizes usam tokens: `--font-size-xs`, `--font-size-sm`, `--font-size-base`, `--font-size-lg`, `--font-size-xl`
- [ ] Inputs/selects têm `height: 32px`
- [ ] Elementos interativos têm `min-height: 36px` (touch target)
- [ ] Nenhum `@media (prefers-color-scheme)` — sistema usa `data-theme`

#### Acessibilidade
- [ ] Ícones decorativos (ao lado de texto) têm `aria-hidden="true"`
- [ ] Botões icon-only têm `aria-label` descritivo
- [ ] Navegação por teclado: Tab, Enter, ESC, setas
- [ ] `announce()` para feedback de ações (toast/status)
- [ ] `:focus-visible` em vez de `:focus` para estilos de foco
- [ ] Testado com leitor de tela (NVDA, VoiceOver ou Narrator)

#### Testes
- [ ] Testes unitários para nova lógica
- [ ] `npx vitest run` passa com 0 falhas
- [ ] Build de produção: `npx vite build` sem erros

## CI automatizado

O workflow em `.github/workflows/ci.yml` roda em todo PR:

- **Go tests**: `go test ./...` — testes unitários e de integração do backend
- **TypeScript**: `tsc --noEmit`
- **ESLint** (incluindo regras `jsx-a11y`): detecta ARIA inválido, roles ausentes
- **Stylelint**: impede cores e font-sizes hardcoded (deve usar tokens do `theme.css`)
- **Vitest** (incluindo testes `axe-core` de acessibilidade)

Se qualquer step falhar, o PR não pode ser mergeado.
