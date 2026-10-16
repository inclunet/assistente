import { createCommandWorkspaceTabWailsPort, type WorkspaceTabCommandWailsOptions } from './commandWorkspaceTabWails';
import type { PageMutationPort, PageMutationRequest, PageMutationResult } from './commandPageMutation';
import type { TaskListWithWorkflow } from '../types/tasklist';
import type { profiles } from '@wailsjs/go/models';

interface PageMutationAPI {
  PreparePageMutationCommand(ticket: string, request: PageMutationRequest): Promise<void>;
  GetPageMutationCommandResult(ticket: string): Promise<PageMutationResult>;
  ReadTaskListCommandTarget(id: string): Promise<{ taskList: TaskListWithWorkflow; fingerprint: string }>;
  ReadProfileCommandTarget(slug: string): Promise<{ profile?: profiles.Profile; fingerprint: string }>;
}
function api(target: Window = window): Partial<PageMutationAPI> { return (target as Window & { go?: { app?: { App?: PageMutationAPI } } }).go?.app?.App ?? {}; }
export async function readTaskListCommandTarget(id: string) {
  const app = api(); if (!app.ReadTaskListCommandTarget) throw new Error('Task list command target API unavailable');
  return app.ReadTaskListCommandTarget(id);
}
export async function readProfileCommandTarget(slug: string) {
  const app = api(); if (!app.ReadProfileCommandTarget) throw new Error('Profile command target API unavailable');
  return app.ReadProfileCommandTarget(slug);
}
export function createPageMutationWailsPort(options: WorkspaceTabCommandWailsOptions = {}): PageMutationPort {
  return { ...createCommandWorkspaceTabWailsPort(options),
    preparePageMutationCommand: async (ticket, request) => {
      const app = api(options.target);
      if (!app.PreparePageMutationCommand) throw new Error('Page mutation API unavailable');
      if (options.contextualPalette && !options.contextualPalette.isCurrent()) throw new Error('contextual-palette-stale');
      await app.PreparePageMutationCommand(ticket, request);
    },
    getPageMutationCommandResult: async ticket => { const app = api(options.target); if (!app.GetPageMutationCommandResult) throw new Error('Page mutation result API unavailable'); return app.GetPageMutationCommandResult(ticket); },
  };
}
