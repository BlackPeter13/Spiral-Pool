#!/bin/bash

# SPDX-License-Identifier: BSD-3-Clause
# SPDX-FileCopyrightText: Copyright (c) 2026 Spiral Pool Contributors

# Enable HTTPS on the Spiral Pool Dashboard
# Called via sudo from the dashboard web UI (Management tab)
# Patches the spiraldash.service to use --certfile/--keyfile, then restarts
#
# Usage: sudo /spiralpool/scripts/enable-https.sh
#
# The script has two phases:
#   1. PATCH (synchronous): validate cert, patch service file, verify
#   2. RESTART (delayed background): daemon-reload + restart after 3 seconds
#
# The 3-second delay allows the calling HTTP response to reach the browser
# before systemctl restart kills the gunicorn process.
#
# Prerequisites:
#   - TLS certificate must already exist at $INSTALL_DIR/certs/dashboard.crt
#   - This script must be in the sudoers NOPASSWD allowlist for $POOL_USER

set -euo pipefail

# ═══════════════════════════════════════════════════════════
# PHASE 1: VALIDATE & PATCH (synchronous — exit code matters)
# ═══════════════════════════════════════════════════════════

SERVICE_FILE="/etc/systemd/system/spiraldash.service"

if [[ ! -f "$SERVICE_FILE" ]]; then
    echo "ERROR: Service file not found: $SERVICE_FILE" >&2
    exit 1
fi

# Detect install dir from the service's WorkingDirectory
# e.g. /spiralpool/dashboard → /spiralpool
INSTALL_DIR="$(grep -oP 'WorkingDirectory=\K\S+' "$SERVICE_FILE" 2>/dev/null | head -1 | sed 's|/dashboard$||')"
if [[ -z "$INSTALL_DIR" ]]; then
    INSTALL_DIR="/spiralpool"
fi

CERT_FILE="$INSTALL_DIR/certs/dashboard.crt"
KEY_FILE="$INSTALL_DIR/certs/dashboard.key"

# Validate certificate and key exist
if [[ ! -f "$CERT_FILE" ]]; then
    echo "ERROR: TLS certificate not found at $CERT_FILE. Run upgrade.sh first." >&2
    exit 1
fi
if [[ ! -f "$KEY_FILE" ]]; then
    echo "ERROR: TLS key not found at $KEY_FILE. Run upgrade.sh first." >&2
    exit 1
fi

# Validate cert is actually parseable (not corrupt/empty)
if ! openssl x509 -noout -in "$CERT_FILE" 2>/dev/null; then
    echo "ERROR: TLS certificate at $CERT_FILE is invalid or corrupt. Delete it and run upgrade.sh to regenerate." >&2
    exit 1
fi

# Idempotent: if already HTTPS, nothing to do
if grep -q "^ExecStart.*\-\-certfile" "$SERVICE_FILE" 2>/dev/null; then
    echo "OK: HTTPS is already enabled"
    exit 0
fi

# Patch ExecStart: insert --certfile and --keyfile after --bind <addr:port>
# Handle both --bind 0.0.0.0:PORT (space) and --bind=0.0.0.0:PORT (equals) syntax
sed -i "s|--bind[= ]\([^ ]*\)|--bind \1 --certfile=${CERT_FILE} --keyfile=${KEY_FILE}|" "$SERVICE_FILE"

# Add /certs to ReadWritePaths if not already there
if ! grep -q "${INSTALL_DIR}/certs" "$SERVICE_FILE" 2>/dev/null; then
    sed -i "s|ReadWritePaths=\(.*\)|ReadWritePaths=\1 ${INSTALL_DIR}/certs|" "$SERVICE_FILE"
fi

# Verify the patch actually worked
if ! grep -q "^ExecStart.*\-\-certfile" "$SERVICE_FILE" 2>/dev/null; then
    echo "ERROR: Failed to patch service file — ExecStart line may have unexpected format" >&2
    exit 1
fi

# ═══════════════════════════════════════════════════════════
# PHASE 2: RESTART (delayed background)
# ═══════════════════════════════════════════════════════════
# 3-second delay so the calling dashboard process can send the
# HTTP response back to the browser before it gets killed.

# Verify systemd is running before attempting restart — WSL2 can have
# systemd disabled, causing silent failure (patch succeeds but restart doesn't).
if ! pidof systemd &>/dev/null; then
    echo "WARNING: systemd is not running — service file patched but restart skipped." >&2
    echo "Manually restart: sudo systemctl daemon-reload && sudo systemctl restart spiraldash" >&2
else
    (
        sleep 3
        systemctl daemon-reload
        systemctl restart spiraldash
    ) &>/dev/null &
    disown
fi

DASH_PORT="$(grep -oP '0\.0\.0\.0:\K[0-9]+' "$SERVICE_FILE" | head -1)"
echo "OK: HTTPS enabled. Dashboard will restart in 3 seconds. Access via https://$(hostname):${DASH_PORT:-1618}"
