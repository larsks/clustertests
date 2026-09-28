package clustertests

import (
	"fmt"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	hostedClusterGVR = schema.GroupVersionResource{
		Group:    "hypershift.openshift.io",
		Version:  "v1beta1",
		Resource: "hostedclusters",
	}
	hostedControlPlaneGVR = schema.GroupVersionResource{
		Group:    "hypershift.openshift.io",
		Version:  "v1beta1",
		Resource: "hostedcontrolplanes",
	}
	nodePoolGVR = schema.GroupVersionResource{
		Group:    "hypershift.openshift.io",
		Version:  "v1beta1",
		Resource: "nodepools",
	}
)

// These checks run against a management cluster running the HyperShift
// operator, and cover the hosted clusters it manages. None of them requires a
// hosted cluster to exist: a management cluster that hasn't created any yet
// is healthy.
var _ = Describe("HostedControlPlanes", Label("hcp"), func() {
	BeforeEach(func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(hostedClusterGVR)
	})

	// Degraded is checked along with Available because they catch different
	// failures. Available reflects whether the hosted cluster's API server is
	// reachable. Degraded is copied up from the HostedControlPlane, and is
	// True when any control plane component deployment has unavailable
	// replicas, which can happen while the API server is still up.
	It("has available, non-degraded HostedClusters", func(ctx SpecContext) {
		expectConditions(ctx, hostedClusterGVR,
			conditionExpectation{Type: "Available", Status: metav1.ConditionTrue},
			conditionExpectation{Type: "Degraded", Status: metav1.ConditionFalse},
		)
	})

	// A HostedControlPlane lives in the per-cluster namespace on the
	// management cluster, and is what actually runs the hosted cluster's
	// control plane components. HostedCluster reports on it, but only what
	// it chooses to copy up.
	It("has available, non-degraded HostedControlPlanes", func(ctx SpecContext) {
		expectConditions(ctx, hostedControlPlaneGVR,
			conditionExpectation{Type: "Available", Status: metav1.ConditionTrue},
			conditionExpectation{Type: "Degraded", Status: metav1.ConditionFalse},
		)
	})

	// Ready is True when every replica is a Ready node. The NodePool also
	// reports AllMachinesReady and AllNodesHealthy, but those only explain a
	// Ready=False, so they aren't checked separately.
	//
	// status.replicas is compared against spec.replicas, which catches a pool
	// that has fewer (or more) machines than requested. A NodePool with
	// autoscaling enabled has no spec.replicas, because the API doesn't allow
	// both, so its replica count is only required to fall within the
	// autoscaler's bounds.
	It("has ready NodePools with the desired number of replicas", func(ctx SpecContext) {
		problems, _, err := conditionProblems(ctx, nodePoolGVR,
			conditionExpectation{Type: "Ready", Status: metav1.ConditionTrue},
		)
		Expect(err).NotTo(HaveOccurred(), "list %s", nodePoolGVR.Resource)

		err = eachResource(ctx, nodePoolGVR, metav1.ListOptions{}, func(pool *unstructured.Unstructured) error {
			if problem := nodePoolReplicaProblem(pool); problem != "" {
				problems = append(problems, problem)
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list %s", nodePoolGVR.Resource)

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})
})

// nodePoolReplicaProblem describes how a NodePool's status.replicas differs
// from what its spec asks for, or returns "" if it matches. status.replicas
// is a required field, so a missing one is read as zero.
func nodePoolReplicaProblem(pool *unstructured.Unstructured) string {
	id := objectID(pool)

	actual, _, err := unstructured.NestedInt64(pool.Object, "status", "replicas")
	if err != nil {
		return fmt.Sprintf("%s: %v", id, err)
	}

	desired, hasDesired, err := unstructured.NestedInt64(pool.Object, "spec", "replicas")
	if err != nil {
		return fmt.Sprintf("%s: %v", id, err)
	}
	if hasDesired {
		if actual != desired {
			return fmt.Sprintf("%s: status.replicas=%d, want spec.replicas=%d", id, actual, desired)
		}
		return ""
	}

	minimum, hasMin, err := unstructured.NestedInt64(pool.Object, "spec", "autoScaling", "min")
	if err != nil {
		return fmt.Sprintf("%s: %v", id, err)
	}
	maximum, hasMax, err := unstructured.NestedInt64(pool.Object, "spec", "autoScaling", "max")
	if err != nil {
		return fmt.Sprintf("%s: %v", id, err)
	}
	if hasMin && hasMax && (actual < minimum || actual > maximum) {
		return fmt.Sprintf(
			"%s: status.replicas=%d, outside autoscaling range %d-%d", id, actual, minimum, maximum,
		)
	}
	return ""
}
