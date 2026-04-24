/**
 * Utilitários para exportação e importação de dados
 */

export interface SelectedImportFile {
  name: string;
  content: string;
}

/**
 * Faz download de um arquivo JSON
 */
export function downloadJSON(data: string, filename: string) {
  const blob = new Blob([data], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

/**
 * Abre o diálogo de seleção de arquivo e retorna o conteúdo
 */
export function openFileDialog(accept: string = '.json'): Promise<string> {
  return openImportFileDialog(accept).then((file) => file.content);
}

/**
 * Abre o diálogo de seleção de arquivo e retorna nome + conteúdo.
 */
export function openImportFileDialog(accept: string = '.json'): Promise<SelectedImportFile> {
  return new Promise((resolve, reject) => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = accept;
    
    input.onchange = (e) => {
      const file = (e.target as HTMLInputElement).files?.[0];
      if (!file) {
        reject(new Error('Nenhum arquivo selecionado'));
        return;
      }
      
      const reader = new FileReader();
      reader.onload = () => {
        resolve({
          name: file.name,
          content: reader.result as string,
        });
      };
      reader.onerror = () => {
        reject(new Error('Erro ao ler arquivo'));
      };
      reader.readAsText(file);
    };
    
    input.click();
  });
}

/**
 * Gera nome de arquivo com timestamp
 */
export function generateFilename(prefix: string): string {
  const now = new Date();
  const timestamp = now.toISOString().replace(/[:.]/g, '-').slice(0, 19);
  return `${prefix}_${timestamp}.json`;
}
