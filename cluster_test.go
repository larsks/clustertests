package clustertests

import (
	"fmt"
	"strings"
	"time"

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

var _ = Describe("cluster health", Label("cluster"), func() {
	It("requires every node to be schedulable, Ready, and free of resource pressure or network problems", Label("nodes"), func(ctx SpecContext) {
		nodes := clusterNodes
		Expect(nodes).NotTo(BeEmpty(), "no nodes found")

		var problems []string
		for _, node := range nodes {
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

		expectNoProblems(problems)
	})

	It("requires every PersistentVolumeClaim to be Bound", Label("storage"), func(ctx SpecContext) {
		var problems []string
		err := eachItem(ctx, coreClient.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).List, metav1.ListOptions{}, func(claim *corev1.PersistentVolumeClaim) error {
			if claim.Status.Phase == corev1.ClaimBound || isExcludedNamespace(claim.Namespace) ||
				pvcAwaitingFirstConsumer(claim) {
				return nil
			}

			phase := string(claim.Status.Phase)
			if phase == "" {
				phase = "<missing>"
			}
			problems = append(problems, fmt.Sprintf("%s/%s: phase=%s", claim.Namespace, claim.Name, phase))
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list PersistentVolumeClaims across all namespaces")

		expectNoProblems(problems)
	})

	It("requires every ClusterOperator to be Available and not Degraded", Label("cluster-operators"), func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(clusterOperatorGVR)

		expectConditions(ctx, clusterOperatorGVR, atLeastOne,
			conditionExpectation{Type: "Available", Status: metav1.ConditionTrue},
			conditionExpectation{Type: "Degraded", Status: metav1.ConditionFalse},
		)
	})

	It("requires ClusterVersion to be Available and not Failing", Label("cluster-version"), func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(clusterVersionGVR)

		expectConditions(ctx, clusterVersionGVR, atLeastOne,
			conditionExpectation{Type: "Available", Status: metav1.ConditionTrue},
			conditionExpectation{Type: "Failing", Status: metav1.ConditionFalse},
		)
	})

	It("requires every MachineConfigPool to be Updated and not Degraded", Label("machine-config"), func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(machineConfigPoolGVR)

		expectConditions(ctx, machineConfigPoolGVR, atLeastOne,
			conditionExpectation{Type: "Updated", Status: metav1.ConditionTrue},
			conditionExpectation{Type: "Degraded", Status: metav1.ConditionFalse},
		)
	})

	// A CertificateSigningRequest with neither condition is awaiting a
	// decision. Node CSRs are normally auto-approved within seconds by the
	// machine-approver, so one still pending usually means that controller,
	// or a human approver for a non-node CSR, isn't keeping up. This is a
	// point-in-time read, so a CSR caught moments after creation can cause a
	// spurious failure.
	It("requires no CertificateSigningRequest to be stuck pending", Label("csr"), func(ctx SpecContext) {
		var problems []string
		err := eachItem(ctx, coreClient.CertificatesV1().CertificateSigningRequests().List, metav1.ListOptions{}, func(csr *certificatesv1.CertificateSigningRequest) error {
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
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list CertificateSigningRequests")

		expectNoProblems(problems)
	})

	// A namespace can't finish terminating until everything in it is gone,
	// so one that stays in the Terminating phase is blocked, most often by a
	// resource whose finalizer belongs to a controller or CRD that has
	// already been removed, or by an unavailable APIService. The namespace
	// reports what is blocking it in its status conditions.
	It("requires no namespaces to be stuck terminating", Label("namespaces"), func(ctx SpecContext) {
		timeout := getEnvWithDefault("TERMINATING_TIMEOUT", defaultTerminatingTimeout)
		now := time.Now()

		var problems []string
		err := eachItem(ctx, coreClient.CoreV1().Namespaces().List, metav1.ListOptions{}, func(namespace *corev1.Namespace) error {
			if namespace.DeletionTimestamp == nil {
				return nil
			}

			if age := now.Sub(namespace.DeletionTimestamp.Time); age > timeout {
				problem := fmt.Sprintf("%s: terminating for %s", namespace.Name, age.Round(time.Second))
				for _, condition := range namespace.Status.Conditions {
					if condition.Status == corev1.ConditionTrue {
						problem += fmt.Sprintf("; %s: %s", condition.Type, condition.Message)
					}
				}
				problems = append(problems, problem)
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list namespaces")

		expectNoProblems(problems)
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

// pvcSelectedNodeAnnotation is set by the scheduler on a PVC once it has
// scheduled a pod that consumes it, to trigger topology-aware provisioning.
// Its presence is what tells a WaitForFirstConsumer PVC that's still waiting
// for a consumer (expected) apart from one whose consumer already showed up
// and provisioning is stuck (a real problem).
const pvcSelectedNodeAnnotation = "volume.kubernetes.io/selected-node"

// pvcAwaitingFirstConsumer reports whether claim's non-Bound phase is the
// ordinary WaitForFirstConsumer wait rather than a stuck binding: its
// StorageClass defers binding until a consumer pod is scheduled, and no
// consumer has been scheduled against it yet.
func pvcAwaitingFirstConsumer(claim *corev1.PersistentVolumeClaim) bool {
	className := defaultStorageClassName()
	if claim.Spec.StorageClassName != nil {
		className = *claim.Spec.StorageClassName
	}
	if className == "" || !storageClassIsWaitForFirstConsumer(className) {
		return false
	}

	_, selected := claim.Annotations[pvcSelectedNodeAnnotation]
	return !selected
}
