# CloudSync

[![Go Report Card](https://goreportcard.com/badge/github.com/danielbehrens/cloudsync)](https://goreportcard.com/report/github.com/danielbehrens/cloudsync)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

CloudSync is a robust, bi-directional file synchronization tool designed specifically for game save files. It enables seamless save file sharing across multiple machines for games that only support host-based multiplayer, allowing any player to host the game session even when the primary host is offline.

**Current configuration targets:** Runescape: Dragonwilds

---

## Features

- **Bi-directional sync**: Automatically syncs files between local storage and cloud (S3/MinIO)
- **Smart conflict resolution**: Uses modification timestamps with configurable tolerance
- **Process-aware**: Pauses synchronization while the game is running to prevent corruption
- **Timestamped backups**: Creates backups before overwriting files
- **Initial sync**: Ensures all saves are current on startup
- **Selective sync**: Only syncs `.sav` files, excluding user-specific settings
- **Production-ready**: Structured logging, error handling, graceful shutdown
- **Cross-platform**: Runs on Windows and Linux

---

## Quick Start

### Prerequisites

- Go 1.21 or later
- S3-compatible storage (MinIO, AWS S3, etc.)

### Installation

```bash
# Clone the repository
git clone https://github.com/danielbehrens/cloudsync.git
cd cloudsync

# Build
make build

# Or build for specific platform
make build-windows
make build-linux
```

### Configuration

CloudSync is configured via command-line flags:

| Flag              | Description                                          | Default                       | Required |
|-------------------|------------------------------------------------------|-------------------------------|----------|
| `-watch-path`     | Path to monitor for save file changes                 | Auto-generated*               | No       |
| `-process-name`   | Game process name (pauses sync when running)         | `RSDragonwilds-Win64-Shipping.exe` | No       |
| `-backup-dir`     | Directory for timestamped backups                     | `{watch-path}/Backup`         | No       |
| `-cloud-endpoint` | S3/MinIO endpoint URL                                 | `localhost:9000`              | Yes      |
| `-access-key`     | S3 access key                                         | -                             | Yes      |
| `-secret-key`     | S3 secret key                                         | -                             | Yes      |
| `-bucket-name`    | S3 bucket name                                        | `gamesync-dragonwilds`        | Yes      |

\* Auto-generated path: `%LOCALAPPDATA%\RSDragonwilds\Saved\SaveGames` (Windows)

### Usage

**Basic usage with default paths:**

```bash
cloudsync \
  -cloud-endpoint "localhost:9000" \
  -access-key "minioadmin" \
  -secret-key "minioadmin"
```

**Custom configuration:**

```bash
cloudsync \
  -watch-path "C:\Games\Dragonwilds\Saves" \
  -backup-dir "C:\Backups\Dragonwilds" \
  -process-name "RSDragonwilds-Win64-Shipping.exe" \
  -cloud-endpoint "s3.amazonaws.com" \
  -access-key "YOUR_ACCESS_KEY" \
  -secret-key "YOUR_SECRET_KEY" \
  -bucket-name "my-game-saves"
```

---

## Running as a Service

### Windows (using NSSM)

1. Download [NSSM](https://nssm.cc/download)
2. Install the service:

```powershell
nssm install CloudSync "C:\path\to\cloudsync.exe"
```

3. In the NSSM GUI, set:
   - **Path**: Location of `cloudsync.exe`
   - **Startup directory**: Directory containing the executable
   - **Arguments**: Your command-line flags

4. Start the service:

```powershell
net start CloudSync
```

### Linux (systemd)

Create `/etc/systemd/system/cloudsync.service`:

```ini
[Unit]
Description=CloudSync - Game Save Synchronization
After=network.target

[Service]
Type=simple
User=youruser
WorkingDirectory=/opt/cloudsync
ExecStart=/opt/cloudsync/cloudsync \
  -watch-path "/home/youruser/.local/share/game/saves" \
  -cloud-endpoint "s3.amazonaws.com" \
  -access-key "YOUR_ACCESS_KEY" \
  -secret-key "YOUR_SECRET_KEY" \
  -bucket-name "game-saves"
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable cloudsync
sudo systemctl start cloudsync
sudo systemctl status cloudsync
```

---

## Development

### Building

```bash
# Build
make build

# Run tests
make test

# Run linter
make lint

# Format code
make fmt

# Clean build artifacts
make clean
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run with race detector
go test -race ./...
```

---

## How It Works

1. **Initial Sync**: On startup, CloudSync performs a full bidirectional sync:
   - Uploads local files that are newer than cloud versions
   - Downloads cloud files that are newer than local versions

2. **File Monitoring**: Uses `fsnotify` to watch for file system changes in real-time

3. **Smart Sync**: When a change is detected:
   - Checks if the game process is running (if so, pauses sync)
   - Compares modification times (with 500ms tolerance)
   - Uploads/downloads the newer version
   - Creates timestamped backups before overwriting

4. **Periodic Sync**: Every 10 seconds, performs a full sync if the game isn't running

5. **Graceful Shutdown**: Handles SIGTERM/SIGINT for clean service stops

---

## Configuration Details

### Time Tolerance

CloudSync uses a 500ms time tolerance when comparing file timestamps. This prevents unnecessary syncs due to minor time differences between systems.

### File Filtering

- Only `.sav` files are synchronized
- `EnhancedInputUserSettings.sav` is excluded (user-specific settings)
- Only files in the root watch directory are synced (subdirectories ignored)

### Event Cooldown

A 1-second cooldown prevents duplicate events from triggering multiple syncs for the same file.

---

## MinIO Setup (for local testing)

```bash
# Run MinIO in Docker
docker run -p 9000:9000 -p 9001:9001 \
  --name minio \
  -e "MINIO_ROOT_USER=minioadmin" \
  -e "MINIO_ROOT_PASSWORD=minioadmin" \
  minio/minio server /data --console-address ":9001"

# Access MinIO Console at http://localhost:9001
```

---

## Troubleshooting

### CloudSync doesn't detect changes

- Verify the watch path is correct
- Ensure you have read permissions on the directory
- Check logs for watcher errors

### Files aren't syncing

- Verify S3 credentials are correct
- Check network connectivity to S3 endpoint
- Ensure the bucket exists or CloudSync has permission to create it
- Check if the game process name matches

### Sync conflicts

CloudSync uses "newest wins" strategy. If two machines edit simultaneously:
- The last file to be synced wins
- Previous versions are saved in timestamped backup folders

---

### Guidelines

- Add tests for new features
- Run `make lint` and `make test` before submitting
- Follow existing code style
- Update documentation as needed

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## Acknowledgments

- Built with [fsnotify](https://github.com/fsnotify/fsnotify) for file system watching
- Uses [MinIO Go SDK](https://github.com/minio/minio-go) for S3 operations
- Process detection via [gopsutil](https://github.com/shirou/gopsutil)

---

