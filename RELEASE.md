# Release Process

## What ships in a release

A release tarball (`aiops-platform-vX.Y.Z.tar.gz`) contains **only these items**:

| Path | Description |
|------|-------------|
| `aiops-server` | Compiled Go backend binary (linux/amd64, no CGO) |
| `frontend/` | Pre-built React static assets |
| `migrations/` | SQL migration files (no secrets) |
| `docker-compose.yml` | Production compose file |
| `Dockerfile` | Image definition |
| `.env.example` | Config template with placeholder values |
| `DEPLOYMENT.md` | Operator deployment guide |
| `RELEASE_MANIFEST.json` | Version, commit, build timestamp |

## What NEVER ships

| Category | Examples |
|----------|---------|
| Secrets | `.env`, `*.key`, `*.pem`, `credentials.json` |
| Source control | `.git/`, `.gitignore` |
| Dependencies | `node_modules/`, Go module cache |
| Build intermediates | `frontend/dist` (raw), `coverage.out` |
| Logs | `*.log`, `logs/` |
| Runtime state | `.agent.id`, `cache-position.json` |
| Dev artefacts | `*.bak`, `.backup_*/`, `~` files |
| IDE files | `.idea/`, `.vscode/` |

The `scripts/validate-package.sh` script enforces all rules above and is
called automatically by `make package` and the CI release workflow.

## How to cut a release

```bash
# 1. Bump the version
echo "1.2.0" > VERSION

# 2. Build and package (automatically validates)
make package

# 3. Inspect the manifest
cat dist/aiops-platform-1.2.0/RELEASE_MANIFEST.json

# 4. Tag and push (triggers CI release workflow)
git tag v1.2.0
git push origin v1.2.0
```

## Release manifest format

The `RELEASE_MANIFEST.json` written into every package:

```json
{
  "product": "ai-incident-platform",
  "version": "1.2.0",
  "git_commit": "abc1234",
  "build_time": "2026-04-21T12:00:00Z",
  "components": {
    "backend": "aiops-server",
    "frontend": "frontend/",
    "migrations": "migrations/"
  }
}
```

## Developer hygiene rules

1. **Never commit `.env`** — copy `.env.example` and fill locally.
2. **Never commit binaries** — `make build` writes to `dist/` which is gitignored.
3. **Never commit `*.bak` files** — use git for version history instead.
4. **Run `make clean` before packaging** — ensures a clean build from source.
5. **Run `make validate-release`** — run manually anytime to verify a tarball.
