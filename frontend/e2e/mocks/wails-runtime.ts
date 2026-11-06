/**
 * Mock do runtime Wails injetado no browser via page.addInitScript().
 *
 * Simula window.go.main.App.* e window.runtime.* para que o frontend
 * funcione sem o backend Go real.
 *
 * Cada teste pode sobrescrever respostas via page.evaluate():
 *   await page.evaluate(() => {
 *     window.__wailsMock.setResponse('SendMessage', '01926b90-0000-7000-8000-000000000099');
 *   });
 */

interface MockConfig {
  responses: Record<string, unknown>;
  eventListeners: Map<string, Array<(data: unknown) => void>>;
}

declare global {
  interface Window {
    __wailsMock: {
      setTimelineRows: (rows: unknown[], parentId?: string, conversationId?: string) => void;
      setResponse: (fn: string, value: unknown) => void;
      clearResponse: (fn: string) => void;
      setError: (fn: string, message: string) => void;
      clearError: (fn: string) => void;
      emit: (event: string, data?: unknown) => void;
      getCallLog: () => Array<{ fn: string; scope?: string; args: unknown[] }>;
      reset: () => void;
    };
    go: Record<string, Record<string, Record<string, (...args: unknown[]) => Promise<unknown>>>>;
    runtime: Record<string, (...args: unknown[]) => unknown>;
  }
}

export function buildWailsMockScript(): string {
  return `
(function() {
  const _config = {
    responses: {},
    errors: {},
    eventListeners: new Map(),
    callLog: [],
  };

  /* ---------- dados padrão ---------- */

  const now = new Date().toISOString();

  const defaultConversation = {
    id: '01926b90-0000-7000-8000-000000000001',
    title: 'Nova conversa',
    created_at: now,
    updated_at: now,
    messages: [],
    message_count: 0,
  };

  const defaultWorkspace = {
    id: 'ws-1',
    name: 'Workspace',
    snapshot_epoch: 'e2e-workspace-epoch',
    snapshot_sequence: '1',
    profile: '',
    created_at: now,
    last_used: now,
    tabs: {
      active: 'tab-1',
      items: [
        {
          id: 'tab-1',
          type: 'chat',
          conversation_id: '01926b90-0000-7000-8000-000000000001',
          title: 'Nova conversa',
          position: 0,
        },
      ],
    },
  };

  const defaultProfile = {
    slug: 'default',
    name: 'Default',
    description: 'Default profile',
    system_prompt: '',
    tts: {},
    stt: {},
  };

  const defaultAuthUser = {
    userId: 'user-e2e',
    sessionId: 'session-e2e',
    role: 'admin',
  };

  // Explicit E2E command map; unknown commands are never implicitly admitted.
  const defaultLocalCommandKeyboardMap = {
    generation: 'e2e-local-command-map-v1',
    ownerId: defaultAuthUser.userId,
    sessionId: defaultAuthUser.sessionId,
    workspaceId: defaultWorkspace.id,
    bindings: [
      { shortcut: { version: 1, code: 'KeyM', modifiers: ['Alt'] }, commandId: 'navigation.menu.open', handler: 'local_ui' },
      { shortcut: { version: 1, code: 'KeyK', modifiers: ['Control'] }, commandId: 'navigation.palette.open', handler: 'local_ui' },
    ],
    // Explicit projection of the product's finite LOCAL_UI_COMMAND_IDS set.
    // This admits only local_ui commands; route/focus/context guards remain
    // responsible for deciding whether a presentation is available now.
    localPaletteCommands: [
      'command_settings.create.open',
      'providers.create.open', 'providers.api.create.open', 'providers.acp.create.open', 'providers.chatgpt.create.open',
      'tasklists.create.open',
      'tasklists.edit.open',
      'tasklists.search.focus',
      'tasklist.task.create.open',
      'profiles.create.open',
      'profiles.edit.open',
      'profiles.search.focus',
      'terminal.sessions.open',
      'terminal.focus.input',
      'terminal.focus.history',
      'chat.focus.input',
      'chat.focus.messages',
      'chat.message.read.open',
      'chat.message.menu.open',
      'chat.message.reasoning.toggle',
      'chat.message.thread.expand',
      'chat.message.thread.collapse',
      'navigation.landmark.next',
      'navigation.landmark.previous',
      'navigation.landmark.default',
      'editor.mermaid.open',
      'chat.message.edit.open',
      'editor.menu.file.open',
      'editor.menu.format.open',
      'editor.menu.insert.open',
      'editor.menu.mode.open',
      'editor.slides.open',
      'editor.presentation.fullscreen',
      'editor.table.cell.next',
      'editor.table.cell.previous',
      'chat.model.open',
      'chat.history.open',
      'chat.profile.open',
      'chat.pinned.open',
      'chat.tokens.open',
      'workspace.tab.next',
      'workspace.tab.previous',
      'workspace.tab.first',
      'workspace.tab.second',
      'workspace.tab.third',
      'workspace.tab.fourth',
      'workspace.tab.fifth',
      'workspace.tab.sixth',
      'workspace.tab.seventh',
      'workspace.tab.eighth',
      'workspace.tab.ninth',
      'workspace.tab.go_to',
      'navigation.workspace.open',
      'navigation.history.open',
      'navigation.memories.open',
      'navigation.tasklists.open',
      'navigation.jobs.open',
      'navigation.profiles.open',
      'navigation.settings.open',
      'navigation.palette.open',
      'navigation.data.export.open',
      'navigation.data.import.open',
      'navigation.help.open',
      'navigation.about.open',
      'navigation.menu.open',
      'help.shortcuts.show',
      'workspace.panel.focus',
    ],
  };

  const defaultGlobalCommandOwnership = {
    version: 1,
    platform: 'windows',
    instanceId: 'e2e00000-0000-4000-8000-000000000001',
    revision: 1,
    combinations: [],
  };
  let defaultGlobalOwnershipAckPending = true;

  // Narrow command-ledger fixture for the real chat submit pipeline used by
  // browser E2E. A command must be reserved, taken once, and then submitted
  // through SendMessage/RetryMessage with its matching handoff.
  const chatSubmissionCommandIDs = new Set(['chat.message.send', 'chat.message.retry']);
  const chatCommandInvocations = new Map();
  let nextChatCommandSequence = 1;

  function beginChatCommand(commandID) {
    if (!chatSubmissionCommandIDs.has(commandID)) return undefined;
    const sequence = nextChatCommandSequence++;
    const ticket = 'e2e-chat-ticket-' + sequence;
    const invocationId = 'e2e-chat-invocation-' + sequence;
    const invocation = { ticket, invocationId, commandId: commandID, handoffId: '', status: 'pending' };
    chatCommandInvocations.set(ticket, invocation);
    return { ticket, invocationId, commandId: commandID };
  }

  function takeChatCommand(ticket) {
    const invocation = chatCommandInvocations.get(ticket);
    if (!invocation) return undefined;
    if (invocation.status !== 'pending') throw new Error('e2e-command-not-takeable');
    invocation.status = 'taken';
    invocation.handoffId = 'e2e-chat-handoff-' + ticket.slice('e2e-chat-ticket-'.length);
    return { ticket, invocationId: invocation.invocationId, commandId: invocation.commandId, handoffId: invocation.handoffId };
  }

  function completeChatCommand(ticket, handoffId, status) {
    const invocation = chatCommandInvocations.get(ticket);
    if (!invocation) return undefined;
    if (invocation.status !== 'taken' || invocation.handoffId !== handoffId ||
        !['succeeded', 'failed', 'cancelled'].includes(status)) throw new Error('e2e-command-completion-mismatch');
    invocation.status = status;
  }

  function acceptChatSubmission(fnName, args) {
    const expectedCommandID = fnName === 'SendMessage' ? 'chat.message.send' : 'chat.message.retry';
    const params = args[fnName === 'SendMessage' ? 3 : 2];
    const proof = params && typeof params === 'object' ? params.command : undefined;
    const invocation = proof && typeof proof === 'object' ? chatCommandInvocations.get(proof.ticket) : undefined;
    if (!invocation || invocation.commandId !== expectedCommandID || invocation.status !== 'taken' ||
        invocation.handoffId !== proof.handoffId) throw new Error('e2e-chat-handoff-invalid');
    // Consume synchronously before the mocked RPC callback can yield. A second
    // submission with the same proof is rejected even while the first is pending.
    invocation.status = 'submitting';
    return invocation;
  }

  const DEFAULT_CONVERSATION_PAGE_LIMIT = 100;

  function normalizeConversationPageLimit(value) {
    const limit = Number(value);
    if (!Number.isFinite(limit) || limit <= 0) return DEFAULT_CONVERSATION_PAGE_LIMIT;
    return Math.min(limit, 500);
  }

  function normalizeConversationPageOffset(value) {
    const offset = Number(value);
    if (!Number.isFinite(offset) || offset < 0) return 0;
    return offset;
  }

  const defaults = {
    /* App init */
    NeedsWelcomeWizard: false,
    RunWelcomeWizard: true,
    GetAppVersion: '1.0.0-test',
    BeginUICommand: beginChatCommand,
    TakeUICommand: takeChatCommand,
    CompleteUICommand: completeChatCommand,
    GetUICommandResult: function(ticket) {
      const invocation = chatCommandInvocations.get(ticket);
      if (!invocation) return undefined;
      return { invocationId: invocation.invocationId, status: invocation.status };
    },
    CancelUICommand: function(ticket) {
      const invocation = chatCommandInvocations.get(ticket);
      if (!invocation || invocation.status !== 'pending') return undefined;
      invocation.status = 'cancelled';
    },
    GetLocalCommandKeyboardMap: defaultLocalCommandKeyboardMap,
    GetGlobalCommandOwnership: function() {
      return { ...defaultGlobalCommandOwnership, combinations: [...defaultGlobalCommandOwnership.combinations] };
    },
    AckGlobalCommandOwnership: function(instanceId, revision) {
      if (!defaultGlobalOwnershipAckPending || instanceId !== defaultGlobalCommandOwnership.instanceId ||
          revision !== defaultGlobalCommandOwnership.revision) return false;
      defaultGlobalOwnershipAckPending = false;
      return true;
    },
    // Valida scope: defaults resolvem só por fnName; sem isso App.IsGlobalHotkeySupported
    // mascararia regressão pós-migração (AEP-0088).
    IsGlobalHotkeySupported: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Hotkeys') {
        throw new Error('IsGlobalHotkeySupported deve ser chamado via wailsapi.Hotkeys');
      }
      return true;
    },
    GetAgentSessionCommands: function(conversationId) {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.ACPCommands') {
        throw new Error('GetAgentSessionCommands deve ser chamado via wailsapi.ACPCommands');
      }
      return { conversationId: conversationId || '', commands: [] };
    },
    GetAgentSessionOptions: function(conversationId) {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.ACPOptions') {
        throw new Error('GetAgentSessionOptions deve ser chamado via wailsapi.ACPOptions');
      }
      return { conversationId: conversationId || '', available: false, options: [] };
    },
    SetAgentSessionOption: function(conversationId, optionId, value) {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.ACPOptions') {
        throw new Error('SetAgentSessionOption deve ser chamado via wailsapi.ACPOptions');
      }
      // Argumento faltando ou trocado escolheria a opção errada no agente; o
      // mock precisa reprovar isso em vez de responder "deu certo".
      if (typeof optionId !== 'string' || optionId === '') {
        throw new Error('SetAgentSessionOption exige optionId como segundo argumento');
      }
      if (typeof value !== 'string' || value === '') {
        throw new Error('SetAgentSessionOption exige value como terceiro argumento');
      }
      // O backend descarta opção sem values; a UI monta o picker a partir deles.
      // Devolver values vazio faria o e2e aceitar um payload que o app real
      // nunca entrega e o seletor nasceria mudo.
      const category = /mode/i.test(optionId) ? 'mode' : 'model';
      return {
        conversationId: conversationId || '',
        available: true,
        options: [{
          id: optionId,
          name: optionId,
          category,
          currentValue: value,
          values: [{ value, name: value }],
        }],
      };
    },
    GetAgentPermissions: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.ACPTrust') {
        throw new Error('GetAgentPermissions deve ser chamado via wailsapi.ACPTrust');
      }
      return [];
    },
    RevokeAgentPermission: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.ACPTrust') {
        throw new Error('RevokeAgentPermission deve ser chamado via wailsapi.ACPTrust');
      }
      return undefined;
    },
    GetAgentConversationWorkDir: function(conversationId) {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.ACPWorkDir') {
        throw new Error('GetAgentConversationWorkDir deve ser chamado via wailsapi.ACPWorkDir');
      }
      return {
        conversationId: conversationId || '',
        available: false,
        dir: '',
        workspaceDir: '',
        pinned: false,
      };
    },
    SetAgentConversationWorkDir: function(conversationId, dir) {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.ACPWorkDir') {
        throw new Error('SetAgentConversationWorkDir deve ser chamado via wailsapi.ACPWorkDir');
      }
      return {
        conversationId: conversationId || '',
        available: Boolean(dir),
        dir: dir || '',
        workspaceDir: '',
        pinned: Boolean(dir),
      };
    },
    ACPAgentInstallPlan: function(agentId) {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.ACPInstall') {
        throw new Error('ACPAgentInstallPlan deve ser chamado via wailsapi.ACPInstall');
      }
      return {
        agent_id: agentId || '',
        name: '',
        version: '',
        distribution: '',
        origin: '',
        dir: '',
        run_args: [],
        runtime: { name: 'node', required: false, found: false },
        can_install: false,
        update: false,
        can_update: false,
        installing: false,
      };
    },
    ListInstalledACPAgents: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.ACPInstall') {
        throw new Error('ListInstalledACPAgents deve ser chamado via wailsapi.ACPInstall');
      }
      return [];
    },
    CanRemoveACPAgent: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.ACPInstall') {
        throw new Error('CanRemoveACPAgent deve ser chamado via wailsapi.ACPInstall');
      }
      return false;
    },
    // Mesmo padrão: UpdateProfileMediaSupport migrou para wailsapi.Profiles.
    UpdateProfileMediaSupport: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Profiles') {
        throw new Error('UpdateProfileMediaSupport deve ser chamado via wailsapi.Profiles');
      }
      return undefined;
    },

    /* Auth */
    GetAuthStatus: {
      vaultConfigured: true,
      vaultUnlocked: true,
      hasUsers: true,
    },
    RefreshAuth: defaultAuthUser,
    Login: defaultAuthUser,
    Logout: undefined,

    /* Workspace */
    GetActiveWorkspace: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('GetActiveWorkspace deve ser chamado via wailsapi.Workspace');
      }
      return defaultWorkspace;
    },
    ListWorkspaces: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('ListWorkspaces deve ser chamado via wailsapi.Workspace');
      }
      return [{ id: 'ws-1', name: 'Workspace', path: '', profile: '', tab_count: 1, is_active: true }];
    },
    AddWorkspaceTab: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('AddWorkspaceTab deve ser chamado via wailsapi.Workspace');
      }
      return defaultWorkspace;
    },
    SaveWorkspace: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('SaveWorkspace deve ser chamado via wailsapi.Workspace');
      }
      return undefined;
    },
    CreateWorkspace: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('CreateWorkspace deve ser chamado via wailsapi.Workspace');
      }
      return defaultWorkspace;
    },
    RemoveWorkspaceTab: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('RemoveWorkspaceTab deve ser chamado via wailsapi.Workspace');
      }
      return defaultWorkspace;
    },
    MoveWorkspaceTabTo: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('MoveWorkspaceTabTo deve ser chamado via wailsapi.Workspace');
      }
      return defaultWorkspace;
    },
    ReorderWorkspaceTabs: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('ReorderWorkspaceTabs deve ser chamado via wailsapi.Workspace');
      }
      return undefined;
    },
    SetActiveWorkspaceTab: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('SetActiveWorkspaceTab deve ser chamado via wailsapi.Workspace');
      }
      return undefined;
    },
    UpdateWorkspaceTab: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('UpdateWorkspaceTab deve ser chamado via wailsapi.Workspace');
      }
      return undefined;
    },
    SwitchWorkspace: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('SwitchWorkspace deve ser chamado via wailsapi.Workspace');
      }
      return defaultWorkspace;
    },
    RenameWorkspace: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('RenameWorkspace deve ser chamado via wailsapi.Workspace');
      }
      return undefined;
    },
    DeleteWorkspace: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('DeleteWorkspace deve ser chamado via wailsapi.Workspace');
      }
      return undefined;
    },
    SetWorkspaceProfile: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('SetWorkspaceProfile deve ser chamado via wailsapi.Workspace');
      }
      return undefined;
    },
    ExportWorkspace: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('ExportWorkspace deve ser chamado via wailsapi.Workspace');
      }
      return '';
    },
    ImportWorkspace: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Workspace') {
        throw new Error('ImportWorkspace deve ser chamado via wailsapi.Workspace');
      }
      return defaultWorkspace;
    },

    /* Conversations */
    EnsureConversation: defaultConversation,
    CreateConversation: defaultConversation,
    GetConversationInfo: defaultConversation,
    GetConversations: [],
    ClearConversation: undefined,
    DeleteConversation: undefined,
    DeleteConversations: (ids) => ids,
    RenameConversation: undefined,
    ClearMessages: undefined,
    SearchConversationHistory: [],

    /* Messages */
    SendMessage: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Chat') {
        throw new Error('SendMessage deve ser chamado via wailsapi.Chat');
      }
      return '01926b90-0000-7000-8000-000000000002';
    },
    RetryMessage: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Chat') {
        throw new Error('RetryMessage deve ser chamado via wailsapi.Chat');
      }
      return '01926b90-0000-7000-8000-000000000003';
    },
    AddMessage: { id: '01926b90-0000-7000-8000-000000000002', conversationId: '01926b90-0000-7000-8000-000000000001', role: 'user', content: '', createdAt: now },
    DeleteMessage: undefined,
    UpdateMessage: undefined,

    /* Profiles */
    GetActiveProfile: defaultProfile,
    GetActiveProfileSlug: 'default',
    GetProfiles: [defaultProfile],
    GetProfile: defaultProfile,
    SetActiveProfile: undefined,

    /* Providers — defaults também em wailsapi.LLMProviders */
    GetLLMProviders: [],
    GetLLMProvidersWithStatus: [],
    GetActiveProviderInfo: {},
    /* Models — defaults também em wailsapi.LLMModels */
    GetModels: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.LLMModels') {
        throw new Error('GetModels deve ser chamado via wailsapi.LLMModels');
      }
      return [];
    },
    GetModelsByProvider: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.LLMModels') {
        throw new Error('GetModelsByProvider deve ser chamado via wailsapi.LLMModels');
      }
      return [];
    },
    RefreshModels: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.LLMModels') {
        throw new Error('RefreshModels deve ser chamado via wailsapi.LLMModels');
      }
      return [];
    },
    RefreshModelsByProvider: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.LLMModels') {
        throw new Error('RefreshModelsByProvider deve ser chamado via wailsapi.LLMModels');
      }
      return [];
    },
    GetModelCatalogByProvider: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.LLMModels') {
        throw new Error('GetModelCatalogByProvider deve ser chamado via wailsapi.LLMModels');
      }
      return { models: [], agent: false };
    },
    RefreshModelCatalogByProvider: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.LLMModels') {
        throw new Error('RefreshModelCatalogByProvider deve ser chamado via wailsapi.LLMModels');
      }
      return { models: [], agent: false };
    },
    CancelStreamingForConversation: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.LLMModels') {
        throw new Error('CancelStreamingForConversation deve ser chamado via wailsapi.LLMModels');
      }
    },
    GetEffectiveModel: 'gpt-4',
    ListModelsRaw: ['gpt-4', 'gpt-4o'],
    SetDefaultProvider: undefined,
    ReloadLLMClient: undefined,
    CreateLLMProvider: {},
    UpdateLLMProvider: {},
    DeleteLLMProvider: undefined,
    CreateDefaultLLMProvider: undefined,
    TestLLMProvider: true,

    /* Skills */
    GetUserInvocableSkillsForProfile: [],
    GetSkills: [],

    /* Tools */
    GetAvailableTools: [],
    GetToolCatalog: {},

    /* Editor — defaults também em wailsapi.Editor */
    EditorLoadState: { fileModeByPath: {}, mergeSessionsByTabId: {} },
    EditorGetDraftPath: '',
    EditorReadDraft: '',
    EditorReadFile: '',
    EditorGetFileInfo: { path: '', exists: false, isDir: false, size: 0, modTimeMs: 0 },
    EditorOpenFile: { path: '', content: '' },
    EditorWriteDraft: undefined,
    EditorWriteFile: undefined,
    EditorDeleteDraft: undefined,
    EditorSaveState: undefined,
    EditorSaveFileDialog: '',
    EditorRenameFile: '',
    EditorWatchFile: undefined,
    EditorUnwatchFile: undefined,

    /* Tokens */
    GetAllTokenStats: {},
    GetConversationTokenStats: {
      conversationId: '01926b90-0000-7000-8000-000000000001',
      promptTokens: 0,
      completionTokens: 0,
      totalTokens: 0,
      contextTokens: 0,
      messageCount: 0,
      mostUsedModel: 'gpt-4',
      contextUsage: 0,
      contextLimit: 128000,
      isNearLimit: false,
      isCritical: false,
    },
    GetTurnTokenStats: { promptTokens: 0, completionTokens: 0, totalTokens: 0 },
    GetRecentMessagesTokenCount: 0,
    CheckContextWindowThreshold: { above: false, percentage: 0 },

    /* Speech */
    GetSpeechProviders: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Speech') {
        throw new Error('GetSpeechProviders deve ser chamado via wailsapi.Speech');
      }
      return [];
    },
    GetNativeTTSProviders: ['webspeech'],
    GetTTSModels: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Speech') {
        throw new Error('GetTTSModels deve ser chamado via wailsapi.Speech');
      }
      return [];
    },
    GetTTSVoices: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Speech') {
        throw new Error('GetTTSVoices deve ser chamado via wailsapi.Speech');
      }
      return [];
    },
    GetSTTModels: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Speech') {
        throw new Error('GetSTTModels deve ser chamado via wailsapi.Speech');
      }
      return [];
    },
    GetOpenAITTSVoices: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Speech') {
        throw new Error('GetOpenAITTSVoices deve ser chamado via wailsapi.Speech');
      }
      return [];
    },
    InitSpeechManager: undefined,
    InitSpeechManagerFromProfile: undefined,
    SpeakMessage: undefined,
    GetMessageAudio: null,
    GenerateAndSaveMessageAudio: null,
    SpeakPreview: undefined,
    DispatchSpeech: undefined,

    /* MCP */
    ListMCPServers: [],

    /* Channels */
    GetAvailableChannels: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Messaging') {
        throw new Error('GetAvailableChannels deve ser chamado via wailsapi.Messaging');
      }
      return [];
    },
    GetAllChannelConfigs: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Messaging') {
        throw new Error('GetAllChannelConfigs deve ser chamado via wailsapi.Messaging');
      }
      return {};
    },
    GetMessagingStatus: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Messaging') {
        throw new Error('GetMessagingStatus deve ser chamado via wailsapi.Messaging');
      }
      return {};
    },
    GetChannelTemplates: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Messaging') {
        throw new Error('GetChannelTemplates deve ser chamado via wailsapi.Messaging');
      }
      return [];
    },
    SaveChannelConfig: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Messaging') {
        throw new Error('SaveChannelConfig deve ser chamado via wailsapi.Messaging');
      }
      return undefined;
    },
    RestartChannel: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Messaging') {
        throw new Error('RestartChannel deve ser chamado via wailsapi.Messaging');
      }
      return undefined;
    },
    CreateChannelFromTemplate: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Messaging') {
        throw new Error('CreateChannelFromTemplate deve ser chamado via wailsapi.Messaging');
      }
      return undefined;
    },
    GetAuthorizedContacts: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Messaging') {
        throw new Error('GetAuthorizedContacts deve ser chamado via wailsapi.Messaging');
      }
      return {};
    },
    RemoveAuthorizedContact: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Messaging') {
        throw new Error('RemoveAuthorizedContact deve ser chamado via wailsapi.Messaging');
      }
      return undefined;
    },
    AssignConversationToChannel: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Messaging') {
        throw new Error('AssignConversationToChannel deve ser chamado via wailsapi.Messaging');
      }
      return undefined;
    },
    UnassignConversationFromChannel: function() {
      const last = _config.callLog[_config.callLog.length - 1];
      if (!last || last.scope !== 'wailsapi.Messaging') {
        throw new Error('UnassignConversationFromChannel deve ser chamado via wailsapi.Messaging');
      }
      return undefined;
    },

    /* Settings */
    ResetConfig: undefined,

    /* Misc */
    RespondQuestionnaire: undefined,
    ListCredentials: [],
    ListExternalSources: [],
    GetAllowlists: [],
    GetNetworkAllowlist: [],
    GetAllTaskLists: [],
    CheckForUpdates: { available: false },
    TestConnection: { success: true },
    ExportConversations: '',
    ImportConversations: undefined,
  };

  // Backend simulado do contrato canônico. As fixtures de mensagens continuam
  // sendo dados de entrada do servidor, nunca uma fonte alternativa no produto.
  const timelineWindows = new Map();
  const timelineRows = new Map();
  const timelineSequences = new Map();
  const sequenceFor = id => timelineSequences.get(id) || 1;
  const timelineThreadRows = new Map();
  function fixtureRows(conversationId) {
    if (!timelineRows.has(conversationId)) timelineRows.set(conversationId, []);
    return timelineRows.get(conversationId);
  }
    function canonicalItem(node, conversationId) {
    const m = node.message || node;
    const id = m.id;
    const segments = m.canonicalSegments || (m.turnSegments?.length
      ? m.turnSegments.map((part, index) => ({ segmentId: id + ':part:' + index,
          kind: part.type === 'tool_calls' ? 'tool_invocations' : part.type,
          text: part.content, invocations: part.toolInvocations || [] }))
      : [{ segmentId: id + ':text', kind: 'text', text: m.content || '' }]);
    if (m.reasoning && !segments.some(part => part.kind === 'reasoning')) {
      segments.unshift({ segmentId: id + ':reasoning', kind: 'reasoning', text: m.reasoning });
    }
    const conclusion = segments.filter(part => part.kind === 'text').slice(-1);
    return { itemId: id, conversationId, scopeParentId: m.parentId || null,
      kind: m.role === 'assistant' ? 'response' : 'message', turnId: m.turnId || null,
      replyToItemId: '', messageIds: [id], representativeMessageId: id,
      revision: String(m.canonicalRevision || 1), state: m.isStreaming ? 'streaming' : 'completed',
      durability: 'persisted', segments,
      metadata: { thinking: !!m.thinking, terminalReason: m.terminalReason, createdAt: m.createdAt || now,
        source: m.source || '', childCount: node.childCount || node.children?.length || 0,
        conclusionSegmentIds: conclusion.map(part => part.segmentId),
        cacheReadTokens: 0, cacheWriteTokens: 0, cacheMissTokens: 0, origin: 'chat',
        role: m.role, pinned: !!m.pinned, model: m.model || '', promptTokens: 0,
        completionTokens: 0, totalTokens: 0 } };
  }
  function timelineSnapshot(window) {
    const root = fixtureRows(window.conversationId);
    const rows = window.scopeParentId
      ? (timelineThreadRows.get(window.scopeParentId) || root.find(row => row.message?.id === window.scopeParentId)?.children || []) : root;
    const items = rows.map(row => ({ ...canonicalItem(row, window.conversationId), scopeParentId: window.scopeParentId }));
    return { ...window, epoch: 'e2e-epoch', windowRevision: String(window.revision),
      sequence: String(sequenceFor(window.conversationId)), items, orderedItemIds: items.map(item => item.itemId),
      retainedItemIds: window.retainedItemIds || [], retentionBlocked: false,
      totalCount: items.length, startIndex: 0, endIndex: items.length,
      hasBefore: false, hasAfter: false, beforeCursor: '', afterCursor: '' };
  }
  function timelineRPC(name, args) {
    if (name === 'Open') {
      const request = args[0];
      const window = { ...request, scopeParentId: request.scopeParentId || null,
        windowId: request.surfaceId, revision: 1, baseline: 1, mode: 'historical', ids: [] };
      timelineWindows.set(window.windowId, window);
      const snapshot = timelineSnapshot(window); window.ids = snapshot.orderedItemIds;
      return snapshot;
    }
    const window = timelineWindows.get(args[0]);
    if (name === 'Close') { timelineWindows.delete(args[0]); return; }
    if (!window) throw new Error('Unknown mock timeline window');
    if (!['SetRetention', 'Navigate', 'Refresh'].includes(name)) throw new Error('Unimplemented mock timeline RPC: ' + name);
    // Refresh aceita checkpoints desde o último snapshot, como acceptsSnapshotRevision
    // no backend; Navigate/SetRetention continuam exigindo a revisão atual.
    const requestedRevision = args[1].windowRevision;
    const validRevision = name === 'Refresh'
      ? /^[1-9][0-9]*$/.test(requestedRevision) && BigInt(requestedRevision) >= BigInt(window.baseline) && BigInt(requestedRevision) <= BigInt(window.revision)
      : requestedRevision === String(window.revision) && args[1].epoch === 'e2e-epoch';
    if (!validRevision) throw new Error('Mock timeline checkpoint mismatch');
    if (name === 'SetRetention') {
      window.retainedItemIds = args[1].itemIds;
      publishTimeline(window.conversationId, false);
      return { windowId: window.windowId, epoch: 'e2e-epoch', windowRevision: String(window.revision) };
    }
    window.revision++;
    if (name === 'Navigate') window.mode = args[1].mode;
    window.baseline = window.revision;
    return timelineSnapshot(window);
  }
  function publishTimeline(conversationId, contentChanged = true) {
    timelineSequences.set(conversationId, sequenceFor(conversationId) + 1);
    const windows = [...timelineWindows.values()].filter(window => window.conversationId === conversationId);
    const patches = windows.map(window => {
      const previousRevision = String(window.revision++);
      const snapshot = timelineSnapshot(window);
      const patch = { windowId: window.windowId, previousRevision, revision: String(window.revision),
        splices: contentChanged ? [{ index: 0, deleteCount: window.ids.length, insertIds: snapshot.orderedItemIds }] : [],
        items: contentChanged ? snapshot.items : [], metadata: snapshot };
      window.ids = snapshot.orderedItemIds;
      return patch;
    });
    for (const win of windows) window.runtime.EventsEmit('chat:timeline', {
      surfaceId: win.surfaceId, conversationId, windowId: win.windowId, epoch: 'e2e-epoch',
      snapshotRevision: String(win.baseline), syncRequired: false,
      updates: [{ conversationId, epoch: 'e2e-epoch', sequence: String(sequenceFor(conversationId)),
        operations: [], windowPatches: patches }] });
  }
  function projectFixtureEvent(event, data) {
    if (event === 'conversation:cleared') {
      timelineRows.set(data.conversation_id, []); publishTimeline(data.conversation_id); return;
    }
    if (!data?.conversationId || (!event.startsWith('chat:') && !event.startsWith('message:'))) return;
    const rows = fixtureRows(data.conversationId);
    if (event === 'chat:messages_ready') {
      if (data.userMessageId && !rows.some(row => row.message.id === data.userMessageId))
        rows.push({ message: { id: data.userMessageId, role: 'user', content: data.userContent || '', conversationId: data.conversationId }, children: [] });
    } else if (event === 'message:pin_changed') {
      const row = rows.find(row => row.message.id === data.messageId);
      if (row) { row.message.pinned = data.pinned; row.message.canonicalRevision = (row.message.canonicalRevision || 1) + 1; }
    } else if (['chat:stream', 'chat:thinking', 'chat:tool_start', 'chat:tool_end', 'chat:segment_done', 'chat:done'].includes(event)) {
      const id = data.assistantMessageId || data.messageId || rows.findLast(row => row.message.role === 'assistant' && row.message.isStreaming)?.message.id;
      if (!id) return;
      let row = rows.find(row => row.message.id === id);
      if (!row) { row = { message: { id, role: 'assistant', content: '', conversationId: data.conversationId, isStreaming: true, canonicalSegments: [] }, children: [] }; rows.push(row); }
      const m = row.message;
      m.canonicalRevision = (m.canonicalRevision || 1) + 1;
      if (data.turnId) m.turnId = data.turnId;
      m.canonicalSegments ||= canonicalItem(row, data.conversationId).segments;
      const segments = m.canonicalSegments;
      if (event === 'chat:stream' && typeof data.delta === 'string') {
        let tail = segments.at(-1);
        if (!tail || tail.kind !== 'text' || m.segmentClosed) {
          tail = { segmentId: id + ':part:' + segments.length, kind: 'text', text: '' }; segments.push(tail);
        }
        tail.text = data.reset ? data.delta : tail.text + data.delta;
        m.content = tail.text; m.segmentClosed = false; m.thinking = false;
      }
      if (event === 'chat:thinking') { m.reasoning = data.content || m.reasoning; m.thinking = !data.done; }
      if (event === 'chat:tool_start') segments.push({ segmentId: id + ':tool:' + data.callId,
        kind: 'tool_invocations', invocations: [{ callId: data.callId, name: data.name,
          status: 'running', hasDetails: false, resultAvailability: 'pending' }] });
      if (event === 'chat:tool_end') for (const part of segments) for (const tool of part.invocations || [])
        if (tool.callId === data.callId) { tool.status = data.status === 'error' ? 'failed' : 'succeeded'; tool.outputPreview = data.summary; }
      if (event === 'chat:segment_done') m.segmentClosed = true;
      if (event === 'chat:done') { m.isStreaming = false; m.thinking = false; m.terminalReason = data.reason; }
    } else return;
    publishTimeline(data.conversationId);
  }

  /* ---------- proxy de funções ---------- */

  function makeProxy(scope) {
    return new Proxy({}, {
      get(_target, prop) {
        // Wails namespaces are ordinary objects, not PromiseLike values.
        // Returning a generic method for then makes async appFor() assimilate
        // this proxy as a never-settling thenable, preventing every API call.
        if (prop === 'then') return undefined;
        const fnName = String(prop);
        return function(...args) {
          _config.callLog.push({ fn: fnName, scope, args });
          const submitted = fnName === 'SendMessage' || fnName === 'RetryMessage'
            ? acceptChatSubmission(fnName, args)
            : undefined;
          if (fnName in _config.errors) {
            // The chat RPC may have persisted the user message before it
            // returns an error; production deliberately reconciles this as
            // outcome_unknown rather than a retryable failure.
            if (submitted) submitted.status = 'outcome_unknown';
            return Promise.reject(new Error(_config.errors[fnName]));
          }
          if (fnName in _config.responses) {
            const val = _config.responses[fnName];
            let response;
            try {
              response = typeof val === 'function' ? val(...args) : val;
            } catch (error) {
              if (submitted) submitted.status = 'outcome_unknown';
              return Promise.reject(error);
            }
            return Promise.resolve(response).then(result => {
              if (submitted) submitted.status = 'succeeded';
              return result;
            }, error => {
              if (submitted) submitted.status = 'outcome_unknown';
              throw error;
            });
          }
          if (scope === 'wailsapi.ChatTimeline') return Promise.resolve(timelineRPC(fnName, args));


          if (fnName === 'GetConversationsPage' && 'GetConversations' in _config.responses) {
            const val = _config.responses.GetConversations;
            const rows = typeof val === 'function' ? val() : val;
            const conversations = Array.isArray(rows) ? rows : [];
            const limit = normalizeConversationPageLimit(args[0]);
            const offset = normalizeConversationPageOffset(args[1]);
            const pageRows = conversations.slice(offset, offset + limit);
            return Promise.resolve({ conversations: pageRows, total: conversations.length });
          }
          if (fnName === 'GetConversationsByIDs' && 'GetConversations' in _config.responses) {
            const val = _config.responses.GetConversations;
            const rows = typeof val === 'function' ? val() : val;
            const conversations = Array.isArray(rows) ? rows : [];
            const ids = Array.isArray(args[0]) ? args[0].map(String) : [];
            return Promise.resolve(ids.flatMap((id) => {
              const row = conversations.find((conversation) => String(conversation?.id) === id);
              return row ? [row] : [];
            }));
          }

          if (fnName in defaults) {
            const val = defaults[fnName];
            let response;
            try {
              response = typeof val === 'function' ? val(...args) : JSON.parse(JSON.stringify(val));
            } catch (error) {
              if (submitted) submitted.status = 'outcome_unknown';
              return Promise.reject(error);
            }
            return Promise.resolve(response).then(result => {
              if (submitted) submitted.status = 'succeeded';
              return result;
            }, error => {
              if (submitted) submitted.status = 'outcome_unknown';
              throw error;
            });
          }
          if (fnName === 'GetConversationsPage') {
            const conversations = Array.isArray(defaults.GetConversations) ? defaults.GetConversations : [];
            const limit = normalizeConversationPageLimit(args[0]);
            const offset = normalizeConversationPageOffset(args[1]);
            const pageRows = conversations.slice(offset, offset + limit);
            return Promise.resolve({ conversations: pageRows, total: conversations.length });
          }
          if (fnName === 'GetConversationsByIDs') {
            return Promise.resolve([]);
          }
          return Promise.resolve(undefined);
        };
      },
    });
  }

  /* ---------- window.go.{app,main}.App + wailsapi.* ---------- */

  window.go = {
    app: {
      App: makeProxy('app.App'),
    },
    main: {
      App: makeProxy('main.App'),
    },
    wailsapi: {
      CommandCatalog: makeProxy('wailsapi.CommandCatalog'),
      Probe: makeProxy('wailsapi.Probe'),
      Tokens: makeProxy('wailsapi.Tokens'),
      Skills: makeProxy('wailsapi.Skills'),
      Allowlists: makeProxy('wailsapi.Allowlists'),
      Tools: makeProxy('wailsapi.Tools'),
      Updater: makeProxy('wailsapi.Updater'),
      Profiles: makeProxy('wailsapi.Profiles'),
      Hotkeys: makeProxy('wailsapi.Hotkeys'),
      NetTrust: makeProxy('wailsapi.NetTrust'),
      FSTrust: makeProxy('wailsapi.FSTrust'),
      Credentials: makeProxy('wailsapi.Credentials'),
      Settings: makeProxy('wailsapi.Settings'),
      MCP: makeProxy('wailsapi.MCP'),
      Signal: makeProxy('wailsapi.Signal'),
      Terminal: makeProxy('wailsapi.Terminal'),
      Memory: makeProxy('wailsapi.Memory'),
      Messaging: makeProxy('wailsapi.Messaging'),
      Welcome: makeProxy('wailsapi.Welcome'),
      Workspace: makeProxy('wailsapi.Workspace'),
      LegacyCleanup: makeProxy('wailsapi.LegacyCleanup'),
      Database: makeProxy('wailsapi.Database'),
      Subagent: makeProxy('wailsapi.Subagent'),
      Tasklist: makeProxy('wailsapi.Tasklist'),
      TasklistActions: makeProxy('wailsapi.TasklistActions'),
      Conversations: makeProxy('wailsapi.Conversations'),
      Speech: makeProxy('wailsapi.Speech'),
      Jobs: makeProxy('wailsapi.Jobs'),
      LLMCatalog: makeProxy('wailsapi.LLMCatalog'),
      LLMProviders: makeProxy('wailsapi.LLMProviders'),
      LLMModels: makeProxy('wailsapi.LLMModels'),
      Chat: makeProxy('wailsapi.Chat'),
      ChatTimeline: makeProxy('wailsapi.ChatTimeline'),
      ACPCommands: makeProxy('wailsapi.ACPCommands'),
      ACPProviders: makeProxy('wailsapi.ACPProviders'),
      ACPOptions: makeProxy('wailsapi.ACPOptions'),
      ACPRegistry: makeProxy('wailsapi.ACPRegistry'),
      ACPWorkDir: makeProxy('wailsapi.ACPWorkDir'),
      ACPInstall: makeProxy('wailsapi.ACPInstall'),
      ACPTrust: makeProxy('wailsapi.ACPTrust'),
      Editor: makeProxy('wailsapi.Editor'),
      ExportImport: makeProxy('wailsapi.ExportImport'),
    },
  };

  /* ---------- window.runtime ---------- */

  window.runtime = {
    EventsOn(eventName, callback) {
      if (!_config.eventListeners.has(eventName)) {
        _config.eventListeners.set(eventName, []);
      }
      _config.eventListeners.get(eventName).push(callback);
      return () => {
        const arr = _config.eventListeners.get(eventName);
        if (arr) {
          const idx = arr.indexOf(callback);
          if (idx >= 0) arr.splice(idx, 1);
        }
      };
    },
    EventsOnMultiple(eventName, callback, maxCallbacks) {
      return window.runtime.EventsOn(eventName, callback);
    },
    EventsOnce(eventName, callback) {
      const unsub = window.runtime.EventsOn(eventName, function once(data) {
        callback(data);
        unsub();
      });
      return unsub;
    },
    EventsOff() {},
    EventsOffAll() {},
    EventsEmit(eventName, ...args) {
      const listeners = _config.eventListeners.get(eventName);
      if (listeners) {
        for (const fn of [...listeners]) {
          try { fn(args[0]); } catch(e) { console.error('[wails-mock] EventsEmit error:', e); }
        }
      }
    },
    LogPrint() {},
    LogTrace() {},
    LogDebug() {},
    LogInfo() {},
    LogWarning() {},
    LogError() {},
    LogFatal() {},
    WindowReload() {},
    WindowReloadApp() {},
    WindowSetAlwaysOnTop() {},
    WindowSetSystemDefaultTheme() {},
    WindowSetLightTheme() {},
    WindowSetDarkTheme() {},
    WindowCenter() {},
    WindowSetTitle() {},
    WindowFullscreen() {},
    WindowUnfullscreen() {},
    WindowSetSize() {},
    WindowGetSize() { return { w: 1280, h: 720 }; },
    WindowSetMaxSize() {},
    WindowSetMinSize() {},
    WindowSetPosition() {},
    WindowGetPosition() { return { x: 0, y: 0 }; },
    WindowHide() {},
    WindowShow() {},
    WindowMaximise() {},
    WindowToggleMaximise() {},
    WindowUnmaximise() {},
    WindowMinimise() {},
    WindowUnminimise() {},
    WindowSetBackgroundColour() {},
    WindowIsNormal() { return true; },
    WindowIsMaximised() { return false; },
    WindowIsMinimised() { return false; },
    WindowIsFullscreen() { return false; },
    BrowserOpenURL() {},
    Quit() {},
    ClipboardGetText() { return ''; },
    ClipboardSetText() {},
  };

  /* ---------- API pública para os testes ---------- */

  window.__wailsMock = {
    setTimelineRows(rows, parentId, conversationId) {
      const copied = JSON.parse(JSON.stringify(rows));
      if (parentId) {
        timelineThreadRows.set(parentId, copied);
        for (const id of timelineRows.keys()) publishTimeline(id);
      } else {
        const id = conversationId ?? rows[0]?.message?.conversationId ?? defaultConversation.id;
        timelineRows.set(id, copied);
        publishTimeline(id);
      }
    },
    setResponse(fn, value) {
      _config.responses[fn] = value;
      delete _config.errors[fn];

    },

    clearResponse(fn) {
      delete _config.responses[fn];
    },

    setError(fn, message) {
      _config.errors[fn] = message;
      delete _config.responses[fn];
    },

    clearError(fn) {
      delete _config.errors[fn];
    },

    emit(event, data) {
      projectFixtureEvent(event, data);
      window.runtime.EventsEmit(event, data);
    },

    getCallLog() {
      return [..._config.callLog];
    },

    reset() {
      _config.responses = {};
      _config.callLog = [];
    },
  };
})();
`;
}
