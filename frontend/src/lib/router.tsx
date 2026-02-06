import { createHashRouter } from 'react-router-dom';
import App from '../App';
import { Layout } from '../components/layout/Layout';
import ChatPage from '../pages/ChatPage';
import SettingsPage from '../pages/SettingsPage';
import HistoryPage from '../pages/HistoryPage';
import AgentsPage from '../pages/AgentsPage';
import SkillsPage from '../pages/SkillsPage';
import OAuthPage from '../pages/OAuthPage';
import HelpPage from '../pages/HelpPage';

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
            path: 'settings',
            element: <SettingsPage />,
          },
          {
            path: 'history',
            element: <HistoryPage />,
          },
          {
            path: 'skills',
            element: <SkillsPage />,
          },
          {
            path: 'agents',
            element: <AgentsPage />,
          },
          {
            path: 'oauth',
            element: <OAuthPage />,
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
