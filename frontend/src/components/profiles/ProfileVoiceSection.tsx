import { useCallback, useEffect, useId, useState } from 'react';
import { SoundOutlined, PlayCircleOutlined, StopOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { VoicePicker, VOICE_DISABLED } from '../pickers/VoicePicker';
import { RangeSlider } from '../ui/RangeSlider';
import { Button } from '../ui/Button';
import { useTTS } from '../../hooks/useTTS';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { BasePicker } from '../pickers/BasePicker';
import { ttsService } from '../../services/tts';
import type { TTSModel, TTSSelectionMode } from '../../services/tts/types';
import './ProfileVoiceSection.css';

const toFieldIdPart = (value?: string) => (value || 'default')
  .trim()
  .toLowerCase()
  .replace(/[^a-z0-9_-]+/g, '-')
  .replace(/^-+|-+$/g, '') || 'default';

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
  selectionMode?: TTSSelectionMode;
  previewLanguage?: string;
  onChange: (field: 'voice' | 'model' | 'selectionMode' | 'rate' | 'volume', value: string | number) => void;
  onChangeMany?: (updates: Partial<Record<'voice' | 'model' | 'selectionMode' | 'rate' | 'volume', string | number>>) => void;
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
  previewLanguage,
  label,
  helpText,
  references,
  resolvedVoiceId,
  selectionMode,
  onChange,
  onChangeMany,
  disabled = false,
}: ProfileVoiceSectionProps) {
  const { t } = useTranslation();
  const { speakWithOverride, stop, isSpeaking } = useTTS();
  const [dynamicModels, setDynamicModels] = useState<TTSModel[]>([]);


  const isHTTPProvider = !!providerId && providerId !== 'webspeech' && providerId !== 'sapi5' && !providerId.startsWith('ref_');
  const generatedModelId = useId();
  const modelFieldIdPart = `${toFieldIdPart(profileId)}-${toFieldIdPart(label)}-${toFieldIdPart(generatedModelId)}`;
  const modelSelectId = `tts-model-${modelFieldIdPart}`;


  useEffect(() => {
    let cancelled = false;
    let generation = 0;
    if (!isHTTPProvider) {
      setDynamicModels([]);
      return;
    }
    if (!providerId) return;
    const load = async () => {
      const request = ++generation;
      const models = await ttsService.getModelsForProvider(providerId);
      if (!cancelled && request === generation) setDynamicModels(models);
    };
    setDynamicModels([]);
    void load();
    const unsubscribe = EventsOn('llm_catalog:changed', () => { void load(); });
    return () => { cancelled = true; unsubscribe(); };
  }, [isHTTPProvider, providerId]);

  const ttsModels = dynamicModels;
  const currentModel = ttsModels.find((m) => m.id === ttsModel);
  const effectiveSelectionMode = selectionMode;
  const isModelOnly = isHTTPProvider && effectiveSelectionMode === 'model_only';

  const applyChanges = useCallback((updates: Partial<Record<'voice' | 'model' | 'selectionMode' | 'rate' | 'volume', string | number>>) => {
    if (onChangeMany) {
      onChangeMany(updates);
      return;
    }
    Object.entries(updates).forEach(([field, value]) => {
      onChange(field as 'voice' | 'model' | 'selectionMode' | 'rate' | 'volume', value);
    });
  }, [onChange, onChangeMany]);

  const handleModelChange = (modelId: string) => {
    if (!modelId) {
      applyChanges({ model: '', selectionMode: '', voice: '' });
      return;
    }
    const nextModel = ttsModels.find((m) => m.id === modelId);
    const nextSelectionMode = nextModel?.selectionMode || '';
    const updates: Partial<Record<'model' | 'selectionMode' | 'voice', string>> = {
      model: modelId,
      selectionMode: nextSelectionMode,
    };
    if (nextSelectionMode === 'model_only') {
      updates.voice = '';
    }
    applyChanges(updates);
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
      language: previewLanguage,
    });
  };

  const modeConflict = !!currentModel && selectionMode !== currentModel.selectionMode;
  const isPreviewDisabled = disabled || modeConflict || (isHTTPProvider && !effectiveSelectionMode) || (isHTTPProvider && !ttsModel) || (!isModelOnly && (!voice || voice === VOICE_DISABLED));

  return (
    <div className="profile-voice-section" data-testid="profile-voice-section">
      {isHTTPProvider && (
        <div className="profile-voice-section__field">
          <BasePicker
            label={t('profiles.voiceSection.modelLabel')}
            variant="form"
            selected={ttsModel || ''}
            items={[{ value: '', label: t('profiles.voiceSection.modelPlaceholder') }, ...ttsModels.map((model) => ({ value: model.id, label: model.name }))]}
            onSelect={handleModelChange}
            allowFreeInput
            disabled={disabled}
            showEmptyState={false}
            helpText={t('profiles.voiceSection.modelHelp')}
          />

        </div>
      )}

      {isHTTPProvider && ttsModel && (
        <div className="profile-voice-section__field">
          <label htmlFor={modelSelectId}>{t('profiles.voiceSection.selectionModeLabel')}</label>
          <select id={modelSelectId} aria-describedby={modeConflict ? `${modelSelectId}-conflict` : undefined} className="profiles-field__select" value={selectionMode || ''} disabled={disabled}
            onChange={(event) => applyChanges({ selectionMode: event.target.value, ...(event.target.value === 'model_only' ? { voice: '' } : {}) })}>
            <option value="">{t('profiles.voiceSection.selectionModePlaceholder')}</option>
            <option value="model_and_voice" disabled={!!currentModel && currentModel.selectionMode !== 'model_and_voice'}>{t('profiles.voiceSection.modelAndVoice')}</option>
            <option value="model_only" disabled={!!currentModel && currentModel.selectionMode !== 'model_only'}>{t('profiles.voiceSection.modelOnly')}</option>
          </select>
          {modeConflict && <p id={`${modelSelectId}-conflict`} className="profiles-field__hint">{t('profiles.voiceSection.modeConflict')}</p>}
        </div>
      )}

      {!isModelOnly && (!isHTTPProvider || !!ttsModel) && (
        <div className="profile-voice-section__field">
          <VoicePicker
            value={voice || ''}
            onChange={(value) => onChange('voice', value)}
            providerId={providerId}
            modelId={ttsModel}
            variant="form"
            label={label ?? t('profiles.voiceSection.voiceLabel')}
            helpText={helpText ?? t('profiles.voiceSection.voiceHelp')}
            icon={<SoundOutlined aria-hidden="true" />}
            allowDisabled={false}
            disabled={disabled}
            references={references ?? []}
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
          disabled={!isSpeaking && isPreviewDisabled}
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
