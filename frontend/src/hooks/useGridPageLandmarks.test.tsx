import { describe, expect, it, vi, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useGridPageLandmarks } from './useGridPageLandmarks';
import { type Landmark } from './useLandmarkNavigation';

const mocks = vi.hoisted(() => ({
  useLandmarkNavigation: vi.fn(),
  parentOwns: vi.fn(() => false),
}));

vi.mock('./useLandmarkNavigation', () => ({
  useLandmarkNavigation: mocks.useLandmarkNavigation,
  useHasParentLandmarks: () => mocks.parentOwns(),
}));

describe('useGridPageLandmarks', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.parentOwns.mockReturnValue(false);
  });

  it('inclui topbar como primeiro landmark', () => {
    renderHook(() => useGridPageLandmarks({ pageClass: 'test-page' }));

    expect(mocks.useLandmarkNavigation).toHaveBeenCalledWith(
      expect.objectContaining({
        landmarks: expect.arrayContaining([
          expect.objectContaining({ id: 'topbar' }),
        ]),
        defaultLandmarkId: 'grid',
      }),
    );

    const { landmarks } = mocks.useLandmarkNavigation.mock.calls[0][0] as { landmarks: Landmark[] };
    expect(landmarks[0].id).toBe('topbar');
  });

  it('inclui toolbar e grid', () => {
    renderHook(() => useGridPageLandmarks({ pageClass: 'test-page' }));

    const { landmarks } = mocks.useLandmarkNavigation.mock.calls[0][0] as { landmarks: Landmark[] };
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

    const { landmarks } = mocks.useLandmarkNavigation.mock.calls[0][0] as { landmarks: Landmark[] };
    const ids = landmarks.map((l: Landmark) => l.id);
    expect(ids).toEqual(['topbar', 'toolbar', 'extra', 'grid']);
  });

  it('desabilita landmarks quando parent gerencia (ParentLandmarkProvider)', () => {
    mocks.parentOwns.mockReturnValue(true);

    renderHook(() => useGridPageLandmarks({ pageClass: 'test-page' }));

    expect(mocks.useLandmarkNavigation).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: false }),
    );
  });

  it('habilita landmarks quando não há parent gerenciando', () => {
    mocks.parentOwns.mockReturnValue(false);

    renderHook(() => useGridPageLandmarks({ pageClass: 'test-page' }));

    expect(mocks.useLandmarkNavigation).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: true }),
    );
  });
});
