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
  return (
    <div className="profile-interaction-section" data-testid="profile-interaction-section">
      {/* STT Provider picker */}
      <div className="profile-interaction-section__field">
        <STTProviderPicker
          value={sttProvider || 'webspeech'}
          onChange={(value) => onChange('sttProvider', value)}
          variant="form"
          label="Reconhecimento de Fala (STT)"
          helpText="Selecione o provedor de reconhecimento de fala"
          icon="🎤"
        />
      </div>

      {/* STT Language */}
      <div className="profile-interaction-section__field">
        <label htmlFor="stt-language" className="profile-interaction-section__label">
          Idioma (STT)
        </label>
        <select
          id="stt-language"
          className="profile-interaction-section__select"
          value={sttLanguage || 'pt-BR'}
          onChange={(e) => onChange('sttLanguage', e.target.value)}
          disabled={disabled}
          data-testid="stt-language-select"
        >
          <option value="pt-BR">Português (Brasil)</option>
          <option value="en-US">English (US)</option>
          <option value="es-ES">Español</option>
          <option value="fr-FR">Français</option>
          <option value="de-DE">Deutsch</option>
          <option value="it-IT">Italiano</option>
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
          <span>Ativar sons de feedback</span>
        </label>
        <span className="profile-interaction-section__hint">
          Sons ao iniciar/parar gravação e outros eventos de interação
        </span>
      </div>
    </div>
  );
}
