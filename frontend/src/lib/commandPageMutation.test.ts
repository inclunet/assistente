import { describe, it, expect, vi } from 'vitest';
import { executePageMutation, type PageMutationPort, type PageMutationTarget } from './commandPageMutation';

function fixture() {
  const reservation = { ticket: 'ticket', invocationId: 'invocation', commandId: 'tasklists.update' };
  const request = { targetId: 'list', expectedFingerprint: 'revision', title: 'Captured', description: '' };
  const target: PageMutationTarget = {
    commandId: 'tasklists.update', instanceId: 'form', isCurrent: vi.fn(() => true), canCommit: vi.fn(() => true),
    readRequest: vi.fn(async () => request), startDecision: vi.fn(), finishDecision: vi.fn(() => true),
    succeeded: vi.fn(async () => undefined), dispose: vi.fn(),
  };
  const port: PageMutationPort = {
    beginUICommand: vi.fn(async () => reservation), preparePageMutationCommand: vi.fn(async () => undefined),
    takeUICommand: vi.fn(async () => ({ ...reservation, handoffId: 'handoff' })),
    commitBackendCommand: vi.fn(async () => undefined),
    getUICommandResult: vi.fn(async () => ({ invocationId: 'invocation', status: 'succeeded' as const })),
    getPageMutationCommandResult: vi.fn(async () => ({ id: 'list', title: 'Captured' })),
    cancelUICommand: vi.fn(async () => undefined), completeUICommand: vi.fn(async () => undefined),
  };
  return { target, port, request };
}
describe('page mutations sealed backend execution', () => {
  it('prepares captured content then commits once and presents only ledger success', async () => {
    const { target, port, request } = fixture();
    expect(await executePageMutation(port, target)).toEqual({ status: 'succeeded', result: { id: 'list', title: 'Captured' } });
    expect(port.preparePageMutationCommand).toHaveBeenCalledWith('ticket', request);
    expect(port.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff');
    expect(target.succeeded).toHaveBeenCalledOnce(); expect(target.dispose).toHaveBeenCalledOnce();
  });
  it('does not reserve after asynchronous target reading becomes stale', async () => {
    const { target, port } = fixture();
    vi.mocked(target.readRequest).mockImplementation(async () => { vi.mocked(target.canCommit).mockReturnValue(false); return { targetId: '', expectedFingerprint: '', title: '', description: '' }; });
    expect((await executePageMutation(port, target)).status).toBe('cancelled');
    expect(port.beginUICommand).not.toHaveBeenCalled();
  });
  it('cancels prepared reservation if source changed', async () => {
    const { target, port } = fixture(); vi.mocked(target.isCurrent).mockReturnValue(false);
    expect((await executePageMutation(port, target)).status).toBe('cancelled');
    expect(port.cancelUICommand).toHaveBeenCalledWith('ticket'); expect(port.commitBackendCommand).not.toHaveBeenCalled();
  });
  it('rejects a mismatched handoff without submitting a write', async () => {
    const { target, port } = fixture(); vi.mocked(port.takeUICommand).mockResolvedValue({ ticket: 'ticket', invocationId: 'other', commandId: 'tasklists.update', handoffId: 'handoff' });
    expect((await executePageMutation(port, target)).status).toBe('cancelled');
    expect(port.commitBackendCommand).not.toHaveBeenCalled();
  });
  it('does not retry or present after transport loses the commit response', async () => {
    const { target, port } = fixture(); vi.mocked(port.commitBackendCommand).mockRejectedValue(new Error('lost'));
    expect((await executePageMutation(port, target)).status).toBe('outcome_unknown');
    expect(port.commitBackendCommand).toHaveBeenCalledOnce(); expect(port.cancelUICommand).not.toHaveBeenCalled(); expect(target.succeeded).not.toHaveBeenCalled();
  });
  it('does not report failure of presentation as failure of a persisted write', async () => {
    const { target, port } = fixture(); vi.mocked(target.succeeded).mockRejectedValue(new Error('unmounted'));
    expect((await executePageMutation(port, target)).status).toBe('succeeded');
  });
  it('keeps the committed result available when the source invalidates during result retrieval', async () => {
    const { target, port } = fixture();
    vi.mocked(port.commitBackendCommand).mockImplementation(async () => {
      vi.mocked(target.isCurrent).mockReturnValue(false);
    });
    const outcome = await executePageMutation(port, target);
    expect(outcome).toEqual({ status: 'succeeded', result: { id: 'list', title: 'Captured' } });
    expect(port.getPageMutationCommandResult).toHaveBeenCalledOnce();
    expect(target.succeeded).toHaveBeenCalledOnce();
  });
  it('never exposes private result for a denied execution', async () => {
    const { target, port } = fixture(); vi.mocked(port.getUICommandResult).mockResolvedValue({ invocationId: 'invocation', status: 'denied' });
    expect((await executePageMutation(port, target)).status).toBe('denied');
    expect(port.getPageMutationCommandResult).not.toHaveBeenCalled(); expect(target.succeeded).not.toHaveBeenCalled();
  });
});
