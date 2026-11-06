FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/golang:1.26.10-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src/deployments/dev/observer-lite

# Build with `--build-context amp-observer=<repo>/agent-manager-observer`: go.mod
# replaces amp-observer with a sibling directory.
COPY --from=amp-observer go.mod go.sum /src/agent-manager-observer/
COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY --from=amp-observer . /src/agent-manager-observer/
COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s" \
    -buildvcs=false \
    -o /out/observer-lite .

FROM public.ecr.aws/docker/library/alpine:3.24

RUN addgroup -g 1000 app && adduser -D -u 1000 -G app app \
    && mkdir -p /tmp/observer-lite && chown 1000:1000 /tmp/observer-lite

COPY --from=builder --chown=1000:1000 /out/observer-lite /observer-lite

USER 1000:1000

EXPOSE 8080 4318

ENTRYPOINT ["/observer-lite"]
