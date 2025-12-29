<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  
  export let title = '';
  export let open = false;
  export let autoFocus = true;  // Se false, o componente pai controla o foco
  
  const dispatch = createEventDispatcher();
  
  let modalElement;
  let contentElement;
  let previousActiveElement;
  let firstFocusable;
  let lastFocusable;

  $: if (open) {
    previousActiveElement = document.activeElement;
    // Aguarda o modal renderizar e foca (apenas se autoFocus estiver ativo)
    if (autoFocus) {
      setTimeout(() => {
        updateFocusableElements();
        // Foca no primeiro elemento do conteúdo, não no botão de fechar
        const initialFocus = findInitialFocus();
        initialFocus?.focus();
      }, 50);
    }
  }

  // Encontra o elemento ideal para foco inicial
  // Prioridade: 1) primeiro input/select/textarea no conteúdo, 2) primeiro botão no conteúdo, 3) qualquer focável
  function findInitialFocus() {
    if (!contentElement) return firstFocusable;
    
    // Primeiro, procura campos de formulário no conteúdo
    const formField = contentElement.querySelector(
      'input:not([type="hidden"]), select, textarea'
    );
    if (formField && formField.offsetParent !== null) {
      return formField;
    }
    
    // Depois, procura botões de ação (não o de fechar)
    const actionButton = contentElement.querySelector('button');
    if (actionButton && actionButton.offsetParent !== null) {
      return actionButton;
    }
    
    // Fallback para qualquer focável no conteúdo
    const anyFocusable = contentElement.querySelector(
      'button, [href], input, select, textarea, [tabindex="0"]'
    );
    if (anyFocusable && anyFocusable.offsetParent !== null) {
      return anyFocusable;
    }
    
    // Último recurso: primeiro focável do modal (pode ser o botão fechar)
    return firstFocusable;
  }

  function updateFocusableElements() {
    if (!modalElement) return;
    // Inclui elementos com tabindex="0" e listas
    const focusables = modalElement.querySelectorAll(
      'button, [href], input, select, textarea, [tabindex="0"], [role="listbox"], [role="option"][tabindex="0"]'
    );
    const focusableArray = Array.from(focusables).filter(el => {
      // Filtra elementos ocultos
      return el.offsetParent !== null;
    });
    firstFocusable = focusableArray[0];
    lastFocusable = focusableArray[focusableArray.length - 1];
  }

  function handleKeyDown(event) {
    if (event.key === 'Escape') {
      event.preventDefault();
      close();
      return;
    }

    if (event.key === 'Tab') {
      updateFocusableElements();
      
      // Verifica se estamos nos limites do modal
      const activeEl = document.activeElement;
      const isAtFirst = activeEl === firstFocusable || firstFocusable?.contains(activeEl);
      const isAtLast = activeEl === lastFocusable || lastFocusable?.contains(activeEl);
      
      if (event.shiftKey) {
        if (isAtFirst) {
          event.preventDefault();
          lastFocusable?.focus();
        }
      } else {
        if (isAtLast) {
          event.preventDefault();
          firstFocusable?.focus();
        }
      }
    }
  }

  function close() {
    dispatch('close');
    previousActiveElement?.focus();
  }

  function handleBackdropClick(event) {
    if (event.target === event.currentTarget) {
      close();
    }
  }
</script>

{#if open}
  <div 
    class="modal-backdrop" 
    on:click={handleBackdropClick}
    on:keydown={handleKeyDown}
  >
    <div 
      bind:this={modalElement}
      class="modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="modal-title"
    >
      <div class="modal-header">
        <button 
          class="modal-close"
          on:click={close}
          aria-label="Fechar (Esc)"
          title="Fechar (Esc)"
        >
          ✕
        </button>
        <h2 id="modal-title">{title}</h2>
      </div>
      <div class="modal-content" bind:this={contentElement}>
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
    z-index: 1000;
  }

  .modal {
    background-color: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius-lg);
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
    padding: var(--spacing-md) var(--spacing-lg);
    border-bottom: 1px solid var(--color-border);
  }

  .modal-header h2 {
    margin: 0;
    font-size: var(--font-size-xl);
    color: var(--color-text-primary);
    flex: 1;
  }

  .modal-close {
    background: none;
    border: none;
    color: var(--color-text-muted);
    font-size: var(--font-size-xl);
    cursor: pointer;
    padding: var(--spacing-xs);
    min-width: 44px;
    min-height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--border-radius);
  }

  .modal-close:hover {
    background-color: var(--color-bg-tertiary);
    color: var(--color-text-primary);
  }

  .modal-close:focus-visible {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
  }

  .modal-content {
    padding: var(--spacing-lg);
  }
</style>

