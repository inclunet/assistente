package acpinstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
)

// artifactUserAgent identifica o app no download do artefato, no mesmo formato
// dos outros clientes HTTP da casa.
const artifactUserAgent = "Assistente/1.0 (ACP Registry)"
// maxArtifactBytes é o teto do que o app aceita baixar de um alvo.
//
// O maior artefato do catálogo de hoje tem dezenas de megabytes. O teto existe
// para o caso em que o outro lado não para de mandar: sem ele, um servidor que
// responde infinitamente encheria o disco de quem só clicou em instalar. É
// folgado o bastante para nenhum agente honesto esbarrar nele.
const maxArtifactBytes = 512 << 20

var (
	// ErrNoPlatformTarget é o agente que não publica alvo para esta máquina.
	// Acontece de verdade: só 7 dos 17 agentes com binário cobrem os seis
	// alvos, e `windows-aarch64` é o mais raro deles.
	ErrNoPlatformTarget = errors.New("este agente não publica artefato para esta plataforma")

	// ErrUnsupportedArchive é o formato que o app não sabe abrir. O registro
	// recusa instalador (`.msi`, `.dmg`, `.deb`, `.pkg`, `.rpm`, `.appimage`),
	// mas o índice é dado externo (D9) e a recusa é refeita aqui: instalar um
	// desses significaria rodar o instalador do sistema, que é outra coisa e
	// pede outra conversa.
	ErrUnsupportedArchive = errors.New("o formato deste artefato não é instalável pelo app")

	// ErrDigestMismatch é o arquivo que chegou diferente do que o registro
	// publicou. É a razão de o digest existir, e o desfecho é apagar o download
	// e dizer o que não bateu (D4).
	ErrDigestMismatch = errors.New("o artefato baixado não bate com o digest publicado")

	// ErrArtifactTooLarge é o download que passou do teto.
	ErrArtifactTooLarge = errors.New("o artefato passa do tamanho que o app aceita baixar")

	// ErrDownload é a falha de rede ou a resposta que não é 200.
	ErrDownload = errors.New("não foi possível baixar o artefato do agente")
)

// Doer é o mínimo que este pacote precisa do cliente HTTP compartilhado do app
// (`internal/tools/http`), que é quem sabe de timeout, proxy e política de rede
// (D9).
type Doer interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
}

// archiveFormat é como o artefato vem embalado.
type archiveFormat string

const (
	// formatRaw é o executável entregue sem embalagem alguma. Quatro dos 90
	// alvos do catálogo são assim.
	formatRaw archiveFormat = "raw"

	formatZip    archiveFormat = "zip"
	formatTarGz  archiveFormat = "tar.gz"
	formatTarBz2 archiveFormat = "tar.bz2"
)

// installerExtensions são os formatos que o app recusa por serem instaladores
// do sistema, e não artefatos que ele possa pôr num diretório seu.
var installerExtensions = []string{".msi", ".dmg", ".deb", ".pkg", ".rpm", ".appimage"}

// PlatformTarget é a chave do alvo desta máquina no índice do registro.
//
// Vazio quer dizer que esta combinação de sistema e arquitetura não é nomeada
// pelo formato do registro — e aí não há alvo a procurar, o que é diferente de
// o agente não publicar um.
func PlatformTarget() string {
	return platformTargetFor(runtime.GOOS, runtime.GOARCH)
}

func platformTargetFor(goos, goarch string) string {
	var arch string
	switch goarch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	default:
		return ""
	}
	switch goos {
	case "windows", "darwin", "linux":
		return goos + "-" + arch
	}
	return ""
}

// formatOf decide como abrir o artefato pelo nome do arquivo na URL.
//
// O nome é o único indício disponível antes de baixar, e ele é do índice, que é
// dado externo. Por isso a decisão é por sufixo conhecido, e o que não é
// reconhecido cai em binário cru em vez de ser adivinhado pelo conteúdo:
// adivinhar abriria a porta para tratar como archive algo que o registro
// descreve como executável.
func formatOf(archiveURL string) (archiveFormat, error) {
	name := strings.ToLower(path.Base(urlPath(archiveURL)))
	switch {
	case strings.HasSuffix(name, ".zip"):
		return formatZip, nil
	case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".tgz"):
		return formatTarGz, nil
	case strings.HasSuffix(name, ".tar.bz2"), strings.HasSuffix(name, ".tbz2"):
		return formatTarBz2, nil
	}
	for _, ext := range installerExtensions {
		if strings.HasSuffix(name, ext) {
			return "", fmt.Errorf("%w: %s", ErrUnsupportedArchive, ext)
		}
	}
	// `.tar` sem compressão e `.gz` de arquivo único não aparecem no catálogo,
	// e tratá-los como binário cru gravaria um arquivo compactado com nome de
	// executável. Melhor recusar dizendo o formato do que instalar algo que não
	// vai rodar.
	for _, ext := range []string{".tar", ".gz", ".bz2", ".xz", ".7z", ".rar"} {
		if strings.HasSuffix(name, ext) {
			return "", fmt.Errorf("%w: %s", ErrUnsupportedArchive, ext)
		}
	}
	return formatRaw, nil
}

// urlPath devolve o caminho da URL sem query nem fragmento, que é onde mora o
// nome do arquivo. Fazer isso com corte de texto, e não com `url.Parse`, é de
// propósito: a URL já passou pelo saneamento do registro, e o que interessa
// aqui é só o sufixo do nome.
func urlPath(raw string) string {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	return raw
}

// artifact é o que o download deixou no disco.
type artifact struct {
	// Path é o arquivo baixado, dentro do diretório de trabalho da instalação.
	Path string

	// SHA256 é o digest do que chegou, sempre calculado. Com digest publicado
	// ele é o que foi conferido; sem, é o observado, que a Fase 5 grava para
	// perceber mudança depois (D4).
	SHA256 string

	// Format é como abrir o arquivo.
	Format archiveFormat

	// Bytes é o tamanho do que chegou.
	Bytes int64
}

// fetchArtifact baixa o alvo para `dest` e confere o digest quando o registro
// publica um.
//
// O digest é calculado durante a transferência, e não numa segunda leitura do
// arquivo: ler de novo do disco custaria o dobro de E/S no maior arquivo que o
// app baixa, e deixaria uma janela entre gravar e conferir em que outra coisa
// poderia mexer no arquivo.
//
// Divergência apaga o download antes de devolver o erro: um artefato que não
// bate não pode ficar no disco esperando alguém executá-lo por engano (D4).
func fetchArtifact(ctx context.Context, client Doer, archiveURL, want, dest string) (artifact, error) {
	format, err := formatOf(archiveURL)
	if err != nil {
		return artifact{}, err
	}
	if client == nil {
		return artifact{}, fmt.Errorf("%w: o cliente HTTP do app não está disponível", ErrDownload)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return artifact{}, fmt.Errorf("%w: %v", ErrDownload, err)
	}
	req.Header.Set("User-Agent", artifactUserAgent)

	resp, err := client.Do(ctx, req)
	if err != nil {
		return artifact{}, downloadError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// Só o código: a linha de status é escrita pelo servidor, e mensagem de
		// erro do app acaba em tela e em anúncio (D9).
		return artifact{}, fmt.Errorf("%w: HTTP %d", ErrDownload, resp.StatusCode)
	}

	art, err := storeArtifact(resp.Body, dest, format, maxArtifactBytes)
	if err != nil {
		return artifact{}, downloadError(ctx, err)
	}
	if want != "" && art.SHA256 != want {
		_ = os.Remove(art.Path)
		return artifact{}, fmt.Errorf("%w: esperava %s e chegou %s", ErrDigestMismatch, want, art.SHA256)
	}
	return art, nil
}

// downloadError distingue o download que falhou do download que foi cancelado.
//
// Quem cancela derruba a requisição, e o que volta do cliente HTTP é um erro de
// transporte. Embrulhá-lo como falha de rede diria a quem clicou em cancelar
// que a rede caiu, e apagaria o `context.Canceled` de que o instalador precisa
// para tratar cancelamento como decisão, e não como defeito.
//
// Só o cancelamento passa. Prazo esgotado é falha de download, e a mesma
// conclusão vale aqui e no marco de desfecho: quem não clicou em nada precisa
// saber que a rede não deu conta, e não ouvir que alguém cancelou.
func downloadError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return ctx.Err()
	}
	if errors.Is(err, ErrArtifactTooLarge) || errors.Is(err, ErrDownload) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrDownload, err)
}

// storeArtifact grava o corpo em disco calculando o digest, recusando o que
// passar de `limit`.
//
// O teto é parâmetro, e não a constante lida direto, para o teste poder provar
// a recusa sem transferir meio gigabyte.
func storeArtifact(body io.Reader, dest string, format archiveFormat, limit int64) (artifact, error) {
	file, err := os.CreateTemp(dest, "artifact-*.download")
	if err != nil {
		return artifact{}, fmt.Errorf("%w: %v", ErrDownload, err)
	}
	name := file.Name()
	digest := sha256.New()

	// O byte de folga é o que permite saber que passou do teto sem ter aceitado
	// o excesso: se o LimitReader entregar o limite inteiro, havia mais.
	written, err := io.Copy(io.MultiWriter(file, digest), io.LimitReader(body, limit+1))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(name)
		return artifact{}, fmt.Errorf("%w: %v", ErrDownload, err)
	}
	if written > limit {
		_ = os.Remove(name)
		// O teto, e não o tamanho: a transferência para no limite mais um byte,
		// então o quanto o artefato tem de verdade não é sabido aqui.
		return artifact{}, fmt.Errorf("%w: o teto é de %d bytes", ErrArtifactTooLarge, limit)
	}

	return artifact{
		Path:   name,
		SHA256: hex.EncodeToString(digest.Sum(nil)),
		Format: format,
		Bytes:  written,
	}, nil
}
