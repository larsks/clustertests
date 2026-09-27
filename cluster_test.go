package cluster_tests

import (
	"fmt"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	clusterOperatorGVR = schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "clusteroperators",
	}
	clusterVersionGVR = schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "clusterversions",
	}
)

var _ = Describe("cluster health", func() {
	It("requires every node to be schedulable, Ready, and free of resource pressure", Label("nodes"), func(ctx SpecContext) {
		nodes, err := coreClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred(), "list nodes")
		Expect(nodes.Items).NotTo(BeEmpty(), "no nodes found")

		var problems []string
		for _, node := range nodes.Items {
			var issues []string

			if node.Spec.Unschedulable {
				issues = append(issues, "Unschedulable")
			}

			readyStatus, hasReadyCondition := nodeConditionStatus(node, corev1.NodeReady)
			if !hasReadyCondition || readyStatus != corev1.ConditionTrue {
				if !hasReadyCondition {
					readyStatus = "<missing>"
				}
				issues = append(issues, fmt.Sprintf("Ready=%s", readyStatus))
			}

			for _, pressureType := range []corev1.NodeConditionType{
				corev1.NodeMemoryPressure,
				corev1.NodeDiskPressure,
				corev1.NodePIDPressure,
			} {
				status, exists := nodeConditionStatus(node, pressureType)
				if exists && status == corev1.ConditionTrue {
					issues = append(issues, fmt.Sprintf("%s=True", pressureType))
				}
			}

			if len(issues) > 0 {
				problems = append(problems, fmt.Sprintf("%s (%s)", node.Name, strings.Join(issues, ", ")))
			}
		}

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})

	It("requires every PersistentVolumeClaim to be Bound", Label("storage"), func(ctx SpecContext) {
		claims, err := coreClient.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred(), "list PersistentVolumeClaims across all namespaces")

		var problems []string
		for _, claim := range claims.Items {
			if claim.Status.Phase != corev1.ClaimBound {
				phase := string(claim.Status.Phase)
				if phase == "" {
					phase = "<missing>"
				}
				problems = append(problems, fmt.Sprintf("%s/%s: phase=%s", claim.Namespace, claim.Name, phase))
			}
		}

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})

	It("requires every ClusterOperator to be Available and not Degraded", Label("cluster-operators"), func(ctx SpecContext) {
		checked := expectConditions(ctx, clusterOperatorGVR,
			conditionExpectation{Type: "Available", Status: metav1.ConditionTrue},
			conditionExpectation{Type: "Degraded", Status: metav1.ConditionFalse},
		)
		Expect(checked).NotTo(BeZero(), "no ClusterOperators found")
	})

	It("requires ClusterVersion to be Available and not Failing", Label("cluster-version"), func(ctx SpecContext) {
		checked := expectConditions(ctx, clusterVersionGVR,
			conditionExpectation{Type: "Available", Status: metav1.ConditionTrue},
			conditionExpectation{Type: "Failing", Status: metav1.ConditionFalse},
		)
		Expect(checked).NotTo(BeZero(), "no ClusterVersion found")
	})
})

func nodeConditionStatus(node corev1.Node, conditionType corev1.NodeConditionType) (corev1.ConditionStatus, bool) {
	for _, condition := range node.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status, true
		}
	}
	return "", false
}
