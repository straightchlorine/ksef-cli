#!/usr/bin/env bash
# Local preview of the bilingual docs.
#
# uv run --isolated --no-project --python 3.12 --with-requirements requirements-docs.txt scripts/preview.sh
# (PORT=8055 by default)
set -euo pipefail
PORT=${PORT:-8055}
cd "$(git rev-parse --show-toplevel)"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

python -m mkdocs build --strict -f mkdocs.yml -d "$TMP/www"
python -m mkdocs build --strict -f mkdocs.en.yml -d "$TMP/www/en"

echo "PL: http://127.0.0.1:$PORT/"
echo "EN: http://127.0.0.1:$PORT/en/"
python -m http.server "$PORT" --bind 127.0.0.1 -d "$TMP/www"
