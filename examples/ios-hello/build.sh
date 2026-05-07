#!/bin/bash
set -euo pipefail

# Builds examples/ios-hello as an iOS Simulator .app and packages it as an .ipa
# ready for `ai-qa-agent-cli session create --upload`.
# Output IPA path is printed on the last line of stdout.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="${1:-/tmp}"
APP_NAME="HelloApp"
APP_BUNDLE="$OUT_DIR/$APP_NAME.app"
IPA_PATH="$OUT_DIR/$APP_NAME.ipa"

if ! xcrun --sdk iphonesimulator --show-sdk-path >/dev/null 2>&1; then
    XCODE_APP="$(ls -d /Applications/Xcode*.app 2>/dev/null | head -1)"
    if [[ -z "$XCODE_APP" ]]; then
        echo "error: iphonesimulator SDK not found and no /Applications/Xcode*.app present" >&2
        exit 1
    fi
    export DEVELOPER_DIR="$XCODE_APP/Contents/Developer"
fi

SDK_PATH="$(xcrun --sdk iphonesimulator --show-sdk-path)"
SDK_VERSION="$(xcrun --sdk iphonesimulator --show-sdk-version)"
# arm64 simulator works on Apple Silicon hosts and matches g2.mac.m2pro RDE templates.
TARGET="arm64-apple-ios15.0-simulator"

rm -rf "$APP_BUNDLE" "$IPA_PATH"
mkdir -p "$APP_BUNDLE"

xcrun -sdk iphonesimulator swiftc \
    -target "$TARGET" \
    -sdk "$SDK_PATH" \
    -parse-as-library \
    -emit-executable \
    -o "$APP_BUNDLE/$APP_NAME" \
    "$SCRIPT_DIR/Sources/"*.swift

sed "s/__SDK_VERSION__/${SDK_VERSION}/g" "$SCRIPT_DIR/Info.plist" > "$APP_BUNDLE/Info.plist"

STAGE_DIR="$(mktemp -d)"
mkdir -p "$STAGE_DIR/Payload"
cp -R "$APP_BUNDLE" "$STAGE_DIR/Payload/"
( cd "$STAGE_DIR" && zip -qr "$IPA_PATH" Payload )
rm -rf "$STAGE_DIR"

echo "$IPA_PATH"
