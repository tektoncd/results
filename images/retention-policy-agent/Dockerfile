# Docker configuration to build Tekton Results apiserver image
FROM registry.access.redhat.com/ubi9/go-toolset:1.22.5 AS builder

WORKDIR /opt/app-root/src

COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor/ vendor/
COPY cmd/ cmd/
COPY proto/ proto/
COPY pkg/ pkg/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o retention-policy-agent cmd/retention-policy-agent/main.go

FROM registry.access.redhat.com/ubi9-minimal:9.5
COPY --from=builder /opt/app-root/src/retention-policy-agent /usr/local/bin/retention-policy-agent
USER 65532:65532

ENTRYPOINT ["retention-policy-agent"]
