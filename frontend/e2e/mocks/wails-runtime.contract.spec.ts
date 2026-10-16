import { expect, test } from '@playwright/test';
import { buildWailsMockScript } from './wails-runtime';

test('Wails mock proxy is not thenable and consumes one ownership ACK', async ({ page }) => {
  await page.goto('about:blank');
  await page.evaluate((script) => { (0, eval)(script); }, buildWailsMockScript());

  const result = await page.evaluate(async () => {
    const app = await Promise.resolve(window.go.app.App);
    const frame = await app.GetGlobalCommandOwnership() as Record<string, unknown>;
    const accepted = await app.AckGlobalCommandOwnership(frame.instanceId, frame.revision);
    const replayAccepted = await app.AckGlobalCommandOwnership(frame.instanceId, frame.revision);
    const map = await app.GetLocalCommandKeyboardMap() as Record<string, unknown>;
    return {
      frame,
      accepted,
      replayAccepted,
      map,
      calls: window.__wailsMock.getCallLog().map((call) => call.fn),
    };
  });

  expect(result.frame).toMatchObject({
    version: 1,
    platform: 'windows',
    revision: 1,
    combinations: [],
  });
  expect(result.accepted).toBe(true);
  expect(result.replayAccepted).toBe(false);
  expect(result.map).toMatchObject({
    generation: 'e2e-local-command-map-v1',
    ownerId: 'user-e2e',
    sessionId: 'session-e2e',
    workspaceId: 'ws-1',
  });
  expect(result.map.localPaletteCommands).toContain('profiles.edit.open');
  expect(result.calls).not.toContain('then');
  expect(result.calls).toContain('GetLocalCommandKeyboardMap');
});

test('default palette fixture exactly projects canonical local UI command IDs', async ({ page }) => {
  await page.addInitScript({ content: buildWailsMockScript() });
  await page.goto('/');

  const projection = await page.evaluate(async () => {
    const { LOCAL_UI_COMMAND_IDS } = await import('/src/lib/commandLocalUI.ts');
    const map = await window.go.app.App.GetLocalCommandKeyboardMap() as { localPaletteCommands: string[] };
    return { canonical: [...LOCAL_UI_COMMAND_IDS], fixture: map.localPaletteCommands };
  });

  expect(projection.fixture).toEqual(projection.canonical);
});

test('chat submission fixture requires a one-use command handoff', async ({ page }) => {
  // Prova do adaptador de UI, não de autorização do backend: a captura da
  // conversa/identidade e a recusa de destinos diferentes são cobertas em Go.
  await page.goto('about:blank');
  await page.evaluate((script) => { (0, eval)(script); }, buildWailsMockScript());

  const result = await page.evaluate(async () => {
    window.__wailsMock.setResponse('GetAppVersion', 'override-one-shot');
    window.__wailsMock.clearResponse('GetAppVersion');
    const defaultVersion = await window.go.app.App.GetAppVersion();
    const app = window.go.app.App;
    const chat = window.go.wailsapi.Chat;
    let unreservedRejected = false;
    try { await chat.SendMessage('conversation', 'sem handoff', '', {}); }
    catch { unreservedRejected = true; }

    const begin = await app.BeginUICommand('chat.message.send') as Record<string, string>;
    const take = await app.TakeUICommand(begin.ticket) as Record<string, string>;
    let secondTakeRejected = false;
    try { await app.TakeUICommand(begin.ticket); }
    catch { secondTakeRejected = true; }
    const messageId = await chat.SendMessage('conversation', 'mensagem válida', '', { command: {
      ticket: take.ticket,
      handoffId: take.handoffId,
    } });
    const outcome = await app.GetUICommandResult(begin.ticket) as Record<string, string>;
    let replayRejected = false;
    try { await chat.SendMessage('conversation', 'replay', '', { command: {
      ticket: take.ticket,
      handoffId: take.handoffId,
    } }); }
    catch { replayRejected = true; }

    const transportBegin = await app.BeginUICommand('chat.message.send') as Record<string, string>;
    const transportTake = await app.TakeUICommand(transportBegin.ticket) as Record<string, string>;
    window.__wailsMock.setError('SendMessage', 'transport-failed');
    let transportRejected = false;
    try { await chat.SendMessage('conversation', 'erro de transporte', '', { command: {
      ticket: transportTake.ticket,
      handoffId: transportTake.handoffId,
    } }); }
    catch { transportRejected = true; }
    const transportOutcome = await app.GetUICommandResult(transportBegin.ticket) as Record<string, string>;
    window.__wailsMock.clearError('SendMessage');

    const callbackBegin = await app.BeginUICommand('chat.message.send') as Record<string, string>;
    const callbackTake = await app.TakeUICommand(callbackBegin.ticket) as Record<string, string>;
    const callbackFailure = new Error('response-callback-failed');
    window.__wailsMock.setResponse('SendMessage', () => Promise.reject(callbackFailure));
    let callbackRejectedOriginal = false;
    try { await chat.SendMessage('conversation', 'erro de callback', '', { command: {
      ticket: callbackTake.ticket,
      handoffId: callbackTake.handoffId,
    } }); }
    catch (error) { callbackRejectedOriginal = error === callbackFailure; }
    const callbackOutcome = await app.GetUICommandResult(callbackBegin.ticket) as Record<string, string>;
    window.__wailsMock.clearResponse('SendMessage');

    const concurrentBegin = await app.BeginUICommand('chat.message.send') as Record<string, string>;
    const concurrentTake = await app.TakeUICommand(concurrentBegin.ticket) as Record<string, string>;
    let releaseFirstSubmission: ((value: string) => void) | undefined;
    window.__wailsMock.setResponse('SendMessage', () => new Promise(resolve => { releaseFirstSubmission = resolve; }));
    const firstSubmission = chat.SendMessage('conversation', 'primeira pendente', '', { command: {
      ticket: concurrentTake.ticket,
      handoffId: concurrentTake.handoffId,
    } });
    let concurrentSubmitRejected = false;
    try { await chat.SendMessage('conversation', 'duplicada', '', { command: {
      ticket: concurrentTake.ticket,
      handoffId: concurrentTake.handoffId,
    } }); }
    catch { concurrentSubmitRejected = true; }
    releaseFirstSubmission?.('01926b90-0000-7000-8000-000000000003');
    await firstSubmission;
    const concurrentOutcome = await app.GetUICommandResult(concurrentBegin.ticket) as Record<string, string>;
    window.__wailsMock.clearResponse('SendMessage');

    const completionBegin = await app.BeginUICommand('chat.message.send') as Record<string, string>;
    const completionTake = await app.TakeUICommand(completionBegin.ticket) as Record<string, string>;
    let wrongCompletionRejected = false;
    try { await app.CompleteUICommand(completionBegin.ticket, completionTake.handoffId + '-wrong', 'cancelled'); }
    catch { wrongCompletionRejected = true; }
    await app.CompleteUICommand(completionBegin.ticket, completionTake.handoffId, 'cancelled');
    const completionOutcome = await app.GetUICommandResult(completionBegin.ticket) as Record<string, string>;

    const unknownCommand = await app.BeginUICommand('chat.message.delete');

    return { defaultVersion, begin, take, messageId, outcome, unreservedRejected, secondTakeRejected, replayRejected,
      transportBegin, transportRejected, transportOutcome, callbackBegin, callbackRejectedOriginal, callbackOutcome,
      concurrentBegin, concurrentSubmitRejected, concurrentOutcome, completionBegin, wrongCompletionRejected, completionOutcome,
      unknownCommand };
  });

  expect(result.defaultVersion).toBe('1.0.0-test');
  expect(result.begin).toMatchObject({ commandId: 'chat.message.send' });
  expect(result.take).toMatchObject({ ticket: result.begin.ticket, invocationId: result.begin.invocationId, commandId: result.begin.commandId });
  expect(result.messageId).toBe('01926b90-0000-7000-8000-000000000002');
  expect(result.outcome).toEqual({ invocationId: result.begin.invocationId, status: 'succeeded' });
  expect(result.unreservedRejected).toBe(true);
  expect(result.secondTakeRejected).toBe(true);
  expect(result.replayRejected).toBe(true);
  expect(result.transportRejected).toBe(true);
  expect(result.transportOutcome).toEqual({ invocationId: result.transportBegin.invocationId, status: 'outcome_unknown' });
  expect(result.callbackRejectedOriginal).toBe(true);
  expect(result.callbackOutcome).toEqual({ invocationId: result.callbackBegin.invocationId, status: 'outcome_unknown' });
  expect(result.concurrentSubmitRejected).toBe(true);
  expect(result.concurrentOutcome).toEqual({ invocationId: result.concurrentBegin.invocationId, status: 'succeeded' });
  expect(result.wrongCompletionRejected).toBe(true);
  expect(result.completionOutcome).toEqual({ invocationId: result.completionBegin.invocationId, status: 'cancelled' });
  expect(result.unknownCommand).toBeUndefined();
});
