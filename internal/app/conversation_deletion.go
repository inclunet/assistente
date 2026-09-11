package app

import (
	"context"
	"sync"
)

// prepareConversationDeletion fecha, em ordem estável, os gates de trabalho
// efêmero que poderiam recriar dados depois do commit da exclusão.
func (a *App) prepareConversationDeletion(ctx context.Context, conversationIDs []string) (func(), error) {
	releases := make([]func(), 0, 3)
	releaseAll := func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}

	if a.subagentMgr != nil {
		release, err := a.subagentMgr.PrepareConversationDeletion(ctx, conversationIDs)
		if err != nil {
			return nil, err
		}
		releases = append(releases, release)
	}
	if a.streamMgr != nil {
		release, err := a.streamMgr.PrepareConversationDeletion(conversationIDs)
		if err != nil {
			releaseAll()
			return nil, err
		}
		releases = append(releases, release)
	}
	if a.responseNotifier != nil {
		release, err := a.responseNotifier.PrepareConversationDeletion(conversationIDs)
		if err != nil {
			releaseAll()
			return nil, err
		}
		releases = append(releases, release)
	}

	var once sync.Once
	return func() {
		once.Do(releaseAll)
	}, nil
}
