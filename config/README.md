# Tekton Results Kustomization

The `/config` directory contains [kustomization](https://kustomize.io/) files
that can be used to deploy Tekton Results on Kubernetes.
The directories are structured as follows:

- `base`: holds the core deployments and configurations for Results.
- `components`: contains patches and additional resources to enable specific
  features.
- `overlays`: contains concrete implementations with a specific feature set
  enabled.

## Deploying with a Kustomization

Results can be deployed from source by running the following:

```sh
kubectl kustomize config/overlays/<overlay> | ko apply -f - 
```

_Note: the steps above assume that you have a development environment set up
with `kubectl` and `ko`. See [DEVELOPMENT.md](../docs/DEVELOPMENT.md) for more information._

## Contributing a new Kustomization

New Kustomizations should adhere to the following guidelines:

- Functionality should be enabled through a kustomize [Component], added in the `components` directory.
- An overlay that deploys Results with the capability enabled should be added to the `overlays` directory.
- The overlay should contain a `kustomization.yaml` file that has the following:
  - Begins with `../../base` as a Resource.
  - Adds/enables functionality with components, referencing a directory in the `../../components` folder.
  - Adds `../../components/metadata` as the final Component.

Be sure to test your overlay and components before submitting a pull request.
