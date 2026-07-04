package adapters

import "sort"

// BackendEntry pairs a configured backend name with its adapter instance.
type BackendEntry[T any] struct {
	Name    string
	Backend T
}

// Registry is a name-sorted collection of a domain's backends.
type Registry[T any] []BackendEntry[T]

// NewRegistry builds a Registry from a name→backend map, sorted by name
// so iteration order is deterministic.
func NewRegistry[T any](m map[string]T) Registry[T] {
	entries := make(Registry[T], 0, len(m))
	for name, backend := range m {
		entries = append(entries, BackendEntry[T]{Name: name, Backend: backend})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// Find returns the backend with the given name.
// If not found, returns the zero value for T and false.
func (r Registry[T]) Find(name string) (T, bool) {
	for _, entry := range r {
		if entry.Name == name {
			return entry.Backend, true
		}
	}
	var zero T
	return zero, false
}
