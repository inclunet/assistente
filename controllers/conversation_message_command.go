package controllers

import (
	"context"
	"errors"
	"strings"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
)

// CommitMessageCommandGuarded is an internal command path: the executor already
// consumed the destructive receipt. Never request a second legacy confirmation.
func (c *ConversationsController) CommitMessageCommandGuarded(ctx context.Context, conversationID, messageID, fingerprint string, deleting bool, guard func(func() error) error) error {
	return c.commitMessageCommandGuarded(ctx, conversationID, messageID, fingerprint, deleting, nil, guard)
}

func (c *ConversationsController) UpdateMessageCommandGuarded(ctx context.Context, conversationID, messageID, fingerprint, content string, guard func(func() error) error) error {
	if strings.TrimSpace(content) == "" || len(content) > chat.MaxMessageContentSize {
		return errors.New("invalid message edit content")
	}
	return c.commitMessageCommandGuarded(ctx, conversationID, messageID, fingerprint, false, &content, guard)
}

func (c *ConversationsController) commitMessageCommandGuarded(ctx context.Context, conversationID, messageID, fingerprint string, deleting bool, content *string, guard func(func() error) error) error {
	if guard == nil {
		return errors.New("message command guard required")
	}
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return err
	}
	if (deleting || content != nil) && c.prepareBatchDelete != nil {
		finalize, err := c.prepareBatchDelete(ctx, []string{conversationID})
		if err != nil {
			return err
		}
		if finalize != nil {
			defer finalize(false)
		}
	}
	return database.WithConversationLifecycle(ctx, func() error {
		return guard(func() error {
			var message *database.ChatMessage
			var err error
			if content != nil {
				message, err = database.UpdateMessageIfUnchangedWithinLifecycleWithContext(ctx, conversationID, messageID, fingerprint, *content)
			} else {
				message, err = database.MutateMessageIfUnchangedWithinLifecycleWithContext(ctx, conversationID, messageID, fingerprint, deleting)
			}
			if err != nil {
				return err
			}
			if content != nil {
				c.messageUpdated(conversationID, messageID, *content)
			} else if deleting {
				c.messageDeleted(conversationID, messageID)
			} else {
				c.messagePinChanged(message)
			}
			return nil
		})
	})
}

func (c *ConversationsController) messageDeleted(conversationID, messageID string) {
	c.emit("message:deleted", ports.MessageDeletedEvent{ConversationID: conversationID, MessageID: messageID})
}

func (c *ConversationsController) messagePinChanged(message *database.ChatMessage) {
	c.emit("message:pin_changed", ports.MessagePinChangedEvent{ConversationID: message.ConversationID, MessageID: message.ID, Pinned: message.Pinned})
}

func (c *ConversationsController) messageUpdated(conversationID, messageID, content string) {
	c.emit("message:updated", ports.MessageUpdatedEvent{ConversationID: conversationID, MessageID: messageID, Content: content})
}
