import { useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import { Combobox, ComboboxItem } from './Combobox';
import { GetChatProfiles, GetDefaultChatProfile, SetDefaultChatProfile } from '../../../wailsjs/go/main/App';

export interface ChatProfilePickerProps {
  value?: number;
  onChange?: (profileId: number) => void;
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
      onChange,
      variant: _variant = 'form',
      label = 'Perfil',
      icon = '💬',
      maxWidth,
      onAnnounce,
    },
    ref
  ) => {
    const [chatProfiles, setChatProfiles] = useState<any[]>([]);
    const [activeProfileId, setActiveProfileId] = useState<number>(0);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const loadProfiles = async () => {
      setLoading(true);
      setError(null);

      try {
        const allProfiles = await GetChatProfiles();
        const defaultProfile = await GetDefaultChatProfile();
        setChatProfiles(allProfiles || []);
        setActiveProfileId(defaultProfile?.id || 0);
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
      getSelectedProfile: () => chatProfiles.find(p => p.id === activeProfileId),
    }));

    const buildItems = (): ComboboxItem[] => {
      return chatProfiles.map(profile => {
        return {
          value: String(profile.id),
          label: `💬 ${profile.name}`.trim(),
          sublabel: profile.model || undefined,
        };
      });
    };

    const handleSelect = async (newValue: string) => {
      try {
        const profileId = parseInt(newValue, 10);
        await SetDefaultChatProfile(profileId);
        setActiveProfileId(profileId);
        const profile = chatProfiles.find(p => p.id === profileId);
        onAnnounce?.(`Perfil alterado para ${profile?.name || newValue}`);
        onChange?.(profileId);
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

    if (chatProfiles.length === 0) {
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
          selected={String(activeProfileId)}
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
