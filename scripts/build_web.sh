#!/bin/sh
# Builds the Godot web client into deploy/web and pre-gzips the large
# assets (the server serves the .gz copies to browsers that accept gzip:
# the engine .wasm is ~35 MB raw, ~9 MB compressed).
#
# Requires: godot 4.4 on PATH and the Web export templates installed
# (~/.local/share/godot/export_templates/4.4.stable/web_nothreads_*.zip).
set -e
cd "$(dirname "$0")/.."
rm -rf deploy/web && mkdir -p deploy/web
godot --headless --path client --import >/dev/null 2>&1 || true
godot --headless --path client --export-release "Web" ../deploy/web/index.html
for f in deploy/web/*.wasm deploy/web/*.js deploy/web/*.pck deploy/web/*.html deploy/web/*.json; do
  [ -f "$f" ] && gzip -9 -k -f "$f"
done
ls -la deploy/web
