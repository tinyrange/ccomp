#!/usr/bin/env bash
set -euo pipefail

GOCACHE="${GOCACHE:-$(pwd)/.cache/go-build}"
GOMODCACHE="${GOMODCACHE:-$(pwd)/.cache/gomod}"

# Build compiler
mkdir -p "$GOCACHE" "$GOMODCACHE"
GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go build -o ccomp ./cmd/ccomp

pass=0
fail=0
total=0
tmpdir=$(pwd)/.test-tmp
rm -rf "$tmpdir" && mkdir -p "$tmpdir"
trap 'rm -rf "$tmpdir"' EXIT

for c in tests/*.c; do
  (( total++ ))
  name=$(basename "$c")
  first=$(head -n1 "$c")
  expect_type=$(echo "$first" | awk '{print $2}')
  expect_val=$(echo "$first" | awk '{print $3}')
  s="$tmpdir/${name%.c}.s"
  bin="$tmpdir/${name%.c}.bin"

  if [[ "$expect_type" == "EXIT" ]]; then
    if ! ./ccomp -o "$s" "$c" > "$tmpdir/$name.log" 2>&1; then
      echo "FAIL $name (expected EXIT $expect_val): compile error"
      (( fail++ ))
      continue
    fi
    if ! gcc -nostdlib "$s" runtime/start_linux_amd64.s -o "$bin" >> "$tmpdir/$name.log" 2>&1; then
      echo "FAIL $name (link error)"
      (( fail++ ))
      continue
    fi
    set +e
    "$bin"
    code=$?
    set -e
    if [[ "$code" == "$expect_val" ]]; then
      echo "PASS $name (exit=$code)"
      (( pass++ ))
    else
      echo "FAIL $name (exit=$code expected=$expect_val)"
      (( fail++ ))
    fi
  elif [[ "$expect_type" == "COMPILE-FAIL" ]]; then
    if ./ccomp -o "$s" "$c" > "$tmpdir/$name.log" 2>&1; then
      echo "FAIL $name (expected COMPILE-FAIL, compiled successfully)"
      (( fail++ ))
    else
      echo "PASS $name (compile-fail)"
      (( pass++ ))
    fi
  else
    echo "Unknown expectation on $name: $first"
    (( fail++ ))
  fi
done

echo
echo "Summary: $pass passed, $fail failed, $total total"
[[ $fail -eq 0 ]]
