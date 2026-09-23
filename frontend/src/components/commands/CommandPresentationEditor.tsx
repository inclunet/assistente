import { type ChangeEvent, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '../ui/Button';
import { Input } from '../ui/Input';
import { Select, type SelectOption } from '../ui/Select';
import { useAnnouncer } from '../../hooks/useAnnouncer';

export const COMMAND_PRESENTATION_LOCALES = ['pt-BR', 'en', 'es'] as const;
type CommandPresentationLocale = typeof COMMAND_PRESENTATION_LOCALES[number];

const COMMAND_PRESENTATION_ICONS = ['settings', 'chat', 'folder', 'play', 'stop', 'back', 'star'] as const;
export const MAX_COMMAND_PRESENTATION_IMAGE_BYTES = 1024 * 1024;
export const COMMAND_PRESENTATION_IMAGE_ACCEPT = 'image/png,image/jpeg';
const COMMAND_PRESENTATION_IMAGE_TYPES = new Set(['image/png', 'image/jpeg']);

export interface CommandPresentationEditorProps {
  readonly value?: Record<string, unknown>;
  readonly disabled?: boolean;
  readonly onBusyChange?: (busy: boolean) => void;
  readonly onChange: (value: Record<string, unknown>) => void;
}

const localeLabels: Record<CommandPresentationLocale, string> = {
  'pt-BR': 'commandSettings.presentation.locales.ptBR',
  en: 'commandSettings.presentation.locales.en',
  es: 'commandSettings.presentation.locales.es',
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function titleByLocale(value: Record<string, unknown> | undefined): Record<string, unknown> {
  return isRecord(value?.title_by_locale) ? value.title_by_locale : {};
}

function presentationIcon(value: Record<string, unknown> | undefined): string {
  return typeof value?.icon === 'string' ? value.icon : '';
}

function hasPresentationImage(value: Record<string, unknown> | undefined, field: 'image_ref' | 'image_upload'): boolean {
  return typeof value?.[field] === 'string' && value[field] !== '';
}

function titleError(value: unknown, t: (key: string) => string): string | undefined {
  if (value === undefined) return undefined;
  if (typeof value !== 'string') return t('commandSettings.presentation.invalid');
  const normalized = value.trim();
  if (normalized.includes('\0')) return t('commandSettings.presentation.invalidNul');
  if (Array.from(normalized).length > 256) return t('commandSettings.presentation.invalidLength');
  return undefined;
}

export function isCommandPresentationValid(value?: Record<string, unknown>): boolean {
  if (value === undefined) return true;
  if (!isRecord(value)) return false;
  if (value.title_by_locale === undefined) return true;
  const titles = value.title_by_locale;
  if (!isRecord(titles)) return false;
  return Object.entries(titles).every(([locale, title]) => {
    if (!COMMAND_PRESENTATION_LOCALES.includes(locale as CommandPresentationLocale)) return false;
    if (typeof title !== 'string') return false;
    const normalized = title.trim();
    return !normalized.includes('\0') && Array.from(normalized).length <= 256;
  });
}

/** Normalize only the editable title map; all other presentation metadata is retained. */
export function normalizeCommandPresentation(
  value?: Record<string, unknown>
): Record<string, unknown> | undefined {
  if (value === undefined) return undefined;
  const next = { ...value };
  const titles = titleByLocale(value);
  const normalizedTitles: Record<string, string> = {};
  for (const locale of COMMAND_PRESENTATION_LOCALES) {
    const title = titles[locale];
    if (typeof title === 'string' && title.trim() !== '') normalizedTitles[locale] = title.trim();
  }
  if (Object.keys(normalizedTitles).length > 0) next.title_by_locale = normalizedTitles;
  else delete next.title_by_locale;
  if (next.icon === '') delete next.icon;
  return next;
}

export function CommandPresentationEditor({
  value,
  disabled = false,
  onBusyChange,
  onChange,
}: CommandPresentationEditorProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const errorId = useId();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const valueRef = useRef(value);
  const readerRef = useRef<FileReader | null>(null);
  const pendingImageStateRef = useRef<{ imageRef: unknown; imageUpload: unknown }>();
  const readGenerationRef = useRef(0);
  const mountedRef = useRef(true);
  const [imageBusy, setImageBusy] = useState(false);
  const [imageError, setImageError] = useState<string>();
  const titles = titleByLocale(value);
  const icon = presentationIcon(value);
  const valid = isCommandPresentationValid(value);
  valueRef.current = value;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      readGenerationRef.current += 1;
      readerRef.current?.abort();
      readerRef.current = null;
      onBusyChange?.(false);
    };
  }, [onBusyChange]);

  const setImageBusyState = (busy: boolean) => {
    if (!mountedRef.current) return;
    setImageBusy(busy);
    onBusyChange?.(busy);
  };

  const invalidateImageRead = () => {
    readGenerationRef.current += 1;
    readerRef.current?.abort();
    readerRef.current = null;
    pendingImageStateRef.current = undefined;
    setImageBusyState(false);
  };

  useEffect(() => {
    const pending = pendingImageStateRef.current;
    if (!pending || !readerRef.current) return;
    if (pending.imageRef === value?.image_ref && pending.imageUpload === value?.image_upload) return;
    readGenerationRef.current += 1;
    readerRef.current.abort();
    readerRef.current = null;
    pendingImageStateRef.current = undefined;
    setImageBusy(false);
    onBusyChange?.(false);
  }, [onBusyChange, value?.image_ref, value?.image_upload]);

  useEffect(() => {
    if (!disabled || !readerRef.current) return;
    invalidateImageRead();
  }, [disabled]);

  const removeImage = () => {
    invalidateImageRead();
    setImageError(undefined);
    if (fileInputRef.current) fileInputRef.current.value = '';
    const next: Record<string, unknown> = { version: 1, ...valueRef.current };
    delete next.image_ref;
    delete next.image_upload;
    onChange(next);
    const message = t('commandSettings.presentation.image.removed');
    announce(message);
  };

  const handleImageChange = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;

    invalidateImageRead();
    setImageError(undefined);
    if (!COMMAND_PRESENTATION_IMAGE_TYPES.has(file.type)) {
      const message = t('commandSettings.presentation.image.invalidFormat');
      setImageError(message);
      return;
    }
    if (file.size > MAX_COMMAND_PRESENTATION_IMAGE_BYTES) {
      const message = t('commandSettings.presentation.image.tooLarge');
      setImageError(message);
      return;
    }

    const generation = readGenerationRef.current;
    const reader = new FileReader();
    readerRef.current = reader;
    pendingImageStateRef.current = {
      imageRef: valueRef.current?.image_ref,
      imageUpload: valueRef.current?.image_upload,
    };
    setImageBusyState(true);
    reader.onload = () => {
      if (!mountedRef.current || generation !== readGenerationRef.current || readerRef.current !== reader) return;
      const result = reader.result;
      if (typeof result !== 'string') {
        const message = t('commandSettings.presentation.image.readError');
        setImageError(message);
        readerRef.current = null;
        pendingImageStateRef.current = undefined;
        setImageBusyState(false);
        return;
      }
      const comma = result.indexOf(',');
      if (comma < 0 || result.slice(comma + 1) === '') {
        const message = t('commandSettings.presentation.image.readError');
        setImageError(message);
        readerRef.current = null;
        pendingImageStateRef.current = undefined;
        setImageBusyState(false);
        return;
      }
      const next: Record<string, unknown> = { version: 1, ...valueRef.current };
      delete next.image_ref;
      next.image_upload = result.slice(comma + 1);
      readerRef.current = null;
      pendingImageStateRef.current = undefined;
      setImageBusyState(false);
      onChange(next);
      announce(t('commandSettings.presentation.image.selected'));
    };
    reader.onerror = () => {
      if (!mountedRef.current || generation !== readGenerationRef.current || readerRef.current !== reader) return;
      readerRef.current = null;
      pendingImageStateRef.current = undefined;
      setImageBusyState(false);
      const message = t('commandSettings.presentation.image.readError');
      setImageError(message);
    };
    reader.onabort = () => {
      if (generation !== readGenerationRef.current || readerRef.current !== reader) return;
      readerRef.current = null;
      pendingImageStateRef.current = undefined;
      setImageBusyState(false);
    };
    try {
      reader.readAsDataURL(file);
    } catch {
      reader.onerror?.(new ProgressEvent('error') as ProgressEvent<FileReader>);
    }
  };

  const iconOptions: SelectOption[] = [
    { value: '', label: t('commandSettings.presentation.icon.none') },
    ...COMMAND_PRESENTATION_ICONS.map((iconToken) => ({
      value: iconToken,
      label: t(`commandSettings.presentation.icon.${iconToken}`),
    })),
    ...(icon && !COMMAND_PRESENTATION_ICONS.includes(icon as typeof COMMAND_PRESENTATION_ICONS[number])
      ? [{
          value: icon,
          label: t('commandSettings.presentation.icon.unavailable', { icon }),
        }]
      : []),
  ];

  const updateIcon = (nextIcon: string) => {
    const next: Record<string, unknown> = { version: 1, ...value };
    if (nextIcon === '') delete next.icon;
    else next.icon = nextIcon;
    onChange(next);
  };

  const updateTitle = (locale: CommandPresentationLocale, nextTitle: string) => {
    const next: Record<string, unknown> = { version: 1, ...value };
    const nextTitles = { ...titleByLocale(value) };
    if (nextTitle === '') delete nextTitles[locale];
    else nextTitles[locale] = nextTitle;
    if (Object.keys(nextTitles).length > 0) next.title_by_locale = nextTitles;
    else delete next.title_by_locale;
    onChange(next);
  };

  return (
    <fieldset disabled={disabled} aria-describedby={valid ? undefined : errorId}>
      <legend>{t('commandSettings.formExtra.presentation')}</legend>
      <p>{t('commandSettings.presentation.help')}</p>
      <Select
        id={`${errorId}-icon`}
        label={t('commandSettings.presentation.icon.label')}
        hint={t('commandSettings.presentation.icon.hint')}
        value={icon}
        options={iconOptions}
        onChange={(event) => updateIcon(event.target.value)}
      />
      {COMMAND_PRESENTATION_LOCALES.map((locale) => (
        <Input
          key={locale}
          id={`${errorId}-${locale}`}
          label={t(localeLabels[locale])}
          value={typeof titles[locale] === 'string' ? titles[locale] as string : ''}
          hint={t('commandSettings.presentation.fallback')}
          error={titleError(titles[locale], t)}
          onChange={(event) => updateTitle(locale, event.target.value)}
        />
      ))}
      <Input
        ref={fileInputRef}
        type="file"
        accept={COMMAND_PRESENTATION_IMAGE_ACCEPT}
        label={t('commandSettings.presentation.image.label')}
        hint={t('commandSettings.presentation.image.hint')}
        error={imageError}
        disabled={imageBusy}
        onChange={handleImageChange}
      />
      {hasPresentationImage(value, 'image_upload') && <p>{t('commandSettings.presentation.image.selected')}</p>}
      {!hasPresentationImage(value, 'image_upload') && hasPresentationImage(value, 'image_ref') && (
        <p>{t('commandSettings.presentation.image.saved')}</p>
      )}
      {(hasPresentationImage(value, 'image_upload') || hasPresentationImage(value, 'image_ref')) && (
        <Button type="button" variant="secondary" onClick={removeImage}>
          {t('commandSettings.presentation.image.remove')}
        </Button>
      )}
      {!valid && <p id={errorId}>{t('commandSettings.presentation.invalid')}</p>}
    </fieldset>
  );
}

export default CommandPresentationEditor;
