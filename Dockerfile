# The server binary is CGO-linked against glibc (gotreesitter, corkscrewdb),
# so it needs a glibc base — musl Alpine lacks its ELF interpreter. Use a
# slim Debian base and an explicit numeric UID so Kubernetes can verify the
# container runs non-root.
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r -g 10001 rostrum \
    && useradd -r -u 10001 -g rostrum -s /usr/sbin/nologin rostrum

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
    DEMO_MODE=false
EXPOSE 8080

ENTRYPOINT ["/app/rostrum"]
