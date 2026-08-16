import { useState, useMemo, useCallback, useRef, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '../../ui/Button';
import { DialogActions } from '../../ui/DialogActions';
import { Input } from '../../ui/Input';
import { Textarea } from '../../ui/Textarea';
import { Select } from '../../ui/Select';
import { FormField } from '../../ui/FormField';
import { Checkbox } from '../../ui/Checkbox';
import { CollapsibleSection } from '../../ui/CollapsibleSection';
import { Combobox, type ComboboxItem } from '../../pickers/Combobox';
import { ToolPicker } from '../../pickers/ToolPicker';
import { SchemaForm } from './SchemaForm';
import { TriggerEditor } from './TriggerEditor';
import { OutputExplorer } from './OutputExplorer';
import { TemplateEditor } from './TemplateEditor';
import { YAMLPreview } from './YAMLPreview';
import { useJobStore } from '../../../store/jobStore';
import { useAnnouncer } from '../../../hooks/useAnnouncer';
import { ListKnownEvents, InferEventSchema } from '@wailsjs/go/wailsapi/Jobs';
import { jobs } from '@wailsjs/go/models';
import './JobBuilder.css';

export type EventMode = 'simple' | 'fanout';

export interface TriggerData {
  type: string;
  expression?: string;
  every?: string;
  listen?: string;
  keys?: string;
  when?: string;
}

export interface JobDraft {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  pipeline: string;
  tags: string[];
  triggers: TriggerData[];
  tool: string;
  inputs: Record<string, unknown>;
  events: {
    emit_success: boolean;
    on_success: string;
    emit_failure: boolean;
    on_failure: string;
    emit_when: string;
    mode: EventMode;
    for_each: string;
    payload_template: string;
  };
  error_policy: {
    strategy: string;
    max_retries: number;
    retry_delay: string;
  };
  max_runs_per_hour: number;
}

interface JobBuilderProps {
  editJob?: jobs.Job | null;
  onClose: () => void;
  onSaved?: () => void;
}

const AUTO_SUCCESS = /^job\..+\.success$/;
const AUTO_FAILURE = /^job\..+\.failure$/;

function slugify(text: string): string {
  return text
    .toLowerCase()
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 60);
}

function createEmptyDraft(): JobDraft {
  return {
    id: '',
    name: '',
    description: '',
    enabled: true,
    pipeline: '',
    tags: [],
    triggers: [{ type: 'manual' }],
    tool: '',
    inputs: {},
    events: {
      emit_success: false,
      on_success: '',
      emit_failure: false,
      on_failure: '',
      emit_when: '',
      mode: 'simple',
      for_each: '',
      payload_template: '',
    },
    error_policy: { strategy: 'stop', max_retries: 3, retry_delay: '5s' },
    max_runs_per_hour: 0,
  };
}

function jobToDraft(job: jobs.Job): JobDraft {
  const hasForEach = Boolean(job.events?.for_each);
  const hasOnSuccess = Boolean(job.events?.on_success);
  const hasOnFailure = Boolean(job.events?.on_failure);
  return {
    id: job.id,
    name: job.name,
    description: job.description,
    enabled: job.enabled,
    pipeline: job.pipeline ?? '',
    tags: job.tags ?? [],
    triggers: (job.triggers ?? [{ type: 'manual' }]).map((t) => ({
      type: t.type as string,
      expression: t.expression,
      every: t.every,
      listen: t.listen,
      keys: t.keys,
      when: t.when,
    })),
    tool: job.tool,
    inputs: (job.inputs as Record<string, unknown>) ?? {},
    events: {
      emit_success: hasOnSuccess,
      on_success: job.events?.on_success ?? '',
      emit_failure: hasOnFailure,
      on_failure: job.events?.on_failure ?? '',
      emit_when: job.events?.emit_when ?? '',
      mode: hasForEach ? 'fanout' : 'simple',
      for_each: job.events?.for_each ?? '',
      payload_template: job.events?.payload_template ?? '',
    },
    error_policy: {
      strategy: job.error_policy?.strategy ?? 'stop',
      max_retries: job.error_policy?.max_retries ?? 3,
      retry_delay: job.error_policy?.retry_delay ?? '5s',
    },
    max_runs_per_hour: job.max_runs_per_hour ?? 0,
  };
}

function hasArraysInData(data: Record<string, unknown> | null): boolean {
  if (!data) return false;
  function walk(obj: unknown): boolean {
    if (Array.isArray(obj)) return true;
    if (obj && typeof obj === 'object') {
      return Object.values(obj as Record<string, unknown>).some(walk);
    }
    return false;
  }
  return walk(data);
}

export function JobBuilder({ editJob, onClose, onSaved }: JobBuilderProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const { saveJob, testTool, fetchToolCatalog } = useJobStore();

  const isEditing = Boolean(editJob);
  const [draft, setDraft] = useState<JobDraft>(() =>
    editJob ? jobToDraft(editJob) : createEmptyDraft()
  );
  const [toolSchema, setToolSchema] = useState<Record<string, unknown> | null>(null);

  useEffect(() => {
    if (!editJob?.tool) return;
    let cancelled = false;
    fetchToolCatalog().then((catalog) => {
      if (cancelled) return;
      const entry = catalog.find((e) => e.name === editJob.tool);
      if (entry?.schema) {
        try {
          const schema = JSON.parse(typeof entry.schema === 'string' ? entry.schema : JSON.stringify(entry.schema));
          setToolSchema(schema);
        } catch { /* ignore */ }
      }
    });
    return () => { cancelled = true; };
  }, [editJob?.tool, fetchToolCatalog]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [testOutput, setTestOutput] = useState<Record<string, unknown> | null>(null);
  const [testing, setTesting] = useState(false);
  const [testDuration, setTestDuration] = useState<string | null>(null);
  const [testJustFinished, setTestJustFinished] = useState(false);
  const testResultRef = useRef<HTMLDivElement>(null);
  const announcedFanoutWarningRef = useRef<Record<string, unknown> | null>(null);

  const [eventSchema, setEventSchema] = useState<Record<string, unknown> | null>(null);
  const [knownEvents, setKnownEvents] = useState<ComboboxItem[]>([]);

  useEffect(() => {
    ListKnownEvents().then((events) => {
      if (events && events.length > 0) {
        setKnownEvents(events.map((e) => ({ value: e, label: e })));
      }
    }).catch(() => {});
  }, []);

  const [triggersOpen, setTriggersOpen] = useState(true);
  const [toolOpen, setToolOpen] = useState(true);
  const [errorPolicyOpen, setErrorPolicyOpen] = useState(() => draft.error_policy.strategy !== 'stop');
  const [yamlOpen, setYamlOpen] = useState(false);

  const outputHasArrays = useMemo(() => hasArraysInData(testOutput), [testOutput]);
  const shouldAnnounceFanoutNoArraysWarning = Boolean(
    testOutput && draft.events.emit_success && draft.events.mode === 'fanout' && !outputHasArrays
  );

  const showError = useCallback((message: string) => {
    setError(message);
    announce(message, 'assertive');
  }, [announce]);

  useEffect(() => {
    if (shouldAnnounceFanoutNoArraysWarning && testOutput && announcedFanoutWarningRef.current !== testOutput) {
      announcedFanoutWarningRef.current = testOutput;
      announce(t('jobs.builder.noArraysWarning'), 'assertive');
    }
    if (!shouldAnnounceFanoutNoArraysWarning) {
      announcedFanoutWarningRef.current = null;
    }
  }, [announce, shouldAnnounceFanoutNoArraysWarning, t, testOutput]);

  const templateContext = useMemo(() => {
    const ctx: { output?: Record<string, unknown>; event?: Record<string, unknown> } = {};
    if (testOutput) ctx.output = testOutput;
    if (eventSchema) ctx.event = eventSchema;
    if (!ctx.output && !ctx.event) return undefined;
    return ctx;
  }, [testOutput, eventSchema]);

  const canSave = useMemo(() => {
    return draft.name.length > 0 && draft.tool.length > 0;
  }, [draft.name, draft.tool]);

  const updateDraft = useCallback(<K extends keyof JobDraft>(field: K, value: JobDraft[K]) => {
    setDraft((prev) => {
      const next = { ...prev, [field]: value };
      if (field === 'name' && !isEditing) {
        const newId = slugify(value as string);
        next.id = newId;
        if (newId) {
          let events = next.events;
          if (events.emit_success && AUTO_SUCCESS.test(events.on_success)) {
            events = { ...events, on_success: `job.${newId}.success` };
          }
          if (events.emit_failure && AUTO_FAILURE.test(events.on_failure)) {
            events = { ...events, on_failure: `job.${newId}.failure` };
          }
          if (events !== next.events) next.events = events;
        }
      }
      return next;
    });
  }, [isEditing]);

  const updateInput = useCallback((key: string, value: unknown) => {
    setDraft((prev) => ({
      ...prev,
      inputs: { ...prev.inputs, [key]: value },
    }));
  }, []);

  const updateEvents = useCallback(<K extends keyof JobDraft['events']>(field: K, value: JobDraft['events'][K]) => {
    setDraft((prev) => ({
      ...prev,
      events: { ...prev.events, [field]: value },
    }));
  }, []);

  const updateErrorPolicy = useCallback((ep: JobDraft['error_policy']) => {
    setDraft((d) => ({ ...d, error_policy: ep }));
  }, []);

  const handleToolSelect = useCallback((_toolName: string, entry: jobs.CatalogEntry | null) => {
    if (!entry) return;
    setDraft((prev) => ({ ...prev, tool: entry.name, inputs: {} }));
    setTestOutput(null);
    try {
      const schema = entry.schema ? JSON.parse(typeof entry.schema === 'string' ? entry.schema : JSON.stringify(entry.schema)) : null;
      setToolSchema(schema);
    } catch {
      setToolSchema(null);
    }
  }, []);

  const hasEventTrigger = useMemo(
    () => draft.triggers.some((t) => t.type === 'event'),
    [draft.triggers],
  );

  const triggerContext = useMemo(() => {
    const ctx: { event?: Record<string, unknown> } = {};
    if (eventSchema) {
      ctx.event = eventSchema;
    } else if (hasEventTrigger) {
      ctx.event = {};
    }
    return ctx;
  }, [eventSchema, hasEventTrigger]);

  const handleTestTool = useCallback(async () => {
    if (!draft.tool) return;
    setTesting(true);
    setTestOutput(null);
    setTestDuration(null);
    setTestJustFinished(false);
    setError(null);
    try {
      let freshEventData: Record<string, unknown> | undefined = eventSchema ?? undefined;

      if (!freshEventData && hasEventTrigger) {
        const evtTrigger = draft.triggers.find((t) => t.type === 'event');
        if (evtTrigger?.listen) {
          const schema = await InferEventSchema(evtTrigger.listen);
          if (schema && Object.keys(schema).length > 0) {
            freshEventData = schema;
            setEventSchema(schema);
          }
        }
      }

      const result = await testTool(draft.tool, draft.inputs, freshEventData);
      if (result?.success) {
        setTestOutput(result.output ?? {});
        setTestDuration(result.duration ?? null);
        setTestJustFinished(true);
        announce(t('jobs.builder.testSuccess', 'Tool test completed successfully'));
        requestAnimationFrame(() => {
          testResultRef.current?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
        });
      } else if (result?.error) {
        showError(result.error);
      }
    } catch (err) {
      showError(String(err));
    } finally {
      setTesting(false);
    }
  }, [announce, draft.tool, draft.inputs, draft.triggers, testTool, eventSchema, hasEventTrigger, showError, t]);

  const handleSave = useCallback(async () => {
    setSaving(true);
    setError(null);
    try {
      const finalId = draft.id || slugify(draft.name);
      const jobData = {
        ...draft,
        id: finalId,
        tags: draft.tags.filter(Boolean),
        output: testOutput ? { schema: testOutput } : undefined,
        events: {
          on_success: draft.events.emit_success ? (draft.events.on_success || undefined) : undefined,
          on_failure: draft.events.emit_failure ? (draft.events.on_failure || undefined) : undefined,
          emit_when: draft.events.emit_success
            ? (draft.events.emit_when || undefined) : undefined,
          for_each: draft.events.emit_success && draft.events.mode === 'fanout'
            ? (draft.events.for_each || undefined) : undefined,
          payload_template: draft.events.emit_success
            ? (draft.events.payload_template || undefined) : undefined,
        },
        error_policy: draft.error_policy.strategy !== 'stop'
          ? draft.error_policy
          : undefined,
        max_runs_per_hour: draft.max_runs_per_hour > 0
          ? draft.max_runs_per_hour
          : undefined,
      };
      await saveJob(JSON.stringify(jobData));
      onSaved?.();
      onClose();
    } catch (err) {
      showError(String(err));
    } finally {
      setSaving(false);
    }
  }, [draft, saveJob, onClose, onSaved, showError, testOutput]);

  const handleFanoutSelect = useCallback((path: string) => {
    updateEvents('for_each', path);
  }, [updateEvents]);

  return (
    <div className="job-builder">
      <div className="job-builder__content">
        {error && (
          <div className="job-builder__error">
            <span>{error}</span>
            <button
              className="job-builder__error-close"
              onClick={() => setError(null)}
              aria-label={t('common.close')}
            >
              <span aria-hidden="true">✕</span>
            </button>
          </div>
        )}

        {/* Basic fields */}
        <div className="job-builder__section">
          <FormField label={t('jobs.builder.jobName')} required>
            <Input
              value={draft.name}
              onChange={(e) => updateDraft('name', e.target.value)}
              placeholder={t('jobs.builder.jobNamePlaceholder')}
              fullWidth
              required
            />
          </FormField>

          <FormField label={t('jobs.builder.description')}>
            <Textarea
              value={draft.description}
              onChange={(e) => updateDraft('description', e.target.value)}
              placeholder={t('jobs.builder.descriptionPlaceholder')}
              rows={2}
              fullWidth
            />
          </FormField>

          <div className="job-builder__row">
            <FormField label={t('jobs.builder.pipeline')}>
              <Input
                value={draft.pipeline}
                onChange={(e) => updateDraft('pipeline', e.target.value)}
                placeholder={t('jobs.builder.pipelinePlaceholder')}
                fullWidth
              />
            </FormField>
            <FormField label={t('jobs.builder.tags')}>
              <Input
                value={draft.tags.join(', ')}
                onChange={(e) => updateDraft('tags', e.target.value.split(',').map((s) => s.trim()))}
                placeholder="tag1, tag2"
                fullWidth
              />
            </FormField>
          </div>

          <Checkbox
            label={t('jobs.builder.startEnabled')}
            checked={draft.enabled}
            onChange={(e) => updateDraft('enabled', e.target.checked)}
          />
        </div>

        {/* Triggers */}
        <CollapsibleSection
          title={t('jobs.builder.sectionTriggers')}
          isOpen={triggersOpen}
          onToggle={() => setTriggersOpen(!triggersOpen)}
        >
          <div className="job-builder__section">
            <p className="job-builder__section-desc">{t('jobs.builder.triggerDesc')}</p>
            <TriggerEditor
              triggers={draft.triggers}
              onChange={(triggers) => updateDraft('triggers', triggers)}
              onEventSchemaResolved={setEventSchema}
              knownEvents={knownEvents}
            />
            {eventSchema && (
              <div className="job-builder__event-schema-badge">
                {t('jobs.builder.eventSchemaFound', { count: Object.keys(eventSchema).length })}
              </div>
            )}
            {hasEventTrigger && !eventSchema && (
              <div className="job-builder__event-schema-hint" role="note">
                {t('jobs.builder.eventSchemaEmpty')}
              </div>
            )}
          </div>
        </CollapsibleSection>

        {/* Tool + Params + Test */}
        <CollapsibleSection
          title={t('jobs.builder.sectionTool')}
          isOpen={toolOpen}
          onToggle={() => setToolOpen(!toolOpen)}
        >
          <div className="job-builder__section">
            <ToolPicker
              value={draft.tool}
              onChange={handleToolSelect}
              variant="form"
              maxWidth="100%"
            />

            {draft.tool && (
              <>
                <div className="job-builder__tool-selected">
                  {t('jobs.builder.selectedTool')}: <code>{draft.tool}</code>
                </div>

                <SchemaForm
                  schema={toolSchema as never}
                  values={draft.inputs}
                  onChange={updateInput}
                  templateMode
                  templateContext={triggerContext}
                />

                <div className="job-builder__test-area">
                  <Button
                    variant="primary"
                    onClick={handleTestTool}
                    loading={testing}
                    disabled={testing || !draft.tool}
                    aria-label={t('jobs.builder.testTool')}
                  >
                    {testing ? t('jobs.builder.testing') : t('jobs.builder.testToolRun')}
                  </Button>
                  {testDuration && (
                    <span className="job-builder__test-duration" aria-label={t('jobs.builder.testDurationLabel', { duration: testDuration })}>
                      {testDuration}
                    </span>
                  )}
                </div>

                <div>
                  {testOutput && (
                    <div ref={testResultRef} className="job-builder__test-result" role="region" aria-label={t('jobs.builder.testResult')}>
                      <h4 className="job-builder__test-result-title">{t('jobs.builder.testResult')}</h4>
                      <OutputExplorer data={testOutput} autoFocus={testJustFinished} />
                    </div>
                  )}
                </div>
              </>
            )}
          </div>
        </CollapsibleSection>

        {/* Success Event */}
        <CollapsibleSection
          title={t('jobs.builder.successEventSection')}
          isOpen={draft.events.emit_success}
          onToggle={() => {
            setDraft(prev => {
              const newEmit = !prev.events.emit_success;
              const events = { ...prev.events, emit_success: newEmit };
              if (newEmit && !events.on_success) {
                const id = prev.id || slugify(prev.name);
                if (id) events.on_success = `job.${id}.success`;
              }
              return { ...prev, events };
            });
          }}
          badge={draft.events.emit_success ? 'on' : 'off'}
        >
          <div className="job-builder__section">
            <FormField label={t('jobs.builder.successEventName')} description={t('jobs.builder.eventNameHint')}>
              <Combobox
                icon="⚡"
                items={knownEvents}
                selected={draft.events.on_success}
                onSelect={(value) => updateEvents('on_success', value)}
                placeholder={t('jobs.builder.eventNamePlaceholder')}
                allowFreeInput
                maxWidth="100%"
              />
            </FormField>

            <FormField label={t('jobs.builder.emitWhen')} description={t('jobs.builder.emitWhenHint')}>
              <Input
                value={draft.events.emit_when}
                onChange={(e) => updateEvents('emit_when', e.target.value)}
                placeholder={'{{ eq .output.status "done" }}'}
                fullWidth
              />
            </FormField>

            <FormField label={t('jobs.builder.eventMode')}>
              <div className="job-builder__mode-cards" role="radiogroup" aria-label={t('jobs.builder.eventMode')}>
                <button
                  type="button"
                  className={`job-builder__mode-card${draft.events.mode === 'simple' ? ' job-builder__mode-card--selected' : ''}`}
                  role="radio"
                  aria-checked={draft.events.mode === 'simple'}
                  onClick={() => {
                    updateEvents('mode', 'simple' as EventMode);
                    updateEvents('for_each', '');
                  }}
                >
                  <span className="job-builder__mode-icon" aria-hidden="true">→</span>
                  <span className="job-builder__mode-label">{t('jobs.builder.eventTypeSimple')}</span>
                  <span className="job-builder__mode-desc">{t('jobs.builder.eventTypeSimpleDesc')}</span>
                </button>
                <button
                  type="button"
                  className={`job-builder__mode-card${draft.events.mode === 'fanout' ? ' job-builder__mode-card--selected' : ''}`}
                  role="radio"
                  aria-checked={draft.events.mode === 'fanout'}
                  onClick={() => updateEvents('mode', 'fanout' as EventMode)}
                >
                  <span className="job-builder__mode-icon" aria-hidden="true">⇉</span>
                  <span className="job-builder__mode-label">{t('jobs.builder.eventTypeFanout')}</span>
                  <span className="job-builder__mode-desc">{t('jobs.builder.eventTypeFanoutDesc')}</span>
                </button>
              </div>
            </FormField>

            {draft.events.mode === 'fanout' && (
              <div className="job-builder__fanout">
                <FormField label={t('jobs.builder.fanoutKey')} description={t('jobs.builder.fanoutKeyHint')} required>
                  <Input
                    value={draft.events.for_each}
                    onChange={(e) => updateEvents('for_each', e.target.value)}
                    placeholder="items"
                    fullWidth
                  />
                </FormField>
                {testOutput && (
                  <div className="job-builder__fanout-explorer">
                    <p className="job-builder__hint">{t('jobs.builder.fanoutSelectArray')}</p>
                    <OutputExplorer
                      data={testOutput}
                      highlightArrays
                      onSelectPath={handleFanoutSelect}
                    />
                    {!outputHasArrays && (
                      <p className="job-builder__warning">
                        {t('jobs.builder.noArraysWarning')}
                      </p>
                    )}
                  </div>
                )}
              </div>
            )}

            <FormField
              label={t('jobs.builder.payloadTemplateEditor')}
              description={t('jobs.builder.payloadTemplateHintEmpty')}
            >
              <TemplateEditor
                value={draft.events.payload_template}
                onChange={(v) => updateEvents('payload_template', v)}
                context={templateContext}
                height="160px"
                placeholder={'{\n  "name": "{{ .output.name }}",\n  "id": "{{ .output.id }}"\n}'}
                ariaLabel={t('jobs.builder.payloadTemplateEditor')}
              />
            </FormField>
            {testOutput && draft.events.payload_template && (
              <div className="job-builder__template-explorer">
                <p className="job-builder__hint">{t('jobs.builder.treeClickToCopy')}</p>
                <OutputExplorer data={testOutput} />
              </div>
            )}
          </div>
        </CollapsibleSection>

        {/* Failure Event */}
        <CollapsibleSection
          title={t('jobs.builder.failureEventSection')}
          isOpen={draft.events.emit_failure}
          onToggle={() => {
            setDraft(prev => {
              const newEmit = !prev.events.emit_failure;
              const events = { ...prev.events, emit_failure: newEmit };
              if (newEmit && !events.on_failure) {
                const id = prev.id || slugify(prev.name);
                if (id) events.on_failure = `job.${id}.failure`;
              }
              return { ...prev, events };
            });
          }}
          badge={draft.events.emit_failure ? 'on' : 'off'}
        >
          <div className="job-builder__section">
            <FormField label={t('jobs.builder.failureEventName')} description={t('jobs.builder.failureEventHint')}>
              <Combobox
                icon="⚡"
                items={knownEvents}
                selected={draft.events.on_failure}
                onSelect={(value) => updateEvents('on_failure', value)}
                placeholder={t('jobs.builder.eventNamePlaceholder')}
                allowFreeInput
                maxWidth="100%"
              />
            </FormField>
          </div>
        </CollapsibleSection>

        {/* Error Policy */}
        <CollapsibleSection
          title={t('jobs.builder.errorPolicySection')}
          isOpen={errorPolicyOpen}
          onToggle={() => setErrorPolicyOpen(!errorPolicyOpen)}
        >
          <div className="job-builder__section">
            <FormField label={t('jobs.builder.maxRunsPerHour')} description={t('jobs.builder.maxRunsPerHourDesc')}>
              <Input
                type="number"
                value={draft.max_runs_per_hour > 0 ? String(draft.max_runs_per_hour) : ''}
                onChange={(e) => {
                  const v = parseInt(e.target.value, 10);
                  setDraft((d) => ({ ...d, max_runs_per_hour: isNaN(v) ? 0 : v }));
                }}
                placeholder="60"
                min={0}
                fullWidth
              />
            </FormField>

            <FormField label={t('jobs.builder.errorStrategy')}>
              <Select
                options={[
                  { value: 'stop', label: t('jobs.builder.strategyStop') },
                  { value: 'retry', label: t('jobs.builder.strategyRetry') },
                  { value: 'skip', label: t('jobs.builder.strategySkip') },
                ]}
                value={draft.error_policy.strategy}
                onChange={(e) => updateErrorPolicy({ ...draft.error_policy, strategy: e.target.value })}
                fullWidth
              />
            </FormField>

            {draft.error_policy.strategy === 'retry' && (
              <>
                <FormField label={t('jobs.builder.maxRetries')}>
                  <Input
                    type="number"
                    value={String(draft.error_policy.max_retries)}
                    onChange={(e) => updateErrorPolicy({
                      ...draft.error_policy,
                      max_retries: parseInt(e.target.value, 10) || 0,
                    })}
                    min={1}
                    max={10}
                    fullWidth
                  />
                </FormField>
                <FormField label={t('jobs.builder.retryDelay')}>
                  <Input
                    value={draft.error_policy.retry_delay}
                    onChange={(e) => updateErrorPolicy({
                      ...draft.error_policy,
                      retry_delay: e.target.value,
                    })}
                    placeholder="5s"
                    fullWidth
                  />
                </FormField>
              </>
            )}
          </div>
        </CollapsibleSection>

        {/* YAML Preview */}
        <CollapsibleSection
          title={t('jobs.builder.yamlPreview')}
          isOpen={yamlOpen}
          onToggle={() => setYamlOpen(!yamlOpen)}
        >
          <YAMLPreview draft={draft} />
        </CollapsibleSection>
      </div>

      {/* Footer — AEP-0090: primária antes de cancelar */}
      <div className="job-builder__footer">
        <DialogActions
          primary={
            <Button variant="primary" onClick={handleSave} loading={saving} disabled={saving || !canSave}>
              {isEditing ? t('common.save') : t('jobs.builder.createJob')}
            </Button>
          }
          secondary={
            <Button variant="ghost" onClick={onClose}>
              {t('common.cancel')}
            </Button>
          }
        />
      </div>
    </div>
  );
}
