package acpinstall

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/logging"
)

var (
	// ErrUnsafeEntry é a entrada que tenta escrever fora do destino, ou que não
	// é arquivo nem diretório. Escrever fora do destino é a forma mais antiga de
	// transformar um download em execução de código, e o desfecho é abortar a
	// instalação inteira em vez de pular a entrada: um archive que tenta isso
	// não é um archive de que se aproveita o resto (D9).
	ErrUnsafeEntry = errors.New("o artefato traz uma entrada que escreveria fora do diretório da instalação")

	// ErrArchiveTooBig é o archive que, aberto, passa do que o app aceita
	// escrever. Um `.zip` de poucos megabytes pode virar gigabytes no disco, e
	// o limite é o que separa instalar de encher o disco de quem clicou.
	ErrArchiveTooBig = errors.New("o conteúdo do artefato passa do que o app aceita escrever")

	// ErrBadArchive é o arquivo que não abre no formato que o nome prometia.
	ErrBadArchive = errors.New("o artefato não pôde ser aberto")
)

const (
	// maxArchiveEntries é o teto de arquivos que um artefato pode trazer.
	// Agentes do catálogo trazem dezenas; o teto existe para o archive de um
	// milhão de arquivos vazios não travar a instalação num laço.
	maxArchiveEntries = 8192

	// maxExtractedBytes é o teto do que se escreve a partir de um artefato.
	maxExtractedBytes = 1 << 30
)

// extractArtifact põe o conteúdo do artefato dentro de `dest`.
//
// `rawName` é o nome que o executável recebe quando o artefato é binário cru —
// ele vem do `cmd` do registro, porque é por esse nome que o agente vai ser
// procurado depois, e gravá-lo com o nome do arquivo da URL deixaria o `cmd`
// apontando para nada.
//
// O arquivo baixado é consumido: ao fim, o que resta em `dest` é o conteúdo, e
// não o pacote de onde ele saiu.
func extractArtifact(ctx context.Context, art artifact, dest, rawName string) error {
	return extractWithBudget(ctx, art, dest, rawName, maxExtractedBytes)
}

// extractWithBudget é a extração com o teto por parâmetro, para o teste poder
// provar a recusa da bomba de descompressão sem escrever um gigabyte.
func extractWithBudget(ctx context.Context, art artifact, dest, rawName string, budget int64) error {
	// Cancelamento vale para todo formato, inclusive o binário cru, que é só um
	// rename: pôr o executável no lugar e devolver sucesso depois de a pessoa
	// ter cancelado deixaria instalado o que ela disse para não instalar.
	if err := ctx.Err(); err != nil {
		return err
	}
	if art.Format == formatRaw {
		// O binário cru não é extraído: ele é o próprio artefato, e o que se
		// faz com ele é pôr no lugar com o nome certo.
		return placeRawBinary(art, dest, rawName)
	}

	var err error
	switch art.Format {
	case formatZip:
		err = extractZip(ctx, art.Path, dest, budget)
	case formatTarGz, formatTarBz2:
		err = extractTar(ctx, art, dest, budget)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedArchive, art.Format)
	}
	if err != nil {
		return err
	}
	// O pacote sai depois de aberto. Ele pode ter dezenas de megabytes, e
	// deixá-lo ao lado do que dele saiu dobraria o espaço que uma instalação
	// ocupa para sempre.
	if err := os.Remove(art.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		logging.Warnf(ctx, component, "não foi possível remover o artefato baixado em %s: %v", art.Path, err)
	}
	return nil
}

// placeRawBinary põe o executável baixado no lugar com o nome pelo qual ele
// será procurado, e com permissão de execução: o arquivo veio de um download, e
// nada garante que o modo dele diga alguma coisa.
func placeRawBinary(art artifact, dest, rawName string) error {
	if art.Bytes == 0 {
		return fmt.Errorf("%w: o artefato veio vazio", ErrBadArchive)
	}
	target, err := resolveEntry(dest, rawName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("não foi possível preparar %s: %w", shownPath(filepath.Dir(target)), causeOf(err))
	}
	if err := os.Rename(art.Path, target); err != nil {
		return fmt.Errorf("não foi possível pôr o executável em %s: %w", shownPath(target), causeOf(err))
	}
	if err := os.Chmod(target, 0o755); err != nil {
		// Sem o bit de execução o arquivo não é o agente, é um arquivo com o
		// nome dele. Deixá-lo ali daria uma instalação que parece pronta e não
		// sobe, então ele sai junto com o erro.
		_ = os.Remove(target)
		return fmt.Errorf("não foi possível dar permissão de execução a %s: %w", shownPath(target), causeOf(err))
	}
	return nil
}

// extractZip abre o `.zip` — 27 dos 90 alvos do catálogo.
func extractZip(ctx context.Context, archivePath, dest string, budget int64) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadArchive, err)
	}
	defer func() { _ = reader.Close() }()

	if len(reader.File) > maxArchiveEntries {
		return fmt.Errorf("%w: %d entradas", ErrArchiveTooBig, len(reader.File))
	}

	var written int64
	for _, entry := range reader.File {
		// Extrair pode demorar, e cancelar tem de valer no meio: quem clicou em
		// cancelar não vai esperar o último arquivo de um pacote de 80 MB.
		if err := ctx.Err(); err != nil {
			return err
		}
		mode := entry.Mode()
		switch {
		// A barra no fim é o que muitos montadores usam para dizer diretório, e
		// nem todos gravam o bit no modo. Sem olhar as duas coisas, um
		// diretório viraria arquivo com o nome dele, e o que deveria cair
		// dentro não teria onde cair.
		case mode.IsDir() || strings.HasSuffix(entry.Name, "/"):
			target, err := resolveEntry(dest, entry.Name)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		case !mode.IsRegular():
			// Link simbólico dentro do archive aponta para onde quem o montou
			// quiser, inclusive para fora daqui, e a guarda de caminho não o
			// alcança — ela vale para o nome da entrada, não para o destino do
			// link. Nenhum agente do catálogo precisa de um, e aceitá-los seria
			// abrir a porta pela qual a guarda existe para não deixar passar.
			return fmt.Errorf("%w: %s não é arquivo nem diretório", ErrUnsafeEntry, acp.SanitizeLabel(entry.Name))
		}

		target, err := resolveFileEntry(dest, entry.Name)
		if err != nil {
			return err
		}
		file, err := entry.Open()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrBadArchive, err)
		}
		n, err := writeEntry(ctx, target, file, mode, budget-written)
		_ = file.Close()
		if err != nil {
			return err
		}
		written += n
	}
	return nil
}

// extractTar abre `.tar.gz` e `.tar.bz2` — 59 dos 90 alvos do catálogo.
func extractTar(ctx context.Context, art artifact, dest string, budget int64) error {
	file, err := os.Open(art.Path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadArchive, err)
	}
	defer func() { _ = file.Close() }()

	var stream io.Reader
	switch art.Format {
	case formatTarGz:
		gz, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrBadArchive, err)
		}
		defer func() { _ = gz.Close() }()
		stream = gz
	default:
		stream = bzip2.NewReader(file)
	}

	reader := tar.NewReader(stream)
	var written int64
	for entries := 0; ; entries++ {
		// O teto é o mesmo do `.zip`, e vale sobre o que se aceita processar: no
		// tar não há lista prévia para contar, então ele é conferido antes de
		// ler cada entrada.
		if entries >= maxArchiveEntries {
			return fmt.Errorf("%w: mais de %d entradas", ErrArchiveTooBig, maxArchiveEntries)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: %v", ErrBadArchive, err)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			target, err := resolveEntry(dest, header.Name)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			target, err := resolveFileEntry(dest, header.Name)
			if err != nil {
				return err
			}
			n, err := writeEntry(ctx, target, reader, header.FileInfo().Mode(), budget-written)
			if err != nil {
				return err
			}
			written += n
		case tar.TypeXGlobalHeader, tar.TypeXHeader:
			// Metadado do formato, não é entrada a escrever.
		default:
			// Link, dispositivo, fifo: pelo mesmo motivo do `.zip`, e aqui com
			// mais razão, porque o `tar` também carrega hardlink.
			return fmt.Errorf("%w: %s não é arquivo nem diretório", ErrUnsafeEntry, acp.SanitizeLabel(header.Name))
		}
	}
}

// writeEntry grava uma entrada, respeitando o que ainda cabe do teto.
//
// O modo do archive não é usado como está: ele pode trazer setuid, setgid e
// sticky, que num arquivo baixado da internet não têm serventia nenhuma e têm
// consequência. O que se aproveita dele é a única informação que importa — se
// aquilo era para ser executável.
func writeEntry(ctx context.Context, target string, src io.Reader, mode fs.FileMode, budget int64) (int64, error) {
	if budget <= 0 {
		return 0, fmt.Errorf("%w: %s", ErrArchiveTooBig, shownPath(filepath.Base(target)))
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, fmt.Errorf("não foi possível preparar %s: %w", shownPath(filepath.Dir(target)), causeOf(err))
	}
	perm := fs.FileMode(0o644)
	if mode&0o111 != 0 {
		perm = 0o755
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return 0, fmt.Errorf("não foi possível criar %s: %w", shownPath(target), causeOf(err))
	}
	written, err := io.Copy(file, io.LimitReader(watchful{ctx: ctx, src: src}, budget+1))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		// A gravação interrompida não deixa arquivo pela metade: ele teria o
		// nome do que o archive prometia e não seria aquilo.
		_ = os.Remove(target)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return written, ctxErr
		}
		return written, fmt.Errorf("não foi possível gravar %s: %w", shownPath(target), causeOf(err))
	}
	if written > budget {
		// O arquivo parcial não fica: ele não é o que o archive trazia, e
		// deixá-lo faria a limpeza da instalação ter de adivinhar o que apagar.
		_ = os.Remove(target)
		return written, fmt.Errorf("%w: %s", ErrArchiveTooBig, shownPath(filepath.Base(target)))
	}
	return written, nil
}

// shownPath é o caminho como ele pode aparecer numa mensagem.
//
// Metade dele é escolha do app e a outra metade vem de dentro do artefato, que
// é o dado mais externo deste caminho. Uma entrada com caractere de controle ou
// com marca de direção faria a frase do erro dizer na tela — e no leitor de
// telas — coisa diferente do que o app escreveu (D9). Uma mensagem de falha é
// justamente onde ninguém pode ser enganado.
func shownPath(path string) string {
	return acp.SanitizeLabel(path)
}

// causeOf tira do erro do sistema de arquivos a parte que interessa, deixando
// para trás o caminho que ele repete.
//
// Sanear só o que o app interpola não bastaria: `os.OpenFile` devolve um
// `*os.PathError`, e o texto dele traz o caminho cru de novo — com o nome que
// veio de dentro do artefato, inteiro, logo depois da versão saneada. O que
// sobra aqui é o motivo, que é o que a mensagem precisa dizer.
//
// A cadeia que importa continua de pé: `errors.Is(err, os.ErrNotExist)` e as
// irmãs dela respondem pelo erro de sistema que fica.
func causeOf(err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return linkErr.Err
	}
	return err
}

// watchful é a leitura que para quando o contexto acaba.
//
// Conferir o contexto só entre entradas não bastaria: um artefato pode ser um
// único arquivo de centenas de megabytes, e nele o laço de entradas passa uma
// vez só. Quem cancelou continuaria esperando o disco encher até o teto.
type watchful struct {
	ctx context.Context
	src io.Reader
}

func (w watchful) Read(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.src.Read(p)
}

// resolveEntry transforma o nome de uma entrada no caminho onde ela pode ser
// escrita, ou recusa.
//
// O nome vem de dentro do artefato, que é o dado mais externo que existe neste
// caminho — mais externo que o índice, porque ninguém do registro revisou o
// conteúdo do `.zip`. A conferência é dupla de propósito: a régua textual
// recusa o que é obviamente hostil, e a comparação com o destino resolvido é a
// que vale, porque é ela que decide onde o arquivo cairia de fato.
func resolveEntry(dest, name string) (string, error) {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return "", fmt.Errorf("%w: entrada sem nome", ErrUnsafeEntry)
	}
	// A contrabarra é separador no Windows e caractere comum de nome em POSIX.
	// Um `..\..\algo` passaria por uma régua que só olha `/` e viraria caminho
	// de verdade ao chegar no Windows, então ela é recusada antes de qualquer
	// interpretação — nenhum archive honesto do catálogo a usa.
	if strings.ContainsRune(clean, '\\') {
		return "", fmt.Errorf("%w: %s", ErrUnsafeEntry, acp.SanitizeLabel(name))
	}
	if path.IsAbs(clean) || filepath.IsAbs(clean) {
		return "", fmt.Errorf("%w: %s", ErrUnsafeEntry, acp.SanitizeLabel(name))
	}
	// Os dois-pontos saem em qualquer posição, e não só no `C:` do começo. No
	// Windows, `agente.exe:carga` não é um arquivo chamado assim: é um fluxo
	// anexado ao `agente.exe` que já foi extraído. O caminho continua dentro do
	// diretório, então a guarda de travessia o deixaria passar — e o que
	// entraria ali é conteúdo que ninguém vê listando o diretório. Nome de
	// arquivo com dois-pontos é ilegal no Windows e não aparece em artefato
	// honesto de nenhum sistema.
	if strings.ContainsRune(clean, ':') {
		return "", fmt.Errorf("%w: %s", ErrUnsafeEntry, acp.SanitizeLabel(name))
	}
	clean = path.Clean(clean)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %s", ErrUnsafeEntry, acp.SanitizeLabel(name))
	}

	target := filepath.Join(dest, filepath.FromSlash(clean))
	if !acp.WithinDir(dest, target) {
		return "", fmt.Errorf("%w: %s", ErrUnsafeEntry, acp.SanitizeLabel(name))
	}
	return target, nil
}

// resolveFileEntry é a resolução para entrada que vira arquivo.
//
// A diferença para a de diretório é o `.`: tarball costuma abrir com a entrada
// `./`, que é o próprio destino e é legítima como diretório. Como arquivo ela
// não é — escrever "o diretório" significaria substituir a instalação por um
// arquivo com o nome dela.
func resolveFileEntry(dest, name string) (string, error) {
	target, err := resolveEntry(dest, name)
	if err != nil {
		return "", err
	}
	if target == filepath.Clean(dest) {
		return "", fmt.Errorf("%w: %s", ErrUnsafeEntry, acp.SanitizeLabel(name))
	}
	return target, nil
}
