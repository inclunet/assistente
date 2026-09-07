import { describe, expect, it, vi } from 'vitest';
import {
  pruneWorkspacePanelFocus,
  queueWorkspacePanelFocus,
  registerWorkspacePanelFocus,
} from './workspacePanelFocusRegistry';

describe('workspacePanelFocusRegistry', () => {
  it('descarta pedidos pendentes de abas removidas', () => {
    const handler = vi.fn(() => true);

    queueWorkspacePanelFocus('removed-tab');
    pruneWorkspacePanelFocus(new Set(['active-tab']));
    const unregister = registerWorkspacePanelFocus('removed-tab', handler);

    expect(handler).not.toHaveBeenCalled();
    unregister();
  });
});
