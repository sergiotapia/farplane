#!/usr/bin/env bash
# Keep coverprofile lines for non-test .go files with substantive (non-whitespace)
# changes since a git ref. Formatting-only edits do not enter the patch gate.
set -euo pipefail

profile="${1:?coverage profile required}"
git_base="${2:?git base ref required}"
out="${3:?output profile required}"
repo_root="${4:?repo root required}"

if [[ ! -f "$profile" ]]; then
	echo "cover profile not found: $profile" >&2
	exit 1
fi

mapfile -t candidates < <(
	git -C "$repo_root" diff --name-only --diff-filter=ACMR "$git_base" -- \
		'farplane-backend/**/*.go' \
		| grep -v '_test\.go$' \
		| grep -v '/features/' \
		|| true
)

changed=()
for path in "${candidates[@]}"; do
	# Skip pure whitespace / blank-line formatting churn from gofumpt/wsl.
	if git -C "$repo_root" diff -w --ignore-blank-lines --quiet "$git_base" -- "$path"; then
		continue
	fi
	changed+=("${path#farplane-backend/}")
done

{
	head -n 1 "$profile"
	if ((${#changed[@]} == 0)); then
		# No substantive production Go changes — empty profile (gate passes).
		exit 0
	fi
	# Match coverprofile paths that end with the relative file path.
	while IFS= read -r line; do
		[[ "$line" == mode:* ]] && continue
		for rel in "${changed[@]}"; do
			if [[ "$line" == *"$rel:"* ]]; then
				printf '%s\n' "$line"
				break
			fi
		done
	done <"$profile"
} >"$out"

echo "cover-backend: $(printf '%s\n' "${changed[@]}" | wc -l) substantively changed production Go file(s) vs ${git_base}"
