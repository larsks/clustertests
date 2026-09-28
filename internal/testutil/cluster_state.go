package testutil

import (
	"context"

	. "github.com/onsi/ginkgo/v2"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// listNodes pages through nodes matching opts and returns them all. Nodes are
// cluster-scoped, so this always spans the whole cluster.
func listNodes(ctx context.Context, opts metav1.ListOptions) ([]corev1.Node, error) {
	var nodes []corev1.Node
	err := EachItem(ctx, CoreClient.CoreV1().Nodes().List, opts, func(node *corev1.Node) error {
		nodes = append(nodes, *node)
		return nil
	})
	return nodes, err
}

// Cluster-wide facts that many checks share. They are fetched once per test
// process by LoadClusterState, in the per-process half of
// SynchronizedBeforeSuite, rather than by each check that needs them, so a
// failure to fetch one aborts the suite at setup instead of failing whichever
// specs happened to ask first. Nodes, StorageClasses and namespaces are few
// enough (unlike pods, which are streamed instead of cached) that holding them
// whole is cheap, and every consumer just filters them locally. They are a
// snapshot from suite start: a node that joins, leaves, or gets relabeled
// later in the same run won't be reflected.
var (
	// ClusterNodes is every node in the cluster.
	ClusterNodes []corev1.Node

	// clusterStorageClasses is every StorageClass in the cluster. Unlike pods,
	// they are few enough in any real cluster that a single unpaged List is
	// fine.
	clusterStorageClasses []storagev1.StorageClass

	// excludedNamespaceNames holds the names of the namespaces matching
	// EXCLUDE_NAMESPACE_SELECTOR. It is empty when the variable is unset.
	excludedNamespaceNames map[string]struct{}
)

// LoadClusterState fetches the cluster-wide facts above.
func LoadClusterState(ctx context.Context) {
	GinkgoHelper()

	var err error
	ClusterNodes, err = listNodes(ctx, metav1.ListOptions{})
	ExpectNoError(err, "list nodes")

	classes, err := CoreClient.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	ExpectNoError(err, "list StorageClasses")
	clusterStorageClasses = classes.Items

	excludedNamespaceNames = listExcludedNamespaces(ctx)
}

// NodesMatching filters ClusterNodes by selector.
func NodesMatching(selector labels.Selector) []corev1.Node {
	var matched []corev1.Node
	for _, node := range ClusterNodes {
		if selector.Matches(labels.Set(node.Labels)) {
			matched = append(matched, node)
		}
	}
	return matched
}

// listExcludedNamespaces returns the names of the namespaces whose labels
// match EXCLUDE_NAMESPACE_SELECTOR, a standard label selector (for example
// `workload=student` or `env in (student,course)`). Checks use it to ignore
// problems in namespaces they aren't responsible for, such as ones that hold
// user workloads. The result is empty when the variable is unset, so by
// default nothing is excluded.
//
// An unparseable selector fails setup rather than being ignored, since
// silently excluding nothing would hide the typo. An empty selector must not
// be handed to labels.Parse, which treats it as matching everything.
func listExcludedNamespaces(ctx context.Context) map[string]struct{} {
	GinkgoHelper()

	excluded := map[string]struct{}{}
	raw := GetEnvWithDefault("EXCLUDE_NAMESPACE_SELECTOR", "")
	if raw == "" {
		return excluded
	}

	selector, err := labels.Parse(raw)
	ExpectNoError(err, "parse EXCLUDE_NAMESPACE_SELECTOR=%q as a label selector", raw)

	err = EachItem(ctx, CoreClient.CoreV1().Namespaces().List, metav1.ListOptions{LabelSelector: selector.String()},
		func(namespace *corev1.Namespace) error {
			excluded[namespace.Name] = struct{}{}
			return nil
		})
	ExpectNoError(err, "list namespaces matching EXCLUDE_NAMESPACE_SELECTOR")
	return excluded
}

// IsExcludedNamespace reports whether namespace matches
// EXCLUDE_NAMESPACE_SELECTOR, and so should be skipped by checks that span all
// namespaces. Cluster-scoped objects have no namespace and are never excluded.
func IsExcludedNamespace(namespace string) bool {
	_, excluded := excludedNamespaceNames[namespace]
	return excluded
}

// DefaultStorageClassName returns the name of the cluster's default
// StorageClass, or "" if none is marked default. A PersistentVolumeClaim
// with no StorageClassName of its own resolves to this one.
func DefaultStorageClassName() string {
	for _, class := range clusterStorageClasses {
		if class.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			return class.Name
		}
	}
	return ""
}

// StorageClassIsWaitForFirstConsumer reports whether the named StorageClass
// has VolumeBindingMode: WaitForFirstConsumer, meaning a PVC on this class is
// deliberately left Pending until a pod that uses it is scheduled, rather
// than being bound as soon as it's created. A StorageClass with no
// VolumeBindingMode set, or no StorageClass by this name at all, reports
// false: VolumeBindingImmediate is both the API's default when the field is
// omitted and the correct answer when the class can't be found.
func StorageClassIsWaitForFirstConsumer(name string) bool {
	for _, class := range clusterStorageClasses {
		if class.Name == name {
			return class.VolumeBindingMode != nil &&
				*class.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
		}
	}
	return false
}
