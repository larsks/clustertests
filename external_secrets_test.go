package cluster_tests

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
		skipIfNamespaceDoesNotExist(ctx, externalSecretsNamespace)
	})

	DescribeTable("has available deployment", func(ctx SpecContext, name string) {
		deploymentIsAvailableByName(ctx, externalSecretsNamespace, name)
	},
		Entry("external-secrets", "external-secrets"),
		Entry("external-secrets-cert-controller", "external-secrets-cert-controller"),
		Entry("external-secrets-operator-controller-manager", "external-secrets-operator-controller-manager"),
		Entry("external-secrets-webhook", "external-secrets-webhook"),
	)

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
