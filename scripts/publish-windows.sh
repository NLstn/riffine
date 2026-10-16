#!/usr/bin/env bash
# Called only after Windows installer acceptance. Main runs are serialized by
# the workflow concurrency group so an older build cannot replace a newer one.
set -euo pipefail

: "${GITHUB_REPOSITORY:?}"
: "${GITHUB_SHA:?}"
: "${GITHUB_REF:?}"
: "${GITHUB_EVENT_NAME:?}"
: "${GITHUB_RUN_ID:?}"
: "${RUNNER_TEMP:?}"

case "$GITHUB_EVENT_NAME:$GITHUB_REF" in
  push:refs/heads/main) channel=nightly ;;
  push:refs/tags/windows-v*) channel=versioned ;;
  *) echo 'Publishing requires a main push or an explicit Windows version tag.' >&2; exit 1 ;;
esac

repository_root=$(pwd)
cd "${1:?installer directory required}"
# PowerShell writes CRLF; normalize the checksum manifest for GNU coreutils.
sed -i 's/\r$//' SHA256SUMS.txt
# Check the downloaded bytes before publishing either channel.
sha256sum --check SHA256SUMS.txt --strict
shopt -s nullglob
installers=(Riffine-*-windows-x64-setup.exe)
if [[ ${#installers[@]} -ne 1 ]]; then
  echo 'Expected exactly one Windows installer.' >&2
  exit 1
fi

notes="$RUNNER_TEMP/windows-release-notes.md"
if [[ "$channel" == nightly ]]; then
  tag=windows-nightly
  title='Windows Nightly'
  # A stable asset name gives users a permanent download URL.
  mv "${installers[0]}" Riffine-nightly-windows-x64-setup.exe
  sha256sum Riffine-nightly-windows-x64-setup.exe > SHA256SUMS.txt
  assets=(Riffine-nightly-windows-x64-setup.exe SHA256SUMS.txt)
  {
    printf 'Latest successful Windows build from a relevant merge to main. Replaced on each successful build.\n\n'
    printf 'Source: [%s](https://github.com/%s/commit/%s)\n\n' "$GITHUB_SHA" "$GITHUB_REPOSITORY" "$GITHUB_SHA"
    printf 'Build: [GitHub Actions](https://github.com/%s/actions/runs/%s)\n\n' "$GITHUB_REPOSITORY" "$GITHUB_RUN_ID"
    cat "$repository_root/desktop/windows/release-notes.md"
  } > "$notes"
  # Listing releases propagates API failures rather than treating them as a
  # missing release. Only this rolling nightly tag is ever moved or clobbered.
  release_id=$(gh api --paginate "repos/$GITHUB_REPOSITORY/releases" --jq '.[] | select(.tag_name == "windows-nightly") | .id')
  if [[ -n "$release_id" ]]; then
    gh api --method PATCH "repos/$GITHUB_REPOSITORY/git/refs/tags/$tag" -f sha="$GITHUB_SHA" -F force=true > /dev/null
    gh release upload "$tag" "${assets[@]}" --clobber --repo "$GITHUB_REPOSITORY"
    gh release edit "$tag" --prerelease --latest=false --title "$title" --notes-file "$notes" --repo "$GITHUB_REPOSITORY"
  else
    gh release create "$tag" "${assets[@]}" --target "$GITHUB_SHA" --prerelease --latest=false --title "$title" --notes-file "$notes" --repo "$GITHUB_REPOSITORY"
  fi
else
  tag=${GITHUB_REF#refs/tags/}
  version=${tag#windows-v}
  if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ || "${installers[0]}" != "Riffine-$version-windows-x64-setup.exe" ]]; then
    echo 'Version tag and installer filename must agree (windows-vMAJOR.MINOR.PATCH).' >&2
    exit 1
  fi
  # Alpha releases remain prereleases until the supported-release gates pass.
  # An existing version is never replaced by a rerun.
  gh release create "$tag" "${installers[0]}" SHA256SUMS.txt --verify-tag --prerelease --latest=false --title "Riffine Windows $version" --notes-file "$repository_root/desktop/windows/release-notes.md" --repo "$GITHUB_REPOSITORY"
fi

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  printf '[Windows downloads](https://github.com/%s/releases/tag/%s)\n' "$GITHUB_REPOSITORY" "$tag" >> "$GITHUB_STEP_SUMMARY"
fi
