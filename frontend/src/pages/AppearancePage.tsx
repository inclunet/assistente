import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import i18n from '../lib/i18n';
import { LANGUAGES, type LanguageId } from '../lib/i18n';
import { useTheme, THEMES, type ThemeId } from '../hooks/useTheme';
import { useSettingsStore } from '../store/settingsStore';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useContentPageLandmarks } from '../hooks/useContentPageLandmarks';
import { useRadioGroup } from '../hooks/useRadioGroup';
import { Checkbox } from '../components/ui/Checkbox';
import './AppearancePage.css';

export default function AppearancePage() {
  const { t } = useTranslation();
  const { theme: currentTheme, setTheme } = useTheme();
  const updateConfig = useSettingsStore((s) => s.updateConfig);
  const decisionAlertSound = useSettingsStore((s) => s.config.decisionAlertSound);
  const preventScreenLock = useSettingsStore((s) => s.config.preventScreenLock);
  const editorExternalChange = useSettingsStore((s) => s.config.editor.externalChange);
  const { announce } = useAnnouncer();
  const currentLang = i18n.language as LanguageId;
  useContentPageLandmarks({ pageClass: 'appearance-page' });

  const themeIds = useMemo(() => THEMES.map((th) => th.id), []);
  const langIds = useMemo(() => LANGUAGES.map((l) => l.id), []);

  const handleThemeChange = useCallback(
    (id: ThemeId) => {
      setTheme(id);
      const fallbackLabel = THEMES.find((th) => th.id === id)?.label ?? id;
      const label = t(`appearance.themes.${id}.label`, fallbackLabel);
      announce(t('appearance.announce.themeChanged', { label }));
    },
    [setTheme, announce, t],
  );

  const handleLanguageChange = useCallback(
    (id: LanguageId) => {
      i18n.changeLanguage(id);
      updateConfig({ language: id });
      const label = LANGUAGES.find((l) => l.id === id)?.nativeLabel ?? id;
      announce(t('appearance.announce.languageChanged', { label }));
    },
    [updateConfig, announce, t],
  );

  const handleDecisionAlertSoundChange = useCallback(
    (enabled: boolean) => {
      updateConfig({ decisionAlertSound: enabled });
      announce(
        enabled
          ? t('appearance.announce.decisionAlertSoundOn')
          : t('appearance.announce.decisionAlertSoundOff'),
      );
    },
    [updateConfig, announce, t],
  );

  const handlePreventScreenLockChange = useCallback(
    (enabled: boolean) => {
      updateConfig({ preventScreenLock: enabled });
      announce(
        enabled
          ? t('appearance.announce.preventScreenLockOn')
          : t('appearance.announce.preventScreenLockOff'),
      );
    },
    [updateConfig, announce, t],
  );

  const handleEditorExternalChange = useCallback(
    (value: 'autoReload' | 'prompt') => {
      updateConfig({ editor: { externalChange: value } });
      announce(
        value === 'autoReload'
          ? t('appearance.announce.editorExternalChangeAutoReload')
          : t('appearance.announce.editorExternalChangePrompt'),
      );
    },
    [updateConfig, announce, t],
  );

  const themeGroupRef = useRadioGroup({
    items: themeIds,
    selectedId: currentTheme,
    onChange: handleThemeChange,
  });

  const langGroupRef = useRadioGroup({
    items: langIds,
    selectedId: currentLang,
    onChange: handleLanguageChange,
  });

  // ─────────────────────────────────────────────────────────────────────────
  // EXCEÇÃO OFICIAL ao sistema de tokens (issue #243).
  //
  // Estes previews precisam mostrar, lado a lado, as cores REAIS de cada tema
  // — inclusive dos temas que NÃO estão ativos no momento. Usar as variáveis CSS
  // do tema (`var(--bg-base)` etc.) faria todos os cards exibirem a paleta do
  // tema atual, tornando o preview inútil. Por isso, e somente aqui, os valores
  // são literais.
  //
  // Cada linha espelha os tokens do `theme.css` na ordem
  // [--bg-base, --bg-surface, --accent, --text-primary]. Ao alterar uma cor de
  // tema no `theme.css`, atualize o valor correspondente abaixo.
  // ─────────────────────────────────────────────────────────────────────────
  const THEME_SWATCHES: Record<ThemeId, string[]> = {
    assistente:      ['#0a1628', '#0f1f3a', '#a78bfa', '#eef2f9'],
    amethyst:        ['#12082a', '#1c1040', '#a78bfa', '#f0ecf9'],
    midnight:        ['#0c0f14', '#151921', '#a78bfa', '#e8ecf2'],
    light:           ['#f0f4fa', '#ffffff', '#820AD1', '#0a1628'],
    'high-contrast': ['#000000', '#0a0a0a', '#c084fc', '#ffffff'],
  };

  return (
    <div className="appearance-page">
      <header className="appearance-page__header">
        <h1>{t('appearance.pageTitle', 'Aparência')}</h1>
        <p>{t('appearance.description', 'Personalize o tema visual e o idioma do aplicativo.')}</p>
      </header>

      <main className="appearance-page__content">
        <section className="appearance-section">
          <h2 className="appearance-section__title">{t('appearance.themeTitle', 'Tema')}</h2>
          <div
            ref={themeGroupRef}
            className="theme-grid"
            role="radiogroup"
            aria-label={t('appearance.aria.selectTheme', 'Selecionar tema')}
          >
            {THEMES.map((theme) => (
              <button
                key={theme.id}
                className={`theme-card${currentTheme === theme.id ? ' theme-card--active' : ''}`}
                role="radio"
                aria-checked={currentTheme === theme.id}
                tabIndex={currentTheme === theme.id ? 0 : -1}
                onClick={() => handleThemeChange(theme.id)}
              >
                <div className="theme-card__preview">
                  {THEME_SWATCHES[theme.id].map((color, idx) => (
                    <div key={idx} className="theme-card__swatch" style={{ background: color }} />
                  ))}
                </div>
                <span className="theme-card__name">{t(`appearance.themes.${theme.id}.label`, theme.label)}</span>
                <span className="theme-card__desc">{t(`appearance.themes.${theme.id}.desc`, theme.description)}</span>
              </button>
            ))}
          </div>
        </section>

        <section className="appearance-section">
          <h2 className="appearance-section__title">{t('appearance.languageTitle', 'Idioma')}</h2>
          <p className="appearance-section__description">
            {t('appearance.languageDescription', 'Selecione o idioma da interface. A alteração é aplicada imediatamente.')}
          </p>
          <div
            ref={langGroupRef}
            className="language-grid"
            role="radiogroup"
            aria-label={t('appearance.aria.selectLanguage', 'Selecionar idioma')}
          >
            {LANGUAGES.map((lang) => (
              <button
                key={lang.id}
                className={`language-card${currentLang === lang.id ? ' language-card--active' : ''}`}
                role="radio"
                aria-checked={currentLang === lang.id}
                tabIndex={currentLang === lang.id ? 0 : -1}
                onClick={() => handleLanguageChange(lang.id)}
              >
                <span className="language-card__name">{lang.nativeLabel}</span>
                {lang.label !== lang.nativeLabel && (
                  <span className="language-card__alt">{lang.label}</span>
                )}
              </button>
            ))}
          </div>
        </section>

        <section className="appearance-section">
          <h2 className="appearance-section__title">
            {t('appearance.accessibilityTitle')}
          </h2>
          <p className="appearance-section__description">
            {t('appearance.accessibilityDescription')}
          </p>
          <div className="appearance-pref">
            <Checkbox
              label={t('appearance.decisionAlertSound')}
              checked={decisionAlertSound}
              aria-describedby="appearance-decision-alert-hint"
              onChange={(e) => handleDecisionAlertSoundChange(e.target.checked)}
            />
            <p id="appearance-decision-alert-hint" className="appearance-pref__hint">
              {t('appearance.decisionAlertSoundHint')}
            </p>
          </div>
          <div className="appearance-pref">
            <Checkbox
              label={t('appearance.preventScreenLock')}
              checked={preventScreenLock}
              aria-describedby="appearance-prevent-screen-lock-hint"
              onChange={(e) => handlePreventScreenLockChange(e.target.checked)}
            />
            <p id="appearance-prevent-screen-lock-hint" className="appearance-pref__hint">
              {t('appearance.preventScreenLockHint')}
            </p>
          </div>
        </section>

        <section className="appearance-section">
          <h2 className="appearance-section__title">
            {t('appearance.editorTitle')}
          </h2>
          <p className="appearance-section__description">
            {t('appearance.editorDescription')}
          </p>
          <div className="appearance-pref">
            <label className="appearance-pref__label" htmlFor="editor-external-change">
              {t('appearance.editorExternalChange')}
            </label>
            <select
              id="editor-external-change"
              className="appearance-pref__select"
              value={editorExternalChange}
              aria-describedby="editor-external-change-hint"
              onChange={(event) =>
                handleEditorExternalChange(event.target.value as 'autoReload' | 'prompt')
              }
            >
              <option value="autoReload">
                {t('appearance.editorExternalChangeOptions.autoReload')}
              </option>
              <option value="prompt">
                {t('appearance.editorExternalChangeOptions.prompt')}
              </option>
            </select>
            <p id="editor-external-change-hint" className="appearance-pref__hint appearance-pref__hint--aligned">
              {t('appearance.editorExternalChangeHint')}
            </p>
          </div>
        </section>
      </main>
    </div>
  );
}
