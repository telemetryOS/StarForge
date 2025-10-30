#!/bin/bash

# Star Forge - Initialization script
# Contains the init() function that sets up the Star Forge environment

# Lightweight environment setup (no shell launch)
setup_env() {
    # Calculate paths
    local SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
    TOOLS_DIR="$PROJECT_DIR/.tools"
    BOOTSTRAP_DIR="$TOOLS_DIR/arch-bootstrap"
    BOOTSTRAP_ROOT="$BOOTSTRAP_DIR/root.x86_64"

    # Download tools if needed
    download_tools || {
        echo "ERROR: Tool download failed"
        exit 1
    }

    # Create target-data directory
    if [[ ! -d "$PROJECT_DIR/target-data" ]]; then
        mkdir -p "$PROJECT_DIR/target-data"
    fi

    # Create empty config.yaml
    if [[ ! -f "$PROJECT_DIR/config.yaml" ]]; then
        cat > "$PROJECT_DIR/config.yaml" << 'EOF'
# Star Forge Configuration
# This file defines OS targets and their partitions

# Current active target (set with: sf use <name>)
current_target: null

# Target OS configurations
targets: []
EOF
    fi

    # Set up environment
    export PATH="$BOOTSTRAP_ROOT/usr/bin:$BOOTSTRAP_ROOT/bin:$TOOLS_DIR/yq:$TOOLS_DIR/pv/bin:$PROJECT_DIR/bin:$PATH"
    export STAR_FORGE_SHELL=1
}

# Function to download all required tools
download_tools() {
    mkdir -p "$TOOLS_DIR"

    # Get the owner of the project directory
    local project_owner=$(stat -c '%U:%G' "$PROJECT_DIR" 2>/dev/null || echo "root:root")

    # Download Arch bootstrap if needed
    if [[ ! -d "$BOOTSTRAP_ROOT" ]]; then
        local bootstrap_url="https://archive.archlinux.org/iso/2025.10.01/archlinux-bootstrap-x86_64.tar.zst"
        local bootstrap_tarball="$BOOTSTRAP_DIR/archlinux-bootstrap-x86_64.tar.zst"

        echo "First-time setup: Downloading Arch Linux bootstrap..."
        echo "This provides pacman and tools for creating Arch targets"
        echo ""

        mkdir -p "$BOOTSTRAP_DIR"

        if ! wget -q --show-progress -O "$bootstrap_tarball" "$bootstrap_url"; then
            echo "ERROR: Failed to download bootstrap tarball"
            echo "You can manually download from: $bootstrap_url"
            return 1
        fi

        echo "Extracting bootstrap..."
        if ! tar --zstd -xf "$bootstrap_tarball" -C "$BOOTSTRAP_DIR" 2>/dev/null; then
            echo "ERROR: Failed to extract bootstrap tarball"
            return 1
        fi

        echo "Bootstrap downloaded successfully!"
        echo ""
    fi

    # Download yq if needed
    if [[ ! -f "$TOOLS_DIR/yq/yq" ]]; then
        local yq_url="https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64"

        echo "Downloading yq (YAML processor)..."
        mkdir -p "$TOOLS_DIR/yq"

        if ! wget -q --show-progress -O "$TOOLS_DIR/yq/yq" "$yq_url"; then
            echo "ERROR: Failed to download yq"
            return 1
        fi
        chmod +x "$TOOLS_DIR/yq/yq"
        echo "yq downloaded successfully!"
        echo ""
    fi

    # Download and build pv if needed
    if [[ ! -f "$TOOLS_DIR/pv/bin/pv" ]]; then
        local pv_version="1.8.14"
        local pv_url="https://www.ivarch.com/programs/sources/pv-${pv_version}.tar.gz"
        local pv_tarball="$TOOLS_DIR/pv-${pv_version}.tar.gz"
        local pv_build_dir="$TOOLS_DIR/pv-build"

        echo "Downloading pv (pipe viewer)..."
        mkdir -p "$pv_build_dir"

        if ! wget -q --show-progress -O "$pv_tarball" "$pv_url"; then
            echo "ERROR: Failed to download pv"
            return 1
        fi

        echo "Extracting and building pv..."
        if ! tar -xzf "$pv_tarball" -C "$pv_build_dir" --strip-components=1 2>/dev/null; then
            echo "ERROR: Failed to extract pv"
            return 1
        fi

        (cd "$pv_build_dir" && ./configure --prefix="$TOOLS_DIR/pv" >/dev/null 2>&1 && make >/dev/null 2>&1 && make install >/dev/null 2>&1) || {
            echo "ERROR: Failed to build pv"
            return 1
        }

        rm -rf "$pv_tarball" "$pv_build_dir"
        echo "pv built successfully!"
        echo ""
    fi

    # Ensure entire .tools directory has correct ownership
    chown -R "$project_owner" "$TOOLS_DIR" 2>/dev/null || true

    return 0
}

# Main initialization function - enters interactive shell
enter_env() {
    # Capture the original directory before any changes
    local ORIGINAL_DIR="$PWD"

    # Calculate paths relative to bin/sf location
    local SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
    local BIN_PATH="$PROJECT_DIR/bin"
    TOOLS_DIR="$PROJECT_DIR/.tools"
    BOOTSTRAP_DIR="$TOOLS_DIR/arch-bootstrap"
    BOOTSTRAP_ROOT="$BOOTSTRAP_DIR/root.x86_64"

    # Download tools if not present
    download_tools || {
        echo "ERROR: Tool download failed"
        exit 1
    }

    # Create target-data directory if missing
    TARGET_DATA_DIR="$PROJECT_DIR/target-data"
    if [[ ! -d "$TARGET_DATA_DIR" ]]; then
        mkdir -p "$TARGET_DATA_DIR"
    fi

    # Create empty config.yaml if missing
    CONFIG_FILE="$PROJECT_DIR/config.yaml"
    if [[ ! -f "$CONFIG_FILE" ]]; then
        cat > "$CONFIG_FILE" << 'EOF'
# Star Forge Configuration
# This file defines OS targets and their partitions

# Current active target (set with: sf use <name>)
current_target: null

# Target OS configurations
targets: []
EOF
        echo "Created empty config.yaml"
        echo ""
    fi

    # Mark that we're in the Star Forge shell
    export STAR_FORGE_SHELL=1

    # Prepend bootstrap environment to PATH (Arch tools take priority, host tools as fallback)
    export PATH="$BOOTSTRAP_ROOT/usr/bin:$BOOTSTRAP_ROOT/bin:$TOOLS_DIR/yq:$TOOLS_DIR/pv/bin:$BIN_PATH:$PATH"

    # Source common functions for banner
    source "$SCRIPT_DIR/common.sh"

    print_banner "S T A R   F O R G E" "⚒"

    local term_width=$(tput cols 2>/dev/null || echo 100)
    local tagline="Arch Linux Image Forging Tool"
    local tagline_len=30
    local tagline_padding=$(( (term_width - tagline_len) / 2 ))

    printf "%${tagline_padding}s" ""
    echo -e "\033[2;37m${tagline}\033[0m"
    echo ""
    echo "Available commands:"
    for script in "$PROJECT_DIR/scripts"/sf_*.sh; do
        if [[ -f "$script" ]]; then
            name=$(basename "$script" .sh)
            cmd_name="${name#sf_}"
            echo "  • sf $cmd_name"
        fi
    done
    echo ""
    echo "Run 'sf' without arguments for usage information"
    echo ""

    # Return to original directory and exec into user's shell with custom prompt
    cd "$ORIGINAL_DIR"

    # Detect shell and create appropriate RC file
    USER_SHELL="${SHELL:-bash}"
    TMP_RC=$(mktemp)

    case "$USER_SHELL" in
        */fish)
            # For fish shell - use fish syntax
            cat > "$TMP_RC" << 'FISH_EOF'
# TOS Development Shell prompt functions
set TOS_PROJECT_DIR "__PROJECT_DIR__"

# Disable fish greeting
set -g fish_greeting

# Load sf completions
if test -f "$TOS_PROJECT_DIR/completions/sf.fish"
    source "$TOS_PROJECT_DIR/completions/sf.fish"
end

function tos_get_target
    if test -f "$TOS_PROJECT_DIR/config.yaml"
        set target ("$TOS_PROJECT_DIR/.tools/yq/yq" -r '.current_target' "$TOS_PROJECT_DIR/config.yaml" 2>/dev/null)
        if test "$target" = "null" -o -z "$target"
            echo "none"
        else
            echo "$target"
        end
    else
        echo "none"
    end
end

function tos_is_mounted
    mountpoint -q "$TOS_PROJECT_DIR/mnt" 2>/dev/null
end

function fish_prompt
    set target (tos_get_target)
    if test "$target" != "none"
        if tos_is_mounted
            set_color yellow
            echo -n "@$target "
            set_color green
            echo -n "[mounted] "
        else
            set_color yellow
            echo -n "@$target "
        end
    else if tos_is_mounted
        set_color green
        echo -n "[mounted] "
    end
    set_color blue
    echo -n (prompt_pwd)
    set_color normal
    echo -n ' > '
end
FISH_EOF
            sed -i "s|__PROJECT_DIR__|$PROJECT_DIR|g" "$TMP_RC"
            exec fish --init-command="source $TMP_RC"
            ;;
        */zsh)
            # For zsh - use bash-compatible syntax
            cat > "$TMP_RC" << 'ZSH_EOF'
# TOS Development Shell prompt functions
TOS_PROJECT_DIR="__PROJECT_DIR__"

# Load sf completions
if [[ -f "$TOS_PROJECT_DIR/completions/sf.zsh" ]]; then
    source "$TOS_PROJECT_DIR/completions/sf.zsh"
fi

tos_get_target() {
    if [[ -f "$TOS_PROJECT_DIR/config.yaml" ]]; then
        local target=$("$TOS_PROJECT_DIR/.tools/yq/yq" -r '.current_target' "$TOS_PROJECT_DIR/config.yaml" 2>/dev/null)
        if [[ "$target" == "null" || -z "$target" ]]; then
            echo "none"
        else
            echo "$target"
        fi
    else
        echo "none"
    fi
}

tos_is_mounted() {
    mountpoint -q "$TOS_PROJECT_DIR/mnt" 2>/dev/null
}

tos_prompt_colored() {
    local target=$(tos_get_target)
    local prompt=""
    if [[ "$target" != "none" ]]; then
        if tos_is_mounted; then
            prompt="%F{yellow}@${target}%f %F{green}[mounted]%f %F{blue}%~%f"
        else
            prompt="%F{yellow}@${target}%f %F{blue}%~%f"
        fi
    else
        if tos_is_mounted; then
            prompt="%F{green}[mounted]%f %F{blue}%~%f"
        else
            prompt="%F{blue}%~%f"
        fi
    fi
    echo "$prompt"
}

setopt PROMPT_SUBST
PS1='$(tos_prompt_colored) > '
ZSH_EOF
            sed -i "s|__PROJECT_DIR__|$PROJECT_DIR|g" "$TMP_RC"
            exec zsh --rcs "$TMP_RC"
            ;;
        *)
            # For bash
            cat > "$TMP_RC" << 'BASH_EOF'
# TOS Development Shell prompt functions
TOS_PROJECT_DIR="__PROJECT_DIR__"

# Load sf completions
if [[ -f "$TOS_PROJECT_DIR/completions/sf.bash" ]]; then
    source "$TOS_PROJECT_DIR/completions/sf.bash"
fi

tos_get_target() {
    if [[ -f "$TOS_PROJECT_DIR/config.yaml" ]]; then
        local target=$("$TOS_PROJECT_DIR/.tools/yq/yq" -r '.current_target' "$TOS_PROJECT_DIR/config.yaml" 2>/dev/null)
        if [[ "$target" == "null" || -z "$target" ]]; then
            echo "none"
        else
            echo "$target"
        fi
    else
        echo "none"
    fi
}

tos_is_mounted() {
    mountpoint -q "$TOS_PROJECT_DIR/mnt" 2>/dev/null
}

tos_prompt_colored() {
    local target=$(tos_get_target)
    if [[ "$target" != "none" ]]; then
        if tos_is_mounted; then
            echo -e "\033[1;33m@${target}\033[0m \033[1;32m[mounted]\033[0m \033[1;34m\w\033[0m"
        else
            echo -e "\033[1;33m@${target}\033[0m \033[1;34m\w\033[0m"
        fi
    else
        if tos_is_mounted; then
            echo -e "\033[1;32m[mounted]\033[0m \033[1;34m\w\033[0m"
        else
            echo -e "\033[1;34m\w\033[0m"
        fi
    fi
}

PS1='\[$(tos_prompt_colored)\] > '
BASH_EOF
            sed -i "s|__PROJECT_DIR__|$PROJECT_DIR|g" "$TMP_RC"
            exec bash --rcfile "$TMP_RC"
            ;;
    esac
}
