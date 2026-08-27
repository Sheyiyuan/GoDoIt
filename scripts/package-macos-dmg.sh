#!/bin/sh
set -eu

version=$1
app=$2
output=$3
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT HUP INT TERM
mkdir -p "$stage/GoDoIt"
cp -R "$app" "$stage/GoDoIt/GoDoIt.app"
ln -s /Applications "$stage/GoDoIt/Applications"
mkdir -p "$output"
hdiutil create -volname GoDoIt -srcfolder "$stage/GoDoIt" -format UDZO -ov "$output/GoDoIt_${version}_darwin_arm64.dmg" >/dev/null
