# Filtering

The reference implementation of the Results API uses
[CEL](https://github.com/google/cel-spec/blob/master/doc/langdef.md) as a
filtering spec. Filter specs expect a boolean result value.

Known types exposed to each RPC method are documented below.

## ListResults

| Known Types | Description                                     |
| ----------- | ----------------------------------------------- |
| `result`    | [Result Object](/proto/v1alpha2/resources.proto) |

## ListRecords

| Known Types | Description                                     |
| ----------- | ----------------------------------------------- |
| `record`    | [Record Object](/proto/v1alpha2/resources.proto) |

### Cookbook

| Filter Spec                                            | Description                                           |
| ------------------------------------------------------ | ----------------------------------------------------- |
| `record.name.startsWith("foo/results/bar")`            | Get all Records belonging to Result `foo/results/bar` |
| `type(record.data) == tekton.pipeline.v1beta1.TaskRun` | Get all Records of type TaskRun                       |
