package acpinstall

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// entradaZip é uma entrada a montar no `.zip` de teste.
type entradaZip struct {
	nome     string
	conteudo string
	modo     fs.FileMode
}

func zipDeTeste(t *testing.T, dir string, entradas []entradaZip) artifact {
	t.Helper()
	return artefatoEmDisco(t, dir, zipDeTesteBytes(t, entradas), formatZip)
}

// zipDeTesteBytes é o mesmo zip antes de tocar o disco, para o teste que o serve
// como corpo de download em vez de abri-lo de um arquivo.
func zipDeTesteBytes(t *testing.T, entradas []entradaZip) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, entrada := range entradas {
		header := &zip.FileHeader{Name: entrada.nome, Method: zip.Deflate}
		if entrada.modo != 0 {
			header.SetMode(entrada.modo)
		}
		file, err := w.CreateHeader(header)
		if err != nil {
			t.Fatalf("erro ao montar o zip: %v", err)
		}
		if _, err := file.Write([]byte(entrada.conteudo)); err != nil {
			t.Fatalf("erro ao escrever no zip: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("erro ao fechar o zip: %v", err)
	}
	return buf.Bytes()
}

// entradaTar é uma entrada a montar no `.tar.gz` de teste.
type entradaTar struct {
	nome     string
	conteudo string
	tipo     byte
	modo     int64
	link     string
}

func tarGzDeTeste(t *testing.T, dir string, entradas []entradaTar) artifact {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	w := tar.NewWriter(gz)
	for _, entrada := range entradas {
		tipo := entrada.tipo
		if tipo == 0 {
			tipo = tar.TypeReg
		}
		modo := entrada.modo
		if modo == 0 {
			modo = 0o644
		}
		header := &tar.Header{
			Name:     entrada.nome,
			Typeflag: tipo,
			Mode:     modo,
			Size:     int64(len(entrada.conteudo)),
			Linkname: entrada.link,
		}
		if tipo != tar.TypeReg {
			header.Size = 0
		}
		if err := w.WriteHeader(header); err != nil {
			t.Fatalf("erro ao montar o tar: %v", err)
		}
		if tipo == tar.TypeReg {
			if _, err := w.Write([]byte(entrada.conteudo)); err != nil {
				t.Fatalf("erro ao escrever no tar: %v", err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("erro ao fechar o tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("erro ao fechar o gzip: %v", err)
	}
	return artefatoEmDisco(t, dir, buf.Bytes(), formatTarGz)
}

func artefatoEmDisco(t *testing.T, dir string, data []byte, format archiveFormat) artifact {
	t.Helper()
	path := filepath.Join(dir, "artefato")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("erro ao gravar o artefato: %v", err)
	}
	return artifact{Path: path, Format: format, Bytes: int64(len(data)), SHA256: digestDe(data)}
}

func TestOZipDoAgenteEExtraidoComOBitDeExecucao(t *testing.T) {
	origem, dest := t.TempDir(), t.TempDir()
	art := zipDeTeste(t, origem, []entradaZip{
		{nome: "dist/", modo: fs.ModeDir | 0o755},
		{nome: "dist/agente", conteudo: "#!/bin/sh\n", modo: 0o755},
		{nome: "dist/LEIAME.md", conteudo: "documentação"},
	})

	if err := extractArtifact(context.Background(), art, dest, ""); err != nil {
		t.Fatalf("esperava extração, veio %v", err)
	}
	if conteudo, err := os.ReadFile(filepath.Join(dest, "dist", "agente")); err != nil || string(conteudo) != "#!/bin/sh\n" {
		t.Fatalf("o executável não foi extraído: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dest, "dist", "agente"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&0o111 == 0 {
			t.Errorf("o executável saiu sem permissão de execução: %v", info.Mode())
		}
		doc, err := os.Stat(filepath.Join(dest, "dist", "LEIAME.md"))
		if err != nil {
			t.Fatal(err)
		}
		if doc.Mode()&0o111 != 0 {
			t.Errorf("a documentação saiu executável: %v", doc.Mode())
		}
	}
}

func TestOZipQueEscreveriaForaDaInstalacaoERecusado(t *testing.T) {
	// A travessia de caminho é a forma mais antiga de transformar um download
	// em execução de código, e a recusa vale para as várias formas de escrevê-la.
	for _, nome := range []string{
		"../fora.txt",
		"dist/../../fora.txt",
		"/etc/cron.d/agente",
		`..\..\fora.txt`,
		`C:\Windows\System32\agente.exe`,
		// No Windows isto não é um arquivo com esse nome: é um fluxo anexado ao
		// executável que já foi extraído, e ninguém o vê listando o diretório.
		"agente.exe:carga",
		"C:agente.exe",
	} {
		origem, dest := t.TempDir(), t.TempDir()
		art := zipDeTeste(t, origem, []entradaZip{{nome: nome, conteudo: "carga"}})

		err := extractArtifact(context.Background(), art, dest, "")
		if !errors.Is(err, ErrUnsafeEntry) {
			t.Errorf("%s: esperava recusa de entrada, veio %v", nome, err)
		}
	}
}

func TestOZipComLinkSimbolicoERecusado(t *testing.T) {
	// A guarda de caminho vale para o nome da entrada, não para o destino do
	// link: um link aponta para onde quem montou o archive quiser.
	origem, dest := t.TempDir(), t.TempDir()
	art := zipDeTeste(t, origem, []entradaZip{
		{nome: "dist/agente", conteudo: "#!/bin/sh\n", modo: 0o755},
		{nome: "dist/atalho", conteudo: "/etc/passwd", modo: fs.ModeSymlink | 0o777},
	})

	if err := extractArtifact(context.Background(), art, dest, ""); !errors.Is(err, ErrUnsafeEntry) {
		t.Fatalf("esperava recusa de entrada, veio %v", err)
	}
}

func TestOZipComEntradaCorrompidaNaoViraArquivoNoDisco(t *testing.T) {
	// O `.zip` guarda o CRC de cada entrada, e ele é conferido no fechamento da
	// leitura. Para o artefato que não publica digest, essa é a única
	// conferência de conteúdo que existe.
	origem, dest := t.TempDir(), t.TempDir()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	file, err := w.CreateHeader(&zip.FileHeader{Name: "agente", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("conteudo intacto")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	// Sem compressão, trocar um byte do conteúdo deixa o CRC declarado no lugar
	// e o dado diferente — que é como um artefato chega truncado ou adulterado.
	bruto := buf.Bytes()
	i := bytes.Index(bruto, []byte("conteudo intacto"))
	if i < 0 {
		t.Fatal("não achei o conteúdo no zip montado")
	}
	bruto[i] = 'C'
	art := artefatoEmDisco(t, origem, bruto, formatZip)

	if err := extractArtifact(context.Background(), art, dest, ""); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("esperava recusa do artefato, veio %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "agente")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a entrada corrompida ficou no disco")
	}
}

func TestOTarGzDoAgenteEExtraido(t *testing.T) {
	origem, dest := t.TempDir(), t.TempDir()
	art := tarGzDeTeste(t, origem, []entradaTar{
		{nome: "pacote", tipo: tar.TypeDir, modo: 0o755},
		{nome: "pacote/agente", conteudo: "binário", modo: 0o755},
	})

	if err := extractArtifact(context.Background(), art, dest, ""); err != nil {
		t.Fatalf("esperava extração, veio %v", err)
	}
	if conteudo, err := os.ReadFile(filepath.Join(dest, "pacote", "agente")); err != nil || string(conteudo) != "binário" {
		t.Fatalf("o executável não foi extraído: %v", err)
	}
}

func TestOTarComEntradaQueNaoEArquivoNemDiretorioERecusado(t *testing.T) {
	for _, entrada := range []entradaTar{
		{nome: "pacote/atalho", tipo: tar.TypeSymlink, link: "/etc/passwd"},
		{nome: "pacote/duro", tipo: tar.TypeLink, link: "/etc/shadow"},
		{nome: "pacote/disco", tipo: tar.TypeBlock},
		{nome: "../fora", tipo: tar.TypeReg, conteudo: "carga"},
	} {
		origem, dest := t.TempDir(), t.TempDir()
		art := tarGzDeTeste(t, origem, []entradaTar{entrada})

		if err := extractArtifact(context.Background(), art, dest, ""); !errors.Is(err, ErrUnsafeEntry) {
			t.Errorf("%s: esperava recusa de entrada, veio %v", entrada.nome, err)
		}
	}
}

func TestOArquivoQueSePropoeAEncherODiscoERecusado(t *testing.T) {
	origem, dest := t.TempDir(), t.TempDir()
	// O nome do arquivo promete pouco e o conteúdo é o que vier: é assim que
	// uma bomba de descompressão funciona. Aqui o teto é o que a corta.
	grande := bytes.Repeat([]byte("a"), 4096)
	art := tarGzDeTeste(t, origem, []entradaTar{{nome: "pacote/grande", conteudo: string(grande)}})

	err := extractWithBudget(context.Background(), art, dest, "", 1024)
	if !errors.Is(err, ErrArchiveTooBig) {
		t.Fatalf("esperava recusa por tamanho, veio %v", err)
	}
}

func TestONomeDaEntradaNaoEscreveNaMensagemDeErro(t *testing.T) {
	// O nome vem de dentro do artefato, e a mensagem vai para a tela e para o
	// leitor de telas. Uma quebra de linha ou uma marca de direção no nome
	// escreveria ali uma frase que o app não escreveu.
	origem, dest := t.TempDir(), t.TempDir()
	// A marca de direção vale nos três sistemas como nome de arquivo, o que
	// deixa o teste medindo o saneamento, e não o que cada um aceita gravar.
	art := tarGzDeTeste(t, origem, []entradaTar{
		{nome: "pacote/agente\u202Eexe", conteudo: "carga além do teto"},
	})

	err := extractWithBudget(context.Background(), art, dest, "", 4)
	if !errors.Is(err, ErrArchiveTooBig) {
		t.Fatalf("esperava recusa por tamanho, veio %v", err)
	}
	if strings.ContainsRune(err.Error(), '\u202E') {
		t.Errorf("a mensagem carregou a marca de direção do nome da entrada: %q", err.Error())
	}
}

func TestOErroDoSistemaNaoRepeteONomeCruDaEntrada(t *testing.T) {
	// `os.OpenFile` devolve um `*os.PathError`, e o texto dele traz o caminho
	// de novo — cru, logo depois da versão saneada. Sanear só o que o app
	// interpola deixaria o nome passar pela porta de trás.
	dest := t.TempDir()
	// Um diretório onde o teste manda criar um arquivo: o sistema recusa, e é
	// a recusa dele que se quer ver na mensagem.
	alvo := filepath.Join(dest, "agente\u202Eexe")
	if err := os.MkdirAll(alvo, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := writeEntry(context.Background(), alvo, strings.NewReader("carga"), 0o644, 1024)
	if err == nil {
		t.Fatal("esperava falha ao gravar por cima de um diretório")
	}
	if strings.ContainsRune(err.Error(), '\u202E') {
		t.Errorf("a mensagem carregou o caminho cru do erro do sistema: %q", err.Error())
	}
}

func TestOBinarioCruRecebeONomePeloQualEleSeraProcurado(t *testing.T) {
	// O `cmd` do registro diz `./opencode` enquanto a URL entrega
	// `opencode-linux-arm64`: gravar com o nome da URL deixaria o `cmd`
	// apontando para nada.
	dir, dest := t.TempDir(), t.TempDir()
	art := artefatoEmDisco(t, dir, []byte("ELF..."), formatRaw)

	if err := extractArtifact(context.Background(), art, dest, "opencode"); err != nil {
		t.Fatalf("esperava sucesso, veio %v", err)
	}
	info, err := os.Stat(filepath.Join(dest, "opencode"))
	if err != nil {
		t.Fatalf("o executável não foi posto no lugar: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		t.Errorf("o executável saiu sem permissão de execução: %v", info.Mode())
	}
}

func TestOExecutavelChegaAoLugarQuandoORenameNaoServe(t *testing.T) {
	// O rename não atravessa sistema de arquivos, e basta o diretório de dados
	// do app estar em outro volume para o binário cru deixar de instalar com o
	// download intacto na mão. Simular volumes num teste não é portável, então
	// o que se prova aqui é o caminho lento em si.
	origem, destino := t.TempDir(), t.TempDir()
	baixado := filepath.Join(origem, "baixado")
	if err := os.WriteFile(baixado, []byte("ELF..."), 0o600); err != nil {
		t.Fatal(err)
	}
	alvo := filepath.Join(destino, "agente")

	if err := copyFile(context.Background(), baixado, alvo); err != nil {
		t.Fatalf("esperava cópia, veio %v", err)
	}
	if conteudo, err := os.ReadFile(alvo); err != nil || string(conteudo) != "ELF..." {
		t.Fatalf("o executável copiado não é o que foi baixado: %v", err)
	}

	// E o caminho lento é onde cancelar mais precisa valer. A cópia que não
	// terminou também não pode ter levado junto o que estava no destino: numa
	// atualização, o que está ali é o agente que funcionava.
	if err := os.WriteFile(alvo, []byte("o que funcionava"), 0o755); err != nil {
		t.Fatal(err)
	}
	cancelado, cancel := context.WithCancel(context.Background())
	cancel()

	if err := copyFile(cancelado, baixado, alvo); !errors.Is(err, context.Canceled) {
		t.Errorf("esperava cancelamento, veio %v", err)
	}
	if conteudo, err := os.ReadFile(alvo); err != nil || string(conteudo) != "o que funcionava" {
		t.Errorf("a cópia interrompida mexeu no que já estava no lugar: %q, %v", conteudo, err)
	}
	if restou, _ := os.ReadDir(destino); len(restou) != 1 {
		t.Errorf("a cópia interrompida deixou resíduo em %s: %v", destino, nomesDe(restou))
	}
}

func TestOBinarioCruComNomeQueSaiDaInstalacaoERecusado(t *testing.T) {
	dir, dest := t.TempDir(), t.TempDir()
	art := artefatoEmDisco(t, dir, []byte("ELF..."), formatRaw)

	if err := extractArtifact(context.Background(), art, dest, "../opencode"); !errors.Is(err, ErrUnsafeEntry) {
		t.Fatalf("esperava recusa de entrada, veio %v", err)
	}
}

func TestOTarballQueAbreComPontoBarraEExtraidoNormalmente(t *testing.T) {
	// `tar czf` grava a entrada `./` quando se empacota o diretório corrente, e
	// ela é o próprio destino. Recusá-la deixaria de fora artefato honesto.
	origem, dest := t.TempDir(), t.TempDir()
	art := tarGzDeTeste(t, origem, []entradaTar{
		{nome: "./", tipo: tar.TypeDir, modo: 0o755},
		{nome: "./agente", conteudo: "binário", modo: 0o755},
	})

	if err := extractArtifact(context.Background(), art, dest, ""); err != nil {
		t.Fatalf("esperava extração, veio %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "agente")); err != nil {
		t.Fatalf("o executável não foi extraído: %v", err)
	}
}

func TestAEntradaQueSubstituiriaODiretorioDaInstalacaoERecusada(t *testing.T) {
	origem, dest := t.TempDir(), t.TempDir()
	art := tarGzDeTeste(t, origem, []entradaTar{{nome: ".", tipo: tar.TypeReg, conteudo: "carga"}})

	if err := extractArtifact(context.Background(), art, dest, ""); !errors.Is(err, ErrUnsafeEntry) {
		t.Fatalf("esperava recusa de entrada, veio %v", err)
	}
}

func TestOPacoteSaiDoDiscoDepoisDeAberto(t *testing.T) {
	// O `.zip` do Cursor tem dezenas de megabytes; guardá-lo ao lado do que
	// dele saiu dobraria o espaço da instalação para sempre.
	dest := t.TempDir()
	art := zipDeTeste(t, dest, []entradaZip{{nome: "agente", conteudo: "binário", modo: 0o755}})

	if err := extractArtifact(context.Background(), art, dest, ""); err != nil {
		t.Fatalf("esperava extração, veio %v", err)
	}
	if _, err := os.Stat(art.Path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("o pacote continuou em %s depois de aberto", art.Path)
	}
}

func TestCancelarInterrompeQualquerFormato(t *testing.T) {
	// Inclusive o binário cru, que é só um rename: pôr o executável no lugar
	// depois do cancelamento deixaria instalado o que a pessoa recusou.
	origem, dest := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	artefatos := map[string]artifact{
		"tar.gz": tarGzDeTeste(t, origem, []entradaTar{{nome: "pacote/agente", conteudo: "binário", modo: 0o755}}),
		"cru":    artefatoEmDisco(t, t.TempDir(), []byte("ELF..."), formatRaw),
	}
	for formato, art := range artefatos {
		if err := extractArtifact(ctx, art, dest, "agente"); !errors.Is(err, context.Canceled) {
			t.Errorf("%s: esperava cancelamento, veio %v", formato, err)
		}
	}
	if restou, _ := os.ReadDir(dest); len(restou) != 0 {
		t.Errorf("o cancelamento deixou arquivo em %s: %v", dest, nomesDe(restou))
	}
}

func TestCancelarNoMeioDeUmaEntradaGrandeInterrompeAGravacao(t *testing.T) {
	// Um artefato pode ser um arquivo único de centenas de megabytes, e nele o
	// laço de entradas passa uma vez só: sem olhar o contexto durante a cópia,
	// quem cancelou esperaria o arquivo inteiro.
	dest := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	fonte := &leitorQueCancelaNoMeio{cancel: cancel}
	alvo := filepath.Join(dest, "agente")

	_, err := writeEntry(ctx, alvo, fonte, 0o755, maxExtractedBytes)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("esperava cancelamento, veio %v", err)
	}
	if _, err := os.Stat(alvo); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a gravação interrompida deixou %s pela metade", alvo)
	}
}

type leitorQueCancelaNoMeio struct {
	cancel context.CancelFunc
	lidos  int
}

func (l *leitorQueCancelaNoMeio) Read(p []byte) (int, error) {
	l.lidos++
	if l.lidos == 2 {
		l.cancel()
	}
	p[0] = 'a'
	return 1, nil
}

func TestOArtefatoQueNaoAbreNoFormatoPrometidoFalhaDizendoIsso(t *testing.T) {
	dir, dest := t.TempDir(), t.TempDir()
	art := artefatoEmDisco(t, dir, []byte("isto não é um zip"), formatZip)

	if err := extractArtifact(context.Background(), art, dest, ""); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("esperava falha de leitura do artefato, veio %v", err)
	}
}
