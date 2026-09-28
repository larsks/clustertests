package testutil

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ObjectID returns namespace/name for namespaced resources and just the name
// for cluster-scoped ones, so that same-named resources in different
// namespaces can be told apart in failure messages. Any *appsv1.Deployment,
// *appsv1.StatefulSet, *appsv1.DaemonSet, or *unstructured.Unstructured
// satisfies metav1.Object, whether through an embedded ObjectMeta or its own
// accessor methods.
func ObjectID(obj metav1.Object) string {
	if namespace := obj.GetNamespace(); namespace != "" {
		return namespace + "/" + obj.GetName()
	}
	return obj.GetName()
}
