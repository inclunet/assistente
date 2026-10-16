import type { NavigateFunction } from 'react-router-dom';
import { isWorkspaceTabNavigationCommand } from './commandWorkspaceTabNavigation';

/** Comandos de navegação publicados pela paleta e seus destinos permitidos. */
export const COMMAND_NAVIGATION_ROUTES = {
  'navigation.workspace.open': '/',
  'navigation.history.open': '/history',
  'navigation.memories.open': '/memories',
  'navigation.tasklists.open': '/tasklists',
  'navigation.jobs.open': '/jobs',
  'navigation.profiles.open': '/profiles',
  'navigation.settings.open': '/settings',
  'navigation.data.export.open': '/settings/data?action=export',
  'navigation.data.import.open': '/settings/data?action=import',
  'navigation.help.open': '/help',
  'navigation.about.open': '/about',
} as const;

export type CommandNavigationID = keyof typeof COMMAND_NAVIGATION_ROUTES;
export const COMMAND_NAVIGATION_EVENT = 'commands:navigation';

export interface CommandNavigationRequest {
  readonly commandId: CommandNavigationID;
}

export function createCommandNavigationHandlers(
  navigate: NavigateFunction,
): ReadonlyMap<string, () => undefined> {
  return new Map(
    Object.entries(COMMAND_NAVIGATION_ROUTES).map(([commandID, path]) => [
      commandID,
      () => {
        navigate(path);
        return undefined;
      },
    ]),
  );
}

export function isCommandNavigationRoute(commandID: string): commandID is CommandNavigationID {
  return Object.prototype.hasOwnProperty.call(COMMAND_NAVIGATION_ROUTES, commandID);
}

/** Solicita navegação ao dispatcher local; sem listener, falha fechado. */
export function requestCommandNavigation(commandID: CommandNavigationID): boolean {
  if (!isCommandNavigationRoute(commandID) || typeof window === 'undefined') return false;
  const event = new CustomEvent<CommandNavigationRequest>(COMMAND_NAVIGATION_EVENT, {
    detail: { commandId: commandID },
    cancelable: true,
  });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}

/** Exceção explícita de navegação; não autoriza comandos contextuais ou futuros. */
export function isCommandNavigation(commandID: string): boolean {
  return commandID === 'navigation.menu.open' || commandID === 'navigation.palette.open' ||
    Object.prototype.hasOwnProperty.call(COMMAND_NAVIGATION_ROUTES, commandID);
}

export function isCommandNavigationTextField(target: unknown): boolean {
  return (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement) &&
    !target.closest('.monaco-editor, [contenteditable="true"]');
}

/**
 * Monaco may originate only the local workspace-tab navigation family.
 * Global navigation and other workspace.tab commands keep the normal editable
 * target restrictions.
 */
export function isWorkspaceTabNavigationTextField(commandID: string, target: unknown): boolean {
  if (!isWorkspaceTabNavigationCommand(commandID)) return false;
  if (isCommandNavigationTextField(target)) return true;
  const monacoInput = (target instanceof HTMLTextAreaElement ||
    target instanceof HTMLElement && target.matches('.native-edit-context')) &&
    target.closest('.monaco-editor') !== null &&
    target.closest('.rich-text-editor') === null;
  return monacoInput;
}
