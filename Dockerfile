# Rostrum builds to a pure-Go, statically-linked binary (Makefile sets
# CGO_ENABLED=0) — gotreesitter exists precisely to avoid cgo, so there is no
# libc dependency. A lean Alpine base is plenty (it would even run on scratch).
# The explicit numeric UID lets Kubernetes verify the container runs non-root.
FROM alpine:3.23

ARG ROSTRUM_VERSION=dev

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 rostrum \
    && adduser -S -u 10001 -G rostrum rostrum

WORKDIR /app
COPY --chown=rostrum:rostrum dist/server/app ./rostrum
COPY --chown=rostrum:rostrum dist/app ./app
COPY --chown=rostrum:rostrum dist/assets ./assets
COPY --chown=rostrum:rostrum dist/public ./public
COPY --chown=rostrum:rostrum dist/data ./data
COPY --chown=rostrum:rostrum dist/build.json dist/gosx-grammar.blob ./
RUN mkdir -p /app/data/uploads && chown -R rostrum:rostrum /app

USER 10001
ENV PORT=8080 \
    DATA_PATH=/app/data/rostrum.json \
    DEMO_MODE=false \
    ROSTRUM_VERSION=${ROSTRUM_VERSION}
EXPOSE 8080

ENTRYPOINT ["/app/rostrum"]
