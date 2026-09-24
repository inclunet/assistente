import { beforeEach, describe, expect, it, vi } from 'vitest';

const getTaskListPage = vi.hoisted(() => vi.fn());
const getAllTaskLists = vi.hoisted(() => vi.fn());
const updateTaskList = vi.hoisted(() => vi.fn());

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/Tasklist', () => ({
  GetTaskListPage: getTaskListPage,
  GetAllTaskLists: getAllTaskLists,
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
    getAllTaskLists.mockReset();
    updateTaskList.mockReset();
    updateTaskList.mockResolvedValue(undefined);
    useTaskListStore.setState({
      taskLists: new Map(),
      taskPages: new Map(),
      loadingByTaskListId: new Map(),
      loadingTaskPagesByListId: new Map(),
      taskPageLoadErrors: new Map(),
      errors: new Map(),
    });
  });

  it('repropaga o erro de edição e registra a falha sem alterar o cache', async () => {
    getAllTaskLists.mockResolvedValue([backendList()]);
    await useTaskListStore.getState().fetchAllTaskLists();
    const cached = useTaskListStore.getState().taskLists.get('list-a');
    const failure = new Error('Falha ao salvar lista');
    updateTaskList.mockRejectedValueOnce(failure);

    await expect(useTaskListStore.getState().updateTaskList('list-a', 'Novo título', 'Nova descrição'))
      .rejects.toBe(failure);

    expect(updateTaskList).toHaveBeenCalledWith('list-a', 'Novo título', 'Nova descrição');
    expect(useTaskListStore.getState().errors.get('updateTaskList:list-a')).toBe(String(failure));
    expect(useTaskListStore.getState().taskLists.get('list-a')).toBe(cached);
    expect(cached?.title).toBe('Lista grande');
    expect(cached?.description).toBe('');
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

  it('cacheia o catálogo sem disparar uma página por lista', async () => {
    getAllTaskLists.mockResolvedValue([
      { ...backendList(), task_count: 2634 },
      { ...backendList(), id: 'list-b', title: 'Outra', task_count: 7 },
    ]);

    const lists = await useTaskListStore.getState().fetchAllTaskLists();

    expect(lists).toHaveLength(2);
    expect(getAllTaskLists).toHaveBeenCalledTimes(1);
    expect(getTaskListPage).not.toHaveBeenCalled();
    expect(useTaskListStore.getState().taskLists.get('list-a')?.taskCount).toBe(2634);
  });

  it('preserva a contagem do catálogo ao carregar a primeira página', async () => {
    getAllTaskLists.mockResolvedValue([{ ...backendList(), task_count: 2634 }]);
    getTaskListPage.mockResolvedValue({
      task_list: backendList(),
      tasks: Array.from({ length: 100 }, (_, index) => backendTask(`task-${index}`, index)),
      next_cursor: 'cursor-100',
      has_more: true,
      total_count: 2634,
    });

    await useTaskListStore.getState().fetchAllTaskLists();
    await useTaskListStore.getState().loadTaskList('list-a');

    expect(useTaskListStore.getState().taskLists.get('list-a')?.taskCount).toBe(2634);
    expect(getTaskListPage).toHaveBeenCalledTimes(1);
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

  it('carrega board com mais de 100 cards até a última página sem tempestade de requests', async () => {
    const secondPage = deferred<Record<string, unknown>>();
    const thirdPage = deferred<Record<string, unknown>>();
    const cards = Array.from({ length: 205 }, (_, index) => ({
      ...backendTask(`task-${index + 1}`, index),
      status_id: (index % 3) + 1,
    }));
    cards[204] = {
      ...cards[204],
      subtasks: [{ ...backendTask('subtask-205-a', 0), parent_id: 'task-205' }],
    } as never;

    getTaskListPage
      .mockResolvedValueOnce({
        task_list: {
          ...backendList(),
          preferred_view_mode: 'kanban',
        },
        tasks: cards.slice(0, 100),
        next_cursor: 'cursor-100',
        has_more: true,
        total_count: 205,
      })
      .mockReturnValueOnce(secondPage.promise)
      .mockReturnValueOnce(thirdPage.promise);

    await useTaskListStore.getState().loadTaskList('list-a');
    expect(useTaskListStore.getState().taskLists.get('list-a')?.tasks).toHaveLength(100);
    expect(useTaskListStore.getState().loadingByTaskListId.has('list-a')).toBe(false);

    const firstLoadPromise = useTaskListStore.getState().loadAllTasksForBoard('list-a');
    const deduplicatedLoadPromise = useTaskListStore.getState().loadAllTasksForBoard('list-a');
    await vi.waitFor(() => expect(getTaskListPage).toHaveBeenCalledTimes(2));
    expect(useTaskListStore.getState().taskLists.get('list-a')?.tasks).toHaveLength(100);
    expect(useTaskListStore.getState().loadingByTaskListId.has('list-a')).toBe(false);
    expect(useTaskListStore.getState().loadingTaskPagesByListId.has('list-a')).toBe(true);

    secondPage.resolve({
      task_list: backendList(),
      tasks: cards.slice(100, 200),
      next_cursor: 'cursor-200',
      has_more: true,
      total_count: 205,
    });
    await vi.waitFor(() => expect(getTaskListPage).toHaveBeenCalledTimes(3));
    expect(useTaskListStore.getState().taskLists.get('list-a')?.tasks).toHaveLength(200);
    expect(useTaskListStore.getState().loadingTaskPagesByListId.has('list-a')).toBe(true);

    thirdPage.resolve({
      task_list: backendList(),
      tasks: cards.slice(200),
      next_cursor: '',
      has_more: false,
      total_count: 205,
    });
    const [firstLoad, deduplicatedLoad] = await Promise.all([firstLoadPromise, deduplicatedLoadPromise]);

    expect(firstLoad).toBe(205);
    expect(deduplicatedLoad).toBe(205);
    expect(getTaskListPage).toHaveBeenCalledTimes(3);
    expect(getTaskListPage.mock.calls).toEqual([
      ['list-a', ''],
      ['list-a', 'cursor-100'],
      ['list-a', 'cursor-200'],
    ]);
    const loaded = useTaskListStore.getState().taskLists.get('list-a')?.tasks ?? [];
    expect(loaded).toHaveLength(205);
    expect(loaded[204]).toEqual(expect.objectContaining({
      id: 'task-205',
      statusId: 1,
      subtasks: [expect.objectContaining({ id: 'subtask-205-a', parentId: 'task-205' })],
    }));
    expect(useTaskListStore.getState().taskPages.get('list-a')).toEqual({
      nextCursor: '',
      hasMore: false,
      totalCount: 205,
    });
    expect(useTaskListStore.getState().loadingTaskPagesByListId.has('list-a')).toBe(false);
  });

  it('mantém a primeira página navegável e permite retry após erro posterior', async () => {
    const cards = Array.from({ length: 105 }, (_, index) => backendTask(`task-${index + 1}`, index));
    getTaskListPage
      .mockResolvedValueOnce({
        task_list: { ...backendList(), preferred_view_mode: 'kanban' },
        tasks: cards.slice(0, 100),
        next_cursor: 'cursor-100',
        has_more: true,
        total_count: 105,
      })
      .mockRejectedValueOnce(new Error('falha transitória'))
      .mockResolvedValueOnce({
        task_list: backendList(),
        tasks: cards.slice(100),
        next_cursor: '',
        has_more: false,
        total_count: 105,
      });

    await useTaskListStore.getState().loadTaskList('list-a');
    await expect(useTaskListStore.getState().loadAllTasksForBoard('list-a')).rejects.toThrow('falha transitória');

    expect(useTaskListStore.getState().taskLists.get('list-a')?.tasks).toHaveLength(100);
    expect(useTaskListStore.getState().loadingByTaskListId.has('list-a')).toBe(false);
    expect(useTaskListStore.getState().loadingTaskPagesByListId.has('list-a')).toBe(false);
    expect(useTaskListStore.getState().taskPageLoadErrors.get('list-a')).toContain('falha transitória');
    expect(useTaskListStore.getState().taskPages.get('list-a')?.hasMore).toBe(true);

    await expect(useTaskListStore.getState().loadAllTasksForBoard('list-a')).resolves.toBe(105);
    expect(getTaskListPage).toHaveBeenCalledTimes(3);
    expect(useTaskListStore.getState().taskLists.get('list-a')?.tasks).toHaveLength(105);
    expect(useTaskListStore.getState().taskPageLoadErrors.has('list-a')).toBe(false);
  });

  it('cancela o carregamento progressivo entre páginas lentas', async () => {
    const secondPage = deferred<Record<string, unknown>>();
    const cards = Array.from({ length: 250 }, (_, index) => backendTask(`task-${index + 1}`, index));
    getTaskListPage
      .mockResolvedValueOnce({
        task_list: { ...backendList(), preferred_view_mode: 'kanban' },
        tasks: cards.slice(0, 100),
        next_cursor: 'cursor-100',
        has_more: true,
        total_count: 250,
      })
      .mockReturnValueOnce(secondPage.promise);

    await useTaskListStore.getState().loadTaskList('list-a');
    const loading = useTaskListStore.getState().loadAllTasksForBoard('list-a');
    await vi.waitFor(() => expect(getTaskListPage).toHaveBeenCalledTimes(2));
    useTaskListStore.getState().cancelBoardTaskLoad('list-a');
    secondPage.resolve({
      task_list: backendList(),
      tasks: cards.slice(100, 200),
      next_cursor: 'cursor-200',
      has_more: true,
      total_count: 250,
    });

    await expect(loading).resolves.toBe(200);
    expect(getTaskListPage).toHaveBeenCalledTimes(2);
    expect(useTaskListStore.getState().taskPages.get('list-a')?.hasMore).toBe(true);
  });
});
