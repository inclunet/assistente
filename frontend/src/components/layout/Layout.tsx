import { Outlet } from 'react-router-dom';
import { Topbar } from './Topbar';
import { useDocumentTitle } from '../../hooks/useDocumentTitle';
import './Layout.css';

export function Layout() {
  // Atualiza o título do documento baseado na página/conversa atual
  useDocumentTitle();

  return (
    <div className="layout">
      <Topbar />
      <main className="layout__content">
        <Outlet />
      </main>
    </div>
  );
}
