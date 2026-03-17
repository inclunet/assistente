import { useNavigate, useLocation } from 'react-router-dom';
import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { MenuButton, MenuItem, MenuButtonRef } from './MenuButton';
import { useTheme, THEMES, type ThemeId } from '../../hooks/useTheme';
import { LANGUAGES, type LanguageId } from '../../lib/i18n';
import { useSettingsStore } from '../../store/settingsStore';
import './Topbar.css';

export function Topbar() {
  const navigate = useNavigate();
  const location = useLocation();
  const menuButtonRef = useRef<MenuButtonRef>(null);
  const { t, i18n } = useTranslation();
  const { theme: currentTheme, setTheme } = useTheme();
  const updateConfig = useSettingsStore((s) => s.updateConfig);
  const currentLang = i18n.language as LanguageId;

  const setLanguage = (id: LanguageId) => {
    i18n.changeLanguage(id);
    updateConfig({ language: id });
  };

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.altKey && !event.ctrlKey && !event.shiftKey && !event.metaKey) {
        const key = event.key.toLowerCase();
        const altRoutes: Record<string, string> = {
          m: '__menu__',
          c: '/',
          e: '/editor',
          t: '/terminal',
          h: '/history',
          p: '/profiles',
        };
        const target = altRoutes[key];
        if (target) {
          event.preventDefault();
          if (target === '__menu__') {
            menuButtonRef.current?.toggleMenu();
          } else {
            navigate(target);
          }
          return;
        }
      }
      if (event.key === 'F1') {
        event.preventDefault();
        navigate('/help');
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [navigate]);

  // Determina a página atual baseada na rota
  const getCurrentPage = (): string => {
    if (location.pathname === '/terminal') return 'terminal';
    if (location.pathname === '/editor') return 'editor';
    if (location.pathname === '/allowlists') return 'allowlists';
    if (location.pathname === '/skills') return 'skills';
    if (location.pathname === '/mcp') return 'mcp';
    if (location.pathname === '/channels') return 'channels';
    if (location.pathname === '/credentials') return 'credentials';
    if (location.pathname === '/providers') return 'providers';
    if (location.pathname === '/settings') return 'settings';
    if (location.pathname === '/profiles') return 'profiles';
    if (location.pathname === '/history') return 'history';
    if (location.pathname === '/help') return 'help';
    if (location.pathname === '/about') return 'about';
    if (location.pathname === '/update') return 'update';
    return 'chat';
  };

  const menuItems: MenuItem[] = [
    {
      id: 'chat',
      label: t('menu.chat'),
      icon: '💬',
      shortcut: 'Alt+C',
      onClick: () => navigate('/'),
    },
    {
      id: 'terminal',
      label: t('menu.terminal'),
      icon: '>_',
      shortcut: 'Alt+T',
      onClick: () => navigate('/terminal'),
    },
    {
      id: 'editor',
      label: t('menu.editor'),
      icon: '📝',
      shortcut: 'Alt+E',
      onClick: () => navigate('/editor'),
    },
    {
      id: 'history',
      label: t('menu.history'),
      icon: '📜',
      shortcut: 'Alt+H',
      onClick: () => navigate('/history'),
    },
    {
      id: 'profiles',
      label: t('menu.profiles'),
      icon: '🎭',
      shortcut: 'Alt+P',
      onClick: () => navigate('/profiles'),
    },
    {
      id: 'allowlists',
      label: t('menu.allowlists'),
      icon: '🛡️',
      onClick: () => navigate('/allowlists'),
    },
    {
      id: 'skills',
      label: t('menu.skills'),
      icon: '🧠',
      onClick: () => navigate('/skills'),
    },
    {
      id: 'mcp',
      label: t('menu.mcp'),
      icon: '🔌',
      onClick: () => navigate('/mcp'),
    },
    {
      id: 'channels',
      label: t('menu.channels'),
      icon: '📡',
      onClick: () => navigate('/channels'),
    },
    {
      id: 'credentials',
      label: t('menu.credentials'),
      icon: '🔐',
      onClick: () => navigate('/credentials'),
    },
    {
      id: 'providers',
      label: t('menu.providers'),
      icon: '🤖',
      onClick: () => navigate('/providers'),
    },
    {
      id: 'theme',
      label: t('menu.theme'),
      icon: '🎨',
      submenu: THEMES.map((th) => ({
        id: `theme-${th.id}`,
        label: `${currentTheme === th.id ? '● ' : ''}${th.label}`,
        icon: currentTheme === th.id ? '✓' : ' ',
        onClick: () => setTheme(th.id as ThemeId),
      })),
    },
    {
      id: 'language',
      label: t('menu.language'),
      icon: '🌐',
      submenu: LANGUAGES.map((lang) => ({
        id: `lang-${lang.id}`,
        label: `${currentLang === lang.id ? '● ' : ''}${lang.nativeLabel}`,
        icon: currentLang === lang.id ? '✓' : ' ',
        onClick: () => setLanguage(lang.id),
      })),
    },
    {
      id: 'settings',
      label: t('menu.restoreDefaults'),
      icon: '↩️',
      onClick: () => navigate('/settings'),
    },
    {
      id: 'help',
      label: t('menu.help'),
      icon: '📚',
      shortcut: 'F1',
      onClick: () => navigate('/help'),
    },
    {
      id: 'about',
      label: t('menu.about'),
      icon: 'ℹ️',
      onClick: () => navigate('/about'),
    },
  ];

  return (
    <header className="topbar">
      <div className="topbar__left">
        <MenuButton 
          ref={menuButtonRef}
          items={menuItems} 
          currentItemId={getCurrentPage()}
          buttonLabel={t('menu.navLabel')}
        />
        <h1 className="topbar__title">{t('menu.appTitle')}</h1>
      </div>
    </header>
  );
}
