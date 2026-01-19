import { Outlet } from 'react-router-dom';
import { Topbar } from './Topbar';
import './Layout.css';

export function Layout() {
  return (
    <div className="layout">
      <Topbar />
      <main className="layout__content">
        <Outlet />
      </main>
    </div>
  );
}
