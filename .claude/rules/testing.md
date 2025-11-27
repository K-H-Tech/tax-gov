# Testing Standards

## Test File Organization

### Naming Conventions
```
package/
├── handler.go          # Implementation
├── handler_test.go     # Unit tests
├── handler_integration_test.go  # Integration tests (optional)
└── testdata/           # Test fixtures
    └── fixtures.json
```

### Build Tags for Integration Tests
```go
//go:build integration

package mypackage_test
```

Run with: `go test -tags=integration ./...`

## Table-Driven Tests

### Standard Pattern
```go
func TestParse(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    Result
        wantErr bool
    }{
        {
            name:  "valid input",
            input: "abc",
            want:  Result{Value: "abc"},
        },
        {
            name:    "empty input",
            input:   "",
            wantErr: true,
        },
        {
            name:  "unicode input",
            input: "مرحبا",
            want:  Result{Value: "مرحبا"},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel() // Run subtests in parallel

            got, err := Parse(tt.input)

            if (err != nil) != tt.wantErr {
                t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
                t.Errorf("Parse() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Test Helpers

### Use t.Helper()
```go
func assertNoError(t *testing.T, err error) {
    t.Helper() // Marks this as helper - errors report caller's line
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}

func assertEqual[T comparable](t *testing.T, got, want T) {
    t.Helper()
    if got != want {
        t.Errorf("got %v, want %v", got, want)
    }
}
```

### Use t.Cleanup()
```go
func TestWithTempFile(t *testing.T) {
    f, err := os.CreateTemp("", "test")
    assertNoError(t, err)

    t.Cleanup(func() {
        os.Remove(f.Name())
    })

    // test code using f
}
```

## Coverage Requirements

### Minimum Coverage
- Critical paths: 80%+
- New code: should not decrease overall coverage
- Focus on behavior, not line coverage

### Running Coverage
```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...

# View in browser
go tool cover -html=coverage.out

# Check coverage percentage
go tool cover -func=coverage.out
```

## Mocking and Stubs

### Interface-Based Mocking
```go
// Define interface for dependencies
type UserStore interface {
    GetUser(id int) (*User, error)
}

// Production implementation
type PostgresUserStore struct { ... }

// Test mock
type mockUserStore struct {
    users map[int]*User
    err   error
}

func (m *mockUserStore) GetUser(id int) (*User, error) {
    if m.err != nil {
        return nil, m.err
    }
    return m.users[id], nil
}

func TestHandler(t *testing.T) {
    store := &mockUserStore{
        users: map[int]*User{1: {Name: "Alice"}},
    }
    handler := NewHandler(store)
    // test handler
}
```

### When to Mock
- External services (APIs, databases)
- Time-dependent code (`time.Now`)
- Random generators
- File system operations (when practical)

### When NOT to Mock
- Your own code (test the real thing)
- Simple data structures
- Pure functions

## HTTP Handler Testing

```go
func TestHandler(t *testing.T) {
    handler := NewHandler()

    req := httptest.NewRequest("GET", "/users/1", nil)
    rec := httptest.NewRecorder()

    handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
    }

    var got User
    json.NewDecoder(rec.Body).Decode(&got)
    // assert got
}
```

## Benchmarks

### When to Benchmark
- Hot paths (called frequently)
- Memory-sensitive operations
- Before/after optimization

### Benchmark Pattern
```go
func BenchmarkParse(b *testing.B) {
    input := "test input"

    b.ResetTimer() // Reset after setup
    for i := 0; i < b.N; i++ {
        Parse(input)
    }
}

func BenchmarkParseAllocs(b *testing.B) {
    input := "test input"

    b.ReportAllocs() // Report allocations
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        Parse(input)
    }
}
```

### Running Benchmarks
```bash
go test -bench=. -benchmem ./...
go test -bench=BenchmarkParse -count=5 ./...
```

## Race Detection

### Always Run with Race Detector in CI
```bash
go test -race ./...
```

### Test Concurrent Code
```go
func TestConcurrentAccess(t *testing.T) {
    cache := NewCache()

    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            cache.Set(fmt.Sprintf("key%d", i), i)
            cache.Get(fmt.Sprintf("key%d", i))
        }(i)
    }
    wg.Wait()
}
```

## Test Data

### Use testdata Directory
```go
func TestParseConfig(t *testing.T) {
    data, err := os.ReadFile("testdata/config.json")
    assertNoError(t, err)

    config, err := ParseConfig(data)
    // assertions
}
```

### Golden Files
```go
func TestOutput(t *testing.T) {
    got := GenerateOutput()

    golden := filepath.Join("testdata", t.Name()+".golden")

    if *update {
        os.WriteFile(golden, got, 0644)
    }

    want, _ := os.ReadFile(golden)
    if !bytes.Equal(got, want) {
        t.Errorf("output mismatch")
    }
}
```
