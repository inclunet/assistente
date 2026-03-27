import { describe, expect, it, vi, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useContentPageLandmarks } from './useContentPageLandmarks';
import { type Landmark } from './useLandmarkNavigation';

const mocks = vi.hoisted(() => ({
  useLandmarkNavigation: vi.fn(),
  parentOwns: vi.fn(() => false),
}));

vi.mock('./useLandmarkNavigation', () => ({
  useLandmarkNavigation: mocks.useLandmarkNavigation,
  useHasParentLandmarks: () => mocks.parentOwns(),
}));

describe('useContentPageLandmarks', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.parentOwns.mockReturnValue(false);
  });

  it('inclui topbar como primeiro landmark', () => {
    renderHook(() => useContentPageLandmarks({ pageClass: 'about-page' }));

    const { landmarks } = mocks.useLandmarkNavigation.mock.calls[0][0] as { landmarks: Landmark[] };
    expect(landmarks[0].id).toBe('topbar');
  });

  it('inclui pageContent como último landmark e default', () => {
    renderHook(() => useContentPageLandmarks({ pageClass: 'about-page' }));

    expect(mocks.useLandmarkNavigation).toHaveBeenCalledWith(
      expect.objectContaining({
        defaultLandmarkId: 'pageContent',
      }),
    );

    const { landmarks } = mocks.useLandmarkNavigation.mock.calls[0][0] as { landmarks: Landmark[] };
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

    const { landmarks } = mocks.useLandmarkNavigation.mock.calls[0][0] as { landmarks: Landmark[] };
    const ids = landmarks.map((l: Landmark) => l.id);
    expect(ids).toEqual(['topbar', 'sidebar', 'pageContent']);
  });

  it('desabilita landmarks quando parent gerencia (ParentLandmarkProvider)', () => {
    mocks.parentOwns.mockReturnValue(true);

    renderHook(() => useContentPageLandmarks({ pageClass: 'about-page' }));

    expect(mocks.useLandmarkNavigation).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: false }),
    );
  });

  it('habilita landmarks quando não há parent gerenciando', () => {
    mocks.parentOwns.mockReturnValue(false);

    renderHook(() => useContentPageLandmarks({ pageClass: 'about-page' }));

    expect(mocks.useLandmarkNavigation).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: true }),
    );
  });
});
