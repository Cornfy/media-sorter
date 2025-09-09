# 🖼️ Go Media Sorter

[简体中文](./README_zh.md) | **English**

---

An intelligent media file organizer written in Go. This tool renames your photos and videos to a clean, consistent `PREFIX_YYYYMMDD_HHMMSS.ext` format based on their metadata. It's safe, fast, configurable, and cross-platform.

This project is a high-performance port and enhancement of an exceptionally well-written shell script.

### ✨ Features

- **Intelligent Timestamping**: Prioritizes authoritative metadata (EXIF/QuickTime) for renaming. If metadata is missing, it safely falls back to the file's last modification time (`mtime`).
- **Metadata Enrichment**: Intelligently fills in empty date/time tags within your media files (e.g., `DateTimeOriginal`, `CreateDate`) using the authoritative timestamp. It **never** overwrites existing valid data.
- **System Timestamp Sync**: Synchronizes the file's system modification time to match the authoritative timestamp, ensuring consistency across your filesystem.
- **Safety First**:
  - **Automatic Backups**: Creates a full `.tar.gz` backup of your target directory before making any changes.
  - **Confirmation Prompt**: Requires explicit user confirmation before starting, preventing accidental runs.
  - **Conflict Resolution**: If two files have the exact same timestamp, it adds a random suffix `_[xxx]` to avoid overwriting.
- **Dependency Awareness**: Automatically detects if the `ExifTool` dependency is missing and enters a safe, limited-functionality mode with clear warnings.
- **Highly Configurable**: All key parameters (file prefixes, supported extensions, timezone) are managed in a simple `config.json` file. No need to edit the code.
- **Cross-Platform**: Built with Go, it can be compiled to a single native executable for Linux, Windows, and macOS.

### 🔧 Prerequisites

This tool relies on the excellent **ExifTool** to read and write media metadata. You must have it installed on your system.

- **On Arch Linux:** `sudo pacman -S perl-image-exiftool`
- **On Debian/Ubuntu:** `sudo apt-get install libimage-exiftool-perl`
- **On macOS (with Homebrew):** `brew install exiftool`
- **On Windows:** Download from the [ExifTool website](https://exiftool.org/).

### 🚀 Getting Started

1.  **Install `ExifTool`** (see above).
2.  Go to the [**Releases Page**](https://github.com/Cornfy/media-sorter/releases) of this repository.
3.  Download the appropriate binary for your operating system (e.g., `media-sorter_linux_amd64`).
4.  Download the `config.json` file.
5.  Place both the executable and `config.json` in the same directory.
6.  **(For Linux/macOS)** Make the binary executable:
    ```bash
    chmod +x ./media-sorter_linux_amd64
    ```

### ⌨️ Usage

Run the program from your terminal, pointing it to the directory you want to organize.

```bash
# Basic usage with the -dir flag
./media-sorter -dir /path/to/your/photos

# You can also use a positional argument
./media-sorter /path/to/your/photos
```

**Command-line Flags:**

| Flag                | Description                                                      | Default             |
| ------------------- | ---------------------------------------------------------------- | ------------------- |
| `-dir`              | The target directory to process. (Required)                      | `""`                |
| `-yes`              | Bypass the interactive confirmation prompt.                      | `false`             |
| `-no-backup`        | Disable the default backup process.                              | `false`             |
| `-backup-dir`       | Directory to store backups.                                      | `"./media_backups"` |
| `-exiftool-path`    | Manually specify the full path to the exiftool executable.       | `""`                |
| `-depth`            | Max depth for directory traversal. `-1` for infinite (default).  | `-1`                |
| `-h`, `--help`      | Show this help message.                                          | `false`             |

### ⚙️ Configuration

You can customize the tool's behavior by editing the `config.json` file.

```json
{
  "image_prefix": "IMG",
  "video_prefix": "VID",
  "target_timezone": "+08:00",
  "supported_image_extensions": [
    "jpg", "jpeg", "png", "heic", "webp", "gif"
  ],
  "supported_video_extensions": [
    "mp4", "mov", "avi", "mkv"
  ]
}
```
- `image_prefix` / `video_prefix`: The text prepended to renamed image/video files.
- `target_timezone`: The timezone used when writing EXIF tags to images.
- `supported_*_extensions`: Case-insensitive lists of file types to process.

<details>
<summary><b>For Developers: Build from Source</b></summary>

1.  [Install Go](https://go.dev/doc/install) (version 1.18+).
2.  Clone the repository: `git clone https://github.com/Cornfy/media-sorter.git`
3.  Navigate into the directory: `cd media-sorter`
4.  Build the optimized binary:
    ```bash
    go build -ldflags="-s -w"
    ```
</details>
