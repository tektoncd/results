# Tekton Results REST API Specification

**NOTE:** This documentation is only for refernce, please use [Swaggar UI](https://petstore.swagger.io/) (or similar services) with link to the [openapi spec](openapi.yaml) to see complete information.

## Version: v1alpha2

[See Results API Documentation](https://github.com/tektoncd/results/tree/main/docs/api)

### `/v1alpha2/parents/{parent}/results`

#### GET

##### Summary

Get the list of the Results

##### Parameters

| Name | Required | Schema |
| ---- | -------- | ---- |
| parent | Yes | [parent](#parent) |

##### Responses

| Code | Description |
| ---- | ----------- |
| 200 | [Results List](#results-list-response) |

---

### `/v1alpha2/parents/{parent}/results/{result_uid}/records/{record_uid}`

#### GET

##### Summary

Get a record given the uid

##### Parameters

| Name | Required | Schema |
| ---- | -------- | ---- |
| parent | Yes | [parent](#parent) |
| result_uid | Yes | [result_uid](#result_uid) |
| record_uid | Yes | [record_uid](#record_uid) |

##### Responses

| Code | Description |
| ---- | ----------- |
| 200 | [Record](#record-response) |

#### POST

##### Summary

Create a record with given uid

##### Parameters

| Name | Required | Schema |
| ---- | -------- | ---- |
| parent | Yes | [parent](#parent) |
| result_uid | Yes | [result_uid](#result_uid) |
| record_uid | Yes | [record_uid](#record_uid) |

##### Responses

| Code | Description |
| ---- | ----------- |
| 200 | [Record](#record-response) |

#### DELETE

##### Summary

Delete a record given the uid

##### Parameters

| Name | Required | Schema |
| ---- | -------- | ---- |
| parent | Yes | [parent](#parent) |
| result_uid | Yes | [result_uid](#result_uid) |
| record_uid | Yes | [record_uid](#record_uid) |

##### Responses

| Code | Description |
| ---- | ----------- |
| 200 | {} |

#### PATCH

##### Summary

Update a record with given uid

##### Parameters

| Name | Required | Schema |
| ---- | -------- | ---- |
| parent | Yes | [parent](#parent) |
| result_uid | Yes | [result_uid](#result_uid) |
| record_uid | Yes | [record_uid](#record_uid) |

##### Responses

| Code | Description |
| ---- | ----------- |
| 200 | [Record](#record-response) |

---

### `/v1alpha2/parents/{parent}/results/{result_uid}`

#### GET

##### Summary

Get a single result given the UID

##### Parameters

| Name | Required | Schema |
| ---- | -------- | ---- |
| parent | Yes | [parent](#parent) |
| result_uid | Yes | [result_uid](#result_uid) |

##### Responses

| Code | Description |
| ---- | ----------- |
| 200 | [Result](#result-response) |

#### POST

##### Summary

Create a Result given data and UID

##### Parameters

| Name | Required | Schema |
| ---- | -------- | ---- |
| parent | Yes | [parent](#parent) |
| result_uid | Yes | [result_uid](#result_uid) |

##### Responses

| Code | Description |
| ---- | ----------- |
| 200 | [Result](#result-response) |

#### DELETE

##### Summary

Delete a particular result using UID

##### Parameters

| Name | Required | Schema |
| ---- | -------- | ---- |
| parent | Yes | [parent](#parent) |
| result_uid | Yes | [result_uid](#result_uid) |

##### Responses

| Code | Description |
| ---- | ----------- |
| 200 | {} |

#### PATCH

##### Summary

Update result given the UID

##### Parameters

| Name | Required | Schema |
| ---- | -------- | ---- |
| parent | Yes | [parent](#parent) |
| result_uid | Yes | [result_uid](#result_uid) |

##### Responses

| Code | Description |
| ---- | ----------- |
| 200 | [Result](#result-response) |

---

### `/v1alpha2/parents/{parent}/results/{result_uid}/logs`

#### GET

##### Summary

List Logs given the Result UID

##### Parameters

| Name | Required | Schema |
| ---- | -------- | ---- |
| parent | Yes | [parent](#parent) |
| result_uid | Yes | [result_uid](#result_uid) |

##### Responses

| Code | Description |
| ---- | ----------- |
| 200 | [Logs List](#records-list-response) |

---

### `/v1alpha2/parents/{parent}/results/{result_uid}/logs/{log_uid}`

#### GET

##### Summary

Get a Log given UID

##### Parameters

| Name | Required | Schema |
| ---- | -------- | ---- |
| parent | Yes | [parent](#parent) |
| result_uid | Yes | [result_uid](#result_uid) |
| log_uid | Yes | [log_uid](#log_uid) |

##### Responses

| Code | Description |
| ---- | ----------- |
| 200 | [Log](#record-response) |

#### DELETE

##### Summary

Delete a log given the UID

##### Parameters

| Name | Required | Schema |
| ---- | -------- | ---- |
| parent | Yes | [parent](#parent) |
| result_uid | Yes | [result_uid](#result_uid) |
| log_uid | Yes | [log_uid](#log_uid) |

##### Responses

| Code | Description |
| ---- | ----------- |
| 200 | {} |

---

### `/v1alpha2/parents/{parent}/results/{result_uid}/records`

#### GET

##### Summary

Get list of records

##### Parameters

| Name | Required | Schema |
| ---- | -------- | ---- |
| parent | Yes | [parent](#parent) |
| result_uid | Yes | [result_uid](#result_uid) |

##### Responses

| Code | Description |
| ---- | ----------- |
| 200 | [Records List](#records-list-response) |

## Parameters Description

### parent

| Type | Located in | Description |
| ---- | ---------- | ----------- |
| string | path | Parent name refers to the namespace name or workspace name. |

### result_uid

| Type | Located in | Description |
| ---- | ---------- | ----------- |
| string | path | Result UID is the server assigned identifier of the result. |

### record_uid

| Type | Located in | Description |
| ---- | ---------- | ----------- |
| string | path | Record UID is the server assigned identifier of the Record. |

### logs_uid

| Type | Located in | Description |
| ---- | ---------- | ----------- |
| string | path | It is an alias to the record uid denoting a log. |

### filter

| Type | Located in | Description |
| ---- | ---------- | ----------- |
| string | query | Add a CEL Expression Filter. See [here](README.md#filtering) for reference. |

### page_size

| Type | Located in | Description |
| ---- | ---------- | ----------- |
| integer | query | This query can be used for pagination. |

### page_token

| Type | Located in | Description |
| ---- | ---------- | ----------- |
| string | query | This can be used to fetch a particular page when response are paginated. |

### order_by

| Type | Located in | Description |
| ---- | ---------- | ----------- |
| string | query | This query can be used to order the response based on `created_on` or `updated_on`. See [here](README.md#ordering) for reference. |

## Responses Example

### Result Response

```json
{
  "name": "default/results/640d1af3-9c75-4167-8167-4d8e4f39d403",
  "id": "338481c9-3bc6-472f-9d1b-0f7705e6cb8c",
  "uid": "338481c9-3bc6-472f-9d1b-0f7705e6cb8c",
  "created_time": "2023-03-02T07:26:48.972907Z",
  "create_time": "2023-03-02T07:26:48.972907Z",
  "updated_time": "2023-03-02T07:26:54.191114Z",
  "update_time": "2023-03-02T07:26:54.191114Z",
  "annotations": {},
  "etag": "338481c9-3bc6-472f-9d1b-0f7705e6cb8c-1677742014191114634",
  "summary": {
    "record": "default/results/640d1af3-9c75-4167-8167-4d8e4f39d403/records/640d1af3-9c75-4167-8167-4d8e4f39d403",
    "type": "tekton.dev/v1beta1.TaskRun",
    "start_time": null,
    "end_time": "2023-03-02T07:26:54Z",
    "status": "SUCCESS",
    "annotations": {}
  }
}
```

### Results List Response

```json
{
  "results": [
    {
      "name": "default/results/640d1af3-9c75-4167-8167-4d8e4f39d403",
      "id": "338481c9-3bc6-472f-9d1b-0f7705e6cb8c",
      "uid": "338481c9-3bc6-472f-9d1b-0f7705e6cb8c",
      "created_time": "2023-03-02T07:26:48.972907Z",
      "create_time": "2023-03-02T07:26:48.972907Z",
      "updated_time": "2023-03-02T07:26:54.191114Z",
      "update_time": "2023-03-02T07:26:54.191114Z",
      "annotations": {},
      "etag": "338481c9-3bc6-472f-9d1b-0f7705e6cb8c-1677742014191114634",
      "summary": {
        "record": "default/results/640d1af3-9c75-4167-8167-4d8e4f39d403/records/640d1af3-9c75-4167-8167-4d8e4f39d403",
        "type": "tekton.dev/v1beta1.TaskRun",
        "start_time": null,
        "end_time": "2023-03-02T07:26:54Z",
        "status": "SUCCESS",
        "annotations": {}
      }
    },
    {
      "name": "default/results/c360def0-d77e-4a3f-a1b0-5b0753e7d5af",
      "id": "9514f318-9329-485b-871c-77a4a6904891",
      "uid": "9514f318-9329-485b-871c-77a4a6904891",
      "created_time": "2023-03-02T07:28:05.535047Z",
      "create_time": "2023-03-02T07:28:05.535047Z",
      "updated_time": "2023-03-02T07:28:10.308632Z",
      "update_time": "2023-03-02T07:28:10.308632Z",
      "annotations": {},
      "etag": "9514f318-9329-485b-871c-77a4a6904891-1677742090308632274",
      "summary": {
        "record": "default/results/c360def0-d77e-4a3f-a1b0-5b0753e7d5af/records/c360def0-d77e-4a3f-a1b0-5b0753e7d5af",
        "type": "tekton.dev/v1beta1.TaskRun",
        "start_time": null,
        "end_time": "2023-03-02T07:28:10Z",
        "status": "SUCCESS",
        "annotations": {}
      }
    }
  ],
  "next_page_token": ""
}
```

### Record Response

```json
{
  "name": "default/results/640d1af3-9c75-4167-8167-4d8e4f39d403/records/640d1af3-9c75-4167-8167-4d8e4f39d403",
  "id": "df3904b8-a6b8-468a-9e3f-8b9386bf3673",
  "uid": "df3904b8-a6b8-468a-9e3f-8b9386bf3673",
  "data": {
    "type": "tekton.dev/v1beta1.TaskRun",
    "value": "VGhpcyBpcyBhbiBleG1hcGxlIG9mIHJlY29yZCBkYXRhCg=="
  },
  "etag": "df3904b8-a6b8-468a-9e3f-8b9386bf3673-1677742019012643389",
  "created_time": "2023-03-02T07:26:48.997424Z",
  "create_time": "2023-03-02T07:26:48.997424Z",
  "updated_time": "2023-03-02T07:26:59.012643Z",
  "update_time": "2023-03-02T07:26:59.012643Z"
}
```

### Records List Response

```json
{
  "records": [
    {
      "name": "default/results/640d1af3-9c75-4167-8167-4d8e4f39d403/records/640d1af3-9c75-4167-8167-4d8e4f39d403",
      "id": "df3904b8-a6b8-468a-9e3f-8b9386bf3673",
      "uid": "df3904b8-a6b8-468a-9e3f-8b9386bf3673",
      "data": {
        "type": "tekton.dev/v1beta1.TaskRun",
        "value": "VGhpcyBpcyBhbiBleG1hcGxlIG9mIHJlY29yZCBkYXRhCg==="
      },
      "etag": "df3904b8-a6b8-468a-9e3f-8b9386bf3673-1677742019012643389",
      "created_time": "2023-03-02T07:26:48.997424Z",
      "create_time": "2023-03-02T07:26:48.997424Z",
      "updated_time": "2023-03-02T07:26:59.012643Z",
      "update_time": "2023-03-02T07:26:59.012643Z"
    },
    {
      "name": "default/results/640d1af3-9c75-4167-8167-4d8e4f39d403/records/77add742-5361-3b14-a1d3-2dae7e4977b2",
      "id": "62e52c4d-9a61-4cf0-8f88-e816fcb0f84a",
      "uid": "62e52c4d-9a61-4cf0-8f88-e816fcb0f84a",
      "data": {
        "type": "results.tekton.dev/v1alpha2.Log",
        "value": "VGhpcyBpcyBhbiBleG1hcGxlIG9mIHJlY29yZCBkYXRhCg=="
      },
      "etag": "62e52c4d-9a61-4cf0-8f88-e816fcb0f84a-1677742014245938484",
      "created_time": "2023-03-02T07:26:54.220068Z",
      "create_time": "2023-03-02T07:26:54.220068Z",
      "updated_time": "2023-03-02T07:26:54.245938Z",
      "update_time": "2023-03-02T07:26:54.245938Z"
    }
  ],
  "next_page_token": ""
}
```

### Get Log Response
REST implementation of GetLog directly sends the bytes with content type "text/plain"
```shell
[prepare] 2023/10/09 11:30:04 Entrypoint initialization

[hello] hello

%!s(<nil>)

```