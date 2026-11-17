# Generator templates

The benchmark generator instantiates realistic PipelineRun/TaskRun objects from
the templates in this directory. Templates are **anonymized real manifests**
captured from an actual CI deployment — strip secrets, credentials, tokens and
internal hostnames, but keep the structural shape: full `status`, conditions,
results, params, and the real label/annotation sets
(`tekton.dev/pipeline`, `pipelinesascode.tekton.dev/*`, `appstudio.openshift.io/*`,
etc.).

## Contract

Each template is a **directory** under `templates/`:

```
templates/
  <template-id>/
    pipelinerun.yaml   # required: a tektonv1.PipelineRun manifest
    taskruns/          # optional: a directory of weighted TaskRun skeletons
      build.yaml
      clone-repository.yaml
      ...
    taskrun.yaml       # optional: a single TaskRun skeleton (ignored if taskruns/ exists)
    template.yaml      # optional: metadata (see below)
```

- `pipelinerun.yaml` — a valid `tekton.dev/v1` PipelineRun. The generator
  **overwrites** the identity/distribution fields on every instance:
  `metadata.name`, `metadata.namespace`, `metadata.uid`,
  `metadata.creationTimestamp`, a configured subset of `metadata.labels`,
  `status.startTime`, `status.completionTime`, `status.conditions[Succeeded]`,
  and `status.childReferences`. Everything else (spec, params, results, the
  remaining labels/annotations) is kept verbatim, so the object's serialized
  size is determined by the template — pick templates that cover the real size
  distribution (~5 KB small runs up to 50+ KB large runs, median 15–20 KB).
- `taskruns/` — a directory of `*.yaml` child TaskRun skeletons. Each child of a
  PipelineRun is drawn from this set **by weight**, so one PipelineRun's children
  reproduce the real per-task size spread (e.g. an 8 KB `init` next to a 57 KB
  `build`). Every skeleton is named after its file basename (without `.yaml`) and
  defaults to weight `1.0`; override individual weights via `childSkeletonWeights`
  in `template.yaml`. Skeletons are loaded in sorted filename order for
  determinism. `taskruns/` takes precedence over a single `taskrun.yaml`.

  A weight is a **relative selection frequency, not a size** — it controls how
  often that skeleton is picked, not how large it is (the KB size comes from the
  file's own content). A skeleton with weight `4` among nine others at `1.0` is
  chosen `4/13 ≈ 31%` of the time. To match a real cluster, set each weight to how
  frequently that TaskRun type actually occurs. The pick is pseudo-random but
  **fully deterministic**: it is driven by the per-instance seeded RNG, so the
  same `(seed, index)` always selects the same child, and a single-skeleton
  template draws no random value at all.
- `taskrun.yaml` — a single TaskRun skeleton used to synthesize the child records
  when no `taskruns/` directory is present. The generator overwrites the same
  identity fields plus the owner reference back to the parent PipelineRun. If both
  `taskruns/` and `taskrun.yaml` are absent, a minimal built-in TaskRun is used.
- `template.yaml` — optional metadata:

  ```yaml
  id: konflux-build       # defaults to the directory name
  weight: 3.0             # relative selection probability (default 1.0)
  childTaskRuns:          # per-template child count range (default 2..15)
    min: 8
    max: 15
  childSkeletonWeights:   # per-skeleton selection weights (default 1.0 each);
    build: 4              # keys are taskruns/ file basenames without .yaml
  description: "Konflux-style container build with 8-15 tasks"
  ```

## Determinism and versioning

Template content is part of the dataset version. **Changing any template changes
the generated data**, so bump `Config.Version` (and regenerate the golden
`datasets/*.md` hash) whenever templates are added or edited. Results are only
comparable within the same dataset version.

## Layout and provided templates

`DefaultTemplates` loads only the directories under **`templates/default/`** — that
is the active set that defines the seed dataset. Templates elsewhere under
`templates/` are reference examples: embedded and loadable on demand, but not part
of the default dataset and with no effect on its golden hash.

- **`default/simple/`** *(default)* — a lightweight two-skeleton build: a small
  `clone-repository` next to a larger `build-container`, with a
  `childSkeletonWeights` override so the larger skeleton is picked more often. It
  exercises the **weighted child mix** while keeping generated objects small, so
  default runs and tests stay fast. This is the copy-paste starting point for a
  new template.
- **`fbc-builder/`** *(reference)* — the anonymized real Konflux FBC build with 10
  child skeletons up to ~57 KB. Realistic but heavy; kept out of `templates/default`
  so it doesn't slow default runs. Move it under `templates/default/` (and bump the
  dataset version) to benchmark against large manifests.
- **`docker-build/`** *(reference)* — a multi-platform container build pipeline with
  security scanning (Clair, Snyk, ClamAV) and compliance tasks. Builds for
  linux/arm64 and linux/x86_64 using matrix tasks (~14 child TaskRuns). Heavier
  than fbc-builder with more complex task graphs; kept out of `templates/default`
  to avoid slowing test runs.
- **`build-multiplatform/`** *(reference)* — a multi-platform ML/AI base image build
  with CUDA support. Hermetic builds with RPM and pip dependency prefetching for
  linux/arm64 and linux/x86_64. Smaller child count (5-10 TaskRuns) than
  docker-build but includes AI/ML toolchain specifics. Kept out of
  `templates/default` to avoid slowing test runs.
- **`sample/`** *(reference)* — a minimal placeholder using a single
  `taskrun.yaml` skeleton, kept as the smallest possible example.

To add a template to the dataset, place its directory under `templates/default/` and
bump the dataset version. Replace these with 5–10 real templates before capturing
a baseline.
