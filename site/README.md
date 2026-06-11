# site/

Static landing page for blackbox (GTM front door). Self-contained — one
HTML file plus the self-hosted JetBrains Mono fonts (OFL licensed, copied
from `web/static/fonts/`).

## Preview locally

```bash
python3 -m http.server -d site 8000
# http://localhost:8000
```

## Publish

Any static host works. For GitHub Pages: Settings → Pages → deploy from a
branch, folder `/site` (or add a `deploy-pages` workflow that uploads the
`site/` directory).
