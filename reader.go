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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

type Reader struct {
	fieldManager string
	client       dynamic.Interface
	mapper       meta.RESTMapper
}

func NewReader(fieldManager string, config *rest.Config) (*Reader, error) {
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	return NewReaderForConfigAndClient(fieldManager, config, httpClient)
}

func NewReaderForConfigAndClient(fieldManager string, config *rest.Config, httpClient *http.Client) (*Reader, error) {
	m, err := newDynamicRESTMapper(config, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the manifest reader: %w", err)
	}

	c, err := dynamic.NewForConfigAndClient(config, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize the manifest reader: %w", err)
	}

	return &Reader{fieldManager: fieldManager, client: c, mapper: m}, nil
}

func (r *Reader) FromUnstructured(resources []*unstructured.Unstructured) (List, error) {
	return &list{resources: resources, fieldManager: r.fieldManager, client: r.client, mapper: r.mapper}, nil
}

func (r *Reader) FromObjects(objects []runtime.Object) (List, error) {
	resources := make([]*unstructured.Unstructured, 0, len(objects))

	for _, obj := range objects {
		// Convert the object into an unstructured map
		unstr, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return nil, err
		}

		// Wrap map into Unstructured
		resources = append(resources, &unstructured.Unstructured{Object: unstr})
	}

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
