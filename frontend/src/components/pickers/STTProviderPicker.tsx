import { useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import { Combobox, ComboboxItem } from './Combobox';
import './STTProviderPicker.css';

// Constantes de provedores STT
export const STT_WEBSPEECH = 'webspeech';
export const STT_WHISPER = 'whisper';

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
      label = 'STT',
      helpText = 'Selecione o provedor de reconhecimento de fala',
      icon = '🎤',
      maxWidth,
      onAnnounce,
    },
    ref
  ) => {
    const [providers, setProviders] = useState<STTProvider[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const loadProviders = async () => {
      setLoading(true);
      setError(null);

      try {
        // Provedores disponíveis
        const providersList: STTProvider[] = [
          { 
            id: STT_WEBSPEECH, 
            name: 'WebSpeech', 
            description: 'Navegador (grátis)',
            icon: '🌐'
          },
          { 
            id: STT_WHISPER, 
            name: 'Whisper', 
            description: 'OpenAI (premium)',
            icon: '🤖'
          },
        ];
        
        setProviders(providersList);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Erro ao carregar provedores');
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

    if (variant === 'toolbar') {
      if (loading) {
        return (
          <div className="stt-picker-toolbar" role="status" aria-live="polite">
            <span className="loading-spinner" aria-hidden="true" />
            <span className="sr-only">Carregando provedores...</span>
          </div>
        );
      }

      if (error) {
        return (
          <div className="stt-picker-toolbar stt-picker-error" role="alert">
            <span>⚠️</span>
            <button onClick={loadProviders} className="retry-btn">
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
      <div className="stt-picker-form">
        <label className="stt-picker-label">
          {icon && <span className="stt-picker-icon">{icon}</span>}
          {label}
        </label>
        
        {loading ? (
          <div className="loading-state" role="status" aria-live="polite">
            <span className="loading-spinner" aria-hidden="true" />
            <span>Carregando provedores...</span>
          </div>
        ) : error ? (
          <div className="error-state" role="alert">
            <span className="error-icon">⚠️</span>
            <span>{error}</span>
            <button onClick={loadProviders} className="retry-btn">
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
