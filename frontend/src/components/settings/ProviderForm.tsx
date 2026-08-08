import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { EyeOutlined, EyeInvisibleOutlined, WarningOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { CreateLLMProvider, UpdateLLMProvider, ListModelsRaw } from '@wailsjs/go/app/App';
import { Input, Select, Button, FormField } from '../';
import { AGENT_API_FORMAT, PROVIDER_CONFIG } from '../../config/providers';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import type { CatalogAgent } from './ACPAgentCatalog';
import { AgentPicker } from './AgentPicker';
import { AgentProviderFields } from './AgentProviderFields';
export { PROVIDER_CONFIG } from '../../config/providers';
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
  default_model?: string;
  api_format?: string;
  /** Comando e argumentos do agente de código, quando o formato é acp. */
  acp_command?: string;
  acp_args?: string[];
  /**
   * Qual agente do registro ACP é este provedor (AEP-0086 D11). Vazio é agente
   * apontado à mão: os campos de comando continuam valendo, e o que depende de
   * saber qual agente é não tem o que oferecer.
   */
  acp_agent_id?: string;
}

export interface ProviderFormProps {
  provider?: ProviderFormData;
  onSave: () => void;
  onCancel: () => void;
}

// Provider configuration is imported from '../../config/providers'
// ProviderPreset type is used internally via PROVIDER_CONFIG

// Generate provider types for dropdown
const providerTypes = (t: TFunction) =>
  Object.entries(PROVIDER_CONFIG).map(([key, config]) => ({
    value: key,
    label: config.labelKey ? t(config.labelKey, config.label) : config.label,
  }));

// Só formatos HTTP: `acp` não entra porque não é uma escolha de protocolo que
// alguém faça para um endereço. Um agente é agente por ser um agente, e a
// combinação "URL + acp" é recusada pelo backend — oferecê-la aqui seria
// oferecer um erro.
export const API_FORMAT_OPTIONS = [
  { value: 'openai_responses', label: 'OpenAI — Responses API' },
  { value: 'openai',           label: 'OpenAI-compatible — Chat Completions' },
  { value: 'anthropic',        label: 'Anthropic — Messages API' },
  { value: 'google',           label: 'Google — Gemini API' },
];

/**
 * Diz se estes dados descrevem um agente de código local. Vem do formato porque
 * é ele que o backend usa para decidir, e um provedor já salvo carrega o dele
 * mesmo que o preset do tipo mude depois.
 */
const isAgentForm = (data: Pick<ProviderFormData, 'type' | 'api_format'>): boolean =>
  (data.api_format || PROVIDER_CONFIG[data.type]?.apiFormat || '') === AGENT_API_FORMAT;

export const ProviderForm = ({ provider, onSave, onCancel }: ProviderFormProps) => {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const tiposDeProvedor = useMemo(() => providerTypes(t), [t]);
  const [formData, setFormData] = useState<ProviderFormData>({
    name: '',
    type: 'openai',
    base_url: '',
    api_key: '',
    api_format: PROVIDER_CONFIG.openai.apiFormat || '',
  });
  const [showPassword, setShowPassword] = useState(false);
  const [showApiKeyField, setShowApiKeyField] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [apiTested, setApiTested] = useState(false);
  const [apiKeyChangedInThisSession, setApiKeyChangedInThisSession] = useState(false);
  const apiKeyInputRef = useRef<HTMLInputElement>(null);

  // Model loading states
  const [models, setModels] = useState<string[]>([]);
  const [loadingModels, setLoadingModels] = useState(false);
  const [endpointNotSupported, setEndpointNotSupported] = useState(false);
  const [modelsLoaded, setModelsLoaded] = useState(false);

  // Agente de código: o formulário deixa de pedir URL, chave e modelo e passa a
  // pedir o comando que sobe o agente (AEP-0084 D12).
  const isAgent = isAgentForm(formData);

  // Referências estáveis: os campos do agente detectam a instalação em um efeito
  // e um callback recriado a cada render disparia detecção sem parar.
  const handleAgentCommandChange = useCallback((command: string) => {
    setFormData((prev) => ({ ...prev, acp_command: command }));
    setErrors((prev) => {
      if (!prev.acp_command) return prev;
      const next = { ...prev };
      delete next.acp_command;
      return next;
    });
  }, []);

  const handleAgentArgsChange = useCallback((args: string[]) => {
    setFormData((prev) => ({ ...prev, acp_args: args }));
  }, []);

  /**
   * Troca o agente do provedor. Comando e argumentos vão junto: eles descrevem
   * como subir o agente anterior, e mantê-los faria o provedor dizer que é um
   * agente enquanto executa outro. Quem escolhe o mesmo agente de novo não perde
   * o que estava configurado — não houve troca nenhuma.
   *
   * O nome também acompanha, enquanto ninguém o tiver escrito: o formulário
   * abre vazio, e obrigar a digitar "Gemini CLI" logo depois de escolher Gemini
   * CLI numa lista é trabalho que a tela já tem como poupar.
   */
  const handleAgentPick = useCallback((agent: CatalogAgent) => {
    setFormData((prev) => {
      if (prev.acp_agent_id === agent.id) return prev;
      return {
        ...prev,
        acp_agent_id: agent.id,
        acp_command: '',
        acp_args: [],
        name: prev.name.trim() === '' ? agent.name : prev.name,
      };
    });
    setErrors({});
  }, []);

  const loadModels = useCallback(async (overrideData?: Partial<ProviderFormData>) => {
    const data = { ...formData, ...overrideData };
    const config = PROVIDER_CONFIG[data.type] || PROVIDER_CONFIG.custom;
    const canonicalUrl = !config.urlEditable ? config.defaultUrl : data.base_url;

    // Agente não tem endpoint de modelos: a lista dele vem da sessão ACP, que é
    // outra fase. Bater aqui só produziria um erro de URL vazia.
    if (isAgentForm(data)) return;
    if (!canonicalUrl.trim()) return;

    setLoadingModels(true);
    setEndpointNotSupported(false);
    setModels([]);
    setModelsLoaded(false);
    setErrors(prev => {
      const ne = { ...prev };
      delete ne.api;
      delete ne.api_key;
      delete ne.base_url;
      return ne;
    });

    try {
      const result = await withTimeout(
        ListModelsRaw({
          type: data.type,
          base_url: canonicalUrl,
          api_key: data.api_key || undefined,
          provider_id: data.id || undefined,
        }),
        15000,
        'ListModelsRaw'
      );

      setModels(result || []);
      setModelsLoaded(true);
      setApiTested(true);

      // Se não tinha modelo selecionado, seleciona o default do provider config
      if (!data.default_model) {
        const suggested = config.defaultModel;
        if (suggested && (result || []).includes(suggested)) {
          setFormData(prev => ({ ...prev, default_model: suggested }));
        }
      }
    } catch (error: unknown) {
      const err = error as { message?: unknown } | null;
      const errorMsg = String(err?.message || error || '');

      if (errorMsg.includes('models_endpoint_not_supported')) {
        setEndpointNotSupported(true);
        setModelsLoaded(true);
        setApiTested(true);
        // Sugere o modelo default se não tem um selecionado
        if (!data.default_model && config.defaultModel) {
          setFormData(prev => ({ ...prev, default_model: config.defaultModel }));
        }
      } else {
        setApiTested(false);
        setModelsLoaded(false);
        setErrors(prev => ({
          ...prev,
          api: String(err?.message || error || t('providerForm.error.testError')),
        }));
      }
    } finally {
      setLoadingModels(false);
    }
  }, [formData, t]);

  useEffect(() => {
    if (provider) {
      const provConfig = PROVIDER_CONFIG[provider.type] || PROVIDER_CONFIG.custom;
      setFormData({
        id: provider.id,
        name: provider.name,
        type: provider.type,
        base_url: provider.base_url,
        api_key: '',
        default_model: provider.default_model || '',
        api_format: provider.api_format ?? provConfig.apiFormat ?? '',
        acp_command: provider.acp_command || '',
        acp_args: provider.acp_args || [],
        acp_agent_id: provider.acp_agent_id || '',
      });
      setApiTested(false);
      setShowApiKeyField(false);
      setApiKeyChangedInThisSession(false);
      setModels([]);
      setModelsLoaded(false);
      setEndpointNotSupported(false);
    } else {
      const defaultType = 'openai';
      const config = PROVIDER_CONFIG[defaultType] || PROVIDER_CONFIG.custom;
      setFormData({
        name: '',
        type: defaultType,
        base_url: config.defaultUrl,
        api_key: '',
        api_format: config.apiFormat || '',
        acp_command: '',
        acp_args: [],
        acp_agent_id: '',
      });
      setApiTested(false);
      setShowApiKeyField(true);
      setApiKeyChangedInThisSession(false);
      setModels([]);
      setModelsLoaded(false);
      setEndpointNotSupported(false);
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

  // Auto-carrega modelos ao abrir provider existente (edição)
  useEffect(() => {
    if (provider && !modelsLoaded && !loadingModels) {
      loadModels({
        id: provider.id,
        type: provider.type,
        base_url: provider.base_url,
        default_model: provider.default_model,
        api_format: provider.api_format,
      });
    }
  }, [provider]);

  /**
   * Recoloca no formulário a configuração do provedor salvo, como a carga
   * inicial faz. O nome fica como está — quem renomeou não pediu para desfazer
   * isso — e a chave digitada nesta sessão também, porque é o único dado do
   * formulário que ainda não existe em lugar nenhum.
   */
  const restoreSavedProvider = () => {
    if (!provider) return;
    const savedConfig = PROVIDER_CONFIG[provider.type] || PROVIDER_CONFIG.custom;
    setFormData((prev) => ({
      ...prev,
      type: provider.type,
      api_format: provider.api_format ?? savedConfig.apiFormat ?? '',
      base_url: provider.base_url,
      default_model: provider.default_model || '',
      api_key: apiKeyChangedInThisSession ? prev.api_key : '',
      acp_command: provider.acp_command || '',
      acp_args: provider.acp_args || [],
      acp_agent_id: provider.acp_agent_id || '',
    }));
    setErrors({});
    setApiTested(false);
    setModels([]);
    setModelsLoaded(false);
    setEndpointNotSupported(false);
    setShowApiKeyField(apiKeyChangedInThisSession);
  };

  /**
   * Trocar o tipo é passar a configurar outra coisa, então o preset do novo tipo
   * passa a valer inteiro — inclusive o `api_format`, que é quem decide a forma
   * do formulário e o caminho de gravação.
   *
   * Isto vive no handler, e não em um efeito de `formData.type`, porque efeito
   * não distingue "a pessoa trocou o tipo" de "o formulário acabou de carregar o
   * provedor salvo". Era por não distinguir que a sincronia precisava ficar de
   * fora da edição (para não sobrescrever a URL salva ao abrir a tela) — e com
   * ela de fora, editar um agente e escolher um tipo HTTP deixava o formulário
   * na forma de agente gravando por um pipeline que discorda do tipo escolhido.
   *
   * A exceção é voltar ao tipo do provedor salvo: aí a configuração existe, e o
   * preset não passa de um palpite sobre ela. Quem troca o tipo e desiste tem de
   * encontrar de volta o que estava salvo — a URL customizada, o comando do
   * agente —, e não o padrão do preset gravado como se nada tivesse acontecido.
   */
  const handleTypeChange = (nextType: string) => {
    const config = PROVIDER_CONFIG[nextType] || PROVIDER_CONFIG.custom;
    const nextIsAgent = (config.apiFormat || '') === AGENT_API_FORMAT;
    const leavingAgent = isAgent && !nextIsAgent;

    if (provider && nextType === provider.type) {
      restoreSavedProvider();
      return;
    }

    setFormData((prev) => ({
      ...prev,
      type: nextType,
      api_format: config.apiFormat || '',
      base_url: config.defaultUrl,
      default_model: '',
      // O que não pertence ao novo tipo não fica pendurado: agente não tem
      // credencial no app, e provedor HTTP não tem comando para subir.
      api_key: nextIsAgent ? '' : prev.api_key,
      acp_command: '',
      acp_args: [],
      acp_agent_id: '',
    }));
    // Erros descrevem a forma anterior do formulário; a validação do submit
    // recalcula o que ainda valer.
    setErrors({});
    setApiTested(false);
    setModels([]);
    setModelsLoaded(false);
    setEndpointNotSupported(false);
    if (leavingAgent) {
      // Um agente não guardou credencial nenhuma, então o botão "alterar chave"
      // mentiria dizendo que já existe uma configurada.
      setShowApiKeyField(true);
      setApiKeyChangedInThisSession(false);
    }
  };

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
    // Trim defensivo: copy/paste de chaves frequentemente arrasta
    // espaco/quebra-de-linha invisível no inicio ou fim, o que quebra
    // o header Authorization no upstream e gera 400 sem motivo claro.
    handleChange('api_key', value.trim());
    setApiKeyChangedInThisSession(true);
    setApiTested(false);
    setModels([]);
    setModelsLoaded(false);
    setEndpointNotSupported(false);
  };

  const handleLoadModels = async () => {
    const config = PROVIDER_CONFIG[formData.type] || PROVIDER_CONFIG.custom;
    const canonicalUrl = getCanonicalUrl(formData.type);

    // Validar URL antes de carregar
    if (!canonicalUrl.trim()) {
      setErrors((prev) => ({ ...prev, base_url: t('providerForm.error.urlRequired') }));
      return;
    }
    try {
      const parsed = new URL(canonicalUrl);
      if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
        setErrors((prev) => ({ ...prev, base_url: t('providerForm.error.urlInvalid') }));
        return;
      }
    } catch {
      setErrors((prev) => ({ ...prev, base_url: t('providerForm.error.urlInvalid') }));
      return;
    }

    // Se requer key, está criando (ou alterou a key), e a key está vazia → erro
    const isEditingWithExistingKey = !!formData.id && !apiKeyChangedInThisSession;
    if (config.testRequiresApiKey && !formData.api_key.trim() && !isEditingWithExistingKey) {
      setErrors((prev) => ({
        ...prev,
        api_key: t('providerForm.error.apiKeyRequiredTest'),
      }));
      return;
    }

    await loadModels();
  };

  const handleApiKeyBlur = async () => {
    const config = PROVIDER_CONFIG[formData.type] || PROVIDER_CONFIG.custom;
    const canonicalUrl = getCanonicalUrl(formData.type);
    
    if (!canonicalUrl.trim()) return;

    // Se não requer key e a key está vazia, carrega modelos mesmo assim (ex: Ollama)
    if (!config.apiKeyRequired && !formData.api_key.trim()) {
      await handleLoadModels();
      return;
    }

    // Se requer key, está criando, e key está vazia → não auto-carrega
    const isEditingWithExistingKey = !!formData.id && !apiKeyChangedInThisSession;
    if (config.testRequiresApiKey && !formData.api_key.trim() && !isEditingWithExistingKey) {
      return;
    }

    await handleLoadModels();
  };

  const validate = (): boolean => {
    const newErrors: Record<string, string> = {};
    const config = PROVIDER_CONFIG[formData.type] || PROVIDER_CONFIG.custom;

    if (!formData.name.trim()) {
      newErrors.name = t('providerForm.error.nameRequired');
    }

    if (isAgent) {
      // O que endereça um agente é o comando; URL, chave e teste de modelos não
      // se aplicam. Salvar sem ter conseguido testar é permitido de propósito:
      // um agente instalado e ainda sem login precisa poder ser cadastrado, e é
      // o diagnóstico que explica o que falta.
      if (!(formData.acp_command || '').trim()) {
        newErrors.acp_command = t('providerForm.agent.error.commandRequired');
      }
      setErrors(newErrors);
      return Object.keys(newErrors).length === 0;
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
    // Na edição, a key já está salva no credential manager, não precisa re-informar
    const isEditing = !!formData.id;
    if (config && config.apiKeyRequired && !formData.api_key.trim() && !isEditing) {
      newErrors.api_key = t('providerForm.error.apiKeyRequired') + ` ${config.label}`;
    }

    // Exige carregamento de modelos (que valida a conexão)
    if (!apiTested) {
      newErrors.api = t('providerForm.error.testFirst');
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  // saveAgentProvider grava um provedor de agente de código. Nada de base_url
  // nem api_key: o backend recusa credencial para um agente, e mandar URL vazia
  // junto com o formato acp é o contrato que ele espera (AEP-0084 D12). O modelo
  // padrão também fica de fora — a lista de modelos de um agente vem da sessão.
  const saveAgentProvider = async () => {
    const command = (formData.acp_command || '').trim();
    const args = formData.acp_args || [];
    const agentId = (formData.acp_agent_id || '').trim();
    if (formData.id) {
      await withTimeout(
        UpdateLLMProvider(formData.id, {
          name: formData.name,
          type: formData.type,
          api_format: AGENT_API_FORMAT,
          acp_command: command,
          acp_args: args,
          acp_agent_id: agentId,
        }),
        15000,
        'UpdateLLMProvider'
      );
      return;
    }
    await withTimeout(
      CreateLLMProvider({
        // O identificador começa pelo agente quando há um: um provedor chamado
        // `acp-...` não diria qual agente é, e quem olha a lista de provedores
        // ou um log precisa disso.
        id: `${agentId || formData.type}-${Date.now()}`,
        name: formData.name,
        type: formData.type,
        base_url: '',
        api_format: AGENT_API_FORMAT,
        acp_command: command,
        acp_args: args,
        acp_agent_id: agentId,
      }),
      15000,
      'CreateLLMProvider'
    );
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!validate()) return;

    setSaving(true);
    try {
      // IMPORTANTE: Sempre usa a URL canônica ao salvar
      // Isso garante que URLs incorretas (ex: Google incompleto) sejam corrigidas automaticamente
      const canonicalUrl = getCanonicalUrl(formData.type);

      if (isAgent) {
        await saveAgentProvider();
        onSave();
        return;
      }

      if (formData.id) {
        // Update
        await withTimeout(
          UpdateLLMProvider(formData.id, {
            name: formData.name,
            type: formData.type,
            base_url: canonicalUrl,
            api_key: formData.api_key || undefined,
            default_model: formData.default_model || undefined,
            api_format: formData.api_format || undefined,
          }),
          15000,
          'UpdateLLMProvider'
        );
      } else {
        // Create - gera ID único
        const suggestedDefault = PROVIDER_CONFIG[formData.type]?.defaultModel;
        await withTimeout(
          CreateLLMProvider({
            id: `${formData.type}-${Date.now()}`,
            name: formData.name,
            type: formData.type,
            base_url: canonicalUrl,
            api_key: formData.api_key || undefined,
            default_model: formData.default_model || suggestedDefault || undefined,
            api_format: formData.api_format || undefined,
          }),
          15000,
          'CreateLLMProvider'
        );
      }

      onSave();
    } catch (error: unknown) {
      const err = error as { message?: unknown; toString?: () => string } | null;
      const message = String(err?.message || err?.toString?.() || error || t('providerForm.error.saveError'));
      setErrors({ submit: message });
      announce(message, 'assertive');
    } finally {
      setSaving(false);
    }
  };

  const config = PROVIDER_CONFIG[formData.type] || PROVIDER_CONFIG.custom;
  const isUrlReadonly = !config.urlEditable;
  const requiresApiKey = config.apiKeyRequired;
  const testRequiresApiKey = config.testRequiresApiKey;
  const canLoadModels = (() => {
    const canonicalUrl = getCanonicalUrl(formData.type);
    if (!canonicalUrl.trim()) return false;
    if (testRequiresApiKey && !formData.api_key.trim()) {
      // Permitir quando editando com credencial existente
      return !!formData.id && !apiKeyChangedInThisSession;
    }
    return true;
  })();

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
          options={tiposDeProvedor}
          value={formData.type}
          onChange={(e) => handleTypeChange(e.target.value)}
          fullWidth
        />
      </FormField>

      {isAgent ? (
        <>
          <AgentPicker
            agentId={formData.acp_agent_id || ''}
            onPick={handleAgentPick}
          />
          <AgentProviderFields
            agentId={formData.acp_agent_id || ''}
            command={formData.acp_command || ''}
            args={formData.acp_args || []}
            onCommandChange={handleAgentCommandChange}
            onArgsChange={handleAgentArgsChange}
            commandError={errors.acp_command}
            // Na edição o comando salvo é a escolha de quem configurou e não se
            // toca — mas se o agente foi trocado, não há nada salvo para o novo,
            // e deixar o campo vazio faria a pessoa procurar o caminho na mão
            // sem motivo. Voltar ao agente salvo cai no primeiro caso: o comando
            // restaurado é o que vale, e a detecção só informa o que existe na
            // máquina.
            autoFill={!formData.id || (formData.acp_agent_id || '') !== (provider?.acp_agent_id || '')}
          />
        </>
      ) : (
        <>
      <FormField
        label={t('providerForm.apiProtocol')}
        description={t('providerForm.apiProtocolHelp')}
      >
        <Select
          options={API_FORMAT_OPTIONS}
          value={formData.api_format || ''}
          onChange={(e) => setFormData(prev => ({ ...prev, api_format: e.target.value }))}
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
        />
        {isUrlReadonly && (
          <span className="provider-form__read-only-note">
            {t('providerForm.urlReadonly')}
          </span>
        )}
      </FormField>

      {/* API Key Field */}
      {requiresApiKey ? (
        <FormField
          label={t('providerForm.apiKey')}
          required
          error={errors.api_key}
          description={
            formData.id && !showApiKeyField
              ? t('providerForm.keyConfigured')
              : formData.id && showApiKeyField
              ? t('providerForm.keepCurrent')
              : t('providerForm.keySaved')
          }
        >
          {formData.id && !showApiKeyField ? (
            <Button
              type="button"
              variant="secondary"
              onClick={() => setShowApiKeyField(true)}
              className="provider-form__change-key-button"
              aria-label={t('providerForm.changeKey')}
            >
              {t('providerForm.changeKeyBtn')}
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
              aria-label={t('providerForm.apiKey')}
            />
            <Button
              type="button"
              variant="secondary"
              onClick={() => setShowPassword(!showPassword)}
              className="provider-form__toggle-password"
              aria-label={showPassword ? t('providerForm.hideKey') : t('providerForm.showKey')}
              aria-pressed={showPassword}
            >
              <span aria-hidden="true">{showPassword ? <EyeInvisibleOutlined /> : <EyeOutlined />}</span>
            </Button>
          </div>
          )}
        </FormField>
      ) : (
        <FormField
          label={t('providerForm.apiKeyOptional')}
          error={errors.api_key}
          description={
            formData.id && !showApiKeyField
              ? t('providerForm.keyConfiguredOptional')
              : t('providerForm.noKeyNeeded')
          }
        >
          {formData.id && !showApiKeyField ? (
            <Button
              type="button"
              variant="secondary"
              onClick={() => setShowApiKeyField(true)}
              className="provider-form__change-key-button"
              aria-label={t('providerForm.changeKey')}
            >
              {t('providerForm.changeKeyBtn')}
            </Button>
          ) : (
          <div className="provider-form__password-field">
            <Input
              ref={apiKeyInputRef}
              type={showPassword ? 'text' : 'password'}
              value={formData.api_key}
              onChange={(e) => handleApiKeyChange(e.target.value)}
              onBlur={handleApiKeyBlur}
              placeholder={t('providerForm.leaveEmpty')}
              fullWidth
              aria-label={t('providerForm.apiKeyOptional')}
            />
            <Button
              type="button"
              variant="secondary"
              onClick={() => setShowPassword(!showPassword)}
              className="provider-form__toggle-password"
              aria-label={showPassword ? t('providerForm.hideKey') : t('providerForm.showKey')}
              aria-pressed={showPassword}
            >
              <span aria-hidden="true">{showPassword ? <EyeInvisibleOutlined /> : <EyeOutlined />}</span>
            </Button>
          </div>
          )}
        </FormField>
      )}

      {/* Default Model — loads models list which also validates the provider */}
      <FormField
        label={t('providerForm.defaultModel')}
        error={errors.api}
        description={
          modelsLoaded && apiTested
            ? t('providerForm.connected')
            : loadingModels
            ? t('providerForm.loadingModels')
            : t('providerForm.defaultModelHelp')
        }
      >
        {modelsLoaded && models.length > 0 ? (
          <Select
            options={[
              { value: '', label: t('providerForm.modelAutomatic') },
              ...models.map(m => ({ value: m, label: m })),
            ]}
            value={formData.default_model || ''}
            onChange={(e) => setFormData(prev => ({ ...prev, default_model: e.target.value }))}
            fullWidth
            aria-label={t('providerForm.defaultModel')}
          />
        ) : modelsLoaded && endpointNotSupported ? (
          <Input
            value={formData.default_model || ''}
            onChange={(e) => setFormData(prev => ({ ...prev, default_model: e.target.value }))}
            placeholder={config.defaultModel || t('providerForm.modelPlaceholder')}
            fullWidth
            aria-label={t('providerForm.defaultModel')}
          />
        ) : (
          <div className="provider-form__model-load-section">
            <Button
              type="button"
              variant="secondary"
              onClick={handleLoadModels}
              disabled={loadingModels || !canLoadModels}
              aria-label={t('providerForm.loadModels')}
            >
              {loadingModels ? t('providerForm.loadingModels') : t('providerForm.loadModelsBtn')}
            </Button>
            {testRequiresApiKey && !formData.api_key.trim() && showApiKeyField && !formData.id && (
              <span className="provider-form__hint">
                {t('providerForm.fillApiKey')}
              </span>
            )}
          </div>
        )}
      </FormField>
        </>
      )}

      {errors.submit && (
        <div className="provider-form__error">
          <WarningOutlined aria-hidden="true" /> {errors.submit}
        </div>
      )}

      <div className="provider-form__actions">
        <Button type="button" variant="secondary" onClick={onCancel}>
          {t('common.cancel')}
        </Button>
        <Button
          type="submit"
          variant="primary"
          disabled={saving || (!isAgent && !apiTested)}
          title={!isAgent && !apiTested ? t('providerForm.error.testFirst') : undefined}
        >
          {saving
            ? t('common.saving')
            : formData.id
              ? t('providerForm.updateBtn')
              : t('common.create')}
        </Button>
      </div>
    </form>
  );
};
