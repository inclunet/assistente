import { useState, useEffect, forwardRef, useImperativeHandle, type ReactNode } from 'react';
import { CloseCircleOutlined, RobotOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { GetLLMProvidersWithStatus } from '@wailsjs/go/app/App';
import { ComboboxItem } from './Combobox';
import { BasePicker } from './BasePicker';
import './LLMProviderPicker.css';

export interface LLMProviderPickerProps {
  value: string;
  onChange: (providerID: string) => void;
  variant?: 'toolbar' | 'form';
  label?: string;
  helpText?: string;
  icon?: ReactNode;
  maxWidth?: string;
  disabled?: boolean;
  onAnnounce?: (message: string) => void;
}

export interface LLMProviderPickerRef {
  reload: () => Promise<void>;
}

const DEFAULT_PROVIDER_SENTINEL = '$default';

interface LLMProvider {
  id: string;
  name: string;
  type: string;
  base_url: string;
  is_default?: boolean;
}

export const LLMProviderPicker = forwardRef<LLMProviderPickerRef, LLMProviderPickerProps>(
  (
    {
      value,
      onChange,
      variant = 'form',
      label = 'Provedor LLM',
      helpText = 'Selecione o provedor de modelo de linguagem',
      icon = <RobotOutlined />,
      maxWidth,
      disabled = false,
      onAnnounce,
    },
    ref
  ) => {
    const { t } = useTranslation();
    const [providers, setProviders] = useState<LLMProvider[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const loadProviders = async () => {
      setLoading(true);
      setError(null);

      try {
        const providersList = await GetLLMProvidersWithStatus();
        if (!providersList || providersList.length === 0) {
          setError(t('pickers.llmProvider.noneConfigured'));
          setProviders([]);
        } else {
          setProviders(providersList as unknown as LLMProvider[]);
        }
      } catch (err) {
        const errorMsg = err instanceof Error ? err.message : t('pickers.llmProvider.loadError');
        setError(errorMsg);
        console.error('[LLMProviderPicker] Falha ao carregar provedores:', err);
      } finally {
        setLoading(false);
      }
    };

    useEffect(() => {
      loadProviders();
    }, []);

    useImperativeHandle(ref, () => ({
      reload: loadProviders,
    }));

    const defaultLabel = t('pickers.llmProvider.default', 'Padrão (recomendado)');
    const defaultOption: ComboboxItem = { value: DEFAULT_PROVIDER_SENTINEL, label: defaultLabel };
    const providerItems: ComboboxItem[] = providers.map((p) => ({
      value: p.id,
      label: `${p.name} (${p.type})`,
      sublabel: p.is_default ? t('pickers.llmProvider.isDefault', 'Padrão') : undefined,
    }));
    const items: ComboboxItem[] = [defaultOption, ...providerItems];

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
        placeholder="Selecione um provedor..."
        disabled={disabled || loading}
        maxWidth={variant === 'form' ? '100%' : maxWidth}
        helpText={variant === 'form' ? helpText : undefined}
        onAnnounce={onAnnounce}
        loading={loading}
        error={error || null}
        onRetry={loadProviders}
        retryLabel={t('common.retry', 'Tentar novamente')}
        showFormLabel={variant === 'form'}
        showFormLabelIcon={false}
        formClassName="llm-provider-picker-form"
        toolbarClassName="llm-provider-picker-toolbar"
        loadingClassName={{ form: 'loading-state', toolbar: 'llm-provider-picker-toolbar loading' }}
        errorClassName={{ form: 'error-state', toolbar: 'llm-provider-picker-toolbar error' }}
        helpTextClassName="help-text"
        errorIcon={{ toolbar: <CloseCircleOutlined />, form: undefined }}
        loadingLabel={{ form: 'Carregando provedores...', toolbar: 'Carregando...' }}
        errorLabel={{ form: error || 'Erro ao carregar provedores', toolbar: 'Erro' }}
      />
    );
  }
);

LLMProviderPicker.displayName = 'LLMProviderPicker';
