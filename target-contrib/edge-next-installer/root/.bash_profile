# Auto-launch TelemetryOS installer on first console (tty1 or ttyS0 for serial)
if [[ "$(tty)" == "/dev/tty1" || "$(tty)" == "/dev/ttyS0" ]] && [[ -f /images/partitions.yaml ]]; then
    exec /install.sh
fi
