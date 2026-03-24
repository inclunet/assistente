import { describe, expect, it, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useGridPageLandmarks } from './useGridPageLandmarks';
import { type Landmark } from './useLandmarkNavigation';

vi.mock('./useLandmarkNavigation', () => ({
  useLandmarkNavigation: vi.fn(),
}));

import { useLandmarkNavigation } from './useLandmarkNavigation';

const mockedUseLandmarkNavigation = useLandmarkNavigation as ReturnType<typeof vi.fn>;

describe('useGridPageLandmarks', () => {
  it('inclui topbar como primeiro landmark', () => {
    renderHook(() => useGridPageLandmarks({ pageClass: 'test-page' }));

    expect(mockedUseLandmarkNavigation).toHaveBeenCalledWith(
      expect.objectContaining({
        landmarks: expect.arrayContaining([
          expect.objectContaining({ id: 'topbar' }),
        ]),
        defaultLandmarkId: 'grid',
      }),
    );

    const { landmarks } = mockedUseLandmarkNavigation.mock.calls[0][0] as { landmarks: Landmark[] };
    expect(landmarks[0].id).toBe('topbar');
  });

  it('inclui toolbar e grid', () => {
    renderHook(() => useGridPageLandmarks({ pageClass: 'test-page' }));

    const { landmarks } = mockedUseLandmarkNavigation.mock.calls[0][0] as { landmarks: Landmark[] };
    const ids = landmarks.map((l: Landmark) => l.id);
    expect(ids).toEqual(['topbar', 'toolbar', 'grid']);
  });

  it('insere extraLandmarks entre toolbar e grid', () => {
    const extra: Landmark[] = [{
      id: 'extra',
      label: 'Extra',
      focus: () => true,
      contains: () => false,
    }];

    renderHook(() => useGridPageLandmarks({ pageClass: 'test-page', extraLandmarks: extra }));

    const { landmarks } = mockedUseLandmarkNavigation.mock.calls[0][0] as { landmarks: Landmark[] };
    const ids = landmarks.map((l: Landmark) => l.id);
    expect(ids).toEqual(['topbar', 'toolbar', 'extra', 'grid']);
  });
});
