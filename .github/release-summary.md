Grasshopper 2.11.3

Safer unattended upgrades.

- The nightly server upgrade only moves forward: it never undoes a rollback,
  never downgrades, and skips a release that was rolled back or failed its
  health check until a newer one ships. `upgrade --to` still installs any
  release.
- Upgrade, rollback and schedule take a lock, so two never overlap; the
  rollback link is set before the restart; pre-upgrade backups keep the
  newest five.
- An upgrade must answer a real search before it counts as healthy, and
  downloads refuse an HTTPS-to-HTTP redirect.
- Release runs never publish the same version twice at once, and archives are
  reproducible.
- `grasshopper check` warns when an agent runs a client newer than the server.
- The server restarts itself if one embedding call is stuck for 30 seconds.
- Search reads at most 200 full-text matches.
