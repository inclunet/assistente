import { forwardRef, useImperativeHandle, type ReactNode } from 'react';
import { describe, expect, it, beforeEach, vi } from 'vitest';
import { render } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { WorkspaceTab } from '../../store/workspaceStore';
import TaskListView from './TaskListView';

const openCreateModalMock = vi.fn();
const workspacePanelState = vi.hoisted(() => ({
  isActive: false,
  tab: {
    id: 'tasklist-tab',
    type: 'tasklist' as const,
    title: 'Lista',
    position: 0,
    state: { tasklistId: 'tasklist-1' },
  } satisfies WorkspaceTab,
}));

const taskListStoreState = vi.hoisted(() => ({
  taskLists: new Map<string, unknown>(),
  loadTaskList: vi.fn(),
  setViewMode: vi.fn(),
  cloneTaskList: vi.fn(),
  clearTaskList: vi.fn(),
  deleteTaskList: vi.fn(),
  updateWorkflowFull: vi.fn(),
  getTaskCountsByStatus: vi.fn(),
}));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}));

vi.mock('../workspace/WorkspacePanelContext', () => ({
  useWorkspacePanel: () => workspacePanelState,
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: (selector: (state: { workspace: { profile: string } }) => unknown) => selector({
    workspace: { profile: 'default' },
  }),
}));

vi.mock('../../store/taskListStore', () => ({
  useTaskListStore: (selector?: (state: typeof taskListStoreState) => unknown) => (
    typeof selector === 'function' ? selector(taskListStoreState) : taskListStoreState
  ),
}));

vi.mock('../../store/workspaceChatModalStore', () => ({
  useWorkspaceChatModalStore: {
    getState: () => ({ requestOpen: vi.fn() }),
  },
}));

vi.mock('../../store/uiStore', () => ({
  useUIStore: (selector: (state: { addToast: ReturnType<typeof vi.fn> }) => unknown) => selector({
    addToast: vi.fn(),
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: vi.fn() }),
}));

vi.mock('../../hooks/useConfirm', () => ({
  useConfirm: () => vi.fn().mockResolvedValue(false),
}));

vi.mock('../../hooks/useDefaultFocus', () => ({
  registerDefaultFocus: vi.fn(),
  unregisterDefaultFocus: vi.fn(),
}));

vi.mock('../../hooks/useRegisterWorkspaceChatAdapter', () => ({
  useRegisterWorkspaceChatAdapter: vi.fn(),
}));

vi.mock('../ui/Modal', () => ({
  isModalOpen: () => false,
  Modal: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock('../ui/Toolbar', () => ({
  Toolbar: ({ actions }: { actions?: Array<{ key: string; label: string; onClick?: () => void }> }) => (
    <div>
      {actions?.map((action) => (
        <button key={action.key} type="button" onClick={action.onClick}>
          {action.label}
        </button>
      ))}
    </div>
  ),
}));

vi.mock('./TasksTable', () => ({
  default: forwardRef((_props, ref) => {
    useImperativeHandle(ref, () => ({
      openCreateModal: openCreateModalMock,
    }));
    return <div>tasks-table</div>;
  }),
}));

vi.mock('./KanbanBoard', () => ({
  default: forwardRef((_props, ref) => {
    useImperativeHandle(ref, () => ({
      openCreateModal: openCreateModalMock,
    }));
    return <div>kanban-board</div>;
  }),
}));

describe('TaskListView', () => {
  beforeEach(() => {
    workspacePanelState.isActive = false;
    openCreateModalMock.mockReset();
    taskListStoreState.loadTaskList.mockReset();
    taskListStoreState.taskLists = new Map([
      ['tasklist-1', {
        id: 'tasklist-1',
        title: 'Lista',
        preferredViewMode: 'list',
        tasks: [],
        workflow: { id: 'workflow-1', taskListId: 'tasklist-1', statuses: [], allowedTransitions: {}, initialStatusId: 1 },
      }],
    ]);
  });

  it('não responde a atalhos globais quando o painel está inativo', async () => {
    const user = userEvent.setup();
    render(<TaskListView taskListId="tasklist-1" />);

    await user.keyboard('n');

    expect(openCreateModalMock).not.toHaveBeenCalled();
  });

  it('responde a atalhos globais quando o painel está ativo', async () => {
    const user = userEvent.setup();
    workspacePanelState.isActive = true;
    render(<TaskListView taskListId="tasklist-1" />);

    await user.keyboard('n');

    expect(openCreateModalMock).toHaveBeenCalledTimes(1);
  });
});
