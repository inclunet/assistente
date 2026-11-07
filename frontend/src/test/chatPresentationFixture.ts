import type { Message, MessageNode, TurnSegment } from '../lib/chatMessageTree';
import type { TimelineItem } from '../types/chatTimelineEvents';

type FixtureMessage = Omit<Partial<Message>, 'timelineItem'> & { timelineItem?: Partial<TimelineItem> };
export function messageFixture(source: FixtureMessage = {}): Message {
  return { id: 'message', conversationId: 'conversation', role: 'assistant', content: '', pinned: false,
    createdAt: '', timestamp: 0, isStreaming: false, internal: false, ...source,
    timelineItem: source.timelineItem as TimelineItem | undefined };
}
export function nodeFixture(source: Omit<Partial<MessageNode>, 'message'> & { message: FixtureMessage; hasChildren?: boolean }): MessageNode {
  return { itemId: `message:${source.message.id ?? "message"}`, level: 0, childCount: 0, children: [], ...source, message: messageFixture(source.message) };
}
export function segmentFixture(source: TurnSegment): TurnSegment { return source; }
