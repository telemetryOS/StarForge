#!/bin/bash
# Bash completion for sf (Star Forge)

_sf_completion() {
    local cur prev words cword
    _init_completion || return

    # Get the sf commands dynamically
    local commands=$(ls -1 "$(dirname "$(which sf)")/../scripts/sf_"*.sh 2>/dev/null | xargs -n1 basename | sed 's/^sf_//;s/.sh$//' | sort)

    # Complete the first argument with available commands
    if [[ $cword -eq 1 ]]; then
        COMPREPLY=($(compgen -W "$commands" -- "$cur"))
        return
    fi

    # Command-specific completions
    local command="${words[1]}"
    local config_file="$(dirname "$(which sf)")/../config.yaml"

    case "$command" in
        write-installer)
            # Complete with block devices
            if [[ $cword -eq 2 ]]; then
                local devices=$(lsblk -ndo NAME 2>/dev/null | sed 's/^/\/dev\//')
                COMPREPLY=($(compgen -W "$devices" -- "$cur"))
            fi
            ;;
        use|delete|run|run-serial)
            # Complete with target names from config.yaml
            if [[ $cword -eq 2 ]]; then
                if [[ -f "$config_file" ]] && command -v yq >/dev/null 2>&1; then
                    local targets=$(yq -r '.targets[].name' "$config_file" 2>/dev/null)
                    COMPREPLY=($(compgen -W "$targets" -- "$cur"))
                fi
            fi
            ;;
        export)
            # First argument: target name, Second argument: output path
            if [[ $cword -eq 2 ]]; then
                if [[ -f "$config_file" ]] && command -v yq >/dev/null 2>&1; then
                    local targets=$(yq -r '.targets[].name' "$config_file" 2>/dev/null)
                    COMPREPLY=($(compgen -W "$targets" -- "$cur"))
                fi
            elif [[ $cword -eq 3 ]]; then
                COMPREPLY=($(compgen -f -- "$cur"))
            fi
            ;;
        import)
            # Complete with .sftar files
            COMPREPLY=($(compgen -f -X '!*.sftar' -- "$cur"))
            ;;
        rename)
            # First argument: old target name, Second argument: new target name
            if [[ $cword -eq 2 ]] || [[ $cword -eq 3 ]]; then
                if [[ -f "$config_file" ]] && command -v yq >/dev/null 2>&1; then
                    local targets=$(yq -r '.targets[].name' "$config_file" 2>/dev/null)
                    COMPREPLY=($(compgen -W "$targets" -- "$cur"))
                fi
            fi
            ;;
        clone)
            # First argument: source target
            if [[ $cword -eq 2 ]]; then
                if [[ -f "$config_file" ]] && command -v yq >/dev/null 2>&1; then
                    local targets=$(yq -r '.targets[].name' "$config_file" 2>/dev/null)
                    COMPREPLY=($(compgen -W "$targets" -- "$cur"))
                fi
            fi
            # Second argument: new target name (no completion)
            # Third argument: description (no completion)
            ;;
        create)
            # No arguments - fully interactive
            ;;
        load-installer)
            # First argument: distribution target, Second argument: installer target
            if [[ $cword -eq 2 ]] || [[ $cword -eq 3 ]]; then
                if [[ -f "$config_file" ]] && command -v yq >/dev/null 2>&1; then
                    local targets=$(yq -r '.targets[].name' "$config_file" 2>/dev/null)
                    COMPREPLY=($(compgen -W "$targets" -- "$cur"))
                fi
            fi
            ;;
        resize-partition-image)
            # First argument: partition name or image file
            if [[ $cword -eq 2 ]]; then
                # Offer partition names from current target
                if [[ -f "$config_file" ]] && command -v yq >/dev/null 2>&1; then
                    local current_target=$(yq -r '.current_target' "$config_file" 2>/dev/null)
                    if [[ -n "$current_target" && "$current_target" != "null" ]]; then
                        local partitions=$(yq -r ".targets[] | select(.name == \"$current_target\") | .partitions[].name" "$config_file" 2>/dev/null)
                        COMPREPLY=($(compgen -W "$partitions" -- "$cur"))
                        # Also allow file path completion
                        COMPREPLY+=($(compgen -f -- "$cur"))
                    else
                        COMPREPLY=($(compgen -f -- "$cur"))
                    fi
                else
                    COMPREPLY=($(compgen -f -- "$cur"))
                fi
            fi
            # Second argument: new size (no completion)
            ;;
        mount|unmount|chroot|status|list)
            # No additional arguments
            ;;
    esac
}

complete -F _sf_completion sf
