import { useState, useEffect, useImperativeHandle, forwardRef, type ReactNode } from 'react';
import { CloseCircleOutlined, RobotOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { GetModels, GetModelsByProvider, GetLLMProvidersWithStatus } from '@wailsjs/go/main/App';
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
  const [models, setModels] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [endpointNotSupported, setEndpointNotSupported] = useState(false);

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

  const loadModels = async () => {
    if (variant === 'form' && !providerID) {
      setLoading(false);
      setError(t('pickers.model.selectProvider'));
      setModels([]);
      setEndpointNotSupported(false);
      return;
    }

    setLoading(true);
    setError('');
    setEndpointNotSupported(false);
    try {
      let modelsList: string[];

      const resolvedID = providerID ? await resolveProviderID(providerID) : '';
      if (resolvedID) {
        modelsList = await GetModelsByProvider(resolvedID);
      } else if (!providerID) {
        modelsList = await GetModels();
      } else {
        modelsList = [];
      }
      
      setModels(modelsList || []);
      if (!modelsList || modelsList.length === 0) {
        const msg = providerID
          ? t('pickers.model.noModels')
          : t('pickers.model.noModelsGlobal');
        setError(msg);
      }
    } catch (e: unknown) {
      const err = e as { message?: unknown } | null;
      const errorMsg = String(err?.message || e || 'Erro desconhecido');
      
      // Detecta se o endpoint de modelos não é suportado (404)
      if (errorMsg.includes('models_endpoint_not_supported')) {
        setEndpointNotSupported(true);
        setError('');
        setModels([]);
      } else if (errorMsg.includes('credencial não configurada') || errorMsg.includes('Missing bearer authentication')) {
        // Detecta erro de credencial não configurada
        setError(t('pickers.model.configureApiKey'));
        setEndpointNotSupported(false);
        setModels([]);
      } else {
        setError(`${t('pickers.model.loadError')} ${errorMsg}`);
        setEndpointNotSupported(false);
        setModels([]);
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadModels();
  }, [providerID]); // Recarrega quando providerID muda

  useImperativeHandle(ref, () => ({
    reload: loadModels
  }));

  const defaultModelLabel = t('pickers.model.default', 'Padrão do provedor');
  const defaultModelOption: ComboboxItem = { value: DEFAULT_MODEL_SENTINEL, label: defaultModelLabel };
  const modelItems: ComboboxItem[] = models.map(m => ({ value: m, label: m }));
  const items: ComboboxItem[] = variant === 'form'
    ? [defaultModelOption, ...modelItems]
    : modelItems;

  const handleSelect = (selectedValue: string) => {
    onChange(selectedValue);
  };

  return (
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
      helpText={variant === 'form' ? (endpointNotSupported ? t('pickers.model.notLoaded') : helpText) : undefined}
      onAnnounce={onAnnounce}
      loading={loading && !endpointNotSupported}
      error={endpointNotSupported ? null : (error || null)}
      onRetry={endpointNotSupported ? undefined : loadModels}
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
});

ModelPicker.displayName = 'ModelPicker';
