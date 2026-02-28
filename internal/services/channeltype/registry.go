package channeltype

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zentra/peridotite/internal/models"
)

var (
	ErrTypeNotFound = errors.New("channel type not found")
	ErrTypeExists   = errors.New("channel type already exists")
	ErrBuiltInType  = errors.New("cannot modify built-in channel types")
	ErrInvalidType  = errors.New("invalid channel type")
)

// Registry manages channel type definitions. It keeps an in-memory cache
// backed by the database so lookups are fast but new types persist across
// restarts. Plugins register types through this registry.
type Registry struct {
	db    *pgxpool.Pool
	mu    sync.RWMutex
	types map[string]*models.ChannelTypeDefinition
}

func NewRegistry(db *pgxpool.Pool) *Registry {
	return &Registry{
		db:    db,
		types: make(map[string]*models.ChannelTypeDefinition),
	}
}

// Load pulls all registered types from the database into the in-memory cache.
// Call this once at startup after migrations have run.
func (r *Registry) Load(ctx context.Context) error {
	rows, err := r.db.Query(ctx,
		`SELECT id, name, description, icon, capabilities, default_metadata, built_in, plugin_id, created_at
		FROM channel_type_definitions ORDER BY built_in DESC, id`,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	r.mu.Lock()
	defer r.mu.Unlock()

	for rows.Next() {
		def := &models.ChannelTypeDefinition{}
		if err := rows.Scan(
			&def.ID, &def.Name, &def.Description, &def.Icon,
			&def.Capabilities, &def.DefaultMetadata, &def.BuiltIn,
			&def.PluginID, &def.CreatedAt,
		); err != nil {
			return err
		}
		r.types[def.ID] = def
	}

	return rows.Err()
}

// Get returns a channel type definition by ID, or an error if it doesn't exist.
func (r *Registry) Get(id string) (*models.ChannelTypeDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.types[id]
	if !ok {
		return nil, ErrTypeNotFound
	}
	return def, nil
}

// All returns every registered channel type.
func (r *Registry) All() []*models.ChannelTypeDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*models.ChannelTypeDefinition, 0, len(r.types))
	for _, def := range r.types {
		out = append(out, def)
	}
	return out
}

// Exists tells you whether a type ID has been registered.
func (r *Registry) Exists(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.types[id]
	return ok
}

// Register adds a new channel type. This is the main extension point for
// plugins - they call Register with their custom type definition and it
// becomes available across the whole app.
func (r *Registry) Register(ctx context.Context, def *models.ChannelTypeDefinition) error {
	if def.ID == "" || def.Name == "" {
		return ErrInvalidType
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.types[def.ID]; exists {
		return ErrTypeExists
	}

	if def.DefaultMetadata == nil {
		def.DefaultMetadata = json.RawMessage("{}")
	}

	_, err := r.db.Exec(ctx,
		`INSERT INTO channel_type_definitions (id, name, description, icon, capabilities, default_metadata, built_in, plugin_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		def.ID, def.Name, def.Description, def.Icon,
		def.Capabilities, def.DefaultMetadata, def.BuiltIn, def.PluginID,
	)
	if err != nil {
		return err
	}

	r.types[def.ID] = def
	return nil
}

// Unregister removes a plugin-provided channel type. Built-in types can't
// be removed.
func (r *Registry) Unregister(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	def, ok := r.types[id]
	if !ok {
		return ErrTypeNotFound
	}
	if def.BuiltIn {
		return ErrBuiltInType
	}

	_, err := r.db.Exec(ctx, `DELETE FROM channel_type_definitions WHERE id = $1 AND built_in = FALSE`, id)
	if err != nil {
		return err
	}

	delete(r.types, id)
	return nil
}

// GetFromDB fetches a single type definition directly from the database.
// Prefer Get() for hot-path lookups since it uses the cache.
func (r *Registry) GetFromDB(ctx context.Context, id string) (*models.ChannelTypeDefinition, error) {
	def := &models.ChannelTypeDefinition{}
	err := r.db.QueryRow(ctx,
		`SELECT id, name, description, icon, capabilities, default_metadata, built_in, plugin_id, created_at
		FROM channel_type_definitions WHERE id = $1`, id,
	).Scan(
		&def.ID, &def.Name, &def.Description, &def.Icon,
		&def.Capabilities, &def.DefaultMetadata, &def.BuiltIn,
		&def.PluginID, &def.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTypeNotFound
		}
		return nil, err
	}
	return def, nil
}
