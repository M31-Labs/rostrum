FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S rostrum \
    && adduser -S -G rostrum rostrum

WORKDIR /app
COPY --chown=rostrum:rostrum dist/server/app ./rostrum
COPY --chown=rostrum:rostrum dist/app ./app
COPY --chown=rostrum:rostrum dist/assets ./assets
COPY --chown=rostrum:rostrum dist/public ./public
COPY --chown=rostrum:rostrum dist/data ./data
COPY --chown=rostrum:rostrum dist/build.json dist/gosx-grammar.blob ./
RUN mkdir -p /app/data/uploads && chown -R rostrum:rostrum /app

USER rostrum
ENV PORT=8080 \
    DATA_PATH=/app/data/rostrum.json \
    DEMO_MODE=false
EXPOSE 8080

ENTRYPOINT ["/app/rostrum"]
