// Package desktopinstance reserves a desktop before services start (AEP-0111).
// Sidecars must not be deleted while participants may be alive. The database
// is only opened read-only for identity, never created or modified.
// Existing hardlinks are covered within the same user's cache directory.
// Hardlinks created after an absent database is created or after replacement,
// hostile sidecar replacement, and network filesystems are not supported.
package desktopinstance

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"assistente/internal/commandinstance"
)

var ErrAlreadyRunning = errors.New("desktop instance already running")
var ErrNotification = errors.New("could not notify desktop instance")

const notificationTimeout = time.Second
const connectionTimeout = 100 * time.Millisecond
const sidecarSuffix = ".desktop-instance"

type endpoint struct {
	Address string `json:"address"`
	Token   string `json:"token"`
}

type heldLock struct {
	file *os.File
	lock *commandinstance.Lock
}

func (h heldLock) close() error { return errors.Join(h.file.Close(), h.lock.Close()) }

// Guard implements io.Closer. Retain until desktop shutdown finishes.
type Guard struct {
	once     sync.Once
	listener net.Listener
	locks    []heldLock
	done     chan struct{}
	served   chan struct{}
	activate chan struct{}
	closeErr error
}

// Acquire runs before SQLite/services. onActivate must only enqueue a request
// (buffered channel of size one), never wait for the UI. The caller drains that
// channel after successful startup. ErrAlreadyRunning means activation was
// accepted by the existing instance. ErrNotification means an occupied lock
// could not be notified. All errors must abort startup; only a guard permits it.
func Acquire(databasePath string, onActivate func()) (*Guard, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	return acquire(databasePath, filepath.Join(cache, "assistente", "desktopinstance"), onActivate)
}

func canonicalPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("empty database path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		info, err := os.Stat(resolved)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			return "", errors.New("database is not a regular file")
		}
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	// Reject dangling symlinks rather than treating them as a new database.
	if _, statErr := os.Lstat(abs); !errors.Is(statErr, os.ErrNotExist) {
		return "", err
	}
	// First launch may not have ~/.assistente yet. Create only directories;
	// existing directory permissions are left unchanged and SQLite stays absent.
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

func acquire(databasePath, identityDir string, onActivate func()) (_ *Guard, resultErr error) {
	if onActivate == nil {
		return nil, errors.New("nil activation callback")
	}
	path, err := canonicalPath(databasePath)
	if err != nil {
		return nil, err
	}
	g := &Guard{done: make(chan struct{}), served: make(chan struct{}), activate: make(chan struct{}, 1)}
	defer func() {
		if resultErr != nil {
			if g.listener != nil {
				_ = g.listener.Close()
			}
			for _, held := range g.locks {
				_ = held.close()
			}
		}
	}()
	if err := g.take(path + sidecarSuffix); err != nil {
		return nil, err
	}
	db, err := os.Open(path)
	if err == nil {
		info, statErr := db.Stat()
		if statErr != nil || !info.Mode().IsRegular() {
			_ = db.Close()
			return nil, errors.New("database is not a regular file")
		}
		identity, identityErr := physicalIdentity(db)
		if err := errors.Join(identityErr, db.Close()); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(identityDir, 0o700); err != nil {
			return nil, err
		}
		key := sha256.Sum256([]byte(identity))
		if err := g.take(filepath.Join(identityDir, hex.EncodeToString(key[:])+".lock")); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	g.listener, err = net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, err
	}
	e := endpoint{Address: g.listener.Addr().String(), Token: hex.EncodeToString(secret[:])}
	data, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	for _, held := range g.locks {
		if err := held.file.Truncate(0); err != nil {
			return nil, err
		}
		if _, err := held.file.WriteAt(data, 0); err != nil {
			return nil, err
		}
	}
	go g.serve(e.Token)
	go func() {
		for {
			select {
			case <-g.done:
				return
			case <-g.activate:
				select {
				case <-g.done:
					return
				default:
					onActivate()
				}
			}
		}
	}()
	return g, nil
}

func (g *Guard) take(path string) error {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("instance sidecar is not a regular file")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	lock, err := commandinstance.Acquire(context.Background(), path)
	if err != nil {
		defer func() { _ = f.Close() }()
		if errors.Is(err, commandinstance.ErrBusy) {
			if notifyErr := notify(f); notifyErr != nil {
				return errors.Join(ErrNotification, notifyErr)
			}
			return ErrAlreadyRunning
		}
		return err
	}
	g.locks = append(g.locks, heldLock{file: f, lock: lock})
	return nil
}

func notify(file *os.File) error {
	deadline := time.Now().Add(notificationTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		var buf [1024]byte
		n, err := file.ReadAt(buf[:], 0)
		if err == nil || errors.Is(err, io.EOF) {
			var e endpoint
			err = json.Unmarshal(buf[:n], &e)
			if err == nil {
				err = sendActivation(e, deadline)
			}
		}
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(min(20*time.Millisecond, max(0, time.Until(deadline))))
	}
	return fmt.Errorf("activation deadline: %w", lastErr)
}

func sendActivation(e endpoint, deadline time.Time) error {
	// Parse numerically: metadata must never cause DNS or a remote connection.
	addr, err := netip.ParseAddrPort(e.Address)
	if err != nil || addr.Addr() != netip.MustParseAddr("127.0.0.1") || addr.Port() == 0 || len(e.Token) != 64 {
		return errors.New("invalid local activation endpoint")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return errors.New("activation deadline exceeded")
	}
	conn, err := net.DialTimeout("tcp4", addr.String(), min(connectionTimeout, remaining))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(minTime(deadline, time.Now().Add(connectionTimeout))); err != nil {
		return err
	}
	if _, err := io.WriteString(conn, e.Token); err != nil {
		return err
	}
	var ack [1]byte
	if _, err := io.ReadFull(conn, ack[:]); err != nil {
		return err
	}
	if ack[0] != 1 {
		return errors.New("activation rejected")
	}
	return nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (g *Guard) serve(token string) {
	defer close(g.served)
	for {
		conn, err := g.listener.Accept()
		if err != nil {
			return
		}
		_ = conn.SetDeadline(time.Now().Add(connectionTimeout))
		var request [64]byte
		if _, err := io.ReadFull(conn, request[:]); err == nil && string(request[:]) == token {
			select {
			case g.activate <- struct{}{}:
			default:
			}
			_, _ = conn.Write([]byte{1})
		}
		_ = conn.Close()
	}
}

// Close does not wait for an activation callback already in flight. Callbacks
// must be short nonblocking enqueues; UI work belongs to the caller's consumer.
func (g *Guard) Close() error {
	if g == nil {
		return nil
	}
	g.once.Do(func() {
		close(g.done)
		g.closeErr = g.listener.Close()
		<-g.served
		for _, held := range g.locks {
			g.closeErr = errors.Join(g.closeErr, held.close())
		}
	})
	return g.closeErr
}
