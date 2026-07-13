# Mutation Testing Configuration

## Backend (Go) - go-mutesting

### Installation
```bash
go install github.com/avito-tech/go-mutesting/cmd/go-mutesting@latest
```

### Running Mutation Tests
```bash
# Run on all packages
go-mutesting ./internal/...

# Run on specific package
go-mutesting ./internal/torrent/...

# With timeout
go-mutesting -timeout=10m ./internal/...
```

### Configuration (.mutesting.toml)
```toml
# Mutation testing configuration for TorrSyncPlayer backend

# Mutation operators to enable
mutators = [
  "arithmetic",
  "logical",
  "comparison",
  "constant",
  "remove",
  "replace",
  "negate",
]

# Exclude test files
exclude = [
  "*_test.go",
  "*_mock.go",
  "vendor/*",
]

# Minimum mutation score (percentage)
# CI will fail if score is below this threshold
threshold = 80.0

# Timeout per test
timeout = "30s"

# Number of parallel workers
workers = 4

# Output format: text, json, html
output = "text"

# Packages to test
packages = [
  "github.com/blagovibe/TorrSyncPlayer/backend/internal/torrent",
  "github.com/blagovibe/TorrSyncPlayer/backend/internal/validation",
  "github.com/blagovibe/TorrSyncPlayer/backend/internal/auth",
  "github.com/blagovibe/TorrSyncPlayer/backend/internal/p2p",
  "github.com/blagovibe/TorrSyncPlayer/backend/internal/sync",
  "github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer",
]

# Patterns to ignore
ignore_patterns = [
  "func main",
  "func init",
  "// mutesting:ignore",
]
```

### CI Integration
```yaml
mutation-test:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version: '1.26'
    
    - name: Install go-mutesting
      run: go install github.com/avito-tech/go-mutesting/cmd/go-mutesting@latest
    
    - name: Run mutation tests
      run: go-mutesting -threshold=80 ./internal/...
      working-directory: backend
```

---

## Frontend (C++) - Mull

### Installation
```bash
# Ubuntu/Debian
sudo apt-get install llvm-15 clang-15

# Or build from source
git clone https://github.com/mull-project/mull
cd mull && mkdir build && cd build
cmake .. -DCMAKE_BUILD_TYPE=Release
make -j$(nproc)
sudo make install
```

### Running Mutation Tests
```bash
# Build with debug info and no optimization
cmake -DCMAKE_BUILD_TYPE=Debug -DCMAKE_CXX_FLAGS="-g -O0" ..
make -j$(nproc)

# Run mull
mull-cxx \
  --test-framework=GoogleTest \
  --workers=4 \
  --timeout=60000 \
  --mutation-score-threshold=70 \
  ./build/test_networkmanager_gmock \
  ./build/test_torrentmanager_gmock \
  ./build/test_roommanager_gmock
```

### Configuration (mull.yml)
```yaml
# Mull mutation testing configuration for TorrSyncPlayer frontend

test_framework: GoogleTest
workers: 4
timeout: 60000  # milliseconds
mutation_score_threshold: 70  # percentage

# Compilation database path
compile_commands: build/compile_commands.json

# Test executables
executables:
  - build/test_networkmanager_gmock
  - build/test_torrentmanager_gmock
  - build/test_roommanager_gmock

# Mutators to use
mutators:
  - cxx_arithmetic
  - cxx_logical
  - cxx_comparison
  - cxx_constant
  - cxx_replace
  - cxx_remove
  - cxx_negate

# Exclude patterns
exclude:
  - "*/tests/*"
  - "*/mocks/*"
  - "*/interfaces/*"
  - "*_test.cpp"

# Include patterns
include:
  - "src/networkmanager.cpp"
  - "src/torrentmanager.cpp"
  - "src/roommanager.cpp"
  - "src/torrentmodel.cpp"
  - "src/utils.cpp"

# Reporters
reporters:
  - IDE
  - SQLite
  - HTML
```

### CI Integration
```yaml
mutation-test-frontend:
  runs-on: ubuntu-latest
  timeout-minutes: 60
  steps:
    - uses: actions/checkout@v4
    
    - name: Install LLVM/Clang
      run: |
        sudo apt-get update
        sudo apt-get install -y llvm-15 clang-15
    
    - name: Install Mull
      run: |
        # Install mull from release or build
        wget https://github.com/mull-project/mull/releases/download/v0.12.0/Mull-0.12.0-Linux.tar.gz
        tar xzf Mull-0.12.0-Linux.tar.gz
        sudo cp Mull-0.12.0-Linux/bin/* /usr/local/bin/
    
    - name: Build frontend with debug info
      working-directory: frontend
      run: |
        mkdir -p build
        cd build
        cmake .. -DCMAKE_BUILD_TYPE=Debug -DBUILD_TESTS=ON -DBUILD_GMOCK=ON
        make -j$(nproc)
    
    - name: Run mutation tests
      working-directory: frontend
      run: |
        mull-cxx \
          --test-framework=GoogleTest \
          --workers=4 \
          --timeout=60000 \
          --mutation-score-threshold=70 \
          ./build/test_networkmanager_gmock \
          ./build/test_torrentmanager_gmock \
          ./build/test_roommanager_gmock
```

---

## Mutation Testing Best Practices

### For Go
1. **Run mutation tests on CI** but allow failures initially
2. **Start with threshold 60%**, gradually increase to 80%+
3. **Exclude generated code** and trivial functions
4. **Focus on business logic packages** first

### For C++
1. **Build with -O0 -g** for best results
2. **Use compile_commands.json** for accurate parsing
3. **Exclude test/mock files** from mutation
4. **Run on subset of tests** first for speed

### Common Patterns to Detect
- **Off-by-one errors** in loops/boundaries
- **Null pointer dereferences**
- **Incorrect comparison operators** (>, <, >=, <=)
- **Missing error handling**
- **Incorrect arithmetic operations**
- **Logical operator bugs** (&& vs ||)

---

## Reports and Metrics

### Go Mutation Report Example
```
Mutation Testing Results
========================
Packages tested: 8
Mutants generated: 1,247
Mutants killed: 1,023
Mutants survived: 224
Mutation Score: 82.0%

Survived by package:
  internal/torrent: 45 survived (78%)
  internal/validation: 32 survived (81%)
  internal/auth: 18 survived (85%)
  internal/p2p: 56 survived (75%)
  internal/sync: 41 survived (79%)
  internal/buffer: 22 survived (83%)
  internal/errors: 8 survived (90%)
  pkg/logger: 2 survived (95%)

Top surviving mutant types:
  1. Constant replacement: 67
  2. Comparison operator: 54
  3. Arithmetic operator: 43
  4. Logical operator: 32
  5. Remove statement: 28
```

### C++ Mutation Report Example
```
Mull Mutation Testing Results
=============================
Test executables: 3
Total mutants: 892
Killed: 624
Survived: 268
Mutation Score: 69.9%

By executable:
  test_networkmanager_gmock: 72%
  test_torrentmanager_gmock: 68%
  test_roommanager_gmock: 71%

Survived mutants by type:
  cxx_comparison: 89
  cxx_arithmetic: 76
  cxx_logical: 54
  cxx_constant: 32
  cxx_remove: 17
```

---

## Improving Mutation Scores

1. **Add tests for uncovered branches**
2. **Test error paths** explicitly
3. **Test boundary conditions** (0, -1, max values)
4. **Test with invalid inputs**
5. **Verify side effects** (not just return values)
6. **Add property-based tests** for complex logic