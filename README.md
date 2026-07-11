# caltui

![caltui demo](docs/demo.gif)

A fast, keyboard-driven calender for the terminal - designed to save busy people
time.

- Supports all major calender providers (Google, iCal, etc)
- Multiple views (month, week, day)
- Quick navigation, scheduling and event management
- Support for common event fields (attendees, location, call links)
- Highly configurable via a single dotfile
- CLI for easily retreiving event information
- Secure. Event and account data is stored in a way that it can be locked down

Built in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and
[Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Installation

### Nix

_Coming soon._

### Arch Linux (AUR)

_Looking for maintainers._

### From source

1. Clone the repository:

    ```sh
    git clone https://github.com/siliconwitch/caltui.git
    cd caltui
    ```

1. Build the static binary:

    ```sh
    CGO_ENABLED=0 go build -o caltui .
    ```

1. Install it onto your `PATH`:

    ```sh
    sudo install -Dm755 caltui /usr/local/bin/caltui
    ```

## Dependencies

| Feature           | Requires                                                  |
| ----------------- | --------------------------------------------------------- |
| Build from source | [Go](https://go.dev) 1.26+                                |

## Usage

Run `caltui` in a terminal. It opens on a month view, with week and day views
a keypress away (`m`/`w`/`d`), and vim-style keys for moving around and
managing events. A context-sensitive hint bar at the bottom of the screen
shows the keys available at any moment. Until you configure an account (see
below), caltui shows sample events so you can explore the UI.

### Keybindings

| Key                 | Action                        |
| ------------------- | ----------------------------- |
| `m` `w` `d`         | Month / week / day view       |
| `hjkl` / arrows     | Move around                   |
| `tab` / `shift-tab` | Select next / previous event  |
| `n`                 | New event                     |
| `e`                 | Edit selected event           |
| `d`                 | Delete selected event         |
| `y`                 | Yank (copy) selected event    |
| `p`                 | Paste event                   |
| `t`                 | Jump to today                 |
| `g`                 | Go to date                    |
| `r`                 | Refresh accounts              |
| `esc`               | Deselect / close popup        |
| `q`                 | Quit                          |

## Config

caltui reads `~/.config/caltui/config.toml` (honouring `$XDG_CONFIG_HOME`,
or an explicit `$CALTUI_CONFIG` path). Every key is optional and falls back
to the default below.

```toml
[calendar]
# IANA name, e.g. "Europe/Stockholm". Defaults to the system timezone.
timezone = ""

# Hex event colors by calendar name (or "account/name" when two accounts
# share a name). Unlisted calendars keep their automatic palette color.
# [calendar.colors]
# "Personal" = "#9ECE6A"

[monthview]
show_week_numbers = false

[weekview]
day_start = "00:00"
day_end = "24:00"

[dayview]
day_start = "00:00"
day_end = "24:00"
```

## Accounts

Accounts connect caltui to real calendars and are declared in the same config
file. Two account types are supported:

- **`caldav`** — read/write. Works with every provider that speaks CalDAV
  with an app password: iCloud, Fastmail, Nextcloud, Radicale, Zoho and
  others. Generate an app password in your provider's security settings;
  `url` is the provider's CalDAV root and calendars are discovered
  automatically. Discovery tries the URL itself, then the server's
  `/.well-known/caldav` (needed by e.g. Zoho), then treats the URL as a
  single calendar — so if in doubt, the exact CalDAV address from your
  provider's settings always works. iCloud is the exception: discovery
  cannot hop from `caldav.icloud.com` to your personal partition server,
  so set `url` to your calendar home directly, e.g.
  `https://pXX-caldav.icloud.com/<userid>/calendars/` (two `PROPFIND`
  requests against `caldav.icloud.com` — `current-user-principal`, then
  `calendar-home-set` — reveal it).
- **`ics`** — read-only subscription to an iCalendar URL. This is how you
  connect Google Calendar without OAuth: copy the calendar's *"Secret address
  in iCal format"* from its settings. Any published `.ics` link works too.

```toml
[[accounts]]
name = "fastmail"
type = "caldav"
url = "https://caldav.fastmail.com"
username = "raj@fastmail.com"
credential_command = "pass show caltui/fastmail"

[[accounts]]
name = "google"
type = "ics"
credential_command = "pass show caltui/google-ics-url"
```

Events sync on startup and on `r`, and are cached locally so views open
instantly and work offline. Events within one year either side of today are
synced. Sync and save problems open an error popup — `y` yanks the error text to
the clipboard (via OSC 52, so it works over SSH too), any other key
dismisses it, and further queued errors follow one at a time. Editing and deleting
recurring events is not supported yet; those events, and everything in an
`ics` subscription, are read-only.

## Security

caltui is designed so that account data is easy to lock down with mandatory
access control, and so that your config file stays publishable in a dotfiles
repository. It touches exactly three fixed paths:

| Path                              | Contents                       | Access     |
| --------------------------------- | ------------------------------ | ---------- |
| `~/.config/caltui/config.toml`    | Config. Never secrets.         | Read       |
| `~/.local/state/caltui/`          | `credentials.toml` secrets     | Read       |
| `~/.cache/caltui/`                | Event cache. Safe to delete.   | Read/write |

Secrets — app passwords and secret ics URLs — reach caltui one of two ways,
per account:

- **`credential_command`** — a shell command whose output is the secret, e.g.
  `pass show caltui/fastmail` or
  `secret-tool lookup service caltui account fastmail`. The config stays free
  of secrets because it only names the command.
- **`~/.local/state/caltui/credentials.toml`** — a plain file for setups
  where spawning helpers is undesirable (this keeps an AppArmor profile
  exec-free). caltui refuses to read it unless it is `chmod 600`:

  ```toml
  [fastmail]
  secret = "app-password-here"

  [google]
  secret = "https://calendar.google.com/calendar/ical/…/basic.ics"
  ```

If you'd rather not publish your email address or server either, leave
`username` and `url` out of the config too — caltui falls back to `username`
and `url` keys in the account's `credentials.toml` section:

```toml
[fastmail]
username = "raj@fastmail.com"
url = "https://caldav.fastmail.com"
secret = "app-password-here"
```

An AppArmor profile covering exactly this footprint ships in
[`contrib/apparmor/caltui`](contrib/apparmor/caltui):

```sh
sudo install -Dm644 contrib/apparmor/caltui /etc/apparmor.d/caltui
sudo apparmor_parser -r /etc/apparmor.d/caltui
```

The defaults honour `$XDG_CONFIG_HOME`, `$XDG_STATE_HOME` and the
`CALTUI_CONFIG`, `CALTUI_CREDENTIALS` and `CALTUI_CACHE` overrides, but the
shipped profile confines the default paths. The binary is static
(`CGO_ENABLED=0`), timezone data is embedded, and sync errors never echo
secret URLs.

## Contributing

Contributions are welcome - open an issue or a pull request. You're welcome
to contribute with AI assistance too. Just read and test your code before
submitting.

## License

[MIT](LICENSE) © Raj Nakarja
