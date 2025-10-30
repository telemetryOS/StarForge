# Fish completion for sf (Star Forge)

# Helper function to get available sf commands
function __sf_commands
    set -l sf_bin (which sf)
    set -l project_dir (dirname (dirname $sf_bin))
    set -l scripts_dir $project_dir/scripts

    if test -d $scripts_dir
        for script in $scripts_dir/sf_*.sh
            set -l cmd (basename $script .sh | string replace 'sf_' '')
            echo $cmd
        end
    end
end

# Helper function to get target names from config
function __sf_targets
    set -l sf_bin (which sf)
    set -l project_dir (dirname (dirname $sf_bin))
    set -l config_file $project_dir/config.yaml

    if test -f $config_file; and command -v yq >/dev/null 2>&1
        yq -r '.targets[].name' $config_file 2>/dev/null
    end
end

# Helper function to get block devices
function __sf_devices
    lsblk -ndo NAME 2>/dev/null | string replace -r '^' '/dev/'
end

# Helper function to get partition names from current target
function __sf_partitions
    set -l sf_bin (which sf)
    set -l project_dir (dirname (dirname $sf_bin))
    set -l config_file $project_dir/config.yaml

    if test -f $config_file; and command -v yq >/dev/null 2>&1
        set -l current_target (yq -r '.current_target' $config_file 2>/dev/null)
        if test -n "$current_target"; and test "$current_target" != "null"
            yq -r ".targets[] | select(.name == \"$current_target\") | .partitions[].name" $config_file 2>/dev/null
        end
    end
end

# Clear existing completions
complete -c sf -e

# Main command completions
complete -c sf -f -n '__fish_use_subcommand' -a '(__sf_commands)'

# Subcommand-specific completions

# write-installer: complete with block devices
complete -c sf -f -n '__fish_seen_subcommand_from write-installer' -a '(__sf_devices)'

# use, delete, run, run-serial: complete with target names
complete -c sf -f -n '__fish_seen_subcommand_from use delete run run-serial' -a '(__sf_targets)'

# export: first arg is target name, second arg is output path
complete -c sf -f -n '__fish_seen_subcommand_from export; and test (count (commandline -opc)) -eq 2' -a '(__sf_targets)'
complete -c sf -r -n '__fish_seen_subcommand_from export; and test (count (commandline -opc)) -eq 3'

# import: complete with .sftar files
complete -c sf -r -n '__fish_seen_subcommand_from import; and test (count (commandline -opc)) -eq 2' -a '(ls *.sftar 2>/dev/null)'

# rename: both args complete with target names
complete -c sf -f -n '__fish_seen_subcommand_from rename; and test (count (commandline -opc)) -eq 2' -a '(__sf_targets)'
complete -c sf -f -n '__fish_seen_subcommand_from rename; and test (count (commandline -opc)) -eq 3' -a '(__sf_targets)'

# clone: first arg is source target
complete -c sf -f -n '__fish_seen_subcommand_from clone; and test (count (commandline -opc)) -eq 2' -a '(__sf_targets)'

# create: no arguments - fully interactive
complete -c sf -f -n '__fish_seen_subcommand_from create'

# load-installer: both args complete with target names
complete -c sf -f -n '__fish_seen_subcommand_from load-installer; and test (count (commandline -opc)) -eq 2' -a '(__sf_targets)'
complete -c sf -f -n '__fish_seen_subcommand_from load-installer; and test (count (commandline -opc)) -eq 3' -a '(__sf_targets)'

# resize-partition-image: first arg is partition name
complete -c sf -f -n '__fish_seen_subcommand_from resize-partition-image; and test (count (commandline -opc)) -eq 2' -a '(__sf_partitions)'

# Commands with no additional arguments
complete -c sf -f -n '__fish_seen_subcommand_from mount unmount chroot status list'

# Command descriptions
complete -c sf -f -n '__fish_use_subcommand' -a 'mount' -d 'Mount the current target'
complete -c sf -f -n '__fish_use_subcommand' -a 'unmount' -d 'Unmount the current target'
complete -c sf -f -n '__fish_use_subcommand' -a 'chroot' -d 'Chroot into the current target'
complete -c sf -f -n '__fish_use_subcommand' -a 'status' -d 'Show mount status and target info'
complete -c sf -f -n '__fish_use_subcommand' -a 'list' -d 'List all available targets'
complete -c sf -f -n '__fish_use_subcommand' -a 'use' -d 'Set the active target'
complete -c sf -f -n '__fish_use_subcommand' -a 'create' -d 'Create a new Arch Linux target'
complete -c sf -f -n '__fish_use_subcommand' -a 'clone' -d 'Clone an existing target'
complete -c sf -f -n '__fish_use_subcommand' -a 'delete' -d 'Delete a target'
complete -c sf -f -n '__fish_use_subcommand' -a 'rename' -d 'Rename a target'
complete -c sf -f -n '__fish_use_subcommand' -a 'export' -d 'Export target to .sftar file'
complete -c sf -f -n '__fish_use_subcommand' -a 'import' -d 'Import target from .sftar file'
complete -c sf -f -n '__fish_use_subcommand' -a 'write-installer' -d 'Write installer to USB drive'
complete -c sf -f -n '__fish_use_subcommand' -a 'load-installer' -d 'Load installer images'
complete -c sf -f -n '__fish_use_subcommand' -a 'resize-partition-image' -d 'Resize a partition image'
complete -c sf -f -n '__fish_use_subcommand' -a 'run' -d 'Run a target in QEMU (graphical mode)'
complete -c sf -f -n '__fish_use_subcommand' -a 'run-serial' -d 'Run a target in QEMU (serial console mode)'
