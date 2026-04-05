import { SoundOutlined, PlayCircleOutlined, StopOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { VoicePicker, VOICE_DISABLED } from '../pickers/VoicePicker';
import { RangeSlider } from '../ui/RangeSlider';
import { Button } from '../ui/Button';
import { useTTS } from '../../hooks/useTTS';
import './ProfileVoiceSection.css';

export interface ProfileVoiceSectionProps {
  voice: string;
  rate: number;
  volume: number;
  providerId?: string;
  profileId?: string;
  label?: string;
  helpText?: string;
  references?: Array<{ id: string; label: string }>;
  resolvedVoiceId?: string;
  ttsModel?: string;
  ttsModels?: Array<{ id: string; name: string }>; // Modelos disponíveis (dinâmico via backend)
  onModelChange?: (model: string) => void;
  onChange: (field: 'voice' | 'rate' | 'volume', value: string | number) => void;
  disabled?: boolean;
}

/**
 * Seção de configuração de voz (TTS) de um perfil.
 * Permite escolher voz, taxa de fala e volume.
 */
export function ProfileVoiceSection({
  voice,
  rate,
  volume,
  providerId,
  profileId,
  ttsModel,
  ttsModels,
  onModelChange,
  label,
  helpText,
  references,
  resolvedVoiceId,
  onChange,
  disabled = false,
}: ProfileVoiceSectionProps) {
  const { t } = useTranslation();
  const { speakWithOverride, stop, isSpeaking } = useTTS();

  const isOpenAILike = providerId && providerId !== 'webspeech' && providerId !== 'sapi5' && !providerId.startsWith('ref_');

  const handlePreview = async () => {
    if (isSpeaking) {
      stop();
      return;
    }

    const testVoice = resolvedVoiceId || voice;
    if (!testVoice || testVoice === VOICE_DISABLED) return;

    await speakWithOverride(t('profiles.voicePreview.sampleText'), {
      voiceName: testVoice,
      providerId,
      rate,
      volume,
      ttsModel,
    });
  };

  const isPreviewDisabled = disabled || !voice || voice === VOICE_DISABLED;

  return (
    <div className="profile-voice-section" data-testid="profile-voice-section">
      {/* Voice picker */}
      <div className="profile-voice-section__field">
        <VoicePicker
          value={voice || ''}
          onChange={(value) => onChange('voice', value)}
          providerId={providerId}
          profileId={profileId}
          variant="form"
          label={label ?? t('profiles.voiceSection.voiceLabel')}
          helpText={helpText ?? t('profiles.voiceSection.voiceHelp')}
          icon={<SoundOutlined />}
          allowDisabled={false}
          references={references ?? []}
        />
      </div>

      {/* Rate slider */}
      <div className="profile-voice-section__field">
        <RangeSlider
          id={`voice-rate-${label?.replace(/\s+/g, '-').toLowerCase() || 'default'}`}
          label={t('profiles.voiceSection.rateLabel')}
          value={rate}
          min={0.5}
          max={2.0}
          step={0.1}
          onChange={(value) => onChange('rate', value)}
          formatValue={(val) => `${val.toFixed(1)}x`}
          disabled={disabled}
        />
      </div>

      {/* Volume slider */}
      <div className="profile-voice-section__field">
        <RangeSlider
          id={`voice-volume-${label?.replace(/\s+/g, '-').toLowerCase() || 'default'}`}
          label={t('profiles.voiceSection.volumeLabel')}
          value={volume}
          min={0.0}
          max={1.0}
          step={0.05}
          onChange={(value) => onChange('volume', value)}
          formatValue={(val) => `${Math.round(val * 100)}%`}
          disabled={disabled}
        />
      </div>

      {/* TTS Model selector (only for OpenAI-like providers) */}
      {isOpenAILike && onModelChange && (
        <div className="profile-voice-section__field">
          <label htmlFor={`tts-model-${label?.replace(/\s+/g, '-').toLowerCase() || 'default'}`} className="profile-voice-section__label">
            {t('profiles.fieldTTSModel', 'Modelo TTS')}
          </label>
          <select
            id={`tts-model-${label?.replace(/\s+/g, '-').toLowerCase() || 'default'}`}
            className="profiles-field__select"
            value={ttsModel || 'tts-1'}
            onChange={(e) => onModelChange(e.target.value)}
            disabled={disabled}
          >
            {(ttsModels && ttsModels.length > 0 ? ttsModels : [
              { id: 'tts-1', name: t('profiles.ttsModel.tts1', 'tts-1 (Rápido)') },
              { id: 'tts-1-hd', name: t('profiles.ttsModel.tts1hd', 'tts-1-hd (Alta Definição)') },
            ]).map((m) => (
              <option key={m.id} value={m.id}>{m.name}</option>
            ))}
          </select>
        </div>
      )}

      <div className="profile-voice-section__actions">
        <Button
          variant="secondary"
          size="sm"
          onClick={handlePreview}
          disabled={isPreviewDisabled}
          aria-label={isSpeaking ? t('profiles.voicePreview.stop') : t('profiles.voicePreview.buttonAria')}
        >
          <span aria-hidden="true">
            {isSpeaking ? <StopOutlined /> : <PlayCircleOutlined />}
          </span>
          {' '}
          {isSpeaking ? t('profiles.voicePreview.stop') : t('profiles.voicePreview.button')}
        </Button>
      </div>
    </div>
  );
}
