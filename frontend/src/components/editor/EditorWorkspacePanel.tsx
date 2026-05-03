import EditorPage from '../../pages/EditorPage';
import type { WorkspacePanelProps } from '../workspace/workspacePanelRegistry';
import { useEditorSurfaceController } from './useEditorSurfaceController';

export function EditorWorkspacePanel({ tab, isActive }: WorkspacePanelProps) {
  useEditorSurfaceController(tab, isActive);

  return <EditorPage />;
}
