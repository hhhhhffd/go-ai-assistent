#!/usr/bin/env bash

set -euo pipefail

readonly app_name="go-ai-assistant"
readonly remote_app_dir="/opt/${app_name}"
readonly remote_state_dir="/var/lib/${app_name}"
readonly remote_binary="/usr/local/bin/${app_name}"
readonly remote_unit="/etc/systemd/system/${app_name}.service"

target="${1:?Usage: ./scripts/deploy.sh user@host [amd64]}"
architecture="${2:-amd64}"
sync_state="${SYNC_STATE:-0}"

if [[ ! "$target" =~ ^[a-zA-Z0-9._-]+@[a-zA-Z0-9._:-]+$ ]]; then
	echo "Target must have the form user@host." >&2
	exit 1
fi
if [[ "$architecture" != "amd64" ]]; then
	echo "Only amd64 is supported by this deployment script." >&2
	exit 1
fi
if [[ "$sync_state" != "0" && "$sync_state" != "1" ]]; then
	echo "SYNC_STATE must be 0 or 1." >&2
	exit 1
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
env_file="${project_dir}/.env"
unit_file="${project_dir}/deploy/systemd/${app_name}.service"
artifact="${project_dir}/bin/${app_name}-linux-${architecture}"

if [[ ! -f "$env_file" ]]; then
	echo "Missing ${env_file}. Copy .env.example and fill the bootstrap credentials first." >&2
	exit 1
fi

config_dir="${CONFIG_DIR:-}"
if [[ -z "$config_dir" ]]; then
	while IFS= read -r line || [[ -n "$line" ]]; do
		if [[ "$line" == CONFIG_DIR=* ]]; then
			config_dir="${line#CONFIG_DIR=}"
			config_dir="${config_dir%\"}"
			config_dir="${config_dir#\"}"
			break
		fi
	done < "$env_file"
fi
config_dir="${config_dir:-.}"
if [[ "$config_dir" != /* ]]; then
	config_dir="${project_dir}/${config_dir}"
fi

config_file="${config_dir}/config.yaml"
providers_file="${config_dir}/providers.yaml"
for required_file in "$config_file" "$providers_file" "$unit_file"; do
	if [[ ! -f "$required_file" ]]; then
		echo "Missing ${required_file}. Run the bot locally once before deploying." >&2
		exit 1
	fi
done

mkdir -p "${project_dir}/bin"
(
	cd "$project_dir"
	GOOS=linux GOARCH="$architecture" CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$artifact" ./cmd/bot
)

remote_stage="/tmp/${app_name}-deploy-$$"
ssh "$target" "mkdir -p '$remote_stage'"
scp "$artifact" "$env_file" "$unit_file" "$config_file" "$providers_file" "$target:$remote_stage/"

ssh "$target" bash -s -- \
	"$remote_stage" \
	"$remote_app_dir" \
	"$remote_state_dir" \
	"$remote_binary" \
	"$remote_unit" \
	"$app_name" \
	"$architecture" \
	"$sync_state" <<'REMOTE_SCRIPT'
set -euo pipefail

remote_stage="$1"
remote_app_dir="$2"
remote_state_dir="$3"
remote_binary="$4"
remote_unit="$5"
app_name="$6"
architecture="$7"
sync_state="$8"

if [ "$(id -u)" -eq 0 ]; then
	sudo_command=()
else
	sudo_command=(sudo)
fi

if ! id -u bot >/dev/null 2>&1; then
	"${sudo_command[@]}" useradd --system --user-group --home-dir /nonexistent --shell /usr/sbin/nologin bot
fi
"${sudo_command[@]}" install -d -o bot -g bot -m 0750 "$remote_app_dir" "$remote_state_dir"
"${sudo_command[@]}" install -o root -g root -m 0755 "$remote_stage/${app_name}-linux-${architecture}" "$remote_binary"
"${sudo_command[@]}" install -o root -g root -m 0600 "$remote_stage/.env" "$remote_app_dir/.env"
"${sudo_command[@]}" sed -i '/^[[:space:]]*CONFIG_DIR=/d' "$remote_app_dir/.env"
if [ "$sync_state" = 1 ] || [ ! -f "$remote_state_dir/config.yaml" ]; then
	"${sudo_command[@]}" install -o bot -g bot -m 0600 "$remote_stage/config.yaml" "$remote_state_dir/config.yaml"
fi
if [ "$sync_state" = 1 ] || [ ! -f "$remote_state_dir/providers.yaml" ]; then
	"${sudo_command[@]}" install -o bot -g bot -m 0600 "$remote_stage/providers.yaml" "$remote_state_dir/providers.yaml"
fi
"${sudo_command[@]}" install -o root -g root -m 0644 "$remote_stage/${app_name}.service" "$remote_unit"
rm -rf -- "$remote_stage"
"${sudo_command[@]}" systemctl daemon-reload
"${sudo_command[@]}" systemctl enable --now "$app_name"
"${sudo_command[@]}" systemctl --no-pager --full status "$app_name"
REMOTE_SCRIPT
