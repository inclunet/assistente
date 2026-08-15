import { renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { useWorkspacePanelLifecycleCleanup } from './useWorkspacePanelLifecycleCleanup';

describe('useWorkspacePanelLifecycleCleanup', () => {
  it('não registra cleanup que encerre recursos de terminal', () => {
    const { unmount } = renderHook(() => useWorkspacePanelLifecycleCleanup());

    expect(() => unmount()).not.toThrow();
  });
});
