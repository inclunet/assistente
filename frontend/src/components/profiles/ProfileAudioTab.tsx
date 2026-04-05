import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { profiles, llm } from '@wailsjs/go/models';
import { GetSpeechProviders } from '@wailsjs/go/main/App';
import { CollapsibleSection } from '../ui/CollapsibleSection';
import { ProfileVoiceSection } from './ProfileVoiceSection';
import { ProfileInteractionSection } from './ProfileInteractionSection';
import { VOICE_REF_ASSISTANT, VOICE_REF_USER, VOICE_REF_SYSTEM } from '../pickers/VoicePicker';
import { VoiceProviderPicker, type VoiceProviderItem } from '../pickers/VoiceProviderPicker';

export interface ProfileAudioTabProps {
  editingProfile: profiles.Profile;
  updateField: (path: string, value: unknown) => void;
  updateFields: (updates: Record<string, unknown>) => void;
  profileId: string;
}

type WailsApp = {
  go?: {
    main?: {
      App?: {
        GetPlatform?: () => Promise<string>;
      };
    };
  };
};

const fetchPlatform = async (): Promise<string | null> => {
  const app = (window as unknown as WailsApp).go?.main?.App;
  if (!app?.GetPlatform) return null;
  return app.GetPlatform();
};

export function ProfileAudioTab({ editingProfile, updateField, updateFields, profileId }: ProfileAudioTabProps) {
  const { t } = useTranslation();
  const [speechProviders, setSpeechProviders] = useState<llm.ProviderConfig[]>([]);
  const [isWindows, setIsWindows] = useState(false);
  const [voiceExpanded, setVoiceExpanded] = useState(false);

  useEffect(() => {
    GetSpeechProviders().then(setSpeechProviders).catch(console.error);
    fetchPlatform()
      .then((platform) => setIsWindows(platform === 'windows'))
      .catch(() => setIsWindows(false));
  }, []);

  const voice = editingProfile.voice;
  const assistantVoice = voice?.assistant;
  const userVoice = voice?.user;
  const systemVoice = voice?.system;

  const isVoiceDisabled = !assistantVoice?.enabled;
  const isSTTDisabled = !editingProfile.input?.stt_provider;

  const llmProviderItems: VoiceProviderItem[] = speechProviders.map((p) => {
    // Mostra o host da base_url para distinguir providers (ex: "api.openai.com", "litellm.local:4000")
    let hostHint = '';
    try {
      hostHint = new URL(p.base_url).host;
    } catch { /* URL inválida */ }
    return {
      id: p.id,
      label: p.name,
      description: hostHint
        ? t('pickers.voiceProvider.llmProviderWithHost', { host: hostHint })
        : t('pickers.voiceProvider.llmProvider'),
    };
  });

  const baseProviderItems: VoiceProviderItem[] = [
    {
      id: 'webspeech',
      label: t('pickers.voiceProvider.webspeech'),
      description: t('pickers.voiceProvider.webspeechDesc'),
    },
    ...(isWindows
      ? [
          {
            id: 'sapi5',
            label: t('pickers.voiceProvider.sapi5'),
            description: t('pickers.voiceProvider.sapi5Desc'),
          },
        ]
      : []),
    ...llmProviderItems,
  ];

  const defaultProvider: VoiceProviderItem = {
    id: '',
    label: t('pickers.voiceProvider.default'),
    description: t('pickers.voiceProvider.defaultDesc'),
  };

  const followAssistantProvider: VoiceProviderItem = {
    id: 'ref_assistant',
    label: t('pickers.voiceProvider.followAssistant'),
    description: t('pickers.voiceProvider.followAssistantDesc'),
  };

  const followUserProvider: VoiceProviderItem = {
    id: 'ref_user',
    label: t('pickers.voiceProvider.followUser'),
    description: t('pickers.voiceProvider.followUserDesc'),
  };

  const followSystemProvider: VoiceProviderItem = {
    id: 'ref_system',
    label: t('pickers.voiceProvider.followSystem'),
    description: t('pickers.voiceProvider.followSystemDesc'),
  };

  const handleProviderChange = (type: 'assistant' | 'user' | 'system', pId: string) => {
    const followVoiceMap: Record<string, string> = {
      ref_assistant: VOICE_REF_ASSISTANT,
      ref_user: VOICE_REF_USER,
      ref_system: VOICE_REF_SYSTEM,
    };

    const updates: Record<string, unknown> = {};

    if (type !== 'assistant' && pId.startsWith('ref_')) {
      updates[`voice.${type}.voice_id`] = followVoiceMap[pId];
    } else if (type !== 'assistant') {
      const currentVoice = type === 'user' ? userVoice?.voice_id : systemVoice?.voice_id;
      if (currentVoice && currentVoice.startsWith('__ref_')) {
        updates[`voice.${type}.voice_id`] = '';
      }
    }

    if (pId === 'webspeech' || pId === 'sapi5') {
      updates[`voice.${type}.provider`] = pId;
      updates[`voice.${type}.llm_provider_id`] = '';
      updates[`voice.${type}.enabled`] = true;
    } else if (!pId.startsWith('ref_')) {
      updates[`voice.${type}.provider`] = 'openai';
      updates[`voice.${type}.llm_provider_id`] = pId;
      updates[`voice.${type}.enabled`] = true;
    }

    updateFields(updates);
  };

  const currentAssistantProvider = assistantVoice?.llm_provider_id || assistantVoice?.provider || 'webspeech';
  const currentUserProvider = userVoice?.llm_provider_id || userVoice?.provider || '';
  const currentSystemProvider = systemVoice?.llm_provider_id || systemVoice?.provider || '';

  const isUserVoiceFollowing = userVoice?.voice_id === VOICE_REF_ASSISTANT || userVoice?.voice_id === VOICE_REF_SYSTEM;
  const isSystemVoiceFollowing = systemVoice?.voice_id === VOICE_REF_ASSISTANT || systemVoice?.voice_id === VOICE_REF_USER;

  const userFollowHelpText = userVoice?.voice_id === VOICE_REF_ASSISTANT
    ? t('profiles.voiceFollow.assistantHelp')
    : userVoice?.voice_id === VOICE_REF_SYSTEM
      ? t('profiles.voiceFollow.systemHelp')
      : undefined;

  const systemFollowHelpText = systemVoice?.voice_id === VOICE_REF_ASSISTANT
    ? t('profiles.voiceFollow.assistantHelp')
    : systemVoice?.voice_id === VOICE_REF_USER
      ? t('profiles.voiceFollow.userHelp')
      : undefined;

  const handleVoiceChange = (type: 'assistant' | 'user' | 'system', field: 'voice' | 'rate' | 'volume', value: string | number) => {
    if (field === 'voice') {
      updateField(`voice.${type}.voice_id`, value);
      return;
    }
    updateField(`voice.${type}.${field}`, value);
  };

  /**
   * Resolve referências de provedor (ex: ref_assistant)
   * Nunca retorna 'disabled' ou IDs de referência — esses são tratados como "sem provedor"
   */
  const resolveProviderId = (pId: string | undefined, type: 'assistant' | 'user' | 'system'): string => {
    if (!pId || pId === 'disabled') return '';
    if (pId.startsWith('ref_')) {
      if (pId === 'ref_assistant' && type !== 'assistant') return resolveProviderId(assistantVoice?.llm_provider_id || assistantVoice?.provider || 'webspeech', 'assistant');
      if (pId === 'ref_user' && type !== 'user') return resolveProviderId(userVoice?.llm_provider_id || userVoice?.provider, 'user');
      if (pId === 'ref_system' && type !== 'system') return resolveProviderId(systemVoice?.llm_provider_id || systemVoice?.provider, 'system');
      return '';
    }
    return pId;
  };

  /**
   * Resolve referências de voz (ex: VOICE_REF_ASSISTANT) para o ID real da voz.
   */
  const resolveVoiceId = (voiceId: string | undefined): string | undefined => {
    if (!voiceId) return undefined;
    if (voiceId === VOICE_REF_ASSISTANT) return assistantVoice?.voice_id;
    if (voiceId === VOICE_REF_USER) return userVoice?.voice_id;
    if (voiceId === VOICE_REF_SYSTEM) return systemVoice?.voice_id;
    return voiceId;
  };

  return (
    <>
      <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
        {/* Voice (TTS) — colapsável geral */}
        <CollapsibleSection
          title={t('profiles.collapseVoice', 'Voz (TTS)')}
          isOpen={!isVoiceDisabled || voiceExpanded}
          onToggle={() => {
            if (isVoiceDisabled) {
              setVoiceExpanded(true);
              updateFields({
                'voice.assistant.enabled': true,
                ...(!assistantVoice?.provider || assistantVoice.provider === 'disabled'
                  ? { 'voice.assistant.provider': 'webspeech' }
                  : {}),
              });
            } else {
              setVoiceExpanded(false);
              updateField('voice.assistant.enabled', false);
            }
          }}
          badge={isVoiceDisabled ? 'off' : 'on'}
        >
          <div style={{ display: 'flex', flexDirection: 'column', gap: '16px', padding: '0 8px' }}>
            {/* Assistant Voice */}
            <CollapsibleSection
              title={t('profiles.voiceLabels.assistant')}
              isOpen={assistantVoice?.enabled ?? false}
              onToggle={() => updateField('voice.assistant.enabled', !(assistantVoice?.enabled ?? false))}
              badge={(assistantVoice?.enabled ?? false) ? 'on' : 'off'}
            >
              <div className="profiles-field" style={{ marginBottom: '8px' }}>
                <label className="profiles-field__label">{t('profiles.fieldVoiceProvider', 'Provedor de Voz')}</label>
                <VoiceProviderPicker
                  value={currentAssistantProvider}
                  onChange={(value) => handleProviderChange('assistant', value)}
                  items={baseProviderItems}
                  label={t('pickers.voiceProvider.label')}
                  helpText={t('pickers.voiceProvider.description')}
                  variant="form"
                />
              </div>
              <ProfileVoiceSection
                voice={assistantVoice?.voice_id || ''}
                resolvedVoiceId={resolveVoiceId(assistantVoice?.voice_id)}
                rate={assistantVoice?.rate ?? 1.0}
                volume={assistantVoice?.volume ?? 1.0}
                providerId={resolveProviderId(currentAssistantProvider, 'assistant')}
                profileId={profileId}
                ttsModel={assistantVoice?.model}
                label={t('profiles.voiceLabels.assistantPicker')}
                onChange={(f, v) => handleVoiceChange('assistant', f, v)}
                disabled={isVoiceDisabled}
              />
            </CollapsibleSection>

            {/* User Voice */}
            <CollapsibleSection
              title={t('profiles.voiceLabels.user')}
              isOpen={userVoice?.enabled ?? false}
              onToggle={() => updateField('voice.user.enabled', !(userVoice?.enabled ?? false))}
              badge={(userVoice?.enabled ?? false) ? 'on' : 'off'}
            >
              <div className="profiles-field" style={{ marginBottom: '8px' }}>
                <label className="profiles-field__label">{t('profiles.fieldVoiceProvider', 'Provedor de Voz')}</label>
                <VoiceProviderPicker
                  value={currentUserProvider}
                  onChange={(value) => handleProviderChange('user', value)}
                  items={[defaultProvider, followAssistantProvider, followSystemProvider, ...baseProviderItems]}
                  label={t('pickers.voiceProvider.label')}
                  helpText={t('pickers.voiceProvider.description')}
                  variant="form"
                />
              </div>
              <ProfileVoiceSection
                voice={isUserVoiceFollowing ? (resolveVoiceId(userVoice?.voice_id) || '') : (userVoice?.voice_id || '')}
                resolvedVoiceId={resolveVoiceId(userVoice?.voice_id)}
                rate={userVoice?.rate ?? 1.0}
                volume={userVoice?.volume ?? 1.0}
                providerId={resolveProviderId(currentUserProvider, 'user')}
                profileId={profileId}
                ttsModel={userVoice?.model}
                label={t('profiles.voiceLabels.userPicker')}
                helpText={userFollowHelpText}
                onChange={(f, v) => handleVoiceChange('user', f, v)}
                disabled={isVoiceDisabled || isUserVoiceFollowing}
              />
            </CollapsibleSection>

            {/* System Voice */}
            <CollapsibleSection
              title={t('profiles.voiceLabels.system')}
              isOpen={systemVoice?.enabled ?? false}
              onToggle={() => updateField('voice.system.enabled', !(systemVoice?.enabled ?? false))}
              badge={(systemVoice?.enabled ?? false) ? 'on' : 'off'}
            >
              <div className="profiles-field" style={{ marginBottom: '8px' }}>
                <label className="profiles-field__label">{t('profiles.fieldVoiceProvider', 'Provedor de Voz')}</label>
                <VoiceProviderPicker
                  value={currentSystemProvider}
                  onChange={(value) => handleProviderChange('system', value)}
                  items={[defaultProvider, followAssistantProvider, followUserProvider, ...baseProviderItems]}
                  label={t('pickers.voiceProvider.label')}
                  helpText={t('pickers.voiceProvider.description')}
                  variant="form"
                />
              </div>
              <ProfileVoiceSection
                voice={isSystemVoiceFollowing ? (resolveVoiceId(systemVoice?.voice_id) || '') : (systemVoice?.voice_id || '')}
                resolvedVoiceId={resolveVoiceId(systemVoice?.voice_id)}
                rate={systemVoice?.rate ?? 1.0}
                volume={systemVoice?.volume ?? 1.0}
                providerId={resolveProviderId(currentSystemProvider, 'system')}
                profileId={profileId}
                ttsModel={systemVoice?.model}
                label={t('profiles.voiceLabels.systemPicker')}
                helpText={systemFollowHelpText}
                onChange={(f, v) => handleVoiceChange('system', f, v)}
                disabled={isVoiceDisabled || isSystemVoiceFollowing}
              />
            </CollapsibleSection>

            <div style={{ marginTop: '1rem', borderTop: '1px solid var(--border-subtle)', paddingTop: '1rem', display: 'flex', flexDirection: 'column', gap: '1rem' }}>
              {currentAssistantProvider !== 'webspeech' && currentAssistantProvider !== 'sapi5' && (
                <div className="profiles-field">
                  <label htmlFor="pf-tts-model" className="profiles-field__label">
                    {t('profiles.fieldTTSModel', 'Modelo TTS')}
                  </label>
                  <select
                    id="pf-tts-model"
                    className="profiles-field__select"
                    value={assistantVoice?.model || 'tts-1'}
                    onChange={(e) => updateField('voice.assistant.model', e.target.value)}
                  >
                    <option value="tts-1">{t('profiles.ttsModel.tts1', 'tts-1 (Rápido)')}</option>
                    <option value="tts-1-hd">{t('profiles.ttsModel.tts1hd', 'tts-1-hd (Alta Definição)')}</option>
                  </select>
                </div>
              )}


              <div className="profiles-field">
                <label htmlFor="pf-channel-response" className="profiles-field__label">
                  {t('profiles.fieldChannelResponse', 'Resposta em canais externos')}
                </label>
                <select
                  id="pf-channel-response"
                  className="profiles-field__select"
                  value={editingProfile.channels?.response_mode || 'mirror'}
                  onChange={(e) => updateField('channels.response_mode', e.target.value)}
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
          </div>
        </CollapsibleSection>

        {/* Input (STT) — colapsável */}
        <CollapsibleSection
          title={t('profiles.collapseInput', 'Entrada de Voz (STT)')}
          isOpen={!isSTTDisabled}
          onToggle={() => {
            if (isSTTDisabled) {
              updateFields({
                'input.stt_provider': 'webspeech',
                'input.enabled': true,
              });
            } else {
              updateFields({
                'input.stt_provider': '',
                'input.enabled': false,
              });
            }
          }}
          badge={isSTTDisabled ? 'off' : 'on'}
        >
          <ProfileInteractionSection
            sttProvider={editingProfile.input?.stt_provider || 'webspeech'}
            sttLLMProviderId={editingProfile.input?.llm_provider_id || ''}
            sttModel={editingProfile.input?.stt_model || ''}
            sttLanguage={editingProfile.input?.language || 'pt-BR'}
            enableFeedbackSounds={editingProfile.input?.feedback_sounds ?? true}
            speechProviders={speechProviders}
            onChange={(field, value) => {
              if (field === 'sttProvider') {
                updateField('input.stt_provider', value);
                return;
              }
              if (field === 'sttLLMProviderId') {
                updateField('input.llm_provider_id', value);
                return;
              }
              if (field === 'sttModel') {
                updateField('input.stt_model', value);
                return;
              }
              if (field === 'sttLanguage') {
                updateField('input.language', value);
                return;
              }
              updateField('input.feedback_sounds', value);
            }}
          />
        </CollapsibleSection>
      </div>
    </>
  );
}
