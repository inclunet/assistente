import { logger } from '../../utils/logger';
import { useState, useEffect, useRef, useCallback, type ReactNode } from 'react';
import { SoundOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { ComboboxItem } from './Combobox';
import { BasePicker } from './BasePicker';
import { Button } from '../ui/Button';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { ttsService } from '../../services/tts';
import { TTSVoice } from '../../services/tts/types';
import './VoicePicker.css';
import { EventsOn } from '@wailsjs/runtime/runtime';

// Valor especial para voz desativada (usa leitor de telas)
export const VOICE_DISABLED = '__disabled__';

// Valores especiais para referenciar outras vozes
export const VOICE_REF_ASSISTANT = '__ref_assistant__';
export const VOICE_REF_USER = '__ref_user__';
export const VOICE_REF_SYSTEM = '__ref_system__';

export interface VoicePickerProps {
  value: string;
  onChange: (voice: string) => void;
  providerId?: string;
  modelId?: string;    // Modelo TTS usado para listar vozes HTTP
  variant?: 'toolbar' | 'form';
  label?: string;
  helpText?: string;
  icon?: ReactNode;
  maxWidth?: string;
  allowDisabled?: boolean;
  references?: Array<{ id: string; label: string }>;
  onAnnounce?: (message: string) => void;
  disabled?: boolean;
}

export function VoicePicker(
  {
    value,
    onChange,
    providerId,
    modelId,
    variant = 'form',
    label,
    helpText,
    icon = <SoundOutlined aria-hidden="true" />,
    maxWidth,
    allowDisabled = true,
    references = [],
    onAnnounce,
    disabled,
  }: VoicePickerProps
) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const effectiveLabel = label ?? t('pickers.voice.label');
  const effectiveHelpText = helpText ?? t('pickers.voice.description');
  const [voices, setVoices] = useState<TTSVoice[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const error = loadFailed ? t('pickers.voice.loadError') : null;
  useEffect(() => {
    if (error) (onAnnounce ?? announce)(error);
  }, [error, onAnnounce, announce]);
  const [completeness, setCompleteness] = useState<'complete' | 'partial' | 'unknown'>('unknown');
  const generation = useRef(0);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const retryRef = useRef<HTMLButtonElement>(null);
  const loadVoices = useCallback(async () => {
    const request = ++generation.current;
    const special = !providerId || providerId === 'disabled' || providerId.startsWith('ref_');
    if (special) {
      setVoices([]);
      setLoading(false);
      setLoadFailed(false);
      return;
    }
    setLoading(true);
    try {
      const result = await ttsService.getVoiceSelectionForProvider(providerId, modelId || '');
      if (request !== generation.current) return;
      if (document.activeElement === retryRef.current) triggerRef.current?.focus();
      setLoadFailed(false);
      setVoices(result.voices);
      setCompleteness(result.completeness);
    } catch (err) {
      if (request !== generation.current) return;
      setLoadFailed(true);
      logger.error('[VoicePicker] Failed to load voices:', err);
    } finally {
      if (request === generation.current) setLoading(false);
    }
  }, [providerId, modelId]);

  useEffect(() => {
    setVoices([]);
    setCompleteness('unknown');
    void loadVoices();
    const unsubscribe = EventsOn('llm_catalog:changed', () => { void loadVoices(); });
    return () => { generation.current++; unsubscribe(); };
  }, [loadVoices]);


  const items: ComboboxItem[] = [
    ...references.map(reference => ({ value: reference.id, label: reference.label })),
    ...(allowDisabled ? [{ value: VOICE_DISABLED, label: t('pickers.voice.screenReader') }] : []),
    ...voices.map(voice => ({ value: voice.id, label: voice.name, sublabel: voice.language })),
  ];
  if (value && !items.some(item => item.value === value)) items.push({ value, label: value });
  const isHTTP = !!providerId && providerId !== 'webspeech' && providerId !== 'sapi5' && providerId !== 'disabled' && !providerId.startsWith('ref_');
  const catalogHelp = isHTTP ? t(`pickers.voice.${completeness}`) : '';

  return (
    <>
    <BasePicker
      triggerRef={triggerRef}
      variant={variant}
      items={items}
      selected={value}
      onSelect={onChange}
      label={effectiveLabel}
      icon={icon}
      maxWidth={maxWidth}
      onAnnounce={onAnnounce}
      busy={loading}
      disabled={disabled}
      allowFreeInput={isHTTP && completeness !== 'complete'}
      showEmptyState={false}
      showErrorState={false}
      error={error}
      showFormLabel={variant === 'form'}
      formClassName="voice-picker-form"
      formLabelClassName="voice-picker-label"
      formLabelIconClassName="voice-picker-icon"
      helpText={variant === 'form' ? [effectiveHelpText, catalogHelp].filter(Boolean).join(' ') : undefined}
      helpTextClassName="help-text"

    />
    {error && <div>
      <p>{error}</p>
      <Button ref={retryRef} variant="secondary" disabled={disabled} aria-disabled={loading} onClick={() => { if (!loading) void loadVoices(); }}>
        {t('pickers.base.retry')}
      </Button>
    </div>}
    </>
  );
}
