/**
 * InteractionProfilePicker - Seletor de Perfil de Interação por Voz
 * 
 * Permite selecionar o perfil de interação ativo na toolbar.
 * O perfil controla: provider STT, idioma e triggers de ativação.
 */

import { useState, useEffect, forwardRef, useImperativeHandle } from 'react';
import { Combobox, ComboboxItem } from './Combobox';
import { GetInteractionProfiles } from '../../../wailsjs/go/main/App';
import { database } from '../../../wailsjs/go/models';
import { useInteractionProfileStore } from '../../store/interactionProfileStore';
import './VoicePicker.css';

type InteractionProfile = database.InteractionProfile;
type InteractionTrigger = database.InteractionTrigger;

export interface InteractionProfilePickerProps {
  value: number;
  onChange: (profileId: number) => void;
  variant?: 'toolbar' | 'form';
  label?: string;
  icon?: string;
  maxWidth?: string;
  onAnnounce?: (message: string) => void;
}

export interface InteractionProfilePickerRef {
  reload: () => Promise<void>;
  getSelectedProfile: () => InteractionProfile | undefined;
}

// Labels dos tipos de trigger
const triggerTypeLabels: Record<string, string> = {
  'hotkey': 'Hotkey',
  'button_ptt': 'PTT',
  'button_toggle': 'Toggle',
  'wakeword': 'Wakeword',
  'vad': 'VAD',
};

// Função para obter o ícone principal do perfil baseado nos triggers
const getProfileIcon = (triggers?: InteractionTrigger[]): string => {
  if (!triggers || triggers.length === 0) return '🔘';
  
  const enabledTriggers = triggers.filter(t => t.enabled);
  if (enabledTriggers.length === 0) return '🔘';
  
  // Prioriza por tipo mais específico
  const hasPTT = enabledTriggers.some(t => t.type === 'button_ptt');
  const hasToggle = enabledTriggers.some(t => t.type === 'button_toggle');
  const hasWakeword = enabledTriggers.some(t => t.type === 'wakeword');
  const hasHotkey = enabledTriggers.some(t => t.type === 'hotkey');
  const hasVAD = enabledTriggers.some(t => t.type === 'vad');
  
  if (hasWakeword) return '🗣️';
  if (hasHotkey) return '⌨️';
  if (hasVAD) return '🔊';
  if (hasPTT) return '🎙️';
  if (hasToggle) return '🔘';
  return '🎤';
};

// Função para criar sublabel com resumo dos triggers
const getTriggersSummary = (triggers?: InteractionTrigger[]): string => {
  if (!triggers || triggers.length === 0) return 'Sem triggers';
  
  const enabledTriggers = triggers.filter(t => t.enabled);
  if (enabledTriggers.length === 0) return 'Triggers desativados';
  
  const parts: string[] = [];
  
  for (const trigger of enabledTriggers) {
    if (trigger.type === 'hotkey' && trigger.hotkey) {
      parts.push(trigger.hotkey);
    } else if (trigger.type === 'wakeword' && trigger.wakeword_keyword) {
      parts.push(`"${trigger.wakeword_keyword}"`);
    } else {
      parts.push(triggerTypeLabels[trigger.type] || trigger.type);
    }
  }
  
  return parts.slice(0, 2).join(' • ') + (parts.length > 2 ? ` +${parts.length - 2}` : '');
};

export const InteractionProfilePicker = forwardRef<InteractionProfilePickerRef, InteractionProfilePickerProps>(
  (
    {
      value,
      onChange,
      label = 'Perfil de Interação',
      icon = '🎙️',
      maxWidth,
      onAnnounce,
    },
    ref
  ) => {
    const [profiles, setProfiles] = useState<InteractionProfile[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    
    const { setActiveProfile } = useInteractionProfileStore();

    const loadProfiles = async () => {
      setLoading(true);
      setError(null);

      try {
        const allProfiles = await GetInteractionProfiles();
        setProfiles(allProfiles || []);
        console.log('[InteractionProfilePicker] Loaded', allProfiles?.length || 0, 'profiles');
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Erro ao carregar perfis');
        console.error('[InteractionProfilePicker] Failed to load profiles:', err);
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

    // Constrói itens do combobox
    const buildItems = (): ComboboxItem[] => {
      return profiles.map(profile => {
        const profileIcon = getProfileIcon(profile.triggers);
        const defaultMark = profile.is_default ? ' ⭐' : '';
        const sublabel = getTriggersSummary(profile.triggers);
        
        return {
          value: profile.id.toString(),
          label: `${profileIcon} ${profile.name}${defaultMark}`,
          sublabel,
        };
      });
    };

    const handleSelect = (newValue: string, _item: ComboboxItem) => {
      const profileId = parseInt(newValue, 10);
      if (!isNaN(profileId)) {
        onChange(profileId);
        setActiveProfile(profileId);
        
        const selectedProfile = profiles.find(p => p.id === profileId);
        if (selectedProfile && onAnnounce) {
          const triggerSummary = getTriggersSummary(selectedProfile.triggers);
          onAnnounce(`Perfil de interação alterado para ${selectedProfile.name}. ${triggerSummary}`);
        }
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
      <div className="voice-picker" data-picker="interaction-profile">
        <Combobox
          items={buildItems()}
          selected={value?.toString() || ''}
          onSelect={handleSelect}
          placeholder="Filtrar perfis..."
          label={label}
          icon={icon}
          maxWidth={maxWidth}
          onAnnounce={onAnnounce}
        />
      </div>
    );
  }
);

InteractionProfilePicker.displayName = 'InteractionProfilePicker';

export default InteractionProfilePicker;
