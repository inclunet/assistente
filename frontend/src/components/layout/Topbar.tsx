import { useNavigate, useLocation } from 'react-router-dom';
import { useEffect, useRef } from 'react';
import { MenuButton, MenuItem, MenuButtonRef } from './MenuButton';
import { useTheme, THEMES, type ThemeId } from '../../hooks/useTheme';
import './Topbar.css';

export function Topbar() {
  const navigate = useNavigate();
  const location = useLocation();
  const menuButtonRef = useRef<MenuButtonRef>(null);
  const { theme: currentTheme, setTheme } = useTheme();

  // Atalho Alt+M para abrir/fechar o menu e F1 para ajuda
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.altKey && event.key.toLowerCase() === 'm') {
        event.preventDefault();
        menuButtonRef.current?.toggleMenu();
      }
      // F1 abre a página de ajuda
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
      label: 'Chat',
      icon: '💬',
      onClick: () => navigate('/'),
    },
    {
      id: 'terminal',
      label: 'Terminal',
      icon: '>_',
      onClick: () => navigate('/terminal'),
    },
    {
      id: 'editor',
      label: 'Editor',
      icon: '📝',
      onClick: () => navigate('/editor'),
    },
    {
      id: 'history',
      label: 'Histórico',
      icon: '📜',
      onClick: () => navigate('/history'),
    },
    {
      id: 'profiles',
      label: 'Perfis',
      icon: '🎭',
      onClick: () => navigate('/profiles'),
    },
    {
      id: 'allowlists',
      label: 'Allowlists',
      icon: '🛡️',
      onClick: () => navigate('/allowlists'),
    },
    {
      id: 'skills',
      label: 'Skills',
      icon: '🧠',
      onClick: () => navigate('/skills'),
    },
    {
      id: 'mcp',
      label: 'MCP',
      icon: '🔌',
      onClick: () => navigate('/mcp'),
    },
    {
      id: 'channels',
      label: 'Canais',
      icon: '📡',
      onClick: () => navigate('/channels'),
    },
    {
      id: 'credentials',
      label: 'Credenciais',
      icon: '🔐',
      onClick: () => navigate('/credentials'),
    },
    {
      id: 'providers',
      label: 'Provedores LLM',
      icon: '🤖',
      onClick: () => navigate('/providers'),
    },
    {
      id: 'theme',
      label: 'Tema',
      icon: '🎨',
      submenu: THEMES.map((t) => ({
        id: `theme-${t.id}`,
        label: `${currentTheme === t.id ? '● ' : ''}${t.label}`,
        icon: currentTheme === t.id ? '✓' : ' ',
        onClick: () => setTheme(t.id as ThemeId),
      })),
    },
    {
      id: 'settings',
      label: 'Restaurar Padrões',
      icon: '↩️',
      onClick: () => navigate('/settings'),
    },
    {
      id: 'help',
      label: 'Ajuda',
      icon: '📚',
      shortcut: 'F1',
      onClick: () => navigate('/help'),
    },
    {
      id: 'about',
      label: 'Sobre',
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
          buttonLabel="Menu de navegação (Alt+M)"
        />
        <h1 className="topbar__title">Assistente IA</h1>
      </div>
    </header>
  );
}
