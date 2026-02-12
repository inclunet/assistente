# Menu de Navegação Acessível

## Visão Geral
O menu principal da Topbar implementa o padrão ARIA Menu Button com navegação completa por teclado, seguindo as melhores práticas de acessibilidade.

## Componentes

### MenuButton
Componente reutilizável que implementa um menu dropdown acessível.

**Localização:** `frontend/src/components/layout/MenuButton.tsx`

## Navegação por Teclado

### No Botão do Menu
- **Enter/Space**: Abre o menu
- **Arrow Down**: Abre o menu e foca o primeiro item

### Dentro do Menu (quando aberto)
- **Arrow Down**: Move para o próximo item (não circular)
- **Arrow Up**: Move para o item anterior (não circular)  
- **Home**: Move para o primeiro item
- **End**: Move para o último item
- **Enter/Space**: Executa a ação do item focado e fecha o menu
- **Escape**: Fecha o menu e retorna o foco ao botão
- **Tab**: Fecha o menu e move o foco para o próximo elemento

### Mouse
- **Click no item**: Executa a ação e fecha o menu
- **Click fora**: Fecha o menu

## Atributos ARIA

### MenuButton (Botão)
```html
<button
  aria-expanded="true|false"    <!-- Estado do menu -->
  aria-haspopup="menu"           <!-- Indica que há um menu -->
  aria-label="Menu de navegação" <!-- Label descritivo -->
>
```

### Menu (Lista)
```html
<ul role="menu" aria-label="Navegação principal">
```

### MenuItem (Item do menu)
```html
<button
  role="menuitem"
  aria-current="page"         <!-- Apenas no item ativo -->
  tabindex="0|-1"            <!-- 0 no focado, -1 nos outros -->
>
```

## Padrão Roving Tabindex

Apenas um item do menu tem `tabindex="0"` por vez:
- Quando o menu abre, foca o item atual (ou o primeiro se não houver atual)
- Arrow keys movem o foco e atualizam o tabindex
- Isso permite navegação fluida sem prender o usuário com Tab

## Estados Visuais

### Item Ativo (página atual)
- Classe CSS: `.nav-item.active`
- Background: `rgba(88, 166, 255, 0.15)`
- Cor do texto: `var(--color-accent)`
- Atributo: `aria-current="page"`

### Item com Foco
- Outline: `2px solid var(--color-accent)`
- Background: `var(--color-bg-tertiary)`

### Item em Hover
- Background: `var(--color-bg-tertiary)`
- Cor do texto: `var(--color-text-primary)`

## Integração com React Router

O menu usa `useNavigate()` e `useLocation()` para navegação:

```typescript
const navigate = useNavigate();
const location = useLocation();

const menuItems: MenuItem[] = [
  {
    id: 'chat',
    label: 'Chat',
    icon: '💬',
    onClick: () => navigate('/'),
  },
  // ...
];
```

## Estrutura de MenuItem

```typescript
interface MenuItem {
  id: string;          // Identificador único
  label: string;       // Texto exibido
  icon: string;        // Emoji/ícone
  shortcut?: string;   // Atalho de teclado (ex: "Alt+1")
  onClick: () => void; // Ação ao clicar/Enter
}
```

## Testes de Acessibilidade

### Com Teclado
1. ✅ Tab até o botão do menu
2. ✅ Enter/Space abre o menu
3. ✅ Arrow Down/Up navega entre itens
4. ✅ Home/End vai para primeiro/último
5. ✅ Enter no item executa ação
6. ✅ Escape fecha o menu
7. ✅ Tab fecha o menu e continua navegação

### Com Leitor de Tela
- [ ] Botão anuncia "Menu de navegação, botão, expandido/recolhido"
- [ ] Menu anuncia "Navegação principal, menu"
- [ ] Itens anunciam "Chat, item de menu" ou "Configurações, item de menu"
- [ ] Item ativo anuncia "página atual"
- [ ] Foco em item anuncia o label e estado

### Com Mouse
1. ✅ Click no botão abre/fecha
2. ✅ Click no item executa ação
3. ✅ Click fora fecha o menu
4. ✅ Hover destaca item

## Diferenças do Svelte

### Mantido
- ✅ Padrão ARIA Menu Button
- ✅ Navegação por teclado completa
- ✅ Roving tabindex
- ✅ Estados visuais consistentes
- ✅ Foco inicial no item atual

### Removido (por enquanto)
- ❌ Atalhos globais Alt+Key (pode ser adicionado depois)
- ❌ Indicador de "página atual" no canto direito da Topbar

### Melhorado
- ✅ Navegação não-circular (para no primeiro/último)
- ✅ Integração nativa com React Router
- ✅ TypeScript com tipos completos
- ✅ Hook reutilizável
- ✅ Componente isolado e testável

## Próximos Passos

1. **Atalhos Globais**: Implementar Alt+Key para abrir menu e navegar
2. **Testes com NVDA/JAWS**: Validar anúncios e navegação
3. **Mais Itens**: Adicionar Histórico, etc.
4. **Animações**: Transições suaves de abertura/fechamento
5. **Temas**: Garantir contraste em modo claro/escuro

## Referências

- [ARIA Authoring Practices: Menu Button](https://www.w3.org/WAI/ARIA/apg/patterns/menubutton/)
- [ARIA Authoring Practices: Menu](https://www.w3.org/WAI/ARIA/apg/patterns/menu/)
- [MDN: ARIA role=menu](https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Roles/menu_role)
