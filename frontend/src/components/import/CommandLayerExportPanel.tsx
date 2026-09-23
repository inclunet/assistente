import { useEffect, useRef, useState } from 'react';
import { ExportData } from '@wailsjs/go/wailsapi/ExportImport';
import { portability } from '../../../wailsjs/go/models';
import { useTranslation } from 'react-i18next';
import { Button } from '../ui/Button';
import { Checkbox } from '../ui/Checkbox';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { downloadJSON, generateFilename } from '../../lib/exportImport';
import './CommandLayerExportPanel.css';

export function CommandLayerExportPanel() {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const [includeWorkspace, setIncludeWorkspace] = useState(false);
  const [isExporting, setIsExporting] = useState(false);
  const [error, setError] = useState('');
  const inFlightRef = useRef(false);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const handleExport = async () => {
    if (inFlightRef.current) return;

    inFlightRef.current = true;
    setIsExporting(true);
    setError('');
    try {
      const request = portability.ExportRequest.createFrom({
        explicitSelection: true,
        includeWorkspace,
        includeCommandLayers: true,
        outputFormat: 'json',
      });
      const jsonData = await ExportData(request);
      if (!mountedRef.current) return;
      downloadJSON(jsonData, generateFilename(t('commandExport.filenamePrefix', 'camadas-comandos')));
      announce(t('commandExport.success', 'Camadas de comandos exportadas com sucesso.'));
    } catch (cause: unknown) {
      if (!mountedRef.current) return;
      const code = typeof cause === 'string' ? cause : cause instanceof Error ? cause.message : '';
      const message = code === 'command_export.empty'
        ? t('commandExport.empty', 'Não há camadas ou personalizações para exportar neste escopo.')
        : code === 'command_export.limit'
          ? t('commandExport.limit', 'A exportação excede o limite de 64 entradas ou 64 KiB. Nenhum arquivo foi gerado.')
          : t('commandExport.error', 'Não foi possível exportar as camadas de comandos.');
      setError(message);
      announce(message, 'assertive');
    } finally {
      inFlightRef.current = false;
      if (mountedRef.current) setIsExporting(false);
    }
  };

  return (
    <section className="command-layer-export-panel" aria-labelledby="command-layer-export-title">
      <div className="command-layer-export-panel-header">
        <div>
          <h2 id="command-layer-export-title">{t('commandExport.title', 'Exportar camadas de comandos')}</h2>
          <p>{t('commandExport.description', 'Exporte somente commandLayers, sem credenciais, grants ou claims.')}</p>
        </div>
      </div>

      <fieldset className="command-layer-export-panel-fieldset">
        <legend>{t('commandExport.scopeTitle', 'Escopo da exportação')}</legend>
        <p className="command-layer-export-panel-hint">{t('commandExport.scopeDescription', 'O escopo global é incluído por padrão. Você pode incluir também os workspaces aos quais tem acesso.')}</p>
        <Checkbox
          checked={includeWorkspace}
          disabled={isExporting}
          onChange={(event) => setIncludeWorkspace(event.target.checked)}
          label={t('commandExport.includeWorkspace', 'Incluir workspaces acessíveis')}
        />
      </fieldset>

      {error && <p className="command-layer-export-panel-error">{error}</p>}

      <div className="command-layer-export-panel-actions">
        <Button type="button" variant="primary" onClick={() => void handleExport()} loading={isExporting} disabled={isExporting}>
          {t('commandExport.export', 'Exportar camadas de comandos')}
        </Button>
      </div>
    </section>
  );
}
