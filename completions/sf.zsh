#compdef sf
# Zsh completion for sf (Star Forge)

_sf() {
    local line state

    # Get the project directory and config file
    local sf_bin="${commands[sf]}"
    local project_dir="${sf_bin:h:h}"
    local scripts_dir="$project_dir/scripts"
    local config_file="$project_dir/config.yaml"

    # Get available commands dynamically
    local -a commands
    commands=(${${(f)"$(ls -1 $scripts_dir/sf_*.sh 2>/dev/null)"}##*/sf_})
    commands=(${commands%.sh})

    _arguments -C \
        '1: :->command' \
        '*::arg:->args'

    case $state in
        command)
            _describe -t commands 'sf command' commands
            ;;
        args)
            local command="$line[1]"
            case $command in
                write-installer)
                    _arguments '1:device:_files -g "/dev/*"'
                    ;;
                use|delete|run|run-graphical)
                    local -a targets
                    if [[ -f "$config_file" ]] && (( $+commands[yq] )); then
                        targets=(${(f)"$(yq -r '.targets[].name' "$config_file" 2>/dev/null)"})
                    fi
                    _arguments "1:target:($targets)"
                    ;;
                export)
                    local -a targets
                    if [[ -f "$config_file" ]] && (( $+commands[yq] )); then
                        targets=(${(f)"$(yq -r '.targets[].name' "$config_file" 2>/dev/null)"})
                    fi
                    _arguments \
                        "1:target:($targets)" \
                        '2:output path:_files'
                    ;;
                import)
                    _arguments '1:sftar file:_files -g "*.sftar"'
                    ;;
                rename)
                    local -a targets
                    if [[ -f "$config_file" ]] && (( $+commands[yq] )); then
                        targets=(${(f)"$(yq -r '.targets[].name' "$config_file" 2>/dev/null)"})
                    fi
                    _arguments \
                        "1:old target:($targets)" \
                        "2:new target:($targets)"
                    ;;
                clone)
                    local -a targets
                    if [[ -f "$config_file" ]] && (( $+commands[yq] )); then
                        targets=(${(f)"$(yq -r '.targets[].name' "$config_file" 2>/dev/null)"})
                    fi
                    _arguments \
                        "1:source target:($targets)" \
                        '2:new target name:' \
                        '3:description:'
                    ;;
                create)
                    # No arguments - fully interactive
                    ;;
                load-installer)
                    local -a targets
                    if [[ -f "$config_file" ]] && (( $+commands[yq] )); then
                        targets=(${(f)"$(yq -r '.targets[].name' "$config_file" 2>/dev/null)"})
                    fi
                    _arguments \
                        "1:distribution target:($targets)" \
                        "2:installer target:($targets)"
                    ;;
                resize-partition-image)
                    local -a partitions
                    if [[ -f "$config_file" ]] && (( $+commands[yq] )); then
                        local current_target=$(yq -r '.current_target' "$config_file" 2>/dev/null)
                        if [[ -n "$current_target" && "$current_target" != "null" ]]; then
                            partitions=(${(f)"$(yq -r ".targets[] | select(.name == \"$current_target\") | .partitions[].name" "$config_file" 2>/dev/null)"})
                        fi
                    fi
                    _arguments \
                        "1:partition or file:($partitions):_files" \
                        '2:new size:'
                    ;;
                mount|unmount|chroot|status|list)
                    # No additional arguments
                    ;;
            esac
            ;;
    esac
}

_sf "$@"
