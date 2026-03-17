import { useEffect } from 'react';
import { Outlet, useLocation } from 'react-router-dom';
import { Topbar } from './Topbar';
import { useDocumentTitle } from '../../hooks/useDocumentTitle';
import { ensureModalCleanup } from '../ui/Modal';
import './Layout.css';

export function Layout() {
  // Atualiza o título do documento baseado na página/conversa atual
  useDocumentTitle();

  const { pathname } = useLocation();

  // Safety net: ao mudar de rota, garante que inert/aria-hidden sejam
  // removidos caso a stack de modais tenha ficado dessincronizada.
  useEffect(() => {
    ensureModalCleanup();
  }, [pathname]);

  return (
    <div className="layout">
      <Topbar />
      <main className="layout__content">
        <Outlet />
      </main>
    </div>
  );
}
