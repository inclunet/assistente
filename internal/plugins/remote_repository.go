package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/plugincatalog"
	"assistente/internal/pluginpackage"
	httpclient "assistente/internal/tools/http"
	"golang.org/x/net/idna"
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
			if len(via) == 0 || next.URL.Scheme != "https" || next.URL.User != nil || addressHost(next.URL, true) != addressHost(via[0].URL, true) {
				return http.ErrUseLastResponse
			}
			// HTTP validators identify a representation at a particular URL.
			next.Header.Del("If-None-Match")
			next.Header.Del("If-Modified-Since")
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

// addressHost normalizes the representation of a validated HTTPS authority.
// The transfer retains an explicit default port for network consent.
func addressHost(u *url.URL, omitDefault bool) string {
	name := u.Hostname()
	if addr, err := netip.ParseAddr(u.Hostname()); err == nil && addr.Is6() {
		name = "[" + addr.String() + "]"
	} else if !isASCIIHost(name) {
		ascii, err := idna.Lookup.ToASCII(name)
		if err != nil {
			return ""
		}
		name = ascii
	} else {
		name = strings.ToLower(name)
	}
	port := u.Port()
	if port == "" {
		return name
	}
	number := strings.TrimLeft(port, "0")
	if omitDefault && number == "443" {
		return name
	}
	return name + ":" + number
}

func (i *Inventory) WithRemoteClient(create RemoteClient, watch func(context.Context) (context.Context, func(), error)) *Inventory {
	copy := *i
	copy.remote = create
	copy.watchRemote = watch
	return &copy
}

func isASCIIHost(host string) bool {
	return strings.IndexFunc(host, func(r rune) bool { return r > 127 }) < 0
}

func remoteURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("PLUGIN_REPOSITORY_URL")
	}
	if addressHost(u, true) == "" {
		return nil, errors.New("PLUGIN_REPOSITORY_URL")
	}
	if u.Path == "" {
		u.Path = "/"
	}
	if port := u.Port(); port != "" {
		n, err := strconv.ParseUint(port, 10, 16)
		if err != nil || n == 0 {
			return nil, errors.New("PLUGIN_REPOSITORY_URL")
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return nil, errors.New("PLUGIN_REPOSITORY_URL")
	}
	return u, nil
}

// Validators describe the saved index. Package previews always read and hash the body.
type repositoryValidators struct {
	etag, modified, root string
}

// If-None-Match also accepts lists and "*". A saved ETag must identify
// one representation, so only accept the entity-tag grammar from RFC 9110.
func validArchiveETag(value string) bool {
	if len(value) > 1024 {
		return false
	}
	tag := strings.TrimPrefix(value, "W/")
	if len(tag) < 2 || tag[0] != '"' || tag[len(tag)-1] != '"' {
		return false
	}
	for _, ch := range []byte(tag[1 : len(tag)-1]) {
		if ch < 0x21 || ch == '"' || ch == 0x7f {
			return false
		}
	}
	return true
}

func (i *Inventory) download(ctx context.Context, origin, reference string, git bool, cache *repositoryValidators) (*pluginpackage.Archive, string, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, "", err
	}
	u, err := remoteURL(origin)
	if err != nil || (reference != "" && addressHost(&url.URL{Host: reference}, true) != addressHost(u, true)) || i.remote == nil {
		return nil, "", errors.New("PLUGIN_REPOSITORY_URL")
	}
	if i.watchRemote != nil {
		var release func()
		ctx, release, err = i.watchRemote(ctx)
		if err != nil {
			return nil, "", err
		}
		defer release()
	}
	client := i.remote(reference)
	if client == nil {
		return nil, "", errors.New("PLUGIN_REPOSITORY_DOWNLOAD")
	}
	if git {
		archive, revision, err := readGitRepository(ctx, client, u.String(), cache.root)
		if err != nil {
			if ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			return nil, "", gitFailure(err)
		}
		return archive, revision, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", errors.New("PLUGIN_REPOSITORY_URL")
	}
	if validArchiveETag(cache.etag) {
		req.Header.Set("If-None-Match", cache.etag)
	}
	if cache.modified != "" {
		req.Header.Set("If-Modified-Since", cache.modified)
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
	if (res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden) && reference != "" {
		return nil, "", errors.New("PLUGIN_REPOSITORY_CREDENTIAL")
	}
	direct := res.Request != nil && res.Request.URL.String() == req.URL.String()
	if direct && res.StatusCode == http.StatusNotModified && cache.root != "" && (validArchiveETag(cache.etag) || cache.modified != "") {
		return nil, cache.root, nil
	}
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
	cache.etag = res.Header.Get("ETag")
	if !validArchiveETag(cache.etag) {
		cache.etag = ""
	}
	cache.modified = res.Header.Get("Last-Modified")
	if _, err := http.ParseTime(cache.modified); err != nil {
		cache.modified = ""
	}
	if !direct {
		cache.etag = ""
		cache.modified = ""
	}
	sum := sha256.Sum256(data)
	return archive, hex.EncodeToString(sum[:]), nil
}

func (i *Inventory) SaveRemoteRepository(ctx context.Context, raw string, authenticated bool, admit commandsecurity.Admission) error {
	return i.addRemote(ctx, raw, authenticated, false, admit)
}
func (i *Inventory) SaveGitRepository(ctx context.Context, raw string, authenticated bool, admit commandsecurity.Admission) error {
	return i.addRemote(ctx, raw, authenticated, true, admit)
}
func (i *Inventory) addRemote(ctx context.Context, raw string, authenticated, git bool, admit commandsecurity.Admission) error {
	u, err := remoteURL(raw)
	if err != nil {
		return err
	}
	ref := ""
	if authenticated {
		ref = u.Host
	}
	u.Host = addressHost(u, false)
	return i.saveRemote(ctx, u.String(), ref, nil, git, admit)
}

// The canonical origin identifies a store; network authorization distinguishes an
// explicitly requested default port from a host-wide destination.
func repositoryDownloadURL(row database.PluginRepository) string {
	if !row.ExplicitHTTPSPort {
		return row.Origin
	}
	u, err := remoteURL(row.Origin)
	if err != nil || u.Port() != "" {
		return row.Origin
	}
	u.Host += ":443"
	return u.String()
}

func (i *Inventory) saveRemote(ctx context.Context, origin, ref string, previous *database.PluginRepository, git bool, admit commandsecurity.Admission) error {
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
	var cache repositoryValidators
	var id, expected string
	if previous != nil {
		cache = repositoryValidators{etag: previous.ArchiveETag, modified: previous.ArchiveLastModified, root: previous.RootID}
		id, expected = previous.ID, previous.Revision
	}
	archive, digest, err := i.download(ctx, origin, ref, git, &cache)
	if err != nil {
		return err
	}
	var indices []plugincatalog.Index
	if archive == nil && previous != nil {
		err = json.Unmarshal(previous.Indices, &indices)
	} else {
		prefix := remoteCatalogPrefix(archive, digest)
		indices, err = plugincatalog.ReadIndices(func(name string) ([]byte, error) { return archive.Read(prefix+name, 10<<20) })
	}
	if err != nil {
		return err
	}
	u, err := remoteURL(origin)
	if err != nil {
		return err
	}
	explicitHTTPSPort := u.Port() == "443"
	u.Host = addressHost(u, true)
	return i.publishRepository(ctx, owner, u.String(), digest, ref, explicitHTTPSPort, id, expected, indices, cache, admit)
}

func (i *Inventory) repositoryPackage(ctx context.Context, row database.PluginRepository, entry string) (*pluginpackage.LocalSnapshot, error) {
	if digest, ok := strings.CutPrefix(row.RootID, localZIPPrefix); ok {
		archive, current, err := pluginpackage.OpenLocalArchive(row.Origin)
		if err != nil {
			return nil, err
		}
		if digest != current {
			return nil, errors.Join(ErrPackageChanged, errors.New("PLUGIN_REPOSITORY_ARCHIVE_CHANGED"))
		}
		return archive.Package(row.Origin+"#"+url.PathEscape(entry), plugincatalog.ArchivePrefix(archive)+entry)
	}
	if !strings.HasPrefix(row.Origin, "https://") {
		return pluginpackage.InspectRepositoryPackage(row.Origin, row.RootID, entry)
	}
	archive, digest, err := i.download(ctx, repositoryDownloadURL(row), row.CredentialRef, strings.HasPrefix(row.RootID, gitRevisionPrefix), &repositoryValidators{})
	if err != nil {
		return nil, err
	}
	if digest != row.RootID {
		return nil, errors.Join(ErrPackageChanged, errors.New("PLUGIN_REPOSITORY_ARCHIVE_CHANGED"))
	}
	return archive.Package(row.Origin+"#"+url.PathEscape(entry), remoteCatalogPrefix(archive, digest)+entry)
}

func remoteCatalogPrefix(archive *pluginpackage.Archive, revision string) string {
	if strings.HasPrefix(revision, gitRevisionPrefix) {
		return ""
	}
	return plugincatalog.ArchivePrefix(archive)
}
