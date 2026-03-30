import { useTranslation } from 'react-i18next';
import { Input, Select } from '../index';

interface McpGeneralSectionProps {
  name: string;
  description: string;
  transport: string;
  onNameChange: (value: string) => void;
  onDescriptionChange: (value: string) => void;
  onTransportChange: (value: string) => void;
}

export function McpGeneralSection({
  name,
  description,
  transport,
  onNameChange,
  onDescriptionChange,
  onTransportChange,
}: McpGeneralSectionProps) {
  const { t } = useTranslation();
  const normalizedTransport = transport === 'sse' ? 'streamable' : transport;

  return (
    <section className="mcp-section" aria-labelledby="mcp-section-general">
      <h3 id="mcp-section-general">{t('mcp.general.title')}</h3>
      <div className="mcp-fields">
        <Input
          label={t('common.name')}
          type="text"
          value={name}
          onChange={(e) => onNameChange(e.target.value)}
          placeholder={t('mcp.general.namePlaceholder')}
          required
          fullWidth
        />

        <Input
          label={t('common.description')}
          type="text"
          value={description}
          onChange={(e) => onDescriptionChange(e.target.value)}
          placeholder={t('mcp.general.descriptionPlaceholder')}
          fullWidth
        />

        <Select
          label={t('mcp.general.transport')}
          value={normalizedTransport === 'stdio' ? 'stdio' : 'streamable'}
          onChange={(e) => onTransportChange(e.target.value)}
          fullWidth
          options={[
            { value: 'stdio', label: t('mcp.general.transportStdio') },
            { value: 'streamable', label: t('mcp.general.transportHTTP') },
          ]}
        />
      </div>
    </section>
  );
}
