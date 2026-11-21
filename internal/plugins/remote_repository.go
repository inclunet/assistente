package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/plugincatalog"
	"assistente/internal/pluginpackage"
)

// RemoteClient receives the exact authority selected for this repository.
// An empty credential reference denotes an anonymous request.
type RemoteClient func(reference string) *http.Client

func RepositoryHTTPClient(manager *credentials.Manager, reference string) *http.Client {
	if reference == "" {
		return &http.Client{}
	}
	if manager == nil {
		return nil
	}
	transport := credentials.NewCredentialTransportWithMode(manager, reference, credentials.AuthRequired)
	transport.RequireCredential = true
	return &http.Client{Transport: transport}
}

func (i *Inventory) WithRemoteClient(create RemoteClient) *Inventory {
	copy := *i
	copy.remote = create
	return &copy
}

func remoteURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("PLUGIN_REPOSITORY_URL")
	}
	u.Host = strings.ToLower(u.Host)
	return u, nil
}

func (i *Inventory) download(ctx context.Context, origin, reference string) (*pluginpackage.Archive, string, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, "", err
	}
	u, err := remoteURL(origin)
	if err != nil || (reference != "" && reference != u.Host) || i.remote == nil {
		return nil, "", errors.New("PLUGIN_REPOSITORY_URL")
	}
	client := i.remote(reference)
	if client == nil {
		return nil, "", errors.New("PLUGIN_REPOSITORY_DOWNLOAD")
	}
	bounded := *client
	bounded.Timeout = 45 * time.Second
	bounded.CheckRedirect = func(next *http.Request, previous []*http.Request) error {
		if next.URL.Scheme != u.Scheme || !strings.EqualFold(next.URL.Host, u.Host) || next.URL.User != nil || len(previous) >= 5 {
			return http.ErrUseLastResponse
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", errors.New("PLUGIN_REPOSITORY_URL")
	}
	res, err := bounded.Do(req)
	if err != nil {
		return nil, "", errors.New("PLUGIN_REPOSITORY_DOWNLOAD")
	}
	defer func() { _ = res.Body.Close() }()
	const maxDownload = 32 << 20
	if res.StatusCode != http.StatusOK || res.ContentLength > maxDownload {
		return nil, "", errors.New("PLUGIN_REPOSITORY_DOWNLOAD")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxDownload+1))
	if err != nil || len(data) > maxDownload {
		return nil, "", errors.New("PLUGIN_REPOSITORY_DOWNLOAD")
	}
	archive, err := pluginpackage.OpenArchive(data)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(data)
	return archive, hex.EncodeToString(sum[:]), nil
}

func (i *Inventory) SaveRemoteRepository(ctx context.Context, raw string, authenticated bool, id, expected string, admit commandsecurity.Admission) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if admit == nil {
		return database.ErrUserScopeRequired
	}
	if err := admit.Check(); err != nil {
		return err
	}
	u, err := remoteURL(raw)
	if err != nil {
		return err
	}
	ref := ""
	if authenticated {
		ref = u.Host
	}
	archive, digest, err := i.download(ctx, u.String(), ref)
	if err != nil {
		return err
	}
	indices, err := plugincatalog.ReadIndices(func(name string) ([]byte, error) { return archive.Read(archive.Wrapper()+name, 10<<20) })
	if err != nil {
		return err
	}
	return i.publishRepository(ctx, owner, u.String(), digest, ref, id, expected, indices, admit)
}

func (i *Inventory) repositoryPackage(ctx context.Context, row database.PluginRepository, entry string) (*pluginpackage.LocalSnapshot, error) {
	if !strings.HasPrefix(row.Origin, "https://") {
		return pluginpackage.InspectRepositoryPackage(row.Origin, row.RootID, entry)
	}
	archive, digest, err := i.download(ctx, row.Origin, row.CredentialRef)
	if err != nil {
		return nil, err
	}
	if digest != row.RootID {
		return nil, ErrPackageChanged
	}
	return archive.Package(row.Origin+"#"+url.PathEscape(entry), archive.Wrapper()+entry)
}
