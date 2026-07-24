# Runtime-only gateway image: copy a prebuilt linux binary (CI artifact).
# Avoids pulling golang:* inside DinD (slow / flaky on some runners).
# Build context should contain sandbox-gateway (e.g. artifacts/).
# Default base uses a CN Docker Hub mirror; override with --build-arg ALPINE_IMAGE=alpine:3.20 overseas.
ARG ALPINE_IMAGE=docker.m.daocloud.io/library/alpine:3.20
FROM ${ALPINE_IMAGE}
RUN apk add --no-cache ca-certificates docker-cli
COPY sandbox-gateway /usr/local/bin/sandbox-gateway
RUN chmod +x /usr/local/bin/sandbox-gateway
ENV SBGW_CONFIG=/etc/sandbox-gateway/config.yaml
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/sandbox-gateway"]
