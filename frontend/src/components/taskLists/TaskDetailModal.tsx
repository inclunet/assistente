import { useState, useEffect, useCallback, type ReactNode } from 'react';
import { CalendarOutlined, DeleteOutlined, EditOutlined, FileTextOutlined, LinkOutlined, MessageOutlined, RobotOutlined, SettingOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Modal } from '../ui/Modal';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { useTaskListStore } from '../../store/taskListStore';
import { useConfirm } from '../../hooks/useConfirm';
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
  const { loadTaskNotes, createTaskNote, updateTaskNote, deleteTaskNote, listCardCustomActions } = useTaskListStore();
  const { runCustomAction } = useCustomActions();

  const [notes, setNotes] = useState<TaskNote[]>([]);
  const [customActions, setCustomActions] = useState<CustomActionView[]>([]);
  const [isLoadingNotes, setIsLoadingNotes] = useState(false);
  const [showNoteForm, setShowNoteForm] = useState(false);
  const [editingNoteId, setEditingNoteId] = useState<string | null>(null);

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
    if (!task || !noteContent.trim()) return;
    const note = await createTaskNote(task.id, noteType, noteContent.trim(), noteAuthor.trim());
    if (note) {
      setNotes((prev) => [...prev, note]);
      resetForm();
    }
  }, [task, noteType, noteContent, noteAuthor, createTaskNote, resetForm]);

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

  const handleLinkClick = useCallback(() => {
    if (!task?.link) return;
    openTaskLink(task.link, { navigate });
  }, [task, navigate]);

  const status = task ? statuses.find((s) => s.id === task.statusId) : undefined;
  const isDueDatePast = task?.dueDate && new Date(task.dueDate) < new Date();

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={task?.title ?? ''}
      size="lg"
      className="task-detail-modal"
    >
      {!task ? null : (
      <>
      {/* Badges: status, code, link, due date */}
      <div className="task-detail__header">
        {status && (
          <span className="task-detail__badge task-detail__badge--status" style={{ borderColor: status.color }}>
            {status.icon} {status.label}
          </span>
        )}
        {task.code && (
          <span
            className={`task-detail__badge task-detail__badge--code${task.link ? ' task-detail__badge--link' : ''}`}
            onClick={task.link ? handleLinkClick : undefined}
            role={task.link ? 'link' : undefined}
            tabIndex={task.link ? 0 : undefined}
            onKeyDown={task.link ? (e) => { if (e.key === 'Enter') handleLinkClick(); } : undefined}
          >
            {task.code}
            {task.link && <> <LinkOutlined aria-hidden="true" /></>}
          </span>
        )}
        {!task.code && task.link && (
          <span
            className="task-detail__badge task-detail__badge--link"
            onClick={handleLinkClick}
            role="link"
            tabIndex={0}
            onKeyDown={(e) => { if (e.key === 'Enter') handleLinkClick(); }}
          >
            <LinkOutlined aria-hidden="true" /> Link
          </span>
        )}
        {task.assigneeName && (
          <span className="task-detail__badge task-detail__badge--assignee" title={task.assigneeId || undefined}>
            👤 {task.assigneeName}
          </span>
        )}
        {task.creatorName && (
          <span className="task-detail__badge task-detail__badge--creator" title={task.creatorId || undefined}>
            ✏️ {task.creatorName}
          </span>
        )}
        {task.dueDate && (
          <span className={`task-detail__badge task-detail__badge--due${isDueDatePast ? ' task-detail__badge--overdue' : ''}`}>
            <CalendarOutlined aria-hidden="true" /> {new Date(task.dueDate).toLocaleDateString()}
          </span>
        )}
      </div>

      {/* Custom actions (AEP-0067): when avaliado server-side */}
      {customActions.length > 0 && (
        <div className="task-detail__custom-actions">
          {customActions.map((ca) => (
            <button
              key={ca.id}
              type="button"
              className={`task-detail__custom-action${ca.danger ? ' task-detail__custom-action--danger' : ''}`}
              onClick={() => { void runCustomAction(ca, task.taskListId, task.id); }}
            >
              {ca.icon ? `${ca.icon} ${ca.label}` : ca.label}
            </button>
          ))}
        </div>
      )}

      {/* Description */}
      <div className="task-detail__section">
        <p className="task-detail__section-title">{t('tasklist.description')}</p>
        {task.description ? (
          <div className="task-detail__description">
            <MarkdownRenderer content={task.description} />
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
                    <MarkdownRenderer content={note.content} />
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
      <div className="task-detail__note-form-actions">
        <button onClick={onCancel}>{t('common.cancel', 'Cancelar')}</button>
        <button data-primary="" onClick={onSave} disabled={!noteContent.trim()}>
          {t('common.save', 'Salvar')}
        </button>
      </div>
    </div>
  );
}
