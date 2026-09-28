package testutil

import . "github.com/onsi/ginkgo/v2"

// EntriesFor returns one table entry for each name, described by the name
// itself and passing it as the entry's only parameter. It is for tables whose
// entries are just the objects to check, where writing Entry("a", "a") for
// each would repeat every name.
func EntriesFor(names ...string) []TableEntry {
	entries := make([]TableEntry, len(names))
	for i, name := range names {
		entries[i] = Entry(name, name)
	}
	return entries
}

// DescribeAvailableDeployments registers a table that checks each named
// deployment in namespace is available (see deploymentIsAvailable). Call it
// inside the Describe container for the component that owns them.
func DescribeAvailableDeployments(namespace string, names ...string) {
	DescribeTable("has available deployment", func(ctx SpecContext, name string) {
		DeploymentIsAvailableByName(ctx, namespace, name)
	}, EntriesFor(names...))
}

// DescribeAvailableDaemonSets is DescribeAvailableDeployments for DaemonSets.
func DescribeAvailableDaemonSets(namespace string, names ...string) {
	DescribeTable("has available daemonset", func(ctx SpecContext, name string) {
		DaemonsetIsAvailableByName(ctx, namespace, name)
	}, EntriesFor(names...))
}
