#!/bin/bash

export KIND_CLUSTER_NAME=${KIND_CLUSTER_NAME:-"tekton-results"}
kind delete cluster --name=tekton-results