package clustertests

import (
	. "github.com/onsi/ginkgo/v2"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	nfdNamespace = "openshift-nfd"
)

// nodeFeatureDiscoveryGVR is used only to detect whether the Node Feature
// Discovery operator is installed on this cluster; the suite doesn't
// otherwise check NodeFeatureDiscovery resources.
var nodeFeatureDiscoveryGVR = schema.GroupVersionResource{
	Group:    "nfd.openshift.io",
	Version:  "v1",
	Resource: "nodefeaturediscoveries",
}

var _ = Describe("NodeFeatureDiscovery", Label("nfd"), func() {
	BeforeEach(func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(nodeFeatureDiscoveryGVR)
	})

	DescribeTable("has available deployment", func(ctx SpecContext, name string) {
		deploymentIsAvailableByName(ctx, nfdNamespace, name)
	},
		entriesFor(
			"nfd-controller-manager",
			"nfd-gc",
			"nfd-master",
		),
	)

	DescribeTable("has available daemonset", func(ctx SpecContext, name string) {
		daemonsetIsAvailableByName(ctx, nfdNamespace, name)
	},
		entriesFor(
			"nfd-worker",
		),
	)

	It("has available NodeFeatureDiscovery instances", func(ctx SpecContext) {
		expectAllReady(ctx, nodeFeatureDiscoveryGVR, "Available")
	})
})
