import { useState, useEffect, useCallback, type ReactNode } from 'react';
import { CalendarOutlined, CopyOutlined, DeleteOutlined, EditOutlined, FileTextOutlined, LinkOutlined, MessageOutlined, RobotOutlined, SettingOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Button } from '../ui/Button';
import { Modal } from '../ui/Modal';
import { DialogActions } from '../ui/DialogActions';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { HistoryPicker } from '../pickers/HistoryPicker';
import { useTaskListStore } from '../../store/taskListStore';
import { useUIStore } from '../../store/uiStore';
import { useConfirm } from '../../hooks/useConfirm';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { openTaskLink } from '../../lib/deepLinks';
import { TASK_NOTE_TYPES } from '../../types/tasklist';
import type { Task, TaskNote, TaskNoteType, TaskListWorkflowStatus, CustomActionView } from '../../types/tasklist';
import { useCustomActions } from './useCustomActions';
import './TaskDetailModal.css';

interface TaskDetailModalProps {
  isOpen: boolean;
  onClose: () => void;
  task: Task | null;
  statuses: TaskListWorkflowStatus[];
}

// Valor sentinela do item "Nenhuma" no HistoryPicker (não pode colidir com ID de conversa).
const CONVERSATION_NONE = '__none__';

// A prop `task` chega como snapshot do momento do clique (KanbanBoard e
// TasksTable guardam em useState); procura a versão viva no cache do store
// para que vínculos e edições feitos com o modal aberto reflitam na hora.
function findLiveTask(
  taskLists: Map<string, { tasks?: Task[] }> | undefined,
  taskId: string,
): Task | undefined {
  if (!taskLists) return undefined;
  for (const taskList of taskLists.values()) {
    const found = taskList.tasks?.find((t) => t.id === taskId);
    if (found) return found;
  }
  return undefined;
}

const NOTE_TYPE_ICONS: Record<TaskNoteType, ReactNode> = {
  1: <FileTextOutlined aria-hidden="true" />,
  2: <MessageOutlined aria-hidden="true" />,
  3: <RobotOutlined aria-hidden="true" />,
  4: <SettingOutlined aria-hidden="true" />,
};

const NOTE_TYPE_CSS: Record<TaskNoteType, string> = {
  1: 'internal',
  2: 'customer',
  3: 'agent',
  4: 'system',
};

function formatDate(dateStr: string): string {
  if (!dateStr) return '';
  try {
    const d = new Date(dateStr);
    return d.toLocaleDateString(undefined, { day: '2-digit', month: '2-digit', year: 'numeric' }) +
      ' ' + d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  } catch {
    return dateStr;
  }
}

export default function TaskDetailModal({ isOpen, onClose, task, statuses }: TaskDetailModalProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const requestConfirm = useConfirm();
  const { loadTaskNotes, createTaskNote, updateTaskNote, deleteTaskNote, listCardCustomActions, setTaskConversation, taskLists } = useTaskListStore();
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const { runCustomAction } = useCustomActions();

  const [notes, setNotes] = useState<TaskNote[]>([]);
  const [customActions, setCustomActions] = useState<CustomActionView[]>([]);
  const [isLoadingNotes, setIsLoadingNotes] = useState(false);
  const [showNoteForm, setShowNoteForm] = useState(false);
  const [editingNoteId, setEditingNoteId] = useState<string | null>(null);

  const [conversationSaving, setConversationSaving] = useState(false);

  // Versão viva da task: o update otimista de setTaskConversation (e outras
  // edições) atualiza o cache do store, mas a prop continua com o snapshot.
  const liveTask = task ? findLiveTask(taskLists, task.id) : undefined;
  const viewTask = liveTask ?? task;

  // Note form state
  const [noteType, setNoteType] = useState<TaskNoteType>(TASK_NOTE_TYPES.INTERNAL);
  const [noteContent, setNoteContent] = useState('');
  const [noteAuthor, setNoteAuthor] = useState('');

  useEffect(() => {
    if (isOpen && task) {
      // Stale guard único para os dois loads assíncronos (notes + custom actions):
      // se o modal fechar/trocar de task antes das Promises resolverem, não
      // sobrescrevemos estado com dados do card anterior.
      let cancelled = false;
      setIsLoadingNotes(true);
      loadTaskNotes(task.id)
        .then((loaded) => { if (!cancelled) { setNotes(loaded); setIsLoadingNotes(false); } })
        .catch(() => { if (!cancelled) { setNotes([]); setIsLoadingNotes(false); } });
      listCardCustomActions(task.id, 'card_detail')
        .then((res) => { if (!cancelled) setCustomActions(res); })
        .catch(() => { if (!cancelled) setCustomActions([]); });
      return () => { cancelled = true; };
    }
    setNotes([]);
    setCustomActions([]);
    // Reseta o loading também: se o modal fechou com loadTaskNotes ainda pendente,
    // o stale guard impede o setIsLoadingNotes(false) na Promise, e sem isto o
    // estado ficaria preso em "carregando" até o próximo open.
    setIsLoadingNotes(false);
    setShowNoteForm(false);
    setEditingNoteId(null);
    return undefined;
  }, [isOpen, task, loadTaskNotes, listCardCustomActions]);

  const resetForm = useCallback(() => {
    setNoteType(TASK_NOTE_TYPES.INTERNAL);
    setNoteContent('');
    setNoteAuthor('');
    setShowNoteForm(false);
    setEditingNoteId(null);
  }, []);

  const handleAddNote = useCallback(async () => {
    if (!viewTask || !noteContent.trim()) return;
    const note = await createTaskNote(viewTask.id, noteType, noteContent.trim(), noteAuthor.trim());
    if (note) {
      setNotes((prev) => [...prev, note]);
      resetForm();
    }
  }, [viewTask, noteType, noteContent, noteAuthor, createTaskNote, resetForm]);

  const handleEditNote = useCallback((note: TaskNote) => {
    setEditingNoteId(note.id);
    setNoteContent(note.content);
    setShowNoteForm(false);
  }, []);

  const handleSaveEditNote = useCallback(async () => {
    if (editingNoteId === null || !noteContent.trim()) return;
    await updateTaskNote(editingNoteId, noteContent.trim());
    setNotes((prev) => prev.map((n) =>
      n.id === editingNoteId ? { ...n, content: noteContent.trim(), updatedAt: new Date().toISOString() } : n
    ));
    setEditingNoteId(null);
    setNoteContent('');
  }, [editingNoteId, noteContent, updateTaskNote]);

  const handleDeleteNote = useCallback(async (noteId: string) => {
    const confirmed = await requestConfirm({
      title: t('tasklist.deleteNote'),
      message: t('tasklist.deleteNoteConfirm'),
      variant: 'danger',
    });
    if (!confirmed) return;
    await deleteTaskNote(noteId);
    setNotes((prev) => prev.filter((n) => n.id !== noteId));
  }, [deleteTaskNote, requestConfirm, t]);

  const handleOpenNewNote = useCallback(() => {
    setEditingNoteId(null);
    setNoteContent('');
    setNoteAuthor('');
    setNoteType(TASK_NOTE_TYPES.INTERNAL);
    setShowNoteForm(true);
  }, []);

  const handleCancelForm = useCallback(() => {
    resetForm();
  }, [resetForm]);

  const handleCopyCode = useCallback(async () => {
    if (!viewTask?.code) return;
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard-unavailable');
      await navigator.clipboard.writeText(viewTask.code);
      const message = t('tasklist.codeCopied', 'Código copiado');
      addToast(message, 'success', undefined, undefined, { suppressAnnounce: true });
      announce(message);
    } catch {
      const message = t('tasklist.codeCopyFailed', 'Não foi possível copiar o código. Tente novamente.');
      addToast(message, 'error', undefined, undefined, { suppressAnnounce: true });
      announce(message);
    }
  }, [viewTask, t, addToast, announce]);

  const handleLinkClick = useCallback(() => {
    if (!viewTask?.link) return;
    openTaskLink(viewTask.link, { navigate });
  }, [viewTask, navigate]);

  const handleConversationClick = useCallback(() => {
    if (!viewTask?.conversationId) return;
    openTaskLink(`assistente://conversation/${viewTask.conversationId}`, { navigate });
    // A navegação troca o contexto por trás do modal; fecha para não dar a
    // impressão de que nada aconteceu (o Modal restaura o foco ao fechar).
    onClose();
  }, [viewTask, navigate, onClose]);

  // Aplica o vínculo imediatamente ao selecionar no HistoryPicker (id) ou ao
  // escolher "Nenhuma"/desvincular (null), espelhando a UX do picker do chat.
  const applyConversation = useCallback(async (conversationId: string | null) => {
    if (!viewTask) return;
    setConversationSaving(true);
    try {
      await setTaskConversation(viewTask.id, conversationId);
      const msg = t('tasklist.conversationLinkSaved', 'Vínculo de conversa atualizado');
      addToast(msg, 'success', undefined, undefined, { suppressAnnounce: true });
      announce(msg);
    } catch (error) {
      // setTaskConversation já registra o erro e recarrega a lista; dá feedback explícito.
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg || t('common.error', 'Erro ao salvar'), 'error');
    } finally {
      setConversationSaving(false);
    }
  }, [viewTask, setTaskConversation, addToast, announce, t]);

  const status = viewTask ? statuses.find((s) => s.id === viewTask.statusId) : undefined;
  const isDueDatePast = viewTask?.dueDate && new Date(viewTask.dueDate) < new Date();

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={viewTask?.title ?? ''}
      size="lg"
      className="task-detail-modal"
      readingMode
    >
      {!viewTask ? null : (
      <>
      {/* Badges: status, code, link, due date */}
      <div className="task-detail__header">
        {status && (
          <span className="task-detail__badge task-detail__badge--status" style={{ borderColor: status.color }}>
            {status.icon} {status.label}
          </span>
        )}
        {viewTask.code && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="task-detail__copy-code"
            onClick={() => void handleCopyCode()}
            aria-label={t('tasklist.copyCode', 'Copiar código {{code}}', { code: viewTask.code })}
          >
            <CopyOutlined aria-hidden="true" />
            {viewTask.code}
          </Button>
        )}
        {viewTask.link && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="task-detail__open-link"
            onClick={handleLinkClick}
          >
            <LinkOutlined aria-hidden="true" /> {t('tasklist.openCardLink', 'Abrir link do card')}
          </Button>
        )}
        {viewTask.assigneeName && (
          <span className="task-detail__badge task-detail__badge--assignee" title={viewTask.assigneeId || undefined}>
            👤 {viewTask.assigneeName}
          </span>
        )}
        {viewTask.creatorName && (
          <span className="task-detail__badge task-detail__badge--creator" title={viewTask.creatorId || undefined}>
            ✏️ {viewTask.creatorName}
          </span>
        )}
        {viewTask.dueDate && (
          <span className={`task-detail__badge task-detail__badge--due${isDueDatePast ? ' task-detail__badge--overdue' : ''}`}>
            <CalendarOutlined aria-hidden="true" /> {new Date(viewTask.dueDate).toLocaleDateString()}
          </span>
        )}
        {viewTask.conversationId && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="task-detail__conversation-link"
            onClick={handleConversationClick}
            title={viewTask.conversationId}
          >
            <MessageOutlined aria-hidden="true" /> {t('tasklist.goToConversation', 'Ir para conversa vinculada')}
          </Button>
        )}
      </div>

      {/* Conversation link editor */}
      <div className="task-detail__conversation">
        <HistoryPicker
          value={viewTask.conversationId}
          onChange={(id) => void applyConversation(id)}
          onSelectExtra={() => void applyConversation(null)}
          extraItems={viewTask.conversationId
            ? [{ value: CONVERSATION_NONE, label: t('tasklist.conversationNone', 'Nenhuma') }]
            : undefined}
          label={viewTask.conversationId
            ? t('tasklist.changeConversation', 'Alterar conversa vinculada')
            : t('tasklist.linkConversation', 'Vincular conversa')}
          description={t('tasklist.conversationDescription', 'Vincula esta tarefa a uma conversa')}
          disabled={conversationSaving}
          maxWidth="100%"
          onAnnounce={announce}
        />
      </div>

      {/* Custom actions (AEP-0067): when avaliado server-side */}
      {customActions.length > 0 && (
        <div className="task-detail__custom-actions">
          {customActions.map((ca) => (
            <button
              key={ca.id}
              type="button"
              className={`task-detail__custom-action${ca.danger ? ' task-detail__custom-action--danger' : ''}`}
              onClick={() => { void runCustomAction(ca, viewTask.taskListId, viewTask.id); }}
              aria-label={ca.label}
            >
              {ca.icon ? <><span aria-hidden="true">{ca.icon}</span> {ca.label}</> : ca.label}
            </button>
          ))}
        </div>
      )}

      {/* Description */}
      <div className="task-detail__section">
        <p className="task-detail__section-title">{t('tasklist.description')}</p>
        {viewTask.description ? (
          <div className="task-detail__description">
            <MarkdownRenderer content={viewTask.description} tabNavigation="enabled" />
          </div>
        ) : (
          <p className="task-detail__description task-detail__description--empty">
            {t('tasklist.descriptionPlaceholder')}
          </p>
        )}
      </div>

      {/* Notes */}
      <div className="task-detail__notes">
        <div className="task-detail__notes-header">
          <h3>{t('tasklist.notesTitle')} ({notes.length})</h3>
          <button className="task-detail__add-note-btn" onClick={handleOpenNewNote}>
            + {t('tasklist.addNote')}
          </button>
        </div>

        <div className="task-detail__notes-list">
          {/* New note form */}
          {showNoteForm && (
            <NoteForm
              noteType={noteType}
              noteContent={noteContent}
              noteAuthor={noteAuthor}
              onTypeChange={setNoteType}
              onContentChange={setNoteContent}
              onAuthorChange={setNoteAuthor}
              onSave={handleAddNote}
              onCancel={handleCancelForm}
              showAuthor
            />
          )}

          {isLoadingNotes && <p className="task-detail__notes-empty">{t('tasklist.loading')}</p>}

          {!isLoadingNotes && notes.length === 0 && !showNoteForm && (
            <p className="task-detail__notes-empty">{t('tasklist.noNotes')}</p>
          )}

          {notes.map((note) => (
            <div key={note.id} className="task-detail__note">
              {editingNoteId === note.id ? (
                <NoteForm
                  noteContent={noteContent}
                  onContentChange={setNoteContent}
                  onSave={handleSaveEditNote}
                  onCancel={() => { setEditingNoteId(null); setNoteContent(''); }}
                />
              ) : (
                <>
                  <div className="task-detail__note-header">
                    <span className={`task-detail__note-type task-detail__note-type--${NOTE_TYPE_CSS[note.type] || 'internal'}`}>
                      {NOTE_TYPE_ICONS[note.type] || <FileTextOutlined aria-hidden="true" />} {t(`tasklist.noteTypes.${NOTE_TYPE_CSS[note.type] || 'internal'}`)}
                    </span>
                    {note.authorName && (
                      <span className="task-detail__note-author" title={note.authorId || undefined}>
                        {note.authorName}
                      </span>
                    )}
                    <span className="task-detail__note-date">{formatDate(note.createdAt)}</span>
                    <div className="task-detail__note-actions">
                      <button
                        className="task-detail__note-action"
                        onClick={() => handleEditNote(note)}
                        aria-label={t('tasklist.editNote')}
                        title={t('tasklist.editNote')}
                      >
                        <EditOutlined aria-hidden="true" />
                      </button>
                      <button
                        className="task-detail__note-action task-detail__note-action--danger"
                        onClick={() => handleDeleteNote(note.id)}
                        aria-label={t('tasklist.deleteNote')}
                        title={t('tasklist.deleteNote')}
                      >
                        <DeleteOutlined aria-hidden="true" />
                      </button>
                    </div>
                  </div>
                  <div className="task-detail__note-content">
                    <MarkdownRenderer content={note.content} tabNavigation="enabled" />
                  </div>
                </>
              )}
            </div>
          ))}
        </div>
      </div>
      </>
      )}
    </Modal>
  );
}

interface NoteFormProps {
  noteType?: TaskNoteType;
  noteContent: string;
  noteAuthor?: string;
  onTypeChange?: (type: TaskNoteType) => void;
  onContentChange: (content: string) => void;
  onAuthorChange?: (author: string) => void;
  onSave: () => void;
  onCancel: () => void;
  showAuthor?: boolean;
}

function NoteForm({
  noteType,
  noteContent,
  noteAuthor,
  onTypeChange,
  onContentChange,
  onAuthorChange,
  onSave,
  onCancel,
  showAuthor,
}: NoteFormProps) {
  const { t } = useTranslation();

  return (
    <div className="task-detail__note-form">
      {onTypeChange && (
        <div className="task-detail__note-form-row">
          <select
            value={noteType}
            onChange={(e) => onTypeChange(Number(e.target.value) as TaskNoteType)}
            aria-label={t('tasklist.noteType')}
          >
            <option value={TASK_NOTE_TYPES.INTERNAL}>{t('tasklist.noteTypes.internal')}</option>
            <option value={TASK_NOTE_TYPES.CUSTOMER}>{t('tasklist.noteTypes.customer')}</option>
            <option value={TASK_NOTE_TYPES.AGENT}>{t('tasklist.noteTypes.agent')}</option>
            <option value={TASK_NOTE_TYPES.SYSTEM}>{t('tasklist.noteTypes.system')}</option>
          </select>
          {showAuthor && onAuthorChange && (
            <input
              type="text"
              value={noteAuthor}
              onChange={(e) => onAuthorChange(e.target.value)}
              placeholder={t('tasklist.noteAuthorPlaceholder')}
              aria-label={t('tasklist.noteAuthor')}
              maxLength={128}
            />
          )}
        </div>
      )}
      <textarea
        value={noteContent}
        onChange={(e) => onContentChange(e.target.value)}
        placeholder={t('tasklist.noteContentPlaceholder')}
        aria-label={t('tasklist.noteContent')}
        autoFocus
      />
      <DialogActions
        className="task-detail__note-form-actions"
        primary={
          <button type="button" data-primary="" onClick={onSave} disabled={!noteContent.trim()}>
            {t('common.save', 'Salvar')}
          </button>
        }
        secondary={
          <button type="button" onClick={onCancel}>{t('common.cancel', 'Cancelar')}</button>
        }
      />
    </div>
  );
}
