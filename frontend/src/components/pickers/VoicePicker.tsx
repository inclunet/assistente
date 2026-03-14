import { useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import { useTranslation } from 'react-i18next';
import { ComboboxItem } from './Combobox';
import { BasePicker } from './BasePicker';
import { ttsService } from '../../services/tts';
import { TTSVoice, TTSProvider } from '../../services/tts/types';
import './VoicePicker.css';

// Valor especial para voz desativada (usa leitor de telas)
export const VOICE_DISABLED = '__disabled__';

export interface VoicePickerProps {
  value: string;
  onChange: (voice: string) => void;
  variant?: 'toolbar' | 'form';
  label?: string;
  helpText?: string;
  icon?: string;
  maxWidth?: string;
  allowDisabled?: boolean;
  onAnnounce?: (message: string) => void;
}

export interface VoicePickerRef {
  reload: () => Promise<void>;
}

// Mapeia provider para label amigável (valores traduzidos via useTranslation no componente)

// Ícones por provider
const providerIcons: Record<TTSProvider, string> = {
  [TTSProvider.DISABLED]: '🔇',
  [TTSProvider.WEBSPEECH]: '🔊',
  [TTSProvider.SAPI5]: '🪟',
  [TTSProvider.OPENAI]: '💎'
};

export const VoicePicker = forwardRef<VoicePickerRef, VoicePickerProps>(
  (
    {
      value,
      onChange,
      variant = 'form',
      label,
      helpText,
      icon = '🔊',
      maxWidth,
      allowDisabled = true,
      onAnnounce,
    },
    ref
  ) => {
    const { t } = useTranslation();
    const effectiveLabel = label ?? t('pickers.voice.label');
    const effectiveHelpText = helpText ?? t('pickers.voice.description');
    const [voices, setVoices] = useState<TTSVoice[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const loadVoices = async () => {
      setLoading(true);
      setError(null);

      try {
        const allVoices = await ttsService.getVoices();

        setVoices(allVoices);
      } catch (err) {
        setError(err instanceof Error ? err.message : t('pickers.voice.loadError'));
        console.error('[VoicePicker] Failed to load voices:', err);
      } finally {
        setLoading(false);
      }
    };

    useEffect(() => {
      loadVoices();
    }, []);

    useImperativeHandle(ref, () => ({
      reload: loadVoices,
    }));

    // Agrupa vozes por provider
    const voicesByProvider = voices.reduce((acc, voice) => {
      if (!acc[voice.provider]) {
        acc[voice.provider] = [];
      }
      acc[voice.provider].push(voice);
      return acc;
    }, {} as Record<TTSProvider, TTSVoice[]>);

    // Opção de desativado
    const providerLabels: Record<TTSProvider, string> = {
      [TTSProvider.DISABLED]: t('pickers.voice.disabled'),
      [TTSProvider.WEBSPEECH]: t('pickers.voice.system'),
      [TTSProvider.SAPI5]: t('pickers.voice.windows'),
      [TTSProvider.OPENAI]: t('pickers.voice.openai'),
    };
    const disabledOption: ComboboxItem = {
      value: VOICE_DISABLED,
      label: t('pickers.voice.screenReader'),
      sublabel: t('pickers.voice.accessibility'),
    };

    // Constrói lista de itens com grupos por provider
    const items: ComboboxItem[] = [
      ...(allowDisabled ? [disabledOption] : []),
    ];

    // Adiciona vozes agrupadas por provider
    const providerOrder = [
      TTSProvider.WEBSPEECH,
      TTSProvider.SAPI5,
      TTSProvider.OPENAI
    ];

    for (const providerType of providerOrder) {
      const providerVoices = voicesByProvider[providerType];
      if (!providerVoices || providerVoices.length === 0) continue;

      // Header do grupo (opcional, pode ser removido se não quiser separadores visuais)
      // items.push({
      //   value: `__header_${providerType}__`,
      //   label: `${providerIcons[providerType]} ${providerLabels[providerType]}`,
      //   sublabel: '',
      //   disabled: true
      // });

      // Adiciona vozes do provider
      providerVoices.forEach(voice => {
        const providerIcon = providerIcons[voice.provider];
        const providerLabel = providerLabels[voice.provider];
        
        items.push({
          value: voice.id,
          label: `${providerIcon} ${voice.name}`,
          sublabel: `${providerLabel} • ${voice.language}${voice.premium ? ` • ${t('pickers.voice.premium')}` : ''}`
        });
      });
    }

    return (
      <BasePicker
        variant={variant}
        items={items}
        selected={value}
        onSelect={onChange}
        label={effectiveLabel}
        icon={icon}
        maxWidth={maxWidth}
        onAnnounce={onAnnounce}
        loading={loading}
        error={error}
        onRetry={loadVoices}
        showFormLabel={variant === 'form'}
        formClassName="voice-picker-form"
        formLabelClassName="voice-picker-label"
        formLabelIconClassName="voice-picker-icon"
        helpText={variant === 'form' ? effectiveHelpText : undefined}
        helpTextClassName="help-text"
        loadingLabel={{ form: t('pickers.voice.loading'), toolbar: t('pickers.voice.loading') }}
        loadingLabelVisuallyHidden={{ toolbar: true }}
        loadingClassName={{ form: 'loading-state', toolbar: 'voice-picker-toolbar' }}
        errorClassName={{ form: 'error-state', toolbar: 'voice-picker-toolbar voice-picker-error' }}
        errorLabel={{ form: error || t('pickers.voice.loadError'), toolbar: '' }}
        errorLabelVisuallyHidden={{ toolbar: true }}
        errorIcon={{ form: '⚠️', toolbar: '⚠️' }}
        retryClassName="retry-btn"
      />
    );
  }
);
