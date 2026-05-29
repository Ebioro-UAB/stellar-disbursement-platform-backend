# Ebioro Deploy Branch

This branch (`ebioro/deploy`) carries Ebioro-only deployment artifacts on top of
`feat/external-tsa-signing`. **Do not PR this branch upstream.**

## What this branch adds

- `heroku.yml` — Heroku container deploy manifest (web, worker, release)
- `Dockerfile.heroku` — image variant without ENTRYPOINT so heroku.yml commands work
- `entrypoint.sh` — port swap for the admin-facing dyno
- `EBIORO_DEPLOY.md` — this file

## Heroku apps

| App | `web` process | `worker` process | Externally exposes |
|---|---|---|---|
| `ebioro-sdp` | `serve` (main API on `$PORT`) | `tss` | Main SDP API |
| `ebioro-sdp-admin` | `serve` (admin API on `$PORT`, via port swap) | not scaled | Admin multi-tenant API |

The admin-facing dyno sets `ADMIN_FACING=true`, which makes `entrypoint.sh`
swap the ports so `$PORT` (the only port Heroku routes externally) lands on
the admin server.

## Release flow

1. New code lands on `feat/external-tsa-signing` (or another feature branch)
2. Merge that branch into `ebioro/deploy`:
   ```
   git checkout ebioro/deploy
   git merge feat/external-tsa-signing
   git push origin ebioro/deploy
   ```
3. Deploy to Heroku from this branch (see deploy method below)

## Sync with upstream Stellar

Upstream changes flow `upstream/develop` → `feat/external-tsa-signing` → `ebioro/deploy`.

## Configuration

All env vars are managed in Doppler (`sdp-backend` project). Doppler is
synced to Heroku config vars via the Doppler-Heroku integration (already
configured on both apps).

Admin-only var on `ebioro-sdp-admin`:
- `ADMIN_FACING=true` — set this **directly on the Heroku app config vars**,
  NOT in Doppler. Both Heroku apps sync from the same Doppler config
  (`sdp-backend/dev`), so anything placed there propagates to both apps.

  Verify the Doppler-Heroku integration is in "additive" mode (does not
  delete keys absent from Doppler) so the manually-set `ADMIN_FACING` is
  preserved across syncs:
  ```bash
  heroku config:set ADMIN_FACING=true --app ebioro-sdp-admin
  ```

## Deploy

Heroku container deploy from the `ebioro/deploy` branch:

```bash
# Login to Heroku Container Registry (once)
heroku container:login

# Deploy ebioro-sdp (main API + tss worker)
heroku stack:set container --app ebioro-sdp
git push https://git.heroku.com/ebioro-sdp.git ebioro/deploy:main

# Deploy ebioro-sdp-admin (admin API, no worker)
heroku stack:set container --app ebioro-sdp-admin
git push https://git.heroku.com/ebioro-sdp-admin.git ebioro/deploy:main

# Scale dynos
heroku ps:scale web=1 worker=1 --app ebioro-sdp
heroku ps:scale web=1 worker=0 --app ebioro-sdp-admin
```

## Migrations

The `release` phase runs admin + TSS schema migrations on every deploy.
Per-tenant SDP/auth migrations run automatically when tenants are
provisioned via the admin API.
