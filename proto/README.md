# Regenerating proto libraries

Generating the proto libraries can be tricky without proper setup. This document
describes how to set up the environment to generate the proto libraries. Please
follow the steps below _exactly_ to avoid any issues.

## Pre-requisites

- Ensure that [Go](https://golang.org/doc/install) is installed and the
  `GOPATH` environment variable is set.
- Install [Proto Buffer](https://developers.google.com/protocol-buffers) compiler (`protoc`) version 3.0.0 or higher.

  - Download the latest version of `protoc` from [here](https://github.com/protocolbuffers/protobuf/releases/latest).
  - Unzip and copy the `protoc` binary to a directory in your `PATH` environment variable. i.e. for linux

    ```bash
    unzip protoc-24.3-linux-x86_64.zip -d $HOME/.local
    export PATH="$PATH:$HOME/.local/bin"
    ```

  - Ensure `protoc` is installed and reachable by running `protoc --version` in a terminal.
  - **Important** Ensure there is a `include` directory containing `proto` files. i.e. for linux

    ```bash
    # .local
    # ├── bin
    # │   └── protoc
    # └── include
    #     └── google
    #         └── protobuf
    #             └── <proto-files>
    ls $HOME/.local/include/google/protobuf
    any.proto  any_test.proto  api.proto  compiler  descriptor.proto  descriptor_test.proto  empty.proto  struct.proto  struct_test.proto  timestamp.proto  timestamp_test.proto  type.proto  type_test.proto  wrappers.proto  wrappers_test.proto
    ```

- Install plugins required for Tekton Results proto library generation.

  ```bash
  go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
  ```

## Generating the proto libraries

Here we will use go generate to generate the proto libraries. The proto libraries
are generated in the `proto` directory.

- Change the directory to `proto` directory.

  ```bash
  cd proto
  ```

- Make your changes to the proto files. If you are adding a new plugin please update
  this document with the steps to install the plugin.
- Run Go Generate

  ```bash
  go generate ./...
  ```

This will generate the proto libraries in the `proto` directory. Please report any issues
with the above steps.
