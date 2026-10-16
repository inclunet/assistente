import { WORKSPACE_TAB_NAVIGATION_COMMAND_IDS, isWorkspaceTabNavigationCommand } from './commandWorkspaceTabNavigation';
import { CHAT_PICKER_COMMAND_IDS } from './commandChatPickers';
import { EDITOR_PRESENTATION_COMMAND_IDS } from './commandEditorPresentation';
import { CHAT_NAVIGATION_COMMAND_IDS } from './commandChatNavigation';
import { PAGE_PRESENTATION_COMMAND_IDS } from './commandPagePresentation';

export const LOCAL_UI_COMMAND_IDS = [
  ...PAGE_PRESENTATION_COMMAND_IDS,
  ...CHAT_NAVIGATION_COMMAND_IDS,
  'navigation.landmark.next',
  'navigation.landmark.previous',
  'navigation.landmark.default',
  'editor.mermaid.open',
  'chat.message.edit.open',
  ...EDITOR_PRESENTATION_COMMAND_IDS,
  ...CHAT_PICKER_COMMAND_IDS,
  ...WORKSPACE_TAB_NAVIGATION_COMMAND_IDS,
  'navigation.workspace.open',
  'navigation.history.open',
  'navigation.memories.open',
  'navigation.tasklists.open',
  'navigation.jobs.open',
  'navigation.profiles.open',
  'navigation.settings.open',
  'navigation.palette.open',
  'navigation.data.export.open',
  'navigation.data.import.open',
  'navigation.help.open',
  'navigation.about.open',
  'navigation.menu.open',
  'help.shortcuts.show',
  'workspace.panel.focus',
] as const;

export type LocalUICommandID = typeof LOCAL_UI_COMMAND_IDS[number];

export function isLocalUICommand(commandID: unknown): commandID is LocalUICommandID {
  return LOCAL_UI_COMMAND_IDS.some((candidate) => candidate === commandID);
}

export { isWorkspaceTabNavigationCommand };
