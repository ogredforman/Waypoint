package profile

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ogredforman/waypoint/internal/api"
	"github.com/ogredforman/waypoint/internal/briefing"
	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
	"github.com/ogredforman/waypoint/internal/pipeline"
	"github.com/ogredforman/waypoint/internal/store"
)

// Options configure a Manager.
type Options struct {
	// Dir holds one <id>.db file per profile. It is created if missing.
	Dir string
	// LegacyDB is the database from before profiles. If it exists at
	// startup it is moved into Dir as a profile named "Personal".
	LegacyDB string
	// Seed is the settings a profile starts with (config.toml's values).
	Seed domain.Settings
	// Extractor processes every profile's entries.
	Extractor extract.Extractor
	// SnapshotRecentEntries is how many recent entries a snapshot holds.
	SnapshotRecentEntries int
	Logger                *slog.Logger
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// Manager runs every profile (ADR 0005): each is its own SQLite file with
// its own store, extraction worker, SSE hub, briefing service, and applier.
// It implements api.Profiles.
type Manager struct {
	opts   Options
	ctx    context.Context // parent of every worker
	cancel context.CancelFunc

	// ops serialises Create, Rename, and Delete, so name and ID checks
	// can't race each other.
	ops sync.Mutex

	mu       sync.RWMutex
	profiles []*running // in creation order
}

// running is one open profile.
type running struct {
	domain.Profile
	createdAt  time.Time
	path       string
	db         *sql.DB
	store      *store.Store
	hub        *api.Hub
	handler    http.Handler
	stop       context.CancelFunc
	workerDone chan struct{}
	// inflight counts requests being served, so Delete can wait for them
	// before closing the database.
	inflight sync.WaitGroup
}

var _ api.Profiles = (*Manager)(nil)

// Open starts every profile in opts.Dir. It first removes profiles a crash
// left half-built and moves the legacy database in; with no profiles at all,
// it creates "Personal".
func Open(ctx context.Context, opts Options) (*Manager, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("profiles: %w", err)
	}
	m := &Manager{opts: opts}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	if err := m.open(ctx); err != nil {
		m.Close()
		return nil, fmt.Errorf("profiles: %w", err)
	}
	return m, nil
}

func (m *Manager) open(ctx context.Context) error {
	if err := removeUnfinished(m.opts.Dir); err != nil {
		return err
	}
	ids, skipped, err := profileFiles(m.opts.Dir)
	if err != nil {
		return err
	}
	for _, name := range skipped {
		m.opts.Logger.Warn("skipping a database whose file name isn't a profile ID", "file", name)
	}
	for _, id := range ids {
		r, err := m.start(ctx, id)
		if err != nil {
			return err
		}
		m.profiles = append(m.profiles, r)
	}
	slices.SortFunc(m.profiles, func(a, b *running) int {
		return cmp.Or(a.createdAt.Compare(b.createdAt), strings.Compare(a.ID, b.ID))
	})
	if err := m.adoptLegacy(ctx); err != nil {
		return err
	}
	m.warnDuplicateNames()
	if len(m.profiles) == 0 {
		if _, err := m.Create(ctx, "Personal", ""); err != nil {
			return err
		}
	}
	return nil
}

// adoptLegacy moves the pre-profile database into the profiles directory.
// The database is migrated (which names it "Personal") and checkpointed, so
// the main file holds everything, then renamed in one step: a crash before
// the rename leaves it where it was, to be adopted on the next start.
func (m *Manager) adoptLegacy(ctx context.Context) error {
	if m.opts.LegacyDB == "" {
		return nil
	}
	if _, err := os.Stat(m.opts.LegacyDB); errors.Is(err, fs.ErrNotExist) {
		// A crash after an earlier move may have left empty sidecars.
		return removeSidecars(m.opts.LegacyDB)
	} else if err != nil {
		return err
	}
	// The migration names it "Personal"; another profile may have that name.
	name, err := prepare(ctx, m.opts.LegacyDB, "", m.opts.Seed, m.opts.Now)
	if err != nil {
		return fmt.Errorf("adopt %s: %w", m.opts.LegacyDB, err)
	}
	if unique := m.uniqueName(name); unique != name {
		if name, err = prepare(ctx, m.opts.LegacyDB, unique, m.opts.Seed, m.opts.Now); err != nil {
			return fmt.Errorf("adopt %s: %w", m.opts.LegacyDB, err)
		}
	}
	id := m.newID(name)
	if err := os.Rename(m.opts.LegacyDB, m.path(id)); err != nil {
		return fmt.Errorf("adopt %s: %w", m.opts.LegacyDB, err)
	}
	r, err := m.start(ctx, id)
	if err != nil {
		return err
	}
	m.profiles = append(m.profiles, r)
	m.opts.Logger.Info("moved the existing database into a profile", "profile", id)
	return nil
}

// start opens profile id's database and starts its services.
func (m *Manager) start(ctx context.Context, id string) (*running, error) {
	path := m.path(id)
	sqlDB, version, applied, err := openMigrated(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("profile %s: %w", id, err)
	}
	r, err := m.startDB(ctx, id, path, sqlDB)
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("profile %s: %w", id, err)
	}
	m.opts.Logger.Info("profile ready", "profile", id, "schema_version", version, "applied", applied)
	return r, nil
}

func (m *Manager) startDB(ctx context.Context, id, path string, sqlDB *sql.DB) (*running, error) {
	now := m.opts.Now
	logger := m.opts.Logger.With("profile", id)
	st := store.New(sqlDB, now)
	// Config values only seed the settings of a database that has none.
	if err := st.SeedSettings(ctx, m.opts.Seed); err != nil {
		return nil, err
	}
	meta, err := st.GetProfile(ctx)
	if err != nil {
		return nil, err
	}
	hub := api.NewHub(logger)
	snapshots := pipeline.NewSnapshotBuilder(st, now, m.opts.SnapshotRecentEntries)
	briefings := briefing.NewService(st, briefing.Heuristic{}, now, hub, logger)
	applier := changeset.NewApplier(st, changeset.AfterApplies{briefings, hub})
	worker := pipeline.NewWorker(st, snapshots, m.opts.Extractor, hub, briefings, logger)

	r := &running{
		Profile:    domain.Profile{ID: id, Name: meta.Name},
		createdAt:  meta.CreatedAt,
		path:       path,
		db:         sqlDB,
		store:      st,
		hub:        hub,
		workerDone: make(chan struct{}),
		handler: api.NewProfileHandler(api.ProfileDeps{
			Logger: logger, Hub: hub,
			Store: st, Applier: applier, Worker: worker, Briefings: briefings,
		}),
	}
	var workerCtx context.Context
	workerCtx, r.stop = context.WithCancel(m.ctx)
	go func() {
		defer close(r.workerDone)
		if err := worker.Run(workerCtx); err != nil {
			logger.Error("extraction worker stopped", "err", err)
		}
	}()
	return r, nil
}

// List returns the profiles in creation order.
func (m *Manager) List() []domain.Profile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.Profile, len(m.profiles))
	for i, r := range m.profiles {
		out[i] = r.Profile
	}
	return out
}

// Serve handles a request with profile id's router.
func (m *Manager) Serve(id string, w http.ResponseWriter, req *http.Request) bool {
	m.mu.RLock()
	r := m.find(id)
	if r != nil {
		r.inflight.Add(1)
	}
	m.mu.RUnlock()
	if r == nil {
		return false
	}
	defer r.inflight.Done()
	r.handler.ServeHTTP(w, req)
	return true
}

// Create makes a new profile named name, with settings from the seed, and
// starts it. If appearanceFrom names a profile, the new one starts with that
// profile's appearance (theme and fonts) instead of the seed's. It is built
// under a temporary file name and renamed into place once ready.
func (m *Manager) Create(ctx context.Context, name, appearanceFrom string) (domain.Profile, error) {
	m.ops.Lock()
	defer m.ops.Unlock()
	name, err := m.checkName(name, "")
	if err != nil {
		return domain.Profile{}, err
	}
	seed, err := m.seedFrom(ctx, appearanceFrom)
	if err != nil {
		return domain.Profile{}, err
	}
	id := m.newID(name)
	tmp := m.path(id) + newExt
	if _, err := prepare(ctx, tmp, name, seed, m.opts.Now); err != nil {
		_ = removeDB(tmp)
		return domain.Profile{}, fmt.Errorf("create profile: %w", err)
	}
	if err := os.Rename(tmp, m.path(id)); err != nil {
		_ = removeDB(tmp)
		return domain.Profile{}, fmt.Errorf("create profile: %w", err)
	}
	r, err := m.start(ctx, id)
	if err != nil {
		// Don't leave a profile behind that the API said wasn't created.
		if rmErr := removeDB(m.path(id)); rmErr != nil {
			m.opts.Logger.Error("removing a profile that failed to start", "profile", id, "err", rmErr)
		}
		return domain.Profile{}, err
	}
	m.mu.Lock()
	m.profiles = append(m.profiles, r)
	m.mu.Unlock()
	return r.Profile, nil
}

// seedFrom is the settings a new profile starts with: the seed, with the
// appearance of profile from if it isn't empty. The caller holds m.ops, so
// from can't be deleted meanwhile.
func (m *Manager) seedFrom(ctx context.Context, from string) (domain.Settings, error) {
	if from == "" {
		return m.opts.Seed, nil
	}
	m.mu.RLock()
	r := m.find(from)
	m.mu.RUnlock()
	if r == nil {
		return domain.Settings{}, &domain.ValidationError{Entity: "profile", Problems: []domain.FieldError{
			{Field: "appearance_from", Problem: "is not a profile"},
		}}
	}
	src, err := r.store.GetSettings(ctx)
	if err != nil {
		return domain.Settings{}, fmt.Errorf("create profile: read appearance: %w", err)
	}
	return m.opts.Seed.CopyAppearance(src), nil
}

// Rename changes profile id's display name. The ID stays the same.
func (m *Manager) Rename(ctx context.Context, id, name string) (domain.Profile, error) {
	m.ops.Lock()
	defer m.ops.Unlock()
	m.mu.RLock()
	r := m.find(id)
	m.mu.RUnlock()
	if r == nil {
		return domain.Profile{}, fmt.Errorf("profile %q: %w", id, store.ErrNotFound)
	}
	name, err := m.checkName(name, id)
	if err != nil {
		return domain.Profile{}, err
	}
	err = r.store.InTx(ctx, func(tx *store.Tx) error {
		name, err = tx.SetProfileName(ctx, name)
		return err
	})
	if err != nil {
		return domain.Profile{}, err
	}
	m.mu.Lock()
	r.Name = name
	p := r.Profile
	m.mu.Unlock()
	return p, nil
}

// Delete stops profile id and removes its database with its -wal and -shm
// files. The last profile can't be deleted. New requests for the profile get
// 404 at once; open event streams end, and requests already running finish
// before the database closes.
func (m *Manager) Delete(_ context.Context, id string) error {
	m.ops.Lock()
	defer m.ops.Unlock()
	m.mu.Lock()
	i := slices.IndexFunc(m.profiles, func(r *running) bool { return r.ID == id })
	if i < 0 {
		m.mu.Unlock()
		return fmt.Errorf("profile %q: %w", id, store.ErrNotFound)
	}
	if len(m.profiles) == 1 {
		m.mu.Unlock()
		return domain.ErrLastProfile
	}
	r := m.profiles[i]
	m.profiles = slices.Delete(m.profiles, i, i+1)
	m.mu.Unlock()

	// A close error still removes the files, so the profile doesn't come back
	// on the next start.
	if err := r.close(); err != nil {
		m.opts.Logger.Error("closing a deleted profile failed", "profile", id, "err", err)
	}
	if err := removeDB(r.path); err != nil {
		return fmt.Errorf("delete profile %s: %w", id, err)
	}
	m.opts.Logger.Info("profile deleted", "profile", id)
	return nil
}

// Ping reports whether every profile's database can be reached.
func (m *Manager) Ping(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.profiles {
		if err := r.db.PingContext(ctx); err != nil {
			return fmt.Errorf("profile %s: %w", r.ID, err)
		}
	}
	return nil
}

// EndStreams ends every profile's open event streams. The server calls it
// when shutdown starts, so it doesn't wait on streams that never finish.
func (m *Manager) EndStreams() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.profiles {
		r.hub.Close()
	}
}

// Close stops every profile's worker and closes its database. Call it once
// the server has stopped.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.profiles {
		if err := r.close(); err != nil {
			m.opts.Logger.Error("closing profile failed", "profile", r.ID, "err", err)
		}
	}
	m.profiles = nil
	m.cancel()
}

// close ends the profile's streams, waits for its requests, stops its
// worker, and closes its database.
func (r *running) close() error {
	r.hub.Close()
	r.inflight.Wait()
	r.stop()
	<-r.workerDone
	return r.db.Close()
}

// find returns profile id, or nil. The caller holds m.mu.
func (m *Manager) find(id string) *running {
	for _, r := range m.profiles {
		if r.ID == id {
			return r
		}
	}
	return nil
}

func (m *Manager) path(id string) string {
	return filepath.Join(m.opts.Dir, id+dbExt)
}

// checkName cleans name and checks no other profile than except uses it,
// ignoring case.
func (m *Manager) checkName(name, except string) (string, error) {
	name, err := domain.CleanProfileName(name)
	if err != nil {
		return "", err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.profiles {
		if r.ID != except && strings.EqualFold(r.Name, name) {
			return "", &domain.ValidationError{Entity: "profile", Problems: []domain.FieldError{
				{Field: "name", Problem: "is already used by another profile"},
			}}
		}
	}
	return name, nil
}

// uniqueName returns name, or name with a number after it if a profile
// already uses it.
func (m *Manager) uniqueName(name string) string {
	candidate := name
	for n := 2; ; n++ {
		if _, err := m.checkName(candidate, ""); err == nil {
			return candidate
		}
		candidate = name + " " + strconv.Itoa(n)
	}
}

// newID returns an unused ID made from name: its slug, with "-2", "-3", …
// added if a profile or a file already has it. IDs never change, so a
// renamed profile keeps its file.
func (m *Manager) newID(name string) string {
	base := domain.ProfileSlug(name)
	for n := 1; ; n++ {
		id := base
		if n > 1 {
			id += "-" + strconv.Itoa(n)
		}
		m.mu.RLock()
		taken := m.find(id) != nil
		m.mu.RUnlock()
		if taken {
			continue
		}
		// Any error but "exists" leaves it to the caller to fail on the file.
		if _, err := os.Stat(m.path(id)); err != nil {
			return id
		}
	}
}

// warnDuplicateNames logs profiles on disk that share a name, ignoring case
// (e.g. a copied file). Only IDs are logged; renaming one fixes it.
func (m *Manager) warnDuplicateNames() {
	for i, a := range m.profiles {
		for _, b := range m.profiles[i+1:] {
			if strings.EqualFold(a.Name, b.Name) {
				m.opts.Logger.Warn("two profiles have the same name; rename one", "profile", a.ID, "other", b.ID)
			}
		}
	}
}
