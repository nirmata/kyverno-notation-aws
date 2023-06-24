package main

import "fmt"

type Image struct {
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

func (i *Image) String() string {
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

type ImageInfo struct {
	Image

	// Pointer is the path to the image object in the resource
	Pointer string `json:"jsonPointer"`
}

type ImageInfos struct {
	// InitContainers is a map of init containers image data from the AdmissionReview request, key is the container name
	InitContainers map[string]ImageInfo `json:"initContainers,omitempty"`

	// Containers is a map of containers image data from the AdmissionReview request, key is the container name
	Containers map[string]ImageInfo `json:"containers,omitempty"`

	// EphemeralContainers is a map of ephemeral containers image data from the AdmissionReview request, key is the container name
	EphemeralContainers map[string]ImageInfo `json:"ephemeralContainers,omitempty"`
}

type Result struct {
	// Name of the container
	Name string `json:"name"`

	// Path to the image object in the resource
	Path string `json:"path"`

	// Updated image with the digest
	Image string `json:"image"`
}

type RequestData struct {
	ImageInfos
}

type ResponseData struct {
	// Vverifiederified is true when all the images are verified.
	Verified bool `json:"verified"`

	// Message contains an optional custom message to send as a response.
	Message string `json:"message,omitempty"`

	// Results contains the list of containers in JSONPatch format
	Results []Result `json:"results"`
}
