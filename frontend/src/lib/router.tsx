import { Suspense, lazy } from 'react';
import { createHashRouter } from 'react-router-dom';
import App from '../App';
import { WorkspaceLayout } from '../components/workspace';

const RestoreDefaultsPage = lazy(() => import('../pages/RestoreDefaultsPage'));
const ProfilesPage = lazy(() => import('../pages/ProfilesPage'));
const HistoryPage = lazy(() => import('../pages/HistoryPage'));
const HelpPage = lazy(() => import('../pages/HelpPage'));
const AllowlistPage = lazy(() => import('../pages/AllowlistPage'));
const SkillsPage = lazy(() => import('../pages/SkillsPage'));
const McpPage = lazy(() => import('../pages/McpPage'));
const ChannelsPage = lazy(() => import('../pages/ChannelsPage'));
const UpdatePage = lazy(() => import('../pages/UpdatePage'));
const AboutPage = lazy(() => import('../pages/AboutPage'));
const CredentialsPage = lazy(() => import('../pages/CredentialsPage'));
const ProvidersPage = lazy(() => import('../pages/ProvidersPage'));
const TaskListsPage = lazy(() => import('../pages/TaskListsPage'));

const withSuspense = (element: JSX.Element) => (
  <Suspense fallback={<div className="page-loading" aria-busy="true" />}>
    {element}
  </Suspense>
);

export const router = createHashRouter([
  {
    path: '/',
    element: <App />,
    children: [
      {
        element: <WorkspaceLayout />,
        children: [
          {
            index: true,
            // WorkspaceLayout renderiza o workspace (abas mistas) na rota raiz
          },
          {
            path: 'allowlists',
            element: withSuspense(<AllowlistPage />),
          },
          {
            path: 'skills',
            element: withSuspense(<SkillsPage />),
          },
          {
            path: 'mcp',
            element: withSuspense(<McpPage />),
          },
          {
            path: 'channels',
            element: withSuspense(<ChannelsPage />),
          },
          {
            path: 'credentials',
            element: withSuspense(<CredentialsPage />),
          },
          {
            path: 'providers',
            element: withSuspense(<ProvidersPage />),
          },
          {
            path: 'settings',
            element: withSuspense(<RestoreDefaultsPage />),
          },
          {
            path: 'profiles',
            element: withSuspense(<ProfilesPage />),
          },
          {
            path: 'history',
            element: withSuspense(<HistoryPage />),
          },
          {
            path: 'help',
            element: withSuspense(<HelpPage />),
          },
          {
            path: 'about',
            element: withSuspense(<AboutPage />),
          },
          {
            path: 'update',
            element: withSuspense(<UpdatePage />),
          },
          {
            path: 'tasklists',
            element: withSuspense(<TaskListsPage />),
          },
        ],
      },
    ],
  },
]);
