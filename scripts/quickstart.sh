#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
readme="$repo_root/README.md"

if [ ! -f "$readme" ]; then
	echo "quickstart.sh: README.md not found at $readme" >&2
	exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

stage="$tmp/stage"
work="$tmp/work"
mkdir -p "$stage" "$work"

# Extract every <!-- quickstart:file <path> --> fenced block from README.md.
awk -v out="$stage" '
/^<!-- quickstart:file [^ ]+ -->$/ {
	target = $3
	sub(/-->/, "", target)
	want_fence = 1
	next
}
want_fence && /^```/ {
	want_fence = 0
	in_block = 1
	path = out "/" target
	cmd = "mkdir -p \"$(dirname \"" path "\")\""
	system(cmd)
	close(cmd)
	printf "" > path
	next
}
in_block && /^```$/ {
	close(path)
	in_block = 0
	count++
	next
}
in_block {
	print > path
}
END {
	if (in_block || want_fence) {
		print "quickstart.sh: unterminated quickstart block in README.md" > "/dev/stderr"
		exit 1
	}
	if (count < 4) {
		print "quickstart.sh: expected at least 4 quickstart files, got " count > "/dev/stderr"
		exit 1
	}
}
' "$readme"

for req in tools.go genx.yaml internal/admin/admin.go main.go; do
	if [ ! -f "$stage/$req" ]; then
		echo "quickstart.sh: missing extracted file $req" >&2
		exit 1
	fi
done

cat > "$work/go.mod" <<EOF
module example.com/quickstart

go 1.26.0

replace (
	github.com/epistemer/handlers => $repo_root
	github.com/epistemer/handlers/authguard => $repo_root/authguard
	github.com/epistemer/handlers/cmd => $repo_root/cmd
	github.com/epistemer/handlers/openapi => $repo_root/openapi
)

tool (
	github.com/epistemer/genx/cmd/genx
	github.com/epistemer/handlers/cmd/handlers
)

require (
	github.com/epistemer/handlers v0.0.0-00010101000000-000000000000
	github.com/epistemer/handlers/cmd v0.0.0-00010101000000-000000000000
)
EOF

mkdir -p "$work/internal/admin"
cp "$stage/tools.go" "$work/tools.go"
cp "$stage/genx.yaml" "$work/genx.yaml"
cp "$stage/internal/admin/admin.go" "$work/internal/admin/admin.go"

cd "$work"

go mod tidy
go tool genx

cp "$stage/main.go" "$work/main.go"

go mod tidy
go tool genx check
go tool handlers vet -strict ./...
go vet ./...
go build ./...
