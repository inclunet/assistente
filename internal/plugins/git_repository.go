package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/pluginpackage"
	httpclient "assistente/internal/tools/http"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

const gitRevisionPrefix = "git:"
const gitPackedLimit = 32 << 20
const gitTreeLimit = 128 << 20
const gitEntryLimit = 16384

var errGitFormat = errors.New("PLUGIN_REPOSITORY_GIT")

// Keep all Git HTTP calls on the application's session-bound credential and
// network policy. Limit both discovery and pack responses before decoding.
type gitHTTPTransport struct{ client *httpclient.Client }

func (t gitHTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := t.client.Do(req.Context(), req)
	if err != nil {
		return nil, err
	}
	body := res.Body
	defer func() { _ = body.Close() }()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return nil, credentials.ErrCredentialResolution
	}
	if res.StatusCode != http.StatusOK {
		return nil, errGitFormat
	}
	limit := int64(gitPackedLimit)
	if req.Method == http.MethodGet {
		limit = 2 << 20
	}
	if res.ContentLength > limit {
		return nil, pluginpackage.ErrTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, pluginpackage.ErrTooLarge
	}
	res.Body = io.NopCloser(bytes.NewReader(data))
	return res, nil
}

// go-git v5 stores transport failures in Err without implementing Unwrap.
func gitFailure(err error) error {
	var wrapped *plumbing.UnexpectedError
	if errors.As(err, &wrapped) {
		err = wrapped.Err
	}
	var blocked *httpclient.BlockedDestinationError
	switch {
	case errors.As(err, &blocked):
		return errors.New("PLUGIN_REPOSITORY_NETWORK_DENIED")
	case errors.Is(err, credentials.ErrCredentialResolution):
		return errors.New("PLUGIN_REPOSITORY_CREDENTIAL")
	case errors.Is(err, pluginpackage.ErrTooLarge):
		return errors.New("PLUGIN_REPOSITORY_TOO_LARGE")
	default:
		return errGitFormat
	}
}

func readGitRepository(ctx context.Context, client *httpclient.Client, origin string) (*pluginpackage.Archive, string, error) {
	endpoint, err := transport.NewEndpoint(origin)
	if err != nil {
		return nil, "", errGitFormat
	}
	gitClient := githttp.NewClientWithOptions(&http.Client{Transport: gitHTTPTransport{client}}, &githttp.ClientOptions{RedirectPolicy: githttp.NoFollowRedirects})
	session, err := gitClient.NewUploadPackSession(endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = session.Close() }()
	refs, err := session.AdvertisedReferencesContext(ctx)
	if err != nil {
		return nil, "", err
	}
	if refs.Head == nil || !refs.Capabilities.Supports(capability.Shallow) {
		return nil, "", errGitFormat
	}
	request := packp.NewUploadPackRequest()
	if err := request.Capabilities.Set(capability.Shallow); err != nil {
		return nil, "", err
	}
	request.Wants = []plumbing.Hash{*refs.Head}
	request.Depth = packp.DepthCommits(1)
	response, err := session.UploadPack(ctx, request)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = response.Close() }()
	data, err := io.ReadAll(io.LimitReader(response, gitPackedLimit+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > gitPackedLimit {
		return nil, "", pluginpackage.ErrTooLarge
	}
	if err := checkGitPack(ctx, data); err != nil {
		return nil, "", err
	}
	storage := memory.NewStorage()
	if err := packfile.UpdateObjectStorage(storage, bytes.NewReader(data)); err != nil {
		return nil, "", err
	}
	commit, err := object.GetCommit(storage, *refs.Head)
	if err != nil {
		return nil, "", err
	}
	archive, err := gitTreeArchive(ctx, storage, commit.TreeHash)
	return archive, gitRevisionPrefix + refs.Head.String(), err
}

// A delta's pack header bounds its instructions, not the resulting blob.
// Budget both passes before letting the decoder allocate reconstructed objects.
func checkGitPack(ctx context.Context, data []byte) error {
	scanner := packfile.NewScanner(bytes.NewReader(data))
	defer func() { _ = scanner.Close() }()
	_, count, err := scanner.Header()
	if err != nil {
		return err
	}
	if count > gitEntryLimit {
		return pluginpackage.ErrTooLarge
	}
	var total int64
	for range count {
		if err := ctx.Err(); err != nil {
			return err
		}
		h, err := scanner.NextObjectHeader()
		if err != nil {
			return err
		}
		if h.Length < 0 || h.Length > 32<<20 || total+h.Length > gitTreeLimit {
			return pluginpackage.ErrTooLarge
		}
		total += h.Length
		var delta bytes.Buffer
		sink := io.Discard
		if h.Type == plumbing.OFSDeltaObject || h.Type == plumbing.REFDeltaObject {
			sink = &delta
		}
		n, _, err := scanner.NextObject(sink)
		if err != nil {
			return err
		}
		if n != h.Length {
			return errGitFormat
		}
		if sink != io.Discard {
			if _, err = binary.ReadUvarint(&delta); err != nil {
				return err
			}
			size, err := binary.ReadUvarint(&delta)
			if err != nil {
				return err
			}
			if size > 32<<20 || size > uint64(gitTreeLimit-total) {
				return pluginpackage.ErrTooLarge
			}
			total += int64(size)
		}
	}
	_, err = scanner.Checksum()
	return err
}

type gitArchiveWriter struct {
	out       io.Writer
	remaining int
}

func (w *gitArchiveWriter) Write(data []byte) (int, error) {
	if len(data) > w.remaining {
		return 0, pluginpackage.ErrTooLarge
	}
	n, err := w.out.Write(data)
	w.remaining -= n
	return n, err
}

// Materialize the commit in memory with the existing portable archive reader.
// Tree reuse still counts towards the expanded byte and entry budgets.
func gitTreeArchive(ctx context.Context, storage *memory.Storage, root plumbing.Hash) (*pluginpackage.Archive, error) {
	var data bytes.Buffer
	writer := zip.NewWriter(&gitArchiveWriter{out: &data, remaining: gitTreeLimit})
	var total int64
	var entries int
	var walk func(plumbing.Hash, string, int) error
	walk = func(id plumbing.Hash, dir string, depth int) error {
		if depth > 32 {
			return pluginpackage.ErrTooLarge
		}
		tree, err := object.GetTree(storage, id)
		if err != nil {
			return err
		}
		for _, e := range tree.Entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries++
			if entries > gitEntryLimit {
				return pluginpackage.ErrTooLarge
			}
			if !fs.ValidPath(e.Name) || e.Name == "." || strings.ContainsAny(e.Name, "/\\:") {
				return errGitFormat
			}
			if strings.EqualFold(e.Name, ".git") {
				continue
			}
			if len(dir)+len(e.Name)+1 > 4096 {
				return pluginpackage.ErrTooLarge
			}
			name := path.Join(dir, e.Name)
			if e.Mode == filemode.Dir {
				if err := walk(e.Hash, name, depth+1); err != nil {
					return err
				}
				continue
			}
			if e.Mode != filemode.Regular && e.Mode != filemode.Executable {
				return errGitFormat
			}
			blob, err := object.GetBlob(storage, e.Hash)
			if err != nil {
				return err
			}
			if blob.Size < 0 || blob.Size > 32<<20 || total+blob.Size > gitTreeLimit {
				return pluginpackage.ErrTooLarge
			}
			total += blob.Size
			output, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
			if err != nil {
				return err
			}
			reader, err := blob.Reader()
			if err != nil {
				return err
			}
			_, err = io.CopyN(output, reader, blob.Size)
			closeErr := reader.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		}
		return nil
	}
	if err := walk(root, "", 0); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return pluginpackage.OpenArchive(data.Bytes())
}
