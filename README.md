# go-manifest

`go-manifest` is a Go library inspired by [manifestival](https://github.com/manifestival/manifestival), but with a focus
on using Kubernetes Server-Side Apply. Server-Side Apply is a powerful feature introduced in Kubernetes that allows you
to declaratively update resources on the server side, which can be more efficient and reliable than traditional
imperative updates.

## Installation

To use `go-manifest` in your Go project, you can simply add it as a dependency:

```shell
go get github.com/cloud104/go-manifest
```

## Usage

Here's a basic example of how to use `go-manifest`:

```go
package main

import (
	"context"
	"log"

	"github.com/cloud104/go-manifest"
	"k8s.io/client-go/rest"
)

func main() {
	// Get the Kubernetes cluster configuration
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal(err)
	}

	// Instantiate a new ManifestReader by specifying the field manager and the Kubernetes cluster configuration
	mr, err := manifest.NewReader("totvs-cloud", config)
	if err != nil {
		log.Fatal(err)
	}

	// Create a new Manifest object
	m, err := mr.FromPath("path/to/your/manifest.yaml", false)
	if err != nil {
		log.Fatal(err)
	}

	// Apply the manifest using Server-Side Apply
	if err = m.Apply(context.Background()); err != nil {
		log.Fatal(err)
	}

	log.Println("Manifest applied successfully!")
}
```

In this example, we create a `Manifest` object by specifying the path to your Kubernetes manifest file. Then, we use
the `Apply` method to apply the manifest using Server-Side Apply.

## Contributing

Contributions are welcome! If you find a bug or have a feature request, please open an issue on the GitHub repository.

## Acknowledgments

This library is inspired by [manifestival](https://github.com/manifestival/manifestival) and leverages the power of
Kubernetes Server-Side Apply. We are grateful to the open-source community and the Kubernetes project for making these
tools available.
