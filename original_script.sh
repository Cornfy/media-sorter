#!/bin/bash

# ==============================================================================
# SCRIPT CONFIGURATION
# Users can customize all parameters in this section.
# ==============================================================================

# 1. Target and Backup Paths
#    - TARGET_DIRECTORY: The default folder to process. Can be overridden by a command-line argument.
#    - ENABLE_BACKUP: If 'true', a full .tar.gz backup is created before any changes are made.
#    - BACKUP_DIRECTORY: The location where the backup file will be stored.
TARGET_DIRECTORY=""
ENABLE_BACKUP=true
BACKUP_DIRECTORY="./media_backups"

# 2. File Naming Conventions
#    - IMAGE_PREFIX: The prefix used for renamed image files (e.g., "IMG").
#    - VIDEO_PREFIX: The prefix used for renamed video files (e.g., "VID").
IMAGE_PREFIX="IMG"
VIDEO_PREFIX="VID"

# 3. Supported File Types
#    - Add or remove case-insensitive file extensions in these arrays.
SUPPORTED_IMAGE_EXTENSIONS=("jpg" "jpeg" "png" "heic" "webp" "gif")
SUPPORTED_VIDEO_EXTENSIONS=("mp4" "mov" "avi" "mkv")

# 4. Metadata Settings
#    - TIMEZONE: Crucial for correct time conversion. E.g., "+08:00", "-05:00".
TIMEZONE="+08:00"


# ==============================================================================
# CORE FUNCTIONS (Do not modify the code below this line)
# ==============================================================================

# ===----------------------------------------------------------------------=== #
# Helper and User Interaction Functions
# ===----------------------------------------------------------------------=== #

# Displays the help message and usage instructions.
show_help() {
    echo "Usage: $0 [TARGET_DIRECTORY]"
    echo
    echo "Intelligently organizes media files in a specified directory with a confirmation step."
    echo
    echo "Arguments:"
    echo "  TARGET_DIRECTORY  (Optional) The directory to process. Overrides the script's internal setting."
    echo
    echo "Options:"
    echo "  --yes, -y         Bypass the interactive confirmation prompt. Use with caution in automated scripts."
    echo "                    This flag is IGNORED if 'exiftool' is not found."
    echo "  --help, -h        Display this help message."
    echo
    echo "Workflow:"
    echo "  1. The script first checks for the 'exiftool' dependency."
    echo "  2. It then displays an 'Execution Plan' detailing what it will do."
    echo "  3. Finally, it requires you to type 'yes' to proceed, preventing accidental runs."
}

# Displays a clear summary of the actions the script is about to perform.
display_execution_plan() {
    local target_dir="$1"; local exiftool_found="$2"

    echo "======================================================================"
    echo "                            EXECUTION PLAN                            "
    echo "======================================================================"; echo
    echo "  TARGET DIRECTORY: $target_dir"; echo
    if [ "$ENABLE_BACKUP" = "true" ]; then
        echo "  BACKUP:           Enabled. A backup will be created in '$BACKUP_DIRECTORY'."
    else
        echo "  BACKUP:           Disabled. Files will be modified in-place without a backup."
    fi; echo
    if [ "$exiftool_found" = "false" ]; then
        echo "  WARNING:          Operating in LIMITED MODE ('exiftool' not found)."
    fi
    echo "  PROCESSING:       Images & Videos"
    if [ ${#SUPPORTED_IMAGE_EXTENSIONS[@]} -gt 0 ]; then
        echo "  Image Types:      ${SUPPORTED_IMAGE_EXTENSIONS[*]}"
    fi
    if [ ${#SUPPORTED_VIDEO_EXTENSIONS[@]} -gt 0 ]; then
        echo "  Video Types:      ${SUPPORTED_VIDEO_EXTENSIONS[*]}"
    fi; echo
    echo "----------------------------------------------------------------------"
    echo "  WORKFLOW OVERVIEW:"
    echo "----------------------------------------------------------------------"
    echo "  1. [Read Time]:   The script will find the single most authoritative"
    echo "                  timestamp from each file's metadata (EXIF/QuickTime)."
    echo "                  If metadata is missing, it will fall back to the"
    echo "                  file's 'last modification time'."
    echo
    echo "  2. [Normalize TZ]: All timestamps will be normalized to your configured"
    echo "                   target timezone ($TIMEZONE)."
    echo
    echo "  3. [Rename File]: Files will be renamed to match the authoritative time:"
    echo "                  - ${IMAGE_PREFIX}_YYYYMMDD_HHMMSS.ext"
    echo "                  - ${IMAGE_PREFIX}_YYYYMMDD_HHMMSS_ms.ext (if milliseconds are present)"
    echo
    echo "  4. [Sync Info]:"
    echo "     - The system file timestamp will always be synced to the authoritative time."
    echo "     - Metadata timestamps will be intelligently enriched:"
    echo "       - EMPTY timestamp fields will be filled in."
    echo "       - EXISTING timestamp fields will NOT be overwritten."
    echo "======================================================================"
}

# Creates a .tar.gz backup of the source directory if enabled in the configuration.
create_backup() {
    local source_dir="$1"; if [ "$ENABLE_BACKUP" != "true" ]; then
        echo "Backup is disabled. Skipping backup step."; return 0
    fi
    echo "--- Starting Backup ---"
    if [ ! -d "$source_dir" ]; then
        echo "ERROR: Source directory '$source_dir' does not exist." >&2; return 1
    fi
    mkdir -p "$BACKUP_DIRECTORY"
    local backup_filename="backup_$(basename "$source_dir")_$(date +'%Y%m%d_%H%M%S').tar.gz"
    local backup_filepath="${BACKUP_DIRECTORY}/${backup_filename}"
    echo "Backing up '$source_dir' to '$backup_filepath'..."
    if tar -czf "$backup_filepath" -C "$(dirname "$source_dir")" "$(basename "$source_dir")"; then
        echo "Backup completed successfully!"; echo "-----------------------"; return 0
    else
        echo "ERROR: Backup failed! Check permissions and available disk space." >&2; echo "-----------------------" >&2; return 1
    fi
}

# ===----------------------------------------------------------------------=== #
# Timestamp and File Operation Functions
# ===----------------------------------------------------------------------=== #

# Converts a user-provided timezone offset (e.g., +08:00) to a POSIX-compliant TZ string (e.g., Etc/GMT-8).
get_posix_tz_from_offset() {
    local offset="$1"
    local sign=${offset:0:1}
    local hours=$(echo "$offset" | cut -d':' -f1 | sed 's/[+-]//')
    # Invert the sign for the Etc/GMT format.
    if [ "$sign" = "+" ]; then
        echo "Etc/GMT-$((10#$hours))"
    else
        echo "Etc/GMT+$((10#$hours))"
    fi
}

# Retrieves the authoritative timestamp from a file's metadata.
# Returns a pure, UTC-based Epoch timestamp with nanoseconds.
get_epoch_from_metadata() {
    local file="$1"; local type="$2"; local time_tags=()
    if [ "$type" = "image" ]; then
        time_tags=(
            "Composite:SubSecDateTimeOriginal"
	    "Composite:DateTimeOriginal"
	    "DateTimeOriginal"
	    "Composite:DateTimeCreated"
	    "CreateDate"
	    "Composite:ModifyDate"
	    "ModifyDate"
        )
    else # video
        time_tags=("MediaCreateDate" "TrackCreateDate" "CreateDate" "ModifyDate")
    fi

    for tag in "${time_tags[@]}"; do
        local raw_time_str; raw_time_str=$(exiftool -q -m -p "\$$tag" -coordFormat "%.8f" "$file")
        if [ -n "$raw_time_str" ] && [ "$raw_time_str" != "0000:00:00 00:00:00" ]; then
            local time_to_parse="$raw_time_str"; local parsing_tz="UTC"
            if [[ "$time_to_parse" =~ [\+\-Z] ]]; then
                parsing_tz=""
            elif [ "$type" = "video" ]; then
                : # For naive video timestamps, we've established they are UTC.
            else
                parsing_tz="$(get_posix_tz_from_offset "$TIMEZONE")" # For other naive timestamps, assume configured TARGET timezone.
            fi
            
            local parsable_date_str; parsable_date_str=$(echo "$time_to_parse" | sed 's/:/-/1; s/:/-/1;')
            local epoch_with_nanos; epoch_with_nanos=$(TZ="$parsing_tz" date -d "$parsable_date_str" "+%s.%N" 2>/dev/null)

            if [ -n "$epoch_with_nanos" ]; then
                echo "$epoch_with_nanos"; return 0
            fi
        fi
    done
}

# Renames a file based on a given epoch timestamp and prefix.
rename_file_from_epoch() {
    local file="$1"; local epoch="$2"; local prefix="$3"
    
    local target_tz; target_tz=$(get_posix_tz_from_offset "$TIMEZONE")
    local time_str; time_str=$(TZ="$target_tz" date -d "@$epoch" +"%Y:%m:%d %H:%M:%S.%3N")
    
    local dir_path; dir_path=$(dirname "$file"); local filename; filename=$(basename "$file"); local extension; extension="${filename##*.}"
    local base_time; base_time=$(echo "$time_str" | cut -d'.' -f1); local ms; ms=$(echo "$time_str" | cut -d'.' -f2)
    local formatted_time; formatted_time=$(echo "$base_time" | sed -e 's/://g' -e 's/ /_/')

    local ideal_basename
    if [[ "$ms" =~ ^[0-9]{3}$ ]] && [ "$ms" != "000" ]; then
        ideal_basename="${prefix}_${formatted_time}_${ms}.${extension}"
    else
        ideal_basename="${prefix}_${formatted_time}.${extension}"
    fi

    if [ "$filename" = "$ideal_basename" ]; then
        echo "Skipping rename: Filename is already perfect." >&2; echo "$file"; return 0
    fi
    
    local final_filepath; final_filepath="${dir_path}/${ideal_basename}"
    if [ -e "$final_filepath" ]; then
        local random_suffix; random_suffix=$(head /dev/urandom | tr -dc 0-9 | head -c3)
        local new_basename
        if [[ "$ms" =~ ^[0-9]{3}$ ]] && [ "$ms" != "000" ]; then
            new_basename="${prefix}_${formatted_time}_${ms}_[${random_suffix}].${extension}"
        else
            new_basename="${prefix}_${formatted_time}_[${random_suffix}].${extension}"
        fi
        final_filepath="${dir_path}/${new_basename}"
    fi
    
    echo "Renaming '$(basename "$file")' -> '$(basename "$final_filepath")'" >&2
    if mv -- "$file" "$final_filepath"; then
        echo "$final_filepath"
    else
        echo "ERROR: Rename failed." >&2; echo "$file"
    fi
}

# Intelligently enriches metadata from a given epoch timestamp.
# It only fills in EMPTY or INVALID fields and never overwrites existing valid data,
# achieving this with a single, high-performance exiftool command.
write_and_enrich_metadata_from_epoch() {
    local file="$1"
    local epoch="$2"
    local file_type="$3"

    echo "Checking and enriching metadata..." >&2

    if [ "$file_type" = "image" ]; then
        local target_tz; target_tz=$(get_posix_tz_from_offset "$TIMEZONE")
        
        # Prepare all necessary time components based on the target timezone.
        local time_str_full; time_str_full=$(TZ="$target_tz" date -d "@$epoch" +"%Y:%m:%d %H:%M:%S.%3N")
        local time_with_zone; time_with_zone="${time_str_full/./,}${TIMEZONE}"
        local time_naive; time_naive=$(echo "$time_str_full" | cut -d'.' -f1)
        local offset_only="$TIMEZONE"
        local subsec_only; subsec_only=$(echo "$time_str_full" | cut -d'.' -f2)

        # Use the robust "-TAG-= -TAG=VALUE" idiom to write a value only if the tag
        # was previously empty or non-existent. This is an atomic and idempotent operation.
        exiftool \
            -if 'not $DateTimeOriginal or $DateTimeOriginal eq "0000:00:00 00:00:00"' -DateTimeOriginal="$time_naive" \
            -if 'not $SubSecTimeOriginal' -SubSecTimeOriginal="$subsec_only" \
            -if 'not $OffsetTimeOriginal' -OffsetTimeOriginal="$offset_only" \
            -if 'not $CreateDate or $CreateDate eq "0000:00:00 00:00:00"' -CreateDate="$time_naive" \
            -if 'not $SubSecTimeDigitized' -SubSecTimeDigitized="$subsec_only" \
            -if 'not $OffsetTimeDigitized' -OffsetTimeDigitized="$offset_only" \
            -if 'not $ModifyDate or $ModifyDate eq "0000:00:00 00:00:00"' -ModifyDate="$time_naive" \
            -if 'not $SubSecTime' -SubSecTime="$subsec_only" \
            -if 'not $OffsetTime' -OffsetTime="$offset_only" \
            -common_args -q -m -overwrite_original "$file" > /dev/null

    else # video
        # For videos, prepare the timestamp in the UTC standard.
        local utc_time_str; utc_time_str=$(date -u -d "@$epoch" +"%Y:%m:%d %H:%M:%S")
        local offset_utc="+00:00"

        # Use a single, atomic command with conditional checks. This is robust for videos.
        exiftool \
            -if 'not $QuickTime:MediaCreateDate or $QuickTime:MediaCreateDate eq "0000:00:00 00:00:00"' -QuickTime:MediaCreateDate="$utc_time_str" \
            -if 'not $QuickTime:TrackCreateDate or $QuickTime:TrackCreateDate eq "0000:00:00 00:00:00"' -QuickTime:TrackCreateDate="$utc_time_str" \
            -if 'not $QuickTime:CreateDate or $QuickTime:CreateDate eq "0000:00:00 00:00:00"' -QuickTime:CreateDate="$utc_time_str" \
            -if 'not $QuickTime:MediaModifyDate or $QuickTime:MediaModifyDate eq "0000:00:00 00:00:00"' -QuickTime:MediaModifyDate="$utc_time_str" \
            -if 'not $QuickTime:TrackModifyDate or $QuickTime:TrackModifyDate eq "0000:00:00 00:00:00"' -QuickTime:TrackModifyDate="$utc_time_str" \
            -if 'not $QuickTime:ModifyDate or $QuickTime:ModifyDate eq "0000:00:00 00:00:00"' -QuickTime:ModifyDate="$utc_time_str" \
            -if 'not $QuickTime:OffsetTimeOriginal' -OffsetTimeOriginal="$offset_utc" \
            -common_args -q -m -overwrite_original "$file" > /dev/null
    fi
    return $?
}

# Syncs the file's system timestamps with a given epoch timestamp.
sync_file_timestamp_from_epoch() {
    local file="$1"; local epoch="$2"
    
    # 'touch -d' can directly use the epoch format with '@'.
    touch -d "@$epoch" "$file"
    echo "System file timestamp synced successfully." >&2
}

# Orchestrates the entire processing pipeline for a single file.
process_file() {
    local original_filepath="$1"; local file_type="$2"; local exiftool_found="$3"
    echo "----------------------------------------" >&2
    echo "Processing [$file_type]: $(basename "$original_filepath")" >&2

    local authoritative_epoch
    
    if [ "$exiftool_found" = "true" ]; then
        authoritative_epoch=$(get_epoch_from_metadata "$original_filepath" "$file_type")
    fi

    if [ -z "$authoritative_epoch" ]; then
        authoritative_epoch=$(date -r "$original_filepath" +%s.%N)
        local target_tz; target_tz=$(get_posix_tz_from_offset "$TIMEZONE")
        local localized_time_for_log; localized_time_for_log=$(TZ="$target_tz" date -d "@$authoritative_epoch" +"%Y:%m:%d %H:%M:%S")
        echo "WARNING: No valid metadata found. Falling back to mtime (in local TZ): $localized_time_for_log" >&2
    else
        local target_tz; target_tz=$(get_posix_tz_from_offset "$TIMEZONE")
        local localized_time_for_log; localized_time_for_log=$(TZ="$target_tz" date -d "@$authoritative_epoch" +"%Y:%m:%d %H:%M:%S.%3N")
        echo "Authoritative timestamp found and localized: $localized_time_for_log" >&2
    fi

    local prefix; prefix=$([ "$file_type" = "image" ] && echo "$IMAGE_PREFIX" || echo "$VIDEO_PREFIX")
    local current_filepath; current_filepath=$(rename_file_from_epoch "$original_filepath" "$authoritative_epoch" "$prefix")

    # Metadata enrichment and timestamp sync are only performed if exiftool exists.
    if [ "$exiftool_found" = "true" ]; then
        # Always attempt to enrich metadata. The function itself is smart enough to not overwrite.
        write_and_enrich_metadata_from_epoch "$current_filepath" "$authoritative_epoch" "$file_type"
        
        # Always sync the system file timestamp to the authoritative time.
        sync_file_timestamp_from_epoch "$current_filepath" "$authoritative_epoch"
    else
        echo "Skipping metadata and timestamp sync ('exiftool' not found)." >&2
    fi
}

# Finds all supported files in a directory and passes them to the processor.
process_directory() {
    local directory="$1"; local exiftool_found="$2"
    local find_image_args=(); for ext in "${SUPPORTED_IMAGE_EXTENSIONS[@]}"; do [ ${#find_image_args[@]} -gt 0 ] && find_image_args+=(-o); find_image_args+=(-iname "*.${ext}"); done
    local find_video_args=(); for ext in "${SUPPORTED_VIDEO_EXTENSIONS[@]}"; do [ ${#find_video_args[@]} -gt 0 ] && find_video_args+=(-o); find_video_args+=(-iname "*.${ext}"); done
    
    echo; echo "=== Processing Image Files... ==="
    if [ ${#find_image_args[@]} -gt 0 ]; then
        while IFS= read -r -d '' file; do
            process_file "$file" "image" "$exiftool_found"
        done < <(find "$directory" -type f \( "${find_image_args[@]}" \) -print0)
    fi
    
    echo; echo "=== Processing Video Files... ==="
    if [ ${#find_video_args[@]} -gt 0 ]; then
        while IFS= read -r -d '' file; do
            process_file "$file" "video" "$exiftool_found"
        done < <(find "$directory" -type f \( "${find_video_args[@]}" \) -print0)
    fi
}

# ==============================================================================
# SCRIPT ENTRY POINT
# ==============================================================================

main() {
    local exiftool_found=true
    if ! command -v exiftool &> /dev/null; then
        exiftool_found=false
    fi

    local target_arg=""; local auto_confirm=false
    for arg in "$@"; do
        case "$arg" in
            --yes | -y) auto_confirm=true;;
            --help | -h) show_help; exit 0;;
            *) if [ -z "$target_arg" ]; then target_arg="$arg"; fi;;
        esac
    done

    if [ "$exiftool_found" = "false" ]; then
        echo "######################################################################" >&2
        echo "#                                                                    #" >&2
        echo "#                 !!! CRITICAL WARNING: 'exiftool' not found !!!       #" >&2
        echo "#                                                                    #" >&2
        echo "######################################################################" >&2
        echo "" >&2;
	echo "This script's core functionality is DISABLED." >&2
        echo "You MUST understand the consequences:" >&2;
	echo "" >&2
        echo "[ WILL NOT WORK ]" >&2;
	echo "  - Reading media metadata (EXIF, QuickTime)." >&2;
	echo "  - Writing/enriching media metadata." >&2;
	echo "" >&2
        echo "[ WILL HAPPEN INSTEAD ]" >&2;
	echo "  - The script will use 'last file modification time' (mtime) for ALL files." >&2;
	echo "" >&2
        echo "[ CONSEQUENCES ]" >&2;
	echo "  - Files may be RENAMED INCORRECTLY based on mtime." >&2;
	echo "  - Millisecond precision from metadata WILL NOT BE USED in new filenames." >&2;
	echo "  - The original metadata INSIDE the files will remain UNTOUCHED and SAFE." >&2;
	echo "" >&2
        echo "[ RECOMMENDATION ]" >&2;
	echo "  - STRONGLY RECOMMENDED to stop and install exiftool." >&2
        echo "    (e.g., 'sudo apt install libimage-exiftool-perl' or 'sudo pacman -S perl-image-exiftool')" >&2;
	echo "" >&2
        local critical_confirmation_phrase="Please continue anyway!"
        read -p "To proceed in this limited mode, type: '$critical_confirmation_phrase' " confirmation
        echo
        if [ "$confirmation" != "$critical_confirmation_phrase" ]; then
            echo "Operation cancelled."; exit 1
        fi
    fi

    local resolved_target_dir="${target_arg:-$TARGET_DIRECTORY}"
    if [ -z "$resolved_target_dir" ]; then echo "ERROR: Target directory not specified." >&2; show_help; exit 1; fi
    if [ ! -d "$resolved_target_dir" ]; then echo "ERROR: Directory not found: '$resolved_target_dir'" >&2; exit 1; fi
    
    local absolute_target_path; absolute_target_path=$(realpath "$resolved_target_dir")
    
    display_execution_plan "$absolute_target_path" "$exiftool_found"

    if [ "$auto_confirm" = "false" ]; then
        read -p "Are you sure you want to proceed? (Type 'yes' to continue): " confirmation
        if [ "$confirmation" != "yes" ]; then echo "Operation cancelled by user."; exit 1; fi
    else
        if [ "$exiftool_found" = "true" ]; then
            echo "Automation flag (--yes) detected. Proceeding automatically..."; sleep 1
        else
            echo "WARNING: --yes flag is ignored when 'exiftool' not found."
        fi
    fi

    if ! create_backup "$absolute_target_path"; then
        read -p "Backup failed! Continue anyway? (y/N) " -n 1 -r; echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then echo "Operation cancelled."; exit 1; fi
    fi

    process_directory "$absolute_target_path" "$exiftool_found"
    
    echo "========================================"
    echo "All files have been processed!"
}

# Execute the main function with all provided command-line arguments.
main "$@"
