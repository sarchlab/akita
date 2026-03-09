# Akita Performance Analysis Toolkit

Akita Performance Analysis Toolkit provides a set of tools for analyzing the performance of Akita-based simulators. The library mainly uses the hooking feature` of Akita to collect the performance metrics during simulation. The metrics will be recorded and stored in a file. 

## `PerfAnalyzer`

The `PerfAnalyzer` is the facade of the performance analysis toolkit. Once a `PerfAnalyzer` is created, simulator developers can register components by calling the `RegisterComponent` method. 

Currently, the `PerfAnalyzer` automatically discovers the ports and buffers used by the components. It will attach hooks to the ports and buffers to collect throughput and buffer level information, respectively.

It is optional the report values in periods. The throughput and buffer level information will be reported periodically. Each report is the metrics collected in the last period. The period is specified by the `WithPeriod` method of the `PerfAnalyzerBuilder`. 

## Output

The output can either be stored a csv file or a sqlite database file. By default, the data is stored in a CSV file. To store the data in a sqlite database, call the `WithSQLiteBackend` method of the `PerfAnalyzerBuilder`. No matter the CSV backend of the SQLite backend is selected, the output file name can be specified by the `WithDBFilename` method of the `PerfAnalyzerBuilder`. Existing files will be overwritten.

The output format is the same for the CSV and SQLite backend. It is designed to be generic so that any type of data can be recorded. The output is organized in a table that includes the following columns:

- `start`: the start time of the period
- `end`: the end time of the period
- `where`: the name of the element that the data is collected from. This field is named as `where_` in SQLite to avoid conflict with the `where` keyword.
- `what`: the name of the metric
- `value`: the value of the metric
- `unit`: the unit of the metric

