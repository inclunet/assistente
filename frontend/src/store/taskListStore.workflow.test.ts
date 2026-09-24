import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Task, TaskListWithWorkflow } from '../types/tasklist';

const getTaskListPage = vi.hoisted(() => vi.fn());
const updateWorkflowFull = vi.hoisted(() => vi.fn());

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/Tasklist', () => ({
  GetTaskListPage: getTaskListPage,
  UpdateWorkflowFull: updateWorkflowFull,
}));

vi.mock('@wailsjs/go/wailsapi/TasklistActions', () => ({}));

import { useTaskListStore } from './taskListStore';

const tasks = [{ id: 't1', statusId: 1 }, { id: 't2', statusId: 2 }] as unknown as Task[];

function cachedList(): TaskListWithWorkflow {
  return {
    id: 'list-a',
    title: 'Lista',
    description: '',
    workflow: {
      id: 'w1',
      taskListId: 'list-a',
      statuses: [
        { id: 1, order: 0, label: 'A Fazer', color: 'var(--color-info)', icon: '⌛' },
        { id: 2, order: 1, label: 'Feito', color: 'var(--color-success)', icon: '✅' },
      ],
      allowedTransitions: { 1: [2], 2: [] },
      initialStatusId: 1,
      createdAt: '2026-09-24',
      updatedAt: '2026-09-24',
    },
    tasks,
  } as unknown as TaskListWithWorkflow;
}

const reordered = [
  { id: 2, order: 0, label: 'Feito', color: 'var(--color-success)', icon: '✅' },
  { id: 1, order: 1, label: 'A Fazer', color: 'var(--color-info)', icon: '⌛' },
];

describe('taskListStore.updateWorkflowFull', () => {
  beforeEach(() => {
    getTaskListPage.mockReset();
    updateWorkflowFull.mockReset();
    updateWorkflowFull.mockResolvedValue(undefined);
    useTaskListStore.setState({
      taskLists: new Map([['list-a', cachedList()]]),
      errors: new Map(),
    });
  });

  it('sem migração atualiza só o workflow em cache, sem recarregar as tarefas', async () => {
    await useTaskListStore.getState().updateWorkflowFull('list-a', reordered, { 1: [], 2: [1] }, 2, {});

    expect(updateWorkflowFull).toHaveBeenCalledWith('list-a', reordered, { 1: [], 2: [1] }, 2, {});
    expect(getTaskListPage).not.toHaveBeenCalled();
    const list = useTaskListStore.getState().taskLists.get('list-a');
    expect(list?.workflow.statuses.map((s) => s.id)).toEqual([2, 1]);
    expect(list?.workflow.allowedTransitions).toEqual({ 1: [], 2: [1] });
    expect(list?.workflow.initialStatusId).toBe(2);
    expect(list?.workflow.id).toBe('w1');
    expect(list?.tasks).toBe(tasks);
  });

  it('com migração recarrega a lista, porque as tarefas mudam de status', async () => {
    getTaskListPage.mockResolvedValue({ taskList: null });
    await useTaskListStore.getState().updateWorkflowFull('list-a', [reordered[1]], { 1: [] }, 1, { 2: 1 });

    expect(updateWorkflowFull).toHaveBeenCalledWith('list-a', [reordered[1]], { 1: [] }, 1, { 2: 1 });
    expect(getTaskListPage).toHaveBeenCalledWith('list-a', '');
  });

  it('falha no backend repropaga o erro e não altera o cache', async () => {
    const before = useTaskListStore.getState().taskLists.get('list-a');
    updateWorkflowFull.mockRejectedValueOnce(new Error('recusado'));

    await expect(useTaskListStore.getState().updateWorkflowFull('list-a', reordered, {}, 2, {}))
      .rejects.toThrow('recusado');

    expect(useTaskListStore.getState().taskLists.get('list-a')).toBe(before);
    expect(getTaskListPage).not.toHaveBeenCalled();
  });
});
