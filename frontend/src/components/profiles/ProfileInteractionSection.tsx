import { AudioOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { llm } from '@wailsjs/go/models';
import { STTProviderPicker, STT_WEBSPEECH } from '../pickers/STTProviderPicker';
import './ProfileInteractionSection.css';

export interface ProfileInteractionSectionProps {
  sttProvider: string;
  sttLLMProviderId: string;
  sttModel: string;
  sttLanguage: string;
  enableFeedbackSounds: boolean;
  onChange: (field: 'sttProvider' | 'sttLLMProviderId' | 'sttModel' | 'sttLanguage' | 'enableFeedbackSounds', value: string | boolean) => void;
  /** Provedores com suporte a speech (TTS/STT). Evita fetch duplicado no STTProviderPicker. */
  speechProviders?: llm.ProviderConfig[];
  /** Modelos STT disponíveis para o provider selecionado (dinâmico via backend). */
  sttModels?: Array<{ id: string; name: string }>;
  disabled?: boolean;
}

/**
 * Seção de configuração de interação de um perfil.
 * Permite escolher provedor STT, modelo, idioma e ativar sons de feedback.
 */
export function ProfileInteractionSection({
  sttProvider,
  sttLLMProviderId,
  sttModel,
  sttLanguage,
  enableFeedbackSounds,
  onChange,
  speechProviders,
  sttModels,
  disabled = false,
}: ProfileInteractionSectionProps) {
  const { t } = useTranslation();

  const isWhisper = sttProvider === 'whisper_api';

  return (
    <div className="profile-interaction-section" data-testid="profile-interaction-section">
      {/* STT Provider picker */}
      <div className="profile-interaction-section__field">
        <STTProviderPicker
          value={isWhisper ? sttLLMProviderId : STT_WEBSPEECH}
          providers={speechProviders}
          onChange={(provider, llmProviderId) => {
            if (provider === STT_WEBSPEECH) {
              onChange('sttProvider', 'webspeech');
              onChange('sttLLMProviderId', '');
            } else {
              onChange('sttProvider', 'whisper_api');
              onChange('sttLLMProviderId', llmProviderId || '');
            }
          }}
          variant="form"
          label={t('profiles.interactionSection.sttTitle')}
          helpText={t('profiles.interactionSection.sttDescription')}
          icon={<AudioOutlined />}
        />
      </div>

      {/* STT Model (só para Whisper) */}
      {isWhisper && (
        <div className="profile-interaction-section__field">
          <label htmlFor="stt-model" className="profile-interaction-section__label">
            {t('profiles.interactionSection.sttModelLabel')}
          </label>
          <select
            id="stt-model"
            className="profile-interaction-section__select"
            value={sttModel || 'whisper-1'}
            onChange={(e) => onChange('sttModel', e.target.value)}
            disabled={disabled}
            data-testid="stt-model-select"
          >
            {(sttModels && sttModels.length > 0 ? sttModels : [
              { id: 'whisper-1', name: t('profiles.interactionSection.sttModelWhisper1') },
              { id: 'gpt-4o-transcribe', name: t('profiles.interactionSection.sttModelGpt4oTranscribe') },
              { id: 'gpt-4o-mini-transcribe', name: t('profiles.interactionSection.sttModelGpt4oMiniTranscribe') },
            ]).map((m) => (
              <option key={m.id} value={m.id}>{m.name}</option>
            ))}
          </select>
          <span className="profile-interaction-section__hint">
            {t('profiles.interactionSection.sttModelHint')}
          </span>
        </div>
      )}

      {/* STT Language */}
      <div className="profile-interaction-section__field">
        <label htmlFor="stt-language" className="profile-interaction-section__label">
          {t('profiles.interactionSection.sttLanguage')}
        </label>
        <select
          id="stt-language"
          className="profile-interaction-section__select"
          value={sttLanguage || 'pt-BR'}
          onChange={(e) => onChange('sttLanguage', e.target.value)}
          disabled={disabled}
          data-testid="stt-language-select"
        >
          <option value="pt-BR">{t('profiles.interactionSection.ptBR')}</option>
          <option value="en-US">{t('profiles.interactionSection.enUS')}</option>
          <option value="es-ES">{t('profiles.interactionSection.es')}</option>
          <option value="fr-FR">{t('profiles.interactionSection.fr')}</option>
          <option value="de-DE">{t('profiles.interactionSection.de')}</option>
          <option value="it-IT">{t('profiles.interactionSection.it')}</option>
        </select>
      </div>

      {/* Feedback Sounds */}
      <div className="profile-interaction-section__field profile-interaction-section__field--checkbox">
        <label className="profile-interaction-section__checkbox-label">
          <input
            type="checkbox"
            checked={enableFeedbackSounds}
            onChange={(e) => onChange('enableFeedbackSounds', e.target.checked)}
            disabled={disabled}
            data-testid="feedback-sounds-checkbox"
          />
          <span>{t('profiles.interactionSection.feedbackSounds')}</span>
        </label>
        <span className="profile-interaction-section__hint">
          {t('profiles.interactionSection.feedbackSoundsHint')}
        </span>
      </div>
    </div>
  );
}
