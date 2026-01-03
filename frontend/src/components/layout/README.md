# Layout

Componentes de estrutura da aplicação.

## Componentes

### Layout

Container principal que organiza a estrutura da página com Topbar e área de conteúdo.

```svelte
<script>
  import { Layout } from '../components/layout';
  
  let currentPage = 'chat';
  let hasApiKey = true;
</script>

<Layout {currentPage} {hasApiKey} on:navigate={handleNavigate}>
  <MyPageContent />
</Layout>
```

**Props:**
- `currentPage` - ID da página atual (para destacar no menu)
- `hasApiKey` - Se o usuário tem API key configurada (controla itens do menu)

**Eventos:**
- `navigate` - Disparado quando o usuário seleciona uma página no menu

**Slots:**
- default - Conteúdo principal da página

---

### Topbar

Barra superior com menu de navegação e indicador de página atual.

```svelte
<script>
  import { Topbar } from '../components/layout';
</script>

<Topbar 
  currentPage="chat" 
  hasApiKey={true} 
  on:navigate={handleNavigate} 
/>
```

**Props:**
- `currentPage` - ID da página atual
- `hasApiKey` - Controla quais itens do menu são exibidos

**Eventos:**
- `navigate` - Disparado com o ID da página selecionada

**Atalhos de Teclado:**
| Atalho | Ação |
|--------|------|
| `Alt+M` | Abre/fecha menu |
| `Alt+1` | Chat |
| `Alt+2` | Histórico |
| `Alt+3` | FAQ |
| `Alt+4` | Memórias |
| `Alt+5` | Agentes |
| `Alt+6` | Conexões OAuth |
| `Alt+7` | Configurações |

**Navegação no Menu (quando aberto):**
- `↑/↓` - Navegar entre itens
- `Home/End` - Primeiro/último item
- `Enter/Space` - Selecionar item
- `Escape` - Fechar menu

---

## Importação

```javascript
import { Layout, Topbar } from '../components/layout';
```



