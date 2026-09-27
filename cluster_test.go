package clustertests

import (
	"context"
	"fmt"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	certificatesv1 "k8s.io/api/certificates/v1"
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
	machineConfigPoolGVR = schema.GroupVersionResource{
		Group:    "machineconfiguration.openshift.io",
		Version:  "v1",
		Resource: "machineconfigpools",
	}
)

var _ = Describe("cluster health", func() {
	It("requires every node to be schedulable, Ready, and free of resource pressure or network problems", Label("nodes"), func(ctx SpecContext) {
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

			// These conditions are normally absent or False; a healthy node
			// simply doesn't report them, so True is the only failing value.
			for _, badConditionType := range []corev1.NodeConditionType{
				corev1.NodeMemoryPressure,
				corev1.NodeDiskPressure,
				corev1.NodePIDPressure,
				corev1.NodeNetworkUnavailable,
			} {
				status, exists := nodeConditionStatus(node, badConditionType)
				if exists && status == corev1.ConditionTrue {
					issues = append(issues, fmt.Sprintf("%s=True", badConditionType))
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

	It("requires every MachineConfigPool to be Updated and not Degraded", Label("machine-config"), func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(machineConfigPoolGVR)

		checked := expectConditions(ctx, machineConfigPoolGVR,
			conditionExpectation{Type: "Updated", Status: metav1.ConditionTrue},
			conditionExpectation{Type: "Degraded", Status: metav1.ConditionFalse},
		)
		Expect(checked).NotTo(BeZero(), "no MachineConfigPools found")
	})

	// A CertificateSigningRequest with neither condition is awaiting a
	// decision. Node CSRs are normally auto-approved within seconds by the
	// machine-approver, so one still pending usually means that controller,
	// or a human approver for a non-node CSR, isn't keeping up. This is a
	// point-in-time read, so a CSR caught moments after creation can cause a
	// spurious failure.
	It("requires no CertificateSigningRequest to be stuck pending", Label("csr"), func(ctx SpecContext) {
		csrs, err := coreClient.CertificatesV1().CertificateSigningRequests().List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred(), "list CertificateSigningRequests")

		var problems []string
		for _, csr := range csrs.Items {
			var decided bool
			for _, condition := range csr.Status.Conditions {
				if condition.Type == certificatesv1.CertificateApproved || condition.Type == certificatesv1.CertificateDenied {
					decided = true
					break
				}
			}
			if !decided {
				problems = append(problems, csr.Name)
			}
		}

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})

	// A pod in Failed phase, crash-looping, or unable to pull its image won't
	// recover on its own. Pending and Running pods are otherwise left alone,
	// since a container can restart occasionally without being unhealthy.
	It("requires no pods to be Failed, crash-looping, or unable to pull their image", Label("pods"), func(ctx SpecContext) {
		badWaitingReasons := []string{"CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull"}

		pods, err := coreClient.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred(), "list pods across all namespaces")

		var problems []string
		for _, pod := range pods.Items {
			id := pod.Namespace + "/" + pod.Name

			if pod.Status.Phase == corev1.PodFailed {
				reason := pod.Status.Reason
				if reason == "" {
					reason = "<missing>"
				}
				problems = append(problems, fmt.Sprintf("%s: phase=Failed (%s)", id, reason))
				continue
			}

			containerStatuses := append(
				append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...),
				pod.Status.ContainerStatuses...,
			)
			for _, status := range containerStatuses {
				if status.State.Waiting == nil {
					continue
				}
				if slices.Contains(badWaitingReasons, status.State.Waiting.Reason) {
					problems = append(problems, fmt.Sprintf(
						"%s: container %s is %s (%d restarts)",
						id, status.Name, status.State.Waiting.Reason, status.RestartCount,
					))
				}
			}
		}

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
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
