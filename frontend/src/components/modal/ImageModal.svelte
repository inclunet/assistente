<script>
  /**
   * ImageModal - Modal para visualização de imagens em tela cheia
   * 
   * Features:
   * - Portal: renderiza no body para evitar problemas de z-index e ARIA
   * - Focus trap: Tab/Shift+Tab cicla entre elementos focáveis
   * - Acessibilidade: role="dialog", aria-modal, aria-label
   * - Suporte a reduced motion
   * - Fecha com Escape ou clique no backdrop
   * 
   * @example
   * <ImageModal 
   *   open={showModal} 
   *   src={imageSrc} 
   *   alt="Descrição da imagem"
   *   on:close={() => showModal = false} 
   * />
   */
  import { createEventDispatcher, tick } from 'svelte';

  /** @type {boolean} Se o modal está aberto */
  export let open = false;
  
  /** @type {string} URL da imagem */
  export let src = '';
  
  /** @type {string} Texto alternativo / caption da imagem */
  export let alt = 'Imagem';

  const dispatch = createEventDispatcher();

  let dialogElement;
  let closeButton;
  let previousFocusElement = null;

  // Action para criar portal no body
  function portal(node) {
    document.body.appendChild(node);
    return {
      destroy() {
        if (node.parentNode === document.body) {
          document.body.removeChild(node);
        }
      }
    };
  }

  // Quando abre, salva o foco atual e foca no botão fechar
  $: if (open) {
    previousFocusElement = document.activeElement;
    tick().then(() => closeButton?.focus());
  }

  function close() {
    dispatch('close');
    tick().then(() => {
      if (previousFocusElement && typeof previousFocusElement.focus === 'function') {
        previousFocusElement.focus();
      }
    });
  }

  function handleKeyDown(event) {
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      close();
      return;
    }

    // Focus trap simples (só tem o botão fechar como focável)
    if (event.key === 'Tab') {
      event.preventDefault();
      closeButton?.focus();
    }
  }

  function handleBackdropClick(event) {
    if (event.target === event.currentTarget) {
      close();
    }
  }
</script>

{#if open}
  <div 
    use:portal
    class="image-modal-backdrop"
    role="dialog"
    aria-modal="true"
    aria-label={alt}
    bind:this={dialogElement}
    tabindex="-1"
    on:click={handleBackdropClick}
    on:keydown={handleKeyDown}
  >
    <div class="image-modal-content">
      <button
        bind:this={closeButton}
        type="button"
        class="image-modal-close"
        aria-label="Fechar visualização de imagem"
        on:click={close}
      >
        <span aria-hidden="true">✕</span>
      </button>
      
      <img 
        {src} 
        {alt}
        class="image-modal-img"
      />
      
      {#if alt && alt !== 'Imagem'}
        <p class="image-modal-caption">{alt}</p>
      {/if}
    </div>
  </div>
{/if}

<style>
  .image-modal-backdrop {
    position: fixed;
    inset: 0;
    z-index: 10000;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.92);
    padding: 1rem;
    animation: fadeIn 0.15s ease-out;
  }
  
  @keyframes fadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }
  
  .image-modal-content {
    position: relative;
    max-width: 90vw;
    max-height: 90vh;
    display: flex;
    flex-direction: column;
    align-items: center;
  }
  
  .image-modal-close {
    position: absolute;
    top: -3rem;
    right: 0;
    padding: 0.5rem 1rem;
    min-width: 44px;
    min-height: 44px;
    background: rgba(255, 255, 255, 0.1);
    border: none;
    border-radius: var(--border-radius, 4px);
    color: white;
    font-size: 1.25rem;
    cursor: pointer;
    transition: background-color 0.15s;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  
  .image-modal-close:hover {
    background: rgba(255, 255, 255, 0.2);
  }
  
  .image-modal-close:focus-visible {
    outline: 2px solid var(--color-accent, #3b82f6);
    outline-offset: 2px;
  }
  
  .image-modal-img {
    max-width: 100%;
    max-height: 80vh;
    object-fit: contain;
    border-radius: var(--border-radius, 4px);
  }
  
  .image-modal-caption {
    margin-top: 1rem;
    color: rgba(255, 255, 255, 0.8);
    font-size: 0.875rem;
    text-align: center;
    max-width: 60ch;
  }
  
  @media (prefers-reduced-motion: reduce) {
    .image-modal-backdrop {
      animation: none;
    }
  }
</style>



