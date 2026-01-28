import { useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import { Combobox, ComboboxItem } from './Combobox';
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

// Mapeia provider para label amigável
const providerLabels: Record<TTSProvider, string> = {
  [TTSProvider.DISABLED]: 'Desativado',
  [TTSProvider.WEBSPEECH]: 'Sistema',
  [TTSProvider.SAPI5]: 'Windows (SAPI5)',
  [TTSProvider.OPENAI]: 'OpenAI (Premium)'
};

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
      label = 'Voz',
      helpText = 'Selecione a voz para síntese de fala',
      icon = '🔊',
      maxWidth,
      allowDisabled = true,
      onAnnounce,
    },
    ref
  ) => {
    const [voices, setVoices] = useState<TTSVoice[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const loadVoices = async () => {
      setLoading(true);
      setError(null);

      try {
        // Busca vozes de TODOS os provedores
        const allVoices = await ttsService.getVoices();
        
        console.log('[VoicePicker] Loaded', allVoices.length, 'voices from all providers');
        
        setVoices(allVoices);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Erro ao carregar vozes');
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
    const disabledOption: ComboboxItem = {
      value: VOICE_DISABLED,
      label: '🔇 Desativada (usar leitor de telas)',
      sublabel: 'Acessibilidade',
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
          sublabel: `${providerLabel} • ${voice.language}${voice.premium ? ' • Premium' : ''}`
        });
      });
    }

    if (variant === 'toolbar') {
      if (loading) {
        return (
          <div className="voice-picker-toolbar" role="status" aria-live="polite">
            <span className="loading-spinner" aria-hidden="true" />
            <span className="sr-only">Carregando vozes...</span>
          </div>
        );
      }

      if (error) {
        return (
          <div className="voice-picker-toolbar voice-picker-error" role="alert">
            <span>⚠️</span>
            <button onClick={loadVoices} className="retry-btn">
              Tentar novamente
            </button>
          </div>
        );
      }

      return (
        <Combobox
          items={items}
          selected={value}
          onSelect={onChange}
          label={label}
          icon={icon}
          maxWidth={maxWidth}
          onAnnounce={onAnnounce}
        />
      );
    }

    // Form variant
    return (
      <div className="voice-picker-form">
        <label className="voice-picker-label">
          {icon && <span className="voice-picker-icon">{icon}</span>}
          {label}
        </label>
        
        {loading ? (
          <div className="loading-state" role="status" aria-live="polite">
            <span className="loading-spinner" aria-hidden="true" />
            <span>Carregando vozes...</span>
          </div>
        ) : error ? (
          <div className="error-state" role="alert">
            <span className="error-icon">⚠️</span>
            <span>{error}</span>
            <button onClick={loadVoices} className="retry-btn">
              Tentar novamente
            </button>
          </div>
        ) : (
          <Combobox
            items={items}
            selected={value}
            onSelect={onChange}
            label={label}
            icon={icon}
            onAnnounce={onAnnounce}
          />
        )}
        
        {helpText && <p className="help-text">{helpText}</p>}
      </div>
    );
  }
);
