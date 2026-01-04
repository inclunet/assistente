<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { GetConversations } from '../../../wailsjs/go/main/App.js';
  import { Combobox as ComboboxPicker } from '../combobox';

  const dispatch = createEventDispatcher();

  // Props
  export let value = null; // ID da conversa selecionada
  export let label = 'Histórico';
  export let icon = '📂';
  export let placeholder = 'Buscar conversa...';
  export let disabled = false;
  export let maxWidth = '220px';
  
  // Variante: 'toolbar' (compacto) ou 'form' (com label e help)
  export let variant = 'toolbar';
  export let helpText = '';

  // Estado
  let conversations = [];
  let loading = true;
  let error = '';
  let pickerComponent;

  // Formata data para exibição
  function formatDate(dateStr) {
    if (!dateStr) return '';
    try {
      const date = new Date(dateStr);
      const now = new Date();
      const diffMs = now - date;
      const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));
      
      if (diffDays === 0) {
        return 'Hoje ' + date.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
      } else if (diffDays === 1) {
        return 'Ontem ' + date.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
      } else if (diffDays < 7) {
        return date.toLocaleDateString('pt-BR', { weekday: 'short' }) + ' ' + 
               date.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
      } else {
        return date.toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit', year: '2-digit' });
      }
    } catch (e) {
      return '';
    }
  }

  // Trunca título se muito longo
  function truncateTitle(title, maxLen = 40) {
    if (!title) return 'Sem título';
    return title.length > maxLen ? title.substring(0, maxLen - 3) + '...' : title;
  }

  // Items formatados para o ComboboxPicker
  $: items = conversations.map(c => ({ 
    value: c.id, 
    label: truncateTitle(c.title),
    sublabel: formatDate(c.updated_at || c.created_at),
    data: c
  }));

  onMount(async () => {
    await loadConversations();
  });

  async function loadConversations() {
    loading = true;
    error = '';
    try {
      conversations = await GetConversations() || [];
    } catch (e) {
      error = 'Erro ao carregar histórico';
      console.error('ConversationPicker: erro ao carregar conversas', e);
    } finally {
      loading = false;
    }
  }

  function handleSelect(event) {
    const selectedItem = event.detail.item;
    value = event.detail.value;
    dispatch('select', { 
      conversationId: event.detail.value, 
      conversation: selectedItem?.data || null 
    });
    dispatch('change', value);
  }

  function handleAnnounce(event) {
    dispatch('announce', event.detail);
  }

  // Expõe métodos do picker
  export function open() {
    // Recarrega ao abrir para ter dados atualizados
    loadConversations().then(() => {
      pickerComponent?.open();
    });
  }

  export function close() {
    pickerComponent?.close();
  }

  export function reload() {
    loadConversations();
  }
</script>

{#if variant === 'form'}
  <div class="conversation-picker-form">
    <label for="conversation-picker-{label}">{label}</label>
    
    {#if loading}
      <div class="loading-state">
        <span class="loading-spinner"></span>
        Carregando histórico...
      </div>
    {:else if error}
      <div class="error-state">
        <span>{error}</span>
        <button type="button" class="retry-btn" on:click={loadConversations}>
          🔄 Tentar novamente
        </button>
      </div>
    {:else if conversations.length === 0}
      <div class="empty-state">
        Nenhuma conversa encontrada
      </div>
    {:else}
      <ComboboxPicker
        bind:this={pickerComponent}
        {icon}
        label={value ? truncateTitle(conversations.find(c => c.id === value)?.title) : 'Selecionar conversa'}
        {items}
        bind:selected={value}
        {placeholder}
        {disabled}
        maxWidth="100%"
        on:select={handleSelect}
        on:announce={handleAnnounce}
      />
    {/if}
    
    {#if helpText}
      <small class="help-text">{helpText}</small>
    {/if}
  </div>
{:else}
  <!-- Variante toolbar (compacta) -->
  {#if loading}
    <div class="loading-status">
      <span class="loading-spinner"></span>
      Carregando...
    </div>
  {:else if error}
    <button class="toolbar-btn error-btn" on:click={loadConversations}>
      🔄 Recarregar
    </button>
  {:else}
    <ComboboxPicker
      bind:this={pickerComponent}
      {icon}
      {label}
      {items}
      bind:selected={value}
      {placeholder}
      {disabled}
      {maxWidth}
      on:select={handleSelect}
      on:announce={handleAnnounce}
    />
  {/if}
{/if}

<style>
  .conversation-picker-form {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .conversation-picker-form label {
    font-weight: 500;
    color: var(--color-text-primary, #e0e0e0);
  }

  .loading-state,
  .error-state,
  .empty-state {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm);
    color: var(--color-text-muted);
    font-size: var(--font-size-sm);
  }

  .error-state {
    color: var(--color-error, #f85149);
  }

  .empty-state {
    font-style: italic;
  }

  .retry-btn {
    padding: var(--spacing-xs) var(--spacing-sm);
    background: transparent;
    border: 1px solid currentColor;
    border-radius: var(--border-radius);
    color: inherit;
    cursor: pointer;
    font-size: var(--font-size-xs);
  }

  .retry-btn:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  .help-text {
    color: var(--color-text-muted, #888);
    font-size: 0.85rem;
  }

  .loading-status {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm);
    color: var(--color-text-muted);
    font-size: var(--font-size-sm);
  }

  .loading-spinner {
    display: inline-block;
    width: 14px;
    height: 14px;
    border: 2px solid var(--color-border, #444);
    border-top-color: var(--color-accent, #58a6ff);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .error-btn {
    display: flex;
    align-items: center;
    gap: var(--spacing-xs);
    padding: var(--spacing-xs) var(--spacing-sm);
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-error, #f85149);
    border-radius: var(--border-radius);
    color: var(--color-error, #f85149);
    font-size: var(--font-size-sm);
    cursor: pointer;
  }

  .error-btn:hover {
    background: rgba(248, 81, 73, 0.1);
  }

  /* Estilo para o ComboboxPicker quando em modo form */
  .conversation-picker-form :global(.combobox-picker) {
    width: 100%;
  }

  .conversation-picker-form :global(.picker-button) {
    width: 100%;
    max-width: none;
  }
</style>

