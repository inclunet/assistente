import { useState, useEffect, forwardRef, useImperativeHandle, type ReactNode } from 'react';
import { AudioOutlined, WarningOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { GetSpeechProviders } from '@wailsjs/go/wailsapi/Speech';
import { llm } from '@wailsjs/go/models';
import { ComboboxItem } from './Combobox';
import { BasePicker } from './BasePicker';
import './STTProviderPicker.css';

// Constantes de provedores STT
export const STT_WEBSPEECH = 'webspeech';
export const STT_WHISPER = 'whisper_api';

export interface STTProviderPickerProps {
  /** Valor atual: 'webspeech' ou ID do provider LLM (ex: 'openai-123') */
  value: string;
  /** Callback com o provider selecionado e o llmProviderId (para provedores LLM) */
  onChange: (provider: string, llmProviderId?: string) => void;
  /** Provedores com suporte a speech (TTS/STT). Se fornecido, evita fetch interno. */
  providers?: llm.ProviderConfig[];
  variant?: 'toolbar' | 'form';
  label?: string;
  helpText?: string;
  icon?: ReactNode;
  maxWidth?: string;
  onAnnounce?: (message: string) => void;
}

export interface STTProviderPickerRef {
  reload: () => Promise<void>;
}

export const STTProviderPicker = forwardRef<STTProviderPickerRef, STTProviderPickerProps>(
  (
    {
      value,
      onChange,
      providers: externalProviders,
      variant = 'form',
      label,
      helpText,
      icon = <AudioOutlined />,
      maxWidth,
      onAnnounce,
    },
    ref
  ) => {
    const { t } = useTranslation();
    const [internalProviders, setInternalProviders] = useState<llm.ProviderConfig[]>([]);
    const [loading, setLoading] = useState(!externalProviders);
    const [error, setError] = useState<string | null>(null);

    const loadProviders = async () => {
      if (externalProviders) return;
      setLoading(true);
      setError(null);

      try {
        const providers = await GetSpeechProviders();
        setInternalProviders(providers || []);
      } catch (err) {
        setError(err instanceof Error ? err.message : t('pickers.stt.loadError'));
      } finally {
        setLoading(false);
      }
    };

    useEffect(() => {
      if (!externalProviders) {
        loadProviders();
      }
    }, [externalProviders]);

    useImperativeHandle(ref, () => ({
      reload: loadProviders,
    }));

    const speechProviders = externalProviders ?? internalProviders;

    const items: ComboboxItem[] = [
      {
        value: STT_WEBSPEECH,
        label: t('pickers.stt.webSpeech'),
        sublabel: t('pickers.stt.webSpeechDesc'),
      },
      ...speechProviders.map((p) => ({
        value: p.id,
        label: p.name,
        sublabel: t('pickers.stt.whisperViaProvider'),
      })),
    ];

    const handleSelect = (selected: string) => {
      if (selected === STT_WEBSPEECH) {
        onChange(STT_WEBSPEECH, undefined);
      } else {
        // Selecionou um provider LLM → usa whisper_api com esse provider
        onChange(STT_WHISPER, selected);
      }
    };

    // Para exibição: se é whisper_api, mostra o llmProviderId (que é o value real)
    // Se é webspeech, mostra 'webspeech'
    const displayValue = value === STT_WEBSPEECH ? STT_WEBSPEECH : value;

    return (
      <BasePicker
        variant={variant}
        items={items}
        selected={displayValue}
        onSelect={handleSelect}
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
        errorIcon={{ form: <WarningOutlined />, toolbar: <WarningOutlined /> }}
        retryClassName="retry-btn"
      />
    );
  }
);
