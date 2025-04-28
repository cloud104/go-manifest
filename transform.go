package manifest

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
