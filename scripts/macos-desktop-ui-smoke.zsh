#!/bin/zsh

set -euo pipefail

PROCESS_NAME="${CITEBOX_MACOS_PROCESS_NAME:-desktop}"
WINDOW_NAME="${CITEBOX_MACOS_WINDOW_NAME:-CiteBox}"
DOCK_ICON_NAME="${CITEBOX_MACOS_DOCK_ICON_NAME:-desktop}"

usage() {
    cat <<'EOF'
Usage:
  scripts/macos-desktop-ui-smoke.zsh <command>

Commands:
  processes        Print matching Unix and macOS application processes
  windows          Print current app window names and AXMain flags
  tree             Dump the current accessibility tree for the first window
  close-prompt     Click the window close button, then dump the window tree
  to-tray          Choose "最小化到托盘" in the close prompt and print window count
  dock-items       Print current Dock item names
  dock-reopen      Click the Dock icon and print window count
  smoke            Run close button -> minimize to tray -> Dock reopen

Notes:
  1. Run the desktop app first, for example `./desktop`.
  2. Terminal/osascript must already have macOS Accessibility permission.
  3. The current process and Dock icon name are both `desktop`.
EOF
}

processes() {
    echo "== ps =="
    ps -axo pid,comm,args | rg './desktop|/Users/xuzhougeng/Documents/Code/citebox/desktop|desktop$' || true
    echo
    echo "== macOS app processes =="
    osascript -e 'tell application "System Events" to get name of every application process whose name contains "desktop" or name contains "CiteBox"'
}

windows() {
    osascript -e "tell application \"System Events\" to tell process \"$PROCESS_NAME\" to get {name of every window, value of attribute \"AXMain\" of every window}"
}

tree() {
    osascript -e "tell application \"System Events\" to tell process \"$PROCESS_NAME\" to get entire contents of window 1"
}

close_prompt() {
    osascript \
        -e "tell application \"System Events\" to tell process \"$PROCESS_NAME\" to click button 1 of window 1" \
        -e 'delay 0.5' \
        -e "tell application \"System Events\" to tell process \"$PROCESS_NAME\" to get entire contents of window 1"
}

to_tray() {
    osascript \
        -e "tell application \"System Events\" to tell process \"$PROCESS_NAME\" to click button \"最小化到托盘\" of group \"关闭 CiteBox\" of UI element 1 of scroll area 1 of group 1 of group 1 of window \"$WINDOW_NAME\"" \
        -e 'delay 0.5' \
        -e "tell application \"System Events\" to tell process \"$PROCESS_NAME\" to get count of windows"
}

dock_items() {
    osascript -e 'tell application "System Events" to tell process "Dock" to get name of every UI element of list 1'
}

dock_reopen() {
    osascript \
        -e "tell application \"System Events\" to tell process \"Dock\" to click UI element \"$DOCK_ICON_NAME\" of list 1" \
        -e 'delay 1' \
        -e "tell application \"System Events\" to tell process \"$PROCESS_NAME\" to get count of windows"
}

smoke() {
    close_prompt
    to_tray
    dock_reopen
}

main() {
    local command="${1:-}"
    case "$command" in
        processes)
            processes
            ;;
        windows)
            windows
            ;;
        tree)
            tree
            ;;
        close-prompt)
            close_prompt
            ;;
        to-tray)
            to_tray
            ;;
        dock-items)
            dock_items
            ;;
        dock-reopen)
            dock_reopen
            ;;
        smoke)
            smoke
            ;;
        *)
            usage
            exit 1
            ;;
    esac
}

main "$@"
