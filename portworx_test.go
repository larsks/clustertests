package clustertests

import (
	"context"
	"fmt"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

// expectPhase lists every object of gvr and fails unless each has
// status.phase == wantPhase. It returns how many were checked, so callers can
// require at least one to exist.
func expectPhase(ctx context.Context, gvr schema.GroupVersionResource, wantPhase string) int {
	GinkgoHelper()

	var problems []string
	count := 0
	err := eachResource(ctx, gvr, metav1.ListOptions{}, func(item *unstructured.Unstructured) error {
		count++
		id := objectID(item)

		phase, found, err := unstructured.NestedString(item.Object, "status", "phase")
		if err != nil {
			return fmt.Errorf("read phase for %s: %w", id, err)
		}
		if !found {
			phase = "<missing>"
		}
		if phase != wantPhase {
			problems = append(problems, fmt.Sprintf("%s: phase=%s", id, phase))
		}
		return nil
	})
	Expect(err).NotTo(HaveOccurred(), "list %s", gvr.Resource)

	slices.Sort(problems)
	Expect(problems).To(BeEmpty())
	return count
}

var _ = Describe("Portworx", Label("portworx"), func() {
	BeforeEach(func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(storageClusterGVR)
	})

	DescribeTable("has available deployment", func(ctx SpecContext, name string) {
		deploymentIsAvailableByName(ctx, portworxNamespace, name)
	},
		Entry("portworx-operator", "portworx-operator"),
		Entry("pure-cosi-driver", "pure-cosi-driver"),
		Entry("px-pure-csi-controller", "px-pure-csi-controller"),
		Entry("px-pure-csi-telemetry-registration", "px-pure-csi-telemetry-registration"),
	)

	It("has a healthy StorageCluster", func(ctx SpecContext) {
		count := expectPhase(ctx, storageClusterGVR, "Running")
		Expect(count).NotTo(BeZero(), "no StorageClusters found")
	})

	It("has a healthy PureStorageCluster", func(ctx SpecContext) {
		_, err := dynamicClient.Resource(purestorageClusterGVR).List(ctx, metav1.ListOptions{Limit: 1})
		if apierrors.IsForbidden(err) {
			Skip("insufficient privileges to list PureStorageCluster; rerun with an admin privileges to include this check")
		}
		Expect(err).NotTo(HaveOccurred(), "list PureStorageClusters")

		count := expectPhase(ctx, purestorageClusterGVR, "Running")
		Expect(count).NotTo(BeZero(), "no PureStorageClusters found")
	})
})
