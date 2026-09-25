import { describe, expect, it, vi } from 'vitest';
import { CHAT_MESSAGE_ACTION_IDS, ChatMessagingStaleError, executeChatMessaging, type ChatMessagingTarget } from './commandChatMessaging';
import type { CommandContextualBackendPort } from './commandContextualBackendExecution';
import type { UICommandStatus } from './commandUIExecution';

function setup(commandId: ChatMessagingTarget['commandId'] = 'chat.message.send', status: UICommandStatus = 'succeeded') {
  const reservation = { ticket: 'ticket-1', invocationId: 'invocation-1', commandId };
  const target: ChatMessagingTarget = {
    commandId, instanceId: 'surface-1',
    isCurrent: vi.fn(() => true), canCommit: vi.fn(() => true),
    execute: vi.fn(async () => undefined), succeeded: vi.fn(), dispose: vi.fn(),
  };
  const port: CommandContextualBackendPort = {
    beginUICommand: vi.fn(async () => reservation),
    takeUICommand: vi.fn(async () => ({ ...reservation, handoffId: 'handoff-1' })),
    commitBackendCommand: vi.fn(async () => undefined),
    getUICommandResult: vi.fn(async () => ({ invocationId: 'invocation-1', status })),
    completeUICommand: vi.fn(async () => undefined), cancelUICommand: vi.fn(async () => undefined),
  };
  return { port, target };
}

describe('protocolo auditado do envio de chat', () => {
  it('abrir edição é apresentação local, sem reserva, fila ou auditoria', async () => {
    const { port, target } = setup('chat.message.edit.open');
    target.prepareAdmission = vi.fn(async () => {});
    target.waitForAdmission = vi.fn(async () => {});
    expect(await executeChatMessaging(port, target)).toBe('succeeded');
    expect(target.execute).toHaveBeenCalledOnce();
    expect(target.prepareAdmission).not.toHaveBeenCalled();
    expect(target.waitForAdmission).not.toHaveBeenCalled();
    expect(port.beginUICommand).not.toHaveBeenCalled();
    expect(port.completeUICommand).not.toHaveBeenCalled();
    expect(port.getUICommandResult).not.toHaveBeenCalled();
  });
  it.each(CHAT_MESSAGE_ACTION_IDS.filter(id => id !== 'chat.message.edit.open'))('%s prepara a mensagem antes do Take e não reenvia', async commandId => {
    const { port, target } = setup(commandId);
    target.prepareAdmission = vi.fn(async () => {});
    expect(await executeChatMessaging(port, target)).toBe('succeeded');
    expect(target.prepareAdmission).toHaveBeenCalledExactlyOnceWith('ticket-1');
    expect(vi.mocked(target.prepareAdmission).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(port.takeUICommand).mock.invocationCallOrder[0]);
    expect(target.execute).toHaveBeenCalledOnce();
    expect(port.commitBackendCommand).not.toHaveBeenCalled();
    if (commandId === 'chat.message.delete' || commandId === 'chat.message.pin.toggle' || commandId === 'chat.message.edit.save') {
      expect(port.completeUICommand).not.toHaveBeenCalled();
    } else {
      expect(port.completeUICommand).toHaveBeenCalledExactlyOnceWith('ticket-1', 'handoff-1', 'succeeded');
      expect(vi.mocked(port.completeUICommand).mock.invocationCallOrder[0]).toBeGreaterThan(vi.mocked(target.execute).mock.invocationCallOrder[0]);
    }
  });
  it('ação sobre mensagem sem preparação falha fechada', async () => {
    const { port, target } = setup('chat.message.delete');
    expect(await executeChatMessaging(port, target)).toBe('cancelled');
    expect(port.takeUICommand).not.toHaveBeenCalled();
    expect(port.cancelUICommand).toHaveBeenCalledExactlyOnceWith('ticket-1');
    expect(target.execute).not.toHaveBeenCalled();
  });
  it('recusa de preparação não abre decisão nem executa', async () => {
    const { port, target } = setup('chat.message.delete');
    target.prepareAdmission = vi.fn(async () => { throw new Error('target refused'); });
    expect(await executeChatMessaging(port, target)).toBe('failed');
    expect(port.takeUICommand).not.toHaveBeenCalled();
    expect(port.cancelUICommand).toHaveBeenCalledOnce();
    expect(target.execute).not.toHaveBeenCalled();
  });
  it('mudança de contexto durante preparação não chega ao Take', async () => {
    const { port, target } = setup('chat.message.pin.toggle');
    target.prepareAdmission = vi.fn(async () => { vi.mocked(target.isCurrent).mockReturnValue(false); });
    expect(await executeChatMessaging(port, target)).toBe('cancelled');
    expect(port.takeUICommand).not.toHaveBeenCalled();
    expect(target.execute).not.toHaveBeenCalled();
  });
  it('falha de clipboard é reportada como falha, sem repetir efeito', async () => {
    const { port, target } = setup('chat.message.copy', 'failed');
    target.prepareAdmission = vi.fn(async () => {});
    vi.mocked(target.execute).mockRejectedValue(new Error('clipboard unavailable'));
    expect(await executeChatMessaging(port, target)).toBe('failed');
    expect(port.completeUICommand).toHaveBeenCalledExactlyOnceWith('ticket-1', 'handoff-1', 'failed');
    expect(target.execute).toHaveBeenCalledOnce();
    expect(target.succeeded).not.toHaveBeenCalled();
  });
  it('resposta perdida de exclusão consulta o resultado sem repetir exclusão', async () => {
    const { port, target } = setup('chat.message.delete');
    target.prepareAdmission = vi.fn(async () => {});
    vi.mocked(target.execute).mockRejectedValue(new Error('transport lost'));
    expect(await executeChatMessaging(port, target)).toBe('succeeded');
    expect(target.execute).toHaveBeenCalledOnce();
    expect(port.completeUICommand).not.toHaveBeenCalled();
  });
  it('decisão de exclusão cancelada não é anunciada como falha nem executada', async () => {
    const { port, target } = setup('chat.message.delete', 'cancelled');
    target.prepareAdmission = vi.fn(async () => {});
    vi.mocked(port.takeUICommand).mockRejectedValue(new Error('decision cancelled'));
    expect(await executeChatMessaging(port, target)).toBe('cancelled');
    expect(target.execute).not.toHaveBeenCalled();
    expect(port.getUICommandResult).toHaveBeenCalledOnce();
  });
  it.each(['chat.message.send', 'chat.message.retry'] as const)('%s usa a correlação sem duplicar o commit de envio', async commandId => {
    const { port, target } = setup(commandId);
    expect(await executeChatMessaging(port, target)).toBe('succeeded');
    expect(target.execute).toHaveBeenCalledExactlyOnceWith({ ticket: 'ticket-1', handoffId: 'handoff-1' });
    expect(port.commitBackendCommand).not.toHaveBeenCalled();
    expect(port.completeUICommand).not.toHaveBeenCalled();
    expect(port.getUICommandResult).toHaveBeenCalledExactlyOnceWith('ticket-1');
    expect(target.succeeded).toHaveBeenCalledOnce();
    expect(target.dispose).toHaveBeenCalledOnce();
  });

  it('cancelamento só apresenta limpeza depois do resultado backend confirmado', async () => {
    const { port, target } = setup('chat.response.cancel');
    expect(await executeChatMessaging(port, target)).toBe('succeeded');
    expect(port.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('ticket-1', 'handoff-1');
    expect(target.execute).toHaveBeenCalledOnce();
    expect(vi.mocked(target.execute).mock.invocationCallOrder[0]).toBeGreaterThan(vi.mocked(port.getUICommandResult).mock.invocationCallOrder[0]);
  });

  it.each(['denied', 'failed', 'outcome_unknown', 'cancelled_stale'] as const)('Promise de envio resolvida não transforma %s em sucesso', async status => {
    const { port, target } = setup('chat.message.send', status);
    expect(await executeChatMessaging(port, target)).toBe(status);
    expect(target.succeeded).not.toHaveBeenCalled();
    expect(target.execute).toHaveBeenCalledOnce();
    expect(port.completeUICommand).not.toHaveBeenCalled();
  });

  it('recusa alvo obsoleto antes de criar a reserva', async () => {
    const { port, target } = setup();
    vi.mocked(target.isCurrent).mockReturnValue(false);
    expect(await executeChatMessaging(port, target)).toBe('cancelled');
    expect(port.beginUICommand).not.toHaveBeenCalled();
    expect(target.execute).not.toHaveBeenCalled();
    expect(target.dispose).toHaveBeenCalledOnce();
  });

  it('revalida depois da espera na fila e não reserva para outro contexto', async () => {
    const { port, target } = setup();
    target.waitForAdmission = vi.fn(async () => { vi.mocked(target.canCommit).mockReturnValue(false); });
    expect(await executeChatMessaging(port, target)).toBe('cancelled');
    expect(port.beginUICommand).not.toHaveBeenCalled();
  });

  it('contexto perdido durante Begin cancela a reserva sem executar', async () => {
    const { port, target } = setup();
    vi.mocked(target.isCurrent).mockReturnValueOnce(true).mockReturnValue(false);
    expect(await executeChatMessaging(port, target)).toBe('cancelled');
    expect(port.cancelUICommand).toHaveBeenCalledExactlyOnceWith('ticket-1');
    expect(port.takeUICommand).not.toHaveBeenCalled();
    expect(target.execute).not.toHaveBeenCalled();
  });

  it('contexto perdido durante Take conclui somente cancelamento', async () => {
    const { port, target } = setup();
    vi.mocked(target.canCommit).mockReturnValueOnce(true).mockReturnValue(false);
    expect(await executeChatMessaging(port, target)).toBe('cancelled');
    expect(port.completeUICommand).toHaveBeenCalledExactlyOnceWith('ticket-1', 'handoff-1', 'cancelled');
    expect(target.execute).not.toHaveBeenCalled();
  });

  it('revalidação falha antes da chamada Wails pode cancelar o ticket tomado', async () => {
    const { port, target } = setup();
    vi.mocked(target.execute).mockRejectedValue(new ChatMessagingStaleError());
    expect(await executeChatMessaging(port, target)).toBe('cancelled');
    expect(port.completeUICommand).toHaveBeenCalledExactlyOnceWith('ticket-1', 'handoff-1', 'cancelled');
    expect(port.getUICommandResult).not.toHaveBeenCalled();
  });

  it('transporte perdido após submissão não repete nem afirma cancelamento', async () => {
    const { port, target } = setup();
    vi.mocked(target.execute).mockRejectedValue(new Error('lost-response'));
    vi.mocked(port.getUICommandResult).mockRejectedValue(new Error('result-unavailable'));
    expect(await executeChatMessaging(port, target)).toBe('outcome_unknown');
    expect(target.execute).toHaveBeenCalledOnce();
    expect(port.beginUICommand).toHaveBeenCalledOnce();
    expect(port.completeUICommand).not.toHaveBeenCalled();
    expect(port.cancelUICommand).not.toHaveBeenCalled();
    expect(target.succeeded).not.toHaveBeenCalled();
  });

  it('consulta o resultado correlacionado após perda da resposta sem repetir o envio', async () => {
    const { port, target } = setup();
    vi.mocked(target.execute).mockRejectedValue(new Error('lost-response'));
    expect(await executeChatMessaging(port, target)).toBe('succeeded');
    expect(target.execute).toHaveBeenCalledOnce();
    expect(port.getUICommandResult).toHaveBeenCalledExactlyOnceWith('ticket-1');
    expect(target.succeeded).toHaveBeenCalledOnce();
    expect(port.cancelUICommand).not.toHaveBeenCalled();
  });

  it('resultado de outra invocação não limpa o rascunho', async () => {
    const { port, target } = setup();
    vi.mocked(port.getUICommandResult).mockResolvedValue({ invocationId: 'other', status: 'succeeded' });
    expect(await executeChatMessaging(port, target)).toBe('outcome_unknown');
    expect(target.succeeded).not.toHaveBeenCalled();
  });

  it('falha de apresentação não reclassifica commit confirmado', async () => {
    const { port, target } = setup();
    vi.mocked(target.succeeded!).mockImplementation(() => { throw new Error('presentation'); });
    expect(await executeChatMessaging(port, target)).toBe('succeeded');
    expect(target.execute).toHaveBeenCalledOnce();
  });
});
