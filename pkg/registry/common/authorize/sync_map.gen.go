// Code generated by "-output sync_map.gen.go -type spiffeIDResourcesMap<github.com/spiffe/go-spiffe/v2/spiffeid.ID,*github.com/networkservicemesh/sdk/pkg/tools/stringset.StringSet> -output sync_map.gen.go -type spiffeIDResourcesMap<github.com/spiffe/go-spiffe/v2/spiffeid.ID,*github.com/networkservicemesh/sdk/pkg/tools/stringset.StringSet>"; DO NOT EDIT.
package authorize

import (
	"sync" // Used by sync.Map.

	"github.com/spiffe/go-spiffe/v2/spiffeid"

	"github.com/networkservicemesh/sdk/pkg/tools/stringset"
)

// Generate code that will fail if the constants change value.
func _() {
	// An "cannot convert spiffeIDResourcesMap literal (type spiffeIDResourcesMap) to type sync.Map" compiler error signifies that the base type have changed.
	// Re-run the go-syncmap command to generate them again.
	_ = (sync.Map)(spiffeIDResourcesMap{})
}

var _nil_spiffeIDResourcesMap_stringset_StringSet_value = func() (val *stringset.StringSet) { return }()

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (m *spiffeIDResourcesMap) Load(key spiffeid.ID) (*stringset.StringSet, bool) {
	value, ok := (*sync.Map)(m).Load(key)
	if value == nil {
		return _nil_spiffeIDResourcesMap_stringset_StringSet_value, ok
	}
	return value.(*stringset.StringSet), ok
}

// Store sets the value for a key.
func (m *spiffeIDResourcesMap) Store(key spiffeid.ID, value *stringset.StringSet) {
	(*sync.Map)(m).Store(key, value)
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (m *spiffeIDResourcesMap) LoadOrStore(key spiffeid.ID, value *stringset.StringSet) (*stringset.StringSet, bool) {
	actual, loaded := (*sync.Map)(m).LoadOrStore(key, value)
	if actual == nil {
		return _nil_spiffeIDResourcesMap_stringset_StringSet_value, loaded
	}
	return actual.(*stringset.StringSet), loaded
}

// LoadAndDelete deletes the value for a key, returning the previous value if any.
// The loaded result reports whether the key was present.
func (m *spiffeIDResourcesMap) LoadAndDelete(key spiffeid.ID) (value *stringset.StringSet, loaded bool) {
	actual, loaded := (*sync.Map)(m).LoadAndDelete(key)
	if actual == nil {
		return _nil_spiffeIDResourcesMap_stringset_StringSet_value, loaded
	}
	return actual.(*stringset.StringSet), loaded
}

// Delete deletes the value for a key.
func (m *spiffeIDResourcesMap) Delete(key spiffeid.ID) {
	(*sync.Map)(m).Delete(key)
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
//
// Range does not necessarily correspond to any consistent snapshot of the Map's
// contents: no key will be visited more than once, but if the value for any key
// is stored or deleted concurrently, Range may reflect any mapping for that key
// from any point during the Range call.
//
// Range may be O(N) with the number of elements in the map even if f returns
// false after a constant number of calls.
func (m *spiffeIDResourcesMap) Range(f func(key spiffeid.ID, value *stringset.StringSet) bool) {
	(*sync.Map)(m).Range(func(key, value interface{}) bool {
		return f(key.(spiffeid.ID), value.(*stringset.StringSet))
	})
}
