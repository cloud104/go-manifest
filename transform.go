package manifest

import (
	"fmt"
	"reflect"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Transformer func(u *unstructured.Unstructured) error

func (l *list) Transform(funcs ...Transformer) (List, error) {
	resources := make([]*unstructured.Unstructured, 0, l.Size())

	for _, v := range l.Resources() {
		resource := v.DeepCopy()
		for _, transform := range funcs {
			if err := transform(resource); err != nil {
				return &list{}, err
			}
		}

		resources = append(resources, resource)
	}

	return &list{resources: resources, fieldManager: l.fieldManager, client: l.client, mapper: l.mapper}, nil
}


// PodTransformer applies 'fn' to corev1.Pod
func PodTransformer(fn func(obj *corev1.Pod) error) Transformer {
	return func(u *unstructured.Unstructured) error {
		if u.GroupVersionKind() != schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"} {
			return nil
		}
		return Object(fn)(u)
	}
}

// ServiceTransformer applies 'fn' to corev1.Service
func ServiceTransformer(fn func(obj *corev1.Service) error) Transformer {
	return func(u *unstructured.Unstructured) error {
		if u.GroupVersionKind() != schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"} {
			return nil
		}
		return Object(fn)(u)
	}
}

// NamespaceTransformer applies 'fn' to corev1.Namespace
func NamespaceTransformer(fn func(obj *corev1.Namespace) error) Transformer {
	return func(u *unstructured.Unstructured) error {
		if u.GroupVersionKind() != schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"} {
			return nil
		}
		return Object(fn)(u)
	}
}

// DeploymentTransformer applies 'fn' to apps/v1.Deployment
func DeploymentTransformer(fn func(obj *appsv1.Deployment) error) Transformer {
	return func(u *unstructured.Unstructured) error {
		if u.GroupVersionKind() != schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"} {
			return nil
		}
		return Object(fn)(u)
	}
}

// StatefulSetTransformer applies 'fn' to apps/v1.StatefulSet
func StatefulSetTransformer(fn func(obj *appsv1.StatefulSet) error) Transformer {
	return func(u *unstructured.Unstructured) error {
		if u.GroupVersionKind() != schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"} {
			return nil
		}
		return Object(fn)(u)
	}
}

// DaemonSetTransformer applies 'fn' to apps/v1.DaemonSet
func DaemonSetTransformer(fn func(obj *appsv1.DaemonSet) error) Transformer {
	return func(u *unstructured.Unstructured) error {
		if u.GroupVersionKind() != schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"} {
			return nil
		}
		return Object(fn)(u)
	}
}

// IngressTransformer applies 'fn' to networking.k8s.io/v1.Ingress
func IngressTransformer(fn func(obj *networkingv1.Ingress) error) Transformer {
	return func(u *unstructured.Unstructured) error {
		if u.GroupVersionKind() != schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"} {
			return nil
		}
		return Object(fn)(u)
	}
}

// Object is a generic transformer that applies a user-defined function 'fn'
// to a Kubernetes runtime.Object, handling conversion from/to unstructured form.
func Object[T runtime.Object](fn func(obj T) error) Transformer {
	return func(u *unstructured.Unstructured) error {
		// Create a new instance of the type
		var t T
		tType := reflect.TypeOf(t)

		if tType == nil {
			return fmt.Errorf("cannot create instance of nil type")
		}

		var obj T
		if tType.Kind() == reflect.Ptr {
			obj = reflect.New(tType.Elem()).Interface().(T)
		} else {
			obj = t
		}

		// Convert the unstructured object into the typed object
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
			return fmt.Errorf("failed to convert %T to %T: %w", u, obj, err)
		}

		// Apply the user-provided function
		if err := fn(obj); err != nil {
			return err
		}

		// Convert the object back into an unstructured map
		unstr, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return fmt.Errorf("failed to convert %T to %T: %w", obj, u, err)
		}

		u.SetUnstructuredContent(unstr)

		return nil
	}
}
