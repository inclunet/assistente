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
        'menu.settings': 'Configura\u00e7\u00f5es',
        'menu.profiles': 'Perfis',
        'menu.history': 'Hist\u00f3rico',
        'menu.memories': 'Mem\u00f3rias',
        'menu.help': 'Ajuda',
        'menu.about': 'Sobre',
        'menu.appTitle': 'Assistente IA',
        'chat.newConversation': 'Nova conversa',
        'update.pageTitle': 'Atualiza\u00e7\u00e3o',
      };
      return translations[key] ?? key;
    },
  }),
}));

vi.mock('../lib/i18n', () => ({
  default: {
    t: (key: string) => (key === 'chat.newConversation' ? 'Nova conversa' : key),
  },
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  WindowSetTitle: (title: string) => setTitleSpy(title),
}));

type MockTab = { id: string; type: string; title: string; position: number; conversationId?: string; state?: Record<string, unknown> };
let mockActiveTab: MockTab | undefined = undefined;

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: (selector: (state: { getActiveTab: () => MockTab | undefined }) => unknown) =>
    selector({ getActiveTab: () => mockActiveTab }),
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
    setTitleSpy.mockClear();
  });

  it('usa titulo da aba de chat renomeada', () => {
    mockActiveTab = { id: '1', type: 'chat', title: 'Conversa Renomeada', conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001', position: 0 };
    render(<Fixture />);

    expect(document.title).toBe(`Conversa Renomeada ${SEP} Assistente IA`);
    expect(setTitleSpy).toHaveBeenCalledWith(`Conversa Renomeada ${SEP} Assistente IA`);
  });

  it('mostra Nova conversa quando nao ha aba ativa', () => {
    render(<Fixture />);
    expect(document.title).toBe(`Nova conversa ${SEP} Assistente IA`);
  });

  it('mostra Nova conversa quando titulo da aba é padrao', () => {
    mockActiveTab = { id: '1', type: 'chat', title: 'Nova conversa', position: 0 };
    render(<Fixture />);
    expect(document.title).toBe(`Nova conversa ${SEP} Assistente IA`);
  });

  it('mostra Nova conversa com capitalizacao diferente', () => {
    mockActiveTab = { id: '1', type: 'chat', title: 'Nova Conversa', position: 0 };
    render(<Fixture />);
    expect(document.title).toBe(`Nova conversa ${SEP} Assistente IA`);
  });

  it('usa titulo da aba para terminal', () => {
    mockActiveTab = { id: '2', type: 'terminal', title: 'Meu Terminal', position: 0 };
    render(<Fixture />);
    expect(document.title).toBe(`Meu Terminal ${SEP} Assistente IA`);
  });

  it('usa titulo da aba para editor', () => {
    mockActiveTab = { id: '3', type: 'editor', title: 'README.md', position: 0 };
    render(<Fixture />);
    expect(document.title).toBe(`README.md ${SEP} Assistente IA`);
  });

  it('usa titulo da aba para tasklist', () => {
    mockActiveTab = { id: '4', type: 'tasklist', title: 'Compras da semana', state: { tasklistId: '5' }, position: 0 };
    render(<Fixture />);
    expect(document.title).toBe(`Compras da semana ${SEP} Assistente IA`);
  });

  it('define titulo para pagina de configurações', () => {
    mockPathname = '/settings';
    render(<Fixture />);

    expect(document.title).toBe(`Configurações ${SEP} Assistente IA`);
    expect(setTitleSpy).toHaveBeenCalledWith(`Configurações ${SEP} Assistente IA`);
  });

  it('define titulo para sub-pagina de configurações', () => {
    mockPathname = '/settings/mcp';
    render(<Fixture />);

    expect(document.title).toBe(`Configurações ${SEP} Assistente IA`);
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

  it('define titulo para pagina de memorias', () => {
    mockPathname = '/memories';
    render(<Fixture />);

    expect(document.title).toBe(`Mem\u00f3rias ${SEP} Assistente IA`);
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
