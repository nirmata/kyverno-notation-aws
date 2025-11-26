# Release Process

This document describes the process for creating new releases.

## Release Types

There are two types of releases:

1. Code releases (tagged as `v*`)
2. Helm chart releases (tagged as `charts-*`)

## Creating a Code Release

1. Ensure all tests pass and the main branch is up to date
2. Update the appVersion in `charts/kyverno-notation-aws/Chart.yaml`
2. Create and push a new tag with format `v*.*.*` (following semantic versioning)
   ```bash
   git tag v1.2.3
   git push origin v1.2.3
   ```
3. The GitHub Actions workflow will automatically:
    - Build and test the code
    - Create a new GitHub release
    - Build and publish container images

## Creating a Helm Chart Release

1. Update the chart version in `charts/kyverno-notation-aws/Chart.yaml`
2. Create and push a new tag with format `charts-*.*.*`
   ```bash
   git tag charts-1.2.3
   git push origin charts-1.2.3
   ```
3. The GitHub Actions workflow will automatically:
    - Package the Helm chart
    - Create a new GitHub release
    - Publish the chart to the Helm repository

## Version Numbers

- Follow [Semantic Versioning](https://semver.org/) (MAJOR.MINOR.PATCH)
- Code versions use format: `v1.2.3`
- Chart versions use format: `charts-1.2.3`
