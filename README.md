# FlexRadio Broadcast Wedge

Makes FlexRadios on one subnet appear in the SmartSDR radio chooser on another
subnet, so SmartSDR can connect straight to the radio over a VPN or routed link
without SmartLink.

A FlexRadio announces itself with a UDP broadcast on port 4992 once a second.
Broadcasts do not cross routers or VPN tunnels, so a remote SmartSDR never sees
the radio even though it could reach the radio's IP address. The wedge carries
those announcements across.

> **SmartLink is the best way to operate a FlexRadio remotely.** This program
> is only for people who already run their own VPN.
>
> **There is no warranty. You are on your own.** This program is provided "as
> is", without warranty of any kind. It is not a FlexRadio product and is not
> supported in any way by FlexRadio Inc.

**New to networking? Follow the step-by-step [Setup Guide](SETUP.md).**

![Broadcast Wedge running as a USC, relaying two radios from a remote RSC](screenshot-usc.png)

## How it works

One program, `BroadcastWedge.exe`, runs at each end in a different role:

```
 radio subnet                         VPN                        user subnet
+-------+  UDP 4992   +-----+   TCP 4996 (USC dials RSC)   +-----+  UDP 4992   +----------+
| radio | ----------> | RSC | <--------------------------- | USC | ----------> | SmartSDR |
+-------+  broadcast  +-----+ ---- discovery packets ----> +-----+  broadcast  +----------+
    ^                                                                               |
    +------------------- SmartSDR connects directly to the radio's IP --------------+
```

- **RSC (Radio Subnet Client)** runs on a PC on the same subnet as the radios.
  It hears every radio's discovery broadcast and streams the packets, byte for
  byte, to each connected USC.
- **USC (User Subnet Client)** runs on a PC on the same subnet as SmartSDR (or
  on the SmartSDR PC itself). It dials one or more RSCs and rebroadcasts what
  they send on its own subnet.
- **Both** does the two jobs in one process, for a PC that has radios of its
  own and also wants to see radios from another site.

Because the packets are the radio's own, SmartSDR shows live status, version,
"in use" and station information, and a radio that is switched off disappears
from the chooser within a few seconds. There is nothing to configure per radio.

Any number of USCs can use one RSC, and one USC can use several RSCs.

## Setup

1. On the radio subnet, run the program, open **Settings**, choose **the radio
   subnet (RSC)**. The radios it hears are listed. Untick **Export** for any
   radio remote users should not see.
2. On the user subnet, run the program, choose **the user subnet (USC)** and
   enter the RSC PC's IP address (as reachable over the VPN).
3. Start SmartSDR. The remote radios are in the chooser.

Settings and logs are stored in `%AppData%\BroadcastWedge`.

## Network requirements

- The USC must be able to open **TCP 4996** to the RSC PC (the port can be
  changed). Allow it in Windows Firewall on the RSC PC.
- The SmartSDR PC must be able to reach the radio's IP address, and the radio
  must be able to send UDP back to the SmartSDR PC. That is the job of the VPN
  or router, not of this program: the wedge only makes the radio visible.
- The RSC shares UDP 4992 with SmartSDR, so both can run on the same PC.

## Raspberry Pi and Debian

A headless build runs the same relay as a background service on 64-bit
Raspberry Pi OS (or any arm64 Debian), so a Pi can take the place of the PC at
either end. It talks to the Windows program in either direction.

Check the Pi is 64-bit (`uname -m` prints `aarch64`), then download
`broadcastwedge_2.1.0_arm64.deb` from the
[releases page](https://github.com/va3mw/FlexRadio-Broadcast-Wedge/releases)
and install it:

```
sudo apt install ./broadcastwedge_2.1.0_arm64.deb
```

The service starts immediately and at every boot. Open
`http://<address-of-the-pi>:4997` in a browser on the same network; it is the
same screen as the Windows program, and the role, RSC addresses and Export
ticks are set there.

| Task | Command |
| --- | --- |
| Is it running? | `systemctl status broadcastwedge` |
| Watch the log | `journalctl -u broadcastwedge -f` |
| Restart | `sudo systemctl restart broadcastwedge` |
| Remove | `sudo apt remove broadcastwedge` (`purge` also deletes settings) |

Settings are in `/var/lib/broadcastwedge`, log files in
`/var/log/broadcastwedge`.

**The web page has no login.** Anyone on the same network who can reach port
4997 can change the settings. To turn the page off, add ` -http ""` to the
`ExecStart` line with `sudo systemctl edit --full broadcastwedge`.

## Tailscale

This has been tested over a [Tailscale](https://tailscale.com) network. On the
USC, enter the **Tailscale IP address of the RSC PC** as the RSC address.

**Tailscale and SmartSDR must be running on the same computer.** A device
that is not itself on the Tailscale network cannot complete a connection to
the radio.

**A Maestro will not work this way.** A Maestro on the same network as the USC
may show the radio in its list, but it cannot complete the connection, because
Tailscale cannot be installed on a Maestro.

This has not been fully tested, and it may not require a full VPN. Reports of
what does and does not work are welcome.

## Notes

- A USC rebroadcasts from every active network interface unless one is picked
  in Settings.
- An RSC relays a discovery packet only when it comes from the radio it
  describes. A packet announced by some other host is another wedge's
  rebroadcast; ignoring those stops packets looping between two sites that
  each run an RSC and a USC.
- The link has no authentication. Anyone who can reach the RSC's TCP port
  receives the same information the radios already broadcast on their own LAN.

## Building

Requires Go and [Wails v2](https://wails.io).

```
go test ./...
wails build
```

The result is `build\bin\BroadcastWedge.exe`.

The Debian package cross-compiles from any machine with Go, Windows included
(run it from Git Bash there):

```
sh packaging/build-deb.sh arm64
```

The result is `build/bin/broadcastwedge_<version>_arm64.deb`. `armhf` and
`amd64` are also accepted.

## History

Version 1 was a Python script that broadcast a hand-written discovery packet
for a single radio; it is kept in [`legacy/`](legacy/).

Written by VA3MW.
