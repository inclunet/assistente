import { createHashRouter } from 'react-router-dom';
import App from '../App';
import { Layout } from '../components/layout/Layout';
import ChatPage from '../pages/ChatPage';
import SettingsPage from '../pages/SettingsPage';
import ProfilesPage from '../pages/ProfilesPage';
import HistoryPage from '../pages/HistoryPage';
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
