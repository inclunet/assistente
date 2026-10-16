import { describe, expect, it, vi } from 'vitest';
import { ChatMessagingStaleError, executeChatMessaging, type ChatMessagingTarget } from './commandChatMessaging';
import type { CommandContextualBackendPort } from './commandContextualBackendExecution';
import type { UICommandStatus } from './commandUIExecution';

function setup(status: UICommandStatus = 'succeeded') {
  const commandId = 'chat.message.edit.save' as const;
  const reservation = { ticket: 'edit-ticket', invocationId: 'edit-invocation', commandId };
  const target: ChatMessagingTarget = {
    commandId, instanceId: 'editor-instance', executionKind: 'backend',
    isCurrent: vi.fn(() => true), canCommit: vi.fn(() => true),
    prepareAdmission: vi.fn(async () => {}), execute: vi.fn(async () => {}),
    succeeded: vi.fn(), settled: vi.fn(), dispose: vi.fn(),
  };
  const port: CommandContextualBackendPort = {
    beginUICommand: vi.fn(async () => reservation),
    takeUICommand: vi.fn(async () => ({ ...reservation, handoffId: 'edit-handoff' })),
    commitBackendCommand: vi.fn(async () => {}),
    completeUICommand: vi.fn(async () => {}), cancelUICommand: vi.fn(async () => {}),
    getUICommandResult: vi.fn(async () => ({ invocationId: reservation.invocationId, status })),
  };
  return { target, port };
}

describe('salvar edição — confirmação autoritativa e uso único', () => {
  it('prepara antes do handoff e nunca conclui escrita com CompleteUICommand', async () => {
    const { target, port } = setup();
    expect(await executeChatMessaging(port, target)).toBe('succeeded');
    expect(target.prepareAdmission).toHaveBeenCalledExactlyOnceWith('edit-ticket');
    expect(vi.mocked(target.prepareAdmission!).mock.invocationCallOrder[0])
      .toBeLessThan(vi.mocked(port.takeUICommand).mock.invocationCallOrder[0]);
    expect(target.execute).toHaveBeenCalledExactlyOnceWith({ ticket: 'edit-ticket', handoffId: 'edit-handoff' });
    expect(port.commitBackendCommand).not.toHaveBeenCalled();
    expect(port.completeUICommand).not.toHaveBeenCalled();
    expect(target.succeeded).toHaveBeenCalledOnce();
    expect(target.dispose).toHaveBeenCalledOnce();
  });

  it('sem captura de conteúdo não admite nem salva', async () => {
    const { target, port } = setup();
    target.prepareAdmission = undefined;
    expect(await executeChatMessaging(port, target)).toBe('cancelled');
    expect(port.takeUICommand).not.toHaveBeenCalled();
    expect(target.execute).not.toHaveBeenCalled();
    expect(port.cancelUICommand).toHaveBeenCalledExactlyOnceWith('edit-ticket');
  });

  it('edição invalidada durante a preparação cancela sem efeito', async () => {
    const { target, port } = setup();
    vi.mocked(target.prepareAdmission!).mockImplementation(async () => {
      vi.mocked(target.isCurrent).mockReturnValue(false);
    });
    expect(await executeChatMessaging(port, target)).toBe('cancelled');
    expect(port.takeUICommand).not.toHaveBeenCalled();
    expect(target.execute).not.toHaveBeenCalled();
    expect(target.succeeded).not.toHaveBeenCalled();
  });

  it('revalida a revisão do rascunho depois do handoff', async () => {
    const { target, port } = setup();
    vi.mocked(port.takeUICommand).mockImplementation(async ticket => {
      vi.mocked(target.canCommit).mockReturnValue(false);
      return { ticket, invocationId: 'edit-invocation', commandId: target.commandId, handoffId: 'edit-handoff' };
    });
    expect(await executeChatMessaging(port, target)).toBe('cancelled');
    expect(target.execute).not.toHaveBeenCalled();
    expect(port.completeUICommand).toHaveBeenCalledExactlyOnceWith('edit-ticket', 'edit-handoff', 'cancelled');
  });

  it.each(['succeeded', 'failed', 'outcome_unknown'] as const)('resposta de commit perdida reconcilia %s sem repetir', async status => {
    const { target, port } = setup(status);
    vi.mocked(target.execute).mockRejectedValue(new Error('transport lost'));
    expect(await executeChatMessaging(port, target)).toBe(status);
    expect(target.execute).toHaveBeenCalledOnce();
    expect(port.getUICommandResult).toHaveBeenCalledOnce();
    expect(port.completeUICommand).not.toHaveBeenCalled();
    expect(port.cancelUICommand).not.toHaveBeenCalled();
    expect(target.succeeded).toHaveBeenCalledTimes(status === 'succeeded' ? 1 : 0);
  });

  it('resultado de outra invocação não limpa o rascunho', async () => {
    const { target, port } = setup();
    vi.mocked(port.getUICommandResult).mockResolvedValue({ invocationId: 'other', status: 'succeeded' });
    expect(await executeChatMessaging(port, target)).toBe('outcome_unknown');
    expect(target.succeeded).not.toHaveBeenCalled();
    expect(target.settled).not.toHaveBeenCalled();
    expect(target.execute).toHaveBeenCalledOnce();
  });

  it('falha ao consultar resultado não repete consulta ou atualização', async () => {
    const { target, port } = setup();
    vi.mocked(port.getUICommandResult).mockRejectedValue(new Error('result lost'));
    expect(await executeChatMessaging(port, target)).toBe('outcome_unknown');
    expect(port.getUICommandResult).toHaveBeenCalledOnce();
    expect(target.execute).toHaveBeenCalledOnce();
    expect(target.succeeded).not.toHaveBeenCalled();
    expect(port.completeUICommand).not.toHaveBeenCalled();
  });

  it('guarda pré-efeito obsoleta não é confundida com commit', async () => {
    const { target, port } = setup();
    vi.mocked(target.execute).mockRejectedValue(new ChatMessagingStaleError());
    expect(await executeChatMessaging(port, target)).toBe('cancelled');
    expect(port.getUICommandResult).not.toHaveBeenCalled();
    expect(target.succeeded).not.toHaveBeenCalled();
    expect(port.completeUICommand).toHaveBeenCalledExactlyOnceWith('edit-ticket', 'edit-handoff', 'cancelled');
  });
});
