package clustertests

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	nvidiaGpuOperatorNamespace = "nvidia-gpu-operator"
)

// gpuNodeSelector matches nodes labeled as having a GPU.
var gpuNodeSelector = labels.SelectorFromSet(labels.Set{
	"nvidia.com/gpu.present": "true",
})

// gpuNodes returns the nodes labeled as having a GPU, filtered locally from
// the shared, once-per-process node cache (see allNodes/nodesMatching in
// helpers_test.go) rather than its own List call.
func gpuNodes(ctx context.Context) []corev1.Node {
	return nodesMatching(ctx, gpuNodeSelector)
}

var _ = Describe("NvidiaGpuOperator", Label("gpu"), func() {
	BeforeEach(func(ctx SpecContext) {
		if len(gpuNodes(ctx)) == 0 {
			Skip("no gpu nodes")
		}
	})

	It("has a gpu.product label on every node with gpu.present=true", func(ctx SpecContext) {
		var problems []string
		for _, node := range gpuNodes(ctx) {
			if _, found := node.Labels["nvidia.com/gpu.product"]; !found {
				problems = append(problems, node.Name)
			}
		}
		Expect(problems).To(BeEmpty())
	})

	DescribeTable("has available deployment", func(ctx SpecContext, name string) {
		deploymentIsAvailableByName(ctx, nvidiaGpuOperatorNamespace, name)
	},
		Entry("gpu-operator", "gpu-operator"),
	)

	DescribeTable("has available daemonset", func(ctx SpecContext, name string) {
		daemonsetIsAvailableByName(ctx, nvidiaGpuOperatorNamespace, name)
	},
		Entry("gpu-feature-discovery", "gpu-feature-discovery"),
		Entry("nvidia-container-toolkit-daemonset", "nvidia-container-toolkit-daemonset"),
		Entry("nvidia-dcgm", "nvidia-dcgm"),
		Entry("nvidia-dcgm-exporter", "nvidia-dcgm-exporter"),
		Entry("nvidia-device-plugin-daemonset", "nvidia-device-plugin-daemonset"),
		Entry("nvidia-device-plugin-mps-control-daemon", "nvidia-device-plugin-mps-control-daemon"),
		Entry("nvidia-mig-manager", "nvidia-mig-manager"),
		Entry("nvidia-node-status-exporter", "nvidia-node-status-exporter"),
		Entry("nvidia-operator-validator", "nvidia-operator-validator"),
	)

	It("has available driver daemonset", func(ctx SpecContext) {
		daemonsets, err := coreClient.AppsV1().DaemonSets(nvidiaGpuOperatorNamespace).
			List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/component=nvidia-driver"})
		Expect(err).NotTo(HaveOccurred(), "list GPU driver daemonset")
		Expect(daemonsets.Items).To(HaveLen(1))
		daemonsetIsAvailable(ctx, &daemonsets.Items[0])
	})
})
