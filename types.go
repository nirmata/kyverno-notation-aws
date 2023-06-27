package main

import (
	imageutils "github.com/kyverno/kyverno/pkg/utils/image"
)

type ImageInfo struct {
	imageutils.ImageInfo

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
	Images ImageInfos `json:"images"`
}

type ResponseData struct {
	// Verified is true when all the images are verified.
	Verified bool `json:"verified"`

	// Message contains an optional custom message to send as a response.
	Message string `json:"message,omitempty"`

	// Results contains the list of containers in JSONPatch format
	Results []Result `json:"results"`
}
