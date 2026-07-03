package registry

import "sort"

// Entry holds a backend name and its corresponding backend instance.
type Entry[T any] struct {
	Name    string
	Backend T
}

// New creates a sorted list of registry entries from a map.
// Entries are sorted by Name in ascending order.
func New[T any](m map[string]T) []Entry[T] {
	entries := make([]Entry[T], 0, len(m))
	for name, backend := range m {
		entries = append(entries, Entry[T]{Name: name, Backend: backend})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// Find searches for an entry by name and returns its backend.
// If not found, returns the zero value for T and false.
func Find[T any](entries []Entry[T], name string) (T, bool) {
	for _, entry := range entries {
		if entry.Name == name {
			return entry.Backend, true
		}
	}
	var zero T
	return zero, false
}
