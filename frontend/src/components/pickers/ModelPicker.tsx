import { useState, useEffect, useImperativeHandle, forwardRef, type ReactNode } from 'react';
import { CloseCircleOutlined, ReloadOutlined, RobotOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import {
  GetModels,
  GetModelCatalogByProvider,
  GetLLMProvidersWithStatus,
  RefreshModels,
  RefreshModelCatalogByProvider,
} from '@wailsjs/go/app/App';
import { Button } from '../ui/Button';
import { ComboboxItem } from './Combobox';
import { BasePicker } from './BasePicker';
import './ModelPicker.css';

const DEFAULT_PROVIDER_SENTINEL = '$default';
const DEFAULT_MODEL_SENTINEL = '$default';

export interface ModelPickerProps {
    value: string;
    onChange: (value: string) => void;
    label?: string;
    icon?: ReactNode;
    placeholder?: string;
    disabled?: boolean;
    maxWidth?: string;
    variant?: 'toolbar' | 'form';
    helpText?: string;
    onAnnounce?: (message: string) => void;
    providerID?: string; // ID do provedor para filtrar modelos
}

export interface ModelPickerRef {
    reload: () => void;
}

/** LoadOutcome é o que uma listagem produziu, para quem precisa anunciá-la. */
interface LoadOutcome {
    ok: boolean;
    /** Message explica o que houve, já traduzida. Vazia quando deu certo. */
    message: string;
    count: number;
}

/** ModelItem é um modelo como ele aparece na lista. */
interface ModelItem {
    value: string;
    label: string;
}

/**
 * AGENT_UNAVAILABLE_CODE é a marca que o backend põe na falha de quem não
 * conseguiu falar com o agente de código. Insistir no recarregar não resolve —
 * quem resolve é a tela de provedores (AEP-0084, Fase 8).
 */
const AGENT_UNAVAILABLE_CODE = 'acp_agent_unavailable';

export const ModelPicker = forwardRef<ModelPickerRef, ModelPickerProps>(({
  value,
  onChange,
  label = 'Modelo',
  icon = <RobotOutlined />,
  placeholder = 'Filtrar modelos...',
  disabled = false,
  maxWidth = '180px',
  variant = 'toolbar',
  helpText = '',
  onAnnounce,
  providerID = '', // Provedor específico (se vazio, usa GetModels do ativo)
}, ref) => {
  const { t } = useTranslation();
  const [models, setModels] = useState<ModelItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [endpointNotSupported, setEndpointNotSupported] = useState(false);
  // notice é o que a lista tem a dizer sem ser falha: um agente de código que
  // não expõe escolha de modelo respondeu, e quem escolhe é ele.
  const [notice, setNotice] = useState('');

  const resolveProviderID = async (pid: string): Promise<string> => {
    if (pid !== DEFAULT_PROVIDER_SENTINEL) return pid;
    try {
      const providers = await GetLLMProvidersWithStatus();
      const defaultProv = (providers || []).find((p: Record<string, unknown>) => p.is_default === true);
      return (defaultProv?.id as string) || '';
    } catch {
      return '';
    }
  };

  /**
   * loadModels busca a lista de modelos. `refresh` diz que foi a pessoa que
   * pediu de novo, e só então o que o provedor tiver guardado é descartado: um
   * agente de código responde de uma sessão de descoberta guardada por processo,
   * e invalidar a cada render faria a tela de perfil bater no agente sem motivo
   * (AEP-0084 D6).
   *
   * Devolve o que houve porque quem chamou pode precisar anunciar: o estado da
   * tela vive em `useState` e não está legível na volta da promessa.
   */
  const loadModels = async (refresh = false): Promise<LoadOutcome> => {
    if (variant === 'form' && !providerID) {
      const msg = t('pickers.model.selectProvider');
      setLoading(false);
      setError(msg);
      setModels([]);
      setNotice('');
      setEndpointNotSupported(false);
      return { ok: false, message: msg, count: 0 };
    }

    setLoading(true);
    setError('');
    setNotice('');
    setEndpointNotSupported(false);
    try {
      let modelsList: ModelItem[] = [];
      let agent = false;

      const resolvedID = providerID ? await resolveProviderID(providerID) : '';
      if (resolvedID) {
        const catalog = refresh
          ? await RefreshModelCatalogByProvider(resolvedID)
          : await GetModelCatalogByProvider(resolvedID);
        agent = catalog?.agent ?? false;
        modelsList = (catalog?.models || []).map(({ value, label }) => ({
          value,
          label: label || value,
        }));
      } else if (!providerID) {
        const names = refresh ? await RefreshModels() : await GetModels();
        modelsList = (names || []).map(name => ({ value: name, label: name }));
      }

      setModels(modelsList);
      if (modelsList.length === 0) {
        // Agente sem escolha de modelo não é lista que faltou: é ele dizendo
        // que a escolha é dele. Marcar como erro mandaria a pessoa procurar
        // conserto para o funcionamento normal do agente (AEP-0084, Fase 8).
        if (agent) {
          const msg = t('pickers.model.agentChooses');
          setNotice(msg);
          return { ok: false, message: msg, count: 0 };
        }
        const msg = providerID
          ? t('pickers.model.noModels')
          : t('pickers.model.noModelsGlobal');
        setError(msg);
        return { ok: false, message: msg, count: 0 };
      }
      return { ok: true, message: '', count: modelsList.length };
    } catch (e: unknown) {
      // Só o que a falha disser de verdade entra aqui: a mensagem é lida em voz
      // alta e exibida nos três idiomas, e texto inventado neste ponto sairia em
      // português para quem usa o app em inglês ou espanhol. Nem o nome da
      // classe do erro serve de detalhe — "Error" não explica nada a ninguém.
      const err = e as { message?: unknown } | null;
      const errorMsg = (typeof err?.message === 'string'
        ? err.message
        : typeof e === 'string' ? e : ''
      ).trim();
      
      // Detecta se o endpoint de modelos não é suportado (404)
      if (errorMsg.includes('models_endpoint_not_supported')) {
        setEndpointNotSupported(true);
        setError('');
        setModels([]);
        return { ok: false, message: t('pickers.model.notLoaded'), count: 0 };
      }
      // O agente não subiu: o comando salvo não existe mais, mudou de lugar ou
      // não roda. O detalhe cru diz "executable file not found in %PATH%", que
      // não conta a quem lê o que fazer — e o que fazer é refazer a detecção na
      // tela de provedores (AEP-0084, Fase 8).
      if (errorMsg.includes(AGENT_UNAVAILABLE_CODE)) {
        const msg = t('pickers.model.agentUnavailable');
        setError(msg);
        setEndpointNotSupported(false);
        setModels([]);
        return { ok: false, message: msg, count: 0 };
      }
      if (errorMsg.includes('credencial não configurada') || errorMsg.includes('Missing bearer authentication')) {
        // Detecta erro de credencial não configurada
        const msg = t('pickers.model.configureApiKey');
        setError(msg);
        setEndpointNotSupported(false);
        setModels([]);
        return { ok: false, message: msg, count: 0 };
      }
      const msg = errorMsg ? `${t('pickers.model.loadError')} ${errorMsg}` : t('pickers.model.loadError');
      setError(msg);
      setEndpointNotSupported(false);
      setModels([]);
      return { ok: false, message: msg, count: 0 };
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadModels();
  }, [providerID]); // Recarrega quando providerID muda

  // Quem chama reload está pedindo a lista de novo, e não a que já tínhamos.
  useImperativeHandle(ref, () => ({
    reload: () => { void loadModels(true); }
  }));

  const defaultModelLabel = t('pickers.model.default', 'Padrão do provedor');
  const defaultModelOption: ComboboxItem = { value: DEFAULT_MODEL_SENTINEL, label: defaultModelLabel };
  const modelItems: ComboboxItem[] = models.map(({ value: id, label: name }) => ({
    value: id,
    label: name,
  }));
  const items: ComboboxItem[] = variant === 'form'
    ? [defaultModelOption, ...modelItems]
    : modelItems;

  const handleSelect = (selectedValue: string) => {
    onChange(selectedValue);
  };

  // Quem pediu o recarregar ouve o que houve, e não sempre "recarregada": a
  // lista pode não ter vindo — credencial que falta, provedor que não lista — e
  // anunciar sucesso nesses casos diria a quem usa leitor de telas o contrário
  // do que a tela mostra.
  //
  // Deu certo, o que é dito é o tamanho da lista, não que ela foi buscada de
  // novo: um agente de código responde da sessão de descoberta que ele já tem —
  // sem `session/close` no protocolo dele, outra sessão ficaria pendurada — e
  // prometer uma consulta que não houve faria a pessoa clicar de novo achando
  // que não funcionou (AEP-0084 D6).
  const handleRefresh = () => {
    void loadModels(true).then(({ ok, message, count }) => {
      onAnnounce?.(ok
        ? t('pickers.model.refreshed', { count, defaultValue: 'Lista de modelos atualizada: {{count}}' })
        : message);
    });
  };

  const picker = (
    <BasePicker
      variant={variant}
      items={items}
      selected={value}
      onSelect={handleSelect}
      label={label}
      icon={icon}
      placeholder={endpointNotSupported ? t('pickers.model.typePlaceholder') : placeholder}
      disabled={disabled}
      maxWidth={variant === 'form' ? '100%' : maxWidth}
      helpText={variant === 'form' ? (endpointNotSupported ? t('pickers.model.notLoaded') : (notice || helpText)) : undefined}
      onAnnounce={onAnnounce}
      loading={loading && !endpointNotSupported}
      error={endpointNotSupported ? null : (error || null)}
      onRetry={endpointNotSupported ? undefined : handleRefresh}
      retryLabel={t('pickers.model.retry')}
      showFormLabel={variant === 'form'}
      showFormLabelIcon={false}
      showEmptyState={!endpointNotSupported}
      allowFreeInput={endpointNotSupported}
      formClassName={`model-picker-form${endpointNotSupported ? ' models-not-available' : ''}`}
      toolbarClassName={`model-picker-toolbar${endpointNotSupported ? ' models-not-available' : ''}`}
      loadingClassName={{ form: 'loading-state', toolbar: 'model-picker-toolbar loading' }}
      errorClassName={{ form: 'error-state', toolbar: 'model-picker-toolbar error' }}
      helpTextClassName="help-text"
      errorIcon={{ toolbar: <CloseCircleOutlined />, form: undefined }}
      loadingLabel={{ form: t('pickers.model.loading'), toolbar: t('common.loading') }}
      errorLabel={{ form: error || t('pickers.model.loadError'), toolbar: t('common.error') }}
    />
  );

  if (variant !== 'form') return picker;

  // O recarregar só existe no formulário porque é lá que a pessoa escolhe o
  // modelo do perfil. Provedor que guarda a lista — o agente de código — só
  // volta a perguntar por aqui.
  return (
    <div className="model-picker-form__stack">
      {picker}
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={handleRefresh}
        disabled={disabled || loading}
        aria-label={t('pickers.model.refreshLabel', 'Recarregar a lista de modelos do provedor')}
      >
        <ReloadOutlined aria-hidden="true" />
        {t('pickers.model.refresh', 'Recarregar modelos')}
      </Button>
    </div>
  );
});

ModelPicker.displayName = 'ModelPicker';
