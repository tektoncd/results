<!--
---
linkTitle: "Results Metrics"
weight: 304
---
-->

# Results Watcher Metrics

The following pipeline metrics are available at `tekton-results-watcher` on port `9090`.

We expose several kinds of exporters, including Prometheus, Google Stackdriver, and many others. You can set them up
using [config-observability](../config/base/config-observability.yaml).

| Name                                                               | Meaning                                                                   | Type                       | Labels/Tags                                                                                                                                                                                | Status       |
|--------------------------------------------------------------------|---------------------------------------------------------------------------|----------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------|
| `watcher_pipelinerun_delete_duration_seconds_[bucket, sum, count]` | The duration that watcher take to delete the PipelineRun since completion | Histogram/LastValue(Gauge) | `*pipeline`=&lt;pipeline_name&gt; <br> `status`=&lt;status&gt; <br> `namespace`=&lt;pipelinerun-namespace&gt;                                                                              | experimental |
| `watcher_taskrun_delete_duration_seconds_[bucket, sum, count]`     | The duration that take to delete the TaskRun since completion             | Histogram/LastValue(Gauge) | `*pipeline`=&lt;pipeline_name&gt; <br> `status`=&lt;status&gt; <br> `*task`=&lt;task_name&gt; <br> `*taskrun`=&lt;taskrun_name&gt;<br> `namespace`=&lt;pipelineruns-taskruns-namespace&gt; | experimental |
| `watcher_pipelinerun_delete_count`                                 | The total count of deleted PipelineRun                                    | Counter                    | `status`=&lt;status&gt; <br> `namespace`=&lt;pipelinerun-namespace&gt;                                                                                                                     | experimental |
| `watcher_taskrun_delete_count`                                     | The total count of deleted TaskRun                                        | Counter                    | `status`=&lt;status&gt; <br> `namespace`=&lt;pipelinerun-namespace&gt;                                                                                                                     | experimental |

The Labels/Tag marked as "*" are optional. And there's a choice between Histogram and LastValue(Gauge) for pipelinerun
and taskrun delete duration metrics.

## Configuring Metrics using `config-observability` configmap

A sample config-map has been provided as [config-observability](./../config/base/config-observability.yaml). By default,
taskrun and pipelinerun metrics have these values:

``` yaml
    metrics.taskrun.level: "task"
    metrics.taskrun.duration-type: "histogram"
    metrics.pipelinerun.level: "pipeline"
    metrics.pipelinerun.duration-type: "histogram"
```

Following values are available in the configmap:

| configmap data                    | value       | description                                                                                |
|-----------------------------------|-------------|--------------------------------------------------------------------------------------------|
| metrics.taskrun.level             | `task`      | Level of metrics is task and taskrun label isn't present in the metrics                    |
| metrics.taskrun.level             | `namespace` | Level of metrics is namespace, and task and taskrun label isn't present in the metrics     |
| metrics.pipelinerun.level         | `pipeline`  | Level of metrics is pipeline and pipelinerun label isn't present in the metrics            |
| metrics.pipelinerun.level         | `namespace` | Level of metrics is namespace, pipeline and pipelinerun label isn't present in the metrics |
| metrics.taskrun.duration-type     | `histogram` | `watcher_taskrun_delete_duration_seconds` is of type histogram                             |
| metrics.taskrun.duration-type     | `lastvalue` | `watcher_taskrun_delete_duration_seconds` is of type lastvalue                             |
| metrics.pipelinerun.duration-type | `histogram` | `watcher_pipelinerun_delete_duration_seconds` is of type histogram                         |
| metrics.pipelinerun.duration-type | `lastvalue` | `watcher_pipelinerun_delete_duration_seconds` is of type lastvalue                         |

To check that appropriate values have been applied in response to configmap changes, use the following commands:

```shell
kubectl port-forward -n tekton-pipelines service/tekton-results-watcher 9090
```

And then check that changes have been applied to metrics coming from `http://127.0.0.1:9090/metrics`
