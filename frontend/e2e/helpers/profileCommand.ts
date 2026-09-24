import type { Page } from '@playwright/test';
import { expect } from '@playwright/test';
import type { WailsMock } from '../fixtures';

export const profileCommandTicket = '01926b90-0000-7000-8000-000000000111';
export const profileCommandInvocation = '01926b90-0000-7000-8000-000000000112';
export const profileCommandHandoff = '01926b90-0000-7000-8000-000000000113';
export const profileCommandFingerprint = '0123456789abcdef'.repeat(4);

export const profileCommandTarget = {
  name: 'Padrão',
  description: 'Perfil padrão de teste',
  icon: 'chatbox',
  chat: {
    llm_provider: '$default', model: 'gpt-4', temperature: 0.7, max_tokens: 4096,
    top_p: 1, response_timeout: 180, enabled_tools: [], enabled_skills: [],
  },
  voice: {
    assistant: { enabled: false, provider: 'disabled', rate: 1, pitch: 1, volume: 1 },
    user: { enabled: false, provider: 'disabled', rate: 1, pitch: 1, volume: 1 },
    system: { enabled: false, provider: 'disabled', rate: 1, pitch: 1, volume: 1 },
  },
  input: { enabled: false, stt_provider: '', language: 'pt-BR', feedback_sounds: true, triggers: [] },
  channels: { response_mode: 'mirror' },
};

/** Establishes the profile-page keyboard and persisted read-target contract. */
export async function configureProfilePage(wails: WailsMock, profile = profileCommandTarget): Promise<void> {
  await wails.setResponse('GetLocalCommandKeyboardMap', {
    generation: 'e2e-profile-keyboard-map-v1',
    ownerId: 'user-e2e',
    sessionId: 'session-e2e',
    workspaceId: 'ws-1',
    bindings: [],
    contextualBindings: [{
      shortcut: { version: 1, code: 'KeyN', modifiers: ['Control'] },
      bySurface: {
        profiles: {
          shortcut: { version: 1, code: 'KeyN', modifiers: ['Control'] },
          commandId: 'profiles.create.open',
          handler: 'local_ui',
        },
      },
      fallback: null,
    }],
    localPaletteCommands: ['profiles.create.open', 'profiles.edit.open', 'profiles.search.focus'],
  });
  await wails.setResponse('ReadProfileCommandTarget', {
    profile,
    fingerprint: profileCommandFingerprint,
  });
  await wails.setResponse('GetProfileSearchPaths', []);
  await wails.setResponse('GetSpeechProviders', []);
  await wails.setResponse('GetSTTModels', []);
  await wails.setResponse('GetUserInvocableSkillsForProfile', []);
  await wails.setResponse('GetAvailableTools', []);
  await wails.setResponse('GetToolCatalog', { tools: [] });
  await wails.setResponse('ListMCPServers', []);
  await wails.setResponse('GetAllowlists', []);
}

/** One explicit, bounded page-mutation invocation per isolated E2E scenario. */
export async function configureProfileMutation(
  wails: WailsMock,
  commandId: 'profiles.create' | 'profiles.update' | 'profiles.duplicate' | 'profiles.delete' | 'profiles.activate',
  resultId = 'coder',
): Promise<void> {
  await wails.setResponse('BeginUICommand', {
    ticket: profileCommandTicket,
    invocationId: profileCommandInvocation,
    commandId,
  });
  await wails.setResponse('PreparePageMutationCommand', undefined);
  await wails.setResponse('TakeUICommand', {
    ticket: profileCommandTicket,
    invocationId: profileCommandInvocation,
    commandId,
    handoffId: profileCommandHandoff,
  });
  await wails.setResponse('CommitWorkspaceTabCommand', undefined);
  await wails.setResponse('GetUICommandResult', {
    invocationId: profileCommandInvocation,
    status: 'succeeded',
  });
  await wails.setResponse('GetPageMutationCommandResult', { id: resultId, title: 'Perfil atualizado' });
}

/** Makes the delete fixture wait for the same explicit decision boundary as the backend. */
export async function installProfileDeleteDecision(page: Page): Promise<void> {
  await page.evaluate(() => {
    const decisionID = '01926b90-0000-7000-8000-000000000114';
    window.__wailsMock.setResponse('PreparePageMutationCommand', () => {
      window.__wailsMock.emit('tool:questionnaire', {
        id: decisionID,
        kind: 'decision',
        title: 'Confirmar exclusão',
        description: 'Confirme a exclusão do perfil.',
        actions: [
          { id: 'apply', label: 'Excluir', primary: true, polarity: 'affirmative', scope: 'current' },
          { id: 'deny', label: 'Cancelar', polarity: 'negative', scope: 'current' },
        ],
        allowCancel: true,
        severity: 'destructive',
        questions: [],
      });
      return new Promise<void>((resolve, reject) => {
        window.__wailsMock.setResponse('RespondQuestionnaire', (id, answers, cancelled) => {
          if (id !== decisionID || cancelled || (answers as { actionId?: unknown })?.actionId !== 'apply') {
            reject(new Error('profile deletion decision was not approved'));
            return;
          }
          resolve();
        });
      });
    });
  });
}

export function expectProfileMutationProtocol(
  log: Array<{ fn: string; args: unknown[] }>,
  commandId: string,
  expectedRequest: { targetId: string; profileName?: string },
): void {
  const protocol = [
    ['BeginUICommand', (args: unknown[]) => args[0] === commandId],
    ['PreparePageMutationCommand', (args: unknown[]) => args[0] === profileCommandTicket],
    ['TakeUICommand', (args: unknown[]) => args[0] === profileCommandTicket],
    ['CommitWorkspaceTabCommand', (args: unknown[]) => args[0] === profileCommandTicket && args[1] === profileCommandHandoff],
    ['GetUICommandResult', (args: unknown[]) => args[0] === profileCommandTicket],
    ['GetPageMutationCommandResult', (args: unknown[]) => args[0] === profileCommandTicket],
  ] as const;
  let previousIndex = -1;
  for (const [fn, matches] of protocol) {
    const matching = log.flatMap((call, index) => call.fn === fn ? [{ index, args: call.args }] : []);
    if (matching.length !== 1 || !matches(matching[0].args) || matching[0].index <= previousIndex) {
      throw new Error(`Expected one ordered ${fn} call in the profile command protocol`);
    }
    previousIndex = matching[0].index;
  }
  expectProfileMutationPreparation(log, expectedRequest);
}

export function expectProfileMutationPreparation(
  log: Array<{ fn: string; args: unknown[] }>,
  expectedRequest: { targetId: string; profileName?: string },
): void {
  const matching = log.filter((call) => call.fn === 'PreparePageMutationCommand' && call.args[0] === profileCommandTicket);
  const request = matching[0]?.args[1] as {
    targetId?: unknown;
    expectedFingerprint?: unknown;
    profile?: { name?: unknown };
  } | undefined;
  if (matching.length !== 1 || request?.targetId !== expectedRequest.targetId ||
      request.expectedFingerprint !== profileCommandFingerprint ||
      (expectedRequest.profileName !== undefined && request.profile?.name !== expectedRequest.profileName)) {
    throw new Error('Expected the profile page-mutation request to carry its captured target, fingerprint, and edited name');
  }
}

/** Polls for the result and exposes the relevant protocol calls on failure. */
export async function waitForProfileMutationResult(wails: WailsMock, ticket: string): Promise<void> {
  await expect.poll(
    async () => (await wails.getCallLog())
      .filter(({ fn }) => /Command|Profile|Questionnaire/.test(fn))
      .map(({ fn, args }) => `${fn}${JSON.stringify(args)}`).join('\n'),
    {
      timeout: 5_000,
      message: `Waiting for GetPageMutationCommandResult(${ticket}); the received value is the Wails call log`,
    },
  ).toContain(`GetPageMutationCommandResult${JSON.stringify([ticket])}`);
}
