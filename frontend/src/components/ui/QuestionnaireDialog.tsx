import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from './Modal';
import { DialogActions } from './DialogActions';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import {
  questionnaireOptionValue,
  resolveQuestionnaireText,
  type QuestionnaireText,
} from '../../lib/questionnaireText';
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

/**
 * Os textos visíveis são QuestionnaireText: chave de tradução com o texto
 * pronto do backend, ou só o texto quando não há o que traduzir (AEP-0085).
 * `content` é conteúdo cru (diff, comando, caminho) e `default` aponta para o
 * valor estável da opção, nunca para o rótulo traduzido.
 */
export interface QuestionnaireQuestion {
  id: string;
  type: QuestionnaireQuestionType;
  prompt: QuestionnaireText;
  description?: QuestionnaireText;
  content?: string;
  required?: boolean;
  options?: QuestionnaireText[];
  min?: number;
  max?: number;
  step?: number;
  placeholder?: QuestionnaireText;
  default?: string | number | boolean | string[];
  /** Recebe o foco inicial quando o diálogo abre (apenas o primeiro marcado). */
  autoFocus?: boolean;
}

export interface QuestionnaireRejectReason {
  id: string;
  label: QuestionnaireText;
  placeholder?: QuestionnaireText;
  maxLen?: number;
}

export interface QuestionnairePayload {
  id: string;
  /** AEP-0091: "decision" renderiza DecisionDialog em vez do formulário. */
  kind?: string;
  title?: QuestionnaireText;
  description?: QuestionnaireText;
  /** Conteúdo só leitura (comando, URL, ação ACP). */
  body?: string;
  /** Texto traduzível secundário (ex.: hint de skill host). */
  hint?: QuestionnaireText;
  actions?: QuestionnaireDecisionAction[];
  questions: QuestionnaireQuestion[];
  allowCancel?: boolean;
  submitLabel?: QuestionnaireText;
  cancelLabel?: QuestionnaireText;
  /**
   * Motivo opcional ao rejeitar — só o DecisionDialog renderiza (confirmação
   * de edição). O QuestionnaireDialog de formulário ignora este campo.
   */
  rejectReason?: QuestionnaireRejectReason;
  createdAt?: string;
}

export interface QuestionnaireDecisionAction {
  id: string;
  label: QuestionnaireText;
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost' | 'outline';
  shortcut?: QuestionnaireText;
  primary?: boolean;
}

export function isDecisionQuestionnaire(
  data: QuestionnairePayload | null | undefined,
): data is QuestionnairePayload & {
  kind: 'decision';
  actions: [QuestionnaireDecisionAction, ...QuestionnaireDecisionAction[]];
} {
  return (
    !!data &&
    data.kind === 'decision' &&
    Array.isArray(data.actions) &&
    data.actions.length > 0
  );
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
  const { t } = useTranslation();
  const [answers, setAnswers] = useState<Record<string, unknown>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});

  const allowCancel = data?.allowCancel !== false;
  const title = resolveQuestionnaireText(t, data?.title, t('ui.questionnaire.defaultTitle', 'Questionário'));
  const description = resolveQuestionnaireText(t, data?.description);
  const submitLabel = resolveQuestionnaireText(t, data?.submitLabel, t('ui.questionnaire.submit', 'Enviar'));
  const cancelLabel = resolveQuestionnaireText(t, data?.cancelLabel, t('ui.questionnaire.cancel', 'Cancelar'));

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

  // Pergunta marcada pelo backend/UI para receber o foco inicial (ex.: bloco
  // "Depois" no editor, ou o primeiro rádio de uma escolha).
  // Perguntas de escolha (boolean/single_choice/multiple_choice) não têm
  // elemento com id `question-<id>`; nelas o alvo é o primeiro input do grupo.
  const initialFocusSelector = useMemo(() => {
    const target = questions.find((q) => q.autoFocus);
    if (!target) return undefined;
    const escapedId = CSS.escape(target.id);
    if (target.type === 'boolean' || target.type === 'single_choice' || target.type === 'multiple_choice') {
      return `input[name="question-${escapedId}"]`;
    }
    return `#question-${escapedId}`;
  }, [questions]);

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
        nextErrors[q.id] = t('ui.questionnaire.requiredAnswer', 'Resposta obrigatória');
      }
    }

    if (Object.keys(nextErrors).length > 0) {
      setErrors(nextErrors);
      const errorMessages = questions
        .filter((q) => nextErrors[q.id])
        .map((q) => `${resolveQuestionnaireText(t, q.prompt)}: ${nextErrors[q.id]}`);
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
    // key={data.id}: questionários em fila trocam o `data` sem o Modal
    // fechar/remontar; sem a key, o efeito de foco inicial não roda de novo e
    // o segundo diálogo abre com o foco perdido. A remontagem refaz o
    // register/unregister na stack do modalRegistry e reaplica o foco inicial.
    <Modal
      key={data.id}
      isOpen={isOpen}
      onClose={handleCancel}
      title={title}
      size="lg"
      returnFocusOnClose={false}
      allowClose={allowCancel}
      readingMode={hasReadonlyCode}
      initialFocusSelector={initialFocusSelector}
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
          // Enter em botões segue a ativação nativa (ex.: "Rejeitar" deve
          // cancelar, não submeter o formulário).
          if (e.target instanceof HTMLButtonElement) return;
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
          const prompt = resolveQuestionnaireText(t, q.prompt);
          const hint = resolveQuestionnaireText(t, q.description);
          const placeholder = resolveQuestionnaireText(t, q.placeholder) || undefined;
          // Rótulo traduzido para a tela; valor estável para a resposta.
          const options = (q.options || []).map((option) => ({
            value: questionnaireOptionValue(option),
            label: resolveQuestionnaireText(t, option),
          }));

          return (
          <div key={q.id} className="questionnaire-dialog__question">
            <div className="questionnaire-dialog__header">
              <label
                id={labelId}
                className="questionnaire-dialog__label"
                {...(controlId ? { htmlFor: controlId } : {})}
              >
                {index + 1}. {prompt}{q.required ? ' *' : ''}
              </label>
              {hint && <div className="questionnaire-dialog__hint">{hint}</div>}
            </div>

            {q.type === 'text' && (
              <input
                id={`question-${q.id}`}
                type="text"
                value={answerText}
                placeholder={placeholder}
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
                placeholder={placeholder}
                onChange={(e) => updateAnswer(q.id, e.target.value)}
                className="questionnaire-dialog__input"
                autoFocus={index === 0}
              />
            )}

            {q.type === 'long_text' && (
              <textarea
                id={`question-${q.id}`}
                value={answerText}
                placeholder={placeholder}
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
                placeholder={placeholder}
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
              <div className="questionnaire-dialog__options" role="radiogroup" aria-label={prompt}>
                {[
                  { value: true, label: t('ui.questionnaire.yes', 'Sim') },
                  { value: false, label: t('ui.questionnaire.no', 'Não') },
                ].map((opt) => (
                  <label key={String(opt.value)} className="questionnaire-dialog__option">
                    <input
                      type="radio"
                      name={`question-${q.id}`}
                      checked={answerBoolean === opt.value}
                      onChange={() => updateAnswer(q.id, opt.value)}
                    />
                    <span>{opt.label}</span>
                  </label>
                ))}
              </div>
            )}

            {(q.type === 'single_choice' || q.type === 'multiple_choice') && (
              <div className="questionnaire-dialog__options" role={q.type === 'single_choice' ? 'radiogroup' : 'group'} aria-label={prompt}>
                {options.map((opt) => {
                  const selected = q.type === 'multiple_choice'
                    ? answerArray.includes(opt.value)
                    : answer === opt.value;

                  return (
                    <label key={opt.value} className="questionnaire-dialog__option">
                      <input
                        type={q.type === 'single_choice' ? 'radio' : 'checkbox'}
                        name={`question-${q.id}`}
                        checked={selected}
                        onChange={(e) => {
                          if (q.type === 'single_choice') {
                            updateAnswer(q.id, opt.value);
                            return;
                          }
                          const current = answerArray;
                          if (e.target.checked) {
                            updateAnswer(q.id, [...current, opt.value]);
                          } else {
                            updateAnswer(q.id, current.filter((v: string) => v !== opt.value));
                          }
                        }}
                      />
                      <span>{opt.label}</span>
                    </label>
                  );
                })}
              </div>
            )}

            {errors[q.id] && <div className="questionnaire-dialog__error">{errors[q.id]}</div>}
          </div>
          );
        })}

        <DialogActions
          className="questionnaire-dialog__footer"
          primary={
            <button type="submit" className="questionnaire-dialog__button primary">
              {submitLabel}
            </button>
          }
          secondary={
            allowCancel ? (
              <button type="button" className="questionnaire-dialog__button secondary" onClick={handleCancel}>
                {cancelLabel}
              </button>
            ) : undefined
          }
        />
      </form>
    </Modal>
  );
}
