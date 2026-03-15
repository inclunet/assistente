import { Suspense, lazy } from 'react';
import { createHashRouter } from 'react-router-dom';
import App from '../App';
import { Layout } from '../components/layout/Layout';

const ChatPage = lazy(() => import('../pages/ChatPage'));
const RestoreDefaultsPage = lazy(() => import('../pages/RestoreDefaultsPage'));
const ProfilesPage = lazy(() => import('../pages/ProfilesPage'));
const HistoryPage = lazy(() => import('../pages/HistoryPage'));
const HelpPage = lazy(() => import('../pages/HelpPage'));
const TerminalPage = lazy(() => import('../pages/TerminalPage'));
const AllowlistPage = lazy(() => import('../pages/AllowlistPage'));
const SkillsPage = lazy(() => import('../pages/SkillsPage'));
const McpPage = lazy(() => import('../pages/McpPage'));
const ChannelsPage = lazy(() => import('../pages/ChannelsPage'));
const UpdatePage = lazy(() => import('../pages/UpdatePage'));
const AboutPage = lazy(() => import('../pages/AboutPage'));
const EditorPage = lazy(() => import('../pages/EditorPage'));
const CredentialsPage = lazy(() => import('../pages/CredentialsPage'));
const ProvidersPage = lazy(() => import('../pages/ProvidersPage'));

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
        element: <Layout />,
        children: [
          {
            index: true,
            element: withSuspense(<ChatPage />),
          },
          {
            path: 'terminal',
            element: withSuspense(<TerminalPage />),
          },
          {
            path: 'editor',
            element: withSuspense(<EditorPage />),
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
        ],
      },
    ],
  },
]);
