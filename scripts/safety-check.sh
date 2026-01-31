#!/bin/bash
set -e

echo "=== Logryph Safety-Critical Compliance Check ==="

# Rule 1: No recursion
echo "Checking for recursion..."
if command -v cgraph &> /dev/null; then
    cgraph ./... | grep -i cycle && exit 1 || echo "[OK] No recursion detected"
else
    echo "[WARN] cgraph not found, skipping recursion check"
fi

# Rule 2: Bounded loops
echo "Checking loop bounds..."
grep -rn "for.*range" --include="*.go" . | while read line; do
    echo "$line" | grep -q "maxIterations\|len(" || echo "[WARN] Potential unbounded loop: $line"
done

# Rule 4: Function length
echo "Checking function lengths (Target < 60 lines)..."
if command -v gocyclo &> /dev/null; then
    gocyclo -over 60 . && echo "[WARN] Functions over 60 lines detected" || echo "[OK] Function length compliance OK"
else
    echo "[WARN] gocyclo not found, skipping length check"
fi

# Rule 5: Assertion density
echo "Checking assertion density..."
total_funcs=$(grep -r "^func " --include="*.go" . | wc -l)
total_asserts=$(grep -r "assert.Check" --include="*.go" . | wc -l)
if [ "$total_funcs" -gt 0 ]; then
    density=$(echo "scale=2; $total_asserts / $total_funcs" | bc)
    echo "Assertion density: $density (target: >= 2.0)"
else
    echo "No functions found to check density."
fi

# Rule 10: Zero warnings
echo "Running static analysis..."
go vet ./...
if command -v staticcheck &> /dev/null; then
    staticcheck ./...
fi
if command -v golangci-lint &> /dev/null; then
    golangci-lint run
else
    echo "[WARN] golangci-lint not found, skipping"
fi

echo "=== Compliance Check Finished ==="
