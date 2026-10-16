import { apidto } from '@wailsjs/go/models';
import { useState, useEffect, useLayoutEffect, useRef, useCallback, useMemo } from 'react';
import { WarningOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { CreateLLMProvider, UpdateLLMProvider, ListModelsRaw } from '@wailsjs/go/wailsapi/LLMProviders';
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
    const canonicalUrl = !config.urlEditable ? config.defaultUrl : data.base_url;

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

      // Se não tinha modelo selecionado, seleciona o default do provider config
      if (!data.default_model) {
        const suggested = config.defaultModel;
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
      if (sequence === loadSequence.current) setLoadingModels(false);
    }
  }, [formData, credentialDraft, t]);

  useEffect(() => {
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



    if (provider && nextType === canonicalFormType(provider)) {
      restoreSavedProvider();
      return;
    }

    setFormData((prev) => ({
      ...prev,
      type: nextType,
      api_format: config.apiFormat || '',
      reasoning_content_mode: config.reasoningContentMode || 'disabled',
      base_url: config.defaultUrl,
      default_model: '',
      // O que não pertence ao novo tipo não fica pendurado: agente não tem
      // credencial no app, e provedor HTTP não tem comando para subir.
      api_key: '',
      auth_mode: defaultAuthMode(nextType),
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
    if (['base_url', 'api_format'].includes(field)) handleCredentialChange(null);
    if (field === 'auth_mode') {
      if (value === 'none' || (formData.auth_mode || defaultAuthMode(formData.type)) === 'none') handleCredentialChange(null);
      else invalidatePreview();
    }
    setFormData((prev) => ({ ...prev, [field]: value, ...(field === 'api_format' && value === 'google' ? { auth_mode: 'required' } : {}) }));
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
      newErrors.base_url = t('providerForm.error.urlRequired');
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
            reasoning_content_mode: formData.reasoning_content_mode || 'disabled',
        }));
      } else {
        // Create - gera ID único
        const suggestedDefault = PROVIDER_CONFIG[formData.type]?.defaultModel;
        await CreateLLMProvider(apidto.CreateLLMProviderRequest.createFrom({
            id: `${formData.type}-${Date.now()}`,
            name: formData.name,
            type: formData.type,
            base_url: canonicalUrl,
            credential: credentialDraft ? credentialInput(credentialDraft) : undefined,
            auth_mode: formData.auth_mode || defaultAuthMode(formData.type),
            default_model: formData.default_model || suggestedDefault || undefined,
            api_format: formData.api_format || undefined,
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
  const isUrlReadonly = !config.urlEditable;
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
            onEnvChange={handleAgentEnvChange}
            commandError={errors.acp_command}
            credentialEnv={formData.acp_credential_env || {}}
            onCredentialEnvChange={handleCredentialEnvChange}
            selectionToken={agentSelectionToken}
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
          onChange={(e) => handleChange('api_format', e.target.value)}
          fullWidth
        />
      </FormField>

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

      <FormField label={t('providerForm.authMode')}>
        <Select value={authMode} fullWidth onChange={e => handleChange('auth_mode', e.target.value)}
          options={['required', 'optional', 'none'].map(value => ({ value, label: t('providerForm.authModes.' + value), disabled: formData.api_format === 'google' && value !== 'required' }))} />
      </FormField>
      {authMode !== 'none' && <>
        <ResourceCredentialEditor
          key={[provider?.id, formData.type, getCanonicalUrl(formData.type), formData.api_format, boundPattern].join(':')}
          url={getCanonicalUrl(formData.type)} pattern={boundPattern} type="bearer"
          allowedTypes={formData.api_format === 'google' ? undefined : HTTP_CREDENTIAL_TYPES}
          onChange={handleCredentialChange} />
      </>}

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
