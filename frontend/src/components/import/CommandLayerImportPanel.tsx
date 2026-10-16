import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ImportDataWithResolutions } from '@wailsjs/go/wailsapi/ExportImport';
import { ListWorkspaces } from '@wailsjs/go/wailsapi/Workspace';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import { Button } from '../ui/Button';
import { FormField } from '../ui/FormField';
import { Input } from '../ui/Input';
import { Select } from '../ui/Select';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { formatPortabilityMessage, portabilityMessageKey, type PortabilityMessage } from '../../lib/portabilityMessages';
import { portability } from '../../../wailsjs/go/models';
import './CommandLayerImportPanel.css';

const COMMAND_LAYERS_RESOURCE = 'commandLayers';
const MAX_COMMAND_IMPORT_BYTES = 64 * 1024;

type CommandLayerImportKind = 'commandLayers' | 'mixed' | 'other' | 'invalid';

export interface CommandLayerImportInspection {
  kind: CommandLayerImportKind;
  layerCount: number;
}

export interface CommandLayerImportSelection {
  fileName: string;
  jsonData: string;
}

interface CommandLayerExport {
  id: string;
  name: string;
  deltaOnly: boolean;
  scopeKind: string;
  workspaceId: string;
}

interface WorkspaceOption {
  id: string;
  name: string;
}

interface ImportResult {
  success: boolean;
  imported: number;
  skipped: number;
  failed: number;
  warnings?: (PortabilityMessage | string)[];
  errors?: (PortabilityMessage | string)[];
  message?: string | PortabilityMessage;
}

interface ImportResolution {
  resourceType: string;
  identifier: string;
  strategy: 'skip' | 'overwrite' | 'rename';
  renameValue?: string;
}

interface CommandLayerImportPanelProps {
  selection: CommandLayerImportSelection;
  onChangeFile: () => void;
  selectionKey?: string;
  onBusyChange?: (busy: boolean, selectionKey: string) => void;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function parseLayers(jsonData: string): CommandLayerExport[] {
  const parsed: unknown = JSON.parse(jsonData);
  if (!isRecord(parsed) || !isRecord(parsed.resources) || !Array.isArray(parsed.resources[COMMAND_LAYERS_RESOURCE])) {
    return [];
  }

  return parsed.resources[COMMAND_LAYERS_RESOURCE].map((value): CommandLayerExport => {
    const layer = isRecord(value) ? value : {};
    const scope = isRecord(layer.scope) ? layer.scope : {};
    return {
      id: typeof layer.id === 'string' ? layer.id : '',
      name: typeof layer.name === 'string' ? layer.name : '',
      deltaOnly: layer.deltaOnly === true,
      scopeKind: typeof scope.kind === 'string' ? scope.kind : '',
      workspaceId: typeof scope.workspaceId === 'string' ? scope.workspaceId : '',
    };
  });
}

export function inspectCommandLayerImport(jsonData: string): CommandLayerImportInspection {
  try {
    const parsed: unknown = JSON.parse(jsonData);
    if (!isRecord(parsed) || !isRecord(parsed.resources)) {
      return { kind: 'other', layerCount: 0 };
    }

    const resources = parsed.resources;
    const hasCommandLayers = Object.prototype.hasOwnProperty.call(resources, COMMAND_LAYERS_RESOURCE);
    if (!hasCommandLayers) return { kind: 'other', layerCount: 0 };

    const hasOtherResources = Object.keys(resources).some((key) => key !== COMMAND_LAYERS_RESOURCE);
    const layerCount = Array.isArray(resources[COMMAND_LAYERS_RESOURCE])
      ? resources[COMMAND_LAYERS_RESOURCE].length
      : 0;
    return {
      kind: hasOtherResources ? 'mixed' : 'commandLayers',
      layerCount,
    };
  } catch {
    return { kind: 'invalid', layerCount: 0 };
  }
}

function isWorkspaceLayer(layer: CommandLayerExport): boolean {
  return layer.scopeKind === 'workspace' && layer.workspaceId.trim().length > 0;
}

function getUniqueWorkspaceIds(layers: CommandLayerExport[]): string[] {
  return Array.from(new Set(layers.filter(isWorkspaceLayer).map((layer) => layer.workspaceId)));
}

function getErrorReason(error: unknown): string {
  if (error instanceof Error && error.message.trim()) return error.message;
  if (typeof error === 'string' && error.trim()) return error;
  if (isRecord(error) && typeof error.message === 'string' && error.message.trim()) return error.message;
  return '';
}

function strategyForPolicy(policy: CommandLayerPolicy): ImportResolution['strategy'] {
  if (policy === 'keep') return 'skip';
  if (policy === 'replace') return 'overwrite';
  return 'rename';
}

type CommandLayerPolicy = '' | 'keep' | 'replace' | 'copy';

function getActionLabel(policy: CommandLayerPolicy, t: TFunction): string {
  if (policy === 'keep') return t('commandImport.policyKeep', 'manter');
  if (policy === 'replace') return t('commandImport.policyReplace', 'substituir');
  if (policy === 'copy') return t('commandImport.policyCopy', 'copiar');
  return '';
}

export function CommandLayerImportPanel({ selection, onChangeFile, selectionKey = selection.fileName, onBusyChange }: CommandLayerImportPanelProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const titleRef = useRef<HTMLHeadingElement>(null);
  const inspection = useMemo(() => inspectCommandLayerImport(selection.jsonData), [selection.jsonData]);
  const layers = useMemo(() => {
    try {
      return parseLayers(selection.jsonData);
    } catch {
      return [];
    }
  }, [selection.jsonData]);
  const sourceWorkspaceIds = useMemo(() => getUniqueWorkspaceIds(layers), [layers]);

  const [policy, setPolicy] = useState<CommandLayerPolicy>('');
  const [layerNames, setLayerNames] = useState<Record<string, string>>({});
  const [workspaceMap, setWorkspaceMap] = useState<Record<string, string>>({});
  const [workspaces, setWorkspaces] = useState<WorkspaceOption[]>([]);
  const [workspaceError, setWorkspaceError] = useState('');
  const [policyError, setPolicyError] = useState('');
  const [isLoadingWorkspaces, setIsLoadingWorkspaces] = useState(sourceWorkspaceIds.length > 0);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [submitError, setSubmitError] = useState('');
  const submitInFlightRef = useRef(false);
  const busyRef = useRef(false);

  const setBusy = useCallback((busy: boolean) => {
    if (busyRef.current === busy) return;
    busyRef.current = busy;
    onBusyChange?.(busy, selectionKey);
  }, [onBusyChange, selectionKey]);

  useEffect(() => () => {
    if (!busyRef.current) return;
    busyRef.current = false;
    onBusyChange?.(false, selectionKey);
  }, [onBusyChange, selectionKey]);

  useEffect(() => {
    titleRef.current?.focus();
  }, []);

  useEffect(() => {
    let cancelled = false;
    if (sourceWorkspaceIds.length === 0) {
      setIsLoadingWorkspaces(false);
      setWorkspaces([]);
      setWorkspaceError('');
      return () => { cancelled = true; };
    }

    setIsLoadingWorkspaces(true);
    setWorkspaceError('');
    void ListWorkspaces()
      .then((items) => {
        if (cancelled) return;
        const next = (items || [])
          .map((item) => ({ id: String(item.id ?? '').trim(), name: String(item.name ?? '').trim() }))
          .filter((item) => item.id.length > 0);
        setWorkspaces(next);
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        const reason = getErrorReason(error);
        const message = t('commandImport.workspaceListError', {
          defaultValue: 'Não foi possível carregar os workspaces de destino{{reason}}.',
          reason: reason ? `: ${reason}` : '',
        });
        setWorkspaceError(message);
        announce(message, 'assertive');
      })
      .finally(() => {
        if (!cancelled) setIsLoadingWorkspaces(false);
      });

    return () => { cancelled = true; };
  }, [announce, sourceWorkspaceIds, t]);

  const updateLayerName = useCallback((layerId: string, value: string) => {
    setLayerNames((previous) => ({ ...previous, [layerId]: value }));
  }, []);

  const updateWorkspaceDestination = useCallback((sourceId: string, value: string) => {
    setWorkspaceMap((previous) => ({ ...previous, [sourceId]: value }));
  }, []);

  const validationError = useCallback((): string => {
    if (policy !== 'keep' && policy !== 'replace' && policy !== 'copy') {
      return t('commandImport.policyRequired', 'Selecione uma política de conflito antes de importar.');
    }
    if (sourceWorkspaceIds.some((sourceId) => !workspaceMap[sourceId])) {
      return t('commandImport.workspaceMappingRequired', 'Selecione um workspace de destino para cada workspace de origem.');
    }
    if (sourceWorkspaceIds.length > 0 && workspaces.length === 0) {
      return t('commandImport.noWorkspaceDestination', 'Nenhum workspace de destino autorizado está disponível.');
    }
    return '';
  }, [policy, sourceWorkspaceIds, t, workspaceMap, workspaces.length]);

  const handleSubmit = useCallback(async () => {
    if (submitInFlightRef.current || isSubmitting || result) return;
    const error = validationError();
    if (error) {
      setPolicyError(error);
      announce(error, 'assertive');
      return;
    }

    setPolicyError('');
    setSubmitError('');
    submitInFlightRef.current = true;
    setBusy(true);
    setIsSubmitting(true);
    announce(t('commandImport.pending', 'Importação de camadas de comandos pendente de confirmação.'), 'polite');
    const resolutions: ImportResolution[] = [
      {
        resourceType: 'commandLayers',
        identifier: '*',
        strategy: strategyForPolicy(policy),
      },
    ];

    if (policy === 'copy' || policy === 'replace') {
      layers.forEach((layer) => {
        if (layer.deltaOnly || !layer.id) return;
        const name = (layerNames[layer.id] || '').trim();
        if (name) {
          resolutions.push({
            resourceType: 'commandLayerName',
            identifier: layer.id,
            strategy: 'rename',
            renameValue: name,
          });
        }
      });
    }

    sourceWorkspaceIds.forEach((sourceId) => {
      resolutions.push({
        resourceType: 'commandWorkspace',
        identifier: sourceId,
        strategy: 'rename',
        renameValue: workspaceMap[sourceId],
      });
    });

    try {
      const imported = await ImportDataWithResolutions(portability.ImportRequest.createFrom({
        jsonData: selection.jsonData,
        resolutions: resolutions.map((resolution) => portability.ImportResolution.createFrom(resolution)),
      })) as ImportResult;
      setResult(imported);
      const messages = [
        ...(imported.errors || []).map((item) => formatPortabilityMessage(item, t)),
        ...(imported.warnings || []).map((item) => formatPortabilityMessage(item, t)),
      ];
      const summary = [
        imported.success
          ? t('commandImport.success', 'Camadas de comandos importadas.')
          : t('commandImport.partial', 'A importação das camadas de comandos não foi concluída integralmente.'),
        t('commandImport.counts', {
          defaultValue: 'Importadas: {{imported}}. Ignoradas: {{skipped}}. Falhas: {{failed}}.',
          imported: imported.imported || 0,
          skipped: imported.skipped || 0,
          failed: imported.failed || 0,
        }),
        ...messages,
      ].filter(Boolean).join(' ');
      announce(summary, imported.success ? 'polite' : 'assertive');
    } catch (error: unknown) {
      const reason = getErrorReason(error);
      const message = t('commandImport.error', {
        defaultValue: 'Não foi possível importar as camadas de comandos{{reason}}.',
        reason: reason ? `: ${reason}` : '',
      });
      setSubmitError(message);
	  announce(message, 'assertive');
    } finally {
      submitInFlightRef.current = false;
      setBusy(false);
      setIsSubmitting(false);
    }
  }, [announce, isSubmitting, layerNames, layers, policy, result, selection.jsonData, setBusy, sourceWorkspaceIds, t, validationError, workspaceMap]);

  const fileSize = new TextEncoder().encode(selection.jsonData).byteLength;
  const isFileTooLarge = fileSize > MAX_COMMAND_IMPORT_BYTES;
  const hasInvalidEnvelope = inspection.kind !== 'commandLayers' || layers.length === 0;

  const fileWarning = inspection.kind === 'mixed'
    ? t('commandImport.mixedResources', 'Arquivos de camadas de comandos não podem misturar outros recursos. Selecione um arquivo somente de commandLayers.')
    : inspection.kind === 'invalid'
      ? t('commandImport.invalidFile', 'O arquivo de camadas de comandos não é um JSON válido.')
      : isFileTooLarge
        ? t('commandImport.fileTooLarge', { defaultValue: 'O arquivo excede o limite de {{limit}} KiB para camadas de comandos.', limit: MAX_COMMAND_IMPORT_BYTES / 1024 })
        : '';
  useEffect(() => { if (fileWarning) announce(fileWarning, 'assertive'); }, [announce, fileWarning]);
  const isApplyDisabled = isSubmitting || !!result || isLoadingWorkspaces || !!workspaceError || hasInvalidEnvelope || isFileTooLarge;
  const policyOptions = [
    { value: '', label: t('commandImport.policyPlaceholder', 'Selecione uma política'), disabled: true },
    { value: 'keep', label: t('commandImport.policyKeep', 'Manter') },
    { value: 'replace', label: t('commandImport.policyReplace', 'Substituir') },
    { value: 'copy', label: t('commandImport.policyCopy', 'Copiar como nova camada') },
  ];
  const workspaceOptions = [
    { value: '', label: t('commandImport.workspacePlaceholder', 'Selecione um destino'), disabled: true },
    ...workspaces.map((workspace) => ({
      value: workspace.id,
      label: workspace.name ? `${workspace.name} (${workspace.id})` : workspace.id,
    })),
  ];

  return (
    <section className="command-layer-import-panel" aria-labelledby="command-layer-import-title">
      <div className="command-layer-import-panel-header">
        <div>
          <h2 id="command-layer-import-title" ref={titleRef} tabIndex={-1}>
            {t('commandImport.title', 'Importar camadas de comandos')}
          </h2>
          <p>{t('commandImport.description', 'Este arquivo contém somente camadas de comandos. Revise a política e os destinos antes da confirmação.')}</p>
        </div>
        <span className="command-layer-import-panel-file" title={selection.fileName}>{selection.fileName}</span>
      </div>

      {inspection.kind === 'mixed' && (
        <p className="command-layer-import-panel-error">
          {t('commandImport.mixedResources', 'Arquivos de camadas de comandos não podem misturar outros recursos. Selecione um arquivo somente de commandLayers.')}
        </p>
      )}
      {inspection.kind === 'invalid' && (
        <p className="command-layer-import-panel-error">
          {t('commandImport.invalidFile', 'O arquivo de camadas de comandos não é um JSON válido.')}
        </p>
      )}
      {isFileTooLarge && (
        <p className="command-layer-import-panel-error">
          {t('commandImport.fileTooLarge', {
            defaultValue: 'O arquivo excede o limite de {{limit}} KiB para camadas de comandos.',
            limit: MAX_COMMAND_IMPORT_BYTES / 1024,
          })}
        </p>
      )}

      <dl className="command-layer-import-panel-summary">
        <div>
          <dt>{t('commandImport.fileLabel', 'Arquivo')}</dt>
          <dd>{selection.fileName}</dd>
        </div>
        <div>
          <dt>{t('commandImport.layerCountLabel', 'Camadas')}</dt>
          <dd>{inspection.layerCount}</dd>
        </div>
        <div>
          <dt>{t('commandImport.policyLabel', 'Política')}</dt>
          <dd>{getActionLabel(policy, t) || t('commandImport.notSelected', 'Não selecionada')}</dd>
        </div>
      </dl>

      <FormField
        label={t('commandImport.policyLabel', 'Política de conflito')}
        description={t('commandImport.policyDescription', 'A política é obrigatória e não possui uma opção destrutiva pré-selecionada.')}
        error={policyError || null}
        required
      >
        <Select
          options={policyOptions}
          value={policy}
          onChange={(event) => {
            setPolicy(event.target.value as CommandLayerPolicy);
            setPolicyError('');
          }}
          required
          aria-required="true"
          disabled={isSubmitting || !!result}
        />
      </FormField>

      {(policy === 'copy' || policy === 'replace') && layers.some((layer) => !layer.deltaOnly && layer.id) && (
        <fieldset className="command-layer-import-panel-fieldset">
          <legend>{t('commandImport.copyNamesTitle', 'Nomes de destino')}</legend>
          <p className="command-layer-import-panel-hint">{t('commandImport.copyNamesDescription', 'Informe um nome de destino quando a importação precisar evitar conflito de nome. Os campos são opcionais; a validação final é do backend.')}</p>
          {layers.filter((layer) => !layer.deltaOnly && layer.id).map((layer) => (
            <FormField key={layer.id} label={t('commandImport.copyNameLabel', { defaultValue: 'Nome para {{name}}', name: layer.name || layer.id })}>
              <Input
                value={layerNames[layer.id] || ''}
                onChange={(event) => updateLayerName(layer.id, event.target.value)}
                placeholder={t('commandImport.copyNamePlaceholder', 'Nome opcional')}
                disabled={isSubmitting || !!result}
              />
            </FormField>
          ))}
        </fieldset>
      )}

      {sourceWorkspaceIds.length > 0 && (
        <fieldset className="command-layer-import-panel-fieldset">
          <legend>{t('commandImport.workspaceTitle', 'Destinos de workspace')}</legend>
          <p className="command-layer-import-panel-hint">{t('commandImport.workspaceDescription', 'Cada workspace de origem precisa de um destino explícito autorizado pela aplicação.')}</p>
          {isLoadingWorkspaces && <p className="command-layer-import-panel-hint">{t('commandImport.workspaceLoading', 'Carregando destinos...')}</p>}
          {workspaceError && <p className="command-layer-import-panel-error">{workspaceError}</p>}
          {sourceWorkspaceIds.map((sourceId) => (
            <FormField key={sourceId} label={t('commandImport.workspaceDestinationLabel', { defaultValue: 'Destino para {{sourceId}}', sourceId })} required>
              <Select
                options={workspaceOptions}
                value={workspaceMap[sourceId] || ''}
                onChange={(event) => updateWorkspaceDestination(sourceId, event.target.value)}
                required
                aria-required="true"
                disabled={isLoadingWorkspaces || workspaces.length === 0 || isSubmitting || !!result}
              />
            </FormField>
          ))}
        </fieldset>
      )}

      {result && (
        <div className="command-layer-import-panel-report">
          <div className="command-layer-import-panel-report-header">
            <strong>{t('commandImport.reportTitle', 'Relatório da importação')}</strong>
            <span>{result.success ? t('commandImport.reportSuccess', 'Concluído') : t('commandImport.reportFailure', 'Não concluído')}</span>
          </div>
          <p>{t('commandImport.counts', {
            defaultValue: 'Importadas: {{imported}}. Ignoradas: {{skipped}}. Falhas: {{failed}}.',
            imported: result.imported || 0,
            skipped: result.skipped || 0,
            failed: result.failed || 0,
          })}</p>
          <p>{t('commandImport.reportLocked', 'Este arquivo não pode ser aplicado novamente. Troque o arquivo para iniciar outra importação.')}</p>
          {!!result.errors?.length && (
            <div>
              <strong>{t('commandImport.errorsLabel', 'Erros')}</strong>
              <ul className="command-layer-import-panel-list">
                {result.errors.map((error, index) => (
                  <li key={portabilityMessageKey(error, index)}>{formatPortabilityMessage(error, t)}</li>
                ))}
              </ul>
            </div>
          )}
          {!!result.warnings?.length && (
            <div>
              <strong>{t('commandImport.warningsLabel', 'Avisos')}</strong>
              <ul className="command-layer-import-panel-list command-layer-import-panel-list-warning">
                {result.warnings.map((warning, index) => (
                  <li key={portabilityMessageKey(warning, index)}>{formatPortabilityMessage(warning, t)}</li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}

      {submitError && <p className="command-layer-import-panel-error">{submitError}</p>}

      <div className="command-layer-import-panel-actions">
        <Button type="button" variant="primary" onClick={() => void handleSubmit()} loading={isSubmitting} disabled={isApplyDisabled}>
          {t('commandImport.apply', 'Aplicar importação')}
        </Button>
        <Button type="button" variant="secondary" onClick={onChangeFile} disabled={isSubmitting}>
          {t('commandImport.changeFile', 'Trocar arquivo')}
        </Button>
      </div>
      {isSubmitting && <p className="command-layer-import-panel-hint">{t('commandImport.pendingHint', 'Aguardando a confirmação do backend. Não envie novamente enquanto esta operação estiver pendente.')}</p>}
    </section>
  );
}

export { MAX_COMMAND_IMPORT_BYTES };
