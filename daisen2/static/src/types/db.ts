// Types for the Database widget, mirroring the Go structs served by
// /api/db_info and /api/db_activity.

/** On-disk size of one index. */
export interface DBIndexInfo {
  name: string;
  bytes: number;
}

/** One table: row count plus the bytes its b-tree and its indexes occupy. */
export interface DBTableInfo {
  name: string;
  rows: number;
  data_bytes: number;
  index_bytes: number;
  indexes: DBIndexInfo[];
}

/** Schema-and-size overview of the loaded trace database. `has_sizes` is false
 *  when the SQLite build lacks dbstat, in which case the byte fields are 0. */
export interface DBInfo {
  file: string;
  file_bytes: number;
  total_rows: number;
  data_bytes: number;
  index_bytes: number;
  has_sizes: boolean;
  tables: DBTableInfo[];
}

/** Envelope for /api/db_info: while the (cached) dbstat scan runs in the
 *  background, `computing` is true and `info` may be null or the prior result. */
export interface DBInfoResponse {
  computing: boolean;
  info: DBInfo | null;
}

/** One in-flight database operation (index build, heavy query, dbstat scan). */
export interface DBActivity {
  id: number;
  op: string; // "index" | "query" | "info"
  name: string;
  detail: string;
  elapsed_seconds: number;
}
