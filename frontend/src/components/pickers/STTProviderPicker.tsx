import { useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import { useTranslation } from 'react-i18next';
import { ComboboxItem } from './Combobox';
import { BasePicker } from './BasePicker';
import './STTProviderPicker.css';

// Constantes de provedores STT
export const STT_WEBSPEECH = 'webspeech';
export const STT_WHISPER = 'whisper_api';

export interface STTProviderPickerProps {
  value: string;
  onChange: (provider: string) => void;
  variant?: 'toolbar' | 'form';
  label?: string;
  helpText?: string;
  icon?: string;
  maxWidth?: string;
  onAnnounce?: (message: string) => void;
}

export interface STTProviderPickerRef {
  reload: () => Promise<void>;
}

interface STTProvider {
  id: string;
  name: string;
  description: string;
  icon: string;
}

export const STTProviderPicker = forwardRef<STTProviderPickerRef, STTProviderPickerProps>(
  (
    {
      value,
      onChange,
      variant = 'form',
      label,
      helpText,
      icon = '🎤',
      maxWidth,
      onAnnounce,
    },
    ref
  ) => {
    const { t } = useTranslation();
    const [providers, setProviders] = useState<STTProvider[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const loadProviders = async () => {
      setLoading(true);
      setError(null);

      try {
        const providersList: STTProvider[] = [
          {
            id: STT_WEBSPEECH,
            name: t('pickers.stt.webSpeech'),
            description: t('pickers.stt.webSpeechDesc'),
            icon: '🌐'
          },
          {
            id: STT_WHISPER,
            name: t('pickers.stt.whisper'),
            description: t('pickers.stt.whisperDesc'),
            icon: '🤖'
          },
        ];

        setProviders(providersList);
      } catch (err) {
        setError(err instanceof Error ? err.message : t('pickers.stt.loadError'));
        console.error('Failed to load STT providers:', err);
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

    const items: ComboboxItem[] = providers.map((provider) => ({
      value: provider.id,
      label: `${provider.icon} ${provider.name}`,
      sublabel: provider.description,
    }));

    return (
      <BasePicker
        variant={variant}
        items={items}
        selected={value}
        onSelect={onChange}
        label={label ?? t('pickers.stt.label')}
        icon={icon}
        maxWidth={maxWidth}
        onAnnounce={onAnnounce}
        loading={loading}
        error={error}
        onRetry={loadProviders}
        showFormLabel={variant === 'form'}
        formClassName="stt-picker-form"
        formLabelClassName="stt-picker-label"
        formLabelIconClassName="stt-picker-icon"
        helpText={variant === 'form' ? (helpText ?? t('pickers.stt.description')) : undefined}
        helpTextClassName="help-text"
        loadingLabel={{ form: t('pickers.stt.loading'), toolbar: t('pickers.stt.loading') }}
        loadingLabelVisuallyHidden={{ toolbar: true }}
        loadingClassName={{ form: 'loading-state', toolbar: 'stt-picker-toolbar' }}
        errorClassName={{ form: 'error-state', toolbar: 'stt-picker-toolbar stt-picker-error' }}
        errorLabel={{ form: error || t('pickers.stt.loadError'), toolbar: '' }}
        errorLabelVisuallyHidden={{ toolbar: true }}
        errorIcon={{ form: '⚠️', toolbar: '⚠️' }}
        retryClassName="retry-btn"
      />
    );
  }
);
