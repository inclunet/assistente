import { lazy } from 'react';
import type { WorkspacePanelProps } from '../workspace/workspacePanelRegistry';
import { useEditorSurfaceController } from './useEditorSurfaceController';

const EditorPage = lazy(() => import('../../pages/EditorPage'));

export function EditorWorkspacePanel({ tab, isActive }: WorkspacePanelProps) {
  useEditorSurfaceController(tab, isActive);

  return <EditorPage />;
}
