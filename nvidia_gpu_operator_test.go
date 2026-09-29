package clustertests

import (
	"fmt"

	"github.com/larsks/clustertests/internal/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	nvidiaGpuOperatorNamespace = "nvidia-gpu-operator"
)

// clusterPolicyGVR is the NVIDIA GPU Operator's top-level resource. Its
// presence is also how the suite detects whether the operator is installed on
// this cluster.
var clusterPolicyGVR = schema.GroupVersionResource{
	Group:    "nvidia.com",
	Version:  "v1",
	Resource: "clusterpolicies",
}

// gpuNodeSelector matches nodes labeled as having a GPU.
var gpuNodeSelector = labels.SelectorFromSet(labels.Set{
	"nvidia.com/gpu.present": "true",
})

// gpuNodes returns the nodes labeled as having a GPU, filtered locally from
// the nodes loaded at suite setup (see testutil.ClusterNodes and
// testutil.NodesMatching) rather than its own List call.
func gpuNodes() []corev1.Node {
	return testutil.NodesMatching(gpuNodeSelector)
}

var _ = Describe("Nvidia GPU operator", Label("gpu"), func() {
	BeforeEach(func(ctx SpecContext) {
		testutil.SkipIfResourceKindDoesNotExist(clusterPolicyGVR)
	})

	It("has a gpu.product label on every node with gpu.present=true", func(ctx SpecContext) {
		var problems []string
		for _, node := range gpuNodes() {
			if _, found := node.Labels["nvidia.com/gpu.product"]; !found {
				problems = append(problems, node.Name)
			}
		}
		testutil.ExpectNoProblems(problems)
	})

	// ClusterPolicy reports its health as status.state rather than through
	// standard conditions: "ready" once every operand it deploys is up,
	// "notReady" while any isn't, and "ignored" for a ClusterPolicy that the
	// operator is disregarding because another one exists. The operator only
	// acts on a single ClusterPolicy, so anything other than "ready" is worth
	// reporting. When the operator sets its "error" condition, that says what
	// is wrong, so it's included in the failure message.
	It("has a ready ClusterPolicy", func(ctx SpecContext) {
		var problems []string
		count := 0
		err := testutil.EachResource(ctx, clusterPolicyGVR, metav1.ListOptions{}, func(policy *unstructured.Unstructured) error {
			count++
			id := testutil.ObjectID(policy)

			state, _, err := unstructured.NestedString(policy.Object, "status", "state")
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", id, err))
				return nil
			}
			if state == "ready" {
				return nil
			}
			if state == "" {
				state = "<missing>"
			}

			problem := fmt.Sprintf("%s: state=%s, want ready", id, state)
			if conditions, err := testutil.ConditionsOf(policy); err == nil {
				if failure := apimeta.FindStatusCondition(conditions, "error"); failure != nil &&
					failure.Status == metav1.ConditionTrue {
					problem += fmt.Sprintf(" (%s: %s)", failure.Reason, failure.Message)
				}
			}
			problems = append(problems, problem)
			return nil
		})
		testutil.ExpectNoError(err, "list ClusterPolicies")
		testutil.AtLeastOne.Expect(clusterPolicyGVR.GroupResource().String(), count)

		testutil.ExpectNoProblems(problems)
	})

	testutil.DescribeAvailableDeployments(nvidiaGpuOperatorNamespace,
		"gpu-operator",
	)

	testutil.DescribeAvailableDaemonSets(nvidiaGpuOperatorNamespace,
		"gpu-feature-discovery",
		"nvidia-container-toolkit-daemonset",
		"nvidia-dcgm",
		"nvidia-dcgm-exporter",
		"nvidia-device-plugin-daemonset",
		"nvidia-device-plugin-mps-control-daemon",
		"nvidia-mig-manager",
		"nvidia-node-status-exporter",
		"nvidia-operator-validator",
	)

	It("has available driver daemonset", func(ctx SpecContext) {
		daemonsets, err := testutil.CoreClient.AppsV1().DaemonSets(nvidiaGpuOperatorNamespace).
			List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/component=nvidia-driver"})
		testutil.ExpectNoError(err, "list GPU driver daemonset")

		// Assert on the names, not the DaemonSets: when the count is wrong,
		// Gomega prints every object it was given, manifest and all.
		names := make([]string, len(daemonsets.Items))
		for i, daemonset := range daemonsets.Items {
			names[i] = testutil.ObjectID(&daemonset)
		}
		Expect(names).To(HaveLen(1), "expected exactly one GPU driver daemonset")
		testutil.DaemonsetIsAvailable(&daemonsets.Items[0])
	})
})
