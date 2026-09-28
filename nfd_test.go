package clustertests

import (
	"github.com/larsks/clustertests/internal/testutil"
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
		testutil.SkipIfResourceKindDoesNotExist(nodeFeatureDiscoveryGVR)
	})

	testutil.DescribeAvailableDeployments(nfdNamespace,
		"nfd-controller-manager",
		"nfd-gc",
		"nfd-master",
	)

	testutil.DescribeAvailableDaemonSets(nfdNamespace,
		"nfd-worker",
	)

	It("has available NodeFeatureDiscovery instances", func(ctx SpecContext) {
		testutil.ExpectAllReady(ctx, nodeFeatureDiscoveryGVR, testutil.NoneOK, "Available")
	})
})
