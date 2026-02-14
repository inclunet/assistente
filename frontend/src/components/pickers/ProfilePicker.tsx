import { useState, useEffect, forwardRef, useImperativeHandle, useCallback } from 'react';
import { Combobox, ComboboxItem } from './Combobox';
import { GetProfiles, GetActiveProfileSlug, SetActiveProfile } from '../../../wailsjs/go/main/App';
import { EventsOn } from '../../../wailsjs/runtime/runtime';

export interface ProfilePickerProps {
  /** Callback when profile is selected */
  onChange?: (slug: string) => void;
  variant?: 'toolbar' | 'form';
  label?: string;
  icon?: string;
  maxWidth?: string;
  onAnnounce?: (message: string) => void;
  /**
   * Controlled value (slug). When set, the picker does NOT change the global
   * active profile — it only calls onChange with the selected slug.
   * Use this for channel/form pickers that need a local selection.
   */
  value?: string;
}

export interface ProfilePickerRef {
  reload: () => Promise<void>;
}

export const ProfilePicker = forwardRef<ProfilePickerRef, ProfilePickerProps>(
  (
    {
      onChange,
      label = 'Perfil',
      icon = '💬',
      maxWidth,
      onAnnounce,
      value,
    },
    ref
  ) => {
    const isControlled = value !== undefined;
    const [profileList, setProfileList] = useState<Array<{ name: string; slug: string; description: string; icon: string; source: string }>>([]);
    const [activeSlug, setActiveSlug] = useState<string>('padrao');
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const loadProfiles = useCallback(async () => {
      setLoading(true);
      setError(null);

      try {
        if (isControlled) {
          // Controlled mode: only load profile list, don't read global active
          const allProfiles = await GetProfiles();
          setProfileList(allProfiles || []);
        } else {
          const [allProfiles, currentSlug] = await Promise.all([
            GetProfiles(),
            GetActiveProfileSlug(),
          ]);
          setProfileList(allProfiles || []);
          setActiveSlug(currentSlug || 'padrao');
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Erro ao carregar perfis');
        console.error('[ProfilePicker] Failed to load profiles:', err);
      } finally {
        setLoading(false);
      }
    }, [isControlled]);

    useEffect(() => {
      loadProfiles();
    }, [loadProfiles]);

    // Escuta eventos de mudança de perfil (apenas no modo global)
    useEffect(() => {
      if (isControlled) {
        // No controlled mode, still reload list on profile CRUD events
        const unsubCreated = EventsOn('profile:created', () => loadProfiles());
        const unsubDeleted = EventsOn('profile:deleted', () => loadProfiles());
        const unsubUpdated = EventsOn('profile:updated', () => loadProfiles());
        return () => { unsubCreated(); unsubDeleted(); unsubUpdated(); };
      }

      const unsubChanged = EventsOn('profile:changed', (data: { slug: string }) => {
        setActiveSlug(data.slug);
      });
      const unsubCreated = EventsOn('profile:created', () => {
        loadProfiles();
      });
      const unsubDeleted = EventsOn('profile:deleted', () => {
        loadProfiles();
      });
      const unsubUpdated = EventsOn('profile:updated', () => {
        loadProfiles();
      });

      return () => {
        unsubChanged();
        unsubCreated();
        unsubDeleted();
        unsubUpdated();
      };
    }, [loadProfiles, isControlled]);

    useImperativeHandle(ref, () => ({
      reload: loadProfiles,
    }));

    const buildItems = (): ComboboxItem[] => {
      return profileList.map(profile => ({
        value: profile.slug,
        label: `${profile.name}`.trim(),
        sublabel: profile.description || undefined,
      }));
    };

    const handleSelect = async (newValue: string) => {
      if (isControlled) {
        // Controlled mode: just call onChange, don't set global profile
        const profile = profileList.find(p => p.slug === newValue);
        onAnnounce?.(`Perfil selecionado: ${profile?.name || newValue}`);
        onChange?.(newValue);
        return;
      }

      try {
        await SetActiveProfile(newValue);
        setActiveSlug(newValue);
        const profile = profileList.find(p => p.slug === newValue);
        onAnnounce?.(`Perfil alterado para ${profile?.name || newValue}`);
        onChange?.(newValue);
      } catch (err) {
        console.error('[ProfilePicker] Error setting profile:', err);
      }
    };

    // Effective selected value
    const selectedSlug = isControlled ? (value || '') : activeSlug;

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

    if (profileList.length === 0) {
      return (
        <div className="voice-picker voice-picker--empty">
          <span className="voice-picker__icon">{icon}</span>
          <span>Nenhum perfil</span>
        </div>
      );
    }

    return (
      <div className="voice-picker" data-picker="profile">
        <Combobox
          selected={selectedSlug}
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

ProfilePicker.displayName = 'ProfilePicker';
