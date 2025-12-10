FROM gcr.io/distroless/static:latest@sha256:4b2a093ef4649bccd586625090a3c668b254cfe180dee54f4c94f3e9bd7e381e
WORKDIR /
COPY manager manager
USER 65532:65532

ENTRYPOINT ["/manager"]
