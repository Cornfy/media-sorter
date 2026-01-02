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

// version 将在编译时通过链接器（linker）进行设置。
// 它的默认值 "development" 会在直接使用 `go run` 时显示。
var version = "development"

type Config struct {
	ImagePrefix              string   `json:"image_prefix"`
	VideoPrefix              string   `json:"video_prefix"`
	TargetTimezone           string   `json:"target_timezone"`
	SupportedImageExtensions []string `json:"supported_image_extensions"`
	SupportedVideoExtensions []string `json:"supported_video_extensions"`
}

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
	if err != nil {	absConfigPath = configFilename }
	configFile, err := os.ReadFile(configFilename)
	if err != nil {
		log.Printf("INFO: Config file %s not found, using default settings.", absConfigPath)
		return defaultConfig
	}
	var userConfig Config
	if err := json.Unmarshal(configFile, &userConfig); err != nil {
		log.Printf("WARNING: Could not parse config file %s (%v), using default settings.", absConfigPath, err)
		return defaultConfig
	}
	log.Printf("INFO: Settings loaded from %s.", absConfigPath)
	return userConfig
}

// REFACTORED: 新增辅助函数，用于将时区偏移量字符串解析为 *time.Location 对象。
// 这是确立 target_timezone 权威性的关键一步。
func parseTimeZone(tzStr string) (*time.Location, error) {
	// 尝试解析为 "UTC", "Local" 等名称
	loc, err := time.LoadLocation(tzStr)
	if err == nil {
		return loc, nil
	}

	// 尝试解析为 "+08:00" 或 "-0700" 这种格式
	// Go 的标准库没有直接解析这种格式的函数，需要手动处理
	if strings.HasPrefix(tzStr, "+") || strings.HasPrefix(tzStr, "-") {
		// 格式化为 time.Parse 需要的 RFC3339 格式
		dummyTimeStr := "2006-01-02T15:04:05" + tzStr
		layouts := []string{"2006-01-02T15:04:05-07:00", "2006-01-02T15:04:05Z0700"}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, dummyTimeStr); err == nil {
				return t.Location(), nil
			}
		}
	}
	return nil, fmt.Errorf("invalid timezone format: %s", tzStr)
}

func main() {
	// 设置和解析命令行参数
	flag.Usage = ui.ShowHelp
	showVersion := flag.Bool("version", false, "Display the application version and exit.")
	flag.BoolVar(showVersion, "v", false, "Display the application version and exit (shorthand).")
	targetDir := flag.String("dir", "", "The target directory to process.")
	maxDepth := flag.Int("depth", -1, "Maximum depth for directory traversal. -1 for infinite, 0 for current directory only.")
	backupDir := flag.String("backup-dir", "./media_backups", "Directory to store backups.")
	exiftoolOverridePath := flag.String("exiftool-path", "", "Manually specify the full path to the exiftool executable.")
	noBackup := flag.Bool("no-backup", false, "Disable the default backup process.")
	autoConfirm := flag.Bool("yes", false, "Bypass the confirmation prompt.")
	flag.Parse()

	// 如果用户使用了 --version 或 -v 标志，则打印版本号并立即退出。
	if *showVersion {
		fmt.Printf("Go Media Sorter version %s\n", version)
		os.Exit(0) // 成功退出，不执行后续任何操作。
	}

	// 加载配置
	cfg := loadConfig()
	imageExtMap := sliceToMap(cfg.SupportedImageExtensions)
	videoExtMap := sliceToMap(cfg.SupportedVideoExtensions)

	// REFACTORED: 立即解析时区，确立其权威地位
	targetLocation, err := parseTimeZone(cfg.TargetTimezone)
	if err != nil {	log.Fatalf("FATAL: Invalid 'target_timezone' in config.json: '%s'. Error: %v", cfg.TargetTimezone, err) }
	log.Printf("INFO: Target timezone set to '%s'.", cfg.TargetTimezone)
	// log.Printf("DEBUG: targetLocation is %#v", targetLocation)	// 调试日志，生产环境应禁用
	
	// 检查 exiftool 依赖
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

	// 确定目标目录
	if *targetDir == "" {
		if len(flag.Args()) > 0 { 
			*targetDir = flag.Arg(0) 
		} else {
		 	log.Println("Error: No target directory specified."); flag.Usage(); os.Exit(1)
		}
	}

	absPath, err := filepath.Abs(*targetDir)
	if err != nil {
		log.Fatalf("ERROR: Failed to resolve absolute path for target directory '%s': %v", *targetDir, err)
	}
	if info, err := os.Stat(absPath); os.IsNotExist(err) || !info.IsDir() {
		log.Fatalf("ERROR: Invalid target directory: '%s'. Directory does not exist or is not a directory.", absPath)
	}
	
	// 显示执行计划
	ui.ShowExecutionPlan(absPath, !*noBackup, *backupDir, exiftoolFound, cfg.SupportedImageExtensions, cfg.SupportedVideoExtensions, *maxDepth)

	// 请求用户确认
	if !*autoConfirm {
		if !ui.RequestConfirmation() { log.Println("Operation cancelled by user."); os.Exit(0) }
	} else {
		fmt.Println("\nAutomation flag (--yes) detected. Proceeding automatically..."); time.Sleep(1 * time.Second)
	}

	// 执行备份
	if !*noBackup {
		fmt.Println("\n--- Starting Backup ---")
		if err := createBackup(absPath, *backupDir); err != nil {
			if *autoConfirm {
				log.Fatalf("ERROR: Backup failed in automated mode (--yes). Aborting operation.")
			} else {
				if !ui.RequestContinueOnFailure(fmt.Sprintf("ERROR: Backup failed! (%v). Continue anyway?", err)) {
					log.Println("Operation cancelled."); os.Exit(1)
				}
			}
		} else {
			fmt.Println("Backup completed successfully!")
		}
		fmt.Println("-----------------------")
	}

	// 开始处理文件 (有微小但关键的修改)
	fmt.Println("\nStarting file processing...")
	cleanAbsPath := filepath.Clean(absPath)

	err = filepath.WalkDir(absPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil { log.Printf("Error: Failed to access path '%s': %v\n", path, err); return err }
		if *maxDepth != -1 {
			relPath, err := filepath.Rel(cleanAbsPath, path)
			if err != nil { return err }
			currentDepth := 0
			if relPath != "." {
				currentDepth = strings.Count(relPath, string(filepath.Separator)) + 1
			}
			if d.IsDir() && currentDepth > *maxDepth {
				return filepath.SkipDir
			}
		}
		if d.IsDir() { return nil }

		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
		isImage := imageExtMap[ext]
		isVideo := videoExtMap[ext]
		if !isImage && !isVideo { return nil }

		var prefix string
		if isImage { prefix = cfg.ImagePrefix } else { prefix = cfg.VideoPrefix }
		
		// CHANGE: 将权威的 targetLocation 对象传递给 processFile
		processFile(path, prefix, exiftoolPath, cfg, imageExtMap, targetLocation)
		return nil
	})

	if err != nil { log.Fatalf("File processing failed during directory traversal: %v", err) }
	fmt.Println("\n========================================")
	fmt.Println("All files have been processed!")
}

// CHANGE: 函数签名变更，接收权威的 targetLocation
func processFile(path, prefix, exiftoolPath string, cfg Config, imageExtMap map[string]bool, targetLocation *time.Location) {
	fmt.Println("----------------------------------------")
	fmt.Printf("Processing files: '%s'\n", filepath.Base(path))

	// CHANGE: 将 targetLocation 传递给 getAuthoritativeTime
	authoritativeTime, source, isAuthoritative, err := getAuthoritativeTime(path, exiftoolPath, imageExtMap, targetLocation)
	if err != nil { log.Printf("  └─ ERROR: Failed to determine authoritative time for %s: %v\n", path, err); return }

	// REFACTORED: 这是整个智能方案的核心！将绝对时刻标准化到目标时区。
	standardizedTime := authoritativeTime.In(targetLocation)
	
	// 从现在起，所有操作都使用 standardizedTime
	// --- OLD ---
	// 直接使用原始的 isAuthoritative
	// newBaseName := generateNewFilename(standardizedTime, prefix, path, isAuthoritative)
	// --- NEW ---
	// 逻辑修正：幂等性修复，“预支”权威时间身份。
	// 修正目的：确保单次运行即可将具备有效毫秒 mtime 的文件提升至亚秒级文件名标准
	// 
	// 如果当前来源不是 EXIF (即 source != "EXIF")，但存在 exiftoolPath (稍后会补录元数据)，
	// 我们检查 standardizedTime 的毫秒是否有效。
	// 如果有效，将从 mtime (回退) 中获得的时间 "提升" 为权威时间。
	if !isAuthoritative && exiftoolPath != "" {
		roundedMs := (standardizedTime.Nanosecond() + 500_000) / 1_000_000
		if roundedMs > 0 { isAuthoritative = true }
	}
	// 此时传入的 isAuthoritative 可能是被我们刚刚 “提升” 过的
	newBaseName := generateNewFilename(standardizedTime, prefix, path, isAuthoritative)
	// -----------
	currentBaseName := filepath.Base(path)
	finalNewPath := path

	if newBaseName != currentBaseName {
		idealNewPath := filepath.Join(filepath.Dir(path), newBaseName)
		finalNewPath, err = getUniquePath(idealNewPath)
		if err != nil { log.Printf("  └─ ERROR: Failed to create unique new path for %s: %v\n", idealNewPath, err); return }
		if err := os.Rename(path, finalNewPath); err != nil {
			log.Printf("  └─ ERROR: Failed to  rename the file to '%s': %v\n", filepath.Base(finalNewPath), err); return
		}
		fmt.Printf("  └─ INFO: Renamed to '%s' (Source: %s)\n", filepath.Base(finalNewPath), source)
	} else {
		fmt.Printf("  └─ INFO: Filename matches standard. No rename performed. (Source: %s)\n", source)
	}

	if err := enrichMetadata(finalNewPath, standardizedTime, exiftoolPath, cfg, imageExtMap); err != nil {
		log.Printf("  └─ ERROR: Failed to enrich metadata: %v\n", err)
	} else if exiftoolPath != "" {
		fmt.Println("  └─ INFO: Metadata checked and enriched.")
	}

	if err := syncFileTimestamp(finalNewPath, standardizedTime); err != nil {
		log.Printf("  └─ ERROR: Failed to sync file system modification time (mtime)for '%s': %v\n", filepath.Base(finalNewPath), err)
	} else {
		fmt.Println("  └─ INFO: File system modification time (mtime) synced to authoritative time.")
	}
}

// getExifDate 调用 exiftool 来读取一个指定文件的单个元数据标签。
func getExifDate(filePath, tagName string, exiftoolPath string) (string, error) {
	args := []string{ "-charset", "UTF8", "-q", "-m", "-p", "$" + tagName, filePath	}
	cmd := exec.Command(exiftoolPath, args...)
	
	// 使用两个独立的 bytes.Buffer 分别捕获 stdout 和 stderr。
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// log.Printf("DEBUG: Executing exiftool command: %s %s", exiftoolPath, strings.Join(cmd.Args[1:], " "))

	if err := cmd.Run(); err != nil {
		fullOutput := strings.TrimSpace(stdoutBuf.String()) + "\n" + strings.TrimSpace(stderrBuf.String())
		return "", fmt.Errorf("exiftool read error for tag '%s' on file '%s': %w, output: %s", tagName, filepath.Base(filePath), err, fullOutput)
	}
	
	dateStr := strings.TrimSpace(stdoutBuf.String())	
	if dateStr == "" || dateStr == "0000:00:00 00:00:00" { return "", nil }
	return dateStr, nil
}


// REFACTORED: 完全重写的 getAuthoritativeTime 函数，实现了智能解析逻辑。
func getAuthoritativeTime(path string, exiftoolPath string, imageExtMap map[string]bool, targetLocation *time.Location) (time.Time, string, bool, error) {
	if exiftoolPath != "" {
		isImage := imageExtMap[strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))]
		
		var timeTags []string
		if isImage {
			// 优先使用带时区的复合标签，其次是 DateTimeOriginal
			timeTags = []string{"Composite:SubSecDateTimeOriginal", "DateTimeOriginal"}
		} else {
			// 视频标签，通常被认为是 UTC
			timeTags = []string{"MediaCreateDate", "TrackCreateDate", "CreateDate"}
		}

		for _, tag := range timeTags {
			dateStr, err := getExifDate(path, tag, exiftoolPath)
			if err != nil {
				// 如果 exiftool 报告错误（如文件编码问题），记录但不中断查找其他标签
				// log.Printf("  └─ DEBUG: ExifTool failed to read tag '%s' for '%s': %v", tag, filepath.Base(path), err)	// 显示调试日志，生产环境可选择禁用
				continue
			}
			if dateStr == "" {
				// dateStr 为空说明标签不存在或无意义
				continue
			}

			// 尝试解析时间字符串
			var parsedTime time.Time
			var parseErr error

			// 检查是否是带时区的格式
			if strings.Contains(dateStr, "+") || strings.Contains(dateStr, "-") || strings.HasSuffix(dateStr, "Z") {
				parsedTime, parseErr = parseExifTime(dateStr, time.UTC) // 初始解析，已包含时区，使用UTC解析，得到绝对时刻
			} else {
				// 无时区信息，根据文件类型应用规则
				var assumedLocation *time.Location
				if isImage {
					// 图片的无时区时间，假定为目标时区
					assumedLocation = targetLocation
				} else {
					// 视频的无时区时间，假定为 UTC
					assumedLocation = time.UTC
				}
				parsedTime, parseErr = parseExifTime(dateStr, assumedLocation)
			}
			
			if parseErr == nil {
				return parsedTime, "metadata (" + tag + ")", true, nil
			}
			// log.Printf("  └─ DEBUG: Failed to parse metadata time '%s' (tag: %s) for '%s': %v", dateStr, tag, filepath.Base(path), parseErr)	// 调试日志，生产环境应禁用
		}
		fmt.Println("  └─ INFO: No relevant metadata found.")
	} else {
		fmt.Printf("  └─ INFO: ExifTool not found. Cannot read metadata.") // 修正提示语
	}

	// 回退到文件 mtime
	fmt.Println("  └─ INFO: Falling back to file modification time (mtime).")
	fileInfo, err := os.Stat(path)
	if err != nil { return time.Time{}, "", false, fmt.Errorf("failed to stat file '%s' for mtime: %w", filepath.Base(path), err) }
	return fileInfo.ModTime(), "mtime", false, nil
}

// REFACTORED & ENHANCED: 函数签名和逻辑变更，通过单词调用写入更全面的元数据标签。
func enrichMetadata(path string, t time.Time, exiftoolPath string, cfg Config, imageExtMap map[string]bool) error {
	if exiftoolPath == "" {
		fmt.Println("  └─ INFO: Skipping metadata enrichment ('exiftool' not found).")
		return nil
	}

	var args []string = []string{"-charset", "UTF8"}
	isImage := imageExtMap[strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))]

	if isImage {
		// === 图片处理逻辑，分为三个独立的步骤 ===

		// 无论时间精度如何，文件都应该有精确到秒的时间信息。
		wallClockStr := t.Format("2006:01:02 15:04:05")
		args = append(args, "-if", `not $DateTimeOriginal or $DateTimeOriginal eq "0000:00:00 00:00:00"`, fmt.Sprintf("-DateTimeOriginal=%s", wallClockStr))
		args = append(args, "-if", `not $CreateDate or $CreateDate eq "0000:00:00 00:00:00"`, fmt.Sprintf("-CreateDate=%s", wallClockStr))
		args = append(args, "-if", `not $ModifyDate or $ModifyDate eq "0000:00:00 00:00:00"`, fmt.Sprintf("-ModifyDate=%s", wallClockStr))

		// 时区为基础时间戳提供上下文，其存在与否与毫秒无关。
		offsetStr := t.Format("-07:00")
		args = append(args, "-if", `not $OffsetTimeOriginal`, fmt.Sprintf("-OffsetTimeOriginal=%s", offsetStr))
		args = append(args, "-if", `not $OffsetTimeDigitized`, fmt.Sprintf("-OffsetTimeDigitized=%s", offsetStr))
		args = append(args, "-if", `not $OffsetTime`, fmt.Sprintf("-OffsetTime=%s", offsetStr))

		// 只有当四舍五入后的毫秒数大于零时，写入才有意义。
		roundedMs := (t.Nanosecond() + 500_000) / 1_000_000
		if roundedMs > 0 {
			if roundedMs >= 1000 { roundedMs = 999 }
			subsecStr := fmt.Sprintf("%03d", roundedMs)
			args = append(args, "-if", `not $SubSecTimeOriginal`, fmt.Sprintf("-SubSecTimeOriginal=%s", subsecStr))
			args = append(args, "-if", `not $SubSecTimeDigitized`, fmt.Sprintf("-SubSecTimeDigitized=%s", subsecStr))
			args = append(args, "-if", `not $SubSecTime`, fmt.Sprintf("-SubSecTime=%s", subsecStr))
		}

	} else {
		// === 视频处理逻辑 ===
		// 准备视频所需的 UTC 时间字符串
		utcTimeStr := t.UTC().Format("2006:01:02 15:04:05")

		// 定义要写入的 QuickTime 标签
		videoTags := []string{
			"MediaCreateDate", "TrackCreateDate", "CreateDate",
			"MediaModifyDate", "TrackModifyDate", "ModifyDate",
		}

		// 构建单一的参数列表
		for _, tag := range videoTags {
			// QuickTime 标签需要明确指定分组
			fullTagName := fmt.Sprintf("QuickTime:%s", tag)
			condition := fmt.Sprintf(`not $%s or $%s eq "0000:00:00 00:00:00"`, fullTagName, fullTagName)
			arg := fmt.Sprintf("-%s=%s", fullTagName, utcTimeStr)
			args = append(args, "-if", condition, arg)
		}
	}

	// 如果除了 -charset 参数之外，没有任何需要执行的操作，则直接返回
	// 这里需要判断 args 的长度是否只有 -charset 的两个参数，则直接返回
	if len(args) == 2 { return nil }

	// 添加通用参数，然后是文件路径
	args = append(args, "-common_args", "-q", "-m", "-overwrite_original", path)

	// 执行单次 exiftool 调用
	cmd := exec.Command(exiftoolPath, args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// ExitCode 2 通常表示 "Minor errors or warnings", 例如文件已包含了部分信息但仍成功更新。可以安全地忽略。
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 2 {
			fmt.Printf("  └─ INFO: Metadata enriched (ExifTool reported minor warnings).\n")
			return nil
		}
		return fmt.Errorf("exiftool write error for '%s': %v, output: %s", filepath.Base(path), err, string(output))
	}

	return nil
}

// REFACTORED: 函数签名和逻辑变更，用于支持智能解析
func parseExifTime(dateStr string, location *time.Location) (time.Time, error) {
	// 增加更多可能的布局，特别是带小数秒和时区的
	layouts := []string{
		"2006:01:02 15:04:05.999999999-07:00",
		"2006:01:02 15:04:05-07:00",
		"2006:01:02 15:04:05Z07:00",
		"2006:01:02 15:04:05.999999999",
		"2006:01:02 15:04:05",
	}
	for _, layout := range layouts {
		// 使用 time.ParseInLocation 来强制应用我们指定的时区规则
		if t, err := time.ParseInLocation(layout, dateStr, location); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("could not parse date: %s", dateStr)
}

func createBackup(sourceDir, backupDir string) error {
	if err := os.MkdirAll(backupDir, 0755); err != nil { return fmt.Errorf("could not create backup directory: %w", err) }
	backupFilename := fmt.Sprintf("backup_%s_%s.tar.gz", filepath.Base(sourceDir), time.Now().Format("20060102_150405"))
	backupFilepath := filepath.Join(backupDir, backupFilename)
	fmt.Printf("Backing up '%s' to '%s'...\n", sourceDir, backupFilepath)
	file, err := os.Create(backupFilepath); if err != nil { return fmt.Errorf("could not create backup file: %w", err) }; defer file.Close()
	gw := gzip.NewWriter(file); defer gw.Close()
	tw := tar.NewWriter(gw); defer tw.Close()

	absBackupDir, err := filepath.Abs(backupDir)
	if err != nil {
		return fmt.Errorf("could not resolve absolute path for backup directory: %w", err)
	}
	return filepath.Walk(sourceDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil { return err }
		if filepath.Clean(path) == filepath.Clean(absBackupDir) {
			log.Printf("INFO: Skipping backup directory itself: %s", path)
			return filepath.SkipDir
		}
		header, err := tar.FileInfoHeader(info, info.Name()); if err != nil { return err }
		relPath, err := filepath.Rel(sourceDir, path); if err != nil { return err }
		header.Name = relPath
		if err := tw.WriteHeader(header); err != nil { return err }
		if !info.Mode().IsRegular() { return nil }
		f, err := os.Open(path); if err != nil { return err }; defer f.Close()
		_, err = io.Copy(tw, f); return err
	})
}

func sliceToMap(s []string) map[string]bool {
	m := make(map[string]bool); for _, v := range s { m[v] = true }; return m
}

func syncFileTimestamp(path string, t time.Time) error { 
	return os.Chtimes(path, t, t) 
}

func getUniquePath(path string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) { return path, nil }
	dir, ext := filepath.Dir(path), filepath.Ext(path)
	baseName := strings.TrimSuffix(filepath.Base(path), ext)
	randNum, err := rand.Int(rand.Reader, big.NewInt(1000)); if err != nil { return "", err }
	return filepath.Join(dir, fmt.Sprintf("%s_[%03d]%s", baseName, randNum, ext)), nil
}

func generateNewFilename(t time.Time, prefix, originalPath string, isAuthoritative bool) string {
	ext, baseTime := filepath.Ext(originalPath), t.Format("20060102_150405")
	if isAuthoritative {
		// 增加半毫秒，以实现四舍五入
		roundedMs := (t.Nanosecond() + 500000) / 1000000
		if roundedMs > 0 { 
			if roundedMs >= 1000 { roundedMs = 999 }
			return fmt.Sprintf("%s_%s_%03d%s", prefix, baseTime, roundedMs, ext)
		}
	}
	return fmt.Sprintf("%s_%s%s", prefix, baseTime, ext)
}

