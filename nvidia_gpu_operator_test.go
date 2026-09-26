package cluster_tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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

	BeforeEach(func(ctx SpecContext) {
		nodes, err := coreClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		Expect(err).NotTo(HaveOccurred(), "list GPU nodes")
		if len(nodes.Items) == 0 {
			Skip("no gpu nodes")
		}
	})

	It("has a gpu.product label on every node with gpu.present=true", func(ctx SpecContext) {
		nodes, err := coreClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		Expect(err).NotTo(HaveOccurred(), "list GPU nodes")

		var missingProductLabel []string
		for _, node := range nodes.Items {
			if _, found := node.Labels["nvidia.com/gpu.product"]; !found {
				missingProductLabel = append(missingProductLabel, node.Name)
			}
		}
		Expect(missingProductLabel).To(BeEmpty(), "nodes missing nvidia.com/gpu.product: %v", missingProductLabel)
	})

	It("has available deployments", func(ctx SpecContext) {
		deploymentsAreAvailable(ctx, "nvidia-gpu-operator", []string{"gpu-operator"})
	})

	It("has available daemonsets", func(ctx SpecContext) {
		if available, problems := daemonsetsAreAvailable(ctx, nvidiaGpuOperatorNamespace, []string{
			"gpu-feature-discovery",
			"nvidia-container-toolkit-daemonset",
			"nvidia-dcgm",
			"nvidia-dcgm-exporter",
			"nvidia-device-plugin-daemonset",
			"nvidia-device-plugin-mps-control-daemon",
			"nvidia-mig-manager",
			"nvidia-node-status-exporter",
			"nvidia-operator-validator",
		}); !available {
			Fail(fmt.Sprintf("daemonsets are not available: %v", problems))
		}
	})

	It("has available driver daemonset", func(ctx SpecContext) {
		daemonsets, err := coreClient.AppsV1().DaemonSets(nvidiaGpuOperatorNamespace).
			List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/component=nvidia-driver"})
		Expect(err).NotTo(HaveOccurred(), "list GPU driver daemonset")
		Expect(daemonsets.Items).To(HaveLen(1))
		ds := daemonsets.Items[0]
		if !daemonsetIsAvailable(ctx, &ds) {
			Fail(fmt.Sprintf("nvidia-driver daemonset %s is not available", ds.GetName()))
		}
	})
})
