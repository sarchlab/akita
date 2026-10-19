# datarecording

Package `datarecording` provides a structured data recording
infrastructure backed by SQLite. It is used by the tracing, monitoring, and
simulation packages to persist simulation results.

## DataRecorder

The `DataRecorder` interface writes structured data into SQLite tables:

```go
type DataRecorder interface {
    CreateTable(tableName string, sampleEntry any)
    InsertData(tableName string, entry any)
    Flush()
    Close() error
}
```

### Creating a Recorder

```go
recorder := datarecording.NewDataRecorder("my_simulation.sqlite3")
defer recorder.Close()
```

The argument is the exact path: no extension is appended. Existing files are
rejected without modification. An empty path chooses a unique SQLite filename.
Simulation-level metadata
such as `exec_info` is recorded by the `sim` package, not by the generic
data recorder.

### Defining Tables

Tables are defined by passing a sample struct. Fields become columns;
struct tags control indexing:

```go
type MyEntry struct {
    ID        uint64  `akita_data:"unique"`
    Category  string  `akita_data:"index"`
    Value     float64
    Ignore    string  `akita_data:"ignore"`
    Location  string  `akita_data:"location"` // auto-mapped to int IDs
}

recorder.CreateTable("my_results", MyEntry{})
```

#### Struct Tags

| Tag | Effect |
|-----|--------|
| `akita_data:"unique"` | Creates a unique index on this column |
| `akita_data:"index"` | Creates a non-unique index |
| `akita_data:"ignore"` | Field is not stored |
| `akita_data:"location"` | String auto-mapped to integer ID via a `location` table |

Only primitive types are allowed as fields (bool, int/uint variants, float,
complex, string).

### Inserting Data

```go
recorder.InsertData("my_results", MyEntry{
    ID:       1,
    Category: "latency",
    Value:    42.5,
    Location: "Cache.L1",
})
```

Data is batched internally (default 100,000 entries) and flushed automatically
or via `Flush()`.

## Reading recordings

Use SQLite directly for custom analysis. The generic reader and existing-DB
writer constructor have been removed. Daisen retains its specialized trace
reader as an internal implementation detail.

Fields tagged `akita_data:"location"` store IDs into the `location` table;
join the recorded column to `location.ID` to retrieve `location.Locale`.
