# Meta
[meta]: #meta
- Name: Verification of Attestations in Notation Kyverno Plugins
- Start Date: 2023-06-27
- Update date: 2023-06-27
- Author(s): @Vishal-Chdhry @realshuting

# Table of Contents
[table-of-contents]: #table-of-contents
- [Meta](#meta)
- [Table of Contents](#table-of-contents)
- [Overview](#overview)
- [Definitions](#definitions)
- [Motivation](#motivation)
- [Proposal](#proposal)
- [Implementation](#implementation)
  - [Fetching Artifacts](#fetching-artifacts)
  - [Verifying Artifacts](#verifying-artifacts)
  - [Fetching payloads](#fetching-payloads)
  - [Link to the Implementation PR](#link-to-the-implementation-pr)
- [Migration (OPTIONAL)](#migration-optional)
- [Drawbacks](#drawbacks)
- [Alternatives](#alternatives)
- [Prior Art](#prior-art)
- [Unresolved Questions](#unresolved-questions)

# Overview
[overview]: #overview

This proposal aims to discuss the approach for attestation verification in `kyverno-notation-aws` plugin, Kyverno extension service for Notation and the AWS signer and all the related plugins. Currently the plugin supports image verification and resolution of digests, This aims to add ability to verify attestations and do conditional checking on it.

# Definitions
[definitions]: #definitions
1. **Artifacts:** Software builds produce artifacts for installation and execution. The type and format of artifacts varies depending on the software. They can be packages, WAR files, container images, or other formats. 
2. **Metadata:** Metadata is used to describe software and the build environment. Provenance (origin) data, SBOMs, and vulnerability scan reports are the essential set of metadata required to assess security risks for software.
3. **Attestations:**  Authenticated metadata is used to attest to the integrity of a software system. Both custom and standardized metadata can be converted into attestations.
4. **Policies:** Policies check and enforce organization standards. Policies should be automatically enforced prior to deployment and via runtime scanning. 
5. **Payload:** Payload in this case refers to the data in the artifact.
 
# Motivation
[motivation]: #motivation

- Adds ability to verify attestations and do conditional checking on it.
- Verification of multiple attestations attached to the image.

# Proposal

Currently, we support image verification by passing the `images` variable from the context to `/checkimages` endpoint which returns a list of JSONPatch compatible list to do mutation on digests.

Current request format:
```json
{
  "images": {
    "containers": {
      "tomcat": {
        "registry": "https://ghcr.io",
        "path": "tomcat",
        "name": "tomcat",
        "tag": "9",
        "jsonPointer": "spec/container/0/image"
      }
    },
    "initContainers": {
      "vault": {
        "registry": "https://ghcr.io",
        "path": "vault",
        "name": "vault",
        "tag": "v3",
        "jsonPointer": "spec/initContainer/0/image"
      }
    }
  }
}
```
We are passing the `images` variable which has `containers`, `initContainers` and `ephimeralContainers` in it, containing information of all the images used.

Current Response Format:
```json
{
  "verified": true,
  "message": "...",
  "results": [
      {
         "name": "container1",
         "path":  "/spec/containers/0",
         "image":  "ghcr.io/kyverno/test-verify-image@sha256:b31bfb4d0213f254d361e0079deaaebefa4f82ba7aa76ef82e90b4935ad5b105"
      } 
  ]
}
```
It returns a verified `boolean` that indicated whether verification was successful or not, `message` which is an optional field, and `results` which is a list of JSONPatch compatible objects containing entries for every image.

We should add an array to the `requestData` called `attestationType` as follows
```json
{
  "images": {
    "containers": {
      "tomcat": {
        "registry": "https://ghcr.io",
        "path": "tomcat",
        "name": "tomcat",
        "tag": "9",
        "jsonPointer": "spec/container/0/image"
      }
    },
    "initContainers": {
      "vault": {
        "registry": "https://ghcr.io",
        "path": "vault",
        "name": "vault",
        "tag": "v3",
        "jsonPointer": "spec/initContainer/0/image"
      }
    }
  },
  "attestationType": [
    "application/vnd.cyclonedx",
    "sbom/cyclone-dx",
    "application/sarif+json"
  ]
}
```
Here is how it will look in the policy
```yaml
context:
- name: response
  apiCall:
    method: POST
    data:
    - key: images
      value: "{{ images }}"
    - key: attestationType
      value: |-
        - application/vnd.cyclonedx
        - sbom/cyclone-dx
        - application/sarif+json
    service:
      url: https://svc.kyverno-notation-aws/checkimages
      caBundle: |-
...
```
Here is a gist showing the entire policy: https://gist.github.com/Vishal-Chdhry/ec90442cf892c4d7db169ff45918615d

For every image we will check whether it has any or all of the attestations attached to it. When the attestations is found, we will return the following:
```json
{
  "verified": true,
  "message": "...",
  "results": [
      {
         "name": "container1",
         "path":  "/spec/containers/0",
         "image":  "ghcr.io/kyverno/test-verify-image@sha256:b31bfb4d0213f254d361e0079deaaebefa4f82ba7aa76ef82e90b4935ad5b105",
         "attestations": [
          {
            "type": "sbom/cyclone-dx",
            "digest": "sha256:b31bfb4d0213f254d361e0079deaaebefa4f82ba7aa76ef82e90b4935ad5b105",
            "payload": "the entire SBOM"
          }
         ]
      } 
  ]
}
``` 

For validation using this result, the policy should look like this
```yaml
validate:
  message: "not allowed"
  foreach:
    - list: response.results
      deny:
        conditions:
          any:
          - key: application/vnd.cyclonedx # To shortcircuit the check if the type is not present in the image
            operator: AnyNotIn
            value: "{{ element.attestation.type[] }}"
          - key: "{{ response.results.attestation[?type = 'application/vnd.cyclonedx'].acomponents[].licenses[].expression }}"
            operator: AllNotIn
            value: ["GPL-2.0", "GPL-3.0"]
```

Here is a gist showing the entire policy: https://gist.github.com/Vishal-Chdhry/ec90442cf892c4d7db169ff45918615d


# Implementation

## Fetching Artifacts
We can use the same logic as used in notary implementation for kyverno.
1. Use `Referrers` method in [remote pkg](https://pkg.go.dev/github.com/google/go-containerregistry/pkg/v1/remote#Referrers) to get a list of all the referrers in the image.
2. Match the list to the list of `attestationType` to filter all the desired attestations.

Note: We should convert the `attestationType` array recieved to a `map[string]bool` to reduce time complexity while matching artifacts
## Verifying Artifacts

Since, the flow for verification of images and attestations are really similar as the signature is attached to the digest of image and attestation in the exact same way, we can just reuse the `verifyImage(ctx context.Context, image string)` method used for image verification by passing it the url of the attestation that we recieved above. 

## Fetching payloads
We can use the gcr crane package for this
1. Get the manifest using `crane.Manifest(ref string, opt ...crane.Option)`.
2. Get the digest of the first layer from the manifest.
3. Get the payload using `crane.PullLayer(ref string, opt ...crane.Option` 

## Link to the Implementation PR

# Migration (OPTIONAL)

This section should document breaks to public API and breaks in compatibility due to this KDP's proposed changes. In addition, it should document the proposed steps that one would need to take to work through these changes.

# Drawbacks

Why should we **not** do this?

# Alternatives

- What other designs have been considered?
- Why is this proposal the best?
- What is the impact of not doing this?

# Prior Art

Discuss prior art, both the good and bad.

# Unresolved Questions

- What parts of the design do you expect to be resolved before this gets merged?
- What parts of the design do you expect to be resolved through implementation of the feature?
- What related issues do you consider out of scope for this KDP that could be addressed in the future independently of the solution that comes out of this KDP?
