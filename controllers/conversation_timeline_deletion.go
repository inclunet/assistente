package controllers

import "context"

func (c *ConversationsController) deleteTimelineWithinMaintenance(ctx context.Context, ids []string) ([]string, error) {
	write := func() ([]string, error) { return c.deleteWithinMaintenance(ctx, ids) }
	if c.deleteTimeline == nil {
		return write()
	}
	return c.deleteTimeline(ctx, ids, write)
}
