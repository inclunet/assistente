import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { CreateLLMProvider, UpdateLLMProvider, TestLLMProvider } from '@wailsjs/go/main/App';
import { Input, Select, Button, FormField } from '../';
import './ProviderForm.css';

// Wrapper com timeout para operações que podem travar
const withTimeout = async <T,>(promise: Promise<T>, timeoutMs: number, operationName: string): Promise<T> => {
  const timeoutPromise = new Promise<never>((_, reject) => {
    setTimeout(() => reject(new Error(`Timeout após ${timeoutMs/1000}s em ${operationName}`)), timeoutMs);
  });
  return Promise.race([promise, timeoutPromise]);
};

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
    defaultUrl: 'https://generativelanguage.googleapis.com/v1beta/openai/',
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
  deepseek: {
    label: 'DeepSeek',
    defaultUrl: 'https://api.deepseek.com',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://platform.deepseek.com',
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
  const { t } = useTranslation();
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
  const [apiKeyChangedInThisSession, setApiKeyChangedInThisSession] = useState(false);
  const apiKeyInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (provider) {
      setFormData({
        id: provider.id,
        name: provider.name,
        type: provider.type,
        base_url: provider.base_url,
        api_key: '', // Não carrega API key por segurança
      });
      // Em modo edição, requer reteste apenas se mudar a key
      setApiTested(true);
      setShowApiKeyField(false); // Oculta campo, mostra indicador
      setApiKeyChangedInThisSession(false);
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
      setApiKeyChangedInThisSession(false);
    }
  }, [provider]);

  // Auto-foca no campo de token quando clica "Alterar Chave"
  useEffect(() => {
    if (showApiKeyField && apiKeyInputRef.current) {
      // Pequeno delay para garantir que o DOM foi atualizado
      setTimeout(() => {
        apiKeyInputRef.current?.focus();
        // Seleciona todo o texto se já existe algo
        if (apiKeyInputRef.current?.value) {
          apiKeyInputRef.current?.select();
        }
      }, 0);
    }
  }, [showApiKeyField]);

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

  // Retorna a URL canônica que será REALMENTE salva no banco
  // Isso garante que URLs do Google sempre sejam corretas, etc.
  const getCanonicalUrl = (type: string): string => {
    const config = PROVIDER_CONFIG[type] || PROVIDER_CONFIG.custom;
    
    // Para provedores com URL não editável (comerciais), retorna a URL padrão
    if (!config.urlEditable) {
      return config.defaultUrl;
    }
    
    // Para provedores editáveis, usa o que o usuário digitou
    return formData.base_url;
  };

  const handleChange = (field: keyof ProviderFormData, value: string) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    // Limpa erro do campo
    if (errors[field]) {
      setErrors((prev) => ({ ...prev, [field]: '' }));
    }
  };

  const handleApiKeyChange = (value: string) => {
    handleChange('api_key', value);
    // Marca que a chave foi alterada nesta sessão
    setApiKeyChangedInThisSession(true);
    setApiTested(false); // Precisa testar de novo
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
    } catch (error: unknown) {
      setApiTested(false);
      const err = error as { message?: unknown } | null;
      setErrors((prev) => ({
        ...prev,
        api: String(err?.message || error || 'Erro ao testar API'),
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
      newErrors.name = t('providerForm.error.nameRequired');
    }

    // URL é sempre validada, mas sempre usa a URL canônica
    const canonicalUrl = getCanonicalUrl(formData.type);
    if (!canonicalUrl.trim()) {
      newErrors.base_url = t('providerForm.error.urlRequired');
    } else {
      try {
        new URL(canonicalUrl);
      } catch {
        newErrors.base_url = t('providerForm.error.urlInvalid');
      }
    }

    // API key - validação conforme configuração do provider
    if (config && config.apiKeyRequired && !formData.api_key.trim()) {
      newErrors.api_key = `API Key é obrigatória para ${config.label}`;
    }

    // Valida se API foi testada
    // EM MODO EDIÇÃO SEM MUDAR CHAVE: não exige reteste (chave já estava salva)
    // EM MODO EDIÇÃO COM MUDANÇA DE CHAVE: exige reteste
    // EM MODO CRIAÇÃO: sempre exige teste
    const isEditingWithoutChangingKey = formData.id && !apiKeyChangedInThisSession;
    const needsTest = !isEditingWithoutChangingKey;
    
    if (!apiTested && needsTest) {
      newErrors.api = t('providerForm.error.testFirst');
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!validate()) return;

    setSaving(true);
    try {
      // IMPORTANTE: Sempre usa a URL canônica ao salvar
      // Isso garante que URLs incorretas (ex: Google incompleto) sejam corrigidas automaticamente
      const canonicalUrl = getCanonicalUrl(formData.type);
      
      if (formData.id) {
        // Update
        await withTimeout(
          UpdateLLMProvider(formData.id, {
            name: formData.name,
            type: formData.type,
            base_url: canonicalUrl,
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
            base_url: canonicalUrl,
            api_key: formData.api_key || undefined,
          }),
          15000,
          'CreateLLMProvider'
        );
      }

      onSave();
    } catch (error: unknown) {
      const err = error as { message?: unknown; toString?: () => string } | null;
      setErrors({ submit: String(err?.message || err?.toString?.() || error || 'Erro ao salvar provedor') });
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
        label={t('providerForm.name')}
        required
        error={errors.name}
      >
        <Input
          value={formData.name}
          onChange={(e) => handleChange('name', e.target.value)}
          placeholder={t('providerForm.namePlaceholder')}
          fullWidth
        />
      </FormField>

      <FormField label={t('providerForm.providerType')} required>
        <Select
          options={PROVIDER_TYPES}
          value={formData.type}
          onChange={(e) => handleChange('type', e.target.value)}
          fullWidth
        />
      </FormField>

      <FormField
        label={t('providerForm.baseUrl')}
        required
        error={errors.base_url}
        description={config.helpText}
      >
        <Input
          value={formData.base_url}
          onChange={(e) => handleChange('base_url', e.target.value)}
          placeholder={t('providerForm.defaultUrl')}
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

      {/* Display URL que será salva no banco */}
      <FormField label="URL que será salva" description="Este é o endpoint que será usar para requisições">
        <Input
          value={getCanonicalUrl(formData.type)}
          readOnly
          disabled
          fullWidth
          aria-label="URL canônica que será salva"
          className="provider-form__read-only-url"
        />
        <span className="provider-form__info" style={{ marginTop: '8px', display: 'block' }}>
          ℹ️ Esta URL é calculada automaticamente e será SALVA no banco
        </span>
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
              ref={apiKeyInputRef}
              type={showPassword ? 'text' : 'password'}
              value={formData.api_key}
              onChange={(e) => handleApiKeyChange(e.target.value)}
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
              ref={apiKeyInputRef}
              type={showPassword ? 'text' : 'password'}
              value={formData.api_key}
              onChange={(e) => handleApiKeyChange(e.target.value)}
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
          disabled={testing || !getCanonicalUrl(formData.type).trim()}
          title={!getCanonicalUrl(formData.type).trim() ? 'Preenchea URL primeiro' : 'Testar conexão'}
          aria-label="Testar conexão com API"
        >
          {testing ? 'Testando...' : '🧪 Testar Conexão'}
        </Button>
        
        {/* Info sobre o que será testado */}
        {formData.id && !apiKeyChangedInThisSession && (
          <span className="provider-form__info" style={{ marginLeft: '12px' }}>
            ℹ️ Testará com a chave já configurada (não alterada nesta sessão)
          </span>
        )}
        
        {testRequiresApiKey && !formData.api_key.trim() && showApiKeyField && (
          <span className="provider-form__info" style={{ marginLeft: '12px' }}>
            ℹ️ Preencha a API Key para testar este provedor
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
