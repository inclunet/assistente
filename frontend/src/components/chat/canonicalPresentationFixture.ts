import { chat } from '../../../wailsjs/go/models';
import { useMemo } from 'react';
import { useChatStore } from '../../store/chatStore';
import { patchChatConversation, getConversationTimeline } from '../../services/chatSessionRegistry';
import type { MessageNode, Message } from '../../lib/chatMessageTree';
import type { TimelineItem } from '../../types/chatTimelineEvents';

const unavailable = new Set<string>();
const canonicalOverrides = new Map<string, TimelineItem>();
export function setFixtureCanonicalItem(id: string, item?: TimelineItem) { if (item) canonicalOverrides.set(id, item); else canonicalOverrides.delete(id); }
export function setFixtureSourceUnavailable(id: string, value: boolean) { if (value) unavailable.add(id); else unavailable.delete(id); }

// Fixture dos testes de comandos e ownership. A origem dos dados é injetada;
// protocolo, IDs visuais e composição do backend têm provas próprias. Preserva
// as referências dos objetos para continuar exercitando os guards CAS/ABA reais.
export function useChatTimelinePresentation(conversationId: string | null, sessionKey: string) {
  useChatStore(state => state);
  const presentation = useMemo(() => {
    if (!conversationId) return null;
    const roots = (): MessageNode[] => {
      const state = useChatStore.getState();
      const timeline = getConversationTimeline(state, conversationId);
      const session = state.surfaceSessionsByKey[sessionKey] ?? state.sessionsByConversationId[conversationId];
      const values = session?.visibleThreadedMessages ?? timeline?.threadedMessages ?? state.sessionsByConversationId[conversationId]?.conversation?.threadedMessages ?? [];
      const decorate = (nodes: MessageNode[]) => {
        for (const node of nodes) {
          node.itemId ??= node.message.id;
          node.message.timelineItem = canonicalOverrides.get(node.message.id) ?? { itemId: node.itemId, representativeMessageId: node.message.id, scopeParentId: node.message.parentId ?? null,
            metadata: { threadParentMessageId: node.message.id }, segments: [] } as unknown as TimelineItem;
          decorate(node.children ?? []);
        }
      };
      decorate(values);
      return values;
    };
    const knownCanonical = new Set<string>();
    const find = (id: string): Message | undefined => {
      if (unavailable.has(conversationId)) return undefined;
      roots();
      const state = useChatStore.getState();
      const canonical = findNode(id, getConversationTimeline(state, conversationId)?.threadedMessages ?? []);
      if (canonical) { knownCanonical.add(id); canonical.message.timelineItem = canonicalOverrides.get(id) ?? { itemId: canonical.itemId ?? id, representativeMessageId: id, scopeParentId: canonical.message.parentId ?? null, metadata: { threadParentMessageId: id }, segments: [] } as unknown as TimelineItem; return canonical.message; }
      if (knownCanonical.has(id)) return undefined;
      const session = state.surfaceSessionsByKey[sessionKey];
      if (session?.conversationId !== conversationId) return undefined;
      return findNode(id, roots())?.message;
    };
    return {
      getMessage: find, getMessageByRepresentative: find,
      getCopyTarget: (id: string) => {
        const item = canonicalOverrides.get(id);
        return item?.recoveryReference ? { windowId: 'copy-window', identity: { itemId: item.itemId, epoch: 'copy-epoch', revision: item.revision, reference: item.recoveryReference } } : undefined;
      },
      subscribe: useChatStore.subscribe, getRevision: () => 0,
      get window() {
        const state = useChatStore.getState();
        const session = state.surfaceSessionsByKey[sessionKey] ?? state.sessionsByConversationId[conversationId];
        if (!getConversationTimeline(state, conversationId) && !state.sessionsByConversationId[conversationId]?.conversation) return undefined;
        return { metadata: session?.messageWindow ?? { totalCount: roots().length, startIndex: 0, endIndex: roots().length, hasBefore: false, hasAfter: false } };
      },
      nodes: (parent?: string) => parent ? findNode(parent, roots())?.children ?? [] : roots(),
      threadWindow: () => undefined,
      openThread: async (parent: string) => { await useChatStore.getState().loadMessageChildren(parent); },
      closeThread: async () => {}, retain: async () => {}, refresh: async () => {},
      recoveredItems: [], loading: false, error: undefined,
    };
  }, [conversationId, sessionKey]);
  return { presentation, nodes: presentation?.nodes() ?? [], revision: 0 };
}
function findNode(id: string, nodes: MessageNode[]): MessageNode | undefined {
  for (const node of nodes) { if (node.message.id === id) return node; const child = findNode(id, node.children ?? []); if (child) return child; }
}

export function publishFixtureMessage(conversationId: string, id: string, content: string) {
  const change = (nodes: MessageNode[]): MessageNode[] => nodes.map(node => ({ ...node,
    message: node.message.id === id ? Object.assign(new chat.EnrichedMessage({ ...node.message, content, turnSegments: [{ type: 'text', content }] }), { _turnSegments: undefined }) : node.message,
    children: change(node.children ?? []),
  }));
  useChatStore.setState(state => patchChatConversation(state, conversationId, conversation => ({ ...conversation, threadedMessages: change(conversation.threadedMessages) })));
}
