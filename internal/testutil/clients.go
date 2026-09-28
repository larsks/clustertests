package testutil

import (
	"context"

	. "github.com/onsi/ginkgo/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/pager"
)

var (
	DynamicClient dynamic.Interface
	CoreClient    kubernetes.Interface
)

// InitClients creates CoreClient and DynamicClient from config. They are
// package-level variables, so every parallel test process must call this
// itself, from the per-process half of SynchronizedBeforeSuite.
func InitClients(config *rest.Config) {
	GinkgoHelper()

	var err error
	CoreClient, err = kubernetes.NewForConfig(config)
	ExpectNoError(err, "create Kubernetes client")

	DynamicClient, err = dynamic.NewForConfig(config)
	ExpectNoError(err, "create dynamic Kubernetes client")
}

// listPageSize bounds how many items are fetched per page for any List call
// in this suite that isn't restricted to a single namespace, so a check
// doesn't pull an entire large cluster's worth of objects into memory, or
// into one oversized request, at once.
const listPageSize = 500

// EachItem pages through every object that list returns for opts and calls fn
// for each one, instead of fetching the whole list at once. list is the List
// method of a typed or dynamic client, for example
//
//	EachItem(ctx, CoreClient.CoreV1().Pods(metav1.NamespaceAll).List, opts,
//		func(pod *corev1.Pod) error { ... })
//
// and T is the item type that fn receives: the typed object for a typed
// client, or unstructured.Unstructured for the dynamic client. For a
// namespaced resource, passing metav1.NamespaceAll to the client spans all
// namespaces.
func EachItem[T any, L runtime.Object](
	ctx context.Context,
	list func(context.Context, metav1.ListOptions) (L, error),
	opts metav1.ListOptions,
	fn func(*T) error,
) error {
	listPages := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		page, err := list(ctx, opts)
		if err != nil {
			return nil, err
		}
		return page, nil
	})
	listPages.PageSize = listPageSize
	return listPages.EachListItem(ctx, opts, func(obj runtime.Object) error {
		return fn(any(obj).(*T))
	})
}

// EachResource is EachItem for a resource type that is only known by its
// GroupVersionResource, such as a CRD. For a namespaced resource, omitting a
// namespace restriction in opts (as every caller here does) spans all
// namespaces.
func EachResource(
	ctx context.Context,
	gvr schema.GroupVersionResource,
	opts metav1.ListOptions,
	fn func(*unstructured.Unstructured) error,
) error {
	return EachItem(ctx, DynamicClient.Resource(gvr).List, opts, fn)
}
