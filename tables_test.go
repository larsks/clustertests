package clustertests

import . "github.com/onsi/ginkgo/v2"

// entriesFor returns one table entry for each name, described by the name
// itself and passing it as the entry's only parameter. It is for tables whose
// entries are just the objects to check, where writing Entry("a", "a") for
// each would repeat every name.
func entriesFor(names ...string) []TableEntry {
	entries := make([]TableEntry, len(names))
	for i, name := range names {
		entries[i] = Entry(name, name)
	}
	return entries
}
