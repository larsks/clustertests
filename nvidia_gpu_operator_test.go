package cluster_tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	nvidiaGpuOperatorNamespace = "nvidia-gpu-operator"
)

var _ = Describe("NvidiaGpuOperator", Label("gpu"), func() {
	var labelSelector = labels.SelectorFromSet(labels.Set{
		"nvidia.com/gpu.present": "true",
	}).String()

	var gpuNodes []v1.Node

	BeforeEach(func(ctx SpecContext) {
		nodes, err := coreClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		Expect(err).NotTo(HaveOccurred(), "list GPU nodes")
		if len(nodes.Items) == 0 {
			Skip("no gpu nodes")
		}
		gpuNodes = nodes.Items
	})

	It("has a gpu.product label on every node with gpu.present=true", func(ctx SpecContext) {
		var missingProductLabel []string
		for _, node := range gpuNodes {
			if _, found := node.Labels["nvidia.com/gpu.product"]; !found {
				missingProductLabel = append(missingProductLabel, node.Name)
			}
		}
		Expect(missingProductLabel).To(BeEmpty(), "nodes missing nvidia.com/gpu.product: %v", missingProductLabel)
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
