import { useState } from 'react';
import { TestHTTPEndpoint } from '../../../wailsjs/go/main/App';
import { Button } from '../ui/Button';
import { Input } from '../ui/Input';
import './EndpointTester.css';

interface EndpointTesterProps {
  agentId: number;
  endpointName: string;
  parametersSchema: string; // JSON Schema
}

interface FieldConfig {
  name: string;
  type: string;
  description?: string;
  required: boolean;
}

export function EndpointTester({ agentId, endpointName, parametersSchema }: EndpointTesterProps) {
  const [fields, setFields] = useState<FieldConfig[]>([]);
  const [values, setValues] = useState<Record<string, string>>({});
  const [result, setResult] = useState<string>('');
  const [error, setError] = useState<string>('');
  const [testing, setTesting] = useState(false);
  const [expanded, setExpanded] = useState(false);

  // Parseia o JSON Schema e extrai os campos
  const parseSchema = () => {
    if (!parametersSchema || parametersSchema.trim() === '') {
      setError('Schema de parâmetros vazio');
      return [];
    }

    try {
      const schema = JSON.parse(parametersSchema);
      const properties = schema.properties || {};
      const required = schema.required || [];

      const fieldsList: FieldConfig[] = Object.keys(properties).map(name => ({
        name,
        type: properties[name].type || 'string',
        description: properties[name].description,
        required: required.includes(name),
      }));

      setFields(fieldsList);
      setError('');
      return fieldsList;
    } catch (err) {
      setError(`Erro ao parsear schema: ${err}`);
      return [];
    }
  };

  const handleTest = async () => {
    setTesting(true);
    setResult('');
    setError('');

    try {
      // Converte valores de string para tipos corretos
      const params: Record<string, any> = {};
      fields.forEach(field => {
        const value = values[field.name];
        if (value !== undefined && value !== '') {
          if (field.type === 'number' || field.type === 'integer') {
            params[field.name] = Number(value);
          } else if (field.type === 'boolean') {
            params[field.name] = value === 'true';
          } else {
            params[field.name] = value;
          }
        }
      });

      const paramsJSON = JSON.stringify(params);
      const response = await TestHTTPEndpoint(agentId, endpointName, paramsJSON);
      setResult(response);
    } catch (err) {
      setError(`Erro ao testar: ${err}`);
    } finally {
      setTesting(false);
    }
  };

  const handleExpand = () => {
    if (!expanded) {
      parseSchema();
    }
    setExpanded(!expanded);
  };

  return (
    <div className="endpoint-tester">
      <button
        className="endpoint-tester-toggle"
        onClick={handleExpand}
        aria-expanded={expanded}
        aria-label={expanded ? 'Ocultar testador' : 'Mostrar testador de endpoint'}
      >
        {expanded ? '▼' : '▶'} Testar Endpoint
      </button>

      {expanded && (
        <div className="endpoint-tester-content">
          <h4>Teste do endpoint: {endpointName}</h4>

          {error && (
            <div className="endpoint-tester-error" role="alert">
              ❌ {error}
            </div>
          )}

          {fields.length > 0 && (
            <div className="endpoint-tester-fields">
              <h5>Parâmetros</h5>
              {fields.map(field => (
                <div key={field.name} className="endpoint-tester-field">
                  <label htmlFor={`test-${field.name}`}>
                    {field.name}
                    {field.required && <span className="required">*</span>}
                    <span className="field-type">({field.type})</span>
                  </label>
                  {field.description && (
                    <div className="field-description">{field.description}</div>
                  )}
                  <Input
                    id={`test-${field.name}`}
                    value={values[field.name] || ''}
                    onChange={(e) => setValues({ ...values, [field.name]: e.target.value })}
                    placeholder={field.description || `Digite ${field.name}`}
                    required={field.required}
                  />
                </div>
              ))}

              <div className="endpoint-tester-actions">
                <Button
                  onClick={handleTest}
                  disabled={testing}
                  variant="primary"
                >
                  {testing ? 'Testando...' : '🧪 Executar Teste'}
                </Button>
              </div>
            </div>
          )}

          {result && (
            <div className="endpoint-tester-result">
              <h5>Resultado</h5>
              <pre aria-label="Resultado do teste">{result}</pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
