# Minimal sandbox stub for CI docker-driver e2e (not the full universal-sandbox).
# Speaks HTTP on 8765 (session) and 8744 (ide), applies SANDBOX_INJECT like startup.sh.
# Default base uses a CN Docker Hub mirror; override with --build-arg ALPINE_IMAGE=alpine:3.20 overseas.
ARG ALPINE_IMAGE=docker.m.daocloud.io/library/alpine:3.20
FROM ${ALPINE_IMAGE}
RUN apk add --no-cache bash curl tar python3
COPY inject.sh /inject.sh
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /inject.sh /entrypoint.sh
ENV CONFIG_ROOT=/root/.cursor
EXPOSE 8765 8744 22
ENTRYPOINT ["/entrypoint.sh"]
