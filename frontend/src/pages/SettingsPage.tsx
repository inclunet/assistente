import { Suspense, lazy, useCallback, useEffect, useLayoutEffect, useMemo, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Spin } from 'antd';
import { Tabs, TabList, Tab, TabPanel } from '../components/ui/tabs';
import { ParentLandmarkProvider } from '../hooks/useLandmarkNavigation';
import { announce } from '../hooks/useAnnouncer';
import './SettingsPage.css';

const ProvidersPage = lazy(() => import('./ProvidersPage'));
const McpPage = lazy(() => import('./McpPage'));
const SkillsPage = lazy(() => import('./SkillsPage'));
const ChannelsPage = lazy(() => import('./ChannelsPage'));
const ContactsPage = lazy(() => import('./ContactsPage'));
const CredentialsPage = lazy(() => import('./CredentialsPage'));
const AllowlistPage = lazy(() => import('./AllowlistPage'));
const NetworkAllowlistPage = lazy(() => import('./NetworkAllowlistPage'));
const PathAllowlistPage = lazy(() => import('./PathAllowlistPage'));
const AgentPermissionsPage = lazy(() => import('./AgentPermissionsPage'));
const AppearancePage = lazy(() => import('./AppearancePage'));
const DataManagementPage = lazy(() => import('./DataManagementPage'));
const RestoreDefaultsPage = lazy(() => import('./RestoreDefaultsPage'));

const SETTINGS_TABS = [
  { id: 'providers',        component: ProvidersPage },
  { id: 'mcp',              component: McpPage },
  { id: 'skills',           component: SkillsPage },
  { id: 'channels',         component: ChannelsPage },
  { id: 'contacts',         component: ContactsPage },
  { id: 'credentials',      component: CredentialsPage },
  { id: 'allowlists',       component: AllowlistPage },
  { id: 'network-allowlist', component: NetworkAllowlistPage },
  { id: 'path-allowlist',   component: PathAllowlistPage },
  { id: 'agent-permissions', component: AgentPermissionsPage },
  { id: 'appearance',       component: AppearancePage },
  { id: 'data',             component: DataManagementPage },
  { id: 'restore-defaults', component: RestoreDefaultsPage },
] as const;

const DEFAULT_TAB = SETTINGS_TABS[0].id;

type SettingsTabId = typeof SETTINGS_TABS[number]['id'];

const VALID_TAB_IDS = new Set<string>(SETTINGS_TABS.map((t) => t.id));
const FOCUSABLE_SELECTOR =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function resolveTab(param: string | undefined): SettingsTabId {
  if (param && VALID_TAB_IDS.has(param)) return param as SettingsTabId;
  return DEFAULT_TAB;
}

export default function SettingsPage() {
  const { tab } = useParams<{ tab?: string }>();
  const navigate = useNavigate();
  const { t } = useTranslation();
  const containerRef = useRef<HTMLDivElement>(null);
  const pendingShortcutFocusRef = useRef<SettingsTabId | null>(null);
  const pendingShortcutFocusTimerRef = useRef<number | null>(null);
  const cleanupPendingContentFocusRef = useRef<(() => void) | null>(null);
  const activeTabRef = useRef<SettingsTabId>(DEFAULT_TAB);
  const focusRequestIdRef = useRef(0);

  const activeTab = useMemo(() => resolveTab(tab), [tab]);
  activeTabRef.current = activeTab;

  const clearPendingShortcutFocus = useCallback(() => {
    pendingShortcutFocusRef.current = null;
    if (pendingShortcutFocusTimerRef.current !== null) {
      window.clearTimeout(pendingShortcutFocusTimerRef.current);
      pendingShortcutFocusTimerRef.current = null;
    }
  }, []);

  const setPendingShortcutFocus = useCallback(
    (tabId: SettingsTabId) => {
      clearPendingShortcutFocus();
      cleanupPendingContentFocusRef.current?.();
      cleanupPendingContentFocusRef.current = null;
      pendingShortcutFocusRef.current = tabId;
      pendingShortcutFocusTimerRef.current = window.setTimeout(() => {
        clearPendingShortcutFocus();
      }, 2000);
    },
    [clearPendingShortcutFocus],
  );

  const handleTabChange = useCallback(
    (tabId: string) => {
      clearPendingShortcutFocus();
      cleanupPendingContentFocusRef.current?.();
      cleanupPendingContentFocusRef.current = null;
      navigate(`/settings/${tabId}`, { replace: true });
    },
    [clearPendingShortcutFocus, navigate],
  );

  const focusSettingsContent = useCallback(
    (tabId: SettingsTabId = activeTab, allowPanelFallback = true): boolean => {
      const el = containerRef.current;
      if (!el) return false;

      const targetPanel = document.getElementById(`settings-tabpanel-${tabId}`) as HTMLElement | null;
      const panel =
        targetPanel && !targetPanel.hidden
          ? targetPanel
          : (el.querySelector('[role="tabpanel"]:not([hidden])') as HTMLElement | null);
      if (!panel) return false;

      const grid = panel.querySelector('[role="grid"]') as HTMLElement | null;
      if (grid) {
        const cell = panel.querySelector(
          '.datagrid-container [role="gridcell"][tabindex="0"], .datagrid-container [role="gridcell"]',
        ) as HTMLElement | null;
        if (cell) {
          cell.focus();
          return true;
        }
        grid.focus();
        return true;
      }

      const toolbar = panel.querySelector('[role="toolbar"]');
      const focusable = Array.from(panel.querySelectorAll(FOCUSABLE_SELECTOR)).find(
        (candidate) => !toolbar?.contains(candidate),
      ) as HTMLElement | undefined;
      if (focusable) {
        focusable.focus();
        return true;
      }

      if (!allowPanelFallback) return false;

      panel.setAttribute('tabindex', '-1');
      panel.focus();
      return true;
    },
    [activeTab],
  );

  const focusSettingsContentWhenReady = useCallback(
    (tabId: SettingsTabId = activeTab) => {
      cleanupPendingContentFocusRef.current?.();
      cleanupPendingContentFocusRef.current = null;
      const requestId = focusRequestIdRef.current + 1;
      focusRequestIdRef.current = requestId;

      const isCurrentRequest = () =>
        focusRequestIdRef.current === requestId && activeTabRef.current === tabId;

      const cleanupHandles: Array<() => void> = [];
      const cleanup = () => {
        for (const dispose of cleanupHandles.splice(0)) dispose();
        if (focusRequestIdRef.current === requestId) {
          focusRequestIdRef.current += 1;
        }
        if (cleanupPendingContentFocusRef.current === cleanup) {
          cleanupPendingContentFocusRef.current = null;
        }
      };
      cleanupPendingContentFocusRef.current = cleanup;

      const tryFocusContent = () => {
        if (focusSettingsContent(tabId, false)) {
          cleanup();
          return true;
        }
        return false;
      };

      if (tryFocusContent()) return;

      // Keep focus out of the panel that just became hidden while lazy content mounts.
      focusSettingsContent(tabId, true);

      requestAnimationFrame(() => {
        if (!isCurrentRequest()) {
          cleanup();
          return;
        }
        if (tryFocusContent()) return;

        const panel = document.getElementById(`settings-tabpanel-${tabId}`);
        const target = panel || containerRef.current;
        if (!target) {
          cleanup();
          return;
        }

        const observer = new MutationObserver(() => {
          if (!isCurrentRequest()) {
            cleanup();
            return;
          }
          if (tryFocusContent()) return;
          if (panel && document.activeElement !== panel) {
            cleanup();
          }
        });
        observer.observe(target, {
          childList: true,
          subtree: true,
          attributes: true,
          attributeFilter: ['hidden', 'aria-hidden', 'tabindex'],
        });
        cleanupHandles.push(() => observer.disconnect());

        const timeoutId = window.setTimeout(() => {
          cleanup();
        }, 2000);
        cleanupHandles.push(() => window.clearTimeout(timeoutId));
      });
    },
    [activeTab, focusSettingsContent],
  );

  useEffect(
    () => () => {
      clearPendingShortcutFocus();
      cleanupPendingContentFocusRef.current?.();
    },
    [clearPendingShortcutFocus],
  );

  useLayoutEffect(() => {
    if (!pendingShortcutFocusRef.current) return;
    if (pendingShortcutFocusRef.current !== activeTab) return;
    clearPendingShortcutFocus();

    focusSettingsContentWhenReady(activeTab);
  }, [activeTab, clearPendingShortcutFocus, focusSettingsContentWhenReady]);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (!e.ctrlKey) return;

      let direction: 1 | -1 | null = null;
      if (e.key === 'Tab' && !e.shiftKey) direction = 1;
      else if (e.key === 'Tab' && e.shiftKey) direction = -1;
      else if (e.key === 'PageDown') direction = 1;
      else if (e.key === 'PageUp') direction = -1;
      if (direction === null) return;

      e.preventDefault();
      e.stopPropagation();

      const currentIndex = SETTINGS_TABS.findIndex((s) => s.id === activeTab);
      let nextIndex = currentIndex + direction;
      if (nextIndex >= SETTINGS_TABS.length) nextIndex = 0;
      if (nextIndex < 0) nextIndex = SETTINGS_TABS.length - 1;

      const nextTabId = SETTINGS_TABS[nextIndex].id;
      setPendingShortcutFocus(nextTabId);
      navigate(`/settings/${nextTabId}`, { replace: true });

      const label = t(`settingsPage.tabs.${nextTabId}`, nextTabId);
      announce(label);
    };

    el.addEventListener('keydown', handleKeyDown);
    return () => el.removeEventListener('keydown', handleKeyDown);
  }, [activeTab, navigate, setPendingShortcutFocus, t]);

  return (
    <div ref={containerRef} className="settings-page" data-tab-scope>
      <Tabs
        value={activeTab}
        onValueChange={handleTabChange}
        activationMode="auto"
        idBase="settings"
        onActivate={() => {
          const focused = focusSettingsContent(activeTab, true);
          focusSettingsContentWhenReady(activeTab);
          return focused;
        }}
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
