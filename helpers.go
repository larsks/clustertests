package cluster_tests

import (
	"context"

	//. "github.com/onsi/ginkgo/v2"
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

// deploymentIsAvailable reports whether the specified deployment has an Available=True condition.
func deploymentIsAvailable(deployment *v1.Deployment) {
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable {
			Expect(condition.Status).To(Equal(corev1.ConditionTrue))
		}
	}
}

func deploymentsAreAvailable(ctx context.Context, namespace string, deploymentNames []string) {
	for _, deploymentName := range deploymentNames {
		deployment, err := coreClient.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		deploymentIsAvailable(deployment)
	}
}

func daemonsetIsAvailable(ctx context.Context, daemonset *appsv1.DaemonSet) bool {
	selector := labels.Set(daemonset.Spec.Template.Spec.NodeSelector).AsSelector()
	nodes, err := coreClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return false
	}
	expected := len(nodes.Items)

	if daemonset.Status.NumberAvailable != daemonset.Status.DesiredNumberScheduled {
		return false
	}

	if int(daemonset.Status.NumberAvailable) != expected {
		return false
	}

	return true
}

func daemonsetsAreAvailable(ctx context.Context, namespace string, names []string) (bool, []string) {
	var problems []string

	for _, name := range names {
		daemonset, err := coreClient.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil || !daemonsetIsAvailable(ctx, daemonset) {
			problems = append(problems, name)
		}
	}

	return len(problems) == 0, problems
}
