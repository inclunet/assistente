import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { WorkspaceTab } from '../../store/workspaceStore';

const taskListMocks = vi.hoisted(() => ({
  taskLists: new Map<string, { id: string; title: string }>(),
  createTaskList: vi.fn(),
  loadTaskList: vi.fn(),
  updateTaskList: vi.fn(),
  subscribe: vi.fn(),
}));

const workspaceMocks = vi.hoisted(() => ({
  updateTab: vi.fn(),
  isInitialized: true,
  tabs: [] as WorkspaceTab[],
}));

vi.mock('../../store/taskListStore', () => ({
  useTaskListStore: Object.assign(
    (selector: (state: typeof taskListMocks) => unknown) => selector(taskListMocks),
    {
      getState: () => taskListMocks,
      subscribe: (listener: (state: typeof taskListMocks) => void) => {
        taskListMocks.subscribe(listener);
        return vi.fn();
      },
    },
  ),
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign(
    (selector: (state: { isInitialized: boolean; updateTab: typeof workspaceMocks.updateTab }) => unknown) => selector({
      isInitialized: workspaceMocks.isInitialized,
      updateTab: workspaceMocks.updateTab,
    }),
    {
      getState: () => ({
        workspace: { tabs: workspaceMocks.tabs },
      }),
    },
  ),
}));

vi.mock('i18next', () => ({
  default: {
    t: vi.fn((key: string) => (key === 'tasklist.newList' ? 'Nova Lista' : key)),
  },
}));

import { useTaskListSurfaceController } from './useTaskListSurfaceController';

const taskListTab: WorkspaceTab = {
  id: 'tasklist-tab',
  type: 'tasklist',
  title: 'Tasklist',
  position: 0,
  state: {},
};

describe('useTaskListSurfaceController', () => {
  beforeEach(() => {
    taskListMocks.taskLists = new Map();
    taskListMocks.createTaskList.mockReset();
    taskListMocks.loadTaskList.mockReset();
    taskListMocks.updateTaskList.mockReset();
    taskListMocks.subscribe.mockReset();
    workspaceMocks.updateTab.mockReset();
    workspaceMocks.isInitialized = true;
    workspaceMocks.tabs = [taskListTab];
  });

  it('cria tasklist para aba ativa sem tasklistId', async () => {
    taskListMocks.createTaskList.mockResolvedValue({ id: 'tasklist-1', title: 'Nova Lista' });

    renderHook(() => useTaskListSurfaceController(taskListTab, true));

    await waitFor(() => {
      expect(taskListMocks.createTaskList).toHaveBeenCalledWith('Nova Lista');
      expect(workspaceMocks.updateTab).toHaveBeenCalledWith('tasklist-tab', {
        state: { tasklistId: 'tasklist-1' },
        title: 'Nova Lista',
      });
    });
  });

  it('carrega tasklist existente por id explícito da aba', async () => {
    renderHook(() => useTaskListSurfaceController({
      ...taskListTab,
      state: { tasklistId: 'tasklist-2' },
    }, true));

    await waitFor(() => {
      expect(taskListMocks.loadTaskList).toHaveBeenCalledWith('tasklist-2');
    });
    expect(taskListMocks.createTaskList).not.toHaveBeenCalled();
  });

  it('não recarrega tasklist existente já presente no cache', async () => {
    taskListMocks.taskLists = new Map([['tasklist-2', { id: 'tasklist-2', title: 'Lista 2' }]]);
    const tabWithTaskList = {
      ...taskListTab,
      state: { tasklistId: 'tasklist-2' },
    };

    renderHook(() => useTaskListSurfaceController(tabWithTaskList, true));

    expect(taskListMocks.loadTaskList).not.toHaveBeenCalled();
  });

  it('trata erro ao carregar tasklist existente', async () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    taskListMocks.loadTaskList.mockRejectedValue(new Error('load failed'));

    renderHook(() => useTaskListSurfaceController({
      ...taskListTab,
      state: { tasklistId: 'tasklist-2' },
    }, true));

    await waitFor(() => {
      expect(errorSpy).toHaveBeenCalledWith(
        '[TaskListSurfaceController] Erro ao sincronizar tasklist:',
        expect.any(Error),
      );
    });

    errorSpy.mockRestore();
  });
});
