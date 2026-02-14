import { createHashRouter } from 'react-router-dom';
import App from '../App';
import { Layout } from '../components/layout/Layout';
import ChatPage from '../pages/ChatPage';
import SettingsPage from '../pages/SettingsPage';
import ProfilesPage from '../pages/ProfilesPage';
import HistoryPage from '../pages/HistoryPage';
import HelpPage from '../pages/HelpPage';
import TerminalPage from '../pages/TerminalPage';
import AllowlistPage from '../pages/AllowlistPage';
import SkillsPage from '../pages/SkillsPage';
import McpPage from '../pages/McpPage';
import ChannelsPage from '../pages/ChannelsPage';

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
            path: 'settings',
            element: <SettingsPage />,
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
        ],
      },
    ],
  },
]);
