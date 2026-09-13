import { beforeEach, describe, expect, it, vi } from 'vitest';

const getTaskListPage = vi.hoisted(() => vi.fn());
const updateTaskList = vi.hoisted(() => vi.fn());

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/Tasklist', () => ({
  GetTaskListPage: getTaskListPage,
  UpdateTaskList: updateTaskList,
}));

vi.mock('@wailsjs/go/wailsapi/TasklistActions', () => ({}));

import { useTaskListStore } from './taskListStore';

function backendTask(id: string, order: number) {
  return {
    id,
    task_list_id: 'list-a',
    title: id,
    description: '',
    status_id: 1,
    order,
    createdAt: '2026-09-13T12:00:00Z',
    updatedAt: '2026-09-13T12:00:00Z',
  };
}

function backendList() {
  return {
    id: 'list-a',
    title: 'Lista grande',
    description: '',
    preferred_view_mode: 'list',
    workflow: {
      id: 'workflow-a',
      task_list_id: 'list-a',
      statuses: '[]',
      allowed_transitions: '{}',
      initial_status_id: 1,
    },
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((fulfill) => {
    resolve = fulfill;
  });
  return { promise, resolve };
}

describe('taskListStore pagination', () => {
  beforeEach(() => {
    getTaskListPage.mockReset();
    updateTaskList.mockReset();
    updateTaskList.mockResolvedValue(undefined);
    useTaskListStore.setState({
      taskLists: new Map(),
      taskPages: new Map(),
      loadingByTaskListId: new Map(),
      errors: new Map(),
    });
  });

  it('carrega páginas sem perder ordem, itens ou compatibilidade da lista', async () => {
    getTaskListPage
      .mockResolvedValueOnce({
        task_list: backendList(),
        tasks: [backendTask('task-b', 1), backendTask('task-a', 0)],
        next_cursor: 'cursor-2',
        has_more: true,
        total_count: 3,
      })
      .mockResolvedValueOnce({
        task_list: backendList(),
        tasks: [backendTask('task-c', 2)],
        next_cursor: '',
        has_more: false,
        total_count: 3,
      });

    const first = await useTaskListStore.getState().loadTaskList('list-a');
    expect(first?.tasks.map((task) => task.id)).toEqual(['task-b', 'task-a']);
    expect(useTaskListStore.getState().taskPages.get('list-a')).toEqual({
      nextCursor: 'cursor-2',
      hasMore: true,
      totalCount: 3,
    });

    await useTaskListStore.getState().loadMoreTasks('list-a');

    expect(getTaskListPage).toHaveBeenNthCalledWith(1, 'list-a', '');
    expect(getTaskListPage).toHaveBeenNthCalledWith(2, 'list-a', 'cursor-2');
    expect(useTaskListStore.getState().taskLists.get('list-a')?.tasks.map((task) => task.id))
      .toEqual(['task-a', 'task-b', 'task-c']);
    expect(useTaskListStore.getState().taskPages.get('list-a')?.hasMore).toBe(false);
  });

  it('preserva páginas carregadas quando evento traz somente metadados', async () => {
    getTaskListPage.mockResolvedValueOnce({
      task_list: backendList(),
      tasks: [backendTask('task-a', 0)],
      next_cursor: '',
      has_more: false,
      total_count: 1,
    });
    await useTaskListStore.getState().loadTaskList('list-a');

    useTaskListStore.getState().cacheTaskList({
      ...backendList(),
      title: 'Título atualizado',
    } as never);

    const cached = useTaskListStore.getState().taskLists.get('list-a');
    expect(cached?.title).toBe('Título atualizado');
    expect(cached?.tasks.map((task) => task.id)).toEqual(['task-a']);
  });

  it('recarrega todas as páginas que já estavam visíveis', async () => {
    getTaskListPage
      .mockResolvedValueOnce({
        task_list: backendList(),
        tasks: [backendTask('task-a', 0), backendTask('task-b', 1)],
        next_cursor: 'cursor-2',
        has_more: true,
        total_count: 3,
      })
      .mockResolvedValueOnce({
        task_list: backendList(),
        tasks: [backendTask('task-c', 2)],
        next_cursor: '',
        has_more: false,
        total_count: 3,
      })
      .mockResolvedValueOnce({
        task_list: { ...backendList(), title: 'Lista atualizada' },
        tasks: [backendTask('task-a', 0), backendTask('task-b', 1)],
        next_cursor: 'cursor-2-refresh',
        has_more: true,
        total_count: 3,
      })
      .mockResolvedValueOnce({
        task_list: backendList(),
        tasks: [backendTask('task-c', 2)],
        next_cursor: '',
        has_more: false,
        total_count: 3,
      });

    await useTaskListStore.getState().loadTaskList('list-a');
    await useTaskListStore.getState().loadMoreTasks('list-a');
    await useTaskListStore.getState().loadTaskList('list-a');

    expect(getTaskListPage).toHaveBeenNthCalledWith(3, 'list-a', '');
    expect(getTaskListPage).toHaveBeenNthCalledWith(4, 'list-a', 'cursor-2-refresh');
    const refreshed = useTaskListStore.getState().taskLists.get('list-a');
    expect(refreshed?.title).toBe('Lista atualizada');
    expect(refreshed?.tasks.map((task) => task.id)).toEqual(['task-a', 'task-b', 'task-c']);
  });

  it('preserva páginas carregadas ao renomear a lista', async () => {
    getTaskListPage
      .mockResolvedValueOnce({
        task_list: backendList(),
        tasks: [backendTask('task-a', 0), backendTask('task-b', 1)],
        next_cursor: 'cursor-2',
        has_more: true,
        total_count: 3,
      })
      .mockResolvedValueOnce({
        task_list: backendList(),
        tasks: [backendTask('task-c', 2)],
        next_cursor: '',
        has_more: false,
        total_count: 3,
      });

    await useTaskListStore.getState().loadTaskList('list-a');
    await useTaskListStore.getState().loadMoreTasks('list-a');
    await useTaskListStore.getState().updateTaskList('list-a', 'Nome novo', 'Descrição nova');

    expect(updateTaskList).toHaveBeenCalledWith('list-a', 'Nome novo', 'Descrição nova');
    const cached = useTaskListStore.getState().taskLists.get('list-a');
    expect(cached?.title).toBe('Nome novo');
    expect(cached?.description).toBe('Descrição nova');
    expect(cached?.tasks.map((task) => task.id)).toEqual(['task-a', 'task-b', 'task-c']);
    expect(useTaskListStore.getState().taskPages.get('list-a')?.totalCount).toBe(3);
  });

  it('serializa carregar mais com recarga disparada por evento', async () => {
    const pendingPage = deferred<Record<string, unknown>>();
    getTaskListPage
      .mockResolvedValueOnce({
        task_list: backendList(),
        tasks: [backendTask('task-a', 0), backendTask('task-b', 1)],
        next_cursor: 'cursor-2',
        has_more: true,
        total_count: 3,
      })
      .mockReturnValueOnce(pendingPage.promise)
      .mockResolvedValueOnce({
        task_list: backendList(),
        tasks: [backendTask('task-a', 0), backendTask('task-b', 1)],
        next_cursor: 'cursor-2-refresh',
        has_more: true,
        total_count: 3,
      })
      .mockResolvedValueOnce({
        task_list: backendList(),
        tasks: [backendTask('task-c', 2)],
        next_cursor: '',
        has_more: false,
        total_count: 3,
      });

    await useTaskListStore.getState().loadTaskList('list-a');
    const loadMore = useTaskListStore.getState().loadMoreTasks('list-a');
    await vi.waitFor(() => expect(getTaskListPage).toHaveBeenCalledTimes(2));
    const reload = useTaskListStore.getState().loadTaskList('list-a');
    expect(getTaskListPage).toHaveBeenCalledTimes(2);

    pendingPage.resolve({
      task_list: backendList(),
      tasks: [backendTask('task-c', 2)],
      next_cursor: '',
      has_more: false,
      total_count: 3,
    });
    await Promise.all([loadMore, reload]);

    expect(getTaskListPage).toHaveBeenNthCalledWith(3, 'list-a', '');
    expect(getTaskListPage).toHaveBeenNthCalledWith(4, 'list-a', 'cursor-2-refresh');
    expect(useTaskListStore.getState().taskLists.get('list-a')?.tasks.map((task) => task.id))
      .toEqual(['task-a', 'task-b', 'task-c']);
  });
});
