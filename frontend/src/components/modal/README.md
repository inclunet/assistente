# Modal Components

Modais acessíveis com focus trap, portal, restauração de foco e suporte completo a teclado.

## Componentes

| Componente | Descrição |
|------------|-----------|
| `Modal` | Modal genérico com título e conteúdo customizável via slot |
| `ImageModal` | Modal otimizado para visualização de imagens em tela cheia |

## Features

- ✅ **Portal** - Renderiza no body, evita problemas de z-index e contextos ARIA
- ✅ **Focus trap** - Foco permanece dentro do modal
- ✅ **Escape para fechar** - Tecla Escape fecha o modal
- ✅ **Restauração de foco** - Retorna ao elemento anterior ao fechar
- ✅ **Auto-focus inteligente** - Foca no elemento mais apropriado
- ✅ **Backdrop clicável** - Clique fora fecha o modal
- ✅ **Tab cycling** - Tab/Shift+Tab circula entre elementos
- ✅ **ARIA completo** - role="dialog", aria-modal, aria-labelledby
- ✅ **Reduced motion** - Respeita preferências de animação do usuário

## Instalação

```javascript
import { Modal, ImageModal } from './components/modal';
```

## Uso Básico

```svelte
<script>
  import { Modal } from './components/modal';
  
  let showModal = false;
  
  function handleSave() {
    // Salvar dados
    showModal = false;
  }
</script>

<button on:click={() => showModal = true}>
  Abrir Modal
</button>

<Modal 
  title="Configurações" 
  open={showModal} 
  on:close={() => showModal = false}
>
  <form on:submit|preventDefault={handleSave}>
    <label>
      Nome:
      <input type="text" name="name" />
    </label>
    
    <div class="actions">
      <button type="button" on:click={() => showModal = false}>
        Cancelar
      </button>
      <button type="submit">
        Salvar
      </button>
    </div>
  </form>
</Modal>
```

## Props

| Prop | Tipo | Default | Descrição |
|------|------|---------|-----------|
| `title` | `string` | `''` | Título do modal (exibido no header) |
| `open` | `boolean` | `false` | Controla visibilidade |
| `autoFocus` | `boolean` | `true` | Se deve focar automaticamente ao abrir |

## Eventos

| Evento | Payload | Descrição |
|--------|---------|-----------|
| `close` | - | Disparado quando modal deve fechar |

## Auto-Focus Inteligente

Quando `autoFocus={true}` (padrão), o modal foca automaticamente no elemento mais apropriado:

1. **Primeiro**: Campos de formulário (`input`, `select`, `textarea`)
2. **Segundo**: Botões de ação dentro do conteúdo
3. **Terceiro**: Qualquer elemento focável no conteúdo
4. **Fallback**: Botão de fechar

```svelte
<!-- Foco vai para o input automaticamente -->
<Modal title="Login" {open} on:close={...}>
  <input type="email" placeholder="Email" />  <!-- ← Foco aqui -->
  <input type="password" placeholder="Senha" />
  <button type="submit">Entrar</button>
</Modal>
```

### Desabilitando Auto-Focus

Use `autoFocus={false}` quando você precisa controlar o foco manualmente:

```svelte
<Modal title="Alerta" {open} autoFocus={false} on:close={...}>
  <p>Tem certeza?</p>
  <button bind:this={confirmBtn}>Confirmar</button>
</Modal>

<script>
  let confirmBtn;
  $: if (open) confirmBtn?.focus();
</script>
```

## Focus Trap

O modal implementa focus trap - o foco permanece dentro do modal:

- `Tab` → Próximo elemento focável (circular)
- `Shift+Tab` → Elemento anterior (circular)
- Foco não escapa para elementos fora do modal

## Navegação por Teclado

| Tecla | Ação |
|-------|------|
| `Escape` | Fecha o modal |
| `Tab` | Próximo elemento (circular) |
| `Shift+Tab` | Elemento anterior (circular) |

## Backdrop

Clicar no backdrop (área escura fora do modal) fecha o modal automaticamente.

## Slots

O modal usa um slot padrão para o conteúdo:

```svelte
<Modal title="Meu Modal" {open} on:close={...}>
  <!-- Seu conteúdo aqui -->
  <p>Qualquer conteúdo HTML/Svelte</p>
  <MyComponent />
</Modal>
```

## Estilização

O componente usa CSS custom properties:

```css
/* Cores */
--color-bg-secondary    /* Fundo do modal */
--color-bg-tertiary     /* Fundo do header */
--color-border          /* Bordas */
--color-text-primary    /* Texto do título */

/* Espaçamentos */
--spacing-md            /* Padding interno */
--spacing-lg            /* Padding do conteúdo */

/* Outros */
--border-radius         /* Arredondamento */
```

## Exemplos Avançados

### Modal de Confirmação

```svelte
<script>
  let showConfirm = false;
  let pendingAction = null;
  
  function confirmDelete(item) {
    pendingAction = () => deleteItem(item);
    showConfirm = true;
  }
  
  function handleConfirm() {
    pendingAction?.();
    showConfirm = false;
  }
</script>

<Modal title="Confirmar Exclusão" open={showConfirm} on:close={() => showConfirm = false}>
  <p>Tem certeza que deseja excluir este item?</p>
  <p>Esta ação não pode ser desfeita.</p>
  
  <div class="modal-actions">
    <button on:click={() => showConfirm = false}>Cancelar</button>
    <button class="danger" on:click={handleConfirm}>Excluir</button>
  </div>
</Modal>
```

### Modal com Formulário

```svelte
<Modal title="Novo Usuário" {open} on:close={close}>
  <form on:submit|preventDefault={save}>
    <div class="form-group">
      <label for="name">Nome</label>
      <input id="name" bind:value={name} required />
    </div>
    
    <div class="form-group">
      <label for="email">Email</label>
      <input id="email" type="email" bind:value={email} required />
    </div>
    
    <div class="form-group">
      <label for="role">Função</label>
      <select id="role" bind:value={role}>
        <option value="user">Usuário</option>
        <option value="admin">Admin</option>
      </select>
    </div>
    
    <div class="modal-actions">
      <button type="button" on:click={close}>Cancelar</button>
      <button type="submit">Criar</button>
    </div>
  </form>
</Modal>
```

### Modal Grande com Scroll

```svelte
<style>
  .scrollable-content {
    max-height: 60vh;
    overflow-y: auto;
  }
</style>

<Modal title="Termos de Uso" {open} on:close={close}>
  <div class="scrollable-content">
    <h3>1. Introdução</h3>
    <p>Lorem ipsum...</p>
    <!-- Muito conteúdo -->
  </div>
  
  <div class="modal-actions">
    <button on:click={close}>Fechar</button>
    <button on:click={accept}>Aceitar</button>
  </div>
</Modal>
```

## Acessibilidade

O componente implementa:

- `role="dialog"` - Semântica correta
- `aria-modal="true"` - Indica modal
- `aria-labelledby` - Conecta ao título (ID único por modal)
- `role="document"` no conteúdo - Permite modo de navegação no NVDA
- **Focus trap** - Foco não escapa
- **Restauração de foco** - Retorna ao elemento anterior
- **Escape para fechar** - Padrão esperado

### Comportamento com Leitores de Tela (NVDA/JAWS)

1. Ao abrir, o modal recebe foco e o título é anunciado
2. O `role="document"` no conteúdo permite navegação normal (setas, H para headings, etc.)
3. Se houver um campo de formulário, o foco move automaticamente para ele
4. Tab/Shift+Tab navegam entre elementos focáveis (circular)
5. Escape fecha e retorna ao elemento anterior

## Migração

Se você estava importando de `./components/Modal.svelte`, atualize para:

```javascript
// Antes
import Modal from './components/Modal.svelte';

// Depois
import { Modal } from './components/modal';
```

---

# ImageModal

Modal especializado para visualização de imagens em tela cheia.

## Uso

```svelte
<script>
  import { ImageModal } from './components/modal';
  
  let showImage = false;
  let imageSrc = '';
  let imageAlt = '';
  
  function openImage(src, alt) {
    imageSrc = src;
    imageAlt = alt;
    showImage = true;
  }
</script>

<button on:click={() => openImage('/photo.jpg', 'Foto de exemplo')}>
  Ver Imagem
</button>

<ImageModal 
  open={showImage}
  src={imageSrc}
  alt={imageAlt}
  on:close={() => showImage = false}
/>
```

## Props

| Prop | Tipo | Default | Descrição |
|------|------|---------|-----------|
| `open` | `boolean` | `false` | Controla visibilidade do modal |
| `src` | `string` | `''` | URL da imagem a ser exibida |
| `alt` | `string` | `'Imagem'` | Texto alternativo / caption da imagem |

## Eventos

| Evento | Payload | Descrição |
|--------|---------|-----------|
| `close` | - | Disparado quando modal deve fechar |

## Features Específicas

- **Layout otimizado**: Imagem centralizada com espaço para caption
- **Responsivo**: Adapta-se ao tamanho da tela (max 90vw x 80vh)
- **Caption**: Exibe texto alternativo como legenda abaixo da imagem
- **Fundo escuro**: Background com 92% de opacidade para destaque
- **Animação suave**: Fade-in ao abrir (respeita prefers-reduced-motion)

## Navegação

| Tecla | Ação |
|-------|------|
| `Escape` | Fecha o modal |
| `Tab` | Foca no botão fechar (único elemento focável) |
| Clique fora | Fecha o modal |

## Estilização

```css
/* Cores */
--color-accent    /* Outline do focus */
--border-radius   /* Arredondamento da imagem */
```

## Exemplo com Galeria

```svelte
<script>
  import { ImageModal } from './components/modal';
  
  let showImage = false;
  let currentImage = { src: '', alt: '' };
  
  const gallery = [
    { src: '/img1.jpg', alt: 'Paisagem' },
    { src: '/img2.jpg', alt: 'Retrato' },
    { src: '/img3.jpg', alt: 'Arquitetura' }
  ];
  
  function openImage(img) {
    currentImage = img;
    showImage = true;
  }
</script>

<div class="gallery">
  {#each gallery as img}
    <button on:click={() => openImage(img)}>
      <img src={img.src} alt={img.alt} />
    </button>
  {/each}
</div>

<ImageModal 
  open={showImage}
  src={currentImage.src}
  alt={currentImage.alt}
  on:close={() => showImage = false}
/>
```

