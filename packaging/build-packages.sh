#!/usr/bin/env bash
# Build the .deb and .rpm packages from the binaries in dist-bin/ (release.yml makes them).
#   usage: packaging/build-packages.sh <version>   (run from the repository root; needs nfpm)
set -euo pipefail
version=${1:?usage: build-packages.sh <version>}
version=${version#v}
stage=dist-bin/stage
trap 'rm -rf "$stage"' EXIT
for arch in amd64 arm64; do
  # nfpm does not expand variables in file paths: stage this architecture's binaries.
  mkdir -p "$stage"
  for pkg in pulse pulse-agent pulse-cli; do
    install -m 0755 "dist-bin/${pkg}_linux_$arch" "$stage/$pkg"
  done
  for pkg in pulse pulse-agent pulse-cli; do
    for packager in deb rpm; do
      VERSION=$version ARCH=$arch nfpm package --config "packaging/nfpm-$pkg.yaml" \
        --packager "$packager" --target dist-bin/
    done
  done
done
ls -1 dist-bin/*.deb dist-bin/*.rpm
