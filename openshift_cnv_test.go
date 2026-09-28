package clustertests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	cnvNamespace = "openshift-cnv"
)

// virtualMachineGVR is used only to detect whether OpenShift Virtualization
// (KubeVirt) is installed on this cluster; the suite doesn't otherwise check
// VirtualMachine resources.
var virtualMachineGVR = schema.GroupVersionResource{
	Group:    "kubevirt.io",
	Version:  "v1",
	Resource: "virtualmachines",
}

// hyperConvergedGVR is the HyperConverged Cluster Operator's top-level
// resource, which rolls up the health of everything OpenShift Virtualization
// deploys. It only exists where OpenShift Virtualization (or the upstream
// HCO) is installed, not on a plain KubeVirt install.
var hyperConvergedGVR = schema.GroupVersionResource{
	Group:    "hco.kubevirt.io",
	Version:  "v1beta1",
	Resource: "hyperconvergeds",
}

// kubeVirtGVR is the KubeVirt resource that virt-operator reconciles into the
// virt-api, virt-controller, and virt-handler components. HyperConverged
// creates and manages one, but a plain KubeVirt install has one too.
var kubeVirtGVR = schema.GroupVersionResource{
	Group:    "kubevirt.io",
	Version:  "v1",
	Resource: "kubevirts",
}

var _ = Describe("OpenShiftVirtualization", Label("cnv"), func() {
	BeforeEach(func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(virtualMachineGVR)
	})

	DescribeTable("has available deployment", func(ctx SpecContext, name string) {
		deploymentIsAvailableByName(ctx, cnvNamespace, name)
	},
		Entry("aaq-operator", "aaq-operator"),
		Entry("cdi-apiserver", "cdi-apiserver"),
		Entry("cdi-deployment", "cdi-deployment"),
		Entry("cdi-operator", "cdi-operator"),
		Entry("cdi-uploadproxy", "cdi-uploadproxy"),
		Entry("cluster-network-addons-operator", "cluster-network-addons-operator"),
		Entry("hco-operator", "hco-operator"),
		Entry("hco-webhook", "hco-webhook"),
		Entry("hostpath-provisioner-operator", "hostpath-provisioner-operator"),
		Entry("hyperconverged-cluster-cli-download", "hyperconverged-cluster-cli-download"),
		Entry("kubemacpool-cert-manager", "kubemacpool-cert-manager"),
		Entry("kubemacpool-mac-controller-manager", "kubemacpool-mac-controller-manager"),
		Entry("kubevirt-apiserver-proxy", "kubevirt-apiserver-proxy"),
		Entry("kubevirt-console-plugin", "kubevirt-console-plugin"),
		Entry("kubevirt-ipam-controller-manager", "kubevirt-ipam-controller-manager"),
		Entry("kubevirt-migration-controller", "kubevirt-migration-controller"),
		Entry("kubevirt-migration-operator", "kubevirt-migration-operator"),
		Entry("ssp-operator", "ssp-operator"),
		Entry("virt-api", "virt-api"),
		Entry("virt-controller", "virt-controller"),
		Entry("virt-exportproxy", "virt-exportproxy"),
		Entry("virt-operator", "virt-operator"),
		Entry("virt-platform-autopilot", "virt-platform-autopilot"),
		Entry("virt-template-validator", "virt-template-validator"),
	)

	DescribeTable("has available daemonset", func(ctx SpecContext, name string) {
		daemonsetIsAvailableByName(ctx, cnvNamespace, name)
	},
		Entry("bridge-marker", "bridge-marker"),
		Entry("kube-cni-linux-bridge-plugin", "kube-cni-linux-bridge-plugin"),
		Entry("virt-handler", "virt-handler"),
	)

	// The deployments above can all be running while the HyperConverged
	// resource, which is what actually reconciles OpenShift Virtualization,
	// reports that something it manages is unavailable or degraded.
	It("has an available, non-degraded HyperConverged", func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(hyperConvergedGVR)

		checked := expectConditions(ctx, hyperConvergedGVR,
			conditionExpectation{Type: "Available", Status: metav1.ConditionTrue},
			conditionExpectation{Type: "Degraded", Status: metav1.ConditionFalse},
		)
		Expect(checked).NotTo(BeZero(), "no HyperConverged found")
	})
	// HyperConverged summarizes this resource's status, but only after the
	// fact; checking it directly points at KubeVirt itself when it's the
	// component in trouble, and covers installs that have no HyperConverged.
	It("has an available, non-degraded KubeVirt", func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(kubeVirtGVR)

		checked := expectConditions(ctx, kubeVirtGVR,
			conditionExpectation{Type: "Available", Status: metav1.ConditionTrue},
			conditionExpectation{Type: "Degraded", Status: metav1.ConditionFalse},
		)
		Expect(checked).NotTo(BeZero(), "no KubeVirt found")
	})
})
