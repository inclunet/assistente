import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { GetPinnedMessages, ToggleMessagePin } from '@wailsjs/go/wailsapi/Conversations';
import { EventsOn } from '@wailsjs/runtime/runtime';
import type { database } from '../../../wailsjs/go/models';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { handleError, ErrorSeverity } from '../../utils/errorHandler';
import { Button } from '../ui/Button';
import { Modal } from '../ui/Modal';
import './PinnedMessagesModal.css';

interface PinnedMessagesModalProps {
  conversationId: string;
  isOpen: boolean;
  onClose: () => void;
}

function roleKey(role: string): string {
  if (role === 'user') return 'chat.you';
  if (role === 'tool') return 'chat.result';
  if (role === 'system') return 'chat.system';
  return 'chat.assistant';
}

export function PinnedMessagesModal({
  conversationId,
  isOpen,
  onClose,
}: PinnedMessagesModalProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const [messages, setMessages] = useState<database.ChatMessage[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!isOpen || !conversationId) return;
    let active = true;
    const load = async () => {
      setLoading(true);
      try {
        const result = await GetPinnedMessages(conversationId);
        if (active) setMessages(result || []);
      } catch (error) {
        handleError(error, {
          source: 'PinnedMessagesModal.load',
          userMessage: t('chat.pins.loadError'),
          severity: ErrorSeverity.RECOVERABLE,
        });
      } finally {
        if (active) setLoading(false);
      }
    };
    void load();
    const unsubscribe = EventsOn('message:pin_changed', (data: unknown) => {
      const event = data as { conversationId?: string };
      if (event.conversationId === conversationId) void load();
    });
    return () => {
      active = false;
      unsubscribe();
    };
  }, [conversationId, isOpen, t]);

  const unpin = async (messageId: string) => {
    try {
      const result = await ToggleMessagePin(messageId);
      announce(result.pinned ? t('chat.announce.messagePinned') : t('chat.announce.messageUnpinned'));
    } catch (error) {
      handleError(error, {
        source: 'PinnedMessagesModal.unpin',
        userMessage: t('chat.pins.toggleError'),
        severity: ErrorSeverity.RECOVERABLE,
      });
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={t('chat.pins.title')} size="md" readingMode>
      <div className="pinned-messages">
        <p className="pinned-messages__description">{t('chat.pins.description')}</p>
        {loading ? (
          <p>{t('chat.pins.loading')}</p>
        ) : messages.length === 0 ? (
          <p>{t('chat.pins.empty')}</p>
        ) : (
          <ul className="pinned-messages__list" aria-label={t('chat.pins.listLabel')}>
            {messages.map((message) => (
              <li className="pinned-messages__item" key={message.id}>
                <article>
                  <h3>{t(roleKey(message.role))}</h3>
                  <p>{message.content || t('chat.pins.noTextContent')}</p>
                  {(message.parentId || message.role === 'tool') && (
                    <p className="pinned-messages__context">
                      {message.parentId ? t('chat.pins.threadMessage') : t('chat.pins.toolMessage')}
                    </p>
                  )}
                  <Button
                    type="button"
                    variant="secondary"
                    size="sm"
                    onClick={() => void unpin(message.id)}
                  >
                    {t('chat.unpinMessage')}
                  </Button>
                </article>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  );
}
