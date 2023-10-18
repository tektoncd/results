# Summary and Aggregation API

Summary and Aggregation API provides aggregated data for list of records. This endpoint is an extension for the list of
records, and you can utilize the full set of filters available for list of records and get a more accurate
summary/aggregation. Here is an example of the curl request for summary:

```shell
curl --insecure
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Accept: application/json" \
  https://localhost:8081/apis/results.tekton.dev/v1alpha2/parents/-/results/-/records/summary?summary=total
```

## Available aggregations

These are all the available aggregations. You can get them or a subset of them by specifying the `summary` parameter. If
nothing is specified, only the `total` will be returned.

| Field            | Type      | Description                                                      | Example     |
|------------------|-----------|------------------------------------------------------------------|-------------|
| `total`          | *integer* | total number of records for a given summary and group            | 10          |
| `succeeded`      | *integer* | number of records with 'Succeeded' status                        | 6           |
| `failed`         | *integer* | number of records with 'Failed' status                           | 2           |
| `cancelled`      | *integer* | number of records with 'Cancelled' status                        | 1           |
| `running`        | *integer* | number of records with 'Running' status                          | 0           |
| `others`         | *integer* | number of records with other statuses                            | 0           |
| `last_runtime`   | *integer* | last runtime of records in Unix seconds                          | 1701849793  |
| `avg_duration`   | *time*    | average duration of records in HH:mm:SS.ms format                | 00:02:42.95 |
| `min_duration`   | *time*    | minimum duration of records in HH:mm:SS.ms format                | 00:00:00.00 |
| `max_duration`   | *time*    | maximum duration of records in HH:mm:SS.ms format                | 00:05:00.00 |
| `total_duration` | *time*    | total duration of records in HH:mm:SS.ms format                  | 00:27:14.70 |
| `group_value`    | *any*     | value of the group field set using group_by, see the group table | -           |

### Summary Example

For `summary=total,succeeded,running,total_duration,avg_duration,last_runtime,min_duration,max_duration,failed,others,cancelled`
the output would be:

```json
{
  "summary": [
    {
      "avg_duration": "00:01:16.21875",
      "cancelled": 0,
      "failed": 3,
      "last_runtime": 1706102048,
      "max_duration": "00:03:18",
      "min_duration": "00:00:01",
      "others": 12,
      "running": 9,
      "succeeded": 91,
      "total": 115,
      "total_duration": "02:01:57"
    }
  ]
}
```

## Grouped Aggregations

You can group the summary based on a time duration or a field. You can specify the group by field using the `group_by`
parameter.

### Group by time duration

You can group the summary based on a time duration. By default, the `creationTimestamp` field is used. You can also
specify `completionTime` or `startTime` fields by using the `time field` format for `group_by` parameter. This will set
the `group_value` field to a number representing the Unix seconds.

You can determine what time duration was used for grouping by checking the most significant fields after converting the
Unix seconds to ISO timestamp. Time based grouping uses an absolute time quantum to define groups i.e. grouping by week
will create group with starting day of a week and not last 7 days. See the table below for examples.

| Group By | Example `group_value` | ISO Timestamp of the `group_value` | Remarks                                |
|----------|-----------------------|------------------------------------|----------------------------------------|
| `minute` | 1701849300            | 2023-12-06T07:55:00.000Z           | In the 55th minute of 7th hour.        |
| `hour`   | 1701846000            | 2023-12-06T07:00:00.000Z           | In the 7th hour of the day.            |
| `day`    | 1701782400            | 2023-12-06T00:00:00.000Z           | On the 6th day of the month.           |
| `week`   | 1701563200            | 2023-12-04T00:00:00.000Z           | Week starting on 4th day of the month. |
| `month`  | 1701427200            | 2023-12-01T00:00:00.000Z           | December of 2023                       |
| `year`   | 1680192000            | 2023-01-01T00:00:00.000Z           | Year of 2023                           |

### Group by field

You can group the summary based on `namespace`, `pipeline` or `repository`. You can specify the group by field using
the `group_by` parameter. This will set the `group_value` field to the string value of the group field.

| Group By     | Example `group_value`     |
|--------------|---------------------------|
| `namespace`  | `my-namespace`            |
| `pipeline`   | `namespace/my-pipeline`   |
| `repository` | `namespace/my-repository` |

### Group by Example

For `group_by=pipeline`, here is an example output, notice the `group_value` field:

```json
{
  "summary": [
    {
      "avg_duration": "00:00:40.5",
      "cancelled": 0,
      "failed": 2,
      "group_value": "default/git-hello-func",
      "last_runtime": 1706101658,
      "max_duration": "00:00:49",
      "min_duration": "00:00:32",
      "others": 0,
      "running": 0,
      "succeeded": 2,
      "total": 4,
      "total_duration": "00:02:42"
    },
    {
      "avg_duration": "00:01:29.461538",
      "cancelled": 0,
      "failed": 3,
      "group_value": "default/",
      "last_runtime": 1706102105,
      "max_duration": "00:03:54",
      "min_duration": "00:00:01",
      "others": 2,
      "running": 0,
      "succeeded": 112,
      "total": 117,
      "total_duration": "02:54:27"
    }
  ]
}
```

## Ordering

You can sort the summary output using any of the aggregation field as parameter. See
[available aggregation](#available-aggregations) for all the valid parameters. The only requirement is that the field
must be one of the `summary` parameter and a valid `group_by` is provided. You can get the out in ascending or
descending order. There is no default, you must pass two values for `order_by`. See the table for examples.

| Type       | Value to be passed                         |
|------------|--------------------------------------------|
| Ascending  | `ASC running` or `asc running              |
| Descending | `DESC avg_duration` or `desc avg_duration` |
