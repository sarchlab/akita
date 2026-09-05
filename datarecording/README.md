# datarecording

Package `datarecording` provides a structured data recording and reading
infrastructure backed by SQLite. It is used by the tracing, monitoring, and
simulation packages to persist simulation results.

## DataRecorder

The `DataRecorder` interface writes structured data into SQLite tables:

```go
type DataRecorder interface {
    CreateTable(tableName string, sampleEntry any) error
    InsertData(tableName string, entry any) error
    ListTables() []string
    Flush() error
    Close() error
}
```

### Creating a Recorder

```go
recorder, err := datarecording.NewDataRecorder("my_simulation")
if err != nil {
    return err
}
defer recorder.Close() // fallback cleanup; check Close explicitly on success
```

This creates a SQLite file `my_simulation.sqlite3`. Simulation-level metadata
such as `exec_info` is recorded by the `simulation` package, not by the generic
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

if err := recorder.CreateTable("my_results", MyEntry{}); err != nil {
    return err
}
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
if err := recorder.InsertData("my_results", MyEntry{
    ID:       1,
    Category: "latency",
    Value:    42.5,
    Location: "Cache.L1",
}); err != nil {
    return err
}
```

Data is batched internally (default 100,000 entries) and flushed automatically
or via `Flush()`.

## DataReader

The `DataReader` interface reads from SQLite databases:

```go
reader, err := datarecording.NewReader("my_simulation.sqlite3")
if err != nil {
    return err
}
defer reader.Close()

reader.MapTable("my_results", MyEntry{})

results, totalCount, err := reader.Query(ctx, "my_results", datarecording.QueryParams{
    Where:   "Category = ?",
    Args:    []any{"latency"},
    OrderBy: "Value DESC",
    Limit:   100,
    Offset:  0,
})
```

### QueryParams

| Field | Description |
|-------|-------------|
| `Where` | SQL WHERE clause (without `WHERE` keyword) |
| `Args` | Placeholder arguments for the WHERE clause |
| `Limit` | Max rows to return (0 = unlimited) |
| `Offset` | Number of rows to skip |
| `OrderBy` | SQL ORDER BY clause (without `ORDER BY` keyword) |

Results are returned as `[]any` where each element is a pointer to the
mapped struct type.

## Recording failures

Creation refuses an existing output file. Check errors from every write and
from `Close`; a failed final flush or index build invalidates requested output.
A failed flush retains its batch for retry. `Close` releases the database even
on failure and cannot be retried. There is no process-wide exit handler.

Inside managed simulation callbacks, `MustRecord(err)` turns an I/O error into
a contained simulation failure. Standalone clients handle the error directly.
The simulation's `Terminate` publishes a `.complete` marker after successful
recording and close; see [output validity](../simulation/ERROR_HANDLING.md).
