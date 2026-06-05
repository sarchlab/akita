# Memory Tracer with SQLite Database Support

This package provides both logger-based and database-based memory tracing capabilities for the Akita simulation framework.

## Overview

The memory tracer captures detailed information about memory system operations, including:
- Memory transaction start/end times
- Memory addresses and access sizes
- Transaction steps and milestones
- Location information (cache levels, memory controllers, etc.)

## Database Schema

### memory_transactions Table
- `ID` (unique): Unique transaction identifier
- `Location` (indexed): Where the transaction occurred (e.g., "L1_cache", "memory_controller")
- `What` (indexed): Type of operation (e.g., "read", "write")
- `StartTime` (indexed): Transaction start time in simulation time units
- `EndTime` (indexed): Transaction end time in simulation time units
- `Address` (indexed): Memory address being accessed
- `ByteSize` (indexed): Size of the memory access in bytes

### memory_steps Table
- `ID` (unique): Unique step identifier
- `TaskID` (indexed): Reference to the parent transaction ID
- `Time` (indexed): When the step occurred
- `What` (indexed): Type of step (e.g., "cache_miss", "cache_hit")

## Usage

### Database-Based Tracer (New)

```go
import (
    "github.com/sarchlab/akita/v5/datarecording"
    "github.com/sarchlab/akita/v5/mem/trace"
)

// Create a data recorder
dataRecorder := datarecording.NewDataRecorder("memory_trace")

// Create the tracer
memTracer := trace.NewDBTracer(dataRecorder, timeTeller)

// Use the tracer in your simulation
tracing.CollectTrace(memoryComponent, memTracer)
```

## Benefits of Database-Based Tracing

1. **Structured Data**: Data is stored in a structured SQLite database with proper indexing
2. **Queryable**: Use SQL queries to analyze memory access patterns
3. **Performant**: Batch processing and indexed access for large-scale simulations
4. **Standardized**: Consistent with other Akita tracing infrastructure
5. **Tools Integration**: Compatible with existing data analysis tools

## Querying Trace Data

After simulation, you can analyze the SQLite database using standard SQL:

```sql
-- Find all cache misses
SELECT * FROM memory_steps WHERE What = 'cache_miss';

-- Analyze memory access patterns by address range
SELECT Address, COUNT(*) as AccessCount 
FROM memory_transactions 
WHERE Address BETWEEN 0x1000 AND 0x2000 
GROUP BY Address;

-- Calculate average transaction duration
SELECT AVG(EndTime - StartTime) as AvgDuration 
FROM memory_transactions 
WHERE EndTime > 0;
```

## Compatibility

The database-based tracer is fully compatible with existing Akita simulations.

## Performance Considerations

- The database tracer uses batch processing (default batch size: 100,000 entries)
- Data is automatically flushed when the batch size is reached or when `Flush()` is called
- For large simulations, consider adjusting the batch size through the data recorder configuration