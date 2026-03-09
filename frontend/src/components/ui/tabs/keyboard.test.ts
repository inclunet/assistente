import { describe, expect, it } from 'vitest';
import { getTabsNavResult } from './keyboard';

describe('getTabsNavResult', () => {
  it('ignora tecla desconhecida', () => {
    expect(getTabsNavResult({ key: 'x', currentIndex: 0, count: 2 })).toEqual({
      handled: false,
      nextIndex: 0,
      bump: false,
    });
  });

  it('faz bump no limite esquerdo', () => {
    expect(getTabsNavResult({ key: 'ArrowLeft', currentIndex: 0, count: 3 })).toEqual({
      handled: true,
      nextIndex: 0,
      bump: true,
    });
  });

  it('vai para a próxima aba', () => {
    expect(getTabsNavResult({ key: 'ArrowRight', currentIndex: 0, count: 3 })).toEqual({
      handled: true,
      nextIndex: 1,
      bump: false,
    });
  });

  it('Home/End respeitam bump', () => {
    expect(getTabsNavResult({ key: 'Home', currentIndex: 0, count: 3 })).toEqual({
      handled: true,
      nextIndex: 0,
      bump: true,
    });

    expect(getTabsNavResult({ key: 'End', currentIndex: 2, count: 3 })).toEqual({
      handled: true,
      nextIndex: 2,
      bump: true,
    });
  });

  it('PageUp/PageDown pulam por padrão 10', () => {
    expect(getTabsNavResult({ key: 'PageDown', currentIndex: 0, count: 30 })).toEqual({
      handled: true,
      nextIndex: 10,
      bump: false,
    });

    expect(getTabsNavResult({ key: 'PageUp', currentIndex: 15, count: 30 })).toEqual({
      handled: true,
      nextIndex: 5,
      bump: false,
    });
  });

  it('clampa índices fora do range', () => {
    expect(getTabsNavResult({ key: 'ArrowRight', currentIndex: 999, count: 3 })).toEqual({
      handled: true,
      nextIndex: 2,
      bump: true,
    });

    expect(getTabsNavResult({ key: 'ArrowLeft', currentIndex: -10, count: 3 })).toEqual({
      handled: true,
      nextIndex: 0,
      bump: true,
    });
  });

  it('Delete retorna action close', () => {
    expect(getTabsNavResult({ key: 'Delete', currentIndex: 1, count: 3 })).toEqual({
      handled: true,
      nextIndex: 1,
      bump: false,
      action: 'close',
    });
  });
});
