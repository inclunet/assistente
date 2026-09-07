import React, { useCallback, useEffect, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FolderOpenOutlined } from '@ant-design/icons';
import { ToolbarButton } from '../ui/Toolbar';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { DialogActions } from '../ui/DialogActions';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import {
  dirName,
  pendingRecreate,
  useAgentConversationWorkDir,
} from './useAgentConversationWorkDir';
import './AgentWorkDirControl.css';

export interface AgentWorkDirControlProps {
  conversationId?: string | null;
  disabled?: boolean;
}

/**
 * AgentWorkDirControl mostra em que diretório o agente desta conversa trabalha e
 * deixa trocá-lo (AEP-0084 D5).
 *
 * O diretório fica visível, e não escondido em configuração, porque ele é o
 * alcance do que a pessoa autorizou o agente a ler e editar: um agente de código
 * agindo na árvore errada é a diferença entre uma sugestão e um estrago.
 *
 * Só aparece quando há agente de código do outro lado. Numa conversa de provedor
 * HTTP não existe diretório nenhum, e o botão seria mais um controle para o Tab
 * atravessar sem nada a fazer.
 */
export const AgentWorkDirControl: React.FC<AgentWorkDirControlProps> = ({
  conversationId,
  disabled = false,
}) => {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const { state, saving, save } = useAgentConversationWorkDir(conversationId);
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState('');
  const [error, setError] = useState('');
  const inputId = useId();
  const warningId = useId();

  // Abrir o diálogo parte de onde o agente está agora: trocar de diretório
  // costuma ser corrigir um caminho, e começar com o campo vazio obrigaria a
  // redigitar tudo.
  useEffect(() => {
    if (!open) return;
    setTyped(state?.dir ?? '');
    setError('');
  }, [open, state?.dir]);

  const close = useCallback(() => setOpen(false), []);

  const apply = useCallback(async (dir: string) => {
    try {
      const next = await save(dir);
      setOpen(false);
      // O que é falado é o caminho que valeu, e não o que foi digitado: um "."
      // vira o caminho inteiro no backend, e é esse o alcance do agente.
      announce(next.pinned
        ? t('chat.agentWorkDir.announceChanged', { dir: next.dir })
        : t('chat.agentWorkDir.announceWorkspace', { dir: next.dir }));
    } catch (err: unknown) {
      // O erro do backend é o que explica o que houve — "o diretório X não
      // existe" —, e ele fica na tela, junto do campo, além de anunciado.
      const message = err instanceof Error ? err.message : String(err);
      setError(message);
      announce(t('chat.agentWorkDir.announceError', { error: message }));
    }
  }, [announce, save, t]);

  if (!state?.available) return null;

  const pending = pendingRecreate(state);
  const short = dirName(state.dir) || state.dir;

  return (
    <>
      <ToolbarButton
        icon={<FolderOpenOutlined />}
        label={short}
        title={state.dir}
        aria-label={state.pinned
          ? t('chat.agentWorkDir.buttonPinned', { dir: state.dir })
          : t('chat.agentWorkDir.buttonWorkspace', { dir: state.dir })}
        onClick={() => setOpen(true)}
        disabled={disabled}
        className={`agent-workdir__button${pending ? ' agent-workdir__button--pending' : ''}`}
      />

      <Modal
        isOpen={open}
        onClose={close}
        title={t('chat.agentWorkDir.title')}
        size="md"
        ariaDescribedBy={warningId}
      >
        <div className="agent-workdir__body">
          <p id={warningId} className="agent-workdir__warning">
            {t('chat.agentWorkDir.warning')}
          </p>

          <label className="agent-workdir__label" htmlFor={inputId}>
            {t('chat.agentWorkDir.fieldLabel')}
          </label>
          <input
            id={inputId}
            className="agent-workdir__input"
            type="text"
            value={typed}
            spellCheck={false}
            autoComplete="off"
            onChange={(event) => {
              setTyped(event.target.value);
              // O erro é da tentativa anterior: mantê-lo enquanto a pessoa
              // corrige o caminho acusaria um problema que ela já está
              // resolvendo.
              if (error) setError('');
            }}
            aria-describedby={error ? `${inputId}-erro` : undefined}
            aria-invalid={error ? true : undefined}
          />
          {/* O erro é descrição do campo, e não região viva: quem anuncia é o
              anunciador global (AEP-0058), e uma live region por diálogo faria
              a mesma frase ser dita duas vezes. */}
          {error && (
            <p id={`${inputId}-erro`} className="agent-workdir__error">
              {error}
            </p>
          )}

          <p className="agent-workdir__hint">
            {t('chat.agentWorkDir.workspaceHint', { dir: state.workspaceDir })}
          </p>

          <div className="agent-workdir__actions">
            <Button
              variant="secondary"
              onClick={() => void apply('')}
              disabled={saving || !state.pinned}
            >
              {t('chat.agentWorkDir.useWorkspace')}
            </Button>
            <DialogActions
              primary={
                <Button
                  variant="primary"
                  onClick={() => void apply(typed)}
                  disabled={saving}
                >
                  {t('chat.agentWorkDir.confirm')}
                </Button>
              }
              secondary={
                <Button variant="ghost" onClick={close} disabled={saving}>
                  {t('common.cancel')}
                </Button>
              }
            />
          </div>
        </div>
      </Modal>
    </>
  );
};
