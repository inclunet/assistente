import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render } from '@testing-library/react';
import { useDocumentTitle } from './useDocumentTitle';

const setTitleSpy = vi.fn();
let mockPathname = '/';

vi.mock('react-router-dom', () => ({
  useLocation: () => ({ pathname: mockPathname }),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'menu.mcp': 'MCP',
        'menu.profiles': 'Perfis',
        'menu.history': 'Hist\u00f3rico',
        'menu.help': 'Ajuda',
        'menu.about': 'Sobre',
        'menu.allowlists': 'Allowlists',
        'menu.skills': 'Skills',
        'menu.channels': 'Canais',
        'menu.credentials': 'Credenciais',
        'menu.providers': 'Provedores LLM',
        'menu.restoreDefaults': 'Restaurar Padr\u00f5es',
        'menu.appTitle': 'Assistente IA',
        'chat.newConversation': 'Nova conversa',
        'update.pageTitle': 'Atualiza\u00e7\u00e3o',
      };
      return translations[key] ?? key;
    },
  }),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  WindowSetTitle: (title: string) => setTitleSpy(title),
}));

// --- workspace store mock ---
type MockTab = { id: string; type: string; title: string; contentId: string; position: number };
let mockActiveTab: MockTab | undefined = undefined;

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: (selector: (state: { getActiveTab: () => MockTab | undefined }) => unknown) =>
    selector({ getActiveTab: () => mockActiveTab }),
}));

// --- chat store mock ---
let mockConversationTitle: string | undefined = undefined;

vi.mock('../store/chatStore', () => ({
  useChatStore: (selector: (state: { activeConversation: { title: string } | null }) => unknown) =>
    selector({
      activeConversation: mockConversationTitle !== undefined ? { title: mockConversationTitle } : null,
    }),
}));

function Fixture() {
  useDocumentTitle();
  return null;
}

const SEP = '\u2014';

describe('useDocumentTitle', () => {
  beforeEach(() => {
    mockPathname = '/';
    mockActiveTab = undefined;
    mockConversationTitle = undefined;
    setTitleSpy.mockClear();
  });

  it('usa conversationTitle do chatStore para chat tabs', () => {
    mockActiveTab = { id: '1', type: 'chat', title: 'Nova conversa', contentId: '1', position: 0 };
    mockConversationTitle = 'Conversa Renomeada';
    render(<Fixture />);

    expect(document.title).toBe(`Conversa Renomeada ${SEP} Assistente IA`);
    expect(setTitleSpy).toHaveBeenCalledWith(`Conversa Renomeada ${SEP} Assistente IA`);
  });

  it('usa titulo da aba do workspace quando chatStore tem titulo padrao', () => {
    mockActiveTab = { id: '1', type: 'chat', title: 'Título da Aba', contentId: '1', position: 0 };
    mockConversationTitle = 'Nova Conversa';
    render(<Fixture />);

    expect(document.title).toBe(`Título da Aba ${SEP} Assistente IA`);
  });

  it('mostra Nova conversa quando nao ha aba ativa nem conversa', () => {
    render(<Fixture />);
    expect(document.title).toBe(`Nova conversa ${SEP} Assistente IA`);
  });

  it('mostra Nova conversa quando titulo da aba é padrao', () => {
    mockActiveTab = { id: '1', type: 'chat', title: 'Nova conversa', contentId: '', position: 0 };
    render(<Fixture />);
    expect(document.title).toBe(`Nova conversa ${SEP} Assistente IA`);
  });

  it('usa titulo da aba para tabs nao-chat (terminal)', () => {
    mockActiveTab = { id: '2', type: 'terminal', title: 'Meu Terminal', contentId: '', position: 0 };
    render(<Fixture />);
    expect(document.title).toBe(`Meu Terminal ${SEP} Assistente IA`);
  });

  it('usa titulo da aba para tabs nao-chat (editor)', () => {
    mockActiveTab = { id: '3', type: 'editor', title: 'README.md', contentId: '', position: 0 };
    render(<Fixture />);
    expect(document.title).toBe(`README.md ${SEP} Assistente IA`);
  });

  it('define titulo para pagina MCP', () => {
    mockPathname = '/mcp';
    render(<Fixture />);

    expect(document.title).toBe(`MCP ${SEP} Assistente IA`);
    expect(setTitleSpy).toHaveBeenCalledWith(`MCP ${SEP} Assistente IA`);
  });

  it('define titulo para pagina de perfis', () => {
    mockPathname = '/profiles';
    render(<Fixture />);

    expect(document.title).toBe(`Perfis ${SEP} Assistente IA`);
  });

  it('define titulo para pagina de historico', () => {
    mockPathname = '/history';
    render(<Fixture />);

    expect(document.title).toBe(`Hist\u00f3rico ${SEP} Assistente IA`);
  });

  it('define titulo para pagina sobre', () => {
    mockPathname = '/about';
    render(<Fixture />);

    expect(document.title).toBe(`Sobre ${SEP} Assistente IA`);
  });

  it('usa pathname para rota desconhecida', () => {
    mockPathname = '/unknown-page';
    render(<Fixture />);

    expect(document.title).toBe(`unknown-page ${SEP} Assistente IA`);
  });
});
