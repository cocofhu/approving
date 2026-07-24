#!/bin/bash
set -eo pipefail
CONFIG_ROOT="${CONFIG_ROOT:-/root/.cursor}"
mkdir -p "$CONFIG_ROOT"

# startup.sh inject helpers assume unset vars are empty (no `set -u`).
export SANDBOX_INJECT_HEADERS="${SANDBOX_INJECT_HEADERS:-}"

# Apply SANDBOX_INJECT (same contract as universal-sandbox startup.sh).
if [ -n "${SANDBOX_INJECT:-}" ]; then
  # shellcheck disable=SC1091
  . /inject.sh
fi

# Session (8765) + IDE (8744) fake HTTP servers.
python3 - <<'PY' &
from http.server import BaseHTTPRequestHandler, HTTPServer
import threading

class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.end_headers()
        self.wfile.write(b"ok")
    def log_message(self, *args):
        pass

def serve(port):
    HTTPServer(("0.0.0.0", port), H).serve_forever()

threading.Thread(target=serve, args=(8765,), daemon=True).start()
threading.Thread(target=serve, args=(8744,), daemon=True).start()
import time
while True:
    time.sleep(3600)
PY
wait
