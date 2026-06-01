# services

systemd units + watchdog config for the exo server (headless Debian 12, x86-64
AMD Phenom II X4). These are deployment **templates** — adjust paths and the
service `User` for your box.

## Units

- `exo-backend.service` — runs the Go backend binary; `Restart=on-failure`,
  starts on boot (`WantedBy=multi-user.target`). Ordered `After=docker.service`
  because Postgres runs as a Docker container (there is no native
  `postgresql.service`); `StartLimitIntervalSec=0` plus the backend's own
  startup DB-retry mean a slow cold-boot DB never parks the unit `failed`.
- `exo-reboot.timer` + `exo-reboot.service` — full-system `systemctl reboot`
  daily at **03:00 America/Toronto** (the timezone is pinned in `OnCalendar`, so
  it is correct regardless of the system clock's zone). `Persistent=false` so a
  missed window (box was off) does **not** fire a catch-up reboot on the next boot.

## Not managed here (must still auto-start on boot)

The nightly reboot is only safe if these come back on a clean cold boot too —
they ship their own units, so just enable them:

- **Postgres** — the Docker container must run with a restart policy
  (`restart: unless-stopped`) and Docker itself enabled on boot
  (`systemctl enable docker`).
- **`cloudflared`** — installs `cloudflared.service`; `systemctl enable cloudflared`.
- **Tailscale** — installs `tailscaled.service`; `systemctl enable tailscaled`.

The box does **not** serve the frontend (Netlify does), so there is no web
server to bring up.

## Watchdog (`watchdog/`)

Hardware watchdog so the box self-recovers from a true hang — the smarter
complement to the blind nightly reboot.

- `watchdog/sp5100_tco.conf` → `/etc/modules-load.d/` — loads the AMD SB7x0/8x0
  TCO watchdog driver (`sp5100_tco`) at boot.
- `watchdog/watchdog.conf` → `/etc/systemd/system.conf.d/` — has systemd feed the
  watchdog (`RuntimeWatchdogSec=20s`) and guard a stalled reboot
  (`RebootWatchdogSec=2min`).

## Install

```bash
sudo cp exo-backend.service exo-reboot.service exo-reboot.timer /etc/systemd/system/
sudo cp watchdog/sp5100_tco.conf  /etc/modules-load.d/
sudo cp watchdog/watchdog.conf    /etc/systemd/system.conf.d/
sudo systemctl daemon-reload
sudo systemctl enable --now exo-backend.service
sudo systemctl enable --now exo-reboot.timer
sudo modprobe sp5100_tco          # or reboot to apply the watchdog settings
```

## Verify (do this before trusting the 3am reboot)

- `systemctl status exo-backend` — running.
- `systemctl list-timers exo-reboot.timer` — next run shows 03:00.
- `wdctl` / `dmesg | grep -i sp5100` — watchdog present and fed by systemd.
- **Cold-boot test:** actually reboot and confirm the backend, the Postgres
  container, cloudflared, and Tailscale all come back and the API + tunnel are
  reachable (the dashboard itself is on Netlify). The nightly reboot is only safe
  if a cold boot is clean.

> If `/dev/watchdog` never appears or `sp5100_tco` errors out, your BIOS may not
> expose the TCO watchdog — fall back to the software watchdog (`softdog`), which
> still recovers from most hangs (just not a fully wedged kernel).
