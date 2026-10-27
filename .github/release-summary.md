Grasshopper 2.11.0

Updates now take care of themselves across machines.

- Releases publish when a VERSION change merges; no tag push needed.
- Clients follow the server: the bridge downloads the client matching the
  server's version, checks it against SHA256SUMS and switches to it. Opt out
  with an updates-off file beside client.json.
- grasshopper-server upgrade on macOS backs up the database, switches a
  current link to the new archive, restarts the launchd job and restores the
  old version if health fails. upgrade --rollback goes back; upgrade
  --schedule runs it nightly.

Plugins need one manual update to 2.11.0; after that, clients update with the
server.
