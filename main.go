package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"media-sorter/ui"
)

// Config 结构体定义了所有用户可通过 config.json 配置的设置。
type Config struct {
	ImagePrefix              string   `json:"image_prefix"`
	VideoPrefix              string   `json:"video_prefix"`
	TargetTimezone           string   `json:"target_timezone"`
	SupportedImageExtensions []string `json:"supported_image_extensions"`
	SupportedVideoExtensions []string `json:"supported_video_extensions"`
}

// loadConfig 从 config.json 读取配置。如果文件不存在或无效，则返回一套安全的默认配置。
func loadConfig() Config {
	defaultConfig := Config{
		ImagePrefix:              "IMG",
		VideoPrefix:              "VID",
		TargetTimezone:           "+08:00",
		SupportedImageExtensions: []string{"jpg", "jpeg", "png", "heic", "webp", "gif"},
		SupportedVideoExtensions: []string{"mp4", "mov", "avi", "mkv"},
	}
	configFilename := "config.json"
	absConfigPath, err := filepath.Abs(configFilename)
	if err != nil {
		absConfigPath = configFilename
	}
	configFile, err := os.ReadFile(configFilename)
	if err != nil {
		log.Printf("INFO: %s not found, using default settings.", absConfigPath)
		return defaultConfig
	}
	var userConfig Config
	if err := json.Unmarshal(configFile, &userConfig); err != nil {
		log.Printf("WARNING: Could not parse %s (%v), using default settings.", absConfigPath, err)
		return defaultConfig
	}
	log.Printf("INFO: Loaded settings from %s.", absConfigPath)
	return userConfig
}

// main 是程序的入口点，负责协调所有操作。
func main() {
	// 1. 加载配置
	cfg := loadConfig()
	imageExtMap := sliceToMap(cfg.SupportedImageExtensions)
	videoExtMap := sliceToMap(cfg.SupportedVideoExtensions)

	// 2. 设置和解析命令行参数
	flag.Usage = ui.ShowHelp
	targetDir := flag.String("dir", "", "The target directory to process.")
	autoConfirm := flag.Bool("yes", false, "Bypass the confirmation prompt.")
	noBackup := flag.Bool("no-backup", false, "Disable the default backup process.")
	backupDir := flag.String("backup-dir", "./media_backups", "Directory to store backups.")

	// --- NEW ---
	// 为支持GUI调用，添加 --exiftool-path 标志。
	exiftoolOverridePath := flag.String("exiftool-path", "", "Manually specify the full path to the exiftool executable.")
	// -----------

	// --- NEW ---
	// 添加 -depth 标志来控制递归深度。
	// 使用 -1 作为默认值，表示无限深度（即原始的递归行为）。
	maxDepth := flag.Int("depth", -1, "Maximum depth for directory traversal. -1 for infinite, 0 for current directory only.")
	// -----------

	flag.Parse()

	// 3. 检查 exiftool 依赖
	// --- OLD ---
	// exiftoolFound := true
	// if _, err := exec.LookPath("exiftool"); err != nil {
	// 	exiftoolFound = false
	// 	ui.ShowExiftoolWarning()
	// 	if !ui.RequestCriticalConfirmation("Please continue anyway!") {
	// 		log.Println("Operation cancelled by user."); os.Exit(1)
	// 	}
	// }
	// --- NEW ---
	// 增强的依赖检查逻辑，优先使用 --exiftool-path 标志。
	var exiftoolPath string
	exiftoolFound := false
	if *exiftoolOverridePath != "" {
		if _, err := os.Stat(*exiftoolOverridePath); err == nil {
			exiftoolPath = *exiftoolOverridePath
			exiftoolFound = true
			log.Printf("INFO: Using exiftool from user-provided path: %s", exiftoolPath)
		} else {
			log.Fatalf("FATAL: exiftool not found at the path provided by --exiftool-path: %s", *exiftoolOverridePath)
		}
	} else {
		pathInSystem, err := exec.LookPath("exiftool")
		if err == nil {
			exiftoolPath = pathInSystem
			exiftoolFound = true
		}
	}

	if !exiftoolFound {
		ui.ShowExiftoolWarning()
		if !ui.RequestCriticalConfirmation("Please continue anyway!") {
			log.Println("Operation cancelled by user."); os.Exit(1)
		}
	}
	// -----------

	// 4. 确定目标目录
	if *targetDir == "" {
		if len(flag.Args()) > 0 { *targetDir = flag.Arg(0) } else {
		 	log.Println("Error: Target directory not specified."); flag.Usage(); os.Exit(1)
		}
	}

	absPath, err := filepath.Abs(*targetDir)
	if err != nil { log.Fatalf("Error resolving absolute path: %v", err) }
	if info, err := os.Stat(absPath); os.IsNotExist(err) || !info.IsDir() {
		log.Fatalf("Error: Invalid target directory: %s", absPath)
	}

	// 5. 显示执行计划
	// --- OLD ---
	// ui.ShowExecutionPlan(absPath, !*noBackup, *backupDir, exiftoolFound, cfg.SupportedImageExtensions, cfg.SupportedVideoExtensions)
	// --- NEW ---
	ui.ShowExecutionPlan(absPath, !*noBackup, *backupDir, exiftoolFound, cfg.SupportedImageExtensions, cfg.SupportedVideoExtensions, *maxDepth)
	// -----------

	// 6. 请求用户确认
	if !*autoConfirm {
		if !ui.RequestConfirmation() { log.Println("Operation cancelled by user."); os.Exit(0) }
	} else {
		fmt.Println("\nAutomation flag (--yes) detected. Proceeding automatically..."); time.Sleep(1 * time.Second)
	}

	// 7. 执行备份
	if !*noBackup {
		fmt.Println("\n--- Starting Backup ---")
		if err := createBackup(absPath, *backupDir); err != nil {
			if *autoConfirm {
				log.Fatalf("ERROR: Backup failed in automated mode (--yes). Aborting operation.")
			} else {
				if !ui.RequestContinueOnFailure(fmt.Sprintf("ERROR: Backup failed! (%v)", err)) {
					log.Println("Operation cancelled."); os.Exit(1)
				}
			}
		} else {
			fmt.Println("Backup completed successfully!")
		}
		fmt.Println("-----------------------")
	}

	// 8. 开始处理文件
	fmt.Println("\nStarting file processing...")

	// --- NEW ---
	// 为了计算深度，需要在 WalkDir 的回调外部能访问到 clean 过的根路径。
	// 这行代码是为深度检查做准备。
	cleanAbsPath := filepath.Clean(absPath)
	// -----------

	err = filepath.WalkDir(absPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil { log.Printf("Error accessing path %q: %v\n", path, err); return err }

		// --- NEW ---
		// 深度控制逻辑：在处理任何文件或目录之前，首先检查深度。
		if *maxDepth != -1 { // 仅当用户指定了深度时才执行检查
			// 计算当前路径相对于根路径的深度
			relPath, err := filepath.Rel(cleanAbsPath, path)
			if err != nil { return err }

			// 根目录 "." 的深度为 0。子目录的深度是路径分隔符的数量。
			// 例： "level1" 深度为1, "level1/level2" 深度为2。
			currentDepth := 0
			if relPath != "." {
				currentDepth = strings.Count(relPath, string(filepath.Separator)) + 1
			}

			// 如果当前项是一个目录，并且它的深度已经达到了不想进入的层级，则跳过。
			// 要处理 depth=N 的目录下的文件，但不想进入 depth=N+1 的目录。
			if d.IsDir() && currentDepth > *maxDepth {
				return filepath.SkipDir
			}
		}
		// -----------

		if d.IsDir() { return nil }
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
		isImage := imageExtMap[ext]
		isVideo := videoExtMap[ext]
		if !isImage && !isVideo { return nil }
		var prefix string
		if isImage { prefix = cfg.ImagePrefix } else { prefix = cfg.VideoPrefix }
		// --- OLD ---
		// processFile(path, prefix, exiftoolFound, cfg, imageExtMap)
		// --- NEW ---
		// 将最终确定的 exiftoolPath 传递下去
		processFile(path, prefix, exiftoolPath, cfg, imageExtMap)
		// -----------
		return nil
	})

	if err != nil { log.Fatalf("Error walking directory: %v", err) }
	fmt.Println("\n========================================"); fmt.Println("All files have been processed!")
}

// processFile 对单个文件执行完整的处理工作流：获取时间、重命名、同步时间戳和丰富元数据。
// --- OLD ---
// func processFile(path, prefix string, exiftoolFound bool, cfg Config, imageExtMap map[string]bool) {
// --- NEW ---
// 签名变更：接收 exiftoolPath 字符串，而不是 exiftoolFound 布尔值。
func processFile(path, prefix string, exiftoolPath string, cfg Config, imageExtMap map[string]bool) {
// -----------
	fmt.Println("----------------------------------------")
	fmt.Printf("Processing %s\n", filepath.Base(path))

	// --- OLD ---
	// authoritativeTime, source, isAuthoritative, err := getAuthoritativeTime(path, exiftoolFound, imageExtMap)
	// --- NEW ---
	// 将 exiftoolPath 传递下去
	authoritativeTime, source, isAuthoritative, err := getAuthoritativeTime(path, exiftoolPath, imageExtMap)
	// -----------
	if err != nil { log.Printf("  └─ ERROR: Could not get time for %s: %v\n", path, err); return }

	newBaseName := generateNewFilename(authoritativeTime, prefix, path, isAuthoritative)
	currentBaseName := filepath.Base(path)
	finalNewPath := path

	if newBaseName != currentBaseName {
		idealNewPath := filepath.Join(filepath.Dir(path), newBaseName)
		finalNewPath, err = getUniquePath(idealNewPath)
		if err != nil { log.Printf("  └─ ERROR: Could not generate unique path for %s: %v\n", idealNewPath, err); return }
		if err := os.Rename(path, finalNewPath); err != nil {
			log.Printf("  └─ ERROR: Failed to rename to '%s': %v\n", filepath.Base(finalNewPath), err); return
		}
		fmt.Printf("  └─ Renamed to '%s' (Source: %s)\n", filepath.Base(finalNewPath), source)
	} else {
		fmt.Printf("  └─ Filename is already perfect. (Source: %s)\n", source)
	}

	if err := syncFileTimestamp(finalNewPath, authoritativeTime); err != nil {
		log.Printf("  └─ ERROR: Failed to sync file timestamp: %v\n", err)
	} else {
		fmt.Println("  └─ System file timestamp synced.")
	}

	// --- OLD ---
	// if err := enrichMetadata(finalNewPath, authoritativeTime, exiftoolFound, cfg, imageExtMap); err != nil {
	// 	log.Printf("  └─ ERROR: Failed to enrich metadata: %v\n", err)
	// } else if exiftoolFound {
	// 	fmt.Println("  └─ Metadata checked and enriched.")
	// }
	// --- NEW ---
	// 将 exiftoolPath 传递下去，并根据它是否为空来判断是否执行。
	if err := enrichMetadata(finalNewPath, authoritativeTime, exiftoolPath, cfg, imageExtMap); err != nil {
		log.Printf("  └─ ERROR: Failed to enrich metadata: %v\n", err)
	} else if exiftoolPath != "" {
		fmt.Println("  └─ Metadata checked and enriched.")
	}
	// -----------
}

// getAuthoritativeTime 从元数据（首选）或文件修改时间（备用）中查找最权威的时间戳。
// --- OLD ---
// func getAuthoritativeTime(path string, exiftoolFound bool, imageExtMap map[string]bool) (time.Time, string, bool, error) {
// 	if exiftoolFound {
// --- NEW ---
// 签名变更：接收 exiftoolPath 字符串。
func getAuthoritativeTime(path string, exiftoolPath string, imageExtMap map[string]bool) (time.Time, string, bool, error) {
	if exiftoolPath != "" {
// -----------
		isImage := imageExtMap[strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))]
		var timeTags []string
		if isImage { timeTags = []string{"Composite:SubSecDateTimeOriginal", "DateTimeOriginal"} } else { timeTags = []string{"MediaCreateDate", "TrackCreateDate", "CreateDate"} }
		for _, tag := range timeTags {
			// --- OLD ---
			// dateStr, err := getExifDate(path, tag)
			// --- NEW ---
			// 将 exiftoolPath 传递下去
			dateStr, err := getExifDate(path, tag, exiftoolPath)
			// -----------
			if err != nil { continue }
			if dateStr != "" {
				if parsedTime, err := parseExifTime(dateStr); err == nil {
					return parsedTime, "metadata (" + tag + ")", true, nil
				}
			}
		}
		fmt.Println("  └─ INFO: No valid metadata tag found in file.")
	}
	fmt.Println("  └─ Falling back to file modification time (mtime).")
	fileInfo, err := os.Stat(path)
	if err != nil { return time.Time{}, "", false, fmt.Errorf("failed to stat file for mtime: %w", err) }
	return fileInfo.ModTime(), "mtime", false, nil
}

// enrichMetadata 使用 exiftool 将权威时间戳写回到文件中缺失的元数据字段。
// --- OLD ---
// func enrichMetadata(path string, t time.Time, exiftoolFound bool, cfg Config, imageExtMap map[string]bool) error {
// 	if !exiftoolFound {
// --- NEW ---
// 签名变更：接收 exiftoolPath 字符串。
func enrichMetadata(path string, t time.Time, exiftoolPath string, cfg Config, imageExtMap map[string]bool) error {
	if exiftoolPath == "" {
// -----------
		fmt.Println("  └─ Skipping metadata enrichment ('exiftool' not found).")
		return nil
	}
	var operations []string
	isImage := imageExtMap[strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))]
	if isImage {
		timeStrWithZone := t.Format("2006:01:02 15:04:05.000") + cfg.TargetTimezone
		tags := []string{"DateTimeOriginal", "CreateDate", "ModifyDate"}
		for _, tag := range tags {
			operations = append(operations, "-if", fmt.Sprintf("not $EXIF:%s", tag), fmt.Sprintf("-%s=%s", tag, timeStrWithZone), "-execute")
		}
	} else {
		utcTimeStr := t.UTC().Format("2006:01:02 15:04:05")
		tags := []string{"MediaCreateDate", "TrackCreateDate", "CreateDate", "MediaModifyDate", "TrackModifyDate", "ModifyDate"}
		for _, tag := range tags {
			condition := fmt.Sprintf("not $QuickTime:%s or $QuickTime:%s eq '0000:00:00 00:00:00'", tag, tag)
			operations = append(operations, "-if", condition, fmt.Sprintf("-QuickTime:%s=%s", tag, utcTimeStr), "-execute")
		}
	}
	if len(operations) == 0 { return nil }
	operations = operations[:len(operations)-1]
	var args []string
	args = append(args, operations...)
	args = append(args, "-common_args", "-q", "-m", "-overwrite_original")
	args = append(args, path)
	// log.Printf("  └─ DEBUG: Executing exiftool write command: exiftool %s", strings.Join(args, " "))
	// --- OLD ---
	// cmd := exec.Command("exiftool", args...)
	// --- NEW ---
	// 使用 exiftoolPath 变量来调用命令。
	cmd := exec.Command(exiftoolPath, args...)
	// -----------
	output, err := cmd.CombinedOutput()
	if err != nil {
		// --- OLD ---
		// if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 2 {
		// 	log.Printf("  └─ INFO: exiftool reported minor warnings but updated the file: %s", string(output)); return nil
		// }
		// --- NEW ---
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 2 {
			fmt.Printf("  └─ INFO: Metadata enriched (with minor warnings from exiftool).\n"); return nil
		}
		// -----------
		return fmt.Errorf("exiftool write error: %v, output: %s", err, string(output))
	}
	return nil
}

// createBackup 将源目录完整地打包成一个带时间戳的 .tar.gz 文件。
func createBackup(sourceDir, backupDir string) error {
	if err := os.MkdirAll(backupDir, 0755); err != nil { return fmt.Errorf("could not create backup directory: %w", err) }
	backupFilename := fmt.Sprintf("backup_%s_%s.tar.gz", filepath.Base(sourceDir), time.Now().Format("20060102_150405"))
	backupFilepath := filepath.Join(backupDir, backupFilename)
	fmt.Printf("Backing up '%s' to '%s'...\n", sourceDir, backupFilepath)
	file, err := os.Create(backupFilepath); if err != nil { return fmt.Errorf("could not create backup file: %w", err) }; defer file.Close()
	gw := gzip.NewWriter(file); defer gw.Close()
	tw := tar.NewWriter(gw); defer tw.Close()

	// --- NEW ---
	// 为了进行排除检查，需要在 Walk 函数外部先获取备份目录的绝对路径。
	absBackupDir, err := filepath.Abs(backupDir)
	if err != nil {
		return fmt.Errorf("could not resolve absolute path for backup directory: %w", err)
	}
	// -----------
	return filepath.Walk(sourceDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil { return err }
		// --- NEW ---
		// 在处理任何文件或目录之前，首先检查它是否是需要被排除的备份目录。
		if filepath.Clean(path) == filepath.Clean(absBackupDir) {
			log.Printf("INFO: Skipping backup directory itself: %s", path)
			return filepath.SkipDir // 如果是备份目录，则跳过整个目录及其内容。
		}
		// -----------
		header, err := tar.FileInfoHeader(info, info.Name()); if err != nil { return err }
		relPath, err := filepath.Rel(sourceDir, path); if err != nil { return err }
		header.Name = relPath
		if err := tw.WriteHeader(header); err != nil { return err }
		if !info.Mode().IsRegular() { return nil }
		f, err := os.Open(path); if err != nil { return err }; defer f.Close()
		_, err = io.Copy(tw, f); return err
	})
}

// sliceToMap 是一个辅助函数，用于将字符串切片转换为 map 以实现 O(1) 复杂度的快速查找。
func sliceToMap(s []string) map[string]bool {
	m := make(map[string]bool); for _, v := range s { m[v] = true }; return m
}

// syncFileTimestamp 是一个辅助函数，用于将文件的系统访问和修改时间同步到一个指定时间。
func syncFileTimestamp(path string, t time.Time) error { return os.Chtimes(path, t, t) }

// getUniquePath 是一个辅助函数，用于在目标路径已存在时，通过附加一个随机后缀来生成一个唯一的路径。
func getUniquePath(path string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) { return path, nil }
	dir, ext := filepath.Dir(path), filepath.Ext(path)
	baseName := strings.TrimSuffix(filepath.Base(path), ext)
	randNum, err := rand.Int(rand.Reader, big.NewInt(1000)); if err != nil { return "", err }
	return filepath.Join(dir, fmt.Sprintf("%s_[%03d]%s", baseName, randNum, ext)), nil
}

// generateNewFilename 根据时间戳、前缀和权威性标志，生成标准的文件名。
func generateNewFilename(t time.Time, prefix, originalPath string, isAuthoritative bool) string {
	ext, baseTime := filepath.Ext(originalPath), t.Format("20060102_150405")
	if isAuthoritative {
		if ms := t.Nanosecond() / 1e6; ms > 0 { return fmt.Sprintf("%s_%s_%03d%s", prefix, baseTime, ms, ext) }
	}
	return fmt.Sprintf("%s_%s%s", prefix, baseTime, ext)
}

// parseExifTime 使用一组预定义的布局来尝试解析 exiftool 返回的各种日期时间字符串格式。
func parseExifTime(dateStr string) (time.Time, error) {
	layouts := []string{"2006:01:02 15:04:05.999999999-07:00", "2006:01:02 15:04:05-07:00", "2006:01:02 15:04:05.999999999", "2006:01:02 15:04:05"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, dateStr); err == nil { return t, nil }
	}
	return time.Time{}, fmt.Errorf("could not parse date: %s", dateStr)
}

// getExifDate 调用 exiftool 来读取一个指定文件的单个元数据标签。
// --- OLD ---
// func getExifDate(filePath, tagName string) (string, error) {
// 	cmd := exec.Command("exiftool", "-q", "-m", "-p", "$"+tagName, filePath)
// --- NEW ---
// 签名变更：接收 exiftoolPath 字符串并用它来执行命令。
func getExifDate(filePath, tagName string, exiftoolPath string) (string, error) {
	cmd := exec.Command(exiftoolPath, "-q", "-m", "-p", "$"+tagName, filePath)
// -----------
	var out bytes.Buffer; cmd.Stdout = &out; cmd.Stderr = &out
	if err := cmd.Run(); err != nil { return "", fmt.Errorf("exiftool read error: %v, output: %s", err, out.String()) }
	dateStr := strings.TrimSpace(out.String())
	if dateStr == "" || dateStr == "0000:00:00 00:00:00" { return "", nil }
	return dateStr, nil
}
