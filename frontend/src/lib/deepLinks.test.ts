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

const mockSetActiveTab = vi.fn().mockResolvedValue(undefined);
const mockOpenConversationInNewTab = vi.fn().mockResolvedValue(undefined);
const mockCreateTab = vi.fn().mockResolvedValue('new-tab-id');
const mockSendMessage = vi.fn().mockResolvedValue(undefined);

let mockTabs: Array<{ id: string; conversationId?: number }> = [];

vi.mock('../store/chatStore', () => ({
  useChatStore: {
    getState: () => ({
      tabs: mockTabs,
      setActiveTab: mockSetActiveTab,
      openConversationInNewTab: mockOpenConversationInNewTab,
      createTab: mockCreateTab,
      sendMessage: mockSendMessage,
    }),
  },
}));

const mockAnnounce = vi.fn();
vi.mock('../hooks/useAnnouncer', () => ({
  announce: (...args: unknown[]) => mockAnnounce(...args),
}));

vi.mock('./i18n', () => ({
  default: { t: (key: string) => key },
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
    it('faz parse de assistente://conversation/{id}', () => {
      const result = parseDeepLink('assistente://conversation/42');
      expect(result).toEqual({ type: 'conversation:open', conversationId: 42 });
    });

    it('aceita IDs grandes', () => {
      const result = parseDeepLink('assistente://conversation/999999');
      expect(result).toEqual({ type: 'conversation:open', conversationId: 999999 });
    });

    it('rejeita ID zero', () => {
      expect(parseDeepLink('assistente://conversation/0')).toBeNull();
    });

    it('rejeita ID negativo', () => {
      expect(parseDeepLink('assistente://conversation/-1')).toBeNull();
    });

    it('rejeita ID não numérico', () => {
      expect(parseDeepLink('assistente://conversation/abc')).toBeNull();
    });

    it('rejeita ID float', () => {
      expect(parseDeepLink('assistente://conversation/3.14')).toBeNull();
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
  });

  describe('conversation:send', () => {
    it('faz parse de send com message', () => {
      const result = parseDeepLink(
        'assistente://conversation/10/send?message=continue+aqui',
      );
      expect(result).toEqual({
        type: 'conversation:send',
        conversationId: 10,
        message: 'continue aqui',
      });
    });

    it('rejeita send sem message', () => {
      expect(parseDeepLink('assistente://conversation/10/send')).toBeNull();
    });

    it('rejeita send com ID inválido', () => {
      expect(parseDeepLink('assistente://conversation/abc/send?message=oi')).toBeNull();
    });
  });

  describe('navigate', () => {
    it('faz parse de rotas válidas', () => {
      const validRoutes = [
        'terminal', 'editor', 'allowlists', 'skills', 'mcp',
        'channels', 'credentials', 'providers', 'settings', 'profiles',
        'history', 'help', 'about', 'update',
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
    const uri = buildDeepLink({ type: 'conversation:open', conversationId: 42 });
    expect(uri).toBe('assistente://conversation/42');
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
      conversationId: 10,
      message: 'continue aqui',
    });
    expect(uri).toContain('assistente://conversation/10/send?');
    expect(uri).toContain('message=continue+aqui');
  });

  it('constrói navigate', () => {
    const uri = buildDeepLink({ type: 'navigate', route: 'history' });
    expect(uri).toBe('assistente://navigate/history');
  });

  it('constrói navigate para raiz', () => {
    const uri = buildDeepLink({ type: 'navigate', route: '' });
    expect(uri).toBe('assistente://navigate/');
  });
});

// ─── roundtrip: build → parse ────────────────────────────────────────

describe('roundtrip build → parse', () => {
  const actions: DeepLinkAction[] = [
    { type: 'conversation:open', conversationId: 7 },
    { type: 'conversation:new', message: 'olá mundo', title: 'Test' },
    { type: 'conversation:new' },
    { type: 'conversation:send', conversationId: 3, message: 'continue' },
    { type: 'navigate', route: 'history' },
    { type: 'navigate', route: '' },
  ];

  for (const action of actions) {
    it(`roundtrip para ${action.type}`, () => {
      const uri = buildDeepLink(action);
      const parsed = parseDeepLink(uri);
      expect(parsed).toEqual(action);
    });
  }
});

// ─── getDeepLinkTypeClass ────────────────────────────────────────────

describe('getDeepLinkTypeClass', () => {
  it('retorna classe correta por tipo', () => {
    expect(getDeepLinkTypeClass({ type: 'conversation:open', conversationId: 1 }))
      .toBe('deep-link--conversation');
    expect(getDeepLinkTypeClass({ type: 'conversation:new' }))
      .toBe('deep-link--new-conversation');
    expect(getDeepLinkTypeClass({ type: 'conversation:send', conversationId: 1, message: 'x' }))
      .toBe('deep-link--send');
    expect(getDeepLinkTypeClass({ type: 'navigate', route: 'help' }))
      .toBe('deep-link--navigate');
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
    mockTabs = [];
  });

  describe('conversation:open — dedup', () => {
    it('ativa aba existente se a conversa já está aberta', async () => {
      mockTabs = [
        { id: 'tab-1', conversationId: 42 },
        { id: 'tab-2', conversationId: 99 },
      ];

      await executeDeepLink(
        { type: 'conversation:open', conversationId: 42 },
        deps,
      );

      expect(mockSetActiveTab).toHaveBeenCalledWith('tab-1');
      expect(mockOpenConversationInNewTab).not.toHaveBeenCalled();
      expect(mockNavigate).toHaveBeenCalledWith('/');
    });

    it('abre nova aba se a conversa não está aberta', async () => {
      mockTabs = [{ id: 'tab-1', conversationId: 99 }];

      await executeDeepLink(
        { type: 'conversation:open', conversationId: 42 },
        deps,
      );

      expect(mockOpenConversationInNewTab).toHaveBeenCalledWith(42);
      expect(mockSetActiveTab).not.toHaveBeenCalled();
      expect(mockNavigate).toHaveBeenCalledWith('/');
    });

    it('abre nova aba quando não há abas', async () => {
      mockTabs = [];

      await executeDeepLink(
        { type: 'conversation:open', conversationId: 7 },
        deps,
      );

      expect(mockOpenConversationInNewTab).toHaveBeenCalledWith(7);
      expect(mockSetActiveTab).not.toHaveBeenCalled();
    });
  });

  describe('conversation:send — dedup', () => {
    it('ativa aba existente se a conversa já está aberta', async () => {
      vi.useFakeTimers();
      mockTabs = [{ id: 'tab-5', conversationId: 10 }];

      const promise = executeDeepLink(
        { type: 'conversation:send', conversationId: 10, message: 'oi' },
        deps,
      );

      await vi.advanceTimersByTimeAsync(150);
      await promise;

      expect(mockSetActiveTab).toHaveBeenCalledWith('tab-5');
      expect(mockOpenConversationInNewTab).not.toHaveBeenCalled();
      expect(mockSendMessage).toHaveBeenCalledWith('oi');
      expect(mockNavigate).toHaveBeenCalledWith('/');
    });

    it('abre nova aba se a conversa não está aberta', async () => {
      vi.useFakeTimers();
      mockTabs = [];

      const promise = executeDeepLink(
        { type: 'conversation:send', conversationId: 10, message: 'oi' },
        deps,
      );

      await vi.advanceTimersByTimeAsync(150);
      await promise;

      expect(mockOpenConversationInNewTab).toHaveBeenCalledWith(10);
      expect(mockSetActiveTab).not.toHaveBeenCalled();
      expect(mockSendMessage).toHaveBeenCalledWith('oi');
    });
  });

  describe('conversation:new', () => {
    it('cria nova aba e navega para chat', async () => {
      await executeDeepLink({ type: 'conversation:new' }, deps);

      expect(mockCreateTab).toHaveBeenCalledWith(true);
      expect(mockNavigate).toHaveBeenCalledWith('/');
      expect(mockSendMessage).not.toHaveBeenCalled();
    });

    it('cria nova aba e envia mensagem se fornecida', async () => {
      vi.useFakeTimers();

      const promise = executeDeepLink(
        { type: 'conversation:new', message: 'analise isso' },
        deps,
      );

      await vi.advanceTimersByTimeAsync(150);
      await promise;

      expect(mockCreateTab).toHaveBeenCalledWith(true);
      expect(mockSendMessage).toHaveBeenCalledWith('analise isso');
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

  describe('announce', () => {
    it('anuncia após cada ação', async () => {
      await executeDeepLink(
        { type: 'conversation:open', conversationId: 1 },
        deps,
      );
      expect(mockAnnounce).toHaveBeenCalled();
    });
  });
});
