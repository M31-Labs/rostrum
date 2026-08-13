# This overlay contains only the fictional evaluation workspace and its
# approved portrait uploads. Build the normal Rostrum image first, then pass
# that exact immutable image as ROSTRUM_IMAGE. Nothing here is part of the
# production Rostrum image or its first-boot defaults.
ARG ROSTRUM_IMAGE=rostrum:local
FROM golang:1.26-alpine AS fixture

WORKDIR /src
COPY go.mod go.sum ./
COPY internal/domain ./internal/domain
COPY examples/demo ./examples/demo
RUN mkdir -p /out/uploads \
    && go run ./examples/demo/prepare \
        -workspace /out/initial-workspace.json \
        -checksum /out/initial-workspace.sha256 \
        -upload-checksums /out/uploads.sha256 \
        -assets /src/examples/demo/assets/headshots \
        -uploads /out/uploads \
        -stored-uploads /app/demo-data/uploads \
    && cp /src/examples/demo/rules/cfp-routing.arb /out/cfp-routing.arb \
    && sha256sum /out/cfp-routing.arb | awk '{print $1}' > /out/cfp-routing.sha256

FROM ${ROSTRUM_IMAGE}

COPY --from=fixture --chown=10001:10001 /out /opt/rostrum-demo
