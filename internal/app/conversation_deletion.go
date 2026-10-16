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
		finalize, err := a.subagentMgr.PrepareConversationDeletion(ctx, conversationIDs)
		if err != nil {
			return nil, err
		}
		finalizers = append(finalizers, finalize)
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

// prepareConversationRestoration usa a mesma ordem de gates da exclusão. O
// finalizador remove tombstones somente dos IDs efetivamente importados.
func (a *App) prepareConversationRestoration(ctx context.Context) (func([]string), error) {
	finalizers := make([]func([]string), 0, 3)
	releaseAll := func(conversationIDs []string) {
		for i := len(finalizers) - 1; i >= 0; i-- {
			finalizers[i](conversationIDs)
		}
	}

	if a.subagentMgr != nil {
		finalize, err := a.subagentMgr.PrepareConversationRestoration(ctx)
		if err != nil {
			return nil, err
		}
		finalizers = append(finalizers, finalize)
	}
	if a.streamMgr != nil {
		finalizers = append(finalizers, a.streamMgr.PrepareConversationRestoration())
	}
	if a.responseNotifier != nil {
		finalizers = append(finalizers, a.responseNotifier.PrepareConversationRestoration())
	}

	var once sync.Once
	return func(conversationIDs []string) {
		once.Do(func() { releaseAll(conversationIDs) })
	}, nil
}
