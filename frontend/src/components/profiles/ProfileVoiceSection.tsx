import { VoicePicker, VOICE_DISABLED } from '../pickers/VoicePicker';
import { RangeSlider } from '../ui/RangeSlider';
import './ProfileVoiceSection.css';

export interface ProfileVoiceSectionProps {
  voice: string;
  rate: number;
  volume: number;
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
  onChange,
  disabled = false,
}: ProfileVoiceSectionProps) {
  return (
    <div className="profile-voice-section" data-testid="profile-voice-section">
      {/* Voice picker */}
      <div className="profile-voice-section__field">
        <VoicePicker
          value={voice || VOICE_DISABLED}
          onChange={(value) => onChange('voice', value)}
          variant="form"
          label="Voz (TTS)"
          helpText="Selecione a voz para síntese de fala"
          icon="🔊"
          allowDisabled={true}
        />
      </div>

      {/* Rate slider */}
      <div className="profile-voice-section__field">
        <RangeSlider
          id="voice-rate"
          label="Taxa de Fala (Rate)"
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
          id="voice-volume"
          label="Volume"
          value={volume}
          min={0.0}
          max={1.0}
          step={0.05}
          onChange={(value) => onChange('volume', value)}
          formatValue={(val) => `${Math.round(val * 100)}%`}
          disabled={disabled}
        />
      </div>
    </div>
  );
}
