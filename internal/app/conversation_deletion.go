package app

import (
	"context"
	"sync"
)

// prepareConversationDeletion fecha, em ordem estável, os gates de trabalho
// efêmero que poderiam recriar dados depois do commit da exclusão.
func (a *App) prepareConversationDeletion(ctx context.Context, conversationIDs []string) (func(committed bool), error) {
	finalizers := make([]func(bool), 0, 3)
	finalizeAll := func(committed bool) {
		for i := len(finalizers) - 1; i >= 0; i-- {
			finalizers[i](committed)
		}
	}

	if a.subagentMgr != nil {
		release, err := a.subagentMgr.PrepareConversationDeletion(ctx, conversationIDs)
		if err != nil {
			return nil, err
		}
		finalizers = append(finalizers, func(bool) { release() })
	}
	if a.streamMgr != nil {
		finalize, err := a.streamMgr.PrepareConversationDeletion(conversationIDs)
		if err != nil {
			finalizeAll(false)
			return nil, err
		}
		finalizers = append(finalizers, finalize)
	}
	if a.responseNotifier != nil {
		finalize, err := a.responseNotifier.PrepareConversationDeletion(conversationIDs)
		if err != nil {
			finalizeAll(false)
			return nil, err
		}
		finalizers = append(finalizers, finalize)
	}

	var once sync.Once
	return func(committed bool) {
		once.Do(func() { finalizeAll(committed) })
	}, nil
}
