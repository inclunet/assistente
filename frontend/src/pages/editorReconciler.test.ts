import { describe, expect, it } from 'vitest';

import { hashStringFNV1a32 } from '../lib/editorMergeUtils';
import { createEmptyTabDiskState, decideExternalChange, type ReconcileInput } from './editorReconciler';

const DISK = hashStringFNV1a32('conteudo do disco');
const LOCAL = hashStringFNV1a32('conteudo local');
const BASELINE = hashStringFNV1a32('baseline conhecido');

/** Input base: evento externo comum, aba destravada e sem merge session. */
function makeInput(patch: Partial<ReconcileInput> = {}): ReconcileInput {
  return {
    trigger: 'file_changed',
    conflictLocked: false,
    promptInFlight: false,
    hasMergeSession: false,
    tabIsDirty: false,
    ...patch,
  };
}

describe('createEmptyTabDiskState', () => {
  it('cria estado sem info nem baseline', () => {
    expect(createEmptyTabDiskState()).toEqual({ info: null, baselineHash: null, baselineContent: null });
  });
});

describe('decideExternalChange', () => {
  describe('selfWrite (origin editor_ui)', () => {
    it('apenas atualiza metadados de disco, sem reload nem prompt', () => {
      expect(decideExternalChange(makeInput({ selfWrite: true }))).toEqual({
        action: 'update_baseline',
        scope: 'info_only',
      });
    });

    it('vale mesmo com a aba travada por conflito (comportamento histórico)', () => {
      expect(decideExternalChange(makeInput({ selfWrite: true, conflictLocked: true }))).toEqual({
        action: 'update_baseline',
        scope: 'info_only',
      });
    });

    it('vale mesmo com a aba suja', () => {
      expect(decideExternalChange(makeInput({ selfWrite: true, tabIsDirty: true }))).toEqual({
        action: 'update_baseline',
        scope: 'info_only',
      });
    });
  });

  describe('guards de estado da aba', () => {
    it('ignora quando há merge session ativa', () => {
      expect(decideExternalChange(makeInput({ hasMergeSession: true, diskHash: DISK, localHash: LOCAL }))).toEqual({
        action: 'ignore',
        reason: 'merge_session',
      });
    });

    it('ignora quando a aba está travada por conflito pendente', () => {
      expect(decideExternalChange(makeInput({ conflictLocked: true, diskHash: DISK, localHash: LOCAL }))).toEqual({
        action: 'ignore',
        reason: 'locked',
      });
    });
  });

  describe('fallback isProbablySelfWrite (evento sem origin)', () => {
    it('ignora evento sem origin dentro da janela de self-write', () => {
      expect(decideExternalChange(makeInput({ probablySelfWrite: true }))).toEqual({
        action: 'ignore',
        reason: 'self_write_window',
      });
    });

    it('não descarta evento assistido dentro da janela de self-write', () => {
      const decision = decideExternalChange(makeInput({ assisted: true, probablySelfWrite: true }));
      expect(decision).toEqual({ action: 'defer_read' });
    });

    it('não se aplica ao pré-autosave', () => {
      // Se o fallback valesse no pré-autosave, a decisão seria 'ignore';
      // em vez disso, metadados divergentes pedem leitura de conteúdo.
      const decision = decideExternalChange(
        makeInput({ trigger: 'pre_save', probablySelfWrite: true, diskInfoChanged: true })
      );
      expect(decision).toEqual({ action: 'defer_read' });
    });
  });

  describe('erro de leitura do disco', () => {
    it('pede decisão explícita quando não dá pra ler o disco', () => {
      expect(decideExternalChange(makeInput({ diskReadError: true }))).toEqual({
        action: 'prompt_conflict',
        openPrompt: true,
        cause: 'external',
      });
    });

    it('não reabre prompt se já há um em voo', () => {
      expect(decideExternalChange(makeInput({ diskReadError: true, promptInFlight: true }))).toEqual({
        action: 'prompt_conflict',
        openPrompt: false,
        cause: 'external',
      });
    });
  });

  describe('pré-autosave', () => {
    it('segue com o save quando os metadados não mudaram (sem IO extra)', () => {
      expect(decideExternalChange(makeInput({ trigger: 'pre_save', diskInfoChanged: false }))).toEqual({
        action: 'ignore',
        reason: 'no_change',
      });
    });

    it('pede leitura de conteúdo quando os metadados divergem (não acusa conflito só por mtime/size)', () => {
      expect(decideExternalChange(makeInput({ trigger: 'pre_save', diskInfoChanged: true, tabIsDirty: true }))).toEqual(
        { action: 'defer_read' }
      );
    });

    it('estado de metadados desconhecido (campo ausente) também pede leitura, não vira no_change', () => {
      expect(decideExternalChange(makeInput({ trigger: 'pre_save', tabIsDirty: true }))).toEqual({
        action: 'defer_read',
      });
    });

    it('só atualiza metadados quando o conteúdo do disco continua sendo o baseline (mtime tocado, ex.: OneDrive)', () => {
      expect(
        decideExternalChange(
          makeInput({
            trigger: 'pre_save',
            diskInfoChanged: true,
            tabIsDirty: true,
            diskHash: BASELINE,
            localHash: LOCAL,
            lastKnownDiskHash: BASELINE,
          })
        )
      ).toEqual({ action: 'update_baseline', scope: 'info_only' });
    });

    it('adota o local como baseline quando o disco já tem o conteúdo local', () => {
      expect(
        decideExternalChange(
          makeInput({
            trigger: 'pre_save',
            diskInfoChanged: true,
            tabIsDirty: true,
            diskHash: LOCAL,
            localHash: LOCAL,
            lastKnownDiskHash: BASELINE,
          })
        )
      ).toEqual({ action: 'update_baseline', scope: 'adopt_local' });
    });

    it('trava e pergunta quando o conteúdo do disco realmente divergiu', () => {
      expect(
        decideExternalChange(
          makeInput({
            trigger: 'pre_save',
            diskInfoChanged: true,
            tabIsDirty: true,
            diskHash: DISK,
            localHash: LOCAL,
            lastKnownDiskHash: BASELINE,
          })
        )
      ).toEqual({ action: 'prompt_conflict', openPrompt: true, cause: 'external' });
    });

    it('não reabre prompt no pré-autosave se já há um em voo', () => {
      expect(
        decideExternalChange(
          makeInput({
            trigger: 'pre_save',
            diskInfoChanged: true,
            diskHash: DISK,
            localHash: LOCAL,
            lastKnownDiskHash: BASELINE,
            promptInFlight: true,
          })
        )
      ).toEqual({ action: 'prompt_conflict', openPrompt: false, cause: 'external' });
    });
  });

  describe('sem conteúdo do disco em mãos', () => {
    it('pede leitura do disco no evento fileChanged', () => {
      expect(decideExternalChange(makeInput())).toEqual({ action: 'defer_read' });
    });

    it('pede leitura do disco na re-checagem de foco', () => {
      expect(decideExternalChange(makeInput({ trigger: 'focus_recheck' }))).toEqual({ action: 'defer_read' });
    });
  });

  describe('convergência de conteúdo', () => {
    it('adota o local como baseline quando disco e editor já têm o mesmo conteúdo', () => {
      expect(decideExternalChange(makeInput({ diskHash: LOCAL, localHash: LOCAL, tabIsDirty: true }))).toEqual({
        action: 'update_baseline',
        scope: 'adopt_local',
      });
    });

    it('só atualiza metadados quando o conteúdo do disco continua sendo o baseline conhecido', () => {
      expect(
        decideExternalChange(
          makeInput({ diskHash: BASELINE, localHash: LOCAL, lastKnownDiskHash: BASELINE, tabIsDirty: true })
        )
      ).toEqual({ action: 'update_baseline', scope: 'info_only' });
    });
  });

  describe('mudança externa real', () => {
    it('auto-recarrega aba limpa quando permitido', () => {
      expect(
        decideExternalChange(
          makeInput({
            diskHash: DISK,
            localHash: LOCAL,
            lastKnownDiskHash: LOCAL,
            tabIsDirty: false,
            allowAutoReload: true,
          })
        )
      ).toEqual({ action: 'auto_reload' });
    });

    it('auto-recarrega aba declarada limpa antes de existir baseline', () => {
      expect(
        decideExternalChange(
          makeInput({
            diskHash: DISK,
            localHash: LOCAL,
            lastKnownDiskHash: null,
            tabIsDirty: false,
            allowAutoReload: true,
          })
        )
      ).toEqual({ action: 'auto_reload' });
    });

    it('preferência prompt força pergunta mesmo em aba externa limpa', () => {
      expect(
        decideExternalChange(
          makeInput({
            diskHash: DISK,
            localHash: LOCAL,
            lastKnownDiskHash: LOCAL,
            tabIsDirty: false,
            allowAutoReload: false,
          })
        )
      ).toEqual({ action: 'prompt_conflict', openPrompt: true, cause: 'external' });
    });

    it('não auto-recarrega na re-checagem de foco (allowAutoReload ausente)', () => {
      expect(
        decideExternalChange(makeInput({ trigger: 'focus_recheck', diskHash: DISK, localHash: LOCAL }))
      ).toEqual({ action: 'prompt_conflict', openPrompt: true, cause: 'external' });
    });

    it('trava e pergunta quando a aba está suja', () => {
      expect(
        decideExternalChange(makeInput({ diskHash: DISK, localHash: LOCAL, tabIsDirty: true, allowAutoReload: true }))
      ).toEqual({ action: 'prompt_conflict', openPrompt: true, cause: 'external' });
    });

    it('não reabre prompt para aba suja se já há um em voo', () => {
      expect(
        decideExternalChange(
          makeInput({ diskHash: DISK, localHash: LOCAL, tabIsDirty: true, allowAutoReload: true, promptInFlight: true })
        )
      ).toEqual({ action: 'prompt_conflict', openPrompt: false, cause: 'external' });
    });
  });

  describe('escrita assistida (assistant_tool)', () => {
    it('auto-recarrega aba suja quando o local ainda é o baseline conhecido do disco', () => {
      expect(
        decideExternalChange(
          makeInput({
            assisted: true,
            diskHash: DISK,
            localHash: LOCAL,
            lastKnownDiskHash: LOCAL,
            tabIsDirty: true,
            allowAutoReload: true,
          })
        )
      ).toEqual({ action: 'auto_reload' });
    });

    it('mantém prompt quando há edição local divergente do baseline, com causa assistida', () => {
      expect(
        decideExternalChange(
          makeInput({
            assisted: true,
            diskHash: DISK,
            localHash: LOCAL,
            lastKnownDiskHash: BASELINE,
            tabIsDirty: true,
            allowAutoReload: true,
          })
        )
      ).toEqual({ action: 'prompt_conflict', openPrompt: true, cause: 'assisted' });
    });

    it('sem baseline conhecido, aba suja assistida também vai para prompt com causa assistida', () => {
      expect(
        decideExternalChange(
          makeInput({
            assisted: true,
            diskHash: DISK,
            localHash: LOCAL,
            lastKnownDiskHash: null,
            tabIsDirty: true,
            allowAutoReload: true,
          })
        )
      ).toEqual({ action: 'prompt_conflict', openPrompt: true, cause: 'assisted' });
    });

    it('erro de leitura do disco em evento assistido também carrega a causa assistida', () => {
      expect(decideExternalChange(makeInput({ assisted: true, diskReadError: true }))).toEqual({
        action: 'prompt_conflict',
        openPrompt: true,
        cause: 'assisted',
      });
    });

    it('aba suja assistida com local igual ao baseline pré-escrita adota o disco (auto_reload)', () => {
      // O usuário aprovou o diff da tool e não digitou nada depois: o conteúdo
      // local ainda é o baseline conhecido do disco → adota a escrita aprovada.
      expect(
        decideExternalChange(
          makeInput({
            assisted: true,
            diskHash: DISK,
            localHash: BASELINE,
            lastKnownDiskHash: BASELINE,
            tabIsDirty: true,
            allowAutoReload: true,
          })
        )
      ).toEqual({ action: 'auto_reload' });
    });
  });
});
