FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S programma \
    && adduser -S -G programma programma

WORKDIR /app
COPY --chown=programma:programma dist/server/app ./programma
COPY --chown=programma:programma dist/app ./app
COPY --chown=programma:programma dist/assets ./assets
COPY --chown=programma:programma dist/public ./public
COPY --chown=programma:programma dist/data ./data
COPY --chown=programma:programma dist/build.json dist/gosx-grammar.blob ./
RUN mkdir -p /app/data/uploads && chown -R programma:programma /app

USER programma
ENV PORT=8080 \
    DATA_PATH=/app/data/programma.json \
    DEMO_MODE=false
EXPOSE 8080

ENTRYPOINT ["/app/programma"]
