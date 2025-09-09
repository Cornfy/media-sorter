// Package ui 包含了所有与用户界面交互的函数，
// 包括打印信息、警告和请求用户输入。
package ui

import (
	"bufio"
	"fmt"

	"os"
	"strings"
)

// helpText 保存了完整的帮助信息，使用反引号以保留格式。
const helpText = `Usage: media-sorter -dir <TARGET_DIRECTORY> [options]

Intelligently organizes media files in a specified directory with a confirmation step.

Arguments:
  TARGET_DIRECTORY  The directory to process. Can be specified with -dir flag or as the first argument.

Options:
  -dir string               The target directory to process. (Required)
  -yes                      Bypass the interactive confirmation prompt.
  -no-backup                Disable the default backup process.
  -backup-dir string        Directory to store backups. (default "./media_backups")
  -exiftool-path string     Manually specify the full path to the exiftool executable.
  -depth int                Maximum depth for directory traversal. -1 for infinite (default), 0 for current directory only.
  -h, --help                Display this help message.

Workflow:
  1. The program first checks for the 'exiftool' dependency.
  2. It then displays an 'Execution Plan' detailing what it will do.
  3. Finally, it requires you to type 'yes' to proceed, preventing accidental runs.
`

// exiftoolWarningText 保存了 exiftool 缺失时的严重警告信息。
const exiftoolWarningText = `
######################################################################
#                                                                    #
#                 !!! CRITICAL WARNING: 'exiftool' not found !!!       #
#                                                                    #
######################################################################

This program's core functionality is DISABLED.
You MUST understand the consequences:

[ WILL NOT WORK ]
  - Reading media metadata (EXIF, QuickTime).
  - Writing/enriching media metadata.

[ WILL HAPPEN INSTEAD ]
  - The program will use the 'last file modification time' (mtime) for ALL files.

[ CONSEQUENCES ]
  - Files will be RENAMED using potentially inaccurate modification times.
  - Millisecond precision from metadata WILL NOT BE USED in new filenames.
  - The original metadata INSIDE the files will remain UNTOUCHED and SAFE.

[ RECOMMENDATION ]
  - STRONGLY RECOMMENDED to stop and install exiftool.
    (e.g., 'sudo apt install libimage-exiftool-perl' or 'sudo pacman -S perl-image-exiftool')
`

// ShowHelp 打印格式化的帮助信息。
func ShowHelp() {
	fmt.Println(helpText)
}

// ShowExiftoolWarning 打印 exiftool 缺失时的严重警告。
func ShowExiftoolWarning() {
	fmt.Println(exiftoolWarningText)
}

// ShowExecutionPlan 打印一个动态生成的执行计划。
// --- OLD ---
// func ShowExecutionPlan(targetDir string, backupEnabled bool, backupDir string, exiftoolFound bool, imageExts, videoExts []string) {
// --- NEW ---
func ShowExecutionPlan(targetDir string, backupEnabled bool, backupDir string, exiftoolFound bool, imageExts, videoExts []string, maxDepth int) {
// -----------
	fmt.Println("======================================================================")
	fmt.Println("                            EXECUTION PLAN                            ")
	fmt.Println("======================================================================")
	fmt.Printf("\n  TARGET DIRECTORY: %s\n\n", targetDir)

	if backupEnabled {
		fmt.Printf("  BACKUP:           Enabled. A backup will be created in '%s'.\n", backupDir)
	} else {
		fmt.Println("  BACKUP:           Disabled. Files will be modified in-place without a backup.")
	}

	if !exiftoolFound {
		fmt.Println("  WARNING:          Operating in LIMITED MODE ('exiftool' not found).")
	}

	// --- NEW ---
	// 根据 maxDepth 的值，显示关于目录遍历深度的信息。
	switch {
	case maxDepth == -1:
		fmt.Println("  TRAVERSAL DEPTH:  Fully recursive (all subdirectories).")
	case maxDepth == 0:
		fmt.Println("  TRAVERSAL DEPTH:  Current directory only (non-recursive).")
	case maxDepth > 0:
		plural := "s"
		if maxDepth == 1 {
			plural = ""
		}
		fmt.Printf("  TRAVERSAL DEPTH:  Limited to %d level%s deep.\n", maxDepth, plural)
	}
	// -----------

	fmt.Println("\n  PROCESSING:       Images & Videos")
	if len(imageExts) > 0 {
		fmt.Printf("  Image Types:      %s\n", strings.Join(imageExts, " "))
	}
	if len(videoExts) > 0 {
		fmt.Printf("  Video Types:      %s\n", strings.Join(videoExts, " "))
	}

	fmt.Println("\n----------------------------------------------------------------------")
	fmt.Println("  WORKFLOW OVERVIEW:")
	fmt.Println("----------------------------------------------------------------------")
	fmt.Println("  1. [Read Time]:   The program will find the authoritative timestamp")
	fmt.Println("                  from each file's metadata (EXIF/QuickTime). If metadata")
	fmt.Println("                  is missing, it will fall back to the file's 'last")
	fmt.Println("                  modification time'.")
	fmt.Println()
	fmt.Println("  2. [Rename File]: Files will be renamed based on the authoritative time:")
	fmt.Println("                  - PREFIX_YYYYMMDD_HHMMSS.ext")
	fmt.Println("                  - PREFIX_YYYYMMDD_HHMMSS_ms.ext (if milliseconds are present in metadata)")
	fmt.Println()
	fmt.Println("  3. [Sync Info]:")
	fmt.Println("     - The system file timestamp (mtime) will be synced to the authoritative time.")
	fmt.Println("     - Metadata timestamps will be enriched (empty fields will be filled).")
	fmt.Println("======================================================================")
}

// RequestConfirmation 请求用户输入 'yes' 来确认一个标准操作。
// 返回 true 表示用户同意，false 表示拒绝。
func RequestConfirmation() bool {
	fmt.Print("\nAre you sure you want to proceed? (Type 'yes' to continue): ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input) == "yes"
}

// RequestCriticalConfirmation 请求用户输入一个特定的危险操作确认短语。
// 只有当输入与所需短语完全匹配时才返回 true。
func RequestCriticalConfirmation(phrase string) bool {
	fmt.Printf("\nTo proceed in this limited mode, type: '%s' ", phrase)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	fmt.Println() // 增加一个换行，保持格式整洁
	return strings.TrimSpace(input) == phrase
}

// RequestContinueOnFailure 在发生非致命错误（如备份失败）后，询问用户是否继续。
// 接受 'y' 或 'Y' 作为确认。
func RequestContinueOnFailure(message string) bool {
	fmt.Printf("\n%s Continue anyway? (y/N) ", message)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	// 将输入转为小写并去除空格
	return strings.ToLower(strings.TrimSpace(input)) == "y"
}
