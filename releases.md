# Tekton Results Releases

## Release Frequency

Tekton Results follows the Tekton community [release policy][release-policy]
as follows:

- Versions are numbered according to semantic versioning: `vX.Y.Z`
- A new release is produced on a monthly basis
- LTS releases are supported for approximately one year; all other releases are
  supported for approximately one month (until the next release is produced)
    - The first Tekton Results LTS release was **v0.10.0** in May 2024

## Release Process

Tekton Results releases are made of YAML manifests and container images.
Manifests are published to cloud object-storage as well as
[GitHub][tekton-results-releases]. Container images are signed by
[Sigstore][sigstore] via [Tekton Chains][tekton-chains]; signatures can be
verified through the [public key][chains-public-key] hosted by the Tekton Chains
project.

Further documentation available:

- [Installing Tekton Results][tekton-installation]
- Standard for [release notes][release-notes-standards]

## Release

### v0.20 (LTS)
- **Latest Release**: [v0.20.0][v0.20-0] (2026-08-04) ([docs][v0.20-0-docs])
- **Initial Release**: [v0.20.0][v0.20-0] (2026-08-04)
- **End of Life**: 2027-08-04
- **Patch Releases**: [v0.20.0][v0.20-0]

### v0.19 (LTS)
- **Latest Release**: [v0.19.0][v0.19-0] (2026-06-15) ([docs][v0.19-0-docs])
- **Initial Release**: [v0.19.0][v0.19-0] (2026-06-15)
- **End of Life**: 2027-06-15
- **Patch Releases**: [v0.19.0][v0.19-0]

### v0.18 (LTS)
- **Latest Release**: [v0.18.0][v0.18-0] (2026-01-18) ([docs][v0.18-0-docs])
- **Initial Release**: [v0.18.0][v0.18-0] (2026-01-18)
- **End of Life**: 2027-01-18
- **Patch Releases**: [v0.18.0][v0.18-0]

### v0.17 (LTS)
- **Latest Release**: [v0.17.2][v0.17-2] (2025-12-08) ([docs][v0.17-2-docs])
- **Initial Release**: [v0.17.0][v0.17-0] (2025-11-07)
- **End of Life**: 2026-11-07
- **Patch Releases**: [v0.17.0][v0.17-0], [v0.17.1][v0.17-1], [v0.17.2][v0.17-2]

### v0.16 (LTS)
- **Latest Release**: [v0.16.0][v0.16-0] (2025-08-18) ([docs][v0.16-0-docs])
- **Initial Release**: [v0.16.0][v0.16-0] (2025-08-18)
- **End of Life**: 2026-08-18
- **Patch Releases**: [v0.16.0][v0.16-0]

## End of Life Releases

### v0.15
- **Latest Release**: [v0.15.3][v0.15-3] (2025-07-01) ([docs][v0.15-3-docs])
- **Initial Release**: [v0.15.0][v0.15-0] (2025-06-01)
- **End of Life**: 2025-08-01
- **Patch Releases**: [v0.15.0][v0.15-0], [v0.15.1][v0.15-1], [v0.15.2][v0.15-2], [v0.15.3][v0.15-3]

### v0.14
- **Latest Release**: [v0.14.0][v0.14-0]
- **Initial Release**: [v0.14.0][v0.14-0]
- **End of Life**: 2025-06-01

### v0.13
- **Latest Release**: [v0.13.4][v0.13-4]
- **Initial Release**: [v0.13.0][v0.13-0]
- **End of Life**: 2025-05-01

### v0.12
- **Latest Release**: [v0.12.3][v0.12-3]
- **Initial Release**: [v0.12.0][v0.12-0]
- **End of Life**: 2025-04-01

### v0.11
- **Latest Release**: [v0.11.0][v0.11-0]
- **Initial Release**: [v0.11.0][v0.11-0]
- **End of Life**: 2024-09-01

### v0.10 (LTS)
- **Latest Release**: [v0.10.0][v0.10-0] (2024-05-10) ([docs][v0.10-0-docs])
- **Initial Release**: [v0.10.0][v0.10-0] (2024-05-10)
- **End of Life**: 2025-05-10
- **Patch Releases**: [v0.10.0][v0.10-0]

### v0.9
- **Latest Release**: [v0.9.0][v0.9-0]
- **Initial Release**: [v0.9.0][v0.9-0]
- **End of Life**: 2024-04-01

### v0.8
- **Latest Release**: [v0.8.0][v0.8-0]
- **Initial Release**: [v0.8.0][v0.8-0]
- **End of Life**: 2024-01-01

### v0.6
- **Latest Release**: [v0.6.0][v0.6-0]
- **Initial Release**: [v0.6.0][v0.6-0]
- **End of Life**: 2023-06-01

Older releases are EOL and available on [GitHub][tekton-results-releases].

[release-policy]: https://github.com/tektoncd/community/blob/main/releases.md
[sigstore]: https://sigstore.dev
[tekton-chains]: https://github.com/tektoncd/chains
[tekton-results-releases]: https://github.com/tektoncd/results/releases
[chains-public-key]: https://github.com/tektoncd/chains/blob/main/tekton.pub
[tekton-installation]: docs/install.md
[release-notes-standards]:
    https://github.com/tektoncd/community/blob/main/standards.md#release-notes

[v0.20-0]: https://github.com/tektoncd/results/releases/tag/v0.20.0
[v0.19-0]: https://github.com/tektoncd/results/releases/tag/v0.19.0
[v0.18-0]: https://github.com/tektoncd/results/releases/tag/v0.18.0
[v0.17-2]: https://github.com/tektoncd/results/releases/tag/v0.17.2
[v0.17-1]: https://github.com/tektoncd/results/releases/tag/v0.17.1
[v0.17-0]: https://github.com/tektoncd/results/releases/tag/v0.17.0
[v0.16-0]: https://github.com/tektoncd/results/releases/tag/v0.16.0
[v0.15-3]: https://github.com/tektoncd/results/releases/tag/v0.15.3
[v0.15-2]: https://github.com/tektoncd/results/releases/tag/v0.15.2
[v0.15-1]: https://github.com/tektoncd/results/releases/tag/v0.15.1
[v0.15-0]: https://github.com/tektoncd/results/releases/tag/v0.15.0
[v0.14-0]: https://github.com/tektoncd/results/releases/tag/v0.14.0
[v0.13-4]: https://github.com/tektoncd/results/releases/tag/v0.13.4
[v0.13-0]: https://github.com/tektoncd/results/releases/tag/v0.13.0
[v0.12-3]: https://github.com/tektoncd/results/releases/tag/v0.12.3
[v0.12-0]: https://github.com/tektoncd/results/releases/tag/v0.12.0
[v0.11-0]: https://github.com/tektoncd/results/releases/tag/v0.11.0
[v0.10-0]: https://github.com/tektoncd/results/releases/tag/v0.10.0
[v0.9-0]: https://github.com/tektoncd/results/releases/tag/v0.9.0
[v0.8-0]: https://github.com/tektoncd/results/releases/tag/v0.8.0
[v0.6-0]: https://github.com/tektoncd/results/releases/tag/v0.6.0

[v0.20-0-docs]: https://github.com/tektoncd/results/tree/v0.20.0/docs
[v0.19-0-docs]: https://github.com/tektoncd/results/tree/v0.19.0/docs
[v0.18-0-docs]: https://github.com/tektoncd/results/tree/v0.18.0/docs
[v0.17-2-docs]: https://github.com/tektoncd/results/tree/v0.17.2/docs
[v0.17-0-docs]: https://github.com/tektoncd/results/tree/v0.17.0/docs
[v0.16-0-docs]: https://github.com/tektoncd/results/tree/v0.16.0/docs
[v0.15-3-docs]: https://github.com/tektoncd/results/tree/v0.15.3/docs
[v0.10-0-docs]: https://github.com/tektoncd/results/tree/v0.10.0/docs
