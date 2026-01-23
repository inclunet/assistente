import { useNavigate, useLocation } from 'react-router-dom';
import { MenuButton, MenuItem } from './MenuButton';
import './Topbar.css';

export function Topbar() {
  const navigate = useNavigate();
  const location = useLocation();

  // Determina a página atual baseada na rota
  const getCurrentPage = (): string => {
    if (location.pathname === '/settings') return 'settings';
    if (location.pathname === '/history') return 'history';
    if (location.pathname === '/memory') return 'memory';
    if (location.pathname === '/faq') return 'faq';
    if (location.pathname === '/agents') return 'agents';
    if (location.pathname === '/oauth') return 'oauth';
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
      id: 'memory',
      label: 'Memória',
      icon: '🧠',
      onClick: () => navigate('/memory'),
    },
    {
      id: 'faq',
      label: 'FAQ',
      icon: '❓',
      onClick: () => navigate('/faq'),
    },
    {
      id: 'agents',
      label: 'Agentes',
      icon: '🤖',
      onClick: () => navigate('/agents'),
    },
    {
      id: 'oauth',
      label: 'OAuth',
      icon: '🔐',
      onClick: () => navigate('/oauth'),
    },
    {
      id: 'settings',
      label: 'Configurações',
      icon: '⚙️',
      onClick: () => navigate('/settings'),
    },
  ];

  return (
    <header className="topbar">
      <div className="topbar__left">
        <MenuButton 
          items={menuItems} 
          currentItemId={getCurrentPage()}
          buttonLabel="Menu de navegação"
        />
        <h1 className="topbar__title">Assistente IA</h1>
      </div>
    </header>
  );
}
