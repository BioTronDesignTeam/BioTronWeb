# services

systemd units + watchdog config for the BioTron VM. These are deployment
**templates** — adjust paths and the service `User` for the box.

## Units

- `exo-reboot.timer` + `exo-reboot.service` — full-system `systemctl reboot`
  daily at **03:00 America/Toronto** (the timezone is pinned in `OnCalendar`, so
  it is correct regardless of the system clock's zone). `Persistent=false` so a
  missed window (box was off) does **not** fire a catch-up reboot on the next boot.
- `exo-backend.service` is retained as a legacy native-backend template. New
  staging and production hosts run exo-gui through Docker and should not install
  this unit.

## Not managed here (must still auto-start on boot)

The nightly reboot is only safe if these come back on a clean cold boot too —
they ship their own units, so just enable them:

- **BioTron containers** — Nginx, cloudflared, Postgres, Redis, and application
  containers use `restart: unless-stopped`; enable Docker on boot with
  `systemctl enable docker`.
- **Tailscale** — installs `tailscaled.service`; `systemctl enable tailscaled`.

Nginx and cloudflared are containers owned by `Server/docker-compose.yml`, not
native host services. The frontends are served by their own containers through
Nginx; Netlify is not part of the intended staging/production topology.

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
sudo cp exo-reboot.service exo-reboot.timer /etc/systemd/system/
sudo cp watchdog/sp5100_tco.conf  /etc/modules-load.d/
sudo cp watchdog/watchdog.conf    /etc/systemd/system.conf.d/
sudo systemctl daemon-reload
sudo systemctl enable --now exo-reboot.timer
sudo modprobe sp5100_tco          # or reboot to apply the watchdog settings
```

## Verify (do this before trusting the 3am reboot)

- `docker compose ps` in the deployed `Server` checkout — Nginx, cloudflared,
  Postgres, and Redis are running/healthy.
- `systemctl list-timers exo-reboot.timer` — next run shows 03:00.
- `wdctl` / `dmesg | grep -i sp5100` — watchdog present and fed by systemd.
- **Cold-boot test:** actually reboot and confirm Docker, every container,
  Tailscale, all public hostnames, and an authenticated OAuthManager flow come
  back. The nightly reboot is only safe if a cold boot is clean.

> If `/dev/watchdog` never appears or `sp5100_tco` errors out, your BIOS may not
> expose the TCO watchdog — fall back to the software watchdog (`softdog`), which
> still recovers from most hangs (just not a fully wedged kernel).
