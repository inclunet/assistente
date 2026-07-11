import { useEffect, useMemo, useState } from 'react';
import { Modal } from './Modal';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import './QuestionnaireDialog.css';

export type QuestionnaireQuestionType =
  | 'text'
  | 'password'
  | 'long_text'
  | 'number'
  | 'boolean'
  | 'single_choice'
  | 'multiple_choice'
  | 'scale'
  | 'date'
  | 'readonly_code';

export interface QuestionnaireQuestion {
  id: string;
  type: QuestionnaireQuestionType;
  prompt: string;
  description?: string;
  content?: string;
  required?: boolean;
  options?: string[];
  min?: number;
  max?: number;
  step?: number;
  placeholder?: string;
  default?: string | number | boolean | string[];
}

export interface QuestionnaireRejectReason {
  id: string;
  label: string;
  placeholder?: string;
  maxLen?: number;
}

export interface QuestionnairePayload {
  id: string;
  title?: string;
  description?: string;
  questions: QuestionnaireQuestion[];
  allowCancel?: boolean;
  submitLabel?: string;
  cancelLabel?: string;
  rejectReason?: QuestionnaireRejectReason;
  createdAt?: string;
}

export interface QuestionnaireDialogProps {
  isOpen: boolean;
  data: QuestionnairePayload | null;
  onSubmit: (answers: Record<string, unknown>) => void;
  onCancel?: (answers?: Record<string, unknown>) => void;
}

function isEmptyValue(value: unknown, type: QuestionnaireQuestionType): boolean {
  if (type === 'multiple_choice') {
    return !Array.isArray(value) || value.length === 0;
  }
  if (type === 'boolean') {
    return value === undefined;
  }
  return value === undefined || value === null || value === '';
}

export function QuestionnaireDialog({ isOpen, data, onSubmit, onCancel }: QuestionnaireDialogProps) {
  const { announce } = useAnnouncer();
  const [answers, setAnswers] = useState<Record<string, unknown>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [rejectReasonText, setRejectReasonText] = useState('');

  const allowCancel = data?.allowCancel !== false;
  const title = data?.title || 'Questionário';
  const description = data?.description || '';
  const submitLabel = data?.submitLabel || 'Enviar';
  const cancelLabel = data?.cancelLabel || 'Cancelar';

  const questions = useMemo(() => data?.questions || [], [data]);

  // Diálogos com blocos readonly_code (confirmação de edição Antes/Depois,
  // diff de conflito, consentimento de rede) são de leitura pesada: o Modal
  // recebe readingMode (role="document"), fazendo o NVDA entrar em modo de
  // navegação e permitir leitura linha a linha com as setas. Questionários
  // só de formulário mantêm role="application" (modo de foco).
  const hasReadonlyCode = useMemo(
    () => questions.some((q) => q.type === 'readonly_code'),
    [questions]
  );

  useEffect(() => {
    if (!isOpen || !data) return;
    const initial: Record<string, unknown> = {};
    for (const q of data.questions) {
      if (q.default !== undefined) {
        initial[q.id] = q.default;
      }
    }
    setAnswers(initial);
    setErrors({});
    setRejectReasonText('');
  }, [isOpen, data]);

  useEffect(() => {
    if (!isOpen || !data) return;
    const message = [title, description].filter(Boolean).join('. ');
    if (message) {
      announce(message, 'assertive');
    }
  }, [isOpen, data, title, description, announce]);

  const handleSubmit = () => {
    if (!data) return;

    const nextErrors: Record<string, string> = {};
    for (const q of questions) {
      if (q.required && isEmptyValue(answers[q.id], q.type)) {
        nextErrors[q.id] = 'Resposta obrigatória';
      }
    }

    if (Object.keys(nextErrors).length > 0) {
      setErrors(nextErrors);
      const errorMessages = questions
        .filter((q) => nextErrors[q.id])
        .map((q) => `${q.prompt}: ${nextErrors[q.id]}`);
      if (errorMessages.length > 0) {
        announce(errorMessages.join('. '), 'assertive');
      }
      return;
    }

    setErrors({});
    onSubmit(answers);
  };

  const handleCancel = () => {
    if (!allowCancel) return;
    if (!onCancel) return;
    const reason = rejectReasonText.trim();
    if (data?.rejectReason && reason) {
      onCancel({ [data.rejectReason.id]: reason });
      return;
    }
    onCancel();
  };

  const updateAnswer = (id: string, value: unknown) => {
    setAnswers((prev) => ({ ...prev, [id]: value }));
    setErrors((prev) => {
      if (!prev[id]) return prev;
      const next = { ...prev };
      delete next[id];
      return next;
    });
  };

  if (!data) return null;

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleCancel}
      title={title}
      size="lg"
      returnFocusOnClose={false}
      allowClose={allowCancel}
      readingMode={hasReadonlyCode}
    >
      {description && <p className="questionnaire-dialog__description">{description}</p>}

      <form
        className="questionnaire-dialog__form"
        onSubmit={(e) => {
          e.preventDefault();
          handleSubmit();
        }}
        onKeyDown={(e) => {
          if (e.key !== 'Enter') return;
          if (e.shiftKey || e.altKey || e.ctrlKey || e.metaKey) return;
          if (e.target instanceof HTMLTextAreaElement) return;
          // Enter com foco no bloco readonly_code (<pre>) não deve submeter:
          // o usuário está apenas lendo/navegando pelo conteúdo.
          if (e.target instanceof HTMLPreElement) return;
          e.preventDefault();
          handleSubmit();
        }}
      >
        {questions.map((q, index) => {
          const labelId = `question-label-${q.id}`;
          const controlId = (q.type === 'text' || q.type === 'password' || q.type === 'long_text' || q.type === 'number' || q.type === 'scale' || q.type === 'date')
            ? `question-${q.id}`
            : undefined;
          const answer = answers[q.id];
          const answerText = typeof answer === 'string' || typeof answer === 'number' ? String(answer) : '';
          const answerNumber = typeof answer === 'number' ? answer : '';
          const answerArray = Array.isArray(answer) ? answer : [];
          const answerBoolean = typeof answer === 'boolean' ? answer : undefined;
          const scaleValue = typeof answer === 'number' ? answer : (q.min ?? 1);

          return (
          <div key={q.id} className="questionnaire-dialog__question">
            <div className="questionnaire-dialog__header">
              <label
                id={labelId}
                className="questionnaire-dialog__label"
                {...(controlId ? { htmlFor: controlId } : {})}
              >
                {index + 1}. {q.prompt}{q.required ? ' *' : ''}
              </label>
              {q.description && <div className="questionnaire-dialog__hint">{q.description}</div>}
            </div>

            {q.type === 'text' && (
              <input
                id={`question-${q.id}`}
                type="text"
                value={answerText}
                placeholder={q.placeholder}
                onChange={(e) => updateAnswer(q.id, e.target.value)}
                className="questionnaire-dialog__input"
                autoFocus={index === 0}
              />
            )}

            {q.type === 'password' && (
              <input
                id={`question-${q.id}`}
                type="password"
                value={answerText}
                placeholder={q.placeholder}
                onChange={(e) => updateAnswer(q.id, e.target.value)}
                className="questionnaire-dialog__input"
                autoFocus={index === 0}
              />
            )}

            {q.type === 'long_text' && (
              <textarea
                id={`question-${q.id}`}
                value={answerText}
                placeholder={q.placeholder}
                onChange={(e) => updateAnswer(q.id, e.target.value)}
                className="questionnaire-dialog__textarea"
                rows={4}
                autoFocus={index === 0}
              />
            )}

            {q.type === 'readonly_code' && (
              // Conteúdo estático: com o readingMode do Modal (role="document"),
              // o NVDA lê o bloco linha a linha em modo de navegação, sem
              // depender de caret. O tabIndex mantém uma parada de Tab para
              // orientação; role="region" dá nome acessível via aria-labelledby.
              <pre
                id={`question-${q.id}`}
                className="questionnaire-dialog__readonly"
                tabIndex={0}
                role="region"
                aria-labelledby={labelId}
              >
                {q.content ?? ''}
              </pre>
            )}

            {q.type === 'number' && (
              <input
                id={`question-${q.id}`}
                type="number"
                value={answerNumber}
                min={q.min}
                max={q.max}
                step={q.step}
                placeholder={q.placeholder}
                onChange={(e) => {
                  const value = e.target.value;
                  updateAnswer(q.id, value === '' ? '' : Number(value));
                }}
                className="questionnaire-dialog__input"
                autoFocus={index === 0}
              />
            )}

            {q.type === 'scale' && (
              <div className="questionnaire-dialog__scale">
                <input
                  id={`question-${q.id}`}
                  type="range"
                  min={q.min ?? 1}
                  max={q.max ?? 5}
                  step={q.step ?? 1}
                  value={scaleValue}
                  onChange={(e) => updateAnswer(q.id, Number(e.target.value))}
                />
                <span className="questionnaire-dialog__scale-value">{scaleValue}</span>
              </div>
            )}

            {q.type === 'date' && (
              <input
                id={`question-${q.id}`}
                type="date"
                value={answerText}
                onChange={(e) => updateAnswer(q.id, e.target.value)}
                className="questionnaire-dialog__input"
              />
            )}

            {q.type === 'boolean' && (
              <div className="questionnaire-dialog__options" role="radiogroup" aria-label={q.prompt}>
                {['Sim', 'Não'].map((label) => (
                  <label key={label} className="questionnaire-dialog__option">
                    <input
                      type="radio"
                      name={`question-${q.id}`}
                      checked={answerBoolean === (label === 'Sim')}
                      onChange={() => updateAnswer(q.id, label === 'Sim')}
                    />
                    <span>{label}</span>
                  </label>
                ))}
              </div>
            )}

            {(q.type === 'single_choice' || q.type === 'multiple_choice') && (
              <div className="questionnaire-dialog__options" role={q.type === 'single_choice' ? 'radiogroup' : 'group'} aria-label={q.prompt}>
                {(q.options || []).map((opt) => {
                  const selected = q.type === 'multiple_choice'
                    ? answerArray.includes(opt)
                    : answer === opt;

                  return (
                    <label key={opt} className="questionnaire-dialog__option">
                      <input
                        type={q.type === 'single_choice' ? 'radio' : 'checkbox'}
                        name={`question-${q.id}`}
                        checked={selected}
                        onChange={(e) => {
                          if (q.type === 'single_choice') {
                            updateAnswer(q.id, opt);
                            return;
                          }
                          const current = answerArray;
                          if (e.target.checked) {
                            updateAnswer(q.id, [...current, opt]);
                          } else {
                            updateAnswer(q.id, current.filter((v: string) => v !== opt));
                          }
                        }}
                      />
                      <span>{opt}</span>
                    </label>
                  );
                })}
              </div>
            )}

            {errors[q.id] && <div className="questionnaire-dialog__error">{errors[q.id]}</div>}
          </div>
          );
        })}

        {data.rejectReason && allowCancel ? (
          /* Ordem DOM (= ordem de Tab) intencional: Aplicar → motivo → Rejeitar,
             para que quem vai aprovar não precise atravessar o campo de justificativa. */
          <div className="questionnaire-dialog__footer questionnaire-dialog__footer--reject-reason">
            <button type="submit" className="questionnaire-dialog__button primary">
              {submitLabel}
            </button>
            <div className="questionnaire-dialog__reject-reason">
              <label className="questionnaire-dialog__label" htmlFor="questionnaire-reject-reason">
                {data.rejectReason.label}
              </label>
              <textarea
                id="questionnaire-reject-reason"
                className="questionnaire-dialog__textarea"
                rows={3}
                value={rejectReasonText}
                placeholder={data.rejectReason.placeholder}
                onChange={(e) => setRejectReasonText(e.target.value)}
                maxLength={data.rejectReason.maxLen && data.rejectReason.maxLen > 0 ? data.rejectReason.maxLen : undefined}
              />
            </div>
            <button type="button" className="questionnaire-dialog__button secondary" onClick={handleCancel}>
              {cancelLabel}
            </button>
          </div>
        ) : (
          <div className="questionnaire-dialog__footer">
            {allowCancel && (
              <button type="button" className="questionnaire-dialog__button secondary" onClick={handleCancel}>
                {cancelLabel}
              </button>
            )}
            <button type="submit" className="questionnaire-dialog__button primary">
              {submitLabel}
            </button>
          </div>
        )}
      </form>
    </Modal>
  );
}
