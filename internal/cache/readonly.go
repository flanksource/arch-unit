package cache

// ReadOnlyCache provides read-only access to cached AST data
// This interface allows extractors to lookup existing node IDs without
// being able to modify the cache or database
type ReadOnlyCache interface {
	// GetASTId looks up the database ID for a node by its key
	// Returns the ID and true if found, 0 and false if not found
	GetASTId(key string) (int64, bool)
}