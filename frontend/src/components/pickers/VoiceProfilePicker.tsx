import { useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import { Combobox, ComboboxItem } from './Combobox';
import { GetAllVoiceProfiles } from '../../../wailsjs/go/main/App';
import './VoicePicker.css';

export interface VoiceProfile {
  id: number;
  name: string;
  description: string;
  provider: string;
  voice_id: string;
  rate: number;
  pitch: number;
  volume: number;
  enabled_for_agent: boolean;
  enabled_for_user: boolean;
  is_default: boolean;
}

export interface VoiceProfilePickerProps {
  value: number;
  onChange: (profileId: number) => void;
  variant?: 'toolbar' | 'form';
  label?: string;
  icon?: string;
  maxWidth?: string;
  onAnnounce?: (message: string) => void;
}

export interface VoiceProfilePickerRef {
  reload: () => Promise<void>;
  getSelectedProfile: () => VoiceProfile | undefined;
}

// Ícones por provider
const providerIcons: Record<string, string> = {
  'disabled': '🔇',
  'webspeech': '🔊',
  'sapi5': '🪟',
  'openai': '💎'
};

export const VoiceProfilePicker = forwardRef<VoiceProfilePickerRef, VoiceProfilePickerProps>(
  (
    {
      value,
      onChange,
      variant = 'form',
      label = 'Perfil de Voz',
      icon = '🔊',
      maxWidth,
      onAnnounce,
    },
    ref
  ) => {
    const [profiles, setProfiles] = useState<VoiceProfile[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const loadProfiles = async () => {
      setLoading(true);
      setError(null);

      try {
        const allProfiles = await GetAllVoiceProfiles();
        setProfiles(allProfiles || []);
        console.log('[VoiceProfilePicker] Loaded', allProfiles?.length || 0, 'profiles');
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Erro ao carregar perfis');
        console.error('[VoiceProfilePicker] Failed to load profiles:', err);
      } finally {
        setLoading(false);
      }
    };

    useEffect(() => {
      loadProfiles();
    }, []);

    useImperativeHandle(ref, () => ({
      reload: loadProfiles,
      getSelectedProfile: () => profiles.find(p => p.id === value),
    }));

    // Constrói itens do combobox (apenas perfis, sem opções especiais)
    const buildItems = (): ComboboxItem[] => {
      return profiles.map(profile => {
        const providerIcon = providerIcons[profile.provider] || '🔊';
        const defaultMark = profile.is_default ? ' ⭐' : '';
        return {
          value: profile.id.toString(),
          label: `${providerIcon} ${profile.name}${defaultMark}`,
          sublabel: profile.description || undefined,
        };
      });
    };

    const handleSelect = (newValue: string) => {
      const profileId = parseInt(newValue, 10);
      if (!isNaN(profileId)) {
        const profile = profiles.find(p => p.id === profileId);
        onChange(profileId);
        onAnnounce?.(`Perfil de voz alterado para ${profile?.name || profileId}`);
      }
    };

    // Loading state
    if (loading) {
      return (
        <div className="voice-picker voice-picker--loading" role="status" aria-live="polite">
          <span className="voice-picker__icon">{icon}</span>
          <span className="voice-picker__loading">Carregando...</span>
        </div>
      );
    }

    // Error state
    if (error) {
      return (
        <div className="voice-picker voice-picker--error" role="alert" aria-live="assertive">
          <span className="voice-picker__icon">⚠️</span>
          <span className="voice-picker__error">{error}</span>
        </div>
      );
    }

    // Empty state - mostra mensagem simples
    if (profiles.length === 0) {
      return (
        <div className="voice-picker voice-picker--empty">
          <span className="voice-picker__icon">{icon}</span>
          <span>Nenhum perfil</span>
        </div>
      );
    }

    // Sempre passa o label para o Combobox (necessário para aria-label)
    return (
      <div className="voice-picker" data-picker="voice-profile">
        <Combobox
          selected={value.toString()}
          onSelect={handleSelect}
          items={buildItems()}
          label={label}
          icon={icon}
          maxWidth={maxWidth}
        />
      </div>
    );
  }
);

VoiceProfilePicker.displayName = 'VoiceProfilePicker';
