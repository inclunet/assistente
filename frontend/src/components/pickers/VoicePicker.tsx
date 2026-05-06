import { useState, useEffect, useRef, forwardRef, useImperativeHandle, type ReactNode } from 'react';
import { SoundOutlined, WarningOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { ComboboxItem } from './Combobox';
import { BasePicker } from './BasePicker';
import { ttsService } from '../../services/tts';
import { TTSVoice, TTSProvider } from '../../services/tts/types';
import './VoicePicker.css';

// Valor especial para voz desativada (usa leitor de telas)
export const VOICE_DISABLED = '__disabled__';

// Valores especiais para referenciar outras vozes
export const VOICE_REF_ASSISTANT = '__ref_assistant__';
export const VOICE_REF_USER = '__ref_user__';
export const VOICE_REF_SYSTEM = '__ref_system__';

export interface VoicePickerProps {
  value: string;
  onChange: (voice: string) => void;
  providerId?: string; // NOVO: Filtra por provedor
  modelId?: string;    // Modelo TTS usado para listar vozes HTTP
  profileId?: string;  // NOVO: Usado para buscar vozes do provedor
  variant?: 'toolbar' | 'form';
  label?: string;
  helpText?: string;
  icon?: ReactNode;
  maxWidth?: string;
  allowDisabled?: boolean;
  references?: Array<{ id: string; label: string }>;
  onAnnounce?: (message: string) => void;
  /** Vozes pré-definidas (ex: OpenAI com variantes HD). Quando fornecido, pula busca no backend. */
  voiceOverrides?: TTSVoice[];
}

export interface VoicePickerRef {
  reload: () => Promise<void>;
}

export const VoicePicker = forwardRef<VoicePickerRef, VoicePickerProps>(
  (
    {
      value,
      onChange,
      providerId,
      modelId,
      profileId,
      variant = 'form',
      label,
      helpText,
      icon = <SoundOutlined />,
      maxWidth,
      allowDisabled = true,
      references = [],
      onAnnounce,
      voiceOverrides,
    },
    ref
  ) => {
    const { t } = useTranslation();
    const effectiveLabel = label ?? t('pickers.voice.label');
    const effectiveHelpText = helpText ?? t('pickers.voice.description');
    const [voices, setVoices] = useState<TTSVoice[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const reloadCancelRef = useRef<(() => void) | null>(null);

    const loadVoices = async (cancelled?: () => boolean) => {
      const voiceModelId = modelId ?? '';

      // Vozes pré-definidas (ex: OpenAI com variantes HD): pula backend
      if (voiceOverrides && voiceOverrides.length > 0) {
        setVoices(voiceOverrides);
        setLoading(false);
        setError(null);
        return;
      }

      // Provedores especiais não têm vozes próprias
      const isSpecialProvider = !providerId || providerId === 'disabled' || providerId.startsWith('ref_');
      
      if (isSpecialProvider && !allowDisabled) {
        setVoices([]);
        setLoading(false);
        return;
      }

      if (isSpecialProvider) {
        // Para provedores especiais com allowDisabled, mantemos a lista vazia (só mostra a opção "disabled")
        setVoices([]);
        setLoading(false);
        return;
      }

      setLoading(true);
      setError(null);

      try {
        let allVoices: TTSVoice[] = [];
        
        if (providerId && profileId) {
          // Busca específica para este provedor
          allVoices = await ttsService.getVoicesForProvider(providerId, profileId, voiceModelId);
        } else if (providerId) {
          // Tem provedor mas não tem profileId — tenta com string vazia
          allVoices = await ttsService.getVoicesForProvider(providerId, '', voiceModelId);
        } else {
          // Fallback legada: busca todas as vozes (usado na Home/Toolbar se não houver perfil)
          allVoices = await ttsService.getVoices();
        }

        // Ignora resultado se o efeito já foi cancelado (nova deps chegaram)
        if (cancelled?.()) return;

        setVoices(allVoices);
      } catch (err) {
        if (cancelled?.()) return;
        setError(err instanceof Error ? err.message : t('pickers.voice.loadError'));
        console.error('[VoicePicker] Failed to load voices:', err);
      } finally {
        if (!cancelled?.()) setLoading(false);
      }
    };

    useEffect(() => {
      let isCancelled = false;
      loadVoices(() => isCancelled);
      return () => { isCancelled = true; };
    }, [providerId, profileId, modelId, voiceOverrides]);

    /** Inicia fetch com cancellation (cancela qualquer fetch manual anterior) */
    const reloadWithCancel = () => {
      reloadCancelRef.current?.();
      let cancelled = false;
      reloadCancelRef.current = () => { cancelled = true; };
      return loadVoices(() => cancelled);
    };

    useImperativeHandle(ref, () => ({
      reload: reloadWithCancel,
    }));

    // Agrupa vozes por provider
    const voicesByProvider = voices.reduce((acc, voice) => {
      const pId = voice.provider.toString();
      if (!acc[pId]) {
        acc[pId] = [];
      }
      acc[pId].push(voice);
      return acc;
    }, {} as Record<string, TTSVoice[]>);

    // Opção de desativado
    const providerLabels: Record<string, string> = {
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
      ...references.map(ref => ({
        value: ref.id,
        label: ref.label,
        sublabel: t('pickers.voice.reference'),
      })),
      ...(allowDisabled ? [disabledOption] : []),
    ];

    // Adiciona vozes agrupadas por provider
    const providerOrder = [
      TTSProvider.WEBSPEECH,
      TTSProvider.SAPI5,
      TTSProvider.OPENAI,
    ];

    if (providerId && !providerOrder.includes(providerId as TTSProvider)) {
      providerOrder.unshift(providerId as TTSProvider);
    }

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
        const providerLabel = providerLabels[voice.provider] ?? String(voice.provider);
        
        items.push({
          value: voice.id,
          label: voice.name,
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
        onRetry={reloadWithCancel}
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
        errorIcon={{ form: <WarningOutlined />, toolbar: <WarningOutlined /> }}
        retryClassName="retry-btn"
      />
    );
  }
);
