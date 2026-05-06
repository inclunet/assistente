import { useEffect, useMemo, useState } from 'react';
import { RobotOutlined, SoundOutlined, PlayCircleOutlined, StopOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { VoicePicker, VOICE_DISABLED } from '../pickers/VoicePicker';
import { RangeSlider } from '../ui/RangeSlider';
import { Button } from '../ui/Button';
import { useTTS } from '../../hooks/useTTS';
import { getTTSCapabilities } from '../../config/providers';
import { ttsService } from '../../services/tts';
import type { TTSModel, TTSVoice, TTSSelectionMode } from '../../services/tts/types';
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
  selectionMode?: TTSSelectionMode;
  onChange: (field: 'voice' | 'model' | 'selectionMode' | 'rate' | 'volume', value: string | number) => void;
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
  selectionMode,
  onChange,
  disabled = false,
}: ProfileVoiceSectionProps) {
  const { t } = useTranslation();
  const { speakWithOverride, stop, isSpeaking } = useTTS();
  const [dynamicModels, setDynamicModels] = useState<TTSModel[]>([]);

  const ttsCapabilities = getTTSCapabilities(providerType || '');
  const isHTTPProvider = !!providerId && providerId !== 'webspeech' && providerId !== 'sapi5' && !providerId.startsWith('ref_');

  useEffect(() => {
    let cancelled = false;
    if (!isHTTPProvider || ttsCapabilities.staticModels.length > 0) {
      setDynamicModels([]);
      return;
    }
    ttsService.getModelsForProvider(providerId)
      .then((models) => {
        if (!cancelled) setDynamicModels(models);
      })
      .catch(() => {
        if (!cancelled) setDynamicModels([]);
      });
    return () => { cancelled = true; };
  }, [isHTTPProvider, providerId, ttsCapabilities.staticModels.length]);

  const ttsModels = useMemo<TTSModel[]>(() => {
    if (ttsCapabilities.staticModels.length > 0) {
      return ttsCapabilities.staticModels.map((m) => ({
        id: m.id,
        name: m.name,
        provider: m.provider,
        selectionMode: m.selectionMode,
        description: m.description,
      }));
    }
    return dynamicModels;
  }, [dynamicModels, ttsCapabilities.staticModels]);

  const currentModel = ttsModels.find((m) => m.id === ttsModel);
  const effectiveSelectionMode = currentModel?.selectionMode || selectionMode || 'model_and_voice';
  const isModelOnly = isHTTPProvider && effectiveSelectionMode === 'model_only';

  // Para provedores com vozes estáticas (OpenAI): converte para TTSVoice[] e passa como override.
  const overrideVoices: TTSVoice[] | undefined = useMemo(() => {
    if (ttsCapabilities.staticVoices.length === 0 || isModelOnly) return undefined;
    return ttsCapabilities.staticVoices.map(sv => ({
      id: sv.id,
      name: sv.name,
      language: sv.language,
      provider: sv.provider,
      gender: 'neutral' as const,
      modelId: ttsModel,
      premium: ttsModel?.includes('hd'),
      localService: false,
      description: sv.name,
    }));
  }, [isModelOnly, ttsModel, ttsCapabilities.staticVoices]);

  const handleModelChange = (modelId: string) => {
    const nextModel = ttsModels.find((m) => m.id === modelId);
    const nextSelectionMode = nextModel?.selectionMode || 'model_and_voice';
    onChange('model', modelId);
    onChange('selectionMode', nextSelectionMode);
    if (nextSelectionMode === 'model_only') {
      onChange('voice', '');
    }
  };

  const handlePreview = async () => {
    if (isSpeaking) {
      stop();
      return;
    }

    const testVoice = resolvedVoiceId || voice;
    if (!ttsModel && isHTTPProvider) return;
    if (!isModelOnly && (!testVoice || testVoice === VOICE_DISABLED)) return;

    await speakWithOverride(t('profiles.voicePreview.sampleText'), {
      voiceName: isModelOnly ? '' : testVoice,
      providerId,
      rate,
      volume,
      ttsModel,
    });
  };

  const isPreviewDisabled = disabled || (isHTTPProvider && !ttsModel) || (!isModelOnly && (!voice || voice === VOICE_DISABLED));

  return (
    <div className="profile-voice-section" data-testid="profile-voice-section">
      {isHTTPProvider && (
        <div className="profile-voice-section__field">
          <label className="profiles-field__label">
            <RobotOutlined /> {t('profiles.voiceSection.modelLabel', 'Modelo TTS')}
          </label>
          <select
            className="profiles-field__select"
            value={ttsModel || ''}
            onChange={(event) => handleModelChange(event.target.value)}
            disabled={disabled}
          >
            <option value="">{t('profiles.voiceSection.modelPlaceholder', 'Selecione um modelo')}</option>
            {ttsModels.map((model) => (
              <option key={model.id} value={model.id}>
                {model.name}
              </option>
            ))}
          </select>
          <p className="profiles-field__hint">
            {t('profiles.voiceSection.modelHelp', 'Escolha o modelo TTS antes da voz. Modelos Piper usam apenas o modelo.')}
          </p>
        </div>
      )}

      {!isModelOnly && (
        <div className="profile-voice-section__field">
          <VoicePicker
            value={voice || ''}
            onChange={(value) => onChange('voice', value)}
            providerId={providerId}
            modelId={ttsModel}
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
      )}

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
