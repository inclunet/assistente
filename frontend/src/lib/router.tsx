import { Suspense, lazy } from 'react';
import { createHashRouter } from 'react-router-dom';
import App from '../App';
import { WorkspaceLayout } from '../components/workspace';
import { PageLoading } from '../components/ui/PageLoading';

const SettingsPage = lazy(() => import('../pages/SettingsPage'));
const ProfilesPage = lazy(() => import('../pages/ProfilesPage'));
const HistoryPage = lazy(() => import('../pages/HistoryPage'));
const HelpPage = lazy(() => import('../pages/HelpPage'));
const AboutPage = lazy(() => import('../pages/AboutPage'));
const UpdatePage = lazy(() => import('../pages/UpdatePage'));
const TaskListsPage = lazy(() => import('../pages/TaskListsPage'));
const JobsPage = lazy(() => import('../pages/JobsPage'));
const MemoriesPage = lazy(() => import('../pages/MemoriesPage'));

const withSuspense = (element: JSX.Element) => (
  <Suspense fallback={<PageLoading />}>
    {element}
  </Suspense>
);

/** Rota index: o conteúdo vem de WorkspaceLayout + WorkspaceContent; este nó só satisfaz o router. */
function WorkspaceIndexRoute() {
  return null;
}

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
            element: <WorkspaceIndexRoute />,
          },
          {
            path: 'settings/:tab?',
            element: withSuspense(<SettingsPage />),
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
          {
            path: 'jobs',
            element: withSuspense(<JobsPage />),
          },
          {
            path: 'memories',
            element: withSuspense(<MemoriesPage />),
          },
        ],
      },
    ],
  },
]);
