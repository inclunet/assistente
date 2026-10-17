import { apidto } from '@wailsjs/go/models';
import { useState, useEffect, useLayoutEffect, useRef, useCallback, useMemo } from 'react';
import { WarningOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { CreateLLMProvider, UpdateLLMProvider, ListModelsRaw } from '@wailsjs/go/wailsapi/LLMProviders';
import { GetLLMCatalog } from '@wailsjs/go/wailsapi/LLMCatalog';
import type { llmcatalog } from '@wailsjs/go/models';
import { Input, Select, Button, FormField } from '../';
import { DialogActions } from '../ui/DialogActions';
import { AGENT_API_FORMAT, PROVIDER_CONFIG } from '../../config/providers';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import type { CatalogAgent } from './ACPAgentCatalog';
import { AgentPicker } from './AgentPicker';
import { AgentProviderFields } from './AgentProviderFields';
export { PROVIDER_CONFIG } from '../../config/providers';
import { ResourceCredentialEditor } from '../credentials/ResourceCredentialEditor';
import { credentialInput, validateCredential, type CredentialDraft } from '../credentials/credentialDraft';
import './ProviderForm.css';
const HTTP_CREDENTIAL_TYPES = ['bearer', 'basic', 'custom'];
const defaultAuthMode = (type: string) => ['ollama', 'llamacpp'].includes(type) ? 'none' : type === 'localai' ? 'optional' : 'required';
const sameOrigin = (a: string, b: string) => { try { return new URL(a).origin === new URL(b).origin; } catch { return false; } };

const PROFILE_FAMILY_TO_RUNTIME_TYPE: Record<string, string> = {
  openai: 'openai', anthropic: 'anthropic', google: 'google', xai: 'xai',
  deepseek: 'deepseek', mistral: 'mistral', groq: 'groq', together: 'together',
  fireworks: 'fireworks', perplexity: 'perplexity', cohere: 'cohere', zai: 'custom',
  openrouter: 'openrouter', litellm: 'litellm', localai: 'localai', ollama: 'ollama',
  llamacpp: 'llamacpp', custom: 'custom',
};
const API_FORMAT_TO_RUNTIME: Record<string, string> = {
  openai_responses: 'openai_responses', openai: 'openai', anthropic: 'anthropic', google: 'google',
};
const AUTH_MODE_TO_RUNTIME: Record<string, string> = {
  bearer_api_key: 'required', api_key_header: 'required', optional_api_key: 'optional', none: 'none',
};
type FieldPolicy = llmcatalog.FieldPolicy;
type ServerProfile = llmcatalog.Server;
const allowlisted = (values: Record<string, string>, key: string): string | undefined =>
  Object.prototype.hasOwnProperty.call(values, key) ? values[key] : undefined;

const policyFor = (profile: ServerProfile | undefined, field: string): FieldPolicy | undefined => profile?.fields?.[field];
const policyValue = (profile: ServerProfile, field: string, fallback: unknown): unknown => {
  const policy = policyFor(profile, field);
  if (policy?.policy === 'fixed' || policy?.policy === 'hidden' || policy?.policy === 'choice') return policy.default ?? fallback;
  const hasFallback = fallback !== undefined && fallback !== null && fallback !== '';
  return hasFallback ? fallback : policy?.default ?? fallback;
};

const customProfile = (): ServerProfile => ({
  id: '', name: '', group: 'custom', provider_family: 'custom', status: 'active',
  api_format: 'openai', adapter_id: 'openai', base_url: '',
  auth: { mode: 'optional_api_key', required: false },
  fields: {
    base_url: { policy: 'editable' }, api_format: { policy: 'editable' },
    'auth.mode': { policy: 'editable' }, default_model_id: { policy: 'editable' },
  },
});

const runtimeProfileValues = (profile: ServerProfile) => {
  const runtimeType = allowlisted(PROFILE_FAMILY_TO_RUNTIME_TYPE, profile.provider_family);
  const rawFormat = policyValue(profile, 'api_format', profile.api_format);
  // Ollama keeps its existing runtime adapter. The catalog's native `ollama`
  // protocol is never copied into the persisted API format; runtime still uses
  // the format already supported by the app for its Ollama adapter.
  const isOllamaAdapter = profile.id === 'ollama' && profile.adapter_id === 'ollama'
    && rawFormat === 'ollama' && runtimeType === 'ollama';
  const apiFormat = isOllamaAdapter
    ? (PROVIDER_CONFIG.ollama.apiFormat || 'openai')
    : typeof rawFormat === 'string' ? allowlisted(API_FORMAT_TO_RUNTIME, rawFormat) : undefined;
  const rawAuthMode = policyValue(profile, 'auth.mode', profile.auth?.mode);
  const authMode = typeof rawAuthMode === 'string' ? allowlisted(AUTH_MODE_TO_RUNTIME, rawAuthMode) : undefined;
  if (!runtimeType || !apiFormat || !authMode) return null;
  const rawBaseURL = policyValue(profile, 'base_url', profile.base_url || '');
  const baseURL = typeof rawBaseURL === 'string' ? rawBaseURL : '';
  return { runtimeType, apiFormat, authMode, baseURL };
};

export interface ProviderFormData {
  id?: string;
  name: string;
  type: string;
  base_url: string;
  api_key: string;
  auth_mode?: string;
  credential_pattern?: string;
  default_model?: string;
  api_format?: string;
  server_profile_id?: string;
  reasoning_content_mode?: string;
  /** Comando e argumentos do agente de código, quando o formato é acp. */
  acp_command?: string;
  acp_args?: string[];
  /**
   * Qual agente do registro ACP é este provedor (AEP-0086 D11). Vazio é agente
   * apontado à mão: os campos de comando continuam valendo, e o que depende de
   * saber qual agente é não tem o que oferecer.
   */
  acp_agent_id?: string;
  /**
   * Variáveis de ambiente do processo do agente (AEP-0084 D12 / AEP-0086).
   * Inclui o `env{}` do alvo binário instalado pelo catálogo.
   */
  acp_env?: Record<string, string>;
  /**
   * Quais variáveis do ambiente do agente recebem credencial do cofre, e de
   * qual entrada dele (AEP-0086 D12). O que trafega é a referência; o segredo
   * fica no cofre e só sai na hora de subir o agente.
   */
  acp_credential_env?: Record<string, string>;
}

export interface ProviderFormProps {
  provider?: ProviderFormData;
  kind?: 'api' | 'acp';
  onSave: () => void;
  onCancel: () => void;
  onBusyChange?: (busy: boolean) => void;
}

// Provider configuration is imported from '../../config/providers'
// ProviderPreset type is used internally via PROVIDER_CONFIG

// Generate provider types for dropdown
const providerTypes = (t: TFunction, agent: boolean) =>
  Object.entries(PROVIDER_CONFIG).filter(([, config]) => (config.apiFormat === AGENT_API_FORMAT) === agent).map(([key, config]) => ({
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

const reasoningContentModeOptions = (t: TFunction) => [
  { value: 'disabled', label: t('providerForm.reasoningContentDisabled') },
  { value: 'replay_with_tools', label: t('providerForm.reasoningContentReplayWithTools') },
];

/**
 * Diz se estes dados descrevem um agente de código local. Vem do formato porque
 * é ele que o backend usa para decidir, e um provedor já salvo carrega o dele
 * mesmo que o preset do tipo mude depois.
 */
const isAgentForm = (data: Pick<ProviderFormData, 'type' | 'api_format'>): boolean =>
  (data.api_format || PROVIDER_CONFIG[data.type]?.apiFormat || '') === AGENT_API_FORMAT;

// Mirror normalizeProviderACP: historical/custom types with ACP format use the
// single ACP preset, preserving their command, arguments and vault references.
const canonicalFormType = (data: Pick<ProviderFormData, 'type' | 'api_format'>) =>
  isAgentForm(data) ? 'acp' : data.type;

export const ProviderForm = ({ provider, kind = provider && isAgentForm(provider) ? 'acp' : 'api', onSave, onCancel, onBusyChange }: ProviderFormProps) => {
  const { t, i18n } = useTranslation();
  const { announce } = useAnnouncer();
  const [catalogProfiles, setCatalogProfiles] = useState<ServerProfile[] | null>(null);
  const catalogDefaultSuppressed = useRef(false);
  useEffect(() => {
    if (kind === 'acp') return;
    let active = true;
    void Promise.resolve().then(() => GetLLMCatalog()).then((response) => {
      if (active) setCatalogProfiles(response?.document?.servers || []);
    }).catch(() => {
      if (active) setCatalogProfiles([]);
    });
    return () => { active = false; };
  }, [kind]);
  const availableProfiles = useMemo(() => {
    const source = catalogProfiles === null ? [] : catalogProfiles;
    // Cohere anuncia uma rota com prefixo que o runtime atual não acrescenta
    // ao base_url; oferecê-lo como funcional geraria configuração inválida.
    const safe = source.filter(profile => profile.id !== 'cohere'
      && profile.status === 'active' && runtimeProfileValues(profile));
    if (catalogProfiles === null || catalogProfiles.length === 0) return [customProfile()];
    return [...safe, customProfile()];
  }, [catalogProfiles]);
  const catalogUnavailable = kind !== 'acp' && catalogProfiles !== null && catalogProfiles.length === 0;
  // i18n.language garante recomputo ao trocar de idioma
  const tiposDeProvedor = useMemo(() => providerTypes(t, kind === 'acp'), [t, i18n.language, kind]);
  const [formData, setFormData] = useState<ProviderFormData>({
    name: '',
    type: kind === 'acp' ? 'acp' : 'openai',
    base_url: '',
    api_key: '',
    api_format: kind === 'acp' ? AGENT_API_FORMAT : PROVIDER_CONFIG.openai.apiFormat || '',
    reasoning_content_mode: PROVIDER_CONFIG.openai.reasoningContentMode || 'disabled',
  });
  const selectedProfile = useMemo(() => {
    const profileId = formData.server_profile_id ?? provider?.server_profile_id;
    return availableProfiles.find(profile => profile.id === profileId) || customProfile();
  }, [availableProfiles, formData.server_profile_id, provider?.server_profile_id]);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const savingStatus = useRef<HTMLParagraphElement>(null);
  const saveReturnFocus = useRef<HTMLElement | null>(null);
  const wasSaving = useRef(false);
  useLayoutEffect(() => {
    if (saving) savingStatus.current?.focus();
    else if (wasSaving.current && saveReturnFocus.current?.isConnected && !saveReturnFocus.current.matches(':disabled')) saveReturnFocus.current.focus();
    wasSaving.current = saving;
  }, [saving]);
  const [apiTested, setApiTested] = useState(false);

  const [credentialDraft, setCredentialDraft] = useState<CredentialDraft | null>(null);
  const loadSequence = useRef(0);
  const formSequence = useRef(0);
  const busyCallback = useRef(onBusyChange);
  busyCallback.current = onBusyChange;
  useEffect(() => { busyCallback.current?.(saving); }, [saving]);
  useEffect(() => () => { loadSequence.current++; formSequence.current++; busyCallback.current?.(false); }, []);
  const invalidatePreview = useCallback(() => {
    loadSequence.current++;
    setApiTested(false);
    setLoadingModels(false);
    setModelsLoaded(false);
    setModels([]);
  }, []);
  const handleCredentialChange = useCallback((draft: CredentialDraft | null) => {
    setCredentialDraft(draft);
    invalidatePreview();
  }, [invalidatePreview]);

  // Model loading states
  const [models, setModels] = useState<string[]>([]);
  const [loadingModels, setLoadingModels] = useState(false);
  const [endpointNotSupported, setEndpointNotSupported] = useState(false);
  const [modelsLoaded, setModelsLoaded] = useState(false);

  // Agente de código: o formulário deixa de pedir URL, chave e modelo e passa a
  // pedir o comando que sobe o agente (AEP-0084 D12).
  const isAgent = isAgentForm(formData);
  // A escolha explícita é também o pedido para resolver o agente. O número
  // distingue duas escolhas seguidas do mesmo item sem guardar estado no
  // catálogo nem confundir a abertura de um provedor salvo com uma escolha.
  const [agentSelectionToken, setAgentSelectionToken] = useState(0);

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

  const handleAgentEnvChange = useCallback((env: Record<string, string>) => {
    // Base = env da instalação; chaves que a pessoa já tinha no formulário
    // vencem — não sobrescrever configuração manual no merge.
    setFormData((prev) => ({
      ...prev,
      acp_env: { ...env, ...(prev.acp_env || {}) },
    }));
  }, []);

  const handleCredentialEnvChange = useCallback((credentialEnv: Record<string, string>) => {
    setFormData((prev) => ({ ...prev, acp_credential_env: credentialEnv }));
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
    setAgentSelectionToken((token) => token + 1);
    setFormData((prev) => {
      if (prev.acp_agent_id === agent.id) return prev;
      return {
        ...prev,
        acp_agent_id: agent.id,
        acp_command: '',
        acp_args: [],
        // A passagem de credencial descrevia o agente anterior: a variável que
        // o Cursor lê não é a que o Gemini CLI lê, e mantê-la entregaria a
        // chave a um programa que ninguém escolheu para recebê-la.
        acp_credential_env: {},
        acp_env: {},
        name: prev.name.trim() === '' ? agent.name : prev.name,
      };
    });
    setErrors({});
  }, []);

  const loadModels = useCallback(async (overrideData?: Partial<ProviderFormData>) => {
    const data = { ...formData, ...overrideData };
    const config = PROVIDER_CONFIG[data.type] || PROVIDER_CONFIG.custom;
    const profile = availableProfiles.find(item => item.id === data.server_profile_id);
    const catalogDefaultModel = typeof profile?.default_model_id === 'string' ? profile.default_model_id : undefined;
    const policy = policyFor(profile, 'base_url');
    const configuredURL = profile && (policy?.policy === 'fixed' || policy?.policy === 'hidden')
      ? policyValue(profile, 'base_url', profile.base_url)
      : data.base_url;
    const canonicalUrl = provider && data.server_profile_id === provider.server_profile_id
      ? data.base_url
      : typeof configuredURL === 'string' ? configuredURL : (!config.urlEditable ? config.defaultUrl : data.base_url);

    // Agente não tem endpoint de modelos: a lista dele vem da sessão ACP, que é
    // outra fase. Bater aqui só produziria um erro de URL vazia.
    if (isAgentForm(data)) return;
    if (!canonicalUrl.trim()) return;

    const sequence = ++loadSequence.current;
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
      const result = await ListModelsRaw(apidto.TestLLMProviderRequest.createFrom({
          type: data.type,
          base_url: canonicalUrl,
          credential: credentialDraft ? credentialInput(credentialDraft) : undefined,
          auth_mode: data.auth_mode || defaultAuthMode(data.type),
          api_format: data.api_format || undefined,
          provider_id: data.id || undefined,
        }));

      if (sequence !== loadSequence.current) return;
      setModels(result || []);
      setModelsLoaded(true);
      setApiTested(true);

      // Se não tinha modelo selecionado, prefere o default do perfil escolhido.
      if (!data.default_model) {
        const suggested = catalogDefaultModel || config.defaultModel;
        if (suggested && (result || []).includes(suggested)) {
          setFormData(prev => ({ ...prev, default_model: suggested }));
        }
      }
    } catch (error: unknown) {
      if (sequence !== loadSequence.current) return;
      const err = error as { message?: unknown } | null;
      const errorMsg = String(err?.message || error || '');

      if (errorMsg.includes('models_endpoint_not_supported')) {
        setEndpointNotSupported(true);
        setModelsLoaded(true);
        setApiTested(true);
        // Sugere o modelo default se não tem um selecionado
        if (!data.default_model && (catalogDefaultModel || config.defaultModel)) {
          setFormData(prev => ({ ...prev, default_model: catalogDefaultModel || config.defaultModel }));
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
      if (sequence === loadSequence.current) setLoadingModels(false);
    }
  }, [formData, credentialDraft, t, provider, availableProfiles]);

  useEffect(() => {
    catalogDefaultSuppressed.current = false;
    setAgentSelectionToken(0);
    formSequence.current++;
    setSaving(false);
    loadSequence.current++;
    setCredentialDraft(null);
    setLoadingModels(false);
    if (provider) {
      const provConfig = PROVIDER_CONFIG[provider.type] || PROVIDER_CONFIG.custom;
      setFormData({
        id: provider.id,
        name: provider.name,
        type: canonicalFormType(provider),
        base_url: provider.base_url,
        api_key: '',
        auth_mode: provider.auth_mode || defaultAuthMode(provider.type),
        credential_pattern: provider.credential_pattern,
        default_model: provider.default_model || '',
        api_format: provider.api_format ?? provConfig.apiFormat ?? '',
        server_profile_id: provider.server_profile_id,
        reasoning_content_mode: provider.reasoning_content_mode
          ?? provConfig.reasoningContentMode
          ?? 'disabled',
        acp_command: provider.acp_command || '',
        acp_args: provider.acp_args || [],
        acp_agent_id: provider.acp_agent_id || '',
        acp_env: provider.acp_env || {},
        acp_credential_env: provider.acp_credential_env || {},
      });
      setApiTested(false);
      setModels([]);
      setModelsLoaded(false);
      setEndpointNotSupported(false);
    } else {
      const defaultType = kind === 'acp' ? 'acp' : 'openai';
      const config = PROVIDER_CONFIG[defaultType] || PROVIDER_CONFIG.custom;
      setFormData({
        name: '',
        type: defaultType,
        base_url: config.defaultUrl,
        api_key: '',
        api_format: config.apiFormat || '',
        server_profile_id: undefined,
        auth_mode: defaultAuthMode(defaultType),
        reasoning_content_mode: config.reasoningContentMode || 'disabled',
        acp_command: '',
        acp_args: [],
        acp_agent_id: '',
        acp_env: {},
        acp_credential_env: {},
      });
      setApiTested(false);
      setModels([]);
      setModelsLoaded(false);
      setEndpointNotSupported(false);
    }
  }, [provider, kind]);

  useEffect(() => {
    if (provider || kind === 'acp' || catalogProfiles === null || catalogDefaultSuppressed.current) return;
    const defaultProfile = availableProfiles.find(profile => profile.id === 'openai') || customProfile();
    const values = runtimeProfileValues(defaultProfile);
    if (!values) return;
    setFormData(prev => ({
      ...prev,
      server_profile_id: defaultProfile.id || undefined,
      type: values.runtimeType,
      base_url: values.baseURL,
      api_format: values.apiFormat,
      auth_mode: values.authMode,
      default_model: typeof policyValue(defaultProfile, 'default_model_id', defaultProfile.default_model_id) === 'string'
        ? policyValue(defaultProfile, 'default_model_id', defaultProfile.default_model_id) as string : '',
    }));
  }, [provider, kind, catalogProfiles, availableProfiles]);

  useEffect(() => {
    if (!provider || kind === 'acp' || catalogProfiles === null || !provider.server_profile_id) return;
    const profile = catalogProfiles.find(candidate => candidate.id === provider.server_profile_id);
    if (policyFor(profile, 'default_model_id')?.policy !== 'fixed' || !profile) return;
    const catalogDefault = policyValue(profile, 'default_model_id', profile.default_model_id);
    if (typeof catalogDefault !== 'string' || !catalogDefault) return;

    setFormData(prev => {
      // The persisted value remains authoritative; only fill a genuinely empty
      // field for this same connection after its explicit profile is available.
      if (prev.id !== provider.id || prev.default_model?.trim() || provider.default_model?.trim()) return prev;
      return { ...prev, default_model: catalogDefault };
    });
  }, [provider, kind, catalogProfiles]);

  /**
   * Recoloca no formulário a configuração do provedor salvo, como a carga
   * inicial faz. O nome fica como está — quem renomeou não pediu para desfazer
   * isso. O rascunho da credencial é descartado para não transportar uma
   * edição feita para outro destino.
   */
  const restoreSavedProvider = () => {
    if (!provider) return;
    const savedConfig = PROVIDER_CONFIG[provider.type] || PROVIDER_CONFIG.custom;
    setFormData((prev) => ({
      ...prev,
      type: canonicalFormType(provider),
      api_format: provider.api_format ?? savedConfig.apiFormat ?? '',
      server_profile_id: provider.server_profile_id,
      reasoning_content_mode: provider.reasoning_content_mode
        ?? savedConfig.reasoningContentMode
        ?? 'disabled',
      base_url: provider.base_url,
      default_model: provider.default_model || '',
      api_key: '',
      auth_mode: provider.auth_mode || defaultAuthMode(provider.type),
      credential_pattern: provider.credential_pattern,
      acp_command: provider.acp_command || '',
      acp_args: provider.acp_args || [],
      acp_agent_id: provider.acp_agent_id || '',
      acp_credential_env: provider.acp_credential_env || {},
    }));
    setErrors({});
    setApiTested(false);
    setModels([]);
    setModelsLoaded(false);
    setEndpointNotSupported(false);
    handleCredentialChange(null);
  };

  /** Selecionar um perfil substitui explicitamente os valores estruturais da conexão. */
  const handleProfileChange = (profileId: string) => {
    // Uma escolha explícita sempre vence a resposta assíncrona do catálogo.
    catalogDefaultSuppressed.current = true;
    const profile = availableProfiles.find(candidate => candidate.id === profileId) || customProfile();
    if (provider && profileId === (provider.server_profile_id || '')) {
      restoreSavedProvider();
      return;
    }
    const values = runtimeProfileValues(profile);
    if (!values) return;

    setFormData((prev) => ({
      ...prev,
      type: values.runtimeType,
      server_profile_id: profileId,
      api_format: values.apiFormat,
      reasoning_content_mode: PROVIDER_CONFIG[values.runtimeType]?.reasoningContentMode || 'disabled',
      base_url: values.baseURL,
      default_model: typeof policyValue(profile, 'default_model_id', profile.default_model_id) === 'string'
        ? policyValue(profile, 'default_model_id', profile.default_model_id) as string : '',
      // O que não pertence ao novo tipo não fica pendurado: agente não tem
      // credencial no app, e provedor HTTP não tem comando para subir.
      api_key: '',
      auth_mode: values.authMode,
      credential_pattern: undefined,
      acp_command: '',
      acp_args: [],
      acp_agent_id: '',
      acp_credential_env: {},
    }));
    // Erros descrevem a forma anterior do formulário; a validação do submit
    // recalcula o que ainda valer.
    setErrors({});
    setApiTested(false);
    setModels([]);
    setModelsLoaded(false);
    setEndpointNotSupported(false);
    handleCredentialChange(null);
  };

  // ACP continua separado do catálogo HTTP e conserva seu seletor existente.
  const handleTypeChange = (nextType: string) => {
    const config = PROVIDER_CONFIG[nextType] || PROVIDER_CONFIG.custom;
    if (provider && nextType === canonicalFormType(provider)) {
      restoreSavedProvider();
      return;
    }
    setFormData(prev => ({
      ...prev,
      type: nextType,
      server_profile_id: undefined,
      api_format: config.apiFormat || '',
      reasoning_content_mode: config.reasoningContentMode || 'disabled',
      base_url: config.defaultUrl,
      default_model: '',
      api_key: '',
      auth_mode: defaultAuthMode(nextType),
      credential_pattern: undefined,
      acp_command: '',
      acp_args: [],
      acp_agent_id: '',
      acp_credential_env: {},
    }));
    setErrors({});
    setApiTested(false);
    setModels([]);
    setModelsLoaded(false);
    setEndpointNotSupported(false);
    handleCredentialChange(null);
  };

  // Aplica o default fixo do perfil em criação e preserva a URL salva em edição.
  const getCanonicalUrl = (type: string): string => {
    if (provider && formData.server_profile_id === provider.server_profile_id) return formData.base_url;
    const profile = selectedProfile;
    const policy = policyFor(profile, 'base_url');
    if (policy?.policy === 'fixed' || policy?.policy === 'hidden') {
      const value = policyValue(profile, 'base_url', profile.base_url);
      if (typeof value === 'string') return value;
    }
    const config = PROVIDER_CONFIG[type] || PROVIDER_CONFIG.custom;
    
    // Para provedores com URL não editável (comerciais), retorna a URL padrão
    if (!config.urlEditable) {
      return config.defaultUrl;
    }
    
    // Para provedores editáveis, usa o que o usuário digitou
    return formData.base_url;
  };

  const handleChange = (field: keyof ProviderFormData, value: string) => {
    const profileFields = ['base_url', 'api_format', 'auth_mode', 'default_model'];
    const preserveCustomDraft = !provider && catalogProfiles === null && profileFields.includes(field);
    if (preserveCustomDraft) catalogDefaultSuppressed.current = true;
    if (['base_url', 'api_format'].includes(field)) handleCredentialChange(null);
    if (field === 'auth_mode') {
      if (value === 'none' || (formData.auth_mode || defaultAuthMode(formData.type)) === 'none') handleCredentialChange(null);
      else invalidatePreview();
    }
    setFormData((prev) => ({
      ...prev,
      [field]: value,
      ...(preserveCustomDraft ? { type: 'custom', server_profile_id: undefined } : {}),
      ...(field === 'api_format' && value === 'google' ? { auth_mode: 'required' } : {}),
    }));
    // Limpa erro do campo
    if (errors[field]) {
      setErrors((prev) => ({ ...prev, [field]: '' }));
    }
  };

  const handleLoadModels = async () => {
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

    if (credentialDraft) {
      const error = validateCredential(credentialDraft, t);
      if (error) { setErrors(prev => ({ ...prev, credential: error })); return; }
    }

    await loadModels();
  };

  const validate = (): boolean => {
    const newErrors: Record<string, string> = {};
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
      if (policyFor(selectedProfile, 'base_url')?.required !== false) {
        newErrors.base_url = t('providerForm.error.urlRequired');
      }
    } else {
      try {
        new URL(canonicalUrl);
      } catch {
        newErrors.base_url = t('providerForm.error.urlInvalid');
      }
    }

    if (credentialDraft) {
      const error = validateCredential(credentialDraft, t);
      if (error) newErrors.credential = error;
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
    const credentialEnv = formData.acp_credential_env || {};
    // ACPEnv não vai na fronteira Create/Update: variável de ambiente é onde
    // token costuma parar, e a tela não a edita. O env do binário instalado
    // (VT_ACP_* etc.) o backend aplica sozinho a partir do installed.json
    // quando há acp_agent_id.
    if (formData.id) {
      await UpdateLLMProvider(formData.id, apidto.UpdateLLMProviderRequest.createFrom({
          name: formData.name,
          type: formData.type,
          api_format: AGENT_API_FORMAT,
          acp_command: command,
          acp_args: args,
          acp_agent_id: agentId,
          // Sempre presente, mesmo vazio: aqui o mapa vazio é o que desliga a
          // passagem, e omiti-lo seria pedir para não mexer — quem tirou o
          // último par continuaria com a credencial indo para o agente.
          acp_credential_env: credentialEnv,
        }));
      return;
    }
    await CreateLLMProvider(apidto.CreateLLMProviderRequest.createFrom({
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
        acp_credential_env: credentialEnv,
      }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (saving || !validate()) return;
    const sequence = formSequence.current;

    saveReturnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setSaving(true);
    try {
      // IMPORTANTE: Sempre usa a URL canônica ao salvar
      // Isso garante que URLs incorretas (ex: Google incompleto) sejam corrigidas automaticamente
      const canonicalUrl = getCanonicalUrl(formData.type);

      if (isAgent) {
        await saveAgentProvider();
        if (sequence === formSequence.current) onSave();
        return;
      }

      if (formData.id) {
        // Update
        await UpdateLLMProvider(formData.id, apidto.UpdateLLMProviderRequest.createFrom({
            name: formData.name,
            type: formData.type,
            base_url: canonicalUrl,
            credential: credentialDraft ? credentialInput(credentialDraft) : undefined,
            auth_mode: formData.auth_mode || defaultAuthMode(formData.type),
            default_model: formData.default_model || undefined,
            api_format: formData.api_format || undefined,
            server_profile_id: formData.server_profile_id !== undefined ? formData.server_profile_id : provider?.server_profile_id,
            reasoning_content_mode: formData.reasoning_content_mode || 'disabled',
        }));
      } else {
        // Create - gera ID único
        const suggestedDefault = typeof selectedProfile.default_model_id === 'string'
          ? selectedProfile.default_model_id
          : PROVIDER_CONFIG[formData.type]?.defaultModel;
        await CreateLLMProvider(apidto.CreateLLMProviderRequest.createFrom({
            id: `${formData.type}-${Date.now()}`,
            name: formData.name,
            type: formData.type,
            base_url: canonicalUrl,
            credential: credentialDraft ? credentialInput(credentialDraft) : undefined,
            auth_mode: formData.auth_mode || defaultAuthMode(formData.type),
            default_model: formData.default_model || suggestedDefault || undefined,
            api_format: formData.api_format || undefined,
            server_profile_id: formData.server_profile_id || undefined,
            reasoning_content_mode: formData.reasoning_content_mode || 'disabled',
      }));
      }

      if (sequence === formSequence.current) onSave();
    } catch (error: unknown) {
      if (sequence !== formSequence.current) return;
      const err = error as { message?: unknown; toString?: () => string } | null;
      const message = String(err?.message || err?.toString?.() || error || t('providerForm.error.saveError'));
      setErrors({ submit: message });
      announce(message, 'assertive');
    } finally {
      if (sequence === formSequence.current) setSaving(false);
    }
  };

  const config = PROVIDER_CONFIG[formData.type] || PROVIDER_CONFIG.custom;
  const baseURLPolicy = policyFor(selectedProfile, 'base_url');
  const baseURLRequired = baseURLPolicy?.required ?? true;
  const defaultModelPolicy = policyFor(selectedProfile, 'default_model_id');
  const defaultModelChoices = defaultModelPolicy?.policy === 'choice'
    ? (defaultModelPolicy.choices || []).filter((value): value is string => typeof value === 'string')
    : [];
  const apiFormatPolicy = policyFor(selectedProfile, 'api_format');
  const authModePolicy = policyFor(selectedProfile, 'auth.mode');
  const isUrlReadonly = baseURLPolicy?.policy === 'fixed' || baseURLPolicy?.policy === 'hidden';
  const isURLHidden = baseURLPolicy?.policy === 'hidden';
  const isApiFormatHidden = apiFormatPolicy?.policy === 'hidden';
  const isApiFormatReadonly = apiFormatPolicy?.policy === 'fixed';
  const isAuthModeHidden = authModePolicy?.policy === 'hidden';
  const isAuthModeReadonly = authModePolicy?.policy === 'fixed';
  const apiFormatChoices = apiFormatPolicy?.policy === 'choice' && apiFormatPolicy.choices?.length
    ? apiFormatPolicy.choices.flatMap(value => typeof value === 'string' && allowlisted(API_FORMAT_TO_RUNTIME, value) ? [allowlisted(API_FORMAT_TO_RUNTIME, value)!] : [])
    : API_FORMAT_OPTIONS.map(option => option.value);
  const baseURLChoices = baseURLPolicy?.policy === 'choice' && baseURLPolicy.choices?.length
    ? baseURLPolicy.choices.filter((value): value is string => typeof value === 'string')
    : [];
  const authChoices = authModePolicy?.policy === 'choice' && authModePolicy.choices?.length
    ? authModePolicy.choices.flatMap(value => typeof value === 'string' && allowlisted(AUTH_MODE_TO_RUNTIME, value) ? [allowlisted(AUTH_MODE_TO_RUNTIME, value)!] : [])
    : ['required', 'optional', 'none'];
  const profileSelection = formData.server_profile_id ?? provider?.server_profile_id ?? '';
  const authMode = formData.auth_mode || defaultAuthMode(formData.type);
  const canLoadModels = Boolean(getCanonicalUrl(formData.type).trim()) &&
    (!credentialDraft || !validateCredential(credentialDraft, t));
  const boundPattern = provider && sameOrigin(getCanonicalUrl(formData.type), provider.base_url)
    ? provider.credential_pattern : undefined;

  return (
    <form className="provider-form" onSubmit={handleSubmit}>
      <fieldset className="provider-form__fields" disabled={saving}>
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

      <FormField label={t(isAgent ? 'providerForm.providerType' : 'providerForm.serverProfile')} required>
        <Select
          options={isAgent ? tiposDeProvedor : availableProfiles.map(profile => ({
            value: profile.id,
            label: profile.name || t('providerForm.customProfile'),
          }))}
          value={isAgent ? formData.type : profileSelection}
          onChange={(e) => isAgent ? handleTypeChange(e.target.value) : handleProfileChange(e.target.value)}
          fullWidth
        />
      </FormField>
      {catalogUnavailable && <p role="status" aria-live="polite" aria-atomic="true">
        {t('providerForm.catalogUnavailable')}
      </p>}

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
            onEnvChange={handleAgentEnvChange}
            commandError={errors.acp_command}
            credentialEnv={formData.acp_credential_env || {}}
            onCredentialEnvChange={handleCredentialEnvChange}
            selectionToken={agentSelectionToken}
          />
        </>
      ) : (
        <>
      {!isApiFormatHidden && <FormField
        label={t('providerForm.apiProtocol')}
        description={t('providerForm.apiProtocolHelp')}
      >
        <Select
          options={API_FORMAT_OPTIONS.filter(option => apiFormatChoices.includes(option.value))}
          value={formData.api_format || ''}
          onChange={(e) => handleChange('api_format', e.target.value)}
          fullWidth
          disabled={isApiFormatReadonly}
        />
      </FormField>}

      <FormField
        label={t('providerForm.reasoningContentMode')}
        description={t('providerForm.reasoningContentModeHelp')}
      >
        <Select
          options={reasoningContentModeOptions(t)}
          value={formData.reasoning_content_mode || 'disabled'}
          onChange={(e) => setFormData(prev => ({
            ...prev,
            reasoning_content_mode: e.target.value,
          }))}
          fullWidth
        />
      </FormField>

      {!isURLHidden && <FormField
        label={t('providerForm.baseUrl')}
        required={baseURLRequired}
        error={errors.base_url}
        description={undefined}
      >
        {baseURLPolicy?.policy === 'choice' ? (
          <Select
            options={baseURLChoices.map(value => ({ value, label: value }))}
            value={formData.base_url}
            onChange={(e) => handleChange('base_url', e.target.value)}
            fullWidth
            aria-label={t('providerForm.baseUrl')}
          />
        ) : (
          <Input
            value={formData.base_url}
            onChange={(e) => handleChange('base_url', e.target.value)}
            placeholder={t('providerForm.defaultUrl')}
            fullWidth
            readOnly={isUrlReadonly}
            disabled={isUrlReadonly}
            aria-label={t('providerForm.baseUrl')}
          />
        )}
        {isUrlReadonly && (
          <span className="provider-form__read-only-note">
            {t('providerForm.urlReadonly')}
          </span>
        )}
      </FormField>}

      {!isAuthModeHidden && <FormField label={t('providerForm.authMode')}>
        <Select value={authMode} fullWidth onChange={e => handleChange('auth_mode', e.target.value)}
          disabled={isAuthModeReadonly}
          options={authChoices.map(value => ({ value, label: t('providerForm.authModes.' + value), disabled: formData.api_format === 'google' && value !== 'required' }))} />
      </FormField>}
      {(selectedProfile.signup_url || selectedProfile.api_key_url) && <div className="provider-form__catalog-links">
        {selectedProfile.signup_url && <a href={selectedProfile.signup_url} target="_blank" rel="noopener noreferrer">{t('providerForm.signupLink', { provider: selectedProfile.name })}</a>}
        {selectedProfile.api_key_url && <a href={selectedProfile.api_key_url} target="_blank" rel="noopener noreferrer">{t('providerForm.apiKeyLink', { provider: selectedProfile.name })}</a>}
      </div>}
      {authMode !== 'none' && <>
        <ResourceCredentialEditor
          key={[provider?.id, formData.type, getCanonicalUrl(formData.type), formData.api_format, boundPattern].join(':')}
          url={getCanonicalUrl(formData.type)} pattern={boundPattern} type="bearer"
          allowedTypes={formData.api_format === 'google' ? undefined : HTTP_CREDENTIAL_TYPES}
          onChange={handleCredentialChange} />
      </>}

      {/* Default Model — loads models list which also validates the provider */}
      {defaultModelPolicy?.policy !== 'hidden' && <FormField
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
        {defaultModelPolicy?.policy === 'fixed' ? (
          <Select
            options={formData.default_model ? [{ value: formData.default_model, label: formData.default_model }] : []}
            value={formData.default_model || ''}
            fullWidth
            aria-label={t('providerForm.defaultModel')}
            disabled
          />
        ) : defaultModelPolicy?.policy === 'choice' ? (
          <Select
            options={defaultModelChoices.map(value => ({ value, label: value }))}
            value={formData.default_model || ''}
            onChange={(e) => setFormData(prev => ({ ...prev, default_model: e.target.value }))}
            fullWidth
            aria-label={t('providerForm.defaultModel')}
          />
        ) : modelsLoaded && models.length > 0 ? (
          <Select
            options={[
              { value: '', label: t('providerForm.modelAutomatic') },
              ...models.map(m => ({ value: m, label: m })),
            ]}
            value={formData.default_model || ''}
            onChange={(e) => setFormData(prev => ({ ...prev, default_model: e.target.value }))}
            fullWidth
            aria-label={t('providerForm.defaultModel')}
            disabled={defaultModelPolicy?.policy === 'fixed'}
          />
        ) : modelsLoaded && endpointNotSupported ? (
          <Input
            value={formData.default_model || ''}
            onChange={(e) => setFormData(prev => ({ ...prev, default_model: e.target.value }))}
            placeholder={(typeof selectedProfile.default_model_id === 'string' ? selectedProfile.default_model_id : config.defaultModel) || t('providerForm.modelPlaceholder')}
            fullWidth
            aria-label={t('providerForm.defaultModel')}
            disabled={defaultModelPolicy?.policy === 'fixed'}
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

          </div>
        )}
      </FormField>}
        </>
      )}

      {errors.submit && (
        <div className="provider-form__error">
          <WarningOutlined aria-hidden="true" /> {errors.submit}
        </div>
      )}

      </fieldset>
      {saving && <p ref={savingStatus} tabIndex={0}>{t('common.saving')}</p>}
      <DialogActions
        className="provider-form__actions"
        primary={
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
        }
        secondary={
          <Button type="button" variant="secondary" onClick={onCancel} disabled={saving}>
            {t('common.cancel')}
          </Button>
        }
      />
    </form>
  );
};
