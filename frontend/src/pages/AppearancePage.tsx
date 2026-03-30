import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import i18n from '../lib/i18n';
import { LANGUAGES, type LanguageId } from '../lib/i18n';
import { useTheme, THEMES, type ThemeId } from '../hooks/useTheme';
import { useSettingsStore } from '../store/settingsStore';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useContentPageLandmarks } from '../hooks/useContentPageLandmarks';
import { useRadioGroup } from '../hooks/useRadioGroup';
import './AppearancePage.css';

export default function AppearancePage() {
  const { t } = useTranslation();
  const { theme: currentTheme, setTheme } = useTheme();
  const updateConfig = useSettingsStore((s) => s.updateConfig);
  const { announce } = useAnnouncer();
  const currentLang = i18n.language as LanguageId;
  useContentPageLandmarks({ pageClass: 'appearance-page' });

  const themeIds = useMemo(() => THEMES.map((th) => th.id), []);
  const langIds = useMemo(() => LANGUAGES.map((l) => l.id), []);

  const handleThemeChange = useCallback(
    (id: ThemeId) => {
      setTheme(id);
      const label = THEMES.find((th) => th.id === id)?.label ?? id;
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

  const THEME_SWATCHES: Record<ThemeId, string[]> = {
    assistente:      ['#0a1628', '#0f1f3a', '#2b7ef4', '#eef2f9'],
    amethyst:        ['#12082a', '#1c1040', '#a78bfa', '#f0ecf9'],
    midnight:        ['#0c0f14', '#151921', '#60a5fa', '#e8ecf2'],
    light:           ['#f0f4fa', '#ffffff', '#2b7ef4', '#0a1628'],
    'high-contrast': ['#000000', '#1a1a1a', '#5babff', '#ffffff'],
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
                <span className="theme-card__name">{theme.label}</span>
                <span className="theme-card__desc">{theme.description}</span>
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
      </main>
    </div>
  );
}
