import { useNavigate, useLocation } from 'react-router-dom';
import { useEffect, useRef } from 'react';
import { MenuButton, MenuItem, MenuButtonRef } from './MenuButton';
import './Topbar.css';

export function Topbar() {
  const navigate = useNavigate();
  const location = useLocation();
  const menuButtonRef = useRef<MenuButtonRef>(null);

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
    if (location.pathname === '/settings') return 'settings';
    if (location.pathname === '/history') return 'history';
    if (location.pathname === '/help') return 'help';
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
      id: 'history',
      label: 'Histórico',
      icon: '📜',
      onClick: () => navigate('/history'),
    },
    {
      id: 'settings',
      label: 'Configurações',
      icon: '⚙️',
      onClick: () => navigate('/settings'),
    },
    {
      id: 'help',
      label: 'Ajuda',
      icon: '📚',
      shortcut: 'F1',
      onClick: () => navigate('/help'),
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
