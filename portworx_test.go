package clustertests

import (
	. "github.com/onsi/ginkgo/v2"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	portworxNamespace = "portworx"
)

var (
	storageClusterGVR = schema.GroupVersionResource{
		Group:    "core.libopenstorage.org",
		Version:  "v1",
		Resource: "storageclusters",
	}
	purestorageClusterGVR = schema.GroupVersionResource{
		Group:    "core.libopenstorage.org",
		Version:  "v1",
		Resource: "purestorageclusters",
	}
)

var _ = Describe("Portworx", Label("portworx"), func() {
	BeforeEach(func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(storageClusterGVR)
	})

	describeAvailableDeployments(portworxNamespace,
		"portworx-operator",
		"pure-cosi-driver",
		"px-pure-csi-controller",
		"px-pure-csi-telemetry-registration",
	)

	describeAvailableDaemonSets(portworxNamespace,
		"px-pure-csi-node",
		"px-pure-csi-telemetry",
	)

	It("has a healthy StorageCluster", func(ctx SpecContext) {
		expectPhase(ctx, storageClusterGVR, metav1.ListOptions{}, atLeastOne, "Running")
	})

	It("has a healthy PureStorageCluster", func(ctx SpecContext) {
		_, err := dynamicClient.Resource(purestorageClusterGVR).List(ctx, metav1.ListOptions{Limit: 1})
		if apierrors.IsForbidden(err) {
			Skip("insufficient privileges to list PureStorageCluster; rerun with an admin privileges to include this check")
		}
		expectNoError(err, "list PureStorageClusters")

		expectPhase(ctx, purestorageClusterGVR, metav1.ListOptions{}, atLeastOne, "Running")
	})
})
