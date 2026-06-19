import { useState, useEffect, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { profiles } from '@wailsjs/go/models';
import { controllers, allowlist, contextprovider, skills } from '@wailsjs/go/models';
import { Tabs, TabList, Tab, TabPanel } from '../ui/tabs';
import { ProfileGeneralSection } from './ProfileGeneralSection';
import { ProfileChatSection } from './ProfileChatSection';
import { ProfileSkillsSection } from './ProfileSkillsSection';
import { ProfileContextProvidersSection } from './ProfileContextProvidersSection';
import { ProfileToolsSection } from './ProfileToolsSection';
import { ProfileAudioTab } from './ProfileAudioTab';
import './ProfileEditorTabs.css';

const EDITOR_TABS = ['general', 'models', 'skills', 'contextProviders', 'tools', 'audio'] as const;
type EditorTabId = (typeof EDITOR_TABS)[number];

export interface ProfileEditorTabsProps {
  editingProfile: profiles.Profile & { id?: string; source?: string; isActive?: boolean };
  availableTools: controllers.ToolInfo[];
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

  const handleTabChange = useCallback((v: string) => {
    setActiveTab(v as EditorTabId);
  }, []);

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

      const currentIndex = EDITOR_TABS.indexOf(activeTab);
      let nextIndex = currentIndex + direction;
      if (nextIndex >= EDITOR_TABS.length) nextIndex = 0;
      if (nextIndex < 0) nextIndex = EDITOR_TABS.length - 1;

      const nextTabId = EDITOR_TABS[nextIndex];

      // Update React state first.
      handleTabChange(nextTabId);

      // Now sync DOM immediately before the event loop processes the keyup/focus fully.
      // This pattern ensures the SR sees the 'aria-selected' change as part of the focus movement.
      const prevBtn = el.querySelector('button[role="tab"][aria-selected="true"]') as HTMLElement | null;
      const nextBtn = el.querySelector(
        `button[role="tab"][data-tab-value="${nextTabId}"]`,
      ) as HTMLButtonElement | null;

      if (prevBtn) {
        prevBtn.setAttribute('aria-selected', 'false');
        prevBtn.tabIndex = -1;
      }
      if (nextBtn) {
        nextBtn.setAttribute('aria-selected', 'true');
        nextBtn.tabIndex = 0;
        nextBtn.focus();
      }
    };

    el.addEventListener('keydown', handleKeyDown);
    return () => el.removeEventListener('keydown', handleKeyDown);
  }, [activeTab, handleTabChange]);

  return (
    <div ref={containerRef} className="profile-editor-tabs" data-tab-scope>
      <Tabs
        value={activeTab}
        onValueChange={handleTabChange}
        activationMode="auto"
        idBase="profile-editor"
      >
        <TabList className="profile-editor-tabs__list" ariaLabel={t('profiles.editorTabsLabel', 'Seções do perfil')}>
          {EDITOR_TABS.map((id) => (
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
            reasoningEffort={editingProfile.chat?.reasoning_effort || ''}
            streamingRecoveryEnabled={editingProfile.chat?.streaming_recovery_enabled ?? true}
            streamingRecoveryMaxAttempts={editingProfile.chat?.streaming_recovery_max_attempts ?? 3}
            streamingRecoveryShowContinue={editingProfile.chat?.streaming_recovery_show_continue ?? true}
            onChange={(field, value) => updateField(`chat.${field}`, value)}
            onMultiChange={(updates) => {
              const prefixedUpdates = Object.fromEntries(
                Object.entries(updates).map(([k, v]) => [`chat.${k}`, v])
              );
              updateFields(prefixedUpdates);
            }}
          />
        </TabPanel>

        {/* Skills */}
        <TabPanel value="skills" className="profile-editor-tabs__panel">
          <ProfileSkillsSection
            availableSkills={availableSkills}
            enabledSkills={editingProfile.chat?.enabled_skills ?? undefined}
            disableOnDemand={editingProfile.chat?.disable_on_demand_skills ?? false}
            skillsDisabled={editingProfile.chat?.disable_skills ?? false}
            onChange={(field, value) => updateField(`chat.${field}`, value)}
          />
        </TabPanel>

        {/* Context Providers */}
        <TabPanel value="contextProviders" className="profile-editor-tabs__panel">
          <ProfileContextProvidersSection
            providers={availableContextProviders}
            value={editingProfile.context_providers ?? undefined}
            onChange={(value) => updateField('context_providers', value)}
          />
        </TabPanel>

        {/* Ferramentas & MCP */}
        <TabPanel value="tools" className="profile-editor-tabs__panel">
          <ProfileToolsSection
            availableTools={availableTools}
            enabledTools={editingProfile.chat?.enabled_tools ?? null}
            toolsDisabled={editingProfile.chat?.disable_tools ?? false}
            commandAllowlist={editingProfile.chat?.command_allowlist || ''}
            availableAllowlists={availableAllowlists}
            maxAgenticIterations={editingProfile.chat?.max_agentic_iterations ?? 0}
            responseTimeout={editingProfile.chat?.response_timeout ?? 180}
            nativeMcp={editingProfile.chat?.native_mcp ?? null}
            onChange={(field, value) => updateField(`chat.${field}`, value)}
          />
        </TabPanel>

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
