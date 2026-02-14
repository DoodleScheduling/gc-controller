FROM gcr.io/distroless/static:latest@sha256:d90359c7a3ad67b3c11ca44fd5f3f5208cbef546f2e692b0dc3410a869de46bf
WORKDIR /
COPY manager manager
USER 65532:65532

ENTRYPOINT ["/manager"]
