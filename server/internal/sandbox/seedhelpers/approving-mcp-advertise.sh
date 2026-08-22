#!/bin/sh
# Placeholder for historical SPA→API host rewrite. Public builds do not rewrite
# APPROVING_ARTIFACT_URL; set mcp_advertise to a URL that serves /mcp.
# Best-effort: refresh artifact-upload from workspace when control plane lags.
if [ -f /usr/local/bin/artifact-upload ] && ! grep -q upload_image_artifact /usr/local/bin/artifact-upload 2>/dev/null; then
  if [ -f /usr/local/share/approving/artifact-upload ] && grep -q upload_image_artifact /usr/local/share/approving/artifact-upload 2>/dev/null; then
    install -m 755 /usr/local/share/approving/artifact-upload /usr/local/bin/artifact-upload
  else
    for inst in /root/workspace/*/server/scripts/install-artifact-upload.sh; do
    if [ -x "$inst" ]; then "$inst" && break; fi
    done
  fi
fi
