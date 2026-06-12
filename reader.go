package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

type Reader struct {
	fieldManager string
	client       dynamic.Interface
	mapper       meta.RESTMapper
	scheme       *runtime.Scheme
}

// Deprecated: use NewReaderWithOptions instead.
func NewReader(fieldManager string, config *rest.Config) (*Reader, error) {
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	return NewReaderWithOptions(fieldManager, ReaderOptions{
		Config:     config,
		HTTPClient: httpClient,
	})
}

// Deprecated: use NewReaderWithOptions instead.
func NewReaderForConfigAndClient(fieldManager string, config *rest.Config, httpClient *http.Client) (*Reader, error) {
	m, err := newDynamicRESTMapper(config, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the manifest reader: %w", err)
	}

	c, err := dynamic.NewForConfigAndClient(config, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the manifest reader: %w", err)
	}

	scheme := runtime.NewScheme()
	if err = clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to initialize the manifest reader: %w", err)
	}

	return &Reader{fieldManager: fieldManager, client: c, mapper: m, scheme: scheme}, nil
}

type ReaderOptions struct {
	Config     *rest.Config
	HTTPClient *http.Client
	Mapper     meta.RESTMapper
	Scheme     *runtime.Scheme
}

func DefaultReader(fieldManager string) (*Reader, error) {
	return NewReaderWithOptions(fieldManager, ReaderOptions{})
}

func NewReaderWithOptions(fieldManager string, options ReaderOptions) (reader *Reader, err error) {
	config := options.Config
	if config == nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize the manifest reader: %w", err)
		}
	}

	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient, err = rest.HTTPClientFor(config)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize the manifest reader: %w", err)
		}
	}

	mapper := options.Mapper
	if mapper == nil {
		mapper, err = newDynamicRESTMapper(config, httpClient)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize the manifest reader: %w", err)
		}
	}

	client, err := dynamic.NewForConfigAndClient(config, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the manifest reader: %w", err)
	}

	scheme := options.Scheme
	if scheme == nil {
		scheme = runtime.NewScheme()
		if err = clientgoscheme.AddToScheme(scheme); err != nil {
			return nil, fmt.Errorf("failed to initialize the manifest reader: %w", err)
		}
	}

	return &Reader{
		fieldManager: fieldManager,
		client:       client,
		mapper:       mapper,
		scheme:       scheme,
	}, nil
}

func (r *Reader) FromObject(resources ...metav1.Object) (List, error) {
	unstructuredResources := make([]*unstructured.Unstructured, 0, len(resources))

	for i, obj := range resources {
		if obj == nil {
			continue
		}

		if u, ok := obj.(*unstructured.Unstructured); ok {
			unstructuredResources = append(unstructuredResources, u)
			continue
		}

		runtimeObj, ok := obj.(runtime.Object)
		if !ok {
			return nil, fmt.Errorf("resource at index %d does not implement runtime.Object", i)
		}

		data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(runtimeObj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert resource at index %d to unstructured: %w", i, err)
		}

		u := &unstructured.Unstructured{Object: data}

		if u.GetAPIVersion() == "" || u.GetKind() == "" {
			gvk := runtimeObj.GetObjectKind().GroupVersionKind()

			if gvk.Empty() {
				gvk, err = r.gvkForObject(runtimeObj)
				if err != nil {
					return nil, fmt.Errorf("failed to determine apiVersion/kind for resource at index %d: %w", i, err)
				}
			}

			u.SetAPIVersion(gvk.GroupVersion().String())
			u.SetKind(gvk.Kind)
		}

		unstructuredResources = append(unstructuredResources, u)
	}

	return r.FromUnstructured(unstructuredResources)
}

func (r *Reader) FromUnstructured(resources []*unstructured.Unstructured) (List, error) {
	return &list{resources: resources, fieldManager: r.fieldManager, client: r.client, mapper: r.mapper}, nil
}

func (r *Reader) FromBytes(data []byte) (List, error) {
	reader := bytes.NewReader(data)
	decoder := yaml.NewYAMLToJSONDecoder(reader)

	var resources []*unstructured.Unstructured

	var err error

	for {
		out := &unstructured.Unstructured{}

		err = decoder.Decode(out)
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil || len(out.Object) == 0 {
			continue
		}

		resources = append(resources, out)
	}

	if !errors.Is(err, io.EOF) {
		return &list{}, fmt.Errorf("unable to parse manifest from bytes: %w", err)
	}

	return r.FromUnstructured(resources)
}

func (r *Reader) FromURL(url string) (List, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifests from URL %q: %w", url, err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifests from URL %q: %w", url, err)
	}

	return r.FromBytes(body)
}

func (r *Reader) FromPath(pathname string, recursive bool) (List, error) {
	info, err := os.Stat(pathname)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifests from path %q: %w", pathname, err)
	}

	if info.IsDir() {
		return r.readDir(pathname, recursive)
	}

	return r.readFile(pathname)
}

func (r *Reader) readFile(pathname string) (List, error) {
	file, err := os.ReadFile(pathname)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifests from file %q: %w", pathname, err)
	}

	return r.FromBytes(file)
}

func (r *Reader) readDir(pathname string, recursive bool) (List, error) {
	contents, err := os.ReadDir(pathname)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifests from dir %q: %w", pathname, err)
	}

	resources := make([]*unstructured.Unstructured, 0)

	for _, f := range contents {
		name := path.Join(pathname, f.Name())

		info, err := os.Stat(name)
		if err != nil {
			return nil, fmt.Errorf("failed to read manifests from dir %q: %w", pathname, err)
		}

		var els List

		switch {
		case info.IsDir() && recursive:
			els, err = r.readDir(name, recursive)
		case !info.IsDir():
			els, err = r.readFile(name)
		}

		if err != nil {
			return nil, err
		}

		resources = append(resources, els.Resources()...)
	}

	return r.FromUnstructured(resources)
}

func (r *Reader) gvkForObject(obj runtime.Object) (schema.GroupVersionKind, error) {
	if r.scheme == nil {
		return schema.GroupVersionKind{}, fmt.Errorf("reader has no scheme")
	}

	gvks, _, err := r.scheme.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}

	if len(gvks) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("no registered kinds for %T", obj)
	}

	for _, gvk := range gvks {
		if gvk.Kind != "" && gvk.Version != "" {
			return gvk, nil
		}
	}

	return schema.GroupVersionKind{}, fmt.Errorf("no usable registered kinds for %T", obj)
}
