import { useMemo } from 'react';
import { SoundOutlined, PlayCircleOutlined, StopOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { VoicePicker, VOICE_DISABLED } from '../pickers/VoicePicker';
import { RangeSlider } from '../ui/RangeSlider';
import { Button } from '../ui/Button';
import { useTTS } from '../../hooks/useTTS';
import { getTTSCapabilities, makeCompositeVoiceId } from '../../config/providers';
import { TTSVoice } from '../../services/tts/types';
import './ProfileVoiceSection.css';

export interface ProfileVoiceSectionProps {
  voice: string;
  rate: number;
  volume: number;
  providerId?: string;
  providerType?: string;
  profileId?: string;
  label?: string;
  helpText?: string;
  references?: Array<{ id: string; label: string }>;
  resolvedVoiceId?: string;
  ttsModel?: string;
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
  providerType,
  profileId,
  ttsModel,
  label,
  helpText,
  references,
  resolvedVoiceId,
  onChange,
  disabled = false,
}: ProfileVoiceSectionProps) {
  const { t } = useTranslation();
  const { speakWithOverride, stop, isSpeaking } = useTTS();

  const ttsCapabilities = getTTSCapabilities(providerType || '');

  // Para provedores com vozes estáticas (OpenAI): converte para TTSVoice[] e passa como override
  const overrideVoices: TTSVoice[] | undefined = useMemo(() => {
    if (ttsCapabilities.staticVoices.length === 0) return undefined;
    return ttsCapabilities.staticVoices.map(sv => ({
      id: sv.id,
      name: sv.name,
      language: sv.language,
      provider: sv.provider,
      gender: 'neutral' as const,
      premium: sv.model.includes('hd'),
      localService: false,
      description: sv.name,
    }));
  }, [ttsCapabilities]);

  // Valor composto para o picker: "voiceId::model" (ex: "nova::tts-1-hd")
  const effectiveVoiceValue = useMemo(() => {
    if (ttsCapabilities.staticVoices.length > 0 && voice) {
      return makeCompositeVoiceId(voice, ttsModel || 'tts-1');
    }
    return voice;
  }, [voice, ttsModel, ttsCapabilities]);

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
          value={effectiveVoiceValue || ''}
          onChange={(value) => onChange('voice', value)}
          providerId={providerId}
          profileId={profileId}
          variant="form"
          label={label ?? t('profiles.voiceSection.voiceLabel')}
          helpText={helpText ?? t('profiles.voiceSection.voiceHelp')}
          icon={<SoundOutlined />}
          allowDisabled={false}
          references={references ?? []}
          voiceOverrides={overrideVoices}
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
