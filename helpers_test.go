package clustertests

import (
	"context"
	"fmt"
	"os"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/pager"
)

var (
	dynamicClient dynamic.Interface
	coreClient    kubernetes.Interface
)

// listPageSize bounds how many items are fetched per page for any List call
// in this suite that isn't restricted to a single namespace, so a check
// doesn't pull an entire large cluster's worth of objects into memory, or
// into one oversized request, at once.
const listPageSize = 500

// eachResource pages through every object of gvr matching opts and calls fn
// for each one, instead of fetching the whole list at once. For a namespaced
// resource, omitting a namespace restriction in opts (as every caller here
// does) spans all namespaces.
func eachResource(
	ctx context.Context,
	gvr schema.GroupVersionResource,
	opts metav1.ListOptions,
	fn func(*unstructured.Unstructured) error,
) error {
	listPages := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return dynamicClient.Resource(gvr).List(ctx, opts)
	})
	listPages.PageSize = listPageSize
	return listPages.EachListItem(ctx, opts, func(obj runtime.Object) error {
		return fn(obj.(*unstructured.Unstructured))
	})
}

// listNodes pages through nodes matching opts and returns them all. Nodes are
// cluster-scoped, so this always spans the whole cluster.
func listNodes(ctx context.Context, opts metav1.ListOptions) ([]corev1.Node, error) {
	listPages := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return coreClient.CoreV1().Nodes().List(ctx, opts)
	})
	listPages.PageSize = listPageSize

	var nodes []corev1.Node
	err := listPages.EachListItem(ctx, opts, func(obj runtime.Object) error {
		nodes = append(nodes, *obj.(*corev1.Node))
		return nil
	})
	return nodes, err
}

// cachedNodes holds every node in the cluster, fetched once per test process.
// A nil value means the fetch hasn't happened yet. Every check that needs
// nodes, whether all of them or a label-selected subset, goes through this
// one cache instead of each making its own List call: node counts are small
// enough (unlike pods, which are streamed instead of cached) that holding the
// full list in memory is cheap, and every consumer just filters it locally.
var cachedNodes *[]corev1.Node

// allNodes returns every node in the cluster, fetching and caching the full,
// unfiltered list on first use. Like the other per-process caches in this
// file, this is a snapshot from the first call: a node that joins, leaves, or
// gets relabeled later in the same run won't be reflected.
func allNodes(ctx context.Context) []corev1.Node {
	GinkgoHelper()

	if cachedNodes == nil {
		nodes, err := listNodes(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred(), "list nodes")
		cachedNodes = &nodes
	}
	return *cachedNodes
}

// nodesMatching filters allNodes(ctx) by selector, without an API call of its
// own.
func nodesMatching(ctx context.Context, selector labels.Selector) []corev1.Node {
	GinkgoHelper()

	var matched []corev1.Node
	for _, node := range allNodes(ctx) {
		if selector.Matches(labels.Set(node.Labels)) {
			matched = append(matched, node)
		}
	}
	return matched
}

// resourceKindExists caches whether the API server serves each resource type
// at all, as distinct from whether any instances of it currently exist. Only
// definitive answers are cached; a failed lookup fails the spec and is
// retried by the next one. Ginkgo runs parallel specs in separate processes,
// so no locking is needed.
var resourceKindExists = map[schema.GroupVersionResource]bool{}

// skipIfResourceKindDoesNotExist skips the current spec if the API server
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
func skipIfResourceKindDoesNotExist(gvr schema.GroupVersionResource) {
	GinkgoHelper()

	exists, cached := resourceKindExists[gvr]
	if !cached {
		resources, err := coreClient.Discovery().ServerResourcesForGroupVersion(gvr.GroupVersion().String())
		if err != nil && !apierrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred(), "discover %s", gvr.GroupVersion())
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

// conditionReady is the name of the standard readiness condition used by
// Certificates, SecretStores, ExternalSecrets, and most other CRDs.
const conditionReady = "Ready"

// resourceID returns namespace/name for namespaced resources and just the name
// for cluster-scoped ones, so that same-named resources in different
// namespaces can be told apart in failure messages.
func resourceID(obj *unstructured.Unstructured) string {
	if namespace := obj.GetNamespace(); namespace != "" {
		return namespace + "/" + obj.GetName()
	}
	return obj.GetName()
}

// objectID is resourceID for a typed Kubernetes object instead of an
// unstructured one. Any *appsv1.Deployment, *appsv1.StatefulSet or
// *appsv1.DaemonSet satisfies metav1.Object through its embedded ObjectMeta.
func objectID(obj metav1.Object) string {
	if namespace := obj.GetNamespace(); namespace != "" {
		return namespace + "/" + obj.GetName()
	}
	return obj.GetName()
}

// conditionsOf decodes status.conditions from any resource into
// metav1.Condition. Projects define their own condition types, but they all
// serialize to the same JSON shape, so this gives a single type for checking
// conditions regardless of which project owns the resource.
func conditionsOf(obj *unstructured.Unstructured) ([]metav1.Condition, error) {
	raw, _, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil {
		return nil, err
	}

	conditions := make([]metav1.Condition, len(raw))
	for i, item := range raw {
		fields, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("status.conditions[%d] is %T, not an object", i, item)
		}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(fields, &conditions[i]); err != nil {
			return nil, fmt.Errorf("status.conditions[%d]: %w", i, err)
		}
	}
	return conditions, nil
}

// conditionExpectation names a condition type and the status it must have.
type conditionExpectation struct {
	Type   string
	Status metav1.ConditionStatus
}

// expectConditions lists every resource of the given type across all
// namespaces and fails unless each has every expected condition with the
// expected status. A missing condition counts as a failure. Offenders are
// reported as namespace/name along with the condition's reason and message. A
// condition whose observedGeneration is older than the resource's generation
// is treated as stale. It returns the number of resources checked so callers
// can insist that at least one exists.
func expectConditions(ctx context.Context, gvr schema.GroupVersionResource, expected ...conditionExpectation) int {
	GinkgoHelper()

	var problems []string
	count := 0
	err := eachResource(ctx, gvr, metav1.ListOptions{}, func(obj *unstructured.Unstructured) error {
		count++
		id := resourceID(obj)

		conditions, err := conditionsOf(obj)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", id, err))
			return nil
		}

		for _, want := range expected {
			condition := apimeta.FindStatusCondition(conditions, want.Type)
			switch {
			case condition == nil:
				problems = append(problems, fmt.Sprintf("%s: no %s condition", id, want.Type))
			case condition.Status != want.Status:
				problems = append(problems, fmt.Sprintf(
					"%s: %s=%s, want %s (%s: %s)",
					id, want.Type, condition.Status, want.Status, condition.Reason, condition.Message,
				))
			case condition.ObservedGeneration != 0 && condition.ObservedGeneration < obj.GetGeneration():
				problems = append(problems, fmt.Sprintf(
					"%s: %s is stale (observed generation %d, current generation %d)",
					id, want.Type, condition.ObservedGeneration, obj.GetGeneration(),
				))
			}
		}
		return nil
	})
	Expect(err).NotTo(HaveOccurred(), "list %s", gvr.Resource)

	slices.Sort(problems)
	Expect(problems).To(BeEmpty())
	return count
}

// expectAllReady fails unless every resource of the given type has a condition
// of type condType with status True.
func expectAllReady(ctx context.Context, gvr schema.GroupVersionResource, condType string) {
	GinkgoHelper()
	expectConditions(ctx, gvr, conditionExpectation{Type: condType, Status: metav1.ConditionTrue})
}

// deploymentCondition returns the named condition, or nil if the deployment
// doesn't report one.
func deploymentCondition(
	deployment *appsv1.Deployment, conditionType appsv1.DeploymentConditionType,
) *appsv1.DeploymentCondition {
	for i, condition := range deployment.Status.Conditions {
		if condition.Type == conditionType {
			return &deployment.Status.Conditions[i]
		}
	}
	return nil
}

// deploymentIsAvailable asserts whether the specified deployment has an Available=True condition.
func deploymentIsAvailable(deployment *appsv1.Deployment) {
	GinkgoHelper()

	id := objectID(deployment)

	Expect(deployment.Spec.Replicas).ToNot(BeNil())
	desired := *deployment.Spec.Replicas
	Expect(desired).To(BeNumerically(">", 0), "deployment %s is scaled to 0 replicas", id)

	available := deploymentCondition(deployment, appsv1.DeploymentAvailable)
	Expect(available).NotTo(BeNil(), "deployment %s has no Available condition", id)
	Expect(available.Status).To(Equal(corev1.ConditionTrue), "deployment %s is not available", id)

	// Progressing=True is the normal steady state once a rollout has ever
	// succeeded; it only turns False, with this reason, when a rollout has
	// been stuck longer than spec.progressDeadlineSeconds (600s by default).
	if progressing := deploymentCondition(deployment, appsv1.DeploymentProgressing); progressing != nil {
		stuck := progressing.Status == corev1.ConditionFalse && progressing.Reason == "ProgressDeadlineExceeded"
		Expect(stuck).To(BeFalse(), "deployment %s rollout is stuck: %s", id, progressing.Message)
	}

	Expect(deployment.Status.ObservedGeneration).To(
		BeNumerically(">=", deployment.Generation),
		"deployment %s status has not observed its current generation", id,
	)
	Expect(deployment.Status.AvailableReplicas).To(
		Equal(desired),
		"deployment %s available replicas", id,
	)
	Expect(deployment.Status.UpdatedReplicas).To(
		Equal(desired),
		"deployment %s updated replicas", id,
	)
}

func deploymentIsAvailableByName(ctx context.Context, namespace, name string) {
	GinkgoHelper()
	deployment, err := coreClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	deploymentIsAvailable(deployment)
}

// deploymentIsAvailableIfPresent behaves like deploymentIsAvailableByName,
// except it skips the spec instead of failing if the deployment doesn't
// exist at all. Use it for a component that some install methods bundle (for
// example, an OpenShift operator) and others don't (for example, a plain
// Helm install).
func deploymentIsAvailableIfPresent(ctx context.Context, namespace, name string) {
	GinkgoHelper()
	deployment, err := coreClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		Skip(fmt.Sprintf("deployment %s/%s not found", namespace, name))
	}
	Expect(err).NotTo(HaveOccurred())
	deploymentIsAvailable(deployment)
}

// envOrDefault returns the value of the named environment variable, or def
// if it's unset or empty.
func envOrDefault(name, def string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return def
}

func statefulSetIsAvailable(statefulSet *appsv1.StatefulSet) {
	GinkgoHelper()

	id := objectID(statefulSet)

	Expect(statefulSet.Status.ObservedGeneration).To(
		BeNumerically(">=", statefulSet.Generation),
		"statefulset %s status has not observed its current generation", id,
	)
	Expect(statefulSet.Spec.Replicas).NotTo(BeNil())
	desired := *statefulSet.Spec.Replicas
	Expect(statefulSet.Status.ReadyReplicas).To(
		Equal(desired),
		"statefulset %s ready replicas", id,
	)
	Expect(statefulSet.Status.UpdatedReplicas).To(
		Equal(desired),
		"statefulset %s updated replicas", id,
	)
}

func statefulSetIsAvailableByName(ctx context.Context, namespace, name string) {
	GinkgoHelper()
	statefulSet, err := coreClient.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred(), "get statefulset %q in namespace %q", name, namespace)
	statefulSetIsAvailable(statefulSet)
}

/*
* daemonsetIsAvailable verifies that the number of active instances of a
* daemonset matches the number of expected instances based on its
* NodeSelector. We are explicitly trying to identify daemonsets that should be
* scheduled on a set of nodes but are not because the nodes are tainted and the
* daemonset pods are missing the appropriate tolerations: DesiredNumberScheduled
* already excludes a taint-blocked node, so comparing it against our own count
* of label-matching nodes is what catches that case.
*
* Two known limits, left as-is rather than "fixed" incorrectly:
*   - This matches on NodeSelector only, not NodeAffinity. A DaemonSet that
*     picks its nodes via NodeAffinity instead could be miscounted. None of
*     the DaemonSets this suite currently checks do that (confirmed against a
*     live NVIDIA GPU Operator install).
*   - NotReady and cordoned (Unschedulable) nodes are deliberately still
*     counted: the DaemonSet controller tolerates both by default, so a
*     healthy DaemonSet's DesiredNumberScheduled includes them too. Excluding
*     them here would cause false failures, not fix anything.
*
* A DaemonSet legitimately scoped to hardware that isn't present anywhere in
* the cluster (for example nvidia-device-plugin-mps-control-daemon on a
* cluster with no MPS-capable nodes) can correctly have
* DesiredNumberScheduled, NumberAvailable and expected all at 0; that's not
* itself treated as a failure.
 */
func daemonsetIsAvailable(ctx context.Context, daemonset *appsv1.DaemonSet) {
	GinkgoHelper()

	id := objectID(daemonset)

	selector := labels.Set(daemonset.Spec.Template.Spec.NodeSelector).AsSelector()
	expected := len(nodesMatching(ctx, selector))

	Expect(daemonset.Status.ObservedGeneration).To(
		BeNumerically(">=", daemonset.Generation),
		"daemonset %s status has not observed its current generation", id,
	)
	Expect(daemonset.Status.NumberMisscheduled).To(
		BeNumerically("==", 0),
		"daemonset %s has pods running on nodes it shouldn't", id,
	)
	Expect(daemonset.Status.UpdatedNumberScheduled).To(
		Equal(daemonset.Status.DesiredNumberScheduled),
		"daemonset %s rollout is incomplete", id,
	)
	// NumberReady isn't checked separately: Available pods are a subset of
	// Ready ones, so NumberAvailable == DesiredNumberScheduled below already
	// implies NumberReady == DesiredNumberScheduled.
	Expect(daemonset.Status.NumberAvailable).To(
		Equal(daemonset.Status.DesiredNumberScheduled),
		"daemonset %s is not fully available", id,
	)
	Expect(daemonset.Status.NumberAvailable).To(
		BeNumerically("==", expected),
		"daemonset %s available count does not match nodes selected by its NodeSelector "+
			"(a mismatch here usually means a taint without a matching toleration)",
		id,
	)
}

func daemonsetIsAvailableByName(ctx context.Context, namespace, name string) {
	GinkgoHelper()
	daemonset, err := coreClient.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	daemonsetIsAvailable(ctx, daemonset)
}
