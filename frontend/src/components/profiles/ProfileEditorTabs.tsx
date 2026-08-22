import { useState, useEffect, useCallback, useLayoutEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import type { profiles, apidto, allowlist, contextprovider, skills } from '@wailsjs/go/models';
import { Tabs, TabList, Tab, TabPanel } from '../ui/tabs';
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';
import { announce } from '../../hooks/useAnnouncer';
import { useAgentProvider } from '../../hooks/useAgentProvider';
import { ProfileGeneralSection } from './ProfileGeneralSection';
import { ProfileChatSection } from './ProfileChatSection';
import { ProfileSkillsSection } from './ProfileSkillsSection';
import { ProfileContextProvidersSection } from './ProfileContextProvidersSection';
import { ProfileToolsSection } from './ProfileToolsSection';
import { ProfileAudioTab } from './ProfileAudioTab';
import './ProfileEditorTabs.css';

const EDITOR_TABS = ['general', 'models', 'skills', 'contextProviders', 'tools', 'audio'] as const;
type EditorTabId = (typeof EDITOR_TABS)[number];

/**
 * AGENT_HIDDEN_TABS são as guias que um perfil com agente de código não usa: o
 * turno passa por cima delas, e o agente tem recurso próprio para o que elas
 * configuram (AEP-0084, Fase 8). Elas somem da tela em vez de aparecerem
 * desabilitadas — as duas formas informam o mesmo, mas o formulário
 * desabilitado ainda pede atenção de quem navega guia por guia.
 *
 * Esconder não apaga: o que está no perfil continua lá, e volta inteiro se o
 * provedor voltar a ser HTTP.
 */
const AGENT_HIDDEN_TABS: readonly EditorTabId[] = ['skills', 'contextProviders', 'tools'];
const FOCUSABLE_SELECTOR =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';
const PRIMARY_FOCUSABLE_SELECTOR =
  'button[aria-haspopup="listbox"]:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [contenteditable]';

function isVisibleFocusTarget(element: HTMLElement): boolean {
  return !element.closest('[hidden], [aria-hidden="true"]');
}

export interface ProfileEditorTabsProps {
  editingProfile: profiles.Profile & { id?: string; source?: string; isActive?: boolean };
  availableTools: apidto.ToolInfo[];
  availableSkills: Array<
    | skills.SkillInfo
    | { slug: string; name: string; description?: string; version?: string; source?: string }
  >;
  availableContextProviders: contextprovider.ProviderMetadata[];
  availableAllowlists: allowlist.AllowlistInfo[];
  updateField: (path: string, value: unknown) => void;
  updateFields: (updates: Record<string, unknown>) => void;
}

export function ProfileEditorTabs({
  editingProfile,
  availableTools,
  availableSkills,
  availableContextProviders,
  availableAllowlists,
  updateField,
  updateFields,
}: ProfileEditorTabsProps) {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState<EditorTabId>('general');
  const containerRef = useRef<HTMLDivElement>(null);
  const pendingShortcutFocusRef = useRef<EditorTabId | null>(null);
  const { isAgent: agentProvider, resolved: agentProviderResolved } = useAgentProvider(
    editingProfile.chat?.llm_provider || '',
  );
  const visibleTabs = agentProvider
    ? EDITOR_TABS.filter((id) => !AGENT_HIDDEN_TABS.includes(id))
    : EDITOR_TABS;
  const enabledSkills = editingProfile.chat?.enabled_skills;
  const hasOnDemandSkills = !(editingProfile.chat?.disable_skills ?? false)
    && !(editingProfile.chat?.disable_on_demand_skills ?? false)
    && (enabledSkills == null || enabledSkills.length > 1);

  const handleTabChange = useCallback((v: string) => {
    pendingShortcutFocusRef.current = null;
    setActiveTab(v as EditorTabId);
  }, []);

  const focusProfileContent = useCallback(
    (tabId: EditorTabId = activeTab): boolean => {
      const el = containerRef.current;
      if (!el) return false;

      const targetPanel = document.getElementById(`profile-editor-tabpanel-${tabId}`) as HTMLElement | null;
      const panel =
        targetPanel && !targetPanel.hidden
          ? targetPanel
          : (el.querySelector('[role="tabpanel"]:not([hidden])') as HTMLElement | null);
      if (!panel) return false;

      const grid = panel.querySelector('[role="grid"]') as HTMLElement | null;
      if (grid && isVisibleFocusTarget(grid)) {
        const cell = panel.querySelector(
          '.datagrid-container [role="gridcell"][tabindex="0"], .datagrid-container [role="gridcell"]',
        ) as HTMLElement | null;
        if (cell && isVisibleFocusTarget(cell)) {
          cell.focus();
          return true;
        }
        grid.focus();
        return true;
      }

      const primary = Array.from(panel.querySelectorAll(PRIMARY_FOCUSABLE_SELECTOR)).find((candidate) =>
        isVisibleFocusTarget(candidate as HTMLElement),
      ) as HTMLElement | undefined;
      if (primary) {
        primary.focus();
        return true;
      }

      const focusable = Array.from(panel.querySelectorAll(FOCUSABLE_SELECTOR)).find(
        (candidate) =>
          candidate.getAttribute('aria-expanded') === null &&
          isVisibleFocusTarget(candidate as HTMLElement),
      ) as HTMLElement | undefined;
      if (focusable) {
        focusable.focus();
        return true;
      }

      const collapsibleToggle = Array.from(panel.querySelectorAll(FOCUSABLE_SELECTOR)).find((candidate) =>
        isVisibleFocusTarget(candidate as HTMLElement),
      ) as HTMLElement | undefined;
      if (collapsibleToggle) {
        collapsibleToggle.focus();
        return true;
      }

      panel.setAttribute('tabindex', '-1');
      panel.focus();
      return true;
    },
    [activeTab],
  );

  useLayoutEffect(() => {
    if (pendingShortcutFocusRef.current !== activeTab) return;
    pendingShortcutFocusRef.current = null;

    if (!focusProfileContent(activeTab)) {
      restoreDefaultFocus();
    }
  }, [activeTab, focusProfileContent]);

  // A troca de provedor muda a contagem de guias debaixo de quem navega. Quem
  // está numa guia que some precisa ir para lugar previsível, e ouvir o que
  // passou a valer (AEP-0084, Fase 8).
  //
  // Corre antes da pintura: a guia ativa que some é trocada no mesmo quadro, sem
  // um instante de editor sem guia selecionada.
  //
  // Descobrir que o perfil já era de um agente não anuncia nada: aí ninguém
  // trocou de provedor, o editor está abrindo como o perfil já era. O anúncio é
  // para quem trocou, e só existe troca depois que a consulta respondeu uma vez.
  const agentProviderRef = useRef<boolean | null>(null);
  useLayoutEffect(() => {
    if (!agentProviderResolved) return;
    const anterior = agentProviderRef.current;
    agentProviderRef.current = agentProvider;

    if (agentProvider && AGENT_HIDDEN_TABS.includes(activeTab)) {
      setActiveTab('models');
    }
    if (anterior === null || anterior === agentProvider) return;
    announce(
      agentProvider
        ? t('profiles.agentProfile.tabsHidden')
        : t('profiles.agentProfile.tabsBack'),
    );
  }, [agentProvider, agentProviderResolved, activeTab, t]);

  // Ctrl+Tab / Ctrl+Shift+Tab / Ctrl+PageDown / Ctrl+PageUp
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

      const currentIndex = visibleTabs.indexOf(activeTab);
      let nextIndex = currentIndex + direction;
      if (nextIndex >= visibleTabs.length) nextIndex = 0;
      if (nextIndex < 0) nextIndex = visibleTabs.length - 1;

      const nextTabId = visibleTabs[nextIndex];

      // Update React state first.
      pendingShortcutFocusRef.current = nextTabId;
      setActiveTab(nextTabId);
      announce(t(`profiles.editorTabs.${nextTabId}`));
    };

    el.addEventListener('keydown', handleKeyDown);
    return () => el.removeEventListener('keydown', handleKeyDown);
  }, [activeTab, t, visibleTabs]);

  return (
    <div ref={containerRef} className="profile-editor-tabs" data-tab-scope>
      <Tabs
        value={activeTab}
        onValueChange={handleTabChange}
        activationMode="auto"
        idBase="profile-editor"
        onActivate={() => {
          return focusProfileContent();
        }}
      >
        <TabList className="profile-editor-tabs__list" ariaLabel={t('profiles.editorTabsLabel', 'Seções do perfil')}>
          {visibleTabs.map((id) => (
            <Tab
              key={id}
              value={id}
              className="profile-editor-tabs__tab"
              activeClassName="profile-editor-tabs__tab--active"
            >
              {t(`profiles.editorTabs.${id}`)}
            </Tab>
          ))}
        </TabList>

        {/* Geral */}
        <TabPanel value="general" className="profile-editor-tabs__panel">
          <ProfileGeneralSection
            name={editingProfile.name}
            description={editingProfile.description || ''}
            icon={editingProfile.icon || ''}
            onChange={(field, value) => updateField(field, value)}
          />
        </TabPanel>

        {/* Modelos */}
        <TabPanel value="models" className="profile-editor-tabs__panel">
          <ProfileChatSection
            llmProvider={editingProfile.chat?.llm_provider || ''}
            model={editingProfile.chat?.model || ''}
            temperature={editingProfile.chat?.temperature ?? 0.7}
            maxTokens={editingProfile.chat?.max_tokens ?? 4096}
            maxTokensMode={editingProfile.chat?.max_tokens_mode || 'legacy'}
            contextWindow={editingProfile.chat?.context_window ?? 0}
            maxContextMessages={editingProfile.chat?.max_context_messages ?? 0}
            minContextMessages={editingProfile.chat?.min_context_messages ?? 0}
            topP={editingProfile.chat?.top_p ?? 1.0}
            responseTimeout={editingProfile.chat?.response_timeout ?? 180}
            rateLimitEnabled={editingProfile.chat?.rate_limit_enabled ?? true}
            rateLimitRpm={editingProfile.chat?.rate_limit_rpm ?? 60}
            rateLimitBurst={editingProfile.chat?.rate_limit_burst ?? 30}
            reasoningEffort={editingProfile.chat?.reasoning_effort || ''}
            promptCache={editingProfile.chat?.prompt_cache}
            debug={editingProfile.chat?.debug}
            streamingRecoveryEnabled={editingProfile.chat?.streaming_recovery_enabled ?? true}
            streamingRecoveryMaxAttempts={editingProfile.chat?.streaming_recovery_max_attempts ?? 3}
            streamingRecoveryShowContinue={editingProfile.chat?.streaming_recovery_show_continue ?? true}
            agentProvider={agentProvider}
            onChange={(field, value) => updateField(`chat.${field}`, value)}
            onMultiChange={(updates) => {
              const prefixedUpdates = Object.fromEntries(
                Object.entries(updates).map(([k, v]) => [`chat.${k}`, v])
              );
              updateFields(prefixedUpdates);
            }}
          />
        </TabPanel>

        {/* Skills, Provedores de contexto e Ferramentas: só num perfil sem
            agente. Com agente, o turno passa por cima deles (D7, D14) e o
            editor não os mostra (Fase 8). */}
        {!agentProvider && (
          <TabPanel value="skills" className="profile-editor-tabs__panel">
            <ProfileSkillsSection
              availableSkills={availableSkills}
              enabledSkills={editingProfile.chat?.enabled_skills ?? undefined}
              disableOnDemand={editingProfile.chat?.disable_on_demand_skills ?? false}
              skillsDisabled={editingProfile.chat?.disable_skills ?? false}
              onChange={(field, value) => updateField(`chat.${field}`, value)}
            />
          </TabPanel>
        )}

        {!agentProvider && (
          <TabPanel value="contextProviders" className="profile-editor-tabs__panel">
            <ProfileContextProvidersSection
              providers={availableContextProviders}
              value={editingProfile.context_providers ?? undefined}
              onChange={(value) => updateField('context_providers', value)}
            />
          </TabPanel>
        )}

        {!agentProvider && (
          <TabPanel value="tools" className="profile-editor-tabs__panel">
            <ProfileToolsSection
              availableTools={availableTools}
              enabledTools={editingProfile.chat?.enabled_tools ?? null}
              toolPolicy={editingProfile.chat?.tool_policy ?? null}
              toolPolicyDefault={editingProfile.chat?.tool_policy_default ?? null}
              runtimeTools={hasOnDemandSkills ? ['load_skill'] : []}
              toolsDisabled={editingProfile.chat?.disable_tools ?? false}
              commandAllowlist={editingProfile.chat?.command_allowlist || ''}
              availableAllowlists={availableAllowlists}
              maxAgenticIterations={editingProfile.chat?.max_agentic_iterations ?? 0}
              responseTimeout={editingProfile.chat?.response_timeout ?? 180}
              nativeMcp={editingProfile.chat?.native_mcp ?? null}
              onChange={(field, value) => updateField(`chat.${field}`, value)}
              onPolicyChange={(policy) => updateFields({
                'chat.tool_policy': policy,
                'chat.enabled_tools': null,
              })}
            />
          </TabPanel>
        )}

        {/* Áudio */}
        <TabPanel value="audio" className="profile-editor-tabs__panel">
          <ProfileAudioTab
            editingProfile={editingProfile}
            updateField={updateField}
            updateFields={updateFields}
            profileId={editingProfile.id || ''}
          />
        </TabPanel>
      </Tabs>
    </div>
  );
}
