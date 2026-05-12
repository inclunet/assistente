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
      setResponse: (fn: string, value: unknown) => void;
      setError: (fn: string, message: string) => void;
      clearError: (fn: string) => void;
      emit: (event: string, data?: unknown) => void;
      getCallLog: () => Array<{ fn: string; args: unknown[] }>;
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

  const defaultConfig = {
    api_key: 'test-key',
    api_base_url: 'https://api.openai.com/v1',
    default_model: 'gpt-4',
    chat_params: {
      model: 'gpt-4',
      temperature: 0.7,
      max_tokens: 2000,
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

  const defaults = {
    /* App init */
    GetConfig: defaultConfig,
    NeedsWelcomeWizard: false,
    RunWelcomeWizard: true,
    GetAppVersion: '1.0.0-test',

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
    GetActiveWorkspace: defaultWorkspace,
    ListWorkspaces: [{ id: 'ws-1', name: 'Workspace', path: '', profile: '', tab_count: 1, is_active: true }],
    AddWorkspaceTab: defaultWorkspace,
    SaveWorkspace: defaultWorkspace,
    CreateWorkspace: defaultWorkspace,
    RemoveWorkspaceTab: defaultWorkspace,
    MoveWorkspaceTabTo: defaultWorkspace,
    ReorderWorkspaceTabs: defaultWorkspace,
    SetActiveWorkspaceTab: undefined,
    UpdateWorkspaceTab: defaultWorkspace,

    /* Conversations */
    EnsureConversation: defaultConversation,
    CreateConversation: defaultConversation,
    GetConversationInfo: defaultConversation,
    GetConversations: [],
    GetMessages: [],
    GetRecentMessages: [],
    GetMessagesBefore: [],
    GetConversationMessageWindow: {
      scope: 'conversation',
      conversationId: defaultConversation.id,
      nodes: [],
      totalCount: 0,
      startIndex: 0,
      endIndex: -1,
      hasBefore: false,
      hasAfter: false,
    },
    GetMessageChildren: [],
    ClearConversation: undefined,
    DeleteConversation: undefined,
    RenameConversation: undefined,
    ClearMessages: undefined,
    SearchConversationHistory: [],

    /* Messages */
    SendMessage: '01926b90-0000-7000-8000-000000000002',
    AddMessage: { id: '01926b90-0000-7000-8000-000000000002', conversationId: '01926b90-0000-7000-8000-000000000001', role: 'user', content: '', createdAt: now },
    DeleteMessage: undefined,
    UpdateMessage: undefined,

    /* Profiles */
    GetActiveProfile: defaultProfile,
    GetActiveProfileSlug: 'default',
    GetProfiles: [defaultProfile],
    GetProfile: defaultProfile,
    SetActiveProfile: undefined,

    /* Providers */
    GetLLMProviders: [],
    GetLLMProvidersWithStatus: [],
    GetActiveProviderInfo: {},
    GetModels: [],
    GetEffectiveModel: 'gpt-4',
    ListModelsRaw: ['gpt-4', 'gpt-4o'],

    /* Skills */
    GetUserInvocableSkills: [],
    GetSkills: [],

    /* Tools */
    GetAvailableTools: [],
    GetToolCatalog: {},

    /* Editor */
    EditorLoadState: { tabs: [], active: '' },

    /* Tokens */
    GetAllTokenStats: {},
    GetConversationTokenStats: {
      conversationId: '01926b90-0000-7000-8000-000000000001',
      promptTokens: 0,
      completionTokens: 0,
      totalTokens: 0,
      messageCount: 0,
      mostUsedModel: 'gpt-4',
      contextUsage: 0,
      contextLimit: 128000,
      isNearLimit: false,
      isCritical: false,
    },
    GetTurnTokenStats: { promptTokens: 0, completionTokens: 0, totalTokens: 0 },
    GetRecentMessagesTokenCount: 0,
    CheckContextWindowThreshold: false,

    /* Speech */
    GetSpeechProviders: [],
    GetNativeTTSProviders: ['webspeech'],
    GetTTSModels: [],
    GetTTSVoices: [],
    GetSTTModels: [],
    GetOpenAITTSVoices: [],
    InitSpeechManager: undefined,
    InitSpeechManagerFromProfile: undefined,
    SpeakMessage: undefined,
    GetMessageAudio: null,
    GenerateAndSaveMessageAudio: null,

    /* MCP */
    ListMCPServers: [],

    /* Channels */
    GetAvailableChannels: [],
    GetAllChannelConfigs: {},

    /* Settings */
    SaveSettings: undefined,
    ResetConfig: undefined,
    GetLLMSettings: {},
    SetDefaultModel: undefined,
    SetDefaultProvider: undefined,
    SetChatModel: undefined,

    /* Misc */
    RespondQuestionnaire: undefined,
    ListCredentials: [],
    GetAllowlists: [],
    GetAllTaskLists: [],
    CheckForUpdates: { available: false },
    ReloadLLMClient: undefined,
    TestConnection: { success: true },
    ExportConversations: '',
    ImportConversations: undefined,
  };

  /* ---------- proxy de funções ---------- */

  function makeProxy() {
    return new Proxy({}, {
      get(_target, prop) {
        const fnName = String(prop);
        return function(...args) {
          _config.callLog.push({ fn: fnName, args });
          if (fnName in _config.errors) {
            return Promise.reject(new Error(_config.errors[fnName]));
          }
          if (fnName in _config.responses) {
            const val = _config.responses[fnName];
            return Promise.resolve(typeof val === 'function' ? val(...args) : val);
          }
          if (fnName === 'GetRecentMessages' && 'GetMessages' in _config.responses) {
            const val = _config.responses.GetMessages;
            return Promise.resolve(typeof val === 'function' ? val(...args) : val);
          }
          if (fnName === 'GetMessagesBefore' && 'GetMessages' in _config.responses) {
            const val = _config.responses.GetMessages;
            return Promise.resolve(typeof val === 'function' ? val(...args) : val);
          }
          if (
            fnName === 'GetConversationMessageWindow'
            && !('GetConversationMessageWindow' in _config.responses)
            && 'GetMessages' in _config.responses
          ) {
            const val = _config.responses.GetMessages;
            const messages = typeof val === 'function' ? val(...args) : val;
            const nodes = Array.isArray(messages) ? messages : [];
            const req = args[0] || {};
            const limit = Math.max(0, Number(req.limit || nodes.length || 0));
            const direction = String(req.direction || 'before');
            const anchor = String(req.anchor || 'end');
            const anchorMessageId = String(req.anchorMessageId || '');
            const getNodeId = (node) => String(node?.message?.id ?? node?.id ?? '');
            const anchorIndex = anchorMessageId
              ? nodes.findIndex((node) => getNodeId(node) === anchorMessageId)
              : -1;
            let startIndex = 0;
            let endIndexExclusive = limit > 0 ? limit : 0;
            if (anchorIndex >= 0 && direction === 'before') {
              endIndexExclusive = anchorIndex;
              startIndex = Math.max(0, endIndexExclusive - limit);
            } else if (anchorIndex >= 0 && direction === 'after') {
              startIndex = Math.min(nodes.length, anchorIndex + 1);
              endIndexExclusive = limit > 0 ? startIndex + limit : startIndex;
            } else if (anchorIndex >= 0 && direction === 'around') {
              startIndex = Math.max(0, anchorIndex - Math.floor(limit / 2));
              if (limit > 0 && startIndex + limit > nodes.length) {
                startIndex = Math.max(0, nodes.length - limit);
              }
              endIndexExclusive = limit > 0 ? startIndex + limit : startIndex;
            } else if (anchorIndex >= 0) {
              startIndex = Math.max(0, Math.min(anchorIndex, nodes.length - limit));
              endIndexExclusive = limit > 0 ? startIndex + limit : startIndex;
            } else if (anchor === 'end' || direction === 'before') {
              startIndex = Math.max(0, nodes.length - limit);
              endIndexExclusive = limit > 0 ? startIndex + limit : startIndex;
            } else {
              endIndexExclusive = limit > 0 ? startIndex + limit : startIndex;
            }
            const visibleNodes = nodes.slice(startIndex, endIndexExclusive).map((node, index) => ({
              ...node,
              originalIndex: startIndex + index,
            }));
            return Promise.resolve({
              scope: req.scope || 'conversation',
              conversationId: req.conversationId || defaultConversation.id,
              threadParentId: req.threadParentId || '',
              nodes: visibleNodes,
              totalCount: nodes.length,
              startIndex,
              endIndex: visibleNodes.length > 0 ? startIndex + visibleNodes.length - 1 : -1,
              hasBefore: startIndex > 0,
              hasAfter: visibleNodes.length > 0 && startIndex + visibleNodes.length < nodes.length,
            });
          }
          if (fnName in defaults) {
            const val = defaults[fnName];
            return Promise.resolve(typeof val === 'function' ? val(...args) : JSON.parse(JSON.stringify(val)));
          }
          return Promise.resolve(undefined);
        };
      },
    });
  }

  /* ---------- window.go.{app,main}.App ---------- */

  const appProxy = makeProxy();
  window.go = {
    app: {
      App: appProxy,
    },
    main: {
      App: appProxy,
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
    setResponse(fn, value) {
      _config.responses[fn] = value;
      delete _config.errors[fn];
    },

    setError(fn, message) {
      _config.errors[fn] = message;
      delete _config.responses[fn];
    },

    clearError(fn) {
      delete _config.errors[fn];
    },

    emit(event, data) {
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
