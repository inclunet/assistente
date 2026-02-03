import { createHashRouter } from 'react-router-dom';
import App from '../App';
import { Layout } from '../components/layout/Layout';
import ChatPage from '../pages/ChatPage';
import SettingsPage from '../pages/SettingsPage';
import HistoryPage from '../pages/HistoryPage';
import FAQPage from '../pages/FAQPage';
import MemoryPage from '../pages/MemoryPage';
import AgentsPage from '../pages/AgentsPage';
import OAuthPage from '../pages/OAuthPage';
import VoiceProfilesPage from '../pages/VoiceProfilesPage';
import InteractionProfilesPage from '../pages/InteractionProfilesPage';
import ChatProfilesPage from '../pages/ChatProfilesPage';
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
            path: 'faq',
            element: <FAQPage />,
          },
          {
            path: 'memory',
            element: <MemoryPage />,
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
            path: 'voice-profiles',
            element: <VoiceProfilesPage />,
          },
          {
            path: 'interaction-profiles',
            element: <InteractionProfilesPage />,
          },
          {
            path: 'chat-profiles',
            element: <ChatProfilesPage />,
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
