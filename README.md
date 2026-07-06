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
shows the keys available at any moment. Note: while the UI is being finalised,
caltui currently ships with sample events rather than your real calendars.

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

[monthview]
show_week_numbers = false

[weekview]
day_start = "00:00"
day_end = "24:00"

[dayview]
day_start = "00:00"
day_end = "24:00"
```

## Contributing

Contributions are welcome - open an issue or a pull request. You're welcome
to contribute with AI assistance too. Just read and test your code before
submitting.

## License

[MIT](LICENSE) © Raj Nakarja
