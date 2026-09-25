import { isWorkspaceMutationCommand } from './commandContextualBackendExecution';
import { isCommandLayerAction } from './commandLayerActions';
import { CHAT_CLEAR_COMMAND } from './commandChatClear';
import { isChatMessagingCommand } from './commandChatMessaging';
import { isEditorFileCommand } from './commandEditorFile';
import { isEditorFormatCommand } from './commandEditorFormatting';
import { TERMINAL_INTERRUPT_COMMAND, isTerminalSessionOperationCommand } from './commandTerminalOperation';

export const CONTEXTUAL_PAGE_PALETTE_COMMAND_IDS = [
  'tasklists.duplicate', 'tasklists.delete', 'tasklists.clear',
  'profiles.duplicate', 'profiles.delete', 'profiles.activate',
] as const;

export function isContextualPagePaletteCommand(id: unknown): id is typeof CONTEXTUAL_PAGE_PALETTE_COMMAND_IDS[number] {
  return CONTEXTUAL_PAGE_PALETTE_COMMAND_IDS.some(command => command === id);
}

export function isContextualPagePaletteSurface(id: string, surfaceType: string): boolean {
  if (!isContextualPagePaletteCommand(id)) return false;
  if (id.startsWith('profiles.')) return surfaceType === 'profiles';
  return surfaceType === 'tasklists' || (surfaceType === 'tasklist' &&
    (id === 'tasklists.duplicate' || id === 'tasklists.clear'));
}

/** Closed product scope. Each group keeps its own preparation and executor. */
export function isContextualPaletteCommand(id: unknown): id is string {
  return typeof id === 'string' && (isCommandLayerAction(id) || isContextualPagePaletteCommand(id) || isWorkspaceMutationCommand(id) || id === CHAT_CLEAR_COMMAND ||
    id === TERMINAL_INTERRUPT_COMMAND || isTerminalSessionOperationCommand(id) ||
    (isChatMessagingCommand(id) && id !== 'chat.message.edit.open') ||
    isEditorFileCommand(id) || isEditorFormatCommand(id));
}
