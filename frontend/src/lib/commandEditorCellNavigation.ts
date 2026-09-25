import type { Editor } from '@tiptap/core';
import { goToNextCell } from '@tiptap/pm/tables';

export const EDITOR_CELL_NAVIGATION_COMMAND_IDS = [
  'editor.table.cell.next',
  'editor.table.cell.previous',
] as const;

export type EditorCellNavigationCommandID = typeof EDITOR_CELL_NAVIGATION_COMMAND_IDS[number];

export const EDITOR_CELL_NAVIGATION_COMMAND_EVENT = 'editor-cell-navigation-request';

export function isEditorCellNavigationCommand(id: string): id is EditorCellNavigationCommandID {
  return (EDITOR_CELL_NAVIGATION_COMMAND_IDS as readonly string[]).includes(id);
}

export function requestEditorCellNavigationCommand(commandID: EditorCellNavigationCommandID, expectedEditor?: Editor): void {
  window.dispatchEvent(new CustomEvent(EDITOR_CELL_NAVIGATION_COMMAND_EVENT, {
    detail: { commandID, ...(expectedEditor === undefined ? {} : { expectedEditor }) },
    cancelable: true,
  }));
}

function navigationCommand(editor: Editor, commandID: EditorCellNavigationCommandID) {
  if (editor.isDestroyed || !editor.isEditable || editor.view.composing) return undefined;
  return goToNextCell(commandID === 'editor.table.cell.next' ? 1 : -1);
}

export function canNavigateCell(editor: Editor, commandID: string): boolean {
  if (!isEditorCellNavigationCommand(commandID)) return false;
  try {
    return navigationCommand(editor, commandID)?.(editor.state) === true;
  } catch {
    return false;
  }
}

export function navigateEditorCell(editor: Editor, commandID: string): boolean {
  if (!isEditorCellNavigationCommand(commandID)) return false;
  try {
    const command = navigationCommand(editor, commandID);
    if (!command) return false;
    const moved = command(editor.state, (transaction) => editor.view.dispatch(transaction));
    if (moved) editor.view.focus();
    return moved;
  } catch {
    return false;
  }
}
