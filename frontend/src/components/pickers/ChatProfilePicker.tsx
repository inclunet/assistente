import { useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import { Combobox, ComboboxItem } from './Combobox';
import { GetProfiles, GetActiveProfileName, SetActiveProfile } from '../../../wailsjs/go/main/App';

interface UnifiedProfile {
  name: string;
  description?: string;
  icon?: string;
  chat: {
    model?: string;
    use_tools?: boolean;
  };
}

export interface ChatProfilePickerProps {
  value: number; // Mantido para compatibilidade, mas ignorado internamente
  onChange: (profileId: number) => void; // Mantido para compatibilidade
  variant?: 'toolbar' | 'form';
  label?: string;
  icon?: string;
  maxWidth?: string;
  onAnnounce?: (message: string) => void;
}

export interface ChatProfilePickerRef {
  reload: () => Promise<void>;
  getSelectedProfile: () => any;
}

export const ChatProfilePicker = forwardRef<ChatProfilePickerRef, ChatProfilePickerProps>(
  (
    {
      onChange: _onChange,
      variant: _variant = 'form',
      label = 'Perfil',
      icon = '💬',
      maxWidth,
      onAnnounce,
    },
    ref
  ) => {
    const [profiles, setProfiles] = useState<UnifiedProfile[]>([]);
    const [activeProfileName, setActiveProfileName] = useState<string>('');
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const loadProfiles = async () => {
      setLoading(true);
      setError(null);

      try {
        const allProfiles = await GetProfiles();
        const activeName = await GetActiveProfileName();
        setProfiles(allProfiles || []);
        setActiveProfileName(activeName || '');
        console.log('[ChatProfilePicker] Loaded', allProfiles?.length || 0, 'profiles, active:', activeName);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Erro ao carregar perfis');
        console.error('[ChatProfilePicker] Failed to load profiles:', err);
      } finally {
        setLoading(false);
      }
    };

    useEffect(() => {
      loadProfiles();
    }, []);

    useImperativeHandle(ref, () => ({
      reload: loadProfiles,
      getSelectedProfile: () => profiles.find(p => p.name === activeProfileName),
    }));

    const buildItems = (): ComboboxItem[] => {
      return profiles.map(profile => {
        const toolsIndicator = profile.chat?.use_tools ? '🔧' : '';
        return {
          value: profile.name,
          label: `${profile.icon || '💬'} ${profile.name} ${toolsIndicator}`.trim(),
          sublabel: profile.description || profile.chat?.model || undefined,
        };
      });
    };

    const handleSelect = async (newValue: string) => {
      try {
        await SetActiveProfile(newValue);
        setActiveProfileName(newValue);
        onAnnounce?.(`Perfil alterado para ${newValue}`);
      } catch (err) {
        console.error('[ChatProfilePicker] Error setting profile:', err);
      }
    };

    if (loading) {
      return (
        <div className="voice-picker voice-picker--loading" role="status" aria-live="polite">
          <span className="voice-picker__icon">{icon}</span>
          <span className="voice-picker__loading">Carregando...</span>
        </div>
      );
    }

    if (error) {
      return (
        <div className="voice-picker voice-picker--error" role="alert" aria-live="assertive">
          <span className="voice-picker__icon">⚠️</span>
          <span className="voice-picker__error">{error}</span>
        </div>
      );
    }

    if (profiles.length === 0) {
      return (
        <div className="voice-picker voice-picker--empty">
          <span className="voice-picker__icon">{icon}</span>
          <span>Nenhum perfil</span>
        </div>
      );
    }

    return (
      <div className="voice-picker" data-picker="chat-profile">
        <Combobox
          selected={activeProfileName}
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

ChatProfilePicker.displayName = 'ChatProfilePicker';
