package chat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unsafe"

	"assistente/internal/apidto"
)

func pageReader(snapshot *TimelineSelectionSnapshot) reconciliationReader {
	return func(context.Context, string, []string) (*TimelineSelectionSnapshot, error) { return snapshot, nil }
}
func recoveryChunkKey(chunk apidto.TimelineRecoveryChunk) string {
	return fmt.Sprintf("%d/%d/%s", chunk.Iteration, chunk.Part, chunk.Kind)
}

func TestRecoveryPagesReconstructBoundedUTF8Buffers(t *testing.T) {
	sub, session, execution, reference, snapshot := reconciliationFixture(t)
	text := strings.Repeat("á🙂\x00", 20000)
	reasoning := strings.Repeat("r", recoveryPageTextBytes+13)
	stream := execution.StartStream(42)
	stream.Append(text)
	stream.Reasoning(reasoning, false)
	stream.Cut()
	stream.Append("fim")
	execution.StartStream(-2).Append("primeiro")
	request := apidto.TimelineRecoveryPageRequest{Reference: reference}
	assembled := map[string]string{}
	version := ""
	pages := 0
	for {
		page, err := sub.ReadRecoveryPage(session.ctx, request, pageReader(snapshot))
		if err != nil {
			t.Fatal(err)
		}
		if !page.Found || page.Reference != reference || page.ConversationID != "conversation" {
			t.Fatal("identidade ausente", page)
		}
		if version == "" {
			version = page.ContentVersion
		} else if version != page.ContentVersion {
			t.Fatal("misturou versões")
		}
		raw, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		bytes := 0
		for _, chunk := range page.Chunks {
			key := recoveryChunkKey(chunk)
			if chunk.ByteOffset != len(assembled[key]) {
				t.Fatal("lacuna ou repetição", chunk)
			}
			assembled[key] += chunk.Text
			bytes += len(chunk.Text)
		}
		if bytes > recoveryPageTextBytes || len(page.Chunks) > recoveryPageVisits || len(raw) > 512*1024 {
			t.Fatal("cota excedida", bytes, len(page.Chunks), len(raw))
		}
		pages++
		if page.NextCursor == "" {
			break
		}
		if page.NextCursor == request.Cursor || pages > 20 {
			t.Fatal("não avançou")
		}
		request.Cursor = page.NextCursor
	}
	if pages < 4 || assembled["-2/0/text"] != "primeiro" || assembled["42/0/text"] != text || assembled["42/0/reasoning"] != reasoning || assembled["42/1/text"] != "fim" {
		t.Fatal("conteúdo divergente", pages)
	}
}

func TestRecoveryPagesBoundEmptyTraversalAndDistinguishMissing(t *testing.T) {
	sub, session, execution, reference, snapshot := reconciliationFixture(t)
	request := apidto.TimelineRecoveryPageRequest{Reference: reference}
	empty, err := sub.ReadRecoveryPage(session.ctx, request, pageReader(snapshot))
	if err != nil || !empty.Found || empty.NextCursor != "" || len(empty.Chunks) != 0 || empty.ContentVersion == "" {
		t.Fatal("execução vazia", empty, err)
	}
	stream := execution.StartStream(0)
	for range 1000 {
		stream.Cut()
	}
	first, err := sub.ReadRecoveryPage(session.ctx, request, pageReader(snapshot))
	if err != nil || len(first.Chunks) != 0 || first.NextCursor == "" {
		t.Fatal("página vazia deve avançar", first, err)
	}
	cursor, err := decodeRecoveryCursor(first.NextCursor)
	if err != nil || cursor.Position.Part != recoveryPageVisits/2 {
		t.Fatal("varreu partes além da cota", cursor, err)
	}
	// Cada iteração vazia também consome trabalho limitado, sem ordenar todo o mapa.
	for iteration := 1; iteration < 300; iteration++ {
		execution.StartStream(iteration)
	}
	request.Cursor = ""
	for pages := 0; pages < 30; pages++ {
		page, readErr := sub.ReadRecoveryPage(session.ctx, request, pageReader(snapshot))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if page.NextCursor == "" {
			break
		}
		if page.NextCursor == request.Cursor || pages == 29 {
			t.Fatal("leitura vazia não terminou")
		}
		request.Cursor = page.NextCursor
	}
	request.Cursor = ""
	request.Reference.ProjectionID = "recreated"
	missing, err := sub.ReadRecoveryPage(session.ctx, request, pageReader(snapshot))
	if err != nil || missing.Found || len(missing.Chunks) != 0 || missing.ContentVersion != "" || missing.NextCursor != "" {
		t.Fatal("ausência virou vazio confirmado", missing, err)
	}
}

func TestRecoveryPageMutationsInvalidateOnlyTheirExecution(t *testing.T) {
	for _, mutation := range []string{"append", "initial", "reasoning", "replace-reasoning", "cut", "seal", "restart", "new-iteration", "other-turn", "retry", "import", "commit", "terminal"} {
		t.Run(mutation, func(t *testing.T) {
			sub, session, execution, reference, snapshot := reconciliationFixture(t)
			text := strings.Repeat("x", recoveryPageTextBytes+12)
			stream := execution.StartStream(0)
			stream.Append(text)
			request := apidto.TimelineRecoveryPageRequest{Reference: reference}
			first, err := sub.ReadRecoveryPage(session.ctx, request, pageReader(snapshot))
			if err != nil || first.NextCursor == "" {
				t.Fatal(first, err)
			}
			switch mutation {
			case "append":
				stream.Append("z")
			case "initial":
				stream.Initial("new")
			case "reasoning":
				stream.Reasoning("r", false)
			case "replace-reasoning":
				stream.Reasoning("r", true)
			case "cut":
				stream.Cut()
			case "seal":
				stream.Seal(text, "")
			case "restart":
				execution.StartStream(0)
			case "new-iteration":
				execution.StartStream(1)
			case "commit":
				finishTestProjection(execution, "completed", "", true, true)
			case "terminal":
				finishTestProjection(execution, "completed", "", true, false)
			case "other-turn":
				other, beginErr := beginTestProjection(sub.owner, session.ctx, "conversation", "other", "other-execution")
				if beginErr != nil {
					t.Fatal(beginErr)
				}
				other.StartStream(0).Append("outro")
				snapshot.Version.Revision++
			case "retry":
				_, beginErr := beginTestProjection(sub.owner, session.ctx, "conversation", "question", "retry")
				if beginErr != nil {
					t.Fatal(beginErr)
				}
				id := "retry"
				snapshot.Turns["question"].ActiveExecutionID = &id
			case "import":
				snapshot.Version.Incarnation = "new-incarnation"
			}
			request.Cursor = first.NextCursor
			page, err := sub.ReadRecoveryPage(session.ctx, request, pageReader(snapshot))
			stable := mutation == "other-turn" || mutation == "retry" || mutation == "import" || mutation == "commit" || mutation == "terminal"
			if stable {
				if err != nil || !page.Found || page.ContentVersion != first.ContentVersion {
					t.Fatal("invalidou bytes preservados", page, err)
				}
			} else if !errors.Is(err, ErrRecoveryCursorStale) || page != nil {
				t.Fatal("cursor antigo aceito", page, err)
			}
		})
	}
}

func TestRecoveryPageRejectsInvalidCursorsAndBounds(t *testing.T) {
	sub, session, execution, reference, snapshot := reconciliationFixture(t)
	execution.StartStream(0).Append(strings.Repeat("x", recoveryPageTextBytes+10))
	request := apidto.TimelineRecoveryPageRequest{Reference: reference}
	first, err := sub.ReadRecoveryPage(session.ctx, request, pageReader(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"garbage", "format", "conversation", "reference", "negative", "stream", "part", "field", "offset", "large"} {
		t.Run(invalid, func(t *testing.T) {
			cursor, decodeErr := decodeRecoveryCursor(first.NextCursor)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			switch invalid {
			case "format":
				cursor.Format++
			case "conversation":
				cursor.ConversationID = "other"
			case "reference":
				cursor.Reference.ExecutionID = "other"
			case "negative":
				cursor.Position.Offset = -1
			case "stream":
				cursor.Position.Stream = 100
			case "part":
				cursor.Position.Part = 100
			case "field":
				cursor.Position.Field = 2
			case "offset":
				cursor.Position.Offset = 1 << 30
			}
			raw, marshalErr := json.Marshal(cursor)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			bad := request
			bad.Cursor = base64.RawURLEncoding.EncodeToString(raw)
			if invalid == "garbage" {
				bad.Cursor = "!"
			}
			if invalid == "large" {
				bad.Cursor = strings.Repeat("a", recoveryCursorBytes+1)
			}
			page, readErr := sub.ReadRecoveryPage(session.ctx, bad, pageReader(snapshot))
			if !errors.Is(readErr, ErrRecoveryCursorInvalid) || page != nil {
				t.Fatal("cursor inválido aceito", page, readErr)
			}
		})
	}
	request.Reference.ExecutionID = strings.Repeat("x", recoveryIdentityBytes+1)
	if page, readErr := sub.ReadRecoveryPage(session.ctx, request, pageReader(snapshot)); !errors.Is(readErr, ErrTimelineSourceLimit) || page != nil {
		t.Fatal("identidade excessiva aceita", page, readErr)
	}
}

func TestRecoveryPageCopiesOnlyUTF8Slices(t *testing.T) {
	sub, session, execution, reference, snapshot := reconciliationFixture(t)
	stream := execution.StartStream(0)
	stream.Append(strings.Repeat("a", recoveryPageTextBytes-1) + "🙂restante")
	request := apidto.TimelineRecoveryPageRequest{Reference: reference}
	first, err := sub.ReadRecoveryPage(session.ctx, request, pageReader(snapshot))
	if err != nil || len(first.Chunks) != 1 || len(first.Chunks[0].Text) != recoveryPageTextBytes-1 {
		t.Fatal("cortou rune", first, err)
	}
	source := sub.conversation.turns["question"].streams[0].parts[0].text.String()
	if unsafe.StringData(first.Chunks[0].Text) == unsafe.StringData(source) {
		t.Fatal("página reteve backing buffer")
	}
	request.Cursor = first.NextCursor
	next, err := sub.ReadRecoveryPage(session.ctx, request, pageReader(snapshot))
	if err != nil || next.Chunks[0].Text != "🙂restante" {
		t.Fatal(next, err)
	}
	stream = execution.StartStream(0)
	stream.Append(string([]byte{0xf0, 0x9f}))
	request.Cursor = ""
	if page, readErr := sub.ReadRecoveryPage(session.ctx, request, pageReader(snapshot)); !errors.Is(readErr, ErrRecoveryEncoding) || page != nil {
		t.Fatal("substituiu UTF-8 incompleto", page, readErr)
	}
	stream.Append(string([]byte{0x99, 0x82}))
	complete, err := sub.ReadRecoveryPage(session.ctx, request, pageReader(snapshot))
	if err != nil || complete.Chunks[0].Text != "🙂" {
		t.Fatal(complete, err)
	}
}

func TestRecoveryPageAuthorizationAndCancellationDoNotRevealMissing(t *testing.T) {
	for _, failure := range []string{"revoked", "storage", "cancel", "closed"} {
		t.Run(failure, func(t *testing.T) {
			sub, session, _, reference, snapshot := reconciliationFixture(t)
			reference.ProjectionID = "missing"
			ctx, cancel := context.WithCancel(session.ctx)
			defer cancel()
			if failure == "closed" {
				sub.Close()
			}
			reader := reconciliationReader(func(context.Context, string, []string) (*TimelineSelectionSnapshot, error) {
				switch failure {
				case "revoked":
					session.denied = true
				case "storage":
					return nil, errors.New("storage")
				case "cancel":
					cancel()
				}
				return snapshot, nil
			})
			page, err := sub.ReadRecoveryPage(ctx, apidto.TimelineRecoveryPageRequest{Reference: reference}, reader)
			if err == nil || page != nil {
				t.Fatal("falha revelou ausência", page, err)
			}
		})
	}
}
