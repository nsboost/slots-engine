#!/bin/sh
# Build the Godot web client and pre-gzip large assets.
# Safe to run locally or in GitHub Actions.
set -e
cd "$(dirname "$0")/.."

mkdir -p deploy/web client/pwa

echo "==> Importing Godot project..."
godot --headless --path client --import 2>&1 | grep -v "^$" | grep -v "progress_dialog" || true

echo "==> Exporting to Web..."
godot --headless --path client --export-release "Web" ../deploy/web/index.html

echo "==> Pre-compressing assets..."
for f in deploy/web/*.wasm deploy/web/*.js deploy/web/*.pck deploy/web/*.html deploy/web/*.json; do
  [ -f "$f" ] || continue
  gzip -9 -k -f "$f"
  echo "  gzipped: $(basename $f)"
done

echo "==> Done."
ls -lh deploy/web/*.wasm 2>/dev/null || true
