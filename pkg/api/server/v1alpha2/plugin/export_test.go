// Package plugin provides log plugin functionality for the API server.
package plugin

// exports for tests

var MergeLogParts = mergeLogParts
var GetLokiLogs = getLokiLogs
var GetSplunkLogs = getSplunkLogs
var GetBlobLogs = getBlobLogs

var HTTPStatusToCode = httpStatusToCode
var TransportErrorToCode = transportErrorToCode
var BlobCodeToGRPC = blobCodeToGRPC
var CodeToHTTPStatus = codeToHTTPStatus
