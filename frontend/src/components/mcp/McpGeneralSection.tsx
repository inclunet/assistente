import { Input, Select } from '../index';

interface McpGeneralSectionProps {
  isNew: boolean;
  slug: string;
  name: string;
  description: string;
  transport: string;
  onSlugChange: (value: string) => void;
  onNameChange: (value: string) => void;
  onDescriptionChange: (value: string) => void;
  onTransportChange: (value: string) => void;
}

export function McpGeneralSection({
  isNew,
  slug,
  name,
  description,
  transport,
  onSlugChange,
  onNameChange,
  onDescriptionChange,
  onTransportChange,
}: McpGeneralSectionProps) {
  return (
    <section className="mcp-section" aria-labelledby="mcp-section-general">
      <h3 id="mcp-section-general">Geral</h3>
      <div className="mcp-fields">
        {isNew && (
          <Input
            label="Slug (identificador)"
            type="text"
            value={slug}
            onChange={(e) => onSlugChange(e.target.value)}
            placeholder="ex: github, filesystem"
            hint="Identificador único. Apenas letras minúsculas, números, - e _"
            required
            fullWidth
          />
        )}

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
          label="Transporte"
          value={transport}
          onChange={(e) => onTransportChange(e.target.value)}
          fullWidth
          options={[
            { value: 'stdio', label: 'stdio (processo local)' },
            { value: 'streamable', label: 'Streamable HTTP (recomendado)' },
            { value: 'sse', label: 'SSE (legado)' },
          ]}
        />
      </div>
    </section>
  );
}
