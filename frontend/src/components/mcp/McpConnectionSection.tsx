import { Checkbox, Input, Textarea } from '../index';

interface McpConnectionSectionProps {
  transport: string;
  command: string;
  args: string;
  url: string;
  envText: string;
  enabled: boolean;
  autoConnect: boolean;
  onCommandChange: (value: string) => void;
  onArgsChange: (value: string) => void;
  onUrlChange: (value: string) => void;
  onEnvTextChange: (value: string) => void;
  onEnabledChange: (value: boolean) => void;
  onAutoConnectChange: (value: boolean) => void;
}

export function McpConnectionSection({
  transport,
  command,
  args,
  url,
  envText,
  enabled,
  autoConnect,
  onCommandChange,
  onArgsChange,
  onUrlChange,
  onEnvTextChange,
  onEnabledChange,
  onAutoConnectChange,
}: McpConnectionSectionProps) {
  return (
    <section className="mcp-section" aria-labelledby="mcp-section-connection">
      <h3 id="mcp-section-connection">Conexão</h3>
      <div className="mcp-fields">
        {transport === 'stdio' && (
          <>
            <Input
              label="Comando"
              type="text"
              value={command}
              onChange={(e) => onCommandChange(e.target.value)}
              placeholder="ex: npx, node, python"
              required
              fullWidth
            />
            <Input
              label="Argumentos (separados por espaço)"
              type="text"
              value={args}
              onChange={(e) => onArgsChange(e.target.value)}
              placeholder="ex: -y @modelcontextprotocol/server-filesystem /home"
              fullWidth
            />
          </>
        )}

        {transport === 'sse' && (
          <Input
            label="URL do servidor"
            type="url"
            value={url}
            onChange={(e) => onUrlChange(e.target.value)}
            placeholder="https://example.com/mcp"
            required
            fullWidth
          />
        )}

        <div>
          <Textarea
            label="Variáveis de ambiente (KEY=VALUE, uma por linha)"
            rows={4}
            value={envText}
            onChange={(e) => onEnvTextChange(e.target.value)}
            placeholder={"GITHUB_TOKEN=ghp_xxx\nNODE_ENV=production"}
            fullWidth
          />
          <p className="mcp-hint">Linhas começando com # são ignoradas.</p>
        </div>

        <div>
          <p className="mcp-options__label">Opções</p>
          <div className="mcp-options">
            <Checkbox
              label="Habilitado"
              checked={enabled}
              onChange={(e) => onEnabledChange(e.target.checked)}
            />
            <Checkbox
              label="Conectar automaticamente no início"
              checked={autoConnect}
              onChange={(e) => onAutoConnectChange(e.target.checked)}
            />
          </div>
        </div>
      </div>
    </section>
  );
}
