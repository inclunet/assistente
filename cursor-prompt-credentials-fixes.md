# Correções no sistema de credenciais

Há dois problemas para corrigir:

## 1. Struct `KeyringEntry` duplicada entre build tags

A struct `KeyringEntry` está definida identicamente em dois arquivos com build tags:

- `internal/credentials/keyring_list.go` (`//go:build windows`) — linhas 12-15
- `internal/credentials/keyring_list_other.go` (`//go:build !windows`) — linhas 6-9

Se alguém editar uma e esquecer a outra, quebra silenciosamente (compila em uma plataforma, falha na outra).

**Correção**: Mover a struct `KeyringEntry` para `internal/credentials/keyring.go` (que não tem build tag, portanto é compilado sempre). Os dois arquivos com build tag ficam só com a função `ListKeyringEntries()`.

Resultado esperado:

- `keyring.go`: adicionar a struct `KeyringEntry` (após as constantes existentes)
- `keyring_list.go`: remover a struct, manter só a função
- `keyring_list_other.go`: remover a struct, manter só a função

## 2. Autocomplete de credenciais com acessibilidade ruim para leitores de tela

O autocomplete no campo Token em `frontend/src/pages/CredentialsPage.tsx` (linhas 238-268) tem problemas sérios de acessibilidade:

### Problemas atuais:
- O `<input>` não tem `role="combobox"` — leitores de tela não anunciam que é um campo com sugestões
- Não tem `aria-expanded` — leitor de tela não sabe se a lista de sugestões está aberta ou fechada
- Não tem `aria-controls` / `aria-owns` ligando o input ao `<ul>` de sugestões
- Não tem `aria-activedescendant` — leitor de tela não sabe qual item está focado
- Não há navegação por teclado (setas ↑/↓) nas sugestões — só funciona com mouse
- Os `<li>` com `role="option"` não têm `id` (necessário para `aria-activedescendant`)
- Os `<li>` não têm `aria-selected`

### Correção: implementar o padrão ARIA combobox com listbox

O padrão é descrito em https://www.w3.org/WAI/ARIA/apg/patterns/combobox/

Comportamento esperado:

**No `<Input>` (combobox):**
- `role="combobox"` 
- `aria-expanded={showSuggestions && suggestions.length > 0}`
- `aria-controls="token-suggestions"` (id do `<ul>`)
- `aria-activedescendant={activeDescendantId}` (id do `<li>` ativo, ou undefined se nenhum)
- `aria-autocomplete="list"`

**No `<ul>` (listbox):**
- `id="token-suggestions"`
- `role="listbox"` (já tem)
- `aria-label="Sugestões de referência"` (já tem)

**Nos `<li>` (options):**
- `id={`suggestion-${index}`}` (necessário para aria-activedescendant)
- `role="option"` (já tem)
- `aria-selected={index === activeIndex}`

**Navegação por teclado no input:**
- `ArrowDown`: move activeIndex para próximo item (ou primeiro se nenhum selecionado)
- `ArrowUp`: move activeIndex para item anterior
- `Enter`: seleciona o item ativo (se houver) e fecha a lista
- `Escape`: fecha a lista de sugestões
- Quando activeIndex muda, atualizar `aria-activedescendant` para o id do `<li>` correspondente

**Estado necessário:**
- Adicionar `const [activeIndex, setActiveIndex] = useState<number>(-1)` (-1 = nenhum ativo)
- Resetar activeIndex para -1 quando a lista de sugestões muda (novo filtro) ou fecha
- Derivar `activeDescendantId`: `activeIndex >= 0 ? `suggestion-${activeIndex}` : undefined`

**Handler de teclado (onKeyDown no Input):**
```tsx
const handleTokenKeyDown = (e: React.KeyboardEvent) => {
  if (!showSuggestions || suggestions.length === 0) return;
  
  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault();
      setActiveIndex(prev => prev < suggestions.length - 1 ? prev + 1 : 0);
      break;
    case 'ArrowUp':
      e.preventDefault();
      setActiveIndex(prev => prev > 0 ? prev - 1 : suggestions.length - 1);
      break;
    case 'Enter':
      if (activeIndex >= 0 && activeIndex < suggestions.length) {
        e.preventDefault();
        crud.updateField('token', suggestions[activeIndex].value);
        setShowSuggestions(false);
        setActiveIndex(-1);
      }
      break;
    case 'Escape':
      setShowSuggestions(false);
      setActiveIndex(-1);
      break;
  }
};
```

**Resetar activeIndex** quando sugestões mudam: no `handleTokenChange`, após atualizar `setSuggestions(...)`, adicionar `setActiveIndex(-1)`.

**Scroll do item ativo para visão**: quando `activeIndex` muda, fazer scroll do `<li>` ativo para dentro da viewport do `<ul>` com `scrollIntoView({ block: 'nearest' })`. Isso requer refs nos `<li>` ou um ref no `<ul>` com `querySelector`.

**Não esquecer:**
- O `onBlur` existente com `setTimeout(150ms)` deve continuar funcionando (permite clique na sugestão)
- Quando o editor fecha (`handleCloseEditor`), resetar `activeIndex` também
