import TaskListView from './TaskListView';
import type { WorkspacePanelProps } from '../workspace/workspacePanelRegistry';
import { useTaskListSurfaceController } from './useTaskListSurfaceController';

export function TaskListWorkspacePanel({ tab, isActive, state }: WorkspacePanelProps) {
  useTaskListSurfaceController(tab, isActive);

  const taskListId = state.tasklistId;
  if (typeof taskListId !== 'string' || !taskListId) {
    return <div className="ws-content__loading" aria-busy="true" />;
  }

  return <TaskListView taskListId={taskListId} />;
}
