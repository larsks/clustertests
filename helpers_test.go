package cluster_tests

import (
	"context"
	"fmt"
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
)

var (
	dynamicClient dynamic.Interface
	coreClient    kubernetes.Interface
)

// namespaceExists caches whether each namespace exists so that the guard
// runs before every spec without repeating the API call. Only definitive
// answers are cached; a failed lookup fails the spec and is retried by the
// next one. Ginkgo runs parallel specs in separate processes, so no locking
// is needed.
var namespaceExists = map[string]bool{}

// skipIfNamespaceDoesNotExist skips the current spec if the namespace does not
// exist. The answer is looked up once per test process.
func skipIfNamespaceDoesNotExist(ctx context.Context, namespace string) {
	GinkgoHelper()

	exists, cached := namespaceExists[namespace]
	if !cached {
		_, err := coreClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred(), "get namespace %q", namespace)
		}
		exists = err == nil
		namespaceExists[namespace] = exists
	}

	if !exists {
		Skip(fmt.Sprintf("namespace %q does not exist", namespace))
	}
}

// conditionReady is the name of the standard readiness condition used by
// Certificates, SecretStores, ExternalSecrets, and most other CRDs.
const conditionReady = "Ready"

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

// expectAllReady lists every resource of the given type across all namespaces
// and fails unless each has a condition of type condType with status True.
// Offenders are reported as namespace/name along with the condition's reason
// and message. A condition whose observedGeneration is older than the
// resource's generation is treated as stale.
func expectAllReady(ctx context.Context, gvr schema.GroupVersionResource, condType string) {
	GinkgoHelper()

	list, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred(), "list %s", gvr.Resource)

	var problems []string
	for i := range list.Items {
		obj := &list.Items[i]
		id := obj.GetName()
		if namespace := obj.GetNamespace(); namespace != "" {
			id = namespace + "/" + id
		}

		conditions, err := conditionsOf(obj)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", id, err))
			continue
		}

		condition := apimeta.FindStatusCondition(conditions, condType)
		switch {
		case condition == nil:
			problems = append(problems, fmt.Sprintf("%s: no %s condition", id, condType))
		case condition.Status != metav1.ConditionTrue:
			problems = append(problems, fmt.Sprintf(
				"%s: %s=%s (%s: %s)", id, condType, condition.Status, condition.Reason, condition.Message,
			))
		case condition.ObservedGeneration != 0 && condition.ObservedGeneration < obj.GetGeneration():
			problems = append(problems, fmt.Sprintf(
				"%s: %s is stale (observed generation %d, current generation %d)",
				id, condType, condition.ObservedGeneration, obj.GetGeneration(),
			))
		}
	}

	slices.Sort(problems)
	Expect(problems).To(BeEmpty())
}

// deploymentIsAvailable reports whether the specified deployment has an Available=True condition.
func deploymentIsAvailable(deployment *appsv1.Deployment) {
	GinkgoHelper()
	var available bool
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable {
			Expect(condition.Status).
				To(Equal(corev1.ConditionTrue), "deployment %q is not available", deployment.Name)
			available = true
			break
		}
	}

	if !available {
		Fail(fmt.Sprintf("deployment %q has no Available condition", deployment.Name))
	}

	Expect(deployment.Status.ObservedGeneration).To(
		BeNumerically(">=", deployment.Generation),
	)
	Expect(deployment.Spec.Replicas).ToNot(BeNil())
	desired := *deployment.Spec.Replicas
	Expect(deployment.Status.AvailableReplicas).To(Equal(desired))
	Expect(deployment.Status.UpdatedReplicas).To(Equal(desired))
}

func deploymentIsAvailableByName(ctx context.Context, namespace, name string) {
	GinkgoHelper()
	deployment, err := coreClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	deploymentIsAvailable(deployment)
}

func statefulSetIsAvailable(statefulSet *appsv1.StatefulSet) {
	GinkgoHelper()
	Expect(statefulSet.Status.ObservedGeneration).To(
		BeNumerically(">=", statefulSet.Generation),
		"statefulset %q status has not observed its current generation", statefulSet.Name,
	)
	Expect(statefulSet.Spec.Replicas).NotTo(BeNil())
	desired := *statefulSet.Spec.Replicas
	Expect(statefulSet.Status.ReadyReplicas).To(
		Equal(desired),
		"statefulset %q ready replicas", statefulSet.Name,
	)
	Expect(statefulSet.Status.UpdatedReplicas).To(
		Equal(desired),
		"statefulset %q updated replicas", statefulSet.Name,
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
* daemonset matches the number of expected instances based on it's
* NodeSelector. We are explicitly trying to identify daemonsets that should be
* scheduled on a set of nodes but are not because the nodes are tainted and the
* daemonset pods are missing the appropriate tolerations.
 */
func daemonsetIsAvailable(ctx context.Context, daemonset *appsv1.DaemonSet) {
	GinkgoHelper()
	selector := labels.Set(daemonset.Spec.Template.Spec.NodeSelector).AsSelector()
	nodes, err := coreClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	Expect(err).NotTo(HaveOccurred())
	expected := len(nodes.Items)

	Expect(daemonset.Status.NumberAvailable).To(Equal(daemonset.Status.DesiredNumberScheduled))
	Expect(daemonset.Status.NumberAvailable).To(BeNumerically("==", expected))
}

func daemonsetIsAvailableByName(ctx context.Context, namespace, name string) {
	GinkgoHelper()
	daemonset, err := coreClient.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	daemonsetIsAvailable(ctx, daemonset)
}
