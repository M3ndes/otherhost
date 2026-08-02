# Third-party notices

Otherhost embeds or imports the following open-source components for its local
dashboard terminal:

| Component | Version | License | Use |
| --- | --- | --- | --- |
| [Xterm.js](https://github.com/xtermjs/xterm.js) | 6.0.0 | MIT | Browser terminal emulator |
| [Xterm.js fit addon](https://github.com/xtermjs/xterm.js) | 0.11.0 | MIT | Responsive terminal sizing |
| [Gorilla WebSocket](https://github.com/gorilla/websocket) | 1.5.3 | BSD-2-Clause | Local terminal transport |
| [creack/pty](https://github.com/creack/pty) | 1.1.24 | MIT | macOS pseudo-terminal management |

The complete Xterm.js and fit-addon license texts are stored beside their
vendored browser assets under `internal/dashboard/assets/vendor/`. Go dependency
license and source information remains available from the linked upstream
modules and `go.sum` records the selected module contents.
