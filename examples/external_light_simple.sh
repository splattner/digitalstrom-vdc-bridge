#!/usr/bin/env bash
set -euo pipefail

host="${1:-127.0.0.1}"
port="${2:-8999}"
name="${3:-vdcgo sample light}"
uid="${4:-vdcgo-sample-light-001}"
mode="${5:-follow}"

# Open persistent TCP connection on FD 3.
exec 3<>"/dev/tcp/${host}/${port}"

# Register one dimmer/light output device via external device API init message.
printf '{"message":"init","protocol":"simple","output":"light","name":"%s","uniqueid":"%s"}\n' "$name" "$uid" >&3

# One-shot mode only registers the device and exits.
if [[ "$mode" == "oneshot" ]]; then
	exit 0
fi

# Keep connection open and print channel updates (for example C0=100).
cat <&3
