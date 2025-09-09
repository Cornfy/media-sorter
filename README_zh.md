# 🖼️ Go Media Sorter

**简体中文** | [English](./README.md)

---

一款智能的 Go 语言媒体文件整理工具。本工具会根据照片和视频的元数据，将其重命名为清晰、一致的 `前缀_YYYYMMDD_HHMMSS.ext` 格式。它安全、快速、可配置且跨平台。

本项目是一个高性能的移植和增强版本，其设计哲学源自一个极其出色的 Shell 脚本。

### ✨ 功能特性

- **智能时间戳**：优先使用权威的元数据（EXIF/QuickTime）进行重命名。如果元数据缺失，将安全地回退到文件的最后修改时间（`mtime`）。
- **元数据丰富**：使用权威时间戳，智能地填充媒体文件中空的日期/时间标签（如 `DateTimeOriginal`, `CreateDate`）。**绝不**覆盖任何已有的有效数据。
- **同步系统时间戳**：将文件的系统修改时间与权威时间戳同步，确保在文件系统中保持一致性。
- **安全第一**：
  - **自动备份**：在执行任何更改前，会自动将目标目录完整地打包成一个 `.tar.gz` 备份文件。
  - **操作确认**：开始执行前需要用户明确输入确认，防止意外运行。
  - **冲突处理**：如果两个文件的时间戳完全相同，会自动添加随机后缀 `_[xxx]` 以避免覆盖。
- **依赖感知**：能自动检测核心依赖 `ExifTool` 是否缺失，并在缺失时进入功能受限的安全模式，同时给出清晰的警告。
- **高度可配置**：所有关键参数（文件名前缀、支持的扩展名、时区）都通过一个简单的 `config.json` 文件进行管理，无需修改代码。
- **跨平台**：基于 Go 语言构建，可被编译成适用于 Linux、Windows 和 macOS 的单一原生可执行文件。

### 🔧 前置依赖

本工具依赖强大的 **ExifTool** 来读写媒体元数据。你必须在系统上安装它。

- **Arch Linux:** `sudo pacman -S perl-image-exiftool`
- **Debian/Ubuntu:** `sudo apt-get install libimage-exiftool-perl`
- **macOS (使用 Homebrew):** `brew install exiftool`
- **Windows:** 从 [ExifTool 官网](https://exiftool.org/)下载。

### 🚀 快速开始

1.  **安装 `ExifTool`** (见上文)。
2.  前往本仓库的 [**Releases 页面**](https://github.com/YOUR_USERNAME/go-media-sorter/releases)。
3.  下载适用于你操作系统的二进制文件（例如 `media-sorter_linux_amd64`）。
4.  下载 `config.json` 配置文件。
5.  将可执行文件和 `config.json` 放在同一个目录下。
6.  **(适用于 Linux/macOS)** 为二进制文件添加可执行权限：
    ```bash
    chmod +x ./media-sorter_linux_amd64
    ```

### ⌨️ 使用方法

在你的终端中运行程序，并指向你想要整理的目录。

```bash
# 使用 -dir 标志的基本用法
./media-sorter -dir /path/to/your/photos

# 你也可以直接使用位置参数
./media-sorter /path/to/your/photos
```

**命令行标志:**

| 标志           | 描述                                             | 默认值              |
| -------------- | ------------------------------------------------ | ------------------- |
| `-dir`         | 需要处理的目标目录。(必需)                       | `""`                |
| `-yes`         | 跳过交互式确认提示。                             | `false`             |
| `-no-backup`   | 禁用默认的备份流程。                             | `false`             |
| `-backup-dir`  | 用于存放备份文件的目录。                         | `"./media_backups"` |

### ⚙️ 配置

你可以通过编辑 `config.json` 文件来自定义工具的行为。

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
- `image_prefix` / `video_prefix`: 用于重命名后的图片/视频文件的前缀。
- `target_timezone`: 向图片写入 EXIF 标签时使用的时区。
- `supported_*_extensions`: 需要处理的文件类型列表（不区分大小写）。

<details>
<summary><b>开发者：从源码构建</b></summary>

1.  [安装 Go](https://go.dev/doc/install) (版本 1.18+)。
2.  克隆本仓库: `git clone https://github.com/YOUR_USERNAME/go-media-sorter.git`
3.  进入项目目录: `cd go-media-sorter`
4.  构建优化后的二进制文件:
    ```bash
    go build -ldflags="-s -w"
    ```
</details>
