import { Suspense, lazy, useMemo } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Spin } from 'antd';
import { Tabs, TabList, Tab, TabPanel } from '../components/ui/tabs';
import { ParentLandmarkProvider } from '../hooks/useLandmarkNavigation';
import './SettingsPage.css';

const ProvidersPage = lazy(() => import('./ProvidersPage'));
const McpPage = lazy(() => import('./McpPage'));
const SkillsPage = lazy(() => import('./SkillsPage'));
const ChannelsPage = lazy(() => import('./ChannelsPage'));
const ContactsPage = lazy(() => import('./ContactsPage'));
const CredentialsPage = lazy(() => import('./CredentialsPage'));
const AllowlistPage = lazy(() => import('./AllowlistPage'));
const RestoreDefaultsPage = lazy(() => import('./RestoreDefaultsPage'));

const SETTINGS_TABS = [
  { id: 'providers',        component: ProvidersPage },
  { id: 'mcp',              component: McpPage },
  { id: 'skills',           component: SkillsPage },
  { id: 'channels',         component: ChannelsPage },
  { id: 'contacts',         component: ContactsPage },
  { id: 'credentials',      component: CredentialsPage },
  { id: 'allowlists',       component: AllowlistPage },
  { id: 'restore-defaults', component: RestoreDefaultsPage },
] as const;

const DEFAULT_TAB = SETTINGS_TABS[0].id;

type SettingsTabId = typeof SETTINGS_TABS[number]['id'];

const VALID_TAB_IDS = new Set<string>(SETTINGS_TABS.map((t) => t.id));

function resolveTab(param: string | undefined): SettingsTabId {
  if (param && VALID_TAB_IDS.has(param)) return param as SettingsTabId;
  return DEFAULT_TAB;
}

export default function SettingsPage() {
  const { tab } = useParams<{ tab?: string }>();
  const navigate = useNavigate();
  const { t } = useTranslation();

  const activeTab = useMemo(() => resolveTab(tab), [tab]);

  const handleTabChange = (tabId: string) => {
    navigate(`/settings/${tabId}`, { replace: true });
  };

  return (
    <div className="settings-page">
      <Tabs
        value={activeTab}
        onValueChange={handleTabChange}
        activationMode="auto"
        idBase="settings"
      >
        <TabList className="settings-tabs__list" ariaLabel={t('settingsPage.tabListLabel', 'Configurações')}>
          {SETTINGS_TABS.map(({ id }) => (
            <Tab
              key={id}
              value={id}
              className="settings-tabs__tab"
              activeClassName="settings-tabs__tab--active"
            >
              {t(`settingsPage.tabs.${id}`, id)}
            </Tab>
          ))}
        </TabList>

        {SETTINGS_TABS.map(({ id, component: Component }) => (
          <TabPanel key={id} value={id} className="settings-tabs__panel">
            <ParentLandmarkProvider value={true}>
              <Suspense
                fallback={
                  <div className="settings-tabs__loading" aria-busy="true">
                    <Spin size="large" />
                  </div>
                }
              >
                <Component />
              </Suspense>
            </ParentLandmarkProvider>
          </TabPanel>
        ))}
      </Tabs>
    </div>
  );
}
