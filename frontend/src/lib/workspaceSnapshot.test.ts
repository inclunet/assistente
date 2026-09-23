import { describe, expect, it } from 'vitest';
import { compareWorkspaceSnapshots, parseWorkspaceSnapshot } from './workspaceSnapshot';

const payload = (sequence: unknown, epoch = 'epoch-1') => ({
  id: 'ws-1',
  name: 'Workspace',
  snapshot_epoch: epoch,
  snapshot_sequence: sequence,
  tabs: { items: [], active: undefined },
});

describe('workspaceSnapshot', () => {
  it.each(['0', '01', ' 1', 1, '18446744073709551616'])('rejects non-canonical sequence %p', (sequence) => {
    expect(parseWorkspaceSnapshot(payload(sequence))).toBeNull();
  });

  it('accepts positive canonical uint64 decimal strings', () => {
    expect(parseWorkspaceSnapshot(payload('18446744073709551615'))?.sequence)
      .toBe('18446744073709551615');
  });

  it.each([
    { tabs: { items: {} } },
    { tabs: { items: [{ id: 'tab-1', type: 'chat', position: 0 }, { id: 'tab-1', type: 'chat', position: 1 }] } },
    { tabs: { items: [{ id: 'tab-1', type: 'chat', title: 'Chat', position: 0 }], active: 'missing' } },
    { tabs: { items: [{ id: 'tab-1', type: 'chat', title: 'Chat', position: 0 }], active: '' } },
    { tabs: { items: [{ id: 'tab-1', type: 'chat', title: 'Chat', position: -1 }] } },
  ])('rejects malformed consumed workspace shape %#', (shape) => {
    expect(parseWorkspaceSnapshot({ ...payload('1'), ...shape })).toBeNull();
  });

  it('allows an empty workspace with an empty active id', () => {
    expect(parseWorkspaceSnapshot({ ...payload('1'), tabs: { items: [], active: '' } })).not.toBeNull();
    expect(parseWorkspaceSnapshot({ ...payload('1'), tabs: { items: null, active: '' } })).not.toBeNull();
    expect(parseWorkspaceSnapshot({ ...payload('1'), tabs: { items: null, active: undefined } })).toBeNull();
  });

  it('compares decimal sequences lexically by normalized length', () => {
    const current = { epoch: 'e', sequence: '9' };
    expect(compareWorkspaceSnapshots(current, { epoch: 'e', sequence: '10' })).toBe('newer');
    expect(compareWorkspaceSnapshots({ epoch: 'e', sequence: '10' }, current)).toBe('older');
    expect(compareWorkspaceSnapshots(current, { epoch: 'other', sequence: '99' })).toBe('different-epoch');
    expect(compareWorkspaceSnapshots(current, { epoch: 'e', sequence: '9' })).toBe('same');
  });
});
