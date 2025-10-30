#!/bin/bash

# Star Forge
# Usage: sf list
# Lists all available targets

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

check_config

echo -e "${BLUE}Available Targets:${NC}"
list_targets
