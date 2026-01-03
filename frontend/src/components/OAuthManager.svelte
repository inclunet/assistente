<script>
  import { onMount } from 'svelte';
  import { 
    GetOAuthProviders, 
    GetOAuthConnections, 
    StartOAuthFlow, 
    CompleteOAuthFlow,
    DisconnectOAuth,
    RefreshOAuthConnection
  } from '../../wailsjs/go/main/App.js';
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime.js';
  import { Modal } from './modal';
  
  export let label = 'Conexões OAuth';
  
  let providers = [];
  let connections = [];
  let loading = true;
  let error = '';
  
  // Estado do fluxo de autorização
  let authorizing = false;
  let authorizingProvider = null;
  let authorizingScopes = [];
  let showScopesModal = false;
  let selectedProvider = null;
  
  // Scopes disponíveis por provider
  const providerServices = {
    google: [
      { id: 'gmail', name: 'Gmail', scopes: ['https://www.googleapis.com/auth/gmail.readonly', 'https://www.googleapis.com/auth/gmail.send'] },
      { id: 'calendar', name: 'Google Calendar', scopes: ['https://www.googleapis.com/auth/calendar', 'https://www.googleapis.com/auth/calendar.events'] },
      { id: 'drive', name: 'Google Drive', scopes: ['https://www.googleapis.com/auth/drive', 'https://www.googleapis.com/auth/drive.file'] },
      { id: 'sheets', name: 'Google Sheets', scopes: ['https://www.googleapis.com/auth/spreadsheets'] },
      { id: 'docs', name: 'Google Docs', scopes: ['https://www.googleapis.com/auth/documents'] },
    ],
    microsoft: [
      { id: 'outlook', name: 'Outlook Mail', scopes: ['Mail.Read', 'Mail.Send'] },
      { id: 'calendar', name: 'Outlook Calendar', scopes: ['Calendars.ReadWrite'] },
      { id: 'onedrive', name: 'OneDrive', scopes: ['Files.ReadWrite.All'] },
      { id: 'teams', name: 'Teams', scopes: ['Chat.Read', 'Chat.ReadWrite'] },
    ],
    github: [
      { id: 'repos', name: 'Repositórios', scopes: ['repo', 'public_repo'] },
      { id: 'gists', name: 'Gists', scopes: ['gist'] },
      { id: 'actions', name: 'Actions', scopes: ['workflow'] },
    ],
    slack: [
      { id: 'channels', name: 'Canais', scopes: ['channels:read', 'channels:write'] },
      { id: 'messages', name: 'Mensagens', scopes: ['chat:write', 'chat:write.public'] },
    ],
  };
  
  let selectedServices = {};
  
  onMount(async () => {
    await loadData();
  });
  
  async function loadData() {
    loading = true;
    error = '';
    
    try {
      providers = await GetOAuthProviders() || [];
      connections = await GetOAuthConnections() || [];
    } catch (err) {
      error = 'Erro ao carregar: ' + (err.message || err);
    } finally {
      loading = false;
    }
  }
  
  function openScopesModal(provider) {
    selectedProvider = provider;
    selectedServices = {};
    showScopesModal = true;
  }
  
  function toggleService(serviceId) {
    selectedServices[serviceId] = !selectedServices[serviceId];
    selectedServices = {...selectedServices}; // Trigger reactivity
  }
  
  async function startAuthorization() {
    if (!selectedProvider) return;
    
    // Coleta scopes selecionados
    const services = providerServices[selectedProvider.id] || [];
    const scopes = [];
    for (const service of services) {
      if (selectedServices[service.id]) {
        scopes.push(...service.scopes);
      }
    }
    
    showScopesModal = false;
    authorizing = true;
    authorizingProvider = selectedProvider;
    error = '';
    
    try {
      // Inicia o fluxo OAuth
      const authURL = await StartOAuthFlow(selectedProvider.id, scopes);
      
      // Abre o navegador
      BrowserOpenURL(authURL);
      
      // Aguarda o callback (5 minutos de timeout)
      const connection = await CompleteOAuthFlow(selectedProvider.id, 300);
      
      // Sucesso! Recarrega as conexões
      await loadData();
    } catch (err) {
      error = 'Erro na autorização: ' + (err.message || err);
    } finally {
      authorizing = false;
      authorizingProvider = null;
    }
  }
  
  async function disconnect(connectionId) {
    if (!confirm('Deseja desconectar esta conta?')) return;
    
    try {
      await DisconnectOAuth(connectionId);
      await loadData();
    } catch (err) {
      error = 'Erro ao desconectar: ' + (err.message || err);
    }
  }
  
  async function refresh(connectionId) {
    try {
      await RefreshOAuthConnection(connectionId);
      await loadData();
    } catch (err) {
      error = 'Erro ao renovar token: ' + (err.message || err);
    }
  }
  
  function getConnectionsForProvider(providerId) {
    return connections.filter(c => c.provider_id === providerId);
  }
</script>

<div class="oauth-manager">
  <header class="oauth-header">
    <div>
      <h2>{label}</h2>
      <p class="subtitle">Conecte suas contas para automações avançadas</p>
    </div>
  </header>
  
  {#if error}
    <div class="error-message">{error}</div>
  {/if}
  
  {#if loading}
    <div class="loading">
      <span class="spinner"></span>
      Carregando...
    </div>
  {:else}
    <!-- Fluxo de autorização em andamento -->
    {#if authorizing}
      <div class="authorizing-overlay">
        <div class="authorizing-card">
          <div class="provider-icon large">{authorizingProvider?.icon}</div>
          <h3>Autorizando {authorizingProvider?.name}</h3>
          <p>Complete a autorização no navegador...</p>
          <div class="spinner large"></div>
          <p class="hint">A janela do navegador foi aberta. Após autorizar, você será redirecionado automaticamente.</p>
        </div>
      </div>
    {/if}
    
    <!-- Lista de Providers -->
    <div class="providers-grid">
      {#each providers as provider}
        <div class="provider-card" class:disabled={!provider.is_configured}>
          <div class="provider-header">
            <span class="provider-icon">{provider.icon}</span>
            <span class="provider-name">{provider.name}</span>
            {#if !provider.is_configured}
              <span class="badge warning">Não configurado</span>
            {/if}
          </div>
          
          <!-- Conexões existentes -->
          {#each getConnectionsForProvider(provider.id) as conn}
            <div class="connection-item" class:expired={conn.is_expired}>
              <div class="connection-info">
                <span class="user-email">{conn.user_email || conn.user_name || 'Conta conectada'}</span>
                {#if conn.is_expired}
                  <span class="badge error">Expirado</span>
                {:else}
                  <span class="badge success">Ativo</span>
                {/if}
              </div>
              <div class="connection-meta">
                <span>Expira: {conn.expires_at}</span>
              </div>
              <div class="connection-actions">
                {#if conn.is_expired}
                  <button class="btn-small" on:click={() => refresh(conn.id)}>🔄 Renovar</button>
                {/if}
                <button class="btn-small btn-danger" on:click={() => disconnect(conn.id)}>Desconectar</button>
              </div>
            </div>
          {/each}
          
          <!-- Botão de conectar -->
          {#if provider.is_configured}
            <button 
              class="btn-connect" 
              on:click={() => openScopesModal(provider)}
              disabled={authorizing}
            >
              + Conectar {provider.name}
            </button>
          {:else}
            <div class="config-hint">
              Configure as variáveis de ambiente:<br>
              <code>{provider.id.toUpperCase()}_CLIENT_ID</code><br>
              <code>{provider.id.toUpperCase()}_CLIENT_SECRET</code>
            </div>
          {/if}
        </div>
      {/each}
    </div>
    
    {#if providers.length === 0}
      <div class="empty">
        <p>Nenhum provider OAuth disponível.</p>
      </div>
    {/if}
  {/if}
</div>

<!-- Modal de seleção de scopes -->
<Modal title="Selecionar Permissões" open={showScopesModal} on:close={() => showScopesModal = false}>
  <div class="scopes-modal">
    {#if selectedProvider}
      <div class="provider-info">
        <span class="provider-icon large">{selectedProvider.icon}</span>
        <h3>{selectedProvider.name}</h3>
      </div>
      
      <p class="scopes-intro">Selecione os serviços que deseja acessar:</p>
      
      {#if providerServices[selectedProvider.id]}
        <div class="services-list">
          {#each providerServices[selectedProvider.id] as service}
            <label class="service-item" class:selected={selectedServices[service.id]}>
              <input 
                type="checkbox" 
                checked={selectedServices[service.id]}
                on:change={() => toggleService(service.id)}
              />
              <span class="service-name">{service.name}</span>
            </label>
          {/each}
        </div>
      {:else}
        <p class="no-services">Permissões padrão serão solicitadas.</p>
      {/if}
      
      <div class="modal-actions">
        <button class="btn-secondary" on:click={() => showScopesModal = false}>Cancelar</button>
        <button class="btn-primary" on:click={startAuthorization}>
          🔐 Autorizar
        </button>
      </div>
    {/if}
  </div>
</Modal>

<style>
  .oauth-manager {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-lg);
  }
  
  .oauth-header h2 {
    margin: 0;
    font-size: var(--font-size-xl);
  }
  
  .subtitle {
    margin: var(--spacing-xs) 0 0;
    color: var(--color-text-muted);
  }
  
  .error-message {
    padding: var(--spacing-sm);
    background: rgba(248, 81, 73, 0.1);
    border: 1px solid var(--color-error);
    border-radius: var(--border-radius);
    color: var(--color-error);
  }
  
  .loading {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--spacing-sm);
    padding: var(--spacing-xl);
    color: var(--color-text-muted);
  }
  
  .spinner {
    width: 16px;
    height: 16px;
    border: 2px solid currentColor;
    border-right-color: transparent;
    border-radius: 50%;
    animation: spin 0.75s linear infinite;
  }
  
  .spinner.large {
    width: 32px;
    height: 32px;
    border-width: 3px;
  }
  
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
  
  /* Providers Grid */
  .providers-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: var(--spacing-md);
  }
  
  .provider-card {
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius-lg, 12px);
    padding: var(--spacing-md);
    background: var(--color-bg-secondary);
  }
  
  .provider-card.disabled {
    opacity: 0.6;
  }
  
  .provider-header {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    margin-bottom: var(--spacing-md);
  }
  
  .provider-icon {
    font-size: 24px;
  }
  
  .provider-icon.large {
    font-size: 48px;
  }
  
  .provider-name {
    font-weight: 600;
    font-size: var(--font-size-md);
  }
  
  .badge {
    padding: 2px 8px;
    border-radius: 12px;
    font-size: var(--font-size-xs);
    font-weight: 500;
  }
  
  .badge.warning {
    background: rgba(250, 204, 21, 0.2);
    color: #ca8a04;
  }
  
  .badge.error {
    background: rgba(248, 81, 73, 0.2);
    color: var(--color-error);
  }
  
  .badge.success {
    background: rgba(74, 222, 128, 0.2);
    color: #16a34a;
  }
  
  /* Connections */
  .connection-item {
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    padding: var(--spacing-sm);
    margin-bottom: var(--spacing-sm);
    background: var(--color-bg-tertiary);
  }
  
  .connection-item.expired {
    border-color: var(--color-error);
  }
  
  .connection-info {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    margin-bottom: var(--spacing-xs);
  }
  
  .user-email {
    font-weight: 500;
  }
  
  .connection-meta {
    font-size: var(--font-size-xs);
    color: var(--color-text-muted);
    margin-bottom: var(--spacing-xs);
  }
  
  .connection-actions {
    display: flex;
    gap: var(--spacing-xs);
  }
  
  .btn-small {
    padding: 4px 12px;
    font-size: var(--font-size-xs);
    border-radius: var(--border-radius);
    cursor: pointer;
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    color: var(--color-text-primary);
  }
  
  .btn-small.btn-danger {
    color: var(--color-error);
    border-color: var(--color-error);
  }
  
  .btn-connect {
    width: 100%;
    padding: var(--spacing-sm);
    background: var(--color-accent);
    color: white;
    border: none;
    border-radius: var(--border-radius);
    cursor: pointer;
    font-weight: 500;
    transition: opacity 0.2s;
  }
  
  .btn-connect:hover:not(:disabled) {
    opacity: 0.9;
  }
  
  .btn-connect:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  .config-hint {
    font-size: var(--font-size-xs);
    color: var(--color-text-muted);
    background: var(--color-bg-tertiary);
    padding: var(--spacing-sm);
    border-radius: var(--border-radius);
  }
  
  .config-hint code {
    display: block;
    font-family: 'Fira Code', monospace;
    font-size: var(--font-size-xs);
    margin-top: var(--spacing-xs);
  }
  
  /* Authorizing overlay */
  .authorizing-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }
  
  .authorizing-card {
    background: var(--color-bg-secondary);
    border-radius: var(--border-radius-lg, 16px);
    padding: var(--spacing-xl);
    text-align: center;
    max-width: 400px;
  }
  
  .authorizing-card h3 {
    margin: var(--spacing-md) 0;
  }
  
  .authorizing-card .hint {
    font-size: var(--font-size-sm);
    color: var(--color-text-muted);
    margin-top: var(--spacing-md);
  }
  
  /* Scopes Modal */
  .scopes-modal {
    min-width: 350px;
  }
  
  .provider-info {
    text-align: center;
    margin-bottom: var(--spacing-md);
  }
  
  .provider-info h3 {
    margin: var(--spacing-sm) 0 0;
  }
  
  .scopes-intro {
    color: var(--color-text-secondary);
    margin-bottom: var(--spacing-md);
  }
  
  .services-list {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-xs);
    margin-bottom: var(--spacing-lg);
  }
  
  .service-item {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    cursor: pointer;
    transition: all 0.2s;
  }
  
  .service-item:hover {
    border-color: var(--color-accent);
  }
  
  .service-item.selected {
    background: rgba(var(--color-accent-rgb), 0.1);
    border-color: var(--color-accent);
  }
  
  .service-name {
    font-weight: 500;
  }
  
  .no-services {
    color: var(--color-text-muted);
    text-align: center;
    padding: var(--spacing-md);
  }
  
  .modal-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--spacing-sm);
    padding-top: var(--spacing-md);
    border-top: 1px solid var(--color-border);
  }
  
  .btn-primary, .btn-secondary {
    padding: var(--spacing-sm) var(--spacing-lg);
    border-radius: var(--border-radius);
    cursor: pointer;
    font-weight: 500;
  }
  
  .btn-primary {
    background: var(--color-accent);
    color: white;
    border: none;
  }
  
  .btn-secondary {
    background: var(--color-bg-tertiary);
    color: var(--color-text-primary);
    border: 1px solid var(--color-border);
  }
  
  .empty {
    text-align: center;
    padding: var(--spacing-xl);
    color: var(--color-text-muted);
  }
</style>






