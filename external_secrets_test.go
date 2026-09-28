package clustertests

import (
	. "github.com/onsi/ginkgo/v2"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	externalSecretsNamespace = "external-secrets"
)

var (
	secretStoreGVR = schema.GroupVersionResource{
		Group:    "external-secrets.io",
		Version:  "v1",
		Resource: "secretstores",
	}
	clusterSecretStoreGVR = schema.GroupVersionResource{
		Group:    "external-secrets.io",
		Version:  "v1",
		Resource: "clustersecretstores",
	}
	externalSecretGVR = schema.GroupVersionResource{
		Group:    "external-secrets.io",
		Version:  "v1",
		Resource: "externalsecrets",
	}
)

var _ = Describe("ExternalSecretsOperator", Label("secrets"), func() {
	BeforeEach(func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(externalSecretGVR)
	})

	describeAvailableDeployments(externalSecretsNamespace,
		"external-secrets",
		"external-secrets-cert-controller",
		"external-secrets-webhook",
	)

	// external-secrets-operator-controller-manager only exists when
	// external-secrets is installed via the OpenShift operator bundle, not on
	// a plain Helm/upstream install.
	It("has available deployment external-secrets-operator-controller-manager, if present", func(ctx SpecContext) {
		deploymentIsAvailableIfPresent(ctx, externalSecretsNamespace, "external-secrets-operator-controller-manager")
	})

	It("has healthy ClusterSecretStores", func(ctx SpecContext) {
		expectAllReady(ctx, clusterSecretStoreGVR, conditionReady)
	})

	It("has healthy SecretStores", func(ctx SpecContext) {
		expectAllReady(ctx, secretStoreGVR, conditionReady)
	})

	It("has healthy external secrets", func(ctx SpecContext) {
		expectAllReady(ctx, externalSecretGVR, conditionReady)
	})
})
