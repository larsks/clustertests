package testutil

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// DeploymentCondition returns the named condition, or nil if the deployment
// doesn't report one.
func DeploymentCondition(
	deployment *appsv1.Deployment, conditionType appsv1.DeploymentConditionType,
) *appsv1.DeploymentCondition {
	for i, condition := range deployment.Status.Conditions {
		if condition.Type == conditionType {
			return &deployment.Status.Conditions[i]
		}
	}
	return nil
}

// DeploymentRolloutStuck reports whether the deployment's rollout has given
// up, along with the Progressing condition's message explaining why (empty if
// it hasn't). Progressing=True is the normal steady state once a rollout has
// ever succeeded; it only turns False, with the ProgressDeadlineExceeded
// reason, when a rollout has been stuck longer than
// spec.progressDeadlineSeconds (600s by default).
func DeploymentRolloutStuck(deployment *appsv1.Deployment) (message string, stuck bool) {
	progressing := DeploymentCondition(deployment, appsv1.DeploymentProgressing)
	if progressing != nil && progressing.Status == corev1.ConditionFalse &&
		progressing.Reason == "ProgressDeadlineExceeded" {
		return progressing.Message, true
	}
	return "", false
}

// DesiredReplicas returns the replica count that a Deployment or StatefulSet
// asks for. spec.replicas defaults to 1 when unset.
func DesiredReplicas(replicas *int32) int32 {
	if replicas == nil {
		return 1
	}
	return *replicas
}

// deploymentIsAvailable asserts whether the specified deployment has an Available=True condition.
func deploymentIsAvailable(deployment *appsv1.Deployment) {
	GinkgoHelper()

	id := ObjectID(deployment)

	desired := DesiredReplicas(deployment.Spec.Replicas)
	Expect(desired).To(BeNumerically(">", 0), "deployment %s is scaled to 0 replicas", id)

	available := DeploymentCondition(deployment, appsv1.DeploymentAvailable)
	Expect(available).NotTo(BeNil(), "deployment %s has no Available condition", id)
	Expect(available.Status).To(Equal(corev1.ConditionTrue), "deployment %s is not available", id)

	stuckMessage, stuck := DeploymentRolloutStuck(deployment)
	Expect(stuck).To(BeFalse(), "deployment %s rollout is stuck: %s", id, stuckMessage)

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

func DeploymentIsAvailableByName(ctx context.Context, namespace, name string) {
	GinkgoHelper()
	deployment, err := CoreClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	ExpectNoError(err)
	deploymentIsAvailable(deployment)
}

// DeploymentIsAvailableIfPresent behaves like DeploymentIsAvailableByName,
// except it skips the spec instead of failing if the deployment doesn't
// exist at all. Use it for a component that some install methods bundle (for
// example, an OpenShift operator) and others don't (for example, a plain
// Helm install).
func DeploymentIsAvailableIfPresent(ctx context.Context, namespace, name string) {
	GinkgoHelper()
	deployment, err := CoreClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		Skip(fmt.Sprintf("deployment %s/%s not found", namespace, name))
	}
	ExpectNoError(err)
	deploymentIsAvailable(deployment)
}

func statefulSetIsAvailable(statefulSet *appsv1.StatefulSet) {
	GinkgoHelper()

	id := ObjectID(statefulSet)

	Expect(statefulSet.Status.ObservedGeneration).To(
		BeNumerically(">=", statefulSet.Generation),
		"statefulset %s status has not observed its current generation", id,
	)
	desired := DesiredReplicas(statefulSet.Spec.Replicas)
	Expect(statefulSet.Status.ReadyReplicas).To(
		Equal(desired),
		"statefulset %s ready replicas", id,
	)
	Expect(statefulSet.Status.UpdatedReplicas).To(
		Equal(desired),
		"statefulset %s updated replicas", id,
	)
}

func StatefulSetIsAvailableByName(ctx context.Context, namespace, name string) {
	GinkgoHelper()
	statefulSet, err := CoreClient.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	ExpectNoError(err, "get statefulset %q in namespace %q", name, namespace)
	statefulSetIsAvailable(statefulSet)
}

/*
* DaemonsetIsAvailable verifies that the number of active instances of a
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
func DaemonsetIsAvailable(daemonset *appsv1.DaemonSet) {
	GinkgoHelper()

	id := ObjectID(daemonset)

	selector := labels.Set(daemonset.Spec.Template.Spec.NodeSelector).AsSelector()
	expected := len(NodesMatching(selector))

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
	daemonset, err := CoreClient.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	ExpectNoError(err)
	DaemonsetIsAvailable(daemonset)
}
