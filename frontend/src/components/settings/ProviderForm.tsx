
  // Wrapper com timeout para operações que podem travar
  const withTimeout = async <T,>(promise: Promise<T>, timeoutMs: number, operationName: string): Promise<T> => {
    const timeoutPromise = new Promise<never>((_, reject) => {
      setTimeout(() => reject(new Error(`Timeout após ${timeoutMs/1000}s em ${operationName}`)), timeoutMs);
    });
    return Promise.race([promise, timeoutPromise]);
  };
import { useState, useEffect } from 'react';
import { Input, Select, Button, FormField } from '../';
import { CreateLLMProvider, UpdateLLMProvider, TestLLMProvider } from '@wailsjs/go/main/App';
import './ProviderForm.css';

export interface ProviderFormData {
  id?: string;
  name: string;
  type: string;
  base_url: string;
  api_key: string;
}

export interface ProviderFormProps {
  provider?: ProviderFormData;
  onSave: () => void;
  onCancel: () => void;
}

// Provider configuration: define behavior for each provider type
interface ProviderConfig {
  label: string;
  defaultUrl: string;
  urlEditable: boolean;
  apiKeyRequired: boolean;
  testRequiresApiKey: boolean;
  helpText?: string;
}

export const PROVIDER_CONFIG: Record<string, ProviderConfig> = {
  // Commercial API providers - require API key only
  openai: {
    label: 'OpenAI',
    defaultUrl: 'https://api.openai.com/v1',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://platform.openai.com/api-keys',
  },
  anthropic: {
    label: 'Anthropic',
    defaultUrl: 'https://api.anthropic.com',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://console.anthropic.com',
  },
  google: {
    label: 'Google (Gemini)',
    defaultUrl: 'https://generativelanguage.googleapis.com',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://aistudio.google.com',
  },
  openrouter: {
    label: 'OpenRouter',
    defaultUrl: 'https://openrouter.ai/api',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://openrouter.ai/keys',
  },
  xai: {
    label: 'xAI (Grok)',
    defaultUrl: 'https://api.x.ai',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://console.x.ai',
  },
  cohere: {
    label: 'Cohere',
    defaultUrl: 'https://api.cohere.ai',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://dashboard.cohere.ai',
  },

  // Local/self-hosted providers - URL editable, token optional
  ollama: {
    label: 'Ollama (local)',
    defaultUrl: 'http://localhost:11434',
    urlEditable: true,
    apiKeyRequired: false,
    testRequiresApiKey: false,
    helpText: 'Running locally. You can change the URL if using a different host/port',
  },
  localai: {
    label: 'LocalAI',
    defaultUrl: 'http://localhost:8080',
    urlEditable: true,
    apiKeyRequired: false,
    testRequiresApiKey: false,
    helpText: 'Running locally. Configure URL and optional API key if needed',
  },

  // LiteLLM proxy - both URL and token required
  litellm: {
    label: 'LiteLLM Proxy',
    defaultUrl: 'http://localhost:4000',
    urlEditable: true,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'LiteLLM proxy server. Requires URL and API key',
  },

  // Custom provider - user must configure everything
  custom: {
    label: 'Custom',
    defaultUrl: '',
    urlEditable: true,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Configure your custom LLM provider',
  },
};

// Generate provider types for dropdown
const PROVIDER_TYPES = Object.entries(PROVIDER_CONFIG).map(([key, config]) => ({
  value: key,
  label: config.label,
}));

export const ProviderForm = ({ provider, onSave, onCancel }: ProviderFormProps) => {
  const [formData, setFormData] = useState<ProviderFormData>({
    name: '',
    type: 'openai',
    base_url: '',
    api_key: '',
  });
  const [showPassword, setShowPassword] = useState(false);
  const [showApiKeyField, setShowApiKeyField] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [apiTested, setApiTested] = useState(false);

  useEffect(() => {
    if (provider) {
      setFormData({
        id: provider.id,
        name: provider.name,
        type: provider.type,
        base_url: provider.base_url,
        api_key: '', // Não carrega API key por segurança
      });
      // Em modo edição, se não alterou a key (campo oculto), considera já testado
      setApiTested(true);
      setShowApiKeyField(false); // Oculta campo, mostra indicador
    } else {
      // Novo provider - define URL padrão baseado no tipo
      const defaultType = 'openai';
      const config = PROVIDER_CONFIG[defaultType] || PROVIDER_CONFIG.custom;
      setFormData({
        name: '',
        type: defaultType,
        base_url: config.defaultUrl,
        api_key: '',
      });
      setApiTested(false);
      setShowApiKeyField(true); // Mostra campo em modo criação
    }
  }, [provider]);

  // Atualiza URL quando tipo de provedor muda
  useEffect(() => {
    if (!provider) {
      const config = PROVIDER_CONFIG[formData.type] || PROVIDER_CONFIG.custom;
      setFormData((prev) => ({
        ...prev,
        base_url: config.defaultUrl,
      }));
      // Reset teste quando muda tipo
      setApiTested(false);
    }
  }, [formData.type, provider]);

  const handleChange = (field: keyof ProviderFormData, value: any) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    // Limpa erro do campo
    if (errors[field]) {
      setErrors((prev) => ({ ...prev, [field]: '' }));
    }
  };

  const handleTestApi = async () => {
    // Testa API manualmente ou ao sair do campo
    const config = PROVIDER_CONFIG[formData.type] || PROVIDER_CONFIG.custom;
    
    // Se requer key e está vazio, não testa
    if (config.testRequiresApiKey && !formData.api_key.trim()) {
      setErrors((prev) => ({
        ...prev,
        api_key: 'API Key é obrigatória para testar este provedor',
      }));
      return;
    }

    setTesting(true);
    try {
      const result = await withTimeout(
        TestLLMProvider({
          type: formData.type,
          base_url: formData.base_url,
          api_key: formData.api_key || undefined,
        }),
        15000,
        'TestLLMProvider'
      );

      if (result) {
        setApiTested(true);
        setErrors((prev) => {
          const newErrors = { ...prev };
          delete newErrors.api_key;
          delete newErrors.api;
          return newErrors;
        });
      } else {
        setApiTested(false);
        setErrors((prev) => ({
          ...prev,
          api: 'API não respondeu. Verifique URL e credenciais.',
        }));
      }
    } catch (error: any) {
      setApiTested(false);
      setErrors((prev) => ({
        ...prev,
        api: error.message || 'Erro ao testar API',
      }));
    } finally {
      setTesting(false);
    }
  };

  const handleApiKeyBlur = async () => {
    // Auto-testa ao sair do campo de API Key (exceto para provedores que não requerem)
    const config = PROVIDER_CONFIG[formData.type] || PROVIDER_CONFIG.custom;
    
    // Se não requer key e a key está vazia, testa mesmo assim
    if (!config.apiKeyRequired && !formData.api_key.trim()) {
      await handleTestApi();
      return;
    }

    // Se requer key e está vazio, apenas marca erro
    if (config.testRequiresApiKey && !formData.api_key.trim()) {
      return;
    }

    // Testa se tem URL e (key se for obrigatória)
    if (formData.base_url.trim()) {
      await handleTestApi();
    }
  };

  const validate = (): boolean => {
    const newErrors: Record<string, string> = {};
    const config = PROVIDER_CONFIG[formData.type] || PROVIDER_CONFIG.custom;

    if (!formData.name.trim()) {
      newErrors.name = 'Nome é obrigatório';
    }

    if (!formData.base_url.trim()) {
      newErrors.base_url = 'URL é obrigatória';
    } else {
      try {
        new URL(formData.base_url);
      } catch {
        newErrors.base_url = 'URL inválida';
      }
    }

    // API key - validação conforme configuração do provider
    if (config && config.apiKeyRequired && !formData.api_key.trim()) {
      newErrors.api_key = `API Key é obrigatória para ${config.label}`;
    }

    // Valida se API foi testada (se requer teste)
    // Em modo edição, se não está mostrando campo de API key, não exige reteste
    const needsTest = !formData.id || (formData.id && showApiKeyField && formData.api_key.trim());
    if (!apiTested && needsTest) {
      newErrors.api = 'Por favor, teste a conexão com a API antes de salvar';
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!validate()) return;

    setSaving(true);
    try {
      if (formData.id) {
        // Update
        await withTimeout(
          UpdateLLMProvider(formData.id, {
            name: formData.name,
            type: formData.type,
            base_url: formData.base_url,
            api_key: formData.api_key || undefined,
          }),
          15000,
          'UpdateLLMProvider'
        );
      } else {
        // Create - gera ID único
        await withTimeout(
          CreateLLMProvider({
            id: `${formData.type}-${Date.now()}`,
            name: formData.name,
            type: formData.type,
            base_url: formData.base_url,
            api_key: formData.api_key || undefined,
          }),
          15000,
          'CreateLLMProvider'
        );
      }

      onSave();
    } catch (error: any) {
      setErrors({ submit: error.message || error.toString() || 'Erro ao salvar provedor' });
    } finally {
      setSaving(false);
    }
  };

  const config = PROVIDER_CONFIG[formData.type] || PROVIDER_CONFIG.custom;
  const isUrlReadonly = !config.urlEditable; // Bloqueia URL independente de modo criação/edição
  const requiresApiKey = config.apiKeyRequired;
  const testRequiresApiKey = config.testRequiresApiKey;

  return (
    <form className="provider-form" onSubmit={handleSubmit}>
      <FormField
        label="Nome"
        required
        error={errors.name}
      >
        <Input
          value={formData.name}
          onChange={(e) => handleChange('name', e.target.value)}
          placeholder="Meu Provedor OpenAI"
          fullWidth
        />
      </FormField>

      <FormField label="Tipo de Provedor" required>
        <Select
          options={PROVIDER_TYPES}
          value={formData.type}
          onChange={(e) => handleChange('type', e.target.value)}
          fullWidth
        />
      </FormField>

      <FormField
        label="Base URL"
        required
        error={errors.base_url}
        description={config.helpText}
      >
        <Input
          value={formData.base_url}
          onChange={(e) => handleChange('base_url', e.target.value)}
          placeholder="https://api.openai.com/v1"
          fullWidth
          readOnly={isUrlReadonly}
          disabled={isUrlReadonly}
          aria-label="Base URL"
          aria-describedby={`base-url-help-${formData.type}`}
        />
        {isUrlReadonly && (
          <span className="provider-form__read-only-note">
            URL padrão - não pode ser alterada para este tipo
          </span>
        )}
      </FormField>

      {/* API Key Field - mostrado quando obrigatório OU opcional */}
      {requiresApiKey ? (
        <FormField
          label="API Key"
          required
          error={errors.api_key}
          description={
            formData.id && !showApiKeyField
              ? '🔑 Chave configurada no gerenciador de credenciais'
              : formData.id && showApiKeyField
              ? 'Deixe em branco para manter a chave atual'
              : 'Será salva criptografada no gerenciador de credenciais'
          }
        >
          {formData.id && !showApiKeyField ? (
            <Button
              type="button"
              variant="secondary"
              onClick={() => setShowApiKeyField(true)}
              className="provider-form__change-key-button"
              aria-label="Alterar API Key"
            >
              🔓 Alterar Chave
            </Button>
          ) : (
          <div className="provider-form__password-field">
            <Input
              type={showPassword ? 'text' : 'password'}
              value={formData.api_key}
              onChange={(e) => handleChange('api_key', e.target.value)}
              onBlur={handleApiKeyBlur}
              placeholder={formData.id ? '••••••••' : 'sk-...'}
              fullWidth
              aria-label="API Key"
              aria-describedby="api-key-description"
            />
            <Button
              type="button"
              variant="secondary"
              onClick={() => setShowPassword(!showPassword)}
              className="provider-form__toggle-password"
              aria-label={showPassword ? 'Ocultar chave' : 'Mostrar chave'}
              aria-pressed={showPassword}
              title={showPassword ? 'Ocultar chave' : 'Mostrar chave'}
            >
              <span aria-hidden="true">{showPassword ? '🔒' : '🔓'}</span>
            </Button>
            {testing && <span className="provider-form__testing">Testando...</span>}
            {apiTested && !testing && (
              <span className="provider-form__success" aria-live="polite">
                ✓ Conectado
              </span>
            )}
          </div>
          )}
        </FormField>
      ) : (
        <FormField
          label="API Key (Opcional)"
          error={errors.api_key}
          description={
            formData.id && !showApiKeyField
              ? '🔑 Chave configurada no gerenciador de credenciais (ou sem chave)'
              : 'Deixe em branco se não precisar de chave para este provedor'
          }
        >
          {formData.id && !showApiKeyField ? (
            <Button
              type="button"
              variant="secondary"
              onClick={() => setShowApiKeyField(true)}
              className="provider-form__change-key-button"
              aria-label="Alterar API Key"
            >
              🔓 Alterar Chave
            </Button>
          ) : (
          <div className="provider-form__password-field">
            <Input
              type={showPassword ? 'text' : 'password'}
              value={formData.api_key}
              onChange={(e) => handleChange('api_key', e.target.value)}
              onBlur={handleApiKeyBlur}
              placeholder="Deixar em branco se não usar chave"
              fullWidth
              aria-label="API Key (opcional)"
            />
            <Button
              type="button"
              variant="secondary"
              onClick={() => setShowPassword(!showPassword)}
              className="provider-form__toggle-password"
              aria-label={showPassword ? 'Ocultar chave' : 'Mostrar chave'}
              aria-pressed={showPassword}
              title={showPassword ? 'Ocultar chave' : 'Mostrar chave'}
            >
              <span aria-hidden="true">{showPassword ? '🔒' : '🔓'}</span>
            </Button>
            {testing && <span className="provider-form__testing">Testando...</span>}
            {apiTested && !testing && (
              <span className="provider-form__success" aria-live="polite">
                ✓ Conectado
              </span>
            )}
          </div>
          )}
        </FormField>
      )}

      {/* Test button */}
      <div className="provider-form__test-section">
        <Button
          type="button"
          variant="secondary"
          onClick={handleTestApi}
          disabled={testing || !formData.base_url.trim()}
          title={!formData.base_url.trim() ? 'Preenchea URL primeiro' : 'Testar conexão'}
          aria-label="Testar conexão com API"
        >
          {testing ? 'Testando...' : 'Testar Conexão'}
        </Button>
        {testRequiresApiKey && !formData.api_key.trim() && (
          <span className="provider-form__info">
            ℹ️ Preenchaa API Key para testar este provedor
          </span>
        )}
      </div>

      {errors.api && (
        <div className="provider-form__error" role="alert">
          ⚠️ {errors.api}
        </div>
      )}

      {errors.submit && (
        <div className="provider-form__error" role="alert">
          ⚠️ {errors.submit}
        </div>
      )}

      <div className="provider-form__actions">
        <Button type="button" variant="secondary" onClick={onCancel}>
          Cancelar
        </Button>
        <Button
          type="submit"
          variant="primary"
          disabled={saving || !apiTested}
          title={!apiTested ? 'Teste a API antes de salvar' : undefined}
        >
          {saving
            ? 'Salvando...'
            : formData.id
              ? 'Atualizar'
              : 'Criar'}
        </Button>
      </div>
    </form>
  );
};
