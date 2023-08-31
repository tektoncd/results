# Fetching large log files using Tekton Results API

Run logs can get huge, Tekton Results help archive logs away from the cluster
and free up space. You can fetch these logs using Tekton Results API. There are
multiple ways to fetch logs from Tekton Results API. You can use gRPC or REST API.

## Fetching logs

You can fetch logs using `grpc_cli` or `curl`. If using in a client, you can implement
the gRPC or REST client.

### Using `grpc_cli`

You can use `grpc_cli` or `grpcurl` to fetch logs using gRPC. Or you can use add
a gRPC client to your own application. Here are examples using `grpc_cli`.

```sh
grpc_cli call \
    --channel_creds_type=ssl \
    --ssl_target=tekton-results-api-service.tekton-pipelines.svc.cluster.local \
    --call_creds=access_token=$ACCESS_TOKEN localhost:8081 \
    tekton.results.v1alpha2.Logs.GetLog 'name:"default/results/93046b50-ff51-45bc-bb4c-de21c33f8b0f/logs/76d0c470-b3ff-3804-8f7d-b5276757eaf1"'
```

### Using REST API

You can use `curl` to fetch logs using REST API. Or you can use add a REST client
to your own application. Here are examples using `curl`.

```sh
curl --insecure \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "Accept: application/json" \
    https://localhost:8081/apis/results.tekton.dev/v1alpha2/parents/default/results/93046b50-ff51-45bc-bb4c-de21c33f8b0f/logs/76d0c470-b3ff-3804-8f7d-b5276757eaf1
```

## Fetching large logs

As logs can get huge, it can get difficult to fetch them due to client receive
and server send limitations. You can increase the limits using `LOGS_BUFFER_SIZE`
environment variable in the [config](../config/base/env/config) for normal use.

But the log size is nondeterministic, and it is not recommended increasing the limits
too much. Also, this workaround does won't be able to accommodate all log sizes.
As long as the log size stays under the `LOGS_BUFFER_SIZE` limit, the log is fetched
in a single chunk, and the response for REST is as below:

```json
{
    "result": {
        "name": "default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/logs/db6a5d59-2170-3367-9eb5-83f3d264ec62",
        "data": "W3ByZXBhcmVdIDIwMjMvMDgvMjkgMTE6MjA6MjYgRW50cnlwb2ludCBpbml0aWFsaXphdGlvbgoKW2hlbGxvXSBoZWxsbwoKJSFzKDxuaWw+KQo="
    }
}
```

Once the size exceeds the `LOGS_BUFFER_SIZE` limit, the log is fetched in chunks
and the response is as below:

```json
{
    "result": {
        "name": "default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/logs/db6a5d59-2170-3367-9eb5-83f3d264ec62",
        "data": "base64encodedchunkpart1"
    }
}
{
    "result": {
        "name": "default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/logs/db6a5d59-2170-3367-9eb5-83f3d264ec62",
        "data": "base64encodedchunkpart2"
    }
}
{
    "result": {
        "name": "default/results/0dfc883d-722a-4489-9ab8-3cccc74ca4f6/logs/db6a5d59-2170-3367-9eb5-83f3d264ec62",
        "data": "base64encodedchunkpartn"
    }
}
```

If you are fetching logs programmatically and parsing the logs, it might fail because
you can see that the response is not a valid JSON.

### Current state

The REST API in Tekton Results is not implemented independently, instead it is a
proxy layer on top of gRPC API. gRPC does support streaming, but the REST API
does not. So, the REST API instead sends data in chunks.

To make this work like normal log streaming works, we will need to change the code
such that the REST API sends logs as bytes instead of JSON. Currently, this solution
is not planned.

### Managing chunks in client

However, it is relatively easy to manage chunks from the client side. You can fetch all
the chunks, parse them as individual JSON, merge the data fields and then decode the
base64 encoded data. Or you can also decode the base64 encoded for each chunk and
then merge them sequentially.

Here is an example of how to do this in bash:

```sh
curl --insecure \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "Accept: application/json" \
    https://localhost:8081/apis/results.tekton.dev/v1alpha2/parents/default/results/93046b50-ff51-45bc-bb4c-de21c33f8b0f/logs/76d0c470-b3ff-3804-8f7d-b5276757eaf1 \
    | sed 's/^/[/; s/$/]/; s/}{/},{/g' \
    | jq -r 'map(.result.data) | join("")' \
    | base64 -d
```

Here we are fetching the data using `curl`, then using `sed` to convert the response
into a valid JSON array, then using `jq` to extract the `data` field from each JSON
and then joining them together and finally decoding the base64 encoded data.

Similarly, you can implement this logic in any general purpose programming language
and properly parse the JSONs and decode the base64 encoded data.
