import { describe, expect, it, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useContentPageLandmarks } from './useContentPageLandmarks';
import { type Landmark } from './useLandmarkNavigation';

vi.mock('./useLandmarkNavigation', () => ({
  useLandmarkNavigation: vi.fn(),
}));

import { useLandmarkNavigation } from './useLandmarkNavigation';

const mockedUseLandmarkNavigation = useLandmarkNavigation as ReturnType<typeof vi.fn>;

describe('useContentPageLandmarks', () => {
  it('inclui topbar como primeiro landmark', () => {
    renderHook(() => useContentPageLandmarks({ pageClass: 'about-page' }));

    const { landmarks } = mockedUseLandmarkNavigation.mock.calls[0][0] as { landmarks: Landmark[] };
    expect(landmarks[0].id).toBe('topbar');
  });

  it('inclui pageContent como último landmark e default', () => {
    renderHook(() => useContentPageLandmarks({ pageClass: 'about-page' }));

    expect(mockedUseLandmarkNavigation).toHaveBeenCalledWith(
      expect.objectContaining({
        defaultLandmarkId: 'pageContent',
      }),
    );

    const { landmarks } = mockedUseLandmarkNavigation.mock.calls[0][0] as { landmarks: Landmark[] };
    const ids = landmarks.map((l: Landmark) => l.id);
    expect(ids).toEqual(['topbar', 'pageContent']);
  });

  it('insere extraLandmarks entre topbar e pageContent', () => {
    const extra: Landmark[] = [{
      id: 'sidebar',
      label: 'Sidebar',
      focus: () => true,
      contains: () => false,
    }];

    renderHook(() => useContentPageLandmarks({ pageClass: 'about-page', extraLandmarks: extra }));

    const { landmarks } = mockedUseLandmarkNavigation.mock.calls[0][0] as { landmarks: Landmark[] };
    const ids = landmarks.map((l: Landmark) => l.id);
    expect(ids).toEqual(['topbar', 'sidebar', 'pageContent']);
  });
});
