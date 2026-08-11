import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// Diferente do WorkspaceContent.test.tsx, aqui o store NÃO é mockado: o defeito
// que este arquivo cobre vive justamente na ponte entre o seletor e o
// useSyncExternalStore do zustand, que um mock síncrono do hook esconde.

vi.mock('@wailsjs/go/app/App', () => ({
  GetActiveWorkspace: vi.fn(),
  ListWorkspaces: vi.fn(),
  CreateWorkspace: vi.fn(),
  SwitchWorkspace: vi.fn(),
  RenameWorkspace: vi.fn(),
  DeleteWorkspace: vi.fn(),
  SetWorkspaceProfile: vi.fn(),
  AddWorkspaceTab: vi.fn(),
  RemoveWorkspaceTab: vi.fn(),
  SetActiveWorkspaceTab: vi.fn(),
  UpdateWorkspaceTab: vi.fn(),
  ReorderWorkspaceTabs: vi.fn(),
  MoveWorkspaceTabTo: vi.fn(),
  ExportWorkspace: vi.fn(),
  ImportWorkspace: vi.fn(),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(() => vi.fn()),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  announce: vi.fn(),
}));

vi.mock('../../lib/modalRegistry', () => ({
  isModalOpen: vi.fn(() => false),
}));

vi.mock('../../lib/waitForWailsBridge', () => ({
  waitForWailsBridge: vi.fn(),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (_key: string, fallback?: string) => fallback ?? _key }),
}));

vi.mock('./workspacePanelRegistry', () => ({
  WorkspaceDomainPanel: () => <div>panel</div>,
}));

import { WorkspaceContent } from './WorkspaceContent';
import { useWorkspaceStore } from '../../store/workspaceStore';

describe('WorkspaceContent no boot', () => {
  beforeEach(() => {
    useWorkspaceStore.setState({ workspace: null, workspaces: [], isInitialized: false });
  });

  it('renderiza sem entrar em laço enquanto o workspace ainda não carregou', () => {
    // O workspace chega do backend depois da primeira renderização. Se o
    // seletor das abas fabricar uma lista nova a cada chamada, o
    // useSyncExternalStore vê um snapshot diferente a cada commit e reagenda
    // render até o React desistir com "Maximum update depth exceeded" — que é
    // o erro #185 que derrubava o app na inicialização.
    expect(() => render(<WorkspaceContent />)).not.toThrow();

    expect(screen.getByText('Nenhuma aba aberta')).toBeInTheDocument();
  });
});
