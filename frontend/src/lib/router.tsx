import { createBrowserRouter } from 'react-router-dom';
import App from '../App';
import { Layout } from '../components/layout/Layout';
import ChatPage from '../pages/ChatPage';
import SettingsPage from '../pages/SettingsPage';
import HistoryPage from '../pages/HistoryPage';
import FAQPage from '../pages/FAQPage';
import MemoryPage from '../pages/MemoryPage';

export const router = createBrowserRouter([
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
        ],
      },
    ],
  },
]);
