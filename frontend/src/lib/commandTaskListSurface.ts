import type { SurfaceContext } from './chatSurface';
import type { ViewMode } from '../types/tasklist';

export interface TaskListSurfaceSource {
  readonly surfaceType: string;
  readonly surfaceId: string;
  readonly panelTabId: string;
  readonly panelTabTaskListId?: string;
  readonly taskListId: string;
  readonly taskList?: {
    readonly id: string;
    readonly title: string;
    readonly updatedAt: string;
    readonly preferredViewMode: ViewMode;
  };
  readonly taskPageAvailable: boolean;
  readonly loading: boolean;
  readonly loadError?: string;
}

/**
 * Produz somente a projeção contextual mínima da superfície tasklist.
 * A fonte já deve ter sido lida pelo getter do componente; esta função não
 * acessa stores, DOM, tarefas ou conteúdo da lista.
 */
export function readTaskListSurfaceContext(
  source: TaskListSurfaceSource,
): SurfaceContext | null {
  if (
    !source ||
    source.surfaceType !== 'tasklist' ||
    !source.surfaceId ||
    source.surfaceId !== source.panelTabId ||
    !source.taskListId ||
    source.panelTabTaskListId !== source.taskListId ||
    !source.taskList ||
    source.taskList.id !== source.taskListId
  ) {
    return null;
  }

  const available = source.taskPageAvailable;
  const loading = source.loading;
  const resourceState = available ? 'available' : loading ? 'loading' : 'unavailable';
  const snapshotSeed = JSON.stringify({
    taskListId: source.taskListId,
    updatedAt: source.taskList.updatedAt,
    viewMode: source.taskList.preferredViewMode,
    resourceState,
    available,
    loading,
    hasLoadError: Boolean(source.loadError),
  });

  return {
    surfaceType: source.surfaceType,
    surfaceId: source.surfaceId,
    title: source.taskList.title,
    mode: source.taskList.preferredViewMode,
    metadata: {
      taskListId: source.taskListId,
      resourceState,
      available,
      loading,
    },
    snapshotVersion: `tasklist-command-v1:${snapshotSeed}`,
  };
}
