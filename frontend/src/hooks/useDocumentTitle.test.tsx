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
        'menu.chat': 'Chat',
        'menu.terminal': 'Terminal',
        'menu.editor': 'Editor',
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

type ChatStoreState = {
  activeConversation: { id: number; title: string } | null;
};

let mockActiveConversation: { id: number; title: string } | null = null;

vi.mock('../store/chatStore', () => ({
  useChatStore: (selector: (state: ChatStoreState) => unknown) => selector({
    activeConversation: mockActiveConversation,
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
    mockActiveConversation = null;
    setTitleSpy.mockClear();
  });

  it('define titulo para chat com conversa ativa', () => {
    mockActiveConversation = { id: 1, title: 'Conversa A' };
    render(<Fixture />);

    expect(document.title).toBe(`Conversa A ${SEP} Assistente IA`);
    expect(setTitleSpy).toHaveBeenCalledWith(`Conversa A ${SEP} Assistente IA`);
  });

  it('mostra Nova conversa quando conversa tem titulo padrao', () => {
    mockActiveConversation = { id: 1, title: 'Nova conversa' };
    render(<Fixture />);

    expect(document.title).toBe(`Nova conversa ${SEP} Assistente IA`);
  });

  it('mostra Nova conversa quando conversa nao tem titulo', () => {
    mockActiveConversation = { id: 1, title: '' };
    render(<Fixture />);

    expect(document.title).toBe(`Nova conversa ${SEP} Assistente IA`);
  });

  it('mostra Nova conversa com capitalizacao diferente', () => {
    mockActiveConversation = { id: 1, title: 'Nova Conversa' };
    render(<Fixture />);

    expect(document.title).toBe(`Nova conversa ${SEP} Assistente IA`);
  });

  it('mostra Nova conversa quando nao ha conversa ativa', () => {
    mockActiveConversation = null;
    render(<Fixture />);

    expect(document.title).toBe(`Nova conversa ${SEP} Assistente IA`);
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
