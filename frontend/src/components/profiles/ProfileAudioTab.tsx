import { useTranslation } from 'react-i18next';
import { profiles } from '@wailsjs/go/models';
import { CollapsibleSection } from '../ui/CollapsibleSection';
import { ProfileVoiceSection } from './ProfileVoiceSection';
import { ProfileInteractionSection } from './ProfileInteractionSection';
import { VOICE_DISABLED } from '../pickers/VoicePicker';

export interface ProfileAudioTabProps {
  editingProfile: profiles.Profile;
  updateField: (path: string, value: unknown) => void;
}

export function ProfileAudioTab({ editingProfile, updateField }: ProfileAudioTabProps) {
  const { t } = useTranslation();

  const isVoiceDisabled = editingProfile.voice?.provider === 'disabled';
  const isSTTDisabled = !editingProfile.interaction?.stt_provider;

  return (
    <>
      {/* Voice (TTS) — colapsável */}
      <CollapsibleSection
        title={t('profiles.collapseVoice', 'Voz (TTS)')}
        isOpen={!isVoiceDisabled}
        onToggle={() => {
          if (isVoiceDisabled) {
            updateField('voice.provider', 'webspeech');
          } else {
            updateField('voice.provider', 'disabled');
            updateField('voice.voice_id', '');
          }
        }}
        badge={isVoiceDisabled ? 'off' : 'on'}
      >
        <ProfileVoiceSection
          voice={isVoiceDisabled ? VOICE_DISABLED : editingProfile.voice?.voice_id || ''}
          rate={editingProfile.voice?.rate ?? 1.0}
          volume={editingProfile.voice?.volume ?? 1.0}
          onChange={(field, value) => {
            if (field === 'voice') {
              if (value === VOICE_DISABLED) {
                updateField('voice.provider', 'disabled');
                updateField('voice.voice_id', '');
                return;
              }
              updateField('voice.voice_id', value);
              if (!editingProfile.voice?.provider || editingProfile.voice.provider === 'disabled') {
                updateField('voice.provider', 'webspeech');
              }
              return;
            }
            updateField(`voice.${field}`, value);
          }}
        />
        <div className="profiles-fields">
          <div className="profiles-field profiles-field--checkbox">
            <input
              id="pf-tts-agent"
              type="checkbox"
              checked={editingProfile.voice?.enabled_for_agent ?? false}
              onChange={(e) => updateField('voice.enabled_for_agent', e.target.checked)}
            />
            <label htmlFor="pf-tts-agent" className="profiles-field__label">
              {t('profiles.fieldTTSAgent', 'TTS para mensagens do assistente')}
            </label>
          </div>
          <div className="profiles-field profiles-field--checkbox">
            <input
              id="pf-tts-user"
              type="checkbox"
              checked={editingProfile.voice?.enabled_for_user ?? false}
              onChange={(e) => updateField('voice.enabled_for_user', e.target.checked)}
            />
            <label htmlFor="pf-tts-user" className="profiles-field__label">
              {t('profiles.fieldTTSUser', 'TTS para mensagens do usuário')}
            </label>
          </div>
          <div className="profiles-field">
            <label htmlFor="pf-channel-response" className="profiles-field__label">
              {t('profiles.fieldChannelResponse', 'Resposta em canais externos')}
            </label>
            <select
              id="pf-channel-response"
              className="profiles-field__select"
              value={editingProfile.voice?.channel_response_mode || 'mirror'}
              onChange={(e) => updateField('voice.channel_response_mode', e.target.value)}
            >
              <option value="mirror">{t('profiles.channelResponse.mirror')}</option>
              <option value="always_text">{t('profiles.channelResponse.alwaysText')}</option>
              <option value="always_audio">{t('profiles.channelResponse.alwaysAudio')}</option>
            </select>
            <p className="profiles-field__hint">
              {t('profiles.channelResponseHint')}
            </p>
          </div>
        </div>
      </CollapsibleSection>

      {/* Interaction (STT) — colapsável */}
      <CollapsibleSection
        title={t('profiles.collapseInteraction', 'Interação (STT)')}
        isOpen={!isSTTDisabled}
        onToggle={() => {
          if (isSTTDisabled) {
            updateField('interaction.stt_provider', 'webspeech');
          } else {
            updateField('interaction.stt_provider', '');
          }
        }}
        badge={isSTTDisabled ? 'off' : 'on'}
      >
        <ProfileInteractionSection
          sttProvider={editingProfile.interaction?.stt_provider || 'webspeech'}
          sttLanguage={editingProfile.interaction?.language || 'pt-BR'}
          enableFeedbackSounds={editingProfile.interaction?.feedback_sounds ?? true}
          onChange={(field, value) => {
            if (field === 'sttProvider') {
              updateField('interaction.stt_provider', value);
              return;
            }
            if (field === 'sttLanguage') {
              updateField('interaction.language', value);
              return;
            }
            updateField('interaction.feedback_sounds', value);
          }}
        />
      </CollapsibleSection>
    </>
  );
}
