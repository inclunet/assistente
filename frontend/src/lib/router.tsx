import { createHashRouter } from 'react-router-dom';
import App from '../App';
import { Layout } from '../components/layout/Layout';
import ChatPage from '../pages/ChatPage';
import RestoreDefaultsPage from '../pages/RestoreDefaultsPage';
import ProfilesPage from '../pages/ProfilesPage';
import HistoryPage from '../pages/HistoryPage';
import HelpPage from '../pages/HelpPage';
import TerminalPage from '../pages/TerminalPage';
import AllowlistPage from '../pages/AllowlistPage';
import SkillsPage from '../pages/SkillsPage';
import McpPage from '../pages/McpPage';
import ChannelsPage from '../pages/ChannelsPage';
import UpdatePage from '../pages/UpdatePage';
import AboutPage from '../pages/AboutPage';
import EditorPage from '../pages/EditorPage';
import CredentialsPage from '../pages/CredentialsPage';
import ProvidersPage from '../pages/ProvidersPage';

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
            element: <ChatPage />,
          },
          {
            path: 'terminal',
            element: <TerminalPage />,
          },
          {
            path: 'editor',
            element: <EditorPage />,
          },
          {
            path: 'allowlists',
            element: <AllowlistPage />,
          },
          {
            path: 'skills',
            element: <SkillsPage />,
          },
          {
            path: 'mcp',
            element: <McpPage />,
          },
          {
            path: 'channels',
            element: <ChannelsPage />,
          },
          {
            path: 'credentials',
            element: <CredentialsPage />,
          },
          {
            path: 'providers',
            element: <ProvidersPage />,
          },
          {
            path: 'settings',
            element: <RestoreDefaultsPage />,
          },
          {
            path: 'profiles',
            element: <ProfilesPage />,
          },
          {
            path: 'history',
            element: <HistoryPage />,
          },
          {
            path: 'help',
            element: <HelpPage />,
          },
          {
            path: 'about',
            element: <AboutPage />,
          },
          {
            path: 'update',
            element: <UpdatePage />,
          },
        ],
      },
    ],
  },
]);
