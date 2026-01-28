<script>
  import { onDestroy, createEventDispatcher, tick } from 'svelte';
  
  /**
   * ConfigModal - Modal de configurações com Aplicar/Salvar/Cancelar
   * 
   * Diferente do Modal padrão:
   * - Não fecha ao clicar no backdrop
   * - Não fecha com Escape (pode ter alterações não salvas)
   * - Tem botões Aplicar, Salvar e Cancelar no footer
   * - Event-driven: dispara eventos para o componente pai gerenciar o estado
   * 
   * Eventos:
   * - apply: Aplicar configurações (salva mas mantém aberto)
   * - save: Salvar e fechar
   * - cancel: Cancelar sem salvar (fecha)
   */
  
  export let title = '';
  export let open = false;
  export let autoFocus = true;
  
  /** Largura máxima do modal */
  export let maxWidth = '700px';
  
  /** Altura máxima do modal (padrão: 80vh) */
  export let maxHeight = '80vh';
  
  /** Mostra o botão Aplicar */
  export let showApply = true;
  
  /** Labels dos botões (i18n) */
  export let applyLabel = 'Aplicar';
  export let saveLabel = 'Salvar';
  export let cancelLabel = 'Cancelar';
  
  /** Estado de loading nos botões */
  export let applying = false;
  export let saving = false;
  
  /** Se há alterações não salvas (habilita os botões) */
  export let hasChanges = false;
  
  const dispatch = createEventDispatcher();
  
  let modalElement;
  let contentElement;
  let previousActiveElement;
  let firstFocusable;
  let lastFocusable;
  
  // ID único para aria-labelledby
  const modalId = `config-modal-${Math.random().toString(36).substr(2, 9)}`;
  const modalTitleId = `${modalId}-title`;
  
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
    
    // Fallback: primeiro focável do modal
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
    // Escape ainda deve disparar cancel (mas não forçar fechamento)
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      handleCancel();
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

  function restoreFocus() {
    tick().then(() => {
      previousActiveElement?.focus();
    });
  }

  function handleApply() {
    dispatch('apply');
  }

  function handleSave() {
    dispatch('save');
  }

  function handleCancel() {
    dispatch('cancel');
    restoreFocus();
  }
  
  /** Método público para fechar o modal e restaurar foco */
  export function close() {
    restoreFocus();
  }
</script>

{#if open}
  <div 
    use:portal
    class="config-modal-backdrop" 
    on:keydown={handleKeyDown}
    role="presentation"
  >
    <div 
      bind:this={modalElement}
      class="config-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby={modalTitleId}
      style="--modal-max-width: {maxWidth}; --modal-max-height: {maxHeight};"
    >
      <div class="config-modal-header">
        <h1 id={modalTitleId}>{title}</h1>
      </div>
      
      <div 
        class="config-modal-content" 
        bind:this={contentElement}
      >
        <slot></slot>
      </div>
      
      <div class="config-modal-footer">
        <div class="config-modal-actions">
          <button 
            type="button"
            class="btn btn-cancel"
            on:click={handleCancel}
            disabled={applying || saving}
          >
            {cancelLabel}
          </button>
          
          <div class="config-modal-actions-right">
            {#if showApply}
              <button 
                type="button"
                class="btn btn-apply"
                on:click={handleApply}
                disabled={!hasChanges || applying || saving}
                aria-busy={applying}
              >
                {#if applying}
                  <span class="loading-spinner" aria-hidden="true"></span>
                {/if}
                {applyLabel}
              </button>
            {/if}
            
            <button 
              type="button"
              class="btn btn-save"
              on:click={handleSave}
              disabled={!hasChanges || applying || saving}
              aria-busy={saving}
            >
              {#if saving}
                <span class="loading-spinner" aria-hidden="true"></span>
              {/if}
              {saveLabel}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  .config-modal-backdrop {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background-color: rgba(0, 0, 0, 0.75);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 10000;
  }

  .config-modal {
    background-color: var(--color-bg-secondary, #1e1e1e);
    border: 1px solid var(--color-border, #3d3d3d);
    border-radius: var(--border-radius-lg, 8px);
    max-width: var(--modal-max-width, 700px);
    width: 95%;
    max-height: var(--modal-max-height, 80vh);
    display: flex;
    flex-direction: column;
    box-shadow: 0 12px 48px rgba(0, 0, 0, 0.6);
  }

  .config-modal-header {
    display: flex;
    align-items: center;
    padding: var(--spacing-md, 12px) var(--spacing-lg, 16px);
    border-bottom: 1px solid var(--color-border, #3d3d3d);
    flex-shrink: 0;
  }

  .config-modal-header h1 {
    margin: 0;
    font-size: var(--font-size-xl, 1.25rem);
    font-weight: 600;
    color: var(--color-text-primary, #e6e6e6);
    flex: 1;
  }

  .config-modal-content {
    flex: 1;
    overflow: auto;
    padding: 0;
    min-height: 200px;
  }

  .config-modal-footer {
    padding: var(--spacing-md, 12px) var(--spacing-lg, 16px);
    border-top: 1px solid var(--color-border, #3d3d3d);
    flex-shrink: 0;
  }

  .config-modal-actions {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--spacing-md, 12px);
  }

  .config-modal-actions-right {
    display: flex;
    gap: var(--spacing-sm, 8px);
  }

  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--spacing-xs, 4px);
    padding: var(--spacing-sm, 8px) var(--spacing-lg, 16px);
    min-height: 44px;
    min-width: 100px;
    font-size: var(--font-size-base, 1rem);
    font-weight: 500;
    font-family: inherit;
    border-radius: var(--border-radius, 4px);
    cursor: pointer;
    transition: all var(--transition-fast, 150ms ease);
  }

  .btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .btn:focus-visible {
    outline: 2px solid var(--color-accent, #0d6efd);
    outline-offset: 2px;
  }

  .btn-cancel {
    background: transparent;
    border: 1px solid var(--color-border, #3d3d3d);
    color: var(--color-text-secondary, #b0b0b0);
  }

  .btn-cancel:hover:not(:disabled) {
    background-color: var(--color-bg-tertiary, #2d2d2d);
    border-color: var(--color-text-muted, #888);
  }

  .btn-apply {
    background: var(--color-bg-tertiary, #2d2d2d);
    border: 1px solid var(--color-border, #3d3d3d);
    color: var(--color-text-primary, #e6e6e6);
  }

  .btn-apply:hover:not(:disabled) {
    background-color: var(--color-bg-hover, #3a3a3a);
    border-color: var(--color-accent, #0d6efd);
  }

  .btn-save {
    background: var(--color-accent, #0d6efd);
    border: 1px solid var(--color-accent, #0d6efd);
    color: white;
  }

  .btn-save:hover:not(:disabled) {
    background-color: var(--color-accent-hover, #0b5ed7);
    border-color: var(--color-accent-hover, #0b5ed7);
  }

  .loading-spinner {
    display: inline-block;
    width: 14px;
    height: 14px;
    border: 2px solid currentColor;
    border-top-color: transparent;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  /* Responsive */
  @media (max-width: 480px) {
    .config-modal {
      width: 100%;
      max-width: 100%;
      max-height: 100%;
      border-radius: 0;
    }
    
    .config-modal-actions {
      flex-direction: column;
    }
    
    .config-modal-actions-right {
      width: 100%;
    }
    
    .btn {
      flex: 1;
    }
  }
</style>

