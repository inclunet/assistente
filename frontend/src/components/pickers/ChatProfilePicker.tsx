import { useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import { Combobox, ComboboxItem } from './Combobox';
import { GetChatProfiles } from '../../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime';
import { database } from '../../../wailsjs/go/models';

type ChatProfile = database.ChatProfile;

export interface ChatProfilePickerProps {
  value: number;
  onChange: (profileId: number) => void;
  variant?: 'toolbar' | 'form';
  label?: string;
  icon?: string;
  maxWidth?: string;
  onAnnounce?: (message: string) => void;
}

export interface ChatProfilePickerRef {
  reload: () => Promise<void>;
  getSelectedProfile: () => ChatProfile | undefined;
}

export const ChatProfilePicker = forwardRef<ChatProfilePickerRef, ChatProfilePickerProps>(
  (
    {
      value,
      onChange,
      variant: _variant = 'form', // eslint-disable-line @typescript-eslint/no-unused-vars
      label = 'Perfil de Conversa',
      icon = '💬',
      maxWidth,
      onAnnounce,
    },
    ref
  ) => {
    const [profiles, setProfiles] = useState<ChatProfile[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const loadProfiles = async () => {
      setLoading(true);
      setError(null);

      try {
        const allProfiles = await GetChatProfiles();
        setProfiles(allProfiles || []);
        console.log('[ChatProfilePicker] Loaded', allProfiles?.length || 0, 'profiles');
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

    // Escuta eventos de atualização de perfis
    useEffect(() => {
      const handleCreated = () => loadProfiles();
      const handleUpdated = () => loadProfiles();
      const handleDeleted = () => loadProfiles();

      EventsOn('chat:profile:created', handleCreated);
      EventsOn('chat:profile:updated', handleUpdated);
      EventsOn('chat:profile:deleted', handleDeleted);

      return () => {
        EventsOff('chat:profile:created');
        EventsOff('chat:profile:updated');
        EventsOff('chat:profile:deleted');
      };
    }, []);

    useImperativeHandle(ref, () => ({
      reload: loadProfiles,
      getSelectedProfile: () => profiles.find(p => p.id === value),
    }));

    // Constrói itens do combobox
    const buildItems = (): ComboboxItem[] => {
      return profiles.map(profile => {
        const defaultMark = profile.is_default ? ' ⭐' : '';
        const toolsIndicator = profile.use_tools ? '🔧' : '';
        return {
          value: profile.id.toString(),
          label: `${profile.icon || '💬'} ${profile.name}${defaultMark} ${toolsIndicator}`.trim(),
          sublabel: profile.description || profile.model || undefined,
        };
      });
    };

    const handleSelect = (newValue: string) => {
      const profileId = parseInt(newValue, 10);
      if (!isNaN(profileId)) {
        const profile = profiles.find(p => p.id === profileId);
        onChange(profileId);
        onAnnounce?.(`Perfil de conversa alterado para ${profile?.name || profileId}`);
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

    // Empty state
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

ChatProfilePicker.displayName = 'ChatProfilePicker';
