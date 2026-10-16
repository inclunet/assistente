import { describe, expect, it } from 'vitest';
import { readTaskListSurfaceContext, type TaskListSurfaceSource } from './commandTaskListSurface';

function source(overrides: Partial<TaskListSurfaceSource> = {}): TaskListSurfaceSource {
  return {
    surfaceType: 'tasklist',
    surfaceId: 'tasklist-tab',
    panelTabId: 'tasklist-tab',
    panelTabTaskListId: 'list-a',
    taskListId: 'list-a',
    taskList: {
      id: 'list-a',
      title: 'Lista A',
      updatedAt: '2026-09-16T12:00:00Z',
      preferredViewMode: 'list',
    },
    taskPageAvailable: true,
    loading: false,
    ...overrides,
  };
}

describe('commandTaskListSurface', () => {
  it('produz contexto mínimo sem tarefas, conteúdo ou seleção', () => {
    const context = readTaskListSurfaceContext(source());

    expect(context).toMatchObject({
      surfaceType: 'tasklist',
      surfaceId: 'tasklist-tab',
      title: 'Lista A',
      mode: 'list',
      metadata: {
        taskListId: 'list-a',
        resourceState: 'available',
        available: true,
        loading: false,
      },
    });
    expect(context).not.toHaveProperty('content');
    expect(context).not.toHaveProperty('selection');
    expect(context?.snapshotVersion).toBeTruthy();
  });

  it('recusa fonte ausente e retarget da aba sem depender de Notify', () => {
    expect(readTaskListSurfaceContext(source({ taskList: undefined }))).toBeNull();
    expect(readTaskListSurfaceContext(source({ panelTabTaskListId: 'list-b' }))).toBeNull();
    expect(readTaskListSurfaceContext(source({ taskListId: 'list-b' }))).toBeNull();
    expect(readTaskListSurfaceContext(source({ surfaceId: 'other-tab' }))).toBeNull();
  });

  it('reflete mudança real de modo e disponibilidade na versão', () => {
    const initial = readTaskListSurfaceContext(source());
    const changed = readTaskListSurfaceContext(source({
      taskPageAvailable: false,
      loading: true,
      taskList: {
        ...source().taskList!,
        preferredViewMode: 'kanban',
        updatedAt: '2026-09-16T12:01:00Z',
      },
    }));

    expect(changed?.metadata).toMatchObject({
      resourceState: 'loading',
      available: false,
      loading: true,
    });
    expect(changed?.snapshotVersion).not.toBe(initial?.snapshotVersion);
  });

  it('mantém leituras separadas equivalentes quando os fatos não mudam', () => {
    const first = readTaskListSurfaceContext(source());
    const second = readTaskListSurfaceContext(source());

    expect(second).toEqual(first);
    expect(first).not.toHaveProperty('capturedAt');
  });
});
