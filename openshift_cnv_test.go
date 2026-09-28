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

	describeAvailableDeployments(cnvNamespace,
		"aaq-operator",
		"cdi-apiserver",
		"cdi-deployment",
		"cdi-operator",
		"cdi-uploadproxy",
		"cluster-network-addons-operator",
		"hco-operator",
		"hco-webhook",
		"hostpath-provisioner-operator",
		"hyperconverged-cluster-cli-download",
		"kubemacpool-cert-manager",
		"kubemacpool-mac-controller-manager",
		"kubevirt-apiserver-proxy",
		"kubevirt-console-plugin",
		"kubevirt-ipam-controller-manager",
		"kubevirt-migration-controller",
		"kubevirt-migration-operator",
		"ssp-operator",
		"virt-api",
		"virt-controller",
		"virt-exportproxy",
		"virt-operator",
		"virt-platform-autopilot",
		"virt-template-validator",
	)

	describeAvailableDaemonSets(cnvNamespace,
		"bridge-marker",
		"kube-cni-linux-bridge-plugin",
		"virt-handler",
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
