<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { GetConfig, SaveSettings, TestConnection, TestEmbeddings } from '../../../wailsjs/go/main/App.js';
  import { TabPanel } from '../../components/tabs';
  import { ModelPicker, ImageModelPicker } from '../../components/pickers';
  import { ChatPreferences } from '../../components/preferences';

  const dispatch = createEventDispatcher();

  // ==================== Configurações de API ====================
  let apiKey = '';
  let apiBaseURL = 'https://api.openai.com/v1';
  
  // ==================== Parâmetros do Chat ====================
  let chatModel = '';
  let chatTemperature = 0.7;
  let chatMaxTokens = 4096;
  let chatTopP = 1.0;
  
  // ==================== Parâmetros de Embeddings ====================
  let embeddingsModel = '';
  let embeddingsDimensions = 0;
  
  // ==================== Modelo auxiliar para imagens ====================
  let imageModel = '';
  
  // ==================== Preferências padrão do Chat ====================
  let useTools = true;
  let showInternalMessages = false;
  
  // ==================== Preferências de Voz ====================
  let voice = 'disabled';
  let autoSpeak = true;
  let voiceVolume = 100;
  let voiceRate = 0;
  
  // ==================== Preferências de STT ====================
  let sttProvider = 'webspeech';
  let recordingMode = 'ptt';
  
  // ==================== Estado da UI ====================
  let saving = false;
  let testing = false;
  let testingEmbeddings = false;
  let message = { type: '', text: '' };
  let showApiKey = false;
  let showAdvanced = false;
  let hasChanges = false;
  
  // Guias da página de configurações
  let activeTab = 'connection';
  const tabs = [
    { id: 'connection', label: 'Conexão', icon: '🔌' },
    { id: 'chat', label: 'Chat', icon: '💬' },
    { id: 'embeddings', label: 'Embeddings', icon: '🧠' },
    { id: 'defaults', label: 'Padrões', icon: '⚙️' }
  ];
  
  // Referência para ChatPreferences
  let chatPreferencesComponent;
  
  // Valores originais para detectar mudanças
  let originalValues = {};

  onMount(async () => {
    try {
      const config = await GetConfig();
      if (config) {
        // API
        apiKey = config.api_key || '';
        apiBaseURL = config.api_base_url || 'https://api.openai.com/v1';
        
        // Chat params
        if (config.chat_params) {
          chatModel = config.chat_params.model || config.default_model || '';
          chatTemperature = config.chat_params.temperature || 0.7;
          chatMaxTokens = config.chat_params.max_tokens || 4096;
          chatTopP = config.chat_params.top_p || 1.0;
        } else {
          chatModel = config.default_model || '';
        }
        
        // Embeddings params
        if (config.embeddings_params) {
          embeddingsModel = config.embeddings_params.model || config.embeddings_model || '';
          embeddingsDimensions = config.embeddings_params.dimensions || 0;
        } else {
          embeddingsModel = config.embeddings_model || '';
        }
        
        // Modelo de imagem
        imageModel = config.image_model || '';
        
        // Voice params
        if (config.voice_params) {
          voice = config.voice_params.voice || 'disabled';
          autoSpeak = config.voice_params.auto_speak !== false;
          voiceVolume = config.voice_params.volume || 100;
          voiceRate = config.voice_params.rate || 0;
        }
        
        // STT params
        if (config.stt_params) {
          sttProvider = config.stt_params.provider || 'webspeech';
          recordingMode = config.stt_params.recording_mode || 'ptt';
        }
        
        // Chat defaults
        if (config.chat_defaults) {
          useTools = config.chat_defaults.use_tools !== false;
          showInternalMessages = config.chat_defaults.show_internal_messages || false;
        }
        
        // Salva valores originais
        saveOriginalValues();
      }
    } catch (error) {
      showMessage('error', 'Erro ao carregar configurações: ' + error);
    }
  });

  function saveOriginalValues() {
    originalValues = {
      apiKey,
      apiBaseURL,
      chatModel,
      chatTemperature,
      chatMaxTokens,
      chatTopP,
      embeddingsModel,
      embeddingsDimensions,
      imageModel,
      useTools,
      showInternalMessages,
      voice,
      autoSpeak,
      voiceVolume,
      voiceRate,
      sttProvider,
      recordingMode
    };
    hasChanges = false;
  }

  function checkChanges() {
    hasChanges = 
      apiKey !== originalValues.apiKey ||
      apiBaseURL !== originalValues.apiBaseURL ||
      chatModel !== originalValues.chatModel ||
      chatTemperature !== originalValues.chatTemperature ||
      chatMaxTokens !== originalValues.chatMaxTokens ||
      chatTopP !== originalValues.chatTopP ||
      embeddingsModel !== originalValues.embeddingsModel ||
      embeddingsDimensions !== originalValues.embeddingsDimensions ||
      imageModel !== originalValues.imageModel ||
      useTools !== originalValues.useTools ||
      showInternalMessages !== originalValues.showInternalMessages ||
      voice !== originalValues.voice ||
      autoSpeak !== originalValues.autoSpeak ||
      voiceVolume !== originalValues.voiceVolume ||
      voiceRate !== originalValues.voiceRate ||
      sttProvider !== originalValues.sttProvider ||
      recordingMode !== originalValues.recordingMode;
  }
  
  // Reativamente verifica mudanças
  $: {
    checkChanges();
    // Força reatividade para todos os campos
    apiKey; apiBaseURL; chatModel; chatTemperature; chatMaxTokens; chatTopP;
    embeddingsModel; embeddingsDimensions; imageModel; useTools; showInternalMessages;
    voice; autoSpeak; voiceVolume; voiceRate; sttProvider; recordingMode;
  }

  function showMessage(type, text) {
    message = { type, text };
    setTimeout(() => {
      const messageEl = document.getElementById('settings-message');
      if (messageEl) {
        messageEl.focus();
      }
    }, 100);
  }
  
  function handlePreferencesChange(event) {
    const { field, value } = event.detail;
    
    // Atualiza os campos locais baseado no que mudou
    switch (field) {
      case 'model':
        chatModel = value;
        break;
      case 'temperature':
        chatTemperature = value;
        break;
      case 'maxTokens':
        chatMaxTokens = value;
        break;
      case 'topP':
        chatTopP = value;
        break;
      case 'useTools':
        useTools = value;
        break;
      case 'showInternalMessages':
        showInternalMessages = value;
        break;
      case 'voice':
        voice = value;
        break;
      case 'autoSpeak':
        autoSpeak = value;
        break;
      case 'voiceVolume':
        voiceVolume = value;
        break;
      case 'voiceRate':
        voiceRate = value;
        break;
      case 'sttProvider':
        sttProvider = value;
        break;
      case 'recordingMode':
        recordingMode = value;
        break;
    }
  }

  // Retorna true se salvar com sucesso, false se falhar
  async function handleSave(silent = false) {
    if (!apiKey.trim()) {
      showMessage('error', 'A chave de API é obrigatória.');
      return false;
    }

    saving = true;
    try {
      await SaveSettings({
        api_key: apiKey,
        api_base_url: apiBaseURL,
        chat_params: {
          model: chatModel,
          temperature: chatTemperature,
          max_tokens: chatMaxTokens,
          top_p: chatTopP
        },
        embeddings_params: {
          model: embeddingsModel,
          dimensions: embeddingsDimensions
        },
        image_model: imageModel,
        voice_params: {
          voice: voice,
          auto_speak: autoSpeak,
          volume: voiceVolume,
          rate: voiceRate
        },
        stt_params: {
          provider: sttProvider,
          recording_mode: recordingMode
        },
        chat_defaults: {
          use_tools: useTools,
          show_internal_messages: showInternalMessages
        }
      });
      if (!silent) {
        showMessage('success', 'Configurações salvas com sucesso!');
        saveOriginalValues();
        dispatch('saved');
      }
      return true;
    } catch (error) {
      showMessage('error', 'Erro ao salvar: ' + error);
      return false;
    } finally {
      saving = false;
    }
  }

  async function handleTestAndSave() {
    if (!apiKey.trim()) {
      showMessage('error', 'Configure a chave de API antes de testar.');
      return;
    }

    testing = true;
    message = { type: '', text: '' };

    try {
      // Primeiro salva as configurações (silenciosamente)
      const saved = await handleSave(true);
      if (!saved) {
        return;
      }

      // Testa a conexão
      const result = await TestConnection();
      if (result) {
        showMessage('success', 'Conexão bem-sucedida! Configurações salvas.');
        saveOriginalValues();
        dispatch('saved');
      }
    } catch (error) {
      showMessage('error', 'Falha na conexão: ' + error);
    } finally {
      testing = false;
    }
  }

  async function handleTestEmbeddings() {
    if (!apiKey.trim()) {
      showMessage('error', 'Configure a chave de API antes de testar.');
      return;
    }

    testingEmbeddings = true;
    message = { type: '', text: '' };

    try {
      // Primeiro salva as configurações (silenciosamente)
      const saved = await handleSave(true);
      if (!saved) {
        return;
      }

      const result = await TestEmbeddings();
      showMessage('success', result);
    } catch (error) {
      showMessage('error', 'Falha no teste de embeddings: ' + error);
    } finally {
      testingEmbeddings = false;
    }
  }

  function toggleShowApiKey() {
    showApiKey = !showApiKey;
  }
</script>

<section class="settings-container">
  {#if message.text}
    <div 
      id="settings-message"
      class="message-{message.type}"
      role="alert"
      aria-live="assertive"
      tabindex="-1"
    >
      {message.text}
    </div>
  {/if}

  <TabPanel 
    {tabs} 
    bind:activeTab
    variant="default"
    size="md"
  >
    <svelte:fragment slot="tab-content" let:tab>
      {#if tab.id === 'connection'}
        <!-- ==================== ABA: CONEXÃO ==================== -->
        <div class="settings-section">
          <p class="settings-description">
            Configure sua conexão com a API da OpenAI ou outro serviço compatível.
          </p>

          <fieldset>
            <legend>Configurações de API</legend>

            <div class="form-group">
              <label for="api-key">
                Chave de API <span class="required" aria-hidden="true">*</span>
                <span class="visually-hidden">(obrigatório)</span>
              </label>
              <div class="api-key-input-wrapper">
                {#if showApiKey}
                  <input
                    id="api-key"
                    type="text"
                    bind:value={apiKey}
                    required
                    autocomplete="off"
                    aria-describedby="api-key-help"
                    placeholder="sk-..."
                  />
                {:else}
                  <input
                    id="api-key"
                    type="password"
                    bind:value={apiKey}
                    required
                    autocomplete="off"
                    aria-describedby="api-key-help"
                    placeholder="sk-..."
                  />
                {/if}
                <button
                  type="button"
                  class="toggle-visibility"
                  on:click={toggleShowApiKey}
                  aria-pressed={showApiKey}
                  aria-label={showApiKey ? 'Ocultar chave de API' : 'Mostrar chave de API'}
                >
                  {showApiKey ? '🙈' : '👁️'}
                </button>
              </div>
              <small id="api-key-help">
                Sua chave de API da OpenAI. Você pode obtê-la em 
                <a href="https://platform.openai.com/api-keys" target="_blank" rel="noopener noreferrer">
                  platform.openai.com/api-keys
                </a>
              </small>
            </div>

            <div class="form-group">
              <label for="api-base-url">URL Base da API</label>
              <input
                id="api-base-url"
                type="url"
                bind:value={apiBaseURL}
                aria-describedby="api-base-url-help"
                placeholder="https://api.openai.com/v1"
              />
              <small id="api-base-url-help">
                URL base da API (sem o caminho do recurso). Use o padrão para OpenAI ou altere para serviços compatíveis como Ollama, LM Studio, LiteLLM Proxy, etc.
              </small>
            </div>
          </fieldset>

          <div class="button-row">
            <button
              type="button"
              class="btn-primary"
              on:click={handleTestAndSave}
              disabled={testing || saving || !apiKey.trim()}
              aria-busy={testing}
            >
              {#if testing}
                <span class="loading-spinner" aria-hidden="true"></span>
                Testando...
              {:else}
                🔌 Testar e Salvar
              {/if}
            </button>
          </div>
        </div>
        
      {:else if tab.id === 'chat'}
        <!-- ==================== ABA: CHAT ==================== -->
        <div class="settings-section">
          <fieldset>
            <legend>Modelo de Chat (LLM)</legend>

            <div class="form-group">
              <ModelPicker 
                bind:value={chatModel}
                label="Modelo"
                placeholder="Selecione ou digite um modelo"
                helpText="Modelo LLM padrão para o chat. Para LiteLLM Proxy, use o nome conforme configurado no proxy."
                allowCustom={true}
                variant="form"
              />
            </div>

            <div class="form-group">
              <label for="chat-temperature">Temperatura: <strong>{chatTemperature}</strong></label>
              <p class="param-description">Controla a criatividade. Valores menores são mais precisos, maiores são mais criativos.</p>
              <input
                id="chat-temperature"
                type="range"
                bind:value={chatTemperature}
                min="0"
                max="2"
                step="0.1"
              />
              <div class="range-labels">
                <span>Preciso (0)</span>
                <span>Criativo (2)</span>
              </div>
            </div>

            <div class="form-group">
              <label for="chat-max-tokens">Máximo de Tokens: <strong>{chatMaxTokens}</strong></label>
              <p class="param-description">Limite de tokens na resposta. Valores maiores permitem respostas mais longas.</p>
              <input
                id="chat-max-tokens"
                type="range"
                bind:value={chatMaxTokens}
                min="100"
                max="16000"
                step="100"
              />
              <div class="range-labels">
                <span>100</span>
                <span>16000</span>
              </div>
            </div>

            <button 
              type="button" 
              class="toggle-advanced"
              on:click={() => showAdvanced = !showAdvanced}
            >
              {showAdvanced ? '▼' : '▶'} Parâmetros Avançados
            </button>

            {#if showAdvanced}
              <div class="advanced-params">
                <div class="form-group">
                  <label for="chat-top-p">Top P: <strong>{chatTopP}</strong></label>
                  <p class="param-description">Controla a diversidade via nucleus sampling. Alternativa à temperatura.</p>
                  <input
                    id="chat-top-p"
                    type="range"
                    bind:value={chatTopP}
                    min="0"
                    max="1"
                    step="0.05"
                  />
                  <div class="range-labels">
                    <span>Focado (0)</span>
                    <span>Diverso (1)</span>
                  </div>
                </div>
              </div>
            {/if}
          </fieldset>

          <fieldset>
            <legend>Modelo de Imagem (Visão)</legend>
            
            <div class="form-group">
              <ImageModelPicker 
                bind:value={imageModel}
                chatModel={chatModel}
                label="Modelo auxiliar"
                variant="form"
                helpText="Modelo para processar imagens. Se vazio, usa o modelo de chat (se suportar visão)."
              />
            </div>
            
            <small class="fieldset-note">
              Quando o modelo de chat não suportar visão, o sistema usará este modelo auxiliar para descrever imagens.
            </small>
          </fieldset>
        </div>
        
      {:else if tab.id === 'embeddings'}
        <!-- ==================== ABA: EMBEDDINGS ==================== -->
        <div class="settings-section">
          <fieldset>
            <legend>Modelo de Embeddings</legend>
            
            <div class="form-group">
              <ModelPicker 
                bind:value={embeddingsModel}
                label="Modelo"
                placeholder="text-embedding-3-small"
                helpText="Modelo para gerar embeddings (busca semântica de FAQs). Deixe em branco para usar o padrão OpenAI."
                allowCustom={true}
                variant="form"
              />
            </div>

            <div class="form-group">
              <label for="embeddings-dimensions">Dimensões (opcional)</label>
              <input
                id="embeddings-dimensions"
                type="number"
                bind:value={embeddingsDimensions}
                min="0"
                max="3072"
                placeholder="Padrão do modelo"
              />
              <small>
                Alguns modelos permitem reduzir dimensões para economizar espaço. 0 = usar padrão do modelo.
              </small>
            </div>
          </fieldset>

          <div class="button-row">
            <button
              type="button"
              class="btn-test"
              on:click={handleTestEmbeddings}
              disabled={testingEmbeddings || saving || !apiKey.trim()}
              aria-busy={testingEmbeddings}
            >
              {#if testingEmbeddings}
                <span class="loading-spinner" aria-hidden="true"></span>
                Testando embeddings...
              {:else}
                🧪 Testar Modelo de Embeddings
              {/if}
            </button>
          </div>
        </div>
        
      {:else if tab.id === 'defaults'}
        <!-- ==================== ABA: PADRÕES ==================== -->
        <div class="settings-section">
          <p class="settings-description">
            Configure as preferências padrão para novas conversas. 
            Cada conversa pode sobrescrever essas configurações localmente.
          </p>
          
          <ChatPreferences
            bind:this={chatPreferencesComponent}
            bind:model={chatModel}
            bind:temperature={chatTemperature}
            bind:maxTokens={chatMaxTokens}
            bind:topP={chatTopP}
            bind:useTools={useTools}
            bind:showInternalMessages={showInternalMessages}
            bind:voice={voice}
            bind:autoSpeak={autoSpeak}
            bind:voiceVolume={voiceVolume}
            bind:voiceRate={voiceRate}
            bind:sttProvider={sttProvider}
            bind:recordingMode={recordingMode}
            showAdvanced={false}
            on:change={handlePreferencesChange}
          />
        </div>
      {/if}
    </svelte:fragment>
  </TabPanel>

  <div class="settings-footer">
    <div class="info-box" role="note">
      <strong>💡 Dica:</strong> As preferências da aba "Padrões" podem ser alteradas por conversa usando o modal de preferências.
    </div>

    <div class="button-group">
      <button
        type="button"
        class="btn-primary"
        on:click={handleSave}
        disabled={saving || testing || !hasChanges}
        aria-busy={saving}
      >
        {#if saving}
          <span class="loading-spinner" aria-hidden="true"></span>
          Salvando...
        {:else}
          💾 Salvar Configurações
        {/if}
      </button>
    </div>
  </div>
</section>

<style>
  .settings-container {
    display: flex;
    flex-direction: column;
    height: 100%;
    max-width: 100%;
  }

  .settings-section {
    padding: var(--spacing-lg, 16px);
    display: flex;
    flex-direction: column;
    gap: var(--spacing-lg, 16px);
  }

  .settings-description {
    color: var(--color-text-secondary);
    margin: 0;
    line-height: var(--line-height);
  }

  .settings-footer {
    padding: var(--spacing-lg, 16px);
    border-top: 1px solid var(--color-border);
    background: var(--color-bg-secondary);
    display: flex;
    flex-direction: column;
    gap: var(--spacing-md, 12px);
  }

  fieldset {
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius-lg);
    padding: var(--spacing-lg);
    margin: 0;
  }

  legend {
    font-weight: 600;
    color: var(--color-text-primary);
    padding: 0 var(--spacing-sm);
    font-size: var(--font-size-lg);
  }

  .form-group {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-xs);
    margin-bottom: var(--spacing-md);
  }
  
  .form-group:last-child {
    margin-bottom: 0;
  }
  
  .form-group label {
    font-weight: 500;
    color: var(--color-text-primary);
  }

  .required {
    color: var(--color-error);
  }

  .api-key-input-wrapper {
    display: flex;
    gap: var(--spacing-sm);
  }

  .api-key-input-wrapper input {
    flex: 1;
  }

  .toggle-visibility {
    padding: var(--spacing-sm);
    min-width: 44px;
    min-height: 44px;
    background-color: var(--color-bg-tertiary);
    border: 1px solid var(--color-border);
    font-size: var(--font-size-lg);
    cursor: pointer;
    border-radius: var(--border-radius);
  }

  .toggle-visibility:hover {
    background-color: var(--color-border);
  }

  .param-description {
    font-size: 0.85rem;
    color: var(--color-text-secondary);
    margin: 0.25rem 0 0.5rem 0;
  }

  .range-labels {
    display: flex;
    justify-content: space-between;
    font-size: 0.8rem;
    color: var(--color-text-secondary);
    margin-top: 0.25rem;
  }

  input[type="range"] {
    width: 100%;
    height: 8px;
    -webkit-appearance: none;
    appearance: none;
    background: var(--color-bg-tertiary);
    border-radius: 4px;
    outline: none;
  }

  input[type="range"]::-webkit-slider-thumb {
    -webkit-appearance: none;
    appearance: none;
    width: 20px;
    height: 20px;
    background: var(--color-accent);
    border-radius: 50%;
    cursor: pointer;
  }

  input[type="range"]::-moz-range-thumb {
    width: 20px;
    height: 20px;
    background: var(--color-accent);
    border-radius: 50%;
    cursor: pointer;
    border: none;
  }

  .toggle-advanced {
    background: transparent;
    border: none;
    color: var(--color-accent);
    cursor: pointer;
    padding: var(--spacing-sm) 0;
    font-size: 0.9rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .toggle-advanced:hover {
    text-decoration: underline;
  }

  .advanced-params {
    margin-top: var(--spacing-md);
    padding-top: var(--spacing-md);
    border-top: 1px solid var(--color-border);
  }

  input[type="text"],
  input[type="password"],
  input[type="url"],
  input[type="number"] {
    width: 100%;
    padding: 0.75rem 1rem;
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    color: var(--color-text-primary);
    font-size: 0.95rem;
    min-height: 44px;
  }

  input:focus {
    outline: none;
    border-color: var(--color-accent);
  }

  .info-box {
    background-color: var(--color-bg-tertiary);
    border: 1px solid var(--color-border);
    border-left: 4px solid var(--color-accent);
    border-radius: var(--border-radius);
    padding: var(--spacing-md);
    color: var(--color-text-secondary);
  }

  .info-box strong {
    color: var(--color-text-primary);
  }

  .button-row {
    display: flex;
    gap: var(--spacing-md);
  }

  .button-group {
    display: flex;
    gap: var(--spacing-md);
    justify-content: flex-end;
  }

  .btn-primary,
  .btn-secondary,
  .btn-test {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    padding: 0.75rem 1.5rem;
    min-height: 44px;
    font-size: 1rem;
    font-weight: 500;
    border-radius: var(--border-radius);
    cursor: pointer;
    transition: all 0.2s;
  }

  .btn-primary {
    background: var(--color-accent);
    border: 1px solid var(--color-accent);
    color: white;
  }

  .btn-primary:hover:not(:disabled) {
    background: var(--color-accent-hover);
    border-color: var(--color-accent-hover);
  }

  .btn-primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .btn-secondary,
  .btn-test {
    background: var(--color-bg-tertiary);
    border: 1px solid var(--color-border);
    color: var(--color-text-primary);
  }

  .btn-secondary:hover:not(:disabled),
  .btn-test:hover:not(:disabled) {
    background: var(--color-bg-hover);
    border-color: var(--color-accent);
  }

  .btn-secondary:disabled,
  .btn-test:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .btn-primary:focus-visible,
  .btn-secondary:focus-visible,
  .btn-test:focus-visible {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
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

  .message-success,
  .message-error {
    padding: var(--spacing-md);
    margin-bottom: var(--spacing-md);
    border-radius: var(--border-radius);
  }

  .message-success {
    background: var(--color-success-bg, rgba(40, 167, 69, 0.15));
    border: 1px solid var(--color-success, #28a745);
    color: var(--color-success, #28a745);
  }

  .message-error {
    background: var(--color-error-bg, rgba(220, 53, 69, 0.15));
    border: 1px solid var(--color-error, #dc3545);
    color: var(--color-error, #dc3545);
  }

  .fieldset-note {
    color: var(--color-text-secondary);
    font-size: 0.875rem;
  }

  a {
    color: var(--color-accent);
    text-decoration: underline;
  }

  a:hover {
    color: var(--color-accent-hover);
  }

  a:focus-visible {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
  }

  .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
</style>
