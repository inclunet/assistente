import { useTranslation } from 'react-i18next';
import { STTProviderPicker } from '../pickers/STTProviderPicker';
import './ProfileInteractionSection.css';

export interface ProfileInteractionSectionProps {
  sttProvider: string;
  sttLanguage: string;
  enableFeedbackSounds: boolean;
  onChange: (field: 'sttProvider' | 'sttLanguage' | 'enableFeedbackSounds', value: string | boolean) => void;
  disabled?: boolean;
}

/**
 * Seção de configuração de interação de um perfil.
 * Permite escolher provedor STT, idioma e ativar sons de feedback.
 */
export function ProfileInteractionSection({
  sttProvider,
  sttLanguage,
  enableFeedbackSounds,
  onChange,
  disabled = false,
}: ProfileInteractionSectionProps) {
  const { t } = useTranslation();
  return (
    <div className="profile-interaction-section" data-testid="profile-interaction-section">
      {/* STT Provider picker */}
      <div className="profile-interaction-section__field">
        <STTProviderPicker
          value={sttProvider || 'webspeech'}
          onChange={(value) => onChange('sttProvider', value)}
          variant="form"
          label={t('profiles.interactionSection.sttTitle')}
          helpText={t('profiles.interactionSection.sttDescription')}
          icon="🎤"
        />
      </div>

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
