<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { GetConfig, SaveSettings, TestConnection, TestEmbeddings } from '../../wailsjs/go/main/App.js';
  import ModelPicker from './ModelPicker.svelte';
  import ImageModelPicker from './ImageModelPicker.svelte';

  const dispatch = createEventDispatcher();

  let apiKey = '';
  let apiBaseURL = 'https://api.openai.com/v1';
  
  // Parâmetros do Chat
  let chatModel = '';
  let chatTemperature = 0.7;
  let chatMaxTokens = 4096;
  let chatTopP = 1.0;
  
  // Parâmetros de Embeddings
  let embeddingsModel = '';
  let embeddingsDimensions = 0;
  
  // Modelo auxiliar para imagens
  let imageModel = '';
  
  let saving = false;
  let testing = false;
  let testingEmbeddings = false;
  let message = { type: '', text: '' };
  let showApiKey = false;
  let showAdvanced = false;

  onMount(async () => {
    try {
      const config = await GetConfig();
      if (config) {
        apiKey = config.api_key || '';
        apiBaseURL = config.api_base_url || 'https://api.openai.com/v1';
        
        // Carregar parâmetros do chat (com retrocompatibilidade)
        if (config.chat_params) {
          chatModel = config.chat_params.model || config.default_model || '';
          chatTemperature = config.chat_params.temperature || 0.7;
          chatMaxTokens = config.chat_params.max_tokens || 4096;
          chatTopP = config.chat_params.top_p || 1.0;
        } else {
          chatModel = config.default_model || '';
        }
        
        // Carregar parâmetros de embeddings (com retrocompatibilidade)
        if (config.embeddings_params) {
          embeddingsModel = config.embeddings_params.model || config.embeddings_model || '';
          embeddingsDimensions = config.embeddings_params.dimensions || 0;
        } else {
          embeddingsModel = config.embeddings_model || '';
        }
        
        // Modelo auxiliar para imagens
        imageModel = config.image_model || '';
      }
    } catch (error) {
      showMessage('error', 'Erro ao carregar configurações: ' + error);
    }
  });

  function showMessage(type, text) {
    message = { type, text };
    setTimeout(() => {
      const messageEl = document.getElementById('settings-message');
      if (messageEl) {
        messageEl.focus();
      }
    }, 100);
  }

  async function handleSave() {
    if (!apiKey.trim()) {
      showMessage('error', 'A chave de API é obrigatória.');
      return;
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
        image_model: imageModel
      });
      showMessage('success', 'Configurações salvas com sucesso!');
      dispatch('saved');
    } catch (error) {
      showMessage('error', 'Erro ao salvar: ' + error);
    } finally {
      saving = false;
    }
  }

  async function handleTest() {
    if (!apiKey.trim()) {
      showMessage('error', 'Configure a chave de API antes de testar.');
      return;
    }

    // Primeiro salva as configurações
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
        image_model: imageModel
      });
    } catch (error) {
      showMessage('error', 'Erro ao salvar configurações: ' + error);
      return;
    }

    testing = true;
    message = { type: '', text: '' };

    try {
      const result = await TestConnection();
      if (result) {
        showMessage('success', 'Conexão bem-sucedida! A API está funcionando e os modelos foram carregados.');
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

    // Primeiro salva as configurações
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
        image_model: imageModel
      });
    } catch (error) {
      showMessage('error', 'Erro ao salvar configurações: ' + error);
      return;
    }

    testingEmbeddings = true;
    message = { type: '', text: '' };

    try {
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
  
  <form on:submit|preventDefault={handleSave} aria-describedby="settings-description">
    <p id="settings-description" class="settings-description">
      Configure sua conexão com a API da OpenAI ou outro serviço compatível.
      Suas configurações serão salvas de forma segura na pasta do seu usuário.
    </p>

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

    <fieldset>
      <legend>Configurações de Conexão</legend>

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

      <div class="form-group">
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
        <small>
          Gera um embedding de teste para verificar se o modelo está configurado corretamente.
        </small>
      </div>
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
        O sistema aprende automaticamente quais modelos suportam visão conforme você os utiliza.
      </small>
    </fieldset>

    <div class="info-box" role="note">
      <strong>💡 Dica:</strong> Os parâmetros de temperatura e tokens também podem ser alterados diretamente no chat a qualquer momento.
    </div>

    <div class="button-group">
      <button
        type="button"
        class="btn-secondary"
        on:click={handleTest}
        disabled={testing || saving}
        aria-busy={testing}
      >
        {#if testing}
          <span class="loading-spinner" aria-hidden="true"></span>
          Testando...
        {:else}
          🔌 Testar Conexão
        {/if}
      </button>
      
      <button
        type="submit"
        class="btn-primary"
        disabled={saving || testing}
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
  </form>
</section>

<style>
  .settings-container {
    max-width: 100%;
  }

  .settings-description {
    color: var(--color-text-secondary);
    margin-bottom: var(--spacing-lg);
    line-height: var(--line-height);
  }

  fieldset {
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius-lg);
    padding: var(--spacing-lg);
    margin-bottom: var(--spacing-lg);
  }

  legend {
    font-weight: 600;
    color: var(--color-text-primary);
    padding: 0 var(--spacing-sm);
    font-size: var(--font-size-lg);
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

  input[type="number"] {
    width: 100%;
    padding: 0.75rem 1rem;
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    color: var(--color-text-primary);
    font-size: 0.95rem;
  }

  input[type="number"]:focus {
    outline: none;
    border-color: var(--color-accent);
  }

  .info-box {
    background-color: var(--color-bg-tertiary);
    border: 1px solid var(--color-border);
    border-left: 4px solid var(--color-accent);
    border-radius: var(--border-radius);
    padding: var(--spacing-md);
    margin-bottom: var(--spacing-lg);
    color: var(--color-text-secondary);
  }

  .info-box strong {
    color: var(--color-text-primary);
  }

  .button-group {
    display: flex;
    gap: var(--spacing-md);
    justify-content: flex-end;
    margin-top: var(--spacing-xl);
  }

  .btn-test {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.6rem 1rem;
    min-height: 40px;
    background: var(--color-bg-tertiary, #3a3a3a);
    border: 1px solid var(--color-border, #555);
    border-radius: var(--border-radius, 6px);
    color: var(--color-text-primary, #e0e0e0);
    font-size: 0.9rem;
    cursor: pointer;
    transition: background-color 0.2s, border-color 0.2s;
  }

  .btn-test:hover:not(:disabled) {
    background: var(--color-bg-hover, #454545);
    border-color: var(--color-accent, #4a9eff);
  }

  .btn-test:focus-visible {
    outline: 2px solid var(--color-accent, #4a9eff);
    outline-offset: 2px;
  }

  .btn-test:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .loading-spinner {
    display: inline-block;
    width: 14px;
    height: 14px;
    border: 2px solid var(--color-border, #444);
    border-top-color: var(--color-accent, #4a9eff);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
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

  /* Garantir área de toque mínima de 44x44px */
  button, input[type="checkbox"], .toggle-visibility {
    min-height: 44px;
  }

  input:not([type="range"]):not([type="checkbox"]) {
    min-height: 44px;
  }
</style>
