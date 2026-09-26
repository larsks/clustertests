package cluster_tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var (
	dynamicClient dynamic.Interface
	coreClient    kubernetes.Interface
)

// hasCondition reports whether conditions contains the requested type and status.
// fieldValues extracts the type and status from each condition.
func hasCondition[C any, T ~string, S ~string](
	conditions []C,
	conditionType T,
	status S,
	fieldValues func(C) (T, S),
) bool {
	for _, condition := range conditions {
		gotType, gotStatus := fieldValues(condition)
		if gotType == conditionType && gotStatus == status {
			return true
		}
	}
	return false
}

// deploymentIsAvailable reports whether the specified deployment has an Available=True condition.
func deploymentIsAvailable(deployment *v1.Deployment) {
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

func deploymentsAreAvailable(ctx context.Context, namespace string, names []string) {
	GinkgoHelper()
	for _, name := range names {
		deployment, err := coreClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		deploymentIsAvailable(deployment)
	}
}

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

func daemonsetsAreAvailable(ctx context.Context, namespace string, names []string) {
	GinkgoHelper()
	for _, name := range names {
		daemonset, err := coreClient.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		daemonsetIsAvailable(ctx, daemonset)
	}
}
