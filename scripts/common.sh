#!/bin/bash

# Star Forge - Common functions and variables
# Source this in scripts: source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

COMMON_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$COMMON_SCRIPT_DIR")"
BIN_DIR="$PROJECT_DIR/bin"
TOOLS_DIR="$PROJECT_DIR/.tools"
CONFIG_FILE="$PROJECT_DIR/config.yaml"
TARGET_DATA_DIR="$PROJECT_DIR/target-data"
MOUNT_DIR="$PROJECT_DIR/mnt"

export PATH="$TOOLS_DIR/yq:$TOOLS_DIR/pv/bin:$BIN_DIR:$PATH"

log_info() {
    echo -e "$1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

log_warn() {
    echo -e "${YELLOW}$1${NC}"
}

# Usage: print_header "My Title"
print_header() {
    local title="$1"
    local title_len=${#title}
    local term_width=$(tput cols 2>/dev/null || echo 100)

    # Use a shorter line (60% of terminal width or title length + 20, whichever is smaller)
    local max_line_width=$(( term_width * 60 / 100 ))
    local min_line_width=$(( title_len + 20 ))
    local line_width=$min_line_width
    if [[ $max_line_width -lt $min_line_width ]]; then
        line_width=$max_line_width
    fi

    local line=$(printf '─%.0s' $(seq 1 $line_width))
    local title_padding=$(( (line_width - title_len) / 2 ))

    echo -e "\033[1;34m${line}\033[0m"
    printf "%${title_padding}s" ""
    echo -e "\033[1;37m${title}\033[0m"
    echo -e "\033[1;34m${line}\033[0m"
}

# Usage: print_banner "TITLE TEXT" "emoji"
print_banner() {
    local title="$1"
    local emoji="${2:-⚒}"

    local term_width=$(tput cols 2>/dev/null || echo 100)
    local line=$(printf '━%.0s' $(seq 1 $term_width))

    local full_title="$emoji  $title  $emoji"
    local title_len=${#full_title}
    local title_padding=$(( (term_width - title_len) / 2 ))

    echo -e "\033[1;36m${line}\033[0m"
    printf "%${title_padding}s" ""
    echo -e "\033[1;33m${full_title}\033[0m"
    echo -e "\033[1;36m${line}\033[0m"
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run with sudo"
        exit 1
    fi
}

check_config() {
    if [[ ! -f "$CONFIG_FILE" ]]; then
        log_error "Configuration file not found: $CONFIG_FILE"
        exit 1
    fi
}

get_current_target() {
    local target
    target=$(yq -r '.current_target' "$CONFIG_FILE" 2>/dev/null)
    if [[ -z "$target" || "$target" == "null" ]]; then
        echo ""
        return 1
    fi
    echo "$target"
}

# Usage: find_target_index "target_name"
find_target_index() {
    local target_name="$1"
    local target_count
    local name

    target_count=$(yq '.targets | length' "$CONFIG_FILE")

    for i in $(seq 0 $((target_count - 1))); do
        name=$(yq -r ".targets[$i].name" "$CONFIG_FILE")
        if [[ "$name" == "$target_name" ]]; then
            echo "$i"
            return 0
        fi
    done

    echo "-1"
    return 0
}

list_targets() {
    local target_count
    local current_target

    target_count=$(yq '.targets | length' "$CONFIG_FILE")
    current_target=$(get_current_target)

    if [[ "$target_count" == "0" ]]; then
        echo -e "  ${YELLOW}No targets defined${NC}"
        echo -e "  Create one with: sf create"
        return
    fi

    for i in $(seq 0 $((target_count - 1))); do
        local name desc type
        name=$(yq -r ".targets[$i].name" "$CONFIG_FILE")
        desc=$(yq -r ".targets[$i].description // \"\"" "$CONFIG_FILE")
        type=$(yq -r ".targets[$i].type // \"distribution\"" "$CONFIG_FILE")

        if [[ "$name" == "$current_target" ]]; then
            echo -e "  ${GREEN}●${NC} $name [$type]: $desc ${GREEN}(current)${NC}"
        else
            echo -e "  ○ $name [$type]: $desc"
        fi
    done
}

# Usage: validate_target "target_name"
validate_target() {
    local target_name="$1"
    local index

    index=$(find_target_index "$target_name")

    if [[ "$index" == "-1" ]]; then
        log_error "Target '$target_name' not found in configuration"
        echo ""
        echo -e "${BLUE}Available targets:${NC}"
        list_targets
        return 1
    fi

    return 0
}

# Usage: get_partition_count <target_index>
get_partition_count() {
    local target_index="$1"
    yq ".targets[$target_index].partitions | length" "$CONFIG_FILE"
}

# Usage: get_target_description <target_index>
get_target_description() {
    local target_index="$1"
    yq -r ".targets[$target_index].description // \"\"" "$CONFIG_FILE"
}

# Usage: get_target_type <target_index>
get_target_type() {
    local target_index="$1"
    yq -r ".targets[$target_index].type // \"distribution\"" "$CONFIG_FILE"
}

# Usage: get_images_partition <target_index>
get_images_partition() {
    local target_index="$1"
    yq -r ".targets[$target_index].images_partition // \"\"" "$CONFIG_FILE"
}

# Usage: validate_target_type <type>
validate_target_type() {
    local type="$1"
    if [[ "$type" == "installer" || "$type" == "distribution" ]]; then
        return 0
    else
        log_error "Invalid target type: $type"
        log_info "Valid types: installer, distribution"
        return 1
    fi
}

# Usage: update_config "yq expression"
update_config() {
    local yq_expression="$1"
    local temp_config=$(mktemp)
    yq "$yq_expression" "$CONFIG_FILE" > "$temp_config"
    chown --reference="$CONFIG_FILE" "$temp_config"
    chmod --reference="$CONFIG_FILE" "$temp_config"
    mv "$temp_config" "$CONFIG_FILE"
}

# Usage: prompt_input "Prompt text" [default_value]
# Returns the user input (or default if provided and user enters nothing)
prompt_input() {
    local prompt="$1"
    local default="$2"
    local response

    if [[ -n "$default" ]]; then
        echo -ne "${CYAN}${prompt} (${default})${NC}: "
    else
        echo -ne "${CYAN}${prompt}${NC}: "
    fi

    read response

    if [[ -z "$response" && -n "$default" ]]; then
        echo "$default"
    else
        echo "$response"
    fi
}

# Usage: prompt_password "Prompt text"
# Returns the password (hidden input)
prompt_password() {
    local prompt="$1"
    local password

    echo -ne "${CYAN}${prompt}${NC}: "
    read -s password
    echo
    echo "$password"
}

# Usage: prompt_yesno "Question text"
# Returns 0 for yes, 1 for no
prompt_yesno() {
    local prompt="$1"
    echo -ne "${CYAN}${prompt}${NC} [${GREEN}y${NC}/${RED}N${NC}]: "
    read -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        return 0
    else
        return 1
    fi
}

# Usage: confirm_or_exit ["Custom prompt"]
confirm_or_exit() {
    local prompt="${1:-Continue?}"
    echo -ne "${CYAN}${prompt}${NC} [${GREEN}y${NC}/${RED}N${NC}] "
    read -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Cancelled"
        exit 0
    fi
}

# Usage: check_not_current_target <target_name> [operation_name]
check_not_current_target() {
    local target_name="$1"
    local operation="${2:-modify}"
    local current_target=$(get_current_target)
    if [[ "$target_name" == "$current_target" ]]; then
        log_error "Cannot $operation the current target while it is active"
        log_info "Switch to a different target first with: sf use <other-target>"
        exit 1
    fi
}

# Usage: check_is_mounted
check_is_mounted() {
    if mountpoint -q "$MOUNT_DIR" 2>/dev/null; then
        return 0
    else
        return 1
    fi
}

# Usage: check_not_mounted [custom_message]
check_not_mounted() {
    local message="${1:-Partitions are currently mounted}"
    if check_is_mounted; then
        log_error "$message at $MOUNT_DIR"
        log_info "Unmount first with: sf unmount"
        exit 1
    fi
}

# Usage: require_current_target
require_current_target() {
    local target
    target=$(get_current_target)
    if [[ -z "$target" ]]; then
        log_error "No current target set in configuration"

        # Check if any targets exist
        local target_count=$(yq '.targets | length' "$CONFIG_FILE" 2>/dev/null || echo "0")
        if [[ "$target_count" == "0" ]]; then
            log_info "No targets defined. Create one with: sf create"
        else
            log_info "Set target with: sf use <target>"
        fi
        exit 1
    fi
    echo "$target"
}

# Usage: relative_path <absolute_path>
relative_path() {
    local path="$1"

    # If path starts with PROJECT_DIR, make it relative
    if [[ "$path" == "$PROJECT_DIR"* ]]; then
        echo ".${path#$PROJECT_DIR}"
    else
        echo "$path"
    fi
}

# Usage: validate_timezone "timezone_string"
validate_timezone() {
    local tz="$1"

    # Check if it's a UTC offset format (+N or -N, with optional decimal)
    if [[ "$tz" =~ ^[+-][0-9]+(\.[0-9]+)?$ ]]; then
        # Extract the numeric value
        local offset="${tz#[+-]}"
        # Check if offset is within valid range (-12 to +14)
        if (( $(echo "$offset >= 0 && $offset <= 14" | bc -l) )); then
            return 0
        else
            return 1
        fi
    fi

    # Check if it's a valid timezone name from the system database
    if [[ -f "/usr/share/zoneinfo/$tz" ]]; then
        return 0
    fi

    return 1
}

# Usage: normalize_timezone "timezone_string"
normalize_timezone() {
    local tz="$1"

    # Check if it's a UTC offset format
    if [[ "$tz" =~ ^([+-])([0-9]+)(\.[0-9]+)?$ ]]; then
        local sign="${BASH_REMATCH[1]}"
        local offset="${BASH_REMATCH[2]}"
        local decimal="${BASH_REMATCH[3]}"

        # Reverse the sign for Etc/GMT format (POSIX quirk)
        if [[ "$sign" == "+" ]]; then
            sign="-"
        else
            sign="+"
        fi

        # Etc/GMT doesn't support decimal offsets, round to nearest hour
        if [[ -n "$decimal" ]]; then
            log_warn "Decimal timezone offsets not fully supported, rounding to nearest hour"
            offset=$(printf "%.0f" "$offset$decimal")
        fi

        echo "Etc/GMT${sign}${offset}"
    else
        # It's already a timezone name, return as-is
        echo "$tz"
    fi
}