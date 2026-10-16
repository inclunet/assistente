import { describe, expect, it } from 'vitest';
import { readChatCommandSurface, type ChatCommandSurfaceSource } from './commandChatSurface';

const source = (overrides: Partial<ChatCommandSurfaceSource> = {}): ChatCommandSurfaceSource => ({
  tabId: 'tab-a',
  conversationId: 'conversation-a',
  sessionKey: 'page:tab:tab-a:conversation-a',
  sessionLoaded: true,
  executionStatus: 'idle',
  ...overrides,
});

describe('readChatCommandSurface', () => {
  it('retorna null quando a conversa ou sessão não está disponível', () => {
    expect(readChatCommandSurface(null)).toBeNull();
    expect(readChatCommandSurface(source({ conversationId: '' }))).toBeNull();
    expect(readChatCommandSurface(source({ sessionLoaded: false }))).toBeNull();
  });

  it('relê fatos sem depender de notificação e muda a versão quando a execução muda', () => {
    let current = source();
    const getter = () => readChatCommandSurface(current);
    const before = getter();
    current = source({ executionStatus: 'running' });
    const after = getter();

    expect(before?.metadata?.executionStatus).toBe('idle');
    expect(after?.metadata?.executionStatus).toBe('running');
    expect(after?.snapshotVersion).not.toBe(before?.snapshotVersion);
  });

  it('produz contexto detached com identidade do recurso e sem conteúdo da conversa', () => {
    const context = readChatCommandSurface(source({ executionStatus: 'running' }));

    expect(context).toEqual({
      surfaceType: 'chat',
      surfaceId: 'tab-a',
      snapshotVersion: 'chat-command-v1:tab-a:conversation-a:page:tab:tab-a:conversation-a:loaded=1;execution=running',
      metadata: {
        conversationId: 'conversation-a',
        sessionKey: 'page:tab:tab-a:conversation-a',
        sessionLoaded: true,
        executionStatus: 'running',
        resourceIdentity: 'tab-a:conversation-a:page:tab:tab-a:conversation-a',
      },
    });
    expect(context).not.toHaveProperty('content');
    expect(context).not.toHaveProperty('selection');
  });
});
