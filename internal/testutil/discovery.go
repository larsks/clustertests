package testutil

import (
	"fmt"
	"slices"

	. "github.com/onsi/ginkgo/v2"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// resourceKindExists caches whether the API server serves each resource type
// at all, as distinct from whether any instances of it currently exist. Only
// definitive answers are cached; a failed lookup fails the spec and is
// retried by the next one. Ginkgo runs parallel specs in separate processes,
// so no locking is needed.
var resourceKindExists = map[schema.GroupVersionResource]bool{}

// SkipIfResourceKindDoesNotExist skips the current spec if the API server
// does not serve the given resource type at all. This is how each optional
// operator suite (ArgoCD, cert-manager, external-secrets, ...) detects
// whether that operator is installed on this cluster, by checking for one of
// the CRDs it owns.
//
// A CRD is a better signal for this than the operator's namespace: the CRD is
// created once, at install time, and normal operator trouble (a crashed
// controller, a deleted namespace, a deployment scaled to 0) doesn't remove
// it. So a missing CRD means the operator was never installed here, while a
// present CRD with a broken deployment or a missing namespace is a real
// failure the specs below will still catch, since they no longer get
// skipped for reasons the CRD guard didn't intend. The one case this doesn't
// distinguish is a full uninstall that also deletes the CRDs, which then
// looks the same as never having been installed; that's a much more
// deliberate action than a namespace merely disappearing, so this narrows
// the blind spot without eliminating it.
//
// Existence is checked via discovery (/apis/<group>/<version>), not a List
// call. Discovery is covered by the system:discovery ClusterRole granted to
// every authenticated user by default, so this works even for an identity
// with no list permission on the resource itself. A List call doesn't have
// that property: RBAC is checked before the API server would report that the
// resource type doesn't exist, so a plain Forbidden (a missing grant, not a
// missing resource) would otherwise be indistinguishable from a genuinely
// absent CRD, and would fail every spec in the suite instead of skipping
// them.
func SkipIfResourceKindDoesNotExist(gvr schema.GroupVersionResource) {
	GinkgoHelper()

	exists, cached := resourceKindExists[gvr]
	if !cached {
		resources, err := CoreClient.Discovery().ServerResourcesForGroupVersion(gvr.GroupVersion().String())
		if err != nil && !apierrors.IsNotFound(err) {
			ExpectNoError(err, "discover %s", gvr.GroupVersion())
		}
		exists = err == nil && slices.ContainsFunc(resources.APIResources, func(r metav1.APIResource) bool {
			return r.Name == gvr.Resource
		})
		resourceKindExists[gvr] = exists
	}

	if !exists {
		Skip(fmt.Sprintf("resource type %q is not served by this API server", gvr.Resource))
	}
}
