#!/usr/bin/env bash
set -euo pipefail

# Find the latest semver tag.
latest=$(git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1 || true)

if [[ -z "$latest" ]]; then
    echo "No existing semver tags found."
    suggested="v0.1.0"
else
    echo "Latest tag: $latest"
    major=$(echo "$latest" | cut -d. -f1 | tr -d 'v')
    minor=$(echo "$latest" | cut -d. -f2)
    patch=$(echo "$latest" | cut -d. -f3)
    suggested="v${major}.${minor}.$((patch + 1))"
fi

read -rp "New tag [$suggested]: " input
tag="${input:-$suggested}"

if ! [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: tag must match vX.Y.Z (got: $tag)" >&2
    exit 1
fi

if git tag | grep -qx "$tag"; then
    echo "Error: tag $tag already exists" >&2
    exit 1
fi

echo ""
echo "Will run:"
echo "  git tag $tag"
echo "  git push origin $tag"
echo ""
read -rp "Proceed? [y/N] " confirm
if [[ "${confirm,,}" != "y" ]]; then
    echo "Aborted."
    exit 0
fi

git tag "$tag"
git push origin "$tag"

echo ""
echo "Tagged and pushed $tag. Waiting for release workflow to start..."

run_id=""
for i in $(seq 1 30); do
    run_id=$(gh run list \
        --workflow release.yml \
        --branch "$tag" \
        --limit 1 \
        --json databaseId \
        --jq '.[0].databaseId' 2>/dev/null || true)
    if [[ -n "$run_id" && "$run_id" != "null" ]]; then
        break
    fi
    sleep 2
done

if [[ -z "$run_id" || "$run_id" == "null" ]]; then
    echo "Could not find workflow run — check manually:"
    echo "  https://github.com/tadamo/podres/actions"
    exit 1
fi

echo "Watching run $run_id..."
echo ""
gh run watch "$run_id" --exit-status
