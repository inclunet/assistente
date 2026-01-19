import React, { useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import { Combobox, ComboboxItem } from './Combobox';
import './VoicePicker.css';

export interface VoicePickerProps {
  value: string;
  onChange: (voice: string) => void;
  variant?: 'toolbar' | 'form';
  label?: string;
  helpText?: string;
  icon?: string;
  maxWidth?: string;
}

export interface VoicePickerRef {
  reload: () => Promise<void>;
}

interface Voice {
  id: string;
  name: string;
  language: string;
}

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
    },
    ref
  ) => {
    const [voices, setVoices] = useState<Voice[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const loadVoices = async () => {
      setLoading(true);
      setError(null);

      try {
        // TODO: Replace with actual Wails backend call
        // const voicesList = await GetVoices() || [];
        
        // Simulated voices for now
        const voicesList: Voice[] = [
          { id: 'pt-BR-FranciscaNeural', name: 'Francisca (Feminina)', language: 'pt-BR' },
          { id: 'pt-BR-AntonioNeural', name: 'Antonio (Masculino)', language: 'pt-BR' },
          { id: 'en-US-JennyNeural', name: 'Jenny (Female)', language: 'en-US' },
          { id: 'en-US-GuyNeural', name: 'Guy (Male)', language: 'en-US' },
        ];
        
        setVoices(voicesList);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Erro ao carregar vozes');
        console.error('Failed to load voices:', err);
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

    const items: ComboboxItem[] = voices.map((voice) => ({
      value: voice.id,
      label: voice.name,
      sublabel: voice.language,
    }));

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
          />
        )}
        
        {helpText && <p className="help-text">{helpText}</p>}
      </div>
    );
  }
);
