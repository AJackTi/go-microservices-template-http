# Release process

Releases are tag-driven.

When a `v*` tag is pushed, GitHub Actions:

1. Builds each application image for `linux/amd64` and `linux/arm64`.
2. Pushes the image to GHCR.
3. Signs the image digest with Cosign using GitHub OIDC.

The release workflow is intentionally container-first so the same artifact
pipeline covers local development and published images.

