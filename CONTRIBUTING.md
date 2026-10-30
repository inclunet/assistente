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

## Revisão assistida por IA

Uma revisão sob demanda pode ser iniciada comentando exatamente `/review` em um
pull request. O PR-Agent publica um resumo e comentários inline para problemas
que conseguir ancorar no diff. A automação só aceita comentários de membros,
colaboradores e donos do repositório; ela não revisa automaticamente ao abrir o
PR nem a cada push.

O modelo é o DeepSeek V4.1 Flash pela API oficial (`https://api.deepseek.com`).
O identificador `deepseek-v4-flash` usado pelo PR-Agent/LiteLLM é o alias de
compatibilidade que a DeepSeek atualmente direciona ao modelo `deepseek-flash`.
Para habilitar o fluxo, crie uma chave na [plataforma da DeepSeek](https://platform.deepseek.com/)
e salve-a no repositório como o segredo `DEEPSEEK_API_KEY`, em
**Settings → Secrets and variables → Actions**. O `GITHUB_TOKEN` é fornecido
automaticamente pelo Actions com permissão para comentar no PR.

O resumo de cada revisão pode incluir modelo, tokens, chamadas e custo estimado
quando esses dados estão disponíveis. A
estimativa do PR-Agent usa a tabela de preços do LiteLLM e pode divergir da
cobrança, pois a DeepSeek aplica tarifas diferentes por horário e cache. Para
acompanhar o valor efetivamente cobrado, confira o uso na plataforma da DeepSeek
e a [tabela oficial de preços](https://api-docs.deepseek.com/quick_start/pricing/).
O workflow define um orçamento de contexto de 128 mil tokens no PR-Agent e
limita a saída de cada chamada a 32.768 tokens. O orçamento disponível para o
prompt é menor que 128 mil, pois o PR-Agent reserva espaço para a resposta e suas
instruções. `reasoning_effort=none` desativa o modo de raciocínio da DeepSeek,
deixando o teto de saída disponível ao texto da revisão.
O workflow também desativa o fallback OpenAI padrão do PR-Agent: a revisão usa
apenas a API DeepSeek configurada. Esses valores são tetos, não consumo garantido;
confira o custo estimado no resumo e o uso efetivamente cobrado no painel da
DeepSeek.
Novas execuções continuam dependendo de um novo `/review` manual. Não há teto
agregado de gastos nem limite de frequência: cada execução adicional pode
gerar custo.

## Catálogo público

Para atualizar provedores, modelos, limites, parâmetros e vozes, siga o
[guia de manutenção do catálogo](docs/content/guias/CATALOGO_PROVEDORES_MODELOS.md).
O asset deve passar pelo JSON Schema e pelo validador do aplicativo antes da
publicação; o workflow do site executa a mesma suíte de validação.
