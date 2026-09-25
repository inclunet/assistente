import { Button } from '../ui';
import type { CommandDeckCaptureEvent } from '../../types/commandSettingsTypes';

export interface StreamDeckCaptureFieldsProps {
  readonly value: string;
  readonly capture: CommandDeckCaptureEvent | null;
  readonly captureActive: boolean;
  readonly disabled: boolean;
  readonly t: (key: string, options?: Record<string, unknown>) => string;
  readonly onCapture: () => void;
  readonly onCancel: () => void;
}

export function StreamDeckCaptureFields({
  value,
  capture,
  captureActive,
  disabled,
  t,
  onCapture,
  onCancel,
}: StreamDeckCaptureFieldsProps) {
  const trigger = parseCapturedTrigger(value);
  const recording = captureActive;
  const capturedLabel =
    capture?.status === 'captured' && trigger
      ? t('commandSettings.form.captureResult', {
          model: capture.model || t('commandSettings.form.captureUnknownModel'),
          key: trigger.key + 1,
        })
      : null;

  return (
    <div className="command-settings__streamdeck-capture">
      <Button variant="secondary" disabled={disabled || recording} onClick={onCapture}>
        {t('commandSettings.form.captureButton')}
      </Button>
      {recording && (
        <Button variant="ghost" disabled={disabled} onClick={onCancel}>
          {t('commandSettings.form.captureCancel')}
        </Button>
      )}
      {recording && capture?.status === 'starting' && <p>{t('commandSettings.form.captureStarting')}</p>}
      {recording && capture?.status === 'waiting' && <p>{t('commandSettings.form.captureWaiting')}</p>}
      {capturedLabel && <p>{capturedLabel}</p>}
      {capture && capture.status !== 'starting' && capture.status !== 'waiting' && capture.status !== 'captured' && (
        <p>
          {t(`commandSettings.form.captureStatus.${capture.status}`)}
        </p>
      )}
      {!capture && !trigger && <p>{t('commandSettings.form.captureInfo')}</p>}
      {trigger && !capturedLabel && (
        <p>
          {t('commandSettings.form.captureExisting', { key: trigger.key + 1 })}
        </p>
      )}
    </div>
  );
}

function parseCapturedTrigger(value: string): { key: number } | null {
  try {
    const parsed = JSON.parse(value) as { version?: unknown; device?: unknown; key?: unknown };
    return parsed.version === 1 &&
      typeof parsed.device === 'string' &&
      /^[A-Za-z0-9_-]{1,128}$/.test(parsed.device) &&
      typeof parsed.key === 'number' &&
      Number.isInteger(parsed.key) &&
      parsed.key >= 0 &&
      parsed.key <= 255
      ? { key: parsed.key }
      : null;
  } catch {
    return null;
  }
}
