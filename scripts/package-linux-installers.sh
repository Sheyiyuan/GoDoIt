#!/bin/sh
set -eu

version=$1
root=$2
output=$3
name="godoit"
package_version=$(printf '%s' "$version" | sed 's/-dev\./~dev./')
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT HUP INT TERM

mkdir -p "$stage/deb/DEBIAN" "$stage/deb/usr/bin" "$stage/deb/usr/share/applications" "$stage/deb/usr/share/doc/$name"
cp "$root/bin/gdit" "$stage/deb/usr/bin/gdit"
cp "$root/project/gui/build/bin/gdit-gui" "$stage/deb/usr/bin/gdit-gui"
cp LICENSE THIRD_PARTY_NOTICES.txt "$stage/deb/usr/share/doc/$name/"
chmod 755 "$stage/deb/usr/bin/gdit" "$stage/deb/usr/bin/gdit-gui"
cat > "$stage/deb/DEBIAN/control" <<EOF
Package: $name
Version: $package_version
Section: devel
Priority: optional
Architecture: amd64
Maintainer: Sheyiyuan <dev@sheyiyuan.com>
Description: GoDoIt Godot engine launcher and version manager
EOF
cat > "$stage/deb/usr/share/applications/godoit.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=GoDoIt
Exec=/usr/bin/gdit-gui
Terminal=false
Categories=Development;Utility;
EOF
mkdir -p "$output"
dpkg-deb --build --root-owner-group "$stage/deb" "$output/GoDoIt_${version}_linux_amd64.deb" >/dev/null

mkdir -p "$stage/rpm/BUILD" "$stage/rpm/RPMS" "$stage/rpm/SOURCES" "$stage/rpm/SPECS" "$stage/rpm/SRPMS"
mkdir -p "$stage/rpm/root/usr/bin" "$stage/rpm/root/usr/share/applications" "$stage/rpm/root/usr/share/doc/$name"
cp "$root/bin/gdit" "$stage/rpm/root/usr/bin/gdit"
cp "$root/project/gui/build/bin/gdit-gui" "$stage/rpm/root/usr/bin/gdit-gui"
cp LICENSE THIRD_PARTY_NOTICES.txt "$stage/rpm/root/usr/share/doc/$name/"
cp "$stage/deb/usr/share/applications/godoit.desktop" "$stage/rpm/root/usr/share/applications/"
chmod 755 "$stage/rpm/root/usr/bin/gdit" "$stage/rpm/root/usr/bin/gdit-gui"
release=1
cat > "$stage/rpm/SPECS/godoit.spec" <<EOF
Name: $name
Version: $package_version
Release: $release
Summary: GoDoIt Godot engine launcher and version manager
License: AGPL-3.0-or-later
BuildArch: x86_64

%description
GoDoIt Godot engine launcher and version manager.

%install
mkdir -p %{buildroot}
cp -a $stage/rpm/root/. %{buildroot}/

%files
/usr/bin/gdit
/usr/bin/gdit-gui
/usr/share/applications/godoit.desktop
/usr/share/doc/godoit/LICENSE
/usr/share/doc/godoit/THIRD_PARTY_NOTICES.txt
EOF
rpmbuild --define "_topdir $stage/rpm" -bb "$stage/rpm/SPECS/godoit.spec" >/dev/null
cp "$stage/rpm/RPMS/x86_64/"*.rpm "$output/GoDoIt_${version}_linux_amd64.rpm"
