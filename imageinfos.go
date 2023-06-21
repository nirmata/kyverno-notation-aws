package main

import "fmt"

type ImageInfo struct {
	// Registry is the URL address of the image registry e.g. `docker.io`
	Registry string `json:"registry,omitempty"`

	// Name is the image name portion e.g. `busybox`
	Name string `json:"name"`

	// Path is the repository path and image name e.g. `some-repository/busybox`
	Path string `json:"path"`

	// Tag is the image tag e.g. `v2`
	Tag string `json:"tag,omitempty"`

	// Digest is the image digest portion e.g. `sha256:128c6e3534b842a2eec139999b8ce8aa9a2af9907e2b9269550809d18cd832a3`
	Digest string `json:"digest,omitempty"`
}

func (i *ImageInfo) String() string {
	var image string
	if i.Registry != "" {
		image = fmt.Sprintf("%s/%s", i.Registry, i.Path)
	} else {
		image = i.Path
	}
	if i.Digest != "" {
		return fmt.Sprintf("%s@%s", image, i.Digest)
	} else {
		return fmt.Sprintf("%s:%s", image, i.Tag)
	}
}

func (i *ImageInfo) ReferenceWithTag() string {
	if i.Registry != "" {
		return fmt.Sprintf("%s/%s:%s", i.Registry, i.Path, i.Tag)
	} else {
		return fmt.Sprintf("%s:%s", i.Path, i.Tag)
	}
}

type ImageInfos struct {
	// InitContainers is a map of init containers image data from the AdmissionReview request, key is the container name
	InitContainers map[string]ImageInfo `json:"initContainers,omitempty"`

	// Containers is a map of containers image data from the AdmissionReview request, key is the container name
	Containers map[string]ImageInfo `json:"containers,omitempty"`

	// EphemeralContainers is a map of ephemeral containers image data from the AdmissionReview request, key is the container name
	EphemeralContainers map[string]ImageInfo `json:"ephemeralContainers,omitempty"`
}
