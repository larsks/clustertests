package cluster_tests

import (
	. "github.com/onsi/ginkgo/v2"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	certManagerNamespace = "cert-manager"
)

var (
	certificateGVR = schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "certificates",
	}
)

var _ = Describe("CertManager", Label("cert-manager"), func() {
	BeforeEach(func(ctx SpecContext) {
		skipIfNamespaceDoesNotExist(ctx, certManagerNamespace)
	})

	DescribeTable("has available deployment", func(ctx SpecContext, name string) {
		deploymentIsAvailableByName(ctx, certManagerNamespace, name)
	},
		Entry("cert-manager", "cert-manager"),
		Entry("cert-manager-cainjector", "cert-manager-cainjector"),
		Entry("cert-manager-webhook", "cert-manager-webhook"),
	)

	It("has healthy certificates", func(ctx SpecContext) {
		expectAllReady(ctx, certificateGVR, conditionReady)
	})
})
