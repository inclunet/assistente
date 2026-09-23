import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '../ui/Input';
import { Select, type SelectOption } from '../ui/Select';

export const COMMAND_PRESENTATION_LOCALES = ['pt-BR', 'en', 'es'] as const;
type CommandPresentationLocale = typeof COMMAND_PRESENTATION_LOCALES[number];

const COMMAND_PRESENTATION_ICONS = ['settings', 'chat', 'folder', 'play', 'stop', 'back', 'star'] as const;

export interface CommandPresentationEditorProps {
  readonly value?: Record<string, unknown>;
  readonly disabled?: boolean;
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
  onChange,
}: CommandPresentationEditorProps) {
  const { t } = useTranslation();
  const errorId = useId();
  const titles = titleByLocale(value);
  const icon = presentationIcon(value);
  const valid = isCommandPresentationValid(value);
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
      {!valid && <p id={errorId}>{t('commandSettings.presentation.invalid')}</p>}
    </fieldset>
  );
}

export default CommandPresentationEditor;
