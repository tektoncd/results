# Docker configuration to build Tekton Results Migrator image
FROM registry.access.redhat.com/ubi9/go-toolset:1.19 AS builder

WORKDIR /opt/app-root/src

COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor/ vendor/
COPY cmd/ cmd/
COPY proto/ proto/
COPY pkg/ pkg/
COPY tools/ tools/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o migrator ./tools/migrator/

FROM registry.access.redhat.com/ubi9-minimal:9.1.0
COPY --from=builder /opt/app-root/src/migrator /usr/local/bin/migrator
USER 65532:65532

ENTRYPOINT ["migrator"]
