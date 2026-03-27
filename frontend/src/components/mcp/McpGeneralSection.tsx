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
  const normalizedTransport = transport === 'sse' ? 'streamable' : transport;

  return (
    <section className="mcp-section" aria-labelledby="mcp-section-general">
      <h3 id="mcp-section-general">Geral</h3>
      <div className="mcp-fields">
        <Input
          label="Nome"
          type="text"
          value={name}
          onChange={(e) => onNameChange(e.target.value)}
          placeholder="ex: GitHub Tools"
          required
          fullWidth
        />

        <Input
          label="Descrição"
          type="text"
          value={description}
          onChange={(e) => onDescriptionChange(e.target.value)}
          placeholder="Descrição opcional do servidor"
          fullWidth
        />

        <Select
          label="Tipo"
          value={normalizedTransport === 'stdio' ? 'stdio' : 'streamable'}
          onChange={(e) => onTransportChange(e.target.value)}
          fullWidth
          options={[
            { value: 'stdio', label: 'Local (stdio)' },
            { value: 'streamable', label: 'Remoto (HTTP)' },
          ]}
        />
      </div>
    </section>
  );
}
