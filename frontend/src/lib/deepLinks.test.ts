import { describe, expect, it, vi, beforeEach } from 'vitest';
import {
  isDeepLink,
  parseDeepLink,
  buildDeepLink,
  getDeepLinkTypeClass,
  executeDeepLink,
  DEEP_LINK_PREFIX,
  type DeepLinkAction,
} from './deepLinks';

// ─── Mocks para executeDeepLink ──────────────────────────────────────

const mockWsSetActiveTab = vi.fn().mockResolvedValue(undefined);
const mockWsAddTab = vi.fn().mockResolvedValue(undefined);
const mockWsUpdateTab = vi.fn().mockResolvedValue(undefined);
const mockSendMessageToConversation = vi.fn().mockResolvedValue(undefined);

let mockWsProfile: string | undefined;
let mockWsTabs: Array<{ id: string; type: string; conversationId?: string; state?: Record<string, unknown>; profileOverride?: Record<string, unknown> }> = [];

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: {
    getState: () => ({
      workspace: { tabs: mockWsTabs, profile: mockWsProfile },
      setActiveTab: mockWsSetActiveTab,
      addTab: mockWsAddTab,
      updateTab: mockWsUpdateTab,
    }),
  },
}));

const mockLoadConversation = vi.fn().mockResolvedValue(undefined);
const mockCreateConversation = vi.fn().mockResolvedValue('01926b90-7a5a-7c4e-8d3f-000000000064');

vi.mock('../store/chatStore', () => ({
  useChatStore: {
    getState: () => ({
      sendMessageToConversation: mockSendMessageToConversation,
      loadConversationSession: mockLoadConversation,
      createConversation: mockCreateConversation,
    }),
  },
}));

const mockLoadTerminalSessions = vi.fn().mockResolvedValue(true);
const mockCreateTerminalSession = vi.fn().mockResolvedValue('term-created');
let mockTerminalSessions: Array<{ id: string }> = [];

vi.mock('../store/terminalStore', () => ({
  useTerminalStore: {
    getState: () => ({
      sessions: mockTerminalSessions,
      loadSessions: mockLoadTerminalSessions,
      createSession: mockCreateTerminalSession,
    }),
  },
}));

const mockRequestResourceEdit = vi.fn();

vi.mock('../store/navigationStore', () => ({
  useNavigationStore: {
    getState: () => ({
      requestResourceEdit: mockRequestResourceEdit,
    }),
  },
}));

const mockAnnounce = vi.fn();
vi.mock('../hooks/useAnnouncer', () => ({
  announce: (...args: unknown[]) => mockAnnounce(...args),
}));

vi.mock('./i18n', () => ({
  default: {
    t: (key: string, opts?: Record<string, string>) => {
      if (key === 'deepLink.newTab' && opts?.type) {
        return `Novo ${opts.type}`;
      }
      const tabTypeKey = key.match(/^workspace\.tabType\.(editor|terminal|tasklist)$/);
      if (tabTypeKey) {
        const labels: Record<string, string> = {
          editor: 'Editor',
          terminal: 'Terminal',
          tasklist: 'Tarefas',
        };
        return labels[tabTypeKey[1]] ?? key;
      }
      const map: Record<string, string> = {
        'chat.newConversation': 'Nova Conversa',
        'terminal.pageTitle': 'Terminal',
        'editor.prompts.file': 'Arquivo',
      };
      return map[key] ?? key;
    },
  },
}));

const mockEditorReadFile = vi.fn().mockResolvedValue('file content');
const mockRunTerminalCommand = vi.fn().mockResolvedValue(undefined);
const mockGetProfile = vi.fn().mockResolvedValue({ slug: 'programacao' });

vi.mock('@wailsjs/go/wailsapi/Editor', () => ({
  EditorReadFile: (...args: unknown[]) => mockEditorReadFile(...args),
}));

vi.mock('@wailsjs/go/wailsapi/Terminal', () => ({
  RunTerminalCommand: (...args: unknown[]) => mockRunTerminalCommand(...args),
}));

vi.mock('@wailsjs/go/wailsapi/Profiles', () => ({
  GetProfile: (...args: unknown[]) => mockGetProfile(...args),
}));

const mockAddToast = vi.fn();
vi.mock('../store/uiStore', () => ({
  useUIStore: {
    getState: () => ({
      addToast: mockAddToast,
    }),
  },
}));

const mockCreateDocument = vi.fn().mockReturnValue('doc-new-id');
const mockSetDocFilePath = vi.fn();

vi.mock('../store/editorStore', () => ({
  useEditorStore: {
    getState: () => ({
      createDocument: mockCreateDocument,
      setDocFilePath: mockSetDocFilePath,
    }),
  },
}));

// ─── isDeepLink ──────────────────────────────────────────────────────

describe('isDeepLink', () => {
  it('retorna true para URIs com protocolo assistente://', () => {
    expect(isDeepLink('assistente://conversation/42')).toBe(true);
    expect(isDeepLink('assistente://navigate/history')).toBe(true);
    expect(isDeepLink('assistente://conversation/new?message=oi')).toBe(true);
  });

  it('retorna false para outros protocolos', () => {
    expect(isDeepLink('https://example.com')).toBe(false);
    expect(isDeepLink('http://example.com')).toBe(false);
    expect(isDeepLink('mailto:test@test.com')).toBe(false);
    expect(isDeepLink('')).toBe(false);
    expect(isDeepLink('assistente:')).toBe(false);
    expect(isDeepLink('assistente:/')).toBe(false);
  });
});

// ─── parseDeepLink ───────────────────────────────────────────────────

describe('parseDeepLink', () => {
  describe('conversation:open', () => {
    it('faz parse de assistente://conversation/{uuid}', () => {
      const result = parseDeepLink('assistente://conversation/01926b90-7a5a-7c4e-8d3f-00000000002a');
      expect(result).toEqual({ type: 'conversation:open', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000002a' });
    });

    it('rejeita UUIDs v4 (somente v7 é aceito)', () => {
      expect(parseDeepLink('assistente://conversation/550e8400-e29b-41d4-a716-446655440000')).toBeNull();
    });

    it('rejeita ID numérico (legado)', () => {
      expect(parseDeepLink('assistente://conversation/0')).toBeNull();
    });

    it('rejeita ID negativo', () => {
      expect(parseDeepLink('assistente://conversation/-1')).toBeNull();
    });

    it('rejeita ID não-UUID', () => {
      expect(parseDeepLink('assistente://conversation/abc')).toBeNull();
    });

    it('rejeita ID float', () => {
      expect(parseDeepLink('assistente://conversation/3.14')).toBeNull();
    });

    it('faz parse com profile', () => {
      const result = parseDeepLink('assistente://conversation/01926b90-7a5a-7c4e-8d3f-00000000002a?profile=programacao');
      expect(result).toEqual({
        type: 'conversation:open',
        conversationId: '01926b90-7a5a-7c4e-8d3f-00000000002a',
        profile: 'programacao',
      });
    });
  });

  describe('conversation:new', () => {
    it('faz parse sem parâmetros', () => {
      const result = parseDeepLink('assistente://conversation/new');
      expect(result).toEqual({ type: 'conversation:new', message: undefined, title: undefined });
    });

    it('faz parse com message', () => {
      const result = parseDeepLink('assistente://conversation/new?message=analise+o+ticket');
      expect(result).toEqual({
        type: 'conversation:new',
        message: 'analise o ticket',
        title: undefined,
      });
    });

    it('faz parse com message e title', () => {
      const result = parseDeepLink(
        'assistente://conversation/new?message=oi&title=Meu%20chat',
      );
      expect(result).toEqual({
        type: 'conversation:new',
        message: 'oi',
        title: 'Meu chat',
      });
    });

    it('decodifica caracteres URL-encoded', () => {
      const result = parseDeepLink(
        'assistente://conversation/new?message=analise%20o%20ticket%20%23123',
      );
      expect(result).toEqual({
        type: 'conversation:new',
        message: 'analise o ticket #123',
        title: undefined,
      });
    });

    it('faz parse com message e profile', () => {
      const result = parseDeepLink('assistente://conversation/new?message=oi&profile=techsupport');
      expect(result).toEqual({
        type: 'conversation:new',
        message: 'oi',
        title: undefined,
        profile: 'techsupport',
      });
    });
  });

  describe('conversation:send', () => {
    it('faz parse de send com message', () => {
      const result = parseDeepLink(
        'assistente://conversation/01926b90-7a5a-7c4e-8d3f-00000000000a/send?message=continue+aqui',
      );
      expect(result).toEqual({
        type: 'conversation:send',
        conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a',
        message: 'continue aqui',
      });
    });

    it('rejeita send sem message', () => {
      expect(parseDeepLink('assistente://conversation/01926b90-7a5a-7c4e-8d3f-00000000000a/send')).toBeNull();
    });

    it('rejeita send com ID não-UUID', () => {
      expect(parseDeepLink('assistente://conversation/abc/send?message=oi')).toBeNull();
    });

    it('faz parse de send com message e profile', () => {
      const result = parseDeepLink(
        'assistente://conversation/01926b90-7a5a-7c4e-8d3f-00000000000a/send?message=continue&profile=techsupport',
      );
      expect(result).toEqual({
        type: 'conversation:send',
        conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a',
        message: 'continue',
        profile: 'techsupport',
      });
    });
  });

  describe('navigate', () => {
    it('faz parse de rotas válidas', () => {
      const validRoutes = [
        'settings', 'settings/providers', 'settings/mcp', 'settings/skills',
        'settings/channels', 'settings/contacts', 'settings/credentials',
        'settings/allowlists', 'settings/network-allowlist',
        'settings/appearance', 'settings/restore-defaults',
        'settings/data',
        'profiles', 'history', 'memories', 'tasklists', 'help', 'about', 'update',
      ];

      for (const route of validRoutes) {
        const result = parseDeepLink(`assistente://navigate/${route}`);
        expect(result).toEqual({ type: 'navigate', route });
      }
    });

    it('faz parse da rota raiz (chat)', () => {
      const result = parseDeepLink('assistente://navigate/');
      expect(result).toEqual({ type: 'navigate', route: '' });
    });

    it('rejeita rotas inválidas', () => {
      expect(parseDeepLink('assistente://navigate/invalid-route')).toBeNull();
      expect(parseDeepLink('assistente://navigate/../../etc/passwd')).toBeNull();
      expect(parseDeepLink('assistente://navigate/admin')).toBeNull();
    });

    it('rejeita navigate/terminal e navigate/editor (são abas, não páginas)', () => {
      expect(parseDeepLink('assistente://navigate/terminal')).toBeNull();
      expect(parseDeepLink('assistente://navigate/editor')).toBeNull();
    });
  });

  describe('resource:edit', () => {
    it('faz parse de assistente://{resource}/edit/{id}', () => {
      expect(parseDeepLink('assistente://profiles/edit/programacao')).toEqual({
        type: 'resource:edit', resource: 'profiles', resourceId: 'programacao',
      });
      expect(parseDeepLink('assistente://providers/edit/openai-1')).toEqual({
        type: 'resource:edit', resource: 'providers', resourceId: 'openai-1',
      });
      expect(parseDeepLink('assistente://credentials/edit/llm%3A%2F%2F*')).toEqual({
        type: 'resource:edit', resource: 'credentials', resourceId: 'llm://*',
      });
      expect(parseDeepLink('assistente://mcp/edit/my-server')).toEqual({
        type: 'resource:edit', resource: 'mcp', resourceId: 'my-server',
      });
      expect(parseDeepLink('assistente://memories/edit/mem-1')).toEqual({
        type: 'resource:edit', resource: 'memories', resourceId: 'mem-1',
      });
    });

    it('rejeita recurso não editável', () => {
      expect(parseDeepLink('assistente://history/edit/1')).toBeNull();
      expect(parseDeepLink('assistente://settings/edit/1')).toBeNull();
    });

    it('rejeita edit sem ID', () => {
      expect(parseDeepLink('assistente://profiles/edit')).toBeNull();
      expect(parseDeepLink('assistente://profiles/edit/')).toBeNull();
    });

  });

  describe('resource:new', () => {
    it('faz parse de assistente://{resource}/new', () => {
      expect(parseDeepLink('assistente://profiles/new')).toEqual({
        type: 'resource:new', resource: 'profiles',
      });
      expect(parseDeepLink('assistente://skills/new')).toEqual({
        type: 'resource:new', resource: 'skills',
      });
      expect(parseDeepLink('assistente://allowlists/new')).toEqual({
        type: 'resource:new', resource: 'allowlists',
      });
      expect(parseDeepLink('assistente://tasklists/new')).toEqual({
        type: 'resource:new', resource: 'tasklists',
      });
      expect(parseDeepLink('assistente://memories/new')).toEqual({
        type: 'resource:new', resource: 'memories',
      });
    });

    it('rejeita new para recurso não editável', () => {
      expect(parseDeepLink('assistente://help/new')).toBeNull();
    });

  });

  describe('resource:edit tasklists', () => {
    it('faz parse de assistente://tasklists/edit/{id}', () => {
      expect(parseDeepLink('assistente://tasklists/edit/42')).toEqual({
        type: 'resource:edit', resource: 'tasklists', resourceId: '42',
      });
    });
  });

  describe('tab:new', () => {
    it('faz parse de assistente://tasklist/new', () => {
      expect(parseDeepLink('assistente://tasklist/new')).toEqual({
        type: 'tab:new', tabType: 'tasklist', title: undefined, cmd: undefined,
      });
    });

    it('faz parse de assistente://tasklist/new?title=Sprint', () => {
      expect(parseDeepLink('assistente://tasklist/new?title=Sprint%2023')).toEqual({
        type: 'tab:new', tabType: 'tasklist', title: 'Sprint 23', cmd: undefined,
      });
    });

    it('faz parse de assistente://editor/new', () => {
      expect(parseDeepLink('assistente://editor/new')).toEqual({
        type: 'tab:new', tabType: 'editor', title: undefined, cmd: undefined,
      });
    });

    it('faz parse de assistente://editor/open?file=path', () => {
      expect(parseDeepLink('assistente://editor/open?file=%2Ftmp%2Ftest.md')).toEqual({
        type: 'tab:new', tabType: 'editor', file: '/tmp/test.md', title: undefined,
      });
    });

    it('faz parse de assistente://editor/open?file=path&title=Nome', () => {
      expect(parseDeepLink('assistente://editor/open?file=readme.md&title=README')).toEqual({
        type: 'tab:new', tabType: 'editor', file: 'readme.md', title: 'README',
      });
    });

    it('rejeita editor/open sem file', () => {
      expect(parseDeepLink('assistente://editor/open')).toBeNull();
      expect(parseDeepLink('assistente://editor/open?title=abc')).toBeNull();
    });

    it('faz parse de assistente://terminal/new', () => {
      expect(parseDeepLink('assistente://terminal/new')).toEqual({
        type: 'tab:new', tabType: 'terminal', title: undefined, cmd: undefined,
      });
    });

    it('faz parse de assistente://terminal/new?cmd=npm+install', () => {
      expect(parseDeepLink('assistente://terminal/new?cmd=npm+install')).toEqual({
        type: 'tab:new', tabType: 'terminal', title: undefined, cmd: 'npm install',
      });
    });

    it('faz parse de assistente://terminal/new?cmd=ls&title=Build', () => {
      expect(parseDeepLink('assistente://terminal/new?cmd=ls&title=Build')).toEqual({
        type: 'tab:new', tabType: 'terminal', title: 'Build', cmd: 'ls',
      });
    });
  });

  describe('tab:open', () => {
    it('faz parse de assistente://tasklist/{id}', () => {
      expect(parseDeepLink('assistente://tasklist/5')).toEqual({
        type: 'tab:open', tabType: 'tasklist', contentId: '5',
      });
    });

    it('faz parse de assistente://editor/{id}', () => {
      expect(parseDeepLink('assistente://editor/doc-123')).toEqual({
        type: 'tab:open', tabType: 'editor', contentId: 'doc-123',
      });
    });

    it('faz parse de assistente://terminal/{id}', () => {
      expect(parseDeepLink('assistente://terminal/sess-1')).toEqual({
        type: 'tab:open', tabType: 'terminal', contentId: 'sess-1',
      });
    });

    it('decodifica contentId URL-encoded', () => {
      expect(parseDeepLink('assistente://editor/meu%20doc')).toEqual({
        type: 'tab:open', tabType: 'editor', contentId: 'meu doc',
      });
    });

    it('rejeita tab sem contentId', () => {
      expect(parseDeepLink('assistente://tasklist')).toBeNull();
      expect(parseDeepLink('assistente://tasklist/')).toBeNull();
      expect(parseDeepLink('assistente://editor')).toBeNull();
      expect(parseDeepLink('assistente://terminal')).toBeNull();
    });
  });

  describe('rejeição de URIs inválidos', () => {
    it('retorna null para URI vazio', () => {
      expect(parseDeepLink('')).toBeNull();
    });

    it('retorna null para protocolo diferente', () => {
      expect(parseDeepLink('https://example.com')).toBeNull();
    });

    it('retorna null para recurso desconhecido', () => {
      expect(parseDeepLink('assistente://unknown/resource')).toBeNull();
    });

    it('retorna null para URI sem recurso', () => {
      expect(parseDeepLink('assistente://')).toBeNull();
    });

    it('retorna null para conversation sem ID nem ação', () => {
      expect(parseDeepLink('assistente://conversation/')).toBeNull();
      expect(parseDeepLink('assistente://conversation')).toBeNull();
    });
  });
});

// ─── buildDeepLink ───────────────────────────────────────────────────

describe('buildDeepLink', () => {
  it('constrói conversation:open', () => {
    const uri = buildDeepLink({ type: 'conversation:open', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000002a' });
    expect(uri).toBe('assistente://conversation/01926b90-7a5a-7c4e-8d3f-00000000002a');
  });

  it('constrói conversation:new sem parâmetros', () => {
    const uri = buildDeepLink({ type: 'conversation:new' });
    expect(uri).toBe('assistente://conversation/new');
  });

  it('constrói conversation:new com message e title', () => {
    const uri = buildDeepLink({
      type: 'conversation:new',
      message: 'analise o ticket #123',
      title: 'Análise',
    });
    expect(uri).toContain('assistente://conversation/new?');
    expect(uri).toContain('message=analise+o+ticket+%23123');
    expect(uri).toContain('title=An%C3%A1lise');
  });

  it('constrói conversation:send', () => {
    const uri = buildDeepLink({
      type: 'conversation:send',
      conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a',
      message: 'continue aqui',
    });
    expect(uri).toContain('assistente://conversation/01926b90-7a5a-7c4e-8d3f-00000000000a/send?');
    expect(uri).toContain('message=continue+aqui');
  });

  it('constrói conversation:open com profile', () => {
    const uri = buildDeepLink({
      type: 'conversation:open',
      conversationId: '01926b90-7a5a-7c4e-8d3f-00000000002a',
      profile: 'programacao',
    });
    expect(uri).toBe('assistente://conversation/01926b90-7a5a-7c4e-8d3f-00000000002a?profile=programacao');
  });

  it('constrói conversation:new com profile', () => {
    const uri = buildDeepLink({ type: 'conversation:new', message: 'oi', profile: 'techsupport' });
    expect(uri).toContain('message=oi');
    expect(uri).toContain('profile=techsupport');
  });

  it('constrói conversation:send com profile', () => {
    const uri = buildDeepLink({
      type: 'conversation:send',
      conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a',
      message: 'continue',
      profile: 'techsupport',
    });
    expect(uri).toContain('message=continue');
    expect(uri).toContain('profile=techsupport');
  });

  it('constrói navigate', () => {
    const uri = buildDeepLink({ type: 'navigate', route: 'history' });
    expect(uri).toBe('assistente://navigate/history');
  });

  it('constrói navigate para raiz', () => {
    const uri = buildDeepLink({ type: 'navigate', route: '' });
    expect(uri).toBe('assistente://navigate/');
  });

  it('constrói resource:edit', () => {
    const uri = buildDeepLink({ type: 'resource:edit', resource: 'profiles', resourceId: 'programacao' });
    expect(uri).toBe('assistente://profiles/edit/programacao');
  });

  it('constrói resource:edit com caracteres especiais no ID', () => {
    const uri = buildDeepLink({ type: 'resource:edit', resource: 'credentials', resourceId: 'llm://*' });
    expect(uri).toBe('assistente://credentials/edit/llm%3A%2F%2F*');
  });

  it('constrói resource:new', () => {
    const uri = buildDeepLink({ type: 'resource:new', resource: 'skills' });
    expect(uri).toBe('assistente://skills/new');
  });

  it('constrói tab:open para tasklist', () => {
    const uri = buildDeepLink({ type: 'tab:open', tabType: 'tasklist', contentId: '5' });
    expect(uri).toBe('assistente://tasklist/5');
  });

  it('constrói tab:open para editor', () => {
    const uri = buildDeepLink({ type: 'tab:open', tabType: 'editor', contentId: 'doc-1' });
    expect(uri).toBe('assistente://editor/doc-1');
  });

  it('constrói tab:open para terminal', () => {
    const uri = buildDeepLink({ type: 'tab:open', tabType: 'terminal', contentId: 'sess' });
    expect(uri).toBe('assistente://terminal/sess');
  });

  it('constrói tab:open com caracteres especiais', () => {
    const uri = buildDeepLink({ type: 'tab:open', tabType: 'editor', contentId: 'meu doc' });
    expect(uri).toBe('assistente://editor/meu%20doc');
  });

  it('constrói tab:new para tasklist sem parâmetros', () => {
    const uri = buildDeepLink({ type: 'tab:new', tabType: 'tasklist' });
    expect(uri).toBe('assistente://tasklist/new');
  });

  it('constrói tab:new para tasklist com title', () => {
    const uri = buildDeepLink({ type: 'tab:new', tabType: 'tasklist', title: 'Sprint 23' });
    expect(uri).toContain('assistente://tasklist/new?');
    expect(uri).toContain('title=Sprint+23');
  });

  it('constrói tab:new para editor/open com file', () => {
    const uri = buildDeepLink({ type: 'tab:new', tabType: 'editor', file: '/tmp/test.md' });
    expect(uri).toContain('assistente://editor/open?');
    expect(uri).toContain('file=%2Ftmp%2Ftest.md');
  });

  it('constrói tab:new para terminal com cmd', () => {
    const uri = buildDeepLink({ type: 'tab:new', tabType: 'terminal', cmd: 'npm install' });
    expect(uri).toContain('assistente://terminal/new?');
    expect(uri).toContain('cmd=npm+install');
  });

  it('constrói tab:new para editor sem parâmetros', () => {
    const uri = buildDeepLink({ type: 'tab:new', tabType: 'editor' });
    expect(uri).toBe('assistente://editor/new');
  });
});

// ─── roundtrip: build → parse ────────────────────────────────────────

describe('roundtrip build → parse', () => {
  const actions: DeepLinkAction[] = [
    { type: 'conversation:open', conversationId: '01926b90-7a5a-7c4e-8d3f-000000000007' },
    { type: 'conversation:open', conversationId: '01926b90-7a5a-7c4e-8d3f-000000000007', profile: 'programacao' },
    { type: 'conversation:new', message: 'olá mundo', title: 'Test' },
    { type: 'conversation:new', message: 'olá mundo', title: 'Test', profile: 'techsupport' },
    { type: 'conversation:new' },
    { type: 'conversation:send', conversationId: '01926b90-7a5a-7c4e-8d3f-000000000003', message: 'continue' },
    { type: 'conversation:send', conversationId: '01926b90-7a5a-7c4e-8d3f-000000000003', message: 'continue', profile: 'techsupport' },
    { type: 'navigate', route: 'history' },
    { type: 'navigate', route: 'tasklists' },
    { type: 'navigate', route: '' },
    { type: 'resource:edit', resource: 'profiles', resourceId: 'programacao' },
    { type: 'resource:edit', resource: 'credentials', resourceId: 'llm://*' },
    { type: 'resource:edit', resource: 'tasklists', resourceId: '42' },
    { type: 'resource:new', resource: 'skills' },
    { type: 'resource:new', resource: 'providers' },
    { type: 'resource:new', resource: 'tasklists' },
    { type: 'tab:open', tabType: 'tasklist', contentId: '5' },
    { type: 'tab:open', tabType: 'editor', contentId: 'doc-1' },
    { type: 'tab:open', tabType: 'terminal', contentId: 'sess' },
    { type: 'tab:new', tabType: 'tasklist', title: 'Sprint' },
    { type: 'tab:new', tabType: 'editor' },
    { type: 'tab:new', tabType: 'editor', file: '/tmp/test.md' },
    { type: 'tab:new', tabType: 'terminal', cmd: 'ls -la' },
  ];

  for (const action of actions) {
    it(`roundtrip para ${action.type}${action.type === 'tab:new' && 'file' in action && (action as Record<string, unknown>).file ? ' (open)' : ''}`, () => {
      const uri = buildDeepLink(action);
      const parsed = parseDeepLink(uri);
      expect(parsed).toEqual(action);
    });
  }
});

// ─── getDeepLinkTypeClass ────────────────────────────────────────────

describe('getDeepLinkTypeClass', () => {
  it('retorna classe correta por tipo', () => {
    expect(getDeepLinkTypeClass({ type: 'conversation:open', conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001' }))
      .toBe('deep-link--conversation');
    expect(getDeepLinkTypeClass({ type: 'conversation:new' }))
      .toBe('deep-link--new-conversation');
    expect(getDeepLinkTypeClass({ type: 'conversation:send', conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001', message: 'x' }))
      .toBe('deep-link--send');
    expect(getDeepLinkTypeClass({ type: 'navigate', route: 'help' }))
      .toBe('deep-link--navigate');
    expect(getDeepLinkTypeClass({ type: 'resource:edit', resource: 'profiles', resourceId: 'x' }))
      .toBe('deep-link--resource-edit');
    expect(getDeepLinkTypeClass({ type: 'resource:new', resource: 'skills' }))
      .toBe('deep-link--resource-new');
    expect(getDeepLinkTypeClass({ type: 'tab:open', tabType: 'tasklist', contentId: '5' }))
      .toBe('deep-link--tasklist');
    expect(getDeepLinkTypeClass({ type: 'tab:open', tabType: 'editor', contentId: 'doc-1' }))
      .toBe('deep-link--editor');
    expect(getDeepLinkTypeClass({ type: 'tab:open', tabType: 'terminal', contentId: 'sess' }))
      .toBe('deep-link--terminal');
    expect(getDeepLinkTypeClass({ type: 'tab:new', tabType: 'tasklist' }))
      .toBe('deep-link--tasklist-new');
    expect(getDeepLinkTypeClass({ type: 'tab:new', tabType: 'editor', file: '/tmp/x.md' }))
      .toBe('deep-link--editor-new');
    expect(getDeepLinkTypeClass({ type: 'tab:new', tabType: 'terminal', cmd: 'ls' }))
      .toBe('deep-link--terminal-new');
  });
});

// ─── DEEP_LINK_PREFIX ────────────────────────────────────────────────

describe('DEEP_LINK_PREFIX', () => {
  it('é assistente://', () => {
    expect(DEEP_LINK_PREFIX).toBe('assistente://');
  });
});

// ─── executeDeepLink ─────────────────────────────────────────────────

describe('executeDeepLink', () => {
  const mockNavigate = vi.fn();
  const deps = { navigate: mockNavigate };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.useRealTimers();
    mockWsTabs = [];
    mockWsProfile = undefined;
    mockTerminalSessions = [];
    mockLoadTerminalSessions.mockResolvedValue(true);
    mockCreateTerminalSession.mockResolvedValue('term-created');
    mockWsAddTab.mockResolvedValue('tab-created');
    mockGetProfile.mockResolvedValue({ slug: 'programacao' });
  });

  describe('conversation:open — dedup', () => {
    it('ativa aba existente se a conversa já está aberta', async () => {
      mockWsTabs = [
        { id: 'tab-1', type: 'chat', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000002a' },
        { id: 'tab-2', type: 'chat', conversationId: '01926b90-7a5a-7c4e-8d3f-000000000063' },
      ];

      await executeDeepLink(
        { type: 'conversation:open', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000002a' },
        deps,
      );

      expect(mockWsSetActiveTab).toHaveBeenCalledWith('tab-1');
      expect(mockWsAddTab).not.toHaveBeenCalled();
      expect(mockNavigate).toHaveBeenCalledWith('/');
    });

    it('abre nova aba se a conversa não está aberta', async () => {
      mockWsTabs = [{ id: 'tab-1', type: 'chat', conversationId: '01926b90-7a5a-7c4e-8d3f-000000000063' }];

      await executeDeepLink(
        { type: 'conversation:open', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000002a' },
        deps,
      );

      expect(mockWsAddTab).toHaveBeenCalledWith('chat', 'Nova Conversa', { conversationId: '01926b90-7a5a-7c4e-8d3f-00000000002a' });
      expect(mockWsSetActiveTab).not.toHaveBeenCalled();
      expect(mockNavigate).toHaveBeenCalledWith('/');
    });

    it('abre nova aba quando não há abas', async () => {
      mockWsTabs = [];

      await executeDeepLink(
        { type: 'conversation:open', conversationId: '01926b90-7a5a-7c4e-8d3f-000000000007' },
        deps,
      );

      expect(mockWsAddTab).toHaveBeenCalledWith('chat', 'Nova Conversa', { conversationId: '01926b90-7a5a-7c4e-8d3f-000000000007' });
      expect(mockWsSetActiveTab).not.toHaveBeenCalled();
    });
  });

  describe('conversation:send — dedup', () => {
    it('ativa aba existente se a conversa já está aberta', async () => {
      mockWsTabs = [{ id: 'tab-5', type: 'chat', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a' }];

      await executeDeepLink(
        { type: 'conversation:send', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a', message: 'oi' },
        deps,
      );

      expect(mockWsSetActiveTab).toHaveBeenCalledWith('tab-5');
      expect(mockWsAddTab).not.toHaveBeenCalled();
      expect(mockLoadConversation).toHaveBeenCalledWith('01926b90-7a5a-7c4e-8d3f-00000000000a');
      expect(mockSendMessageToConversation).toHaveBeenCalledWith(
        '01926b90-7a5a-7c4e-8d3f-00000000000a',
        'oi',
        undefined,
        expect.objectContaining({ tabType: 'chat' }),
        {
          origin: expect.objectContaining({
            conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a',
            sessionKey: 'page:tab:tab-5:01926b90-7a5a-7c4e-8d3f-00000000000a',
            surfaceId: 'page:tab:tab-5',
            surfaceType: 'page',
            tabId: 'tab-5',
          }),
        },
      );
      expect(mockNavigate).toHaveBeenCalledWith('/');
    });

    it('abre nova aba se a conversa não está aberta', async () => {
      mockWsTabs = [];

      await executeDeepLink(
        { type: 'conversation:send', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a', message: 'oi' },
        deps,
      );

      expect(mockWsAddTab).toHaveBeenCalledWith('chat', 'Nova Conversa', { conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a' });
      expect(mockWsSetActiveTab).not.toHaveBeenCalled();
      expect(mockLoadConversation).toHaveBeenCalledWith('01926b90-7a5a-7c4e-8d3f-00000000000a');
      expect(mockSendMessageToConversation).toHaveBeenCalledWith(
        '01926b90-7a5a-7c4e-8d3f-00000000000a',
        'oi',
        undefined,
        expect.objectContaining({ tabType: 'chat' }),
        {
          origin: expect.objectContaining({
            conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a',
            sessionKey: 'page:tab:tab-created:01926b90-7a5a-7c4e-8d3f-00000000000a',
            surfaceId: 'page:tab:tab-created',
            surfaceType: 'page',
            tabId: 'tab-created',
          }),
        },
      );
    });
  });

  describe('conversation:new', () => {
    it('cria conversa e abre aba', async () => {
      await executeDeepLink({ type: 'conversation:new' }, deps);

      expect(mockCreateConversation).toHaveBeenCalledWith('Nova Conversa');
      expect(mockWsAddTab).toHaveBeenCalledWith('chat', 'Nova Conversa', { conversationId: '01926b90-7a5a-7c4e-8d3f-000000000064' });
      expect(mockNavigate).toHaveBeenCalledWith('/');
      expect(mockSendMessageToConversation).not.toHaveBeenCalled();
    });

    it('cria conversa e envia mensagem se fornecida', async () => {
      await executeDeepLink(
        { type: 'conversation:new', message: 'analise isso' },
        deps,
      );

      expect(mockCreateConversation).toHaveBeenCalled();
      expect(mockWsAddTab).toHaveBeenCalledWith('chat', 'Nova Conversa', { conversationId: '01926b90-7a5a-7c4e-8d3f-000000000064' });
      expect(mockSendMessageToConversation).toHaveBeenCalledWith(
        '01926b90-7a5a-7c4e-8d3f-000000000064',
        'analise isso',
        undefined,
        expect.objectContaining({ tabType: 'chat' }),
        {
          origin: expect.objectContaining({
            conversationId: '01926b90-7a5a-7c4e-8d3f-000000000064',
            sessionKey: 'page:tab:tab-created:01926b90-7a5a-7c4e-8d3f-000000000064',
            surfaceId: 'page:tab:tab-created',
            surfaceType: 'page',
            tabId: 'tab-created',
          }),
        },
      );
    });

    it('usa título customizado quando fornecido', async () => {
      await executeDeepLink(
        { type: 'conversation:new', title: 'Minha Análise' },
        deps,
      );

      expect(mockCreateConversation).toHaveBeenCalledWith('Minha Análise');
      expect(mockWsAddTab).toHaveBeenCalledWith('chat', 'Minha Análise', { conversationId: '01926b90-7a5a-7c4e-8d3f-000000000064' });
    });
  });

  describe('profile override', () => {
    it('conversation:new aplica profile como override da aba e usa no envio', async () => {
      await executeDeepLink(
        { type: 'conversation:new', message: 'analise o ticket', profile: 'techsupport' },
        deps,
      );

      expect(mockGetProfile).toHaveBeenCalledWith('techsupport');
      expect(mockWsUpdateTab).toHaveBeenCalledWith('tab-created', { profile_override: { slug: 'techsupport' } });
      expect(mockSendMessageToConversation).toHaveBeenCalledWith(
        '01926b90-7a5a-7c4e-8d3f-000000000064',
        'analise o ticket',
        undefined,
        expect.objectContaining({ profileSlug: 'techsupport' }),
        expect.anything(),
      );
    });

    it('conversation:open aplica profile como override da aba existente', async () => {
      mockWsTabs = [{ id: 'tab-1', type: 'chat', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000002a' }];

      await executeDeepLink(
        { type: 'conversation:open', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000002a', profile: 'programacao' },
        deps,
      );

      expect(mockWsSetActiveTab).toHaveBeenCalledWith('tab-1');
      expect(mockGetProfile).toHaveBeenCalledWith('programacao');
      expect(mockWsUpdateTab).toHaveBeenCalledWith('tab-1', { profile_override: { slug: 'programacao' } });
    });

    it('conversation:send aplica profile e o usa nos params do envio', async () => {
      mockWsTabs = [{ id: 'tab-5', type: 'chat', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a' }];

      await executeDeepLink(
        { type: 'conversation:send', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a', message: 'oi', profile: 'techsupport' },
        deps,
      );

      expect(mockWsUpdateTab).toHaveBeenCalledWith('tab-5', { profile_override: { slug: 'techsupport' } });
      expect(mockSendMessageToConversation).toHaveBeenCalledWith(
        '01926b90-7a5a-7c4e-8d3f-00000000000a',
        'oi',
        undefined,
        expect.objectContaining({ profileSlug: 'techsupport' }),
        expect.anything(),
      );
    });

    it('perfil inexistente: não aplica override, avisa via toast "warning" e cai no padrão', async () => {
      mockGetProfile.mockRejectedValueOnce(new Error('profile not found'));

      await executeDeepLink(
        { type: 'conversation:new', message: 'oi', profile: 'inexistente' },
        deps,
      );

      expect(mockGetProfile).toHaveBeenCalledWith('inexistente');
      expect(mockWsUpdateTab).not.toHaveBeenCalled();
      expect(mockAddToast).toHaveBeenCalledWith('deepLink.invalidProfile', 'warning', undefined, undefined, {
        suppressAnnounce: true,
      });
      expect(mockSendMessageToConversation).toHaveBeenCalledWith(
        '01926b90-7a5a-7c4e-8d3f-000000000064',
        'oi',
        undefined,
        expect.objectContaining({ profileSlug: undefined }),
        expect.anything(),
      );
    });

    it('falha inesperada ao carregar perfil: toast "error" genérico e cai no padrão', async () => {
      mockGetProfile.mockRejectedValueOnce(new Error('unexpected: failed to parse profile json'));

      await executeDeepLink(
        { type: 'conversation:new', message: 'oi', profile: 'quebrado' },
        deps,
      );

      expect(mockGetProfile).toHaveBeenCalledWith('quebrado');
      expect(mockWsUpdateTab).not.toHaveBeenCalled();
      expect(mockAddToast).toHaveBeenCalledWith('deepLink.profileLoadError', 'error', undefined, undefined, {
        suppressAnnounce: true,
      });
      expect(mockSendMessageToConversation).toHaveBeenCalledWith(
        '01926b90-7a5a-7c4e-8d3f-000000000064',
        'oi',
        undefined,
        expect.objectContaining({ profileSlug: undefined }),
        expect.anything(),
      );
    });

    it('sem profile no deeplink: não chama GetProfile nem updateTab', async () => {
      await executeDeepLink({ type: 'conversation:new', message: 'oi' }, deps);

      expect(mockGetProfile).not.toHaveBeenCalled();
      expect(mockWsUpdateTab).not.toHaveBeenCalled();
    });
  });

  describe('navigate', () => {
    it('navega para a rota informada', async () => {
      await executeDeepLink({ type: 'navigate', route: 'history' }, deps);
      expect(mockNavigate).toHaveBeenCalledWith('/history');
    });

    it('navega para raiz quando rota é vazia', async () => {
      await executeDeepLink({ type: 'navigate', route: '' }, deps);
      expect(mockNavigate).toHaveBeenCalledWith('/');
    });
  });

  describe('resource:edit', () => {
    it('navega para a página e solicita edição no store', async () => {
      await executeDeepLink(
        { type: 'resource:edit', resource: 'profiles', resourceId: 'programacao' },
        deps,
      );

      expect(mockRequestResourceEdit).toHaveBeenCalledWith('profiles', 'programacao', 'edit');
      expect(mockNavigate).toHaveBeenCalledWith('/profiles');
      expect(mockAnnounce).toHaveBeenCalled();
    });

    it('navega para memories como página first-level', async () => {
      await executeDeepLink(
        { type: 'resource:edit', resource: 'memories', resourceId: 'mem-1' },
        deps,
      );

      expect(mockRequestResourceEdit).toHaveBeenCalledWith('memories', 'mem-1', 'edit');
      expect(mockNavigate).toHaveBeenCalledWith('/memories');
      expect(mockAnnounce).toHaveBeenCalled();
    });
  });

  describe('resource:new', () => {
    it('navega para settings/skills e solicita criação no store', async () => {
      await executeDeepLink(
        { type: 'resource:new', resource: 'skills' },
        deps,
      );

      expect(mockRequestResourceEdit).toHaveBeenCalledWith('skills', '', 'new');
      expect(mockNavigate).toHaveBeenCalledWith('/settings/skills');
      expect(mockAnnounce).toHaveBeenCalled();
    });

    it('navega para memories como página first-level', async () => {
      await executeDeepLink(
        { type: 'resource:new', resource: 'memories' },
        deps,
      );

      expect(mockRequestResourceEdit).toHaveBeenCalledWith('memories', '', 'new');
      expect(mockNavigate).toHaveBeenCalledWith('/memories');
      expect(mockAnnounce).toHaveBeenCalled();
    });
  });

  describe('tab:open — dedup', () => {
    it('ativa aba existente se a tab já está aberta (tasklist)', async () => {
      mockWsTabs = [
        { id: 'tab-10', type: 'tasklist', state: { tasklistId: '5' } },
      ];

      await executeDeepLink(
        { type: 'tab:open', tabType: 'tasklist', contentId: '5' },
        deps,
      );

      expect(mockWsSetActiveTab).toHaveBeenCalledWith('tab-10');
      expect(mockWsAddTab).not.toHaveBeenCalled();
      expect(mockNavigate).toHaveBeenCalledWith('/');
    });

    it('cria nova aba de tasklist se não está aberta', async () => {
      mockWsTabs = [];

      await executeDeepLink(
        { type: 'tab:open', tabType: 'tasklist', contentId: '5' },
        deps,
      );

      expect(mockWsAddTab).toHaveBeenCalledWith('tasklist', 'tasklist 5', { tasklistId: '5' });
      expect(mockWsSetActiveTab).not.toHaveBeenCalled();
      expect(mockNavigate).toHaveBeenCalledWith('/');
    });

    it('diferencia tabs pelo tipo', async () => {
      mockWsTabs = [
        { id: 'tab-20', type: 'editor' },
      ];

      await executeDeepLink(
        { type: 'tab:open', tabType: 'tasklist', contentId: '5' },
        deps,
      );

      expect(mockWsAddTab).toHaveBeenCalledWith('tasklist', 'tasklist 5', { tasklistId: '5' });
      expect(mockWsSetActiveTab).not.toHaveBeenCalled();
    });

    it('abre exatamente o terminal vivo solicitado', async () => {
      mockTerminalSessions = [{ id: 'terminal-live' }];

      await executeDeepLink(
        { type: 'tab:open', tabType: 'terminal', contentId: 'terminal-live' },
        deps,
      );

      expect(mockLoadTerminalSessions).toHaveBeenCalled();
      expect(mockWsAddTab).toHaveBeenCalledWith(
        'terminal',
        'terminal terminal-live',
        { sessionId: 'terminal-live' },
      );
    });

    it('não substitui silenciosamente um terminal encerrado', async () => {
      await executeDeepLink(
        { type: 'tab:open', tabType: 'terminal', contentId: 'terminal-dead' },
        deps,
      );

      expect(mockWsAddTab).not.toHaveBeenCalled();
      expect(mockWsSetActiveTab).not.toHaveBeenCalled();
      expect(mockAddToast).toHaveBeenCalledWith(
        'deepLink.terminalUnavailable',
        'warning',
        undefined,
        undefined,
        { suppressAnnounce: true },
      );
    });

    it('diferencia falha de listagem de terminal encerrado', async () => {
      mockLoadTerminalSessions.mockResolvedValueOnce(false);

      await executeDeepLink(
        { type: 'tab:open', tabType: 'terminal', contentId: 'terminal-unknown' },
        deps,
      );

      expect(mockWsAddTab).not.toHaveBeenCalled();
      expect(mockAddToast).toHaveBeenCalledWith(
        'deepLink.terminalListFailed',
        'error',
        undefined,
        undefined,
        { suppressAnnounce: true },
      );
    });
  });

  describe('tab:new', () => {
    it('cria nova aba de tasklist', async () => {
      await executeDeepLink(
        { type: 'tab:new', tabType: 'tasklist', title: 'Sprint 23' },
        deps,
      );

      expect(mockWsAddTab).toHaveBeenCalledWith('tasklist', 'Sprint 23');
      expect(mockNavigate).toHaveBeenCalledWith('/');
    });

    it('cria nova aba de editor vazio', async () => {
      await executeDeepLink(
        { type: 'tab:new', tabType: 'editor' },
        deps,
      );

      expect(mockWsAddTab).toHaveBeenCalledWith('editor', 'Novo Editor');
      expect(mockNavigate).toHaveBeenCalledWith('/');
    });

    it('cria aba de editor com arquivo', async () => {
      mockWsAddTab.mockResolvedValueOnce('tab-new-editor');
      await executeDeepLink(
        { type: 'tab:new', tabType: 'editor', file: '/tmp/test.md' },
        deps,
      );

      expect(mockEditorReadFile).toHaveBeenCalledWith('/tmp/test.md');
      expect(mockWsAddTab).toHaveBeenCalledWith('editor', 'test.md', { filePath: '/tmp/test.md' });
      expect(mockCreateDocument).toHaveBeenCalledWith({
        id: 'tab-new-editor',
        title: 'test.md',
        markdown: 'file content',
        filePath: '/tmp/test.md',
      });
      expect(mockNavigate).toHaveBeenCalledWith('/');
    });

    it('cria nova aba de terminal', async () => {
      mockWsAddTab.mockResolvedValueOnce('tab-new-term');
      await executeDeepLink(
        { type: 'tab:new', tabType: 'terminal' },
        deps,
      );

      expect(mockCreateTerminalSession).toHaveBeenCalled();
      expect(mockWsAddTab).toHaveBeenCalledWith('terminal', 'Terminal', { sessionId: 'term-created' });
      expect(mockRunTerminalCommand).not.toHaveBeenCalled();
      expect(mockNavigate).toHaveBeenCalledWith('/');
    });

    it('cria terminal antes de executar cmd do deep link', async () => {
      await executeDeepLink(
        { type: 'tab:new', tabType: 'terminal', title: 'Build', cmd: 'go test ./...' },
        deps,
      );

      expect(mockCreateTerminalSession).toHaveBeenCalledWith('Build');
      expect(mockWsAddTab).toHaveBeenCalledWith('terminal', 'Build', { sessionId: 'term-created' });
      expect(mockRunTerminalCommand).toHaveBeenCalledWith('term-created', 'go test ./...');
    });
  });

  describe('resource:new tasklists', () => {
    it('navega para tasklists e solicita criação', async () => {
      await executeDeepLink(
        { type: 'resource:new', resource: 'tasklists' },
        deps,
      );

      expect(mockRequestResourceEdit).toHaveBeenCalledWith('tasklists', '', 'new');
      expect(mockNavigate).toHaveBeenCalledWith('/tasklists');
    });
  });

  describe('resource:edit tasklists', () => {
    it('navega para tasklists e solicita edição', async () => {
      await executeDeepLink(
        { type: 'resource:edit', resource: 'tasklists', resourceId: '42' },
        deps,
      );

      expect(mockRequestResourceEdit).toHaveBeenCalledWith('tasklists', '42', 'edit');
      expect(mockNavigate).toHaveBeenCalledWith('/tasklists');
    });
  });

  describe('announce', () => {
    it('anuncia após cada ação', async () => {
      await executeDeepLink(
        { type: 'conversation:open', conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001' },
        deps,
      );
      expect(mockAnnounce).toHaveBeenCalled();
    });
  });
});
