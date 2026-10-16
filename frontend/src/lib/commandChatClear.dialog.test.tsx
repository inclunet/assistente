import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { DecisionDialog } from '../components/ui/DecisionDialog';
import { Modal } from '../components/ui/Modal';
import { getModalRegistrySnapshot } from './modalRegistry';
import { CHAT_CLEAR_COMMAND, captureChatClearTarget, executeChatClear, registerChatClearSurface } from './commandChatClear';
import type { CommandContextualBackendPort } from './commandContextualBackendExecution';
import type { UICommandTakeResponse } from './commandUIExecution';

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

describe('chat clear with real DecisionDialog and modal stack', () => {
  it.each(['page-confirm', 'modal-confirm', 'modal-cancel'] as const)('%s preserves origin until decision closes', async scenario => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const modal = scenario.startsWith('modal');
    const reservation = { ticket: 'clear-ticket', invocationId: 'clear-invocation', commandId: CHAT_CLEAR_COMMAND };
    let accept!: (value: UICommandTakeResponse) => void;
    let reject!: (error: Error) => void;
    const pending = new Promise<UICommandTakeResponse>((resolve, fail) => { accept = resolve; reject = fail; });
    const port: CommandContextualBackendPort = {
      beginUICommand: vi.fn(async () => reservation), takeUICommand: vi.fn(() => pending),
      commitBackendCommand: vi.fn(async () => undefined),
      getUICommandResult: vi.fn(async () => ({ invocationId: reservation.invocationId, status: 'succeeded' as const })),
      completeUICommand: vi.fn(async () => undefined), cancelUICommand: vi.fn(async () => undefined),
    };
    const succeeded = vi.fn();
    let closeDecision = () => {};
    const origin = <section data-testid="origin"><textarea aria-label="chat input" /></section>;
    const tree = (open: boolean) => <>
      {modal ? <Modal isOpen title="Chat" onClose={() => {}}>{origin}</Modal> : origin}
      <DecisionDialog isOpen={open} title="Clear conversation" description="Delete test content?" severity="destructive"
        actions={[{ id: 'apply', label: 'Apply', primary: true, polarity: 'affirmative', scope: 'current' }]}
        onAction={() => { accept({ ...reservation, handoffId: 'clear-handoff' }); closeDecision(); }}
        onCancel={() => { reject(new Error('cancelled')); closeDecision(); }} />
    </>;
    const view = render(tree(false));
    closeDecision = () => view.rerender(tree(false));
    const root = screen.getByTestId('origin');
    const originID = getModalRegistrySnapshot().topID;
    const unregister = registerChatClearSurface({
      root, instanceId: 'real-dialog-origin', modalId: originID ?? undefined,
      isCurrent: () => root.isConnected,
      canStart: () => getModalRegistrySnapshot().topID === originID,
      subscribe: () => () => {}, succeeded,
    });
    const target = captureChatClearTarget(() => '/')!;
    expect(target).toBeDefined();
    let execution!: ReturnType<typeof executeChatClear>;
    try {
      await act(async () => { execution = executeChatClear(port, target); });
      await waitFor(() => expect(port.takeUICommand).toHaveBeenCalledOnce());
      view.rerender(tree(true));
      await screen.findByRole('alertdialog');
      expect(target.isCurrent()).toBe(true);
      expect(target.canCommit()).toBe(false);
      expect(port.commitBackendCommand).not.toHaveBeenCalled();
      if (scenario.endsWith('cancel')) {
        fireEvent.keyDown(document.activeElement!, { key: 'Escape', code: 'Escape' });
      } else {
        fireEvent.click(screen.getByRole('button', { name: /Apply/ }));
      }
      const status = await execution;
      expect(status).toBe(scenario.endsWith('cancel') ? 'cancelled' : 'succeeded');
      expect(getModalRegistrySnapshot().topID).toBe(originID);
      expect(port.commitBackendCommand).toHaveBeenCalledTimes(scenario.endsWith('cancel') ? 0 : 1);
      expect(succeeded).toHaveBeenCalledTimes(scenario.endsWith('cancel') ? 0 : 1);
    } finally { unregister(); target.dispose(); }
  });
});
