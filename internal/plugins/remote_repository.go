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
	httpclient "assistente/internal/tools/http"
)

// RemoteClient receives the exact authority selected for this repository.
// An empty credential reference denotes an anonymous request.
type RemoteClient func(reference string) *httpclient.Client

func RepositoryHTTPClient(manager *credentials.Manager, reference string, authorizer httpclient.NetworkAuthorizer) *httpclient.Client {
	guardRedirect := httpclient.RedirectGuard(4, nil)
	client := httpclient.New(&httpclient.Config{
		Timeout:    45 * time.Second,
		Authorizer: authorizer,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) == 0 || next.URL.Scheme != "https" || next.URL.User != nil || canonicalHost(next.URL) != canonicalHost(via[0].URL) {
				return http.ErrUseLastResponse
			}
			return guardRedirect(next, via)
		},
	}, nil)
	base := httpclient.NewGuardedTransport(nil)
	// Each connection must carry the authorization of the current request.
	base.DisableKeepAlives = true
	client.GetBaseClient().Transport = base
	if reference != "" {
		transport := credentials.NewCredentialTransportWithMode(manager, reference, credentials.AuthRequired)
		transport.RequireCredential = true
		transport.Base = base
		client.GetBaseClient().Transport = transport
	}
	return client
}

func canonicalHost(u *url.URL) string {
	host := strings.ToLower(u.Host)
	return strings.TrimSuffix(host, ":443")
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
	if err != nil || (reference != "" && canonicalHost(&url.URL{Host: reference}) != canonicalHost(u)) || i.remote == nil {
		return nil, "", errors.New("PLUGIN_REPOSITORY_URL")
	}
	client := i.remote(reference)
	if client == nil {
		return nil, "", errors.New("PLUGIN_REPOSITORY_DOWNLOAD")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", errors.New("PLUGIN_REPOSITORY_URL")
	}
	res, err := client.Do(ctx, req)
	if errors.Is(err, credentials.ErrCredentialResolution) {
		return nil, "", errors.New("PLUGIN_REPOSITORY_CREDENTIAL")
	}
	var blocked *httpclient.BlockedDestinationError
	if errors.As(err, &blocked) {
		return nil, "", errors.New("PLUGIN_REPOSITORY_NETWORK_DENIED")
	}
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

func (i *Inventory) SaveRemoteRepository(ctx context.Context, raw string, authenticated bool, admit commandsecurity.Admission) error {
	u, err := remoteURL(raw)
	if err != nil {
		return err
	}
	ref := ""
	if authenticated {
		ref = u.Host
	}
	u.Host = canonicalHost(u)
	return i.saveRemote(ctx, u.String(), ref, "", "", admit)
}

func (i *Inventory) saveRemote(ctx context.Context, origin, ref, id, expected string, admit commandsecurity.Admission) error {
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
	archive, digest, err := i.download(ctx, origin, ref)
	if err != nil {
		return err
	}
	prefix := plugincatalog.ArchivePrefix(archive)
	indices, err := plugincatalog.ReadIndices(func(name string) ([]byte, error) { return archive.Read(prefix+name, 10<<20) })
	if err != nil {
		return err
	}
	return i.publishRepository(ctx, owner, origin, digest, ref, id, expected, indices, admit)
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
		return nil, errors.Join(ErrPackageChanged, errors.New("PLUGIN_REPOSITORY_ARCHIVE_CHANGED"))
	}
	return archive.Package(row.Origin+"#"+url.PathEscape(entry), plugincatalog.ArchivePrefix(archive)+entry)
}
