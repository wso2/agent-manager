# Mock LLM backend for E2E tests (see main.go). Standard library only.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY main.go .
RUN CGO_ENABLED=0 go build -o /mockllm main.go

FROM alpine:3.22
COPY --from=build /mockllm /mockllm
EXPOSE 8000
USER 65534
ENTRYPOINT ["/mockllm"]
