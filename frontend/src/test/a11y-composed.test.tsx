import { describe, expect, it, vi } from 'vitest';
import { render } from '@testing-library/react';
import { axe } from './a11yAxe';
import { Toolbar, ToolbarButton } from '../components/ui/Toolbar';
import { DataGrid } from '../components/ui/DataGrid';
import { ContextMenu } from '../components/menu';
import { Topbar } from '../components/layout/Topbar';
import { WorkspaceTabList } from '../components/workspace/WorkspaceTabList';
import { Button } from '../components/ui/Button';
import { Checkbox } from '../components/ui/Checkbox';
import { Input } from '../components/ui/Input';
import { Select } from '../components/ui/Select';
import { Textarea } from '../components/ui/Textarea';

const { workspaceState } = vi.hoisted(() => {
  const workspaceState = {
    workspace: {
      id: 'ws-1',
      name: 'Test Workspace',
      profile: '',
      tabs: [
        {
          id: 't1',
          type: 'chat' as const,
          conversationId: 1,
          title: 'Chat 1',
          position: 0,
        },
        {
          id: 't2',
          type: 'editor' as const,
          state: { filePath: '/tmp/c2.md' },
          title: 'Editor',
          position: 1,
        },
      ],
      activeTabId: 't1',
    },
    workspaces: [] as Array<{ id: string; name: string; is_active: boolean; tab_count: number }>,
    switchWorkspace: vi.fn(),
    createWorkspace: vi.fn(),
    renameWorkspace: vi.fn(),
    setActiveTab: vi.fn(),
    removeTab: vi.fn(),
    updateTab: vi.fn(),
    reorderTabs: vi.fn(),
    moveTabToWorkspace: vi.fn(),
    renameTabContent: vi.fn(),
  };
  return { workspaceState };
});

const navigateSpy = vi.fn();
const toggleMenuSpy = vi.fn();
const announceSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fb?: string) => fb || key,
    i18n: { language: 'pt-BR', changeLanguage: vi.fn() },
  }),
}));

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom');
  return {
    ...actual,
    useNavigate: () => navigateSpy,
    useLocation: () => ({ pathname: '/history' }),
  };
});

vi.mock('../store/settingsStore', () => ({
  useSettingsStore: (selector: (state: { updateConfig: (cfg: unknown) => void }) => unknown) =>
    selector({ updateConfig: vi.fn() }),
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign(
    (selector?: (state: typeof workspaceState) => unknown) =>
      selector ? selector(workspaceState) : workspaceState,
    {
      getState: () => ({
        exportWorkspace: vi.fn(),
        importWorkspace: vi.fn(),
      }),
    },
  ),
}));

vi.mock('../hooks/useAnchoredContextMenu', () => ({
  useAnchoredContextMenu: () => ({
    menu: { visible: false, items: [], x: 0, y: 0, ariaLabel: '' },
    openForTrigger: vi.fn(),
    openAtPoint: vi.fn(),
    closeMenu: vi.fn(),
    onSelectItem: vi.fn(),
  }),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceSpy }),
}));

vi.mock('../services/audioFeedback', () => ({
  playBumpSound: vi.fn(),
}));

vi.mock('../components/layout/MenuButton', () => {
  const React = require('react');
  const MenuButton = React.forwardRef(
    (
      props: {
        items: Array<{ id: string; onClick?: () => void }>;
        buttonLabel: string;
        currentItemId: string;
      },
      ref: React.Ref<{ toggleMenu: () => void }>,
    ) => {
      React.useImperativeHandle(ref, () => ({ toggleMenu: toggleMenuSpy }));

      return (
        <div>
          <button type="button" onClick={() => props.items[0]?.onClick?.()}>
            {props.buttonLabel}
          </button>
          <div data-testid="current-item">{props.currentItemId}</div>
          <div data-testid="menu-items">{props.items.map((item) => item.id).join(',')}</div>
        </div>
      );
    },
  );

  return { MenuButton, MenuItem: {}, MenuButtonRef: {} };
});

describe('a11y componentes compostos', () => {
  it('Toolbar com busca e botoes nao tem violacoes', async () => {
    const { container } = render(
      <Toolbar
        ariaLabel="Barra de ferramentas"
        searchValue=""
        onSearchChange={() => {}}
        searchPlaceholder="Buscar..."
        left={<ToolbarButton label="Ação" onClick={() => {}} />}
      />,
    );
    expect(await axe(container)).toHaveNoViolations();
  });

  it('DataGrid com items e colunas nao tem violacoes', async () => {
    const { container } = render(
      <DataGrid
        items={[
          { id: '1', name: 'Item A', status: 'Ativo' },
          { id: '2', name: 'Item B', status: 'Inativo' },
        ]}
        columns={[
          { key: 'name', label: 'Nome' },
          { key: 'status', label: 'Status' },
        ]}
        label="Tabela de teste"
        autoFocusOnMount={false}
      />,
    );
    expect(await axe(container)).toHaveNoViolations();
  });

  it('DataGrid vazio nao tem violacoes', async () => {
    const { container } = render(
      <DataGrid items={[]} columns={[{ key: 'name', label: 'Nome' }]} label="Tabela vazia" />,
    );
    expect(await axe(container)).toHaveNoViolations();
  });

  it('ContextMenu visivel com items nao tem violacoes', async () => {
    const { container } = render(
      <ContextMenu
        visible
        items={[
          { id: 'edit', label: 'Editar' },
          { id: 'delete', label: 'Excluir' },
        ]}
        x={100}
        y={100}
        ariaLabel="Menu de acoes"
        onClose={() => {}}
      />,
    );
    expect(await axe(container)).toHaveNoViolations();
  });

  it('Topbar nao tem violacoes', async () => {
    const { container } = render(<Topbar />);
    expect(await axe(container)).toHaveNoViolations();
  });

  it('WorkspaceTabList com abas nao tem violacoes', async () => {
    const { container } = render(<WorkspaceTabList />);
    expect(await axe(container)).toHaveNoViolations();
  });

  it('Formulario composto (inputs + select + textarea) nao tem violacoes', async () => {
    const { container } = render(
      <form aria-label="Formulario de teste">
        <fieldset>
          <legend>Dados gerais</legend>
          <Input label="Nome" value="" onChange={() => {}} />
          <Select label="Tipo" value="a" onChange={() => {}} options={[{ value: 'a', label: 'A' }]} />
          <Textarea label="Descricao" value="" onChange={() => {}} />
          <Checkbox label="Ativo" checked={false} onChange={() => {}} />
        </fieldset>
        <Button type="submit">Salvar</Button>
      </form>,
    );
    expect(await axe(container)).toHaveNoViolations();
  });
});
