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
    taskrun.yaml       # optional: a tektonv1.TaskRun skeleton for child records
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
- `taskrun.yaml` — a TaskRun skeleton used to synthesize the child records. The
  generator overwrites the same identity fields plus the owner reference back to
  the parent PipelineRun. If omitted, a minimal built-in TaskRun is used.
- `template.yaml` — optional metadata:

  ```yaml
  id: konflux-build       # defaults to the directory name
  weight: 3.0             # relative selection probability (default 1.0)
  childTaskRuns:          # per-template child count range (default 2..15)
    min: 8
    max: 15
  description: "Konflux-style container build with 8-15 tasks"
  ```

## Determinism and versioning

Template content is part of the dataset version. **Changing any template changes
the generated data**, so bump `Config.Version` (and regenerate the golden
`datasets/*.md` hash) whenever templates are added or edited. Results are only
comparable within the same dataset version.

## Provided sample

`sample/` is a structurally-representative placeholder so the framework builds
and tests run before the real anonymized manifests land. Replace it with 5–10
real templates before capturing a baseline.
