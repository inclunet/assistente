import { useNavigate, useLocation } from 'react-router-dom';
import { MenuButton, MenuItem } from './MenuButton';
import './Topbar.css';

export function Topbar() {
  const navigate = useNavigate();
  const location = useLocation();

  // Determina a página atual baseada na rota
  const getCurrentPage = (): string => {
    if (location.pathname === '/settings') return 'settings';
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
