package filesystem

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"assistente/internal/questionnaire"
)

func patchArgs(t *testing.T, path string, hunks ...applyPatchHunk) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(applyPatchArgs{Path: path, Hunks: hunks})
	if err != nil {
		t.Fatalf("serializar argumentos: %v", err)
	}
	return encoded
}

func decodePatchResponse(t *testing.T, content string) applyPatchResponse {
	t.Helper()
	var response applyPatchResponse
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		t.Fatalf("resultado não é JSON estruturado: %v\n%s", err, content)
	}
	return response
}

func waitForMutationLockRefs(t *testing.T, path string, want int) {
	t.Helper()
	if resolved, err := resolveForComparison(path); err == nil {
		path = resolved
	}
	key := normalizeForComparison(path)
	deadline := time.Now().Add(time.Second)
	for {
		fileMutationLocks.Lock()
		entry := fileMutationLocks.entries[key]
		refs := 0
		if entry != nil {
			refs = entry.refs
		}
		fileMutationLocks.Unlock()
		if refs >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("lock de %q não alcançou %d referências; atual=%d", path, want, refs)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestApplyPatchContract(t *testing.T) {
	tool := NewApplyPatch(t.TempDir(), nil)
	if tool.Name() != "apply_patch" {
		t.Fatalf("Name() = %q", tool.Name())
	}
	meta := tool.CatalogMetadata()
	if meta.Category != "filesystem" || meta.Class != "edit_files" ||
		meta.Package != "coding_edit" || meta.Risk != "write" {
		t.Fatalf("metadados inesperados: %#v", meta)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("schema inválido: %v", err)
	}
}

func TestApplyPatchAplicaMultiplosHunksComUmaGravacao(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	original := "package main\n\nfunc first() { oldOne() }\n\nfunc second() { oldTwo() }\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	writes := 0
	var committed []bool
	tool := NewApplyPatch(dir, nil, WithApplyPatchWriteObserver(func(gotPath string) func(bool) {
		if gotPath != path {
			t.Errorf("observer path = %q, want %q", gotPath, path)
		}
		writes++
		return func(ok bool) { committed = append(committed, ok) }
	}))

	result, err := tool.Execute(context.Background(), patchArgs(t, "main.go",
		applyPatchHunk{OldString: "func first() { oldOne() }", NewString: "func first() { newOne() }"},
		applyPatchHunk{OldString: "func second() { oldTwo() }", NewString: "func second() {\n\tnewTwo()\n}"},
	))
	if err != nil || result.IsError {
		t.Fatalf("Execute err=%v result=%s", err, result.Content)
	}
	if !result.Structured {
		t.Fatal("resultado deve ser estruturado")
	}
	response := decodePatchResponse(t, result.Content)
	if !response.Applied || response.Hunks != 2 || response.Status != "ok" {
		t.Fatalf("resposta inesperada: %#v", response)
	}
	if writes != 1 || !reflect.DeepEqual(committed, []bool{true}) {
		t.Fatalf("observer writes=%d committed=%v", writes, committed)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	want := "package main\n\nfunc first() { newOne() }\n\nfunc second() {\n\tnewTwo()\n}\n"
	if string(data) != want {
		t.Fatalf("conteúdo:\n%s\nwant:\n%s", data, want)
	}
}

func TestApplyPatchFalhaEmUmHunkSemAlterarArquivo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	original := []byte("alpha\nbeta\ngamma\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewApplyPatch(dir, nil)
	result, err := tool.Execute(context.Background(), patchArgs(t, "doc.txt",
		applyPatchHunk{OldString: "alpha", NewString: "ALPHA"},
		applyPatchHunk{OldString: "não existe", NewString: "delta"},
	))
	if err != nil || !result.IsError || !result.Structured {
		t.Fatalf("Execute err=%v result=%#v", err, result)
	}
	response := decodePatchResponse(t, result.Content)
	if response.Applied || len(response.Errors) != 1 {
		t.Fatalf("resposta inesperada: %#v", response)
	}
	if got := response.Errors[0]; got.Hunk != 2 || got.Code != "context_not_found" {
		t.Fatalf("erro não localizado: %#v", got)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(data, original) {
		t.Fatalf("arquivo mudou apesar da falha: %q", data)
	}
}

func TestApplyPatchReportaContextoAmbiguoComLinhas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte("igual\noutra\nigual\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewApplyPatch(dir, nil).Execute(context.Background(), patchArgs(t, "doc.txt",
		applyPatchHunk{OldString: "igual", NewString: "novo"},
	))
	if err != nil || !result.IsError {
		t.Fatalf("Execute err=%v result=%#v", err, result)
	}
	got := decodePatchResponse(t, result.Content).Errors[0]
	if got.Hunk != 1 || got.Code != "ambiguous_context" ||
		!reflect.DeepEqual(got.CandidateLines, []int{1, 3}) {
		t.Fatalf("erro ambíguo inesperado: %#v", got)
	}
}

func TestApplyPatchContaOcorrenciasSobrepostasComoAmbiguas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte("aaa"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewApplyPatch(dir, nil).Execute(context.Background(), patchArgs(t, "doc.txt",
		applyPatchHunk{OldString: "aa", NewString: "b"},
	))
	if err != nil || !result.IsError {
		t.Fatalf("Execute err=%v result=%#v", err, result)
	}
	got := decodePatchResponse(t, result.Content).Errors[0]
	if got.Code != "ambiguous_context" || got.Occurrences != 2 ||
		!reflect.DeepEqual(got.CandidateLines, []int{1, 1}) {
		t.Fatalf("erro auto-sobreposto inesperado: %#v", got)
	}
}

func TestApplyPatchRejeitaHunksSobrepostos(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	original := []byte("prefix alpha beta suffix\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewApplyPatch(dir, nil).Execute(context.Background(), patchArgs(t, "doc.txt",
		applyPatchHunk{OldString: "alpha beta", NewString: "one"},
		applyPatchHunk{OldString: "beta suffix", NewString: "two"},
	))
	if err != nil || !result.IsError {
		t.Fatalf("Execute err=%v result=%#v", err, result)
	}
	got := decodePatchResponse(t, result.Content).Errors[0]
	if got.Hunk != 2 || got.Code != "overlapping_hunks" || got.ConflictsWith != 1 {
		t.Fatalf("erro de sobreposição inesperado: %#v", got)
	}
	data, _ := os.ReadFile(path)
	if !bytes.Equal(data, original) {
		t.Fatalf("arquivo mudou apesar da sobreposição: %q", data)
	}
}

func TestApplyPatchPreservaBOMCRLFEUnicode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	original := append([]byte{0xEF, 0xBB, 0xBF}, []byte("título\r\nlinha antiga\r\nfim\r\n")...)
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewApplyPatch(dir, nil).Execute(context.Background(), patchArgs(t, "doc.txt",
		applyPatchHunk{OldString: "linha antiga\nfim", NewString: "linha nova — ação\nfim"},
	))
	if err != nil || result.IsError {
		t.Fatalf("Execute err=%v result=%#v", err, result)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	want := append([]byte{0xEF, 0xBB, 0xBF}, []byte("título\r\nlinha nova — ação\r\nfim\r\n")...)
	if !bytes.Equal(data, want) {
		t.Fatalf("bytes = %q, want %q", data, want)
	}
}

type patchMutatingRequester struct {
	path string
}

func (r *patchMutatingRequester) RequestQuestionnaire(
	_ context.Context,
	_ questionnaire.RequestPayload,
) (questionnaire.Response, error) {
	if err := WriteFileBytes(r.path, []byte("mudança concorrente\n"), 0644); err != nil {
		return questionnaire.Response{}, err
	}
	return questionnaire.Response{
		Answers: map[string]any{questionnaire.AnswerActionID: editDecisionApply},
	}, nil
}

func TestApplyPatchDetectaMudancaDuranteConfirmacao(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte("antes\n"), 0644); err != nil {
		t.Fatal(err)
	}
	observerCalls := 0
	tool := NewApplyPatch(
		dir,
		&patchMutatingRequester{path: path},
		WithApplyPatchWriteObserver(func(string) func(bool) {
			observerCalls++
			return nil
		}),
	)

	result, err := tool.Execute(writeConfirmEditorCtx(path), patchArgs(t, "doc.txt",
		applyPatchHunk{OldString: "antes", NewString: "depois"},
	))
	if err != nil || !result.IsError {
		t.Fatalf("Execute err=%v result=%#v", err, result)
	}
	got := decodePatchResponse(t, result.Content).Errors[0]
	if got.Hunk != 0 || got.Code != "stale_file" {
		t.Fatalf("erro stale inesperado: %#v", got)
	}
	if observerCalls != 0 {
		t.Fatalf("observer chamado %d vez(es)", observerCalls)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "mudança concorrente\n" {
		t.Fatalf("mudança concorrente foi sobrescrita: %q", data)
	}
}

func TestApplyPatchRejeicaoMantemResultadoEstruturado(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte("antes\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewApplyPatch(dir, &writeConfirmFakeRequester{cancelled: true}).Execute(
		writeConfirmEditorCtx(path),
		patchArgs(t, "doc.txt", applyPatchHunk{OldString: "antes", NewString: "depois"}),
	)
	if err != nil || !result.IsError || !result.Structured {
		t.Fatalf("Execute err=%v result=%#v", err, result)
	}
	got := decodePatchResponse(t, result.Content).Errors[0]
	if got.Code != "confirmation_not_applied" {
		t.Fatalf("erro de confirmação inesperado: %#v", got)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "antes\n" {
		t.Fatalf("arquivo mudou após rejeição: %q", data)
	}
}

func TestApplyPatchSerializaMutacoesDoMesmoArquivo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte("antes\n"), 0644); err != nil {
		t.Fatal(err)
	}

	firstAtWrite := make(chan struct{})
	releaseFirst := make(chan struct{})
	first := NewApplyPatch(dir, nil, WithApplyPatchWriteObserver(func(string) func(bool) {
		close(firstAtWrite)
		<-releaseFirst
		return nil
	}))
	second := NewApplyPatch(dir, nil)
	type outcome struct {
		result applyPatchResponse
		err    error
	}
	run := func(tool *ApplyPatch, args json.RawMessage, done chan<- outcome) {
		result, err := tool.Execute(context.Background(), args)
		var response applyPatchResponse
		if err == nil {
			err = json.Unmarshal([]byte(result.Content), &response)
		}
		done <- outcome{result: response, err: err}
	}

	firstArgs := patchArgs(t, "doc.txt", applyPatchHunk{OldString: "antes", NewString: "primeiro"})
	secondArgs := patchArgs(t, "doc.txt", applyPatchHunk{OldString: "antes", NewString: "segundo"})
	firstDone := make(chan outcome, 1)
	secondDone := make(chan outcome, 1)
	go run(first, firstArgs, firstDone)
	<-firstAtWrite
	go run(second, secondArgs, secondDone)

	waitForMutationLockRefs(t, path, 2)
	select {
	case result := <-secondDone:
		t.Fatalf("segunda mutação atravessou o lock: %#v", result)
	default:
	}
	close(releaseFirst)
	firstResult := <-firstDone
	secondResult := <-secondDone
	if firstResult.err != nil || !firstResult.result.Applied {
		t.Fatalf("primeira mutação falhou: %#v", firstResult)
	}
	if secondResult.err != nil || secondResult.result.Applied ||
		secondResult.result.Errors[0].Code != "stale_file" {
		t.Fatalf("segunda mutação não observou snapshot novo: %#v", secondResult)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "primeiro\n" {
		t.Fatalf("conteúdo final = %q", data)
	}
}

type patchSymlinkSwapRequester struct {
	link      string
	newTarget string
}

func (r *patchSymlinkSwapRequester) RequestQuestionnaire(
	_ context.Context,
	_ questionnaire.RequestPayload,
) (questionnaire.Response, error) {
	if err := os.Remove(r.link); err != nil {
		return questionnaire.Response{}, err
	}
	if err := os.Symlink(r.newTarget, r.link); err != nil {
		return questionnaire.Response{}, err
	}
	return questionnaire.Response{
		Answers: map[string]any{questionnaire.AnswerActionID: editDecisionApply},
	}, nil
}

func TestApplyPatchNaoSegueSymlinkTrocadoDepoisDaAutorizacao(t *testing.T) {
	dir := t.TempDir()
	approvedTarget := filepath.Join(dir, "approved.txt")
	outsideDir := t.TempDir()
	outsideTarget := filepath.Join(outsideDir, "outside.txt")
	link := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(approvedTarget, []byte("antes\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideTarget, []byte("antes\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(approvedTarget, link); err != nil {
		t.Skipf("symlink indisponível neste ambiente: %v", err)
	}

	tool := NewApplyPatch(dir, &patchSymlinkSwapRequester{link: link, newTarget: outsideTarget})
	result, err := tool.Execute(writeConfirmEditorCtx(link), patchArgs(t, "doc.txt",
		applyPatchHunk{OldString: "antes", NewString: "depois"},
	))
	if err != nil || result.IsError {
		t.Fatalf("Execute err=%v result=%#v", err, result)
	}
	approved, _ := os.ReadFile(approvedTarget)
	outside, _ := os.ReadFile(outsideTarget)
	if string(approved) != "depois\n" {
		t.Fatalf("destino autorizado não foi editado: %q", approved)
	}
	if string(outside) != "antes\n" {
		t.Fatalf("symlink trocado redirecionou a escrita: %q", outside)
	}
}

func TestFileMutationLockUnificaAliasEPathResolvido(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "alias.txt")
	if err := os.WriteFile(target, []byte("conteúdo"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink indisponível neste ambiente: %v", err)
	}

	unlockTarget := lockFileMutation(target)
	acquiredAlias := make(chan func(), 1)
	go func() {
		acquiredAlias <- lockFileMutation(link)
	}()
	waitForMutationLockRefs(t, target, 2)
	select {
	case unlockAlias := <-acquiredAlias:
		unlockAlias()
		unlockTarget()
		t.Fatal("alias adquiriu lock enquanto o destino estava bloqueado")
	default:
	}
	unlockTarget()
	select {
	case unlockAlias := <-acquiredAlias:
		unlockAlias()
	case <-time.After(time.Second):
		t.Fatal("alias não adquiriu lock após liberação do destino")
	}
}

func TestFileMutationLockUnificaHardlinksPelaIdentidade(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	hardlink := filepath.Join(dir, "hardlink.txt")
	if err := os.WriteFile(target, []byte("conteúdo"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(target, hardlink); err != nil {
		t.Skipf("hardlink indisponível neste ambiente: %v", err)
	}

	unlockTarget := lockFileMutation(target)
	acquiredHardlink := make(chan func(), 1)
	go func() {
		acquiredHardlink <- lockFileMutation(hardlink)
	}()
	waitForMutationLockRefs(t, target, 2)
	select {
	case unlockHardlink := <-acquiredHardlink:
		unlockHardlink()
		unlockTarget()
		t.Fatal("hardlink adquiriu lock enquanto o mesmo arquivo estava bloqueado")
	default:
	}
	unlockTarget()
	select {
	case unlockHardlink := <-acquiredHardlink:
		unlockHardlink()
	case <-time.After(time.Second):
		t.Fatal("hardlink não adquiriu lock após liberação")
	}
}
