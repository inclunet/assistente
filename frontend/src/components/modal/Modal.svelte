<script>
  import { onDestroy, createEventDispatcher, tick } from 'svelte';
  
  export let title = '';
  export let open = false;
  export let autoFocus = true;
  
  const dispatch = createEventDispatcher();
  
  let modalElement;
  let contentElement;
  let previousActiveElement;
  let firstFocusable;
  let lastFocusable;
  
  // ID único para aria-labelledby
  const modalId = `modal-${Math.random().toString(36).substr(2, 9)}`;
  const modalTitleId = `${modalId}-title`;
  
  // Action para criar portal no body
  function portal(node) {
    // Move o elemento para o body (fora de qualquer role="application")
    document.body.appendChild(node);
    
    return {
      destroy() {
        // Remove do body quando destruído
        if (node.parentNode === document.body) {
          document.body.removeChild(node);
        }
      }
    };
  }
  
  // Quando abre
  $: if (open) {
    previousActiveElement = document.activeElement;
    tick().then(() => {
      if (autoFocus) {
        handleOpen();
      }
    });
  }
  
  async function handleOpen() {
    await tick();
    updateFocusableElements();
    
    // Foca no primeiro elemento focável do conteúdo
    const initialFocus = findInitialFocus();
    if (initialFocus) {
      initialFocus.focus();
    }
  }
  
  function findInitialFocus() {
    if (!contentElement) return firstFocusable;
    
    // Primeiro: campos de formulário
    const formField = contentElement.querySelector(
      'input:not([type="hidden"]):not([disabled]), select:not([disabled]), textarea:not([disabled])'
    );
    if (formField && isVisible(formField)) {
      return formField;
    }
    
    // Segundo: botões de ação no conteúdo
    const actionButton = contentElement.querySelector('button:not([disabled])');
    if (actionButton && isVisible(actionButton)) {
      return actionButton;
    }
    
    // Terceiro: qualquer focável
    const anyFocusable = contentElement.querySelector(
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex="0"]'
    );
    if (anyFocusable && isVisible(anyFocusable)) {
      return anyFocusable;
    }
    
    // Fallback: botão fechar
    return firstFocusable;
  }
  
  function isVisible(el) {
    return el.offsetParent !== null;
  }

  function updateFocusableElements() {
    if (!modalElement) return;
    const focusables = modalElement.querySelectorAll(
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex="0"]'
    );
    const focusableArray = Array.from(focusables).filter(isVisible);
    firstFocusable = focusableArray[0];
    lastFocusable = focusableArray[focusableArray.length - 1];
  }

  function handleKeyDown(event) {
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      close();
      return;
    }

    if (event.key === 'Tab') {
      updateFocusableElements();
      
      const activeEl = document.activeElement;
      
      if (event.shiftKey) {
        if (activeEl === firstFocusable) {
          event.preventDefault();
          lastFocusable?.focus();
        }
      } else {
        if (activeEl === lastFocusable) {
          event.preventDefault();
          firstFocusable?.focus();
        }
      }
    }
  }

  function close() {
    dispatch('close');
    tick().then(() => {
      previousActiveElement?.focus();
    });
  }

  function handleBackdropClick(event) {
    if (event.target === event.currentTarget) {
      close();
    }
  }
</script>

{#if open}
  <!-- Portal: renderiza diretamente no body, fora de role="application" -->
  <div 
    use:portal
    class="modal-backdrop" 
    on:click={handleBackdropClick}
    on:keydown={handleKeyDown}
    role="presentation"
  >
    <div 
      bind:this={modalElement}
      class="modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby={modalTitleId}
    >
      <div class="modal-header">
        <button 
          type="button"
          class="modal-close"
          on:click={close}
          aria-label="Fechar"
        >
          <span aria-hidden="true">✕</span>
        </button>
        <h1 id={modalTitleId}>{title}</h1>
      </div>
      <div 
        class="modal-content" 
        bind:this={contentElement}
      >
        <slot></slot>
      </div>
    </div>
  </div>
{/if}

<style>
  .modal-backdrop {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background-color: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 10000;
  }

  .modal {
    background-color: var(--color-bg-secondary, #1e1e1e);
    border: 1px solid var(--color-border, #3d3d3d);
    border-radius: var(--border-radius-lg, 8px);
    max-width: 500px;
    width: 90%;
    max-height: 90vh;
    overflow: auto;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
  }

  .modal-header {
    display: flex;
    flex-direction: row-reverse;
    align-items: center;
    justify-content: space-between;
    padding: var(--spacing-md, 12px) var(--spacing-lg, 16px);
    border-bottom: 1px solid var(--color-border, #3d3d3d);
  }

  .modal-header h1 {
    margin: 0;
    font-size: var(--font-size-xl, 1.25rem);
    font-weight: 600;
    color: var(--color-text-primary, #e6e6e6);
    flex: 1;
  }

  .modal-close {
    background: none;
    border: none;
    color: var(--color-text-muted, #888);
    font-size: var(--font-size-xl, 1.25rem);
    cursor: pointer;
    padding: var(--spacing-xs, 4px);
    min-width: 44px;
    min-height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--border-radius, 4px);
    margin-left: var(--spacing-sm, 8px);
  }

  .modal-close:hover {
    background-color: var(--color-bg-tertiary, #2d2d2d);
    color: var(--color-text-primary, #e6e6e6);
  }

  .modal-close:focus-visible {
    outline: 2px solid var(--color-accent, #0d6efd);
    outline-offset: 2px;
  }

  .modal-content {
    padding: var(--spacing-lg, 16px);
  }
</style>
