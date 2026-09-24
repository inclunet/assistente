import { describe, expect, it } from 'vitest';
import { isTaskListConfigConflict } from './taskListConfigConflict';

describe('isTaskListConfigConflict', () => {
  it('reconhece o código em string (rejeição do Wails) e em Error (repassado)', () => {
    expect(isTaskListConfigConflict('TASKLIST_CONFIG_CONFLICT: alterado')).toBe(true);
    expect(isTaskListConfigConflict(new Error('TASKLIST_CONFIG_CONFLICT: alterado'))).toBe(true);
  });

  it('não confunde outros erros com conflito', () => {
    expect(isTaskListConfigConflict('status_id 2 está em uso')).toBe(false);
    expect(isTaskListConfigConflict(new Error('recusado'))).toBe(false);
    expect(isTaskListConfigConflict(undefined)).toBe(false);
    expect(isTaskListConfigConflict(null)).toBe(false);
  });

  it('exige o código no início: citado no meio de outro erro não é conflito', () => {
    expect(isTaskListConfigConflict('falha ao validar: TASKLIST_CONFIG_CONFLICT citado')).toBe(false);
    expect(isTaskListConfigConflict(new Error('outro erro (TASKLIST_CONFIG_CONFLICT)'))).toBe(false);
  });
});
