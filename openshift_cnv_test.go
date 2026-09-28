package clustertests

import (
	. "github.com/onsi/ginkgo/v2"

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
})
