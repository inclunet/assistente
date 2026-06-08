import { logger } from '../../utils/logger';
import { useState, useEffect, forwardRef, useImperativeHandle, useCallback, type ReactNode } from 'react';
import { MessageOutlined, WarningOutlined } from '@ant-design/icons';
import { ComboboxItem } from './Combobox';
import { BasePicker } from './BasePicker';
import { GetProfiles, GetActiveProfileSlug, SetActiveProfile } from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { useTranslation } from 'react-i18next';

export interface ProfilePickerProps {
  /** Callback when profile is selected */
  onChange?: (slug: string) => void;
  variant?: 'toolbar' | 'form';
  label?: string;
  description?: string;
  icon?: ReactNode;
  maxWidth?: string;
  onAnnounce?: (message: string) => void;
  /** Called after a profile is selected. Use to customize focus restoration. */
  onAfterSelect?: () => void;
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
      variant,
      label = 'Perfil',
      description,
      icon = <MessageOutlined />,
      maxWidth,
      onAnnounce,
      onAfterSelect,
      value,
    },
    ref
  ) => {
    const isControlled = value !== undefined;
    const { t } = useTranslation();
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
        logger.error('[ProfilePicker] Failed to load profiles:', err);
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
      const items: ComboboxItem[] = [];
      // In controlled mode (channel pickers), add option to use global active profile
      if (isControlled) {
        items.push({
          value: '',
          label: t('profiles.useActiveGlobal'),
        });
      }
      for (const profile of profileList) {
        items.push({
          value: profile.slug,
          label: `${profile.name}`.trim(),
          sublabel: profile.description || undefined,
        });
      }
      return items;
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
        logger.error('[ProfilePicker] Error setting profile:', err);
      }
    };

    // Effective selected value
    const selectedSlug = isControlled ? (value || '') : activeSlug;

    const loadingState = (
      <div className="voice-picker voice-picker--loading" role="status" aria-live="polite">
        <span className="voice-picker__icon" aria-hidden="true">{icon}</span>
        <span className="voice-picker__loading">Carregando...</span>
      </div>
    );

    const errorState = (
      <div className="voice-picker voice-picker--error" role="alert" aria-live="assertive">
        <span className="voice-picker__icon"><WarningOutlined aria-hidden="true" /></span>
        <span className="voice-picker__error">{error}</span>
      </div>
    );

    const emptyState = (
      <div className="voice-picker voice-picker--empty">
        <span className="voice-picker__icon" aria-hidden="true">{icon}</span>
        <span>Nenhum perfil</span>
      </div>
    );

    return (
      <BasePicker
        variant={variant ?? 'form'}
        items={buildItems()}
        selected={selectedSlug}
        onSelect={handleSelect}
        label={label}
        description={description}
        icon={icon}
        maxWidth={maxWidth}
        onAnnounce={onAnnounce}
        loading={loading}
        error={error}
        loadingState={loadingState}
        errorState={errorState}
        emptyState={emptyState}
        showFormLabel={false}
        wrapCombobox
        comboboxWrapperClassName="voice-picker"
        comboboxWrapperProps={{ 'data-picker': 'profile' } as React.HTMLAttributes<HTMLDivElement>}
        onAfterSelect={onAfterSelect}
      />
    );
  }
);

ProfilePicker.displayName = 'ProfilePicker';
