# Privileged resolver-sync helper

lanfirst adds a third process, `lanfirst-resolverd`: a root LaunchDaemon
(`/Library/LaunchDaemons/com.lanfirst.resolverd.plist`, `KeepAlive=true`) that watches the
user's `config.yaml` and reconciles `/etc/resolver/<domain>` to match the configured parent
domains. It is the only root-level component, and the only writer of `/etc/resolver`.

We did this because v1 leaked `sudo` into everyday use: `config.yaml` is hot-reloaded by the
daemon, but the `/etc/resolver` files are not, so adding a parent domain meant the user hand-ran
`sudo` to create the matching resolver file — two sources of truth that drift, and a privileged
step in a routine GUI action. The system "should end inside the software itself": install once,
then Add/Remove domains entirely from the menu bar with `/etc/resolver` kept in sync.

We rejected **passwordless sudoers** for the menu-bar app (broad, brittle, and grants the GUI a
general root escape hatch) and **leaving the files manual** (the status quo we are removing). A
root LaunchDaemon reconciling a declarative config is the standard macOS pattern and keeps the
privileged surface to one small, auditable program.

This extends [ADR 0002](./0002-split-daemon-vs-single-app.md), which deliberately kept everything
user-level. The flow is fully decoupled: the menu-bar app sends `CmdAddEntry`/`CmdRemoveEntry`
over the existing IPC socket; `lanfirstd` (the sole config writer) validates, atomically rewrites
`config.yaml`, and reloads its own routing; `lanfirst-resolverd`'s `fsnotify` watch then fires and
reconciles `/etc/resolver`. No process calls another with elevated privilege.

## Trust boundary

A root daemon consuming a **user-writable** config is a privilege-boundary surface, so the
privilege is bounded by construction:

- **Writes are confined to `/etc/resolver/`** — paths are built only as `filepath.Join("/etc/resolver", domain)`.
- **Filenames are validated as real hostnames** before use: dot-separated labels of `[a-z0-9-]`,
  each non-empty, no leading/trailing dot, and **no `..`** components — rejecting path traversal.
  (A naive `^[a-z0-9.-]+$` would *admit* `..`; per-label validation is the gate.)
- **Content is fixed**: `nameserver 127.0.0.1` + the numeric port parsed from `listen`.
- **Deletion is marker-gated**: every managed file begins with `# lanfirst-managed`, and the helper
  removes only files carrying that marker. Hand-created resolver files are never touched.
- **The binary is root-owned and not user-writable** (`/usr/local/sbin/lanfirst-resolverd`,
  `root:wheel 0755`): a root LaunchDaemon must never exec a path a normal user could overwrite.

## Consequences

- Install needs a single `sudo` (install the root-owned binary + plist, `launchctl bootstrap system`).
  After that, no recurring sudo. Uninstall reverses it (`-cleanup` removes marker-managed files,
  then `bootout` + remove plist/binary).
- Comment-preserving config writes are out of scope: `lanfirstd` serialises via `yaml.Marshal`, so
  managing the config by hand and by GUI both work, but hand-written comments are not retained — the
  annotated template stays in `config.example.yaml`.
- A code-signed `SMAppService` helper (vs a plain root LaunchDaemon) is deferred; it only matters for
  distribution beyond a personal machine.
